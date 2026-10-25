---
title: Project Plugins 最終 Review
subtitle: 通用 plugin 架構與 github-app · 待使用者最終核准
lang: zh-Hant
template: doc
theme: shadcn
source: 34b3414 → 823da26
---

## A 本次交付

- main 註冊完整 GitHub App 實作，並注入 Console 與 CLI。
- 通用核心只理解 Plugin 契約、通用狀態與 opaque JSON。
- 新增 plugin 只需實作契約並在 main 註冊。
- Console 依 metadata 產生欄位、選項與逐 plugin 狀態。
- Host 與 Container hook 都是 Host Go 程式。
- Container 指令由注入的 executor 經 devcontainer exec 執行。
- GitHub 認證、服務、回滾與接線全部移入 plugins/githubapp。
- 正式 plugin 只有 github-app；兩個不同 fake plugin 只存在測試。
- GitHub 表單只詢問 App ID、Installation ID 與兩種權限。
- 預設 pass 路徑由 App ID 產生；舊自訂路徑仍有效。
- 未設定 repository 時，Token 可存取 installation 內所有可用 repository。
- 舊 repository 限制仍有效；至少必須選擇一項權限。
- ID 或欄位型別錯誤顯示固定安全訊息，不回顯使用者輸入。
- Installation ID 需由使用者填入；Notices 指出 installation settings URL 的數字。
- local.d 取代舊 local.json。此不相容變更維持原決定。
- 真實 Linux、Docker、systemd 與 GitHub 驗收仍未執行。

```callout warn 核准範圍
本頁有 6 個獨立 review 勾選框，初始均未勾選。
勾選只用於本頁核對，不儲存狀態。
全部勾選不會 push 或建立 PR。
仍需在對話中明確給予最終核准。
```

完整功能基準：`34b3414f2451e2e713a0880d3f533bd48e5b4614`。
架構修訂比較基準：`e1b666a`；先前實作快照：`9cc1f5f`。

T20 修訂基準：`29e8e6b`；先前實作快照：`5451f85`。
T21 修訂基準：`73050d4`；最終實作快照：`823da2667f6a542a78b1072dfcde9311ddd94bb5`。
完整實作 diff 排除生成的審查頁與 diff 本身。

[完整實作 diff](2026-10-24-project-plugins-changes.diff) · [已核准架構規格](2026-10-24-project-plugins-spec.html)

## B 架構與變更地圖

```flow TB
+ main.go -> 通用 Manager: 建立並 Register
+ main.go -> githubapp.New: 建立正式實作
+ main.go -> Console 與 Commands: 注入同一 Manager
+ 通用 Manager -> Plugin interface: 依穩定 ID 排序分派
+ Plugin interface --> githubapp.Plugin: 契約實作
+ 通用 Manager -> common config store: 逐 plugin 狀態與 scoped JSON
+ githubapp.Plugin -> Host 服務與認證: Host hook
+ githubapp.Plugin -> 注入 Executor: Container hook 在 Host 呼叫
+ 注入 Executor -> devcontainer exec: 外部程序
~ main.go
- 舊核心 GitHub 專屬協調
- 舊 Console GitHub 專屬表單
```

標記以架構修訂前 `e1b666a` 為基準。
舊 internal/githubapp 與核心 GitHub 協調已移入具體實作子樹。
下列目錄樹描述實際完成後的檔案，不是提案。

```tree list
main.go | 唯一組裝入口：建立、註冊與注入 manager
internal/
  plugins/
    plugin.go | Plugin、HostService、Executor 與資料契約
    manager.go | registry、排序分派、逐 plugin 狀態與 Project 鎖
    manager_test.go | 兩個不同 fake plugin 的通用管理行為
    manager_lifecycle_test.go | 事件、pending、鎖與失敗重試
    githubapp/
      plugin.go | metadata、驗證、Host 與 Container hook
      config.go | GitHub 參數與權限
      manager.go | GitHub 資源、回滾、systemd 與 ServeHost
      container_config.go | 本機 Docker、mount、PATH 與 Compose 限制
      adapter.go | 容器 shell 腳本、Git helper 與 gh wrapper
      provider.go | Host pass、JWT、GitHub API 與記憶體快取
      server.go | Host Unix Socket HTTP server
      runtime.go | Project 專屬私有 runtime 路徑
  common/
    config_store.go | opaque JSON、相容解碼與跨 Project 更新鎖
    devcontainer_config.go | local.d 合併
    remove_project.go | 註冊清理
  console/
    console.go | 接收 manager
    projects_menu.go | 動作分派與 Project 狀態
    plugins.go | metadata 驅動的通用表單與選單
    up.go | Start 鎖內生命週期
    build.go | Build 確認與鎖內生命週期
    connect.go | Connect 守門、hook 與 shell 啟動
    devcontainer.go | 合併驗證、容器 executor 與暫存設定
    config_preview.go | 合併預覽
  commands/
    commands.go | 通用 HostService 入口與 metadata alias
    register.go | 註冊時協調共享設定寫入
```

| 模組／檔案 | 責任與依賴 |
| --- | --- |
| `main.go` | 唯一同時認識通用 manager 與 githubapp 的組裝入口。建立實例後注入 Console／Commands。 |
| `internal/plugins/plugin.go` | 公開契約與通用表單描述；不引用具體 GitHub 型別。 |
| `internal/plugins/manager.go` | 註冊、ID 驗證、Host／Container 分派、scoped store 與待 Build 聯集。 |
| `internal/common/config_store.go` | RawMessage 儲存與舊 metadata 相容；讀寫使用短期共享設定鎖。 |
| `internal/plugins/githubapp/plugin.go` | 將 GitHub 實作接上通用事件；它自行提供表單與安全訊息。 |
| `internal/plugins/githubapp/manager.go` | 完整資源生命週期、所有權摘要、回滾、systemd 與常駐服務。 |
| `internal/plugins/githubapp/container_config.go` | 所有 GitHub 專屬 Docker 與最終設定限制。 |
| `internal/plugins/githubapp/provider.go`、`server.go`、`runtime.go` | Host 認證與 Socket 邊界。私鑰不進容器。 |
| `internal/plugins/githubapp/adapter.go` | 容器端 shell 產物；不包含容器內 dch binary。 |
| `internal/console/` | 顯示通用 metadata、執行 Project 操作與提供容器 executor。 |
| `internal/commands/commands.go` | 依 manager 查找 HostService 與 alias；不硬編碼 GitHub ID。 |

