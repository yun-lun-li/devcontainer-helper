---
title: Project Plugins 與 GitHub App 認證規格
subtitle: 已核准：通用 plugin 架構修訂
lang: zh-Hant
template: doc
theme: paper
---

## A 問題、目標與使用情境

目前 manager 與共用設定綁定 GitHub App。
新增 plugin 仍須修改核心流程，無法只靠註冊擴充。
本次修訂建立真正的 Plugin interface 與通用 manager。
GitHub App 成為第一個完整實作，保留原有認證行為。
此架構修訂與新增測試 seam 已由使用者核准。

| 決定 | 交付範圍 |
| --- | --- |
| Plugin | Go 內建 registry；正式實作只有 github-app |
| 使用入口 | Console 的 Project → Plugins |
| 核心 | 管理註冊、設定、生命週期與逐 plugin 狀態 |
| GitHub App | Linux、本機 Docker、每 Project 一個 systemd user service |
| 容器 | GitHub App 支援 image／Dockerfile，拒絕 Compose |
| 設定 | Host JSON 保存參數；local.d 保存容器設定片段 |
| 認證 | Host pass 保存私鑰；Unix Socket 按需供應 Token |

- 身為開發者，我要按 Project 啟用不同 plugin，以隔離設定與狀態。
- 身為開發者，我要從 Plugins 選單設定及停用 plugin，以集中操作。
- 身為開發者，我要預覽合併設定，以確認容器會採用的內容。
- 身為開發者，我要看見逐 plugin 錯誤與待 Build，以決定下一步。
- 身為開發者，我要保留手改片段，以免設定遭自動覆寫。
- 身為開發者，我要限制 GitHub repo 與權限，以縮小授權範圍。
- 身為開發者，我要只在 Host pass 保存私鑰，以避免容器取得私鑰。
- 身為開發者，我要啟用時驗證服務，以提早發現認證錯誤。
- 身為開發者，我要直接使用 Git 與 gh，以沿用日常工作流程。
- 身為開發者，我要關閉 dch 後仍能認證，以避免依賴 Console。
- 身為開發者，我要停用後立即停止供應認證，以終止後續請求。
- 身為維護者，我要只新增實作並註冊，以免修改核心與 Console。
- 身為維護者，我要分別處理 Host 與容器 hook，以明確辨識執行目標。
- 身為維護者，我要保留已完成 plugin 狀態，以便失敗後重試。

## B 通用架構與公開契約

```flow LR
main 組裝入口 -> githubapp 實例: 建立
main 組裝入口 -> 通用 Manager: 註冊實例
Console 與 Commands -> 通用 Manager: 注入使用
通用 Manager -> Plugin interface: 依 ID 分派
Plugin interface -> Host hook: Host 作業
Plugin interface -> Container hook: 注入 executor
Container hook -> devcontainer exec: 容器內指令
```

```tree
internal/
  plugins/
    plugin.go | Plugin 與可選 HostService 公開契約
    manager.go | 通用註冊、分派與狀態協調
    manager_test.go | 兩個測試用 plugin 驗證通用行為
    githubapp/
      config.go | GitHub 表單、參數與權限驗證
      plugin.go | Plugin 實作與事件對應
      manager.go | 原 manager 中的 GitHub 專屬協調
      service.go | systemd、Socket 與 ServeHost
      adapter.go | Git helper、gh wrapper 與初始化
      provider.go | pass、JWT 與 GitHub API
      server.go | HTTP over Unix Socket
      runtime.go | GitHub runtime 路徑與檔案安全
      tests | 移入原測試，依行為並置
  common/
    config_store.go | 通用 JSON 設定與狀態儲存
  console/ | 通用表單 renderer 與 plugin 操作
  commands/ | 注入 manager 的命令入口
main.go | 唯一組裝入口與內部 CLI 分派
```

