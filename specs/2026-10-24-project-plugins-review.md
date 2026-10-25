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

八組實際 diff 直接顯示，不需點擊。
新檔案完整列出；既有檔案保留完整 hunk。
生成的規格 HTML 另見完整 diff。

```html
<div><h3>G1 修改：local.d 合併與預覽</h3><p>確認舊 local.json 靜默停用。需要保留設定者須自行搬移。</p><p>需你 review：是</p><label><input type="checkbox" id="check-G1"> 我已確認 G1 的決策與風險</label><p><strong>檔案與新版本 hunk 起始行</strong></p><ul><li>internal/common/devcontainer_config.go:3,10,19</li><li>internal/console/config_preview.go:18,72</li><li>.gitignore:3</li></ul></div><div id="review-G1"><p>版本 759b9d8；以下為完整檔案 diff（保留完整 hunk）。行號見 @@ 的 + 區段。</p><h4>internal/common/devcontainer_config.go</h4><pre style="white-space:pre-wrap;overflow-wrap:anywhere"><code>diff --git a/internal/common/devcontainer_config.go b/internal/common/devcontainer_config.go
index 02065fc..6f2df42 100644
--- a/internal/common/devcontainer_config.go
+++ b/internal/common/devcontainer_config.go
@@ -3,7 +3,6 @@ package common
 import (
 	&quot;bytes&quot;
 	&quot;encoding/json&quot;
-	&quot;errors&quot;
 	&quot;fmt&quot;
 	&quot;io&quot;
 	&quot;os&quot;
@@ -11,7 +10,7 @@ import (
 )
 
 // MergedDevcontainerConfig returns the project&#x27;s base configuration merged with
-// its optional per-user devcontainer.local.json file.
+// its optional first-level JSON fragments from devcontainer.local.d, in filename order.
 func MergedDevcontainerConfig(projectPath string) ([]byte, error) {
 	configDir := filepath.Join(projectPath, &quot;.devcontainer&quot;)
 	basePath := filepath.Join(configDir, &quot;devcontainer.json&quot;)
@@ -20,16 +19,31 @@ func MergedDevcontainerConfig(projectPath string) ([]byte, error) {
 		return nil, err
 	}
 
-	localPath := filepath.Join(configDir, &quot;devcontainer.local.json&quot;)
-	local, err := readJSONCObject(localPath)
-	if err != nil {
-		if !errors.Is(err, os.ErrNotExist) {
+	merged := base
+	localDir := filepath.Join(configDir, &quot;devcontainer.local.d&quot;)
+	entries, err := os.ReadDir(localDir)
+	if err != nil &amp;&amp; !os.IsNotExist(err) {
+		return nil, fmt.Errorf(&quot;%s: %w&quot;, localDir, err)
+	}
+	for _, entry := range entries {
+		if filepath.Ext(entry.Name()) != &quot;.json&quot; {
+			continue
+		}
+		path := filepath.Join(localDir, entry.Name())
+		info, err := entry.Info()
+		if err != nil {
+			return nil, fmt.Errorf(&quot;%s: %w&quot;, path, err)
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
 	}
 
-	merged := mergeConfigValues(base, local).(map[string]any)
 	var out bytes.Buffer
 	encoder := json.NewEncoder(&amp;out)
 	encoder.SetIndent(&quot;&quot;, &quot;  &quot;)
</code></pre><h4>internal/console/config_preview.go</h4><pre style="white-space:pre-wrap;overflow-wrap:anywhere"><code>diff --git a/internal/console/config_preview.go b/internal/console/config_preview.go
index eeafd16..0259656 100644
--- a/internal/console/config_preview.go
+++ b/internal/console/config_preview.go
@@ -18,7 +18,7 @@ func showProjectConfig(p Project) error {
 	content := strings.TrimSuffix(string(data), &quot;\n&quot;)
 	if !screenActive {
 		fmt.Println(content)
-		fmt.Println(&quot;\nKeep .devcontainer/devcontainer.local.json out of version control.&quot;)
+		fmt.Println(&quot;\nKeep .devcontainer/devcontainer.local.d/ out of version control.&quot;)
 		return nil
 	}
 	return showConfigPages(content)
@@ -72,7 +72,7 @@ func showConfigPages(content string) error {
 		}
 		fmt.Printf(&quot;  ╰%s╯\n&quot;, strings.Repeat(&quot;─&quot;, width-4))
 		fmt.Printf(&quot;  Lines %d-%d of %d  ·  ↑/↓ or j/k scroll  ·  q/Esc back\n&quot;, offset+1, end, len(lines))
-		fmt.Println(&quot;  Keep .devcontainer/devcontainer.local.json out of version control.&quot;)
+		fmt.Println(&quot;  Keep .devcontainer/devcontainer.local.d/ out of version control.&quot;)
 	}
 	render()
 	for {
</code></pre><h4>.gitignore</h4><pre style="white-space:pre-wrap;overflow-wrap:anywhere"><code>diff --git a/.gitignore b/.gitignore
index c31985e..4bf530f 100644
--- a/.gitignore
+++ b/.gitignore
@@ -3,3 +3,4 @@
 .DS_Store
 
 .devcontainer/devcontainer.local.json
+.devcontainer/devcontainer.local.d/
</code></pre></div>
<div><h3>G2 新增／修改：Project plugin 管理</h3><p>確認每個 Project 獨立設定與服務。摘要比對保護手改檔；失敗保留可重試狀態。</p><p>需你 review：是</p><label><input type="checkbox" id="check-G2"> 我已確認 G2 的決策與風險</label><p><strong>檔案與新版本 hunk 起始行</strong></p><ul><li>internal/plugins/registry.go:1</li><li>internal/plugins/manager.go:1</li><li>internal/common/config_store.go:7</li></ul></div><div id="review-G2"><p>版本 759b9d8；以下為完整檔案 diff（保留完整 hunk）。行號見 @@ 的 + 區段。</p><h4>internal/plugins/registry.go</h4><pre style="white-space:pre-wrap;overflow-wrap:anywhere"><code>diff --git a/internal/plugins/registry.go b/internal/plugins/registry.go
new file mode 100644
index 0000000..dbd5108
--- /dev/null
+++ b/internal/plugins/registry.go
@@ -0,0 +1,10 @@
+package plugins
+
+type Plugin struct {
+	ID   string
+	Name string
+}
+
+func Builtins() []Plugin {
+	return []Plugin{{ID: githubAppID, Name: &quot;GitHub App&quot;}}
+}
</code></pre><h4>internal/plugins/manager.go</h4><pre style="white-space:pre-wrap;overflow-wrap:anywhere"><code>diff --git a/internal/plugins/manager.go b/internal/plugins/manager.go
new file mode 100644
index 0000000..b197254
--- /dev/null
+++ b/internal/plugins/manager.go
@@ -0,0 +1,769 @@
+package plugins
+
+import (
+	&quot;bytes&quot;
+	&quot;context&quot;
+	&quot;crypto/sha256&quot;
+	&quot;encoding/hex&quot;
+	&quot;encoding/json&quot;
+	&quot;errors&quot;
+	&quot;fmt&quot;
+	&quot;net&quot;
+	&quot;os&quot;
+	&quot;os/exec&quot;
+	&quot;path/filepath&quot;
+	&quot;strings&quot;
+	&quot;syscall&quot;
+	&quot;time&quot;
+
+	&quot;dch/internal/common&quot;
+	&quot;dch/internal/githubapp&quot;
+)
+
+const githubAppID = &quot;github-app&quot;
+
+type Status struct {
+	Enabled       bool
+	PendingBuild  bool
+	ServiceActive bool
+	Message       string
+}
+
+type Manager struct{}
+
+func NewManager() *Manager { return &amp;Manager{} }
+
+func (m *Manager) Enable(ctx context.Context, projectPath string, cfg githubapp.Config) error {
+	if err := cfg.Validate(); err != nil {
+		return err
+	}
+	path, err := common.NormalizePath(projectPath)
+	if err != nil {
+		return err
+	}
+	unlock, err := lockConfig()
+	if err != nil {
+		return err
+	}
+	defer unlock()
+
+	config, err := common.LoadConfig()
+	if err != nil {
+		return err
+	}
+	old, hadOld := config.ProjectPlugins[path]
+	for id := range old.Enabled {
+		if id != githubAppID {
+			return fmt.Errorf(&quot;unknown enabled plugin %q&quot;, id)
+		}
+	}
+	_, wasEnabled := old.Enabled[githubAppID]
+	wasActive, _ := m.serviceActive(ctx, path)
+	fragment, runtimeDir, err := renderFragment(path)
+	if err != nil {
+		return err
+	}
+	fragmentPath := managedFragment(path)
+	oldBytes, oldFileExists, err := readOwnedFile(fragmentPath, old.States[githubAppID].FragmentDigest)
+	if err != nil {
+		return err
+	}
+	if !oldFileExists {
+		if _, err := os.Lstat(fragmentPath); err == nil {
+			return fmt.Errorf(&quot;plugin fragment already exists and is not managed by dch&quot;)
+		} else if !errors.Is(err, os.ErrNotExist) {
+			return err
+		}
+	}
+	if err := os.MkdirAll(filepath.Dir(fragmentPath), 0700); err != nil {
+		return err
+	}
+	if err := writePrivateFile(fragmentPath, fragment); err != nil {
+		return err
+	}
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
+	if err := common.SaveConfig(config); err != nil {
+		cleanupErr := restoreFile(fragmentPath, oldBytes, oldFileExists)
+		cleanupErr = errors.Join(cleanupErr, unit.restore())
+		return errors.Join(err, cleanupErr)
+	}
+
+	serviceTouched := false
+	rollback := func(cause error) error {
+		var cleanup []error
+		if serviceTouched {
+			for _, action := range [][]string{{&quot;stop&quot;, unitName(path)}, {&quot;disable&quot;, unitName(path)}, {&quot;daemon-reload&quot;}} {
+				if err := runSystemctl(context.Background(), action...); err != nil {
+					cleanup = append(cleanup, err)
+				}
+			}
+		}
+		if len(cleanup) == 0 {
+			if err := unit.restore(); err != nil {
+				cleanup = append(cleanup, err)
+			}
+		}
+		if err := os.Remove(filepath.Join(runtimeDir, &quot;credential.sock&quot;)); err != nil &amp;&amp; !errors.Is(err, os.ErrNotExist) {
+			cleanup = append(cleanup, err)
+		}
+		if len(cleanup) &gt; 0 {
+			return fmt.Errorf(&quot;%w; cleanup failed, settings remain enabled for retry: %w&quot;, cause, errors.Join(cleanup...))
+		}
+		if err := restoreFile(fragmentPath, oldBytes, oldFileExists); err != nil {
+			cleanup = append(cleanup, err)
+		}
+		current, loadErr := common.LoadConfig()
+		if loadErr != nil {
+			cleanup = append(cleanup, loadErr)
+		} else {
+			if hadOld {
+				current.ProjectPlugins[path] = old
+			} else {
+				delete(current.ProjectPlugins, path)
+			}
+			if err := common.SaveConfig(current); err != nil {
+				cleanup = append(cleanup, err)
+			}
+		}
+		if wasActive &amp;&amp; wasEnabled {
+			if err := m.startAndCheck(context.Background(), path); err != nil {
+				cleanup = append(cleanup, err)
+			}
+		}
+		if len(cleanup) &gt; 0 {
+			return fmt.Errorf(&quot;%w; rollback incomplete: %w&quot;, cause, errors.Join(cleanup...))
+		}
+		return cause
+	}
+
+	if err := runSystemctl(ctx, &quot;daemon-reload&quot;); err != nil {
+		return rollback(err)
+	}
+	serviceTouched = true
+	if wasActive &amp;&amp; wasEnabled {
+		if err := runSystemctl(ctx, &quot;restart&quot;, unitName(path)); err != nil {
+			return rollback(err)
+		}
+	} else if err := runSystemctl(ctx, &quot;enable&quot;, &quot;--now&quot;, unitName(path)); err != nil {
+		return rollback(err)
+	}
+	if err := m.checkReady(ctx, runtimeDir); err != nil {
+		return rollback(err)
+	}
+	return nil
+}
+
+func (m *Manager) Ensure(ctx context.Context, projectPath string) error {
+	path, err := common.NormalizePath(projectPath)
+	if err != nil {
+		return err
+	}
+	unlock, err := lockConfig()
+	if err != nil {
+		return err
+	}
+	defer unlock()
+	config, err := common.LoadConfig()
+	if err != nil {
+		return err
+	}
+	project, ok := config.ProjectPlugins[path]
+	if !ok {
+		return nil
+	}
+	for id := range project.Enabled {
+		if id != githubAppID {
+			return fmt.Errorf(&quot;unknown enabled plugin %q&quot;, id)
+		}
+	}
+	_, enabled := project.Enabled[githubAppID]
+	if !enabled {
+		return nil
+	}
+	if _, exists, err := readOwnedFile(managedFragment(path), project.States[githubAppID].FragmentDigest); err != nil {
+		return err
+	} else if !exists {
+		return fmt.Errorf(&quot;managed plugin fragment is missing&quot;)
+	}
+	_, runtimeDir, err := renderFragment(path)
+	if err != nil {
+		return err
+	}
+	if err := githubapp.WriteAdapterScripts(runtimeDir); err != nil {
+		return err
+	}
+	unit, err := m.prepareServiceUnit(path, project.States[githubAppID])
+	if err != nil {
+		return err
+	}
+	oldState := project.States[githubAppID]
+	state := oldState
+	state.UnitDigest = unit.digest
+	state.UnitPath = unit.path
+	metadataChanged := state != oldState
+	if metadataChanged {
+		project.States[githubAppID] = state
+		config.ProjectPlugins[path] = project
+		if err := common.SaveConfig(config); err != nil {
+			return errors.Join(err, unit.restore())
+		}
+	}
+	active, _ := m.serviceActive(ctx, path)
+	restoreUnit := func(cause error) error {
+		if !metadataChanged {
+			return cause
+		}
+		var restoreErr error
+		restoreErr = errors.Join(restoreErr, unit.restore())
+		project.States[githubAppID] = oldState
+		config.ProjectPlugins[path] = project
+		restoreErr = errors.Join(restoreErr, common.SaveConfig(config))
+		restoreErr = errors.Join(restoreErr, runSystemctl(context.Background(), &quot;daemon-reload&quot;))
+		if restoreErr != nil {
+			return fmt.Errorf(&quot;%w; previous unit restore failed: %w&quot;, cause, restoreErr)
+		}
+		return cause
+	}
+	if err := runSystemctl(ctx, &quot;daemon-reload&quot;); err != nil {
+		return restoreUnit(err)
+	}
+	if active &amp;&amp; unit.changed {
+		if err := runSystemctl(ctx, &quot;restart&quot;, unitName(path)); err != nil {
+			return restoreUnit(err)
+		}
+	} else if err := runSystemctl(ctx, &quot;enable&quot;, &quot;--now&quot;, unitName(path)); err != nil {
+		return restoreUnit(err)
+	}
+	return m.checkReady(ctx, runtimeDir)
+}
+
+func (m *Manager) Status(ctx context.Context, projectPath string) (Status, error) {
+	path, err := common.NormalizePath(projectPath)
+	if err != nil {
+		return Status{}, err
+	}
+	unlock, err := lockConfig()
+	if err != nil {
+		return Status{}, err
+	}
+	defer unlock()
+	config, err := common.LoadConfig()
+	if err != nil {
+		return Status{}, err
+	}
+	project, ok := config.ProjectPlugins[path]
+	if !ok {
+		return Status{}, nil
+	}
+	for id := range project.Enabled {
+		if id != githubAppID {
+			return Status{}, fmt.Errorf(&quot;unknown enabled plugin %q&quot;, id)
+		}
+	}
+	_, enabled := project.Enabled[githubAppID]
+	active, activeErr := m.serviceActive(ctx, path)
+	status := Status{Enabled: enabled, PendingBuild: project.States[githubAppID].BuildPending, ServiceActive: active}
+	if activeErr != nil {
+		status.Message = activeErr.Error()
+	}
+	return status, nil
+}
+
+func (m *Manager) MarkBuilt(projectPath string) error {
+	path, err := common.NormalizePath(projectPath)
+	if err != nil {
+		return err
+	}
+	unlock, err := lockConfig()
+	if err != nil {
+		return err
+	}
+	defer unlock()
+	config, err := common.LoadConfig()
+	if err != nil {
+		return err
+	}
+	project, ok := config.ProjectPlugins[path]
+	if !ok {
+		return nil
+	}
+	for id := range project.Enabled {
+		if id != githubAppID {
+			return fmt.Errorf(&quot;unknown enabled plugin %q&quot;, id)
+		}
+	}
+	state := project.States[githubAppID]
+	state.BuildPending = false
+	project.States[githubAppID] = state
+	config.ProjectPlugins[path] = project
+	return common.SaveConfig(config)
+}
+
+func (m *Manager) Disable(ctx context.Context, projectPath string) error {
+	path, err := common.NormalizePath(projectPath)
+	if err != nil {
+		return err
+	}
+	unlock, err := lockConfig()
+	if err != nil {
+		return err
+	}
+	defer unlock()
+	config, err := common.LoadConfig()
+	if err != nil {
+		return err
+	}
+	project, ok := config.ProjectPlugins[path]
+	if !ok {
+		return nil
+	}
+	for id := range project.Enabled {
+		if id != githubAppID {
+			return fmt.Errorf(&quot;unknown enabled plugin %q&quot;, id)
+		}
+	}
+	if _, enabled := project.Enabled[githubAppID]; !enabled {
+		return nil
+	}
+	unitFile := project.States[githubAppID].UnitPath
+	if unitFile == &quot;&quot; {
+		unitFile, err = unitPath(path)
+		if err != nil {
+			return err
+		}
+	} else if err := validateUnitPath(unitFile, path); err != nil {
+		return err
+	}
+	if info, err := os.Lstat(unitFile); err == nil {
+		if !info.Mode().IsRegular() || !ownedByCurrentUser(info) {
+			return fmt.Errorf(&quot;systemd unit is unsafe&quot;)
+		}
+		data, err := os.ReadFile(unitFile)
+		if err != nil {
+			return err
+		}
+		if project.States[githubAppID].UnitDigest == &quot;&quot; || digest(data) != project.States[githubAppID].UnitDigest {
+			return fmt.Errorf(&quot;systemd unit was modified; refusing to remove it&quot;)
+		}
+	} else if !errors.Is(err, os.ErrNotExist) {
+		return err
+	}
+	if _, _, err := readOwnedFile(managedFragment(path), project.States[githubAppID].FragmentDigest); err != nil {
+		return err
+	}
+	if err := runSystemctl(ctx, &quot;stop&quot;, unitName(path)); err != nil {
+		return err
+	}
+	if err := runSystemctl(ctx, &quot;disable&quot;, unitName(path)); err != nil {
+		return err
+	}
+	if err := os.Remove(unitFile); err != nil &amp;&amp; !errors.Is(err, os.ErrNotExist) {
+		return err
+	}
+	if err := runSystemctl(ctx, &quot;daemon-reload&quot;); err != nil {
+		return err
+	}
+	fragmentPath := managedFragment(path)
+	if _, exists, err := readOwnedFile(fragmentPath, project.States[githubAppID].FragmentDigest); err != nil {
+		return err
+	} else if exists {
+		if err := os.Remove(fragmentPath); err != nil {
+			return err
+		}
+	}
+	runtimeDir, err := githubapp.RuntimeDir(path)
+	if err != nil {
+		return err
+	}
+	if err := os.RemoveAll(runtimeDir); err != nil {
+		return err
+	}
+	delete(project.Enabled, githubAppID)
+	state := project.States[githubAppID]
+	state.FragmentDigest = &quot;&quot;
+	state.UnitDigest = &quot;&quot;
+	state.UnitPath = &quot;&quot;
+	state.BuildFingerprint = &quot;&quot;
+	state.BuildPending = true
+	project.States[githubAppID] = state
+	config.ProjectPlugins[path] = project
+	return common.SaveConfig(config)
+}
+
+func (m *Manager) ServeProject(ctx context.Context, projectPath string) error {
+	path, err := common.NormalizePath(projectPath)
+	if err != nil {
+		return err
+	}
+	config, err := common.LoadConfig()
+	if err != nil {
+		return err
+	}
+	project, ok := config.ProjectPlugins[path]
+	if !ok {
+		return fmt.Errorf(&quot;github-app is not enabled for this project&quot;)
+	}
+	for id := range project.Enabled {
+		if id != githubAppID {
+			return fmt.Errorf(&quot;unknown enabled plugin %q&quot;, id)
+		}
+	}
+	cfg, ok := project.Enabled[githubAppID]
+	if !ok {
+		return fmt.Errorf(&quot;github-app is not enabled for this project&quot;)
+	}
+	runtimeDir, err := githubapp.RuntimeDir(path)
+	if err != nil {
+		return err
+	}
+	return githubapp.Serve(ctx, filepath.Join(runtimeDir, &quot;credential.sock&quot;), cfg)
+}
+
+func renderFragment(path string) ([]byte, string, error) {
+	runtimeDir, err := githubapp.RuntimeDir(path)
+	if err != nil {
+		return nil, &quot;&quot;, err
+	}
+	fragment, err := githubapp.AdapterFragment(runtimeDir)
+	return fragment, runtimeDir, err
+}
+
+func (m *Manager) checkReady(ctx context.Context, runtimeDir string) error {
+	deadline := time.NewTimer(5 * time.Second)
+	defer deadline.Stop()
+	tick := time.NewTicker(100 * time.Millisecond)
+	defer tick.Stop()
+	socketPath := filepath.Join(runtimeDir, &quot;credential.sock&quot;)
+	for {
+		conn, err := net.DialTimeout(&quot;unix&quot;, socketPath, 100*time.Millisecond)
+		if err == nil {
+			_ = conn.Close()
+			break
+		}
+		select {
+		case &lt;-ctx.Done():
+			return ctx.Err()
+		case &lt;-deadline.C:
+			return fmt.Errorf(&quot;credential service socket did not become ready&quot;)
+		case &lt;-tick.C:
+		}
+	}
+	if err := githubapp.CheckCredential(ctx, socketPath); err != nil {
+		return fmt.Errorf(&quot;credential service validation failed: %w&quot;, err)
+	}
+	return nil
+}
+
+func (m *Manager) serviceActive(ctx context.Context, path string) (bool, error) {
+	err := runSystemctl(ctx, &quot;is-active&quot;, &quot;--quiet&quot;, unitName(path))
+	if err == nil {
+		return true, nil
+	}
+	return false, err
+}
+
+func (m *Manager) startAndCheck(ctx context.Context, path string) error {
+	_, runtimeDir, err := renderFragment(path)
+	if err != nil {
+		return err
+	}
+	if err := runSystemctl(ctx, &quot;daemon-reload&quot;); err != nil {
+		return err
+	}
+	if err := runSystemctl(ctx, &quot;enable&quot;, &quot;--now&quot;, unitName(path)); err != nil {
+		return err
+	}
+	return m.checkReady(ctx, runtimeDir)
+}
+
+type serviceUnitChange struct {
+	path    string
+	digest  string
+	changed bool
+	before  []byte
+	existed bool
+}
+
+func (change serviceUnitChange) restore() error {
+	if change.existed {
+		return writePrivateFile(change.path, change.before)
+	}
+	if err := os.Remove(change.path); err != nil &amp;&amp; !errors.Is(err, os.ErrNotExist) {
+		return err
+	}
+	return nil
+}
+
+func (m *Manager) prepareServiceUnit(projectPath string, previous common.PluginState) (serviceUnitChange, error) {
+	var change serviceUnitChange
+	unit, err := unitPath(projectPath)
+	if previous.UnitPath != &quot;&quot; {
+		unit = previous.UnitPath
+		if err := validateUnitPath(unit, projectPath); err != nil {
+			return change, err
+		}
+	} else if err != nil {
+		return change, err
+	}
+	content, err := m.serviceUnitContent(projectPath)
+	if err != nil {
+		return change, err
+	}
+	change = serviceUnitChange{path: unit, digest: digest(content)}
+	if info, err := os.Lstat(unit); err == nil {
+		if !info.Mode().IsRegular() || !ownedByCurrentUser(info) {
+			return change, fmt.Errorf(&quot;systemd unit is unsafe&quot;)
+		}
+		existing, err := os.ReadFile(unit)
+		if err != nil {
+			return change, err
+		}
+		if previous.UnitDigest == &quot;&quot; || digest(existing) != previous.UnitDigest {
+			return change, fmt.Errorf(&quot;systemd unit was modified; refusing to replace it&quot;)
+		}
+		change.before = existing
+		change.existed = true
+		change.changed = !bytes.Equal(existing, content)
+		if change.changed {
+			if err := writePrivateFile(unit, content); err != nil {
+				return change, err
+			}
+		}
+	} else if errors.Is(err, os.ErrNotExist) {
+		if err := os.MkdirAll(filepath.Dir(unit), 0700); err != nil {
+			return change, err
+		}
+		if err := writePrivateFile(unit, content); err != nil {
+			return change, err
+		}
+		change.changed = true
+	} else {
+		return change, err
+	}
+	return change, nil
+}
+
+func validateUnitPath(path, projectPath string) error {
+	if !filepath.IsAbs(path) || filepath.Clean(path) != path || filepath.Base(path) != unitName(projectPath) || filepath.Base(filepath.Dir(path)) != &quot;user&quot; || filepath.Base(filepath.Dir(filepath.Dir(path))) != &quot;systemd&quot; {
+		return fmt.Errorf(&quot;stored systemd unit path is invalid&quot;)
+	}
+	return nil
+}
+
+func (m *Manager) serviceUnitContent(projectPath string) ([]byte, error) {
+	configPath, err := common.ConfigPath()
+	if err != nil {
+		return nil, err
+	}
+	bin, err := os.Executable()
+	if err != nil {
+		return nil, err
+	}
+	content := &quot;[Unit]\nDescription=dch GitHub App credential service\n\n[Service]\nType=simple\nExecStart=&quot; + systemdExecArg(bin) + &quot; _github-app-serve --project &quot; + systemdExecArg(projectPath) + &quot;\nRestart=on-failure\n&quot;
+	for _, key := range []string{&quot;DCH_CONFIG&quot;, &quot;XDG_RUNTIME_DIR&quot;, &quot;PATH&quot;, &quot;GNUPGHOME&quot;, &quot;PASSWORD_STORE_DIR&quot;} {
+		value := os.Getenv(key)
+		if key == &quot;DCH_CONFIG&quot; {
+			value = configPath
+		}
+		if value != &quot;&quot; {
+			content += &quot;Environment=&quot; + systemdEnv(key, value) + &quot;\n&quot;
+		}
+	}
+	content += &quot;\n[Install]\nWantedBy=default.target\n&quot;
+	return []byte(content), nil
+}
+
+func ownedByCurrentUser(info os.FileInfo) bool {
+	stat, ok := info.Sys().(*syscall.Stat_t)
+	return ok &amp;&amp; int(stat.Uid) == os.Getuid()
+}
+
+func runSystemctl(ctx context.Context, args ...string) error {
+	args = append([]string{&quot;--user&quot;}, args...)
+	commandCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
+	defer cancel()
+	cmd := exec.CommandContext(commandCtx, &quot;systemctl&quot;, args...)
+	var output limitedOutput
+	cmd.Stdout = &amp;output
+	cmd.Stderr = &amp;output
+	if err := cmd.Run(); err != nil {
+		return fmt.Errorf(&quot;systemctl %s failed: %s&quot;, args[1], strings.TrimSpace(output.String()))
+	}
+	return nil
+}
+
+type limitedOutput struct{ buffer bytes.Buffer }
+
+func (output *limitedOutput) Write(data []byte) (int, error) {
+	const limit = 4096
+	originalLength := len(data)
+	remaining := limit - output.buffer.Len()
+	if remaining &gt; 0 {
+		if len(data) &gt; remaining {
+			data = data[:remaining]
+		}
+		_, _ = output.buffer.Write(data)
+	}
+	return originalLength, nil
+}
+func (output *limitedOutput) String() string { return output.buffer.String() }
+
+func systemdArg(s string) string {
+	return &quot;\&quot;&quot; + strings.NewReplacer(&quot;\\&quot;, &quot;\\\\&quot;, &quot;\&quot;&quot;, &quot;\\\&quot;&quot;, &quot;\n&quot;, &quot;\\n&quot;, &quot;\r&quot;, &quot;\\r&quot;, &quot;%&quot;, &quot;%%&quot;).Replace(s) + &quot;\&quot;&quot;
+}
+func systemdExecArg(s string) string      { return systemdArg(strings.ReplaceAll(s, &quot;$&quot;, &quot;$$&quot;)) }
+func systemdEnv(key, value string) string { return systemdArg(key + &quot;=&quot; + value) }
+
+func unitName(path string) string { return &quot;dch-github-app-&quot; + digest([]byte(path))[:16] + &quot;.service&quot; }
+func unitPath(path string) (string, error) {
+	config, err := os.UserConfigDir()
+	if err != nil {
+		return &quot;&quot;, err
+	}
+	if !filepath.IsAbs(config) {
+		return &quot;&quot;, fmt.Errorf(&quot;user config directory must be absolute&quot;)
+	}
+	return filepath.Join(config, &quot;systemd&quot;, &quot;user&quot;, unitName(path)), nil
+}
+func managedFragment(path string) string {
+	return filepath.Join(path, &quot;.devcontainer&quot;, &quot;devcontainer.local.d&quot;, &quot;90-dch-github-app.json&quot;)
+}
+func digest(data []byte) string { sum := sha256.Sum256(data); return hex.EncodeToString(sum[:]) }
+
+func buildFingerprint(config githubapp.Config, fragmentDigest string) string {
+	data, _ := json.Marshal(struct {
+		Config         githubapp.Config `json:&quot;config&quot;`
+		FragmentDigest string           `json:&quot;fragment_digest&quot;`
+	}{config, fragmentDigest})
+	return digest(data)
+}
+
+func cloneProjectPlugins(project common.ProjectPlugins) common.ProjectPlugins {
+	clone := common.ProjectPlugins{Enabled: make(map[string]githubapp.Config, len(project.Enabled)), States: make(map[string]common.PluginState, len(project.States))}
+	for id, cfg := range project.Enabled {
+		permissions := make(map[string]string, len(cfg.Permissions))
+		for key, value := range cfg.Permissions {
+			permissions[key] = value
+		}
+		cfg.Permissions = permissions
+		clone.Enabled[id] = cfg
+	}
+	for id, state := range project.States {
+		clone.States[id] = state
+	}
+	return clone
+}
+
+func readOwnedFile(path, expected string) ([]byte, bool, error) {
+	info, err := os.Lstat(path)
+	if errors.Is(err, os.ErrNotExist) {
+		return nil, false, nil
+	}
+	if err != nil {
+		return nil, false, err
+	}
+	if !info.Mode().IsRegular() {
+		return nil, false, fmt.Errorf(&quot;plugin fragment is not a regular file&quot;)
+	}
+	data, err := os.ReadFile(path)
+	if errors.Is(err, os.ErrNotExist) {
+		return nil, false, nil
+	}
+	if err != nil {
+		return nil, false, err
+	}
+	if expected == &quot;&quot; || digest(data) != expected {
+		return nil, false, fmt.Errorf(&quot;plugin fragment was modified; refusing to overwrite or remove it&quot;)
+	}
+	return data, true, nil
+}
+
+func writePrivateFile(path string, data []byte) error {
+	tmp, err := os.CreateTemp(filepath.Dir(path), &quot;.dch-plugin-*&quot;)
+	if err != nil {
+		return err
+	}
+	defer os.Remove(tmp.Name())
+	if err := tmp.Chmod(0600); err != nil {
+		tmp.Close()
+		return err
+	}
+	if _, err := tmp.Write(data); err != nil {
+		tmp.Close()
+		return err
+	}
+	if err := tmp.Sync(); err != nil {
+		tmp.Close()
+		return err
+	}
+	if err := tmp.Close(); err != nil {
+		return err
+	}
+	return os.Rename(tmp.Name(), path)
+}
+
+func restoreFile(path string, data []byte, existed bool) error {
+	if existed {
+		return writePrivateFile(path, data)
+	}
+	if err := os.Remove(path); err != nil &amp;&amp; !errors.Is(err, os.ErrNotExist) {
+		return err
+	}
+	return nil
+}
+
+func lockConfig() (func(), error) {
+	path, err := common.ConfigPath()
+	if err != nil {
+		return nil, err
+	}
+	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
+		return nil, err
+	}
+	fd, err := syscall.Open(path+&quot;.plugins.lock&quot;, syscall.O_CREAT|syscall.O_RDWR|syscall.O_CLOEXEC|syscall.O_NOFOLLOW, 0600)
+	if err != nil {
+		return nil, err
+	}
+	file := os.NewFile(uintptr(fd), path+&quot;.plugins.lock&quot;)
+	info, err := file.Stat()
+	if err != nil || !info.Mode().IsRegular() || !ownedByCurrentUser(info) {
+		_ = file.Close()
+		return nil, fmt.Errorf(&quot;plugin config lock is unsafe&quot;)
+	}
+	if err := file.Chmod(0600); err != nil {
+		_ = file.Close()
+		return nil, err
+	}
+	if err := syscall.Flock(fd, syscall.LOCK_EX); err != nil {
+		_ = file.Close()
+		return nil, err
+	}
+	return func() { _ = syscall.Flock(fd, syscall.LOCK_UN); _ = file.Close() }, nil
+}
</code></pre><h4>internal/common/config_store.go</h4><pre style="white-space:pre-wrap;overflow-wrap:anywhere"><code>diff --git a/internal/common/config_store.go b/internal/common/config_store.go
index 891c8c8..9fad571 100644
--- a/internal/common/config_store.go
+++ b/internal/common/config_store.go
@@ -7,13 +7,34 @@ import (
 	&quot;os&quot;
 	&quot;path/filepath&quot;
 	&quot;strings&quot;
+
+	&quot;dch/internal/githubapp&quot;
 )
 
 type Config struct {
-	Version  int      `json:&quot;version&quot;`
-	Projects []string `json:&quot;projects&quot;`
+	Version        int                       `json:&quot;version&quot;`
+	Projects       []string                  `json:&quot;projects&quot;`
+	ProjectPlugins map[string]ProjectPlugins `json:&quot;project_plugins,omitempty&quot;`
+}
+
+// ProjectPlugins stores host-side configuration and ownership state for one project.
+type ProjectPlugins struct {
+	Enabled map[string]githubapp.Config `json:&quot;enabled,omitempty&quot;`
+	States  map[string]PluginState      `json:&quot;states,omitempty&quot;`
 }
 
+// PluginState tracks generated-file ownership and whether containers need a Build.
+type PluginState struct {
+	FragmentDigest   string `json:&quot;fragment_digest,omitempty&quot;`
+	UnitDigest       string `json:&quot;unit_digest,omitempty&quot;`
+	UnitPath         string `json:&quot;unit_path,omitempty&quot;`
+	BuildFingerprint string `json:&quot;build_fingerprint,omitempty&quot;`
+	BuildPending     bool   `json:&quot;build_pending,omitempty&quot;`
+}
+
+// ConfigPath returns the absolute path used to load and save the host configuration.
+func ConfigPath() (string, error) { return configPath() }
+
 func configPath() (string, error) {
 	if p := os.Getenv(&quot;DCH_CONFIG&quot;); p != &quot;&quot; {
 		return filepath.Abs(p)
</code></pre></div>
<div><h3>G3 新增：GitHub App provider</h3><p>確認單一 repository 與選定權限。先用 App JWT 驗證 installation，再申請 Token。</p><p>需你 review：是</p><label><input type="checkbox" id="check-G3"> 我已確認 G3 的決策與風險</label><p><strong>檔案與新版本 hunk 起始行</strong></p><ul><li>internal/githubapp/config.go:1</li><li>internal/githubapp/provider.go:1</li></ul></div><div id="review-G3"><p>版本 759b9d8；以下為完整檔案 diff（保留完整 hunk）。行號見 @@ 的 + 區段。</p><h4>internal/githubapp/config.go</h4><pre style="white-space:pre-wrap;overflow-wrap:anywhere"><code>diff --git a/internal/githubapp/config.go b/internal/githubapp/config.go
new file mode 100644
index 0000000..b1a5420
--- /dev/null
+++ b/internal/githubapp/config.go
@@ -0,0 +1,69 @@
+package githubapp
+
+import (
+	&quot;fmt&quot;
+	&quot;regexp&quot;
+	&quot;strconv&quot;
+	&quot;strings&quot;
+)
+
+type Config struct {
+	AppID               int64             `json:&quot;app_id&quot;`
+	InstallationID      int64             `json:&quot;installation_id&quot;`
+	PrivateKeyPassEntry string            `json:&quot;private_key_pass_entry&quot;`
+	Repository          string            `json:&quot;repository&quot;`
+	Permissions         map[string]string `json:&quot;permissions&quot;`
+}
+
+func (c Config) appIDString() string { return strconv.FormatInt(c.AppID, 10) }
+
+var passEntryPattern = regexp.MustCompile(`^[A-Za-z0-9_./-]+$`)
+
+func (c Config) Validate() error {
+	if c.AppID &lt;= 0 || c.InstallationID &lt;= 0 {
+		return fmt.Errorf(&quot;app and installation IDs must be positive&quot;)
+	}
+	if c.PrivateKeyPassEntry == &quot;&quot; || strings.HasPrefix(c.PrivateKeyPassEntry, &quot;-&quot;) || !passEntryPattern.MatchString(c.PrivateKeyPassEntry) || strings.Contains(c.PrivateKeyPassEntry, &quot;..&quot;) {
+		return fmt.Errorf(&quot;invalid pass entry&quot;)
+	}
+	parts := strings.Split(c.Repository, &quot;/&quot;)
+	if len(parts) != 2 || !validOwner(parts[0]) || !validRepository(parts[1]) {
+		return fmt.Errorf(&quot;repository must be owner/repository&quot;)
+	}
+	if len(c.Permissions) == 0 {
+		return fmt.Errorf(&quot;at least one permission is required&quot;)
+	}
+	for name, level := range c.Permissions {
+		if name != &quot;contents&quot; &amp;&amp; name != &quot;pull_requests&quot; {
+			return fmt.Errorf(&quot;unsupported permission&quot;)
+		}
+		if level != &quot;read&quot; &amp;&amp; level != &quot;write&quot; {
+			return fmt.Errorf(&quot;invalid permission level&quot;)
+		}
+	}
+	return nil
+}
+
+func validOwner(name string) bool {
+	if name == &quot;&quot; || len(name) &gt; 39 || name[0] == &#x27;-&#x27; || strings.HasSuffix(name, &quot;-&quot;) || strings.Contains(name, &quot;--&quot;) {
+		return false
+	}
+	for _, r := range name {
+		if !(r &gt;= &#x27;a&#x27; &amp;&amp; r &lt;= &#x27;z&#x27; || r &gt;= &#x27;A&#x27; &amp;&amp; r &lt;= &#x27;Z&#x27; || r &gt;= &#x27;0&#x27; &amp;&amp; r &lt;= &#x27;9&#x27; || r == &#x27;-&#x27;) {
+			return false
+		}
+	}
+	return true
+}
+
+func validRepository(name string) bool {
+	if name == &quot;&quot; || len(name) &gt; 100 || name == &quot;.&quot; || name == &quot;..&quot; || strings.HasSuffix(name, &quot;.git&quot;) || strings.HasPrefix(name, &quot;.&quot;) || strings.HasSuffix(name, &quot;.&quot;) {
+		return false
+	}
+	for _, r := range name {
+		if !(r &gt;= &#x27;a&#x27; &amp;&amp; r &lt;= &#x27;z&#x27; || r &gt;= &#x27;A&#x27; &amp;&amp; r &lt;= &#x27;Z&#x27; || r &gt;= &#x27;0&#x27; &amp;&amp; r &lt;= &#x27;9&#x27; || r == &#x27;-&#x27; || r == &#x27;_&#x27; || r == &#x27;.&#x27;) {
+			return false
+		}
+	}
+	return true
+}
</code></pre><h4>internal/githubapp/provider.go</h4><pre style="white-space:pre-wrap;overflow-wrap:anywhere"><code>diff --git a/internal/githubapp/provider.go b/internal/githubapp/provider.go
new file mode 100644
index 0000000..bd174b0
--- /dev/null
+++ b/internal/githubapp/provider.go
@@ -0,0 +1,265 @@
+package githubapp
+
+import (
+	&quot;bytes&quot;
+	&quot;context&quot;
+	&quot;crypto&quot;
+	&quot;crypto/rand&quot;
+	&quot;crypto/rsa&quot;
+	&quot;crypto/sha256&quot;
+	&quot;crypto/x509&quot;
+	&quot;encoding/base64&quot;
+	&quot;encoding/json&quot;
+	&quot;encoding/pem&quot;
+	&quot;fmt&quot;
+	&quot;io&quot;
+	&quot;net/http&quot;
+	&quot;net/url&quot;
+	&quot;os&quot;
+	&quot;os/exec&quot;
+	&quot;strings&quot;
+	&quot;sync&quot;
+	&quot;time&quot;
+)
+
+const maxResponseBytes = 1 &lt;&lt; 20
+
+type ProviderOptions struct {
+	PassPath       string
+	PassTimeout    time.Duration
+	RequestTimeout time.Duration
+	HTTPClient     *http.Client
+	APIBaseURL     string
+	Now            func() time.Time
+}
+
+type Provider struct {
+	cfg        Config
+	opts       ProviderOptions
+	mu         sync.Mutex
+	refreshing *refreshCall
+	token      string
+	expires    time.Time
+}
+
+type refreshCall struct {
+	done  chan struct{}
+	token string
+	err   error
+}
+
+func NewProvider(cfg Config, options ...ProviderOptions) (*Provider, error) {
+	if err := cfg.Validate(); err != nil {
+		return nil, err
+	}
+	if len(options) &gt; 1 {
+		return nil, fmt.Errorf(&quot;only one provider options value is supported&quot;)
+	}
+	opts := ProviderOptions{}
+	if len(options) == 1 {
+		opts = options[0]
+	}
+	if opts.PassPath == &quot;&quot; {
+		opts.PassPath = &quot;pass&quot;
+	}
+	if opts.PassTimeout &lt;= 0 {
+		opts.PassTimeout = 10 * time.Second
+	}
+	if opts.RequestTimeout &lt;= 0 {
+		opts.RequestTimeout = 15 * time.Second
+	}
+	if opts.APIBaseURL == &quot;&quot; {
+		opts.APIBaseURL = &quot;https://api.github.com&quot;
+	}
+	if opts.Now == nil {
+		opts.Now = time.Now
+	}
+	if opts.HTTPClient == nil {
+		opts.HTTPClient = &amp;http.Client{Timeout: opts.RequestTimeout, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
+	} else {
+		client := *opts.HTTPClient
+		client.Timeout = opts.RequestTimeout
+		client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
+		opts.HTTPClient = &amp;client
+	}
+	u, err := url.Parse(opts.APIBaseURL)
+	if err != nil || u.Scheme != &quot;https&quot; &amp;&amp; opts.APIBaseURL != &quot;http://127.0.0.1&quot; &amp;&amp; !strings.HasPrefix(opts.APIBaseURL, &quot;http://127.0.0.1:&quot;) {
+		return nil, fmt.Errorf(&quot;GitHub API URL must use HTTPS&quot;)
+	}
+	return &amp;Provider{cfg: cfg, opts: opts}, nil
+}
+
+func (p *Provider) Token(ctx context.Context) (string, error) {
+	for {
+		p.mu.Lock()
+		if p.token != &quot;&quot; &amp;&amp; p.opts.Now().Add(5*time.Minute).Before(p.expires) {
+			token := p.token
+			p.mu.Unlock()
+			return token, nil
+		}
+		if call := p.refreshing; call != nil {
+			p.mu.Unlock()
+			select {
+			case &lt;-call.done:
+				if call.err != nil {
+					return &quot;&quot;, call.err
+				}
+				return call.token, nil
+			case &lt;-ctx.Done():
+				return &quot;&quot;, ctx.Err()
+			}
+		}
+		call := &amp;refreshCall{done: make(chan struct{})}
+		p.refreshing = call
+		p.mu.Unlock()
+		token, expiry, err := p.mint(ctx)
+		p.mu.Lock()
+		if err == nil {
+			p.token, p.expires = token, expiry
+		}
+		if err != nil {
+			call.err = err
+		} else {
+			call.token = token
+		}
+		p.refreshing = nil
+		close(call.done)
+		p.mu.Unlock()
+		if err != nil {
+			return &quot;&quot;, err
+		}
+		return token, nil
+	}
+}
+
+func (p *Provider) mint(ctx context.Context) (string, time.Time, error) {
+	passCtx, cancel := context.WithTimeout(ctx, p.opts.PassTimeout)
+	defer cancel()
+	cmd := exec.CommandContext(passCtx, p.opts.PassPath, &quot;show&quot;, p.cfg.PrivateKeyPassEntry)
+	cmd.Stdin = nil
+	cmd.Stdout = nil
+	cmd.Stderr = io.Discard
+	cmd.Env = securePassEnvironment(os.Environ())
+	var keyOutput boundedBuffer
+	cmd.Stdout = &amp;keyOutput
+	if err := cmd.Run(); err != nil || keyOutput.overflow {
+		return &quot;&quot;, time.Time{}, fmt.Errorf(&quot;pass failed&quot;)
+	}
+	keyPEM := keyOutput.Bytes()
+	key, err := parsePrivateKey(keyPEM)
+	if err != nil {
+		return &quot;&quot;, time.Time{}, err
+	}
+	now := p.opts.Now()
+	issued := now.Add(-60 * time.Second)
+	claims, _ := json.Marshal(map[string]any{&quot;iat&quot;: issued.Unix(), &quot;exp&quot;: now.Add(9 * time.Minute).Unix(), &quot;iss&quot;: p.cfg.appIDString()})
+	header := base64.RawURLEncoding.EncodeToString([]byte(`{&quot;alg&quot;:&quot;RS256&quot;,&quot;typ&quot;:&quot;JWT&quot;}`))
+	unsigned := header + &quot;.&quot; + base64.RawURLEncoding.EncodeToString(claims)
+	digest := sha256.Sum256([]byte(unsigned))
+	signature, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, digest[:])
+	if err != nil {
+		return &quot;&quot;, time.Time{}, err
+	}
+	jwt := unsigned + &quot;.&quot; + base64.RawURLEncoding.EncodeToString(signature)
+	owner, repo, _ := strings.Cut(p.cfg.Repository, &quot;/&quot;)
+	verifyURL := strings.TrimRight(p.opts.APIBaseURL, &quot;/&quot;) + &quot;/repos/&quot; + url.PathEscape(owner) + &quot;/&quot; + url.PathEscape(repo) + &quot;/installation&quot;
+	verifyReq, _ := http.NewRequestWithContext(ctx, http.MethodGet, verifyURL, nil)
+	verifyReq.Header.Set(&quot;Authorization&quot;, &quot;Bearer &quot;+jwt)
+	verifyReq.Header.Set(&quot;Accept&quot;, &quot;application/vnd.github+json&quot;)
+	verifyReq.Header.Set(&quot;X-GitHub-Api-Version&quot;, &quot;2022-11-28&quot;)
+	verifyResp, err := p.opts.HTTPClient.Do(verifyReq)
+	if err != nil {
+		return &quot;&quot;, time.Time{}, fmt.Errorf(&quot;repository installation verification failed&quot;)
+	}
+	defer verifyResp.Body.Close()
+	verifyData, err := io.ReadAll(io.LimitReader(verifyResp.Body, maxResponseBytes+1))
+	var installation struct {
+		ID int64 `json:&quot;id&quot;`
+	}
+	if err != nil || len(verifyData) &gt; maxResponseBytes || verifyResp.StatusCode != http.StatusOK || json.Unmarshal(verifyData, &amp;installation) != nil || installation.ID != p.cfg.InstallationID {
+		return &quot;&quot;, time.Time{}, fmt.Errorf(&quot;repository installation verification failed&quot;)
+	}
+	var body bytes.Buffer
+	permissions := make(map[string]string, len(p.cfg.Permissions))
+	for k, v := range p.cfg.Permissions {
+		permissions[k] = v
+	}
+	if err := json.NewEncoder(&amp;body).Encode(map[string]any{&quot;repositories&quot;: []string{repo}, &quot;permissions&quot;: permissions}); err != nil {
+		return &quot;&quot;, time.Time{}, err
+	}
+	u := strings.TrimRight(p.opts.APIBaseURL, &quot;/&quot;) + fmt.Sprintf(&quot;/app/installations/%d/access_tokens&quot;, p.cfg.InstallationID)
+	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, u, &amp;body)
+	req.Header.Set(&quot;Authorization&quot;, &quot;Bearer &quot;+jwt)
+	req.Header.Set(&quot;Accept&quot;, &quot;application/vnd.github+json&quot;)
+	req.Header.Set(&quot;X-GitHub-Api-Version&quot;, &quot;2022-11-28&quot;)
+	req.Header.Set(&quot;Content-Type&quot;, &quot;application/json&quot;)
+	resp, err := p.opts.HTTPClient.Do(req)
+	if err != nil {
+		return &quot;&quot;, time.Time{}, fmt.Errorf(&quot;GitHub token request failed&quot;)
+	}
+	defer resp.Body.Close()
+	data, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
+	if err != nil || len(data) &gt; maxResponseBytes || resp.StatusCode &lt; 200 || resp.StatusCode &gt;= 300 {
+		return &quot;&quot;, time.Time{}, fmt.Errorf(&quot;GitHub token request failed&quot;)
+	}
+	var minted struct {
+		Token     string `json:&quot;token&quot;`
+		ExpiresAt string `json:&quot;expires_at&quot;`
+	}
+	if json.Unmarshal(data, &amp;minted) != nil || minted.Token == &quot;&quot; || strings.ContainsAny(minted.Token, &quot;\r\n&quot;) {
+		return &quot;&quot;, time.Time{}, fmt.Errorf(&quot;invalid GitHub token response&quot;)
+	}
+	expires, err := time.Parse(time.RFC3339, minted.ExpiresAt)
+	if err != nil || !expires.After(p.opts.Now().Add(5*time.Minute)) {
+		return &quot;&quot;, time.Time{}, fmt.Errorf(&quot;invalid GitHub token expiry&quot;)
+	}
+	return minted.Token, expires, nil
+}
+
+func parsePrivateKey(data []byte) (*rsa.PrivateKey, error) {
+	block, _ := pem.Decode(data)
+	if block == nil {
+		return nil, fmt.Errorf(&quot;invalid RSA private key&quot;)
+	}
+	if key, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
+		return key, nil
+	}
+	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
+	if err != nil {
+		return nil, fmt.Errorf(&quot;invalid RSA private key&quot;)
+	}
+	key, ok := parsed.(*rsa.PrivateKey)
+	if !ok {
+		return nil, fmt.Errorf(&quot;invalid RSA private key&quot;)
+	}
+	return key, nil
+}
+
+type boundedBuffer struct {
+	bytes.Buffer
+	overflow bool
+}
+
+func securePassEnvironment(env []string) []string {
+	filtered := make([]string, 0, len(env)+2)
+	var gpgOptions string
+	for _, item := range env {
+		if strings.HasPrefix(item, &quot;GPG_TTY=&quot;) {
+			continue
+		}
+		if strings.HasPrefix(item, &quot;PASSWORD_STORE_GPG_OPTS=&quot;) {
+			gpgOptions = strings.TrimPrefix(item, &quot;PASSWORD_STORE_GPG_OPTS=&quot;)
+			continue
+		}
+		filtered = append(filtered, item)
+	}
+	return append(filtered, &quot;GPG_TTY=&quot;, &quot;PASSWORD_STORE_GPG_OPTS=&quot;+strings.TrimSpace(gpgOptions+&quot; --batch --pinentry-mode error&quot;))
+}
+
+func (b *boundedBuffer) Write(p []byte) (int, error) {
+	if b.Len()+len(p) &gt; maxResponseBytes {
+		b.overflow = true
+		return 0, fmt.Errorf(&quot;output too large&quot;)
+	}
+	return b.Buffer.Write(p)
+}
</code></pre></div>
<div><h3>G4 新增：Host Unix Socket</h3><p>確認信任邊界：可連 Socket 的程序可取 Token。目錄 0700、Socket 0600；不開 TCP。</p><p>需你 review：是</p><label><input type="checkbox" id="check-G4"> 我已確認 G4 的決策與風險</label><p><strong>檔案與新版本 hunk 起始行</strong></p><ul><li>internal/githubapp/runtime.go:1</li><li>internal/githubapp/server.go:1</li></ul></div><div id="review-G4"><p>版本 759b9d8；以下為完整檔案 diff（保留完整 hunk）。行號見 @@ 的 + 區段。</p><h4>internal/githubapp/runtime.go</h4><pre style="white-space:pre-wrap;overflow-wrap:anywhere"><code>diff --git a/internal/githubapp/runtime.go b/internal/githubapp/runtime.go
new file mode 100644
index 0000000..1ec1dc3
--- /dev/null
+++ b/internal/githubapp/runtime.go
@@ -0,0 +1,51 @@
+package githubapp
+
+import (
+	&quot;crypto/sha256&quot;
+	&quot;encoding/hex&quot;
+	&quot;fmt&quot;
+	&quot;os&quot;
+	&quot;path/filepath&quot;
+	&quot;syscall&quot;
+)
+
+func RuntimeDir(projectPath string) (string, error) {
+	runtime := os.Getenv(&quot;XDG_RUNTIME_DIR&quot;)
+	if runtime == &quot;&quot; {
+		return &quot;&quot;, fmt.Errorf(&quot;XDG_RUNTIME_DIR is required&quot;)
+	}
+	info, err := os.Lstat(runtime)
+	if err != nil || !info.IsDir() || info.Mode()&amp;os.ModeSymlink != 0 || !ownedByCurrentUser(info) || info.Mode().Perm()&amp;0077 != 0 {
+		return &quot;&quot;, fmt.Errorf(&quot;XDG_RUNTIME_DIR is not a private directory owned by the current user&quot;)
+	}
+	abs, err := filepath.Abs(projectPath)
+	if err != nil || projectPath == &quot;&quot; {
+		return &quot;&quot;, fmt.Errorf(&quot;invalid project path&quot;)
+	}
+	key := sha256.Sum256([]byte(filepath.Clean(abs)))
+	dir := filepath.Join(runtime, &quot;dch&quot;, hex.EncodeToString(key[:12]))
+	parent := filepath.Join(runtime, &quot;dch&quot;)
+	if err := ensurePrivateDir(parent); err != nil {
+		return &quot;&quot;, err
+	}
+	if err := ensurePrivateDir(dir); err != nil {
+		return &quot;&quot;, err
+	}
+	return dir, nil
+}
+
+func ensurePrivateDir(path string) error {
+	if err := os.Mkdir(path, 0700); err != nil &amp;&amp; !os.IsExist(err) {
+		return fmt.Errorf(&quot;cannot create runtime directory&quot;)
+	}
+	info, err := os.Lstat(path)
+	if err != nil || !info.IsDir() || info.Mode()&amp;os.ModeSymlink != 0 || !ownedByCurrentUser(info) || info.Mode().Perm() != 0700 {
+		return fmt.Errorf(&quot;runtime directory is unsafe&quot;)
+	}
+	return nil
+}
+
+func ownedByCurrentUser(info os.FileInfo) bool {
+	stat, ok := info.Sys().(*syscall.Stat_t)
+	return ok &amp;&amp; int(stat.Uid) == os.Getuid()
+}
</code></pre><h4>internal/githubapp/server.go</h4><pre style="white-space:pre-wrap;overflow-wrap:anywhere"><code>diff --git a/internal/githubapp/server.go b/internal/githubapp/server.go
new file mode 100644
index 0000000..e2cd8ea
--- /dev/null
+++ b/internal/githubapp/server.go
@@ -0,0 +1,145 @@
+package githubapp
+
+import (
+	&quot;context&quot;
+	&quot;fmt&quot;
+	&quot;io&quot;
+	&quot;net&quot;
+	&quot;net/http&quot;
+	&quot;os&quot;
+	&quot;path/filepath&quot;
+	&quot;strings&quot;
+	&quot;syscall&quot;
+	&quot;time&quot;
+)
+
+func Serve(ctx context.Context, socketPath string, cfg Config) error {
+	if err := cfg.Validate(); err != nil {
+		return err
+	}
+	if len(socketPath) &gt; 100 {
+		return fmt.Errorf(&quot;socket path is too long&quot;)
+	}
+	parentInfo, err := os.Lstat(filepath.Dir(socketPath))
+	if err != nil || !parentInfo.IsDir() || parentInfo.Mode()&amp;os.ModeSymlink != 0 || !ownedByCurrentUser(parentInfo) || parentInfo.Mode().Perm() != 0700 {
+		return fmt.Errorf(&quot;socket directory is unsafe&quot;)
+	}
+	releaseLock, err := acquireServerLock(socketPath + &quot;.lock&quot;)
+	if err != nil {
+		return err
+	}
+	defer releaseLock()
+	if info, err := os.Lstat(socketPath); err == nil {
+		if info.Mode()&amp;os.ModeSocket == 0 {
+			return fmt.Errorf(&quot;socket path is unsafe&quot;)
+		}
+		if err := os.Remove(socketPath); err != nil {
+			return fmt.Errorf(&quot;cannot remove stale socket&quot;)
+		}
+	} else if !os.IsNotExist(err) {
+		return err
+	}
+	listener, err := net.Listen(&quot;unix&quot;, socketPath)
+	if err != nil {
+		return fmt.Errorf(&quot;cannot listen on credential socket&quot;)
+	}
+	if err := os.Chmod(socketPath, 0600); err != nil {
+		listener.Close()
+		os.Remove(socketPath)
+		return fmt.Errorf(&quot;cannot secure credential socket&quot;)
+	}
+	provider, err := NewProvider(cfg)
+	if err != nil {
+		listener.Close()
+		os.Remove(socketPath)
+		return err
+	}
+	mux := http.NewServeMux()
+	mux.HandleFunc(&quot;/health&quot;, func(w http.ResponseWriter, r *http.Request) {
+		if r.Method != http.MethodGet {
+			http.Error(w, &quot;method not allowed&quot;, http.StatusMethodNotAllowed)
+			return
+		}
+		w.WriteHeader(http.StatusOK)
+		_, _ = io.WriteString(w, &quot;ok\n&quot;)
+	})
+	mux.HandleFunc(&quot;/credential&quot;, func(w http.ResponseWriter, r *http.Request) {
+		w.Header().Set(&quot;Cache-Control&quot;, &quot;no-store&quot;)
+		if r.Method != http.MethodGet {
+			http.Error(w, &quot;method not allowed&quot;, http.StatusMethodNotAllowed)
+			return
+		}
+		token, err := provider.Token(r.Context())
+		if err != nil {
+			http.Error(w, &quot;credential unavailable&quot;, http.StatusServiceUnavailable)
+			return
+		}
+		w.Header().Set(&quot;Content-Type&quot;, &quot;text/plain; charset=utf-8&quot;)
+		_, _ = io.WriteString(w, token)
+	})
+	server := &amp;http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
+	done := make(chan struct{})
+	go func() {
+		select {
+		case &lt;-ctx.Done():
+			_ = server.Close()
+		case &lt;-done:
+		}
+	}()
+	err = server.Serve(listener)
+	close(done)
+	_ = os.Remove(socketPath)
+	if err == http.ErrServerClosed {
+		return nil
+	}
+	return fmt.Errorf(&quot;credential server failed&quot;)
+}
+
+func acquireServerLock(path string) (func(), error) {
+	fd, err := syscall.Open(path, syscall.O_CREAT|syscall.O_RDWR|syscall.O_CLOEXEC|syscall.O_NOFOLLOW, 0600)
+	if err != nil {
+		return nil, fmt.Errorf(&quot;cannot open credential server lock&quot;)
+	}
+	file := os.NewFile(uintptr(fd), path)
+	info, err := file.Stat()
+	if err != nil || !info.Mode().IsRegular() || !ownedByCurrentUser(info) {
+		file.Close()
+		return nil, fmt.Errorf(&quot;credential server lock is unsafe&quot;)
+	}
+	if err := file.Chmod(0600); err != nil {
+		file.Close()
+		return nil, fmt.Errorf(&quot;cannot secure credential server lock&quot;)
+	}
+	if err := syscall.Flock(fd, syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
+		file.Close()
+		return nil, fmt.Errorf(&quot;credential server is already running&quot;)
+	}
+	return func() { _ = syscall.Flock(fd, syscall.LOCK_UN); _ = file.Close() }, nil
+}
+
+func CheckCredential(ctx context.Context, socketPath string) error {
+	client := unixHTTPClient(socketPath)
+	req, err := http.NewRequestWithContext(ctx, http.MethodGet, &quot;http://unix/credential&quot;, nil)
+	if err != nil {
+		return fmt.Errorf(&quot;cannot check credential service&quot;)
+	}
+	resp, err := client.Do(req)
+	if err != nil {
+		return fmt.Errorf(&quot;credential service unavailable&quot;)
+	}
+	defer resp.Body.Close()
+	if resp.StatusCode != http.StatusOK {
+		return fmt.Errorf(&quot;credential service rejected check&quot;)
+	}
+	data, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
+	if err != nil || len(data) == 0 || len(data) &gt; maxResponseBytes || strings.ContainsAny(string(data), &quot;\r\n&quot;) {
+		return fmt.Errorf(&quot;credential service returned invalid token&quot;)
+	}
+	return nil
+}
+
+func unixHTTPClient(path string) *http.Client {
+	return &amp;http.Client{Timeout: 20 * time.Second, Transport: &amp;http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
+		return (&amp;net.Dialer{}).DialContext(ctx, &quot;unix&quot;, path)
+	}}, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
+}
</code></pre></div>
<div><h3>G5 新增：Git／gh 容器接線</h3><p>確認唯讀掛載整個 runtime 目錄。HTTPS github.com 使用 helper；gh 子程序取得 GH_TOKEN。</p><p>需你 review：是</p><label><input type="checkbox" id="check-G5"> 我已確認 G5 的決策與風險</label><p><strong>檔案與新版本 hunk 起始行</strong></p><ul><li>internal/githubapp/adapter.go:1</li></ul></div><div id="review-G5"><p>版本 759b9d8；以下為完整檔案 diff（保留完整 hunk）。行號見 @@ 的 + 區段。</p><h4>internal/githubapp/adapter.go</h4><pre style="white-space:pre-wrap;overflow-wrap:anywhere"><code>diff --git a/internal/githubapp/adapter.go b/internal/githubapp/adapter.go
new file mode 100644
index 0000000..229d772
--- /dev/null
+++ b/internal/githubapp/adapter.go
@@ -0,0 +1,194 @@
+package githubapp
+
+import (
+	&quot;bytes&quot;
+	&quot;encoding/json&quot;
+	&quot;fmt&quot;
+	&quot;io/fs&quot;
+	&quot;os&quot;
+	&quot;path/filepath&quot;
+	&quot;strings&quot;
+)
+
+const containerSocketDir = &quot;/run/dch-github&quot;
+
+// AdapterFragment returns the devcontainer settings required by the GitHub App adapter.
+func AdapterFragment(socketDir string) ([]byte, error) {
+	if !filepath.IsAbs(socketDir) || socketDir == &quot;/&quot; || strings.ContainsAny(socketDir, &quot;,\r\n&quot;) || strings.Contains(socketDir, &quot;${&quot;) {
+		return nil, fmt.Errorf(&quot;invalid GitHub App runtime directory&quot;)
+	}
+	fragment := struct {
+		Mounts    []string          `json:&quot;mounts&quot;`
+		RemoteEnv map[string]string `json:&quot;remoteEnv&quot;`
+	}{
+		Mounts:    []string{&quot;source=&quot; + socketDir + &quot;,target=&quot; + containerSocketDir + &quot;,type=bind,readonly&quot;},
+		RemoteEnv: map[string]string{&quot;PATH&quot;: containerSocketDir + &quot;/bin:${containerEnv:PATH}&quot;},
+	}
+	var out bytes.Buffer
+	encoder := json.NewEncoder(&amp;out)
+	encoder.SetEscapeHTML(false)
+	if err := encoder.Encode(fragment); err != nil {
+		return nil, fmt.Errorf(&quot;encode GitHub App adapter fragment: %w&quot;, err)
+	}
+	return out.Bytes(), nil
+}
+
+// WriteAdapterScripts securely writes the non-secret container adapter scripts under socketDir/bin.
+func WriteAdapterScripts(socketDir string) error {
+	if err := validateRuntimeDir(socketDir); err != nil {
+		return err
+	}
+	binDir := filepath.Join(socketDir, &quot;bin&quot;)
+	if err := os.Mkdir(binDir, 0700); err != nil &amp;&amp; !os.IsExist(err) {
+		return fmt.Errorf(&quot;cannot create adapter scripts directory&quot;)
+	}
+	if err := validateOwnedDir(binDir, 0700); err != nil {
+		return err
+	}
+	for name, content := range adapterScripts() {
+		path := filepath.Join(binDir, name)
+		if err := writePrivateScript(path, []byte(content)); err != nil {
+			return fmt.Errorf(&quot;cannot install adapter script %s&quot;, name)
+		}
+	}
+	if err := writePrivateFile(filepath.Join(socketDir, &quot;gitconfig&quot;), []byte(adapterGitConfig), 0600); err != nil {
+		return fmt.Errorf(&quot;cannot install adapter Git config&quot;)
+	}
+	return nil
+}
+
+// AdapterInstallCommand returns the command run in the container after startup.
+func AdapterInstallCommand() []string {
+	return []string{&quot;/bin/sh&quot;, &quot;-c&quot;, adapterInstallScript}
+}
+
+func validateRuntimeDir(path string) error {
+	if !filepath.IsAbs(path) || path == &quot;/&quot; || strings.ContainsAny(path, &quot;,\r\n&quot;) || strings.Contains(path, &quot;${&quot;) {
+		return fmt.Errorf(&quot;invalid GitHub App runtime directory&quot;)
+	}
+	return validateOwnedDir(path, 0700)
+}
+
+func validateOwnedDir(path string, mode fs.FileMode) error {
+	info, err := os.Lstat(path)
+	if err != nil || !info.IsDir() || info.Mode()&amp;os.ModeSymlink != 0 || !ownedByCurrentUser(info) || info.Mode().Perm() != mode.Perm() {
+		return fmt.Errorf(&quot;GitHub App adapter directory is unsafe&quot;)
+	}
+	return nil
+}
+
+func writePrivateScript(path string, content []byte) error {
+	return writePrivateFile(path, content, 0700)
+}
+
+func writePrivateFile(path string, content []byte, mode fs.FileMode) error {
+	if info, err := os.Lstat(path); err == nil {
+		if !info.Mode().IsRegular() || !ownedByCurrentUser(info) || info.Mode().Perm() != mode.Perm() {
+			return fmt.Errorf(&quot;unsafe existing adapter script&quot;)
+		}
+	} else if !os.IsNotExist(err) {
+		return err
+	}
+	tmp, err := os.CreateTemp(filepath.Dir(path), &quot;.dch-adapter-*&quot;)
+	if err != nil {
+		return err
+	}
+	tmpPath := tmp.Name()
+	defer os.Remove(tmpPath)
+	if err := tmp.Chmod(mode); err != nil {
+		_ = tmp.Close()
+		return err
+	}
+	if _, err := tmp.Write(content); err != nil {
+		_ = tmp.Close()
+		return err
+	}
+	if err := tmp.Close(); err != nil {
+		return err
+	}
+	return os.Rename(tmpPath, path)
+}
+
+func adapterScripts() map[string]string {
+	return map[string]string{
+		&quot;git-credential-dch-github&quot;: gitCredentialScript,
+		&quot;gh&quot;:                        ghWrapperScript,
+	}
+}
+
+const gitCredentialScript = `#!/bin/sh
+set -eu
+operation=${1-}
+[ &quot;$operation&quot; = get ] || exit 0
+protocol=
+host=
+while IFS= read -r line; do
+	case &quot;$line&quot; in
+		protocol=*) protocol=${line#protocol=} ;;
+		host=*) host=${line#host=} ;;
+	esac
+done
+[ &quot;$protocol&quot; = https ] &amp;&amp; [ &quot;$host&quot; = github.com ] || exit 0
+token=$(curl --disable --noproxy &#x27;*&#x27; --unix-socket /run/dch-github/credential.sock --fail --silent --show-error --max-time 20 http://localhost/credential) || exit 1
+[ -n &quot;$token&quot; ] || exit 1
+printf &#x27;username=x-access-token\npassword=%s\n\n&#x27; &quot;$token&quot;
+`
+
+const ghWrapperScript = `#!/bin/sh
+set -eu
+adapter_bin=/run/dch-github/bin
+rest=${PATH-}
+clean_path=
+while :; do
+	case &quot;$rest&quot; in
+		*:*) part=${rest%%:*}; rest=${rest#*:}; more=yes ;;
+		*) part=$rest; rest=; more=no ;;
+	esac
+	if [ &quot;$part&quot; != &quot;$adapter_bin&quot; ]; then
+		if [ -n &quot;$clean_path&quot; ]; then clean_path=$clean_path:$part; else clean_path=$part; fi
+	fi
+	[ &quot;$more&quot; = yes ] || break
+done
+PATH=$clean_path
+export PATH
+real_gh=$(command -v gh) || { echo &#x27;dch: gh is required&#x27; &gt;&amp;2; exit 127; }
+token=$(curl --disable --noproxy &#x27;*&#x27; --unix-socket /run/dch-github/credential.sock --fail --silent --show-error --max-time 20 http://localhost/credential) || { echo &#x27;dch: GitHub credential unavailable&#x27; &gt;&amp;2; exit 1; }
+[ -n &quot;$token&quot; ] || { echo &#x27;dch: GitHub credential unavailable&#x27; &gt;&amp;2; exit 1; }
+GH_TOKEN=$token
+export GH_TOKEN
+exec &quot;$real_gh&quot; &quot;$@&quot;
+`
+
+const adapterGitConfig = `[credential &quot;https://github.com&quot;]
+	helper =
+	helper = /run/dch-github/bin/git-credential-dch-github
+`
+
+const adapterInstallScript = `set -eu
+adapter_bin=/run/dch-github/bin
+rest=${PATH-}
+clean_path=
+while :; do
+	case &quot;$rest&quot; in
+		*:*) part=${rest%%:*}; rest=${rest#*:}; more=yes ;;
+		*) part=$rest; rest=; more=no ;;
+	esac
+	if [ &quot;$part&quot; != &quot;$adapter_bin&quot; ]; then
+		if [ -n &quot;$clean_path&quot; ]; then clean_path=$clean_path:$part; else clean_path=$part; fi
+	fi
+	[ &quot;$more&quot; = yes ] || break
+done
+PATH=$clean_path
+export PATH
+for dependency in git gh curl sh; do
+	command -v &quot;$dependency&quot; &gt;/dev/null 2&gt;&amp;1 || { echo &quot;dch: required command missing: $dependency&quot; &gt;&amp;2; exit 1; }
+done
+[ -x /run/dch-github ] || { echo &#x27;dch: cannot access GitHub runtime directory; verify the bind mount and align the container UID with the host user&#x27; &gt;&amp;2; exit 1; }
+[ -S /run/dch-github/credential.sock ] || { echo &#x27;dch: credential socket unavailable; verify the runtime mount and align the container UID with the host user&#x27; &gt;&amp;2; exit 1; }
+[ -w /run/dch-github/credential.sock ] || { echo &#x27;dch: no permission to connect to the credential socket; align the container UID with the host user&#x27; &gt;&amp;2; exit 1; }
+curl --disable --noproxy &#x27;*&#x27; --unix-socket /run/dch-github/credential.sock --fail --silent --show-error --max-time 20 http://localhost/health &gt;/dev/null || { echo &#x27;dch: credential service unavailable or socket permission denied; check the host service and align the container UID with the host user&#x27; &gt;&amp;2; exit 1; }
+include=/run/dch-github/gitconfig
+if ! git config --global --fixed-value --get-all include.path &quot;$include&quot; &gt;/dev/null 2&gt;&amp;1; then
+	git config --global --add include.path &quot;$include&quot;
+fi
+`
</code></pre></div>
<div><h3>G6 新增／修改：Console 與執行生命週期</h3><p>確認既有容器待 Build 時阻擋 Start／Connect。Build 保留移除確認；移除前清理服務。</p><p>需你 review：是</p><label><input type="checkbox" id="check-G6"> 我已確認 G6 的決策與風險</label><p><strong>檔案與新版本 hunk 起始行</strong></p><ul><li>internal/console/plugins.go:1</li><li>internal/console/plugin_integration.go:1</li><li>internal/console/projects_menu.go:1,17,39,87,108,120,138,176,192</li><li>internal/console/devcontainer.go:1,11,70,116,126</li><li>internal/console/up.go:1</li><li>internal/console/build.go:1,42</li><li>internal/console/connect.go:1</li><li>internal/common/remove_project.go:2,24</li><li>internal/commands/commands.go:1,57,82</li></ul></div><div id="review-G6"><p>版本 759b9d8；以下為完整檔案 diff（保留完整 hunk）。行號見 @@ 的 + 區段。</p><h4>internal/console/plugins.go</h4><pre style="white-space:pre-wrap;overflow-wrap:anywhere"><code>diff --git a/internal/console/plugins.go b/internal/console/plugins.go
new file mode 100644
index 0000000..a26ce66
--- /dev/null
+++ b/internal/console/plugins.go
@@ -0,0 +1,157 @@
+package console
+
+import (
+	&quot;context&quot;
+	&quot;errors&quot;
+	&quot;fmt&quot;
+	&quot;io&quot;
+	&quot;sort&quot;
+	&quot;strconv&quot;
+	&quot;strings&quot;
+
+	&quot;dch/internal/githubapp&quot;
+	&quot;dch/internal/plugins&quot;
+)
+
+func pluginsMenu(project Project) error {
+	manager := plugins.NewManager()
+	for {
+		status, err := manager.Status(context.Background(), project.Path)
+		if err != nil {
+			return err
+		}
+		label := &quot;GitHub App: disabled&quot;
+		if status.Enabled {
+			label = &quot;GitHub App: enabled&quot;
+		}
+		if status.ServiceActive {
+			label += &quot; · service active&quot;
+		} else if status.Enabled {
+			label += &quot; · service inactive&quot;
+		}
+		if status.PendingBuild {
+			label += &quot; · Build required&quot;
+		}
+		options := []string{label, &quot;Configure and enable GitHub App&quot;, &quot;Disable GitHub App&quot;, &quot;Back&quot;}
+		choice, err := choose(&quot;Project plugins&quot;, options)
+		if errors.Is(err, errCancelled) || choice == 3 {
+			return nil
+		}
+		if err != nil {
+			return err
+		}
+		switch choice {
+		case 0:
+			message := label
+			if status.Message != &quot;&quot; {
+				message += &quot;\n&quot; + status.Message
+			}
+			screenNotice(&quot;Project plugins&quot;, message)
+		case 1:
+			screenHeading(&quot;Configure GitHub App&quot;)
+			cfg, err := promptGitHubAppConfig()
+			if errors.Is(err, errCancelled) {
+				continue
+			}
+			if err == nil {
+				err = manager.Enable(context.Background(), project.Path, cfg)
+			}
+			if err != nil {
+				if screenActive {
+					screenError(err)
+					continue
+				}
+				return err
+			}
+			screenNotice(&quot;GitHub App&quot;, &quot;Enabled and verified. Build the project to apply its container settings.&quot;)
+		case 2:
+			if !status.Enabled {
+				continue
+			}
+			if err := manager.Disable(context.Background(), project.Path); err != nil {
+				if screenActive {
+					screenError(err)
+					continue
+				}
+				return err
+			}
+			screenNotice(&quot;GitHub App&quot;, &quot;Disabled. Build the project to remove its container settings.&quot;)
+		}
+	}
+}
+
+func promptGitHubAppConfig() (githubapp.Config, error) {
+	var cfg githubapp.Config
+	appID, err := promptPositiveID(&quot;GitHub App ID&quot;)
+	if err != nil {
+		return cfg, err
+	}
+	cfg.AppID = appID
+	installationID, err := promptPositiveID(&quot;Installation ID&quot;)
+	if err != nil {
+		return cfg, err
+	}
+	cfg.InstallationID = installationID
+	cfg.PrivateKeyPassEntry, err = promptText(&quot;pass entry containing the private key&quot;)
+	if err != nil {
+		return cfg, err
+	}
+	cfg.Repository, err = promptText(&quot;GitHub repository (owner/repository)&quot;)
+	if err != nil {
+		return cfg, err
+	}
+	cfg.Permissions = map[string]string{}
+	for _, permission := range []string{&quot;contents&quot;, &quot;pull_requests&quot;} {
+		choice, err := choose(permission+&quot; permission&quot;, []string{&quot;Read&quot;, &quot;Write&quot;, &quot;Disabled&quot;})
+		if err != nil {
+			return cfg, err
+		}
+		if choice &lt; 2 {
+			cfg.Permissions[permission] = []string{&quot;read&quot;, &quot;write&quot;}[choice]
+		}
+	}
+	if err := cfg.Validate(); err != nil {
+		return cfg, err
+	}
+	scopes := make([]string, 0, len(cfg.Permissions))
+	for name, level := range cfg.Permissions {
+		scopes = append(scopes, name+&quot;:&quot;+level)
+	}
+	sort.Strings(scopes)
+	choice, err := choose(&quot;Authorize repository access: &quot;+cfg.Repository+&quot; (&quot;+strings.Join(scopes, &quot;, &quot;)+&quot;)&quot;, []string{&quot;Enable and verify&quot;, &quot;Cancel&quot;})
+	if err != nil {
+		return cfg, err
+	}
+	if choice != 0 {
+		return cfg, errCancelled
+	}
+	return cfg, nil
+}
+
+func promptPositiveID(label string) (int64, error) {
+	value, err := promptText(label)
+	if err != nil {
+		return 0, err
+	}
+	id, err := strconv.ParseInt(value, 10, 64)
+	if err != nil || id &lt;= 0 {
+		return 0, fmt.Errorf(&quot;%s must be a positive number&quot;, label)
+	}
+	return id, nil
+}
+
+func promptText(label string) (string, error) {
+	fmt.Printf(&quot;%s (q to cancel): &quot;, label)
+	value, err := input.ReadString(&#x27;\n&#x27;)
+	if err != nil &amp;&amp; !errors.Is(err, io.EOF) {
+		return &quot;&quot;, err
+	}
+	value = strings.TrimSpace(value)
+	if value == &quot;q&quot; || value == &quot;Q&quot; || (errors.Is(err, io.EOF) &amp;&amp; value == &quot;&quot;) {
+		return &quot;&quot;, errCancelled
+	}
+	if value == &quot;&quot; {
+		return &quot;&quot;, fmt.Errorf(&quot;%s is required&quot;, label)
+	}
+	return value, nil
+}
</code></pre><h4>internal/console/plugin_integration.go</h4><pre style="white-space:pre-wrap;overflow-wrap:anywhere"><code>diff --git a/internal/console/plugin_integration.go b/internal/console/plugin_integration.go
new file mode 100644
index 0000000..2aae0ef
--- /dev/null
+++ b/internal/console/plugin_integration.go
@@ -0,0 +1,150 @@
+package console
+
+import (
+	&quot;context&quot;
+	&quot;encoding/json&quot;
+	&quot;fmt&quot;
+	&quot;net/url&quot;
+	&quot;os&quot;
+	&quot;os/exec&quot;
+	&quot;path&quot;
+	&quot;strings&quot;
+	&quot;time&quot;
+
+	&quot;dch/internal/githubapp&quot;
+	&quot;dch/internal/plugins&quot;
+)
+
+func prepareProjectPlugin(p Project) error {
+	manager := plugins.NewManager()
+	if err := requireLocalDocker(); err != nil {
+		return err
+	}
+	if err := manager.Ensure(context.Background(), p.Path); err != nil {
+		return fmt.Errorf(&quot;prepare GitHub App credential service: %w&quot;, err)
+	}
+	return nil
+}
+
+func requireLocalDocker() error {
+	contextName := os.Getenv(&quot;DOCKER_CONTEXT&quot;)
+	if host := os.Getenv(&quot;DOCKER_HOST&quot;); host != &quot;&quot; &amp;&amp; contextName == &quot;&quot; {
+		endpoint, err := url.Parse(host)
+		if err != nil || endpoint.Scheme != &quot;unix&quot; {
+			return fmt.Errorf(&quot;GitHub App projects require a local Docker Unix socket&quot;)
+		}
+		return nil
+	}
+	args := []string{&quot;context&quot;, &quot;inspect&quot;}
+	if contextName != &quot;&quot; {
+		args = append(args, contextName)
+	}
+	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
+	defer cancel()
+	cmd := exec.CommandContext(ctx, &quot;docker&quot;, args...)
+	var output boundedOutput
+	cmd.Stdout = &amp;output
+	if err := cmd.Run(); err != nil || output.overflow {
+		return fmt.Errorf(&quot;cannot verify local Docker context&quot;)
+	}
+	data := output.Bytes()
+	var contexts []struct {
+		Endpoints map[string]struct {
+			Host string `json:&quot;Host&quot;`
+		} `json:&quot;Endpoints&quot;`
+	}
+	if json.Unmarshal(data, &amp;contexts) != nil || len(contexts) == 0 {
+		return fmt.Errorf(&quot;cannot verify local Docker context&quot;)
+	}
+	for _, dockerContext := range contexts {
+		endpoint := dockerContext.Endpoints[&quot;docker&quot;]
+		parsed, err := url.Parse(endpoint.Host)
+		if err != nil || parsed.Scheme != &quot;unix&quot; {
+			return fmt.Errorf(&quot;GitHub App projects require a local Docker Unix socket&quot;)
+		}
+	}
+	return nil
+}
+
+func validateGitHubAppConfig(data []byte, socketDir string) error {
+	expected, err := githubapp.AdapterFragment(socketDir)
+	if err != nil {
+		return err
+	}
+	var required struct {
+		Mounts    []string          `json:&quot;mounts&quot;`
+		RemoteEnv map[string]string `json:&quot;remoteEnv&quot;`
+	}
+	if err := json.Unmarshal(expected, &amp;required); err != nil {
+		return fmt.Errorf(&quot;cannot validate GitHub App adapter settings&quot;)
+	}
+	var config map[string]json.RawMessage
+	if err := json.Unmarshal(data, &amp;config); err != nil {
+		return fmt.Errorf(&quot;cannot validate GitHub App adapter settings&quot;)
+	}
+	if _, exists := config[&quot;dockerComposeFile&quot;]; exists {
+		return fmt.Errorf(&quot;GitHub App plugin does not support Docker Compose&quot;)
+	}
+	var mounts []json.RawMessage
+	if raw, exists := config[&quot;mounts&quot;]; !exists || json.Unmarshal(raw, &amp;mounts) != nil {
+		return fmt.Errorf(&quot;GitHub App socket mount was removed or overridden&quot;)
+	}
+	expectedMount := required.Mounts[0]
+	foundMount := false
+	for _, raw := range mounts {
+		var mountString string
+		if json.Unmarshal(raw, &amp;mountString) == nil {
+			fields := strings.Split(mountString, &quot;,&quot;)
+			targetConflict, exact := false, mountString == expectedMount
+			for _, field := range fields {
+				for _, key := range []string{&quot;target&quot;, &quot;dst&quot;, &quot;destination&quot;} {
+					if strings.HasPrefix(field, key+&quot;=&quot;) &amp;&amp; overlapsAdapterMount(strings.TrimPrefix(field, key+&quot;=&quot;)) {
+						targetConflict = true
+					}
+				}
+			}
+			if targetConflict &amp;&amp; !exact {
+				return fmt.Errorf(&quot;GitHub App socket mount was overridden&quot;)
+			}
+			foundMount = foundMount || exact
+			continue
+		}
+		var mountObject map[string]any
+		if json.Unmarshal(raw, &amp;mountObject) == nil {
+			for _, key := range []string{&quot;target&quot;, &quot;dst&quot;, &quot;destination&quot;} {
+				if target, ok := mountObject[key].(string); ok &amp;&amp; overlapsAdapterMount(target) {
+					return fmt.Errorf(&quot;GitHub App socket mount was overridden&quot;)
+				}
+			}
+		}
+	}
+	if !foundMount {
+		return fmt.Errorf(&quot;GitHub App socket mount was removed or overridden&quot;)
+	}
+	var remoteEnv map[string]any
+	if raw, exists := config[&quot;remoteEnv&quot;]; !exists || json.Unmarshal(raw, &amp;remoteEnv) != nil || remoteEnv[&quot;PATH&quot;] != required.RemoteEnv[&quot;PATH&quot;] {
+		return fmt.Errorf(&quot;GitHub App adapter PATH was removed or overridden&quot;)
+	}
+	return nil
+}
+
+func overlapsAdapterMount(target string) bool {
+	target = path.Clean(target)
+	return target == &quot;/run/dch-github&quot; || strings.HasPrefix(target, &quot;/run/dch-github/&quot;)
+}
+
+type boundedOutput struct {
+	data     []byte
+	overflow bool
+}
+
+func (output *boundedOutput) Write(data []byte) (int, error) {
+	const max = 64 * 1024
+	if len(output.data)+len(data) &gt; max {
+		output.overflow = true
+		return 0, fmt.Errorf(&quot;command output too large&quot;)
+	}
+	output.data = append(output.data, data...)
+	return len(data), nil
+}
+func (output *boundedOutput) Bytes() []byte { return output.data }
</code></pre><h4>internal/console/projects_menu.go</h4><pre style="white-space:pre-wrap;overflow-wrap:anywhere"><code>diff --git a/internal/console/projects_menu.go b/internal/console/projects_menu.go
index 597ba7c..c5db69f 100644
--- a/internal/console/projects_menu.go
+++ b/internal/console/projects_menu.go
@@ -1,11 +1,13 @@
 package console
 
 import (
+	&quot;context&quot;
 	&quot;errors&quot;
 	&quot;fmt&quot;
 	&quot;strings&quot;
 
 	&quot;dch/internal/common&quot;
+	&quot;dch/internal/plugins&quot;
 )
 
 type projectAction string
@@ -15,6 +17,7 @@ const (
 	actionStart   projectAction = &quot;Start&quot;
 	actionStop    projectAction = &quot;Stop&quot;
 	actionBuild   projectAction = &quot;Build&quot;
+	actionPlugins projectAction = &quot;Plugins&quot;
 	actionConfig  projectAction = &quot;Show config&quot;
 	actionRemove  projectAction = &quot;Remove Project&quot;
 	actionBack    projectAction = &quot;Back to projects&quot;
@@ -36,7 +39,15 @@ func projectsMenu() error {
 		statusErr := common.UpdateStatuses(projects)
 		options := make([]string, 0, len(projects)+1)
 		for _, p := range projects {
-			options = append(options, fmt.Sprintf(&quot;%s  [%s]  %s&quot;, p.Name, p.State, p.Path))
+			status, statusErr := plugins.NewManager().Status(context.Background(), p.Path)
+			if statusErr != nil {
+				return statusErr
+			}
+			state := p.State
+			if status.PendingBuild {
+				state += &quot; · Build required&quot;
+			}
+			options = append(options, fmt.Sprintf(&quot;%s  [%s]  %s&quot;, p.Name, state, p.Path))
 		}
 		options = append(options, &quot;Quit console&quot;)
 		title := &quot;Projects&quot;
@@ -76,8 +87,12 @@ func projectMenu(path string) error {
 		current := []Project{p}
 		statusErr := common.UpdateStatuses(current)
 		p = current[0]
-		actions := availableProjectActions(p, statusErr == nil)
-		choice, err := chooseProject(p, actions)
+		pluginStatus, err := plugins.NewManager().Status(context.Background(), p.Path)
+		if err != nil {
+			return err
+		}
+		actions := availableProjectActions(p, statusErr == nil, pluginStatus.PendingBuild)
+		choice, err := chooseProject(p, actions, pluginStatus.PendingBuild)
 		if errors.Is(err, errCancelled) {
 			return nil
 		}
@@ -93,6 +108,8 @@ func projectMenu(path string) error {
 			err = stopProject(p)
 		case actionBuild:
 			err = buildProject(p)
+		case actionPlugins:
+			err = pluginsMenu(p)
 		case actionConfig:
 			err = showProjectConfig(p)
 		case actionRemove:
@@ -103,7 +120,7 @@ func projectMenu(path string) error {
 			if chooseErr != nil {
 				err = chooseErr
 			} else if removeChoice == 0 {
-				err = common.RemoveProjectRegistration(p.Path)
+				err = removeProject(p)
 				if err == nil {
 					return nil
 				}
@@ -121,7 +138,33 @@ func projectMenu(path string) error {
 	}
 }
 
-func chooseProject(p Project, actions []projectAction) (int, error) {
+func removeProject(p Project) error {
+	path, err := common.NormalizePath(p.Path)
+	if err != nil {
+		return err
+	}
+	config, err := common.LoadConfig()
+	if err != nil {
+		return err
+	}
+	projectPlugins := config.ProjectPlugins[path]
+	if err := common.ValidateProjectPlugins(projectPlugins, map[string]struct{}{&quot;github-app&quot;: {}}); err != nil {
+		return err
+	}
+	manager := plugins.NewManager()
+	status, err := manager.Status(context.Background(), path)
+	if err != nil {
+		return err
+	}
+	if status.Enabled {
+		if err := manager.Disable(context.Background(), path); err != nil {
+			return fmt.Errorf(&quot;disable project plugins before removal: %w&quot;, err)
+		}
+	}
+	return common.RemoveProjectRegistration(path)
+}
+
+func chooseProject(p Project, actions []projectAction, pendingBuild bool) (int, error) {
 	options := make([]string, len(actions))
 	for i, action := range actions {
 		switch action {
@@ -133,6 +176,11 @@ func chooseProject(p Project, actions []projectAction) (int, error) {
 			options[i] = &quot;■ Stop  Stop the running container&quot;
 		case actionBuild:
 			options[i] = &quot;▣ Build  Rebuild container or image&quot;
+			if pendingBuild {
+				options[i] = &quot;▣ Build required  Apply project plugin changes&quot;
+			}
+		case actionPlugins:
+			options[i] = &quot;⚙ Plugins  Configure project plugins&quot;
 		case actionConfig:
 			options[i] = &quot;⚙ Show config  Preview merged devcontainer settings&quot;
 		case actionRemove:
@@ -144,16 +192,21 @@ func chooseProject(p Project, actions []projectAction) (int, error) {
 	return chooseView(menuView{project: &amp;p, options: options})
 }
 
-func availableProjectActions(p Project, dockerAvailable bool) []projectAction {
-	actions := make([]projectAction, 0, 5)
+func availableProjectActions(p Project, dockerAvailable, pendingBuild bool) []projectAction {
+	actions := make([]projectAction, 0, 6)
 	if p.Valid &amp;&amp; dockerAvailable {
 		switch {
 		case strings.HasPrefix(p.State, &quot;RUNNING&quot;):
-			actions = append(actions, actionConnect, actionStop)
+			if !pendingBuild {
+				actions = append(actions, actionConnect)
+			}
+			actions = append(actions, actionStop)
 		case strings.HasPrefix(p.State, &quot;STOPPED&quot;), p.State == &quot;NOT CREATED&quot;:
-			actions = append(actions, actionStart)
+			if !pendingBuild || p.State == &quot;NOT CREATED&quot; {
+				actions = append(actions, actionStart)
+			}
 		}
 		actions = append(actions, actionBuild)
 	}
-	return append(actions, actionConfig, actionRemove, actionBack)
+	return append(actions, actionPlugins, actionConfig, actionRemove, actionBack)
 }
</code></pre><h4>internal/console/devcontainer.go</h4><pre style="white-space:pre-wrap;overflow-wrap:anywhere"><code>diff --git a/internal/console/devcontainer.go b/internal/console/devcontainer.go
index d2491ba..57bbc4c 100644
--- a/internal/console/devcontainer.go
+++ b/internal/console/devcontainer.go
@@ -1,6 +1,8 @@
 package console
 
 import (
+	&quot;bytes&quot;
+	&quot;context&quot;
 	&quot;crypto/sha256&quot;
 	&quot;encoding/hex&quot;
 	&quot;encoding/json&quot;
@@ -9,8 +11,11 @@ import (
 	&quot;os/exec&quot;
 	&quot;path/filepath&quot;
 	&quot;strings&quot;
+	&quot;sync&quot;
 
 	&quot;dch/internal/common&quot;
+	&quot;dch/internal/githubapp&quot;
+	&quot;dch/internal/plugins&quot;
 )
 
 func requireExecutable(name string) error {
@@ -65,10 +70,31 @@ func hasArgument(args []string, want string) bool {
 }
 
 func invokeProjectDevcontainer(p Project, args ...string) error {
+	return invokeProjectDevcontainerMode(p, false, args...)
+}
+
+func invokeProjectDevcontainerQuiet(p Project, args ...string) error {
+	return invokeProjectDevcontainerMode(p, true, args...)
+}
+
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
 	if len(args) &gt; 0 &amp;&amp; args[0] == &quot;up&quot; {
 		config, err = withStableContainerName(config, p)
 		if err != nil {
@@ -90,8 +116,8 @@ func invokeProjectDevcontainer(p Project, args ...string) error {
 		return fmt.Errorf(&quot;write temporary merged devcontainer config: %w&quot;, err)
 	}
 
-	args = append(args, &quot;--config&quot;, mergedPath)
-	if args[0] == &quot;up&quot; {
+	extraArgs := []string{&quot;--config&quot;, mergedPath}
+	if args[0] == &quot;up&quot; || args[0] == &quot;exec&quot; {
 		basePath, err := filepath.Abs(filepath.Join(configDir, &quot;devcontainer.json&quot;))
 		if err != nil {
 			return fmt.Errorf(&quot;resolve devcontainer config path: %w&quot;, err)
@@ -100,12 +126,73 @@ func invokeProjectDevcontainer(p Project, args ...string) error {
 		if err != nil {
 			return fmt.Errorf(&quot;resolve workspace path: %w&quot;, err)
 		}
-		args = append(args,
+		extraArgs = append(extraArgs,
 			&quot;--id-label&quot;, &quot;devcontainer.local_folder=&quot;+workspacePath,
 			&quot;--id-label&quot;, &quot;devcontainer.config_file=&quot;+basePath,
 		)
 	}
-	return invokeDevcontainerWithCleanup(func() { _ = os.RemoveAll(tempDir) }, args...)
+	if args[0] == &quot;exec&quot; &amp;&amp; len(args) &gt;= 4 &amp;&amp; args[1] == &quot;--workspace-folder&quot; {
+		args = append(args[:3], append(extraArgs, args[3:]...)...)
+	} else {
+		args = append(args, extraArgs...)
+	}
+	cleanup := func() { _ = os.RemoveAll(tempDir) }
+	if quiet {
+		err = invokeDevcontainerQuiet(args...)
+		cleanup()
+		return err
+	}
+	return invokeDevcontainerWithCleanup(cleanup, args...)
+}
+
+func invokeDevcontainerQuiet(args ...string) error {
+	if len(args) == 0 {
+		return fmt.Errorf(&quot;devcontainer command is required&quot;)
+	}
+	screenHeading(&quot;Configure GitHub App authentication&quot;)
+	cmd := exec.Command(&quot;devcontainer&quot;, args...)
+	var output diagnosticCapture
+	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, &amp;output, &amp;output
+	if err := cmd.Run(); err != nil {
+		detail := strings.TrimSpace(output.String())
+		if output.truncated {
+			detail += &quot; (diagnostic output truncated)&quot;
+		}
+		if detail != &quot;&quot; {
+			return fmt.Errorf(&quot;GitHub App container setup failed: %s&quot;, detail)
+		}
+		return fmt.Errorf(&quot;GitHub App container setup failed: %w&quot;, err)
+	}
+	return nil
+}
+
+type diagnosticCapture struct {
+	mu        sync.Mutex
+	buffer    bytes.Buffer
+	truncated bool
+}
+
+func (capture *diagnosticCapture) Write(data []byte) (int, error) {
+	capture.mu.Lock()
+	defer capture.mu.Unlock()
+	const maxDiagnosticBytes = 4096
+	remaining := maxDiagnosticBytes - capture.buffer.Len()
+	if remaining &gt; 0 {
+		if len(data) &gt; remaining {
+			_, _ = capture.buffer.Write(data[:remaining])
+			capture.truncated = true
+		} else {
+			_, _ = capture.buffer.Write(data)
+		}
+	} else {
+		capture.truncated = true
+	}
+	return len(data), nil
+}
+func (capture *diagnosticCapture) String() string {
+	capture.mu.Lock()
+	defer capture.mu.Unlock()
+	return capture.buffer.String()
 }
 
 func invokeDevcontainerWithCleanup(cleanup func(), args ...string) error {
</code></pre><h4>internal/console/up.go</h4><pre style="white-space:pre-wrap;overflow-wrap:anywhere"><code>diff --git a/internal/console/up.go b/internal/console/up.go
index 825ac04..0bd878f 100644
--- a/internal/console/up.go
+++ b/internal/console/up.go
@@ -1,8 +1,44 @@
 package console
 
+import (
+	&quot;context&quot;
+	&quot;fmt&quot;
+	&quot;strings&quot;
+
+	&quot;dch/internal/plugins&quot;
+)
+
 func startProject(p Project) error {
+	manager := plugins.NewManager()
+	status, err := manager.Status(context.Background(), p.Path)
+	if err != nil {
+		return err
+	}
+	if status.PendingBuild &amp;&amp; hasExistingContainer(p) {
+		return fmt.Errorf(&quot;%s has project plugin changes; Build the project before starting&quot;, p.Name)
+	}
 	if err := requireExecutable(&quot;devcontainer&quot;); err != nil {
 		return err
 	}
-	return invokeProjectDevcontainer(p, &quot;up&quot;, &quot;--workspace-folder&quot;, p.Path)
+	if status.Enabled {
+		if err := prepareProjectPlugin(p); err != nil {
+			return err
+		}
+	}
+	if err := invokeProjectDevcontainer(p, &quot;up&quot;, &quot;--workspace-folder&quot;, p.Path); err != nil {
+		return err
+	}
+	if status.Enabled {
+		if err := runAdapterInstall(p); err != nil {
+			return err
+		}
+	}
+	if p.State == &quot;NOT CREATED&quot; {
+		return manager.MarkBuilt(p.Path)
+	}
+	return nil
+}
+
+func hasExistingContainer(p Project) bool {
+	return p.ID != &quot;&quot; || strings.HasPrefix(p.State, &quot;RUNNING&quot;) || strings.HasPrefix(p.State, &quot;STOPPED&quot;)
 }
</code></pre><h4>internal/console/build.go</h4><pre style="white-space:pre-wrap;overflow-wrap:anywhere"><code>diff --git a/internal/console/build.go b/internal/console/build.go
index 7f93d79..535c1f4 100644
--- a/internal/console/build.go
+++ b/internal/console/build.go
@@ -1,9 +1,36 @@
 package console
 
+import (
+	&quot;context&quot;
+	&quot;dch/internal/plugins&quot;
+)
+
 func buildProject(p Project) error {
+	if hasExistingContainer(p) {
+		choice, err := choose(&quot;Rebuild container? Writable-layer data may be discarded&quot;, []string{
+			&quot;Remove the existing container and rebuild&quot;,
+			&quot;Cancel&quot;,
+		})
+		if err != nil {
+			return err
+		}
+		if choice != 0 {
+			return errCancelled
+		}
+	}
 	if err := requireExecutable(&quot;devcontainer&quot;); err != nil {
 		return err
 	}
+	manager := plugins.NewManager()
+	status, err := manager.Status(context.Background(), p.Path)
+	if err != nil {
+		return err
+	}
+	if status.Enabled {
+		if err := prepareProjectPlugin(p); err != nil {
+			return err
+		}
+	}
 	cacheChoice, err := choose(&quot;Build cache&quot;, []string{
 		&quot;Use build cache&quot;,
 		&quot;Build without cache&quot;,
@@ -15,5 +42,13 @@ func buildProject(p Project) error {
 	if cacheChoice == 1 {
 		args = append(args, &quot;--build-no-cache&quot;)
 	}
-	return invokeProjectDevcontainer(p, args...)
+	if err := invokeProjectDevcontainer(p, args...); err != nil {
+		return err
+	}
+	if status.Enabled {
+		if err := runAdapterInstall(p); err != nil {
+			return err
+		}
+	}
+	return manager.MarkBuilt(p.Path)
 }
</code></pre><h4>internal/console/connect.go</h4><pre style="white-space:pre-wrap;overflow-wrap:anywhere"><code>diff --git a/internal/console/connect.go b/internal/console/connect.go
index 3e6b215..10fb9be 100644
--- a/internal/console/connect.go
+++ b/internal/console/connect.go
@@ -1,18 +1,42 @@
 package console
 
 import (
+	&quot;context&quot;
 	&quot;fmt&quot;
 	&quot;strings&quot;
+
+	&quot;dch/internal/githubapp&quot;
+	&quot;dch/internal/plugins&quot;
 )
 
 func connectProject(p Project) error {
+	status, err := plugins.NewManager().Status(context.Background(), p.Path)
+	if err != nil {
+		return err
+	}
+	if status.PendingBuild &amp;&amp; hasExistingContainer(p) {
+		return fmt.Errorf(&quot;%s has project plugin changes; Build the project before connecting&quot;, p.Name)
+	}
 	if err := requireExecutable(&quot;devcontainer&quot;); err != nil {
 		return err
 	}
 	if !strings.HasPrefix(p.State, &quot;RUNNING&quot;) {
 		return fmt.Errorf(&quot;%s is not RUNNING; use Start in dch console first&quot;, p.Name)
 	}
+	if status.Enabled {
+		if err := prepareProjectPlugin(p); err != nil {
+			return err
+		}
+		if err := runAdapterInstall(p); err != nil {
+			return err
+		}
+	}
 	// Use devcontainer exec so remoteUser and remoteEnv are applied.
 	// Prefer bash when present; /bin/sh is the portable fallback.
-	return invokeDevcontainer(&quot;exec&quot;, &quot;--workspace-folder&quot;, p.Path, &quot;/bin/sh&quot;, &quot;-c&quot;, &quot;if command -v bash &gt;/dev/null 2&gt;&amp;1; then exec bash; else exec sh; fi&quot;)
+	return invokeProjectDevcontainer(p, &quot;exec&quot;, &quot;--workspace-folder&quot;, p.Path, &quot;/bin/sh&quot;, &quot;-c&quot;, &quot;if command -v bash &gt;/dev/null 2&gt;&amp;1; then exec bash; else exec sh; fi&quot;)
+}
+
+func runAdapterInstall(p Project) error {
+	args := append([]string{&quot;exec&quot;, &quot;--workspace-folder&quot;, p.Path}, githubapp.AdapterInstallCommand()...)
+	return invokeProjectDevcontainerQuiet(p, args...)
 }
</code></pre><h4>internal/common/remove_project.go</h4><pre style="white-space:pre-wrap;overflow-wrap:anywhere"><code>diff --git a/internal/common/remove_project.go b/internal/common/remove_project.go
index 2a3c8cb..ba8b925 100644
--- a/internal/common/remove_project.go
+++ b/internal/common/remove_project.go
@@ -2,6 +2,21 @@ package common
 
 import &quot;fmt&quot;
 
+// ValidateProjectPlugins rejects plugin metadata that this version cannot manage.
+func ValidateProjectPlugins(project ProjectPlugins, knownIDs map[string]struct{}) error {
+	for id := range project.Enabled {
+		if _, ok := knownIDs[id]; !ok {
+			return fmt.Errorf(&quot;unknown enabled plugin %q&quot;, id)
+		}
+	}
+	for id := range project.States {
+		if _, ok := knownIDs[id]; !ok {
+			return fmt.Errorf(&quot;unknown plugin state %q&quot;, id)
+		}
+	}
+	return nil
+}
+
 func RemoveProjectRegistration(path string) error {
 	cfg, err := LoadConfig()
 	if err != nil {
@@ -9,7 +24,12 @@ func RemoveProjectRegistration(path string) error {
 	}
 	for i, registered := range cfg.Projects {
 		if registered == path {
+			projectPlugins := cfg.ProjectPlugins[path]
+			if err := ValidateProjectPlugins(projectPlugins, map[string]struct{}{&quot;github-app&quot;: {}}); err != nil {
+				return err
+			}
 			cfg.Projects = append(cfg.Projects[:i], cfg.Projects[i+1:]...)
+			delete(cfg.ProjectPlugins, path)
 			return SaveConfig(cfg)
 		}
 	}
</code></pre><h4>internal/commands/commands.go</h4><pre style="white-space:pre-wrap;overflow-wrap:anywhere"><code>diff --git a/internal/commands/commands.go b/internal/commands/commands.go
index 65ae9d1..4315979 100644
--- a/internal/commands/commands.go
+++ b/internal/commands/commands.go
@@ -1,7 +1,13 @@
 package commands
 
 import (
+	&quot;context&quot;
 	&quot;fmt&quot;
+	&quot;os&quot;
+	&quot;os/signal&quot;
+	&quot;syscall&quot;
+
+	&quot;dch/internal/plugins&quot;
 )
 
 // Run dispatches a CLI command. The console is also the default command.
@@ -51,6 +57,13 @@ func Run(args []string, openConsole func() error) error {
 		}
 		fmt.Printf(&quot;dch %s\n&quot;, version)
 		return nil
+	case &quot;_github-app-serve&quot;:
+		if len(args) != 3 || args[1] != &quot;--project&quot; || args[2] == &quot;&quot; {
+			return fmt.Errorf(&quot;usage: dch _github-app-serve --project PATH&quot;)
+		}
+		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
+		defer stop()
+		return plugins.NewManager().ServeProject(ctx, args[2])
 	default:
 		return fmt.Errorf(&quot;unknown command %q&quot;, args[0])
 	}
@@ -69,7 +82,7 @@ Usage:
   dch help                  Show this help
 
 Container actions: devcontainer CLI and Docker daemon required
-Config file:   ~/.dch/config (JSON; stores only workspace paths)
+Config file:   ~/.dch/config (JSON; stores project paths and plugin settings)
 Navigation:    ↑ / ↓ + Enter, q/Esc to go back (or numbered selection if not a TTY)
 `)
 }
</code></pre></div>
<div><h3>G7 新增／修改：行為測試</h3><p>測試是上述行為的證據。涵蓋假指令、真實暫存 Socket 與 Git plumbing。</p><p>需你 review：否</p><p><strong>檔案與新版本 hunk 起始行</strong></p><ul><li>internal/commands/plugin_serve_test.go:1</li><li>internal/common/common_test.go:65</li><li>internal/common/config_store_test.go:1</li><li>internal/common/remove_project_test.go:1</li><li>internal/console/build_plugin_test.go:1</li><li>internal/console/config_preview_test.go:1</li><li>internal/console/devcontainer_plugin_test.go:1</li><li>internal/console/plugin_integration_test.go:1</li><li>internal/console/plugins_test.go:1</li><li>internal/console/projects_menu_test.go:10</li><li>internal/githubapp/adapter_test.go:1</li><li>internal/githubapp/config_test.go:1</li><li>internal/githubapp/provider_test.go:1</li><li>internal/githubapp/server_test.go:1</li><li>internal/plugins/manager_test.go:1</li></ul></div><div id="review-G7"><p>版本 759b9d8；以下為完整檔案 diff（保留完整 hunk）。行號見 @@ 的 + 區段。</p><h4>internal/commands/plugin_serve_test.go</h4><pre style="white-space:pre-wrap;overflow-wrap:anywhere"><code>diff --git a/internal/commands/plugin_serve_test.go b/internal/commands/plugin_serve_test.go
new file mode 100644
index 0000000..b6cc2a7
--- /dev/null
+++ b/internal/commands/plugin_serve_test.go
@@ -0,0 +1,20 @@
+package commands
+
+import (
+	&quot;path/filepath&quot;
+	&quot;strings&quot;
+	&quot;testing&quot;
+)
+
+func TestInternalGitHubAppServerCommandDispatchesAndValidatesArguments(t *testing.T) {
+	root := t.TempDir()
+	t.Setenv(&quot;DCH_CONFIG&quot;, filepath.Join(root, &quot;config&quot;))
+	err := Run([]string{&quot;_github-app-serve&quot;, &quot;--project&quot;, filepath.Join(root, &quot;project&quot;)}, nil)
+	if err == nil || !strings.Contains(err.Error(), &quot;not enabled&quot;) {
+		t.Fatalf(&quot;internal server dispatch error=%v&quot;, err)
+	}
+	err = Run([]string{&quot;_github-app-serve&quot;, &quot;--project&quot;}, nil)
+	if err == nil || !strings.Contains(err.Error(), &quot;usage&quot;) {
+		t.Fatalf(&quot;invalid invocation error=%v&quot;, err)
+	}
+}
</code></pre><h4>internal/common/common_test.go</h4><pre style="white-space:pre-wrap;overflow-wrap:anywhere"><code>diff --git a/internal/common/common_test.go b/internal/common/common_test.go
index 898400f..3918def 100644
--- a/internal/common/common_test.go
+++ b/internal/common/common_test.go
@@ -65,6 +65,120 @@ func TestInvalidJSONC(t *testing.T) {
 	}
 }
 
+func TestMergedDevcontainerConfigUsesSortedLocalFragments(t *testing.T) {
+	root := t.TempDir()
+	configDir := filepath.Join(root, &quot;.devcontainer&quot;)
+	localDir := filepath.Join(configDir, &quot;devcontainer.local.d&quot;)
+	if err := os.MkdirAll(localDir, 0700); err != nil {
+		t.Fatal(err)
+	}
+	files := map[string]string{
+		&quot;devcontainer.json&quot;:                   `{&quot;customizations&quot;:{&quot;vscode&quot;:{&quot;extensions&quot;:[&quot;base&quot;,&quot;shared&quot;]}},&quot;name&quot;:&quot;base&quot;}`,
+		&quot;devcontainer.local.json&quot;:             `{&quot;name&quot;:&quot;legacy&quot;}`,
+		&quot;devcontainer.local.d/20-user.json&quot;:   `{&quot;name&quot;:&quot;user&quot;,&quot;customizations&quot;:{&quot;vscode&quot;:{&quot;extensions&quot;:[&quot;shared&quot;,&quot;user&quot;,],},},}`,
+		&quot;devcontainer.local.d/10-plugin.json&quot;: `{&quot;customizations&quot;:{&quot;vscode&quot;:{&quot;extensions&quot;:[&quot;plugin&quot;,&quot;shared&quot;]}}}`,
+	}
+	for name, contents := range files {
+		if err := os.WriteFile(filepath.Join(configDir, filepath.FromSlash(name)), []byte(contents), 0600); err != nil {
+			t.Fatal(err)
+		}
+	}
+	got, err := MergedDevcontainerConfig(root)
+	if err != nil {
+		t.Fatal(err)
+	}
+	var config map[string]any
+	if err := json.Unmarshal(got, &amp;config); err != nil {
+		t.Fatal(err)
+	}
+	if config[&quot;name&quot;] != &quot;user&quot; {
+		t.Fatalf(&quot;name = %v, want user&quot;, config[&quot;name&quot;])
+	}
+	customizations := config[&quot;customizations&quot;].(map[string]any)
+	vscode := customizations[&quot;vscode&quot;].(map[string]any)
+	if want := []any{&quot;base&quot;, &quot;shared&quot;, &quot;plugin&quot;, &quot;user&quot;}; !reflect.DeepEqual(vscode[&quot;extensions&quot;], want) {
+		t.Fatalf(&quot;extensions = %#v, want %#v&quot;, vscode[&quot;extensions&quot;], want)
+	}
+}
+
+func TestMergedDevcontainerConfigIgnoresLegacyFileWithoutLocalDirectory(t *testing.T) {
+	root := t.TempDir()
+	configDir := filepath.Join(root, &quot;.devcontainer&quot;)
+	if err := os.MkdirAll(configDir, 0700); err != nil {
+		t.Fatal(err)
+	}
+	if err := os.WriteFile(filepath.Join(configDir, &quot;devcontainer.json&quot;), []byte(`{&quot;name&quot;:&quot;base&quot;}`), 0600); err != nil {
+		t.Fatal(err)
+	}
+	if err := os.WriteFile(filepath.Join(configDir, &quot;devcontainer.local.json&quot;), []byte(`this legacy file is invalid`), 0600); err != nil {
+		t.Fatal(err)
+	}
+	got, err := MergedDevcontainerConfig(root)
+	if err != nil {
+		t.Fatal(err)
+	}
+	var config map[string]any
+	if err := json.Unmarshal(got, &amp;config); err != nil {
+		t.Fatal(err)
+	}
+	if config[&quot;name&quot;] != &quot;base&quot; {
+		t.Fatalf(&quot;name = %v, want base&quot;, config[&quot;name&quot;])
+	}
+}
+
+func TestMergedDevcontainerConfigUsesOnlyRegularFirstLevelJSONFragments(t *testing.T) {
+	root := t.TempDir()
+	configDir := filepath.Join(root, &quot;.devcontainer&quot;)
+	localDir := filepath.Join(configDir, &quot;devcontainer.local.d&quot;)
+	if err := os.MkdirAll(filepath.Join(localDir, &quot;nested&quot;), 0700); err != nil {
+		t.Fatal(err)
+	}
+	if err := os.WriteFile(filepath.Join(configDir, &quot;devcontainer.json&quot;), []byte(`{&quot;name&quot;:&quot;base&quot;}`), 0600); err != nil {
+		t.Fatal(err)
+	}
+	if err := os.WriteFile(filepath.Join(localDir, &quot;valid.json&quot;), []byte(`{&quot;name&quot;:&quot;valid&quot;}`), 0600); err != nil {
+		t.Fatal(err)
+	}
+	if err := os.WriteFile(filepath.Join(localDir, &quot;ignored.txt&quot;), []byte(`not json`), 0600); err != nil {
+		t.Fatal(err)
+	}
+	if err := os.WriteFile(filepath.Join(localDir, &quot;nested&quot;, &quot;nested.json&quot;), []byte(`{&quot;name&quot;:&quot;nested&quot;}`), 0600); err != nil {
+		t.Fatal(err)
+	}
+	if err := os.Symlink(filepath.Join(localDir, &quot;nested&quot;, &quot;nested.json&quot;), filepath.Join(localDir, &quot;link.json&quot;)); err != nil {
+		t.Fatal(err)
+	}
+	got, err := MergedDevcontainerConfig(root)
+	if err != nil {
+		t.Fatal(err)
+	}
+	var config map[string]any
+	if err := json.Unmarshal(got, &amp;config); err != nil {
+		t.Fatal(err)
+	}
+	if config[&quot;name&quot;] != &quot;valid&quot; {
+		t.Fatalf(&quot;name = %v, want valid&quot;, config[&quot;name&quot;])
+	}
+}
+
+func TestMergedDevcontainerConfigReportsInvalidFragmentName(t *testing.T) {
+	root := t.TempDir()
+	configDir := filepath.Join(root, &quot;.devcontainer&quot;)
+	localDir := filepath.Join(configDir, &quot;devcontainer.local.d&quot;)
+	if err := os.MkdirAll(localDir, 0700); err != nil {
+		t.Fatal(err)
+	}
+	if err := os.WriteFile(filepath.Join(configDir, &quot;devcontainer.json&quot;), []byte(`{}`), 0600); err != nil {
+		t.Fatal(err)
+	}
+	if err := os.WriteFile(filepath.Join(localDir, &quot;broken.json&quot;), []byte(`{`), 0600); err != nil {
+		t.Fatal(err)
+	}
+	if _, err := MergedDevcontainerConfig(root); err == nil || !strings.Contains(err.Error(), &quot;broken.json&quot;) {
+		t.Fatalf(&quot;error = %v, want broken.json path&quot;, err)
+	}
+}
+
 func TestSaveLoadConfig(t *testing.T) {
 	path := filepath.Join(t.TempDir(), &quot;.dch&quot;, &quot;config&quot;)
 	t.Setenv(&quot;DCH_CONFIG&quot;, path)
</code></pre><h4>internal/common/config_store_test.go</h4><pre style="white-space:pre-wrap;overflow-wrap:anywhere"><code>diff --git a/internal/common/config_store_test.go b/internal/common/config_store_test.go
new file mode 100644
index 0000000..5ad2a93
--- /dev/null
+++ b/internal/common/config_store_test.go
@@ -0,0 +1,37 @@
+package common
+
+import (
+	&quot;os&quot;
+	&quot;path/filepath&quot;
+	&quot;testing&quot;
+
+	&quot;dch/internal/githubapp&quot;
+)
+
+func TestConfigRoundTripsProjectPluginsAndLoadsLegacyConfig(t *testing.T) {
+	path := filepath.Join(t.TempDir(), &quot;config&quot;)
+	t.Setenv(&quot;DCH_CONFIG&quot;, path)
+	if err := os.WriteFile(path, []byte(`{&quot;version&quot;:1,&quot;projects&quot;:[&quot;/old&quot;]}`), 0600); err != nil {
+		t.Fatal(err)
+	}
+	cfg, err := LoadConfig()
+	if err != nil || len(cfg.Projects) != 1 || len(cfg.ProjectPlugins) != 0 {
+		t.Fatalf(&quot;legacy config = %#v, %v&quot;, cfg, err)
+	}
+	cfg.ProjectPlugins = map[string]ProjectPlugins{}
+	cfg.ProjectPlugins[&quot;/project&quot;] = ProjectPlugins{
+		Enabled: map[string]githubapp.Config{&quot;github-app&quot;: {AppID: 7, InstallationID: 8, PrivateKeyPassEntry: &quot;key&quot;, Repository: &quot;owner/repo&quot;, Permissions: map[string]string{&quot;contents&quot;: &quot;read&quot;}}},
+		States:  map[string]PluginState{&quot;github-app&quot;: {FragmentDigest: &quot;digest&quot;, UnitDigest: &quot;unit-digest&quot;, UnitPath: &quot;/home/user/.config/systemd/user/dch.service&quot;, BuildPending: true}},
+	}
+	if err := SaveConfig(cfg); err != nil {
+		t.Fatal(err)
+	}
+	loaded, err := LoadConfig()
+	if err != nil {
+		t.Fatal(err)
+	}
+	got := loaded.ProjectPlugins[&quot;/project&quot;]
+	if got.Enabled[&quot;github-app&quot;].AppID != 7 || got.States[&quot;github-app&quot;].FragmentDigest != &quot;digest&quot; || got.States[&quot;github-app&quot;].UnitDigest != &quot;unit-digest&quot; || got.States[&quot;github-app&quot;].UnitPath != &quot;/home/user/.config/systemd/user/dch.service&quot; || !got.States[&quot;github-app&quot;].BuildPending {
+		t.Fatalf(&quot;plugin state did not round trip: %#v&quot;, got)
+	}
+}
</code></pre><h4>internal/common/remove_project_test.go</h4><pre style="white-space:pre-wrap;overflow-wrap:anywhere"><code>diff --git a/internal/common/remove_project_test.go b/internal/common/remove_project_test.go
new file mode 100644
index 0000000..2d3028f
--- /dev/null
+++ b/internal/common/remove_project_test.go
@@ -0,0 +1,68 @@
+package common
+
+import (
+	&quot;os&quot;
+	&quot;path/filepath&quot;
+	&quot;testing&quot;
+
+	&quot;dch/internal/githubapp&quot;
+)
+
+func TestRemoveProjectRegistrationDeletesPerProjectPluginMetadata(t *testing.T) {
+	root := t.TempDir()
+	t.Setenv(&quot;DCH_CONFIG&quot;, filepath.Join(root, &quot;config&quot;))
+	project := filepath.Join(root, &quot;project&quot;)
+	config := Config{Version: 1, Projects: []string{project}, ProjectPlugins: map[string]ProjectPlugins{project: {Enabled: map[string]githubapp.Config{}, States: map[string]PluginState{&quot;github-app&quot;: {BuildPending: true}}}}}
+	if err := SaveConfig(config); err != nil {
+		t.Fatal(err)
+	}
+	if err := RemoveProjectRegistration(project); err != nil {
+		t.Fatal(err)
+	}
+	loaded, err := LoadConfig()
+	if err != nil {
+		t.Fatal(err)
+	}
+	if len(loaded.Projects) != 0 {
+		t.Fatalf(&quot;registered projects=%v&quot;, loaded.Projects)
+	}
+	if _, exists := loaded.ProjectPlugins[project]; exists {
+		t.Fatal(&quot;removed project plugin metadata remains&quot;)
+	}
+	if _, err := os.Stat(filepath.Join(root, &quot;config&quot;)); err != nil {
+		t.Fatal(err)
+	}
+}
+
+func TestRemoveProjectRegistrationPreservesUnknownPluginMetadata(t *testing.T) {
+	root := t.TempDir()
+	t.Setenv(&quot;DCH_CONFIG&quot;, filepath.Join(root, &quot;config&quot;))
+	project := filepath.Join(root, &quot;project&quot;)
+	for _, tc := range []struct {
+		name    string
+		plugins ProjectPlugins
+	}{
+		{name: &quot;enabled plugin&quot;, plugins: ProjectPlugins{Enabled: map[string]githubapp.Config{&quot;future-plugin&quot;: {AppID: 7}}}},
+		{name: &quot;plugin state&quot;, plugins: ProjectPlugins{States: map[string]PluginState{&quot;future-plugin&quot;: {BuildPending: true}}}},
+	} {
+		t.Run(tc.name, func(t *testing.T) {
+			config := Config{Version: 1, Projects: []string{project}, ProjectPlugins: map[string]ProjectPlugins{project: tc.plugins}}
+			if err := SaveConfig(config); err != nil {
+				t.Fatal(err)
+			}
+			if err := RemoveProjectRegistration(project); err == nil {
+				t.Fatal(&quot;expected removal to reject unknown plugin metadata&quot;)
+			}
+			loaded, err := LoadConfig()
+			if err != nil {
+				t.Fatal(err)
+			}
+			if len(loaded.Projects) != 1 || loaded.Projects[0] != project {
+				t.Fatalf(&quot;unknown plugin project was removed: %v&quot;, loaded.Projects)
+			}
+			if _, exists := loaded.ProjectPlugins[project]; !exists {
+				t.Fatal(&quot;unknown plugin metadata was removed&quot;)
+			}
+		})
+	}
+}
</code></pre><h4>internal/console/build_plugin_test.go</h4><pre style="white-space:pre-wrap;overflow-wrap:anywhere"><code>diff --git a/internal/console/build_plugin_test.go b/internal/console/build_plugin_test.go
new file mode 100644
index 0000000..7034e41
--- /dev/null
+++ b/internal/console/build_plugin_test.go
@@ -0,0 +1,92 @@
+package console
+
+import (
+	&quot;bufio&quot;
+	&quot;errors&quot;
+	&quot;os&quot;
+	&quot;path/filepath&quot;
+	&quot;strings&quot;
+	&quot;testing&quot;
+
+	&quot;dch/internal/common&quot;
+)
+
+func TestBuildRequiresConfirmationBeforeReplacingExistingContainer(t *testing.T) {
+	root := t.TempDir()
+	project := filepath.Join(root, &quot;project&quot;)
+	configDir := filepath.Join(project, &quot;.devcontainer&quot;)
+	if err := os.MkdirAll(configDir, 0700); err != nil {
+		t.Fatal(err)
+	}
+	if err := os.WriteFile(filepath.Join(configDir, &quot;devcontainer.json&quot;), []byte(`{&quot;image&quot;:&quot;example:latest&quot;}`), 0600); err != nil {
+		t.Fatal(err)
+	}
+	bin := filepath.Join(root, &quot;bin&quot;)
+	if err := os.Mkdir(bin, 0700); err != nil {
+		t.Fatal(err)
+	}
+	marker := filepath.Join(root, &quot;called&quot;)
+	if err := os.WriteFile(filepath.Join(bin, &quot;devcontainer&quot;), []byte(&quot;#!/bin/sh\ntouch &quot;+shellQuote(marker)+&quot;\n&quot;), 0700); err != nil {
+		t.Fatal(err)
+	}
+	t.Setenv(&quot;PATH&quot;, bin+string(os.PathListSeparator)+os.Getenv(&quot;PATH&quot;))
+	t.Setenv(&quot;DCH_CONFIG&quot;, filepath.Join(root, &quot;config&quot;))
+	oldInput := input
+	input = bufio.NewReader(strings.NewReader(&quot;2\n&quot;))
+	defer func() { input = oldInput }()
+	err := buildProject(Project{Name: &quot;project&quot;, Path: project, State: &quot;RUNNING&quot;})
+	if !errors.Is(err, errCancelled) {
+		t.Fatalf(&quot;build error=%v, want cancellation&quot;, err)
+	}
+	if _, err := os.Stat(marker); !os.IsNotExist(err) {
+		t.Fatalf(&quot;devcontainer ran before confirmation, stat err=%v&quot;, err)
+	}
+}
+
+func TestBuildAfterDisableAppliesConfigAndClearsPending(t *testing.T) {
+	root := t.TempDir()
+	project := filepath.Join(root, &quot;project&quot;)
+	configDir := filepath.Join(project, &quot;.devcontainer&quot;)
+	if err := os.MkdirAll(configDir, 0700); err != nil {
+		t.Fatal(err)
+	}
+	if err := os.WriteFile(filepath.Join(configDir, &quot;devcontainer.json&quot;), []byte(`{&quot;image&quot;:&quot;example:latest&quot;,&quot;postCreateCommand&quot;:&quot;echo keep&quot;}`), 0600); err != nil {
+		t.Fatal(err)
+	}
+	bin := filepath.Join(root, &quot;bin&quot;)
+	if err := os.Mkdir(bin, 0700); err != nil {
+		t.Fatal(err)
+	}
+	logPath := filepath.Join(root, &quot;args.log&quot;)
+	script := &quot;#!/bin/sh\nprintf &#x27;%s\\n&#x27; \&quot;$@\&quot; &gt; &quot; + shellQuote(logPath) + &quot;\nconfig=\nprev=\nfor arg in \&quot;$@\&quot;; do if [ \&quot;$prev\&quot; = --config ]; then config=$arg; fi; prev=$arg; done\n[ -f \&quot;$config\&quot; ] || exit 12\n&quot;
+	if err := os.WriteFile(filepath.Join(bin, &quot;devcontainer&quot;), []byte(script), 0700); err != nil {
+		t.Fatal(err)
+	}
+	t.Setenv(&quot;PATH&quot;, bin+string(os.PathListSeparator)+os.Getenv(&quot;PATH&quot;))
+	configPath := filepath.Join(root, &quot;config&quot;)
+	t.Setenv(&quot;DCH_CONFIG&quot;, configPath)
+	projectPlugins := common.ProjectPlugins{States: map[string]common.PluginState{&quot;github-app&quot;: {BuildPending: true}}}
+	if err := common.SaveConfig(common.Config{Version: 1, Projects: []string{project}, ProjectPlugins: map[string]common.ProjectPlugins{project: projectPlugins}}); err != nil {
+		t.Fatal(err)
+	}
+	oldInput := input
+	input = bufio.NewReader(strings.NewReader(&quot;1\n&quot;))
+	defer func() { input = oldInput }()
+	if err := buildProject(Project{Name: &quot;project&quot;, Path: project, State: &quot;NOT CREATED&quot;}); err != nil {
+		t.Fatal(err)
+	}
+	loaded, err := common.LoadConfig()
+	if err != nil {
+		t.Fatal(err)
+	}
+	if loaded.ProjectPlugins[project].States[&quot;github-app&quot;].BuildPending {
+		t.Fatal(&quot;successful Build did not clear disabled-plugin pending state&quot;)
+	}
+	args, err := os.ReadFile(logPath)
+	if err != nil {
+		t.Fatal(err)
+	}
+	if !strings.Contains(string(args), &quot;--remove-existing-container&quot;) {
+		t.Fatalf(&quot;Build did not request rebuild: %s&quot;, args)
+	}
+}
</code></pre><h4>internal/console/config_preview_test.go</h4><pre style="white-space:pre-wrap;overflow-wrap:anywhere"><code>diff --git a/internal/console/config_preview_test.go b/internal/console/config_preview_test.go
new file mode 100644
index 0000000..3c713d3
--- /dev/null
+++ b/internal/console/config_preview_test.go
@@ -0,0 +1,50 @@
+package console
+
+import (
+	&quot;io&quot;
+	&quot;os&quot;
+	&quot;path/filepath&quot;
+	&quot;strings&quot;
+	&quot;testing&quot;
+)
+
+func TestShowProjectConfigMentionsLocalFragments(t *testing.T) {
+	previousStdout := os.Stdout
+	previousScreenActive := screenActive
+	r, w, err := os.Pipe()
+	if err != nil {
+		t.Fatal(err)
+	}
+	os.Stdout = w
+	screenActive = false
+	t.Cleanup(func() {
+		os.Stdout = previousStdout
+		screenActive = previousScreenActive
+	})
+
+	projectPath := t.TempDir()
+	if err := os.Mkdir(filepath.Join(projectPath, &quot;.devcontainer&quot;), 0700); err != nil {
+		t.Fatal(err)
+	}
+	if err := os.WriteFile(filepath.Join(projectPath, &quot;.devcontainer&quot;, &quot;devcontainer.json&quot;), []byte(`{&quot;image&quot;:&quot;ubuntu&quot;}`), 0600); err != nil {
+		t.Fatal(err)
+	}
+	err = showProjectConfig(Project{Path: projectPath})
+	if err != nil {
+		t.Fatal(err)
+	}
+	if err := w.Close(); err != nil {
+		t.Fatal(err)
+	}
+	os.Stdout = previousStdout
+	output, err := io.ReadAll(r)
+	if err != nil {
+		t.Fatal(err)
+	}
+	if !strings.Contains(string(output), &quot;.devcontainer/devcontainer.local.d/&quot;) {
+		t.Fatalf(&quot;preview does not mention local.d: %s&quot;, output)
+	}
+	if strings.Contains(string(output), &quot;devcontainer.local.json&quot;) {
+		t.Fatalf(&quot;preview mentions legacy local file: %s&quot;, output)
+	}
+}
</code></pre><h4>internal/console/devcontainer_plugin_test.go</h4><pre style="white-space:pre-wrap;overflow-wrap:anywhere"><code>diff --git a/internal/console/devcontainer_plugin_test.go b/internal/console/devcontainer_plugin_test.go
new file mode 100644
index 0000000..7eff69f
--- /dev/null
+++ b/internal/console/devcontainer_plugin_test.go
@@ -0,0 +1,99 @@
+package console
+
+import (
+	&quot;os&quot;
+	&quot;path/filepath&quot;
+	&quot;strings&quot;
+	&quot;testing&quot;
+
+	&quot;dch/internal/common&quot;
+	&quot;dch/internal/githubapp&quot;
+)
+
+func TestProjectExecUsesStableContainerLabelsAndLiveMergedConfig(t *testing.T) {
+	root := t.TempDir()
+	project := filepath.Join(root, &quot;project&quot;)
+	configDir := filepath.Join(project, &quot;.devcontainer&quot;)
+	if err := os.MkdirAll(configDir, 0700); err != nil {
+		t.Fatal(err)
+	}
+	if err := os.WriteFile(filepath.Join(configDir, &quot;devcontainer.json&quot;), []byte(`{&quot;image&quot;:&quot;example:latest&quot;,&quot;postCreateCommand&quot;:&quot;echo preserved&quot;}`), 0600); err != nil {
+		t.Fatal(err)
+	}
+	bin := filepath.Join(root, &quot;bin&quot;)
+	if err := os.Mkdir(bin, 0700); err != nil {
+		t.Fatal(err)
+	}
+	logPath := filepath.Join(root, &quot;args.log&quot;)
+	script := &quot;#!/bin/sh\nprintf &#x27;%s\\n&#x27; \&quot;$@\&quot; &gt; &quot; + shellQuote(logPath) + &quot;\nconfig=\nwhile [ $# -gt 0 ]; do if [ \&quot;$1\&quot; = --config ]; then config=$2; fi; shift; done\n[ -f \&quot;$config\&quot; ] || exit 12\n&quot;
+	if err := os.WriteFile(filepath.Join(bin, &quot;devcontainer&quot;), []byte(script), 0700); err != nil {
+		t.Fatal(err)
+	}
+	t.Setenv(&quot;PATH&quot;, bin+string(os.PathListSeparator)+os.Getenv(&quot;PATH&quot;))
+	if err := invokeProjectDevcontainer(Project{Name: &quot;project&quot;, Path: project}, &quot;exec&quot;, &quot;--workspace-folder&quot;, project, &quot;true&quot;); err != nil {
+		t.Fatal(err)
+	}
+	args, err := os.ReadFile(logPath)
+	if err != nil {
+		t.Fatal(err)
+	}
+	got := string(args)
+	for _, want := range []string{&quot;devcontainer.local_folder=&quot; + project, &quot;devcontainer.config_file=&quot; + filepath.Join(configDir, &quot;devcontainer.json&quot;), &quot;--config&quot;} {
+		if !strings.Contains(got, want) {
+			t.Errorf(&quot;exec args missing %q: %s&quot;, want, got)
+		}
+	}
+}
+
+func TestConnectRefusesExistingContainerWithPendingPluginBuild(t *testing.T) {
+	root := t.TempDir()
+	project := filepath.Join(root, &quot;project&quot;)
+	if err := os.MkdirAll(filepath.Join(project, &quot;.devcontainer&quot;), 0700); err != nil {
+		t.Fatal(err)
+	}
+	t.Setenv(&quot;DCH_CONFIG&quot;, filepath.Join(root, &quot;dch-config&quot;))
+	config := common.Config{Version: 1, Projects: []string{project}, ProjectPlugins: map[string]common.ProjectPlugins{
+		project: {Enabled: map[string]githubapp.Config{&quot;github-app&quot;: {AppID: 1}}, States: map[string]common.PluginState{&quot;github-app&quot;: {BuildPending: true}}},
+	}}
+	if err := common.SaveConfig(config); err != nil {
+		t.Fatal(err)
+	}
+	err := connectProject(Project{Name: &quot;project&quot;, Path: project, State: &quot;RUNNING&quot;})
+	if err == nil || !strings.Contains(err.Error(), &quot;Build&quot;) {
+		t.Fatalf(&quot;connect error = %v, want pending Build message&quot;, err)
+	}
+}
+
+func TestConnectAlsoBlocksAfterPluginWasDisabledUntilBuild(t *testing.T) {
+	root := t.TempDir()
+	project := filepath.Join(root, &quot;project&quot;)
+	if err := os.MkdirAll(filepath.Join(project, &quot;.devcontainer&quot;), 0700); err != nil {
+		t.Fatal(err)
+	}
+	t.Setenv(&quot;DCH_CONFIG&quot;, filepath.Join(root, &quot;dch-config&quot;))
+	config := common.Config{Version: 1, Projects: []string{project}, ProjectPlugins: map[string]common.ProjectPlugins{project: {States: map[string]common.PluginState{&quot;github-app&quot;: {BuildPending: true}}}}}
+	if err := common.SaveConfig(config); err != nil {
+		t.Fatal(err)
+	}
+	err := connectProject(Project{Name: &quot;project&quot;, Path: project, State: &quot;RUNNING&quot;, ID: &quot;container-id&quot;})
+	if err == nil || !strings.Contains(err.Error(), &quot;Build&quot;) {
+		t.Fatalf(&quot;connect error=%v, want Build requirement&quot;, err)
+	}
+}
+
+func TestStartRefusesExistingContainerWithPendingPluginBuild(t *testing.T) {
+	root := t.TempDir()
+	project := filepath.Join(root, &quot;project&quot;)
+	_ = os.MkdirAll(filepath.Join(project, &quot;.devcontainer&quot;), 0700)
+	t.Setenv(&quot;DCH_CONFIG&quot;, filepath.Join(root, &quot;dch-config&quot;))
+	config := common.Config{Version: 1, Projects: []string{project}, ProjectPlugins: map[string]common.ProjectPlugins{project: {Enabled: map[string]githubapp.Config{&quot;github-app&quot;: {AppID: 1}}, States: map[string]common.PluginState{&quot;github-app&quot;: {BuildPending: true}}}}}
+	if err := common.SaveConfig(config); err != nil {
+		t.Fatal(err)
+	}
+	err := startProject(Project{Name: &quot;project&quot;, Path: project, State: &quot;STOPPED&quot;})
+	if err == nil || !strings.Contains(err.Error(), &quot;Build&quot;) {
+		t.Fatalf(&quot;start error = %v, want pending Build message&quot;, err)
+	}
+}
+
+func shellQuote(s string) string { return &quot;&#x27;&quot; + strings.ReplaceAll(s, &quot;&#x27;&quot;, &quot;&#x27;\\&#x27;&#x27;&quot;) + &quot;&#x27;&quot; }
</code></pre><h4>internal/console/plugin_integration_test.go</h4><pre style="white-space:pre-wrap;overflow-wrap:anywhere"><code>diff --git a/internal/console/plugin_integration_test.go b/internal/console/plugin_integration_test.go
new file mode 100644
index 0000000..dd95350
--- /dev/null
+++ b/internal/console/plugin_integration_test.go
@@ -0,0 +1,313 @@
+package console
+
+import (
+	&quot;context&quot;
+	&quot;net&quot;
+	&quot;net/http&quot;
+	&quot;os&quot;
+	&quot;path/filepath&quot;
+	&quot;strings&quot;
+	&quot;testing&quot;
+
+	&quot;dch/internal/common&quot;
+	&quot;dch/internal/githubapp&quot;
+	&quot;dch/internal/plugins&quot;
+)
+
+func TestValidatePluginConfigRequiresExactSocketMountAndAdapterPath(t *testing.T) {
+	root := t.TempDir()
+	project := filepath.Join(root, &quot;project&quot;)
+	devcontainerDir := filepath.Join(project, &quot;.devcontainer&quot;)
+	if err := os.MkdirAll(filepath.Join(devcontainerDir, &quot;devcontainer.local.d&quot;), 0700); err != nil {
+		t.Fatal(err)
+	}
+	runtime, err := os.MkdirTemp(&quot;/tmp&quot;, &quot;dchrt&quot;)
+	if err != nil {
+		t.Fatal(err)
+	}
+	defer os.RemoveAll(runtime)
+	t.Setenv(&quot;XDG_RUNTIME_DIR&quot;, runtime)
+	socketDir, err := githubapp.RuntimeDir(project)
+	if err != nil {
+		t.Fatal(err)
+	}
+	fragment, err := githubapp.AdapterFragment(socketDir)
+	if err != nil {
+		t.Fatal(err)
+	}
+	if err := os.WriteFile(filepath.Join(devcontainerDir, &quot;devcontainer.local.d&quot;, &quot;90-plugin.json&quot;), fragment, 0600); err != nil {
+		t.Fatal(err)
+	}
+	if err := os.WriteFile(filepath.Join(devcontainerDir, &quot;devcontainer.json&quot;), []byte(`{&quot;image&quot;:&quot;example:latest&quot;,&quot;postCreateCommand&quot;:&quot;echo keep&quot;}`), 0600); err != nil {
+		t.Fatal(err)
+	}
+	t.Setenv(&quot;DCH_CONFIG&quot;, filepath.Join(root, &quot;dch-config&quot;))
+	if err := common.SaveConfig(common.Config{Version: 1, Projects: []string{project}, ProjectPlugins: map[string]common.ProjectPlugins{project: {Enabled: map[string]githubapp.Config{&quot;github-app&quot;: {AppID: 1}}}}}); err != nil {
+		t.Fatal(err)
+	}
+	bin := filepath.Join(root, &quot;bin&quot;)
+	if err := os.Mkdir(bin, 0700); err != nil {
+		t.Fatal(err)
+	}
+	marker := filepath.Join(root, &quot;devcontainer-called&quot;)
+	script := &quot;#!/bin/sh\nconfig=\nprev=\nfor arg in \&quot;$@\&quot;; do if [ \&quot;$prev\&quot; = --config ]; then config=$arg; fi; prev=$arg; done\n[ -f \&quot;$config\&quot; ] || exit 12\ntouch &quot; + shellQuote(marker) + &quot;\n&quot;
+	if err := os.WriteFile(filepath.Join(bin, &quot;devcontainer&quot;), []byte(script), 0700); err != nil {
+		t.Fatal(err)
+	}
+	t.Setenv(&quot;PATH&quot;, bin+string(os.PathListSeparator)+os.Getenv(&quot;PATH&quot;))
+	if err := invokeProjectDevcontainer(Project{Name: &quot;demo&quot;, Path: project}, &quot;exec&quot;, &quot;--workspace-folder&quot;, project, &quot;true&quot;); err != nil {
+		t.Fatalf(&quot;valid merged config: %v&quot;, err)
+	}
+	if err := os.Remove(marker); err != nil {
+		t.Fatal(err)
+	}
+	conflicts := []string{
+		`{&quot;mounts&quot;:[&quot;type=bind,source=/evil,destination=/run/dch-github&quot;]}`,
+		`{&quot;mounts&quot;:[&quot;type=bind,source=/evil,dst=/run/dch-github/&quot;]}`,
+		`{&quot;mounts&quot;:[&quot;type=bind,source=/evil,target=/run/dch-github/bin&quot;]}`,
+		`{&quot;mounts&quot;:[{&quot;type&quot;:&quot;bind&quot;,&quot;source&quot;:&quot;/evil&quot;,&quot;target&quot;:&quot;/run/dch-github/bin&quot;}]}`,
+		`{&quot;mounts&quot;:[&quot;type=bind,source=/evil,target=/run/dch-github,type=bind,target=/else&quot;]}`,
+		`{&quot;remoteEnv&quot;:{&quot;PATH&quot;:&quot;/usr/bin&quot;}}`,
+	}
+	for _, conflict := range conflicts {
+		if err := os.WriteFile(filepath.Join(devcontainerDir, &quot;devcontainer.local.d&quot;, &quot;99-conflict.json&quot;), []byte(conflict), 0600); err != nil {
+			t.Fatal(err)
+		}
+		if err := invokeProjectDevcontainer(Project{Name: &quot;demo&quot;, Path: project}, &quot;exec&quot;, &quot;--workspace-folder&quot;, project, &quot;true&quot;); err == nil {
+			t.Errorf(&quot;conflicting merged config accepted: %s&quot;, conflict)
+		}
+		if _, err := os.Stat(marker); !os.IsNotExist(err) {
+			t.Fatalf(&quot;devcontainer ran with invalid merged config: %s&quot;, conflict)
+		}
+		if err := os.Remove(filepath.Join(devcontainerDir, &quot;devcontainer.local.d&quot;, &quot;99-conflict.json&quot;)); err != nil {
+			t.Fatal(err)
+		}
+	}
+	if err := os.WriteFile(filepath.Join(devcontainerDir, &quot;devcontainer.json&quot;), []byte(`{&quot;dockerComposeFile&quot;:&quot;compose.yaml&quot;}`), 0600); err != nil {
+		t.Fatal(err)
+	}
+	if err := invokeProjectDevcontainer(Project{Name: &quot;demo&quot;, Path: project}, &quot;exec&quot;, &quot;--workspace-folder&quot;, project, &quot;true&quot;); err == nil {
+		t.Fatal(&quot;Compose configuration was accepted&quot;)
+	}
+}
+
+func TestPluginRejectsRemoteDockerHostAndContext(t *testing.T) {
+	root := t.TempDir()
+	bin := filepath.Join(root, &quot;bin&quot;)
+	if err := os.Mkdir(bin, 0700); err != nil {
+		t.Fatal(err)
+	}
+	docker := filepath.Join(bin, &quot;docker&quot;)
+	if err := os.WriteFile(docker, []byte(&quot;#!/bin/sh\nprintf &#x27;%s&#x27; &#x27;[{\&quot;Endpoints\&quot;:{\&quot;docker\&quot;:{\&quot;Host\&quot;:\&quot;tcp://remote:2375\&quot;}}}]&#x27;\n&quot;), 0700); err != nil {
+		t.Fatal(err)
+	}
+	t.Setenv(&quot;PATH&quot;, bin+string(os.PathListSeparator)+os.Getenv(&quot;PATH&quot;))
+	t.Setenv(&quot;DOCKER_HOST&quot;, &quot;&quot;)
+	t.Setenv(&quot;DOCKER_CONTEXT&quot;, &quot;&quot;)
+	if err := requireLocalDocker(); err == nil || !strings.Contains(err.Error(), &quot;local Docker&quot;) {
+		t.Fatalf(&quot;remote context error=%v&quot;, err)
+	}
+	t.Setenv(&quot;DOCKER_HOST&quot;, &quot;tcp://remote:2375&quot;)
+	if err := requireLocalDocker(); err == nil || !strings.Contains(err.Error(), &quot;local Docker&quot;) {
+		t.Fatalf(&quot;remote DOCKER_HOST error=%v&quot;, err)
+	}
+	t.Setenv(&quot;DOCKER_HOST&quot;, &quot;unix:///run/user/1000/docker.sock&quot;)
+	if err := requireLocalDocker(); err != nil {
+		t.Fatalf(&quot;local socket rejected: %v&quot;, err)
+	}
+	t.Setenv(&quot;DOCKER_CONTEXT&quot;, &quot;remote&quot;)
+	if err := requireLocalDocker(); err == nil || !strings.Contains(err.Error(), &quot;local Docker&quot;) {
+		t.Fatalf(&quot;remote context did not override local DOCKER_HOST: %v&quot;, err)
+	}
+}
+
+func TestStartRunsAdapterInstallAfterUpAndMarksBuildApplied(t *testing.T) {
+	root := t.TempDir()
+	project := filepath.Join(root, &quot;project&quot;)
+	configDir := filepath.Join(project, &quot;.devcontainer&quot;)
+	if err := os.MkdirAll(configDir, 0700); err != nil {
+		t.Fatal(err)
+	}
+	if err := os.WriteFile(filepath.Join(configDir, &quot;devcontainer.json&quot;), []byte(`{&quot;image&quot;:&quot;example:latest&quot;,&quot;postCreateCommand&quot;:&quot;echo keep&quot;}`), 0600); err != nil {
+		t.Fatal(err)
+	}
+	runtime, err := os.MkdirTemp(&quot;/tmp&quot;, &quot;dchrt&quot;)
+	if err != nil {
+		t.Fatal(err)
+	}
+	t.Cleanup(func() { _ = os.RemoveAll(runtime) })
+	configHome := filepath.Join(root, &quot;config-home&quot;)
+	if err := os.Mkdir(configHome, 0700); err != nil {
+		t.Fatal(err)
+	}
+	bin := filepath.Join(root, &quot;bin&quot;)
+	if err := os.Mkdir(bin, 0700); err != nil {
+		t.Fatal(err)
+	}
+	argsLog, configLog, labelsLog := filepath.Join(root, &quot;args.log&quot;), filepath.Join(root, &quot;merged.json&quot;), filepath.Join(root, &quot;labels.log&quot;)
+	scripts := map[string]string{
+		&quot;systemctl&quot;:    &quot;#!/bin/sh\nexit 0\n&quot;,
+		&quot;docker&quot;:       &quot;#!/bin/sh\nprintf &#x27;%s&#x27; &#x27;[{\&quot;Endpoints\&quot;:{\&quot;docker\&quot;:{\&quot;Host\&quot;:\&quot;unix:///var/run/docker.sock\&quot;}}}]&#x27;\n&quot;,
+		&quot;devcontainer&quot;: &quot;#!/bin/sh\nprintf &#x27;%s\\n&#x27; \&quot;$1\&quot; &gt;&gt; &quot; + shellQuote(argsLog) + &quot;\nif [ \&quot;$1\&quot; = exec ] &amp;&amp; [ \&quot;$DCH_TEST_FAIL_INSTALL\&quot; = yes ]; then echo &#x27;dch: required command missing: gh&#x27; &gt;&amp;2; exit 13; fi\nconfig=\nprev=\nfor arg in \&quot;$@\&quot;; do case \&quot;$arg\&quot; in devcontainer.local_folder=*|devcontainer.config_file=*) printf &#x27;%s\\n&#x27; \&quot;$arg\&quot; &gt;&gt; &quot; + shellQuote(labelsLog) + &quot; ;; esac; if [ \&quot;$prev\&quot; = --config ]; then config=$arg; fi; prev=$arg; done\n[ -f \&quot;$config\&quot; ] || exit 12\ncat \&quot;$config\&quot; &gt;&gt; &quot; + shellQuote(configLog) + &quot;\nprintf &#x27;\\n&#x27; &gt;&gt; &quot; + shellQuote(configLog) + &quot;\n&quot;,
+	}
+	for name, content := range scripts {
+		if err := os.WriteFile(filepath.Join(bin, name), []byte(content), 0700); err != nil {
+			t.Fatal(err)
+		}
+	}
+	t.Setenv(&quot;PATH&quot;, bin+string(os.PathListSeparator)+os.Getenv(&quot;PATH&quot;))
+	t.Setenv(&quot;DCH_CONFIG&quot;, filepath.Join(root, &quot;dch-config&quot;))
+	t.Setenv(&quot;XDG_RUNTIME_DIR&quot;, runtime)
+	t.Setenv(&quot;XDG_CONFIG_HOME&quot;, configHome)
+	t.Setenv(&quot;DOCKER_HOST&quot;, &quot;&quot;)
+	t.Setenv(&quot;DOCKER_CONTEXT&quot;, &quot;&quot;)
+	t.Setenv(&quot;DCH_TEST_FAIL_INSTALL&quot;, &quot;&quot;)
+	runtimeDir, err := githubapp.RuntimeDir(project)
+	if err != nil {
+		t.Fatal(err)
+	}
+	socket := filepath.Join(runtimeDir, &quot;credential.sock&quot;)
+	listener, err := net.Listen(&quot;unix&quot;, socket)
+	if err != nil {
+		t.Fatal(err)
+	}
+	credentialServer := &amp;http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
+		w.WriteHeader(http.StatusOK)
+		_, _ = w.Write([]byte(&quot;opaque-token&quot;))
+	})}
+	go credentialServer.Serve(listener)
+	defer credentialServer.Close()
+	cfg := githubapp.Config{AppID: 1, InstallationID: 2, PrivateKeyPassEntry: &quot;test/key&quot;, Repository: &quot;octo/repo&quot;, Permissions: map[string]string{&quot;contents&quot;: &quot;read&quot;}}
+	if err := plugins.NewManager().Enable(context.Background(), project, cfg); err != nil {
+		t.Fatal(err)
+	}
+	devNull, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
+	if err != nil {
+		t.Fatal(err)
+	}
+	oldStdout := os.Stdout
+	os.Stdout = devNull
+	defer func() { os.Stdout = oldStdout; devNull.Close() }()
+	if err := startProject(Project{Name: &quot;demo&quot;, Path: project, State: &quot;NOT CREATED&quot;}); err != nil {
+		t.Fatal(err)
+	}
+	status, err := plugins.NewManager().Status(context.Background(), project)
+	if err != nil {
+		t.Fatal(err)
+	}
+	if status.PendingBuild {
+		t.Fatal(&quot;pending Build remained after successful create and adapter install&quot;)
+	}
+	if err := connectProject(Project{Name: &quot;demo&quot;, Path: project, State: &quot;RUNNING&quot;, ID: &quot;container-id&quot;}); err != nil {
+		t.Fatal(err)
+	}
+	args, err := os.ReadFile(argsLog)
+	if err != nil {
+		t.Fatal(err)
+	}
+	got := string(args)
+	if got != &quot;up\nexec\nexec\nexec\n&quot; {
+		t.Fatalf(&quot;devcontainer actions=%q&quot;, got)
+	}
+	labels, err := os.ReadFile(labelsLog)
+	if err != nil {
+		t.Fatal(err)
+	}
+	for _, label := range []string{&quot;devcontainer.local_folder=&quot; + project, &quot;devcontainer.config_file=&quot; + filepath.Join(configDir, &quot;devcontainer.json&quot;)} {
+		if strings.Count(string(labels), label) != 4 {
+			t.Errorf(&quot;stable label %q count=%d labels=%s&quot;, label, strings.Count(string(labels), label), labels)
+		}
+	}
+	merged, err := os.ReadFile(configLog)
+	if err != nil {
+		t.Fatal(err)
+	}
+	if strings.Count(string(merged), &quot;postCreateCommand&quot;) != 4 || !strings.Contains(string(merged), &quot;/run/dch-github&quot;) {
+		t.Fatalf(&quot;container config did not preserve hooks and plugin mount: %s&quot;, merged)
+	}
+	if err := plugins.NewManager().Enable(context.Background(), project, cfg); err != nil {
+		t.Fatal(err)
+	}
+	t.Setenv(&quot;DCH_TEST_FAIL_INSTALL&quot;, &quot;yes&quot;)
+	err = startProject(Project{Name: &quot;demo&quot;, Path: project, State: &quot;NOT CREATED&quot;})
+	if err == nil || !strings.Contains(err.Error(), &quot;required command missing: gh&quot;) {
+		t.Fatalf(&quot;start error = %v, want actionable adapter install diagnostic&quot;, err)
+	}
+	status, err = plugins.NewManager().Status(context.Background(), project)
+	if err != nil {
+		t.Fatal(err)
+	}
+	if !status.PendingBuild {
+		t.Fatal(&quot;failed adapter install cleared pending Build&quot;)
+	}
+}
+
+func TestRemoveProjectPreservesRegistrationWhenDisableFails(t *testing.T) {
+	root := t.TempDir()
+	project := filepath.Join(root, &quot;project&quot;)
+	if err := os.MkdirAll(project, 0700); err != nil {
+		t.Fatal(err)
+	}
+	bin := filepath.Join(root, &quot;bin&quot;)
+	if err := os.Mkdir(bin, 0700); err != nil {
+		t.Fatal(err)
+	}
+	if err := os.WriteFile(filepath.Join(bin, &quot;systemctl&quot;), []byte(&quot;#!/bin/sh\nshift\nif [ \&quot;$1\&quot; = stop ]; then echo stop-denied &gt;&amp;2; exit 1; fi\nexit 0\n&quot;), 0700); err != nil {
+		t.Fatal(err)
+	}
+	runtime, err := os.MkdirTemp(&quot;/tmp&quot;, &quot;dchrt&quot;)
+	if err != nil {
+		t.Fatal(err)
+	}
+	defer os.RemoveAll(runtime)
+	configHome := filepath.Join(root, &quot;config-home&quot;)
+	if err := os.Mkdir(configHome, 0700); err != nil {
+		t.Fatal(err)
+	}
+	t.Setenv(&quot;PATH&quot;, bin+string(os.PathListSeparator)+os.Getenv(&quot;PATH&quot;))
+	t.Setenv(&quot;XDG_RUNTIME_DIR&quot;, runtime)
+	t.Setenv(&quot;XDG_CONFIG_HOME&quot;, configHome)
+	t.Setenv(&quot;DCH_CONFIG&quot;, filepath.Join(root, &quot;dch-config&quot;))
+	config := common.Config{Version: 1, Projects: []string{project}, ProjectPlugins: map[string]common.ProjectPlugins{project: {Enabled: map[string]githubapp.Config{&quot;github-app&quot;: {AppID: 1}}, States: map[string]common.PluginState{&quot;github-app&quot;: {BuildPending: true}}}}}
+	if err := common.SaveConfig(config); err != nil {
+		t.Fatal(err)
+	}
+	err = removeProject(Project{Name: &quot;demo&quot;, Path: project})
+	if err == nil || !strings.Contains(err.Error(), &quot;stop-denied&quot;) {
+		t.Fatalf(&quot;remove error=%v&quot;, err)
+	}
+	loaded, err := common.LoadConfig()
+	if err != nil {
+		t.Fatal(err)
+	}
+	if len(loaded.Projects) != 1 {
+		t.Fatal(&quot;project was unregistered despite cleanup failure&quot;)
+	}
+	if _, enabled := loaded.ProjectPlugins[project].Enabled[&quot;github-app&quot;]; !enabled {
+		t.Fatal(&quot;enabled plugin settings were removed after cleanup failure&quot;)
+	}
+}
+
+func TestRemoveProjectRefusesUnknownPluginMetadata(t *testing.T) {
+	root := t.TempDir()
+	project := filepath.Join(root, &quot;project&quot;)
+	if err := os.MkdirAll(project, 0700); err != nil {
+		t.Fatal(err)
+	}
+	t.Setenv(&quot;DCH_CONFIG&quot;, filepath.Join(root, &quot;dch-config&quot;))
+	config := common.Config{Version: 1, Projects: []string{project}, ProjectPlugins: map[string]common.ProjectPlugins{project: {States: map[string]common.PluginState{&quot;future-plugin&quot;: {BuildPending: true}}}}}
+	if err := common.SaveConfig(config); err != nil {
+		t.Fatal(err)
+	}
+	err := removeProject(Project{Name: &quot;demo&quot;, Path: project})
+	if err == nil || !strings.Contains(err.Error(), &quot;unknown plugin state&quot;) {
+		t.Fatalf(&quot;remove error=%v&quot;, err)
+	}
+	loaded, err := common.LoadConfig()
+	if err != nil {
+		t.Fatal(err)
+	}
+	if len(loaded.Projects) != 1 {
+		t.Fatal(&quot;unknown plugin data was removed&quot;)
+	}
+}
</code></pre><h4>internal/console/plugins_test.go</h4><pre style="white-space:pre-wrap;overflow-wrap:anywhere"><code>diff --git a/internal/console/plugins_test.go b/internal/console/plugins_test.go
new file mode 100644
index 0000000..03e8d8c
--- /dev/null
+++ b/internal/console/plugins_test.go
@@ -0,0 +1,29 @@
+package console
+
+import (
+	&quot;bufio&quot;
+	&quot;strings&quot;
+	&quot;testing&quot;
+)
+
+func TestPromptGitHubAppConfigCollectsAndValidatesAuthorizedScopes(t *testing.T) {
+	previous := input
+	input = bufio.NewReader(strings.NewReader(&quot;12\n34\ngithub/key\norg/repo\n1\n2\n1\n&quot;))
+	defer func() { input = previous }()
+	cfg, err := promptGitHubAppConfig()
+	if err != nil {
+		t.Fatal(err)
+	}
+	if cfg.AppID != 12 || cfg.InstallationID != 34 || cfg.Repository != &quot;org/repo&quot; || cfg.Permissions[&quot;contents&quot;] != &quot;read&quot; || cfg.Permissions[&quot;pull_requests&quot;] != &quot;write&quot; {
+		t.Fatalf(&quot;unexpected prompt config: %+v&quot;, cfg)
+	}
+}
+
+func TestPromptGitHubAppConfigAllowsCancel(t *testing.T) {
+	previous := input
+	input = bufio.NewReader(strings.NewReader(&quot;q\n&quot;))
+	defer func() { input = previous }()
+	if _, err := promptGitHubAppConfig(); err != errCancelled {
+		t.Fatalf(&quot;cancel error = %v, want errCancelled&quot;, err)
+	}
+}
</code></pre><h4>internal/console/projects_menu_test.go</h4><pre style="white-space:pre-wrap;overflow-wrap:anywhere"><code>diff --git a/internal/console/projects_menu_test.go b/internal/console/projects_menu_test.go
index 6f41898..d912d4f 100644
--- a/internal/console/projects_menu_test.go
+++ b/internal/console/projects_menu_test.go
@@ -10,30 +10,118 @@ import (
 	&quot;reflect&quot;
 	&quot;strings&quot;
 	&quot;testing&quot;
+
+	&quot;dch/internal/common&quot;
+	&quot;dch/internal/githubapp&quot;
 )
 
 func TestAvailableProjectActions(t *testing.T) {
 	cases := []struct {
-		state  string
-		valid  bool
-		docker bool
-		want   []projectAction
+		state   string
+		valid   bool
+		docker  bool
+		pending bool
+		want    []projectAction
 	}{
-		{&quot;RUNNING&quot;, true, true, []projectAction{actionConnect, actionStop, actionBuild, actionConfig, actionRemove, actionBack}},
-		{&quot;RUNNING (2)&quot;, true, true, []projectAction{actionConnect, actionStop, actionBuild, actionConfig, actionRemove, actionBack}},
-		{&quot;STOPPED&quot;, true, true, []projectAction{actionStart, actionBuild, actionConfig, actionRemove, actionBack}},
-		{&quot;NOT CREATED&quot;, true, true, []projectAction{actionStart, actionBuild, actionConfig, actionRemove, actionBack}},
-		{&quot;UNKNOWN&quot;, true, false, []projectAction{actionConfig, actionRemove, actionBack}},
-		{&quot;CONFIG MISSING/INVALID&quot;, false, true, []projectAction{actionConfig, actionRemove, actionBack}},
+		{&quot;RUNNING&quot;, true, true, false, []projectAction{actionConnect, actionStop, actionBuild, actionPlugins, actionConfig, actionRemove, actionBack}},
+		{&quot;RUNNING pending&quot;, true, true, true, []projectAction{actionStop, actionBuild, actionPlugins, actionConfig, actionRemove, actionBack}},
+		{&quot;STOPPED&quot;, true, true, false, []projectAction{actionStart, actionBuild, actionPlugins, actionConfig, actionRemove, actionBack}},
+		{&quot;STOPPED pending&quot;, true, true, true, []projectAction{actionBuild, actionPlugins, actionConfig, actionRemove, actionBack}},
+		{&quot;NOT CREATED&quot;, true, true, true, []projectAction{actionStart, actionBuild, actionPlugins, actionConfig, actionRemove, actionBack}},
+		{&quot;UNKNOWN&quot;, true, false, false, []projectAction{actionPlugins, actionConfig, actionRemove, actionBack}},
+		{&quot;CONFIG MISSING/INVALID&quot;, false, true, false, []projectAction{actionPlugins, actionConfig, actionRemove, actionBack}},
 	}
 	for _, tc := range cases {
-		got := availableProjectActions(Project{State: tc.state, Valid: tc.valid}, tc.docker)
+		got := availableProjectActions(Project{State: tc.state, Valid: tc.valid}, tc.docker, tc.pending)
 		if !reflect.DeepEqual(got, tc.want) {
 			t.Errorf(&quot;state %q: got %v, want %v&quot;, tc.state, got, tc.want)
 		}
 	}
 }
 
+func TestProjectMenuShowsPendingBuildAndPluginsWithoutConnect(t *testing.T) {
+	actions := availableProjectActions(Project{Name: &quot;demo&quot;, State: &quot;RUNNING&quot;, Valid: true}, true, true)
+	previousInput := input
+	input = bufio.NewReader(strings.NewReader(&quot;6\n&quot;))
+	defer func() { input = previousInput }()
+	r, w, err := os.Pipe()
+	if err != nil {
+		t.Fatal(err)
+	}
+	defer r.Close()
+	previousStdout := os.Stdout
+	os.Stdout = w
+	_, chooseErr := chooseProject(Project{Name: &quot;demo&quot;, State: &quot;RUNNING&quot;}, actions, true)
+	w.Close()
+	os.Stdout = previousStdout
+	if chooseErr != nil {
+		t.Fatal(chooseErr)
+	}
+	output, err := io.ReadAll(r)
+	if err != nil {
+		t.Fatal(err)
+	}
+	got := string(output)
+	if !strings.Contains(got, &quot;Build required&quot;) || !strings.Contains(got, &quot;Plugins&quot;) || strings.Contains(got, &quot;Connect&quot;) {
+		t.Fatalf(&quot;unexpected pending menu: %s&quot;, got)
+	}
+}
+
+func TestPluginsStatusIsReachableWithFakeDockerAndSystemctl(t *testing.T) {
+	root := t.TempDir()
+	project, err := os.MkdirTemp(&quot;/tmp&quot;, &quot;dchp&quot;)
+	if err != nil {
+		t.Fatal(err)
+	}
+	t.Cleanup(func() { _ = os.RemoveAll(project) })
+	if err := os.MkdirAll(filepath.Join(project, &quot;.devcontainer&quot;), 0700); err != nil {
+		t.Fatal(err)
+	}
+	if err := os.WriteFile(filepath.Join(project, &quot;.devcontainer&quot;, &quot;devcontainer.json&quot;), []byte(`{&quot;name&quot;:&quot;demo&quot;,&quot;image&quot;:&quot;example:latest&quot;}`), 0600); err != nil {
+		t.Fatal(err)
+	}
+	bin := filepath.Join(root, &quot;bin&quot;)
+	if err := os.Mkdir(bin, 0700); err != nil {
+		t.Fatal(err)
+	}
+	for name, script := range map[string]string{&quot;docker&quot;: &quot;#!/bin/sh\nexit 0\n&quot;, &quot;systemctl&quot;: &quot;#!/bin/sh\nexit 0\n&quot;} {
+		if err := os.WriteFile(filepath.Join(bin, name), []byte(script), 0700); err != nil {
+			t.Fatal(err)
+		}
+	}
+	t.Setenv(&quot;PATH&quot;, bin+string(os.PathListSeparator)+os.Getenv(&quot;PATH&quot;))
+	t.Setenv(&quot;DCH_CONFIG&quot;, filepath.Join(root, &quot;dch-config&quot;))
+	config := common.Config{Version: 1, Projects: []string{project}, ProjectPlugins: map[string]common.ProjectPlugins{project: {Enabled: map[string]githubapp.Config{&quot;github-app&quot;: {AppID: 1}}, States: map[string]common.PluginState{&quot;github-app&quot;: {BuildPending: true}}}}}
+	if err := common.SaveConfig(config); err != nil {
+		t.Fatal(err)
+	}
+	previousInput := input
+	input = bufio.NewReader(strings.NewReader(&quot;1\n3\n1\n4\n6\n2\n&quot;))
+	defer func() { input = previousInput }()
+	r, w, err := os.Pipe()
+	if err != nil {
+		t.Fatal(err)
+	}
+	defer r.Close()
+	previousStdout := os.Stdout
+	os.Stdout = w
+	menuErr := projectsMenu()
+	w.Close()
+	os.Stdout = previousStdout
+	if menuErr != nil {
+		t.Fatal(menuErr)
+	}
+	output, err := io.ReadAll(r)
+	if err != nil {
+		t.Fatal(err)
+	}
+	got := string(output)
+	if !strings.Contains(got, &quot;Build required&quot;) || !strings.Contains(got, &quot;Plugins&quot;) || !strings.Contains(got, &quot;GitHub App: enabled · service active · Build required&quot;) {
+		t.Fatalf(&quot;missing plugin state in menu output: %s&quot;, got)
+	}
+	t.Logf(&quot;console status capture:\n%s&quot;, strings.ReplaceAll(got, project, &quot;/workspace/demo&quot;))
+}
+
 func TestStopOffersOptionalDeleteForMatchingContainers(t *testing.T) {
 	for _, tc := range []struct {
 		name, answer, want string
</code></pre><h4>internal/githubapp/adapter_test.go</h4><pre style="white-space:pre-wrap;overflow-wrap:anywhere"><code>diff --git a/internal/githubapp/adapter_test.go b/internal/githubapp/adapter_test.go
new file mode 100644
index 0000000..6169913
--- /dev/null
+++ b/internal/githubapp/adapter_test.go
@@ -0,0 +1,408 @@
+package githubapp
+
+import (
+	&quot;encoding/json&quot;
+	&quot;fmt&quot;
+	&quot;net&quot;
+	&quot;os&quot;
+	&quot;os/exec&quot;
+	&quot;path/filepath&quot;
+	&quot;reflect&quot;
+	&quot;strings&quot;
+	&quot;testing&quot;
+)
+
+func TestAdapterFragmentMountsRuntimeDirectoryAndAddsAdapterPath(t *testing.T) {
+	fragment, err := AdapterFragment(&quot;/run/user/1000/dch/project&quot;)
+	if err != nil {
+		t.Fatal(err)
+	}
+	var config struct {
+		Mounts    []string          `json:&quot;mounts&quot;`
+		RemoteEnv map[string]string `json:&quot;remoteEnv&quot;`
+	}
+	if err := json.Unmarshal(fragment, &amp;config); err != nil {
+		t.Fatal(err)
+	}
+	wantMount := &quot;source=/run/user/1000/dch/project,target=/run/dch-github,type=bind,readonly&quot;
+	if !reflect.DeepEqual(config.Mounts, []string{wantMount}) {
+		t.Fatalf(&quot;mounts = %#v, want %#v&quot;, config.Mounts, []string{wantMount})
+	}
+	if got, want := config.RemoteEnv[&quot;PATH&quot;], &quot;/run/dch-github/bin:${containerEnv:PATH}&quot;; got != want {
+		t.Fatalf(&quot;PATH = %q, want %q&quot;, got, want)
+	}
+}
+
+func TestAdapterFragmentRejectsUnrepresentableRuntimePaths(t *testing.T) {
+	for _, socketDir := range []string{&quot;relative/path&quot;, &quot;/&quot;, &quot;/runtime/with,comma&quot;, &quot;/runtime/with\nnewline&quot;, &quot;/runtime/${interpolation}&quot;} {
+		if _, err := AdapterFragment(socketDir); err == nil {
+			t.Errorf(&quot;AdapterFragment(%q) succeeded, want invalid path error&quot;, socketDir)
+		}
+	}
+}
+
+func TestWriteAdapterScriptsCreatesPrivateExecutableScriptsAndRefreshesThem(t *testing.T) {
+	runtimeDir := t.TempDir()
+	if err := os.Chmod(runtimeDir, 0700); err != nil {
+		t.Fatal(err)
+	}
+	if err := WriteAdapterScripts(runtimeDir); err != nil {
+		t.Fatal(err)
+	}
+	gitConfig := filepath.Join(runtimeDir, &quot;gitconfig&quot;)
+	gitConfigInfo, err := os.Stat(gitConfig)
+	if err != nil {
+		t.Fatal(err)
+	}
+	if gitConfigInfo.Mode().Perm() != 0600 {
+		t.Fatalf(&quot;gitconfig mode = %v, want 0600&quot;, gitConfigInfo.Mode())
+	}
+	gitConfigContents, err := os.ReadFile(gitConfig)
+	if err != nil || !strings.Contains(string(gitConfigContents), &quot;helper = &quot;) || !strings.Contains(string(gitConfigContents), &quot;git-credential-dch-github&quot;) {
+		t.Fatalf(&quot;gitconfig = %q, error %v&quot;, gitConfigContents, err)
+	}
+	for name, want := range map[string]string{
+		&quot;git-credential-dch-github&quot;: &quot;host=*&quot;,
+		&quot;gh&quot;:                        &quot;GH_TOKEN=$token&quot;,
+	} {
+		path := filepath.Join(runtimeDir, &quot;bin&quot;, name)
+		info, err := os.Stat(path)
+		if err != nil {
+			t.Fatal(err)
+		}
+		if !info.Mode().IsRegular() || info.Mode().Perm() != 0700 {
+			t.Fatalf(&quot;%s mode = %v, want regular 0700&quot;, name, info.Mode())
+		}
+		contents, err := os.ReadFile(path)
+		if err != nil {
+			t.Fatal(err)
+		}
+		if !strings.Contains(string(contents), want) {
+			t.Fatalf(&quot;%s does not contain %q&quot;, name, want)
+		}
+	}
+	if err := WriteAdapterScripts(runtimeDir); err != nil {
+		t.Fatalf(&quot;second install: %v&quot;, err)
+	}
+}
+
+func TestWriteAdapterScriptsRejectsUnsafeRuntimeEntries(t *testing.T) {
+	runtimeDir := t.TempDir()
+	if err := os.Chmod(runtimeDir, 0700); err != nil {
+		t.Fatal(err)
+	}
+	binDir := filepath.Join(runtimeDir, &quot;bin&quot;)
+	if err := os.Mkdir(binDir, 0700); err != nil {
+		t.Fatal(err)
+	}
+	target := filepath.Join(runtimeDir, &quot;target&quot;)
+	if err := os.WriteFile(target, []byte(&quot;untouched&quot;), 0600); err != nil {
+		t.Fatal(err)
+	}
+	if err := os.Symlink(target, filepath.Join(binDir, &quot;gh&quot;)); err != nil {
+		t.Fatal(err)
+	}
+	if err := WriteAdapterScripts(runtimeDir); err == nil {
+		t.Fatal(&quot;expected unsafe symlink rejection&quot;)
+	}
+	contents, err := os.ReadFile(target)
+	if err != nil || string(contents) != &quot;untouched&quot; {
+		t.Fatalf(&quot;symlink target = %q, error %v&quot;, contents, err)
+	}
+}
+
+func TestAdapterInstallCommandChecksRuntimeAndConfiguresScopedHelper(t *testing.T) {
+	command := AdapterInstallCommand()
+	if !reflect.DeepEqual(command[:2], []string{&quot;/bin/sh&quot;, &quot;-c&quot;}) {
+		t.Fatalf(&quot;install command = %#v&quot;, command)
+	}
+	script := command[2]
+	for _, expected := range []string{
+		&quot;command -v \&quot;$dependency\&quot;&quot;,
+		&quot;PATH=$clean_path&quot;,
+		&quot;curl --disable --noproxy &#x27;*&#x27; --unix-socket /run/dch-github/credential.sock&quot;,
+		&quot;--fixed-value --get-all include.path&quot;,
+		&quot;--add include.path&quot;,
+	} {
+		if !strings.Contains(script, expected) {
+			t.Errorf(&quot;install script does not contain %q&quot;, expected)
+		}
+	}
+	if strings.Contains(script, &quot;--replace-all credential.https://github.com.helper&quot;) {
+		t.Fatal(&quot;install command replaces existing GitHub credential helpers&quot;)
+	}
+	if strings.Index(script, &quot;PATH=$clean_path&quot;) &gt; strings.Index(script, &quot;command -v \&quot;$dependency\&quot;&quot;) {
+		t.Fatal(&quot;install command checks gh before removing the adapter wrapper from PATH&quot;)
+	}
+}
+
+func TestAdapterInstallCommandReportsUIDAndServicePermissionFailures(t *testing.T) {
+	for _, testCase := range []struct {
+		name         string
+		dirMode      os.FileMode
+		socketMode   os.FileMode
+		curlExit     int
+		wantMessage  string
+		wantCurlCall bool
+	}{
+		{name: &quot;runtime directory&quot;, dirMode: 0600, socketMode: 0600, wantMessage: &quot;align the container UID with the host user&quot;},
+		{name: &quot;socket permissions&quot;, dirMode: 0700, socketMode: 0400, wantMessage: &quot;align the container UID with the host user&quot;},
+		{name: &quot;health request&quot;, dirMode: 0700, socketMode: 0600, curlExit: 22, wantMessage: &quot;service unavailable or socket permission denied&quot;, wantCurlCall: true},
+	} {
+		t.Run(testCase.name, func(t *testing.T) {
+			if os.Geteuid() == 0 &amp;&amp; testCase.name != &quot;health request&quot; {
+				t.Skip(&quot;root can bypass Unix DAC permission checks&quot;)
+			}
+			root := t.TempDir()
+			runtimeDir := filepath.Join(root, &quot;runtime&quot;)
+			if err := os.Mkdir(runtimeDir, 0700); err != nil {
+				t.Fatal(err)
+			}
+			if err := WriteAdapterScripts(runtimeDir); err != nil {
+				t.Fatal(err)
+			}
+			socketPath := filepath.Join(runtimeDir, &quot;credential.sock&quot;)
+			listener, err := net.Listen(&quot;unix&quot;, socketPath)
+			if err != nil {
+				t.Fatal(err)
+			}
+			listener.(*net.UnixListener).SetUnlinkOnClose(false)
+			if err := listener.Close(); err != nil {
+				t.Fatal(err)
+			}
+			if err := os.Chmod(socketPath, testCase.socketMode); err != nil {
+				t.Fatal(err)
+			}
+			t.Cleanup(func() { _ = os.Chmod(runtimeDir, 0700) })
+			if err := os.Chmod(runtimeDir, testCase.dirMode); err != nil {
+				t.Fatal(err)
+			}
+			bin := filepath.Join(root, &quot;bin&quot;)
+			if err := os.Mkdir(bin, 0700); err != nil {
+				t.Fatal(err)
+			}
+			curlLog := filepath.Join(root, &quot;curl.log&quot;)
+			writeExecutable(t, filepath.Join(bin, &quot;git&quot;), &quot;#!/bin/sh\nexit 0\n&quot;)
+			writeExecutable(t, filepath.Join(bin, &quot;gh&quot;), &quot;#!/bin/sh\nexit 0\n&quot;)
+			writeExecutable(t, filepath.Join(bin, &quot;sh&quot;), &quot;#!/bin/sh\nexit 0\n&quot;)
+			writeExecutable(t, filepath.Join(bin, &quot;curl&quot;), fmt.Sprintf(&quot;#!/bin/sh\nprintf called &gt; %q\nexit %d\n&quot;, curlLog, testCase.curlExit))
+
+			command := AdapterInstallCommand()
+			script := strings.ReplaceAll(command[2], &quot;/run/dch-github&quot;, runtimeDir)
+			cmd := exec.Command(command[0], command[1], script)
+			cmd.Env = []string{&quot;PATH=&quot; + filepath.Join(runtimeDir, &quot;bin&quot;) + &quot;:&quot; + bin}
+			output, err := cmd.CombinedOutput()
+			if err == nil || !strings.Contains(string(output), testCase.wantMessage) {
+				t.Fatalf(&quot;installer output = %q, error %v; want failure mentioning %q&quot;, output, err, testCase.wantMessage)
+			}
+			_, curlErr := os.Stat(curlLog)
+			if testCase.wantCurlCall &amp;&amp; curlErr != nil {
+				t.Fatalf(&quot;curl was not called, stat error %v&quot;, curlErr)
+			}
+			if !testCase.wantCurlCall &amp;&amp; !os.IsNotExist(curlErr) {
+				t.Fatalf(&quot;curl was called before permission checks, stat error %v&quot;, curlErr)
+			}
+		})
+	}
+}
+
+func TestGitCredentialAdapterScopesTokenAndPreservesOtherHosts(t *testing.T) {
+	root := t.TempDir()
+	runtimeDir := filepath.Join(root, &quot;runtime&quot;)
+	if err := os.Mkdir(runtimeDir, 0700); err != nil {
+		t.Fatal(err)
+	}
+	if err := WriteAdapterScripts(runtimeDir); err != nil {
+		t.Fatal(err)
+	}
+	adapterConfigPath := filepath.Join(runtimeDir, &quot;gitconfig&quot;)
+	adapterConfig, err := os.ReadFile(adapterConfigPath)
+	if err != nil {
+		t.Fatal(err)
+	}
+	adapterConfig = []byte(strings.ReplaceAll(string(adapterConfig), &quot;/run/dch-github&quot;, runtimeDir))
+	if err := os.WriteFile(adapterConfigPath, adapterConfig, 0600); err != nil {
+		t.Fatal(err)
+	}
+	bin := filepath.Join(root, &quot;bin&quot;)
+	if err := os.Mkdir(bin, 0700); err != nil {
+		t.Fatal(err)
+	}
+	curlLog := filepath.Join(root, &quot;curl.log&quot;)
+	writeExecutable(t, filepath.Join(bin, &quot;curl&quot;), fmt.Sprintf(&quot;#!/bin/sh\nprintf &#x27;%%s\\n&#x27; \&quot;$*\&quot; &gt;&gt; %q\nprintf &#x27;fresh-token&#x27;\n&quot;, curlLog))
+	otherHelper := filepath.Join(root, &quot;other-helper&quot;)
+	writeExecutable(t, otherHelper, &quot;#!/bin/sh\n[ \&quot;${1-}\&quot; = get ] || exit 0\nprotocol=; host=\nwhile IFS= read -r line; do case \&quot;$line\&quot; in protocol=*) protocol=${line#protocol=} ;; host=*) host=${line#host=} ;; esac; done\n[ \&quot;$protocol\&quot; = https ] &amp;&amp; [ \&quot;$host\&quot; = github.com ] &amp;&amp; exit 0\nprintf &#x27;username=other-user\\npassword=other-token\\n\\n&#x27;\n&quot;)
+	previousGithubHelper := filepath.Join(root, &quot;previous-github-helper&quot;)
+	writeExecutable(t, previousGithubHelper, &quot;#!/bin/sh\n[ \&quot;${1-}\&quot; = get ] || exit 0\nprintf &#x27;username=old-user\\npassword=old-token\\n\\n&#x27;\n&quot;)
+	config := filepath.Join(root, &quot;gitconfig&quot;)
+	for _, args := range [][]string{
+		{&quot;--file&quot;, config, &quot;--add&quot;, &quot;credential.helper&quot;, otherHelper},
+		{&quot;--file&quot;, config, &quot;--add&quot;, &quot;credential.https://github.com.helper&quot;, previousGithubHelper},
+	} {
+		if output, err := exec.Command(&quot;git&quot;, append([]string{&quot;config&quot;}, args...)...).CombinedOutput(); err != nil {
+			t.Fatalf(&quot;git config: %v: %s&quot;, err, output)
+		}
+	}
+	includePath := filepath.Join(runtimeDir, &quot;gitconfig&quot;)
+	for range 2 {
+		args := []string{&quot;config&quot;, &quot;--file&quot;, config, &quot;--fixed-value&quot;, &quot;--get-all&quot;, &quot;include.path&quot;, includePath}
+		if err := exec.Command(&quot;git&quot;, args...).Run(); err != nil {
+			if output, addErr := exec.Command(&quot;git&quot;, &quot;config&quot;, &quot;--file&quot;, config, &quot;--add&quot;, &quot;include.path&quot;, includePath).CombinedOutput(); addErr != nil {
+				t.Fatalf(&quot;add include path: %v: %s&quot;, addErr, output)
+			}
+		}
+	}
+	fill := func(configPath, url string) (string, error) {
+		cmd := exec.Command(&quot;git&quot;, &quot;credential&quot;, &quot;fill&quot;)
+		cmd.Env = append(os.Environ(), &quot;GIT_CONFIG_GLOBAL=&quot;+configPath, &quot;HOME=&quot;+root, &quot;PATH=&quot;+bin+&quot;:&quot;+os.Getenv(&quot;PATH&quot;))
+		cmd.Stdin = strings.NewReader(&quot;url=&quot; + url + &quot;\n\n&quot;)
+		output, err := cmd.CombinedOutput()
+		return string(output), err
+	}
+	github, err := fill(config, &quot;https://github.com/example/repo&quot;)
+	if err != nil || !strings.Contains(github, &quot;username=x-access-token&quot;) || !strings.Contains(github, &quot;password=fresh-token&quot;) {
+		t.Fatalf(&quot;github credentials = %q, error %v&quot;, github, err)
+	}
+	other, err := fill(config, &quot;https://gitlab.com/example/repo&quot;)
+	if err != nil || !strings.Contains(other, &quot;username=other-user&quot;) || !strings.Contains(other, &quot;password=other-token&quot;) {
+		t.Fatalf(&quot;other host credentials = %q, error %v&quot;, other, err)
+	}
+	for _, url := range []string{&quot;http://github.com/example/repo&quot;, &quot;https://github.com.evil/example/repo&quot;} {
+		fallback, err := fill(config, url)
+		if err != nil || !strings.Contains(fallback, &quot;password=other-token&quot;) {
+			t.Fatalf(&quot;credentials for %s = %q, error %v&quot;, url, fallback, err)
+		}
+	}
+	fallbackConfig := filepath.Join(root, &quot;gitconfig-without-adapter&quot;)
+	for _, args := range [][]string{
+		{&quot;--file&quot;, fallbackConfig, &quot;--add&quot;, &quot;credential.helper&quot;, otherHelper},
+		{&quot;--file&quot;, fallbackConfig, &quot;--add&quot;, &quot;credential.https://github.com.helper&quot;, previousGithubHelper},
+		{&quot;--file&quot;, fallbackConfig, &quot;--add&quot;, &quot;include.path&quot;, filepath.Join(runtimeDir, &quot;missing-gitconfig&quot;)},
+	} {
+		if output, err := exec.Command(&quot;git&quot;, append([]string{&quot;config&quot;}, args...)...).CombinedOutput(); err != nil {
+			t.Fatalf(&quot;git config: %v: %s&quot;, err, output)
+		}
+	}
+	previous, err := fill(fallbackConfig, &quot;https://github.com/example/repo&quot;)
+	if err != nil || !strings.Contains(previous, &quot;password=old-token&quot;) {
+		t.Fatalf(&quot;credentials after runtime config removal = %q, error %v&quot;, previous, err)
+	}
+	for _, operation := range []string{&quot;approve&quot;, &quot;reject&quot;} {
+		cmd := exec.Command(&quot;git&quot;, &quot;credential&quot;, operation)
+		cmd.Env = append(os.Environ(), &quot;GIT_CONFIG_GLOBAL=&quot;+config, &quot;HOME=&quot;+root, &quot;PATH=&quot;+bin+&quot;:&quot;+os.Getenv(&quot;PATH&quot;))
+		cmd.Stdin = strings.NewReader(&quot;protocol=https\nhost=github.com\nusername=manual\npassword=manual-secret\n\n&quot;)
+		if output, err := cmd.CombinedOutput(); err != nil {
+			t.Fatalf(&quot;git credential %s: %v: %s&quot;, operation, err, output)
+		}
+	}
+	calls, err := os.ReadFile(curlLog)
+	if err != nil || !strings.HasPrefix(string(calls), &quot;--disable --noproxy * --unix-socket /run/dch-github/credential.sock&quot;) || !strings.Contains(string(calls), &quot;http://localhost/credential&quot;) {
+		t.Fatalf(&quot;curl calls = %q, error %v&quot;, calls, err)
+	}
+}
+
+func TestGHWrapperFetchesEachTimeAndPreservesArgumentsAndExitStatus(t *testing.T) {
+	root := t.TempDir()
+	runtimeDir := filepath.Join(root, &quot;runtime&quot;)
+	if err := os.Mkdir(runtimeDir, 0700); err != nil {
+		t.Fatal(err)
+	}
+	if err := WriteAdapterScripts(runtimeDir); err != nil {
+		t.Fatal(err)
+	}
+	bin := filepath.Join(root, &quot;bin&quot;)
+	if err := os.Mkdir(bin, 0700); err != nil {
+		t.Fatal(err)
+	}
+	counter := filepath.Join(root, &quot;counter&quot;)
+	curlLog := filepath.Join(root, &quot;curl.log&quot;)
+	writeExecutable(t, filepath.Join(bin, &quot;curl&quot;), fmt.Sprintf(&quot;#!/bin/sh\nprintf &#x27;%%s\\n&#x27; \&quot;$*\&quot; &gt;&gt; %q\nn=0; [ ! -f %q ] || n=$(cat %q); n=$((n + 1)); printf &#x27;%%s&#x27; \&quot;$n\&quot; &gt; %q; printf &#x27;token-%%s&#x27; \&quot;$n\&quot;\n&quot;, curlLog, counter, counter, counter))
+	ghLog := filepath.Join(root, &quot;gh.log&quot;)
+	writeExecutable(t, filepath.Join(bin, &quot;gh&quot;), fmt.Sprintf(&quot;#!/bin/sh\nprintf &#x27;%%s\\n&#x27; \&quot;$GH_TOKEN\&quot; &gt;&gt; %q\nprintf &#x27;&lt;%%s&gt;\\n&#x27; \&quot;$@\&quot; &gt;&gt; %q\nexit 23\n&quot;, ghLog, ghLog))
+	wrapper := filepath.Join(runtimeDir, &quot;bin&quot;, &quot;gh&quot;)
+	for range 2 {
+		cmd := exec.Command(wrapper, &quot;pr&quot;, &quot;create&quot;, &quot;--title&quot;, &quot;a title&quot;)
+		cmd.Env = append(os.Environ(), &quot;PATH=/run/dch-github/bin:&quot;+bin+&quot;:/run/dch-github/bin:&quot;+os.Getenv(&quot;PATH&quot;), &quot;GH_TOKEN=ambient-token&quot;)
+		if err := cmd.Run(); err == nil {
+			t.Fatal(&quot;expected wrapped gh exit 23&quot;)
+		} else if exit, ok := err.(*exec.ExitError); !ok || exit.ExitCode() != 23 {
+			t.Fatalf(&quot;gh exit = %v, want 23&quot;, err)
+		}
+	}
+	contents, err := os.ReadFile(ghLog)
+	if err != nil {
+		t.Fatal(err)
+	}
+	want := &quot;token-1\n&lt;pr&gt;\n&lt;create&gt;\n&lt;--title&gt;\n&lt;a title&gt;\ntoken-2\n&lt;pr&gt;\n&lt;create&gt;\n&lt;--title&gt;\n&lt;a title&gt;\n&quot;
+	if string(contents) != want {
+		t.Fatalf(&quot;gh invocations = %q, want %q&quot;, contents, want)
+	}
+	curlArgs, err := os.ReadFile(curlLog)
+	if err != nil {
+		t.Fatal(err)
+	}
+	if !strings.HasPrefix(string(curlArgs), &quot;--disable --noproxy * --unix-socket /run/dch-github/credential.sock&quot;) {
+		t.Fatalf(&quot;curl does not begin with fixed config and proxy options: %q&quot;, curlArgs)
+	}
+	for _, required := range []string{&quot;--disable&quot;, &quot;--noproxy *&quot;, &quot;--unix-socket /run/dch-github/credential.sock&quot;, &quot;--max-time 20&quot;, &quot;http://localhost/credential&quot;} {
+		if !strings.Contains(string(curlArgs), required) {
+			t.Errorf(&quot;curl arguments %q missing %q&quot;, curlArgs, required)
+		}
+	}
+}
+
+func TestGHWrapperDoesNotRunGHWithoutCredential(t *testing.T) {
+	for _, curlBody := range []string{&quot;exit 22&quot;, &quot;exit 0&quot;} {
+		t.Run(curlBody, func(t *testing.T) {
+			root := t.TempDir()
+			runtimeDir := filepath.Join(root, &quot;runtime&quot;)
+			if err := os.Mkdir(runtimeDir, 0700); err != nil {
+				t.Fatal(err)
+			}
+			if err := WriteAdapterScripts(runtimeDir); err != nil {
+				t.Fatal(err)
+			}
+			bin := filepath.Join(root, &quot;bin&quot;)
+			if err := os.Mkdir(bin, 0700); err != nil {
+				t.Fatal(err)
+			}
+			writeExecutable(t, filepath.Join(bin, &quot;curl&quot;), &quot;#!/bin/sh\n&quot;+curlBody+&quot;\n&quot;)
+			ghLog := filepath.Join(root, &quot;gh.log&quot;)
+			writeExecutable(t, filepath.Join(bin, &quot;gh&quot;), fmt.Sprintf(&quot;#!/bin/sh\nprintf &#x27;called&#x27; &gt; %q\n&quot;, ghLog))
+			cmd := exec.Command(filepath.Join(runtimeDir, &quot;bin&quot;, &quot;gh&quot;), &quot;pr&quot;, &quot;create&quot;)
+			cmd.Env = append(os.Environ(), &quot;PATH=/run/dch-github/bin:&quot;+bin+&quot;:&quot;+os.Getenv(&quot;PATH&quot;))
+			if err := cmd.Run(); err == nil {
+				t.Fatal(&quot;expected credential failure&quot;)
+			} else if exit, ok := err.(*exec.ExitError); !ok || exit.ExitCode() != 1 {
+				t.Fatalf(&quot;wrapper exit = %v, want 1&quot;, err)
+			}
+			if _, err := os.Stat(ghLog); !os.IsNotExist(err) {
+				t.Fatalf(&quot;gh ran without credential, stat error %v&quot;, err)
+			}
+		})
+	}
+}
+
+func TestGHWrapperReportsMissingUnderlyingGHBeforeRequestingCredential(t *testing.T) {
+	runtimeDir := t.TempDir()
+	if err := os.Chmod(runtimeDir, 0700); err != nil {
+		t.Fatal(err)
+	}
+	if err := WriteAdapterScripts(runtimeDir); err != nil {
+		t.Fatal(err)
+	}
+	cmd := exec.Command(filepath.Join(runtimeDir, &quot;bin&quot;, &quot;gh&quot;), &quot;version&quot;)
+	cmd.Env = []string{&quot;PATH=/run/dch-github/bin&quot;}
+	output, err := cmd.CombinedOutput()
+	exit, ok := err.(*exec.ExitError)
+	if !ok || exit.ExitCode() != 127 || !strings.Contains(string(output), &quot;dch: gh is required&quot;) {
+		t.Fatalf(&quot;missing gh result = %q, error %v&quot;, output, err)
+	}
+}
+
+func writeExecutable(t *testing.T, path, content string) {
+	t.Helper()
+	if err := os.WriteFile(path, []byte(content), 0700); err != nil {
+		t.Fatal(err)
+	}
+}
</code></pre><h4>internal/githubapp/config_test.go</h4><pre style="white-space:pre-wrap;overflow-wrap:anywhere"><code>diff --git a/internal/githubapp/config_test.go b/internal/githubapp/config_test.go
new file mode 100644
index 0000000..f452a62
--- /dev/null
+++ b/internal/githubapp/config_test.go
@@ -0,0 +1,31 @@
+package githubapp
+
+import &quot;testing&quot;
+
+func TestConfigValidateAcceptsSingleRepositoryAndSupportedPermissions(t *testing.T) {
+	cfg := Config{AppID: 12, InstallationID: 34, PrivateKeyPassEntry: &quot;dch/project/key&quot;, Repository: &quot;octo/repo&quot;, Permissions: map[string]string{&quot;contents&quot;: &quot;read&quot;, &quot;pull_requests&quot;: &quot;write&quot;}}
+	if err := cfg.Validate(); err != nil {
+		t.Fatalf(&quot;Validate() error = %v&quot;, err)
+	}
+}
+
+func TestConfigValidateAllowsGitHubRepositoryPunctuation(t *testing.T) {
+	cfg := Config{AppID: 1, InstallationID: 2, PrivateKeyPassEntry: &quot;valid/key&quot;, Repository: &quot;octo/devcontainer_helper.test&quot;, Permissions: map[string]string{&quot;contents&quot;: &quot;read&quot;}}
+	if err := cfg.Validate(); err != nil {
+		t.Fatalf(&quot;Validate() error = %v&quot;, err)
+	}
+}
+
+func TestConfigValidateRejectsUnsafeOrBroaderConfiguration(t *testing.T) {
+	cases := []Config{
+		{AppID: 1, InstallationID: 2, PrivateKeyPassEntry: &quot;-bad&quot;, Repository: &quot;octo/repo&quot;, Permissions: map[string]string{&quot;contents&quot;: &quot;read&quot;}},
+		{AppID: 1, InstallationID: 2, PrivateKeyPassEntry: &quot;valid/key&quot;, Repository: &quot;octo/repo/other&quot;, Permissions: map[string]string{&quot;contents&quot;: &quot;read&quot;}},
+		{AppID: 1, InstallationID: 2, PrivateKeyPassEntry: &quot;valid/key&quot;, Repository: &quot;octo/repo&quot;, Permissions: map[string]string{&quot;issues&quot;: &quot;write&quot;}},
+		{AppID: 1, InstallationID: 2, PrivateKeyPassEntry: &quot;valid/key&quot;, Repository: &quot;octo/repo&quot;, Permissions: map[string]string{&quot;contents&quot;: &quot;admin&quot;}},
+	}
+	for _, cfg := range cases {
+		if err := cfg.Validate(); err == nil {
+			t.Errorf(&quot;Validate() accepted %#v&quot;, cfg)
+		}
+	}
+}
</code></pre><h4>internal/githubapp/provider_test.go</h4><pre style="white-space:pre-wrap;overflow-wrap:anywhere"><code>diff --git a/internal/githubapp/provider_test.go b/internal/githubapp/provider_test.go
new file mode 100644
index 0000000..2d08a58
--- /dev/null
+++ b/internal/githubapp/provider_test.go
@@ -0,0 +1,395 @@
+package githubapp
+
+import (
+	&quot;context&quot;
+	&quot;crypto&quot;
+	&quot;crypto/rand&quot;
+	&quot;crypto/rsa&quot;
+	&quot;crypto/sha256&quot;
+	&quot;crypto/x509&quot;
+	&quot;encoding/base64&quot;
+	&quot;encoding/json&quot;
+	&quot;encoding/pem&quot;
+	&quot;fmt&quot;
+	&quot;net/http&quot;
+	&quot;net/http/httptest&quot;
+	&quot;os&quot;
+	&quot;path/filepath&quot;
+	&quot;strings&quot;
+	&quot;sync&quot;
+	&quot;sync/atomic&quot;
+	&quot;testing&quot;
+	&quot;time&quot;
+)
+
+func testPrivateKey(t *testing.T) string {
+	t.Helper()
+	key, err := rsa.GenerateKey(rand.Reader, 2048)
+	if err != nil {
+		t.Fatal(err)
+	}
+	return string(pem.EncodeToMemory(&amp;pem.Block{Type: &quot;RSA PRIVATE KEY&quot;, Bytes: x509.MarshalPKCS1PrivateKey(key)}))
+}
+
+func TestProviderMintsScopedTokenAndCachesIt(t *testing.T) {
+	key := testPrivateKey(t)
+	t.Setenv(&quot;GPG_TTY&quot;, &quot;/dev/pts/9&quot;)
+	t.Setenv(&quot;PASSWORD_STORE_GPG_OPTS&quot;, &quot;--no-tty --pinentry-mode loopback&quot;)
+	privateKey, err := parsePrivateKey([]byte(key))
+	if err != nil {
+		t.Fatal(err)
+	}
+	now := time.Date(2029, 12, 31, 0, 0, 0, 0, time.UTC)
+	var tokenCalls, repoCalls int
+	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
+		if r.URL.Path == &quot;/repos/octo/repo/installation&quot; {
+			repoCalls++
+			if r.Header.Get(&quot;Authorization&quot;) == &quot;&quot; || !strings.HasPrefix(r.Header.Get(&quot;Authorization&quot;), &quot;Bearer &quot;) {
+				t.Error(&quot;installation lookup did not use App JWT&quot;)
+			}
+			_, _ = w.Write([]byte(`{&quot;id&quot;:34}`))
+			return
+		}
+		if r.URL.Path == &quot;/app/installations/34/access_tokens&quot; {
+			tokenCalls++
+			jwt := strings.TrimPrefix(r.Header.Get(&quot;Authorization&quot;), &quot;Bearer &quot;)
+			parts := strings.Split(jwt, &quot;.&quot;)
+			if len(parts) != 3 {
+				t.Fatalf(&quot;invalid JWT: %q&quot;, jwt)
+			}
+			headerJSON, _ := base64.RawURLEncoding.DecodeString(parts[0])
+			var header map[string]string
+			_ = json.Unmarshal(headerJSON, &amp;header)
+			if header[&quot;alg&quot;] != &quot;RS256&quot; || header[&quot;typ&quot;] != &quot;JWT&quot; {
+				t.Errorf(&quot;unexpected JWT header: %#v&quot;, header)
+			}
+			unsigned := parts[0] + &quot;.&quot; + parts[1]
+			digest := sha256.Sum256([]byte(unsigned))
+			sig, decodeErr := base64.RawURLEncoding.DecodeString(parts[2])
+			if decodeErr != nil || rsa.VerifyPKCS1v15(&amp;privateKey.PublicKey, crypto.SHA256, digest[:], sig) != nil {
+				t.Error(&quot;JWT signature did not verify&quot;)
+			}
+			claimsJSON, _ := base64.RawURLEncoding.DecodeString(parts[1])
+			var claims map[string]any
+			_ = json.Unmarshal(claimsJSON, &amp;claims)
+			if claims[&quot;iss&quot;] != &quot;12&quot; || claims[&quot;iat&quot;] != float64(now.Unix()-60) || claims[&quot;exp&quot;] != float64(now.Unix()+9*60) {
+				t.Errorf(&quot;unexpected claims: %#v&quot;, claims)
+			}
+			var body map[string]any
+			_ = json.NewDecoder(r.Body).Decode(&amp;body)
+			if body[&quot;repositories&quot;].([]any)[0] != &quot;repo&quot; || body[&quot;permissions&quot;].(map[string]any)[&quot;contents&quot;] != &quot;read&quot; {
+				t.Errorf(&quot;unexpected scope: %#v&quot;, body)
+			}
+			w.Header().Set(&quot;Content-Type&quot;, &quot;application/json&quot;)
+			_, _ = w.Write([]byte(`{&quot;token&quot;:&quot;opaque-token&quot;,&quot;expires_at&quot;:&quot;2030-01-01T00:00:00Z&quot;}`))
+			return
+		}
+		http.NotFound(w, r)
+	}))
+	defer srv.Close()
+	pass := filepath.Join(t.TempDir(), &quot;pass&quot;)
+	fakePass := &quot;#!/bin/sh\n[ \&quot;$1\&quot; = show ] &amp;&amp; [ \&quot;$2\&quot; = key ] || exit 9\n[ -z \&quot;$GPG_TTY\&quot; ] || exit 10\n[ \&quot;$PASSWORD_STORE_GPG_OPTS\&quot; = &#x27;--no-tty --pinentry-mode loopback --batch --pinentry-mode error&#x27; ] || exit 11\ncat &lt;&lt;&#x27;KEY&#x27;\n&quot; + key + &quot;\nKEY\n&quot;
+	if err := os.WriteFile(pass, []byte(fakePass), 0700); err != nil {
+		t.Fatal(err)
+	}
+	cfg := Config{AppID: 12, InstallationID: 34, PrivateKeyPassEntry: &quot;key&quot;, Repository: &quot;octo/repo&quot;, Permissions: map[string]string{&quot;contents&quot;: &quot;read&quot;}}
+	p, err := NewProvider(cfg, ProviderOptions{PassPath: pass, APIBaseURL: srv.URL, HTTPClient: srv.Client(), Now: func() time.Time { return now }})
+	if err != nil {
+		t.Fatal(err)
+	}
+	for i := 0; i &lt; 2; i++ {
+		got, err := p.Token(context.Background())
+		if err != nil || got != &quot;opaque-token&quot; {
+			t.Fatalf(&quot;Token() = %q, %v&quot;, got, err)
+		}
+	}
+	if tokenCalls != 1 || repoCalls != 1 {
+		t.Fatalf(&quot;calls token=%d installation=%d&quot;, tokenCalls, repoCalls)
+	}
+}
+
+func TestProviderRejectsWrongRepositoryInstallationBeforeMinting(t *testing.T) {
+	key := testPrivateKey(t)
+	var tokenCalls, lookupCalls atomic.Int32
+	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
+		if r.URL.Path == &quot;/repos/octo/repo/installation&quot; {
+			lookupCalls.Add(1)
+			_, _ = w.Write([]byte(`{&quot;id&quot;:99}`))
+			return
+		}
+		if r.URL.Path == &quot;/app/installations/34/access_tokens&quot; {
+			tokenCalls.Add(1)
+			_, _ = w.Write([]byte(`{&quot;token&quot;:&quot;wrong-installation-token&quot;,&quot;expires_at&quot;:&quot;2030-01-01T00:00:00Z&quot;}`))
+			return
+		}
+		if r.URL.Path == &quot;/repos/octo/repo&quot; {
+			// GitHub returns public repository metadata without installation scope.
+			_, _ = w.Write([]byte(`{&quot;full_name&quot;:&quot;octo/repo&quot;}`))
+			return
+		}
+		http.NotFound(w, r)
+	}))
+	defer srv.Close()
+	pass := filepath.Join(t.TempDir(), &quot;pass&quot;)
+	if err := os.WriteFile(pass, []byte(&quot;#!/bin/sh\ncat &lt;&lt;&#x27;KEY&#x27;\n&quot;+key+&quot;\nKEY\n&quot;), 0700); err != nil {
+		t.Fatal(err)
+	}
+	p, err := NewProvider(Config{AppID: 1, InstallationID: 34, PrivateKeyPassEntry: &quot;key&quot;, Repository: &quot;octo/repo&quot;, Permissions: map[string]string{&quot;contents&quot;: &quot;read&quot;}}, ProviderOptions{PassPath: pass, APIBaseURL: srv.URL, HTTPClient: srv.Client()})
+	if err != nil {
+		t.Fatal(err)
+	}
+	if token, err := p.Token(context.Background()); err == nil || token != &quot;&quot; {
+		t.Fatalf(&quot;Token() = %q, %v; want rejected installation&quot;, token, err)
+	}
+	if tokenCalls.Load() != 0 {
+		t.Fatalf(&quot;minted %d tokens for wrong installation&quot;, tokenCalls.Load())
+	}
+	if lookupCalls.Load() != 1 {
+		t.Fatalf(&quot;installation lookup calls=%d; want 1&quot;, lookupCalls.Load())
+	}
+}
+
+func TestProviderRejectsFailedRepositoryInstallationLookup(t *testing.T) {
+	for _, tc := range []struct {
+		name, body string
+		status     int
+	}{{&quot;not found&quot;, `{&quot;message&quot;:&quot;Not Found&quot;}`, http.StatusNotFound}, {&quot;malformed&quot;, `{&quot;id&quot;:&quot;bad&quot;}`, http.StatusOK}} {
+		t.Run(tc.name, func(t *testing.T) {
+			key := testPrivateKey(t)
+			var tokenCalls atomic.Int32
+			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
+				if r.URL.Path == &quot;/repos/octo/repo/installation&quot; {
+					w.WriteHeader(tc.status)
+					_, _ = w.Write([]byte(tc.body))
+					return
+				}
+				if strings.HasSuffix(r.URL.Path, &quot;/access_tokens&quot;) {
+					tokenCalls.Add(1)
+					_, _ = w.Write([]byte(`{&quot;token&quot;:&quot;must-not-mint&quot;,&quot;expires_at&quot;:&quot;2030-01-01T00:00:00Z&quot;}`))
+					return
+				}
+				http.NotFound(w, r)
+			}))
+			defer srv.Close()
+			pass := filepath.Join(t.TempDir(), &quot;pass&quot;)
+			if err := os.WriteFile(pass, []byte(&quot;#!/bin/sh\ncat &lt;&lt;&#x27;KEY&#x27;\n&quot;+key+&quot;\nKEY\n&quot;), 0700); err != nil {
+				t.Fatal(err)
+			}
+			p, err := NewProvider(Config{AppID: 1, InstallationID: 34, PrivateKeyPassEntry: &quot;key&quot;, Repository: &quot;octo/repo&quot;, Permissions: map[string]string{&quot;contents&quot;: &quot;read&quot;}}, ProviderOptions{PassPath: pass, APIBaseURL: srv.URL, HTTPClient: srv.Client()})
+			if err != nil {
+				t.Fatal(err)
+			}
+			if token, err := p.Token(context.Background()); err == nil || token != &quot;&quot; {
+				t.Fatalf(&quot;Token() = %q, %v; want lookup failure&quot;, token, err)
+			}
+			if tokenCalls.Load() != 0 {
+				t.Fatalf(&quot;minted %d tokens after failed lookup&quot;, tokenCalls.Load())
+			}
+		})
+	}
+}
+
+func TestProviderRejectsMalformedOrExpiredResponsesAndRetries(t *testing.T) {
+	cases := []struct{ name, response string }{{&quot;malformed&quot;, `{&quot;token&quot;:`}, {&quot;expired&quot;, `{&quot;token&quot;:&quot;old-token&quot;,&quot;expires_at&quot;:&quot;2000-01-01T00:00:00Z&quot;}`}, {&quot;invalid-expiry&quot;, `{&quot;token&quot;:&quot;bad-expiry&quot;,&quot;expires_at&quot;:&quot;tomorrow&quot;}`}, {&quot;invalid-token&quot;, `{&quot;token&quot;:&quot;bad\ntoken&quot;,&quot;expires_at&quot;:&quot;2030-01-01T00:00:00Z&quot;}`}}
+	for _, tc := range cases {
+		t.Run(tc.name, func(t *testing.T) {
+			key := testPrivateKey(t)
+			var calls atomic.Int32
+			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
+				if r.URL.Path == &quot;/app/installations/2/access_tokens&quot; {
+					if calls.Add(1) == 1 {
+						_, _ = w.Write([]byte(tc.response))
+						return
+					}
+					_, _ = w.Write([]byte(`{&quot;token&quot;:&quot;retry-token&quot;,&quot;expires_at&quot;:&quot;2030-01-01T00:00:00Z&quot;}`))
+					return
+				}
+				_, _ = w.Write([]byte(`{&quot;id&quot;:2}`))
+			}))
+			defer srv.Close()
+			pass := filepath.Join(t.TempDir(), &quot;pass&quot;)
+			_ = os.WriteFile(pass, []byte(&quot;#!/bin/sh\ncat &lt;&lt;&#x27;KEY&#x27;\n&quot;+key+&quot;\nKEY\n&quot;), 0700)
+			p, err := NewProvider(Config{AppID: 1, InstallationID: 2, PrivateKeyPassEntry: &quot;key&quot;, Repository: &quot;octo/repo&quot;, Permissions: map[string]string{&quot;contents&quot;: &quot;read&quot;}}, ProviderOptions{PassPath: pass, APIBaseURL: srv.URL, HTTPClient: srv.Client()})
+			if err != nil {
+				t.Fatal(err)
+			}
+			if _, err := p.Token(context.Background()); err == nil {
+				t.Fatal(&quot;invalid response accepted&quot;)
+			}
+			got, err := p.Token(context.Background())
+			if err != nil || got != &quot;retry-token&quot; {
+				t.Fatalf(&quot;retry = %q, %v&quot;, got, err)
+			}
+			if calls.Load() != 2 {
+				t.Fatalf(&quot;API calls=%d&quot;, calls.Load())
+			}
+		})
+	}
+}
+
+func TestProviderSanitizesAPIErrorAndRetriesNextRequest(t *testing.T) {
+	key := testPrivateKey(t)
+	var tokenCalls atomic.Int32
+	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
+		if r.URL.Path == &quot;/app/installations/2/access_tokens&quot; {
+			if tokenCalls.Add(1) == 1 {
+				http.Error(w, &quot;sensitive github body&quot;, http.StatusForbidden)
+				return
+			}
+			_, _ = w.Write([]byte(`{&quot;token&quot;:&quot;retry-token&quot;,&quot;expires_at&quot;:&quot;2030-01-01T00:00:00Z&quot;}`))
+			return
+		}
+		_, _ = w.Write([]byte(`{&quot;id&quot;:2}`))
+	}))
+	defer srv.Close()
+	pass := filepath.Join(t.TempDir(), &quot;pass&quot;)
+	_ = os.WriteFile(pass, []byte(&quot;#!/bin/sh\ncat &lt;&lt;&#x27;KEY&#x27;\n&quot;+key+&quot;\nKEY\n&quot;), 0700)
+	p, err := NewProvider(Config{AppID: 1, InstallationID: 2, PrivateKeyPassEntry: &quot;key&quot;, Repository: &quot;octo/repo&quot;, Permissions: map[string]string{&quot;contents&quot;: &quot;read&quot;}}, ProviderOptions{PassPath: pass, APIBaseURL: srv.URL, HTTPClient: srv.Client()})
+	if err != nil {
+		t.Fatal(err)
+	}
+	_, err = p.Token(context.Background())
+	if err == nil || strings.Contains(err.Error(), &quot;sensitive&quot;) {
+		t.Fatalf(&quot;unsafe error: %v&quot;, err)
+	}
+	got, err := p.Token(context.Background())
+	if err != nil || got != &quot;retry-token&quot; {
+		t.Fatalf(&quot;retry Token() = %q, %v&quot;, got, err)
+	}
+}
+
+func TestProviderRefreshesWhenTokenHasFiveMinutesLeft(t *testing.T) {
+	testProviderRefresh(t, time.Date(2029, 12, 31, 23, 54, 0, 0, time.UTC), 2)
+}
+
+func TestProviderCoalescesConcurrentMints(t *testing.T) {
+	key := testPrivateKey(t)
+	var calls atomic.Int32
+	release := make(chan struct{})
+	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
+		if r.URL.Path == &quot;/app/installations/2/access_tokens&quot; {
+			calls.Add(1)
+			&lt;-release
+			_, _ = w.Write([]byte(`{&quot;token&quot;:&quot;shared-token&quot;,&quot;expires_at&quot;:&quot;2030-01-01T00:00:00Z&quot;}`))
+			return
+		}
+		_, _ = w.Write([]byte(`{&quot;id&quot;:2}`))
+	}))
+	defer srv.Close()
+	pass := filepath.Join(t.TempDir(), &quot;pass&quot;)
+	_ = os.WriteFile(pass, []byte(&quot;#!/bin/sh\ncat &lt;&lt;&#x27;KEY&#x27;\n&quot;+key+&quot;\nKEY\n&quot;), 0700)
+	p, err := NewProvider(Config{AppID: 1, InstallationID: 2, PrivateKeyPassEntry: &quot;key&quot;, Repository: &quot;octo/repo&quot;, Permissions: map[string]string{&quot;contents&quot;: &quot;read&quot;}}, ProviderOptions{PassPath: pass, APIBaseURL: srv.URL, HTTPClient: srv.Client()})
+	if err != nil {
+		t.Fatal(err)
+	}
+	var wg sync.WaitGroup
+	errs := make(chan error, 12)
+	for i := 0; i &lt; 12; i++ {
+		wg.Add(1)
+		go func() {
+			defer wg.Done()
+			token, err := p.Token(context.Background())
+			if err == nil &amp;&amp; token != &quot;shared-token&quot; {
+				err = fmt.Errorf(&quot;token=%q&quot;, token)
+			}
+			errs &lt;- err
+		}()
+	}
+	time.Sleep(25 * time.Millisecond)
+	close(release)
+	wg.Wait()
+	close(errs)
+	for err := range errs {
+		if err != nil {
+			t.Error(err)
+		}
+	}
+	if calls.Load() != 1 {
+		t.Fatalf(&quot;mint calls=%d&quot;, calls.Load())
+	}
+}
+
+func TestProviderCoalescesConcurrentFailureAndAllowsLaterRetry(t *testing.T) {
+	key := testPrivateKey(t)
+	var calls atomic.Int32
+	release := make(chan struct{})
+	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
+		if r.URL.Path == &quot;/app/installations/2/access_tokens&quot; {
+			if calls.Add(1) == 1 {
+				&lt;-release
+				http.Error(w, &quot;private API details&quot;, http.StatusTooManyRequests)
+				return
+			}
+			_, _ = w.Write([]byte(`{&quot;token&quot;:&quot;recovered-token&quot;,&quot;expires_at&quot;:&quot;2030-01-01T00:00:00Z&quot;}`))
+			return
+		}
+		_, _ = w.Write([]byte(`{&quot;id&quot;:2}`))
+	}))
+	defer srv.Close()
+	pass := filepath.Join(t.TempDir(), &quot;pass&quot;)
+	_ = os.WriteFile(pass, []byte(&quot;#!/bin/sh\ncat &lt;&lt;&#x27;KEY&#x27;\n&quot;+key+&quot;\nKEY\n&quot;), 0700)
+	p, err := NewProvider(Config{AppID: 1, InstallationID: 2, PrivateKeyPassEntry: &quot;key&quot;, Repository: &quot;octo/repo&quot;, Permissions: map[string]string{&quot;contents&quot;: &quot;read&quot;}}, ProviderOptions{PassPath: pass, APIBaseURL: srv.URL, HTTPClient: srv.Client()})
+	if err != nil {
+		t.Fatal(err)
+	}
+	var wg sync.WaitGroup
+	errs := make(chan error, 8)
+	for i := 0; i &lt; 8; i++ {
+		wg.Add(1)
+		go func() { defer wg.Done(); _, err := p.Token(context.Background()); errs &lt;- err }()
+	}
+	time.Sleep(25 * time.Millisecond)
+	close(release)
+	wg.Wait()
+	close(errs)
+	for err := range errs {
+		if err == nil {
+			t.Error(&quot;expected shared failed refresh&quot;)
+		}
+	}
+	if calls.Load() != 1 {
+		t.Fatalf(&quot;concurrent failed mint calls=%d&quot;, calls.Load())
+	}
+	got, err := p.Token(context.Background())
+	if err != nil || got != &quot;recovered-token&quot; {
+		t.Fatalf(&quot;retry = %q, %v&quot;, got, err)
+	}
+}
+
+func testProviderRefresh(t *testing.T, now time.Time, expectedCalls int32) {
+	t.Helper()
+	key := testPrivateKey(t)
+	var calls atomic.Int32
+	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
+		if r.URL.Path == &quot;/app/installations/2/access_tokens&quot; {
+			count := calls.Add(1)
+			if count == 1 {
+				_, _ = w.Write([]byte(`{&quot;token&quot;:&quot;refresh-token&quot;,&quot;expires_at&quot;:&quot;2030-01-01T00:00:00Z&quot;}`))
+			} else {
+				_, _ = w.Write([]byte(`{&quot;token&quot;:&quot;refresh-token-2&quot;,&quot;expires_at&quot;:&quot;2030-01-01T01:00:00Z&quot;}`))
+			}
+			return
+		}
+		_, _ = w.Write([]byte(`{&quot;id&quot;:2}`))
+	}))
+	defer srv.Close()
+	pass := filepath.Join(t.TempDir(), &quot;pass&quot;)
+	_ = os.WriteFile(pass, []byte(&quot;#!/bin/sh\ncat &lt;&lt;&#x27;KEY&#x27;\n&quot;+key+&quot;\nKEY\n&quot;), 0700)
+	current := now
+	p, err := NewProvider(Config{AppID: 1, InstallationID: 2, PrivateKeyPassEntry: &quot;key&quot;, Repository: &quot;octo/repo&quot;, Permissions: map[string]string{&quot;contents&quot;: &quot;read&quot;}}, ProviderOptions{PassPath: pass, APIBaseURL: srv.URL, HTTPClient: srv.Client(), Now: func() time.Time { return current }})
+	if err != nil {
+		t.Fatal(err)
+	}
+	_, err = p.Token(context.Background())
+	if err != nil {
+		t.Fatal(err)
+	}
+	current = current.Add(time.Minute)
+	_, err = p.Token(context.Background())
+	if err != nil {
+		t.Fatal(err)
+	}
+	if calls.Load() != expectedCalls {
+		t.Fatalf(&quot;mint calls=%d want %d&quot;, calls.Load(), expectedCalls)
+	}
+}
</code></pre><h4>internal/githubapp/server_test.go</h4><pre style="white-space:pre-wrap;overflow-wrap:anywhere"><code>diff --git a/internal/githubapp/server_test.go b/internal/githubapp/server_test.go
new file mode 100644
index 0000000..9252176
--- /dev/null
+++ b/internal/githubapp/server_test.go
@@ -0,0 +1,186 @@
+package githubapp
+
+import (
+	&quot;context&quot;
+	&quot;io&quot;
+	&quot;net&quot;
+	&quot;net/http&quot;
+	&quot;os&quot;
+	&quot;path/filepath&quot;
+	&quot;strings&quot;
+	&quot;testing&quot;
+	&quot;time&quot;
+)
+
+func TestServerServesCredentialAndHealthOnlyOverUnixSocket(t *testing.T) {
+	sock := filepath.Join(t.TempDir(), &quot;credential.sock&quot;)
+	if err := os.Chmod(filepath.Dir(sock), 0700); err != nil {
+		t.Fatal(err)
+	}
+	cfg := Config{AppID: 1, InstallationID: 2, PrivateKeyPassEntry: &quot;key&quot;, Repository: &quot;octo/repo&quot;, Permissions: map[string]string{&quot;contents&quot;: &quot;read&quot;}}
+	ctx, cancel := context.WithCancel(context.Background())
+	defer cancel()
+	go func() { _ = Serve(ctx, sock, cfg) }()
+	waitForSocket(t, sock)
+	socketInfo, err := os.Stat(sock)
+	if err != nil {
+		t.Fatal(err)
+	}
+	if socketInfo.Mode().Perm() != 0600 {
+		t.Fatalf(&quot;socket mode = %o&quot;, socketInfo.Mode().Perm())
+	}
+	client := unixHTTPClient(sock)
+	resp, err := client.Get(&quot;http://unix/health&quot;)
+	if err != nil {
+		t.Fatal(err)
+	}
+	resp.Body.Close()
+	if resp.StatusCode != http.StatusOK {
+		t.Fatalf(&quot;health status = %d&quot;, resp.StatusCode)
+	}
+	resp, err = client.Get(&quot;http://unix/credential&quot;)
+	if err != nil {
+		t.Fatal(err)
+	}
+	data, _ := io.ReadAll(resp.Body)
+	resp.Body.Close()
+	if resp.StatusCode != http.StatusServiceUnavailable || !strings.Contains(resp.Header.Get(&quot;Cache-Control&quot;), &quot;no-store&quot;) {
+		t.Fatalf(&quot;credential response status=%d headers=%v body=%q&quot;, resp.StatusCode, resp.Header, data)
+	}
+	resp, err = client.Get(&quot;http://unix/other&quot;)
+	if err != nil {
+		t.Fatal(err)
+	}
+	resp.Body.Close()
+	if resp.StatusCode != http.StatusNotFound {
+		t.Fatalf(&quot;other path status = %d&quot;, resp.StatusCode)
+	}
+	request, _ := http.NewRequest(http.MethodPost, &quot;http://unix/health&quot;, nil)
+	resp, err = client.Do(request)
+	if err != nil {
+		t.Fatal(err)
+	}
+	resp.Body.Close()
+	if resp.StatusCode != http.StatusMethodNotAllowed {
+		t.Fatalf(&quot;health POST status = %d&quot;, resp.StatusCode)
+	}
+	if err := Serve(context.Background(), sock, cfg); err == nil {
+		t.Fatal(&quot;second server acquired the active socket&quot;)
+	}
+	cancel()
+	waitForSocketRemoval(t, sock)
+}
+
+func TestServerRecoversStaleSocketAfterCrash(t *testing.T) {
+	sock := filepath.Join(t.TempDir(), &quot;credential.sock&quot;)
+	_ = os.Chmod(filepath.Dir(sock), 0700)
+	stale, err := net.Listen(&quot;unix&quot;, sock)
+	if err != nil {
+		t.Fatal(err)
+	}
+	stale.(*net.UnixListener).SetUnlinkOnClose(false)
+	if err := stale.Close(); err != nil {
+		t.Fatal(err)
+	}
+	cfg := Config{AppID: 1, InstallationID: 2, PrivateKeyPassEntry: &quot;key&quot;, Repository: &quot;octo/repo&quot;, Permissions: map[string]string{&quot;contents&quot;: &quot;read&quot;}}
+	ctx, cancel := context.WithCancel(context.Background())
+	defer cancel()
+	result := make(chan error, 1)
+	go func() { result &lt;- Serve(ctx, sock, cfg) }()
+	waitForHealth(t, sock)
+	cancel()
+	waitForSocketRemoval(t, sock)
+	if err := &lt;-result; err != nil {
+		t.Fatal(err)
+	}
+}
+
+func TestRuntimeDirUsesPrivateProjectSpecificDirectory(t *testing.T) {
+	runtime := t.TempDir()
+	if err := os.Chmod(runtime, 0700); err != nil {
+		t.Fatal(err)
+	}
+	t.Setenv(&quot;XDG_RUNTIME_DIR&quot;, runtime)
+	a, err := RuntimeDir(&quot;/workspace/one&quot;)
+	if err != nil {
+		t.Fatal(err)
+	}
+	b, err := RuntimeDir(&quot;/workspace/two&quot;)
+	if err != nil {
+		t.Fatal(err)
+	}
+	if a == b || !strings.HasPrefix(a, runtime) {
+		t.Fatalf(&quot;unexpected paths %q %q&quot;, a, b)
+	}
+	info, err := os.Stat(a)
+	if err != nil {
+		t.Fatal(err)
+	}
+	if info.Mode().Perm() != 0700 {
+		t.Fatalf(&quot;mode = %o&quot;, info.Mode().Perm())
+	}
+}
+
+func TestCheckCredentialUsesUnixSocket(t *testing.T) {
+	sock := filepath.Join(t.TempDir(), &quot;health.sock&quot;)
+	listener, err := net.Listen(&quot;unix&quot;, sock)
+	if err != nil {
+		t.Fatal(err)
+	}
+	defer listener.Close()
+	server := &amp;http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
+		if r.URL.Path != &quot;/credential&quot; {
+			t.Errorf(&quot;path=%s&quot;, r.URL.Path)
+		}
+		w.WriteHeader(http.StatusOK)
+		_, _ = w.Write([]byte(&quot;token&quot;))
+	})}
+	go server.Serve(listener)
+	defer server.Close()
+	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
+	defer cancel()
+	if err := CheckCredential(ctx, sock); err != nil {
+		t.Fatal(err)
+	}
+}
+
+func waitForSocket(t *testing.T, path string) {
+	t.Helper()
+	deadline := time.Now().Add(time.Second)
+	for time.Now().Before(deadline) {
+		if _, err := os.Stat(path); err == nil {
+			return
+		}
+		time.Sleep(5 * time.Millisecond)
+	}
+	t.Fatal(&quot;socket did not appear&quot;)
+}
+
+func waitForSocketRemoval(t *testing.T, path string) {
+	t.Helper()
+	deadline := time.Now().Add(time.Second)
+	for time.Now().Before(deadline) {
+		if _, err := os.Lstat(path); os.IsNotExist(err) {
+			return
+		}
+		time.Sleep(5 * time.Millisecond)
+	}
+	t.Fatal(&quot;socket was not removed&quot;)
+}
+
+func waitForHealth(t *testing.T, path string) {
+	t.Helper()
+	deadline := time.Now().Add(time.Second)
+	client := unixHTTPClient(path)
+	for time.Now().Before(deadline) {
+		resp, err := client.Get(&quot;http://unix/health&quot;)
+		if err == nil {
+			resp.Body.Close()
+			if resp.StatusCode == http.StatusOK {
+				return
+			}
+		}
+		time.Sleep(5 * time.Millisecond)
+	}
+	t.Fatal(&quot;credential server did not become healthy&quot;)
+}
</code></pre><h4>internal/plugins/manager_test.go</h4><pre style="white-space:pre-wrap;overflow-wrap:anywhere"><code>diff --git a/internal/plugins/manager_test.go b/internal/plugins/manager_test.go
new file mode 100644
index 0000000..77ed353
--- /dev/null
+++ b/internal/plugins/manager_test.go
@@ -0,0 +1,536 @@
+package plugins
+
+import (
+	&quot;context&quot;
+	&quot;net&quot;
+	&quot;net/http&quot;
+	&quot;os&quot;
+	&quot;path/filepath&quot;
+	&quot;strings&quot;
+	&quot;testing&quot;
+	&quot;time&quot;
+
+	&quot;dch/internal/common&quot;
+	&quot;dch/internal/githubapp&quot;
+)
+
+func TestEnableValidatesCredentialOverUnixSocketAndMarksBuildPending(t *testing.T) {
+	env := setupManagerTest(t, &quot;ok&quot;)
+	projectPath := filepath.Join(env.root, &quot;project$workspace&quot;)
+	if err := os.MkdirAll(filepath.Join(projectPath, &quot;.devcontainer&quot;), 0700); err != nil {
+		t.Fatal(err)
+	}
+	env.setSocket(t, projectPath)
+	cfg := validConfig()
+	if err := NewManager().Enable(context.Background(), projectPath, cfg); err != nil {
+		t.Fatal(err)
+	}
+	loaded, err := common.LoadConfig()
+	if err != nil {
+		t.Fatal(err)
+	}
+	normalized, _ := common.NormalizePath(projectPath)
+	state := loaded.ProjectPlugins[normalized]
+	if state.Enabled[githubAppID].AppID != cfg.AppID || !state.States[githubAppID].BuildPending || state.States[githubAppID].FragmentDigest == &quot;&quot; {
+		t.Fatalf(&quot;enable did not persist config and Build state: %+v&quot;, state)
+	}
+	unit, err := os.ReadFile(state.States[githubAppID].UnitPath)
+	if err != nil {
+		t.Fatal(err)
+	}
+	if !strings.Contains(string(unit), strings.ReplaceAll(projectPath, &quot;$&quot;, &quot;$$&quot;)) {
+		t.Fatalf(&quot;ExecStart does not escape dollar in project path: %s&quot;, unit)
+	}
+	if _, err := os.Stat(managedFragment(normalized)); err != nil {
+		t.Fatalf(&quot;managed fragment missing: %v&quot;, err)
+	}
+}
+
+func TestEnableFailureRestoresPriorProjectConfigAndFragment(t *testing.T) {
+	env := setupManagerTest(t, &quot;fail&quot;)
+	projectPath := filepath.Join(env.root, &quot;project&quot;)
+	if err := os.MkdirAll(filepath.Join(projectPath, &quot;.devcontainer&quot;, &quot;devcontainer.local.d&quot;), 0700); err != nil {
+		t.Fatal(err)
+	}
+	env.setSocket(t, projectPath)
+	normalized, _ := common.NormalizePath(projectPath)
+	oldCfg := validConfig()
+	oldCfg.AppID = 3
+	oldFragment := []byte(`{&quot;old&quot;:true}`)
+	fragmentPath := managedFragment(normalized)
+	if err := os.WriteFile(fragmentPath, oldFragment, 0600); err != nil {
+		t.Fatal(err)
+	}
+	old := common.ProjectPlugins{Enabled: map[string]githubapp.Config{githubAppID: oldCfg}, States: map[string]common.PluginState{githubAppID: {FragmentDigest: digest(oldFragment), BuildFingerprint: &quot;old&quot;, BuildPending: false}}}
+	if err := common.SaveConfig(common.Config{Version: 1, Projects: []string{}, ProjectPlugins: map[string]common.ProjectPlugins{normalized: old}}); err != nil {
+		t.Fatal(err)
+	}
+	cfg := validConfig()
+	cfg.AppID = 99
+	ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
+	defer cancel()
+	if err := NewManager().Enable(ctx, projectPath, cfg); err == nil {
+		t.Fatal(&quot;Enable succeeded despite credential failure&quot;)
+	}
+	loaded, err := common.LoadConfig()
+	if err != nil {
+		t.Fatal(err)
+	}
+	got := loaded.ProjectPlugins[normalized]
+	if got.Enabled[githubAppID].AppID != oldCfg.AppID || got.States[githubAppID].BuildFingerprint != &quot;old&quot; || got.States[githubAppID].BuildPending {
+		t.Fatalf(&quot;failed enable changed existing project config: %+v&quot;, got)
+	}
+	fragment, err := os.ReadFile(fragmentPath)
+	if err != nil || string(fragment) != string(oldFragment) {
+		t.Fatalf(&quot;failed enable did not restore fragment: %q, %v&quot;, fragment, err)
+	}
+}
+
+func TestEnableCleanupFailureKeepsRetryableSettings(t *testing.T) {
+	env := setupManagerTest(t, &quot;fail&quot;)
+	t.Setenv(&quot;DCH_TEST_FAIL_STOP_ONCE&quot;, filepath.Join(env.root, &quot;stop-failed&quot;))
+	projectPath := filepath.Join(env.root, &quot;project&quot;)
+	if err := os.MkdirAll(filepath.Join(projectPath, &quot;.devcontainer&quot;), 0700); err != nil {
+		t.Fatal(err)
+	}
+	env.setSocket(t, projectPath)
+	ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
+	defer cancel()
+	if err := NewManager().Enable(ctx, projectPath, validConfig()); err == nil || !strings.Contains(err.Error(), &quot;cleanup failed&quot;) {
+		t.Fatalf(&quot;Enable error = %v, want cleanup failure&quot;, err)
+	}
+	normalized, _ := common.NormalizePath(projectPath)
+	loaded, err := common.LoadConfig()
+	if err != nil {
+		t.Fatal(err)
+	}
+	if _, ok := loaded.ProjectPlugins[normalized].Enabled[githubAppID]; !ok {
+		t.Fatal(&quot;retryable config was not retained&quot;)
+	}
+	if err := NewManager().Disable(context.Background(), projectPath); err != nil {
+		t.Fatalf(&quot;retry disable: %v&quot;, err)
+	}
+	loaded, err = common.LoadConfig()
+	if err != nil {
+		t.Fatal(err)
+	}
+	if _, ok := loaded.ProjectPlugins[normalized].Enabled[githubAppID]; ok {
+		t.Fatal(&quot;retry cleanup left plugin enabled&quot;)
+	}
+}
+
+func TestEnableRestartsActiveServiceWhenConfigurationChanges(t *testing.T) {
+	env := setupManagerTest(t, &quot;ok&quot;)
+	projectPath := filepath.Join(env.root, &quot;project&quot;)
+	if err := os.MkdirAll(filepath.Join(projectPath, &quot;.devcontainer&quot;), 0700); err != nil {
+		t.Fatal(err)
+	}
+	env.setSocket(t, projectPath)
+	manager := NewManager()
+	cfg := validConfig()
+	if err := manager.Enable(context.Background(), projectPath, cfg); err != nil {
+		t.Fatal(err)
+	}
+	first, err := common.LoadConfig()
+	if err != nil {
+		t.Fatal(err)
+	}
+	normalized, _ := common.NormalizePath(projectPath)
+	if _, ok := first.ProjectPlugins[normalized].Enabled[githubAppID]; !ok {
+		t.Fatalf(&quot;first enable did not persist enabled state: %+v&quot;, first.ProjectPlugins)
+	}
+	firstFingerprint := first.ProjectPlugins[normalized].States[githubAppID].BuildFingerprint
+	cfg.AppID = 42
+	if err := manager.Enable(context.Background(), projectPath, cfg); err != nil {
+		t.Fatal(err)
+	}
+	log, err := os.ReadFile(filepath.Join(env.root, &quot;systemctl.log&quot;))
+	if err != nil {
+		t.Fatal(err)
+	}
+	if !strings.Contains(string(log), &quot;restart&quot;) {
+		debug, _ := os.ReadFile(filepath.Join(env.root, &quot;service.debug&quot;))
+		t.Fatalf(&quot;active service was not restarted: %s / %s&quot;, log, debug)
+	}
+	normalized, _ = common.NormalizePath(projectPath)
+	loaded, err := common.LoadConfig()
+	if err != nil {
+		t.Fatal(err)
+	}
+	if got := loaded.ProjectPlugins[normalized].Enabled[githubAppID].AppID; got != 42 {
+		t.Fatalf(&quot;AppID = %d, want 42&quot;, got)
+	}
+	if got := loaded.ProjectPlugins[normalized].States[githubAppID].BuildFingerprint; got == firstFingerprint {
+		t.Fatal(&quot;configuration change did not update Build fingerprint&quot;)
+	}
+}
+
+func TestEnsureUpdatesOwnedUnitAfterProcessEnvironmentChanges(t *testing.T) {
+	env := setupManagerTest(t, &quot;ok&quot;)
+	projectPath := filepath.Join(env.root, &quot;project&quot;)
+	if err := os.MkdirAll(filepath.Join(projectPath, &quot;.devcontainer&quot;), 0700); err != nil {
+		t.Fatal(err)
+	}
+	env.setSocket(t, projectPath)
+	manager := NewManager()
+	if err := manager.Enable(context.Background(), projectPath, validConfig()); err != nil {
+		t.Fatal(err)
+	}
+	normalized, _ := common.NormalizePath(projectPath)
+	before, err := common.LoadConfig()
+	if err != nil {
+		t.Fatal(err)
+	}
+	oldUnitPath := before.ProjectPlugins[normalized].States[githubAppID].UnitPath
+	oldUnitDigest := before.ProjectPlugins[normalized].States[githubAppID].UnitDigest
+	oldPath := os.Getenv(&quot;PATH&quot;)
+	extra := filepath.Join(env.root, &quot;extra-bin&quot;)
+	if err := os.Mkdir(extra, 0700); err != nil {
+		t.Fatal(err)
+	}
+	t.Setenv(&quot;PATH&quot;, extra+&quot;:&quot;+oldPath)
+	newConfigHome := filepath.Join(env.root, &quot;new-config&quot;)
+	if err := os.Mkdir(newConfigHome, 0700); err != nil {
+		t.Fatal(err)
+	}
+	t.Setenv(&quot;XDG_CONFIG_HOME&quot;, newConfigHome)
+	if err := manager.Ensure(context.Background(), projectPath); err != nil {
+		t.Fatalf(&quot;Ensure after PATH change: %v&quot;, err)
+	}
+	after, err := common.LoadConfig()
+	if err != nil {
+		t.Fatal(err)
+	}
+	afterState := after.ProjectPlugins[normalized].States[githubAppID]
+	if afterState.UnitPath != oldUnitPath || afterState.UnitDigest == oldUnitDigest {
+		t.Fatalf(&quot;unit metadata was not updated at stored path: before=%+v after=%+v&quot;, before.ProjectPlugins[normalized].States[githubAppID], afterState)
+	}
+	newUnitPath, err := unitPath(projectPath)
+	if err != nil {
+		t.Fatal(err)
+	}
+	if _, err := os.Stat(newUnitPath); !os.IsNotExist(err) {
+		t.Fatalf(&quot;Ensure created a duplicate unit under changed XDG_CONFIG_HOME: %v&quot;, err)
+	}
+	if err := manager.Disable(context.Background(), projectPath); err != nil {
+		t.Fatalf(&quot;Disable after PATH change: %v&quot;, err)
+	}
+	if _, err := os.Stat(oldUnitPath); !os.IsNotExist(err) {
+		t.Fatalf(&quot;Disable left the stored service unit behind: %v&quot;, err)
+	}
+}
+
+func TestEnsureRetriesRestartAfterSystemctlRestartFailure(t *testing.T) {
+	env := setupManagerTest(t, &quot;ok&quot;)
+	projectPath := filepath.Join(env.root, &quot;project&quot;)
+	if err := os.MkdirAll(filepath.Join(projectPath, &quot;.devcontainer&quot;), 0700); err != nil {
+		t.Fatal(err)
+	}
+	env.setSocket(t, projectPath)
+	manager := NewManager()
+	if err := manager.Enable(context.Background(), projectPath, validConfig()); err != nil {
+		t.Fatal(err)
+	}
+	normalized, _ := common.NormalizePath(projectPath)
+	config, err := common.LoadConfig()
+	if err != nil {
+		t.Fatal(err)
+	}
+	stateBefore := config.ProjectPlugins[normalized].States[githubAppID]
+	unitBefore, err := os.ReadFile(stateBefore.UnitPath)
+	if err != nil {
+		t.Fatal(err)
+	}
+	t.Setenv(&quot;PATH&quot;, filepath.Join(env.root, &quot;new-path&quot;)+&quot;:&quot;+os.Getenv(&quot;PATH&quot;))
+	t.Setenv(&quot;DCH_TEST_FAIL_RESTART_ONCE&quot;, filepath.Join(env.root, &quot;restart-failed&quot;))
+	if err := manager.Ensure(context.Background(), projectPath); err == nil {
+		t.Fatal(&quot;Ensure succeeded despite failed restart&quot;)
+	}
+	unitAfterFailure, err := os.ReadFile(stateBefore.UnitPath)
+	if err != nil || string(unitAfterFailure) != string(unitBefore) {
+		t.Fatalf(&quot;failed restart did not restore unit: %v&quot;, err)
+	}
+	configAfterFailure, err := common.LoadConfig()
+	if err != nil {
+		t.Fatal(err)
+	}
+	if got := configAfterFailure.ProjectPlugins[normalized].States[githubAppID].UnitDigest; got != stateBefore.UnitDigest {
+		t.Fatalf(&quot;failed restart persisted digest %q, want %q&quot;, got, stateBefore.UnitDigest)
+	}
+	if err := manager.Ensure(context.Background(), projectPath); err != nil {
+		t.Fatalf(&quot;Ensure retry: %v&quot;, err)
+	}
+	log, err := os.ReadFile(filepath.Join(env.root, &quot;systemctl.log&quot;))
+	if err != nil {
+		t.Fatal(err)
+	}
+	if strings.Count(string(log), &quot;restart&quot;) &lt; 2 {
+		t.Fatalf(&quot;Ensure retry did not retry restart: %s&quot;, log)
+	}
+}
+
+func TestDisableRefusesManuallyModifiedUnit(t *testing.T) {
+	env := setupManagerTest(t, &quot;ok&quot;)
+	projectPath := filepath.Join(env.root, &quot;project&quot;)
+	if err := os.MkdirAll(filepath.Join(projectPath, &quot;.devcontainer&quot;), 0700); err != nil {
+		t.Fatal(err)
+	}
+	env.setSocket(t, projectPath)
+	manager := NewManager()
+	if err := manager.Enable(context.Background(), projectPath, validConfig()); err != nil {
+		t.Fatal(err)
+	}
+	normalized, _ := common.NormalizePath(projectPath)
+	config, err := common.LoadConfig()
+	if err != nil {
+		t.Fatal(err)
+	}
+	unitPath := config.ProjectPlugins[normalized].States[githubAppID].UnitPath
+	manual := []byte(&quot;\n# user change\n&quot;)
+	file, err := os.OpenFile(unitPath, os.O_APPEND|os.O_WRONLY, 0)
+	if err != nil {
+		t.Fatal(err)
+	}
+	if _, err := file.Write(manual); err != nil {
+		t.Fatal(err)
+	}
+	if err := file.Close(); err != nil {
+		t.Fatal(err)
+	}
+	if err := manager.Disable(context.Background(), projectPath); err == nil {
+		t.Fatal(&quot;Disable accepted a manually edited unit&quot;)
+	}
+	data, err := os.ReadFile(unitPath)
+	if err != nil || !strings.HasSuffix(string(data), string(manual)) {
+		t.Fatalf(&quot;manual unit edit changed: %q, %v&quot;, data, err)
+	}
+}
+
+func TestFailedReconfigureRestoresPriorUnitAndConfig(t *testing.T) {
+	env := setupManagerTest(t, &quot;ok&quot;)
+	projectPath := filepath.Join(env.root, &quot;project&quot;)
+	if err := os.MkdirAll(filepath.Join(projectPath, &quot;.devcontainer&quot;), 0700); err != nil {
+		t.Fatal(err)
+	}
+	env.setSocket(t, projectPath)
+	manager := NewManager()
+	oldConfig := validConfig()
+	if err := manager.Enable(context.Background(), projectPath, oldConfig); err != nil {
+		t.Fatal(err)
+	}
+	normalized, _ := common.NormalizePath(projectPath)
+	stored, err := common.LoadConfig()
+	if err != nil {
+		t.Fatal(err)
+	}
+	state := stored.ProjectPlugins[normalized].States[githubAppID]
+	unitBefore, err := os.ReadFile(state.UnitPath)
+	if err != nil {
+		t.Fatal(err)
+	}
+	newPath := filepath.Join(env.root, &quot;changed-path&quot;)
+	t.Setenv(&quot;PATH&quot;, newPath+&quot;:&quot;+os.Getenv(&quot;PATH&quot;))
+	t.Setenv(&quot;DCH_TEST_MODE&quot;, &quot;fail&quot;)
+	updated := oldConfig
+	updated.AppID++
+	if err := manager.Enable(context.Background(), projectPath, updated); err == nil {
+		t.Fatal(&quot;Enable succeeded despite credential validation failure&quot;)
+	}
+	unitAfter, err := os.ReadFile(state.UnitPath)
+	if err != nil || string(unitAfter) != string(unitBefore) {
+		t.Fatalf(&quot;rollback did not restore unit: %v&quot;, err)
+	}
+	loaded, err := common.LoadConfig()
+	if err != nil {
+		t.Fatal(err)
+	}
+	got := loaded.ProjectPlugins[normalized]
+	if got.Enabled[githubAppID].AppID != oldConfig.AppID || got.States[githubAppID].UnitDigest != state.UnitDigest {
+		t.Fatalf(&quot;rollback did not restore config state: %+v&quot;, got)
+	}
+}
+
+func TestStatusReportsPendingBuildAndInactiveService(t *testing.T) {
+	env := setupManagerTest(t, &quot;ok&quot;)
+	path := filepath.Join(env.root, &quot;project&quot;)
+	normalized, _ := common.NormalizePath(path)
+	if err := common.SaveConfig(common.Config{Version: 1, Projects: []string{}, ProjectPlugins: map[string]common.ProjectPlugins{
+		normalized: {Enabled: map[string]githubapp.Config{githubAppID: {}}, States: map[string]common.PluginState{githubAppID: {BuildPending: true}}},
+	}}); err != nil {
+		t.Fatal(err)
+	}
+	status, err := NewManager().Status(context.Background(), path)
+	if err != nil {
+		t.Fatal(err)
+	}
+	if !status.Enabled || !status.PendingBuild || status.ServiceActive {
+		t.Fatalf(&quot;unexpected status: %+v&quot;, status)
+	}
+}
+
+func TestDisableRefusesEditedFragmentBeforeStoppingService(t *testing.T) {
+	env := setupManagerTest(t, &quot;ok&quot;)
+	projectPath := filepath.Join(env.root, &quot;project&quot;)
+	if err := os.MkdirAll(filepath.Dir(managedFragment(projectPath)), 0700); err != nil {
+		t.Fatal(err)
+	}
+	fragmentPath := managedFragment(projectPath)
+	if err := os.WriteFile(fragmentPath, []byte(`{&quot;manual&quot;:true}`), 0600); err != nil {
+		t.Fatal(err)
+	}
+	normalized, _ := common.NormalizePath(projectPath)
+	if err := common.SaveConfig(common.Config{Version: 1, Projects: []string{}, ProjectPlugins: map[string]common.ProjectPlugins{
+		normalized: {Enabled: map[string]githubapp.Config{githubAppID: validConfig()}, States: map[string]common.PluginState{githubAppID: {FragmentDigest: digest([]byte(`{&quot;generated&quot;:true}`))}}},
+	}}); err != nil {
+		t.Fatal(err)
+	}
+	if err := NewManager().Disable(context.Background(), projectPath); err == nil {
+		t.Fatal(&quot;Disable accepted a manually edited fragment&quot;)
+	}
+	data, err := os.ReadFile(fragmentPath)
+	if err != nil || string(data) != `{&quot;manual&quot;:true}` {
+		t.Fatalf(&quot;edited fragment changed: %q %v&quot;, data, err)
+	}
+	log, err := os.ReadFile(filepath.Join(env.root, &quot;systemctl.log&quot;))
+	if err == nil &amp;&amp; strings.Contains(string(log), &quot;stop&quot;) {
+		t.Fatalf(&quot;service was stopped before ownership check: %s&quot;, log)
+	}
+	loaded, err := common.LoadConfig()
+	if err != nil {
+		t.Fatal(err)
+	}
+	if _, ok := loaded.ProjectPlugins[normalized].Enabled[githubAppID]; !ok {
+		t.Fatal(&quot;enabled config was lost&quot;)
+	}
+}
+
+func TestEnablePreservesForeignSystemdUnit(t *testing.T) {
+	env := setupManagerTest(t, &quot;ok&quot;)
+	projectPath := filepath.Join(env.root, &quot;project&quot;)
+	if err := os.MkdirAll(filepath.Join(projectPath, &quot;.devcontainer&quot;), 0700); err != nil {
+		t.Fatal(err)
+	}
+	env.setSocket(t, projectPath)
+	unit, err := unitPath(projectPath)
+	if err != nil {
+		t.Fatal(err)
+	}
+	if err := os.MkdirAll(filepath.Dir(unit), 0700); err != nil {
+		t.Fatal(err)
+	}
+	foreign := []byte(&quot;foreign unit\n&quot;)
+	if err := os.WriteFile(unit, foreign, 0600); err != nil {
+		t.Fatal(err)
+	}
+	if err := NewManager().Enable(context.Background(), projectPath, validConfig()); err == nil {
+		t.Fatal(&quot;Enable overwrote a foreign unit&quot;)
+	}
+	data, err := os.ReadFile(unit)
+	if err != nil || string(data) != string(foreign) {
+		t.Fatalf(&quot;foreign unit changed: %q, %v&quot;, data, err)
+	}
+	log, _ := os.ReadFile(filepath.Join(env.root, &quot;systemctl.log&quot;))
+	if strings.Contains(string(log), &quot;stop&quot;) || strings.Contains(string(log), &quot;disable&quot;) {
+		t.Fatalf(&quot;foreign unit was managed: %s&quot;, log)
+	}
+}
+
+func TestCredentialServiceHelper(t *testing.T) {
+	if os.Getenv(&quot;DCH_TEST_HELPER&quot;) != &quot;1&quot; {
+		return
+	}
+	socket := os.Getenv(&quot;DCH_TEST_SOCKET&quot;)
+	_ = os.Remove(socket)
+	listener, err := net.Listen(&quot;unix&quot;, socket)
+	if err != nil {
+		os.Exit(2)
+	}
+	_ = os.Chmod(socket, 0600)
+	status := http.StatusOK
+	if os.Getenv(&quot;DCH_TEST_MODE&quot;) == &quot;fail&quot; {
+		status = http.StatusServiceUnavailable
+	}
+	server := &amp;http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(status); _, _ = w.Write([]byte(&quot;token&quot;)) })}
+	_ = server.Serve(listener)
+	os.Exit(0)
+}
+
+type managerTestEnv struct {
+	root    string
+	runtime string
+}
+
+func (env managerTestEnv) setSocket(t *testing.T, projectPath string) {
+	t.Helper()
+	dir, err := githubapp.RuntimeDir(projectPath)
+	if err != nil {
+		t.Fatal(err)
+	}
+	t.Setenv(&quot;DCH_TEST_SOCKET&quot;, filepath.Join(dir, &quot;credential.sock&quot;))
+}
+
+func setupManagerTest(t *testing.T, mode string) managerTestEnv {
+	t.Helper()
+	root := t.TempDir()
+	runtime, err := os.MkdirTemp(&quot;/tmp&quot;, &quot;dchrt&quot;)
+	if err != nil {
+		t.Fatal(err)
+	}
+	t.Cleanup(func() { _ = os.RemoveAll(runtime) })
+	configHome := filepath.Join(root, &quot;config-home&quot;)
+	if err := os.Mkdir(configHome, 0700); err != nil {
+		t.Fatal(err)
+	}
+	binDir := filepath.Join(root, &quot;bin&quot;)
+	if err := os.Mkdir(binDir, 0700); err != nil {
+		t.Fatal(err)
+	}
+	binary, err := os.Executable()
+	if err != nil {
+		t.Fatal(err)
+	}
+	pidPath := filepath.Join(root, &quot;service.pid&quot;)
+	logPath := filepath.Join(root, &quot;systemctl.log&quot;)
+	debugPath := filepath.Join(root, &quot;service.debug&quot;)
+	script := `#!/bin/sh
+shift
+action=$1
+echo &quot;$action&quot; &gt;&gt; &quot;$DCH_TEST_LOG&quot;
+case &quot;$action&quot; in
+  daemon-reload) exit 0 ;;
+  is-active) [ -f &quot;$DCH_TEST_PID&quot; ] &amp;&amp; kill -0 &quot;$(cat &quot;$DCH_TEST_PID&quot;)&quot; 2&gt;/dev/null ; status=$?; echo &quot;active=$status pid=$(cat &quot;$DCH_TEST_PID&quot; 2&gt;/dev/null)&quot; &gt;&gt; &quot;$DCH_TEST_DEBUG&quot;; exit &quot;$status&quot; ;;
+  stop) if [ -n &quot;$DCH_TEST_FAIL_STOP_ONCE&quot; ] &amp;&amp; [ ! -f &quot;$DCH_TEST_FAIL_STOP_ONCE.done&quot; ]; then touch &quot;$DCH_TEST_FAIL_STOP_ONCE.done&quot;; exit 1; fi; if [ -f &quot;$DCH_TEST_PID&quot; ]; then kill &quot;$(cat &quot;$DCH_TEST_PID&quot;)&quot; 2&gt;/dev/null || true; rm -f &quot;$DCH_TEST_PID&quot;; fi; exit 0 ;;
+  disable) if [ -f &quot;$DCH_TEST_PID&quot; ]; then kill &quot;$(cat &quot;$DCH_TEST_PID&quot;)&quot; 2&gt;/dev/null || true; rm -f &quot;$DCH_TEST_PID&quot;; fi; exit 0 ;;
+  enable|restart|start)
+    if [ &quot;$action&quot; = &quot;restart&quot; ] &amp;&amp; [ -n &quot;$DCH_TEST_FAIL_RESTART_ONCE&quot; ] &amp;&amp; [ ! -f &quot;$DCH_TEST_FAIL_RESTART_ONCE.done&quot; ]; then touch &quot;$DCH_TEST_FAIL_RESTART_ONCE.done&quot;; exit 1; fi
+    if [ -f &quot;$DCH_TEST_PID&quot; ]; then kill &quot;$(cat &quot;$DCH_TEST_PID&quot;)&quot; 2&gt;/dev/null || true; rm -f &quot;$DCH_TEST_PID&quot;; fi
+    DCH_TEST_HELPER=1 &quot;$DCH_TEST_BINARY&quot; -test.run &#x27;^TestCredentialServiceHelper$&#x27; &gt;&gt; &quot;$DCH_TEST_DEBUG&quot; 2&gt;&amp;1 &amp;
+    echo $! &gt; &quot;$DCH_TEST_PID&quot;
+    sleep 0.03
+    exit 0 ;;
+esac
+exit 1
+`
+	if err := os.WriteFile(filepath.Join(binDir, &quot;systemctl&quot;), []byte(script), 0700); err != nil {
+		t.Fatal(err)
+	}
+	oldPath := os.Getenv(&quot;PATH&quot;)
+	t.Setenv(&quot;PATH&quot;, binDir+&quot;:&quot;+oldPath)
+	t.Setenv(&quot;DCH_CONFIG&quot;, filepath.Join(root, &quot;config&quot;))
+	t.Setenv(&quot;XDG_RUNTIME_DIR&quot;, runtime)
+	t.Setenv(&quot;XDG_CONFIG_HOME&quot;, configHome)
+	t.Setenv(&quot;DCH_TEST_BINARY&quot;, binary)
+	t.Setenv(&quot;DCH_TEST_PID&quot;, pidPath)
+	t.Setenv(&quot;DCH_TEST_LOG&quot;, logPath)
+	t.Setenv(&quot;DCH_TEST_DEBUG&quot;, debugPath)
+	t.Setenv(&quot;DCH_TEST_FAIL_STOP_ONCE&quot;, &quot;&quot;)
+	t.Setenv(&quot;DCH_TEST_FAIL_RESTART_ONCE&quot;, &quot;&quot;)
+	t.Setenv(&quot;DCH_TEST_SOCKET&quot;, &quot;&quot;)
+	t.Setenv(&quot;DCH_TEST_MODE&quot;, mode)
+	t.Setenv(&quot;DCH_TEST_HELPER&quot;, &quot;&quot;)
+	return managerTestEnv{root: root, runtime: runtime}
+}
+
+func validConfig() githubapp.Config {
+	return githubapp.Config{AppID: 1, InstallationID: 2, PrivateKeyPassEntry: &quot;github/key&quot;, Repository: &quot;owner/repo&quot;, Permissions: map[string]string{&quot;contents&quot;: &quot;read&quot;}}
+}
</code></pre></div>
<div><h3>G8 新增／修改：文件與核准規格</h3><p>README 說明操作、限制與人工驗收。規格 HTML 為已核准規格的呈現產物。</p><p>需你 review：否</p><p><strong>檔案與新版本 hunk 起始行</strong></p><ul><li>README.md:5,21,57,68</li><li>specs/2026-10-24-project-plugins-spec.md:1</li><li>specs/2026-10-24-project-plugins-spec.html:1</li></ul></div><div id="review-G8"><p>版本 759b9d8；以下為完整檔案 diff（保留完整 hunk）。行號見 @@ 的 + 區段。</p><h4>README.md</h4><pre style="white-space:pre-wrap;overflow-wrap:anywhere"><code>diff --git a/README.md b/README.md
index 9c3f336..ea70463 100644
--- a/README.md
+++ b/README.md
@@ -5,7 +5,8 @@
 | Feature | Description |
 | --- | --- |
 | Manage Dev Containers in one place | View the status of multiple projects in a terminal interface, and connect to, start, build, or stop their containers. |
-| Apply personal settings | Merge `devcontainer.local.json` with the project configuration for previews and container operations, without changing the original configuration file. |
+| Apply personal settings | Merge sorted `.devcontainer/devcontainer.local.d/*.json` fragments with the project configuration for previews and container operations, without changing the original configuration file. |
+| Project plugins | Configure per-project host services such as GitHub App credentials, then apply their container settings with Build. |
 
 ## Prerequisites
 
@@ -20,8 +21,11 @@ Install the latest release and the Dev Container CLI with POSIX `sh` (no Node.js
 ```sh
 curl -fsSL https://raw.githubusercontent.com/yun-lun-li/devcontainer-helper/main/install.sh | sh
 dch version
+```
 
 ## Update
+
+```sh
 dch update 
 ```
 
@@ -53,7 +57,7 @@ dch help                  # Show command help
 
 ### Configuration file
 
-`~/.dch/config` is JSON without a `.json` extension. It stores **only project paths**. dch reads each project&#x27;s name from the merged `.devcontainer/devcontainer.json` and optional `.devcontainer/devcontainer.local.json`; comments and trailing commas are supported in both. If `name` is missing, the directory name is used.
+`~/.dch/config` is JSON without a `.json` extension. It stores registered project paths and enabled plugin settings. Private keys, JWTs, and access tokens are not stored there. dch reads each project&#x27;s name from the merged `.devcontainer/devcontainer.json` and optional first-level `.devcontainer/devcontainer.local.d/*.json` fragments; comments and trailing commas are supported. If `name` is missing, the directory name is used.
 
 ```json
 {
@@ -64,3 +68,46 @@ dch help                  # Show command help
   ]
 }
 ```
+
+### GitHub App plugin
+
+The built-in GitHub App plugin is Linux-only and gives one project access to one selected GitHub repository. It requires local Docker, a systemd user service, `pass`, and a GPG agent on the host. Create and install the GitHub App, then store its RSA private key in the host&#x27;s `pass` store. The key entry must be readable by the host user running the dch systemd user service.
+
+Before enabling the plugin, unlock the host GPG agent without printing the key:
+
+```sh
+pass show &lt;entry&gt; &gt;/dev/null
+```
+
+The GPG agent must be able to decrypt without opening a prompt. dch invokes `pass` non-interactively; if the GPG agent&#x27;s cached unlock expires, unlock the entry again before the next token refresh. The private key remains on the host and is never mounted into the container.
+
+Open **Project → Plugins** to configure and enable the plugin. Enter the App ID, installation ID, pass entry, repository (`owner/repository`), and choose `read`, `write`, or disabled for `contents` and `pull_requests`. dch verifies the service and credential before enabling. The resulting installation token is limited to that repository and selected permissions.
+
+The container needs Git 2.30 or newer, GitHub CLI (`gh`), curl, and POSIX `sh`. The plugin adds a read-only mount at `/run/dch-github`, a Git credential helper for HTTPS `github.com`, and a `gh` wrapper available from Connect shells. The wrapper requests a token for each invocation and sets `GH_TOKEN` only for that `gh` process. Other Git hosts keep their existing credential helpers; SSH URLs are unchanged. Existing lifecycle hooks are preserved. The socket directory is mode `0700` and the socket is mode `0600`; the container user must be able to connect to it, so a mismatched UID will fail.
+
+Any container process that can access the socket can request that project&#x27;s short-lived token, and Docker administrators can access the mount. Use the plugin only with containers you trust.
+
+After enabling, Build the project to apply the mount and initialize the Git helper. Configuration changes and disabling the plugin also mark an existing container as needing Build. dch does not rebuild automatically. Start and Connect are blocked for an existing container until Build succeeds. Disabling stops the host service and removes its runtime files immediately; removing a project also disables its plugin before unregistering it.
+
+The systemd user service stays active when dch closes or the container stops. Tokens are kept in host memory only. The service reuses a token while it has more than five minutes left and refreshes it on the next credential request when five minutes or less remain. It does not write token files or run a refresh timer.
+
+To check that the host service is alive without requesting a credential, use:
+
+```sh
+curl --fail --silent --show-error --max-time 5 --unix-socket &quot;$XDG_RUNTIME_DIR/dch/&lt;project-hash&gt;/credential.sock&quot; http://localhost/health
+```
+
+The health endpoint returns only service status. Do not request or print `/credential` when checking service health.
+
+#### Manual Linux verification
+
+1. In **Project → Plugins**, enable the GitHub App and confirm dch verifies the service and credential without printing the token.
+2. Close dch and stop the container. Confirm the user service remains active; run the `/health` request above and expect `ok`.
+3. Build and Connect. Confirm Git fetch/push and `gh pr create` use the configured repository and expected GitHub identity and permissions.
+4. Restart the user service while the container is running, then use Git or `gh` again to confirm the mounted runtime directory exposes the replacement socket.
+5. Change the plugin settings or disable it. Confirm the existing container shows **Build required**, Start and Connect are blocked, and disabling stops the service immediately. Build to apply or remove the mount.
+6. Repeat with a container user whose UID does not match the host socket owner and confirm connection fails. Also try enabling while the GPG agent cannot decrypt non-interactively and confirm activation fails without exposing key or token contents.
+
+These checks require Linux, Docker, systemd, `pass`, and GPG agent access; they are manual integration checks.
+
+Project-specific settings belong in `.devcontainer/devcontainer.local.d/` as first-level `.json` files. dch merges them in filename order; keep local fragments out of version control when they contain machine-specific settings. dch no longer reads `.devcontainer/devcontainer.local.json`; existing files are ignored and never converted automatically.
</code></pre><h4>specs/2026-10-24-project-plugins-spec.md</h4><pre style="white-space:pre-wrap;overflow-wrap:anywhere"><code>diff --git a/specs/2026-10-24-project-plugins-spec.md b/specs/2026-10-24-project-plugins-spec.md
new file mode 100644
index 0000000..07a6cb5
--- /dev/null
+++ b/specs/2026-10-24-project-plugins-spec.md
@@ -0,0 +1,264 @@
+---
+title: Project Plugins 與 GitHub App 認證規格
+subtitle: hard dev-flow · Unix Socket 簡化版 · 已核准 · 2026-10-24
+lang: zh-Hant
+template: doc
+theme: paper
+---
+
+## A 目標與範圍
+
+dch 讓每個 Project 獨立啟用內建 plugin。
+Plugin 可執行 Host 前置作業、提供容器設定與容器初始化。
+第一個 plugin 讓 Git 與 gh 向 Host Unix Socket 取得 GitHub App Token。
+私鑰留在 Host pass，Token 只在需要時產生。
+
+| 決定 | 本次交付 |
+| --- | --- |
+| Plugin | Go 內建 registry，第一個為 github-app |
+| 使用入口 | Console 的 Project → Plugins |
+| Host | Linux、本機 Docker、每個 Project 一個 systemd user service |
+| 容器 | image／Dockerfile；已有 Git、gh、curl 與 POSIX shell |
+| GitHub | github.com；每個 Project 指定單一 owner/repository |
+| 設定 | Host dch JSON 保存 plugin 參數；local.d 保存容器設定片段 |
+| Token | Host 記憶體快取，收到請求時按需更新 |
+
+此版以 Unix Socket 取代前版的 Token File 與 timer。
+不寫入持久 Token，也不做週期性刷新。
+其他已確認的 local.d、Console 與待 Build 行為保留。
+
+## B 使用情境與 Console
+
+1. 身為開發者，我要為不同 Project 啟用不同 plugin。
+2. 身為開發者，我要從 Plugins 選單設定及停用 GitHub App。
+3. 身為開發者，我要指定 App、installation、repository 與權限。
+4. 身為開發者，我要只在 Host pass 保存私鑰。
+5. 身為開發者，我要在啟用時發現認證或服務設定錯誤。
+6. 身為開發者，我要將個人設定與 plugin 設定分成多個片段。
+7. 身為開發者，我要預覽 deep merge 後的設定。
+8. 身為開發者，我要直接使用 git pull／push 及 gh pr create。
+9. 身為開發者，我要在 Token 到期時自動取得新 Token。
+10. 身為開發者，我要關閉 dch 後仍能向 Host 取得認證。
+11. 身為開發者，我要看見服務失敗與待 Build 狀態。
+12. 身為開發者，我要停用 plugin 後停止提供認證。
+13. 身為開發者，我要保留手動修改的片段。
+
+Plugins 提供設定、啟用、停用與狀態檢視。
+顯示服務狀態、待 Build 與可處理的錯誤。
+不增加 Token 到期倒數或更新排程介面。
+不顯示 Token、JWT 或私鑰內容。
+
+啟用時呈現授權範圍，驗證 pass、GitHub 與 systemd service。
+需驗證服務能在非互動條件下取得憑證。
+驗證失敗不標記啟用，保留原有有效設定。
+中途失敗清理此次新建資源，不覆寫同名使用者檔案。
+
+設定變更或停用後，既有容器顯示需要 Build。
+不自動重建，保留既有確認流程。
+停用會立即停止 Host 認證服務，不等待 Build。
+既有容器的認證設定恢復則需 Build。
+
+## C local.d 與 plugin 框架
+
+```flow LR
+devcontainer.json -&gt; local.d 排序片段: deep merge
+內建 plugin -&gt; 專屬 JSON 片段: 設定與掛載
+local.d 排序片段 -&gt; 合併結果: 預覽與執行共用
+合併結果 -&gt; Host 準備: 確保服務可用
+Host 準備 -&gt; devcontainer up: 掛載 Socket 目錄
+```
+
+使用 `.devcontainer/devcontainer.local.d/` 第一層的 `.json` 正常檔案。
+沿用 JSONC 解析，支援註解及 trailing comma。
+先讀 base，再按檔名位元組順序合併片段。
+缺少目錄代表沒有 local 設定；無效片段回報檔名並停止。
+
+Object 遞迴合併；array 追加並依 JSON 值去重。
+Scalar 與不同型別由後者覆蓋。
+必要 Socket mount 或初始化設定被覆寫時，停止啟動。
+
+舊 `.devcontainer/devcontainer.local.json` 不再讀取。
+不提示、不自動搬移，也不新增遷移功能。
+README、預覽提示與測試同步改為 local.d。
+這是不相容變更。
+
+每個 plugin 使用固定專屬片段。
+片段只放掛載與初始化設定，不放認證參數或 Token。
+Host 保存生成內容摘要，更新或刪除前比對。
+遇到手改內容停止並提示，不覆寫或刪除。
+其他片段保留；不擅自修改 Project 的忽略規則。
+
+Registry 使用穩定 plugin ID。
+提供設定驗證、片段生成、Host 準備與停用清理。
+容器初始化透過受管理的腳本與設定完成。
+未知的已啟用 plugin 明確報錯。
+保持簡單，不加入外部 plugin 協定或通用工作流引擎。
+
+沿用 Host JSON，保留既有 Project 路徑清單相容性。
+新增按正規化 Project 路徑索引的 plugin 設定。
+不同路徑使用獨立服務、Socket 與快取。
+未啟用 plugin 的 Project 不啟動認證服務。
+
+## D Unix Socket 認證架構
+
+```flow LR
+Git 指令 -&gt; Credential Helper
+Credential Helper -&gt; curl Unix Socket
+GH 指令 -&gt; gh wrapper
+ gh wrapper -&gt; curl Unix Socket
+curl Unix Socket -&gt; Host HTTP Server: HTTP over Unix Socket
+Host HTTP Server -&gt; 內建憑證取得邏輯
+內建憑證取得邏輯 -&gt; pass 與 GPG: 讀取私鑰
+內建憑證取得邏輯 -&gt; GitHub API: JWT 換 Installation Token
+GitHub API -&gt; Host HTTP Server: Token 留在記憶體快取
+Host HTTP Server -&gt; curl Unix Socket: 短期 Token
+```
+
+Host server 由 dch 內建，不額外拆出 get-github-credential 程式。
+每個 server 僅服務一個固定 Project。
+容器請求不能指定 pass entry、App、repository 或權限。
+HTTP 只監聽 Unix Socket，不開 TCP port。
+
+僅提供 GET /credential 與 GET /health。
+Credential 成功回傳純文字 Token，並設定 Cache-Control: no-store。
+Health 只回報服務存活，不呼叫 pass 或 GitHub。
+其他路徑或方法拒絕，不提供通用命令執行或 API proxy。
+錯誤回應不包含命令輸出、GitHub 回應本文或秘密。
+
+Socket 放在有效且由目前使用者擁有的 XDG_RUNTIME_DIR。
+每個 Project 使用短路徑摘要建立獨立目錄。
+目錄 0700、Socket 0600；拒絕不安全擁有者與 symlink。
+不退回工作區或共用暫存目錄。
+容器使用者需有權連接 Socket，UID 不符時明確報錯。
+
+將整個 Socket 目錄唯讀 bind mount 至 `/run/dch-github`。
+容器連線路徑為 `/run/dch-github/credential.sock`。
+不只掛載單一 Socket，讓服務重啟後的新 Socket 仍可見。
+服務重啟保留目錄；停用時才清理目錄。
+唯讀掛載不妨礙連線，也不限制已授權的認證請求。
+
+## E Token、Git 與 gh
+
+Console 收集 App ID、Installation ID、pass entry 與 owner/repository。
+Contents 與 pull_requests 分別可選 read／write。
+驗證 repository 屬於指定 owner，申請時限制到該 repo 與權限。
+GitHub App 建立、安裝與私鑰匯入 pass 由使用者先完成。
+
+以參數陣列呼叫 pass，不拼接 shell 指令。
+驗證 pass entry，拒絕空值與選項形式。
+支援 GitHub 提供的 RSA PEM 私鑰。
+JWT 使用 RS256；iat 回溯 60 秒、exp 不超過未來 10 分鐘。
+使用 App ID 作為 iss。
+
+GitHub HTTPS 請求設逾時、回應大小上限及 redirect 限制。
+Token 視為不透明字串，不假設長度。
+拒絕空值、換行與無效 expires_at。
+私鑰、JWT 與 Token 不進日誌、設定、預覽或命令參數。
+
+每個 Project 在 server 記憶體快取 Token 與 expires_at。
+有效期剩餘超過五分鐘時直接回傳快取。
+不足五分鐘或快取為空時，收到請求才重新取得 Token。
+並行請求共用一次取得流程，避免重複呼叫 pass 與 GitHub。
+服務重啟後首次請求重新取得，不保存 Token 到磁碟。
+
+取得失敗回報認證失敗，不回傳過期或接近到期的 Token。
+不做背景重試排程；下一次認證請求可再嘗試。
+Pass 與 GitHub 操作設逾時，服務不等待互動輸入。
+Host GPG Agent 必須已具備非互動解密能力。
+
+Git helper 僅對 HTTPS 與精確 github.com 回應認證。
+每次 get 用 curl --unix-socket 呼叫固定 endpoint。
+使用 x-access-token username；store／erase 不保存資料。
+其他 Host 保留原認證行為，SSH URL 不自動改寫。
+確保 github.com 不被既有 helper 搶先回傳其他憑證。
+
+Gh wrapper 透過相同 Socket 取得 Token。
+僅對該次 gh 子程序設定 GH_TOKEN。
+保留參數與 exit code，避免 wrapper 遞迴。
+Curl 使用固定 Socket、URL、逾時並停用 proxy 與個人 curl 設定。
+錯誤時停止，不帶空 Token 繼續執行 gh。
+
+容器需要既有 Git、gh、curl 與 POSIX shell。
+缺少依賴或無法連接 Socket 時提供明確錯誤。
+不自動安裝套件，不將 Token 寫入持久環境或 Git 設定。
+保留既有 lifecycle hooks，初始化可重複執行。
+Start／Build／Connect 使用一致的 helper 與 wrapper 接線。
+
+## F 服務生命週期
+
+每個 Project 使用一個 systemd user service，不建立 timer。
+Service 以 dch 的內部 server 入口執行，使用絕對路徑。
+不新增公開 dch up 或 plugin 管理 CLI。
+正確處理命令參數與 systemd escaping。
+Service 不攜帶秘密參數，不自動啟用 linger。
+
+啟用驗證會透過實際 service 請求一次 credential。
+Start／Build 確保服務可用，並驗證認證後才啟動容器。
+失敗則停止，回報可處理的錯誤。
+不需要額外的認證檢查 endpoint。
+
+關閉 Console 或 Stop 容器後保留 service。
+無認證請求時，不讀 pass、不呼叫 GitHub。
+Host 工作階段、GPG Agent 與 systemd 可用性仍是前提。
+再次 Start 時補齊服務及 runtime 目錄。
+
+修改認證設定需重啟服務，丟棄舊的記憶體快取。
+停用或移除 Project 時，先停止服務，再清理 Socket 與 unit。
+停止服務會終止進行中的請求，不再接受新的認證。
+清理失敗需回報，保留可重試的設定。
+移除 Project 仍保留工作區與容器，沿用原確認行為。
+
+同一 Project 的服務建立、設定與清理需互斥。
+不允許兩個程序接管同一 Socket。
+本機停止服務不會撤銷已回傳的 Token。
+
+## G 驗收與測試邊界
+
+| 邊界 | 驗收 |
+| --- | --- |
+| 設定合併 | 排序、JSONC、deep merge、array 去重、舊檔忽略 |
+| Plugin 設定 | 舊設定相容、跨 Project 隔離、片段手改保護 |
+| GitHub provider | 假 pass 與 httptest 驗證 JWT、scope、expiry 與錯誤 |
+| HTTP Unix Socket | 真實暫存 Socket 驗證 endpoint、方法、權限及錯誤 |
+| 記憶體快取 | 可控時間驗證重用、到期前更新、並行去重、失敗後重試 |
+| Systemd | 假 systemctl 驗證啟動、重啟、停止與清理失敗 |
+| 容器接線 | 假 devcontainer 驗證目錄掛載與 Start／Build／Connect |
+| Git | 真實 Git plumbing 驗證 Host 過濾與 Socket 取憑證 |
+| gh | 假 gh 驗證每次取憑證、GH_TOKEN、參數與 exit code |
+| Console | 啟用驗證、服務狀態、待 Build、停用與錯誤 |
+| 保密 | 設定、日誌、預覽、參數不含秘密 |
+
+使用可觀察行為測試，沿用 PATH 假指令慣例。
+一般測試不依賴真實 GitHub、Docker 或 systemd。
+以可控時間測試快取，不等待真實過期。
+每項行為遵守 red → green，整合後執行 go test ./... 及編譯。
+
+手動驗收涵蓋真實 Docker Socket 連線、UID 不符與服務重啟。
+也涵蓋 Git／gh 實際操作、關閉 dch 後認證與 GPG 失敗。
+驗證停用後不再提供憑證，且其他 Project 不受影響。
+未執行的環境驗收需在最終 review 明列。
+
+## H 安全邊界、範圍外與依據
+
+能連接 Socket 的容器程序可以取得該 Project 的短期 Token。
+這不是對不可信任容器的完整隔離。
+Docker 管理員仍能存取掛載，權限應限制在所需 repo。
+Branch Protection、Rulesets、Review 與 CI 權限另行設定。
+
+本次不支援外部 plugin、Compose、遠端 Docker 或非 Linux Host。
+不支援 GitHub Enterprise、多 repo Token 或 SSH 認證。
+不自動建立 GitHub App、安裝依賴或遷移舊 local.json。
+不掛載 pass、GPG Key 或 App Private Key。
+不實作 Token File、timer、排程刷新或持久 Token 快取。
+
+[GitHub JWT 官方文件](https://docs.github.com/en/apps/creating-github-apps/authenticating-with-a-github-app/generating-a-json-web-token-jwt-for-a-github-app)
+[Installation Token 官方文件](https://docs.github.com/en/apps/creating-github-apps/authenticating-with-a-github-app/generating-an-installation-access-token-for-a-github-app)
+[gh 環境變數](https://cli.github.com/manual/gh_help_environment)
+[curl Unix Socket 選項](https://curl.se/docs/manpage.html#--unix-socket)
+[Linux Unix Socket 權限](https://man7.org/linux/man-pages/man7/unix.7.html)
+[Docker bind mounts](https://docs.docker.com/engine/storage/bind-mounts/)
+
+此版規格與 Unix Socket 測試邊界已由使用者核准。
+接著確認本機 tickets 分工，再由子代理 TDD 實作。
+完成獨立 Code／Security Review 後提供最終 HTML review。
+最終 review 核准後才 push 與建立 PR。
</code></pre><p>已核准規格 HTML：2824 行新增。完整內容見永久 diff；此處不重複嵌入生成版面。</p></div>
```

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
| HTML diff 直接顯示 | PASS | 靜態檢查八組正常文流、37 份 diff、6 個未勾選框與連結 |
| HTML 瀏覽器／窄視窗 | NOT RUN | Chromium 導覽時程序關閉；未完成桌面／窄視窗實測 |

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
