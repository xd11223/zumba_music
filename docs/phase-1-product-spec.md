# Zumba Music Bot：第一階段產品規格與 Codex 交接紀錄

> 最後更新：2026-08-15
> 文件用途：保存本專案經過 `grill-me` 討論後的產品決策、第一階段預期功能、目前實作進度與下一步。換一台電腦或開啟新的 Codex task 時，請先讀完本文件，再讀 `README.md`、`docs/glossary.md`、`docs/adr/` 與現有程式碼。
> 重要：本文件大部分內容是「已確認、尚未實作的第一階段規格」，不是目前程式已經具備的功能。

## 1. 產品背景

目前專案是一個 Go Telegram Bot，使用 SQLite 保存資料，主要用途是：

- 管理每位 Zumba 老師目前使用中的 Live 歌單。
- 保存歌曲加入與移出 Live 的歷史區間。
- 匯入每月 ZIN 教材。
- 查詢某位老師使用過哪些教材歌曲。

新的產品方向是讓系統未來可以服務多位 Zumba 老師：

- ZIN 與 MM 教材由管理員統一維護，所有老師共用。
- 每位老師的 Live 歌單和使用歷史彼此隔離。
- 第一階段仍以 Telegram Bot 操作。
- 未來可增加自架 Web 管理後台、老師個人儀表板、廣告或訂閱，但都不在第一階段實作。

## 2. Grill-me 討論後的核心決策

### 2.1 保留目前技術方向

第一階段繼續使用：

- Go
- Telegram Bot
- SQLite
- Transaction
- Go Mutex 序列化資料庫寫入

不以 Notion 取代正式資料庫。Notion雖然能快速提供視覺化介面，但長期產品需要完整的權限、歷史、交易一致性及自訂流程，因此未來如需管理介面，改為自架 Web 後台。

### 2.2 第一階段不做圖片 OCR

原始需求曾考慮讓使用者直接傳送 ZIN、MM 或 Live 截圖，由 Bot 自動辨識。討論後決定第一階段暫緩，避免 OpenAI Vision API 或其他 OCR API 的用量費用與額外複雜度。

暫定人工流程：

1. 管理者把圖片貼到 Codex。
2. Codex 協助 OCR、整理、去重並輸出固定文字格式。
3. 管理者把文字複製到 Telegram Bot。
4. Bot 解析、比較、顯示異常與預覽。
5. 人工最終確認後才修改資料庫。

未來若加入自動 OCR，只要讓 OCR 輸出相同的結構，就可以接回既有匯入流程，不需要推翻資料模型。

### 2.3 所有寫入必須先預覽、再確認

教材或 Live 資料都不能在收到文字後立即寫入：

1. 建立草稿。
2. 解析輸入。
3. 比對既有資料。
4. 只逐筆處理異常。
5. 顯示完整變更摘要。
6. 使用者明確確認。
7. 使用單一 Transaction 寫入。

取消、逾時、Bot 重啟或寫入失敗，都不能留下部分正式變更。

## 3. 第一階段功能範圍

### 3.1 通用 ZIN／MM 教材

目前資料表是 ZIN 專用：

```text
zin_albums
zin_songs
```

第一階段改成通用模型：

```text
program_releases
program_tracks
```

教材類型至少支援：

- `ZIN`
- `MM`

教材唯一識別條件：

```text
program_type + release_month
```

例如 `ZIN + 2026/07` 與 `MM + 2026/07` 可以同時存在，但不能存在兩筆 `ZIN + 2026/07`。

建議結構：

```text
program_releases
- id
- program_type       必填，ZIN / MM
- release_month      必填，建議正規化為 YYYY/MM
- release_code       選填，例如 124
- title              選填，例如 Summer State of Mind
- description        選填
- created_at
- updated_at
```

```text
program_tracks
- id
- program_release_id
- song_id
- sequence           教材中的原始順序
- category           選填，例如 Warm-up、Salsa、Cooldown
- prefix_tag         選填，保留舊格式相容能力
- created_at
```

### 3.2 現有 ZIN 資料 Migration

必須安全遷移現有資料：

