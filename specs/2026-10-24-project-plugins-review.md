---
title: Project Plugins 最終 Review
subtitle: github-app · hard dev-flow · 待使用者最終核准
lang: zh-Hant
template: doc
theme: shadcn
source: 34b3414 → 6f092a6
---

## A 本次交付

- 每個 Project 可在 Console 啟用內建 GitHub App plugin。
- Git 與 gh 透過 Host Unix Socket 取得短期 Token。
- 私鑰留在 Host pass；Token 只存 Host 記憶體。
- local.d 取代舊 local.json。這是不相容變更。
- 本輪依新增品質要求，將新增及修改函式複雜度控制在 15 以內。
- 自動檢查通過；真實 Linux 環境驗收仍未執行。

```callout warn 核准範圍
本頁有 6 個獨立 review 勾選框。
勾選只用於本頁核對，不儲存狀態。
全部勾選不會 push 或建立 PR。
仍需在對話中明確給予最終核准。
```

完整版本：`34b3414f2451e2e713a0880d3f533bd48e5b4614` → `6f092a63bbf1951934f163d0a19f291f3ec849d3`。

完整實作 diff 排除生成的審查頁與 diff 本身。

[完整實作 diff](2026-10-24-project-plugins-changes.diff) · [已核准規格](2026-10-24-project-plugins-spec.html)

## B 架構與變更地圖

```flow LR
+ G1 local.d -> 合併設定: 排序與 deep merge
+ G2 plugin manager -> G4 Host Socket: systemd user service
+ G3 provider -> pass 與 GitHub: JWT 與單一 repo Token
+ G4 Host Socket -> G3 provider: 按需取得與快取
+ G5 Git 與 gh adapter -> G4 Host Socket: HTTP over Unix Socket
+ G6 Console -> G2 plugin manager: 設定與生命週期
+ G6 Console -> 合併設定: Start Build Connect
~ 合併設定
```

G1–G6 對應下方功能流程與審查決策。
G7 是測試；G8 是文件。
「新增／修改」指群組內包含兩類檔案。

## C 功能流程與審查決策

以函式為單位，從使用者入口追到檔案內函式與後續影響。
實線標示函式呼叫或明示的外部操作；虛線標示資料、狀態或流程。
圖中群組是檔案；表格補充函式、型別與腳本責任。

名稱與呼叫關係依實作快照 `6f092a6` 核對。
本節不顯示原始碼或 diff。

### G1 修改：local.d 合併與預覽

確認舊 local.json 靜默停用。需要保留設定者須自行搬移。

需你 review：是。

```html
<label><input type="checkbox" id="check-G1"> 我已確認 G1 的決策與風險</label>
```

入口：Project → Show config。Start、Build、Connect 也共用相同合併函式。

```flow TB
(Project 選 Show config) -> runProjectAction: 使用者操作
runProjectAction -> showProjectConfig: 呼叫
showProjectConfig -> MergedDevcontainerConfig: 呼叫
MergedDevcontainerConfig -> readJSONCObject: 讀 base 與排序片段
MergedDevcontainerConfig -> mergeConfigValues: 逐片段合併
mergeConfigValues -> mergeConfigArrays: array 追加去重
MergedDevcontainerConfig --> showProjectConfig: 資料回傳：合併 JSON
showProjectConfig -> showConfigPages: 螢幕模式呼叫
showConfigPages --> configPager: 狀態：分頁與按鍵
showProjectConfig --> 終端標準輸出: 非螢幕模式
group internal/console/projects_menu.go: runProjectAction
group internal/console/config_preview.go: showProjectConfig, showConfigPages, configPager
group internal/common/devcontainer_config.go: MergedDevcontainerConfig, readJSONCObject, mergeConfigValues, mergeConfigArrays
```

| 檔案／函式 | 責任與後續影響 |
| --- | --- |
| `internal/common/devcontainer_config.go` · `MergedDevcontainerConfig` | 讀取 base，再按檔名處理 local.d 第一層正常 `.json` 檔。缺少 local.d 時只用 base。 |
| 同檔 · `readJSONCObject`、`mergeConfigValues`、`mergeConfigArrays` | 解析 JSONC；object 深合併；array 追加去重；其他值由後片段覆蓋。 |
| `internal/console/config_preview.go` · `showProjectConfig`、`showConfigPages`、`configPager` | 顯示合併結果。螢幕模式支援分頁，非螢幕模式直接輸出。 |
| `internal/console/devcontainer.go` · `prepareMergedProjectConfig` | 執行容器操作前也呼叫合併函式。啟用 plugin 時另驗證掛載與 PATH。後續流程見 G6。 |
| `.gitignore` | 忽略本 repository 的 local.d 設定。其他 Project 仍須自行設定忽略規則。 |

