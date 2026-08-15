# Zumba Music Bot

Zumba Music Bot 是一個以 Go 開發的 Telegram Bot，用來管理每位使用者目前正在使用的 Zumba Live 歌單、保留歌曲使用歷史，並比對各期 ZIN 教材的歌曲使用狀況。

## 功能

### Live 歌單管理

- 查看目前正在使用的 Live 歌單與各歌曲加入日期。
- 貼上完整歌單即可更新目前狀態。
- 自動找出新增與移除的歌曲。
- 保留每首歌的多次使用區間歷史。
- 每位 Telegram 使用者的 Live 歌單與使用紀錄彼此獨立。
- 保存完整 Live 歌單的播放順序，並依照輸入行序顯示。
- 預設第一首標示為暖身、最後一首標示為收操。
- 只調整歌曲順序時，不會重建或結束歌曲使用歷史。
- 同一份 Live 歌單不允許重複歌曲。

### ZIN 教材管理

- 匯入 ZIN 教材的發行月份、教材名稱、說明與歌曲清單。
- 保留歌曲的 ZIN 前綴標記，例如 `#123` 或 `#MM114(2027/7月)`。
- 重複匯入同名教材時，更新教材資訊並以新歌單覆蓋原內容。
- 列出所有教材，並透過 Telegram 按鈕快速查詢。
- 查詢某期教材中已使用、未使用的歌曲，以及每首歌的所有使用期間。
- ZIN 教材為全系統共用，但使用狀況依查詢者的個人 Live 歷史計算。

### 圖片辨識教材格式與 Mega Mix

- 支援 `program-import-v1` 固定文字格式。
- 教材圖片可先交由 Codex 辨識，再將整理結果整段貼入 Bot。
- 支援 `TYPE: MM` 與 `TYPE: ZIN`，教材以類型加期數識別。
- `RELEASE_MONTH` 可以留空；期數 `ISSUE` 必填。
- 保存教材歌曲順序、歌手、BPM、時長與曲風等選填資料。
- 再次匯入相同類型與期數時，完整覆蓋該期教材歌曲。
- 可列出教材，並查詢個人對每首教材歌曲的 Live 使用歷史。

### 歌名處理

- 自動移除歌名前後空白與常見的全形、半形引號。
- 以不分大小寫的標準化名稱比對歌曲。
- 使用 Levenshtein Distance 進行模糊比對。
- 相似度達 `95%` 時，自動視為同一首歌曲，以降低細微拼寫差異造成的重複資料。

### 權限與資料安全

- 可透過 Telegram User ID 白名單限制 Bot 使用者。
- 未設定白名單時，預設允許所有使用者。
- 使用 SQLite Transaction 寫入資料。
- 使用 Mutex 序列化寫入操作，避免同時更新造成 SQLite 鎖定衝突。

## 系統需求

- Go 1.25 或相容版本
- Telegram Bot Token
- 不需要另外安裝 SQLite；專案使用純 Go SQLite driver

## 安裝與設定

### 1. 取得專案並安裝相依套件

```bash
git clone <repository-url>
cd zumba_music
go mod download
```

### 2. 建立 Telegram Bot