目錄樹是提議邊界，檔案可按職責拆分。
原 internal/githubapp 全部移入 plugins/githubapp。
原 plugins 中的 GitHub 專屬邏輯也移入該子樹。
不建立 pluginB 或 pluginC 正式目錄。

| Plugin 方法 | 契約 |
| --- | --- |
| Definition | 穩定 ID、顯示名稱、說明與通用表單欄位描述 |
| ValidateConfig | 驗證自身設定，錯誤能對應表單欄位 |
| HandleHost | 接收事件、Project 與自身狀態存取能力 |
| HandleContainer | 接收事件、限定 context 與容器 executor |
| ValidateContainerConfig | 檢查最終合併設定是否滿足自身需求 |
| Status | 回傳通用狀態與可處理訊息，不暴露秘密 |
| HostService.ServeHost | 可選契約；只供需要常駐 Host 程序的 plugin |

上述方法構成實際 Go interface，參數型別於實作時細化。
本頁固定責任與呼叫時機，避免預先堆疊型別樣板。
無相關作業的 hook 回傳成功且無變更。
表單 renderer 只理解文字與選項欄位描述。
GitHub 欄位、權限選項與驗證全部由 githubapp 提供。

Manager、common、Console 與 Commands 不引用 githubapp.Config。
這些核心層不硬編碼 GitHub ID、systemd 或 Socket 邏輯。
子 package 可匯入父 package 的契約。
父 package 不匯入 githubapp，避免循環依賴。
main 建立實作、註冊 manager，並注入 Console 與 Commands。
新增內建 plugin 只需新增實作並在組裝入口註冊。

## C 生命週期、狀態與失敗策略

```flow
(Start／Build／Connect) -> 同 Project 互斥鎖
同 Project 互斥鎖 -> 檢查所有設定與殘留狀態 ID
檢查所有設定與殘留狀態 ID -> 排序並分派 Host 事件
排序並分派 Host 事件 -> 合併設定與逐 plugin 驗證
合併設定與逐 plugin 驗證 -> 容器操作
容器操作 -> 分派 Container 事件: up 成功或進 shell 前
分派 Container 事件 -> 保存逐 plugin 狀態
```

第一版 Enable／Disable 只執行 Host hook。
不為啟用或停用強制啟動容器。
啟用、更新、停用也在鎖內執行，不需要容器操作。

未知 ID 在任何副作用前報錯，包含僅剩 state 的記錄。
Registry 拒絕空白或重複 ID。
Manager 按穩定 ID 排序，逐一分派已註冊且適用的 plugin。
停用與移除 Project 也處理尚有清理狀態的 plugin。

| 事件 | Host hook | Container hook |
| --- | --- | --- |
| Enable／更新 | 驗證後準備資源；成功才確認啟用 | 無 |
| Disable | 立即停止並清理自身資源 | 無；既有容器改動待重建 |
| Start | 容器操作前確保 Host 前置條件 | up 成功後執行 |
| Build | 重建前確保 Host 前置條件 | up 成功後執行 |
| Connect | 進入既有容器前確保 Host 前置條件 | 開啟 shell 前執行 |

Build 指重建容器，讓新掛載與設定生效。
是否重建 image，取決於容器設定與快取。

Container hook 的 Go 程式仍在 Host 執行。
它只能透過注入的 executor 發出容器指令。
Executor 使用 devcontainer exec 與參數陣列。
它沿用合併設定、remoteUser 與 remoteEnv。
不把 dch binary 複製進容器。
Host 與 Container 分別使用具名方法，避免混淆執行目標。

Manager 管理 enabled、pending-recreate 與通用狀態。
ServiceActive 僅屬 GitHub 狀態細節，不是所有 plugin 必填欄位。
其他 plugin 可用通用訊息提供自身狀態。


Hook result 明確回報是否需要重建容器。
Manager 不比較設定差異或雜湊。
GitHub 啟用、更新、停用保守沿用標記待 Build。
其他 plugin 可回報不需重建。
Project 待 Build 是各 plugin pending 的聯集。

