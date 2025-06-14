package bencode

import (
	"bytes"
	"errors"
	"io"
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

func TestEncodeStructNoBencodeNameTag(t *testing.T) {
	type TestStruct struct {
		Name  string `bencode:"name"`
		Value int    // No bencode tag, should use field name
	}

	tests := []struct {
		name     string
		value    any
		expected string
	}{
		{
			name:     "struct with bencode tag and no tag",
			value:    TestStruct{Name: "test", Value: 123},
			expected: "d5:Valuei123e4:name4:teste", // Note: 'Value' is used as the key for the int field
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

// failingWriter is an io.Writer that always returns an error.
type failingWriter struct {
	err error
}

func (fw *failingWriter) Write(p []byte) (n int, err error) {
	return 0, fw.err
}

func TestEncodeErrors(t *testing.T) {
	tests := []struct {
		name          string
		value         any
		writer        io.Writer // Use a specific writer for write error tests
		expectedError *Error    // Expect a specific bencode.Error
		wantErrMsg    string    // For more generic error messages if needed
		checkWrapped  bool      // Whether to check the wrapped error
		wrappedError  error     // Expected wrapped error
	}{
		{
			name:          "unsupported type (chan)",
			value:         make(chan int),
			expectedError: &Error{Type: ErrEncodeUnsupportedType},
		},
		{
			name:          "map with non-string key",
			value:         map[int]string{1: "one"},
			expectedError: &Error{Type: ErrEncodeMapKeyNotString},
		},
		{
			name:          "write error on integer",
			value:         123,
			writer:        &failingWriter{err: errors.New("simulated write fail")},
			expectedError: &Error{Type: ErrEncodeWriteError},
			checkWrapped:  true,
			wrappedError:  errors.New("simulated write fail"),
		},
		{
			name:          "write error on string",
			value:         "test string",
			writer:        &failingWriter{err: errors.New("simulated write fail")},
			expectedError: &Error{Type: ErrEncodeWriteError},
			checkWrapped:  true,
			wrappedError:  errors.New("simulated write fail"),
		},
		{
			name:          "write error on list start",
			value:         []int{1},
			writer:        &failingWriter{err: errors.New("simulated write fail")},
			expectedError: &Error{Type: ErrEncodeWriteError},
			checkWrapped:  true,
			wrappedError:  errors.New("simulated write fail"),
		},
		{
			name:          "write error on dict start",
			value:         map[string]int{"a": 1},
			writer:        &failingWriter{err: errors.New("simulated write fail")},
			expectedError: &Error{Type: ErrEncodeWriteError},
			checkWrapped:  true,
			wrappedError:  errors.New("simulated write fail"),
		},
		{
			name: "write error on struct dict start",
			value: struct {
				Name string `bencode:"name"`
			}{Name: "test"},
			writer:        &failingWriter{err: errors.New("simulated write fail")},
			expectedError: &Error{Type: ErrEncodeWriteError},
			checkWrapped:  true,
			wrappedError:  errors.New("simulated write fail"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var enc *Encoder
			if tt.writer != nil {
				enc = NewEncoder(tt.writer)
			} else {
				var b bytes.Buffer // Default writer if not specified
				enc = NewEncoder(&b)
			}

			err := enc.Encode(tt.value)

			if err == nil {
				t.Fatalf("Encode() expected an error, but got nil")
			}

			bencodeErr, ok := err.(*Error)
			if !ok {
				if tt.wantErrMsg != "" && err.Error() == tt.wantErrMsg {
					return // Generic error message matched
				}
				t.Fatalf("Encode() error = %v, want type *bencode.Error", err)
			}

			if tt.expectedError != nil {
				if bencodeErr.Type != tt.expectedError.Type {
					t.Errorf("Encode() error type = %q, want %q", bencodeErr.Type, tt.expectedError.Type)
				}
				if tt.expectedError.FieldName != "" && bencodeErr.FieldName != tt.expectedError.FieldName {
					t.Errorf("Encode() error field name = %q, want %q", bencodeErr.FieldName, tt.expectedError.FieldName)
				}
			}

			if tt.checkWrapped {
				unwrapped := errors.Unwrap(bencodeErr)
				if unwrapped == nil {
					t.Errorf("Encode() expected a wrapped error, but got nil")
				} else if tt.wrappedError != nil && unwrapped.Error() != tt.wrappedError.Error() {
					// Note: Comparing error messages for wrapped errors as direct comparison might fail
					// if they are not the exact same instance but semantically equivalent.
					t.Errorf("Encode() wrapped error = %q, want %q", unwrapped.Error(), tt.wrappedError.Error())
				}
			}
		})
	}
}

func TestMarshal(t *testing.T) {
	got, err := Marshal(metainfoTestData)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	if !bytes.Equal(got, unmarshalTestData) {
		t.Errorf("Marshal() = %s, want %s", got, unmarshalTestData)
	}
}

func TestEncodeZeroValue(t *testing.T) {
	type NonRequiredStruct struct {
		Value int `bencode:"value"`
	}

	nonRequired := NonRequiredStruct{Value: 0} // Zero value for int, should not error
	expected := []byte("d5:valuei0ee")         // Expected bencode output for zero value int
	var b bytes.Buffer
	enc := NewEncoder(&b)

	err := enc.Encode(nonRequired)
	if err != nil {
		t.Errorf("Encode() error = %v, want nil", err)
	}
	if !bytes.Equal(b.Bytes(), expected) {
		t.Errorf("Encode() = %s, want %s", b.Bytes(), expected)
	}
}
