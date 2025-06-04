package bencode

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strconv"
	"strings"
	"unicode"
)

// Error represents an error that occurred during bencode processing.
type Error struct {
	// Type categorizes the error.
	Type ErrorType
	// Msg provides a human-readable description of the error.
	Msg string
	// FieldName is relevant for struct field errors or map key errors.
	FieldName string
	// WrappedErr holds the underlying error, if any.
	WrappedErr error
}

func (e *Error) Error() string {
	var sb strings.Builder
	sb.WriteString("bencode: ")
	if e.FieldName != "" {
		sb.WriteString(fmt.Sprintf("field %q: ", e.FieldName))
	}
	sb.WriteString(e.Msg)
	if e.WrappedErr != nil {
		sb.WriteString(": ")
		sb.WriteString(e.WrappedErr.Error())
	}
	return sb.String()
}

func (e *Error) Unwrap() error {
	return e.WrappedErr
}

// ErrorType defines the category of a bencode error.
type ErrorType string

const (
	// ErrSyntax indicates an error in the bencode syntax.
	ErrSyntax ErrorType = "syntax error"
	// ErrSyntaxInteger indicates an invalid integer format.
	ErrSyntaxInteger ErrorType = "integer syntax error"
	// ErrSyntaxStringLength indicates an invalid string length format.
	ErrSyntaxStringLength ErrorType = "string length syntax error"
	// ErrSyntaxUnexpectedToken indicates an unexpected token in the input.
	ErrSyntaxUnexpectedToken ErrorType = "unexpected token"
	// ErrSyntaxEOF indicates an unexpected end of input.
	ErrSyntaxEOF ErrorType = "unexpected EOF"

	// ErrStructureList indicates an error in list structure (e.g., not terminated).
	ErrStructureList ErrorType = "list structure error"
	// ErrStructureDict indicates an error in dictionary structure (e.g., not terminated, key not string).
	ErrStructureDict ErrorType = "dictionary structure error"
	// ErrStructureDictKeySort indicates dictionary keys are not sorted lexicographically.
	ErrStructureDictKeySort ErrorType = "dictionary key sort order error"
	// ErrStructureDictKeyDup indicates a duplicate key in a dictionary.
	ErrStructureDictKeyDup ErrorType = "duplicate dictionary key"
	// ErrStructureDictValue indicates a missing value for a dictionary key.
	ErrStructureDictValue ErrorType = "missing dictionary value"

	// ErrUnmarshalType indicates a mismatch between bencode type and Go type during unmarshaling.
	ErrUnmarshalType ErrorType = "unmarshal type mismatch"
	// ErrUnmarshalOverflow indicates a numeric value overflows the target Go type.
	ErrUnmarshalOverflow ErrorType = "unmarshal overflow"
	// ErrUnmarshalToNil indicates an attempt to unmarshal to a Go nil pointer or assign nil to a non-nillable type.
	ErrUnmarshalToNil ErrorType = "unmarshal to nil/non-nillable"
	// ErrUnmarshalToInvalid indicates the target Go value for unmarshaling is invalid (e.g., not a pointer, unsettable).
	ErrUnmarshalToInvalid ErrorType = "unmarshal to invalid Go type"
	// ErrUnmarshalMapKey indicates the Go map's key type is not string.
	ErrUnmarshalMapKey ErrorType = "unmarshal map key type error"

	// ErrUsage indicates incorrect usage of the bencode API.
	ErrUsage ErrorType = "API usage error"
	// ErrInternal indicates an internal decoder error.
	ErrInternal ErrorType = "internal decoder error"
)

// Sentinel errors for common, specific conditions.
var (
	ErrNullRootValue           = &Error{Type: ErrSyntax, Msg: "null root value"}
	ErrDuplicateDictionaryKey  = &Error{Type: ErrStructureDictKeyDup, Msg: "duplicate key in dictionary"}
	ErrDictionaryKeysNotSorted = &Error{Type: ErrStructureDictKeySort, Msg: "dictionary keys must be sorted lexicographically"}
	ErrUnexpectedEOF           = &Error{Type: ErrSyntaxEOF, Msg: "unexpected end of input"}
)

