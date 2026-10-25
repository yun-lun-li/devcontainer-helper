---
title: Project Plugins 與 GitHub App 認證規格
subtitle: 已核准：Remote 優先選 Repo 與 cli/oauth Device Flow
lang: zh-Hant
template: doc
theme: paper
---

## A 目標與使用情境

通用 Plugin interface、Manager、local.d 與 Host／Container 分工沿用已核准設計。本次改以 GitHub App Device Flow 取得使用者認證。Git 與 gh 代表授權使用者操作；建立 PR 的作者是該使用者。

Git commit author 沿用現有 gitconfig，不隨 Token 改寫。不再以 Installation Token 代表 App bot 操作。使用者已核准本頁認證變更；實作及驗證進度另記於本機 tickets。

| 決定 | 交付範圍 |
| --- | --- |
| Plugin | Go 內建 registry；正式實作只有 github-app |
| 使用入口 | 主選單 Settings → Plugins 設共用 Client ID；Project → Plugins 啟用與重新授權 |
| 核心 | 通用註冊、opaque 設定、生命週期與逐 plugin 狀態 |
| GitHub App | Linux、本機 Docker、每 Project 一個 systemd user service |
| 容器 | 支援 image／Dockerfile，拒絕 Compose；保留 Git helper 與 gh wrapper |
| 設定 | Host JSON 保存非秘密參數；local.d 保存容器設定片段 |
| 認證 | 每 Project 的 pass JSON secret 保存 user token、refresh token 與期限 |

使用者只需在主選單 Settings → Plugins → GitHub App 輸入一次 Client ID。每個 Project 啟用時沒有 App ID、Installation ID、私鑰、pass entry、repo 或權限文字欄位。先由 Project Git remote 取得 repo ID，通常只需一次 GitHub 網頁驗證碼授權；無法取得 ID 時才明確引導額外 discovery 授權。不手填 repo ID。

共用 Client ID 不代表共用 Token。不同 Project 可明確授權不同帳號，互不覆寫認證。

使用者可按 Project 啟用、查看狀態、重新授權及停用 plugin。他們仍能預覽合併設定、保留手改片段、看見錯誤與待 Build。關閉 dch 後仍可認證；停用立即停止後續供應。

維護者新增內建 plugin 時，只需實作契約並在 main 註冊。

GitHub App 的 Device Flow、安裝範圍與權限先在 GitHub 設定，包含 discovery 所需 Metadata read。有效權限是使用者與 App 的交集，再以所選 repository_id 請求單一 repo Token；不提供 Project permissions selector。

