---
title: Project Plugins 與 GitHub App 認證規格
subtitle: hard dev-flow · Unix Socket 簡化版 · 已核准 · 2026-10-24
lang: zh-Hant
template: doc
theme: paper
---

## A 目標與範圍

dch 讓每個 Project 獨立啟用內建 plugin。
Plugin 可執行 Host 前置作業、提供容器設定與容器初始化。
第一個 plugin 讓 Git 與 gh 向 Host Unix Socket 取得 GitHub App Token。
私鑰留在 Host pass，Token 只在需要時產生。

| 決定 | 本次交付 |
| --- | --- |
| Plugin | Go 內建 registry，第一個為 github-app |
| 使用入口 | Console 的 Project → Plugins |
| Host | Linux、本機 Docker、每個 Project 一個 systemd user service |
| 容器 | image／Dockerfile；已有 Git、gh、curl 與 POSIX shell |
| GitHub | github.com；每個 Project 指定單一 owner/repository |
| 設定 | Host dch JSON 保存 plugin 參數；local.d 保存容器設定片段 |
| Token | Host 記憶體快取，收到請求時按需更新 |

此版以 Unix Socket 取代前版的 Token File 與 timer。
不寫入持久 Token，也不做週期性刷新。
其他已確認的 local.d、Console 與待 Build 行為保留。

## B 使用情境與 Console

1. 身為開發者，我要為不同 Project 啟用不同 plugin。
2. 身為開發者，我要從 Plugins 選單設定及停用 GitHub App。
3. 身為開發者，我要指定 App、installation、repository 與權限。
4. 身為開發者，我要只在 Host pass 保存私鑰。
5. 身為開發者，我要在啟用時發現認證或服務設定錯誤。
6. 身為開發者，我要將個人設定與 plugin 設定分成多個片段。
7. 身為開發者，我要預覽 deep merge 後的設定。
8. 身為開發者，我要直接使用 git pull／push 及 gh pr create。
9. 身為開發者，我要在 Token 到期時自動取得新 Token。
10. 身為開發者，我要關閉 dch 後仍能向 Host 取得認證。
11. 身為開發者，我要看見服務失敗與待 Build 狀態。
12. 身為開發者，我要停用 plugin 後停止提供認證。
13. 身為開發者，我要保留手動修改的片段。

Plugins 提供設定、啟用、停用與狀態檢視。
顯示服務狀態、待 Build 與可處理的錯誤。
不增加 Token 到期倒數或更新排程介面。
不顯示 Token、JWT 或私鑰內容。

啟用時呈現授權範圍，驗證 pass、GitHub 與 systemd service。
需驗證服務能在非互動條件下取得憑證。
驗證失敗不標記啟用，保留原有有效設定。
中途失敗清理此次新建資源，不覆寫同名使用者檔案。

設定變更或停用後，既有容器顯示需要 Build。
不自動重建，保留既有確認流程。
停用會立即停止 Host 認證服務，不等待 Build。
既有容器的認證設定恢復則需 Build。

## C local.d 與 plugin 框架

```flow LR
devcontainer.json -> local.d 排序片段: deep merge
內建 plugin -> 專屬 JSON 片段: 設定與掛載
local.d 排序片段 -> 合併結果: 預覽與執行共用
合併結果 -> Host 準備: 確保服務可用
Host 準備 -> devcontainer up: 掛載 Socket 目錄
```

使用 `.devcontainer/devcontainer.local.d/` 第一層的 `.json` 正常檔案。
沿用 JSONC 解析，支援註解及 trailing comma。
先讀 base，再按檔名位元組順序合併片段。
缺少目錄代表沒有 local 設定；無效片段回報檔名並停止。

Object 遞迴合併；array 追加並依 JSON 值去重。
Scalar 與不同型別由後者覆蓋。
必要 Socket mount 或初始化設定被覆寫時，停止啟動。

舊 `.devcontainer/devcontainer.local.json` 不再讀取。
不提示、不自動搬移，也不新增遷移功能。
README、預覽提示與測試同步改為 local.d。
這是不相容變更。

每個 plugin 使用固定專屬片段。
片段只放掛載與初始化設定，不放認證參數或 Token。
Host 保存生成內容摘要，更新或刪除前比對。
遇到手改內容停止並提示，不覆寫或刪除。
其他片段保留；不擅自修改 Project 的忽略規則。

Registry 使用穩定 plugin ID。
提供設定驗證、片段生成、Host 準備與停用清理。
容器初始化透過受管理的腳本與設定完成。
未知的已啟用 plugin 明確報錯。
保持簡單，不加入外部 plugin 協定或通用工作流引擎。

