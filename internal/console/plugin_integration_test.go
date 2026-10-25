package console

import (
	"context"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"dch/internal/common"
	"dch/internal/githubapp"
	"dch/internal/plugins"
)

func TestValidatePluginConfigRequiresExactSocketMountAndAdapterPath(t *testing.T) {
	root := t.TempDir()
	project := filepath.Join(root, "project")
	devcontainerDir := filepath.Join(project, ".devcontainer")
	if err := os.MkdirAll(filepath.Join(devcontainerDir, "devcontainer.local.d"), 0700); err != nil {
		t.Fatal(err)
	}
	runtime, err := os.MkdirTemp("/tmp", "dchrt")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(runtime)
	t.Setenv("XDG_RUNTIME_DIR", runtime)
	socketDir, err := githubapp.RuntimeDir(project)
	if err != nil {
		t.Fatal(err)
	}
	fragment, err := githubapp.AdapterFragment(socketDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(devcontainerDir, "devcontainer.local.d", "90-plugin.json"), fragment, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(devcontainerDir, "devcontainer.json"), []byte(`{"image":"example:latest","postCreateCommand":"echo keep"}`), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DCH_CONFIG", filepath.Join(root, "dch-config"))
	if err := common.SaveConfig(common.Config{Version: 1, Projects: []string{project}, ProjectPlugins: map[string]common.ProjectPlugins{project: {Enabled: map[string]githubapp.Config{"github-app": {AppID: 1}}}}}); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(root, "bin")
	if err := os.Mkdir(bin, 0700); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(root, "devcontainer-called")
	script := "#!/bin/sh\nconfig=\nprev=\nfor arg in \"$@\"; do if [ \"$prev\" = --config ]; then config=$arg; fi; prev=$arg; done\n[ -f \"$config\" ] || exit 12\ntouch " + shellQuote(marker) + "\n"
	if err := os.WriteFile(filepath.Join(bin, "devcontainer"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	if err := invokeProjectDevcontainer(Project{Name: "demo", Path: project}, "exec", "--workspace-folder", project, "true"); err != nil {
		t.Fatalf("valid merged config: %v", err)
	}
	if err := os.Remove(marker); err != nil {
		t.Fatal(err)
	}
	assertInvalidPluginConfigsRejected(t, devcontainerDir, project, marker)
	if err := os.WriteFile(filepath.Join(devcontainerDir, "devcontainer.json"), []byte(`{"dockerComposeFile":"compose.yaml"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := invokeProjectDevcontainer(Project{Name: "demo", Path: project}, "exec", "--workspace-folder", project, "true"); err == nil {
		t.Fatal("Compose configuration was accepted")
	}
}

func assertInvalidPluginConfigsRejected(t *testing.T, devcontainerDir, project, marker string) {
	t.Helper()
	conflicts := []string{
		`{"mounts":["type=bind,source=/evil,destination=/run/dch-github"]}`,
		`{"mounts":["type=bind,source=/evil,dst=/run/dch-github/"]}`,
		`{"mounts":["type=bind,source=/evil,target=/run/dch-github/bin"]}`,
		`{"mounts":[{"type":"bind","source":"/evil","target":"/run/dch-github/bin"}]}`,
		`{"mounts":["type=bind,source=/evil,target=/run/dch-github,type=bind,target=/else"]}`,
		`{"remoteEnv":{"PATH":"/usr/bin"}}`,
	}
	for _, conflict := range conflicts {
		if err := os.WriteFile(filepath.Join(devcontainerDir, "devcontainer.local.d", "99-conflict.json"), []byte(conflict), 0600); err != nil {
			t.Fatal(err)
		}
		if err := invokeProjectDevcontainer(Project{Name: "demo", Path: project}, "exec", "--workspace-folder", project, "true"); err == nil {
			t.Errorf("conflicting merged config accepted: %s", conflict)
		}
		if _, err := os.Stat(marker); !os.IsNotExist(err) {
			t.Fatalf("devcontainer ran with invalid merged config: %s", conflict)
		}
		if err := os.Remove(filepath.Join(devcontainerDir, "devcontainer.local.d", "99-conflict.json")); err != nil {
			t.Fatal(err)
		}
	}
}

func TestPluginRejectsRemoteDockerHostAndContext(t *testing.T) {
	root := t.TempDir()
	bin := filepath.Join(root, "bin")
	if err := os.Mkdir(bin, 0700); err != nil {
		t.Fatal(err)
	}
	docker := filepath.Join(bin, "docker")
	if err := os.WriteFile(docker, []byte("#!/bin/sh\nprintf '%s' '[{\"Endpoints\":{\"docker\":{\"Host\":\"tcp://remote:2375\"}}}]'\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("DOCKER_HOST", "")
	t.Setenv("DOCKER_CONTEXT", "")
	if err := requireLocalDocker(); err == nil || !strings.Contains(err.Error(), "local Docker") {
		t.Fatalf("remote context error=%v", err)
	}
	t.Setenv("DOCKER_HOST", "tcp://remote:2375")
	if err := requireLocalDocker(); err == nil || !strings.Contains(err.Error(), "local Docker") {
		t.Fatalf("remote DOCKER_HOST error=%v", err)
	}
	t.Setenv("DOCKER_HOST", "unix:///run/user/1000/docker.sock")
	if err := requireLocalDocker(); err != nil {
		t.Fatalf("local socket rejected: %v", err)
	}
	t.Setenv("DOCKER_CONTEXT", "remote")
	if err := requireLocalDocker(); err == nil || !strings.Contains(err.Error(), "local Docker") {
		t.Fatalf("remote context did not override local DOCKER_HOST: %v", err)
	}
}

func TestStartRunsAdapterInstallAfterUpAndMarksBuildApplied(t *testing.T) {
	fixture := setupStartPluginFixture(t)
	if err := startProject(Project{Name: "demo", Path: fixture.project, State: "NOT CREATED"}); err != nil {
		t.Fatal(err)
	}
	assertSuccessfulPluginStart(t, fixture)
	assertFailedAdapterInstall(t, fixture)
}

type startPluginFixture struct {
	project, configDir, argsLog, configLog, labelsLog string
	cfg                                               githubapp.Config
}

func setupStartPluginFixture(t *testing.T) startPluginFixture {
	t.Helper()
	root := t.TempDir()
	project := filepath.Join(root, "project")
	configDir := filepath.Join(project, ".devcontainer")
	if err := os.MkdirAll(configDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "devcontainer.json"), []byte(`{"image":"example:latest","postCreateCommand":"echo keep"}`), 0600); err != nil {
		t.Fatal(err)
	}
	runtime, err := os.MkdirTemp("/tmp", "dchrt")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(runtime) })
	configHome := filepath.Join(root, "config-home")
	if err := os.Mkdir(configHome, 0700); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(root, "bin")
	if err := os.Mkdir(bin, 0700); err != nil {
		t.Fatal(err)
	}
	argsLog, configLog, labelsLog := filepath.Join(root, "args.log"), filepath.Join(root, "merged.json"), filepath.Join(root, "labels.log")
	scripts := map[string]string{
		"systemctl":    "#!/bin/sh\nexit 0\n",
		"docker":       "#!/bin/sh\nprintf '%s' '[{\"Endpoints\":{\"docker\":{\"Host\":\"unix:///var/run/docker.sock\"}}}]'\n",
		"devcontainer": "#!/bin/sh\nprintf '%s\\n' \"$1\" >> " + shellQuote(argsLog) + "\nif [ \"$1\" = exec ] && [ \"$DCH_TEST_FAIL_INSTALL\" = yes ]; then echo 'dch: required command missing: gh' >&2; exit 13; fi\nconfig=\nprev=\nfor arg in \"$@\"; do case \"$arg\" in devcontainer.local_folder=*|devcontainer.config_file=*) printf '%s\\n' \"$arg\" >> " + shellQuote(labelsLog) + " ;; esac; if [ \"$prev\" = --config ]; then config=$arg; fi; prev=$arg; done\n[ -f \"$config\" ] || exit 12\ncat \"$config\" >> " + shellQuote(configLog) + "\nprintf '\\n' >> " + shellQuote(configLog) + "\n",
	}
	for name, content := range scripts {
		if err := os.WriteFile(filepath.Join(bin, name), []byte(content), 0700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("DCH_CONFIG", filepath.Join(root, "dch-config"))
	t.Setenv("XDG_RUNTIME_DIR", runtime)
	t.Setenv("XDG_CONFIG_HOME", configHome)
	t.Setenv("DOCKER_HOST", "")
	t.Setenv("DOCKER_CONTEXT", "")
	t.Setenv("DCH_TEST_FAIL_INSTALL", "")
	runtimeDir, err := githubapp.RuntimeDir(project)
	if err != nil {
		t.Fatal(err)
	}
	socket := filepath.Join(runtimeDir, "credential.sock")
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	credentialServer := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("opaque-token"))
	})}
	go credentialServer.Serve(listener)
	t.Cleanup(func() { _ = credentialServer.Close() })
	cfg := githubapp.Config{AppID: 1, InstallationID: 2, PrivateKeyPassEntry: "test/key", Repository: "octo/repo", Permissions: map[string]string{"contents": "read"}}
	if err := plugins.NewManager().Enable(context.Background(), project, cfg); err != nil {
		t.Fatal(err)
	}
	devNull, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	oldStdout := os.Stdout
	os.Stdout = devNull
	t.Cleanup(func() { os.Stdout = oldStdout; _ = devNull.Close() })
	return startPluginFixture{project: project, configDir: configDir, argsLog: argsLog, configLog: configLog, labelsLog: labelsLog, cfg: cfg}
}

func assertSuccessfulPluginStart(t *testing.T, fixture startPluginFixture) {
	t.Helper()
	status, err := plugins.NewManager().Status(context.Background(), fixture.project)
	if err != nil {
		t.Fatal(err)
	}
	if status.PendingBuild {
		t.Fatal("pending Build remained after successful create and adapter install")
	}
	if err := connectProject(Project{Name: "demo", Path: fixture.project, State: "RUNNING", ID: "container-id"}); err != nil {
		t.Fatal(err)
	}
	args, err := os.ReadFile(fixture.argsLog)
	if err != nil {
		t.Fatal(err)
	}
	got := string(args)
	if got != "up\nexec\nexec\nexec\n" {
		t.Fatalf("devcontainer actions=%q", got)
	}
	labels, err := os.ReadFile(fixture.labelsLog)
	if err != nil {
		t.Fatal(err)
	}
	for _, label := range []string{"devcontainer.local_folder=" + fixture.project, "devcontainer.config_file=" + filepath.Join(fixture.configDir, "devcontainer.json")} {
		if strings.Count(string(labels), label) != 4 {
			t.Errorf("stable label %q count=%d labels=%s", label, strings.Count(string(labels), label), labels)
		}
	}
	merged, err := os.ReadFile(fixture.configLog)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(merged), "postCreateCommand") != 4 || !strings.Contains(string(merged), "/run/dch-github") {
		t.Fatalf("container config did not preserve hooks and plugin mount: %s", merged)
	}
}

func assertFailedAdapterInstall(t *testing.T, fixture startPluginFixture) {
	t.Helper()
	if err := plugins.NewManager().Enable(context.Background(), fixture.project, fixture.cfg); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DCH_TEST_FAIL_INSTALL", "yes")
	err := startProject(Project{Name: "demo", Path: fixture.project, State: "NOT CREATED"})
	if err == nil || !strings.Contains(err.Error(), "required command missing: gh") {
		t.Fatalf("start error = %v, want actionable adapter install diagnostic", err)
	}
	status, err := plugins.NewManager().Status(context.Background(), fixture.project)
	if err != nil {
		t.Fatal(err)
	}
	if !status.PendingBuild {
		t.Fatal("failed adapter install cleared pending Build")
	}
}

func TestRemoveProjectPreservesRegistrationWhenDisableFails(t *testing.T) {
	root := t.TempDir()
	project := filepath.Join(root, "project")
	if err := os.MkdirAll(project, 0700); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(root, "bin")
	if err := os.Mkdir(bin, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bin, "systemctl"), []byte("#!/bin/sh\nshift\nif [ \"$1\" = stop ]; then echo stop-denied >&2; exit 1; fi\nexit 0\n"), 0700); err != nil {
		t.Fatal(err)
	}
	runtime, err := os.MkdirTemp("/tmp", "dchrt")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(runtime)
	configHome := filepath.Join(root, "config-home")
	if err := os.Mkdir(configHome, 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("XDG_RUNTIME_DIR", runtime)
	t.Setenv("XDG_CONFIG_HOME", configHome)
	t.Setenv("DCH_CONFIG", filepath.Join(root, "dch-config"))
	config := common.Config{Version: 1, Projects: []string{project}, ProjectPlugins: map[string]common.ProjectPlugins{project: {Enabled: map[string]githubapp.Config{"github-app": {AppID: 1}}, States: map[string]common.PluginState{"github-app": {BuildPending: true}}}}}
	if err := common.SaveConfig(config); err != nil {
		t.Fatal(err)
	}
	err = removeProject(Project{Name: "demo", Path: project})
	if err == nil || !strings.Contains(err.Error(), "stop-denied") {
		t.Fatalf("remove error=%v", err)
	}
	loaded, err := common.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Projects) != 1 {
		t.Fatal("project was unregistered despite cleanup failure")
	}
	if _, enabled := loaded.ProjectPlugins[project].Enabled["github-app"]; !enabled {
		t.Fatal("enabled plugin settings were removed after cleanup failure")
	}
}

func TestRemoveProjectRefusesUnknownPluginMetadata(t *testing.T) {
	root := t.TempDir()
	project := filepath.Join(root, "project")
	if err := os.MkdirAll(project, 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DCH_CONFIG", filepath.Join(root, "dch-config"))
	config := common.Config{Version: 1, Projects: []string{project}, ProjectPlugins: map[string]common.ProjectPlugins{project: {States: map[string]common.PluginState{"future-plugin": {BuildPending: true}}}}}
	if err := common.SaveConfig(config); err != nil {
		t.Fatal(err)
	}
	err := removeProject(Project{Name: "demo", Path: project})
	if err == nil || !strings.Contains(err.Error(), "unknown plugin state") {
		t.Fatalf("remove error=%v", err)
	}
	loaded, err := common.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Projects) != 1 {
		t.Fatal("unknown plugin data was removed")
	}
}
