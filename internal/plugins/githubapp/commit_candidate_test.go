package githubapp

import (
	"context"
	"encoding/json"
	"fmt"
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
	store := &testScopedStore{}
	baseline := ownershipState{FragmentDigest: "fragment", UnitDigest: "unit", UnitPath: "/unit", CredentialCleanup: []string{"other-client"}}
	_, err := plugin.commitCandidate(context.Background(), plugins.HostContext{ProjectPath: cfg.ProjectPath, Config: raw, PreviousConfig: raw, State: encodeState(baseline), Store: store}, cfg, candidateTokenSet(cfg), raw)
	if err == nil || !strings.Contains(err.Error(), "previous credential restore failed") {
		t.Fatalf("commitCandidate() error = %v", err)
	}
	assertRecoveryClient(t, store.state, cfg.ClientID)
	recovered, _ := decodeState(store.state)
	if recovered.FragmentDigest != baseline.FragmentDigest || recovered.UnitDigest != baseline.UnitDigest || recovered.UnitPath != baseline.UnitPath || !containsClientID(recovered.CredentialCleanup, "other-client") {
		t.Fatalf("existing ownership state was not preserved: %#v", recovered)
	}
}

func TestCommitCandidateReportsWhenRecoveryMetadataCannotBeSaved(t *testing.T) {
	cfg := testConfig(t.TempDir())
	_, passPath, _ := failingCandidatePass(t, cfg, false, true)
	plugin := &Plugin{PassPath: passPath}
	raw, _ := json.Marshal(cfg)
	_, err := plugin.commitCandidate(context.Background(), plugins.HostContext{ProjectPath: cfg.ProjectPath, Config: raw, PreviousConfig: raw, Store: failingStateStore{}}, cfg, candidateTokenSet(cfg), raw)
	if err == nil || !strings.Contains(err.Error(), "recovery metadata could not be saved") {
		t.Fatalf("commitCandidate() error = %v", err)
	}
}

type failingStateStore struct{}

func (failingStateStore) SaveConfig(json.RawMessage) error { return nil }
func (failingStateStore) SaveState(json.RawMessage) error {
	return fmt.Errorf("state store unavailable")
}

func TestStatusSurfacesRecoveryMetadataWhenProjectIsDisabled(t *testing.T) {
	state := encodeState(ownershipState{CredentialCleanup: []string{"client-id"}})
	status, err := New().Status(context.Background(), plugins.HostContext{ProjectPath: t.TempDir(), State: state})
	if err != nil || status.Enabled || !status.CleanupPending || !strings.Contains(status.Message, "cleanup is pending") {
		t.Fatalf("Status() = %#v, %v", status, err)
	}
}

func TestEnsureRejectsRecoveryMarkerForActiveCredentialBeforeServiceChanges(t *testing.T) {
	env := setupHostHookTest(t, false)
	project := t.TempDir()
	cfg := testConfig(project)
	fragment, _, err := renderFragment(project)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(managedFragment(project)), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(managedFragment(project), fragment, 0600); err != nil {
		t.Fatal(err)
	}
	state := ownershipState{FragmentDigest: digest(fragment), CredentialCleanup: []string{cfg.ClientID}}
	raw, _ := json.Marshal(cfg)
	plugin := &Plugin{PassPath: filepath.Join(env.root, "bin", "pass")}
	if err := os.WriteFile(plugin.PassPath, []byte("#!/bin/sh\necho invoked >> \"$DCH_PASS_CALLS\"\nexit 1\n"), 0700); err != nil {
		t.Fatal(err)
	}
	passCalls := filepath.Join(env.root, "pass.log")
	t.Setenv("DCH_PASS_CALLS", passCalls)
	host := plugins.HostContext{ProjectPath: project, Config: raw, State: encodeState(state), Store: &testScopedStore{}}
	_, err = plugin.ensure(context.Background(), host)
	if err == nil || !strings.Contains(err.Error(), "credential recovery is pending") {
		t.Fatalf("ensure() error = %v", err)
	}
	assertNoSystemctlCalls(t, env)
	if _, err := os.Stat(passCalls); !os.IsNotExist(err) {
		t.Fatalf("active credential was accessed for cleanup: %v", err)
	}
}

