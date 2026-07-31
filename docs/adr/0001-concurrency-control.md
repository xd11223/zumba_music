# 0001. 使用 Go Mutex 進行 SQLite 併發控制

## 狀態
已接受 (Accepted)

## 上下文
我們的 Telegram Bot 採用 SQLite 作為本機資料庫。由於 Telegram Bot 訊息處理器是非同步多執行緒運作，當多個使用者同時發送更新歌單或匯入教材的請求時，SQLite 可能會因為併發寫入而觸發 `database is locked` 錯誤，導致資料庫事務 (Transaction) 失敗。

雖然 SQLite 可以透過啟用 WAL (Write-Ahead Logging) 模式來改善讀寫併發，但這會增加資料庫檔案管理（會產生 .db-wal 與 .db-shm 暫存檔）與連線配置的複雜度。

## 決策
我們決定在 Go 語言層面解決此併發寫入問題。
在資料庫封裝結構 `ZumbaDB` 中引入 Go 標準庫的 `sync.Mutex`（互斥鎖）。對於所有涉及寫入與修改資料庫的事務操作（如 `UpdateLivePlaylist` 與 `AddZinAlbum`），在函式進入點進行 Mutex 鎖定，確保在任何時間點，只有一個執行緒能對 SQLite 進行寫入，從而在記憶體中實現寫入操作的序列化 (Serialization)。

## 後果
- **優點**：
  - 實作極為簡單（僅需數行 Go 程式碼），維護成本極低。
  - 完全防範了 SQLite 併發事務衝突導致的鎖定錯誤。
  - 不需要對 SQLite 進行複雜的連線池或 journal_mode 參數調優。
- **缺點**：
  - 寫入操作會變成排隊執行。但在 Zumba 歌單管理 Bot 的情境下，使用者寫入頻率極低（每天數次），此效能瓶頸在實務上完全可以忽略。
