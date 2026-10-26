---
title: GitHub 認證與 Host 密碼庫設定規格
subtitle: BYO GitHub App、Fine-grained PAT、GPG/pass 初始化與背景 pinentry
lang: zh-Hant
template: doc
theme: paper
---

本規格待使用者核准。核准後才拆分本機 tickets 與實作。

## A 問題與方案

目前 GitHub App plugin 已有 Device Flow、pass 儲存與 host credential service。
使用者必須先自行準備 GPG/pass，且目前背景解密禁止 pinentry。
目前沒有 Fine-grained personal access token（PAT）的設定入口。

本次提供兩種認證方式。BYO GitHub App 為預設與推薦選項。
兩種方式共用 host GPG/pass 前置設定，完成驗證後才取得或輸入 token。
PAT 每個專案獨立儲存，沿用既有 host credential service 與容器 adapter。
背景服務允許觸發圖形 pinentry；純 SSH/headless 使用 host 終端解鎖後重試。

| 項目 | 決定 |
| --- | --- |
| 開發模式 | hard：規格核准、tickets、子代理 TDD、Code/Security Review、最終核准、PR |
| 起點 | 7aa1998c882c665554bcedb30e5763d53a28846f |
| 工作分支 | feat/github-auth-pass-setup |
| 主機範圍 | 沿用既有 Linux、local Docker、systemd user service 條件 |
| 自動安裝 | apt、dnf、pacman；每次先顯示命令並取得產品使用者同意 |
| 套件選擇 | 保留現有 pass 封裝；本次不引入 byteness/keyring |
| 原有改動 | 不納入既存 update.go、先前 spec/review 與 .scratch 工作 |

## B 使用情境與操作順序

1. 使用者在 Project plugins 選 Configure and enable 或 Reauthorize。
2. 使用者選 BYO GitHub App（Recommended）或 Fine-grained token。
3. dch 檢查 host 的 GPG 與 pass；缺少時提出安裝建議與確認。
4. pass 已初始化時，顯示並沿用適用於 dch 憑證路徑的收件金鑰。
5. pass 未初始化時，列出有效、可加密且可解密的 secret key 供選擇。
6. 沒有可用 secret key 時，經確認開啟互動式 GPG 金鑰建立流程。
7. 使用者由 GPG 設定身分與 passphrase；dch 不接收或儲存該 passphrase。
8. 建立後重新列出可用 key，再讓使用者選擇。
9. 使用選定 key 的完整 fingerprint 執行 pass init，再檢查初始化結果。
10. 寫入隨機測試 secret、讀回比對、刪除並確認刪除成功。
11. 全部前置檢查成功後，才進行 App Device Flow 或隱藏輸入 PAT。
12. 使用者確認 GitHub 帳號與目標 repository，再提交候選設定。
13. 憑證儲存、服務啟動與驗證成功後，才完成專案啟用。
14. 切換認證方式失敗時，維持原有設定、服務與憑證的可恢復狀態。
15. PAT 過期、撤銷或權限不足時，提示檢查或重新設定 PAT。
16. 使用者可在圖形 pinentry 輸入 passphrase；取消或逾時則取得明確失敗訊息。

```flow
設定認證方式 -> 檢查 GPG 與 pass
檢查 GPG 與 pass -> 檢查密碼庫與金鑰
檢查密碼庫與金鑰 -> 測試寫入讀取刪除
測試寫入讀取刪除 -> {認證方式}
{認證方式} -> App Device Flow
{認證方式} -> 隱藏輸入 PAT
App Device Flow -> 驗證帳號與 repository
隱藏輸入 PAT -> 驗證帳號與 repository
驗證帳號與 repository -> 儲存候選憑證並啟用服務
```

## C GPG 與 pass 前置設定

### 套件安裝

先檢查可執行檔與 host 發行版，選擇適用的套件管理器。
只安裝缺少的套件，不以 shell 拼接使用者輸入。