變更索引（檔案與新版本 hunk 起始行）：

- `internal/common/devcontainer_config.go:3,10,19,104`
- `internal/console/config_preview.go:18,43`
- `.gitignore:3`

### G2 新增／修改：Project plugin 管理

確認每個 Project 獨立設定與服務。摘要比對保護手改檔；失敗保留可重試狀態。

需你 review：是。

```html
<label><input type="checkbox" id="check-G2"> 我已確認 G2 的決策與風險</label>
```

入口：Project → Plugins → Configure and enable GitHub App。
Console 收集設定後呼叫 `Manager.Enable`。

```flow TB
(Console 確認啟用) -> Manager.Enable: 呼叫
Manager.Enable -> renderFragment: 呼叫
Manager.Enable -> replaceManagedFragment: 呼叫
Manager.Enable -> saveEnabledPlugin: 呼叫
Manager.Enable -> activateEnabledPlugin: 呼叫
saveEnabledPlugin -> SaveConfig: 呼叫
activateEnabledPlugin -> runSystemctl: 啟用或重啟服務
activateEnabledPlugin -> checkReady: 驗證 Socket 與 credential
activateEnabledPlugin -> enableRollback.run: 失敗時呼叫
SaveConfig --> ProjectPlugins 與 PluginState: 狀態：摘要與 BuildPending
group internal/plugins/manager.go: Manager.Enable, renderFragment, activateEnabledPlugin, runSystemctl, checkReady
group internal/plugins/manager_enable.go: replaceManagedFragment, saveEnabledPlugin, enableRollback.run
group internal/common/config_store.go: SaveConfig, ProjectPlugins 與 PluginState
```

| 檔案／函式或型別 | 責任與後續影響 |
| --- | --- |
| `internal/plugins/registry.go` · `Plugin`、`Builtins` | 宣告內建 github-app 資訊；此流程由 Console 直接管理 GitHub App。 |
| `internal/plugins/manager.go` · `Manager.Enable`、`renderFragment` | 驗證設定並鎖定 config；產生片段與 Project 專屬 runtime 路徑。 |
| 同檔 · `prepareServiceUnit`、`serviceUnitChange` | 準備 systemd user unit。使用內容摘要拒絕覆蓋手改 unit。 |
| `internal/plugins/manager_enable.go` · `replaceManagedFragment`、`saveEnabledPlugin` | 拒絕覆蓋未管理或手改片段；儲存設定、檔案摘要與待 Build 狀態。 |
| 同檔 · `enableRollback`、`ensureRollback` | 啟用失敗時還原檔案與設定。清理失敗則回報錯誤，保留可重試設定。 |
| `internal/common/config_store.go` · `Config`、`ProjectPlugins`、`PluginState`、`LoadConfig`、`SaveConfig` | 用正規化 Project 路徑分開儲存啟用設定與管理狀態；原子替換 config。 |
| `internal/plugins/manager.go` · `Manager.Ensure`、`Manager.Status`、`Manager.MarkBuilt` | `Ensure` 修復已啟用服務；停用時直接返回。Status 回報狀態；MarkBuilt 清除待 Build。 |
| 同檔 · `Manager.Disable`、`ownedUnitPath`、`removePluginFiles` | 驗證所有權後停止服務、刪除產物；成功才停用設定並標記待 Build。 |

已移除未使用的 `BuildFingerprint` 欄位與計算。
待 Build 判定仍使用 `BuildPending`；片段與 unit 摘要保護保持不變。

`Enable` 也呼叫 `WriteAdapterScripts` 與 `prepareServiceUnit`。
`checkReady` 呼叫 `githubapp.CheckCredential`，實際驗證 Token 可取得。
這些跨模組呼叫分別連到 G4 與 G5。

變更索引（檔案與新版本 hunk 起始行）：

- `internal/plugins/manager_enable.go:1`
- `internal/plugins/registry.go:1`
- `internal/plugins/manager.go:1`
- `internal/common/config_store.go:7`

### G3 新增：GitHub App provider

