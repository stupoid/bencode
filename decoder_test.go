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
			expected: int64(42),
		},
		{
			name:     "negative integer",
			input:    "i-42e",
			expected: int64(-42),
		},
		{
			name:     "list",
			input:    "l4:spam4:eggse",
			expected: []any{[]byte("spam"), []byte("eggs")},
		},
		{
			name:     "nested list",
			input:    "l4:spamli42e3:eggee",
			expected: []any{[]byte("spam"), []any{int64(42), []byte("egg")}},
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
				"foo": int64(42),
				"bar": map[string]any{
					"qux": []byte("qux"),
				},
			},
		},
	}
	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			decoder := NewDecoder(strings.NewReader(tc.input))
			result, err := decoder.decode()
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
		name            string
		input           string
		expectedErrType ErrorType // Use ErrorType for general categorization
		expectedErr     error     // Use for specific sentinel errors like ErrNullRootValue
		expectedMsg     string    // Optional: for checking specific error messages
		expectedField   string    // Optional: for checking FieldName
	}{
		{
			name:        "null root value",
			input:       "",
			expectedErr: ErrNullRootValue, // This is a sentinel *Error
		},
		{
			name:            "invalid type - unexpected token",
			input:           "x",
			expectedErrType: ErrSyntaxUnexpectedToken,
		},
		{
			name:            "missing list terminator",
			input:           "l4:spamli42e3:egg",
			expectedErrType: ErrSyntaxEOF,     // EOF because 'e' is missing
			expectedErr:     ErrUnexpectedEOF, // Underlying sentinel
		},
		{
			name:            "missing dict terminator",
			input:           "d3:bar3:baz",
			expectedErrType: ErrSyntaxEOF,     // EOF because 'e' is missing
			expectedErr:     ErrUnexpectedEOF, // Underlying sentinel
		},
		{
			name:            "invalid integer format - empty",
			input:           "ie",
			expectedErrType: ErrSyntaxInteger,
			expectedMsg:     "empty integer",
		},
		{
			name:            "integer leading zero",
			input:           "i01e",
			expectedErrType: ErrSyntaxInteger,
			expectedMsg:     "invalid integer format (leading zero): 01",
		},
		{
			name:            "integer leading negative zero",
			input:           "i-01e",
			expectedErrType: ErrSyntaxInteger,
			expectedMsg:     "invalid integer format (leading zero): -01",
		},
		{
			name:            "integer empty - just 'ie'",
			input:           "ide",
			expectedErrType: ErrSyntaxInteger,
			expectedMsg:     "cannot parse integer \"d\"",
		},
		{
			name:            "string with negative length",
			input:           "-4:spam", // This will be caught by IsDigit check first
			expectedErrType: ErrSyntaxUnexpectedToken,
		},
		{
			name:            "string with invalid length (non-numeric)",
			input:           "a:spam",
			expectedErrType: ErrSyntaxUnexpectedToken, // Caught by IsDigit
		},
		{
			name:            "string length not terminated",
			input:           "4spam",
			expectedErrType: ErrSyntaxEOF,
			expectedErr:     ErrUnexpectedEOF,
		},
		{
			name:            "string EOF before full read",
			input:           "10:spam",
			expectedErrType: ErrSyntaxEOF,
			expectedErr:     ErrUnexpectedEOF,
		},
		{
			name:            "dictionary with non-string key",
			input:           "di42e3:valee", // Key is an integer
			expectedErrType: ErrStructureDict,
			expectedMsg:     "dictionary key type int64 is not a bencode string",
		},
		{
			name:            "duplicate dictionary key",
			input:           "d3:fooi42e3:fooi43ee",
			expectedErrType: ErrStructureDictKeyDup,
			expectedErr:     ErrDuplicateDictionaryKey, // Underlying sentinel
			expectedField:   "foo",
		},
		{

			name:            "missing dictionary value (EOF)",
			input:           "d3:keye",
			expectedErrType: ErrSyntaxUnexpectedToken,
		},
		{
			name:            "dictionary keys not sorted",
			input:           "d3:foo3:bar1:a3:quxe", // "a" should come before "foo"
			expectedErrType: ErrStructureDictKeySort,
			expectedErr:     ErrDictionaryKeysNotSorted, // Underlying sentinel
			expectedField:   "a",
		},
		{
			name:            "integer not terminated",
			input:           "i123",
			expectedErrType: ErrSyntaxEOF,
			expectedErr:     ErrUnexpectedEOF,
		},
	}
	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			decoder := NewDecoder(strings.NewReader(tc.input))
			_, err := decoder.decode()

			if err == nil {
				t.Fatalf("Expected an error, but got nil")
			}

			// Check if the error is of our custom type *Error
			bencodeErr, ok := err.(*Error)
			if !ok {
				// If it's not a *Error, it might be a wrapped standard error or an unexpected error type.
				// For sentinel errors defined as *Error, errors.Is should work directly.
				if tc.expectedErr != nil && errors.Is(err, tc.expectedErr) {
					return // Correct sentinel error
				}
				t.Fatalf("Expected error of type *bencode.Error, got %T: %v", err, err)
			}

			// If a specific sentinel *Error instance is expected, check with errors.Is
			if tc.expectedErr != nil {
				if !errors.Is(err, tc.expectedErr) {
					t.Errorf("Expected error to be or wrap %q, got %q (type %s, msg %s)", tc.expectedErr, err, bencodeErr.Type, bencodeErr.Msg)
				}
				// If errors.Is matches, we can often assume the type is also correct,
				// but we can add an explicit type check if needed for that sentinel.
				if bErr, isBencodeError := tc.expectedErr.(*Error); isBencodeError {
					if bencodeErr.Type != bErr.Type {
						t.Errorf("For sentinel error %v, expected type %q, got %q", tc.expectedErr, bErr.Type, bencodeErr.Type)
					}
				}

			} else if tc.expectedErrType != "" { // Otherwise, check the ErrorType
				if bencodeErr.Type != tc.expectedErrType {
					t.Errorf("Expected error type %q, got %q (full error: %v)", tc.expectedErrType, bencodeErr.Type, err)
				}
			} else {
				t.Errorf("Test case %q is missing expectedErr or expectedErrType", tc.name)
			}

			if tc.expectedMsg != "" && !strings.Contains(bencodeErr.Msg, tc.expectedMsg) {
				t.Errorf("Expected error message to contain %q, got %q", tc.expectedMsg, bencodeErr.Msg)
			}

			if tc.expectedField != "" && bencodeErr.FieldName != tc.expectedField {
				t.Errorf("Expected error field name %q, got %q", tc.expectedField, bencodeErr.FieldName)
			}
		})
	}
}

