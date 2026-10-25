---
title: Project Plugins 最終 Review
subtitle: github-app · hard dev-flow · 待使用者最終核准
lang: zh-Hant
template: doc
theme: shadcn
source: 34b3414 → 759b9d8
---

## A 本次交付

- 每個 Project 可在 Console 啟用內建 GitHub App plugin。
- Git 與 gh 透過 Host Unix Socket 取得短期 Token。
- 私鑰留在 Host pass；Token 只存 Host 記憶體。
- local.d 取代舊 local.json。這是不相容變更。
- 自動檢查通過；真實 Linux 環境驗收仍未執行。

```callout warn 核准範圍
本頁有 6 個獨立 review 勾選框。
勾選只用於本頁核對，不儲存狀態。
全部勾選不會 push 或建立 PR。
仍需在對話中明確給予最終核准。
```

完整版本：`34b3414f2451e2e713a0880d3f533bd48e5b4614` → `759b9d8f45700dbc60014f7bce52277e48a7eb3d`。

[完整最終 diff](2026-10-24-project-plugins-changes.diff) · [已核准規格](2026-10-24-project-plugins-spec.html)

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

G1–G6 對應下方直接展開的變更群組。
G7 是測試；G8 是文件。
「新增／修改」指群組內包含兩類檔案。

## C 程式碼與決策

八組重點變更使用原生 diff 格式。
配色區分新增與刪除，兩欄行號對照原始檔案。
節錄保留連續原文，hunk 範圍按實際行數標示。
其餘變更請由索引查閱[完整 diff](2026-10-24-project-plugins-changes.diff)。

### G1 修改：local.d 合併與預覽

確認舊 local.json 靜默停用。需要保留設定者須自行搬移。

需你 review：是。

```html
<label><input type="checkbox" id="check-G1"> 我已確認 G1 的決策與風險</label>
```

以下是實際 diff 節錄。完整內容見[完整 diff](2026-10-24-project-plugins-changes.diff)。

以排序片段取代單一 local 檔。

```diff file=internal/common/devcontainer_config.go
@@ -20,10 +19,26 @@
 		return nil, err
 	}
 
-	localPath := filepath.Join(configDir, "devcontainer.local.json")
-	local, err := readJSONCObject(localPath)
-	if err != nil {
-		if !errors.Is(err, os.ErrNotExist) {
+	merged := base
+	localDir := filepath.Join(configDir, "devcontainer.local.d")
+	entries, err := os.ReadDir(localDir)
+	if err != nil && !os.IsNotExist(err) {
+		return nil, fmt.Errorf("%s: %w", localDir, err)
+	}
+	for _, entry := range entries {
+		if filepath.Ext(entry.Name()) != ".json" {
+			continue
+		}
+		path := filepath.Join(localDir, entry.Name())
+		info, err := entry.Info()
+		if err != nil {
+			return nil, fmt.Errorf("%s: %w", path, err)
+		}
+		if !info.Mode().IsRegular() {
+			continue
+		}
+		fragment, err := readJSONCObject(path)
+		if err != nil {
 			return nil, err
 		}
-		local = map[string]any{}
+		merged = mergeConfigValues(merged, fragment).(map[string]any)
```

變更索引（檔案與新版本 hunk 起始行）：

- `internal/common/devcontainer_config.go:3,10,19`
- `internal/console/config_preview.go:18,72`
- `.gitignore:3`

### G2 新增／修改：Project plugin 管理

確認每個 Project 獨立設定與服務。摘要比對保護手改檔；失敗保留可重試狀態。

需你 review：是。

```html
<label><input type="checkbox" id="check-G2"> 我已確認 G2 的決策與風險</label>
```

以下是實際 diff 節錄。完整內容見[完整 diff](2026-10-24-project-plugins-changes.diff)。

啟用時記錄摘要與待 Build 狀態。

