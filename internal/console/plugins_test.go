package console

import (
	"bufio"
	"strings"
	"testing"
)

func TestPromptGitHubAppConfigCollectsAndValidatesAuthorizedScopes(t *testing.T) {
	previous := input
	input = bufio.NewReader(strings.NewReader("12\n34\ngithub/key\norg/repo\n1\n2\n1\n"))
	defer func() { input = previous }()
	cfg, err := promptGitHubAppConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.AppID != 12 || cfg.InstallationID != 34 || cfg.Repository != "org/repo" || cfg.Permissions["contents"] != "read" || cfg.Permissions["pull_requests"] != "write" {
		t.Fatalf("unexpected prompt config: %+v", cfg)
	}
}

func TestPromptGitHubAppConfigAllowsCancel(t *testing.T) {
	previous := input
	input = bufio.NewReader(strings.NewReader("q\n"))
	defer func() { input = previous }()
	if _, err := promptGitHubAppConfig(); err != errCancelled {
		t.Fatalf("cancel error = %v, want errCancelled", err)
	}
}
