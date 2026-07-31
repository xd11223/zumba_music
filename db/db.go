package db

import (
	"database/sql"
	"fmt"
	"sync"
	"time"

	"zumba_music/parser"

	_ "modernc.org/sqlite"
)

// ZumbaDB 封裝 SQLite 資料庫操作
type ZumbaDB struct {
	conn *sql.DB
	mu   sync.Mutex
}

// ZinAlbumInfo 儲存教材的簡短資訊
type ZinAlbumInfo struct {
	Name         string
	ReleaseMonth string
	Description  string
}

// ZinSongStatus 代表單首教材歌曲的 live 使用狀況
type ZinSongStatus struct {
	Prefix      string
	DisplayName string
	Used        bool
	History     []string // 使用期間歷史，例如 "2026-06-15 ~ 2026-06-17" 或 "2026-07-15 ~ 使用中"
}

// ZinStatusResult 包含整期教材的查詢結果
type ZinStatusResult struct {
	AlbumName    string
	ReleaseMonth string
	Description  string
	Songs        []ZinSongStatus
}

// LiveSongInfo 儲存使用中歌曲的顯示名稱與啟用日期
type LiveSongInfo struct {
	DisplayName string
	StartDate   string
}

// NewDB 初始化並建立資料庫連線，同時建立所需的 Table
func NewDB(dbPath string) (*ZumbaDB, error) {
	conn, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// 啟用外鍵支援
	_, err = conn.Exec("PRAGMA foreign_keys = ON;")
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to enable foreign keys: %w", err)
	}

	zdb := &ZumbaDB{conn: conn}
	if err := zdb.createTables(); err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to create tables: %w", err)
	}

	return zdb, nil
}

// Close 關閉資料庫連線
func (db *ZumbaDB) Close() error {
	return db.conn.Close()
}

