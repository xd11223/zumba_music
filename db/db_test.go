package db

import (
	"database/sql"
	"path/filepath"
	"reflect"
	"testing"

	"zumba_music/parser"
)

func TestZumbaDB_Workflow(t *testing.T) {
	// 使用記憶體 SQLite 進行測試
	zdb, err := NewDB(":memory:")
	if err != nil {
		t.Fatalf("Failed to create in-memory DB: %v", err)
	}
	defer zdb.Close()

	userID := int64(99999)

	// ----------------------------------------------------
	// 測試 1: 第一次初始化 Live 歌單
	// ----------------------------------------------------
	firstPlaylist := []string{
		"Es Salsa",
		"Wilfrido",
		"Que Tienes Ahi",
	}

	added, removed, _, err := zdb.UpdateLivePlaylist(userID, firstPlaylist)
	if err != nil {
		t.Fatalf("First update failed: %v", err)
	}

	expectedAdded1 := []string{"Es Salsa", "Wilfrido", "Que Tienes Ahi"}
	if !reflect.DeepEqual(added, expectedAdded1) {
		t.Errorf("First update added mismatch.\nGot: %v\nExpected: %v", added, expectedAdded1)
	}
	if len(removed) != 0 {
		t.Errorf("First update should not remove anything. Got: %v", removed)
	}

	// 驗證當前 Active 歌單
	active, err := zdb.GetActiveLivePlaylist(userID)
	if err != nil {
		t.Fatalf("Get active failed: %v", err)
	}
	if !reflect.DeepEqual(getSongNames(active), expectedAdded1) {
		t.Errorf("Active playlist mismatch after first update.\nGot: %v\nExpected: %v", getSongNames(active), expectedAdded1)
	}

	// ----------------------------------------------------
	// 測試 2: 更新 Live 歌單 (有新增、有移除)
	// ----------------------------------------------------
	secondPlaylist := []string{
		"Es Salsa", // 保留
		// "Wilfrido" 被移除
		"Que Tienes Ahi", // 保留
		"Voltaje",        // 新增
	}

	added, removed, _, err = zdb.UpdateLivePlaylist(userID, secondPlaylist)
	if err != nil {
		t.Fatalf("Second update failed: %v", err)
	}

	expectedAdded2 := []string{"Voltaje"}
	expectedRemoved2 := []string{"Wilfrido"}

	if !reflect.DeepEqual(added, expectedAdded2) {
		t.Errorf("Second update added mismatch. Got: %v", added)
	}
	if !reflect.DeepEqual(removed, expectedRemoved2) {
		t.Errorf("Second update removed mismatch. Got: %v", removed)
	}

	// 驗證當前 Active 歌單是否正確更新
	active, err = zdb.GetActiveLivePlaylist(userID)
	if err != nil {
		t.Fatalf("Get active failed: %v", err)
	}
	expectedActive2 := []string{"Es Salsa", "Que Tienes Ahi", "Voltaje"}
	if !reflect.DeepEqual(getSongNames(active), expectedActive2) {
		t.Errorf("Active playlist mismatch after second update.\nGot: %v\nExpected: %v", getSongNames(active), expectedActive2)
	}

	// ----------------------------------------------------
	// 測試 3: 多次使用同一首歌 (把 Wilfrido 再加回來)
	// ----------------------------------------------------
	thirdPlaylist := []string{
		"Es Salsa",
		"Que Tienes Ahi",
		"Voltaje",
		"Wilfrido", // 再次加回
	}
	added, removed, _, err = zdb.UpdateLivePlaylist(userID, thirdPlaylist)
	if err != nil {
		t.Fatalf("Third update failed: %v", err)
	}
	if !reflect.DeepEqual(added, []string{"Wilfrido"}) {
		t.Errorf("Third update added mismatch. Got: %v", added)
	}
	if len(removed) != 0 {
		t.Errorf("Third update removed mismatch. Got: %v", removed)
	}

	// ----------------------------------------------------
	// 測試 4: 匯入 ZIN 教材並查詢使用狀況
	// ----------------------------------------------------
	zinSongs := []parser.ZinSong{
		{Prefix: "#123", SongName: "Es Salsa"},
		{Prefix: "#123", SongName: "Wilfrido"},
		{Prefix: "", SongName: "Que Tienes Ahi"},
		{Prefix: "#123", SongName: "Virou Baile"}, // 未在 live 歌單使用過
	}

	err = zdb.AddZinAlbum("2026/07", "Zin123", "2027/6月教材", zinSongs)
	if err != nil {
		t.Fatalf("Failed to add ZIN album: %v", err)
	}

	// 查詢 ZIN 教材狀態
	res, err := zdb.QueryZinStatus(userID, "Zin123")
	if err != nil {
		t.Fatalf("Query ZIN status failed: %v", err)
	}

	if res.AlbumName != "Zin123" || res.ReleaseMonth != "2026/07" || res.Description != "2027/6月教材" {
		t.Errorf("ZIN album metadata mismatch: %+v", res)
	}

	// 預期歌曲狀態：
	// 1. Es Salsa: Used=true, History=1
	// 2. Wilfrido: Used=true, History=2 (因為有兩段使用歷史)
	// 3. Que Tienes Ahi: Used=true, History=1
	// 4. Virou Baile: Used=false, History=0
	expectedStatus := map[string]struct {
		prefix    string
		used      bool
		histCount int
	}{
		"Es Salsa":       {"#123", true, 1},
		"Wilfrido":       {"#123", true, 2},
		"Que Tienes Ahi": {"", true, 1},
		"Virou Baile":    {"#123", false, 0},
	}

	if len(res.Songs) != 4 {
		t.Errorf("Expected 4 songs in ZIN status, got %d", len(res.Songs))
	}

	for _, song := range res.Songs {
		exp, ok := expectedStatus[song.DisplayName]
		if !ok {
			t.Errorf("Unexpected song in status: %s", song.DisplayName)
			continue
		}
		if song.Prefix != exp.prefix {
			t.Errorf("Song %s prefix = %q, expected %q", song.DisplayName, song.Prefix, exp.prefix)
		}
		if song.Used != exp.used {
			t.Errorf("Song %s Used = %v, expected %v", song.DisplayName, song.Used, exp.used)
		}
		if len(song.History) != exp.histCount {
			t.Errorf("Song %s History count = %d, expected %d. History: %v", song.DisplayName, len(song.History), exp.histCount, song.History)
		}
	}
}

