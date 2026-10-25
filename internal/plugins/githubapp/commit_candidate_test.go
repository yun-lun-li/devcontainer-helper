package githubapp

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"dch/internal/plugins"
)

func TestCommitCandidateRepairsMissingSameClientCredential(t *testing.T) {
	env := setupHostHookTest(t, true)
	cfg := testConfig(t.TempDir())
	oldRaw, _ := json.Marshal(cfg)
	candidateRaw, _ := json.Marshal(cfg)
	passPath, entryPath := fakeRepairPass(t, "missing")
	plugin := &Plugin{PassPath: passPath}
	listener := readyCredentialSocket(t, cfg.ProjectPath)
	defer listener.Close()
	store := &testScopedStore{}
	host := plugins.HostContext{ProjectPath: cfg.ProjectPath, Config: candidateRaw, PreviousConfig: oldRaw, Store: store}
	now := time.Now()
	tokens := TokenSet{Schema: 1, ClientID: cfg.ClientID, ProjectPath: cfg.ProjectPath, UserID: cfg.UserID, RepositoryID: cfg.RepositoryID, Repository: cfg.Repository, AccessToken: "fresh-access", RefreshToken: "fresh-refresh", AccessTokenExpiresAt: now.Add(time.Hour), RefreshTokenExpiresAt: now.Add(24 * time.Hour)}
	result, err := plugin.commitCandidate(context.Background(), host, cfg, tokens, candidateRaw)
	if err != nil {
		t.Fatal(err)
	}
	if result.NeedsBuild {
		t.Fatal("reauthorization unexpectedly added a pending build")
	}
	data, err := os.ReadFile(entryPath)
	if err != nil {
		t.Fatal(err)
	}
	var saved TokenSet
	if err := json.Unmarshal(data, &saved); err != nil {
		t.Fatal(err)
	}
	if saved.AccessToken != tokens.AccessToken || string(store.config) != string(candidateRaw) {
		t.Fatalf("credential/config not committed: token=%q config=%s", saved.AccessToken, store.config)
	}
	if got := strings.TrimSpace(readTestFile(t, filepath.Join(env.root, "systemctl.log"))); got == "" {
		t.Fatal("credential repair did not activate the managed service")
	}
}

func TestCommitCandidateDoesNotTreatPassFailureAsMissingCredential(t *testing.T) {
	setupHostHookTest(t, true)
	cfg := testConfig(t.TempDir())
	raw, _ := json.Marshal(cfg)
	passPath, entryPath := fakeRepairPass(t, "gpg-error")
	plugin := &Plugin{PassPath: passPath}
	now := time.Now()
	tokens := TokenSet{Schema: 1, ClientID: cfg.ClientID, ProjectPath: cfg.ProjectPath, UserID: cfg.UserID, RepositoryID: cfg.RepositoryID, Repository: cfg.Repository, AccessToken: "candidate", RefreshToken: "refresh", AccessTokenExpiresAt: now.Add(time.Hour), RefreshTokenExpiresAt: now.Add(24 * time.Hour)}
	if _, err := plugin.commitCandidate(context.Background(), plugins.HostContext{ProjectPath: cfg.ProjectPath, Config: raw, PreviousConfig: raw, Store: &testScopedStore{}}, cfg, tokens, raw); err == nil {
		t.Fatal("GPG/pass error was treated as a missing entry")
	}
	if _, err := os.Stat(entryPath); !os.IsNotExist(err) {
		t.Fatalf("candidate entry was written: %v", err)
	}
}

