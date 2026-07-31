package bot

import (
	"fmt"
	"log"
	"strings"

	"zumba_music/db"
	"zumba_music/parser"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// ZumbaBot 封裝 Telegram Bot 的實例與邏輯
type ZumbaBot struct {
	api          *tgbotapi.BotAPI
	db           *db.ZumbaDB
	allowedUsers map[int64]bool
}

// NewBot 建立並初始化 ZumbaBot，支援白名單設定
func NewBot(token string, zdb *db.ZumbaDB, allowedUsers map[int64]bool) (*ZumbaBot, error) {
	api, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize telegram bot: %w", err)
	}

	log.Printf("Authorized on account %s", api.Self.UserName)

	return &ZumbaBot{
		api:          api,
		db:           zdb,
		allowedUsers: allowedUsers,
	}, nil
}

// Start 啟動 Bot 的訊息輪詢監聽
func (b *ZumbaBot) Start() {
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates := b.api.GetUpdatesChan(u)

	for update := range updates {
		if update.Message != nil {
			b.handleMessage(update.Message)
		} else if update.CallbackQuery != nil {
			b.handleCallbackQuery(update.CallbackQuery)
		}
	}
}

// isAllowed 判斷使用者是否在白名單中
func (b *ZumbaBot) isAllowed(userID int64) bool {
	if len(b.allowedUsers) == 0 {
		return true // 若未設定白名單，預設不攔截
	}
	return b.allowedUsers[userID]
}

// sendNotAllowed 當權限不足時，提示使用者其 Telegram ID 方便加入白名單
func (b *ZumbaBot) sendNotAllowed(chatID int64, userID int64) {
	text := fmt.Sprintf("❌ *您沒有權限使用此 Bot。*\n\n您的 Telegram ID 是：`%d`\n\n如需使用，請聯絡管理員將您的 ID 加入白名單。", userID)
	reply := tgbotapi.NewMessage(chatID, text)
	reply.ParseMode = tgbotapi.ModeMarkdown
	b.send(reply)
}

// handleMessage 處理一般文字訊息或指令
func (b *ZumbaBot) handleMessage(msg *tgbotapi.Message) {
	// 1. 權限攔截
	if !b.isAllowed(msg.From.ID) {
		b.sendNotAllowed(msg.Chat.ID, msg.From.ID)
		return
	}

	text := strings.TrimSpace(msg.Text)
	if text == "" {
		return
	}

	// 根據常駐底部鍵盤傳入的文字做轉發
	switch text {
	case "📊 目前 Live 歌單":
		b.cmdStatus(msg.Chat.ID, msg.From.ID)
		return
	case "📚 瀏覽 ZIN 教材":
		b.cmdListZin(msg.Chat.ID)
		return
	case "ℹ️ 使用說明":
		b.cmdHelp(msg.Chat.ID)
		return
	}

	// 處理 Slash Commands
	if msg.IsCommand() {
		switch msg.Command() {
		case "start", "help":
			b.cmdHelp(msg.Chat.ID)
		case "status":
			b.cmdStatus(msg.Chat.ID, msg.From.ID)
		case "update_live":
			b.cmdUpdateLive(msg.Chat.ID, msg.From.ID, text)
		case "add_zin":
			b.cmdAddZin(msg.Chat.ID, text)
		case "query_zin":
			b.cmdQueryZin(msg.Chat.ID, msg.From.ID, msg.CommandArguments())
		case "list_zin":
			b.cmdListZin(msg.Chat.ID)
		default:
			reply := tgbotapi.NewMessage(msg.Chat.ID, "❌ 未知的指令，請輸入 /help 查看使用說明。")
			b.send(reply)
		}
		return
	}

	// 處理沒有以 '/' 開頭，但可能是直接貼上歌單的更新
	if strings.Contains(text, "\n") {
		reply := tgbotapi.NewMessage(msg.Chat.ID, "💡 偵測到您發送了多行文字。如果是要「更新 Live 歌單」，請在歌單最前面加上 `/update_live` ；若是要「匯入 ZIN 教材」，請在最前面加上 `/add_zin`。")
		reply.ParseMode = tgbotapi.ModeMarkdown
		b.send(reply)
	}
}