```diff file=internal/plugins/manager.go
@@ -0,0 +84,28 @@
+	if err := githubapp.WriteAdapterScripts(runtimeDir); err != nil {
+		return errors.Join(err, restoreFile(fragmentPath, oldBytes, oldFileExists))
+	}
+	unit, err := m.prepareServiceUnit(path, old.States[githubAppID])
+	if err != nil {
+		return errors.Join(err, restoreFile(fragmentPath, oldBytes, oldFileExists))
+	}
+
+	newDigest := digest(fragment)
+	if config.ProjectPlugins == nil {
+		config.ProjectPlugins = map[string]common.ProjectPlugins{}
+	}
+	state := old.States[githubAppID]
+	state.FragmentDigest = newDigest
+	state.UnitDigest = unit.digest
+	state.UnitPath = unit.path
+	state.BuildFingerprint = buildFingerprint(cfg, newDigest)
+	state.BuildPending = true
+	project := cloneProjectPlugins(old)
+	if project.Enabled == nil {
+		project.Enabled = map[string]githubapp.Config{}
+	}
+	if project.States == nil {
+		project.States = map[string]common.PluginState{}
+	}
+	project.Enabled[githubAppID] = cfg
+	project.States[githubAppID] = state
+	config.ProjectPlugins[path] = project
```

停用前驗證檔案，停止並清理服務。

```diff file=internal/plugins/manager.go
@@ -0,0 +364,27 @@
+	if info, err := os.Lstat(unitFile); err == nil {
+		if !info.Mode().IsRegular() || !ownedByCurrentUser(info) {
+			return fmt.Errorf("systemd unit is unsafe")
+		}
+		data, err := os.ReadFile(unitFile)
+		if err != nil {
+			return err
+		}
+		if project.States[githubAppID].UnitDigest == "" || digest(data) != project.States[githubAppID].UnitDigest {
+			return fmt.Errorf("systemd unit was modified; refusing to remove it")
+		}
+	} else if !errors.Is(err, os.ErrNotExist) {
+		return err
+	}
+	if _, _, err := readOwnedFile(managedFragment(path), project.States[githubAppID].FragmentDigest); err != nil {
+		return err
+	}
+	if err := runSystemctl(ctx, "stop", unitName(path)); err != nil {
+		return err
+	}
+	if err := runSystemctl(ctx, "disable", unitName(path)); err != nil {
+		return err
+	}
+	if err := os.Remove(unitFile); err != nil && !errors.Is(err, os.ErrNotExist) {
+		return err
+	}
+	if err := runSystemctl(ctx, "daemon-reload"); err != nil {
```

變更索引（檔案與新版本 hunk 起始行）：

- `internal/plugins/registry.go:1`
- `internal/plugins/manager.go:1`
- `internal/common/config_store.go:7`

### G3 新增：GitHub App provider

確認單一 repository 與選定權限。先用 App JWT 驗證 installation，再申請 Token。

需你 review：是。

```html
<label><input type="checkbox" id="check-G3"> 我已確認 G3 的決策與風險</label>
```

以下是實際 diff 節錄。完整內容見[完整 diff](2026-10-24-project-plugins-changes.diff)。

五分鐘快取門檻與並行去重。

```diff file=internal/githubapp/provider.go
@@ -0,0 +92,24 @@
+func (p *Provider) Token(ctx context.Context) (string, error) {
+	for {
+		p.mu.Lock()
+		if p.token != "" && p.opts.Now().Add(5*time.Minute).Before(p.expires) {
+			token := p.token
+			p.mu.Unlock()
+			return token, nil
+		}
+		if call := p.refreshing; call != nil {
+			p.mu.Unlock()
+			select {
+			case <-call.done:
+				if call.err != nil {
+					return "", call.err
+				}
+				return call.token, nil
+			case <-ctx.Done():
+				return "", ctx.Err()
+			}
+		}
+		call := &refreshCall{done: make(chan struct{})}
+		p.refreshing = call
+		p.mu.Unlock()
+		token, expiry, err := p.mint(ctx)
```

先驗證 installation，再限定 repo 與權限。