| 平台 | GPG 套件 | pass 套件 | 安裝命令範本 |
| --- | --- | --- | --- |
| Debian / Ubuntu | gnupg | pass | apt-get install 套件名稱 |
| Fedora / RHEL 系列 | gnupg2 | pass | dnf install 套件名稱 |
| Arch Linux | gnupg | pass | pacman -S --needed 套件名稱 |

顯示實際缺少項目、將執行的命令與需管理員權限的原因，再詢問同意。
非 root 安裝使用 sudo，認證由 sudo 自行處理。
只有套件安裝可提升權限；GPG、pass 與服務設定以目前使用者身分執行。
保留套件管理器本身的確認，不強制無互動安裝。

不支援的系統、缺少 sudo、拒絕安裝、取消或安裝失敗時，提供手動處理指引並停止啟用。
安裝後重新檢查可執行檔；不在本次流程自動升級套件或修改套件來源。

### 金鑰與初始化

尊重目前的 GNUPGHOME 與 PASSWORD_STORE_DIR，未指定時使用工具預設值。
解析 GPG 機器可讀的 secret key 清單，顯示 UID 與完整 fingerprint。
排除過期、撤銷、停用及沒有可用加密能力的 key。
選擇的 key 必須包含可用的加密 secret key／subkey；實際可用性再以讀寫驗證確認。

沒有合適 key 時，說明即將建立金鑰，確認後執行 GPG 互動式建立。
建立失敗或取消時停止，不自動建立無 passphrase 的 key。

已初始化的 pass 沿用既有設定；不重跑根目錄 pass init，也不重新加密其他秘密。
應考慮 dch 儲存路徑上既有的子目錄 .gpg-id 覆寫規則。
若適用的收件設定損壞、缺少可用私鑰或無法解密，提供修復指引並中止。
未初始化時才使用使用者選定的 key 初始化密碼庫。
若在操作期間偵測到其他程序已初始化，重新讀取設定，不覆寫該結果。

### 讀寫驗證與清理

測試 secret 使用密碼學安全亂數，entry 位於相同的 dch 憑證加密範圍。
entry 名稱隨機且僅屬於本次測試，不覆寫既有憑證。
以正式服務相同的 pass／GPG 選項與環境規則寫入並讀回完整比對。
測試值不得顯示於畫面、log 或錯誤訊息。

成功、比對失敗、取消與逾時都必須嘗試刪除測試 entry。
取消後清理使用獨立且有限的等待時間，不無限阻塞。
刪除失敗需顯示待清理的 entry 名稱與重試指引，且不得繼續認證或啟用。

已同意完成的套件安裝、新金鑰與 pass 初始化不會因後續取消而自動移除。
這些 host 變更與專案憑證切換的回復範圍不同，介面需提前說明。

## D App 與 PAT 認證契約

| 項目 | BYO GitHub App（Recommended） | Fine-grained token |
| --- | --- | --- |
| 輸入 | GitHub App client_id | 隱藏輸入使用者建立的 PAT |
| 授權 | Device Flow，沿用目前流程 | 不啟動 Device Flow |
| 帳號驗證 | 沿用目前 GitHub user 驗證 | 呼叫 GET /user 確認帳號 |
| repository | 沿用選擇與 scoped token 驗證 | 驗證目標 repository 的 API 可讀性與 ID |
| 更新 | 沿用 refresh token 流程 | 不呼叫 refresh endpoint；失效時重新輸入 |
| 儲存 | pass 內含 access／refresh token | pass 內僅含 PAT 與必要綁定資料 |

App 設定時可沿用或修改 client_id。
舊專案目前使用的 client_id 優先保留；共用設定仍可作為新設定的預設值。
本次設定中輸入的 client_id 綁定該專案，不因修改輸入而偷偷更新其他專案。
已儲存的專案認證不因共用預設值不同就被直接停用。
GitHub App 本身必須啟用 Device Flow，且完成適當的 GitHub 權限與安裝設定。

PAT 只接受 Fine-grained token，不接受 classic PAT 或 App token 冒充。
格式檢查不能取代 GitHub API 驗證，錯誤不得含有輸入值。
PAT 輸入必須關閉終端 echo；完成、取消或錯誤後都恢復終端狀態。
不支援安全輸入的介面應明確失敗，不降級成明文輸入。