確認單一 repository 與選定權限。先用 App JWT 驗證 installation，再申請 Token。

需你 review：是。

```html
<label><input type="checkbox" id="check-G3"> 我已確認 G3 的決策與風險</label>
```

入口：Socket 的 `/credential` handler 呼叫 `Provider.Token`。
圖中 mint 的子步驟均由 `Provider.mint` 呼叫。

```flow TB
(Socket credentialHandler) -> Provider.Token: 呼叫
Provider.Token --> 記憶體 Token: 資料：有效期超過五分鐘
Provider.Token -> awaitRefresh: 已有更新時呼叫
Provider.Token -> Provider.refreshToken: 取得更新權時呼叫
Provider.refreshToken -> Provider.mint: 呼叫
Provider.mint -> Provider.loadPrivateKey: 呼叫
Provider.mint -> Provider.createAppJWT: 呼叫
Provider.mint -> Provider.verifyRepositoryInstallation: 呼叫
Provider.mint -> Provider.requestInstallationToken: installation 符合才呼叫
Provider.refreshToken --> Provider 與 refreshCall: 狀態：快取與等待者結果
group internal/githubapp/provider.go: Provider.Token, awaitRefresh, Provider.refreshToken, Provider.mint, Provider.loadPrivateKey, Provider.createAppJWT, Provider.verifyRepositoryInstallation, Provider.requestInstallationToken, Provider 與 refreshCall
```

| 檔案／函式或型別 | 責任與後續影響 |
| --- | --- |
| `internal/githubapp/config.go` · `Config`、`Config.Validate` | 限制正整數 ID、單一 owner/repository、contents 與 pull_requests 的 read/write。 |
| `internal/githubapp/provider.go` · `Provider`、`ProviderOptions`、`NewProvider` | 保存設定、逾時、HTTP client 與時鐘；停用 HTTP redirect。 |
| 同檔 · `Provider.Token`、`refreshCall`、`awaitRefresh`、`Provider.refreshToken` | mutex 同時保護快取判定與更新權；並行請求共用結果。更新失敗可由下次請求重試。 |
| 同檔 · `Provider.loadPrivateKey`、`securePassEnvironment`、`parsePrivateKey` | 非互動執行 pass；限制輸出大小；解析 Host RSA 私鑰。 |
| 同檔 · `Provider.createAppJWT`、`Provider.verifyRepositoryInstallation` | 簽發 App JWT；查詢該 repo 的 installation，必須符合設定 ID。 |
| 同檔 · `Provider.requestInstallationToken` | 只申請該 repo 與選定權限；驗證 Token 格式及有效期。結果只留 Host 記憶體。 |

變更索引（檔案與新版本 hunk 起始行）：

- `internal/githubapp/config.go:1`
- `internal/githubapp/provider.go:1`

### G4 新增：Host Unix Socket

確認信任邊界：可連 Socket 的程序可取 Token。目錄 0700、Socket 0600；不開 TCP。

需你 review：是。

```html
<label><input type="checkbox" id="check-G4"> 我已確認 G4 的決策與風險</label>
```

入口：systemd user unit 啟動 `dch _github-app-serve --project`。
這是獨立 Host 程序，不依賴 Console 保持開啟。

```flow TB
(systemd user unit) -> runGitHubAppServe: 程序入口經 CLI dispatch
runGitHubAppServe -> Manager.ServeProject: 呼叫
Manager.ServeProject -> RuntimeDir: 呼叫
Manager.ServeProject -> Serve: 呼叫
Serve -> prepareCredentialSocket: 呼叫
prepareCredentialSocket -> listenCredentialSocket: 驗證與鎖定後呼叫
listenCredentialSocket -> NewProvider: 呼叫
Serve -> credentialMux: 註冊 handlers
Serve -> serveUntilCancelled: 呼叫
credentialMux --> healthHandler 與 credentialHandler: HTTP 路由
group internal/commands/commands.go: runGitHubAppServe
group internal/plugins/manager.go: Manager.ServeProject
group internal/githubapp/runtime.go: RuntimeDir
group internal/githubapp/server.go: Serve, prepareCredentialSocket, listenCredentialSocket, credentialMux, serveUntilCancelled, healthHandler 與 credentialHandler
group internal/githubapp/provider.go: NewProvider
```