```diff file=internal/githubapp/provider.go
@@ -0,0 +163,28 @@
+	jwt := unsigned + "." + base64.RawURLEncoding.EncodeToString(signature)
+	owner, repo, _ := strings.Cut(p.cfg.Repository, "/")
+	verifyURL := strings.TrimRight(p.opts.APIBaseURL, "/") + "/repos/" + url.PathEscape(owner) + "/" + url.PathEscape(repo) + "/installation"
+	verifyReq, _ := http.NewRequestWithContext(ctx, http.MethodGet, verifyURL, nil)
+	verifyReq.Header.Set("Authorization", "Bearer "+jwt)
+	verifyReq.Header.Set("Accept", "application/vnd.github+json")
+	verifyReq.Header.Set("X-GitHub-Api-Version", "2022-11-28")
+	verifyResp, err := p.opts.HTTPClient.Do(verifyReq)
+	if err != nil {
+		return "", time.Time{}, fmt.Errorf("repository installation verification failed")
+	}
+	defer verifyResp.Body.Close()
+	verifyData, err := io.ReadAll(io.LimitReader(verifyResp.Body, maxResponseBytes+1))
+	var installation struct {
+		ID int64 `json:"id"`
+	}
+	if err != nil || len(verifyData) > maxResponseBytes || verifyResp.StatusCode != http.StatusOK || json.Unmarshal(verifyData, &installation) != nil || installation.ID != p.cfg.InstallationID {
+		return "", time.Time{}, fmt.Errorf("repository installation verification failed")
+	}
+	var body bytes.Buffer
+	permissions := make(map[string]string, len(p.cfg.Permissions))
+	for k, v := range p.cfg.Permissions {
+		permissions[k] = v
+	}
+	if err := json.NewEncoder(&body).Encode(map[string]any{"repositories": []string{repo}, "permissions": permissions}); err != nil {
+		return "", time.Time{}, err
+	}
+	u := strings.TrimRight(p.opts.APIBaseURL, "/") + fmt.Sprintf("/app/installations/%d/access_tokens", p.cfg.InstallationID)
```

變更索引（檔案與新版本 hunk 起始行）：

- `internal/githubapp/config.go:1`
- `internal/githubapp/provider.go:1`

### G4 新增：Host Unix Socket

確認信任邊界：可連 Socket 的程序可取 Token。目錄 0700、Socket 0600；不開 TCP。

需你 review：是。

```html
<label><input type="checkbox" id="check-G4"> 我已確認 G4 的決策與風險</label>
```

以下是實際 diff 節錄。完整內容見[完整 diff](2026-10-24-project-plugins-changes.diff)。

Project 專屬私有 runtime 目錄。

```diff file=internal/githubapp/runtime.go
@@ -0,0 +12,23 @@
+func RuntimeDir(projectPath string) (string, error) {
+	runtime := os.Getenv("XDG_RUNTIME_DIR")
+	if runtime == "" {
+		return "", fmt.Errorf("XDG_RUNTIME_DIR is required")
+	}
+	info, err := os.Lstat(runtime)
+	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || !ownedByCurrentUser(info) || info.Mode().Perm()&0077 != 0 {
+		return "", fmt.Errorf("XDG_RUNTIME_DIR is not a private directory owned by the current user")
+	}
+	abs, err := filepath.Abs(projectPath)
+	if err != nil || projectPath == "" {
+		return "", fmt.Errorf("invalid project path")
+	}
+	key := sha256.Sum256([]byte(filepath.Clean(abs)))
+	dir := filepath.Join(runtime, "dch", hex.EncodeToString(key[:12]))
+	parent := filepath.Join(runtime, "dch")
+	if err := ensurePrivateDir(parent); err != nil {
+		return "", err
+	}
+	if err := ensurePrivateDir(dir); err != nil {
+		return "", err
+	}
+	return dir, nil
```

Unix Socket 權限與固定 endpoint。

```diff file=internal/githubapp/server.go
@@ -0,0 +42,35 @@
+	listener, err := net.Listen("unix", socketPath)
+	if err != nil {
+		return fmt.Errorf("cannot listen on credential socket")
+	}
+	if err := os.Chmod(socketPath, 0600); err != nil {
+		listener.Close()
+		os.Remove(socketPath)
+		return fmt.Errorf("cannot secure credential socket")
+	}
+	provider, err := NewProvider(cfg)
+	if err != nil {
+		listener.Close()
+		os.Remove(socketPath)
+		return err
+	}
+	mux := http.NewServeMux()
+	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
+		if r.Method != http.MethodGet {
+			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
+			return
+		}
+		w.WriteHeader(http.StatusOK)
+		_, _ = io.WriteString(w, "ok\n")
+	})
+	mux.HandleFunc("/credential", func(w http.ResponseWriter, r *http.Request) {
+		w.Header().Set("Cache-Control", "no-store")
+		if r.Method != http.MethodGet {
+			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
+			return
+		}
+		token, err := provider.Token(r.Context())
+		if err != nil {
+			http.Error(w, "credential unavailable", http.StatusServiceUnavailable)
+			return
+		}
```

