package config

import (
	"bufio"
	"os"
	"strconv"
	"strings"
)

// Config 儲存應用程式的設定資訊
type Config struct {
	BotToken     string
	DBPath       string
	AllowedUsers map[int64]bool
}

// LoadConfig 載入設定，優先讀取 .env，再讀取環境變數
func LoadConfig() (*Config, error) {
	// 嘗試讀取同目錄下的 .env 檔案並載入環境變數
	_ = loadEnvFile(".env")

	botToken := os.Getenv("TELEGRAM_BOT_TOKEN")
	dbPath := os.Getenv("DATABASE_PATH")
	if dbPath == "" {
		dbPath = "zumba.db" // 預設資料庫名稱
	}

	allowedUsers := make(map[int64]bool)
	allowedUsersStr := os.Getenv("ALLOWED_USERS")
	if allowedUsersStr != "" {
		ids := strings.Split(allowedUsersStr, ",")
		for _, idStr := range ids {
			idStr = strings.TrimSpace(idStr)
			if idStr == "" {
				continue
			}
			id, err := strconv.ParseInt(idStr, 10, 64)
			if err == nil {
				allowedUsers[id] = true
			}
		}
	}

	return &Config{
		BotToken:     botToken,
		DBPath:       dbPath,
		AllowedUsers: allowedUsers,
	}, nil
}

// loadEnvFile 讀取 .env 檔案並設定環境變數
func loadEnvFile(filename string) error {
	file, err := os.Open(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		// 忽略空行與註解
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			key := strings.TrimSpace(parts[0])
			val := strings.TrimSpace(parts[1])
			// 去除可能包覆的值引號
			val = strings.Trim(val, `"'`)
			os.Setenv(key, val)
		}
	}
	return scanner.Err()
}