func TestZumbaDB_MultiUser(t *testing.T) {
	zdb, err := NewDB(":memory:")
	if err != nil {
		t.Fatalf("Failed to create in-memory DB: %v", err)
	}
	defer zdb.Close()

	user1 := int64(1111)
	user2 := int64(2222)

	// User 1 使用歌單 A
	playlist1 := []string{"Es Salsa", "Wilfrido"}
	_, _, _, err = zdb.UpdateLivePlaylist(user1, playlist1)
	if err != nil {
		t.Fatalf("User 1 update failed: %v", err)
	}

	// User 2 使用歌單 B
	playlist2 := []string{"Que Tienes Ahi"}
	_, _, _, err = zdb.UpdateLivePlaylist(user2, playlist2)
	if err != nil {
		t.Fatalf("User 2 update failed: %v", err)
	}

	// 驗證 user1 歌單獨立
	active1, err := zdb.GetActiveLivePlaylist(user1)
	if err != nil {
		t.Fatalf("User 1 get active failed: %v", err)
	}
	if !reflect.DeepEqual(getSongNames(active1), []string{"Es Salsa", "Wilfrido"}) {
		t.Errorf("User 1 active playlist mismatch: %v", getSongNames(active1))
	}

	// 驗證 user2 歌單獨立
	active2, err := zdb.GetActiveLivePlaylist(user2)
	if err != nil {
		t.Fatalf("User 2 get active failed: %v", err)
	}
	if !reflect.DeepEqual(getSongNames(active2), []string{"Que Tienes Ahi"}) {
		t.Errorf("User 2 active playlist mismatch: %v", getSongNames(active2))
	}

	// 匯入教材 ZIN 123，包含三首歌
	zinSongs := []parser.ZinSong{
		{Prefix: "#123", SongName: "Es Salsa"},
		{Prefix: "#123", SongName: "Wilfrido"},
		{Prefix: "", SongName: "Que Tienes Ahi"},
	}
	err = zdb.AddZinAlbum("2026/07", "Zin123", "2027/6月教材", zinSongs)
	if err != nil {
		t.Fatalf("Failed to add ZIN: %v", err)
	}

	// 查詢 User 1 的 ZIN 123 狀況
	res1, err := zdb.QueryZinStatus(user1, "Zin123")
	if err != nil {
		t.Fatalf("Query status for user1 failed: %v", err)
	}
	// User 1 應該用過 Es Salsa, Wilfrido，沒用過 Que Tienes Ahi
	for _, s := range res1.Songs {
		switch s.DisplayName {
		case "Es Salsa", "Wilfrido":
			if !s.Used {
				t.Errorf("User 1 should have used %s", s.DisplayName)
			}
		case "Que Tienes Ahi":
			if s.Used {
				t.Errorf("User 1 should NOT have used %s", s.DisplayName)
			}
		}
	}

	// 查詢 User 2 的 ZIN 123 狀況
	res2, err := zdb.QueryZinStatus(user2, "Zin123")
	if err != nil {
		t.Fatalf("Query status for user2 failed: %v", err)
	}
	// User 2 應該只用過 Que Tienes Ahi，沒用過 Es Salsa, Wilfrido
	for _, s := range res2.Songs {
		switch s.DisplayName {
		case "Es Salsa", "Wilfrido":
			if s.Used {
				t.Errorf("User 2 should NOT have used %s", s.DisplayName)
			}
		case "Que Tienes Ahi":
			if !s.Used {
				t.Errorf("User 2 should have used %s", s.DisplayName)
			}
		}
	}
}