Build 或「沒有既有容器」的 Start 可解除 pending。
必須等 up 與所有必要 Container hook 成功才解除。
Connect 或沿用既有容器的 Start 不解除 pending。
停用記錄保留到重建與資源清理完成，避免遺失待辦。
不自動重建，沿用既有重建與移除確認流程。
不加入設定 fingerprint。

任何 plugin 失敗即停止後續分派。
錯誤包含 plugin ID、事件與可處理訊息。
已成功的獨立 plugin 保留自身成果與狀態。
當前 plugin 負責回復自身未完成作業。
清理失敗保留可重試資料，不宣告停用或回復完成。
不跨 plugin 自動回滾，也不建立相依性引擎。

同一 Project 跨程序序列化設定、服務與清理操作。
共享 JSON 寫入仍須協調，以免覆蓋其他 Project。
Plugin 只取得自身設定與私有 metadata 的存取能力。
它不能透過契約覆寫其他 plugin 記錄。
這是 Go 內建實作的責任邊界，並非惡意程式 sandbox。

允許 hook 先保存自身設定，再啟動會讀取設定的服務。
未完成啟用仍須維持原有回滾與重試保證。

## D Host JSON 與 local.d

沿用 Host JSON 與既有 Project 路徑清單。
Project 以正規化路徑索引，彼此隔離。
每個 plugin 的參數與私有 metadata 使用 json.RawMessage。
通用層只解析通用狀態，不解碼 GitHub 結構。
GitHub App 自行解碼參數與片段、unit 的擁有權摘要。

既有 enabled JSON 內容保持可讀。
遷移須保留 fragment_digest、unit_digest 與 unit_path。
既有 build_pending 映射成同等通用待重建狀態。

舊 metadata 由相容解碼或受控遷移搬入 plugin 私有資料。
遷移不得遺失清理與回滾所需資料，也不得建立新服務。
失敗不覆寫原設定；未知 ID 不靜默丟棄。

```flow LR
devcontainer.json -> local.d 排序片段: deep merge
Plugin Host hook -> 專屬受管理片段: 必要時產生
local.d 排序片段 -> 最終設定: 預覽與執行共用
最終設定 -> 各 Plugin 驗證: 自身必要設定
各 Plugin 驗證 -> devcontainer up
```

讀取 `.devcontainer/devcontainer.local.d/` 第一層正常 `.json`。
沿用 JSONC，支援註解與 trailing comma。
先讀 base，再依檔名位元組順序合併片段。
缺少目錄代表無 local 設定。
無效片段顯示檔名並停止。

Object 遞迴合併；array 追加並依 JSON 值去重。
Scalar 與不同型別由後者覆蓋。
GitHub validator 檢查必要 mount、PATH 與初始化接線。
必要設定遭覆寫時停止，Compose 也由該 plugin 拒絕。
不把上述 GitHub 限制放入通用 manager。

舊 `.devcontainer/devcontainer.local.json` 不再讀取。
不提示、不自動搬移，也不新增遷移功能。
README、預覽提示與測試同步採用 local.d。
此項不相容變更維持原決定。

每個需要片段的 plugin 使用固定專屬檔案。
GitHub 片段只放掛載與初始化設定。
認證參數與 Token 不進片段。

更新或刪除前，比對保存的生成內容摘要。
手改內容停止並提示，不覆寫或刪除。
保留其他片段，不修改 Project 忽略規則。
摘要用於檔案擁有權保護，不作為重建 fingerprint。

## E GitHub Unix Socket 與安全邊界

```flow LR
Git -> Credential Helper
Credential Helper -> curl Unix Socket
GH -> gh wrapper
 gh wrapper -> curl Unix Socket
curl Unix Socket -> Host HTTP Server
Host HTTP Server -> pass 與 GPG: 取得 RSA 私鑰
Host HTTP Server -> GitHub API: JWT 換 Installation Token
GitHub API -> Host HTTP Server: 記憶體快取
Host HTTP Server -> curl Unix Socket: 短期 Token
```

