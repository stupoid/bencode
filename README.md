# bencode

A robust and straightforward Go library for encoding and decoding data in Bencode format. The API is designed to be familiar, mirroring the standard library's [`encoding/json`](https://pkg.go.dev/encoding/json) package.

Bencode (pronounced B-encode) is a data serialization format used primarily by the BitTorrent peer-to-peer file sharing system.

## Features

- **Simple API:** Marshal and Unmarshal functions similar to `encoding/json`.
- **Streaming Support:** `Encoder` and `Decoder` types for working with `io.Reader` and `io.Writer`.
- **Struct Tagging:** Customize struct field encoding with `bencode` tags (e.g., `bencode:"custom_name"`).
- **Comprehensive Type Support:**
  - Integers (int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64)
  - Strings and `[]byte`
  - Slices (encoded as Bencode lists)
  - Maps with string keys (encoded as Bencode dictionaries, keys are automatically sorted)
  - Structs (encoded as Bencode dictionaries)
- **Detailed Error Handling:** Custom error types for precise error identification.

## Installation

```sh
go get github.com/stupoid/bencode@latest
```

## Usage

### Basic Marshaling and Unmarshaling

The `Marshal` and `Unmarshal` functions provide a quick way to encode Go values to Bencode and decode Bencode data back into Go values.

```go
package main

import (
    "fmt"
    "log"

    "github.com/stupoid/bencode"
)

type Info struct {
    Length int64  `bencode:"length"`
    Name   string `bencode:"name"`
}

type Metainfo struct {
    Announce string `bencode:"announce"`
    Info     Info   `bencode:"info"`
}

func main() {
    meta := Metainfo{
        Announce: "http://example.com/announce",
        Info: Info{
            Length: 123456789,
            Name:   "example_file.txt",
        },
    }

    // Encoding the Metainfo struct to bencode format
    bencodedBytes, err := bencode.Marshal(meta)
    if err != nil {
        log.Fatalf("Failed to marshal: %v", err)
    }

    // Output the bencoded data
    fmt.Println(string(bencodedBytes))
    // Output: d8:announce27:http://example.com/announce4:infod6:lengthi123456789e4:name16:example_file.txtee

    // Decoding the bencoded data back to Metainfo struct
    var decodedMeta Metainfo
    err = bencode.Unmarshal(bencodedBytes, &decodedMeta)
    if err != nil {
        log.Fatalf("Failed to unmarshal: %v", err)
    }

    fmt.Printf("Decoded Announce: %s\n", decodedMeta.Announce)
    fmt.Printf("Decoded Info Name: %s\n", decodedMeta.Info.Name)
}
```

### Using Encoder and Decoder with Streams

For more control, especially when working with network connections or files, you can use the `Encoder` and `Decoder` types.

```go
package main

import (
    "bytes"
    "fmt"
    "log"

    "github.com/stupoid/bencode"
)

type TorrentFile struct {
    Announce     string `bencode:"announce"`
    CreationDate int64  `bencode:"creation date"`
}

func main() {
    torrent := TorrentFile{
        Announce:     "udp://tracker.openbittorrent.com:80",
        CreationDate: 1678886400, // Example timestamp
    }

    var buf bytes.Buffer

    // Encoding to a buffer
    encoder := bencode.NewEncoder(&buf)
    if err := encoder.Encode(torrent); err != nil {
        log.Fatalf("Encoder failed: %v", err)
    }

    fmt.Printf("Encoded data: %s\n", buf.String())
    // Output: Encoded data: d8:announce33:udp://tracker.openbittorrent.com:8013:creation datei1678886400ee

    // Decoding from the buffer
    var decodedTorrent TorrentFile
    decoder := bencode.NewDecoder(&buf) // buf now contains the encoded data
    if err := decoder.Decode(&decodedTorrent); err != nil {
        log.Fatalf("Decoder failed: %v", err)
    }

    fmt.Printf("Decoded Announce URL: %s\n", decodedTorrent.Announce)
    fmt.Printf("Decoded Creation Date: %d\n", decodedTorrent.CreationDate)
}
```

### Decoding into Generic Types with `DecodeValue`

If you don't know the structure of the Bencode data beforehand, or if you want to inspect it generically, you can use `Decoder.DecodeValue()`.

```go
package main

import (
    "bytes"
    "fmt"
    "log"

    "github.com/stupoid/bencode"
)

func main() {
    bencodedData := []byte("d3:key5:value4:listli1ei2ei3ee5:innerd1:ai10eee")

    decoder := bencode.NewDecoder(bytes.NewReader(bencodedData))
    decodedValue, err := decoder.DecodeValue()
    if err != nil {
        log.Fatalf("DecodeValue failed: %v", err)
    }

    // decodedValue will be a map[string]any, []any, int64, or []byte
    // You need to use type assertions to work with the data.

    dataMap, ok := decodedValue.(map[string]any)
    if !ok {
        log.Fatalf("Expected top-level to be a dictionary, got %T", decodedValue)
    }

    for key, value := range dataMap {
        fmt.Printf("Key: %s\n", key)
        switch v := value.(type) {
        case []byte:
            fmt.Printf("  Type: string, Value: %s\n", string(v))
        case int64:
            fmt.Printf("  Type: int64, Value: %d\n", v)
        case []any:
            fmt.Printf("  Type: list, Value: %v\n", v)
        case map[string]any:
            fmt.Printf("  Type: dictionary, Value: %v\n", v)
        default:
            fmt.Printf("  Type: unknown, Value: %v\n", v)
        }
    }
    /*
    Output:
    Key: inner
      Type: dictionary, Value: map[a:10]
    Key: key
      Type: string, Value: value
    Key: list
      Type: list, Value: [1 2 3]
    */
}
```

The `bencode.Error` struct has the following fields:

- `Type`: An `ErrorType` (string) categorizing the error (e.g., `bencode.ErrSyntax`, `bencode.ErrUnmarshalType`).
- `Msg`: A human-readable description of the error.
- `FieldName`: The name of the struct field or map key related to the error, if applicable.
- `WrappedErr`: The underlying error, if any, allowing for error chaining.

You can check the specific `ErrorType` constants defined in `error.go`, `encoder.go`, and `decoder.go` for more granular error handling.

## Struct Tags

When encoding or decoding structs, you can control how fields are processed using the `bencode` struct tag:

### Basic Tag Format

```go
type Example struct {
    // Format: `bencode:"key_name"`
    Field string `bencode:"custom_key_name"`
}
```

### Tag Behavior Notes

- If no `bencode` tag is provided, the field's name is used as the key

## Contributing

Contributions are welcome! Please feel free to submit a pull request or open an issue.

## License

This library is [MIT licensed](./LICENSE).
