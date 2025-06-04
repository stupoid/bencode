package bencode

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

type TorrentInfo struct {
	Pieces      string `bencode:"pieces"`
	PieceLength int64  `bencode:"piece length"`
	Length      int64  `bencode:"length"`
	Name        string `bencode:"name"`
}

type Torrent struct {
	Announce string      `bencode:"announce"`
	Comment  string      `bencode:"comment"`
	Info     TorrentInfo `bencode:"info"`
}

var (
	unmarshalTestData  = []byte("d8:announce38:udp://tracker.publicbt.com:80/announce13:announce-listll38:udp://tracker.publicbt.com:80/announceel44:udp://tracker.openbittorrent.com:80/announceee7:comment33:Debian CD from cdimage.debian.org4:infod6:lengthi170917888e4:name30:debian-8.8.0-arm64-netinst.iso12:piece lengthi262144eee")
	bytesInt64TestData = map[string]any{
		"announce": []byte("udp://tracker.publicbt.com:80/announce"),
		"announce-list": []any{
			[]any{[]byte("udp://tracker.publicbt.com:80/announce")},
			[]any{[]byte("udp://tracker.openbittorrent.com:80/announce")},
		},
		"comment": []byte("Debian CD from cdimage.debian.org"),
		"info": map[string]any{
			"name":         []byte("debian-8.8.0-arm64-netinst.iso"),
			"length":       int64(170917888),
			"piece length": int64(262144),
		},
	}
	torrentTestData = Torrent{
		Announce: "udp://tracker.publicbt.com:80/announce",
		Comment:  "Debian CD from cdimage.debian.org",
		Info: TorrentInfo{
			Name:        "debian-8.8.0-arm64-netinst.iso",
			Length:      170917888,
			PieceLength: 262144,
		},
	}
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
			_, err := decoder.decode()
			if !errors.Is(err, tc.expectedErr) {
				t.Errorf("Expected error %v, got %v", tc.expectedErr, err)
			}
		})
	}
}

func TestUnmarshal(t *testing.T) {
	var torrent Torrent
	err := Unmarshal(unmarshalTestData, &torrent)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(torrentTestData, torrent) {
		t.Errorf("Expected %v, got %v", torrentTestData, torrent)
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
