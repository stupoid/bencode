package bencode

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
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
)

type Decoder struct {
	r *bufio.Reader
}

func NewDecoder(r io.Reader) *Decoder {
	return &Decoder{r: bufio.NewReader(r)}
}

func (d *Decoder) Decode() (any, error) {
	for {
		next, err := d.r.Peek(1)
		if err != nil {
			if err == io.EOF {
				return nil, ErrNullRootValue
			}
			return nil, err
		}
		token := rune(next[0])
		switch {
		case unicode.IsDigit(token):
			lengthString, err := d.r.ReadString(':')
			if err != nil {
				return nil, err
			}
			length, err := strconv.Atoi(lengthString[:len(lengthString)-1])
			if err != nil {
				return nil, err
			}
			data := make([]byte, length)
			_, err = io.ReadFull(d.r, data)
			if err != nil {
				return nil, err
			}
			return string(data), nil

		case token == 'i':
			_, err = d.r.Discard(1) // discard 'i'
			if err != nil {
				return nil, err
			}
			numString, err := d.r.ReadString('e')
			if err != nil {
				return nil, err
			}
			numString = numString[:len(numString)-1] // remove trailing 'e'
			num, err := strconv.Atoi(numString)
			if err != nil {
				return nil, fmt.Errorf("%w: %s", ErrInvalidInteger, numString)
			}
			if numString[0] == '0' || (len(numString) > 1 && numString[0] == '-' && numString[1] == '0') {
				return nil, fmt.Errorf("%w: %s", ErrInvalidInteger, numString)
			}

			return num, nil

		case token == 'l':
			_, err = d.r.Discard(1) // discard 'l'
			if err != nil {
				return nil, err
			}
			var list []any
			for {
				if next, err := d.r.Peek(1); err == nil {
					token := rune(next[0])
					if token == 'e' {
						if _, err = d.r.Discard(1); err != nil {
							return nil, err
						}
						break
					}
				}

				item, err := d.Decode()
				if err != nil {
					return nil, err
				}
				if item == nil { // EOF or end of list
					break
				}
				list = append(list, item)
			}

			return list, nil

		case token == 'd':
			_, err = d.r.Discard(1) // discard 'd'
			if err != nil {
				return nil, err
			}

			dict := make(map[string]any)
			prevKey := ""
			for {
				if next, err := d.r.Peek(1); err == nil {
					token := rune(next[0])
					if token == 'e' {
						if _, err = d.r.Discard(1); err != nil {
							return nil, err
						}
						break
					}
				}
				key, err := d.Decode()
				if err != nil {
					return nil, err
				}
				if key == nil { // EOF or end of dict
					break
				}
				strKey, ok := key.(string)
				if !ok {
					return nil, fmt.Errorf("%w: key %v is not a string", ErrInvalidDictionaryKey, key)
				}
				if _, exists := dict[strKey]; exists {
					return nil, fmt.Errorf("%w: key %q already exists in dictionary", ErrDuplicateDictionaryKey, strKey)
				}
				if prevKey != "" && prevKey > strKey {
					return nil, ErrDictionaryKeysNotSorted
				}
				prevKey = strKey
				value, err := d.Decode()
				if err != nil {
					return nil, fmt.Errorf("%w: missing value for key %q", ErrMissingDictionaryValue, strKey)
				}
				dict[strKey] = value
			}
			return dict, nil
		default:
			return nil, fmt.Errorf("%w: unexpected token %q", ErrInvalidType, token)
		}
	}
}