Host server 由 githubapp 實作，使用同一份 dch binary。
每個 server 固定服務一個 Project 與指定 Installation。
新設定沿用 Installation 授權的 Repository 範圍；既有明確 repo 限制繼續有效。
容器不能指定 pass entry、App、repository 或權限。
HTTP 只監聽 Unix Socket，不開 TCP port。

只提供 GET /credential 與 GET /health。
Credential 回傳純文字 Token 與 Cache-Control: no-store。
Health 只回報存活，不呼叫 pass 或 GitHub。
拒絕其他方法與路徑，不提供命令執行或 API proxy。
錯誤回應不含命令輸出、API 回應本文或秘密。

Socket 位於有效且由目前使用者擁有的 XDG_RUNTIME_DIR。
各 Project 使用短路徑摘要建立獨立目錄。
目錄 0700、Socket 0600；拒絕不安全擁有者與 symlink。

不退回工作區或共用暫存目錄。
容器使用者須能連線；UID 不符時明確報錯。

整個 Socket 目錄唯讀掛載至 `/run/dch-github`。
連線路徑為 `/run/dch-github/credential.sock`。
目錄掛載讓服務重啟後的新 Socket 仍可見。
重啟保留目錄；停用才清理。
唯讀掛載允許連線，不能限制已授權程序索取認證。

能連接 Socket 的容器程序可取得該 Project Token。
此設計不提供不可信任容器的完整隔離。
Docker 管理員仍能存取掛載。
Installation 應只授權所需 repo；Branch Protection 與 CI 權限另設。
不掛載 pass、GPG Key 或 App Private Key。

## F GitHub Token、Git 與 gh

GitHub 表單只收集 App ID、Installation ID 與所需權限。
Private key pass entry 與 Owner/repository 不再顯示輸入欄位。
新設定從 Host pass 的 `dch/github-app/<App ID>/private-key` 讀取私鑰。

新設定省略 Token request 的 repositories，沿用 Installation 已授權的 Repository 範圍。
既有明確 pass entry 與單一 repo 保留；重新設定不默默擴大原本範圍。

Contents 與 pull_requests 各可選 read／write，或不要求該權限。
至少必須要求一種權限；兩者都不要求時拒絕啟用。
既有 repo 限制仍驗證其 Installation 並限制 Token；新設定只限制所選權限。

建立 App、安裝 App 與匯入私鑰由使用者先完成。

使用參數陣列呼叫 pass，不拼接 shell 指令。
缺少 pass entry 時使用上述預設；拒絕不安全或選項形式的明確 pass entry。
支援 GitHub 提供的 RSA PEM 私鑰。
JWT 使用 RS256，iss 使用 App ID。
iat 回溯 60 秒，exp 不超過未來 10 分鐘。

GitHub HTTPS 設逾時、回應大小上限與 redirect 限制。
Token 視為不透明字串，不假設長度。
拒絕空值、換行與無效 expires_at。
私鑰、JWT 與 Token 不進日誌、設定、預覽或命令參數。

每 Project 在 server 記憶體快取 Token 與 expires_at。
剩餘有效期超過五分鐘時重用快取。
不足五分鐘或無快取時，收到請求才重新取得。
並行請求共用一次取得流程。
服務重啟後首次請求重新取得，不持久保存 Token。

取得失敗回報認證失敗，不回傳過期或接近到期 Token。
不排程背景重試；下一次請求可重試。
Pass 與 GitHub 操作設逾時，不等待互動輸入。
Host GPG Agent 須已能非互動解密。

Git helper 只回應 HTTPS 與精確 github.com。
每次 get 使用 curl --unix-socket 呼叫固定 endpoint。
Username 使用 x-access-token；store／erase 不保存資料。
其他 Host 保留原認證行為，SSH URL 不自動改寫。
避免 github.com 被既有 helper 搶先回傳其他憑證。