func fakeRepairPass(t *testing.T, mode string) (string, string) {
	t.Helper()
	passPath, entry := filepath.Join(t.TempDir(), "pass"), filepath.Join(t.TempDir(), "secret.json")
	script := "#!/bin/sh\ncase \"$1\" in\nshow) if [ -f \"$DCH_REPAIR_ENTRY\" ]; then cat \"$DCH_REPAIR_ENTRY\"; else echo 'not in the password store' >&2; exit 1; fi ;;\ninsert) cat > \"$DCH_REPAIR_ENTRY\" ;;\nrm) rm -f \"$DCH_REPAIR_ENTRY\" ;;\nesac\n"
	if mode == "gpg-error" {
		script = "#!/bin/sh\ncase \"$1\" in\nshow) echo 'decryption failed' >&2; exit 1 ;;\n*) echo invoked >> \"$DCH_REPAIR_ENTRY\" ;;\nesac\n"
	}
	if err := os.WriteFile(passPath, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DCH_REPAIR_ENTRY", entry)
	return passPath, entry
}

func TestCommitCandidateRestoresSnapshotWhenCandidateReadbackFails(t *testing.T) {
	cfg := testConfig(t.TempDir())
	oldSecret, passPath, entryPath := failingCandidatePass(t, cfg, false, false)
	plugin := &Plugin{PassPath: passPath}
	raw, _ := json.Marshal(cfg)
	if _, err := plugin.commitCandidate(context.Background(), plugins.HostContext{ProjectPath: cfg.ProjectPath, Config: raw, PreviousConfig: raw, Store: &testScopedStore{}}, cfg, candidateTokenSet(cfg), raw); err == nil {
		t.Fatal("candidate save unexpectedly succeeded")
	}
	got, err := os.ReadFile(entryPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(oldSecret) {
		t.Fatalf("old credential not restored: %s", got)
	}
}

func TestCommitCandidateDeletesPartialEntryWhenMissingEntryReadbackFails(t *testing.T) {
	cfg := testConfig(t.TempDir())
	_, passPath, entryPath := failingCandidatePass(t, cfg, true, false)
	plugin := &Plugin{PassPath: passPath}
	raw, _ := json.Marshal(cfg)
	if _, err := plugin.commitCandidate(context.Background(), plugins.HostContext{ProjectPath: cfg.ProjectPath, Config: raw, PreviousConfig: raw, Store: &testScopedStore{}}, cfg, candidateTokenSet(cfg), raw); err == nil {
		t.Fatal("candidate save unexpectedly succeeded")
	}
	if _, err := os.Stat(entryPath); !os.IsNotExist(err) {
		t.Fatalf("partial candidate credential remains: %v", err)
	}
}

func TestCommitCandidateReportsSnapshotRestoreFailure(t *testing.T) {
	cfg := testConfig(t.TempDir())
	_, passPath, _ := failingCandidatePass(t, cfg, false, true)
	plugin := &Plugin{PassPath: passPath}
	raw, _ := json.Marshal(cfg)
	_, err := plugin.commitCandidate(context.Background(), plugins.HostContext{ProjectPath: cfg.ProjectPath, Config: raw, PreviousConfig: raw, Store: &testScopedStore{}}, cfg, candidateTokenSet(cfg), raw)
	if err == nil || !strings.Contains(err.Error(), "previous credential restore failed") {
		t.Fatalf("commitCandidate() error = %v", err)
	}
}

func candidateTokenSet(cfg Config) TokenSet {
	now := time.Now()
	return TokenSet{Schema: 1, ClientID: cfg.ClientID, ProjectPath: cfg.ProjectPath, UserID: cfg.UserID, RepositoryID: cfg.RepositoryID, Repository: cfg.Repository, AccessToken: "candidate", RefreshToken: "candidate-refresh", AccessTokenExpiresAt: now.Add(time.Hour), RefreshTokenExpiresAt: now.Add(24 * time.Hour)}
}

func failingCandidatePass(t *testing.T, cfg Config, missing, failRestore bool) ([]byte, string, string) {
	t.Helper()
	passPath, entryPath := filepath.Join(t.TempDir(), "pass"), filepath.Join(t.TempDir(), "secret.json")
	countPath := filepath.Join(t.TempDir(), "calls")
	var original []byte
	if !missing {
		original, _ = json.Marshal(testTokenSet(cfg, time.Now().Add(time.Hour), time.Now().Add(24*time.Hour)))
		if err := os.WriteFile(entryPath, original, 0600); err != nil {
			t.Fatal(err)
		}
	}
	script := "#!/bin/sh\nn=0; [ -f \"$DCH_FAIL_COUNT\" ] && n=$(cat \"$DCH_FAIL_COUNT\"); n=$((n+1)); echo $n > \"$DCH_FAIL_COUNT\"\ncase \"$1\" in\nshow) if [ \"$n\" = 3 ]; then printf '{}'; exit 0; fi; if [ -f \"$DCH_FAIL_ENTRY\" ]; then cat \"$DCH_FAIL_ENTRY\"; else echo 'not in the password store' >&2; exit 1; fi ;;\ninsert) if [ \"$n\" = 4 ] && [ \"$DCH_FAIL_RESTORE\" = 1 ]; then exit 1; fi; cat > \"$DCH_FAIL_ENTRY\" ;;\nrm) rm -f \"$DCH_FAIL_ENTRY\" ;;\nesac\n"
	if err := os.WriteFile(passPath, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DCH_FAIL_ENTRY", entryPath)
	t.Setenv("DCH_FAIL_COUNT", countPath)
	if failRestore {
		t.Setenv("DCH_FAIL_RESTORE", "1")
	} else {
		t.Setenv("DCH_FAIL_RESTORE", "0")
	}
	return original, passPath, entryPath
}
