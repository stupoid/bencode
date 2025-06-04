package bencode

import (
	"bytes"
	"reflect"
	"testing"
)

type TorrentInfo struct {
	Pieces      string `bencode:"pieces"`
	PieceLength int64  `bencode:"piece length"`
	Length      int64  `bencode:"length"`
	Name        string `bencode:"name"`
}

type Torrent struct {
	Announce     string      `bencode:"announce"`
	AnnounceList [][]string  `bencode:"announce-list"`
	Comment      string      `bencode:"comment"`
	Info         TorrentInfo `bencode:"info"`
}

var (
	unmarshalTestData = []byte("d8:announce38:udp://tracker.publicbt.com:80/announce13:announce-listll38:udp://tracker.publicbt.com:80/announceel44:udp://tracker.openbittorrent.com:80/announceee7:comment33:Debian CD from cdimage.debian.org4:infod6:lengthi170917888e4:name30:debian-8.8.0-arm64-netinst.iso12:piece lengthi262144eee")
	torrentTestData   = Torrent{
		Announce: "udp://tracker.publicbt.com:80/announce",
		AnnounceList: [][]string{
			{"udp://tracker.publicbt.com:80/announce"},
			{"udp://tracker.openbittorrent.com:80/announce"},
		},
		Comment: "Debian CD from cdimage.debian.org",
		Info: TorrentInfo{
			Name:        "debian-8.8.0-arm64-netinst.iso",
			Length:      170917888,
			PieceLength: 262144,
		},
	}
)

func TestMarshalUnmarshal(t *testing.T) {
	bencodedBytes, err := Marshal(torrentTestData)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	if !bytes.Equal(bencodedBytes, unmarshalTestData) {
		t.Errorf("Marshal() = %v, want %v", bencodedBytes, unmarshalTestData)
	}

	var decodedStruct Torrent

	if err := Unmarshal(bencodedBytes, &decodedStruct); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	if !reflect.DeepEqual(decodedStruct, torrentTestData) {
		t.Errorf("Unmarshal() = %v, want %v", decodedStruct, torrentTestData)
	}
}
