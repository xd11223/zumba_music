package db

import (
	"database/sql"
	"fmt"
	"strings"
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
	Position    int
}

// ProgramReleaseInfo 是教材列表中的摘要資訊。
type ProgramReleaseInfo struct {
	Type         string
	Issue        string
	ReleaseMonth string
	Title        string
	TrackCount   int
}

// ProgramTrackStatus 是教材歌曲及指定使用者的 Live 使用狀況。
type ProgramTrackStatus struct {
	Sequence        int
	DisplayName     string
	Artist          string
	BPM             int
	DurationSeconds int
	Style           string
	Used            bool
	History         []string
}

// ProgramStatusResult 是一期通用教材的完整查詢結果。
type ProgramStatusResult struct {
	Type         string
	Issue        string
	ReleaseMonth string
	Title        string
	Tracks       []ProgramTrackStatus
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
			position INTEGER,
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
		`CREATE TABLE IF NOT EXISTS program_releases (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			program_type TEXT NOT NULL,
			issue TEXT NOT NULL,
			release_month TEXT NOT NULL DEFAULT '',
			title TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(program_type, issue)
		);`,
		`CREATE TABLE IF NOT EXISTS program_tracks (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			program_release_id INTEGER NOT NULL,
			song_id INTEGER NOT NULL,
			sequence INTEGER NOT NULL,
			style TEXT NOT NULL DEFAULT '',
			FOREIGN KEY(program_release_id) REFERENCES program_releases(id) ON DELETE CASCADE,
			FOREIGN KEY(song_id) REFERENCES songs(id),
			UNIQUE(program_release_id, sequence),
			UNIQUE(program_release_id, song_id)
		);`,
	}

	for _, query := range queries {
		if _, err := db.conn.Exec(query); err != nil {
			return err
		}
	}

	if err := db.migrateLivePlaylistPosition(); err != nil {
		return err
	}
	return db.migrateSongMetadata()
}

func (db *ZumbaDB) migrateSongMetadata() error {
	columns := []struct {
		name       string
		definition string
	}{
		{name: "artist", definition: "TEXT NOT NULL DEFAULT ''"},
		{name: "bpm", definition: "INTEGER"},
		{name: "duration_seconds", definition: "INTEGER"},
	}
	for _, column := range columns {
		exists, err := db.tableHasColumn("songs", column.name)
		if err != nil {
			return err
		}
		if !exists {
			query := fmt.Sprintf("ALTER TABLE songs ADD COLUMN %s %s", column.name, column.definition)
			if _, err := db.conn.Exec(query); err != nil {
				return fmt.Errorf("failed to add songs.%s: %w", column.name, err)
			}
		}
	}
	return nil
}

func (db *ZumbaDB) tableHasColumn(tableName, columnName string) (bool, error) {
	rows, err := db.conn.Query("PRAGMA table_info(" + tableName + ")")
	if err != nil {
		return false, err
	}
	defer rows.Close()

	for rows.Next() {
		var cid int
		var name, columnType string
		var notNull, primaryKey int
		var defaultValue sql.NullString
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			return false, err
		}
		if name == columnName {
			return true, nil
		}
	}
	return false, rows.Err()
}

