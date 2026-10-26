package githubapp

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestAdapterFragmentMountsRuntimeDirectoryAndAddsAdapterPath(t *testing.T) {
	fragment, err := AdapterFragment("/run/user/1000/dch/project")
	if err != nil {
		t.Fatal(err)
	}
	var config struct {
		Mounts    []string          `json:"mounts"`
		RemoteEnv map[string]string `json:"remoteEnv"`
	}
	if err := json.Unmarshal(fragment, &config); err != nil {
		t.Fatal(err)
	}
	wantMount := "source=/run/user/1000/dch/project,target=/run/dch-github,type=bind,readonly"
	if !reflect.DeepEqual(config.Mounts, []string{wantMount}) {
		t.Fatalf("mounts = %#v, want %#v", config.Mounts, []string{wantMount})
	}
	if got, want := config.RemoteEnv["PATH"], "/run/dch-github/bin:${containerEnv:PATH}"; got != want {
		t.Fatalf("PATH = %q, want %q", got, want)
	}
}

func TestAdapterFragmentRejectsUnrepresentableRuntimePaths(t *testing.T) {
	for _, socketDir := range []string{"relative/path", "/", "/runtime/with,comma", "/runtime/with\nnewline", "/runtime/${interpolation}"} {
		if _, err := AdapterFragment(socketDir); err == nil {
			t.Errorf("AdapterFragment(%q) succeeded, want invalid path error", socketDir)
		}
	}
}

func TestWriteAdapterScriptsCreatesPrivateExecutableScriptsAndRefreshesThem(t *testing.T) {
	runtimeDir := t.TempDir()
	if err := os.Chmod(runtimeDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := WriteAdapterScripts(runtimeDir); err != nil {
		t.Fatal(err)
	}
	gitConfig := filepath.Join(runtimeDir, "gitconfig")
	assertPrivateGitConfig(t, gitConfig)
	for name, want := range map[string]string{
		"git-credential-dch-github": "host=*",
		"gh":                        "GH_TOKEN=$token",
	} {
		path := filepath.Join(runtimeDir, "bin", name)
		assertPrivateExecutableScript(t, path, name, want)
	}
	if err := WriteAdapterScripts(runtimeDir); err != nil {
		t.Fatalf("second install: %v", err)
	}
}

func assertPrivateGitConfig(t *testing.T, path string) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("gitconfig mode = %v, want 0600", info.Mode())
	}
	contents, err := os.ReadFile(path)
	if err != nil || !strings.Contains(string(contents), "helper = ") || !strings.Contains(string(contents), "git-credential-dch-github") {
		t.Fatalf("gitconfig = %q, error %v", contents, err)
	}
}

func assertPrivateExecutableScript(t *testing.T, path, name, expected string) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !info.Mode().IsRegular() || info.Mode().Perm() != 0700 {
		t.Fatalf("%s mode = %v, want regular 0700", name, info.Mode())
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(contents), expected) {
		t.Fatalf("%s does not contain %q", name, expected)
	}
}

func TestWriteAdapterScriptsRejectsUnsafeRuntimeEntries(t *testing.T) {
	runtimeDir := t.TempDir()
	if err := os.Chmod(runtimeDir, 0700); err != nil {
		t.Fatal(err)
	}
	binDir := filepath.Join(runtimeDir, "bin")
	if err := os.Mkdir(binDir, 0700); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(runtimeDir, "target")
	if err := os.WriteFile(target, []byte("untouched"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(binDir, "gh")); err != nil {
		t.Fatal(err)
	}
	if err := WriteAdapterScripts(runtimeDir); err == nil {
		t.Fatal("expected unsafe symlink rejection")
	}
	contents, err := os.ReadFile(target)
	if err != nil || string(contents) != "untouched" {
		t.Fatalf("symlink target = %q, error %v", contents, err)
	}
}

func TestAdapterInstallCommandChecksRuntimeAndConfiguresScopedHelper(t *testing.T) {
	command := AdapterInstallCommand()
	if !reflect.DeepEqual(command[:2], []string{"/bin/sh", "-c"}) {
		t.Fatalf("install command = %#v", command)
	}
	script := command[2]
	for _, expected := range []string{
		"command -v \"$dependency\"",
		"PATH=$clean_path",
		"curl --disable --noproxy '*' --unix-socket /run/dch-github/credential.sock",
		"--fixed-value --get-all include.path",
		"--add include.path",
	} {
		if !strings.Contains(script, expected) {
			t.Errorf("install script does not contain %q", expected)
		}
	}
	if strings.Contains(script, "--replace-all credential.https://github.com.helper") {
		t.Fatal("install command replaces existing GitHub credential helpers")
	}
	if strings.Index(script, "PATH=$clean_path") > strings.Index(script, "command -v \"$dependency\"") {
		t.Fatal("install command checks gh before removing the adapter wrapper from PATH")
	}
}

func TestAdapterInstallCommandReportsUIDAndServicePermissionFailures(t *testing.T) {
	for _, testCase := range []struct {
		name         string
		dirMode      os.FileMode
		socketMode   os.FileMode
		curlExit     int
		wantMessage  string
		wantCurlCall bool
	}{
		{name: "runtime directory", dirMode: 0600, socketMode: 0600, wantMessage: "align the container UID with the host user"},
		{name: "socket permissions", dirMode: 0700, socketMode: 0400, wantMessage: "align the container UID with the host user"},
		{name: "health request", dirMode: 0700, socketMode: 0600, curlExit: 22, wantMessage: "service unavailable or socket permission denied", wantCurlCall: true},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			assertInstallerPermissionFailure(t, testCase)
		})
	}
}