func TestLivePlaylistPreservesOrderWithoutRecreatingHistory(t *testing.T) {
	zdb, err := NewDB(":memory:")
	if err != nil {
		t.Fatalf("Failed to create in-memory DB: %v", err)
	}
	defer zdb.Close()

	userID := int64(3333)
	initial := []string{"Warm Up", "Main Song", "Cool Down"}
	if _, _, _, err := zdb.UpdateLivePlaylist(userID, initial); err != nil {
		t.Fatalf("Initial update failed: %v", err)
	}

	reordered := []string{"Main Song", "Warm Up", "Cool Down"}
	added, removed, orderChanged, err := zdb.UpdateLivePlaylist(userID, reordered)
	if err != nil {
		t.Fatalf("Reorder failed: %v", err)
	}
	if len(added) != 0 || len(removed) != 0 {
		t.Fatalf("Reordering should not add or remove songs: added=%v removed=%v", added, removed)
	}
	if !orderChanged {
		t.Fatal("Expected reordered playlist to report an order change")
	}

	active, err := zdb.GetActiveLivePlaylist(userID)
	if err != nil {
		t.Fatalf("Get active failed: %v", err)
	}
	if !reflect.DeepEqual(getSongNames(active), reordered) {
		t.Fatalf("Playlist order mismatch: got=%v expected=%v", getSongNames(active), reordered)
	}
	for i, song := range active {
		if song.Position != i+1 {
			t.Errorf("Song %q position=%d, expected=%d", song.DisplayName, song.Position, i+1)
		}
	}

	var historyCount int
	if err := zdb.conn.QueryRow("SELECT COUNT(*) FROM live_history WHERE user_id = ?", userID).Scan(&historyCount); err != nil {
		t.Fatalf("Count history failed: %v", err)
	}
	if historyCount != len(initial) {
		t.Errorf("Reordering created history rows: got=%d expected=%d", historyCount, len(initial))
	}
}

func TestLivePlaylistRejectsDuplicateSongs(t *testing.T) {
	zdb, err := NewDB(":memory:")
	if err != nil {
		t.Fatalf("Failed to create in-memory DB: %v", err)
	}
	defer zdb.Close()

	_, _, _, err = zdb.UpdateLivePlaylist(4444, []string{"Es Salsa", "es salsa"})
	if err == nil {
		t.Fatal("Expected duplicate song error")
	}

	var historyCount int
	if err := zdb.conn.QueryRow("SELECT COUNT(*) FROM live_history").Scan(&historyCount); err != nil {
		t.Fatalf("Count history failed: %v", err)
	}
	if historyCount != 0 {
		t.Errorf("Duplicate playlist should roll back, got %d history rows", historyCount)
	}
}