| 檔案／函式 | 責任與後續影響 |
| --- | --- |
| `internal/commands/commands.go` · `runCommand`、`runGitHubAppServe` | 分派內部命令；接收 SIGTERM／interrupt 並取消服務 context。 |
| `internal/plugins/manager.go` · `Manager.ServeProject` | 只讀取該 Project 已啟用設定；呼叫 RuntimeDir 與 Serve。 |
| `internal/githubapp/runtime.go` · `RuntimeDir`、`ensurePrivateDir`、`ownedByCurrentUser` | 驗證 XDG_RUNTIME_DIR 所有權；由 Project 路徑雜湊分隔 0700 目錄。 |
| `internal/githubapp/server.go` · `prepareCredentialSocket`、`acquireServerLock`、`listenCredentialSocket` | 驗證父目錄、排除重複 server、清除舊 Socket；建立 0600 Unix Socket。 |
| 同檔 · `credentialMux`、`healthHandler`、`credentialHandler` | `/health` 只回狀態；`/credential` 呼叫 Provider.Token，回應帶 no-store。只接受 GET。 |
| 同檔 · `serveUntilCancelled` | 執行 HTTP server；context 取消時關閉 server 並移除 Socket。 |
| 同檔 · `CheckCredential`、`unixHTTPClient` | Manager 的啟用驗證走 Unix Socket，檢查 credential 回應且不輸出 Token。 |

變更索引（檔案與新版本 hunk 起始行）：

- `internal/githubapp/runtime.go:1`
- `internal/githubapp/server.go:1`

### G5 新增：Git／gh 容器接線

確認唯讀掛載整個 runtime 目錄。HTTPS github.com 使用 helper；gh 子程序取得 GH_TOKEN。

需你 review：是。

```html
<label><input type="checkbox" id="check-G5"> 我已確認 G5 的決策與風險</label>
```

入口有兩段：Manager 產生接線檔案；Start、Build、Connect 安裝容器端設定。
腳本名稱是 shell 產物，不是 Go 函式。

```flow TB
(Manager 準備接線) -> AdapterFragment: renderFragment 呼叫
(Manager 準備接線) -> WriteAdapterScripts: Enable 或 Ensure 流程呼叫
AdapterFragment --> local.d 片段: 資料：唯讀 mount 與 PATH
WriteAdapterScripts -> adapterScripts: 呼叫
adapterScripts --> Git helper 與 gh wrapper: 資料：腳本內容
(Start Build Connect) -> runAdapterInstall: 呼叫
runAdapterInstall -> AdapterInstallCommand: 呼叫
runAdapterInstall -> invokeProjectDevcontainerQuiet: 容器 exec
AdapterInstallCommand --> 容器安裝腳本: 資料：shell 命令
容器安裝腳本 --> Git 與 gh 使用 Socket: 設定 Git include 與 PATH
group internal/githubapp/adapter.go: AdapterFragment, WriteAdapterScripts, adapterScripts, AdapterInstallCommand
group internal/console/connect.go: runAdapterInstall
group internal/console/devcontainer.go: invokeProjectDevcontainerQuiet
```

| 檔案／函式或產物 | 責任與後續影響 |
| --- | --- |
| `internal/githubapp/adapter.go` · `AdapterFragment` | 輸出整個 runtime 目錄的唯讀 mount，並將 bin 放入 remoteEnv PATH。 |
| 同檔 · `WriteAdapterScripts`、`adapterScripts`、`writePrivateFile` | 驗證 runtime 所有權與權限；原子寫入非機密腳本及 gitconfig。 |
| 同檔 · `AdapterInstallCommand`、`adapterInstallScript` | 回傳 shell 安裝命令；檢查工具與 Socket health；加入 Git include，不改既有 lifecycle hooks。 |
| 同檔 · `gitCredentialScript` | 只對 get、HTTPS、github.com 取 `/credential`；其他操作或 Host 不輸出憑證。 |
| 同檔 · `adapterGitConfig` | 讓 HTTPS github.com 使用專屬 helper；保留其他 Host 原有 helper。 |
| 同檔 · `ghWrapperScript` | 每次呼叫都向 Socket 取憑證；只對真正 gh 子程序設定 GH_TOKEN；保留參數與退出狀態。 |
| `internal/console/connect.go` · `runAdapterInstall` | 以 devcontainer exec 執行安裝命令；服務或 UID 驗證失敗時阻止後續操作。 |