func TestStatusRequiresReauthorizationForActiveRecoveryMarker(t *testing.T) {
	setupHostHookTest(t, true)
	project := t.TempDir()
	cfg := testConfig(project)
	raw, _ := json.Marshal(cfg)
	status, err := New().Status(context.Background(), plugins.HostContext{ProjectPath: project, Config: raw, State: encodeState(ownershipState{CredentialCleanup: []string{cfg.ClientID}})})
	if err != nil || !strings.Contains(status.Message, "Authorization required") {
		t.Fatalf("Status() = %#v, %v", status, err)
	}
}

func TestServeHostRejectsActiveRecoveryMarker(t *testing.T) {
	project := t.TempDir()
	cfg := testConfig(project)
	raw, _ := json.Marshal(cfg)
	err := New().ServeHost(context.Background(), plugins.HostContext{ProjectPath: project, Config: raw, State: encodeState(ownershipState{CredentialCleanup: []string{cfg.ClientID}})})
	if err == nil || !strings.Contains(err.Error(), "credential recovery is pending") {
		t.Fatalf("ServeHost() error = %v", err)
	}
}

func TestServeHostRejectsActivePATRecoveryMarker(t *testing.T) {
	project := t.TempDir()
	cfg := Config{AuthMethod: authMethodPAT, ProjectPath: project, UserID: 8, RepositoryID: 9, Repository: "octo/repo"}
	raw, _ := json.Marshal(cfg)
	state := encodeState(ownershipState{CredentialCleanupIdentities: []credentialIdentity{identityForConfig(cfg)}})
	err := New().ServeHost(context.Background(), plugins.HostContext{ProjectPath: project, Config: raw, State: state})
	if err == nil || !strings.Contains(err.Error(), "credential recovery is pending") {
		t.Fatalf("ServeHost() error = %v", err)
	}
}

func TestDisableRetriesRecoveryOnlyCredentialCleanup(t *testing.T) {
	project := t.TempDir()
	clientID := "client-id"
	passPath, entry := retryDeletePass(t)
	if err := os.WriteFile(entry, []byte("candidate credential"), 0600); err != nil {
		t.Fatal(err)
	}
	store := &testScopedStore{state: encodeState(ownershipState{CredentialCleanup: []string{clientID}})}
	plugin := &Plugin{PassPath: passPath}
	host := plugins.HostContext{ProjectPath: project, State: store.state, Store: store}
	if _, err := plugin.disable(context.Background(), host); err == nil {
		t.Fatal("first credential cleanup unexpectedly succeeded")
	}
	status, err := plugin.Status(context.Background(), plugins.HostContext{ProjectPath: project, State: store.state})
	if err != nil || !strings.Contains(status.Message, "cleanup is pending") {
		t.Fatalf("pending cleanup status = %#v, %v", status, err)
	}
	host.State = store.state
	if _, err := plugin.disable(context.Background(), host); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(entry); !os.IsNotExist(err) {
		t.Fatalf("recovery credential remains: %v", err)
	}
	if len(store.state) != 0 {
		t.Fatalf("recovery metadata remains: %s", store.state)
	}
}

