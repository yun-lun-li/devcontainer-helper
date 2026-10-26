---
template: doc
theme: blueprint
lang: zh-Hant
title: GitHub 授權與 pass 設定：最終審查
subtitle: 五項行為待確認；自動審查無未解 finding
revision: 96796cf5e9e31431dc7ba10f587c86f49763757e
---

## A Executive Summary

- Project Plugins 新增授權方式選擇。預設推薦自備 GitHub App，也可使用 fine-grained PAT。
- App Client ID 可在本專案輸入。沿用目前值或共享預設值，不改寫其他專案。
- 設定流程先準備 GPG 與 pass，再開始 GitHub 授權。套件安裝、建金鑰均保留互動確認。
- PAT 隱藏輸入後驗證帳號與儲存庫。憑證存入 host pass，不寫入專案設定。
- 背景取憑證允許 GUI pinentry。整個請求最多等待 120 秒。
- App／PAT 切換保留回復與延後清理。兩輪審查後無未解 finding。

本次審查從 task base `7aa1998c882c665554bcedb30e5763d53a28846f` 起算。
實作 snapshot 是 `96796cf5e9e31431dc7ba10f587c86f49763757e`。
分支為 `feat/github-auth-pass-setup`。

預計 PR target 是 `main`，當時版本為 `34b3414`。
分支已承接先前的 Project Plugins 工作；這些工作尚未在 target main。
本頁只審查 task base 之後的授權與 pass 設定變更。
原有使用者未提交變更不在範圍內。

```callout warn 核准邊界
請確認 C 區五項決策，並在對話中明確給出最終核准。
勾選 checkbox 不會自動建立或發布 PR。
本頁尚未代表使用者核准。
```

## B Architecture & Change Map

`+` 表示新增，`~` 表示修改。無標記的節點是未修改的上下文。
本次沒有整個模組移除。
圖內箭頭標明呼叫、資料或外部程序。

```flow TB
[console/plugins.go]
+ [console/plugin_input.go]
[plugins/manager.go]
~ [githubapp/plugin.go]
+ [githubapp/host_setup.go]
+ [githubapp/host_install.go]
+ [githubapp/gpg.go]
+ [githubapp/pat.go]
~ [githubapp/manager.go]
~ [githubapp/provider.go]
~ [githubapp/secret.go]
~ [githubapp/server.go 與 adapter.go]
console/plugins.go -> plugins/manager.go: 呼叫 ConfigureAndEnable
plugins/manager.go -> githubapp/plugin.go: 呼叫 Configure
+ githubapp/plugin.go -> console/plugin_input.go: 介面呼叫 Input
+ githubapp/plugin.go -> githubapp/host_setup.go: 呼叫 prepareHost
+ githubapp/host_setup.go -> githubapp/host_install.go: 呼叫 installationPlan
+ githubapp/host_setup.go -> githubapp/gpg.go: 呼叫 parseSecretKeys
+ githubapp/plugin.go -> githubapp/pat.go: 呼叫 configurePAT
+ githubapp/pat.go --> githubapp/plugin.go: 資料 PreparedConfiguration
+ githubapp/pat.go --> githubapp/plugin.go: 資料，Commit closure 呼叫 commitCandidate
+ githubapp/plugin.go -> githubapp/manager.go: 呼叫 enable，經 commitCandidate
+ githubapp/server.go 與 adapter.go -> githubapp/provider.go: 請求與呼叫 Token
+ githubapp/provider.go -> githubapp/secret.go: 呼叫 Read／Write
```

| 邊界 | 本次責任 |
|---|---|
| `plugins/plugin.go`：修改 | 新增可選的 InputInteraction 與 HostCommandInteraction 介面 |
| `githubapp/config.go`：修改 | 分開 App／PAT 設定；保留舊 App 解讀 |
| `githubapp/manager.go`：修改 | credentialIdentity 型別、回復與清理身分 |
| `githubapp/host_cancel_test.go`：新增 | 子程序持有輸出 pipe 時的取消回歸測試 |
| `go.mod`：修改 | 既有 x/sys 從 indirect 改列直接依賴；未新增 library |
| `go.sum`：未修改 | 沿用既有 checksum |
| `README.md`：修改 | 記錄授權選擇、host 設定與使用限制 |