後續：容器內 Git／gh 透過 Unix Socket 呼叫 G4，再由 G3 提供短期 Token。
掛載目錄保留相同位置，服務重啟後可看到替換的 Socket。

變更索引（檔案與新版本 hunk 起始行）：

- `internal/githubapp/adapter.go:1`

### G6 新增／修改：Console 與執行生命週期

確認既有容器待 Build 時阻擋 Start／Connect。Build 保留移除確認；移除前清理服務。

需你 review：是。

```html
<label><input type="checkbox" id="check-G6"> 我已確認 G6 的決策與風險</label>
```

入口：Project 選單選 Plugins、Start、Build、Connect 或 Remove Project。
`runProjectAction` 分派前四項；移除由 `performProjectMenuAction` 處理。

```flow TB
(Project 選 Plugins) -> runProjectAction: 使用者操作
runProjectAction -> pluginsMenu: 呼叫
pluginsMenu -> choosePluginAction: 顯示 Status
pluginsMenu -> handlePluginAction: 呼叫
handlePluginAction -> enableProjectPlugin: 選啟用
enableProjectPlugin -> promptGitHubAppConfig: 呼叫
enableProjectPlugin -> Manager.Enable: 確認後呼叫
handlePluginAction -> Manager.Disable: 選停用
group internal/console/projects_menu.go: runProjectAction
group internal/console/plugins.go: pluginsMenu, choosePluginAction, handlePluginAction, enableProjectPlugin, promptGitHubAppConfig
group internal/plugins/manager.go: Manager.Enable, Manager.Disable
```

容器執行流程分為守門、設定準備及成功後收尾。
圖中虛線表示流程或狀態，不代表直接呼叫。

```flow TB
(Start Build Connect) --> startProject buildProject connectProject: 選單分派
startProject buildProject connectProject --> 狀態守門與確認: Status 與 BuildPending
startProject buildProject connectProject -> prepareProjectPlugin: 啟用 plugin 時呼叫
prepareProjectPlugin -> requireLocalDocker: 呼叫
prepareProjectPlugin -> Manager.Ensure: 呼叫
startProject buildProject connectProject -> invokeProjectDevcontainer: 呼叫
invokeProjectDevcontainer -> invokeProjectDevcontainerMode: 呼叫
invokeProjectDevcontainerMode -> prepareMergedProjectConfig: 呼叫
prepareMergedProjectConfig -> MergedDevcontainerConfig: 呼叫
prepareMergedProjectConfig -> validateGitHubAppConfig: 啟用 plugin 時呼叫
invokeProjectDevcontainerMode --> devcontainer up 或 exec: 轉換設定後執行
group internal/console/up.go build.go connect.go: startProject buildProject connectProject
group internal/console/plugin_integration.go: prepareProjectPlugin, requireLocalDocker, validateGitHubAppConfig
group internal/console/devcontainer.go: invokeProjectDevcontainer, invokeProjectDevcontainerMode, prepareMergedProjectConfig
group internal/common/devcontainer_config.go: MergedDevcontainerConfig
group internal/plugins/manager.go: Manager.Ensure
```

`startProject buildProject connectProject` 是圖中三個入口的合併標籤。
實際函式名稱與各自責任如下。

| 檔案／函式或型別 | 責任與後續影響 |
| --- | --- |
| `internal/console/projects_menu.go` · `projectAction`、`projectMenu`、`availableProjectActions` | 顯示 plugin 狀態；既有容器待 Build 時隱藏 Start／Connect。 |
| `internal/console/plugins.go` · `pluginsMenu`、`promptGitHubAppConfig` | 顯示服務狀態；收集 App ID、installation ID、pass entry、repo、權限並確認。 |
| `internal/console/up.go` · `startProject`、`hasExistingContainer` | 再次檢查待 Build；成功 up 後安裝 adapter。新建容器成功才 MarkBuilt。 |
| `internal/console/build.go` · `buildProject`、`confirmRebuild` | 既有容器先確認丟棄 writable-layer 資料；選快取、重建、安裝 adapter；成功才 MarkBuilt。 |
| `internal/console/connect.go` · `connectProject`、`runAdapterInstall` | 再查待 Build 與 RUNNING；確認服務及安裝 adapter 後，以 devcontainer exec 開啟 shell。 |
| `internal/console/plugin_integration.go` · `prepareProjectPlugin`、`requireLocalDocker` | 確認 Docker 使用本機 Unix endpoint，再呼叫 `Ensure` 確認 Host 服務。 |
| 同檔 · `adapterSettings`、`validateGitHubAppConfig`、`requiredAdapterSettings`、`validateAdapterMounts`、`hasAdapterPath` | 禁止 Compose、缺失或重疊掛載、覆蓋 PATH；防止 local.d 破壞必要接線。 |
| `internal/console/devcontainer.go` · `invokeProjectDevcontainerMode` | 合併後補穩定容器名、轉換相對路徑、寫暫存 config、加入 CLI 參數；完成後清理。 |
| 同檔 · `prepareMergedProjectConfig`、`withStableContainerName`、`rebaseConfigPaths`、`appendDevcontainerConfigArgs` | 合併驗證、容器名稱、路徑重定位與 config/id-label 參數各由專責函式處理。 |