沿用 Host JSON，保留既有 Project 路徑清單相容性。
新增按正規化 Project 路徑索引的 plugin 設定。
不同路徑使用獨立服務、Socket 與快取。
未啟用 plugin 的 Project 不啟動認證服務。

## D Unix Socket 認證架構

```flow LR
Git 指令 -> Credential Helper
Credential Helper -> curl Unix Socket
GH 指令 -> gh wrapper
 gh wrapper -> curl Unix Socket
curl Unix Socket -> Host HTTP Server: HTTP over Unix Socket
Host HTTP Server -> 內建憑證取得邏輯
內建憑證取得邏輯 -> pass 與 GPG: 讀取私鑰
內建憑證取得邏輯 -> GitHub API: JWT 換 Installation Token
GitHub API -> Host HTTP Server: Token 留在記憶體快取
Host HTTP Server -> curl Unix Socket: 短期 Token
```

Host server 由 dch 內建，不額外拆出 get-github-credential 程式。
每個 server 僅服務一個固定 Project。
容器請求不能指定 pass entry、App、repository 或權限。
HTTP 只監聽 Unix Socket，不開 TCP port。

僅提供 GET /credential 與 GET /health。
Credential 成功回傳純文字 Token，並設定 Cache-Control: no-store。
Health 只回報服務存活，不呼叫 pass 或 GitHub。
其他路徑或方法拒絕，不提供通用命令執行或 API proxy。
錯誤回應不包含命令輸出、GitHub 回應本文或秘密。

Socket 放在有效且由目前使用者擁有的 XDG_RUNTIME_DIR。
每個 Project 使用短路徑摘要建立獨立目錄。
目錄 0700、Socket 0600；拒絕不安全擁有者與 symlink。
不退回工作區或共用暫存目錄。
容器使用者需有權連接 Socket，UID 不符時明確報錯。

將整個 Socket 目錄唯讀 bind mount 至 `/run/dch-github`。
容器連線路徑為 `/run/dch-github/credential.sock`。
不只掛載單一 Socket，讓服務重啟後的新 Socket 仍可見。
服務重啟保留目錄；停用時才清理目錄。
唯讀掛載不妨礙連線，也不限制已授權的認證請求。

## E Token、Git 與 gh

Console 收集 App ID、Installation ID、pass entry 與 owner/repository。
Contents 與 pull_requests 分別可選 read／write。
驗證 repository 屬於指定 owner，申請時限制到該 repo 與權限。
GitHub App 建立、安裝與私鑰匯入 pass 由使用者先完成。

以參數陣列呼叫 pass，不拼接 shell 指令。
驗證 pass entry，拒絕空值與選項形式。
支援 GitHub 提供的 RSA PEM 私鑰。
JWT 使用 RS256；iat 回溯 60 秒、exp 不超過未來 10 分鐘。
使用 App ID 作為 iss。

GitHub HTTPS 請求設逾時、回應大小上限及 redirect 限制。
Token 視為不透明字串，不假設長度。
拒絕空值、換行與無效 expires_at。
私鑰、JWT 與 Token 不進日誌、設定、預覽或命令參數。

每個 Project 在 server 記憶體快取 Token 與 expires_at。
有效期剩餘超過五分鐘時直接回傳快取。
不足五分鐘或快取為空時，收到請求才重新取得 Token。
並行請求共用一次取得流程，避免重複呼叫 pass 與 GitHub。
服務重啟後首次請求重新取得，不保存 Token 到磁碟。

取得失敗回報認證失敗，不回傳過期或接近到期的 Token。
不做背景重試排程；下一次認證請求可再嘗試。
Pass 與 GitHub 操作設逾時，服務不等待互動輸入。
Host GPG Agent 必須已具備非互動解密能力。

Git helper 僅對 HTTPS 與精確 github.com 回應認證。
每次 get 用 curl --unix-socket 呼叫固定 endpoint。
使用 x-access-token username；store／erase 不保存資料。
其他 Host 保留原認證行為，SSH URL 不自動改寫。
確保 github.com 不被既有 helper 搶先回傳其他憑證。

Gh wrapper 透過相同 Socket 取得 Token。
僅對該次 gh 子程序設定 GH_TOKEN。
保留參數與 exit code，避免 wrapper 遞迴。
Curl 使用固定 Socket、URL、逾時並停用 proxy 與個人 curl 設定。
錯誤時停止，不帶空 Token 繼續執行 gh。

容器需要既有 Git、gh、curl 與 POSIX shell。
缺少依賴或無法連接 Socket 時提供明確錯誤。
不自動安裝套件，不將 Token 寫入持久環境或 Git 設定。
保留既有 lifecycle hooks，初始化可重複執行。
Start／Build／Connect 使用一致的 helper 與 wrapper 接線。