- `zin_albums` 搬到 `program_releases`，類型設為 `ZIN`。
- `zin_songs` 搬到 `program_tracks`。
- 保留既有歌曲 ID、教材關聯和 Live 歷史。
- Migration 必須可以安全重複執行。
- 不可先刪除舊資料再重建。
- 應先備份實際 SQLite 檔案，再用複本驗證 Migration。

是否在 Migration 完成後立即刪除舊表屬於實作細節；建議第一版先保留舊表或完成資料核對後再移除。

### 3.3 歌曲主檔擴充

建議歌曲主檔：

```text
songs
- id
- normalized_name
- display_name
- artist             選填
- bpm                選填
- duration_seconds   選填
- created_at
- updated_at
```

規則：

- 歌名必填。
- 歌手、BPM、時長為選填。
- 教材內分類屬於 `program_tracks.category`，不是歌曲永久屬性。
- 新輸入沒有某欄位時，不清除既有資料。
- 既有欄位空白、新輸入有值時，可列為「補充資料」。
- 新舊非空值衝突時，必須人工選擇。
- 不保存 App 評分、下載狀態、ZF 或其他狀態圖示。
- 被截斷或不確定的文字不能自行猜測。

### 3.4 歌曲匹配改為保守策略

目前程式在 Levenshtein 相似度達 95% 時會自動合併。第一階段必須取消此行為。

新規則：

- 標準化後完全一致：自動匹配。
- 已確認別名完全一致：自動匹配。
- 只有模糊相似：顯示候選，必須人工確認。
- 一般老師不能直接合併全域歌曲主檔。
- 管理員未來才能執行真正的歌曲合併。

標準化仍包括：

- 去除首尾空白。
- 去除常見全形／半形引號。
- 轉小寫。

### 3.5 個人與全域歌曲別名

建議新增：

```text
song_aliases
- id
- song_id
- normalized_alias
- user_id            全域別名可為 NULL
- scope              global / personal
- created_at
```

規則：

- 管理員在教材匯入時確認的別名，可保存為全域別名。
- 一般老師在 Live 更新時確認的別名，只對本人有效。
- 個人別名不能污染其他使用者的匹配結果。
- 下一次輸入完全符合已確認別名時，可以自動匹配。
- 別名不能與另一首歌曲的正式名稱或別名衝突。

### 3.6 跨教材歌曲

理論上同一首歌不常同時出現在多份教材，但資料模型必須支援此例外：

- `songs` 中仍只有一筆歌曲。
- 一首歌曲可以關聯多個 ZIN／MM 教材。
- Live 使用歷史仍只有一份。
- 查詢任一相關教材時，都顯示該歌曲的真實使用歷史。
- 畫面標示此歌曲同時收錄於其他教材。
- 系統不能猜測老師是「因為哪一份教材」使用該歌。

### 3.7 正式使用者模型

目前 Live 歷史直接保存 Telegram User ID。第一階段新增正式使用者：

```text
users
- id
- telegram_user_id   UNIQUE
- display_name
- username
- role               admin / instructor
- status             active / suspended
- created_at
- updated_at
```

Live 歷史應逐步改為關聯內部 `users.id`。這能讓未來網站帳號、Google 登入或其他身份綁定至同一位老師。

第一階段只實作 Telegram 身份，不實作帳號密碼或 Google 登入。

### 3.8 `.env` 權限

第一階段使用：

```dotenv
ALLOWED_USERS=123456789,987654321
ADMIN_USERS=123456789
```

規則：

- 在 `ADMIN_USERS`：管理員，並同時擁有老師功能。
- 只在 `ALLOWED_USERS`：一般老師。
- 兩者皆不在：拒絕使用並顯示其 Telegram ID。
- 不自動把第一位使用者設為管理員。
- 不提供使用者自行升級管理員的入口。
- 第一階段維持封閉白名單；公開註冊或邀請碼留待未來。

新增「我的帳號」功能，顯示：

- Telegram ID
- 名稱／username
- 角色
- 狀態

### 3.9 Telegram 按鈕

管理員看到：