移除流程在清理成功後才取消註冊。

```flow TB
(Project 選 Remove Project) -> performProjectMenuAction: 選單操作
performProjectMenuAction -> confirmProjectRemoval: 呼叫
confirmProjectRemoval -> removeProject: 使用者確認後呼叫
removeProject -> Manager.Disable: 啟用時先清理
removeProject -> RemoveProjectRegistration: 清理成功才呼叫
RemoveProjectRegistration -> SaveConfig: 移除註冊與 plugin metadata
group internal/console/projects_menu.go: performProjectMenuAction, confirmProjectRemoval, removeProject
group internal/plugins/manager.go: Manager.Disable
group internal/common/remove_project.go: RemoveProjectRegistration
group internal/common/config_store.go: SaveConfig
```

變更索引（檔案與新版本 hunk 起始行）：

- `internal/console/plugins.go:1`
- `internal/console/plugin_integration.go:1`
- `internal/console/projects_menu.go:1,17,30,51,225,241`
- `internal/console/devcontainer.go:1,11,70,103`
- `internal/console/up.go:1`
- `internal/console/build.go:1,33`
- `internal/console/connect.go:1`
- `internal/common/remove_project.go:2,24`
- `internal/commands/commands.go:1,114`

### G7 新增／修改：行為測試

測試是上述行為的證據。涵蓋假指令、真實暫存 Socket 與 Git plumbing。

需你 review：否。

測試入口：`go test ./... -count=1`。
執行範圍包含合併、狀態、回滾、憑證快取、Socket 與容器接線。

回歸函式 `TestEnsureDoesNothingForDisabledPluginWithPersistedState` 位於 `internal/plugins/manager_test.go`。
它確認停用且保留待 Build 狀態時，`Ensure` 不啟動服務。
此案例經 red → green；完整驗證結果見 F。

本輪更新 `TestEnableRestartsActiveServiceWhenConfigurationChanges`。
首次啟用後先 MarkBuilt，再確認修改設定會重新標記待 Build。

變更索引（檔案與新版本 hunk 起始行）：

- `internal/commands/plugin_serve_test.go:1`
- `internal/common/common_test.go:65`
- `internal/common/config_store_test.go:1`
- `internal/common/remove_project_test.go:1`
- `internal/console/build_plugin_test.go:1`
- `internal/console/config_preview_test.go:1`
- `internal/console/devcontainer_plugin_test.go:1`
- `internal/console/plugin_integration_test.go:1`
- `internal/console/plugins_test.go:1`
- `internal/console/projects_menu_test.go:10`
- `internal/githubapp/adapter_test.go:1`
- `internal/githubapp/config_test.go:1`
- `internal/githubapp/provider_test.go:1`
- `internal/githubapp/server_test.go:1`
- `internal/plugins/manager_test.go:1`

### G8 新增／修改：文件與核准規格

README 說明操作、限制與人工驗收。規格 HTML 為已核准規格的呈現產物。

需你 review：否。

這組只有文件與 Git 屬性，沒有函式呼叫流程。
README 說明 local.d 遷移、Project plugin 操作與 Linux 人工驗收。
規格 Markdown 與 HTML 保留已核准需求；`.gitattributes` 標記 HTML 產物。

變更索引（檔案與新版本 hunk 起始行）：

- `.gitattributes:1`
- `README.md:5,21,57,68`
- `specs/2026-10-24-project-plugins-spec.md:1`
- `specs/2026-10-24-project-plugins-spec.html:1`

## D 使用行為與相容性

