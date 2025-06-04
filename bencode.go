package bencode

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"reflect"
	"strconv"
	"strings"
	"unicode"
)

var (
	ErrNullRootValue           = fmt.Errorf("bencode: null root value")
	ErrInvalidType             = fmt.Errorf("bencode: invalid type")
	ErrInvalidInteger          = fmt.Errorf("bencode: invalid integer format")
	ErrInvalidDictionaryKey    = fmt.Errorf("bencode: invalid dictionary key type, expected string")
	ErrDuplicateDictionaryKey  = fmt.Errorf("bencode: duplicate key in dictionary")
	ErrMissingDictionaryValue  = fmt.Errorf("bencode: missing value for dictionary key")
	ErrDictionaryKeysNotSorted = fmt.Errorf("bencode: dictionary keys must be sorted in lexicographical order")
	ErrUnexpectedEOF           = fmt.Errorf("bencode: unexpected EOF")
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
		return fmt.Errorf("bencode: expected a non-nil pointer, got %T", v)
	}

	elem := val.Elem()

	decoded, err := d.decode()
	if err != nil {
		return fmt.Errorf("bencode: error decoding bencode data: %w", err)
	}

	return d.assignDecodedToValue(elem, decoded)
}

// assignDecodedToValue populates 'destVal' with 'srcData'.
// 'destVal' is the reflect.Value of the target Go variable (e.g., struct, slice, int).
// 'srcData' is the data decoded by d.decode() (e.g., map[string]any, []any, int64, []byte).
func (d *Decoder) assignDecodedToValue(destVal reflect.Value, srcData any) error {
	if !destVal.IsValid() {
		return fmt.Errorf("bencode: destination value is invalid")
	}
	if !destVal.CanSet() {
		return fmt.Errorf("bencode: cannot set destination value of type %s", destVal.Type())
	}

	// Handle nil source data: if destVal is a pointer, slice, map, or interface, it can be set to nil.
	// Otherwise, trying to convert nil to a non-nillable type (like struct, int, string) is an error.
	if srcData == nil {
		switch destVal.Kind() {
		case reflect.Interface, reflect.Slice, reflect.Map, reflect.Ptr:
			// Only set to nil if the type is actually nillable and destVal is not already nil (to avoid panic on Zero value)
			if destVal.IsNil() || destVal.Type().Comparable() { // Check if it's safe to set to Zero
				destVal.Set(reflect.Zero(destVal.Type()))
				return nil
			}
			// If it's already a zero value of a nillable type, that's fine.
			if reflect.DeepEqual(destVal.Interface(), reflect.Zero(destVal.Type()).Interface()) {
				return nil
			}
			return fmt.Errorf("bencode: cannot set non-nil nillable type %s to nil from srcData", destVal.Type())
		default:
			return fmt.Errorf("bencode: cannot assign nil to non-nillable type %s", destVal.Type())
		}
	}

	srcType := reflect.TypeOf(srcData)

	switch destVal.Kind() {
	case reflect.String:
		byteSlice, ok := srcData.([]byte)
		if !ok {
			return fmt.Errorf("bencode: expected []byte for string destination, got %T", srcData)
		}
		destVal.SetString(string(byteSlice))
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		intVal, ok := srcData.(int64)
		if !ok {
			return fmt.Errorf("bencode: expected int64 for numeric type %s, got %T", destVal.Type(), srcData)
		}
		if destVal.OverflowInt(intVal) {
			return fmt.Errorf("bencode: value %d overflows type %s", intVal, destVal.Type())
		}
		destVal.SetInt(intVal)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		intVal, ok := srcData.(int64) // Bencode integers are signed
		if !ok {
			return fmt.Errorf("bencode: expected int64 for numeric type %s, got %T", destVal.Type(), srcData)
		}
		if intVal < 0 {
			return fmt.Errorf("bencode: cannot assign negative value %d to unsigned type %s", intVal, destVal.Type())
		}
		uintVal := uint64(intVal)
		if destVal.OverflowUint(uintVal) {
			return fmt.Errorf("bencode: value %d overflows type %s", uintVal, destVal.Type())
		}
		destVal.SetUint(uintVal)
	case reflect.Slice:
		srcSlice, ok := srcData.([]any)
		if !ok {
			return fmt.Errorf("bencode: expected []any for slice destination, got %T", srcData)
		}
		sliceType := destVal.Type()
		elemType := sliceType.Elem()
		newSlice := reflect.MakeSlice(sliceType, len(srcSlice), len(srcSlice))
		for i, item := range srcSlice {
			sliceElemVal := reflect.New(elemType).Elem()
			if err := d.assignDecodedToValue(sliceElemVal, item); err != nil {
				return fmt.Errorf("bencode: error decoding slice element %d: %w", i, err)
			}
			newSlice.Index(i).Set(sliceElemVal)
		}
		destVal.Set(newSlice)
	case reflect.Map:
		if destVal.Type().Key().Kind() != reflect.String {
			return fmt.Errorf("bencode: map keys must be strings for destination type %s, got key type %s", destVal.Type(), destVal.Type().Key())
		}
		srcMap, ok := srcData.(map[string]any)
		if !ok {
			return fmt.Errorf("bencode: expected map[string]any for map destination, got %T", srcData)
		}
		mapType := destVal.Type()
		elemType := mapType.Elem()
		newMap := reflect.MakeMap(mapType)
		for key, item := range srcMap {
			mapElemVal := reflect.New(elemType).Elem()
			if err := d.assignDecodedToValue(mapElemVal, item); err != nil {
				return fmt.Errorf("bencode: error decoding map value for key %q: %w", key, err)
			}
			newMap.SetMapIndex(reflect.ValueOf(key), mapElemVal)
		}
		destVal.Set(newMap)
	case reflect.Struct:
		srcMap, ok := srcData.(map[string]any)
		if !ok {
			return fmt.Errorf("bencode: expected map[string]any for struct destination %s, got %T", destVal.Type(), srcData)
		}
		return d.populateStruct(destVal, srcMap)
	case reflect.Interface:
		if !srcType.AssignableTo(destVal.Type()) {
			return fmt.Errorf("bencode: cannot assign type %s to interface %s", srcType, destVal.Type())
		}
		destVal.Set(reflect.ValueOf(srcData))
	default:
		// Attempt direct assignment if types are compatible.
		// This might cover cases like assigning to a custom type that's an alias of int64, etc.
		if srcType.AssignableTo(destVal.Type()) {
			destVal.Set(reflect.ValueOf(srcData))
		} else {
			return fmt.Errorf("bencode: unhandled destination type %s (source type %s)", destVal.Type(), srcType)
		}
	}
	return nil
}