所有測試仍與受測 package 並置；完整檔案索引見 C。

## C 功能流程與審查決策

以函式為單位，從使用者入口追到檔案內函式與後續影響。
實線表示呼叫或明示的外部程序操作；虛線表示資料、狀態或流程。
檔案群組與責任表共同列出實際函式、型別與腳本名稱。

本節不顯示原始碼、diff 或完整檔案展開控制。
名稱、呼叫關係與完整索引依最終快照 `823da26` 核對。

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
| `internal/console/devcontainer.go` · `prepareProjectDevcontainer` | 透過 `mergedProjectConfig` 共用合併結果，再分派所有已啟用 plugin 的驗證。後續流程見 G5。 |
| `.gitignore` | 忽略本 repository 的 local.d 設定。其他 Project 仍須自行設定忽略規則。 |

變更索引（34b3414 → 823da26；檔案與新版本 hunk 起始行）：

- `.gitignore:3`
- `internal/common/devcontainer_config.go:3,10,19,104`
- `internal/console/config_preview.go:18,43`

### G2 新增／修改：通用契約、管理與狀態

確認核心不再理解 GitHub 參數。每個 plugin 只取得自身設定與私有資料；失敗不回滾其他 plugin。

需你 review：是。

```html
<label><input type="checkbox" id="check-G2"> 我已確認 G2 的決策與風險</label>
```

入口：main 建立與註冊實作；Console 啟用時傳入 Project、plugin ID 與 RawMessage。

```flow TB
(main) -> NewManager: 呼叫
(main) -> githubapp.New: 呼叫
(main) -> Manager.Register: 呼叫
Manager.Register -> Plugin.Definition: 查驗 ID 與 metadata
(Console 確認啟用) -> Manager.Enable: 呼叫
Manager.Enable -> Plugin.ValidateConfig: 呼叫
Manager.Enable -> Plugin.HandleHost: EventEnable
Manager.Enable -> commitEnable: hook 成功後呼叫
commitEnable -> common.UpdateConfig: 呼叫
common.UpdateConfig --> ProjectPlugins 與 PluginState: 狀態：enabled 與 pending
group internal/plugins/manager.go: NewManager, Manager.Register, Manager.Enable, commitEnable
group internal/plugins/plugin.go: Plugin.Definition, Plugin.ValidateConfig, Plugin.HandleHost
group internal/plugins/githubapp/plugin.go: githubapp.New
group internal/common/config_store.go: common.UpdateConfig, ProjectPlugins 與 PluginState
```

| 檔案／函式或型別 | 責任與後續影響 |
| --- | --- |
| `internal/plugins/plugin.go` · `Plugin` | Definition、ValidateConfig、HandleHost、HandleContainer、ValidateContainerConfig、Status 構成實際 Go interface。 |
| 同檔 · `Definition`、`Field`、`Choice` | 定義穩定 ID、顯示訊息、文字與選項欄位；表單屬於實作 metadata。 |
| 同檔 · `HostContext`、`ContainerContext`、`ScopedStore`、`Result` | hook 只接收自身 config/state；Result 明確回報 NeedsBuild。ContainerContext 額外取得 Executor。 |
| `internal/plugins/manager.go` · `Manager.Register`、`validateAliases` | 拒絕空白／重複 ID、重複 alias，以及未實作 HostService 卻宣告 alias。 |
| 同檔 · `loadProject`、`validateIDs` | enabled 與殘留 state 的未知 ID 在副作用前報錯。 |
| 同檔 · `Manager.Prepare`、`Manager.HandleContainer`、`sortedEnabled` | 按 ID 排序，遇錯即停；前項已成功的獨立成果保留。 |
| 同檔 · `Manager.Enable`、`commitEnable`、`commitDisable`、`commitHookResult` | 合併 hook 結果與通用 pending；不比較設定 fingerprint。當前 plugin 自行回復未完成作業。 |
| 同檔 · `scopedStore.SaveConfig`、`scopedStore.SaveState` | 能提前儲存自身記錄供 Host 服務讀取；無法透過此能力覆寫其他 plugin。 |
| 同檔 · `Manager.WithProjectOperation`、`lockProject` | 同 Project 跨程序互斥；巢狀 manager 呼叫沿用 operation context。結束後 context 失效。 |
| `internal/common/config_store.go` · `ProjectPlugins`、`PluginState` | enabled 使用 RawMessage；通用 BuildPending 與 plugin 私有 Data 分開保存。 |
| 同檔 · `PluginState.UnmarshalJSON` | 舊 build_pending 保留；片段摘要、unit 摘要與路徑搬入 opaque Data，供 GitHub 自行解碼。 |
| 同檔 · `ReadConfig`、`UpdateConfig`、`lockConfigFile` | 短期共享鎖避免不同 Project 的 JSON 更新互相覆蓋。 |
| `internal/plugins/manager.go` · `Manager.Status`、`Manager.MarkBuilt` | Project pending 是各 plugin 聯集；成功重建後清除 pending，必要私有資料仍保留。 |

ScopedStore 是內建 Go plugin 的責任邊界，不是惡意程式 sandbox。
停用或移除也清理殘留 Data；清理失敗保留可重試記錄。
`PluginStatus` 使用通用 Message；ServiceActive 不再是核心必要欄位。

變更索引（34b3414 → 823da26；檔案與新版本 hunk 起始行）：

- `internal/commands/register.go:19`
- `internal/common/config_store.go:7,95,154`
- `internal/common/remove_project.go:2`
- `internal/plugins/manager.go:1`
- `internal/plugins/plugin.go:1`
- `main.go:6`

### G3 新增：GitHub App provider