Gh wrapper 透過相同 Socket 取 Token。
只對該次 gh 子程序設定 GH_TOKEN。
保留參數與 exit code，避免 wrapper 遞迴。
Curl 固定 Socket、URL 與逾時，停用 proxy 與個人設定。
錯誤時停止，不帶空 Token 繼續執行 gh。

容器須已具備 Git、gh、curl 與 POSIX shell。
缺少依賴或 Socket 無法連線時明確報錯。
不自動安裝套件，不持久寫入 Token。
保留既有 lifecycle hooks，初始化可重複執行。
Start、Build 與 Connect 採用一致接線。

## G GitHub 事件對應與常駐服務

| 通用契約／事件 | githubapp 責任 |
| --- | --- |
| Definition／ValidateConfig | 提供 GitHub 欄位、權限及參數驗證 |
| Enable／更新 Host | 準備片段、腳本與 unit，啟動並驗證認證 |
| Start／Build／Connect Host | 檢查本機 Docker、runtime、受管理資源與服務 |
| ValidateContainerConfig | 驗證 mount、PATH 與接線；拒絕 Compose |
| Start／Build Container | up 成功後安裝／確認 Git 與 gh adapter |
| Connect Container | 開啟 shell 前安裝／確認 adapter |
| Disable Host | 停止服務，依擁有權驗證清理資源 |
| Status | 將服務狀態映射為通用狀態與安全訊息 |
| HostService.ServeHost | Socket HTTP server 與 GitHub provider |

GitHub App 每 Project 建立一個 systemd user service。
不建立 timer，不自動啟用 linger。
其他 plugin 不必使用 systemd、Socket 或常駐服務。

通用內部入口為 `_plugin-serve --plugin ID --project PATH`。
入口依 registry ID 分派可選 HostService.ServeHost。
未知 ID、未支援 HostService 或無效 Project 明確報錯。
此入口不新增公開 plugin 管理 CLI 或公開 dch up。

只有 githubapp 決定 unit、Socket 與 systemd 策略。
它產生 ExecStart，指向同一份 dch 的絕對路徑。
正確處理參數與 systemd escaping，不攜帶秘密參數。
服務從已保存的自身設定讀取參數。

切換入口須相容處理既有 `_github-app-serve` unit。
不得讓已安裝或執行中的 GitHub 服務失去可用入口。
更新需保留舊 unit 路徑、摘要與清理能力。
具體相容方式於實作決定，不預先限制為 alias。

啟用透過實際 service 請求一次 credential。
同時驗證非互動 pass、GitHub 與服務可用性。
驗證失敗不確認啟用，保留原有效設定。
中途失敗只清理此次新建資源，不覆寫使用者同名檔案。
回滾失敗保留清理與重試資料。

Start／Build 在操作容器前驗證服務與認證。
Connect 也先確保服務可用，再執行容器接線。
失敗即停止並回報，不增加認證檢查 endpoint。
未啟用的 Project 不啟動 GitHub 認證服務。
不同 Project 的服務、Socket 與快取各自獨立。

關閉 Console 或 Stop 容器後保留服務。
沒有認證請求時，不讀 pass 或呼叫 GitHub。
Host 工作階段、GPG Agent 與 systemd 可用性仍是前提。
再次 Start 可補齊服務與 runtime 目錄。
不允許兩個程序接管同一 Socket。

修改認證設定重啟服務，丟棄舊快取。
停用或移除 Project 先停止服務，再清理 Socket 與 unit。
停止服務終止進行中請求，拒絕新的認證請求。
本機停止服務不撤銷已回傳 Token。
清理失敗需回報並保留可重試設定。
移除 Project 保留工作區與容器，沿用原確認行為。

## H 驗收、測試與範圍外