```text
📘 匯入 ZIN 教材
📙 匯入 MM 教材
🎵 更新我的 Live 歌單
📚 瀏覽教材
📊 我的 Live 歌單
👤 我的帳號
```

一般老師看到：

```text
🎵 更新我的 Live 歌單
📚 瀏覽教材
📊 我的 Live 歌單
👤 我的帳號
```

後端仍必須驗證權限，不能只靠隱藏按鈕。

### 3.10 Commands

按鈕是給一般使用者的直覺入口；管理員仍希望可以用 Commands。

第一階段預計支援：

```text
/start
/help
/status
/update_live
/add_zin
/add_mm
/list_programs
/query_program
/cancel
/resume
/my_account
```

保留舊指令相容：

```text
/list_zin
/query_zin
```

按鈕與 Commands 必須共用相同的 service、草稿、驗證與確認流程。任何寫入型 Command 都不能繞過預覽直接修改資料庫。

## 4. 文字匯入格式

### 4.1 ZIN／MM 教材

教材類型由按鈕或 `/add_zin`、`/add_mm` 決定，文字不用重複指定類型。

建議格式：

```text
月份：2026/07
編號：124
名稱：Summer State of Mind
說明：選填

Movimiento | Cloonee, Yandel, Alac | 129 | 2:36
Tumbala | CHCKN | 130 | 3:01 | Warm-up
Clap Clap | Zumba | 128 | 3:05 | Warm-up
```

每首歌欄位順序：

```text
歌名 | 歌手 | BPM | 時長 | 分類
```

只有歌名必填，其他欄位允許省略或留空。Parser 應容忍全形冒號、前後空白與空白行，但不應猜測缺失欄位。

### 4.2 Live 完整歌單

每行一首，代表「更新後的完整 Live 歌單」：

```text
Turn Up The Bass
Boompala
LEMONADE
Goals
Virou Baile
```

這不是只列新增或移除的差異；Bot 必須用完整清單和目前 active 歌單比較。

## 5. 教材匯入與覆蓋流程

每次 ZIN／MM 匯入都代表該類型、該月份的完整教材歌單。

若資料不存在：

- 顯示準備建立的教材資料。
- 顯示正常歌曲及異常歌曲。
- 最終確認後建立。

若資料已存在：

- 比較新增教材關聯。
- 比較待移除教材關聯。
- 顯示保持不變的歌曲。
- 比較教材基本資料變更。
- 比較歌曲選填欄位補充或衝突。

移除教材關聯：

- 必須明確列出所有歌曲。
- 可以整批確認，不必逐首按一次。
- 移除的只是 `program_tracks` 關聯。
- 不刪除 `songs`。
- 不刪除或改動任何老師的 Live 歷史。
- 若移除很多或風險較高，可以先確認移除，再做最終整體確認。
- 若只有少量且畫面清楚，也可和整體確認合併；但不能隱藏任何移除。

## 6. Live 更新流程

### 6.1 差異比較

Bot 比較新完整歌單與目前 active 歌單：

- 新清單有、目前沒有：待新增。
- 目前有、新清單沒有：待移除。
- 兩邊都有：保持使用中。
- 模糊匹配：要求人工確認。
- 已在使用卻被判定新增：顯示異常，不重複建立 active 歷史。
- 不在使用卻被判定移除：顯示異常，不建立假的歷史。

移除必須清楚列出，因為它會結束目前 active 的使用區間。

### 6.2 每筆異動日期

每個新增或移除項目都有自己的 `effective_date`，不能再固定整批使用 `time.Now()`。

操作方式：

1. 先選一個預設日期，套用所有異動。
2. 預設選項提供「今天」、「昨天」、「其他日期」。
3. 日期不同的歌曲可個別覆寫。
4. 多首相同日期可以多選後批次套用。
5. 最終確認逐首顯示異動類型與日期。

驗證規則：

- 日期不能晚於今天。
- 移除日期不能早於該 active 區間的開始日期。
- 新紀錄不能與既有歷史重疊。
- 不能產生倒置或交錯區間。
- 發現衝突時阻止寫入並要求重新選擇日期。
- 日常 Live 更新不能自動修改舊歷史。
- 歷史人工修正留給未來管理員功能。