確認 installation 全範圍與選定權限。
只有舊設定指定 repository 時，才先核對該 repo 的 installation。

需你 review：是。

```html
<label><input type="checkbox" id="check-G3"> 我已確認 G3 的決策與風險</label>
```

入口一：Console 表單由 Manager.Enable 呼叫 plugin 驗證。
錯誤先指出欄位；未通過驗證時不啟用服務。

```flow TB
(Console 確認表單) -> Manager.Enable: 呼叫
Manager.Enable -> Plugin.ValidateConfig: 呼叫
Plugin.ValidateConfig -> decodeConfig: 呼叫
decodeConfig -> Config.UnmarshalJSON: json.Unmarshal 呼叫
Config.UnmarshalJSON -> decodeID: 解析兩種 ID
Config.UnmarshalJSON -> safeConfigTypeError: 其他欄位型別錯誤時呼叫
Plugin.ValidateConfig -> Config.Validate: 解碼成功後呼叫
Config.Validate -> validateIDs: 檢查正數與缺少欄位
configFieldError --> decodeConfig: 資料：允許顯示的固定訊息
decodeConfig --> Console 錯誤訊息: 回傳安全錯誤，不回顯輸入
group internal/plugins/manager.go: Manager.Enable
group internal/plugins/githubapp/plugin.go: Plugin.ValidateConfig, decodeConfig
group internal/plugins/githubapp/config.go: Config.UnmarshalJSON, decodeID, safeConfigTypeError, Config.Validate, validateIDs, configFieldError
```

入口二：Socket 的 `/credential` handler 呼叫 `Provider.Token`。
圖中 mint 的子步驟均由 `Provider.mint` 呼叫。

```flow TB
(Socket credentialHandler) -> Provider.Token: 呼叫
Provider.Token --> 記憶體 Token: 資料：有效期超過五分鐘
Provider.Token -> awaitRefresh: 已有更新時呼叫
Provider.Token -> Provider.refreshToken: 取得更新權時呼叫
Provider.refreshToken -> Provider.mint: 呼叫
Provider.mint -> Provider.loadPrivateKey: 呼叫
Provider.mint -> Provider.createAppJWT: 呼叫
Provider.mint -> Provider.verifyRepositoryInstallation: 僅舊 repository 非空時呼叫
Provider.mint -> Provider.requestInstallationToken: 無 repository 或核對成功後呼叫
Provider.refreshToken --> Provider 與 refreshCall: 狀態：快取與等待者結果
group internal/plugins/githubapp/provider.go: Provider.Token, awaitRefresh, Provider.refreshToken, Provider.mint, Provider.loadPrivateKey, Provider.createAppJWT, Provider.verifyRepositoryInstallation, Provider.requestInstallationToken, Provider 與 refreshCall
```

| 檔案／函式或型別 | 責任與後續影響 |
| --- | --- |
| `internal/plugins/githubapp/plugin.go` · `Plugin.ValidateConfig`、`decodeConfig` | 呼叫 JSON 解碼與驗證。只保留已知 configFieldError；其他解碼錯誤回報固定通用訊息。 |
| `internal/plugins/githubapp/config.go` · `Config.UnmarshalJSON`、`decodeID`、`safeConfigTypeError` | 接受十進位文字與 numeric ID；ID 與允許欄位的型別錯誤轉為固定訊息，不回顯值。 |
| 同檔 · `configFieldError`（型別）、`configFieldError.Error` | 標記可安全顯示的固定錯誤。缺少、零與負數 ID 由 validateIDs 指出欄位。 |
| `internal/plugins/githubapp/config.go` · `Config`、`Config.normalized` | 空白 pass entry 衍生為 `dch/github-app/<AppID>/private-key`。明確設定的舊路徑與 repository 保留。 |
| 同檔 · `Config.Validate`、`validateIDs`、`validatePassEntry`、`validateOptionalRepository`、`validatePermissions` | 驗證正整數 ID、pass 路徑與可選 owner/repository。權限只接受 contents／pull_requests 的 read/write，且不可全空。 |
| `internal/plugins/githubapp/provider.go` · `Provider`、`ProviderOptions`、`NewProvider` | 先正規化 pass entry，再保存設定、逾時、HTTP client 與時鐘；停用 HTTP redirect。 |
| 同檔 · `Provider.Token`、`refreshCall`、`awaitRefresh`、`Provider.refreshToken` | mutex 同時保護快取判定與更新權；並行請求共用結果。更新失敗可由下次請求重試。 |
| 同檔 · `Provider.loadPrivateKey`、`securePassEnvironment`、`parsePrivateKey` | 非互動執行 pass；限制輸出大小；解析 Host RSA 私鑰。 |
| 同檔 · `Provider.createAppJWT`、`Provider.verifyRepositoryInstallation` | 簽發 App JWT。僅舊 repository 非空時查詢該 repo，且 installation ID 必須符合設定。 |
| 同檔 · `Provider.requestInstallationToken` | 始終送出選定權限。未設定 repository 時省略 repositories；舊設定只送出該 repo。驗證 Token 格式及有效期，結果只留 Host 記憶體。 |

變更索引（34b3414 → 823da26；檔案與新版本 hunk 起始行）：

- `internal/plugins/githubapp/config.go:1`
- `internal/plugins/githubapp/provider.go:1`

### G4 新增／修改：GitHub Host 服務、Socket 與相容入口

確認 systemd 僅由 GitHub 實作管理。舊 unit alias 仍可用；能連 Socket 的程序仍可取得短期 Token。

需你 review：是。

```html
<label><input type="checkbox" id="check-G4"> 我已確認 G4 的決策與風險</label>
```

入口：GitHub Host hook 產生 systemd user unit。
unit 啟動同一份 Host dch 的通用 `_plugin-serve` 入口。