// populateStruct populates the fields of 'structVal' using data from 'dictData'.
// 'structVal' is the reflect.Value of the struct to populate.
// 'dictData' is a map[string]any, typically from d.decode().
func (d *Decoder) populateStruct(structVal reflect.Value, dictData map[string]any) error {
	if structVal.Kind() != reflect.Struct {
		// This should ideally not be reached if called correctly from assignDecodedToValue
		return fmt.Errorf("bencode: internal error: populateStruct called with non-struct type %s", structVal.Type())
	}

	typ := structVal.Type()
	for i := range typ.NumField() {
		fieldStructDef := typ.Field(i)        // This is reflect.StructField
		fieldRuntimeVal := structVal.Field(i) // This is reflect.Value

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
			return fmt.Errorf("bencode: error setting field %s (tag %q): %w", fieldStructDef.Name, tag, err)
		}
	}
	return nil
}

func (d *Decoder) decode() (any, error) {
	next, err := d.r.Peek(1)
	if err != nil {
		if err == io.EOF {
			return nil, ErrNullRootValue // End of stream before any token
		}
		return nil, err
	}
	token := rune(next[0])
	switch {
	case unicode.IsDigit(token):
		lengthString, err := d.r.ReadString(':')
		if err != nil {
			if err == io.EOF {
				return nil, ErrUnexpectedEOF // EOF while reading string length
			}
			return nil, err
		}
		length, err := strconv.Atoi(lengthString[:len(lengthString)-1])
		if err != nil {
			return nil, fmt.Errorf("bencode: invalid string length: %w", err)
		}
		if length < 0 {
			return nil, fmt.Errorf("bencode: invalid negative string length: %d", length)
		}
		data := make([]byte, length)
		n, err := io.ReadFull(d.r, data)
		if err != nil {
			if err == io.EOF || err == io.ErrUnexpectedEOF { // EOF while reading string data
				return nil, fmt.Errorf("%w: expected %d bytes for string, got %d", ErrUnexpectedEOF, length, n)
			}
			return nil, err
		}
		return data, nil

	case token == 'i':
		_, _ = d.r.Discard(1) // discard 'i'
		numString, err := d.r.ReadString('e')
		if err != nil {
			if err == io.EOF {
				return nil, fmt.Errorf("%w: integer not terminated by 'e'", ErrUnexpectedEOF)
			}
			return nil, err
		}
		numString = numString[:len(numString)-1] // remove trailing 'e'
		if len(numString) == 0 {
			return nil, fmt.Errorf("%w: empty integer", ErrInvalidInteger)
		}

		num, err := strconv.ParseInt(numString, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("%w: %s (%v)", ErrInvalidInteger, numString, err)
		}

		if len(numString) > 1 && (strings.HasPrefix(numString, "0") || strings.HasPrefix(numString, "-0")) {
			return nil, fmt.Errorf("%w: invalid format %s", ErrInvalidInteger, numString)
		}

		return num, nil

	case token == 'l':
		_, _ = d.r.Discard(1) // discard 'l'

		var list []any
		for {
			peeked, err := d.r.Peek(1)
			if err != nil {
				if err == io.EOF { // EOF before list termination 'e' or next item
					return nil, fmt.Errorf("%w: list not terminated by 'e'", ErrUnexpectedEOF)
				}
				return nil, err
			}

			if rune(peeked[0]) == 'e' {
				if _, err = d.r.Discard(1); err != nil { // Consume 'e'
					return nil, err
				}
				break // End of list
			}

			item, err := d.decode()
			if err != nil {
				// If Decode() returned ErrNullRootValue, it means EOF was hit where an item was expected.
				if err == ErrNullRootValue {
					return nil, fmt.Errorf("%w: unexpected end of list", ErrUnexpectedEOF)
				}
				return nil, err // Propagate other errors from item decoding
			}
			list = append(list, item)
		}
		return list, nil

	case token == 'd':
		_, _ = d.r.Discard(1) // discard 'd'

		dict := make(map[string]any)
		var prevKey string // Initialize to empty string, first key doesn't need comparison
		firstKey := true

		for {
			peeked, err := d.r.Peek(1)
			if err != nil {
				if err == io.EOF { // EOF before dict termination 'e' or next key
					return nil, fmt.Errorf("%w: dictionary not terminated by 'e'", ErrUnexpectedEOF)
				}
				return nil, err
			}

			if rune(peeked[0]) == 'e' {
				if _, err = d.r.Discard(1); err != nil { // Consume 'e'
					return nil, err
				}
				break // End of dictionary
			}

			// Decode key
			keyVal, err := d.decode()
			if err != nil {
				if err == ErrNullRootValue { // EOF instead of key
					return nil, fmt.Errorf("%w: unexpected end of dictionary, expected key", ErrUnexpectedEOF)
				}
				return nil, err
			}
			byteKey, ok := keyVal.([]byte)
			if !ok {
				return nil, fmt.Errorf("%w: key %v (type %T) is not a string", ErrInvalidDictionaryKey, keyVal, keyVal)
			}

			strKey := string(byteKey)

			if _, exists := dict[strKey]; exists {
				return nil, fmt.Errorf("%w: key %q", ErrDuplicateDictionaryKey, strKey)
			}

			if !firstKey && prevKey >= strKey { // Keys must be strictly sorted
				return nil, fmt.Errorf("%w: key %q is not lexicographically after %q", ErrDictionaryKeysNotSorted, strKey, prevKey)
			}

			// Decode value
			value, err := d.decode()
			if err != nil {
				if err == ErrNullRootValue { // EOF instead of value
					return nil, fmt.Errorf("%w for key %q: %v", ErrMissingDictionaryValue, strKey, ErrUnexpectedEOF)
				}
				// Propagate error from value decoding, possibly wrapping it
				return nil, fmt.Errorf("bencode: error decoding value for key %q: %w", strKey, err)
			}
			dict[strKey] = value
			prevKey = strKey
			firstKey = false
		}
		return dict, nil
	default:
		return nil, fmt.Errorf("%w: unexpected token %q", ErrInvalidType, token)
	}
}
