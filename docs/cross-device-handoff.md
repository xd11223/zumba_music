# Zumba Music Bot 跨電腦 Codex 交接文件

> 最後重新檢查：2026-08-21（Asia/Taipei）  
> 檢查基準 commit：`04e46bd8b0f9f0455fecda700c4c91b25d5372a1`  
> Git 遠端：`https://github.com/xd11223/zumba_music.git`  
> 用途：換電腦後，讓 Codex 不依賴舊對話也能理解目前程式、產品決策與下一步。

## 1. 新電腦上的接手方式

```bash
git clone https://github.com/xd11223/zumba_music.git
cd zumba_music
go mod download
go test ./...
go run .
```

交給新 Codex 時可直接貼這段：

```text
請先完整閱讀 docs/cross-device-handoff.md，再閱讀 README.md、
docs/phase-1-product-spec.md、docs/glossary.md、docs/adr/、
docs/program-image-recognition-format.md 與目前程式碼。
請先執行 git status 和 go test ./...，以實際程式碼與測試為準，
不要把 phase-1-product-spec.md 中未勾選的長期規格誤認為目前 MVP 必須立刻完成。
修改前先告訴我你理解的目前功能、尚未完成項目與準備修改的範圍。
```

## 2. 目前產品定位

目前先完成「一位管理者／老師本人可實際使用」的 Telegram Bot MVP：

- 使用 Go、Telegram Bot、SQLite。
- 管理個人的 Live 歌單與每首歌的使用歷史。
- Live 歌單有播放順序，第一首視為暖身、最後一首視為收操。
- 管理共用的 ZIN 與 Mega Mix（MM）教材。
- 教材圖片暫時由 Codex 讀圖並轉成固定文字，不在 Bot 中串接付費 OCR。
- 使用者人工確認文字後，再貼給 Bot 寫入資料庫。
- 多人角色、登入、草稿確認、Web 儀表板、廣告與訂閱都屬未來功能。

## 3. 已實作功能

### 3.1 Live 歌單

- `/status`：顯示目前使用中的 Live 歌單。
- `/update_live`：後面貼完整的新歌單，每行一首。
- 新歌建立 `start_date`，被移除歌曲填入 `end_date`。
- 日期固定使用 Bot 收到更新的當天，不能補登自訂日期。
- 以 Telegram User ID 隔離各使用者的 Live 歷史。
- 保存輸入行序至 `live_history.position`。
- `/status` 依 `position` 顯示。
- 第一首標示暖身，最後一首標示收操；只有一首時同時標示兩者。
- 只改順序時只更新 `position`，不建立或結束使用歷史。
- 同一份歌單不能出現重複歌曲；解析成相同 song ID 也會拒絕。
- 舊資料庫啟動時會自動新增並回填 `position`。

注意：`/update_live` 接收的是「更新後完整歌單」，不是只有本次新增歌曲。沒有出現在新清單的 active 歌曲會被視為移除。

### 3.2 通用 ZIN／MM 教材

- 支援 `program-import-v1` 固定格式。
- `TYPE` 只允許 `ZIN` 或 `MM`。
- 教材以 `TYPE + ISSUE` 唯一識別，例如 `ZIN 123`、`MM 114`。
- `RELEASE_MONTH` 可留空，不會由期數推測月份。
- 保存教材標題、期數、月份、歌曲順序、歌手、BPM、時長與曲風。
- 同一 `TYPE + ISSUE` 再次匯入時，會直接完整覆蓋教材歌曲關聯。
- 教材依「首次匯入時間由新到舊」瀏覽；覆蓋不改變首次匯入順序。
- 教材查詢會依教材歌曲順序，顯示目前使用者是否使用過及所有 Live 使用期間。
- 教材詳細畫面目前刻意精簡：只顯示歌名、使用狀態與使用日期／歷史，不顯示歌手、BPM、時長、曲風；這些欄位仍有保存。

可直接貼上以 `FORMAT_VERSION:` 開頭的完整內容，不必先打指令。亦支援：

```text
/add_mm
/add_program
/list_zin
/list_mm
/list_programs
/query_zin 123
/query_program ZIN 123
/query_program MM 114
```

底部常駐鍵盤目前包含：

- `📊 目前 Live 歌單`
- `📘 瀏覽 ZIN 教材`
- `📙 瀏覽 Mega Mix 教材`
- `ℹ️ 使用說明`

教材詳細結果下方另有 ZIN／Mega Mix 快速切換按鈕。

### 3.3 圖片轉文字流程

目前固定流程：

```text
使用者把教材截圖貼給 Codex
→ Codex 依 docs/program-image-recognition-format.md 辨識
→ 產生 program-import-v1 Markdown
→ 使用者人工確認
→ 將 FORMAT_VERSION 開頭的 code block 貼給 Bot
→ Bot 解析並寫入 SQLite
```

辨識規則：