func TestNewDBMigratesLegacyLivePlaylistPosition(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "legacy.db")
	legacy, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("Open legacy DB failed: %v", err)
	}
	_, err = legacy.Exec(`
		CREATE TABLE songs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT UNIQUE NOT NULL,
			display_name TEXT NOT NULL
		);
		CREATE TABLE live_history (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER NOT NULL,
			song_id INTEGER NOT NULL,
			start_date TEXT NOT NULL,
			end_date TEXT
		);
		INSERT INTO songs (name, display_name) VALUES ('warm up', 'Warm Up'), ('cool down', 'Cool Down');
		INSERT INTO live_history (user_id, song_id, start_date, end_date)
		VALUES (5555, 1, '2026-08-01', NULL), (5555, 2, '2026-08-02', NULL);
	`)
	if err != nil {
		legacy.Close()
		t.Fatalf("Prepare legacy DB failed: %v", err)
	}
	if err := legacy.Close(); err != nil {
		t.Fatalf("Close legacy DB failed: %v", err)
	}

	zdb, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("Migrate legacy DB failed: %v", err)
	}
	defer zdb.Close()

	active, err := zdb.GetActiveLivePlaylist(5555)
	if err != nil {
		t.Fatalf("Get migrated playlist failed: %v", err)
	}
	if !reflect.DeepEqual(getSongNames(active), []string{"Warm Up", "Cool Down"}) {
		t.Fatalf("Migrated order mismatch: %v", getSongNames(active))
	}
	if active[0].Position != 1 || active[1].Position != 2 {
		t.Fatalf("Migrated positions mismatch: %+v", active)
	}
}

func TestProgramImportAndQueryMegaMix(t *testing.T) {
	zdb, err := NewDB(":memory:")
	if err != nil {
		t.Fatalf("Failed to create in-memory DB: %v", err)
	}
	defer zdb.Close()

	program := &parser.ProgramImport{
		FormatVersion: 1,
		Type:          "MM",
		Issue:         "114",
		Title:         "Mega Mix 114",
		Tracks: []parser.ProgramTrack{
			{Sequence: 1, SongName: "Hoy No Me Llamen", Artist: "Pipo Daniel", BPM: 101, DurationSeconds: 191},
			{Sequence: 2, SongName: "Princeso", Artist: "Briella", BPM: 126, DurationSeconds: 204, Style: "Merengue"},
			{Sequence: 3, SongName: "Que Tienes Ahi", Artist: "Zumba", BPM: 135, DurationSeconds: 176, Style: "Caribbean Fusion"},
		},
	}
	if err := zdb.AddProgramRelease(program); err != nil {
		t.Fatalf("AddProgramRelease failed: %v", err)
	}

	releases, err := zdb.ListProgramReleases()
	if err != nil {
		t.Fatalf("ListProgramReleases failed: %v", err)
	}
	if len(releases) != 1 || releases[0].Type != "MM" || releases[0].Issue != "114" || releases[0].TrackCount != 3 {
		t.Fatalf("Unexpected releases: %+v", releases)
	}

	userID := int64(6666)
	if _, _, _, err := zdb.UpdateLivePlaylist(userID, []string{"Princeso"}); err != nil {
		t.Fatalf("Update Live failed: %v", err)
	}
	status, err := zdb.QueryProgramStatus(userID, "mm", "114")
	if err != nil {
		t.Fatalf("QueryProgramStatus failed: %v", err)
	}
	if status == nil || len(status.Tracks) != 3 {
		t.Fatalf("Unexpected program status: %+v", status)
	}
	if status.Tracks[0].DisplayName != "Hoy No Me Llamen" || status.Tracks[1].DisplayName != "Princeso" {
		t.Errorf("Track order mismatch: %+v", status.Tracks)
	}
	if !status.Tracks[1].Used || len(status.Tracks[1].History) != 1 {
		t.Errorf("Expected Princeso usage history: %+v", status.Tracks[1])
	}
}