## 7. 草稿與確認狀態

建議新增：

```text
import_sessions
- id
- user_id
- import_type        ZIN / MM / LIVE
- status             pending / completed / cancelled / expired
- raw_input
- parsed_payload     JSON
- current_step
- expires_at
- created_at
- updated_at
```

實際實作可再將異常項目或日期拆成正規化子表，但第一階段至少要能完整保存並恢復狀態。

規則：

- 每位使用者同時只能有一個進行中草稿。
- 草稿保留 24 小時。
- Bot 重啟後仍可 `/resume`。
- `/cancel` 取消目前草稿。
- 開始新操作時若已有草稿，先詢問繼續或放棄。
- Telegram Callback 必須包含或能安全解析草稿 ID。
- Callback 仍需重新核對當前 Telegram 使用者是否擁有該草稿。
- 過期、取消或未完成的草稿不能修改正式資料。
- 完成寫入後標記為 `completed`。

### 7.1 審核 UX

正常項目整批顯示，不逐首詢問。

只逐筆處理：

- 模糊歌曲匹配。
- 可能重複歌曲。
- 歌曲欄位衝突。
- 別名衝突。
- 無效或衝突日期。
- 無法解析的輸入。

異常處理完畢後顯示最終摘要：

- 新增歌曲數。
- 更新歌曲資料數。
- 新增／移除教材關聯數。
- Live 新增／移除數及逐首日期。
- 保持不變數量。

只有最終確認後才能開啟正式資料庫 Transaction。

## 8. 權限與資料隔離

### 8.1 個人 Live

- 每位老師只能查看與修改自己的 Live。
- 後端必須從 Telegram Message／Callback 的身份取得目前使用者。
- 不信任文字或 Callback payload 內傳入的 User ID。
- 每個 Live 查詢、更新與草稿讀取都必須限制為目前 `users.id`。
- 一般老師不能修改其他老師的 Live。

### 8.2 共用教材

- ZIN／MM、歌曲主檔和全域別名由管理員維護。
- 一般老師可查詢共用教材。
- 一般老師只能管理自己的 Live 和個人別名。
- 未來可增加「回報錯誤」功能，但不在第一階段。

## 9. 歷史與管理紀錄

個人 Live 第一階段不另外建立完整 audit log，因為 `live_history` 已保存使用區間。

應增加或保存：

- `created_at`
- `updated_at`
- 新增來源 `import_session_id`
- 移除來源 `import_session_id`（欄位名稱可在實作時決定）

共用資料應保存簡單管理紀錄，至少能追查：

- 哪位管理員修改。
- 何時修改。
- 來源草稿。
- 教材、歌曲主檔或全域別名的主要前後差異。

建議資料表可命名為 `admin_audit_events`，但實際 schema 可在不改變上述行為的前提下調整。

## 10. Telegram 預期互動摘要

### 10.1 管理員匯入教材

```text
點擊「匯入 ZIN／MM」或使用 /add_zin、/add_mm
→ 貼上固定格式文字
→ 建立草稿
→ 解析及比對資料庫
→ 處理模糊匹配／欄位衝突
→ 確認待移除教材關聯
→ 顯示最終摘要
→ 確認寫入
```

### 10.2 老師更新 Live

```text
點擊「更新我的 Live 歌單」或使用 /update_live
→ 貼上完整歌單
→ 建立草稿
→ 比對 active 歌單
→ 處理模糊匹配
→ 設定預設日期
→ 必要時調整個別／多首歌曲日期
→ 確認移除及整體摘要
→ 確認寫入
```

### 10.3 草稿恢復

```text
/resume
→ 顯示目前草稿類型、建立時間與所在步驟
→ 繼續處理
```

```text
/cancel
→ 二次確認
→ 標記草稿 cancelled
```

## 11. 第一階段明確不做

