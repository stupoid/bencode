package bencode

import (
	"bytes"
	"testing"
)

func TestEncode(t *testing.T) {
	tests := []struct {
		name     string
		value    any
		expected string
	}{
		{
			name:     "string",
			value:    "hello",
			expected: "5:hello",
		},
		{
			name:     "int",
			value:    42,
			expected: "i42e",
		},
		{
			name:     "int64",
			value:    int64(1234567890),
			expected: "i1234567890e",
		},
		{
			name:     "byte slice",
			value:    []byte("world"),
			expected: "5:world",
		},
		{
			name:     "unprintable byte slice",
			value:    []byte{0, 1, 2, 3, 4},
			expected: "5:\x00\x01\x02\x03\x04",
		},
		{
			name:     "slice of byte slices",
			value:    [][]byte{[]byte("foo"), []byte("bar")},
			expected: "l3:foo3:bare",
		},
		{
			name:     "slice of strings",
			value:    []string{"foo", "bar"},
			expected: "l3:foo3:bare",
		},
		{
			name:     "slice of mixed types",
			value:    []any{"foo", 42, []byte("bar")},
			expected: "l3:fooi42e3:bare",
		},
		{
			name:     "map",
			value:    map[string]any{"key1": "value1", "key2": 42},
			expected: "d4:key16:value14:key2i42ee",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var b bytes.Buffer
			enc := NewEncoder(&b)
			err := enc.Encode(tt.value)
			if err != nil {
				t.Errorf("Encode() error = %v", err)
				return
			}

			if got := b.String(); got != tt.expected {
				t.Errorf("Encode() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestEncodeStruct(t *testing.T) {
	type TestStruct struct {
		Name  string `bencode:"name"`
		Value int    `bencode:"value"`
	}

	tests := []struct {
		name     string
		value    any
		expected string
	}{
		{
			name:     "struct with string and int",
			value:    TestStruct{Name: "test", Value: 123},
			expected: "d4:name4:test5:valuei123ee",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var b bytes.Buffer
			enc := NewEncoder(&b)
			err := enc.Encode(tt.value)
			if err != nil {
				t.Errorf("Encode() error = %v", err)
				return
			}

			if got := b.String(); got != tt.expected {
				t.Errorf("Encode() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestMarshal(t *testing.T) {
	got, err := Marshal(torrentTestData)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	if !bytes.Equal(got, unmarshalTestData) {
		t.Errorf("Marshal() = %s, want %s", got, unmarshalTestData)
	}
}