圖中 `githubapp/` 均指 `internal/plugins/githubapp/`。
`console/` 與 `plugins/` 均位於 `internal/`。
完整位置索引在 F 區。

## C 功能流程與審查決策

所有流程使用實作 snapshot 的函式名稱。
實線標示呼叫或外部程序啟動；虛線標示資料或狀態。
標為「型別」的節點不代表函式呼叫。

### C1 授權方式、Client ID 與 Device Flow

**需你 review：是。**

```flow TB
[使用者：Project Plugins 設定] -> [console/plugins.go · configurePlugin]: 呼叫
console/plugins.go · configurePlugin -> [plugins/manager.go · Manager.ConfigureAndEnable]: 呼叫
plugins/manager.go · Manager.ConfigureAndEnable -> [plugin.go · Plugin.Configure]: 介面呼叫
plugin.go · Plugin.Configure -> [plugin.go · chooseAuthenticationMethod]: 呼叫
plugin.go · Plugin.Configure -> [plugin.go · configuredAppClientID]: App 分支呼叫
plugin.go · configuredAppClientID --> [型別 Config · ClientID]: 資料，本專案值或共享預設
plugin.go · Plugin.Configure -> [host_setup.go · Plugin.prepareHost]: 呼叫，成功後繼續
plugin.go · Plugin.Configure -> [plugin.go · Plugin.chooseRepository]: App 分支呼叫
plugin.go · Plugin.Configure -> [plugin.go · Plugin.authorizeRepository]: App 分支呼叫
plugin.go · Plugin.authorizeRepository -> [plugin.go · Plugin.requestDeviceToken]: 呼叫
plugin.go · Plugin.requestDeviceToken -> [外部 GitHub Device Flow API／瀏覽器]: SDK 呼叫與 OpenBrowser
plugin.go · Plugin.authorizeRepository -> [repository.go · VerifyRepositoryScope]: 呼叫
plugin.go · Plugin.Configure -> [plugin.go · Plugin.prepareConfiguration]: 呼叫
plugin.go · Plugin.prepareConfiguration --> [型別 plugins.PreparedConfiguration]: 資料，設定與 Commit closure
```

App 使用 Device Flow；不要求使用者提供 client secret。
無 remote 時，App 路徑仍可探索可存取的儲存庫。
探索與授權帳號必須一致。
最終 App 授權必須確認只限選定儲存庫。

```html
<label><input type="checkbox" data-feature="app-device-flow" data-revision="96796cf"> 我已確認 C1：保留 App 為推薦選項；Client ID 僅綁本專案。</label>
```

需確認：共享 Client ID 只作為新設定預設值。
修改共享預設不會停用既有專案。

### C2 Host 安裝、金鑰、pass 初始化與 probe 清理

**需你 review：是。**