// createTables 建立系統所需的資料表
func (db *ZumbaDB) createTables() error {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS songs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT UNIQUE NOT NULL,
			display_name TEXT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS live_history (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER NOT NULL,
			song_id INTEGER NOT NULL,
			start_date TEXT NOT NULL,
			end_date TEXT,
			FOREIGN KEY(song_id) REFERENCES songs(id)
		);`,
		`CREATE INDEX IF NOT EXISTS idx_live_history_user ON live_history(user_id);`,
		`CREATE TABLE IF NOT EXISTS zin_albums (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT UNIQUE NOT NULL,
			release_month TEXT NOT NULL,
			description TEXT
		);`,
		`CREATE TABLE IF NOT EXISTS zin_songs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			album_id INTEGER NOT NULL,
			song_id INTEGER NOT NULL,
			prefix_tag TEXT,
			FOREIGN KEY(album_id) REFERENCES zin_albums(id),
			FOREIGN KEY(song_id) REFERENCES songs(id),
			UNIQUE(album_id, song_id)
		);`,
	}

	for _, query := range queries {
		if _, err := db.conn.Exec(query); err != nil {
			return err
		}
	}
	return nil
}

// getOrCreateSongTx 在 transaction 中取得或建立歌曲，回傳 song_id 與其原始顯示名稱
func (db *ZumbaDB) getOrCreateSongTx(tx *sql.Tx, displayName string) (int64, string, error) {
	normName := parser.NormalizeSongName(displayName)
	cleanedName := parser.CleanSongName(displayName)

	var id int64
	var existingDisplay string
	// 1. 精確比對
	err := tx.QueryRow("SELECT id, display_name FROM songs WHERE name = ?", normName).Scan(&id, &existingDisplay)
	if err == nil {
		return id, existingDisplay, nil
	} else if err != sql.ErrNoRows {
		return 0, "", err
	}

	// 2. 模糊比對：若精確比對不到，撈出所有歌曲計算 Levenshtein 相似度
	rows, err := tx.Query("SELECT id, name, display_name FROM songs")
	if err == nil {
		defer rows.Close()
		var bestMatchId int64
		var bestMatchDisplay string
		bestScore := 0.0

		for rows.Next() {
			var oid int64
			var oname string
			var odisplay string
			if err := rows.Scan(&oid, &oname, &odisplay); err == nil {
				score := parser.CalculateSimilarity(normName, oname)
				if score >= 0.95 && score > bestScore {
					bestScore = score
					bestMatchId = oid
					bestMatchDisplay = odisplay
				}
			}
		}

		if bestScore >= 0.95 {
			// 找到相似度高於 95% 的歌，自動關聯舊歌
			return bestMatchId, bestMatchDisplay, nil
		}
	}

	// 3. 歌曲不存在且無高相似度歌曲，新增
	res, err := tx.Exec("INSERT INTO songs (name, display_name) VALUES (?, ?)", normName, cleanedName)
	if err != nil {
		return 0, "", err
	}
	lastId, err := res.LastInsertId()
	if err != nil {
		return 0, "", err
	}
	return lastId, cleanedName, nil
}

// UpdateLivePlaylist 更新當前指定使用者的 Live 歌單。
// 會比對目前該使用者正在使用 (end_date IS NULL) 的歌曲與傳入的新歌單，
// 找出「新增的歌曲」與「移除的歌曲」，並記錄進 live_history 中。
func (db *ZumbaDB) UpdateLivePlaylist(userID int64, newSongNames []string) (added []string, removed []string, err error) {
	db.mu.Lock()
	defer db.mu.Unlock()

	tx, err := db.conn.Begin()
	if err != nil {
		return nil, nil, err
	}
	defer tx.Rollback()

	today := time.Now().Format("2006-01-02")

	// 1. 解析傳入的歌單，轉換成 song_id 集合與顯示名稱 map
	newSongIds := make(map[int64]string)
	for _, name := range newSongNames {
		id, finalName, err := db.getOrCreateSongTx(tx, name)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to process song %q: %w", name, err)
		}
		newSongIds[id] = finalName
	}

	// 2. 獲取該使用者當前 Active (使用中，無結束日期) 的歌曲列表
	// 結構：live_history_id -> song_id
	type activeRecord struct {
		historyId int64
		songId    int64
		name      string
	}
	var activeList []activeRecord
	rows, err := tx.Query(`
		SELECT lh.id, lh.song_id, s.display_name 
		FROM live_history lh 
		JOIN songs s ON lh.song_id = s.id 
		WHERE lh.user_id = ? AND lh.end_date IS NULL
	`, userID)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	activeMap := make(map[int64]int64) // song_id -> history_id
	for rows.Next() {
		var rec activeRecord
		if err := rows.Scan(&rec.historyId, &rec.songId, &rec.name); err != nil {
			return nil, nil, err
		}
		activeList = append(activeList, rec)
		activeMap[rec.songId] = rec.historyId
	}

	// 3. 找出新增的歌：按照傳入歌單的原始順序遍歷，確保排序穩定
	addedSet := make(map[int64]bool)
	for _, name := range newSongNames {
		normName := parser.NormalizeSongName(name)
		var songId int64
		err := tx.QueryRow("SELECT id FROM songs WHERE name = ?", normName).Scan(&songId)
		if err != nil {
			continue 
		}

		if _, exists := activeMap[songId]; !exists {
			if !addedSet[songId] {
				// 新增使用紀錄
				_, err = tx.Exec("INSERT INTO live_history (user_id, song_id, start_date, end_date) VALUES (?, ?, ?, NULL)", userID, songId, today)
				if err != nil {
					return nil, nil, fmt.Errorf("failed to insert live history for %q: %w", name, err)
				}
				addedSet[songId] = true
				added = append(added, newSongIds[songId])
			}
		}
	}

	// 4. 找出移除的歌：在 activeMap 但不在 newSongIds
	for _, rec := range activeList {
		if _, exists := newSongIds[rec.songId]; !exists {
			// 填寫結束使用日期
			_, err = tx.Exec("UPDATE live_history SET end_date = ? WHERE id = ?", today, rec.historyId)
			if err != nil {
				return nil, nil, fmt.Errorf("failed to update live history end date for %q: %w", rec.name, err)
			}
			removed = append(removed, rec.name)
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, nil, err
	}

	return added, removed, nil
}

// GetActiveLivePlaylist 獲取指定使用者目前正在使用的 Live 歌單與其加入日期 (以開始時間排序)
func (db *ZumbaDB) GetActiveLivePlaylist(userID int64) ([]LiveSongInfo, error) {
	rows, err := db.conn.Query(`
		SELECT s.display_name, lh.start_date
		FROM live_history lh 
		JOIN songs s ON lh.song_id = s.id 
		WHERE lh.user_id = ? AND lh.end_date IS NULL
		ORDER BY lh.start_date ASC, lh.id ASC
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var songs []LiveSongInfo
	for rows.Next() {
		var song LiveSongInfo
		if err := rows.Scan(&song.DisplayName, &song.StartDate); err != nil {
			return nil, err
		}
		songs = append(songs, song)
	}
	return songs, nil
}