變更索引（檔案與新版本 hunk 起始行）：

- `internal/githubapp/runtime.go:1`
- `internal/githubapp/server.go:1`

### G5 新增：Git／gh 容器接線

確認唯讀掛載整個 runtime 目錄。HTTPS github.com 使用 helper；gh 子程序取得 GH_TOKEN。

需你 review：是。

```html
<label><input type="checkbox" id="check-G5"> 我已確認 G5 的決策與風險</label>
```

以下是實際 diff 節錄。完整內容見[完整 diff](2026-10-24-project-plugins-changes.diff)。

唯讀掛載整個 runtime 目錄。

```diff file=internal/githubapp/adapter.go
@@ -0,0 +15,17 @@
+// AdapterFragment returns the devcontainer settings required by the GitHub App adapter.
+func AdapterFragment(socketDir string) ([]byte, error) {
+	if !filepath.IsAbs(socketDir) || socketDir == "/" || strings.ContainsAny(socketDir, ",\r\n") || strings.Contains(socketDir, "${") {
+		return nil, fmt.Errorf("invalid GitHub App runtime directory")
+	}
+	fragment := struct {
+		Mounts    []string          `json:"mounts"`
+		RemoteEnv map[string]string `json:"remoteEnv"`
+	}{
+		Mounts:    []string{"source=" + socketDir + ",target=" + containerSocketDir + ",type=bind,readonly"},
+		RemoteEnv: map[string]string{"PATH": containerSocketDir + "/bin:${containerEnv:PATH}"},
+	}
+	var out bytes.Buffer
+	encoder := json.NewEncoder(&out)
+	encoder.SetEscapeHTML(false)
+	if err := encoder.Encode(fragment); err != nil {
+		return nil, fmt.Errorf("encode GitHub App adapter fragment: %w", err)
```

Git helper 僅回應 HTTPS github.com。