```flow TB
[系統入口：Plugin.Configure] -> [host_setup.go · Plugin.prepareHost]: 呼叫
host_setup.go · Plugin.prepareHost -> [host_setup.go · installRequirements]: 呼叫
host_setup.go · installRequirements -> [host_install.go · installationPlan]: 呼叫
host_install.go · installationPlan --> [型別 packagePlan]: 固定安裝命令資料
host_setup.go · installRequirements -> [host_setup.go · confirmHostInstall]: 有缺少工具時呼叫
host_setup.go · confirmHostInstall -> [console/plugin_input.go · consoleInteraction.RunHostCommand]: 確認後介面呼叫
console/plugin_input.go · consoleInteraction.RunHostCommand -> [外部 apt-get／dnf／pacman；必要時 sudo]: 程序啟動
host_setup.go · Plugin.prepareHost -> [host_setup.go · Plugin.initializePass]: 呼叫
host_setup.go · Plugin.initializePass -> [host_setup.go · applicableGPGID]: 呼叫，沿憑證路徑查 recipient
host_setup.go · Plugin.initializePass -> [host_setup.go · Plugin.createPassStore]: 沒有 recipient 時呼叫
host_setup.go · Plugin.createPassStore -> [host_setup.go · Plugin.selectPassRecipient]: 呼叫
host_setup.go · Plugin.selectPassRecipient -> [host_setup.go · Plugin.usableKeys]: 呼叫
host_setup.go · Plugin.usableKeys -> [gpg.go · parseSecretKeys]: 呼叫
host_setup.go · Plugin.selectPassRecipient -> [外部 gpg --full-generate-key]: 沒有可用金鑰且確認後啟動
host_setup.go · Plugin.createPassStore -> [host_setup.go · Plugin.initializeRecipient]: 重查 recipient 後呼叫
host_setup.go · Plugin.initializeRecipient -> [外部 pass init]: 程序啟動，使用目前使用者
host_setup.go · Plugin.prepareHost -> [host_setup.go · Plugin.verifyPass]: 呼叫
host_setup.go · Plugin.verifyPass -> [secret.go · SecretStore.runCaptured]: 呼叫，隨機 probe 寫入／讀回／移除
host_setup.go · Plugin.verifyPass --> [狀態：probe 確認移除才繼續授權]: 成功條件
```

只支援固定的 Debian、Fedora、Arch 安裝方案。
其他平台或沒有 sudo 時，顯示手動安裝指引。
只有套件安裝可用 sudo。
GPG 與 pass 操作使用目前使用者。

已有 recipient 時沿用設定，不重新初始化。
新 store 在選金鑰後重查 recipient，避免蓋掉中途建立的設定。
GPG parser 排除停用與不可用的加密金鑰。
離線 primary 配有可用 secret encryption subkey 時仍可使用。

probe 清理使用獨立的 10 秒 context。
清理失敗時停止授權，顯示可手動移除的 entry。
`runCommand` 使用 15 秒期限與 100 毫秒 WaitDelay。
WaitDelay 限制輸出 pipe 等待，不保證終止所有孫程序。

```html
<label><input type="checkbox" data-feature="host-bootstrap" data-revision="96796cf"> 我已確認 C2：允許互動安裝與初始化；取消後保留已完成的 host 變更。</label>
```

需確認：已安裝套件、新金鑰與 pass init 不會自動回復。
GPG 自行處理 passphrase；dch 不接收 passphrase。

### C3 PAT 隱藏輸入、驗證與綁定

**需你 review：是。**

```flow TB
[使用者：選擇 fine-grained PAT] --> [plugin.go · Plugin.Configure]: 選擇資料
plugin.go · Plugin.Configure -> [pat.go · Plugin.configurePAT]: host 準備成功後呼叫
pat.go · Plugin.configurePAT -> [pat.go · choosePATRepository]: 呼叫
pat.go · Plugin.configurePAT -> [console/plugin_input.go · consoleInteraction.Input]: secret=true 介面呼叫
console/plugin_input.go · consoleInteraction.Input -> [console/plugin_input.go · readWithTerminal]: 呼叫，先關閉 echo 再印 prompt
console/plugin_input.go · readWithTerminal -> [console/plugin_input.go · readTerminalInput]: 呼叫，結束時還原終端
pat.go · Plugin.configurePAT -> [pat.go · isFineGrainedPAT]: 呼叫，格式驗證
pat.go · Plugin.configurePAT -> [plugin.go · Plugin.fetchUser]: 呼叫 GitHub 帳號驗證
pat.go · Plugin.configurePAT -> [pat.go · lookupRepositoryWithToken]: 呼叫 GitHub repository metadata API
pat.go · Plugin.configurePAT -> [pat.go · Plugin.preparePATConfiguration]: 呼叫，確認帳號與儲存庫
pat.go · Plugin.preparePATConfiguration --> [型別 Config／TokenSet schema 2]: 資料，project／user／repository 綁定
pat.go · Plugin.preparePATConfiguration --> [型別 plugins.PreparedConfiguration]: 資料，Commit closure 延後存入 pass
[系統入口：Provider.Token] -> [provider.go · Provider.tokenLocked]: 經 SecretStore.WithLock 呼叫
provider.go · Provider.tokenLocked -> [provider.go · verifyGitHubUser]: PAT 分支透過 VerifyUser 呼叫
provider.go · Provider.tokenLocked -> [repository.go · VerifyRepositoryAccess]: PAT 分支透過 VerifyScope 呼叫
```