- Telegram 圖片上傳與自動 OCR。
- OpenAI API 或其他 Vision API 串接。
- Notion整合。
- Web 管理後台。
- 網站帳號密碼登入。
- Google 登入或其他身份提供者。
- 老師個人網頁儀表板。
- 公開註冊或邀請碼。
- 廣告、訂閱或付款。
- PostgreSQL 遷移。
- 管理員手動修正 Live 歷史的介面。
- 模糊歌名自動合併。
- 一般老師修改共用教材。

## 12. 目前已實作的功能

截至 2026-08-15，程式仍是改版前版本，目前已有：

- Telegram Bot long polling。
- `/start`、`/help`。
- `/status` 查看個人 active Live 歌單。
- `/update_live` 貼上完整歌單並立即比較、寫入。
- `/add_zin` 匯入或覆蓋 ZIN 教材。
- `/list_zin` 列出 ZIN 教材並提供 Inline Keyboard。
- `/query_zin` 查詢個人對某期 ZIN 歌曲的使用歷史。
- `ALLOWED_USERS` 白名單。
- 每位 Telegram User ID 的 Live 資料隔離。
- 歌名清理、標準化。
- Levenshtein 模糊比對。
- SQLite Transaction。
- Go Mutex 序列化寫入。
- Live 多段使用歷史。
- ZIN 教材重新匯入覆蓋。
- Parser 與 DB 自動化測試。
- 完整 README（已在 Git commit `d05adf5 update readme`）。

目前 SQLite schema 仍只有：

```text
songs
live_history
zin_albums
zin_songs
```

## 13. 目前尚未實作的第一階段工作

以下全部尚未開始：

- [ ] 通用 `program_releases`／`program_tracks` schema。
- [ ] 現有 ZIN 資料 migration。
- [ ] MM 教材支援。
- [ ] 歌手、BPM、時長、分類欄位。
- [ ] 正式 `users` 資料表。
- [ ] `ADMIN_USERS` 設定與管理員權限。
- [ ] `song_aliases`。
- [ ] 取消 95% 自動模糊合併。
- [ ] 固定教材文字格式 parser。
- [ ] 教材按類型＋月份查詢。
- [ ] 教材完整清單差異預覽。
- [ ] 教材關聯移除確認。
- [ ] `import_sessions` 草稿。
- [ ] `/resume`、`/cancel`。
- [ ] 所有寫入的預覽與最終確認。
- [ ] Live 每筆異動日期。
- [ ] Telegram 日期選擇／批次日期調整。
- [ ] 歷史日期衝突驗證。
- [ ] 管理員與一般老師的不同鍵盤。
- [ ] `/add_mm`、`/list_programs`、`/query_program`、`/my_account`。
- [ ] 管理員共用資料操作紀錄。
- [ ] 新功能測試與文件更新。

## 14. 建議實作順序

下一個 Codex 不應直接從 Telegram UI 開始。建議依序：

1. 重新讀取全部程式、測試、ADR 與本文件。
2. 執行 `git status`，不要覆蓋使用者尚未提交的修改。
3. 建立現有 SQLite 測試資料庫的備份／複本測試策略。
4. 設計並測試 migration：`users`、通用教材、歌曲擴充。
5. 將 DB API 從 ZIN 專用改成通用 Program service，保留舊 API 相容層。
6. 實作新的歌曲匹配結果模型，不再自動模糊合併。
7. 實作固定教材文字 parser 與相關測試。
8. 實作教材草稿、差異計算和確認後 Transaction。
9. 實作 Live 草稿、差異計算、逐筆日期與歷史驗證。
10. 實作角色權限、鍵盤與 Commands。
11. 實作舊指令相容。
12. 更新 README、`.env.example`、Glossary 與 ADR。
13. 執行完整測試，並用舊資料庫複本驗證 migration 前後筆數和查詢結果。

每個階段都應先完成 DB／service／parser 測試，再接 Telegram 呈現，避免商業邏輯繼續集中在 `bot/bot.go`。

## 15. 建議程式分層

目前主要邏輯集中在 `db/db.go` 和 `bot/bot.go`。為未來 Web API 做準備，第一階段建議逐步拆分：

```text
Telegram handlers
       ↓
Application services
       ↓
Repository / SQLite
```

至少應讓以下邏輯不依賴 Telegram 型別：