```flow TB
(systemd user unit) -> runPluginService: Host CLI dispatch
runPluginService -> parsePluginServeArgs: 呼叫
parsePluginServeArgs -> Manager.ServiceAlias: 舊入口查 metadata
runPluginService -> Manager.Serve: 呼叫
Manager.Serve -> Plugin.ServeHost: HostService 分派
Plugin.ServeHost -> RuntimeDir: 呼叫
Plugin.ServeHost -> Serve: 呼叫
Serve -> prepareCredentialSocket: 呼叫
Serve -> credentialMux: 註冊 HTTP handlers
Serve -> serveUntilCancelled: 呼叫
group internal/commands/commands.go: runPluginService, parsePluginServeArgs
group internal/plugins/manager.go: Manager.ServiceAlias, Manager.Serve
group internal/plugins/githubapp/manager.go: Plugin.ServeHost
group internal/plugins/githubapp/runtime.go: RuntimeDir
group internal/plugins/githubapp/server.go: Serve, prepareCredentialSocket, credentialMux, serveUntilCancelled
```

| 檔案／函式或型別 | 責任與後續影響 |
| --- | --- |
| `internal/plugins/githubapp/plugin.go` · `Plugin.Definition` | 宣告 `_github-app-serve` alias。通用 CLI 不保存此 GitHub 常數。 |
| `internal/commands/commands.go` · `parsePluginServeArgs`、`runPluginService` | 支援 `_plugin-serve --plugin ID --project PATH`；alias 由 registry 查找。接收訊號並取消服務。 |
| `internal/plugins/manager.go` · `Manager.Serve` | 確認註冊、可選服務能力及已啟用記錄；讀設定後不持有 Project 操作鎖。 |
| `internal/plugins/githubapp/manager.go` · `Plugin.enable`、`prepareEnableResources` | 準備片段、adapter、unit；先經 ScopedStore 保存自身設定，再啟動服務。 |
| 同檔 · `ownershipState`、`decodeState`、`encodeState` | 自行解碼片段摘要、unit 摘要與路徑。核心不解析這些 GitHub 私有欄位。 |
| 同檔 · `serviceUnitContent`、`prepareServiceUnit`、`checkReady` | 新 unit 使用通用入口；保留原 unit 路徑與摘要保護。啟動後實際請求 credential。 |
| 同檔 · `Plugin.rollbackEnable`、`Plugin.rollbackEnsure`、`restoreScoped` | 回復自身檔案與 scoped 設定；失敗保留重試資料，不宣告清理完成。 |
| 同檔 · `Plugin.disable`、`removePluginFiles` | 先停服務，再依摘要清理 unit、片段與 runtime。成功才讓 manager 移除 enabled。 |
| `internal/plugins/githubapp/runtime.go` · `RuntimeDir`、`ensurePrivateDir` | 每 Project 專屬私有目錄 0700；拒絕 symlink 與錯誤所有權。 |
| `internal/plugins/githubapp/server.go` · `prepareCredentialSocket`、`listenCredentialSocket`、`acquireServerLock` | 排除重複 server、處理舊 Socket、建立 0600 Unix Socket；不開 TCP。 |
| 同檔 · `healthHandler`、`credentialHandler`、`CheckCredential` | health 不取 Token；credential 呼叫 Provider.Token 並帶 no-store；啟用驗證不輸出 Token。 |

變更索引（34b3414 → 823da26；檔案與新版本 hunk 起始行）：

- `internal/commands/commands.go:1,134`
- `internal/plugins/githubapp/manager.go:1`
- `internal/plugins/githubapp/plugin.go:1`
- `internal/plugins/githubapp/runtime.go:1`
- `internal/plugins/githubapp/server.go:1`

### G5 新增／修改：Container hook、合併驗證與 executor

確認 Container hook 的 Go 方法仍在 Host。它只能透過注入 executor 執行容器指令；GitHub 限制由實作驗證。

需你 review：是。

```html
<label><input type="checkbox" id="check-G5"> 我已確認 G5 的決策與風險</label>
```

入口：Start／Build 的 up 成功後，或 Connect 開啟 shell 前。
Manager 分派 Container hook；GitHub hook 請 executor 執行安裝腳本。

```flow TB
(Host Console 生命週期) -> Manager.HandleContainer: 呼叫
Manager.HandleContainer -> Plugin.HandleContainer: Host Go 方法
Plugin.HandleContainer -> projectExecutor.Exec: 注入 Executor 呼叫
projectExecutor.Exec -> prepareProjectDevcontainer: Host 呼叫
prepareProjectDevcontainer -> mergedProjectConfig: 呼叫
prepareProjectDevcontainer -> validateMergedProjectConfig: 呼叫
validateMergedProjectConfig -> Manager.ValidateMergedConfig: 呼叫
Manager.ValidateMergedConfig -> Plugin.ValidateContainerConfig: 逐 plugin 呼叫
projectExecutor.Exec -> devcontainer exec: Host 啟動外部程序
devcontainer exec --> 容器 shell 安裝 Git helper 與 gh: 目標是 Container
group internal/plugins/manager.go: Manager.HandleContainer, Manager.ValidateMergedConfig
group internal/plugins/githubapp/plugin.go: Plugin.HandleContainer, Plugin.ValidateContainerConfig
group internal/console/devcontainer.go: projectExecutor.Exec, prepareProjectDevcontainer, mergedProjectConfig, validateMergedProjectConfig
```

