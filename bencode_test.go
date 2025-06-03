package bencode

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestDecoder(t *testing.T) {
	testcases := []struct {
		name     string
		input    string
		expected any
	}{
		{
			name:     "simple string",
			input:    "4:spam",
			expected: []byte("spam"),
		},
		{
			name:     "empty string",
			input:    "0:",
			expected: []byte(""),
		},
		{
			name:     "integer",
			input:    "i42e",
			expected: 42,
		},
		{
			name:     "negative integer",
			input:    "i-42e",
			expected: -42,
		},
		{
			name:     "list",
			input:    "l4:spam4:eggse",
			expected: []any{[]byte("spam"), []byte("eggs")},
		},
		{
			name:     "nested list",
			input:    "l4:spamli42e3:eggee",
			expected: []any{[]byte("spam"), []any{42, []byte("egg")}},
		},
		{
			name:  "dictionary",
			input: "d3:baz3:qux3:foo3:bare",
			expected: map[string]any{
				"foo": []byte("bar"),
				"baz": []byte("qux"),
			},
		},
		{
			name:  "nested dictionary",
			input: "d3:bard3:qux3:quxe3:fooi42ee",
			expected: map[string]any{
				"foo": 42,
				"bar": map[string]any{
					"qux": []byte("qux"),
				},
			},
		},
	}
	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			decoder := NewDecoder(strings.NewReader(tc.input))
			result, err := decoder.Decode()
			if err != nil {
				t.Fatalf("Decode failed: %v", err)
			}
			if !reflect.DeepEqual(result, tc.expected) {
				t.Errorf("Expected %v, got %v", tc.expected, result)
			}
		})
	}
}

func TestDecoderErr(t *testing.T) {
	testcases := []struct {
		name        string
		input       string
		expectedErr error
	}{
		{
			name:        "null root value",
			input:       "",
			expectedErr: ErrNullRootValue,
		},
		{
			name:        "invalid type",
			input:       "x",
			expectedErr: ErrInvalidType,
		},
		{
			name:        "missing list terminator",
			input:       "l4:spamli42e3:egg",
			expectedErr: ErrUnexpectedEOF,
		},
		{
			name:        "missing dict terminator",
			input:       "d3:bar3:baz",
			expectedErr: ErrUnexpectedEOF,
		},
		{
			name:        "invalid integer format",
			input:       "ide",
			expectedErr: ErrInvalidInteger,
		},
		{
			name:        "integer leading zero",
			input:       "i01e",
			expectedErr: ErrInvalidInteger,
		},
		{
			name:        "integer leading negative zero",
			input:       "i-01e",
			expectedErr: ErrInvalidInteger,
		},
		{
			name:        "integer empty",
			input:       "ie",
			expectedErr: ErrInvalidInteger,
		},
		{
			name:        "string with negative length",
			input:       "-4:spam",
			expectedErr: ErrInvalidType,
		},
		{
			name:        "dictionary with non-string key",
			input:       "d3:foo3:bari42e",
			expectedErr: ErrInvalidDictionaryKey,
		},
		{
			name:        "duplicate dictionary key",
			input:       "d3:fooi42e3:fooi43ee",
			expectedErr: ErrDuplicateDictionaryKey,
		},
		{
			name:        "missing dictionary value",
			input:       "d3:bare",
			expectedErr: ErrInvalidType,
		},
		{
			name:        "dictionary keys not sorted",
			input:       "d3:foo3:bar3:baz3:quxe",
			expectedErr: ErrDictionaryKeysNotSorted,
		},
	}
	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			decoder := NewDecoder(strings.NewReader(tc.input))
			_, err := decoder.Decode()
			if !errors.Is(err, tc.expectedErr) {
				t.Errorf("Expected error %v, got %v", tc.expectedErr, err)
			}
		})
	}
}

var (
	unmarshalTestData  = []byte("d8:announce38:udp://tracker.publicbt.com:80/announce13:announce-listll38:udp://tracker.publicbt.com:80/announceel44:udp://tracker.openbittorrent.com:80/announceee7:comment33:Debian CD from cdimage.debian.org4:infod6:lengthi170917888e4:name30:debian-8.8.0-arm64-netinst.iso12:piece lengthi262144eee")
	bytesInt64TestData = map[string]any{}{
		"announce": []byte("udp://tracker.publicbt.com:80/announce"),
		"announce-list": []any{}{
			[]any{}{[]byte("udp://tracker.publicbt.com:80/announce")},
			[]any{}{[]byte("udp://tracker.openbittorrent.com:80/announce")},
		},
		"comment": []byte("Debian CD from cdimage.debian.org"),
		"info": map[string]any{}{
			"name":         []byte("debian-8.8.0-arm64-netinst.iso"),
			"length":       int64(170917888),
			"piece length": int64(262144),
		},
	}
)

func TestUnmarshal(t *testing.T) {
	torrent, err := Unmarshal(unmarshalTestData)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(bytesInt64TestData, torrent) {
		t.Errorf("Expected %v, got %v", bytesInt64TestData, torrent)
	}
}