| 測試邊界 | 必須觀察到的行為 |
| --- | --- |
| 通用 manager | 兩個不同 fake plugin，獨立設定與私有狀態 |
| 生命週期 | ID 排序、Host／Container 分派與無關事件 no-op |
| 失敗策略 | Fail-fast、前項成果保留、當前回滾與清理重試 |
| Pending | 各 plugin 聯集；純 Host 變更不強制重建 |
| Pending 清除 | up 與必要 hook 全成功；Connect 不清除 |
| Registry | 未知 enabled／殘留 state 在副作用前報錯 |
| 狀態能力 | Plugin 只能保存自身記錄；同 Project 互斥 |
| 相容設定 | 舊 JSON 可讀，摘要與 unit 路徑無損遷移 |
| local.d | 排序、JSONC、deep merge、去重、舊檔忽略 |
| 資源保護 | 手改片段與 unit 不覆寫；跨 Project 隔離 |
| Container executor | 合併設定、remoteUser、remoteEnv 與參數保留 |
| GitHub provider | 假 pass 與 httptest 驗證 JWT、scope、expiry、錯誤 |
| Unix Socket | 真實暫存 Socket 驗證方法、endpoint、權限與錯誤 |
| 記憶體快取 | 可控時間驗證重用、更新、並行去重與失敗重試 |
| Systemd | 假 systemctl 驗證啟動、重啟、停止與清理失敗 |
| Git／gh | Git plumbing 與假 gh 驗證過濾、GH_TOKEN、參數及 exit code |
| CLI／Console | 只透過 registry 與契約提供表單、狀態、操作與內部入口 |
| 保密 | 日誌、設定、預覽、錯誤與命令參數不含秘密 |

兩個 fake plugin 只存在測試，不新增正式空殼實作。
Fake plugin 使用不同欄位與 state，避免只測 GitHub 特例。
新增通用 manager seam；保留原 GitHub 行為測試並移入子樹。
沿用 PATH 假指令與可觀察行為測試，不鎖定內部函式拆分。
一般測試不依賴真實 GitHub、Docker 或 systemd。

核准後每項行為採 red → green。
執行 go test ./...、go build ./... 與 go vet ./...。
使用 Cognitive Complexity analyzer 檢查所有變動函式。
測試函式與匿名函式也須不超過 15，不抑制檢查結果。

真實整合驗收須有操作證據，不能以假指令測試代稱完成。
涵蓋 Docker Socket、UID 不符、服務重啟與 Git／gh 操作。
也驗證關閉 dch 後認證、GPG 失敗與停用後拒絕供應。
驗證其他 Project 不受影響；未執行項目於 review 明列。
此架構修訂尚未執行新增實作或真實整合驗收。

不支援外部動態 plugin、下載協定或通用工作流引擎。
不建立跨 plugin 相依性或自動回滾協調。
GitHub App 不支援 Compose、遠端 Docker 或非 Linux Host。
不支援 GitHub Enterprise、多 repo Token 或 SSH 認證。
不自動建立 App、安裝依賴或遷移舊 local.json。
不實作 Token File、timer、排程刷新或持久 Token 快取。

[GitHub JWT 官方文件](https://docs.github.com/en/apps/creating-github-apps/authenticating-with-a-github-app/generating-a-json-web-token-jwt-for-a-github-app)
[Installation Token 官方文件](https://docs.github.com/en/apps/creating-github-apps/authenticating-with-a-github-app/generating-an-installation-access-token-for-a-github-app)
[gh 環境變數](https://cli.github.com/manual/gh_help_environment)
[curl Unix Socket 選項](https://curl.se/docs/manpage.html#--unix-socket)
[Linux Unix Socket 權限](https://man7.org/linux/man-pages/man7/unix.7.html)
[Docker bind mounts](https://docs.docker.com/engine/storage/bind-mounts/)

架構與新增測試 seam 已核准；原認證需求維持先前決定。
接著確認本機 tickets 分工，再由子代理 TDD 實作。
獨立 Code／Security Review 後提供最終 HTML review。
最終 review 核准後才 push 與建立 PR。