func TestUnmarshal(t *testing.T) {
	var metainfo Metainfo
	err := Unmarshal(unmarshalTestData, &metainfo)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(metainfoTestData, metainfo) {
		t.Errorf("Expected %v, got %v", metainfoTestData, metainfo)
	}
}

func TestUnmarshalRequiredTag(t *testing.T) {
	type RequiredStruct struct {
		Name     string `bencode:"name,required"`
		Age      int    `bencode:"age,required"`
		City     string `bencode:"city"` // Optional
		Country  string `bencode:"country,required"`
		Optional string `bencode:"optional_field"` // No ,required tag
	}

	testCases := []struct {
		name            string
		input           string
		target          any // Pointer to the struct
		expectErr       bool
		expectedErrType ErrorType
		expectedField   string
		expectedValue   any // Expected struct value if no error
	}{
		{
			name:          "all required fields present",
			input:         "d3:agei30e4:city7:NewYork7:country3:USA4:name4:Johne", // Keys: age, city, country, name
			target:        &RequiredStruct{},
			expectErr:     false,
			expectedValue: &RequiredStruct{Name: "John", Age: 30, City: "NewYork", Country: "USA"},
		},
		{
			name:            "one required field missing (name)",
			input:           "d3:agei30e4:city7:NewYork7:country3:USAe", // Keys: age, city, country
			target:          &RequiredStruct{},
			expectErr:       true,
			expectedErrType: ErrUnmarshalRequiredFieldMissing,
			expectedField:   "name",
		},
		{
			name:            "one required field missing (age)",
			input:           "d4:city7:NewYork7:country3:USA4:name4:Johne", // Keys: city, country, name
			target:          &RequiredStruct{},
			expectErr:       true,
			expectedErrType: ErrUnmarshalRequiredFieldMissing,
			expectedField:   "age",
		},
		{
			name:            "one required field missing (country)",
			input:           "d3:agei30e4:city7:NewYork4:name4:Johne", // Keys: age, city, name
			target:          &RequiredStruct{},
			expectErr:       true,
			expectedErrType: ErrUnmarshalRequiredFieldMissing,
			expectedField:   "country",
		},
		{
			name:            "multiple required fields missing",
			input:           "d4:city7:NewYorke", // Key: city
			target:          &RequiredStruct{},
			expectErr:       true,
			expectedErrType: ErrUnmarshalRequiredFieldMissing,
			expectedField:   "age", // First required field missing
		},
		{
			name:          "optional field missing, required present",
			input:         "d3:agei30e7:country3:USA4:name4:Johne", // Keys: age, country, name
			target:        &RequiredStruct{},
			expectErr:     false,
			expectedValue: &RequiredStruct{Name: "John", Age: 30, Country: "USA"},
		},
		{
			name:          "all fields present including optional",
			input:         "d3:agei30e4:city7:NewYork7:country3:USA4:name4:John14:optional_field5:Valuee", // Keys: age, city, country, name, optional_field
			target:        &RequiredStruct{},
			expectErr:     false,
			expectedValue: &RequiredStruct{Name: "John", Age: 30, City: "NewYork", Country: "USA", Optional: "Value"},
		},
		{
			name:            "empty input for struct with required fields",
			input:           "de", // Empty dictionary
			target:          &RequiredStruct{},
			expectErr:       true,
			expectedErrType: ErrUnmarshalRequiredFieldMissing,
			expectedField:   "age", // First required field missing
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Reset target for each test case by creating a new instance
			// This is important because Unmarshal modifies the pointed-to value.
			// We use reflection to create a new instance of the same type as tc.target.
			val := reflect.ValueOf(tc.target)
			if val.Kind() != reflect.Ptr {
				t.Fatalf("Test case %q target must be a pointer to a struct", tc.name)
			}
			newTarget := reflect.New(val.Elem().Type()).Interface()

			err := Unmarshal([]byte(tc.input), newTarget)

			if tc.expectErr {
				if err == nil {
					t.Fatalf("Expected an error, but got nil")
				}
				bencodeErr, ok := err.(*Error)
				if !ok {
					t.Fatalf("Expected error of type *bencode.Error, got %T: %v", err, err)
				}
				if bencodeErr.Type != tc.expectedErrType {
					t.Errorf("Expected error type %q, got %q (full error: %v)", tc.expectedErrType, bencodeErr.Type, err)
				}
				if tc.expectedField != "" && bencodeErr.FieldName != tc.expectedField {
					t.Errorf("Expected error field name %q, got %q", tc.expectedField, bencodeErr.FieldName)
				}
			} else {
				if err != nil {
					t.Fatalf("Expected no error, but got: %v", err)
				}
				if !reflect.DeepEqual(newTarget, tc.expectedValue) {
					t.Errorf("Expected value %+v, got %+v", tc.expectedValue, newTarget)
				}
			}
		})
	}
}