PAT 不寫入專案 JSON、共用設定、命令參數、環境變數或產生的 unit／adapter。

PAT 提供 GitHub 建立指引，建議只選目前 repository 並依用途授予最小權限。
從 git remote 推導 repository；無法推導時可輸入 owner/repo。
用該 PAT 驗證帳號與目標 repository，顯示帳號及完整 repository 名稱供確認。
公開 repository 的 metadata 可讀性不代表具備 git push 或其他操作權限。

不得以 metadata 驗證宣稱 PAT 權限完整，或宣稱已證明 PAT 僅授權該 repository。
GitHub 上的組織核准與實際權限仍由使用者管理。
無法讀取、核准未完成或認證失敗時提供可操作的指引，不儲存候選憑證。

## E 背景 pinentry 與憑證生命週期

移除目前強制禁止 pinentry 的設定，明確使用允許正常 pinentry 的 GPG 模式。
繼續隔離外部 PASSWORD_STORE_GPG_OPTS，保留輸出大小限制與秘密遮蔽。
不使用 loopback 將 passphrase 送入 dch，也不儲存 passphrase。
service 保留 GNUPGHOME、PASSWORD_STORE_DIR，並傳遞必要的 GUI session 環境。

包含存在時的 DISPLAY、WAYLAND_DISPLAY、XAUTHORITY 與 DBUS_SESSION_BUS_ADDRESS。
不把設定時的 TTY 永久寫入背景 unit；不自動修改 gpg-agent.conf 或快取政策。
不自動安裝或更換使用者的 pinentry frontend；不可用時提供安裝或設定指引。

每次憑證請求允許最多 120 秒進行解鎖與取得憑證。
pass 執行受請求 context 與上述預算約束；取消後須停止等待並釋放鎖。
Unix HTTP client、容器端 git/gh adapter 與啟用驗證採相容期限。
外層傳輸可保留少量緩衝，但不可讓某一層仍以既有 10／20 秒提早中斷解鎖。

HTTP 到 GitHub 的網路請求仍採短而有限的逾時，不因 pinentry 而全部拉長。
service socket readiness 與實際 credential readiness 分開處理。

GUI host 上，由同一使用者的 gpg-agent 呼叫可用的圖形 pinentry。
關閉或拒絕 pinentry、session 不可用、缺少 frontend 或逾時時，憑證請求失敗。

介面說明如何確認 session／pinentry，或在 host 終端解鎖後重試。
純 SSH/headless 不自動把背景提示導向任意終端，使用者主動解鎖同一 agent 後重試。
快取中仍有效的憑證可能直接使用，不保證每次請求都會跳出 pinentry。

專案 config 與 pass token schema 新增明確的認證方式識別。
舊版未提供識別的有效設定解讀為 GitHub App，保留既有 entry 與 refresh 行為。
PAT 不偽造 refresh token 或無限的 App token 到期時間。

PAT 綁定專案、帳號與 repository；讀取時驗證綁定，並檢查已知的失效資訊。
GitHub 驗證失敗時提示檢查 PAT；不承諾 metadata 檢查能預知所有權限錯誤。
到期資訊若無可靠來源，不杜撰到期日，並以 GitHub 回應判斷是否需重新設定。

App 與 PAT 使用可區別且可安全清理的憑證識別，PAT 不依賴虛構 client_id。
沿用既有 project lock、pass entry lock、候選憑證驗證與 commit／rollback 邏輯。
切換成功才清理先前憑證；清理失敗保留待重試狀態。

切換失敗需還原被修改的憑證與設定；不得刪除其他專案或未擁有的 entry。
disable、重試清理與服務重啟都須辨識兩種認證方式。
更早的 installation private-key legacy 設定繼續隔離，不恢復已停用的舊模式。

## F 改動後的目錄架構與職責

下列新增檔案均為提案，實作前不建立空殼。
既有模組維持責任邊界；不因這次功能建立通用認證框架。

