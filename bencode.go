package bencode

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
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

// Unmarshal parses the Bencode-encoded data and returns the result.
// This is a helper function for when the data is already in a byte slice.
func Unmarshal(data []byte) (any, error) {
	dec := &Decoder{r: bufio.NewReaderSize(bytes.NewReader(data), len(data))}
	return dec.Decode()

}

type Decoder struct {
	r *bufio.Reader
}

func NewDecoder(r io.Reader) *Decoder {
	return &Decoder{r: bufio.NewReader(r)}
}

func (d *Decoder) Decode() (any, error) {
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

			item, err := d.Decode()
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
			keyVal, err := d.Decode()
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
			value, err := d.Decode()
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