無 remote 時，PAT 可手動輸入 `owner/repo`。
秘密輸入拒絕非終端 stdin。
正常返回、取消或錯誤時均還原終端狀態。
PAT 不含 Client ID、refresh token 或虛構到期時間。
PAT 每次交付前重驗帳號與儲存庫。

```html
<label><input type="checkbox" data-feature="pat-binding" data-revision="96796cf"> 我已確認 C3：PAT 的儲存庫範圍與操作權限由我在 GitHub 選定。</label>
```

需確認：metadata API 無法證明 PAT 只限單一儲存庫。
公開儲存庫 metadata 可讀，不代表能 push。
到期、撤銷或組織未核准時，需要重新設定 PAT。
PAT 不會自動 refresh。

### C4 背景 GUI pinentry 與純 SSH 回復方式

**需你 review：是。**

```flow TB
[使用者：container 中執行 git／gh] -> [adapter.go · adapterScripts 產生的 shell adapter]: 外部程序執行
adapter.go · adapterScripts 產生的 shell adapter -> [server.go · credentialHandler]: curl 經 Unix socket 請求，125 秒上限
server.go · credentialHandler -> [provider.go · Provider.Token]: 呼叫，120 秒 context
provider.go · Provider.Token -> [secret.go · SecretStore.WithLock]: 呼叫，同一請求期限
secret.go · SecretStore.WithLock -> [provider.go · Provider.tokenLocked]: callback 呼叫
provider.go · Provider.tokenLocked -> [secret.go · SecretStore.Read]: 呼叫
secret.go · SecretStore.Read -> [secret.go · SecretStore.run]: 呼叫
secret.go · SecretStore.run -> [secret.go · SecretStore.runCaptured]: 呼叫
secret.go · SecretStore.runCaptured -> [provider.go · securePassEnvironment]: 呼叫，建立 pass 環境
secret.go · SecretStore.runCaptured -> [標準庫 os/exec · CommandContext／Cmd.Run]: 呼叫，建立並啟動程序
標準庫 os/exec · CommandContext／Cmd.Run -> [外部 pass／GPG agent／pinentry]: pass 程序啟動；GPG agent 處理解鎖
provider.go · securePassEnvironment --> [外部 pass／GPG agent／pinentry]: 環境資料，pinentry-mode ask
[manager.go · serviceUnitContent] --> [外部 systemd user service]: GUI session 環境資料
[外部 pass／GPG agent／pinentry] --> [狀態：成功回傳；失敗提示 host unlock 後重試]: 結果
```

service unit 帶入可用的 GUI session 環境。
pass 環境移除過時 GPG_TTY，指定 `--pinentry-mode ask`。
整個請求的 120 秒涵蓋鎖定、pass 與 provider。
GitHub API 仍保留較短的 HTTP timeout。

```html
<label><input type="checkbox" data-feature="pinentry-deadline" data-revision="96796cf"> 我已確認 C4：GUI 可提示解鎖；純 SSH 在 host 手動解鎖後重試，接受最多等待 120 秒。</label>
```

這項互動策略已於需求階段確認，最終實機行為仍未驗證。
dch 不修改 agent config，也不儲存 passphrase。

### C5 切換、回復、清理與舊資料相容

**需你 review：是。**

