package bot

import (
	"testing"

	"zumba_music/db"
)

func TestFormatLiveSongLineRoles(t *testing.T) {
	songs := []db.LiveSongInfo{
		{DisplayName: "Warm Up", StartDate: "2026-08-01", Position: 1},
		{DisplayName: "Main Song", StartDate: "2026-08-02", Position: 2},
		{DisplayName: "Cool Down", StartDate: "2026-08-03", Position: 3},
	}

	warmUp := formatLiveSongLine(0, len(songs), songs[0])
	if warmUp != "1. *Warm Up* 🔥 暖身 (26/08/01)\n" {
		t.Errorf("Unexpected warm-up line: %q", warmUp)
	}

	mainSong := formatLiveSongLine(1, len(songs), songs[1])
	if mainSong != "2. *Main Song* (26/08/02)\n" {
		t.Errorf("Unexpected main song line: %q", mainSong)
	}

	coolDown := formatLiveSongLine(2, len(songs), songs[2])
	if coolDown != "3. *Cool Down* 🧘 收操 (26/08/03)\n" {
		t.Errorf("Unexpected cool-down line: %q", coolDown)
	}
}

func TestFormatSingleLiveSongHasBothRoles(t *testing.T) {
	song := db.LiveSongInfo{DisplayName: "Only Song", StartDate: "2026-08-01", Position: 1}
	line := formatLiveSongLine(0, 1, song)
	if line != "1. *Only Song* 🔥 暖身／🧘 收操 (26/08/01)\n" {
		t.Errorf("Unexpected single song line: %q", line)
	}
}

func TestFormatDuration(t *testing.T) {
	if got := formatDuration(191); got != "3:11" {
		t.Fatalf("formatDuration(191) = %q, want 3:11", got)
	}
}

func TestNormalizeZinIssue(t *testing.T) {
	for _, input := range []string{"123", "ZIN 123", "Zin123"} {
		if got := normalizeZinIssue(input); got != "123" {
			t.Errorf("normalizeZinIssue(%q) = %q, want 123", input, got)
		}
	}
}