// handleCallbackQuery 處理內嵌按鈕的點擊回呼事件
func (b *ZumbaBot) handleCallbackQuery(cb *tgbotapi.CallbackQuery) {
	// 1. 權限攔截
	if !b.isAllowed(cb.From.ID) {
		b.sendNotAllowed(cb.Message.Chat.ID, cb.From.ID)
		_, _ = b.api.Request(tgbotapi.NewCallback(cb.ID, "❌ 權限不足"))
		return
	}

	data := cb.Data

	// 處理 ZIN 查詢的 Callback：格式為 "query_zin:<ZIN名稱>"
	if strings.HasPrefix(data, "query_zin:") {
		albumName := strings.TrimPrefix(data, "query_zin:")
		
		// 呼叫 ZIN 查詢邏輯，帶入點擊使用者的 ID
		replyText := b.formatQueryZinResult(cb.From.ID, albumName)

		// 發送詳細資料回對話框
		msg := tgbotapi.NewMessage(cb.Message.Chat.ID, replyText)
		msg.ParseMode = tgbotapi.ModeMarkdown
		b.send(msg)

		// 回應 Telegram 完成 Callback 動態
		callbackResp := tgbotapi.NewCallback(cb.ID, fmt.Sprintf("已查詢 %s", albumName))
		if _, err := b.api.Request(callbackResp); err != nil {
			log.Printf("Failed to answer callback query: %v", err)
		}
	}
}

// send 輔助函數：發送訊息並記錄錯誤
func (b *ZumbaBot) send(c tgbotapi.Chattable) {
	if _, err := b.api.Send(c); err != nil {
		log.Printf("Error sending message to Telegram: %v", err)
	}
}

// ----------------------------------------------------
// 指令處理函數
// ----------------------------------------------------

// cmdHelp 回覆使用說明，並附帶常駐底部鍵盤
func (b *ZumbaBot) cmdHelp(chatID int64) {
	helpText := `🎵 *Zumba Live 歌單管理 Bot* 🎵

您可以透過此 Bot 記錄與更新 Live 歌單，並與 ZIN 教材歌單比對使用狀況。

*📌 指令說明：*
1️⃣ *查看 Live 歌單：* 點擊下方按鈕或輸入 */status*
2️⃣ *更新 Live 歌單：*
   請輸入 */update_live* 並換行貼上新的 Live 歌單。系統會自動比對：
   • 出現在新歌單中 ➡️ *新增使用* (start_date)
   • 沒出現在新歌單中 ➡️ *移除使用* (end_date)
   *輸入格式範例：*
   ` + "```" + `
   /update_live
   "Joga Bonito - DOSE"
   "Es Salsa"
   "Wilfrido"
   ` + "```" + `
3️⃣ *匯入 ZIN 每月教材：*
   請輸入 */add_zin* 並換行貼上教材資訊。
   *輸入格式範例：*
   ` + "```" + `
   /add_zin
   2026/07
   Zin123(2027/6月教材）
   #123"Es Salsa"
   "Que Tienes Ahi"
   #MM114(2027/7月）"Princeso"
   ` + "```" + `
4️⃣ *查詢 ZIN 使用狀況：* 
   輸入 */query_zin <ZIN名稱>* (例如 */query_zin Zin123*)，或輸入 */list_zin* 點選教材按鈕。`

	msg := tgbotapi.NewMessage(chatID, helpText)
	msg.ParseMode = tgbotapi.ModeMarkdown

	// 建立常駐底部鍵盤 (Reply Keyboard)
	keyboard := tgbotapi.NewReplyKeyboard(
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton("📊 目前 Live 歌單"),
			tgbotapi.NewKeyboardButton("📚 瀏覽 ZIN 教材"),
		),
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton("ℹ️ 使用說明"),
		),
	)
	keyboard.ResizeKeyboard = true // 自動調整大小以適應螢幕
	msg.ReplyMarkup = keyboard

	b.send(msg)
}

// cmdStatus 顯示指定使用者當前使用中 (Active) 的 Live 歌單
func (b *ZumbaBot) cmdStatus(chatID int64, userID int64) {
	songs, err := b.db.GetActiveLivePlaylist(userID)
	if err != nil {
		msg := tgbotapi.NewMessage(chatID, fmt.Sprintf("❌ 獲取 Live 歌單失敗: %v", err))
		b.send(msg)
		return
	}

	if len(songs) == 0 {
		msg := tgbotapi.NewMessage(chatID, "📝 目前 Live 歌單中沒有歌曲。請使用 `/update_live` 開始新增歌曲！")
		b.send(msg)
		return
	}

	var sb strings.Builder
	sb.WriteString("📊 *目前正在使用的 Live Zumba 歌單* (共 ")
	sb.WriteString(fmt.Sprintf("%d", len(songs)))
	sb.WriteString(" 首)：\n\n")

	for i, song := range songs {
		sb.WriteString(fmt.Sprintf("%d. *%s* (%s)\n", i+1, song.DisplayName, formatShortDate(song.StartDate)))
	}

	msg := tgbotapi.NewMessage(chatID, sb.String())
	msg.ParseMode = tgbotapi.ModeMarkdown
	b.send(msg)
}