- 只填圖片清楚可見的資料。
- 被 `...` 截斷的歌手或曲風留空，不猜測。
- 多圖重疊歌曲去重並保持正確順序。
- 評分、下載狀態、ZF、App 勾選圖示不匯入。
- DJ Mix 是教材提供的獨立音軌，應保留為一首獨立歌曲，但不會自動加入 Live。

現有範例：

- `imports/mm-114.md`
- `imports/zin-123.md`

### 3.4 舊版 ZIN 相容功能

程式仍保留舊格式：

```text
/add_zin
2026/07
Zin123(2027/6月教材）
#123"Es Salsa"
"Que Tienes Ahi"
```

舊資料使用 `zin_albums`、`zin_songs`。但目前 `/list_zin`、`/query_zin` 與舊 Telegram 按鈕已導向新版 `program_releases`／`program_tracks` 查詢，避免 UI 同時顯示兩套來源。

重要限制：舊版 ZIN 資料尚未 migration 到新版通用教材表。只用舊 `/add_zin` 匯入的教材，不會自然出現在新版列表，除非另用 `program-import-v1` 匯入。

### 3.5 歌名比對

- 清除常見半形／全形引號與前後空白。
- 標準化名稱使用小寫字串。
- 先精確匹配；找不到時，以 Levenshtein 相似度比對全部歌曲。
- 相似度 `>= 95%` 時，目前會自動沿用既有 song ID。
- 這是單人 MVP 的既有行為；未來需求已決定模糊匹配應改成人工確認，但尚未實作。

### 3.6 權限與寫入安全

- `.env` 的 `ALLOWED_USERS` 可用逗號列出 Telegram User ID。
- 未設定 `ALLOWED_USERS` 時，目前預設所有人可使用。
- 寫入使用 SQLite Transaction。
- `ZumbaDB` 使用 Go `sync.Mutex` 序列化寫入。
- 尚未有正式 `users` 表、Admin／Instructor 角色或 `ADMIN_USERS`。

## 4. Telegram 訊息路由

主要入口是 `bot/bot.go`：

- 一般訊息先檢查 `ALLOWED_USERS`。
- 常駐鍵盤文字轉到對應 command handler。
- Slash command 由 `handleMessage` switch 分派。
- 非指令文字若以 `FORMAT_VERSION:` 開頭，直接走通用教材匯入。
- Inline callback 支援：
  - `query_program:<TYPE>:<ISSUE>`
  - `list_program:<TYPE>`
  - 舊 `query_zin:<value>`，會轉成新版 ZIN issue。

## 5. 程式結構

```text
main.go
  載入設定、開啟 DB、建立 Telegram Bot、啟動 long polling

config/config.go
  讀取 .env 與環境變數

bot/bot.go
  Telegram 指令、常駐鍵盤、Inline callback、結果格式化

parser/parser.go
  Live／舊 ZIN／program-import-v1 解析
  歌名清理、標準化、Levenshtein 相似度

db/db.go
  SQLite schema、自動 migration、Transaction、Live 與教材查詢

docs/
  產品規格、詞彙、ADR、圖片辨識格式、本交接文件

imports/
  經圖片辨識後的人類可讀＋程式可貼入教材資料
```

目前商業邏輯主要集中在 `bot/bot.go` 與 `db/db.go`，尚未拆成 application service／repository 層。

## 6. SQLite 資料模型

### `songs`

- 共用歌曲主檔。
- `name`：標準化名稱，唯一。
- `display_name`：顯示名稱。
- 啟動時 migration 補上 `artist`、`bpm`、`duration_seconds`。

### `live_history`

- `user_id`、`song_id`、`start_date`、`end_date`、`position`。
- `end_date IS NULL` 代表目前使用中。
- 只保存目前 active 歌單順序，沒有歷史 playlist snapshot。

### `program_releases`

- 新版通用教材主表。
- `program_type + issue` 唯一。
- 保存 `release_month`、`title`、`created_at`、`updated_at`。

### `program_tracks`

- 新版教材與歌曲關聯。
- 保存 `sequence` 與 `style`。
- 同一期不能有重複順序或重複 song ID。

### `zin_albums`、`zin_songs`

- 舊版 `/add_zin` 相容資料表。
- 尚未搬到新版通用教材表。

## 7. 設定與跨電腦資料注意事項

程式會讀取：

```dotenv
TELEGRAM_BOT_TOKEN=...
DATABASE_PATH=zumba_music.db
ALLOWED_USERS=123456789
```

`TELEGRAM_BOT_TOKEN` 必填；`DATABASE_PATH` 未設定時，程式碼預設是 `zumba.db`。

### 重要安全現況

本文件建立後已開始修正：

- `.env` 已執行 `git rm --cached`，本機檔案仍保留，等待 commit 後才會正式停止追蹤。
- 已新增 `.gitignore` 排除 `.env`、`.env.*`，並保留 `.env.example` 可追蹤。
- `zumba_music.db` **被 Git 追蹤**。
- 舊 Token 仍存在 Git 歷史，必須先由 BotFather 產生新 Token，並另行決定是否重寫 Git 歷史。