func Unmarshal(data []byte, v any) error {
	dec := &Decoder{r: bufio.NewReaderSize(bytes.NewReader(data), len(data))}
	return dec.Decode(v)
}

type Decoder struct {
	r *bufio.Reader
}

func NewDecoder(r io.Reader) *Decoder {
	return &Decoder{r: bufio.NewReader(r)}
}

func (d *Decoder) Decode(v any) error {
	val := reflect.ValueOf(v)
	if val.Kind() != reflect.Ptr || val.IsNil() {
		return &Error{Type: ErrUsage, Msg: fmt.Sprintf("expected a non-nil pointer, got %T", v)}
	}

	elem := val.Elem()

	decoded, err := d.decode()
	if err != nil {
		return err
	}

	return d.assignDecodedToValue(elem, decoded)
}

// assignDecodedToValue populates 'destVal' with 'srcData'.
// 'destVal' is the reflect.Value of the target Go variable (e.g., struct, slice, int).
// 'srcData' is the data decoded by d.decode() (e.g., map[string]any, []any, int64, []byte).
func (d *Decoder) assignDecodedToValue(destVal reflect.Value, srcData any) error {
	if !destVal.IsValid() {
		return &Error{Type: ErrUnmarshalToInvalid, Msg: "destination value is invalid"}
	}
	if !destVal.CanSet() {
		return &Error{Type: ErrUnmarshalToInvalid, Msg: fmt.Sprintf("cannot set destination value of type %s", destVal.Type())}
	}

	if srcData == nil {
		switch destVal.Kind() {
		case reflect.Interface, reflect.Slice, reflect.Map, reflect.Ptr:
			if destVal.IsNil() || destVal.Type().Comparable() {
				destVal.Set(reflect.Zero(destVal.Type()))
				return nil
			}
			if reflect.DeepEqual(destVal.Interface(), reflect.Zero(destVal.Type()).Interface()) {
				return nil
			}
			return &Error{Type: ErrUnmarshalToNil, Msg: fmt.Sprintf("cannot set non-nil nillable type %s to nil from srcData", destVal.Type())}
		default:
			return &Error{Type: ErrUnmarshalToNil, Msg: fmt.Sprintf("cannot assign nil to non-nillable type %s", destVal.Type())}
		}
	}

	srcType := reflect.TypeOf(srcData)

	switch destVal.Kind() {
	case reflect.String:
		byteSlice, ok := srcData.([]byte)
		if !ok {
			return &Error{Type: ErrUnmarshalType, Msg: fmt.Sprintf("expected []byte for string destination, got %T", srcData)}
		}
		destVal.SetString(string(byteSlice))
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		intVal, ok := srcData.(int64)
		if !ok {
			return &Error{Type: ErrUnmarshalType, Msg: fmt.Sprintf("expected int64 for numeric type %s, got %T", destVal.Type(), srcData)}
		}
		if destVal.OverflowInt(intVal) {
			return &Error{Type: ErrUnmarshalOverflow, Msg: fmt.Sprintf("value %d overflows type %s", intVal, destVal.Type())}
		}
		destVal.SetInt(intVal)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		intVal, ok := srcData.(int64) // Bencode integers are signed
		if !ok {
			return &Error{Type: ErrUnmarshalType, Msg: fmt.Sprintf("expected int64 for numeric type %s, got %T", destVal.Type(), srcData)}
		}
		if intVal < 0 {
			return &Error{Type: ErrUnmarshalType, Msg: fmt.Sprintf("cannot assign negative value %d to unsigned type %s", intVal, destVal.Type())}
		}
		uintVal := uint64(intVal)
		if destVal.OverflowUint(uintVal) {
			return &Error{Type: ErrUnmarshalOverflow, Msg: fmt.Sprintf("value %d overflows type %s", uintVal, destVal.Type())}
		}
		destVal.SetUint(uintVal)
	case reflect.Slice:
		srcSlice, ok := srcData.([]any)
		if !ok {
			return &Error{Type: ErrUnmarshalType, Msg: fmt.Sprintf("expected []any for slice destination, got %T", srcData)}
		}
		sliceType := destVal.Type()
		elemType := sliceType.Elem()
		newSlice := reflect.MakeSlice(sliceType, len(srcSlice), len(srcSlice))
		for i, item := range srcSlice {
			sliceElemVal := reflect.New(elemType).Elem()
			if err := d.assignDecodedToValue(sliceElemVal, item); err != nil {
				// err is already *Error
				return &Error{
					Type:       err.(*Error).Type, // Propagate original error type
					Msg:        fmt.Sprintf("decoding slice element %d", i),
					WrappedErr: err,
					FieldName:  strconv.Itoa(i),
				}
			}
			newSlice.Index(i).Set(sliceElemVal)
		}
		destVal.Set(newSlice)
	case reflect.Map:
		if destVal.Type().Key().Kind() != reflect.String {
			return &Error{Type: ErrUnmarshalMapKey, Msg: fmt.Sprintf("map keys must be strings for destination type %s, got key type %s", destVal.Type(), destVal.Type().Key())}
		}
		srcMap, ok := srcData.(map[string]any)
		if !ok {
			return &Error{Type: ErrUnmarshalType, Msg: fmt.Sprintf("expected map[string]any for map destination, got %T", srcData)}
		}
		mapType := destVal.Type()
		elemType := mapType.Elem()
		newMap := reflect.MakeMap(mapType)
		for key, item := range srcMap {
			mapElemVal := reflect.New(elemType).Elem()
			if err := d.assignDecodedToValue(mapElemVal, item); err != nil {
				// err is already *Error
				return &Error{
					Type:       err.(*Error).Type,
					Msg:        fmt.Sprintf("decoding map value for key %q", key),
					WrappedErr: err,
					FieldName:  key,
				}
			}
			newMap.SetMapIndex(reflect.ValueOf(key), mapElemVal)
		}
		destVal.Set(newMap)
	case reflect.Struct:
		srcMap, ok := srcData.(map[string]any)
		if !ok {
			return &Error{Type: ErrUnmarshalType, Msg: fmt.Sprintf("expected map[string]any for struct destination %s, got %T", destVal.Type(), srcData)}
		}
		return d.populateStruct(destVal, srcMap)
	default:
		if !srcType.AssignableTo(destVal.Type()) {
			return &Error{Type: ErrUnmarshalType, Msg: fmt.Sprintf("unhandled destination type %s (source type %s)", destVal.Type(), srcType)}
		}
		destVal.Set(reflect.ValueOf(srcData))
	}
	return nil
}