```flow TB
[使用者：完成設定或切換 App／PAT] -> [plugins/manager.go · Manager.ConfigureAndEnable]: 呼叫並驗證設定快照
plugins/manager.go · Manager.ConfigureAndEnable -> [plugin.go · Plugin.commitCandidate]: PreparedConfiguration.Commit closure 呼叫
plugin.go · Plugin.commitCandidate -> [manager.go · identityForConfig]: 呼叫
manager.go · identityForConfig --> [型別 credentialIdentity]: AuthMethod 與 Key 資料
plugin.go · Plugin.commitCandidate -> [plugin.go · saveCandidate]: 呼叫，保留前一份 secret
plugin.go · Plugin.commitCandidate -> [manager.go · Plugin.enable]: 呼叫
manager.go · Plugin.enable -> [manager.go · Plugin.rollbackEnable]: 啟用失敗時呼叫
plugin.go · Plugin.commitCandidate -> [secret.go · SecretStore.RestoreRaw／Delete]: 啟用失敗後回復候選憑證
plugin.go · Plugin.commitCandidate -> [plugin.go · recordCredentialRecoveryIdentity]: secret 回復失敗時呼叫
plugin.go · Plugin.commitCandidate -> [plugin.go · Plugin.cleanupPreviousCredential]: 啟用成功後呼叫
plugin.go · Plugin.cleanupPreviousCredential --> [型別 ownershipState]: 清理失敗保存待重試身分
[系統入口：Plugin.ensure] -> [manager.go · Plugin.retryCredentialCleanupFor]: 呼叫，保護 active identity
[使用者：停用／清理] -> [manager.go · Plugin.disable]: Host lifecycle 呼叫
manager.go · Plugin.disable -> [manager.go · Plugin.disableOwned]: 具所有權資料時呼叫
manager.go · Plugin.disableOwned -> [manager.go · Plugin.removeCredentialIdentities]: 呼叫，只處理本專案
```

App 舊 schema 1 與既有 pass entry 繼續可用。
PAT 使用獨立 schema 與 entry 身分。
清理不刪除目前使用中的身分。
secret 回復失敗會記錄 recovery 狀態，要求重新授權。

更舊的 installation credential 路徑仍拒絕使用。
`Plugin.rejectLegacy`／`Plugin.quarantineLegacy` 保留隔離行為。
這項隔離不是本次新增的遷移要求。

```html
<label><input type="checkbox" data-feature="switch-recovery" data-revision="96796cf"> 我已確認 C5：接受啟用失敗回復、清理失敗重試，以及回復失敗時重新授權。</label>
```

需確認：憑證回復與 host bootstrap 是不同範圍。
啟用失敗的回復不會移除 C2 已完成的 host 變更。

## D Behavior Changes

**本次未新增需使用者遷移的 breaking change。**
實機相容性仍受 E 區未執行項目限制。

| 項目 | 原行為 | 本次行為／遷移 |
|---|---|---|
| 授權選擇 | GitHub App | App 預設；新增 fine-grained PAT |
| Client ID | 使用既有設定流程 | 可於本專案輸入；沿用目前值或共享預設 |
| 共享預設變更 | 驗證會受既有專案 Client ID 影響 | 既有專案保留綁定，不因共享預設變更而停用 |
| Host 前置需求 | 需預先備妥 GPG／pass | 缺少時提供固定安裝方案與明確確認 |
| pass store | 使用既有 store | 沿用 recipient；新 store 可選金鑰並初始化 |
| 背景解鎖 | 原有期限不足以涵蓋互動 | 120 秒取憑證期限，允許 GUI pinentry |
| 純 SSH | 取憑證可能失敗 | 提示到 host 手動解鎖並重試 |
| App 既有資料 | schema 1 與既有 entry | 保留；無需搬移或重新授權 |
| PAT 生命週期 | 不支援 | 綁定帳號與儲存庫；失效後手動重設 |
| Host 取消 | 無此安裝導引 | 不回復已安裝套件、金鑰與 pass init |

## E Test & Verification

下列結果來自最終修正後的驗證紀錄。
此頁產生程序未重跑產品測試。

