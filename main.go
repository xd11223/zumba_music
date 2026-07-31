package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	"zumba_music/bot"
	"zumba_music/config"
	"zumba_music/db"
)

func main() {
	log.Println("Starting Zumba Music Telegram Bot...")

	// 1. 載入設定檔與環境變數
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("Error loading config: %v", err)
	}

	if cfg.BotToken == "" {
		log.Println("=========================================================================")
		log.Println("❌ 錯誤: 未偵測到 TELEGRAM_BOT_TOKEN 設定！")
		log.Println("請在環境變數中設定 TELEGRAM_BOT_TOKEN，或在專案根目錄下建立 .env 檔案並寫入：")
		log.Println("TELEGRAM_BOT_TOKEN=your_telegram_bot_token_here")
		log.Println("=========================================================================")
		os.Exit(1)
	}

	// 2. 初始化 SQLite 資料庫
	log.Printf("Connecting to SQLite database at: %s", cfg.DBPath)
	zdb, err := db.NewDB(cfg.DBPath)
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer func() {
		log.Println("Closing database connection...")
		if err := zdb.Close(); err != nil {
			log.Printf("Error closing database: %v", err)
		}
	}()

	// 3. 初始化 Telegram Bot
	zbot, err := bot.NewBot(cfg.BotToken, zdb, cfg.AllowedUsers)
	if err != nil {
		log.Fatalf("Failed to initialize Telegram Bot: %v", err)
	}

	// 4. 監聽系統退出訊號以確保資源釋放
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigChan
		log.Printf("Received signal %v, shutting down...", sig)
		// 由於 zdb 的 defer 會在 main 退出時執行，我們在這裡直接 exit 即可觸發
		os.Exit(0)
	}()

	// 5. 啟動 Bot
	log.Println("Telegram Bot is running! Press Ctrl+C to stop.")
	zbot.Start()
}