| 檔案／函式或型別 | 責任與後續影響 |
| --- | --- |
| `internal/plugins/plugin.go` · `Executor.Exec`、`ContainerContext` | 接收 context 與參數陣列，回傳 stdout／錯誤；契約不要求容器安裝 dch。 |
| `internal/console/devcontainer.go` · `projectExecutor.Exec` | Host 以 devcontainer exec 執行原始參數；沿用合併設定、remoteUser、remoteEnv。stdout 與診斷分開擷取並限制大小。 |
| 同檔 · `prepareProjectDevcontainer`、`mergedProjectConfig`、`validateMergedProjectConfig` | 共用合併、逐 plugin 驗證、路徑重定位、暫存 config 與 id-label；呼叫端負責清理。 |
| `internal/plugins/githubapp/plugin.go` · `Plugin.ValidateContainerConfig` | 自行拒絕 Compose，取得 runtime 路徑後驗證自身必要設定。 |
| `internal/plugins/githubapp/container_config.go` · `validateContainerConfig`、`validateAdapterMounts`、`hasAdapterPath` | 確認精確 mount、PATH，拒絕缺失或重疊掛載。這些限制不在通用核心。 |
| 同檔 · `requireLocalDocker` | GitHub Host 的 `enable`／`ensure` 確認本機 Unix Docker endpoint；其他 plugin 不必採用此限制。 |
| `internal/plugins/githubapp/plugin.go` · `Plugin.HandleContainer` | Start／Build／Connect 呼叫注入 Executor，參數由 AdapterInstallCommand 提供。 |
| `internal/plugins/githubapp/adapter.go` · `AdapterFragment`、`WriteAdapterScripts`、`AdapterInstallCommand` | 產生唯讀 runtime mount、PATH、私有腳本與可重複安裝的 shell 命令。 |
| 同檔 · `gitCredentialScript`、`adapterGitConfig`、`ghWrapperScript` | 容器內 shell 產物。Git 只回應 HTTPS github.com；gh 每次取 Token，只設定子程序 GH_TOKEN。 |

容器只需要 Git、gh、curl 與 POSIX shell，不複製 dch binary。
接線保留既有 lifecycle hooks；腳本從固定 Socket 取得短期 Token。

變更索引（34b3414 → 823da26；檔案與新版本 hunk 起始行）：

- `internal/console/devcontainer.go:1,11,59,95`
- `internal/plugins/githubapp/adapter.go:1`
- `internal/plugins/githubapp/container_config.go:1`

### G6 新增／修改：動態 Console、生命週期與移除

確認 Project 操作跨程序互斥。既有容器待 Build 時阻擋 Start／Connect；成功 up 與必要 hook 完成才清除 pending。

需你 review：是。

```html
<label><input type="checkbox" id="check-G6"> 我已確認 G6 的決策與風險</label>
```

入口：Project → Plugins。Console 只理解 Definition 與通用狀態。
GitHub metadata 移除 pass entry 與 Owner/repository 提問。
Notices 顯示預設 pass 路徑與 installation 全範圍。
Notices 指示填入 installation settings URL 的數字 ID。
App ID 與 Client ID 不能取代 Installation ID。

既有 JSON 未建模欄位保留；隱藏欄位不會清除舊限制。

```flow TB
(Project 選 Plugins) -> pluginsMenu: 選單分派
pluginsMenu -> Manager.Definitions: 取得欄位描述
pluginsMenu -> Manager.Statuses: 取得逐 plugin 狀態
pluginsMenu -> configurePlugin: 呼叫
configurePlugin -> enableProjectPlugin: 選設定與啟用
enableProjectPlugin -> promptPluginConfig: 通用 renderer
enableProjectPlugin -> confirmPluginEnable: 確認授權範圍
confirmPluginEnable -> pluginConfirmationSummary: 依 Field.Confirm 產生摘要
enableProjectPlugin -> Manager.Enable: 確認後呼叫
configurePlugin -> disableProjectPlugin: 選停用
disableProjectPlugin -> Manager.Disable: 呼叫
group internal/console/plugins.go: pluginsMenu, configurePlugin, enableProjectPlugin, promptPluginConfig, confirmPluginEnable, pluginConfirmationSummary, disableProjectPlugin
group internal/plugins/manager.go: Manager.Definitions, Manager.Statuses, Manager.Enable, Manager.Disable
```

Start／Build 在同 Project 操作鎖內完成 Host hook、容器 up 與 Container hook。
任何錯誤都停止後續步驟，保持必要 pending。

```flow TB
(Start 或 Build) -> Manager.WithProjectOperation: 入口取得 Project 鎖
Manager.WithProjectOperation --> startProjectOperation 或 buildProjectOperation: callback 分派
startProjectOperation 或 buildProjectOperation -> Manager.Prepare: Host hook
startProjectOperation 或 buildProjectOperation -> invokeProjectDevcontainer: up
startProjectOperation 或 buildProjectOperation -> Manager.HandleContainer: up 成功後
startProjectOperation 或 buildProjectOperation -> Manager.MarkBuilt: 符合解除條件才呼叫
group internal/console/up.go build.go: startProjectOperation 或 buildProjectOperation
group internal/plugins/manager.go: Manager.WithProjectOperation, Manager.Prepare, Manager.HandleContainer, Manager.MarkBuilt
group internal/console/devcontainer.go: invokeProjectDevcontainer
```

| 檔案／函式或型別 | 責任與後續影響 |
| --- | --- |
| `internal/console/plugins.go` · `pluginsMenu`、`pluginOptions`、`statusFor` | 按 registry 顯示各 plugin 的啟用、待 Build 與安全訊息。 |
| 同檔 · `promptPluginConfig`、`promptPluginField`、`getPluginField`、`setPluginField` | 渲染文字與選項欄位，處理巢狀 JSON Path；保留既有未建模值。 |
| 同檔 · `confirmPluginEnable`、`pluginConfirmationSummary`、`pluginChoiceLabel` | 只顯示 Field.Confirm 指定的非秘密欄位；依通用 metadata 顯示權限授權摘要；installation 範圍由 Notices 說明。 |
| `internal/plugins/githubapp/plugin.go` · `Plugin.Definition`、`Plugin.ValidateConfig` | GitHub 欄位、權限與驗證全部由具體實作提供。 |
| `internal/console/up.go` · `startProject`、`startProjectOperation` | 操作前後查 pending；只有新建容器的 Start 在 up 與所有 hook 成功後 MarkBuilt。 |
| `internal/console/build.go` · `confirmRebuild`、`buildProject`、`buildProjectOperation` | 保留資料移除確認與快取選項；重建與所有 hook 成功後 MarkBuilt。 |
| `internal/console/connect.go` · `connectProject`、`prepareProjectConnect` | 鎖內完成 hook、最終設定與 pending 守門後啟動 CLI；解鎖後等待互動 shell。 |
| `internal/console/projects_menu.go` · `availableProjectActions`、`projectOptions` | 依通用 pending 顯示 Build required；既有容器隱藏 Start／Connect，函式入口仍再次守門。 |
| `internal/plugins/manager.go` · `Manager.RemoveProject` | 先驗證全部 ID，按 ID 清理 enabled 或殘留 Data；成功後才取消註冊。 |