| 實際命令／範圍 | 結果 | 證據與界線 |
|---|---|---|
| `go test ./... -count=1` | PASS | 所有 package 通過；githubapp 21.677 秒 |
| `go vet ./...` | PASS | 無診斷 |
| `go build -o .scratch/github-auth-pass-setup/dch .` | PASS | 等效 build 成功 |
| `gofmt -l`，全部修改的 source／tests | PASS | 無檔名輸出 |
| `git diff --check`，task paths | PASS | 無 whitespace finding |
| `python3 .scratch/github-auth-pass-setup/check-complexity.py` | PASS | gocognit v1.3.0，26 個受影響 Go 檔 |
| `gocognit -test -exact-recursion`，由上述腳本呼叫 | PASS | 最大 Cognitive Complexity 15；無超限函式 |
| 真 PTY 隱藏輸入檢查 | PASS | prompt capture 無秘密回顯 |
| `make build` | Unavailable | repository 沒有 Makefile；上述 go build 通過 |
| 實際 apt／dnf／pacman 安裝 | Not run | 自動測試使用程序替身 |
| 真 GPG／pass bootstrap | Not run | 未實際安裝、建金鑰或初始化真 store |
| GUI pinentry／headless agent 解鎖 | Not run | 環境與 timeout 由替身測試覆蓋 |
| 真 systemd／Docker／GitHub 整合 | Not run | API、程序與 socket 替身不代表實機驗證 |
| HTML 靜態結構與未勾 checkbox | PASS | 原生 flow 渲染；五項獨立未勾 checkbox |
| 瀏覽器與手機／窄畫面視覺 | Not verified | 此環境沒有可用的瀏覽器工具 |

複雜度分析包含測試；匿名函式計入所在函式分數。
自動測試涵蓋 host setup、取消、PAT、binding 與切換回復。
尚未以實機驗證支援平台矩陣。

### 兩位 reviewer 結論

| Reviewer | 第一輪 | 第二輪，最終 snapshot |
|---|---|---|
| Code Review | Standards 0；Spec 0 confirmed findings | Standards 0；Spec 0 confirmed findings |
| Security Review | 兩個 P2：停用 GPG 金鑰、取消後輸出 pipe 延遲 | 兩項已解；無新 finding |

Code Review 覆蓋完整產品 diff、規範與核准 spec。
Security Review 覆蓋秘密處理、權限、binding 與清理。
也覆蓋鎖定、期限、儲存路徑與兩項回歸修正。
兩位 reviewer 均未把替身測試當成實機驗證。

停用金鑰修正在 `8d4ead8`。
`96796cf` 加入 100 毫秒 WaitDelay 並傳回取消錯誤。
`host_cancel_test.go` 驗證取消後不繼續 Device Flow。
本次使用兩輪自動審查，尚待使用者最終核准。

### 開發流程偏差

T01 部分 host 測試是在實作後補上。
T03 部分共用路徑回歸測試也是後補。
因此不能聲稱全部測試都嚴格遵守先 RED 再 GREEN。
已記錄的 RED／GREEN 證據仍保留於 scratch。
核心行為有自動覆蓋；此偏差不是未解程式 finding。

## F 文件、證據與實作位置索引

[完整變更 diff](2026-10-26-github-auth-pass-setup-changes.diff) 包含完整產品與測試變更。
它省略生成的 HTML，避免版面產物淹沒程式差異。
[已核准 spec](2026-10-26-github-auth-pass-setup-spec.md) 與 [spec HTML](2026-10-26-github-auth-pass-setup-spec.html) 是需求基準。
[本頁 Markdown](2026-10-26-github-auth-pass-setup-review.md) 保留 review 來源。

下列位置依 snapshot 核對。路徑均相對於本頁所在的 specs。

