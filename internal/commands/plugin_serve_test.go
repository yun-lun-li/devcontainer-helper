package commands

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestInternalGitHubAppServerCommandDispatchesAndValidatesArguments(t *testing.T) {
	root := t.TempDir()
	t.Setenv("DCH_CONFIG", filepath.Join(root, "config"))
	err := Run([]string{"_github-app-serve", "--project", filepath.Join(root, "project")}, nil)
	if err == nil || !strings.Contains(err.Error(), "not enabled") {
		t.Fatalf("internal server dispatch error=%v", err)
	}
	err = Run([]string{"_github-app-serve", "--project"}, nil)
	if err == nil || !strings.Contains(err.Error(), "usage") {
		t.Fatalf("invalid invocation error=%v", err)
	}
}