func TestDecodeTypeStruct(t *testing.T) {
	type TestStruct struct {
		Name  string `bencode:"name"`
		Value int64  `bencode:"value"`
	}

	var got TestStruct

	bencodeString := "d4:name4:Test5:valuei42ee"
	expected := TestStruct{
		Name:  "Test",
		Value: int64(42),
	}

	decoder := NewDecoder(strings.NewReader(bencodeString))
	err := decoder.Decode(&got)
	if err != nil {
		t.Fatalf("DecodeType failed: %v", err)
	}

	if !reflect.DeepEqual(got, expected) {
		t.Errorf("Expected %v, got %v", expected, got)
	}
}

func TestDencodeStructNoBencodeNameTag(t *testing.T) {
	type TestStruct struct {
		Name  string `bencode:"name"`
		Value int64  // No bencode tag, should use field name
	}

	var got TestStruct

	bencodeString := "d5:Valuei123e4:name4:teste"
	expected := TestStruct{
		Name:  "test",
		Value: int64(123),
	}

	decoder := NewDecoder(strings.NewReader(bencodeString))
	err := decoder.Decode(&got)
	if err != nil {
		t.Fatalf("DecodeType failed: %v", err)
	}

	if !reflect.DeepEqual(got, expected) {
		t.Errorf("Expected %v, got %v", expected, got)
	}
}

func TestDecodeTypeString(t *testing.T) {
	var got string

	bencodeString := "4:spam"
	expected := "spam"

	decoder := NewDecoder(strings.NewReader(bencodeString))
	err := decoder.Decode(&got)

	if err != nil {
		t.Fatalf("DecodeType failed: %v", err)
	}
	if !reflect.DeepEqual(got, expected) {
		t.Errorf("Expected %v, got %v", expected, got)
	}
}

func TestDecodeTypeInt64(t *testing.T) {
	var got int64

	bencodeString := "i42e"
	expected := int64(42)

	decoder := NewDecoder(strings.NewReader(bencodeString))
	err := decoder.Decode(&got)

	if err != nil {
		t.Fatalf("DecodeType failed: %v", err)
	}
	if !reflect.DeepEqual(got, expected) {
		t.Errorf("Expected %v, got %v", expected, got)
	}
}

func TestDecodeTypeSlice(t *testing.T) {
	var got []string

	bencodeString := "l4:spam4:eggse"
	expected := []string{"spam", "eggs"}

	decoder := NewDecoder(strings.NewReader(bencodeString))
	err := decoder.Decode(&got)

	if err != nil {
		t.Fatalf("DecodeType failed: %v", err)
	}
	if !reflect.DeepEqual(got, expected) {
		t.Errorf("Expected %v, got %v", expected, got)
	}
}

func TestDecodeTypeMap(t *testing.T) {
	var got map[string]string

	bencodeString := "d3:baz3:qux3:foo3:bare"
	expected := map[string]string{
		"foo": "bar",
		"baz": "qux",
	}

	decoder := NewDecoder(strings.NewReader(bencodeString))
	err := decoder.Decode(&got)

	if err != nil {
		t.Fatalf("DecodeType failed: %v", err)
	}
	if !reflect.DeepEqual(got, expected) {
		t.Errorf("Expected %v, got %v", expected, got)
	}
}

func TestDecodeTypeMapAny(t *testing.T) {
	var got map[string]any

	bencodeString := "d3:baz3:qux3:fooi123ee"
	expected := map[string]any{
		"baz": []byte("qux"),
		"foo": int64(123),
	}

	decoder := NewDecoder(strings.NewReader(bencodeString))
	err := decoder.Decode(&got)

	if err != nil {
		t.Fatalf("DecodeType failed: %v", err)
	}
	if !reflect.DeepEqual(got, expected) {
		t.Errorf("Expected %v, got %v", expected, got)
	}
}
