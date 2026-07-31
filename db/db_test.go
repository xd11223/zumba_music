package db

import (
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

	added, removed, err := zdb.UpdateLivePlaylist(userID, firstPlaylist)
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

	added, removed, err = zdb.UpdateLivePlaylist(userID, secondPlaylist)
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
	added, removed, err = zdb.UpdateLivePlaylist(userID, thirdPlaylist)
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
	_, _, err = zdb.UpdateLivePlaylist(user1, playlist1)
	if err != nil {
		t.Fatalf("User 1 update failed: %v", err)
	}

	// User 2 使用歌單 B
	playlist2 := []string{"Que Tienes Ahi"}
	_, _, err = zdb.UpdateLivePlaylist(user2, playlist2)
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
	_, _, err = zdb.UpdateLivePlaylist(userID, playlist1)
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
	added, removed, err := zdb.UpdateLivePlaylist(userID, playlist2)
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
	added, removed, err = zdb.UpdateLivePlaylist(userID, playlist3)
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