```tree
devcontainer-helper/
  internal/
    plugins/
      plugin.go | 既有互動契約；新增文字、秘密輸入與受控 host 互動執行能力
      manager.go | 既有設定快照、專案鎖與候選提交入口
      githubapp/
        plugin.go | 組裝認證選擇、前置驗證與候選設定
        config.go | 認證方式、舊設定相容與綁定驗證
        host_setup.go（新增提案） | GPG/pass 檢查、初始化與測試 secret 清理
        host_install.go（新增提案） | 發行版與 apt/dnf/pacman 命令規劃
        gpg.go（新增提案） | secret key 清單解析、可用性與互動建立
        pat.go（新增提案） | PAT 輸入後驗證、repository 選擇與候選憑證
        repository.go | 共用 GitHub API 與 repository 解析
        provider.go | App refresh、PAT 讀取與有界請求
        secret.go | pass 封裝、鎖、輸出限制與讀寫驗證
        manager.go | systemd 環境、服務啟用與資源回復
        server.go | Host credential socket、請求 context 與期限
        adapter.go | Container 的 git/gh adapter 與相容等待期限
        *_test.go | 各行為既有測試與新增回歸測試
    console/
      plugins.go | 互動能力組裝與產品訊息
      plugin_input.go（新增提案） | 終端隱藏輸入、復原與取消
      *_test.go | 輸入與選單行為測試
  README.md | 認證選擇、安裝與 pinentry 操作指引
  specs/
    2026-10-26-github-auth-pass-setup-spec.md | 本規格來源
    2026-10-26-github-auth-pass-setup-spec.html | 完整審閱頁
```

共用互動契約放在 internal/plugins，由 console 提供實作。
可用小型可選介面新增輸入及互動執行能力，避免迫使所有非互動 plugin 實作無關方法。
plugin 僅請求受控的 host 命令；console 負責終端接管、取消與畫面恢復。
安裝器、GPG 建立與隱藏輸入的新增公開方法遵循 Go 文件慣例。

```flow
Host console -> Plugins Manager: ConfigureAndEnable
Plugins Manager -> GitHub plugin: Configure
GitHub plugin -> Host setup: 安裝與密碼庫驗證
Host setup -> Host console: 確認及互動命令
GitHub plugin -> App 或 PAT: 認證與驗證
App 或 PAT -> SecretStore: 提交候選憑證
SecretStore -> Host pass 與 gpg-agent
Plugins Manager -> Host systemd service: 啟用與驗證
Container git 與 gh -> Host credential socket
Host credential socket -> Provider
Provider -> SecretStore
Host pass 與 gpg-agent -> GUI pinentry: 需要解鎖時
```

## G 測試邊界與驗收條件

優先使用 ConfigureAndEnable／Configure、Provider／credential service 等既有邊界。
外部命令替身用於觀察安裝、GPG 與 pass 行為，不測私有函式實作細節。
沿用現有 device、provider、secret、manager 與 adapter 測試風格。
隱藏輸入以可控制終端或替身驗證 echo 恢復與取消，不使用真實 token。

| 編號 | 必須可觀察的驗收行為 |
| --- | --- |
| AC01 | 顯示兩種選項；App 預設且標示 Recommended；PAT 不要求 client_id |
| AC02 | App 能沿用或輸入 client_id，完成既有 Device Flow、帳號與 scope 驗證 |
| AC03 | 前置驗證完成前，不呼叫 Device Flow，也不詢問 PAT |
| AC04 | apt、dnf、pacman 只安裝缺少項目；未同意不執行安裝 |
| AC05 | 安裝失敗、拒絕、取消與不支援平台均不啟用，並提供指引 |
| AC06 | 選擇有效加密 secret key；建立流程失敗或取消不繼續 |
| AC07 | 未初始化時 pass init 使用完整 fingerprint；已初始化不重寫收件人 |
| AC08 | 測試 secret 寫讀相符且清除；錯誤、取消及清理失敗都阻止啟用 |
| AC09 | PAT 隱藏輸入且復原終端；無安全輸入能力時失敗，秘密不外洩 |
| AC10 | PAT 驗證帳號與 repository，拒絕 classic／錯誤／失效 token，無 refresh 呼叫 |
| AC11 | PAT provider 能提供憑證；失效與權限錯誤提供重新設定指引 |
| AC12 | 背景 pass 允許 pinentry；unit 傳遞必要 session 與密碼庫環境 |
| AC13 | 解鎖等待容納 120 秒；各層不提早截斷；取消／逾時停止並釋放鎖 |
| AC14 | 舊 App 設定與 pass entry 可繼續使用；既有 App refresh 回歸測試通過 |
| AC15 | App/PAT 雙向切換及重新授權失敗時保留原設定，成功後才清理舊 entry |
| AC16 | disable、清理重試不刪除其他專案秘密；legacy quarantine 行為保留 |