```diff file=internal/githubapp/adapter.go
@@ -0,0 +119,17 @@
+const gitCredentialScript = `#!/bin/sh
+set -eu
+operation=${1-}
+[ "$operation" = get ] || exit 0
+protocol=
+host=
+while IFS= read -r line; do
+	case "$line" in
+		protocol=*) protocol=${line#protocol=} ;;
+		host=*) host=${line#host=} ;;
+	esac
+done
+[ "$protocol" = https ] && [ "$host" = github.com ] || exit 0
+token=$(curl --disable --noproxy '*' --unix-socket /run/dch-github/credential.sock --fail --silent --show-error --max-time 20 http://localhost/credential) || exit 1
+[ -n "$token" ] || exit 1
+printf 'username=x-access-token\npassword=%s\n\n' "$token"
+`
```

gh wrapper 只對子程序設定 Token。

```diff file=internal/githubapp/adapter.go
@@ -0,0 +151,12 @@
+done
+PATH=$clean_path
+export PATH
+real_gh=$(command -v gh) || { echo 'dch: gh is required' >&2; exit 127; }
+token=$(curl --disable --noproxy '*' --unix-socket /run/dch-github/credential.sock --fail --silent --show-error --max-time 20 http://localhost/credential) || { echo 'dch: GitHub credential unavailable' >&2; exit 1; }
+[ -n "$token" ] || { echo 'dch: GitHub credential unavailable' >&2; exit 1; }
+GH_TOKEN=$token
+export GH_TOKEN
+exec "$real_gh" "$@"
+`
+
+const adapterGitConfig = `[credential "https://github.com"]
```

變更索引（檔案與新版本 hunk 起始行）：

- `internal/githubapp/adapter.go:1`

### G6 新增／修改：Console 與執行生命週期

確認既有容器待 Build 時阻擋 Start／Connect。Build 保留移除確認；移除前清理服務。

需你 review：是。

```html
<label><input type="checkbox" id="check-G6"> 我已確認 G6 的決策與風險</label>
```

以下是實際 diff 節錄。完整內容見[完整 diff](2026-10-24-project-plugins-changes.diff)。

既有容器待 Build 時阻擋 Start。

```diff file=internal/console/up.go
@@ -3,5 +10,29 @@
+
 func startProject(p Project) error {
+	manager := plugins.NewManager()
+	status, err := manager.Status(context.Background(), p.Path)
+	if err != nil {
+		return err
+	}
+	if status.PendingBuild && hasExistingContainer(p) {
+		return fmt.Errorf("%s has project plugin changes; Build the project before starting", p.Name)
+	}
 	if err := requireExecutable("devcontainer"); err != nil {
 		return err
 	}
-	return invokeProjectDevcontainer(p, "up", "--workspace-folder", p.Path)
+	if status.Enabled {
+		if err := prepareProjectPlugin(p); err != nil {
+			return err
+		}
+	}
+	if err := invokeProjectDevcontainer(p, "up", "--workspace-folder", p.Path); err != nil {
+		return err
+	}
+	if status.Enabled {
+		if err := runAdapterInstall(p); err != nil {
+			return err
+		}
+	}
+	if p.State == "NOT CREATED" {
+		return manager.MarkBuilt(p.Path)
+	}
```

啟動前檢查合併後的必要設定。

```diff file=internal/console/devcontainer.go
@@ -68,7 +80,21 @@
+func invokeProjectDevcontainerMode(p Project, quiet bool, args ...string) error {
 	config, err := common.MergedDevcontainerConfig(p.Path)
 	if err != nil {
 		return err
 	}
+	status, err := plugins.NewManager().Status(context.Background(), p.Path)
+	if err != nil {
+		return err
+	}
+	if status.Enabled {
+		runtimeDir, err := githubapp.RuntimeDir(p.Path)
+		if err != nil {
+			return err
+		}
+		if err := validateGitHubAppConfig(config, runtimeDir); err != nil {
+			return err
+		}
+	}
 	if len(args) > 0 && args[0] == "up" {
 		config, err = withStableContainerName(config, p)
 		if err != nil {
```

變更索引（檔案與新版本 hunk 起始行）：

- `internal/console/plugins.go:1`
- `internal/console/plugin_integration.go:1`
- `internal/console/projects_menu.go:1,17,39,87,108,120,138,176,192`
- `internal/console/devcontainer.go:1,11,70,116,126`
- `internal/console/up.go:1`
- `internal/console/build.go:1,42`
- `internal/console/connect.go:1`
- `internal/common/remove_project.go:2,24`
- `internal/commands/commands.go:1,57,82`

### G7 新增／修改：行為測試

測試是上述行為的證據。涵蓋假指令、真實暫存 Socket 與 Git plumbing。

需你 review：否。

以下是實際 diff 節錄。完整內容見[完整 diff](2026-10-24-project-plugins-changes.diff)。

錯誤 installation 的 API 回應。

```diff file=internal/githubapp/provider_test.go
@@ -0,0 +111,9 @@
+func TestProviderRejectsWrongRepositoryInstallationBeforeMinting(t *testing.T) {
+	key := testPrivateKey(t)
+	var tokenCalls, lookupCalls atomic.Int32
+	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
+		if r.URL.Path == "/repos/octo/repo/installation" {
+			lookupCalls.Add(1)
+			_, _ = w.Write([]byte(`{"id":99}`))
+			return
+		}
```

驗證失敗不得申請 Token。

```diff file=internal/githubapp/provider_test.go
@@ -0,0 +137,13 @@
+	p, err := NewProvider(Config{AppID: 1, InstallationID: 34, PrivateKeyPassEntry: "key", Repository: "octo/repo", Permissions: map[string]string{"contents": "read"}}, ProviderOptions{PassPath: pass, APIBaseURL: srv.URL, HTTPClient: srv.Client()})
+	if err != nil {
+		t.Fatal(err)
+	}
+	if token, err := p.Token(context.Background()); err == nil || token != "" {
+		t.Fatalf("Token() = %q, %v; want rejected installation", token, err)
+	}
+	if tokenCalls.Load() != 0 {
+		t.Fatalf("minted %d tokens for wrong installation", tokenCalls.Load())
+	}
+	if lookupCalls.Load() != 1 {
+		t.Fatalf("installation lookup calls=%d; want 1", lookupCalls.Load())
+	}
```

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

以下是實際 diff 節錄。完整內容見[完整 diff](2026-10-24-project-plugins-changes.diff)。

文件列出 local.d 與 Project plugin 行為。

```diff file=README.md
@@ -5,7 +5,8 @@
 | Feature | Description |
 | --- | --- |
 | Manage Dev Containers in one place | View the status of multiple projects in a terminal interface, and connect to, start, build, or stop their containers. |
-| Apply personal settings | Merge `devcontainer.local.json` with the project configuration for previews and container operations, without changing the original configuration file. |
+| Apply personal settings | Merge sorted `.devcontainer/devcontainer.local.d/*.json` fragments with the project configuration for previews and container operations, without changing the original configuration file. |
+| Project plugins | Configure per-project host services such as GitHub App credentials, then apply their container settings with Build. |
 
 ## Prerequisites
 
```

變更索引（檔案與新版本 hunk 起始行）：

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

| 指令或範圍 | 結果 | 證據與限制 |
| --- | --- | --- |
| go test ./... -count=1 | PASS | 最終版本全套測試；由主代理執行 |
| go build ./... | PASS | 最終版本編譯；由主代理執行 |
| go vet ./... | PASS | 最終版本靜態檢查；由主代理執行 |
| git diff --check | PASS | 最終版本格式檢查；由主代理執行 |
| go run . help | PASS | CLI help 可執行；由主代理執行 |
| 設定與生命週期 | PASS | 排序／JSONC／手改保護／回滾／清理重試／待 Build |
| Provider 與 Socket | PASS | 假 pass、httptest、可控時間、真實暫存 Unix Socket |
| 容器接線 | PASS | 假 devcontainer／systemctl／Docker context；非真實整合 |
| Git／gh | PASS | 真實 Git credential plumbing；假 curl 與 gh |
| go test -race ./... | NOT RUN | CGO 關閉，且無 C compiler |
| 真實 Linux 驗收 | NOT RUN | 缺 Docker／systemctl／pass；未用真實 GitHub 與 GPG |
| UID／重啟／關閉 dch 後認證 | NOT RUN | 需在真實 Linux 容器執行 README 驗收 |
| HTML 原生 diff | PASS | 靜態檢查配色、雙欄行號、真實節錄、完整索引與六個未勾選框 |
| HTML 瀏覽器／窄視窗 | PASS | 獨立 Chromium 155；桌面 1440×900、手機 390×844 |
| HTML 原生 diff 與勾選 | PASS | 15 塊可見；雙欄行號、檔名與增刪配色；六框可勾選及取消 |
| HTML 版面與 CSP | PASS | 無頁面水平溢出；無自訂 popover；無 CSP 錯誤 |

HTML 實測使用獨立 Chromium，不是應用程式預覽。
實測涵蓋 DOM、互動與視窗寬度；未驗收中文字型顯示。
此結果不代表真實 Linux 整合驗收已完成。

Root 執行測試時，DAC 拒絕案例會跳過。
假 health 失敗與 UID 錯誤提示仍有測試。
這不代表真實 UID 不符驗收已完成。

| 獨立 reviewer | 範圍 | 最終結論 |
| --- | --- | --- |
| Code reviewer | 最終完整 diff；Standards 與核准 Spec | 0 項未解決有效 finding |
| Security reviewer | provider、Socket、runtime、adapter、manager 與設定接線 | 0 項未解決可採取行動的安全 finding |

兩位 reviewer 均覆核最終版本 759b9d8。
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
本頁包含 repository 實際程式碼 diff。
未讀取或嵌入使用者本機憑證與 local 設定。
本次交付只建立 review 文件，不執行 PR 或 push。