GitHub 會忽略無法存取的 repository_id，因此不能只送參數就宣告限制成功。本版發布前必須驗證 repository 範圍，無法驗證即拒絕；共用 Client ID 本身不構成 repo 限制。語意依據 [GitHub user token 文件](https://docs.github.com/en/apps/creating-github-apps/authenticating-with-a-github-app/generating-a-user-access-token-for-a-github-app)。

## B 改動後架構、職責與公開契約

以下列出實際預計路徑。標示「提案」的檔案與契約尚未建立。其他路徑已存在，本次依表列職責修改；不建立空殼 plugin。

```tree
internal/
  plugins/
    plugin.go | Host：既有契約；新增共用設定與互動能力提案
    manager.go | Host：registry、opaque 設定、生命週期與狀態協調
    manager_test.go | 通用 fake plugin 與新契約測試
    githubapp/
      config.go | Host：共用 Client ID、Project 授權綁定與舊設定辨識
      plugin.go | Host：Plugin 實作、授權互動與事件對應
      manager.go | Host：受管理資源、systemd 與清理協調
      device.go（提案） | Host：cli/oauth Device Flow、必要時 discovery 與身分確認
      oauth_http.go（提案） | Host：限定 token exchange 的 repository_id 表單 adapter
      repository.go（提案） | Host：remote 解析、repo ID 查詢、discovery 與範圍驗證
      provider.go | Host：user token 快取、按需 refresh 與期限驗證
      secret.go（提案） | Host：pass 單一 JSON secret、完整保存與認證鎖
      adapter.go | Host 產生／Container 執行：Git helper、gh wrapper
      server.go | Host：每 Project HTTP over Unix Socket
      runtime.go | Host：runtime 路徑、Socket 與檔案安全
      *_test.go | 各職責旁的行為測試；新檔案屬提案
  common/
    config_store.go | Host：通用 JSON、全域 plugin 設定與 Project 狀態儲存
    devcontainer_config.go | Host：local.d 合併與預覽共用設定
  console/
    plugins.go | Host：通用表單、操作與授權進度顯示
    settings.go（提案） | Host：依 registry metadata 顯示共用 Settings
    menu.go | Host：Settings 入口接線
    up.go、build.go、connect.go | Host：容器生命週期入口
    devcontainer.go | Host：devcontainer exec executor
  commands/
    commands.go | Host：注入 Manager；內部服務入口分派
main.go | Host：唯一實例建立、註冊與依賴組裝入口
go.mod、go.sum | 提案：新增並鎖定 github.com/cli/oauth 依賴
```

```flow LR
Host main -> 通用 Manager: 註冊 githubapp
Host Console 與 Commands -> 通用 Manager: 契約呼叫
通用 Manager -> Plugin: 只傳自身 opaque 設定與狀態
Plugin -> Host 互動能力: URL、驗證碼、項目選擇、取消
Plugin -> Host pass 與 GitHub: 授權及 refresh
Plugin -> 容器 executor: devcontainer exec
容器 Git 與 gh -> Host Unix Socket: 只取 access token
```

| 既有公開契約 | 責任 |
| --- | --- |
| Definition／ValidateConfig | 穩定 ID、metadata、Project 參數驗證 |
| HandleHost | 事件、Project 與自身設定／狀態能力 |
| HandleContainer | 事件、限定 context 與容器 executor |
| ValidateContainerConfig | 驗證最終合併設定是否滿足自身需求 |
| Status | 通用狀態與安全訊息，不暴露秘密 |
| HostService.ServeHost | 可選常駐 Host 程序；不持有 Project 生命週期鎖 |

以下是最小擴充提案；名稱可於實作細化，責任固定。Definition 新增 SharedFields，沿用既有 Field 的文字／選項 metadata。可選 SharedSettings 契約驗證 plugin 自己的全域 opaque JSON。

Manager 提供依 plugin ID 讀寫共用設定的能力，不解碼 Client ID。共用 Settings renderer 依 registry 顯示欄位，不硬編碼 GitHub。

可選 InteractiveConfigurer 契約處理需要互動的 Project 啟用／重新授權。它接收自身共用設定、Project context 與 HostInteraction 能力。HostInteraction 只負責 URL／驗證碼、開啟瀏覽器、通用項目選擇及取消訊號。選項只含不透明 key 與顯示文字，不包含 GitHub 型別。

githubapp 組裝 cli/oauth，處理授權結果、repo 清單及帳號辨識；不自行重寫輪詢器。沒有互動契約的 plugin 保留原表單與啟用路徑。Console 在生命週期操作前執行互動；Manager 不持鎖等待瀏覽器。

授權結果先留在記憶體，不覆寫 active secret。提交時在 Project 鎖內重新驗證設定版本，拒絕過期候選資料。

版本及帳號確認通過後才取認證鎖保存、切換服務；取消或版本衝突不替換現有認證。

核心不 import githubapp.Config，也不認識 GitHub endpoint、systemd 或 Socket。githubapp 不 import console；以注入能力避免循環依賴。main 建立實作並注入 Console、Commands 與 Manager。

容器 hook 的 Go 程式仍在 Host，只經 executor 執行容器命令。

## C 生命週期、狀態與失敗策略

Start／Build／Connect 在 Project 跨程序生命週期鎖內執行。先檢查所有 enabled／殘留 state ID，再依穩定 ID 排序分派 Host hook。之後合併容器設定、逐 plugin 驗證、執行容器操作，最後分派 Container hook。

未知 ID 在副作用前報錯；registry 拒絕空白與重複 ID。

| 事件 | Host | Container |
| --- | --- | --- |
| Enable／更新 | 驗證並準備；成功才確認啟用 | 無，不強制啟動容器 |
| Disable | 立即停止並清理自身資源 | 無，既有改動待 Build |
| Start／Build | 容器操作前確認前置條件 | up 成功後執行 |
| Connect | 進 shell 前確認前置條件 | shell 前執行 |

Container hook 使用 devcontainer exec 參數陣列，沿用合併設定、remoteUser 與 remoteEnv。不把 dch binary 複製進容器；不需要的 hook 成功 no-op。Hook result 回報是否待 Build；Manager 不比較設定 fingerprint。

Project pending 是各 plugin 聯集；GitHub 啟用、更新、停用保守標記 pending。

Build 或無既有容器的 Start，須等 up 及所有必要 hook 成功才清除 pending。Connect 或沿用容器的 Start 不清除；停用記錄保留到重建與清理完成。不自動重建，保留既有重建／移除確認。

任一 plugin 失敗即停止；錯誤含 plugin ID、事件與安全可處理訊息。前項成功成果保留；當前 plugin 回復自己未完成的作業，清理失敗保留重試資料。不跨 plugin 回滾，不建立相依性引擎；遠端 refresh 的例外依 F 處理。

Plugin 只能讀寫自身 opaque 設定與狀態，這是責任邊界，並非惡意程式 sandbox。

共享 JSON 仍協調跨 Project 寫入；允許 hook 先存設定再啟動讀取它的服務。HostService 不持生命週期鎖；認證更新另外使用 F 的鎖規則。

## D Host JSON 與 local.d

沿用 Host JSON 與 Project 路徑清單，Project 以正規化路徑索引。全域 plugin 設定與各 Project 的參數、私有 metadata 使用 json.RawMessage。通用層不解碼 GitHub；Project 保存已授權 Client ID、身分摘要、所選 repository ID／owner/name 與 secret 擁有權。

秘密只在 pass；共用值改動不自動換綁 Project。

既有 JSON 可讀，保留 fragment_digest、unit_digest、unit_path 與待重建狀態。受控遷移不得丟失清理資料、未知 ID 或失敗時覆寫原設定；認證轉換依 G。

```flow LR
devcontainer.json -> local.d 排序片段: deep merge
Plugin Host hook -> 專屬受管理片段
local.d 排序片段 -> 最終設定: 預覽及執行共用
最終設定 -> 各 Plugin 驗證 -> devcontainer up
```

讀取 `.devcontainer/devcontainer.local.d/` 第一層正常 `.json`，支援 JSONC。先 base，再依檔名位元組排序合併；缺目錄代表無設定，錯誤標示檔名並停止。Object 遞迴合併；array 追加並按 JSON 值去重；scalar／不同型別後者覆蓋。

GitHub validator 驗證必要 mount、PATH、初始化接線，並自行拒絕 Compose。

舊 `.devcontainer/devcontainer.local.json` 不讀、不提示、不自動搬移。README、預覽及測試同步採用 local.d，此不相容決定維持。每 plugin 使用固定專屬片段；GitHub 片段只有掛載及初始化，沒有認證參數。

更新／刪除前比對保存摘要；手改內容停止，不碰其他片段或 Project 忽略規則。摘要只保護檔案擁有權，不作重建 fingerprint。

## E GitHub Unix Socket 與安全邊界

```flow LR
Git -> Credential Helper
Credential Helper -> curl Unix Socket
gh -> gh wrapper
gh wrapper -> curl Unix Socket
curl Unix Socket -> Host HTTP Server
Host HTTP Server -> Project token provider: 快取或按需 refresh
Project token provider -> pass 與 GPG: 解密／完整保存 JSON secret
Project token provider -> GitHub OAuth: 更新 user token
Host HTTP Server -> curl Unix Socket: 只回 access token
```

Host server 使用同一份 dch binary，由 githubapp 實作。每個 server 固定綁定一個正規化 Project 與已明確授權的 Client ID。容器不能指定帳號、Client ID、pass entry、repository 或權限。

HTTP 只監聽 Unix Socket，不開 TCP port。

只提供 GET /credential 與 GET /health。Credential 回傳純文字 access token 與 Cache-Control: no-store。Health 只回報存活，不呼叫 pass 或 GitHub。

拒絕其他方法與路徑，不提供命令執行或 API proxy。錯誤回應不含命令輸出、API 回應本文或秘密。

Socket 位於有效且由目前使用者擁有的 XDG_RUNTIME_DIR。各 Project 以短路徑摘要建立獨立目錄。目錄 0700、Socket 0600；拒絕不安全擁有者與 symlink。

不退回工作區或共用暫存目錄。容器使用者須能連線；UID 不符時明確報錯。

整個 Socket 目錄唯讀掛載至 `/run/dch-github`。連線路徑為 `/run/dch-github/credential.sock`。目錄掛載讓服務重啟後的新 Socket 仍可見。

重啟保留目錄；停用才清理。唯讀掛載允許連線，不能限制已授權程序索取認證。

能連接 Socket 的容器程序可取得該 Project 的 user token。發布的 Token 須通過所選 repository 範圍驗證。僅在 fallback 需要 discovery 時，較廣 Token 才短暫存在 Host 記憶體，絕不供應容器。本設計不提供不可信任容器的完整隔離。

Docker 管理員仍能存取掛載。應在 GitHub 限制 App 安裝與權限，另設 Branch Protection 與 CI 權限。容器不取得 refresh token、pass、GPG Key 或其他 Host 秘密。

## F Device Flow、Token 儲存與 Git／gh

共用 Settings 只保存非秘密 Client ID；Project Plugins 的「啟用／重新授權」先辨識 Git remote，再說明本次需要一次或額外 discovery 授權。缺少 Client ID 時引導到共用 Settings。Repo ID 已取得時直接進行一次受限 Device Flow。

```flow
啟用或重新授權 -> 讀取與確認 Project remote
讀取與確認 Project remote -> 公開 REST 查 repo ID
公開 REST 查 repo ID -> 受限 Device Flow: 已取得 ID
公開 REST 查 repo ID -> Host gh api 查 repo ID: 無法匿名解析
Host gh api 查 repo ID -> 受限 Device Flow: 已取得 ID
Host gh api 查 repo ID -> 使用者選擇 discovery 或取消: 仍無 ID
使用者選擇 discovery 或取消 -> Host 記憶體 discovery 授權
Host 記憶體 discovery 授權 -> 驗證已選 remote 對應 ID
驗證已選 remote 對應 ID -> 受限 Device Flow
受限 Device Flow -> 身分確認及完整範圍驗證
身分確認及完整範圍驗證 -> 鎖內重驗版本並提交
```

Host 在 Project cwd 以參數陣列執行 `git remote` 與 `git remote get-url`。只讀本機設定，不用 shell，不執行 fetch、remote helper 或其他網路 Git 指令。URL 支援 HTTPS github.com、`ssh://git@github.com/owner/repo.git` 與 `git@github.com:owner/repo.git`。

拒絕其他 Host、HTTPS 內嵌憑證、歧義或格式錯誤；只正規化合法 owner/repo 與可選 .git 後綴。SSH 格式只用來解析名稱，SSH clone 能力或 SSH key 不代表 REST 已認證。錯誤不得回顯可能含秘密的完整 URL。

單一支援的 remote 顯示並確認；多個則列出名稱與 owner/repo，origin 為預設選項，仍由使用者選擇。不任意猜測 push／upstream remote。沒有支援 remote 時顯示可處理訊息，讓使用者選 discovery repo 清單或取消。

先以無認證的 `GET /repos/{owner}/{repo}` 取得公開 repo 的 id/full_name。404 無法區分 private 與不存在，不宣稱已找到 private repo。匿名查不到時可用既有 Host GitHub 認證查詢；不要先要求新的 discovery 授權。

最小 Host 查詢使用真實 Host `gh api repos/{owner}/{repo}`，固定 GET 與 github.com，由 gh 自行讀取既有認證。不得誤用容器 wrapper；不執行 `gh auth token`，不提取或自行轉送其 Token。呼叫設逾時、非互動與有限 stdout，只解析 id/full_name，清理 stderr，不輸出認證資料。

核對 ID 為有效正整數及 full_name 與已確認 remote 一致；不默默接受跨 repo redirect。Host gh 缺少、未登入、查詢失敗或仍無有效 ID，才顯示「額外授權以查詢 repo」或取消。不讀其他 pass entry／credential helper，也不把 Host 既有認證送進容器。

使用 [github.com/cli/oauth](https://github.com/cli/oauth)；這是使用者明確指定的依賴例外。實作時選定相容 release／commit，驗證 API 後鎖進 go.mod／go.sum，不追蹤未指定版本的 main。

使用低階 device.RequestCode 與 device.Wait，不啟用 Web Flow fallback。每次 code 請求帶 client_id、scope=offline_access；不傳 Client Secret。Wait 由 library 處理 pending、interval、slow_down、拒絕及到期，context 取消停止等待。

目前 [WaitOptions 原始碼](https://github.com/cli/oauth/blob/main/device/device_flow.go) 沒有 repository_id 欄位。以狹窄 PostForm adapter 僅對受限 Device Flow 的固定 token endpoint／grant 注入所選 repository_id，每次 poll 都帶入。不 fork library、不自行實作 poller，也不把參數加到 discovery 或 refresh。

HTTP adapter 保留 library 回應解析，設定每請求逾時、回應大小及 redirect 限制。若選定版本未將 context 傳至 HTTP，adapter 必須將操作 context 綁定到請求，取消亦中止 in-flight HTTP。可用版本的 oauth.Refresh API 須一併驗證；無此 API 時僅實作單次 refresh HTTP，不重寫 Device Flow。

目前 [AccessToken 型別](https://github.com/cli/oauth/blob/main/api/access_token.go) 未提供 expiry 欄位。HTTP adapter 在大小上限內擷取成功回應的 expires_in／refresh_token_expires_in，保留原回應供 library 解析；不記錄本文。選定版本若已提供期限則直接使用，不能因 library 遺漏欄位而寫死期限。

ID 已解析時，直接取得 device code，所有 token exchange poll 都帶所選 repository_id。成功後以 `/user` 顯示帳號供確認；重新授權若改變既有 Project user ID，必須明確確認帳號切換。Host gh 的查詢帳號不被默認為本次授權帳號。

只有無法預先取得 ID 時才使用 discovery Device Flow，先明確告知本次會需要額外一次 GitHub 授權。Discovery Token 呼叫 `/user` 確認帳號，再完整分頁 `/user/installations` 與每個 `/user/installations/{id}/repositories`。有已選 remote 時必須找到相符 repo，不得默默改選其他 repo。

沒有支援 remote 且使用者選擇 discovery 時，才從上述 user／App 交集清單選一個 owner/name。Numeric ID 必須來自此次 API 結果，不手動輸入。無相符 repo、分頁失敗、取消或授權拒絕都不替換現有認證。

Fallback 隨後另取 device code 進行受限授權，每個 exchange poll 都帶解析出的 repository_id。最終 `/user` ID 必須與 discovery 確認的 ID 相同，否則拒絕並要求重來。兩條路徑都先驗證並確認帳號，候選只留記憶體，再由生命週期鎖內重驗版本及認證鎖內保存、切換服務。

候選 Token 必須完整枚舉可存取的 installations／repositories，去重後恰為所選 ID，並核對目標 repo 的實際 ID。不能以單次 GET 目標 repo 成功作為範圍證明。任何額外 repo、缺失、權限錯誤、不完整分頁或無法確認 API 語意，都拒絕持久保存與發布。這只驗證 repository 範圍，不宣稱移除 App 原有的帳號層級權限。

[官方 installation API 文件](https://docs.github.com/en/rest/apps/installations#list-repositories-accessible-to-the-user-access-token) 支援 user token 與 Metadata read，但未明說縮限 Token 的枚舉結果保證。上述驗證是必須以真實 GitHub 證明的實作 gate；若無法證明就回報限制，不能降級為較廣 Token。

Discovery 的 access／refresh token 只保留 Host 記憶體，完成、取消或失敗即丟棄；不進 pass、設定、日誌或容器。不為清理暫存認證而撤銷整個 App authorization，避免影響其他 Project。

瀏覽器開啟失敗仍顯示安全 GitHub HTTPS URL 與 user_code，供手動輸入。Device code 不保存、不記錄；user_code 只顯示於當次畫面。沒有 JWT、RSA 私鑰或 Installation Token；不接受容器傳入 repo 或帳號參數。

每個正規化 Project 與 Client ID 自動導出一個 pass entry。提案路徑為 `dch/github-app/users/<project-digest>/<client-digest>`。不接受自訂 pass entry，也不設密碼輸入框。

單一加密 JSON 保存 access_token、refresh_token 及兩者 expires_at。另保存 schema、Client ID、Project、user ID 與所選 repository ID／名稱綁定，讀取時驗證一致性。期限由實際 expires_in 與 refresh_token_expires_in 計算，不寫死時數。

空 Token、換行、缺少 refresh token 或無效期限均拒絕保存。offline_access 必須得到可到期的 Token；不默默接受永不到期模式。

認證鎖涵蓋讀取、refresh、保存驗證與發布新 Token。鎖以 Project 與 Client ID 為範圍，跨程序序列化。它與生命週期鎖分離；provider 不取得生命週期鎖。

生命週期操作若需兩者，固定先生命週期鎖再認證鎖。不持共享 JSON 寫入鎖等待 pass、網路或 systemd。共享設定變更亦不以全域鎖等待各 Project 操作。

Secret store 透過 pass 一次保存完整 JSON，再讀回驗證；不分開更新 Token 與期限。這是整組認證的邏輯提交，不假設 pass insert 有磁碟原子替換保證。

使用 pass 的既有加密與 recipient 規則，不直接改寫 password-store 或建立明文暫存檔。保存失敗或讀回不完整即拒絕發布；崩潰後驗證失敗則要求重新授權。

Credential 請求先用記憶體中仍有效超過五分鐘的 Token。沒有快取時，在認證鎖內讀 pass；有效超過五分鐘即可使用。剩餘期限不超過五分鐘時，按需 refresh，並行請求共用一次更新。

Refresh 使用 client_id、grant_type=refresh_token、refresh_token，Device Flow 來源不需 Client Secret。官方 refresh 參數不含 repository_id，不自行附加；延續已保存的 repo 綁定。

每次 refresh 後重新驗證身分與單一 repo 範圍，再保存並讀回驗證整組新認證，才發布快取。範圍擴大或無法證明時拒絕供應並要求重新授權，不回退到 discovery／較廣 Token。

GitHub 會使舊組認證失效；遠端成功後本機保存失敗無法承諾無縫回滾。保存失敗不回應新 Token；嘗試受控恢復，失敗即要求重新授權。程序在遠端更新後崩潰，也可能需要重新授權。

Rotation 語意依據 [GitHub refresh 官方文件](https://docs.github.com/en/apps/creating-github-apps/authenticating-with-a-github-app/refreshing-user-access-tokens)。

過期、撤銷或無效 refresh 使狀態成為 authorization-required。暫時網路或 GPG 錯誤則回報可重試錯誤，不回傳過期／近到期 Token。不排程背景重試；下一次請求可重試。

服務重啟後從 pass 恢復，不必每次重新開瀏覽器。pass 以參數陣列及 stdin 呼叫；秘密不放命令參數。Host GPG Agent 須可非互動解密；不自行提示輸入 pass 密碼。

HTTP 與 pass 操作有逾時，HTTP 有大小及 redirect 限制。Token 視為不透明字串，不假設長度或前綴。秘密不進 Host 設定、local.d、日誌、預覽、錯誤或容器檔案。

Git helper 只回應 HTTPS 與精確 github.com。每次 get 使用 curl --unix-socket 呼叫固定 endpoint。Username 使用 x-access-token；store／erase 不保存資料。

其他 Host 保留原認證行為，SSH URL 不自動改寫。避免 github.com 被既有 helper 搶先回傳其他憑證。gh wrapper 只對該次子程序設定 GH_TOKEN。

保留參數與 exit code，避免 wrapper 遞迴。curl 固定 Socket、URL 與逾時，停用 proxy 與個人設定。錯誤時停止，不帶空 Token 繼續執行 gh。

容器須已有 Git、gh、curl 與 POSIX shell，不自動安裝套件。保留既有 lifecycle hooks；初始化可重複執行。Start、Build 與 Connect 採用一致接線。

## G 事件、常駐服務與認證遷移

| 通用契約／事件 | githubapp 責任 |
| --- | --- |
| Shared Settings（提案） | 提供並驗證共用 Client ID |
| 互動設定（提案） | Remote 選擇、ID 查詢、受限 Device Flow；必要時額外 discovery |
| Enable／更新 Host | 準備片段、腳本與 unit，啟動並驗證認證 |
| Start／Build／Connect Host | 檢查本機 Docker、runtime、資源、服務與認證 |
| ValidateContainerConfig | 驗證 mount、PATH 與接線；拒絕 Compose |
| Start／Build Container | up 成功後安裝／確認 Git 與 gh adapter |
| Connect Container | 開啟 shell 前安裝／確認 adapter |
| Disable Host | 停止服務，再依擁有權清理資源與自身 Token |
| Status | 顯示服務、授權帳號、待 Build 或 authorization-required |
| HostService.ServeHost | Socket server 與 user token provider；不得啟動瀏覽器 |

每 Project 一個 systemd user service；不設 timer，不自動啟用 linger。通用內部入口為 `_plugin-serve --plugin ID --project PATH`。入口依 registry 分派可選 HostService.ServeHost。

未知 ID、未支援 HostService 或無效 Project 明確報錯。不新增公開 plugin CLI 或公開 dch up。只有 githubapp 決定 unit、Socket 與 systemd 策略。

ExecStart 指向同一份 dch 的絕對路徑，正確處理 escaping。服務從自身已保存設定讀取參數，不帶秘密參數。

啟用透過實際 service 請求 credential，驗證 pass 與服務可用性。先完成 Device Flow 與身分確認；候選仍在記憶體。鎖內重驗設定版本後才保存並切換，再確認啟用。失敗保留原有效設定，只清理此次新建且確認擁有的資源。

已完成遠端 Token rotation 的情況依 F 的恢復規則處理。回滾或清理失敗保留重試 metadata，不虛報完成。

Start／Build／Connect 認證失敗就停止，不在 daemon 自動啟動瀏覽器。顯示到 Project Plugins 重新授權的操作指引。未啟用 Project 不啟動服務；各 Project 的服務、Socket、快取與 secret 獨立。

關閉 Console 或 Stop 容器後保留服務。沒有認證請求時不讀 pass 或呼叫 GitHub。Host 工作階段、GPG Agent 與 systemd 可用性仍是前提。

再次 Start 可補齊服務與 runtime；禁止兩程序接管同一 Socket。

共用 Client ID 變更只更新共用設定，不替任何 Project 換帳號或 Token。既有 Project 保留其已明確授權 Client ID 綁定，顯示需重新授權。舊認證仍綁原 Client ID，不因新共用值自動擴張或重新綁定。

使用者於各 Project 明確完成新受限 Device Flow 後，才切換綁定並重啟服務。變更 repo 亦須明確重新授權、範圍驗證及重啟服務；adapter／mount 未變時不新增待 Build。切換成功後只清理該 Project 擁有的舊 user token secret。取消或失敗保留舊綁定與可用認證；不影響其他 Project。

舊 Installation 設定不能自動轉成 user token。相容讀取保留原 App／Installation、repo 限制與擁有權資料，以供辨識及清理。升級偵測舊模式時停止舊認證供應，顯示 authorization-required。

未明確授權前不建立新 user service，也不繼續以 bot 冒充使用者。保留舊入口相容分派，確保既有 unit 能被辨識、停止及清理。不刪除、不移動使用者既有 private-key pass entry。

轉換失敗保留清理 metadata；不恢復 bot 模式作隱含替代。

停用或移除 Project 先停止服務、終止請求，再清理 Socket、unit 與受管理片段。成功停用刪除該 Project 擁有的 user token secret；失敗保留可重試記錄。舊 Client ID 的殘留 user secret 亦只按該 Project 擁有權清理。

不撤銷 GitHub 遠端授權或已交付的 access token。不刪其他 Project 的認證與共用 Client ID，也不刪舊私鑰。停用仍標記待 Build；既有容器 adapter 待重建移除。單純重新授權只更新 Host 憑證及重啟服務，不新增待 Build，也不清除原有待 Build。

移除 Project 保留工作區與容器，沿用既有確認行為。

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
| cli/oauth Device Flow | 真正 library 配假 HTTP／browser；驗證通常一次／fallback 額外授權、offline_access、slow_down、取消及每次受限 poll 的 repository_id |
| Remote 與 ID | 三種 URL、多 remote 選擇、origin 預設、無效／含憑證 URL；公開查詢、Host gh 既有認證、404 不假定 private、fallback／取消 |
| Repository | discovery 全分頁、匹配已選 remote、fallback user ID 一致；無效 ID 被忽略、範圍擴大／無法確認均拒絕 |
| 共用設定與互動 | metadata 表單、只收 Client ID、URL fallback、身分確認、候選版本衝突 |
| Token provider | 假 pass／HTTP 驗證期限、refresh 無 Client Secret、範圍再驗證及無效／撤銷授權 |
| Secret store | 完整 JSON 保存並讀回驗證；故障注入、遠端 rotation 後保存失敗及崩潰恢復 |
| 授權提交 | 取消／版本衝突不覆寫 active secret；帳號確認後才鎖內切換 |
| 跨程序鎖 | 同 Project refresh 一次；與生命週期不死鎖；不同 Project 不共用認證 |
| 認證遷移 | 舊模式須重新授權、保留擁有權、舊私鑰不刪；Client ID 改動不默換帳號 |
| Unix Socket | 真實暫存 Socket 驗證方法、endpoint、權限與錯誤 |
| 記憶體快取 | 可控時間驗證五分鐘邊界、讀 pass、重啟恢復、並行更新與失敗重試 |
| Systemd | 假 systemctl 驗證啟動、重啟、停止與清理失敗 |
| Git／gh | Git plumbing 與假 gh 驗證過濾、GH_TOKEN、參數及 exit code |
| CLI／Console | 只透過 registry 與契約提供表單、狀態、操作與內部入口 |
| 保密 | discovery 認證不進 pass／容器；日誌、設定、預覽、錯誤與命令參數均不含秘密 |

兩個不同欄位／state 的 fake plugin 僅存在測試；保留既有 GitHub 行為測試與 PATH 假指令 seam。一般測試不依賴真實 GitHub、Docker 或 systemd，不鎖定內部函式拆分。

核准後採 red → green，執行 go test ./...、go build ./...、go vet ./...。使用 Cognitive Complexity analyzer 檢查所有變動函式，含測試及匿名函式，皆不得超過 15；不抑制結果。

真實整合須留下操作證據，涵蓋 Docker Socket、UID 不符、服務重啟、Git／gh、關閉 dch 後認證、GPG 失敗及停用拒絕供應。驗證 PR 是授權使用者建立、gitconfig 不變、其他 Project 不受影響。真實 GitHub 必須證明初次與 refresh 後範圍枚舉恰為所選 repo；無法證明時列為未完成，不發布 Token。

不支援外部動態 plugin、下載協定、跨 plugin 相依性或通用工作流引擎。GitHub App 不支援 Compose、遠端 Docker、非 Linux、Enterprise 或 SSH 認證。不自動建立 App、安裝依賴或遷移舊 local.json。

不提供每 Project 權限 selector、容器 Token File、timer、排程刷新、PAT 或 Web Flow。Host pass 持久保存 Token 是本次交付範圍。

[gh api](https://cli.github.com/manual/gh_api)
[GitHub Get a repository](https://docs.github.com/en/rest/repos/repos#get-a-repository)
[gh 環境變數](https://cli.github.com/manual/gh_help_environment)
[curl Unix Socket 選項](https://curl.se/docs/manpage.html#--unix-socket)
[Linux Unix Socket 權限](https://man7.org/linux/man-pages/man7/unix.7.html)
[Docker bind mounts](https://docs.docker.com/engine/storage/bind-mounts/)

使用者已於「實作吧」核准本版規格並授權實作。通用架構的既有決定沿用；實作、測試及真實整合驗收結果另於 review 記錄，不因規格核准視為完成。

核准後依 dev-flow 更新本機 tickets、由子代理 TDD 實作及獨立審查。最終 review 核准後才 push 與建立 PR。
