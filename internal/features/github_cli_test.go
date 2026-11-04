package features_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestGitCredentialHelperOnlySuppliesGitHubHTTPS(t *testing.T) {
	dir := installFakeCurl(t)
	script := filepath.Join("..", "..", ".devcontainer", "features", "github-cli", "credential-helper.sh")
	cases := []struct {
		name, action, input string
		supplies            bool
	}{
		{"github HTTPS", "get", "protocol=https\nhost=github.com\n\n", true},
		{"other host", "get", "protocol=https\nhost=example.com\n\n", false},
		{"SSH", "get", "protocol=ssh\nhost=github.com\n\n", false},
		{"store", "store", "protocol=https\nhost=github.com\n\n", false},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			cmd := exec.Command("sh", script, test.action)
			cmd.Stdin = strings.NewReader(test.input)
			output, err := cmd.Output()
			if err != nil {
				t.Fatal(err)
			}
			if test.supplies {
				assertCredentialOutput(t, output)
			} else if len(output) != 0 {
				t.Fatal("supplied credential for unsupported operation")
			}
		})
	}
	args, err := os.ReadFile(filepath.Join(dir, "curl-args"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(args), "--unix-socket\n/run/dch-github-auth/pass.sock\n") {
		t.Fatal("helper did not use host socket")
	}
}

func assertCredentialOutput(t *testing.T, output []byte) {
	t.Helper()
	if string(output) != "username=x-access-token\npassword=github_pat_TEST_ONLY\n\n" {
		t.Fatal("unexpected credential format")
	}
}

func TestGitHubWrapperUsesHostPATForChildOnly(t *testing.T) {
	dir := installFakeCurl(t)
	real := filepath.Join(dir, "gh-real")
	script := "#!/bin/sh\nprintf '%s\\n' \"$GH_HOST\" \"$GH_TOKEN\" \"$GITHUB_TOKEN\" \"$@\"\n"
	if err := os.WriteFile(real, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	source, err := os.ReadFile(filepath.Join("..", "..", ".devcontainer", "features", "github-cli", "gh-wrapper.sh"))
	if err != nil {
		t.Fatal(err)
	}
	wrapper := filepath.Join(dir, "gh")
	source = bytes.ReplaceAll(source, []byte("/usr/local/lib/dch-github-auth/gh-real"), []byte(real))
	if err := os.WriteFile(wrapper, source, 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GH_TOKEN", "inherited-credential")
	output, err := exec.Command(wrapper, "repo", "view").Output()
	if err != nil {
		t.Fatal(err)
	}
	if string(output) != "github.com\ngithub_pat_TEST_ONLY\ngithub_pat_TEST_ONLY\nrepo\nview\n" {
		t.Fatal("wrapper did not pass host credential to gh")
	}
	if os.Getenv("GH_TOKEN") != "inherited-credential" {
		t.Fatal("changed parent environment")
	}
}

func TestAdaptersFailWithoutPrintingPATWhenSocketFails(t *testing.T) {
	dir := installFakeCurl(t)
	failing := "#!/bin/sh\nprintf 'github_pat_TEST_ONLY'\nexit 7\n"
	if err := os.WriteFile(filepath.Join(dir, "curl"), []byte(failing), 0700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"credential-helper.sh", "gh-wrapper.sh"} {
		cmd := exec.Command("sh", filepath.Join("..", "..", ".devcontainer", "features", "github-cli", name), "get")
		cmd.Stdin = strings.NewReader("protocol=https\nhost=github.com\n\n")
		output, err := cmd.CombinedOutput()
		if err == nil || bytes.Contains(output, []byte("github_pat_TEST_ONLY")) {
			t.Fatalf("unsafe adapter failure: %s", name)
		}
	}
}

func installFakeCurl(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$DCH_TEST_CURL_ARGS\"\nprintf 'github_pat_TEST_ONLY'\n"
	if err := os.WriteFile(filepath.Join(dir, "curl"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("DCH_TEST_CURL_ARGS", filepath.Join(dir, "curl-args"))
	return dir
}
