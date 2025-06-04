package bencode

import (
	"bytes"
	"fmt"
	"io"
	"reflect"
	"slices"
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
//     for key names and 'required' option (e.g., `bencode:"custom_name,required"`).
//     Zero-value fields are omitted unless marked as 'required'.
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
	switch v := v.(type) {
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		_, err := fmt.Fprintf(e.w, "i%de", v)
		return err
	case string:
		_, err := fmt.Fprintf(e.w, "%d:%s", len([]byte(v)), v)
		return err
	case []byte:
		_, err := fmt.Fprintf(e.w, "%d:%s", len(v), v)
		return err
	default:
		// Handle complex types
		val := reflect.ValueOf(v)

		switch val.Kind() {
		case reflect.Slice:
			if _, err := e.w.Write([]byte{'l'}); err != nil {
				return err
			}
			for i := range val.Len() {
				if err := e.Encode(val.Index(i).Interface()); err != nil {
					return err
				}
			}
			_, err := e.w.Write([]byte{'e'})
			return err
		case reflect.Map:
			if val.Type().Key().Kind() != reflect.String {
				return fmt.Errorf("unsupported map key type: %s", val.Type().Key().Kind())
			}
			sortedKeys := make([]string, 0, val.Len())
			keys := val.MapKeys()
			for _, key := range keys {
				sortedKeys = append(sortedKeys, key.String())
			}
			slices.Sort(sortedKeys)
			if _, err := e.w.Write([]byte{'d'}); err != nil {
				return err
			}
			for _, key := range sortedKeys {
				if err := e.Encode(key); err != nil {
					return err
				}
				if err := e.Encode(val.MapIndex(reflect.ValueOf(key)).Interface()); err != nil {
					return err
				}
			}
			_, err := e.w.Write([]byte{'e'})
			return err
		case reflect.Struct:
			if _, err := e.w.Write([]byte{'d'}); err != nil {
				return err
			}
			cachedFields := getCachedStructInfo(val.Type())
			for _, fieldInfo := range cachedFields {
				if val.FieldByName(fieldInfo.fieldName).IsZero() {
					if fieldInfo.required {
						return fmt.Errorf("required field %s is missing", fieldInfo.bencodeTag)
					}
					continue // Skip zero-value fields unless they are required
				}
				if _, err := fmt.Fprintf(e.w, "%d:%s", len([]byte(fieldInfo.bencodeTag)), fieldInfo.bencodeTag); err != nil {
					return err
				}
				if err := e.Encode(val.FieldByName(fieldInfo.fieldName).Interface()); err != nil {
					return err
				}
			}
			_, err := e.w.Write([]byte{'e'})
			return err
		default:
			return fmt.Errorf("unsupported type: %T", v)
		}
	}

}