// migrateLivePlaylistPosition 為既有資料庫補上 Live 播放順序，並依舊有顯示順序回填 active 歌單。
func (db *ZumbaDB) migrateLivePlaylistPosition() error {
	rows, err := db.conn.Query("PRAGMA table_info(live_history)")
	if err != nil {
		return err
	}

	hasPosition := false
	for rows.Next() {
		var cid int
		var name, columnType string
		var notNull, primaryKey int
		var defaultValue sql.NullString
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			rows.Close()
			return err
		}
		if name == "position" {
			hasPosition = true
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}

	if !hasPosition {
		if _, err := db.conn.Exec("ALTER TABLE live_history ADD COLUMN position INTEGER"); err != nil {
			return fmt.Errorf("failed to add live playlist position: %w", err)
		}
	}

	_, err = db.conn.Exec(`
		UPDATE live_history AS current
		SET position = (
			SELECT COUNT(*)
			FROM live_history AS previous
			WHERE previous.user_id = current.user_id
			  AND previous.end_date IS NULL
			  AND (
				previous.start_date < current.start_date
				OR (previous.start_date = current.start_date AND previous.id <= current.id)
			  )
		)
		WHERE current.end_date IS NULL
		  AND (current.position IS NULL OR current.position < 1)
	`)
	if err != nil {
		return fmt.Errorf("failed to backfill live playlist positions: %w", err)
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
func (db *ZumbaDB) UpdateLivePlaylist(userID int64, newSongNames []string) (added []string, removed []string, orderChanged bool, err error) {
	db.mu.Lock()
	defer db.mu.Unlock()

	tx, err := db.conn.Begin()
	if err != nil {
		return nil, nil, false, err
	}
	defer tx.Rollback()

	today := time.Now().Format("2006-01-02")

	// 1. 解析傳入歌單並保留順序。相同 song_id 不允許重複出現在同一份 Live 歌單。
	type resolvedSong struct {
		id          int64
		displayName string
		position    int
	}
	var resolvedSongs []resolvedSong
	newSongIds := make(map[int64]string)
	for index, name := range newSongNames {
		id, finalName, err := db.getOrCreateSongTx(tx, name)
		if err != nil {
			return nil, nil, false, fmt.Errorf("failed to process song %q: %w", name, err)
		}
		if _, exists := newSongIds[id]; exists {
			return nil, nil, false, fmt.Errorf("duplicate song in live playlist: %q", finalName)
		}
		newSongIds[id] = finalName
		resolvedSongs = append(resolvedSongs, resolvedSong{
			id:          id,
			displayName: finalName,
			position:    index + 1,
		})
	}

	// 2. 獲取該使用者當前 Active (使用中，無結束日期) 的歌曲列表
	// 結構：live_history_id -> song_id
	type activeRecord struct {
		historyId int64
		songId    int64
		name      string
		position  int
	}
	var activeList []activeRecord
	rows, err := tx.Query(`
		SELECT lh.id, lh.song_id, s.display_name, lh.position
		FROM live_history lh 
		JOIN songs s ON lh.song_id = s.id 
		WHERE lh.user_id = ? AND lh.end_date IS NULL
	`, userID)
	if err != nil {
		return nil, nil, false, err
	}
	defer rows.Close()

	activeMap := make(map[int64]int64) // song_id -> history_id
	for rows.Next() {
		var rec activeRecord
		if err := rows.Scan(&rec.historyId, &rec.songId, &rec.name, &rec.position); err != nil {
			return nil, nil, false, err
		}
		activeList = append(activeList, rec)
		activeMap[rec.songId] = rec.historyId
	}

	// 3. 依輸入順序新增歌曲，或更新既有 active 歌曲的位置。
	for _, song := range resolvedSongs {
		if historyID, exists := activeMap[song.id]; exists {
			for _, rec := range activeList {
				if rec.songId == song.id && rec.position != song.position {
					orderChanged = true
					break
				}
			}
			if _, err := tx.Exec("UPDATE live_history SET position = ? WHERE id = ?", song.position, historyID); err != nil {
				return nil, nil, false, fmt.Errorf("failed to update live position for %q: %w", song.displayName, err)
			}
			continue
		}

		_, err = tx.Exec(`
			INSERT INTO live_history (user_id, song_id, start_date, end_date, position)
			VALUES (?, ?, ?, NULL, ?)
		`, userID, song.id, today, song.position)
		if err != nil {
			return nil, nil, false, fmt.Errorf("failed to insert live history for %q: %w", song.displayName, err)
		}
		added = append(added, song.displayName)
	}

	// 4. 找出移除的歌：在 activeMap 但不在 newSongIds
	for _, rec := range activeList {
		if _, exists := newSongIds[rec.songId]; !exists {
			// 填寫結束使用日期
			_, err = tx.Exec("UPDATE live_history SET end_date = ? WHERE id = ?", today, rec.historyId)
			if err != nil {
				return nil, nil, false, fmt.Errorf("failed to update live history end date for %q: %w", rec.name, err)
			}
			removed = append(removed, rec.name)
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, nil, false, err
	}

	return added, removed, orderChanged, nil
}

// GetActiveLivePlaylist 獲取指定使用者目前正在使用的 Live 歌單與其加入日期，依播放順序排列。
func (db *ZumbaDB) GetActiveLivePlaylist(userID int64) ([]LiveSongInfo, error) {
	rows, err := db.conn.Query(`
		SELECT s.display_name, lh.start_date, lh.position
		FROM live_history lh 
		JOIN songs s ON lh.song_id = s.id 
		WHERE lh.user_id = ? AND lh.end_date IS NULL
		ORDER BY lh.position ASC, lh.id ASC
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var songs []LiveSongInfo
	for rows.Next() {
		var song LiveSongInfo
		if err := rows.Scan(&song.DisplayName, &song.StartDate, &song.Position); err != nil {
			return nil, err
		}
		songs = append(songs, song)
	}
	return songs, nil
}

// AddProgramRelease 新增或完整覆蓋一期 ZIN／MM 教材。
func (db *ZumbaDB) AddProgramRelease(program *parser.ProgramImport) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	tx, err := db.conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var releaseID int64
	err = tx.QueryRow(`
		SELECT id FROM program_releases WHERE program_type = ? AND issue = ?
	`, program.Type, program.Issue).Scan(&releaseID)
	if err == sql.ErrNoRows {
		res, err := tx.Exec(`
			INSERT INTO program_releases (program_type, issue, release_month, title)
			VALUES (?, ?, ?, ?)
		`, program.Type, program.Issue, program.ReleaseMonth, program.Title)
		if err != nil {
			return err
		}
		releaseID, err = res.LastInsertId()
		if err != nil {
			return err
		}
	} else if err == nil {
		if _, err := tx.Exec(`
			UPDATE program_releases
			SET release_month = ?, title = ?, updated_at = CURRENT_TIMESTAMP
			WHERE id = ?
		`, program.ReleaseMonth, program.Title, releaseID); err != nil {
			return err
		}
		if _, err := tx.Exec("DELETE FROM program_tracks WHERE program_release_id = ?", releaseID); err != nil {
			return err
		}
	} else {
		return err
	}

	seenSongs := make(map[int64]bool)
	for _, track := range program.Tracks {
		songID, _, err := db.getOrCreateSongTx(tx, track.SongName)
		if err != nil {
			return fmt.Errorf("failed to process program song %q: %w", track.SongName, err)
		}
		if seenSongs[songID] {
			return fmt.Errorf("duplicate song in program: %q", track.SongName)
		}
		seenSongs[songID] = true

		if _, err := tx.Exec(`
			UPDATE songs
			SET artist = CASE WHEN ? <> '' THEN ? ELSE artist END,
				bpm = CASE WHEN ? > 0 THEN ? ELSE bpm END,
				duration_seconds = CASE WHEN ? > 0 THEN ? ELSE duration_seconds END
			WHERE id = ?
		`, track.Artist, track.Artist, track.BPM, track.BPM,
			track.DurationSeconds, track.DurationSeconds, songID); err != nil {
			return fmt.Errorf("failed to update song metadata for %q: %w", track.SongName, err)
		}

		if _, err := tx.Exec(`
			INSERT INTO program_tracks (program_release_id, song_id, sequence, style)
			VALUES (?, ?, ?, ?)
		`, releaseID, songID, track.Sequence, track.Style); err != nil {
			return fmt.Errorf("failed to add track %q: %w", track.SongName, err)
		}
	}

	return tx.Commit()
}

// ListProgramReleases 依首次匯入時間由新到舊列出教材。
func (db *ZumbaDB) ListProgramReleases() ([]ProgramReleaseInfo, error) {
	rows, err := db.conn.Query(`
		SELECT pr.program_type, pr.issue, pr.release_month, pr.title, COUNT(pt.id)
		FROM program_releases pr
		LEFT JOIN program_tracks pt ON pt.program_release_id = pr.id
		GROUP BY pr.id
		ORDER BY pr.created_at DESC, pr.id DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var releases []ProgramReleaseInfo
	for rows.Next() {
		var release ProgramReleaseInfo
		if err := rows.Scan(&release.Type, &release.Issue, &release.ReleaseMonth, &release.Title, &release.TrackCount); err != nil {
			return nil, err
		}
		releases = append(releases, release)
	}
	return releases, rows.Err()
}

// QueryProgramStatus 查詢一期教材及指定使用者對其中歌曲的使用歷史。
func (db *ZumbaDB) QueryProgramStatus(userID int64, programType, issue string) (*ProgramStatusResult, error) {
	var releaseID int64
	var result ProgramStatusResult
	err := db.conn.QueryRow(`
		SELECT id, program_type, issue, release_month, title
		FROM program_releases
		WHERE program_type = ? AND issue = ?
	`, strings.ToUpper(strings.TrimSpace(programType)), strings.TrimSpace(issue)).
		Scan(&releaseID, &result.Type, &result.Issue, &result.ReleaseMonth, &result.Title)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	rows, err := db.conn.Query(`
		SELECT pt.sequence, s.id, s.display_name, s.artist, s.bpm, s.duration_seconds,
			pt.style, lh.start_date, lh.end_date
		FROM program_tracks pt
		JOIN songs s ON s.id = pt.song_id
		LEFT JOIN live_history lh ON lh.song_id = s.id AND lh.user_id = ?
		WHERE pt.program_release_id = ?
		ORDER BY pt.sequence ASC, lh.start_date ASC, lh.id ASC
	`, userID, releaseID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	trackBySongID := make(map[int64]*ProgramTrackStatus)
	var songOrder []int64
	for rows.Next() {
		var sequence int
		var songID int64
		var displayName, artist, style string
		var bpm, duration sql.NullInt64
		var startDate, endDate sql.NullString
		if err := rows.Scan(&sequence, &songID, &displayName, &artist, &bpm, &duration, &style, &startDate, &endDate); err != nil {
			return nil, err
		}

		track, exists := trackBySongID[songID]
		if !exists {
			track = &ProgramTrackStatus{
				Sequence:    sequence,
				DisplayName: displayName,
				Artist:      artist,
				Style:       style,
			}
			if bpm.Valid {
				track.BPM = int(bpm.Int64)
			}
			if duration.Valid {
				track.DurationSeconds = int(duration.Int64)
			}
			trackBySongID[songID] = track
			songOrder = append(songOrder, songID)
		}
		if startDate.Valid {
			track.Used = true
			end := "目前使用中"
			if endDate.Valid && endDate.String != "" {
				end = endDate.String
			}
			track.History = append(track.History, fmt.Sprintf("%s ~ %s", startDate.String, end))
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for _, songID := range songOrder {
		result.Tracks = append(result.Tracks, *trackBySongID[songID])
	}
	return &result, nil
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