```flow TB
(Project 選 Remove Project) -> confirmProjectRemoval: 先沿用確認
confirmProjectRemoval -> removeProject: 確認後呼叫
removeProject -> Manager.RemoveProject: 取得 Project 鎖並清理
Manager.RemoveProject -> Plugin.HandleHost: EventDisable，按 ID 排序
Manager.RemoveProject -> common.RemoveProjectRegistration: 全部清理成功後
group internal/console/projects_menu.go: confirmProjectRemoval, removeProject
group internal/plugins/manager.go: Manager.RemoveProject
group internal/plugins/plugin.go: Plugin.HandleHost
group internal/common/remove_project.go: common.RemoveProjectRegistration
```

Connect 在同一把操作鎖內完成最後守門與 CLI 啟動。
CLI 成功 Start 後解鎖，再於鎖外等待互動 shell 結束。
因此 Disable 不會被整段互動 session 阻擋。

```flow TB
connectProject -> Manager.WithProjectOperation: 取得 Project 鎖
Manager.WithProjectOperation --> 鎖內匿名 callback: 執行 callback
鎖內匿名 callback -> prepareProjectConnect: hooks 與驗證
鎖內匿名 callback -> prepareProjectDevcontainer: 準備最終設定與參數
鎖內匿名 callback -> rejectPendingConnect: 啟動前最後守門
鎖內匿名 callback -> startDevcontainerWithCleanup: 呼叫
startDevcontainerWithCleanup -> cmd.Start: 啟動 Host devcontainer 程序
Manager.WithProjectOperation --> waitForShell: callback 返回並解鎖後，由 connectProject 呼叫
waitForShell -> cmd.Wait: 鎖外等待與清理
 group internal/console/connect.go: connectProject, 鎖內匿名 callback, prepareProjectConnect, rejectPendingConnect, waitForShell
 group internal/console/devcontainer.go: prepareProjectDevcontainer, startDevcontainerWithCleanup
 group internal/plugins/manager.go: Manager.WithProjectOperation
```

`waitForShell` 是函式區域變數，內容由 startDevcontainerWithCleanup 回傳。
此 closure 執行 cmd.Wait、暫存設定清理與命令錯誤回報。
Shell 程序不綁定結束後即取消的 operation context。

取消註冊保留工作區與容器；清理失敗保留註冊與重試資料。

變更索引（34b3414 → 823da26；檔案與新版本 hunk 起始行）：

- `internal/console/build.go:1,16`
- `internal/console/connect.go:1`
- `internal/console/console.go:1`
- `internal/console/plugins.go:1`
- `internal/console/projects_menu.go:1,17,203,219`
- `internal/console/up.go:1`

### G7 新增／修改：通用行為與 GitHub 回歸測試

測試驗證契約行為與外部可觀察結果，不建立正式空殼 plugin。

需你 review：否。

`internal/plugins/manager_test.go` 與 `manager_lifecycle_test.go` 使用兩個不同 fake plugin。
它們使用不同欄位與私有狀態，驗證排序、隔離、hook、pending 與失敗重試。

`internal/console/plugin_executor_test.go` 驗證容器參數、stdout、錯誤與合併設定。
生命週期測試驗證操作鎖、二次 pending 守門與成功後清除條件。
GitHub 舊測試已移入 `internal/plugins/githubapp/`，保留認證與 Socket 回歸範圍。
最終指令、複雜度與 reviewer 結論見 F。

變更索引（34b3414 → 823da26；檔案與新版本 hunk 起始行）：

- `internal/commands/plugin_serve_test.go:1`
- `internal/commands/register_test.go:28,44`
- `internal/commands/run_test.go:7,17`
- `internal/common/common_test.go:65`
- `internal/common/config_store_test.go:1`
- `internal/common/remove_project_test.go:1`
- `internal/console/build_plugin_test.go:1`
- `internal/console/config_preview_test.go:1`
- `internal/console/connect_lock_test.go:1`
- `internal/console/devcontainer_plugin_test.go:1`
- `internal/console/menu_test.go:30`
- `internal/console/plugin_executor_test.go:1`
- `internal/console/plugin_integration_test.go:1`
- `internal/console/plugin_lifecycle_test.go:1`
- `internal/console/plugin_test_helpers_test.go:1`
- `internal/console/plugins_test.go:1`
- `internal/console/projects_menu_test.go:10`
- `internal/plugins/githubapp/adapter_test.go:1`
- `internal/plugins/githubapp/config_test.go:1`
- `internal/plugins/githubapp/plugin_contract_test.go:1`
- `internal/plugins/githubapp/provider_test.go:1`
- `internal/plugins/githubapp/server_test.go:1`
- `internal/plugins/manager_lifecycle_test.go:1`
- `internal/plugins/manager_test.go:1`

### G8 新增／修改：架構規格、README 與 Git 屬性

文件描述已核准架構與實際操作，不替文件虛構函式。

需你 review：否。

README 說明 Console 路徑、GitHub 前置條件、local.d 遷移與人工 Linux 驗收。
規格 Markdown／HTML 包含獨立目錄樹、module／file 職責與 Host／Container 邊界。
`.gitattributes` 保留審查產物中的字面空白。

變更索引（34b3414 → 823da26；檔案與新版本 hunk 起始行）：

- `.gitattributes:1`
- `README.md:5,21,57,68`
- `specs/2026-10-24-project-plugins-spec.html:1`
- `specs/2026-10-24-project-plugins-spec.md:1`

### 架構修訂移除舊路徑

下表另以 `e1b666a` → `9cc1f5f` 比較架構修訂。
行號屬被刪除檔案的原版 hunk 起始行。
這些舊路徑在原始功能基準 `34b3414` 尚不存在。
因此它們不會出現在整體功能 diff 的刪除清單。

