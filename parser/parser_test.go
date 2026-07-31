package parser

import (
	"reflect"
	"testing"
)

func TestCleanSongName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{`"Es Salsa"`, `Es Salsa`},
		{`“Lo Menea Como E"`, `Lo Menea Como E`},
		{`'Voltaje'`, `Voltaje`},
		{`  "Roar"  `, `Roar`},
		{`Bye Bye`, `Bye Bye`},
		{`“Found My Way”`, `Found My Way`},
	}

	for _, test := range tests {
		result := CleanSongName(test.input)
		if result != test.expected {
			t.Errorf("CleanSongName(%q) = %q, expected %q", test.input, result, test.expected)
		}
	}
}

func TestParseLivePlaylist(t *testing.T) {
	input := `
/update_live
"Joga Bonito (World Cup Anthems) - DOSE"
"Es Salsa"
"Wilfrido"

"Que Tienes Ahi"
`
	expected := []string{
		"Joga Bonito (World Cup Anthems) - DOSE",
		"Es Salsa",
		"Wilfrido",
		"Que Tienes Ahi",
	}

	result := ParseLivePlaylist(input)
	if !reflect.DeepEqual(result, expected) {
		t.Errorf("ParseLivePlaylist output mismatch.\nGot: %v\nExpected: %v", result, expected)
	}
}

func TestParseZinSong(t *testing.T) {
	tests := []struct {
		input    string
		expected ZinSong
	}{
		{`#123"Es Salsa"`, ZinSong{Prefix: "#123", SongName: "Es Salsa"}},
		{`#123 “Lo Menea Como E"`, ZinSong{Prefix: "#123", SongName: "Lo Menea Como E"}},
		{`"Que Tienes Ahi"`, ZinSong{Prefix: "", SongName: "Que Tienes Ahi"}},
		{`#MM114(2027/7月）"Princeso"`, ZinSong{Prefix: "#MM114(2027/7月）", SongName: "Princeso"}},
		{`#124先行曲"Boompala"`, ZinSong{Prefix: "#124先行曲", SongName: "Boompala"}},
	}

	for _, test := range tests {
		result := ParseZinSong(test.input)
		if result != test.expected {
			t.Errorf("ParseZinSong(%q) = %+v, expected %+v", test.input, result, test.expected)
		}
	}
}

func TestParseZinInput(t *testing.T) {
	input := `
/add_zin
2026/07
Zin123(2027/6月教材）
#123"Es Salsa"
"Que Tienes Ahi"
#MM114(2027/7月）"Princeso"
`
	expectedMonth := "2026/07"
	expectedAlbum := "Zin123"
	expectedDesc := "2027/6月教材"
	expectedSongs := []ZinSong{
		{Prefix: "#123", SongName: "Es Salsa"},
		{Prefix: "", SongName: "Que Tienes Ahi"},
		{Prefix: "#MM114(2027/7月）", SongName: "Princeso"},
	}

	month, album, desc, songs, err := ParseZinInput(input)
	if err != nil {
		t.Fatalf("ParseZinInput failed: %v", err)
	}

	if month != expectedMonth {
		t.Errorf("Month = %q, expected %q", month, expectedMonth)
	}
	if album != expectedAlbum {
		t.Errorf("AlbumName = %q, expected %q", album, expectedAlbum)
	}
	if desc != expectedDesc {
		t.Errorf("Description = %q, expected %q", desc, expectedDesc)
	}
	if !reflect.DeepEqual(songs, expectedSongs) {
		t.Errorf("Songs mismatch.\nGot: %+v\nExpected: %+v", songs, expectedSongs)
	}
}

func TestCalculateSimilarity(t *testing.T) {
	tests := []struct {
		s        string
		t        string
		expected float64
	}{
		{"", "", 1.0},
		{"Voltaje", "Voltaje", 1.0},
		// 編輯距離 1，長度 10 -> 相似度 0.90
		{"Voltaje-12", "Voltaje-1", 0.90},
		// 編輯距離 1，長度 21 -> 相似度 20/21 = 0.95238
		{"Joga Bonito-DOSE-1234", "Joga Bonito-DOSE-123", 0.95238},
	}

	for _, test := range tests {
		res := CalculateSimilarity(test.s, test.t)
		// 容忍浮點數些微誤差
		if res < test.expected-0.0001 || res > test.expected+0.0001 {
			t.Errorf("Similarity(%q, %q) = %f, expected %f", test.s, test.t, res, test.expected)
		}
	}
}