目前 clone 仍可取得 SQLite 資料；新的 commit 將不再包含 `.env`，但 Telegram Bot Token 仍可能存在 Git 歷史及 GitHub。不要在文件、聊天或終端輸出 Token 值。

建議之後另開一個安全性任務處理：

1. 向 BotFather 旋轉／撤銷舊 Token。
2. 新增 `.gitignore`，排除 `.env`、`*.db`、`*.db-wal`、`*.db-shm`。
3. Git 只保留不含秘密的 `.env.example`。
4. SQLite 改用加密備份、私有雲端或人工複製，不要把正式資料庫放進一般 Git commit。
5. 若 GitHub repository 曾公開，需要評估清理 Git 歷史；只刪目前檔案不會移除歷史 Token。

在使用者明確同意前，不要擅自刪除、停止追蹤或重寫 Git 歷史。

## 8. 目前測試狀態

2026-08-21 已重新執行：

```bash
go test -count=1 ./...
go build -o /tmp/zumba-music-bot .
```

兩者皆通過。

自動測試目前涵蓋：

- Live parser 與 stakeholder 18 首順序。
- MM 114 固定格式解析、空欄位、時長與 escaped pipe。
- program track 順序驗證。
- 舊 ZIN parser。
- Levenshtein 相似度。
- Live 新增、移除、多次歷史與多使用者隔離。
- 純重新排序不重建歷史。
- Live 重複歌曲拒絕。
- 舊資料庫 `position` migration。
- MM 教材匯入、列表、使用歷史查詢。
- 教材首次匯入時間排序。
- Telegram 暖身／收操格式與 ZIN issue 正規化。

不足之處：Bot handler／callback 沒有完整整合測試，通用教材覆蓋行為也缺少針對「移除關聯但保留歌曲與 Live 歷史」的專門測試。

## 9. 已確認但尚未實作

### 單人 MVP 後續最相關

1. 舊 `zin_albums`／`zin_songs` 到通用教材表的 migration。
2. 教材覆蓋前顯示新增、移除、不變差異。
3. 移除教材歌曲關聯前人工確認；不能刪除共用歌曲或 Live 歷史。
4. 將 95% 模糊自動合併改為：完全一致才自動匹配，模糊結果由人確認。
5. `song_aliases` 與匹配衝突處理。
6. 依實際使用情況補 Bot handler／callback 整合測試。

### 未來多人／產品化

- 正式 `users` 表。
- `ADMIN_USERS`、Admin／Instructor 角色與不同鍵盤。
- `/my_account`。
- 使用者只能操作自己的 Live、草稿與 callback。
- 管理員共用教材操作紀錄。
- 教材與 Live 的 `import_sessions` 草稿。
- 寫入前預覽、最終確認、`/resume`、`/cancel`、草稿過期。
- 每首 Live 異動可指定不同日期。
- Telegram 日期選擇與批次日期套用。
- 未來日期、起訖顛倒、重疊歷史驗證。
- 自架 Web 後台、登入帳密、個人儀表板。
- 公開註冊、廣告、訂閱。
- Bot 內圖片 OCR；目前刻意維持 Codex 人工轉文字以避免 API 成本。

## 10. 文件與程式不一致時的判斷原則

- 以目前程式碼、測試與本交接文件為準。
- `docs/phase-1-product-spec.md` 保存完整 grill-me 討論與長期方向，部分 checklist 可能尚未同步最新實作狀態。
- ADR 0002 曾記錄 85% 閾值，但 ADR 0004 與目前程式實際使用 95%。
- ADR 0005 的「Live 無順序」已被 ADR 0006 取代。
- 舊 ZIN API 仍在程式中是相容層，不代表新功能應繼續基於舊表開發。
- 新教材功能應優先使用 `program-import-v1`、`program_releases`、`program_tracks`。

## 11. 下一步建議

維持單人 MVP 的前提下，建議順序：

1. 在實際 Bot 驗收 MM 114、ZIN 123 的匯入、列表與查詢。
2. 確認正式 SQLite 是否已有舊 ZIN 資料需要 migration。
3. 決定是否先處理 Git 中 `.env`／資料庫的安全問題。
4. 若繼續功能開發，優先做舊 ZIN migration，或做教材覆蓋差異預覽；不要直接開始多人角色系統。
5. 每次修改後執行 `gofmt`、`git diff --check`、`go test ./...` 與 `go build`。

## 12. Git 狀態基準

本次盤點開始時：

- 分支：`main`
- 與 `origin/main` 同步
- 工作區乾淨
- 最新 commit：`04e46bd feat: simplify program detail display`

建立本文件後，工作區會只有本文件是尚未 commit 的新變更；除非使用者明確要求，Codex 不應自行 commit 或 push。