func TestListProgramReleasesUsesImportOrderNewestFirst(t *testing.T) {
	zdb, err := NewDB(":memory:")
	if err != nil {
		t.Fatalf("Failed to create in-memory DB: %v", err)
	}
	defer zdb.Close()

	newProgram := func(programType, issue, song string) *parser.ProgramImport {
		return &parser.ProgramImport{
			FormatVersion: 1,
			Type:          programType,
			Issue:         issue,
			Title:         programType + " " + issue,
			Tracks: []parser.ProgramTrack{
				{Sequence: 1, SongName: song},
			},
		}
	}

	if err := zdb.AddProgramRelease(newProgram("MM", "113", "First Song")); err != nil {
		t.Fatalf("Add first program failed: %v", err)
	}
	if err := zdb.AddProgramRelease(newProgram("ZIN", "124", "Second Song")); err != nil {
		t.Fatalf("Add second program failed: %v", err)
	}

	releases, err := zdb.ListProgramReleases()
	if err != nil {
		t.Fatalf("ListProgramReleases failed: %v", err)
	}
	if len(releases) != 2 || releases[0].Type != "ZIN" || releases[0].Issue != "124" || releases[1].Issue != "113" {
		t.Fatalf("Expected newest import first, got: %+v", releases)
	}

	// 覆蓋舊教材時保留其首次匯入位置，不應移到最前面。
	if err := zdb.AddProgramRelease(newProgram("MM", "113", "First Song Updated")); err != nil {
		t.Fatalf("Overwrite first program failed: %v", err)
	}
	releases, err = zdb.ListProgramReleases()
	if err != nil {
		t.Fatalf("ListProgramReleases after overwrite failed: %v", err)
	}
	if releases[0].Type != "ZIN" || releases[1].Type != "MM" {
		t.Fatalf("Overwrite changed initial import order: %+v", releases)
	}
}

func TestZumbaDB_FuzzyMatching(t *testing.T) {
	zdb, err := NewDB(":memory:")
	if err != nil {
		t.Fatalf("Failed to create in-memory DB: %v", err)
	}
	defer zdb.Close()

	userID := int64(1234)

	// 1. 初始化一個包含長歌名的 Live 歌單
	playlist1 := []string{
		"Joga Bonito (World Cup Anthems) - DOSE",
		"Voltaje",
	}
	_, _, _, err = zdb.UpdateLivePlaylist(userID, playlist1)
	if err != nil {
		t.Fatalf("Failed to initialize: %v", err)
	}

	// 2. 更新為帶有極微小 typos (相似度 >= 95%) 的歌名
	// "Joga Bonito (World Cup Anthems) - DOS" 比對 "Joga Bonito (World Cup Anthems) - DOSE" 相似度 97.3%
	// 系統應自動判定為同一首歌，回傳無任何變動
	playlist2 := []string{
		"Joga Bonito (World Cup Anthems) - DOS",
		"Voltaje",
	}
	added, removed, _, err := zdb.UpdateLivePlaylist(userID, playlist2)
	if err != nil {
		t.Fatalf("Fuzzy update failed: %v", err)
	}

	if len(added) != 0 || len(removed) != 0 {
		t.Errorf("Expected fuzzy match to unify songs, but got added: %v, removed: %v", added, removed)
	}

	// 3. 更新為相似度低於 95% 的歌名
	// "Voltaje 2" 比對 "Voltaje" 相似度 77.7% (低於 95%)
	// 系統應將其視為一首獨立的新歌，並移除舊歌 Voltaje
	playlist3 := []string{
		"Joga Bonito (World Cup Anthems) - DOSE",
		"Voltaje 2",
	}
	added, removed, _, err = zdb.UpdateLivePlaylist(userID, playlist3)
	if err != nil {
		t.Fatalf("Normal update failed: %v", err)
	}

	if !reflect.DeepEqual(added, []string{"Voltaje 2"}) {
		t.Errorf("Expected to add 'Voltaje 2', got: %v", added)
	}
	if !reflect.DeepEqual(removed, []string{"Voltaje"}) {
		t.Errorf("Expected to remove 'Voltaje', got: %v", removed)
	}
}

func getSongNames(songs []LiveSongInfo) []string {
	var names []string
	for _, s := range songs {
		names = append(names, s.DisplayName)
	}
	return names
}