| 面向 | 原行為 | 最終行為 |
| --- | --- | --- |
| 個人容器設定 | 讀取 devcontainer.local.json | 只讀 local.d 第一層正常 .json 檔 |
| 合併 | 單一 local 檔 | 檔名排序；JSONC；object 深合併；array 追加去重 |
| 認證 | 使用者自備 | Console 驗證服務與 credential 後啟用 |
| 設定變更 | 無 plugin 狀態 | 既有容器標記 Build required；阻擋 Start／Connect |
| 停用 | 無 plugin 服務 | 停止服務並清理；容器設定需 Build 才恢復 |
| 移除 Project | 取消註冊 | 先清理 plugin；失敗保留設定供重試 |
| Git | 使用既有 helper | 僅 HTTPS github.com 走 plugin；其他 Host 保留原 helper |
| gh | 原有命令 | wrapper 每次取憑證；只對該子程序設定 GH_TOKEN |

```callout warn Breaking change：舊 local.json 使用者
舊檔會被靜默忽略，不提示也不自動搬移。
若要保留設定，請手動搬到 local.d 的 .json 片段。
例如：`.devcontainer/devcontainer.local.d/90-user.json`。
請自行將個人片段加入版本控制忽略規則。
```

新建容器可由 Start 套用設定。
既有容器需 Build，且保留資料移除確認。
服務在關閉 dch 或 Stop 容器後保留。
停用不會撤銷已回傳的短期 Token。

## E 認證與信任邊界

```sequence num
Git 或 gh -> Socket: GET /credential
Socket -> Provider: 請求 Token
Provider -> Provider: 檢查記憶體快取
Provider -> pass: 需要更新才讀取私鑰
Provider -> GitHub: App JWT 驗證 repo installation
GitHub --> Provider: installation ID 必須符合設定
Provider -> GitHub: 申請單一 repo 與選定權限
Provider --> Socket: 短期 Token
Socket --> Git 或 gh: no-store 回應
```

快取剩餘時間超過五分鐘時直接重用。
剩餘五分鐘以下時，由下一次請求更新。
並行請求共用一次更新；沒有 timer 或 Token File。

| 邊界 | 限制 |
| --- | --- |
| Host | Linux、本機 Docker、systemd user service、pass／GPG |
| 容器 | image／Dockerfile；Git 2.30+、gh、curl、POSIX sh |
| Socket | 目前使用者擁有的 XDG_RUNTIME_DIR；目錄 0700、Socket 0600 |
| 掛載 | 整個 runtime 目錄唯讀；服務重啟後可看見新 Socket |
| 信任 | 可連 Socket 的程序可取得該 Project Token；Docker 管理員亦可存取 |
| 不支援 | Compose、遠端 Docker、GitHub Enterprise、SSH 認證、多 repo Token |

容器 UID 必須能連線 Socket；錯誤訊息提示檢查 UID。
此環境未驗證真實容器 UID 與重啟行為。

## F 測試與獨立 Review

本輪使用者新增 Cognitive Complexity ≤15 要求。
先前已抽出具名 helper，保留既有行為。
本輪依最新要求移除未使用的 BuildFingerprint。
既有 JSON 的舊欄位會被忽略；下次存檔時省略。

| 複雜度檢查 | 結果 |
| --- | --- |
| 工具 | gocognit v1.3.0；-json -exact-recursion . |
| 判定範圍 | 相對 34b3414；本功能新增／修改 266 個具名函式，包含測試 |
| 匿名函式 | 納入所屬函式計分 |
| 最終值 | 最高 15；0 項違規 |

| Manager 函式 | 重構前 | 重構後 |
| --- | --- | --- |
| Enable | 67 | 11 |
| `Ensure` | 26 | 12 |
| Disable | 34 | 9 |
| prepareServiceUnit | 25 | 5 |

新增回歸測試確認：停用狀態的 `Ensure` 不啟動服務。
Token 快取檢查、加入等待與刷新擁有權維持同一個 critical section。

以下八個 legacy 函式完全未修改，超過 15，屬本輪範圍外。
此結果不表示整個 repository 已無既存違規。

| 未修改函式 | 複雜度 |
| --- | --- |
| `internal/common/project.go` · `jsoncToJSON` | 56 |
| `internal/commands/update.go` · `update` | 53 |
| `internal/console/menu.go` · `arrowChoose` | 46 |
| `internal/console/logs.go` · `showLogs` | 41 |
| `internal/console/projects_menu_test.go` · `TestStopHandlesContainersDisappearing` | 40 |
| `internal/console/stop.go` · `stopProject` | 26 |
| `internal/console/devcontainer.go` · `rebaseConfigPaths` | 26 |
| `internal/console/menu.go` · `chooseView` | 19 |