- 教材匯入解析及差異計算。
- Live 差異計算。
- 歌曲匹配候選。
- 日期驗證。
- 草稿狀態轉移。
- 最終 Transaction 寫入。

按鈕與 Commands 只負責把輸入轉交 service 並格式化輸出。

## 16. 必要測試清單

- [ ] ZIN 與 MM 同月份可並存。
- [ ] 同類型、同月份不可重複建立。
- [ ] 舊 ZIN 資料完整遷移且不重複。
- [ ] Migration 可重複執行。
- [ ] Admin／Instructor 權限正確。
- [ ] 使用者只能存取自己的 Live 與草稿。
- [ ] 教材完整覆蓋能正確產生新增、移除與不變差異。
- [ ] 移除教材關聯不刪歌曲或 Live 歷史。
- [ ] Live 完整清單差異正確。
- [ ] 完全一致自動匹配。
- [ ] 模糊匹配永遠等待人工確認。
- [ ] 個人別名不影響其他使用者。
- [ ] 全域別名正確套用。
- [ ] 空欄位補充與非空欄位衝突正確。
- [ ] 每筆 Live 異動日期正確保存。
- [ ] 多首批次日期正確套用。
- [ ] 未來日期被拒絕。
- [ ] 移除早於開始日期被拒絕。
- [ ] 重疊或交錯歷史被拒絕。
- [ ] 草稿可恢復、取消、過期。
- [ ] 每位使用者只能有一個 active 草稿。
- [ ] Callback 不能操作他人的草稿。
- [ ] 未最終確認不會修改正式資料。
- [ ] Transaction 中途失敗會完整 Rollback。
- [ ] 舊 Commands 仍可使用。

## 17. 驗收標準

第一階段完成時，至少要能示範：

1. 管理員透過按鈕或 `/add_zin` 貼文字，預覽並建立某月份 ZIN。
2. 管理員透過按鈕或 `/add_mm` 貼文字，預覽並建立同月份 MM。
3. 重複匯入同月份教材時，清楚顯示新增與移除關聯，確認後覆蓋。
4. 一般老師無法看到或呼叫教材寫入功能。
5. 老師貼上完整 Live 歌單，看到新增、移除和不變項目。
6. 老師可為所有異動指定預設日期，再修改特定歌曲日期。
7. 模糊歌名必須人工選擇，不能自動合併。
8. 未確認前資料庫完全不變。
9. Bot 重啟後可以繼續未過期草稿。
10. 查詢 ZIN／MM 時能顯示該老師對教材歌曲的所有使用歷史。
11. 現有資料在 migration 後仍可查詢且筆數合理。
12. `go test ./...` 全部通過。

## 18. 給下一個 Codex 的注意事項

- 本文件代表產品共識；若實作細節和現有程式衝突，應以保持資料與相容性為優先，再向使用者說明必要調整。
- 不要使用正式 `zumba_music.db` 直接測試破壞性 migration。
- 不要讀出或回覆 `.env` 中的 Telegram Bot Token。
- 不要將 `.env`、資料庫備份或真實 Telegram User ID 提交到 Git。
- 使用 `apply_patch` 編輯檔案。
- 先看 `git status`，保留使用者既有變更。
- 所有寫入路徑都必須有服務端權限檢查。
- 不要因為 Telegram 已隱藏按鈕就省略後端驗證。
- 不要在確認之前寫入部分正式資料。
- 若必須縮小一次實作範圍，先依「schema／migration → services → drafts → Telegram UI」拆分，但不能改變最終規格。

## 19. Git 交接方式

本文件加入專案後，需要由使用者執行或授權 Codex執行：

```bash
git add docs/phase-1-product-spec.md
git commit -m "docs: add phase 1 product specification"
git push
```

回家後：

```bash
git pull
```

然後可對新的 Codex 說：

```text
請先完整閱讀 docs/phase-1-product-spec.md、README.md、docs/glossary.md、docs/adr/ 與目前程式碼，確認 Git 狀態，再依文件中的第一階段規格繼續工作。不要直接修改正式資料庫。
```