## F 服務生命週期

每個 Project 使用一個 systemd user service，不建立 timer。
Service 以 dch 的內部 server 入口執行，使用絕對路徑。
不新增公開 dch up 或 plugin 管理 CLI。
正確處理命令參數與 systemd escaping。
Service 不攜帶秘密參數，不自動啟用 linger。

啟用驗證會透過實際 service 請求一次 credential。
Start／Build 確保服務可用，並驗證認證後才啟動容器。
失敗則停止，回報可處理的錯誤。
不需要額外的認證檢查 endpoint。

關閉 Console 或 Stop 容器後保留 service。
無認證請求時，不讀 pass、不呼叫 GitHub。
Host 工作階段、GPG Agent 與 systemd 可用性仍是前提。
再次 Start 時補齊服務及 runtime 目錄。

修改認證設定需重啟服務，丟棄舊的記憶體快取。
停用或移除 Project 時，先停止服務，再清理 Socket 與 unit。
停止服務會終止進行中的請求，不再接受新的認證。
清理失敗需回報，保留可重試的設定。
移除 Project 仍保留工作區與容器，沿用原確認行為。

同一 Project 的服務建立、設定與清理需互斥。
不允許兩個程序接管同一 Socket。
本機停止服務不會撤銷已回傳的 Token。

## G 驗收與測試邊界

| 邊界 | 驗收 |
| --- | --- |
| 設定合併 | 排序、JSONC、deep merge、array 去重、舊檔忽略 |
| Plugin 設定 | 舊設定相容、跨 Project 隔離、片段手改保護 |
| GitHub provider | 假 pass 與 httptest 驗證 JWT、scope、expiry 與錯誤 |
| HTTP Unix Socket | 真實暫存 Socket 驗證 endpoint、方法、權限及錯誤 |
| 記憶體快取 | 可控時間驗證重用、到期前更新、並行去重、失敗後重試 |
| Systemd | 假 systemctl 驗證啟動、重啟、停止與清理失敗 |
| 容器接線 | 假 devcontainer 驗證目錄掛載與 Start／Build／Connect |
| Git | 真實 Git plumbing 驗證 Host 過濾與 Socket 取憑證 |
| gh | 假 gh 驗證每次取憑證、GH_TOKEN、參數與 exit code |
| Console | 啟用驗證、服務狀態、待 Build、停用與錯誤 |
| 保密 | 設定、日誌、預覽、參數不含秘密 |

使用可觀察行為測試，沿用 PATH 假指令慣例。
一般測試不依賴真實 GitHub、Docker 或 systemd。
以可控時間測試快取，不等待真實過期。
每項行為遵守 red → green，整合後執行 go test ./... 及編譯。

手動驗收涵蓋真實 Docker Socket 連線、UID 不符與服務重啟。
也涵蓋 Git／gh 實際操作、關閉 dch 後認證與 GPG 失敗。
驗證停用後不再提供憑證，且其他 Project 不受影響。
未執行的環境驗收需在最終 review 明列。

## H 安全邊界、範圍外與依據

能連接 Socket 的容器程序可以取得該 Project 的短期 Token。
這不是對不可信任容器的完整隔離。
Docker 管理員仍能存取掛載，權限應限制在所需 repo。
Branch Protection、Rulesets、Review 與 CI 權限另行設定。

本次不支援外部 plugin、Compose、遠端 Docker 或非 Linux Host。
不支援 GitHub Enterprise、多 repo Token 或 SSH 認證。
不自動建立 GitHub App、安裝依賴或遷移舊 local.json。
不掛載 pass、GPG Key 或 App Private Key。
不實作 Token File、timer、排程刷新或持久 Token 快取。

[GitHub JWT 官方文件](https://docs.github.com/en/apps/creating-github-apps/authenticating-with-a-github-app/generating-a-json-web-token-jwt-for-a-github-app)
[Installation Token 官方文件](https://docs.github.com/en/apps/creating-github-apps/authenticating-with-a-github-app/generating-an-installation-access-token-for-a-github-app)
[gh 環境變數](https://cli.github.com/manual/gh_help_environment)
[curl Unix Socket 選項](https://curl.se/docs/manpage.html#--unix-socket)
[Linux Unix Socket 權限](https://man7.org/linux/man-pages/man7/unix.7.html)
[Docker bind mounts](https://docs.docker.com/engine/storage/bind-mounts/)

此版規格與 Unix Socket 測試邊界已由使用者核准。
接著確認本機 tickets 分工，再由子代理 TDD 實作。
完成獨立 Code／Security Review 後提供最終 HTML review。
最終 review 核准後才 push 與建立 PR。