func assertInstallerPermissionFailure(t *testing.T, testCase struct {
	name         string
	dirMode      os.FileMode
	socketMode   os.FileMode
	curlExit     int
	wantMessage  string
	wantCurlCall bool
}) {
	t.Helper()
	if os.Geteuid() == 0 && testCase.name != "health request" {
		t.Skip("root can bypass Unix DAC permission checks")
	}
	runtimeDir, bin, curlLog := makeInstallerFixture(t, testCase)
	command := AdapterInstallCommand()
	script := strings.ReplaceAll(command[2], "/run/dch-github", runtimeDir)
	cmd := exec.Command(command[0], command[1], script)
	cmd.Env = []string{"PATH=" + filepath.Join(runtimeDir, "bin") + ":" + bin}
	output, err := cmd.CombinedOutput()
	assertInstallerOutput(t, output, err, testCase.wantMessage)
	assertCurlCallState(t, curlLog, testCase.wantCurlCall)
}

func makeInstallerFixture(t *testing.T, testCase struct {
	name         string
	dirMode      os.FileMode
	socketMode   os.FileMode
	curlExit     int
	wantMessage  string
	wantCurlCall bool
}) (string, string, string) {
	t.Helper()
	root := t.TempDir()
	runtimeDir := filepath.Join(root, "runtime")
	if err := os.Mkdir(runtimeDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := WriteAdapterScripts(runtimeDir); err != nil {
		t.Fatal(err)
	}
	socketPath := filepath.Join(runtimeDir, "credential.sock")
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatal(err)
	}
	listener.(*net.UnixListener).SetUnlinkOnClose(false)
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(socketPath, testCase.socketMode); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(runtimeDir, 0700) })
	if err := os.Chmod(runtimeDir, testCase.dirMode); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(root, "bin")
	if err := os.Mkdir(bin, 0700); err != nil {
		t.Fatal(err)
	}
	curlLog := filepath.Join(root, "curl.log")
	writeExecutable(t, filepath.Join(bin, "git"), "#!/bin/sh\nexit 0\n")
	writeExecutable(t, filepath.Join(bin, "gh"), "#!/bin/sh\nexit 0\n")
	writeExecutable(t, filepath.Join(bin, "sh"), "#!/bin/sh\nexit 0\n")
	writeExecutable(t, filepath.Join(bin, "curl"), fmt.Sprintf("#!/bin/sh\nprintf called > %q\nexit %d\n", curlLog, testCase.curlExit))
	return runtimeDir, bin, curlLog
}

func assertInstallerOutput(t *testing.T, output []byte, err error, message string) {
	t.Helper()
	if err == nil || !strings.Contains(string(output), message) {
		t.Fatalf("installer output = %q, error %v; want failure mentioning %q", output, err, message)
	}
}

func assertCurlCallState(t *testing.T, path string, wantCall bool) {
	t.Helper()
	_, err := os.Stat(path)
	if wantCall && err != nil {
		t.Fatalf("curl was not called, stat error %v", err)
	}
	if !wantCall && !os.IsNotExist(err) {
		t.Fatalf("curl was called before permission checks, stat error %v", err)
	}
}