在 Telegram 中聯絡 [@BotFather](https://t.me/BotFather)，建立 Bot 並取得 API Token。

### 3. 建立環境設定

複製設定範例：

```bash
cp .env.example .env
```

編輯 `.env`：

```dotenv
TELEGRAM_BOT_TOKEN=your_telegram_bot_token_here
DATABASE_PATH=zumba.db
ALLOWED_USERS=123456789,987654321
```

設定項目：

| 變數 | 必填 | 說明 |
| --- | --- | --- |
| `TELEGRAM_BOT_TOKEN` | 是 | 由 BotFather 提供的 Bot Token |
| `DATABASE_PATH` | 否 | SQLite 檔案路徑，未設定時使用 `zumba.db` |
| `ALLOWED_USERS` | 否 | 允許使用的 Telegram User ID，以逗號分隔；留空表示不限制 |

`.env` 會先被載入，既有的同名系統環境變數可能會被 `.env` 中的值取代。

## 啟動方式

在專案根目錄執行：

```bash
go run .
```

看到以下訊息即表示 Bot 已開始透過 long polling 接收 Telegram 訊息：

```text
Telegram Bot is running! Press Ctrl+C to stop.
```

可使用 `Ctrl+C` 停止服務。

若要先編譯執行檔：

```bash
go build -o zumba-music-bot .
./zumba-music-bot
```

## Telegram 使用方式

### 顯示說明

```text
/start
```

或：

```text
/help
```

Bot 會顯示操作說明與常駐鍵盤：

- `📊 目前 Live 歌單`
- `📘 瀏覽 ZIN 教材`
- `📙 瀏覽 Mega Mix 教材`
- `ℹ️ 使用說明`

### 查看目前 Live 歌單

```text
/status
```

Bot 會列出目前使用中的歌曲，以及每首歌加入歌單的日期。

### 更新 Live 歌單

`/update_live` 後換行貼上「更新後的完整歌單」：

```text
/update_live
"Joga Bonito - DOSE"
"Es Salsa"
"Wilfrido"
```

系統會將這份內容視為新的完整狀態：

- 新歌會建立一筆以當天為 `start_date` 的使用紀錄。
- 原本存在但沒有出現在新歌單的歌曲，會以當天填入 `end_date`。
- 仍然存在的歌曲不會重建歷史紀錄。
- 每行的順序就是播放順序；第一首為暖身，最後一首為收操。
- 只改變順序時只會更新位置，不會產生新的使用區間。
- 更新日期以 Bot 收到指令當天為準，不支援指定或補登日期。

> 請勿只貼本次新增的歌曲，否則未出現在訊息中的既有歌曲會被判定為移除。

### 匯入 ZIN 教材

格式依序為：發行年月、教材名稱與選填說明、歌曲清單。

```text
/add_zin
2026/07
Zin123(2027/6月教材）
#123"Es Salsa"
"Que Tienes Ahi"
#MM114(2027/7月）"Princeso"
```

以上內容會解析為：

- 發行年月：`2026/07`
- 教材名稱：`Zin123`
- 說明：`2027/6月教材`
- 歌曲前綴與歌名：例如 `#123`、`Es Salsa`

教材說明可省略，例如：

```text
/add_zin
2026/07
Zin123
"Es Salsa"
```

### 瀏覽新版 ZIN 與 Mega Mix 教材

```text
/list_zin
/list_mm
```

`/list_zin` 只列出新版 ZIN 教材，`/list_mm` 只列出 Mega Mix 教材；兩者都會顯示可直接點擊的期數按鈕。舊版 ZIN 按鈕與 `/query_zin Zin123` 也會自動導向新版的 `ZIN 123` 資料，避免兩套資料來源顯示不一致。

### 查詢 ZIN 使用狀況

```text
/query_zin Zin123
```

查詢結果包含：

- 教材基本資訊。
- 已使用歌曲與所有使用期間。
- 目前仍在 Live 歌單中的歌曲。
- 從未使用過的歌曲。

### 匯入 Mega Mix 或固定格式教材

可直接將完整內容貼給 Bot，不需要加指令；Bot 會偵測 `FORMAT_VERSION:`：

```text
FORMAT_VERSION: 1
TYPE: MM
ISSUE: 114
RELEASE_MONTH:
TITLE: Mega Mix 114

TRACKS:
01 | Hoy No Me Llamen | Pipo Daniel | 101 | 3:11 |
02 | Princeso | Briella | 126 | 3:24 | Merengue
```

每首歌曲固定為：

```text
順序 | 歌名 | 歌手 | BPM | 時長 | 曲風
```

也可以使用：

```text
/add_mm
FORMAT_VERSION: 1
TYPE: MM
...
```

或通用指令 `/add_program`。同一類型與期數再次匯入時，會完整覆蓋該期教材。

### 瀏覽與查詢固定格式教材

```text
/list_programs
```

查詢指定教材：

```text
/query_program MM 114
```

也可以點擊常駐鍵盤的 `📘 瀏覽 ZIN 教材` 或 `📙 瀏覽 Mega Mix 教材`，再選擇教材按鈕。查詢結果以卡片式文字顯示：歌曲名稱使用粗體，並用 `✅`／`⬜` 標示使用狀態；歌手、BPM、時長、曲風與使用歷史分別使用圖示呈現。結果底下也提供 ZIN 與 Mega Mix 的快速瀏覽按鈕。

教材列表依首次匯入時間由新到舊排列；重新匯入並覆蓋既有教材時，不會改變原本的排序位置。

## 資料模型

SQLite 資料庫包含以下資料表：

| 資料表 | 用途 |
| --- | --- |
| `songs` | 共用歌曲主檔與標準化名稱 |
| `live_history` | 每位使用者的歌曲啟用、停用歷史 |
| `program_releases` | ZIN／MM 教材期數、名稱與發行月份 |
| `program_tracks` | 教材內歌曲、順序與曲風 |
| `zin_albums` | ZIN 教材基本資料 |
| `zin_songs` | 教材、歌曲與前綴的關聯 |

資料表會在 Bot 啟動時自動建立。

## 測試

執行所有測試：

```bash
go test ./...
```

目前測試涵蓋：

- 歌名清理與標準化。
- Live 與 ZIN 輸入格式解析。
- Levenshtein 相似度計算。
- Live 歌單新增與移除流程。
- 多使用者資料隔離。
- ZIN 教材匯入與使用狀況查詢。
- 歌名模糊匹配。

## 專案結構

```text
.
├── main.go             # 程式進入點與生命週期
├── bot/bot.go          # Telegram 指令、按鈕與訊息處理
├── config/config.go    # .env 與環境變數設定
├── db/db.go            # SQLite Schema 與資料存取邏輯
├── parser/parser.go    # 輸入解析、歌名清理與模糊比對
├── docs/               # 名詞定義與架構決策紀錄
└── *_test.go           # 自動化測試
```

更完整的商業規則請參考 [`docs/glossary.md`](docs/glossary.md)，架構決策請參考 [`docs/adr`](docs/adr)。