| 位置 | 核對入口與責任 |
|---|---|
| [console/plugins.go](../internal/console/plugins.go#L74) | configurePlugin；中斷感知 context |
| [console/plugin_input.go](../internal/console/plugin_input.go#L17) | Input；readWithTerminal:36；readTerminalInput:89；RunHostCommand:164 |
| [plugins/plugin.go](../internal/plugins/plugin.go#L124) | InputInteraction、HostCommandInteraction、PreparedConfiguration 型別 |
| [plugins/manager.go](../internal/plugins/manager.go#L118) | Manager.ConfigureAndEnable；快照檢查與 Commit |
| [githubapp/plugin.go](../internal/plugins/githubapp/plugin.go#L135) | Configure:135；configuredAppClientID:170；commitCandidate:241 |
| [App Device Flow](../internal/plugins/githubapp/plugin.go#L535) | authorizeRepository:535；requestDeviceToken:561；prepareConfiguration:222 |
| [host_setup.go](../internal/plugins/githubapp/host_setup.go#L18) | prepareHost:18；initializePass:65；createPassStore:75；verifyPass:213 |
| [host command 取消](../internal/plugins/githubapp/host_setup.go#L175) | runCommand；WaitDelay 與 context error |
| [host_install.go](../internal/plugins/githubapp/host_install.go#L15) | installationPlan；installationPlanFor；packageManagerPlanFor |
| [gpg.go](../internal/plugins/githubapp/gpg.go#L16) | parseSecretKeys；consumeKeyRecord；secretKey 型別 |
| [pat.go](../internal/plugins/githubapp/pat.go#L15) | configurePAT:15；lookupRepositoryWithToken:94；preparePATConfiguration:107 |
| [provider.go](../internal/plugins/githubapp/provider.go#L140) | Token:140；tokenLocked:161；securePassEnvironment:315 |
| [secret.go](../internal/plugins/githubapp/secret.go#L28) | SecretStore.Read／Write／WithLock／runCaptured |
| [repository.go](../internal/plugins/githubapp/repository.go#L349) | VerifyRepositoryScope；VerifyRepositoryAccess:360 |
| [server.go](../internal/plugins/githubapp/server.go#L94) | credentialHandler；120 秒 request context |
| [adapter.go](../internal/plugins/githubapp/adapter.go#L112) | adapterScripts；Git／gh adapter 外部 shell |
| [githubapp/manager.go](../internal/plugins/githubapp/manager.go#L22) | ownershipState:22；credentialIdentity:30；identityForConfig:35 |
| [cleanup 與回復](../internal/plugins/githubapp/manager.go#L233) | disable:233；retryCredentialCleanupFor:336；serviceUnitContent:798 |
| [host_cancel_test.go](../internal/plugins/githubapp/host_cancel_test.go#L13) | Configure 層級取消與子程序 pipe 回歸 |
| [config.go](../internal/plugins/githubapp/config.go#L57) | decodeConfig；Config.Validate；缺少 authMethod 解讀為 App |

### 暫存證據

以下連結供本機審查。成功建立 PR 後會清理 scratch。
重要驗證結果與 reviewer 結論已寫入 E 區。

- [整合驗證](../.scratch/github-auth-pass-setup/verification.md)、[複雜度結果](../.scratch/github-auth-pass-setup/complexity.json)。
- [PTY capture](../.scratch/github-auth-pass-setup/console-input-verification.txt)。
- [Code Review 第二輪](../.scratch/github-auth-pass-setup/reviews/code-round2.md)、[Security Review 第二輪](../.scratch/github-auth-pass-setup/reviews/security-round2.md)。
- [第二輪完整 source diff](../.scratch/github-auth-pass-setup/reviews/round2.diff)、[snapshot manifest](../.scratch/github-auth-pass-setup/reviews/round2-manifest.md)。
- [T01 證據](../.scratch/github-auth-pass-setup/t01-evidence.md)、[終端輸入證據](../.scratch/github-auth-pass-setup/t01-console-evidence.md)、[T02 證據](../.scratch/github-auth-pass-setup/t02-evidence.md)。
- [T03 證據](../.scratch/github-auth-pass-setup/t03-evidence.md)、[第一輪修正證據](../.scratch/github-auth-pass-setup/r1-fixes-evidence.md)。