func TestGitCredentialAdapterScopesTokenAndPreservesOtherHosts(t *testing.T) {
	fixture := newGitCredentialFixture(t)
	configureGitCredentialConfig(t, fixture)
	assertGitCredentialResults(t, fixture)
	assertGitCredentialFallback(t, fixture)
	assertGitCredentialOperations(t, fixture)
}

type gitCredentialFixture struct {
	root, runtimeDir, bin, curlLog            string
	config, otherHelper, previousGithubHelper string
}

func newGitCredentialFixture(t *testing.T) gitCredentialFixture {
	t.Helper()
	root := t.TempDir()
	runtimeDir := filepath.Join(root, "runtime")
	if err := os.Mkdir(runtimeDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := WriteAdapterScripts(runtimeDir); err != nil {
		t.Fatal(err)
	}
	adapterConfigPath := filepath.Join(runtimeDir, "gitconfig")
	adapterConfig, err := os.ReadFile(adapterConfigPath)
	if err != nil {
		t.Fatal(err)
	}
	adapterConfig = []byte(strings.ReplaceAll(string(adapterConfig), "/run/dch-github", runtimeDir))
	if err := os.WriteFile(adapterConfigPath, adapterConfig, 0600); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(root, "bin")
	if err := os.Mkdir(bin, 0700); err != nil {
		t.Fatal(err)
	}
	curlLog := filepath.Join(root, "curl.log")
	writeExecutable(t, filepath.Join(bin, "curl"), fmt.Sprintf("#!/bin/sh\nprintf '%%s\\n' \"$*\" >> %q\nprintf 'fresh-token'\n", curlLog))
	otherHelper := filepath.Join(root, "other-helper")
	writeExecutable(t, otherHelper, "#!/bin/sh\n[ \"${1-}\" = get ] || exit 0\nprotocol=; host=\nwhile IFS= read -r line; do case \"$line\" in protocol=*) protocol=${line#protocol=} ;; host=*) host=${line#host=} ;; esac; done\n[ \"$protocol\" = https ] && [ \"$host\" = github.com ] && exit 0\nprintf 'username=other-user\\npassword=other-token\\n\\n'\n")
	previousGithubHelper := filepath.Join(root, "previous-github-helper")
	writeExecutable(t, previousGithubHelper, "#!/bin/sh\n[ \"${1-}\" = get ] || exit 0\nprintf 'username=old-user\\npassword=old-token\\n\\n'\n")
	return gitCredentialFixture{
		root: root, runtimeDir: runtimeDir, bin: bin, curlLog: curlLog,
		config: filepath.Join(root, "gitconfig"), otherHelper: otherHelper, previousGithubHelper: previousGithubHelper,
	}
}

func configureGitCredentialConfig(t *testing.T, fixture gitCredentialFixture) {
	t.Helper()
	config := fixture.config
	for _, args := range [][]string{
		{"--file", config, "--add", "credential.helper", fixture.otherHelper},
		{"--file", config, "--add", "credential.https://github.com.helper", fixture.previousGithubHelper},
	} {
		if output, err := exec.Command("git", append([]string{"config"}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git config: %v: %s", err, output)
		}
	}
	includePath := filepath.Join(fixture.runtimeDir, "gitconfig")
	for range 2 {
		ensureGitIncludePath(t, config, includePath)
	}
}

func ensureGitIncludePath(t *testing.T, config, includePath string) {
	t.Helper()
	args := []string{"config", "--file", config, "--fixed-value", "--get-all", "include.path", includePath}
	if err := exec.Command("git", args...).Run(); err != nil {
		addArgs := []string{"config", "--file", config, "--add", "include.path", includePath}
		if output, addErr := exec.Command("git", addArgs...).CombinedOutput(); addErr != nil {
			t.Fatalf("add include path: %v: %s", addErr, output)
		}
	}
}

func assertGitCredentialResults(t *testing.T, fixture gitCredentialFixture) {
	t.Helper()
	fill := fixture.fill(t)
	github, err := fill(fixture.config, "https://github.com/example/repo")
	if err != nil || !strings.Contains(github, "username=x-access-token") || !strings.Contains(github, "password=fresh-token") {
		t.Fatalf("github credentials = %q, error %v", github, err)
	}
	other, err := fill(fixture.config, "https://gitlab.com/example/repo")
	if err != nil || !strings.Contains(other, "username=other-user") || !strings.Contains(other, "password=other-token") {
		t.Fatalf("other host credentials = %q, error %v", other, err)
	}
	assertFallbackHosts(t, fill, fixture.config)
}

func assertFallbackHosts(t *testing.T, fill func(string, string) (string, error), config string) {
	t.Helper()
	for _, url := range []string{"http://github.com/example/repo", "https://github.com.evil/example/repo"} {
		fallback, err := fill(config, url)
		if err != nil || !strings.Contains(fallback, "password=other-token") {
			t.Fatalf("credentials for %s = %q, error %v", url, fallback, err)
		}
	}
}

func assertGitCredentialFallback(t *testing.T, fixture gitCredentialFixture) {
	t.Helper()
	fallbackConfig := filepath.Join(fixture.root, "gitconfig-without-adapter")
	for _, args := range [][]string{
		{"--file", fallbackConfig, "--add", "credential.helper", fixture.otherHelper},
		{"--file", fallbackConfig, "--add", "credential.https://github.com.helper", fixture.previousGithubHelper},
		{"--file", fallbackConfig, "--add", "include.path", filepath.Join(fixture.runtimeDir, "missing-gitconfig")},
	} {
		if output, err := exec.Command("git", append([]string{"config"}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git config: %v: %s", err, output)
		}
	}
	previous, err := fixture.fill(t)(fallbackConfig, "https://github.com/example/repo")
	if err != nil || !strings.Contains(previous, "password=old-token") {
		t.Fatalf("credentials after runtime config removal = %q, error %v", previous, err)
	}
}

func assertGitCredentialOperations(t *testing.T, fixture gitCredentialFixture) {
	t.Helper()
	for _, operation := range []string{"approve", "reject"} {
		cmd := exec.Command("git", "credential", operation)
		cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL="+fixture.config, "HOME="+fixture.root, "PATH="+fixture.bin+":"+os.Getenv("PATH"))
		cmd.Stdin = strings.NewReader("protocol=https\nhost=github.com\nusername=manual\npassword=manual-secret\n\n")
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git credential %s: %v: %s", operation, err, output)
		}
	}
	calls, err := os.ReadFile(fixture.curlLog)
	if err != nil || !strings.HasPrefix(string(calls), "--disable --noproxy * --unix-socket /run/dch-github/credential.sock") || !strings.Contains(string(calls), "http://localhost/credential") {
		t.Fatalf("curl calls = %q, error %v", calls, err)
	}
}

func (fixture gitCredentialFixture) fill(t *testing.T) func(string, string) (string, error) {
	t.Helper()
	return func(configPath, url string) (string, error) {
		cmd := exec.Command("git", "credential", "fill")
		cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL="+configPath, "HOME="+fixture.root, "PATH="+fixture.bin+":"+os.Getenv("PATH"))
		cmd.Stdin = strings.NewReader("url=" + url + "\n\n")
		output, err := cmd.CombinedOutput()
		return string(output), err
	}
}

func TestGHWrapperFetchesEachTimeAndPreservesArgumentsAndExitStatus(t *testing.T) {
	root := t.TempDir()
	runtimeDir := filepath.Join(root, "runtime")
	if err := os.Mkdir(runtimeDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := WriteAdapterScripts(runtimeDir); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(root, "bin")
	if err := os.Mkdir(bin, 0700); err != nil {
		t.Fatal(err)
	}
	counter := filepath.Join(root, "counter")
	curlLog := filepath.Join(root, "curl.log")
	writeExecutable(t, filepath.Join(bin, "curl"), fmt.Sprintf("#!/bin/sh\nprintf '%%s\\n' \"$*\" >> %q\nn=0; [ ! -f %q ] || n=$(cat %q); n=$((n + 1)); printf '%%s' \"$n\" > %q; printf 'token-%%s' \"$n\"\n", curlLog, counter, counter, counter))
	ghLog := filepath.Join(root, "gh.log")
	writeExecutable(t, filepath.Join(bin, "gh"), fmt.Sprintf("#!/bin/sh\nprintf '%%s\\n' \"$GH_TOKEN\" >> %q\nprintf '<%%s>\\n' \"$@\" >> %q\nexit 23\n", ghLog, ghLog))
	wrapper := filepath.Join(runtimeDir, "bin", "gh")
	runGHWrapperTwice(t, wrapper, bin)
	assertGHWrapperLogs(t, ghLog, curlLog)
}

func runGHWrapperTwice(t *testing.T, wrapper, bin string) {
	t.Helper()
	for range 2 {
		cmd := exec.Command(wrapper, "pr", "create", "--title", "a title")
		cmd.Env = append(os.Environ(), "PATH=/run/dch-github/bin:"+bin+":/run/dch-github/bin:"+os.Getenv("PATH"), "GH_TOKEN=ambient-token")
		if err := cmd.Run(); err == nil {
			t.Fatal("expected wrapped gh exit 23")
		} else if exit, ok := err.(*exec.ExitError); !ok || exit.ExitCode() != 23 {
			t.Fatalf("gh exit = %v, want 23", err)
		}
	}
}

func assertGHWrapperLogs(t *testing.T, ghLog, curlLog string) {
	t.Helper()
	contents, err := os.ReadFile(ghLog)
	if err != nil {
		t.Fatal(err)
	}
	want := "token-1\n<pr>\n<create>\n<--title>\n<a title>\ntoken-2\n<pr>\n<create>\n<--title>\n<a title>\n"
	if string(contents) != want {
		t.Fatalf("gh invocations = %q, want %q", contents, want)
	}
	curlArgs, err := os.ReadFile(curlLog)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(curlArgs), "--disable --noproxy * --unix-socket /run/dch-github/credential.sock") {
		t.Fatalf("curl does not begin with fixed config and proxy options: %q", curlArgs)
	}
	for _, required := range []string{"--disable", "--noproxy *", "--unix-socket /run/dch-github/credential.sock", "--max-time 125", "http://localhost/credential"} {
		if !strings.Contains(string(curlArgs), required) {
			t.Errorf("curl arguments %q missing %q", curlArgs, required)
		}
	}
	if !strings.Contains(ghWrapperScript, "unlock the host GPG agent in a terminal and retry") {
		t.Fatal("adapter does not explain how to retry on a headless host")
	}
	if !strings.Contains(ghWrapperScript, "reconfigure GitHub authorization") {
		t.Fatal("adapter does not explain how to reconfigure an invalid PAT")
	}
}

func TestGHWrapperDoesNotRunGHWithoutCredential(t *testing.T) {
	for _, curlBody := range []string{"exit 22", "exit 0"} {
		t.Run(curlBody, func(t *testing.T) {
			assertGHWrapperNoCredential(t, curlBody)
		})
	}
}

func assertGHWrapperNoCredential(t *testing.T, curlBody string) {
	t.Helper()
	root := t.TempDir()
	runtimeDir := filepath.Join(root, "runtime")
	if err := os.Mkdir(runtimeDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := WriteAdapterScripts(runtimeDir); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(root, "bin")
	if err := os.Mkdir(bin, 0700); err != nil {
		t.Fatal(err)
	}
	writeExecutable(t, filepath.Join(bin, "curl"), "#!/bin/sh\n"+curlBody+"\n")
	ghLog := filepath.Join(root, "gh.log")
	writeExecutable(t, filepath.Join(bin, "gh"), fmt.Sprintf("#!/bin/sh\nprintf 'called' > %q\n", ghLog))
	cmd := exec.Command(filepath.Join(runtimeDir, "bin", "gh"), "pr", "create")
	cmd.Env = append(os.Environ(), "PATH=/run/dch-github/bin:"+bin+":"+os.Getenv("PATH"))
	assertWrapperExitsWithoutCredential(t, cmd)
	if _, err := os.Stat(ghLog); !os.IsNotExist(err) {
		t.Fatalf("gh ran without credential, stat error %v", err)
	}
}

func assertWrapperExitsWithoutCredential(t *testing.T, cmd *exec.Cmd) {
	t.Helper()
	if err := cmd.Run(); err == nil {
		t.Fatal("expected credential failure")
	} else if exit, ok := err.(*exec.ExitError); !ok || exit.ExitCode() != 1 {
		t.Fatalf("wrapper exit = %v, want 1", err)
	}
}

func TestGHWrapperReportsMissingUnderlyingGHBeforeRequestingCredential(t *testing.T) {
	runtimeDir := t.TempDir()
	if err := os.Chmod(runtimeDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := WriteAdapterScripts(runtimeDir); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(filepath.Join(runtimeDir, "bin", "gh"), "version")
	cmd.Env = []string{"PATH=/run/dch-github/bin"}
	output, err := cmd.CombinedOutput()
	exit, ok := err.(*exec.ExitError)
	if !ok || exit.ExitCode() != 127 || !strings.Contains(string(output), "dch: gh is required") {
		t.Fatalf("missing gh result = %q, error %v", output, err)
	}
}

func writeExecutable(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0700); err != nil {
		t.Fatal(err)
	}
}
