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

func TestParseLivePlaylistPreservesStakeholderOrder(t *testing.T) {
	input := `
"Dosie 100 - Bootyshake"
"Es Salsa"
"Wilfrido"
"Que Tienes Ahi"
"Lo Menea Como E"
"Roar"
"Voltaje"
"Princeso"
"Me Enamore De Madrid"
"Boompala"
"Turn Up The Bass"
"LEMONADE"
"BAD"
"Goals"
"Virou Baile"
"Sexy For Me"
"Sueldo De Mendigo"
"Found My Way"
`

	songs := ParseLivePlaylist(input)
	if len(songs) != 18 {
		t.Fatalf("Expected 18 songs, got %d: %v", len(songs), songs)
	}
	if songs[0] != "Dosie 100 - Bootyshake" {
		t.Errorf("Warm-up song mismatch: %q", songs[0])
	}
	if songs[len(songs)-1] != "Found My Way" {
		t.Errorf("Cool-down song mismatch: %q", songs[len(songs)-1])
	}
}

func TestParseProgramImportMegaMix114(t *testing.T) {
	input := `FORMAT_VERSION: 1
TYPE: MM
ISSUE: 114
RELEASE_MONTH:
TITLE: Mega Mix 114

TRACKS:
01 | Hoy No Me Llamen | Pipo Daniel | 101 | 3:11 |
02 | Princeso | Briella | 126 | 3:24 | Merengue
03 | Mentiroso | | 102 | 3:00 |
04 | Boomba | Zumba for Zumba Music Lab | 96 | 3:04 |
05 | Púlele | Max Pizzolante | 75 | 3:42 | African Fusion
06 | Bochinche | Baudhy LBA | 128 | 3:01 | Latin House
07 | Salta | Lelo y Rodri for Zumba Music Lab | 80 | 3:03 | Soca
08 | Que Tienes Ahi | Zumba | 135 | 2:56 | Caribbean Fusion
09 | Ponmela Ya | Moino | 105 | 3:06 | Flamenco
10 | Lokita x Ti | Ysa C, Mati Gomez | 102 | 2:27 | AfroBeat`

	program, err := ParseProgramImport(input)
	if err != nil {
		t.Fatalf("ParseProgramImport failed: %v", err)
	}
	if program.Type != "MM" || program.Issue != "114" || program.Title != "Mega Mix 114" {
		t.Fatalf("Unexpected program metadata: %+v", program)
	}
	if program.ReleaseMonth != "" {
		t.Errorf("Expected empty release month, got %q", program.ReleaseMonth)
	}
	if len(program.Tracks) != 10 {
		t.Fatalf("Expected 10 tracks, got %d", len(program.Tracks))
	}
	if program.Tracks[0].DurationSeconds != 191 {
		t.Errorf("Unexpected duration: %+v", program.Tracks[0])
	}
	if program.Tracks[2].Artist != "" || program.Tracks[2].Style != "" {
		t.Errorf("Expected optional fields to remain empty: %+v", program.Tracks[2])
	}
	if program.Tracks[9].SongName != "Lokita x Ti" || program.Tracks[9].Style != "AfroBeat" {
		t.Errorf("Unexpected final track: %+v", program.Tracks[9])
	}
}

func TestParseProgramImportAllowsEscapedPipe(t *testing.T) {
	input := `FORMAT_VERSION: 1
TYPE: MM
ISSUE: 1
RELEASE_MONTH:
TITLE: Test
TRACKS:
01 | Song \| Remix | Artist | 120 | 3:00 | Salsa`

	program, err := ParseProgramImport(input)
	if err != nil {
		t.Fatalf("ParseProgramImport failed: %v", err)
	}
	if program.Tracks[0].SongName != "Song | Remix" {
		t.Fatalf("Unexpected escaped song name: %q", program.Tracks[0].SongName)
	}
}

func TestParseProgramImportRejectsInvalidTrackSequence(t *testing.T) {
	input := `FORMAT_VERSION: 1
TYPE: ZIN
ISSUE: 124
RELEASE_MONTH: 2026/08
TITLE: ZIN 124
TRACKS:
02 | Song | Artist | 120 | 3:00 | Salsa`

	if _, err := ParseProgramImport(input); err == nil {
		t.Fatal("Expected invalid sequence error")
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
