package parser

import (
	"errors"
	"regexp"
	"strings"
)

// ZinSong 代表解析後的 ZIN 教材歌曲資訊
type ZinSong struct {
	Prefix   string // 例如 "#123", "#MM114(2027/7月)" 等，可為空
	SongName string // 清理後的歌名
}

var (
	// zinSongRegex 用於解析 ZIN 教材單行歌曲。
	// 第一個 capture group 匹配選用前綴 (#開頭且不含空白與左引號)
	// 第二個 capture group 匹配歌名本身 (不含引號的內容)
	zinSongRegex = regexp.MustCompile(`^(#[^\s"“'‘]+)?\s*["“'‘]?([^"”'’]+)["”'’]?$`)

	// zinAlbumRegex 用於解析教材名稱與描述。例如 "Zin123(2027/6月教材）"
	// 第一個 capture group 匹配名稱
	// 第二個 capture group 匹配括號內的描述
	zinAlbumRegex = regexp.MustCompile(`^([^(（]+)[(（]([^)）]+)[)）]?`)
)

// CleanSongName 清除歌名頭尾的引號 (半角/全角單雙引號) 與空白
func CleanSongName(name string) string {
	name = strings.TrimSpace(name)
	// 定義要清除的引號字元
	quotes := `"'“` + "`" + `”‘’`
	name = strings.Trim(name, quotes)
	return strings.TrimSpace(name)
}

// NormalizeSongName 將歌名標準化 (轉小寫並去首尾空白)，主要用於資料庫唯一鍵比對
func NormalizeSongName(name string) string {
	return strings.ToLower(CleanSongName(name))
}

// ParseLivePlaylist 解析使用者輸入的 Live 歌單，回傳清理後的歌名列表
func ParseLivePlaylist(input string) []string {
	var songs []string
	lines := strings.Split(input, "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		// 忽略空行與指令行本身
		if line == "" || strings.HasPrefix(line, "/") {
			continue
		}
		cleaned := CleanSongName(line)
		if cleaned != "" {
			songs = append(songs, cleaned)
		}
	}

	return songs
}

// ParseZinSong 解析單行 ZIN 歌曲，回傳前綴標記與清理後的歌名
func ParseZinSong(line string) ZinSong {
	line = strings.TrimSpace(line)
	matches := zinSongRegex.FindStringSubmatch(line)
	if len(matches) < 3 {
		// 若 regex 未完全匹配，直接退化為清理歌名
		return ZinSong{
			Prefix:   "",
			SongName: CleanSongName(line),
		}
	}

	prefix := strings.TrimSpace(matches[1])
	songName := CleanSongName(matches[2])

	return ZinSong{
		Prefix:   prefix,
		SongName: songName,
	}
}

// ParseZinInput 解析完整的 ZIN 教材輸入文字
// 格式：
// [指令 /add_zin (可選)]
// 2026/07
// Zin123(2027/6月教材）
// #123"Es Salsa"
// ...
func ParseZinInput(input string) (month string, albumName string, description string, songs []ZinSong, err error) {
	lines := strings.Split(input, "\n")
	var contentLines []string

	// 先過濾掉指令行與多餘空行，只保留內容行
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "/") {
			continue
		}
		contentLines = append(contentLines, line)
	}

	if len(contentLines) < 2 {
		return "", "", "", nil, errors.New("invalid ZIN input: at least 2 lines required")
	}

	// 第一行：發行年月 (如 2026/07)
	month = strings.TrimSpace(contentLines[0])

	// 第二行：教材名稱與描述 (如 Zin123(2027/6月教材）)
	albumLine := strings.TrimSpace(contentLines[1])
	albumMatches := zinAlbumRegex.FindStringSubmatch(albumLine)
	if len(albumMatches) >= 3 {
		albumName = strings.TrimSpace(albumMatches[1])
		description = strings.TrimSpace(albumMatches[2])
	} else {
		albumName = albumLine
		description = ""
	}

	// 第三行之後：歌曲清單
	for i := 2; i < len(contentLines); i++ {
		song := ParseZinSong(contentLines[i])
		if song.SongName != "" {
			songs = append(songs, song)
		}
	}

	return month, albumName, description, songs, nil
}

// LevenshteinDistance 計算兩個 []rune 之間的編輯距離 (適用於中英文 Unicode)
func LevenshteinDistance(s, t []rune) int {
	d := make([][]int, len(s)+1)
	for i := range d {
		d[i] = make([]int, len(t)+1)
	}

	for i := 0; i <= len(s); i++ {
		d[i][0] = i
	}
	for j := 0; j <= len(t); j++ {
		d[0][j] = j
	}

	for i := 1; i <= len(s); i++ {
		for j := 1; j <= len(t); j++ {
			cost := 0
			if s[i-1] != t[j-1] {
				cost = 1
			}
			d[i][j] = minInt(
				d[i-1][j]+1,      // deletion
				minInt(
					d[i][j-1]+1,  // insertion
					d[i-1][j-1]+cost, // substitution
				),
			)
		}
	}
	return d[len(s)][len(t)]
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// CalculateSimilarity 計算兩個字串的相似度百分比 (0.0 到 1.0)
func CalculateSimilarity(sStr, tStr string) float64 {
	s := []rune(sStr)
	t := []rune(tStr)

	if len(s) == 0 && len(t) == 0 {
		return 1.0
	}
	maxLen := len(s)
	if len(t) > maxLen {
		maxLen = len(t)
	}

	dist := LevenshteinDistance(s, t)
	return 1.0 - float64(dist)/float64(maxLen)
}