// populateStruct populates the fields of 'structVal' using data from 'dictData'.
// 'structVal' is the reflect.Value of the struct to populate.
// 'dictData' is a map[string]any, typically from d.decode().
func (d *Decoder) populateStruct(structVal reflect.Value, dictData map[string]any) error {
	if structVal.Kind() != reflect.Struct {
		return &Error{Type: ErrInternal, Msg: fmt.Sprintf("populateStruct called with non-struct type %s", structVal.Type())}
	}

	typ := structVal.Type()
	for i := range typ.NumField() {
		fieldStructDef := typ.Field(i)
		fieldRuntimeVal := structVal.Field(i)

		// Skip unexported fields or fields we cannot set.
		if !fieldRuntimeVal.CanSet() {
			continue
		}

		tag := fieldStructDef.Tag.Get("bencode")
		if tag == "" {
			// Skip fields without a bencode tag
			continue
		}

		bencodeValue, exists := dictData[tag]
		if !exists {
			// Skip fields that are not present in the bencode data.
			continue
		}

		// Recursively call assignDecodedToValue for the field.
		if err := d.assignDecodedToValue(fieldRuntimeVal, bencodeValue); err != nil {
			// err is already *Error
			return &Error{
				Type:       err.(*Error).Type,
				Msg:        fmt.Sprintf("setting field %s (tag %q)", fieldStructDef.Name, tag),
				WrappedErr: err,
				FieldName:  tag, // Use tag as FieldName as it's the bencode key
			}
		}
	}
	return nil
}