// cmdUpdateLive 處理 Live 歌單更新 (限定指定使用者)
func (b *ZumbaBot) cmdUpdateLive(chatID int64, userID int64, rawText string) {
	songs := parser.ParseLivePlaylist(rawText)
	if len(songs) == 0 {
		msg := tgbotapi.NewMessage(chatID, "❌ 請在 `/update_live` 指令後面換行並貼上您的歌單。例如：\n`/update_live\n\"Es Salsa\"\n\"Voltaje\"`")
		msg.ParseMode = tgbotapi.ModeMarkdown
		b.send(msg)
		return
	}

	added, removed, err := b.db.UpdateLivePlaylist(userID, songs)
	if err != nil {
		msg := tgbotapi.NewMessage(chatID, fmt.Sprintf("❌ 更新 Live 歌單失敗: %v", err))
		b.send(msg)
		return
	}

	var sb strings.Builder
	sb.WriteString("✅ *Live 歌單更新成功！*\n\n")

	if len(added) > 0 {
		sb.WriteString("➕ *新增的歌曲：*\n")
		for _, song := range added {
			sb.WriteString(fmt.Sprintf("• %s\n", song))
		}
		sb.WriteString("\n")
	}

	if len(removed) > 0 {
		sb.WriteString("➖ *移除的歌曲：*\n")
		for _, song := range removed {
			sb.WriteString(fmt.Sprintf("• %s\n", song))
		}
		sb.WriteString("\n")
	}

	if len(added) == 0 && len(removed) == 0 {
		sb.WriteString("💡 歌單內容與原先一致，無任何變更。\n\n")
	}

	// 順便顯示更新後的當前歌單
	activeSongs, err := b.db.GetActiveLivePlaylist(userID)
	if err == nil && len(activeSongs) > 0 {
		sb.WriteString("📋 *更新後的 Live 歌單：*\n")
		for i, song := range activeSongs {
			sb.WriteString(fmt.Sprintf("%d. *%s* (%s)\n", i+1, song.DisplayName, formatShortDate(song.StartDate)))
		}
	}

	msg := tgbotapi.NewMessage(chatID, sb.String())
	msg.ParseMode = tgbotapi.ModeMarkdown
	b.send(msg)
}

// cmdAddZin 處理 ZIN 教材歌單的匯入 (共用資源，無須隔離)
func (b *ZumbaBot) cmdAddZin(chatID int64, rawText string) {
	month, albumName, desc, songs, err := parser.ParseZinInput(rawText)
	if err != nil || albumName == "" || len(songs) == 0 {
		msg := tgbotapi.NewMessage(chatID, "❌ 請確認 `/add_zin` 輸入格式是否正確。必須包含發行年月、教材名稱，以及歌曲列表。例如：\n`/add_zin\n2026/07\nZin123(2027/6月教材）\n#123\"Es Salsa\"`")
		msg.ParseMode = tgbotapi.ModeMarkdown
		b.send(msg)
		return
	}

	err = b.db.AddZinAlbum(month, albumName, desc, songs)
	if err != nil {
		msg := tgbotapi.NewMessage(chatID, fmt.Sprintf("❌ 匯入 ZIN 教材失敗: %v", err))
		b.send(msg)
		return
	}

	replyText := fmt.Sprintf("💾 *ZIN 教材匯入成功！*\n• 教材期數: *%s*\n• 發行月份: %s\n• 說明: %s\n• 歌曲數: %d 首", albumName, month, desc, len(songs))
	msg := tgbotapi.NewMessage(chatID, replyText)
	msg.ParseMode = tgbotapi.ModeMarkdown
	b.send(msg)
}