| 已移除舊路徑／原版行號 | 最終實作位置或替代責任 |
| --- | --- |
| `internal/console/plugin_integration.go:1` | `internal/plugins/githubapp/container_config.go` |
| `internal/githubapp/adapter.go:1` | `internal/plugins/githubapp/adapter.go` |
| `internal/githubapp/adapter_test.go:1` | `internal/plugins/githubapp/adapter_test.go` |
| `internal/githubapp/config.go:1` | `internal/plugins/githubapp/config.go` |
| `internal/githubapp/config_test.go:1` | `internal/plugins/githubapp/config_test.go` |
| `internal/githubapp/provider.go:1` | `internal/plugins/githubapp/provider.go` |
| `internal/githubapp/provider_test.go:1` | `internal/plugins/githubapp/provider_test.go` |
| `internal/githubapp/runtime.go:1` | `internal/plugins/githubapp/runtime.go` |
| `internal/githubapp/server.go:1` | `internal/plugins/githubapp/server.go` |
| `internal/githubapp/server_test.go:1` | `internal/plugins/githubapp/server_test.go` |
| `internal/plugins/manager_enable.go:1` | `internal/plugins/githubapp/manager.go` |
| `internal/plugins/registry.go:1` | `internal/plugins/plugin.go 與 manager.go` |

## D 使用行為與相容性

| 面向 | 原有／先前行為 | 最終行為 |
| --- | --- | --- |
| plugin 架構 | manager 與 Console 綁定 GitHub | 通用 Plugin interface；main 註冊與注入 |
| 設定表單 | Console 定義 GitHub 欄位 | Plugin.Definition 提供欄位；Console 通用渲染 |
| GitHub 新設定 | 詢問 pass entry 與 owner/repository | 只詢問兩個 ID 與權限；pass 路徑由 App ID 衍生 |
| GitHub Token 範圍 | 必須指定單一 repo | 未指定時使用 installation 全範圍；仍受選定權限限制 |
| GitHub 舊設定 | 明確 pass entry 與 repository | 兩欄保留且生效；不需遷移 |
| Host JSON | GitHub 型別與專屬摘要 | 通用 RawMessage、BuildPending 與私有 Data |
| 舊 metadata | 摘要及 unit 路徑直接放 state | 相容解碼保留至 Data；不建立服務、不遺失所有權 |
| 內部 Host 入口 | `_github-app-serve` | 通用 `_plugin-serve`；舊入口由 metadata alias 相容 |
| 個人容器設定 | 讀取 devcontainer.local.json | 只讀 local.d 第一層正常 .json；排序、JSONC、深合併與 array 去重 |
| 待 Build | GitHub 專屬旗標 | 各 plugin NeedsBuild 的聯集；不比較 fingerprint |
| Start／Build | GitHub 專屬準備與安裝 | 操作鎖內依序執行 Host hook、合併驗證、up、Container hook |
| Connect | 直接開啟 shell | hook 與 pending 守門成功後才啟動 shell；不清除 pending |
| 失敗 | 單一 plugin 回滾 | 逐 ID fail-fast；已成功 plugin 保留成果；當前 plugin 自行回復 |
| 移除 Project | 取消註冊 | 先清理所有 enabled／殘留 plugin；失敗保留重試記錄 |
| Git／gh | 使用者自備認證 | GitHub helper 與 gh wrapper 從 Host Socket 按需取得 Token |

```callout warn Breaking change：舊 local.json 使用者
舊檔會被靜默忽略，不提示也不自動搬移。
若要保留設定，請手動搬到 local.d 的 .json 片段。
例如：`.devcontainer/devcontainer.local.d/90-user.json`。
請自行將個人片段加入版本控制忽略規則。
```

T20 無既有設定破壞性變更。
新設定使用 installation 全範圍，必須先確認 installation 可存取的 repository。
Build 與容器資料移除仍需原有確認，不自動重建。
服務在關閉 dch 或 Stop 容器後保留。
停用立即停止後續憑證供應，但不撤銷已回傳的短期 Token。

## E 認證與信任邊界