func (d *Decoder) decode() (any, error) {
	next, err := d.r.Peek(1)
	if err != nil {
		if errors.Is(err, io.EOF) {
			return nil, ErrNullRootValue // End of stream before any token
		}
		return nil, &Error{Type: ErrSyntaxEOF, Msg: "failed to peek next token", WrappedErr: err}
	}
	token := rune(next[0])
	switch {
	case unicode.IsDigit(token):
		lengthString, err := d.r.ReadString(':')
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil, &Error{Type: ErrSyntaxEOF, Msg: "unterminated string length", WrappedErr: ErrUnexpectedEOF}
			}
			return nil, &Error{Type: ErrSyntaxStringLength, Msg: "error reading string length", WrappedErr: err}
		}
		length, convErr := strconv.Atoi(lengthString[:len(lengthString)-1])
		if convErr != nil {
			return nil, &Error{Type: ErrSyntaxStringLength, Msg: "invalid string length format", WrappedErr: convErr}
		}
		if length < 0 {
			return nil, &Error{Type: ErrSyntaxStringLength, Msg: fmt.Sprintf("negative string length: %d", length)}
		}
		data := make([]byte, length)
		n, readErr := io.ReadFull(d.r, data)
		if readErr != nil {
			// Use ErrUnexpectedEOF as the wrapped error for consistency if it's an EOF variant
			wrapped := readErr
			if errors.Is(readErr, io.EOF) || errors.Is(readErr, io.ErrUnexpectedEOF) {
				wrapped = ErrUnexpectedEOF
			}
			return nil, &Error{Type: ErrSyntaxEOF, Msg: fmt.Sprintf("expected %d bytes for string, got %d", length, n), WrappedErr: wrapped}
		}
		return data, nil

	case token == 'i':
		_, _ = d.r.Discard(1) // discard 'i'
		numString, err := d.r.ReadString('e')
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil, &Error{Type: ErrSyntaxEOF, Msg: "integer not terminated by 'e'", WrappedErr: ErrUnexpectedEOF}
			}
			return nil, &Error{Type: ErrSyntaxInteger, Msg: "error reading integer", WrappedErr: err}
		}
		numString = numString[:len(numString)-1] // remove trailing 'e'
		if len(numString) == 0 {
			return nil, &Error{Type: ErrSyntaxInteger, Msg: "empty integer"}
		}

		if (len(numString) > 1 && numString[0] == '0') || (len(numString) > 2 && numString[0] == '-' && numString[1] == '0') {
			return nil, &Error{Type: ErrSyntaxInteger, Msg: fmt.Sprintf("invalid integer format (leading zero): %s", numString)}
		}
		if numString == "-0" { // "-0" is invalid
			return nil, &Error{Type: ErrSyntaxInteger, Msg: "invalid integer format: -0"}
		}

		num, convErr := strconv.ParseInt(numString, 10, 64)
		if convErr != nil {
			return nil, &Error{Type: ErrSyntaxInteger, Msg: fmt.Sprintf("cannot parse integer %q", numString), WrappedErr: convErr}
		}
		return num, nil

	case token == 'l':
		_, _ = d.r.Discard(1) // discard 'l'
		var list []any
		for {
			peeked, err := d.r.Peek(1)
			if err != nil {
				if errors.Is(err, io.EOF) {
					return nil, &Error{Type: ErrSyntaxEOF, Msg: "list not terminated by 'e'", WrappedErr: ErrUnexpectedEOF}
				}
				return nil, &Error{Type: ErrSyntax, Msg: "peeking in list", WrappedErr: err}
			}

			if rune(peeked[0]) == 'e' {
				if _, err = d.r.Discard(1); err != nil { // Consume 'e'
					return nil, &Error{Type: ErrSyntax, Msg: "consuming list terminator 'e'", WrappedErr: err}
				}
				break // End of list
			}

			item, decodeErr := d.decode()
			if decodeErr != nil {
				// If d.decode() returned ErrNullRootValue, it means EOF was hit where an item was expected.
				if errors.Is(decodeErr, ErrNullRootValue) {
					return nil, &Error{Type: ErrSyntaxEOF, Msg: "unexpected end of list, expected item or 'e'", WrappedErr: ErrUnexpectedEOF}
				}
				// decodeErr is already *Error
				return nil, decodeErr
			}
			list = append(list, item)
		}
		return list, nil

	case token == 'd':
		_, _ = d.r.Discard(1) // discard 'd'
		dict := make(map[string]any)
		var prevKey string
		firstKey := true

		for {
			peeked, err := d.r.Peek(1)
			if err != nil {
				if errors.Is(err, io.EOF) {
					return nil, &Error{Type: ErrSyntaxEOF, Msg: "dictionary not terminated by 'e'", WrappedErr: ErrUnexpectedEOF}
				}
				return nil, &Error{Type: ErrSyntax, Msg: "peeking in dictionary", WrappedErr: err}
			}

			if rune(peeked[0]) == 'e' {
				if _, err = d.r.Discard(1); err != nil { // Consume 'e'
					return nil, &Error{Type: ErrSyntax, Msg: "consuming dictionary terminator 'e'", WrappedErr: err}
				}
				break // End of dictionary
			}

			keyVal, keyErr := d.decode()
			if keyErr != nil {
				if errors.Is(keyErr, ErrNullRootValue) {
					return nil, &Error{Type: ErrSyntaxEOF, Msg: "unexpected end of dictionary, expected key or 'e'", WrappedErr: ErrUnexpectedEOF}
				}
				return nil, keyErr // keyErr is *Error
			}
			byteKey, ok := keyVal.([]byte)
			if !ok {
				return nil, &Error{Type: ErrStructureDict, Msg: fmt.Sprintf("dictionary key type %T is not a bencode string", keyVal)}
			}
			strKey := string(byteKey)

			if _, exists := dict[strKey]; exists {
				return nil, &Error{Type: ErrStructureDictKeyDup, Msg: fmt.Sprintf("key %q", strKey), WrappedErr: ErrDuplicateDictionaryKey, FieldName: strKey}
			}

			if !firstKey && prevKey >= strKey {
				return nil, &Error{Type: ErrStructureDictKeySort, Msg: fmt.Sprintf("key %q is not lexicographically after %q", strKey, prevKey), WrappedErr: ErrDictionaryKeysNotSorted, FieldName: strKey}
			}

			value, valErr := d.decode()
			if valErr != nil {
				if errors.Is(valErr, ErrNullRootValue) {
					return nil, &Error{Type: ErrStructureDictValue, Msg: "missing value (unexpected EOF)", WrappedErr: ErrUnexpectedEOF, FieldName: strKey}
				}
				// valErr is *Error, wrap it to add FieldName context
				return nil, &Error{Type: valErr.(*Error).Type, Msg: "decoding value", WrappedErr: valErr, FieldName: strKey}
			}
			dict[strKey] = value
			prevKey = strKey
			firstKey = false
		}
		return dict, nil
	default:
		return nil, &Error{Type: ErrSyntaxUnexpectedToken, Msg: fmt.Sprintf("unexpected token %q", token)}
	}
}