自動測試使用替身命令、本機 HTTP 與 Unix socket，不安裝 host 套件。
完整測試不依賴 live Docker、GitHub、systemd 或實際圖形桌面。
逾時測試使用可注入的短期限，不讓測試等待完整 120 秒。
需檢查生成 adapter／unit 的等待與環境契約，並測試整合取消傳播。

整合檢查包含 gofmt、go test ./...、go vet ./... 與 make build。
使用適合 Go 的 Cognitive Complexity 分析器，檢查所有新增或修改函式，含測試與匿名函式。
每個函式上限 15；超過時重構，不以 suppression 排除。

TDD 每項行為保留預期失敗與通過證據。
Code 與 Security Review 最多三輪，修正 item 各自提交。

實機手動驗證另外記錄，包含三種套件管理器的安裝路徑、GUI pinentry 解鎖／取消、
agent 快取失效與 SSH/headless 解鎖後重試。
無對應實機時明列未驗證，不把替身測試當成真實平台測試。

## H 範圍外、流程與參考資料

本次不新增 macOS、Windows、遠端 Docker 或 Docker Compose 支援。
不自動設定 GitHub App 權限／安裝、不替使用者建立 PAT、不管理組織審批。
不提供跨專案共用 PAT、classic PAT、其他 keyring backend 或無人值守 passphrase 儲存。
不重新加密既有密碼庫、不自動變更 gpg-agent 快取政策。
不自動把背景 TTY pinentry 接到 SSH session，不保證所有桌面皆有可用 frontend。

本規格核准前不拆 tickets 或修改產品程式。
核准後在 .scratch/github-auth-pass-setup 建立本機 tickets 與進度索引。
每個可實作 item 驗證通過後獨立 commit，子代理不自行 stage／commit。
最終提供 Code/Security Review、測試證據與 HTML review，等待明確核准再 push 與建立 PR。

規劃前基線：2026-10-26 執行 go test ./...，全部套件通過。
該結果僅為現有程式基線，不代表本功能已實作或驗證。

參考資料：

- [GitHub App 使用者認證](https://docs.github.com/en/apps/creating-github-apps/authenticating-with-a-github-app/authenticating-with-a-github-app-on-behalf-of-a-user)
- [GitHub PAT 建立與權限](https://docs.github.com/en/authentication/keeping-your-account-and-data-secure/managing-your-personal-access-tokens)
- [GitHub repository API](https://docs.github.com/en/rest/repos/repos#get-a-repository)
- [GitHub PAT repository 清單 API 的限制](https://docs.github.com/en/rest/orgs/personal-access-tokens#list-repositories-a-fine-grained-personal-access-token-has-access-to)
- [pass 初始化與儲存模型](https://www.passwordstore.org/)
- [GnuPG agent 與 pinentry](https://www.gnupg.org/documentation/manuals/gnupg/Invoking-GPG_002dAGENT.html)
- [GnuPG 啟動 TTY 與 display 更新](https://www.gnupg.org/documentation/manuals/gnupg/Agent-UPDATESTARTUPTTY.html)
- [byteness/keyring pass backend](https://github.com/byteness/keyring/blob/main/pass.go)

keyring 評估結論：其 pass backend 無呼叫端 context／timeout，且無現有交易與讀回驗證。
直接替換須補回既有能力，本次保留現有封裝。