```sequence num
Git 或 gh -> Socket: GET /credential
Socket -> Provider: 請求 Token
Provider -> Provider: 檢查記憶體快取
Provider -> pass: 需要更新才讀取私鑰
Provider -> GitHub: 舊 repository 非空才驗證 repo installation
GitHub --> Provider: 舊設定的 installation ID 必須符合
Provider -> GitHub: POST installation Token 與選定權限
note Provider, GitHub: 無 repository 時省略 repositories；舊設定只送出單一 repo
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
| pass | 未指定 entry 時使用 `dch/github-app/<AppID>/private-key`；舊自訂 entry 保留 |
| Token 範圍 | 無 repository 時使用 installation 可用 repository；舊 repository 限制保留 |
| 不支援 | Compose、遠端 Docker、GitHub Enterprise、SSH 認證、自訂多 repo 清單 |

容器 UID 必須能連線 Socket；錯誤訊息提示檢查 UID。
此環境未驗證真實容器 UID 與重啟行為。

## F 測試與獨立 Review

T21 最終驗證快照：`823da2667f6a542a78b1072dfcde9311ddd94bb5`。
修訂基準為 `73050d4`。
使用者確認將 Client ID 填入 Installation ID，造成原本通用錯誤。
本輪新增欄位診斷；未新增 installation 自動查找。

| T21 指令或範圍 | 結果 | 證據與限制 |
| --- | --- | --- |
| focused config／Console 測試 | PASS | 真實 prompt → Plugin.ValidateConfig；合法文字／numeric ID、Client ID 誤填、型別錯誤與清理訊息 |
| go test ./... -count=1 | PASS | 主代理於 823da26 執行；全部 6 個 package 通過 |
| go build ./...／go vet ./... | PASS | 主代理於 823da26 執行 |
| git diff --check | PASS | 主代理檢查最終差異 |
| Cognitive Complexity | PASS | gocognit v1.3.0；AST 辨識 445 個新增／修改具名函式，最高 15，0 違規 |
| 完整 diff 與索引 | PASS | 34b3414 → 823da26；逐檔核對新版本 hunk 起始行 |
| 更新後 HTML 產生與內容 | PASS | am render --no-open；7 panels、11 flow、1 tree、1 sequence；6 個未勾選框；C 無原始碼或 diff |
| 更新後 HTML 瀏覽器與互動 | PASS | Chromium 155；1440×900 與 390×844；11 個 flow、6 個初始未勾選且可切換框；無水平溢出或 CSP 錯誤 |
| Standards＋Spec review | PASS | 73050d4 → 823da26；0 項未解決 finding；enable-config-code.md |
| Security review | PASS | 73050d4 → 823da26；0 項安全 finding；enable-config-security.md；靜態 review |

T21 已保存行為 red → green。
原本 ID 與型別錯誤只回傳通用訊息，focused 測試先失敗。
修正後 focused 測試通過，malformed JSON 仍使用清理後通用訊息。
缺少、零與負數 ID 的六個欄位案例亦先失敗再通過。
證據：`.scratch/project-plugins/enable-config-evidence.md`。
測試未呼叫會操作 Docker／systemd 的正式 Manager.Enable。

以下為 T20 與更早歷史證據，不代表更新後 T21 已通過覆核。

T20 驗證快照：`5451f858193b38de930cec75c6375b69d3cda45b`。
T20 修訂基準為 `29e8e6b`。

| T20 指令或範圍 | 結果 | 證據與限制 |
| --- | --- | --- |
| go test ./... -count=1 | PASS | 主代理於 5451f85 執行；全部 6 個 package 通過 |
| go build ./... | PASS | 主代理於 5451f85 執行；通過 |
| go vet ./... | PASS | 主代理於 5451f85 執行；通過 |
| Cognitive Complexity | PASS | gocognit v1.3.0；AST 辨識 439 個新增／修改具名函式，最高 15，0 違規 |
| git diff --check | PASS | 主代理檢查 34b3414 → 5451f85 完整差異 |
| 最終完整 diff 與索引 | PASS | 34b3414 → 5451f85；主代理已更新完整 diff，逐檔核對新版本 hunk 起始行 |
| 更新後 HTML 產生與內容 | PASS | am render --no-open；7 panels、10 flow、1 tree、1 sequence；6 個未勾選框；C 無原始碼、diff 或展開控制 |
| 更新後 HTML 瀏覽器與互動 | PASS | Chromium 155；1440×900 與 390×844；10 個 flow、6 個可切換勾選框；無頁面溢出或 CSP 錯誤 |
| 真實 Linux 驗收 | NOT RUN | 未驗收 Docker／systemd／pass／GPG／GitHub 整合 |
| UID／重啟／關閉 dch 後認證 | NOT RUN | 需在真實 Linux 容器執行 README 驗收 |
| go test -race ./... | NOT RUN | CGO 關閉，且無 C compiler |

| T20 獨立 reviewer | 範圍 | 結論 |
| --- | --- | --- |
| Standards＋Spec | 29e8e6b → 5451f85；簡化表單、預設 pass 與 installation 範圍 | 最終重查 0 項未解決 finding；複雜度超標已修正 |
| Security | 29e8e6b → 5451f85；表單、舊設定、Token 請求與 pass 邊界 | 最終重查 0 項安全 finding；靜態 review，未連線真實 GitHub |

匿名函式納入所屬函式計分；未抑制 findings。
八個既有超標函式未修改，仍屬本次範圍外。
此結果不表示整個 repository 已無既存違規。

T20 報告：`simplified-code.md`、`simplified-security.md`。
報告位於 `.scratch/project-plugins/reviews/`。

| 歷史證據 | 快照與結果 | 限制 |
| --- | --- | --- |
| 測試／編譯／vet | 9cc1f5f；PASS，全部 6 個 package 通過 | 尚未簡化 GitHub 表單 |
| Cognitive Complexity | 9cc1f5f；424 個新增／修改具名函式，最高 15，0 違規 | gocognit v1.3.0；匿名函式納入所屬函式計分 |
| Standards／Spec／Security | 9cc1f5f；0 項未解決 finding | generic-standards.md、generic-spec.md、generic-security.md |
| 舊 HTML 版面與互動 | Chromium 155；1440×900、390×844 無水平溢出；六框可切換 | 僅先前頁面；不涵蓋本輪新內容 |
| 通用表單擷取 | 9cc1f5f；真實測試輸出見 G | Sample metadata，未連線 GitHub 或 Docker |

已保存 red → green 證據：executor stdout、overflow 與 Connect 最後守門。
T13／T19 未完整保存最初行為 red 紀錄，部分只有編譯失敗。
因此不宣稱所有項目均有完整 TDD 歷程證據。

T20 最初 red 只有缺少 Config.normalized 的編譯失敗。
後續 focused package 為 green；不宣稱已保存完整行為 red。
證據：`.scratch/project-plugins/simplified-setup-evidence.md`。

Source review 不取代真實環境驗收。
Root 執行測試時，DAC 拒絕案例會跳過。
假指令與 Socket 測試不代表真實 UID 或 systemd 驗收已完成。

## G 真實擷取與核准提示

以下是通用表單測試的實際輸出，使用測試用 Sample metadata。
來源：`TestPromptPluginConfigUsesGenericFieldsAndNestedPaths`。
它驗證通用欄位與巢狀 JSON Path，不是現場 GitHub／Docker 畫面。

```text title="9cc1f5f 實際測試輸出"
=== RUN   TestPromptPluginConfigUsesGenericFieldsAndNestedPaths
Display name (q to cancel): 
Access

  1) Read
  2) Write
Choose number (q to cancel): --- PASS: TestPromptPluginConfigUsesGenericFieldsAndNestedPaths (0.00s)
PASS
ok  	dch/internal/console	0.003s
```

請先確認 C 的 6 個決策，再在對話中給予最終核准。
本頁顯示函式流程；完整實作 diff 另附參考連結。
未讀取或嵌入使用者本機憑證與 local 設定。
本次交付只更新 review 文件，不執行 PR 或 push。