func TestDisableRetriesPATRecoveryCleanup(t *testing.T) {
	project := t.TempDir()
	patConfig := Config{AuthMethod: authMethodPAT, ProjectPath: project, UserID: 8, RepositoryID: 9, Repository: "octo/repo"}
	passPath, root := credentialMapPass(t)
	entry := filepath.Join(root, secretEntry(patConfig))
	if err := os.MkdirAll(filepath.Dir(entry), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(entry, []byte("PAT credential"), 0600); err != nil {
		t.Fatal(err)
	}
	state := encodeState(ownershipState{CredentialCleanupIdentities: []credentialIdentity{identityForConfig(patConfig)}})
	store := &testScopedStore{state: state}
	plugin := &Plugin{PassPath: passPath}
	host := plugins.HostContext{ProjectPath: project, State: state, Store: store}
	if _, err := plugin.HandleHost(context.Background(), host, plugins.EventDisable); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(entry); !os.IsNotExist(err) {
		t.Fatalf("PAT credential remains after disable: %v", err)
	}
}

func retryDeletePass(t *testing.T) (string, string) {
	t.Helper()
	passPath, entry := filepath.Join(t.TempDir(), "pass"), filepath.Join(t.TempDir(), "secret.json")
	count := filepath.Join(t.TempDir(), "rm-count")
	script := "#!/bin/sh\ncase \"$1\" in\nshow) [ -f \"$DCH_RETRY_ENTRY\" ] && cat \"$DCH_RETRY_ENTRY\" || { echo 'not in the password store' >&2; exit 1; } ;;\nrm) n=0; [ -f \"$DCH_RETRY_COUNT\" ] && n=$(cat \"$DCH_RETRY_COUNT\"); n=$((n+1)); echo $n > \"$DCH_RETRY_COUNT\"; [ \"$n\" = 1 ] && exit 1; rm -f \"$DCH_RETRY_ENTRY\" ;;\nesac\n"
	if err := os.WriteFile(passPath, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DCH_RETRY_ENTRY", entry)
	t.Setenv("DCH_RETRY_COUNT", count)
	return passPath, entry
}

func TestCommitCandidateRecordsRecoveryWhenEnableRollbackFails(t *testing.T) {
	env := setupHostHookTest(t, false)
	project := t.TempDir()
	cfg := testConfig(project)
	raw, _ := json.Marshal(cfg)
	store := &testScopedStore{}
	passPath, entry := failingEnableRollbackPass(t, cfg)
	plugin := &Plugin{PassPath: passPath}
	if err := os.WriteFile(filepath.Join(env.root, "bin", "systemctl"), []byte("#!/bin/sh\nshift\necho \"$1\" >> \"$DCH_TEST_SYSTEMCTL_LOG\"\ncase \"$1\" in\nis-active) exit 3 ;;\nenable) exit 1 ;;\n*) exit 0 ;;\nesac\n"), 0700); err != nil {
		t.Fatal(err)
	}
	host := plugins.HostContext{ProjectPath: project, Config: raw, PreviousConfig: raw, Store: store}
	_, err := plugin.commitCandidate(context.Background(), host, cfg, candidateTokenSet(cfg), raw)
	if err == nil || !strings.Contains(err.Error(), "previous credential restore failed") {
		t.Fatalf("commitCandidate() error = %v", err)
	}
	assertRecoveryClient(t, store.state, cfg.ClientID)
	if _, err := os.Stat(entry); err != nil {
		t.Fatalf("unrestored credential entry not retained for recovery: %v", err)
	}
}

func TestCommitCandidateRecoveryPreservesLatestUnrolledBackOwnershipState(t *testing.T) {
	env := setupHostHookTest(t, false)
	project := t.TempDir()
	cfg := testConfig(project)
	raw, _ := json.Marshal(cfg)
	passPath, _ := failingEnableRollbackPass(t, cfg)
	plugin := &Plugin{PassPath: passPath}
	if err := os.WriteFile(filepath.Join(env.root, "bin", "systemctl"), []byte("#!/bin/sh\nshift\necho \"$1\" >> \"$DCH_TEST_SYSTEMCTL_LOG\"\ncase \"$1\" in\nis-active) exit 3 ;;\nenable|stop) exit 1 ;;\n*) exit 0 ;;\nesac\n"), 0700); err != nil {
		t.Fatal(err)
	}
	store := &stateHistoryStore{}
	host := plugins.HostContext{ProjectPath: project, Config: raw, PreviousConfig: raw, Store: store}
	_, err := plugin.commitCandidate(context.Background(), host, cfg, candidateTokenSet(cfg), raw)
	if err == nil || !strings.Contains(err.Error(), "previous credential restore failed") {
		t.Fatalf("commitCandidate() error = %v", err)
	}
	if len(store.states) < 2 {
		t.Fatalf("expected enable and recovery state writes, got %d", len(store.states))
	}
	latest, err := decodeState(store.states[len(store.states)-1])
	if err != nil || latest.UnitDigest == "" || latest.UnitPath == "" || latest.FragmentDigest == "" || !containsClientID(latest.CredentialCleanup, cfg.ClientID) {
		t.Fatalf("latest ownership/recovery state = %#v, %v", latest, err)
	}
}

func TestSuccessfulReauthorizationKeepsActiveRecoveryEntryAndCleansOthers(t *testing.T) {
	project := t.TempDir()
	activeID, oldID := "active-client", "old-client"
	activeCfg := testConfig(project)
	activeCfg.ClientID = activeID
	oldCfg := activeCfg
	oldCfg.ClientID = oldID
	passPath, passRoot := credentialMapPass(t)
	writeMappedCredential(t, passRoot, activeCfg, candidateTokenSet(activeCfg))
	writeMappedCredential(t, passRoot, oldCfg, candidateTokenSet(oldCfg))
	state := ownershipState{CredentialCleanup: []string{activeID, oldID}}
	store := &testScopedStore{}
	result := plugins.Result{State: encodeState(state)}
	plugin := &Plugin{PassPath: passPath}
	result, err := plugin.cleanupPreviousClient(context.Background(), plugins.HostContext{ProjectPath: project, Store: store}, result, oldID, activeID)
	if err != nil {
		t.Fatal(err)
	}
	activeEntry := filepath.Join(passRoot, secretEntry(activeCfg))
	if _, err := os.Stat(activeEntry); err != nil {
		t.Fatalf("active credential was removed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(passRoot, secretEntry(oldCfg))); !os.IsNotExist(err) {
		t.Fatalf("old recovery credential remains: %v", err)
	}
	cleaned, err := decodeState(result.State)
	if err != nil || len(cleaned.CredentialCleanup) != 0 {
		t.Fatalf("cleanup state = %#v, error = %v", cleaned, err)
	}
}

func TestPATCleanupFailureRemainsPendingAndCanBeRetried(t *testing.T) {
	project := t.TempDir()
	patCfg := Config{AuthMethod: authMethodPAT, ProjectPath: project, UserID: 8, RepositoryID: 9, Repository: "octo/repo"}
	passPath, passRoot := credentialMapPass(t)
	writeMappedCredential(t, passRoot, patCfg, TokenSet{Schema: 2, AuthMethod: authMethodPAT, ProjectPath: project, UserID: patCfg.UserID, RepositoryID: patCfg.RepositoryID, Repository: patCfg.Repository, PersonalAccessToken: "github_pat_old"})
	entry := filepath.Join(passRoot, secretEntry(patCfg))
	failureMarker := filepath.Join(t.TempDir(), "fail-rm")
	if err := os.WriteFile(failureMarker, nil, 0600); err != nil {
		t.Fatal(err)
	}
	script := readTestFile(t, passPath)
	script = strings.Replace(script, "rm) rm -f", "rm) [ ! -f \"$DCH_FAIL_RM\" ] || exit 1; rm -f", 1)
	if err := os.WriteFile(passPath, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DCH_FAIL_RM", failureMarker)
	plugin := &Plugin{PassPath: passPath}
	host := plugins.HostContext{ProjectPath: project}
	state := ownershipState{CredentialCleanupIdentities: []credentialIdentity{identityForConfig(patCfg)}}
	remaining, err := plugin.retryCredentialCleanupFor(context.Background(), host, state, credentialIdentity{Method: authMethodApp, Key: "Iv1_active"})
	if err == nil || !containsRecoveryIdentity(remaining, identityForConfig(patCfg)) {
		t.Fatalf("failed PAT cleanup state = %#v, error = %v", remaining, err)
	}
	if _, err := os.Stat(entry); err != nil {
		t.Fatalf("failed cleanup removed PAT entry: %v", err)
	}
	if err := os.Remove(failureMarker); err != nil {
		t.Fatal(err)
	}
	remaining, err = plugin.retryCredentialCleanupFor(context.Background(), host, remaining, credentialIdentity{Method: authMethodApp, Key: "Iv1_active"})
	if err != nil || hasCredentialCleanup(remaining) {
		t.Fatalf("retried PAT cleanup state = %#v, error = %v", remaining, err)
	}
	if _, err := os.Stat(entry); !os.IsNotExist(err) {
		t.Fatalf("retried cleanup left PAT entry: %v", err)
	}
}

func TestSwitchingFromAppToPATKeepsPATAndCleansOldAppEntry(t *testing.T) {
	setupHostHookTest(t, true)
	project := t.TempDir()
	appCfg := testConfig(project)
	patCfg := Config{AuthMethod: authMethodPAT, ProjectPath: project, UserID: appCfg.UserID, Login: appCfg.Login, RepositoryID: appCfg.RepositoryID, Repository: appCfg.Repository}
	appRaw, _ := json.Marshal(appCfg)
	patRaw, _ := json.Marshal(patCfg)
	passPath, passRoot := credentialMapPass(t)
	writeMappedCredential(t, passRoot, appCfg, candidateTokenSet(appCfg))
	patSet := TokenSet{Schema: 2, AuthMethod: authMethodPAT, ProjectPath: project, UserID: patCfg.UserID, RepositoryID: patCfg.RepositoryID, Repository: patCfg.Repository, PersonalAccessToken: "github_pat_candidate"}
	listener := readyCredentialSocket(t, project)
	defer listener.Close()
	store := &testScopedStore{}
	plugin := &Plugin{PassPath: passPath}
	if _, err := plugin.commitCandidate(context.Background(), plugins.HostContext{ProjectPath: project, Config: patRaw, PreviousConfig: appRaw, Store: store}, patCfg, patSet, patRaw); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(passRoot, secretEntry(patCfg))); err != nil {
		t.Fatalf("PAT entry was removed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(passRoot, secretEntry(appCfg))); !os.IsNotExist(err) {
		t.Fatalf("old App entry remains: %v", err)
	}
	state, err := decodeState(store.state)
	if err != nil || hasCredentialCleanup(state) {
		t.Fatalf("cleanup state = %#v, %v", state, err)
	}
}

func TestSwitchingFromPATToAppKeepsAppAndCleansOldPATEntry(t *testing.T) {
	setupHostHookTest(t, true)
	project := t.TempDir()
	patCfg := Config{AuthMethod: authMethodPAT, ProjectPath: project, UserID: 8, Login: "tester", RepositoryID: 9, Repository: "octo/repo"}
	appCfg := testConfig(project)
	patRaw, _ := json.Marshal(patCfg)
	appRaw, _ := json.Marshal(appCfg)
	passPath, passRoot := credentialMapPass(t)
	patSet := TokenSet{Schema: 2, AuthMethod: authMethodPAT, ProjectPath: project, UserID: patCfg.UserID, RepositoryID: patCfg.RepositoryID, Repository: patCfg.Repository, PersonalAccessToken: "github_pat_previous"}
	writeMappedCredential(t, passRoot, patCfg, patSet)
	listener := readyCredentialSocket(t, project)
	defer listener.Close()
	store := &testScopedStore{}
	plugin := &Plugin{PassPath: passPath}
	if _, err := plugin.commitCandidate(context.Background(), plugins.HostContext{ProjectPath: project, Config: appRaw, PreviousConfig: patRaw, Store: store}, appCfg, candidateTokenSet(appCfg), appRaw); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(passRoot, secretEntry(appCfg))); err != nil {
		t.Fatalf("App entry was removed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(passRoot, secretEntry(patCfg))); !os.IsNotExist(err) {
		t.Fatalf("old PAT entry remains: %v", err)
	}
	state, err := decodeState(store.state)
	if err != nil || hasCredentialCleanup(state) {
		t.Fatalf("cleanup state = %#v, %v", state, err)
	}
}

func TestPATReauthorizationRestoresPreviousTokenWhenEnableFails(t *testing.T) {
	env := setupHostHookTest(t, false)
	project := t.TempDir()
	cfg := Config{AuthMethod: authMethodPAT, ProjectPath: project, UserID: 8, Login: "tester", RepositoryID: 9, Repository: "octo/repo"}
	raw, _ := json.Marshal(cfg)
	previous := TokenSet{Schema: 2, AuthMethod: authMethodPAT, ProjectPath: project, UserID: cfg.UserID, RepositoryID: cfg.RepositoryID, Repository: cfg.Repository, PersonalAccessToken: "github_pat_previous"}
	candidate := previous
	candidate.PersonalAccessToken = "github_pat_candidate"
	passPath, root := credentialMapPass(t)
	writeMappedCredential(t, root, cfg, previous)
	if err := os.WriteFile(filepath.Join(env.root, "bin", "systemctl"), []byte("#!/bin/sh\nshift\ncase \"$1\" in\nis-active) exit 3 ;;\nenable) exit 1 ;;\n*) exit 0 ;;\nesac\n"), 0700); err != nil {
		t.Fatal(err)
	}
	plugin := &Plugin{PassPath: passPath}
	_, err := plugin.commitCandidate(context.Background(), plugins.HostContext{ProjectPath: project, Config: raw, PreviousConfig: raw, Store: &testScopedStore{}}, cfg, candidate, raw)
	if err == nil || !strings.Contains(err.Error(), "enable") {
		t.Fatalf("commitCandidate() error = %v", err)
	}
	store := &SecretStore{PassPath: passPath, Entry: secretEntry(cfg), LockDir: filepath.Join(t.TempDir(), "locks")}
	got, err := store.Read(context.Background(), cfg)
	if err != nil || got.PersonalAccessToken != previous.PersonalAccessToken {
		t.Fatalf("restored PAT = %#v, %v", got, err)
	}
}

func TestCommitCandidateClearsActiveRecoveryBeforeStartingService(t *testing.T) {
	setupHostHookTest(t, true)
	cfg := testConfig(t.TempDir())
	raw, _ := json.Marshal(cfg)
	passPath, _ := fakeRepairPass(t, "missing")
	listener := readyCredentialSocket(t, cfg.ProjectPath)
	defer listener.Close()
	store := &stateHistoryStore{}
	oldState := encodeState(ownershipState{CredentialCleanup: []string{cfg.ClientID, "old-client"}})
	host := plugins.HostContext{ProjectPath: cfg.ProjectPath, Config: raw, PreviousConfig: raw, State: oldState, PreviousState: oldState, Store: store}
	plugin := &Plugin{PassPath: passPath}
	if _, err := plugin.commitCandidate(context.Background(), host, cfg, candidateTokenSet(cfg), raw); err != nil {
		t.Fatal(err)
	}
	if len(store.states) == 0 {
		t.Fatal("service enable did not persist ownership state")
	}
	started, err := decodeState(store.states[0])
	if err != nil || containsClientID(started.CredentialCleanup, cfg.ClientID) {
		t.Fatalf("active recovery marker persisted before service startup: %#v, %v", started, err)
	}
	if !containsClientID(started.CredentialCleanup, "old-client") {
		t.Fatalf("other cleanup marker was lost: %#v", started.CredentialCleanup)
	}
}

type stateHistoryStore struct{ states []json.RawMessage }

func (*stateHistoryStore) SaveConfig(json.RawMessage) error { return nil }
func (store *stateHistoryStore) SaveState(raw json.RawMessage) error {
	store.states = append(store.states, append(json.RawMessage(nil), raw...))
	return nil
}

func TestRetryCredentialCleanupNeverDeletesProtectedActiveClient(t *testing.T) {
	project := t.TempDir()
	activeID, oldID := "active-client", "old-client"
	activeCfg := testConfig(project)
	activeCfg.ClientID = activeID
	oldCfg := activeCfg
	oldCfg.ClientID = oldID
	passPath, passRoot := credentialMapPass(t)
	writeMappedCredential(t, passRoot, activeCfg, candidateTokenSet(activeCfg))
	writeMappedCredential(t, passRoot, oldCfg, candidateTokenSet(oldCfg))
	plugin := &Plugin{PassPath: passPath}
	state, err := plugin.retryCredentialCleanup(context.Background(), plugins.HostContext{ProjectPath: project}, ownershipState{CredentialCleanup: []string{activeID, oldID}}, activeID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(passRoot, secretEntry(activeCfg))); err != nil {
		t.Fatalf("active credential was removed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(passRoot, secretEntry(oldCfg))); !os.IsNotExist(err) {
		t.Fatalf("old recovery credential remains: %v", err)
	}
	if len(state.CredentialCleanup) != 1 || state.CredentialCleanup[0] != activeID {
		t.Fatalf("protected recovery marker = %#v", state.CredentialCleanup)
	}
}

func credentialMapPass(t *testing.T) (string, string) {
	t.Helper()
	passPath, root := filepath.Join(t.TempDir(), "pass"), t.TempDir()
	script := "#!/bin/sh\nfor arg in \"$@\"; do entry=\"$arg\"; done\nentry=\"$DCH_MAPPED_PASS_ROOT/$entry\"\ncase \"$1\" in\nshow) [ -f \"$entry\" ] && cat \"$entry\" || { echo 'not in the password store' >&2; exit 1; } ;;\ninsert) mkdir -p \"$(dirname \"$entry\")\"; cat > \"$entry\" ;;\nrm) rm -f \"$entry\" ;;\nesac\n"
	if err := os.WriteFile(passPath, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DCH_MAPPED_PASS_ROOT", root)
	return passPath, root
}

func writeMappedCredential(t *testing.T, root string, cfg Config, tokens TokenSet) {
	t.Helper()
	data, err := json.Marshal(tokens)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, secretEntry(cfg))
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
}

func failingEnableRollbackPass(t *testing.T, cfg Config) (string, string) {
	t.Helper()
	passPath, entry := filepath.Join(t.TempDir(), "pass"), filepath.Join(t.TempDir(), "secret.json")
	old, _ := json.Marshal(testTokenSet(cfg, time.Now().Add(time.Hour), time.Now().Add(24*time.Hour)))
	if err := os.WriteFile(entry, old, 0600); err != nil {
		t.Fatal(err)
	}
	count := filepath.Join(t.TempDir(), "insert-count")
	script := "#!/bin/sh\ncase \"$1\" in\nshow) cat \"$DCH_ENABLE_ENTRY\" ;;\ninsert) n=0; [ -f \"$DCH_ENABLE_INSERT_COUNT\" ] && n=$(cat \"$DCH_ENABLE_INSERT_COUNT\"); n=$((n+1)); echo $n > \"$DCH_ENABLE_INSERT_COUNT\"; [ \"$n\" = 2 ] && exit 1; cat > \"$DCH_ENABLE_ENTRY\" ;;\n*) exit 0 ;;\nesac\n"
	if err := os.WriteFile(passPath, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DCH_ENABLE_ENTRY", entry)
	t.Setenv("DCH_ENABLE_INSERT_COUNT", count)
	return passPath, entry
}

func assertRecoveryClient(t *testing.T, raw json.RawMessage, clientID string) {
	t.Helper()
	state, err := decodeState(raw)
	if err != nil || !containsClientID(state.CredentialCleanup, clientID) {
		t.Fatalf("recovery metadata = %s, error = %v", raw, err)
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