| 指令或範圍 | 結果 | 證據與限制 |
| --- | --- | --- |
| go test ./... -count=1 | PASS | 最終版本全套測試；由主代理執行 |
| go build ./... | PASS | 最終版本編譯；由主代理執行 |
| go vet ./... | PASS | 最終版本靜態檢查；由主代理執行 |
| git diff --check | PASS | 最終版本格式檢查；由主代理執行 |
| go run . help | NOT RUN | 本輪未重跑；先前版本 smoke check 已通過 |
| 設定與生命週期 | PASS | 排序／JSONC／手改保護／回滾／清理重試／待 Build |
| Provider 與 Socket | PASS | 假 pass、httptest、可控時間、真實暫存 Unix Socket |
| 容器接線 | PASS | 假 devcontainer／systemctl／Docker context；非真實整合 |
| Git／gh | PASS | 真實 Git credential plumbing；假 curl 與 gh |
| go test -race ./... | NOT RUN | CGO 關閉，且無 C compiler |
| 真實 Linux 驗收 | NOT RUN | 缺 Docker／systemctl／pass；未用真實 GitHub 與 GPG |
| UID／重啟／關閉 dch 後認證 | NOT RUN | 需在真實 Linux 容器執行 README 驗收 |
| HTML 功能流程 | PASS | G1–G6 對照 6f092a6 函式、型別與呼叫；40 個實作變更檔案均列入索引 |
| HTML 產生與內容 | PASS | am render --no-open；9 個 flow；0 項 STE 警告；C 無原始碼或 diff；六個未勾選框 |
| HTML 流程與版面 | PASS | 主代理覆核 6f092a6；9 個 flow 可見；G1–G8 與關鍵函式齊全；1440×900、390×844 無水平溢出 |
| HTML 互動 | PASS | 六框初始未勾選，可勾選及取消；0 項 CSP 錯誤；無舊左右對照或完整原始碼 |

本輪 6f092a6 流程版已由主代理以獨立 Chromium 覆核。
檢查涵蓋 DOM、流程圖、勾選互動與視窗寬度。
未驗收中文字型顯示。
真實 Linux 整合驗收仍未執行。

Root 執行測試時，DAC 拒絕案例會跳過。
假 health 失敗與 UID 錯誤提示仍有測試。
這不代表真實 UID 不符驗收已完成。

| 獨立 reviewer | 範圍 | 最終結論 |
| --- | --- | --- |
| Code reviewer（歷史） | d1591ef → 0fecc5e；完整功能重構的 Standards、Spec 與複雜度 | 0 項未解決有效 finding |
| Security reviewer（歷史） | 截至 0fecc5e；憑證邊界、快取互斥、生命週期與設定 | 0 項未解決可採取行動的安全 finding |
| Standards 與 Spec reviewer（本輪） | 5f7ac13 → 6f092a6；欄位、計算移除與待 Build 回歸測試 | 各 0 項 finding |
| Security reviewer（本輪） | 5f7ac13 → 6f092a6；狀態移除、所有權摘要與舊 JSON 相容性 | 0 項安全 finding |

本輪獨立報告：`fingerprint-standards.md`、`fingerprint-spec.md`、`fingerprint-security.md`。
報告位於 `.scratch/project-plugins/reviews/`。
獨立 source review 不取代真實環境驗收。

## G 真實擷取與核准提示

以下是測試擷取節錄，Project 路徑已去識別化。
來源：`TestPluginsStatusIsReachableWithFakeDockerAndSystemctl`。
Docker 與 systemctl 是假指令，並非現場整合畫面。

```text title="實際測試輸出節錄"
Project plugins

  1) GitHub App: enabled · service active · Build required
  2) Configure and enable GitHub App
  3) Disable GitHub App
  4) Back
Choose number (q to cancel): GitHub App: enabled · service active · Build required

```

請先確認 C 的 6 個決策，再給予最終核准。
本頁顯示功能流程；完整實作 diff 另附參考連結。
未讀取或嵌入使用者本機憑證與 local 設定。
本次交付只建立 review 文件，不執行 PR 或 push。