// AddZinAlbum 匯入 ZIN 教材專輯與其歌單。如果專輯已存在，會更新其發行年月與描述，並覆蓋其歌單。
func (db *ZumbaDB) AddZinAlbum(releaseMonth, albumName, description string, zinSongs []parser.ZinSong) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	tx, err := db.conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// 1. 插入或更新 ZIN 教材主表
	var albumId int64
	err = tx.QueryRow("SELECT id FROM zin_albums WHERE name = ?", albumName).Scan(&albumId)
	if err == sql.ErrNoRows {
		res, err := tx.Exec("INSERT INTO zin_albums (name, release_month, description) VALUES (?, ?, ?)", albumName, releaseMonth, description)
		if err != nil {
			return err
		}
		albumId, err = res.LastInsertId()
		if err != nil {
			return err
		}
	} else if err == nil {
		_, err = tx.Exec("UPDATE zin_albums SET release_month = ?, description = ? WHERE id = ?", releaseMonth, description, albumId)
		if err != nil {
			return err
		}
	} else {
		return err
	}

	// 2. 清除該專輯現有的關聯歌曲 (用以支援重新匯入覆蓋)
	_, err = tx.Exec("DELETE FROM zin_songs WHERE album_id = ?", albumId)
	if err != nil {
		return err
	}

	// 3. 逐一插入歌曲，並建立與專輯的關聯
	for _, zs := range zinSongs {
		songId, _, err := db.getOrCreateSongTx(tx, zs.SongName)
		if err != nil {
			return fmt.Errorf("failed to process ZIN song %q: %w", zs.SongName, err)
		}

		_, err = tx.Exec("INSERT INTO zin_songs (album_id, song_id, prefix_tag) VALUES (?, ?, ?)", albumId, songId, zs.Prefix)
		if err != nil {
			return fmt.Errorf("failed to associate song %q with album %q: %w", zs.SongName, albumName, err)
		}
	}

	return tx.Commit()
}

// ListZinAlbums 獲取所有 ZIN 教材期數列表
func (db *ZumbaDB) ListZinAlbums() ([]ZinAlbumInfo, error) {
	rows, err := db.conn.Query("SELECT name, release_month, description FROM zin_albums ORDER BY release_month DESC, name DESC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var albums []ZinAlbumInfo
	for rows.Next() {
		var a ZinAlbumInfo
		var desc sql.NullString
		if err := rows.Scan(&a.Name, &a.ReleaseMonth, &desc); err != nil {
			return nil, err
		}
		if desc.Valid {
			a.Description = desc.String
		}
		albums = append(albums, a)
	}
	return albums, nil
}

// QueryZinStatus 查詢指定使用者對某一期 ZIN 教材歌曲的 Live 使用狀況
func (db *ZumbaDB) QueryZinStatus(userID int64, albumName string) (*ZinStatusResult, error) {
	var res ZinStatusResult
	var albumId int64
	var desc sql.NullString

	err := db.conn.QueryRow("SELECT id, name, release_month, description FROM zin_albums WHERE name = ?", albumName).
		Scan(&albumId, &res.AlbumName, &res.ReleaseMonth, &desc)
	if err == sql.ErrNoRows {
		return nil, nil // 教材不存在
	} else if err != nil {
		return nil, err
	}
	if desc.Valid {
		res.Description = desc.String
	}

	// 查詢歌曲與指定使用者的 Live 使用歷史。
	// 使用 LEFT JOIN。一首歌若該使用者有多次 live_history，會產生多筆 row。
	rows, err := db.conn.Query(`
		SELECT 
			zs.prefix_tag,
			s.display_name,
			s.id,
			lh.start_date,
			lh.end_date
		FROM zin_songs zs
		JOIN songs s ON zs.song_id = s.id
		LEFT JOIN live_history lh ON s.id = lh.song_id AND lh.user_id = ?
		WHERE zs.album_id = ?
		ORDER BY zs.id ASC, lh.start_date ASC, lh.id ASC
	`, userID, albumId)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	// 用來合併同一首歌的多次使用紀錄
	type songRecord struct {
		prefix      string
		displayName string
		used        bool
		intervals   []string
	}
	// 由於需要保持歌曲在 ZIN 教材中的原始排序 (zs.id ASC)，我們使用 slice 搭配 map
	var songList []int64
	songMap := make(map[int64]*songRecord)

	for rows.Next() {
		var prefix sql.NullString
		var displayName string
		var songId int64
		var startDate sql.NullString
		var endDate sql.NullString

		if err := rows.Scan(&prefix, &displayName, &songId, &startDate, &endDate); err != nil {
			return nil, err
		}

		rec, exists := songMap[songId]
		if !exists {
			rec = &songRecord{
				prefix:      prefix.String,
				displayName: displayName,
				used:        false,
				intervals:   []string{},
			}
			songMap[songId] = rec
			songList = append(songList, songId)
		}

		if startDate.Valid {
			rec.used = true
			start := startDate.String
			end := "目前使用中"
			if endDate.Valid && endDate.String != "" {
				end = endDate.String
			}
			rec.intervals = append(rec.intervals, fmt.Sprintf("%s ~ %s", start, end))
		}
	}

	// 組裝成最終結果
	for _, songId := range songList {
		rec := songMap[songId]
		res.Songs = append(res.Songs, ZinSongStatus{
			Prefix:      rec.prefix,
			DisplayName: rec.displayName,
			Used:        rec.used,
			History:     rec.intervals,
		})
	}

	return &res, nil
}
