package bencode

import (
	"bytes"
	"fmt"
	"io"
	"reflect"
	"slices"
)

var (
	// ErrEncodeUnsupportedType indicates that a Go type cannot be marshaled into bencode.
	ErrEncodeUnsupportedType ErrorType = "encode: unsupported type"
	// ErrEncodeMapKeyNotString indicates that a Go map's key type is not string, which is required for bencode dictionaries.
	ErrEncodeMapKeyNotString ErrorType = "encode: map key not string"
	// ErrEncodeWriteError indicates an error occurred while writing to the output stream.
	ErrEncodeWriteError ErrorType = "encode: write error"
)

// Marshal returns the bencode encoding of v.
//
// Marshal traverses the value v recursively.
// Supported types are:
//   - int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64: encoded as bencode integers.
//   - string, []byte: encoded as bencode strings.
//   - slices: encoded as bencode lists.
//   - maps with string keys: encoded as bencode dictionaries. Keys are sorted lexicographically.
//   - structs: encoded as bencode dictionaries. Exported fields are used, respecting 'bencode' tags
//     for key names (e.g., `bencode:"custom_name"`).
//
// Unsupported types will result in an error.
func Marshal(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := NewEncoder(&buf)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

type Encoder struct {
	w io.Writer
}

// NewEncoder returns a new encoder that writes to w.
func NewEncoder(w io.Writer) *Encoder {
	return &Encoder{w: w}
}

// Encode writes the bencode encoding of v to the stream.
//
// See the documentation for Marshal for details about the conversion
// of a Go value to bencode.
func (e *Encoder) Encode(v any) error {
	switch valTyped := v.(type) {
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		if _, err := fmt.Fprintf(e.w, "i%de", valTyped); err != nil {
			return &Error{Type: ErrEncodeWriteError, Msg: "failed to write integer", WrappedErr: err}
		}
		return nil
	case string:
		if _, err := fmt.Fprintf(e.w, "%d:%s", len([]byte(valTyped)), valTyped); err != nil {
			return &Error{Type: ErrEncodeWriteError, Msg: "failed to write string", WrappedErr: err}
		}
		return nil
	case []byte:
		if _, err := fmt.Fprintf(e.w, "%d:%s", len(valTyped), valTyped); err != nil {
			return &Error{Type: ErrEncodeWriteError, Msg: "failed to write byte slice", WrappedErr: err}
		}
		return nil
	default:
		val := reflect.ValueOf(v)

		switch val.Kind() {
		case reflect.Slice:
			if _, err := e.w.Write([]byte{'l'}); err != nil {
				return &Error{Type: ErrEncodeWriteError, Msg: "failed to write list start token 'l'", WrappedErr: err}
			}
			for i := range val.Len() {
				if err := e.Encode(val.Index(i).Interface()); err != nil {
					// Propagate error, potentially wrapping if it's a write error from a sub-call
					// For now, assume Encode returns *Error or nil
					return err
				}
			}
			if _, err := e.w.Write([]byte{'e'}); err != nil {
				return &Error{Type: ErrEncodeWriteError, Msg: "failed to write list end token 'e'", WrappedErr: err}
			}
			return nil
		case reflect.Map:
			if val.Type().Key().Kind() != reflect.String {
				return &Error{Type: ErrEncodeMapKeyNotString, Msg: fmt.Sprintf("map key type %s is not supported; only string keys are allowed", val.Type().Key().Kind())}
			}
			sortedKeys := make([]string, 0, val.Len())
			mapKeys := val.MapKeys()
			for _, key := range mapKeys {
				sortedKeys = append(sortedKeys, key.String())
			}
			slices.Sort(sortedKeys)

			if _, err := e.w.Write([]byte{'d'}); err != nil {
				return &Error{Type: ErrEncodeWriteError, Msg: "failed to write dictionary start token 'd'", WrappedErr: err}
			}
			for _, keyStr := range sortedKeys {
				// Encode key (which is a string)
				if _, err := fmt.Fprintf(e.w, "%d:%s", len([]byte(keyStr)), keyStr); err != nil {
					return &Error{Type: ErrEncodeWriteError, Msg: fmt.Sprintf("failed to write dictionary key %q", keyStr), WrappedErr: err, FieldName: keyStr}
				}
				// Encode value
				if err := e.Encode(val.MapIndex(reflect.ValueOf(keyStr)).Interface()); err != nil {
					// If err is already *Error, add FieldName context if not present or enhance.
					if bErr, ok := err.(*Error); ok {
						if bErr.FieldName == "" {
							bErr.FieldName = keyStr
						}
						return bErr
					}
					return &Error{Type: ErrEncodeWriteError, Msg: fmt.Sprintf("failed to encode value for dictionary key %q", keyStr), WrappedErr: err, FieldName: keyStr}
				}
			}
			if _, err := e.w.Write([]byte{'e'}); err != nil {
				return &Error{Type: ErrEncodeWriteError, Msg: "failed to write dictionary end token 'e'", WrappedErr: err}
			}
			return nil
		case reflect.Struct:
			if _, err := e.w.Write([]byte{'d'}); err != nil {
				return &Error{Type: ErrEncodeWriteError, Msg: "failed to write dictionary start token 'd' for struct", WrappedErr: err}
			}
			cachedFields := getCachedStructInfo(val.Type()) // Assuming this doesn't error or panics on setup
			for _, fieldInfo := range cachedFields {
				fieldVal := val.FieldByIndex([]int{fieldInfo.index})
				// Encode key (bencodeTag)
				if _, err := fmt.Fprintf(e.w, "%d:%s", len([]byte(fieldInfo.bencodeTag)), fieldInfo.bencodeTag); err != nil {
					return &Error{Type: ErrEncodeWriteError, Msg: fmt.Sprintf("failed to write struct field key %q", fieldInfo.bencodeTag), WrappedErr: err, FieldName: fieldInfo.bencodeTag}
				}
				// Encode field value
				if err := e.Encode(fieldVal.Interface()); err != nil {
					if bErr, ok := err.(*Error); ok {
						if bErr.FieldName == "" { // Add context if sub-encoding didn't
							bErr.FieldName = fieldInfo.bencodeTag
						}
						return bErr
					}
					return &Error{Type: ErrEncodeWriteError, Msg: fmt.Sprintf("failed to encode struct field %q (tag %q)", fieldInfo.fieldName, fieldInfo.bencodeTag), WrappedErr: err, FieldName: fieldInfo.bencodeTag}
				}
			}
			if _, err := e.w.Write([]byte{'e'}); err != nil {
				return &Error{Type: ErrEncodeWriteError, Msg: "failed to write dictionary end token 'e' for struct", WrappedErr: err}
			}
			return nil
		default:
			return &Error{Type: ErrEncodeUnsupportedType, Msg: fmt.Sprintf("cannot marshal type %T (%s)", v, val.Kind())}
		}
	}

}