// cmdListZin 列出所有 ZIN 教材並附帶內嵌按鈕
func (b *ZumbaBot) cmdListZin(chatID int64) {
	albums, err := b.db.ListZinAlbums()
	if err != nil {
		msg := tgbotapi.NewMessage(chatID, fmt.Sprintf("❌ 獲取 ZIN 教材列表失敗: %v", err))
		b.send(msg)
		return
	}

	if len(albums) == 0 {
		msg := tgbotapi.NewMessage(chatID, "📚 目前沒有已記錄的 ZIN 教材。請先以 `/add_zin` 匯入！")
		b.send(msg)
		return
	}

	var sb strings.Builder
	sb.WriteString("📚 *已記錄的 Zumba ZIN 教材期數：*\n\n")
	for _, a := range albums {
		descText := ""
		if a.Description != "" {
			descText = fmt.Sprintf(" (%s)", a.Description)
		}
		sb.WriteString(fmt.Sprintf("• *%s* - %s%s\n", a.Name, a.ReleaseMonth, descText))
	}
	sb.WriteString("\n*請點選下方按鈕直接查詢各期教材的歌曲使用狀態：*")

	msg := tgbotapi.NewMessage(chatID, sb.String())
	msg.ParseMode = tgbotapi.ModeMarkdown

	// 建立內嵌鍵盤 (Inline Keyboard) 按鈕
	var rows [][]tgbotapi.InlineKeyboardButton
	var currentRow []tgbotapi.InlineKeyboardButton
	
	for i, a := range albums {
		btn := tgbotapi.NewInlineKeyboardButtonData(a.Name, fmt.Sprintf("query_zin:%s", a.Name))
		currentRow = append(currentRow, btn)
		
		// 每行放置 2 個按鈕
		if (i+1)%2 == 0 || i == len(albums)-1 {
			rows = append(rows, currentRow)
			currentRow = []tgbotapi.InlineKeyboardButton{}
		}
	}

	msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(rows...)
	b.send(msg)
}

// cmdQueryZin 處理手動查詢特定 ZIN 教材
func (b *ZumbaBot) cmdQueryZin(chatID int64, userID int64, arguments string) {
	albumName := strings.TrimSpace(arguments)
	if albumName == "" {
		msg := tgbotapi.NewMessage(chatID, "❌ 請指定要查詢的教材名稱。例如：`/query_zin Zin123`")
		msg.ParseMode = tgbotapi.ModeMarkdown
		b.send(msg)
		return
	}

	replyText := b.formatQueryZinResult(userID, albumName)
	msg := tgbotapi.NewMessage(chatID, replyText)
	msg.ParseMode = tgbotapi.ModeMarkdown
	b.send(msg)
}

// formatQueryZinResult 格式化特定 ZIN 專輯的使用狀態查詢結果 (對應特定使用者)
func (b *ZumbaBot) formatQueryZinResult(userID int64, albumName string) string {
	res, err := b.db.QueryZinStatus(userID, albumName)
	if err != nil {
		return fmt.Sprintf("❌ 查詢資料庫時發生錯誤: %v", err)
	}
	if res == nil {
		return fmt.Sprintf("❌ 找不到名為 *%s* 的 ZIN 教材。請先確認名稱是否正確！", albumName)
	}

	var sb strings.Builder
	descText := ""
	if res.Description != "" {
		descText = fmt.Sprintf(" (%s)", res.Description)
	}
	sb.WriteString(fmt.Sprintf("📖 *Zumba ZIN 教材查詢結果*\n"))
	sb.WriteString(fmt.Sprintf("📂 教材名稱: *%s*%s\n", res.AlbumName, descText))
	sb.WriteString(fmt.Sprintf("📅 發行年月: %s\n", res.ReleaseMonth))
	sb.WriteString(strings.Repeat("—", 18) + "\n\n")

	var usedSongs []string
	var unusedSongs []string

	for _, song := range res.Songs {
		// 組合前綴與歌名
		songLine := ""
		if song.Prefix != "" {
			songLine = fmt.Sprintf("`%s` *%s*", song.Prefix, song.DisplayName)
		} else {
			songLine = fmt.Sprintf("*%s*", song.DisplayName)
		}

		if song.Used {
			historyText := strings.Join(song.History, "、")
			usedSongs = append(usedSongs, fmt.Sprintf("• %s\n  └ ⏰ _使用時間: %s_", songLine, historyText))
		} else {
			unusedSongs = append(unusedSongs, fmt.Sprintf("• %s", songLine))
		}
	}

	sb.WriteString(fmt.Sprintf("✅ *已使用歌曲* (共 %d 首)：\n", len(usedSongs)))
	if len(usedSongs) > 0 {
		sb.WriteString(strings.Join(usedSongs, "\n") + "\n\n")
	} else {
		sb.WriteString("_無使用紀錄_\n\n")
	}

	sb.WriteString(fmt.Sprintf("❌ *未使用歌曲* (共 %d 首)：\n", len(unusedSongs)))
	if len(unusedSongs) > 0 {
		sb.WriteString(strings.Join(unusedSongs, "\n") + "\n")
	} else {
		sb.WriteString("_本期所有歌曲皆已使用過！_\n")
	}

	return sb.String()
}

// formatShortDate 將 YYYY-MM-DD (2026-07-31) 格式化為 YY/MM/DD (26/07/31)
func formatShortDate(dateStr string) string {
	if len(dateStr) == 10 && dateStr[4] == '-' && dateStr[7] == '-' {
		return dateStr[2:4] + "/" + dateStr[5:7] + "/" + dateStr[8:10]
	}
	return dateStr
}
