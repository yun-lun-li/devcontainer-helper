package githubapp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"dch/internal/plugins"
)

func TestConfigureUsesRemoteIDAndOneRepositoryScopedDeviceFlow(t *testing.T) {
	project := gitProject(t, true)
	fixture := newOAuthFixture(t, []int64{9001})
	ui := &fakeInteraction{selected: "77"}
	plugin := &Plugin{HTTPClient: fixture.client, APIBaseURL: "https://api.github.com"}
	prepared, err := plugin.Configure(context.Background(), configureContext(project), ui)
	if err != nil {
		t.Fatal(err)
	}
	assertPreparedUserBinding(t, prepared.Config, project, 77)
	var cfg Config
	if err := json.Unmarshal(prepared.Config, &cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.AuthMethod != authMethodApp {
		t.Fatalf("prepared authentication method = %q, want explicit App method", cfg.AuthMethod)
	}
	if len(fixture.polls) != 1 || fixture.polls[0].Get("repository_id") != "77" {
		t.Fatalf("device polls = %#v", fixture.polls)
	}
	if fixture.polls[0].Get("client_secret") != "" || fixture.polls[0].Get("scope") != "" {
		t.Fatalf("poll included forbidden fields: %#v", fixture.polls[0])
	}
	if len(fixture.deviceRequests) != 1 || fixture.deviceRequests[0].Get("scope") != "offline_access" {
		t.Fatalf("device requests = %#v", fixture.deviceRequests)
	}
	if fixture.deviceCodes != 1 || ui.opened != 1 || ui.confirms != 2 {
		t.Fatalf("device codes=%d browser=%d confirms=%d", fixture.deviceCodes, ui.opened, ui.confirms)
	}
	if len(ui.hostCommands) != 0 || len(ui.notices) == 0 || ui.notices[0] != "Using existing pass recipient" {
		t.Fatalf("setup commands=%#v notices=%#v", ui.hostCommands, ui.notices)
	}
}

func TestConfigureOffersAppAsRecommendedAuthenticationMethod(t *testing.T) {
	project := gitProject(t, true)
	fixture := newOAuthFixture(t, []int64{9001})
	ui := &fakeInteraction{selected: "77"}
	plugin := &Plugin{HTTPClient: fixture.client, APIBaseURL: "https://api.github.com"}
	if _, err := plugin.Configure(context.Background(), configureContext(project), ui); err != nil {
		t.Fatal(err)
	}
	if len(ui.selections) == 0 {
		t.Fatal("Configure() did not ask for an authentication method")
	}
	selection := ui.selections[0]
	if selection.title != "Choose GitHub authentication" || selection.defaultIndex != 0 {
		t.Fatalf("first selection = %#v, want authentication method with App selected", selection)
	}
	if len(selection.choices) != 2 || selection.choices[0] != (plugins.Choice{Value: "app", Label: "BYO GitHub App (Recommended)"}) || selection.choices[1] != (plugins.Choice{Value: "pat", Label: "Fine-grained personal access token"}) {
		t.Fatalf("authentication choices = %#v", selection.choices)
	}
}

func TestConfigureUsesEnteredAppClientIDForOnlyThisProject(t *testing.T) {
	project := gitProject(t, true)
	fixture := newOAuthFixture(t, []int64{9001})
	ui := &fakeInteraction{selected: "77", inputValue: "Iv1_project-specific"}
	plugin := &Plugin{HTTPClient: fixture.client, APIBaseURL: "https://api.github.com"}
	prepared, err := plugin.Configure(context.Background(), configureContext(project), ui)
	if err != nil {
		t.Fatal(err)
	}
	var cfg Config
	if err := json.Unmarshal(prepared.Config, &cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.ClientID != "Iv1_project-specific" {
		t.Fatalf("prepared ClientID = %q, want project-specific value", cfg.ClientID)
	}
	if fixture.deviceRequests[0].Get("client_id") != "Iv1_project-specific" {
		t.Fatalf("device flow used ClientID %q", fixture.deviceRequests[0].Get("client_id"))
	}
	if len(ui.inputs) != 1 || ui.inputs[0].secret {
		t.Fatalf("App Client ID inputs = %#v, want one non-secret input", ui.inputs)
	}
}

func TestConfigurePATValidatesAccountAndTargetRepositoryAfterHostSetup(t *testing.T) {
	project := gitProject(t, true)
	var requests []string
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.Method+" "+r.URL.Path)
		if r.Header.Get("Authorization") != "Bearer github_pat_test" {
			t.Errorf("authorization = %q", r.Header.Get("Authorization"))
		}
		switch r.URL.Path {
		case "/user":
			fmt.Fprint(w, `{"id":9001,"login":"octo-user"}`)
		case "/repos/octo/repo":
			fmt.Fprint(w, `{"id":77,"full_name":"octo/repo"}`)
		default:
			t.Errorf("unexpected API request %q", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer api.Close()
	ui := &fakeInteraction{selected: "ignored", authMethod: "pat", inputValue: "github_pat_test"}
	plugin := &Plugin{HTTPClient: api.Client(), APIBaseURL: api.URL}
	prepared, err := plugin.Configure(context.Background(), plugins.ConfigureContext{Host: plugins.HostContext{ProjectPath: project}}, ui)
	if err != nil {
		t.Fatal(err)
	}
	var cfg Config
	if err := json.Unmarshal(prepared.Config, &cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.AuthMethod != authMethodPAT || cfg.ClientID != "" || cfg.UserID != 9001 || cfg.RepositoryID != 77 || cfg.Repository != "octo/repo" {
		t.Fatalf("prepared PAT config = %#v", cfg)
	}
	if strings.Contains(string(prepared.Config), "github_pat_test") {
		t.Fatal("PAT was included in project config")
	}
	if len(requests) != 2 || requests[0] != "GET /user" || requests[1] != "GET /repos/octo/repo" {
		t.Fatalf("PAT validation requests = %#v", requests)
	}
	if len(ui.inputs) != 1 || !ui.inputs[0].secret {
		t.Fatalf("PAT inputs = %#v, want one hidden input", ui.inputs)
	}
	if len(ui.noticeMessages) == 0 || !strings.Contains(ui.noticeMessages[len(ui.noticeMessages)-1], "https://github.com/settings/personal-access-tokens/new") || !strings.Contains(ui.noticeMessages[len(ui.noticeMessages)-1], "minimum permissions") || !strings.Contains(ui.noticeMessages[len(ui.noticeMessages)-1], "does not grant Git or gh push access") {
		t.Fatalf("PAT creation guidance = %#v", ui.noticeMessages)
	}
}

func TestConfigureRejectsMalformedPATRepositoryBeforeSecretInput(t *testing.T) {
	project := gitProject(t, false)
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Errorf("GitHub was called for a malformed repository")
		http.Error(w, "unexpected", http.StatusInternalServerError)
	}))
	defer api.Close()
	ui := &fakeInteraction{authMethod: authMethodPAT, inputValues: map[string]string{"GitHub repository (owner/repo)": "../repo"}}
	plugin := &Plugin{HTTPClient: api.Client(), APIBaseURL: api.URL}
	_, err := plugin.Configure(context.Background(), plugins.ConfigureContext{Host: plugins.HostContext{ProjectPath: project}}, ui)
	if err == nil || !strings.Contains(err.Error(), "owner/repo") {
		t.Fatalf("Configure() error = %v, want repository format guidance", err)
	}
	for _, input := range ui.inputs {
		if input.secret {
			t.Fatal("PAT was requested before repository input was validated")
		}
	}
}

func TestConfigurePATAllowsManualRepositoryWhenProjectHasNoGitRemote(t *testing.T) {
	setupConfigureHost(t)
	project := filepath.Clean(t.TempDir())
	var requests []string
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.Method+" "+r.URL.Path)
		switch r.URL.Path {
		case "/user":
			fmt.Fprint(w, `{"id":9001,"login":"octo-user"}`)
		case "/repos/octo/repo":
			fmt.Fprint(w, `{"id":77,"full_name":"octo/repo"}`)
		default:
			t.Errorf("unexpected API request %q", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer api.Close()
	ui := &fakeInteraction{authMethod: authMethodPAT, inputValues: map[string]string{
		"GitHub repository (owner/repo)":     "octo/repo",
		"Fine-grained personal access token": "github_pat_test",
	}}
	plugin := &Plugin{HTTPClient: api.Client(), APIBaseURL: api.URL}
	prepared, err := plugin.Configure(context.Background(), plugins.ConfigureContext{Host: plugins.HostContext{ProjectPath: project}}, ui)
	if err != nil {
		t.Fatal(err)
	}
	var cfg Config
	if err := json.Unmarshal(prepared.Config, &cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.Repository != "octo/repo" || cfg.AuthMethod != authMethodPAT {
		t.Fatalf("prepared PAT config = %#v", cfg)
	}
	if len(requests) != 2 || requests[0] != "GET /user" || requests[1] != "GET /repos/octo/repo" {
		t.Fatalf("PAT validation requests = %#v", requests)
	}
}

func TestConfigureRequiresHostSetupCapabilitiesBeforeDeviceFlow(t *testing.T) {
	project := gitProject(t, true)
	fixture := newOAuthFixture(t, []int64{9001})
	plugin := &Plugin{HTTPClient: fixture.client, APIBaseURL: "https://api.github.com"}
	_, err := plugin.Configure(context.Background(), configureContext(project), &legacyInteraction{})
	if err == nil || !strings.Contains(err.Error(), "host setup interaction") {
		t.Fatalf("Configure() error = %v, want missing host setup interaction", err)
	}
	if fixture.deviceCodes != 0 {
		t.Fatalf("device flow started %d times before host setup", fixture.deviceCodes)
	}
}

func TestConfigureStopsBeforeDeviceFlowWhenHostRequirementsAreUnavailable(t *testing.T) {
	project := gitProject(t, true)
	missingPath := t.TempDir()
	t.Setenv("PATH", missingPath)
	ui := &fakeInteraction{denyAt: 1}
	fixture := newOAuthFixture(t, []int64{9001})
	plugin := &Plugin{HTTPClient: fixture.client, APIBaseURL: "https://api.github.com"}
	_, err := plugin.Configure(context.Background(), configureContext(project), ui)
	if err == nil || (!strings.Contains(err.Error(), "not supported") && !strings.Contains(err.Error(), "manually")) {
		t.Fatalf("Configure() error = %v, want host setup guidance", err)
	}
	if len(ui.hostCommands) != 0 || len(fixture.deviceRequests) != 0 {
		t.Fatalf("host commands=%#v device requests=%#v", ui.hostCommands, fixture.deviceRequests)
	}
}

func TestConfigureInitializesUninitializedPassWithPrimaryFingerprint(t *testing.T) {
	project := gitProject(t, true)
	root := os.Getenv("PASSWORD_STORE_DIR")
	if err := os.Remove(filepath.Join(root, ".gpg-id")); err != nil {
		t.Fatal(err)
	}
	fixture := newOAuthFixture(t, []int64{9001})
	ui := &fakeInteraction{selected: "77"}
	plugin := &Plugin{HTTPClient: fixture.client, APIBaseURL: "https://api.github.com"}
	if _, err := plugin.Configure(context.Background(), configureContext(project), ui); err != nil {
		t.Fatal(err)
	}
	if len(ui.hostCommands) != 1 || ui.hostCommands[0] != "pass init 0123456789012345678901234567890123456789" {
		t.Fatalf("host commands = %#v", ui.hostCommands)
	}
}

func TestConfigureDoesNotOverwriteRecipientInitializedDuringKeySelection(t *testing.T) {
	project := gitProject(t, true)
	root := os.Getenv("PASSWORD_STORE_DIR")
	if err := os.Remove(filepath.Join(root, ".gpg-id")); err != nil {
		t.Fatal(err)
	}
	fixture := newOAuthFixture(t, []int64{9001})
	ui := &fakeInteraction{selected: "77"}
	ui.onSelect = func(title string) {
		if strings.Contains(title, "GPG key") {
			if err := os.WriteFile(filepath.Join(root, ".gpg-id"), []byte("AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA\n"), 0600); err != nil {
				t.Fatal(err)
			}
		}
	}
	plugin := &Plugin{HTTPClient: fixture.client, APIBaseURL: "https://api.github.com"}
	if _, err := plugin.Configure(context.Background(), configureContext(project), ui); err != nil {
		t.Fatal(err)
	}
	if len(ui.hostCommands) != 0 {
		t.Fatalf("host commands overwrote concurrent init: %#v", ui.hostCommands)
	}
	if got := readTestFile(t, filepath.Join(root, ".gpg-id")); strings.TrimSpace(got) != "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA" {
		t.Fatalf("recipient was changed: %q", got)
	}
}

func TestConfigureDoesNotOfferDisabledPrimaryGPGKey(t *testing.T) {
	assertDisabledGPGKeyNotOffered(t, "scESCD", "e")
}

func TestConfigureDoesNotOfferDisabledSubkeyBehindAggregateCapability(t *testing.T) {
	assertDisabledGPGKeyNotOffered(t, "scESC", "eD")
}

func assertDisabledGPGKeyNotOffered(t *testing.T, primaryCapabilities, subkeyCapabilities string) {
	t.Helper()
	project := gitProject(t, true)
	root := os.Getenv("PASSWORD_STORE_DIR")
	if err := os.Remove(filepath.Join(root, ".gpg-id")); err != nil {
		t.Fatal(err)
	}
	bin := strings.Split(os.Getenv("PATH"), string(os.PathListSeparator))[0]
	listing := fmt.Sprintf("sec:u:2048:1:ABCDEF:0:0:::::%s:\nfpr:::::::::0123456789012345678901234567890123456789:\nuid:u::::0::AABBCCDDEEFF::Disabled Key <disabled@example.com>::::::::::0:\nssb:u:2048:1:SUBKEY:0:0:::::%s:\nfpr:::::::::AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA:\n", primaryCapabilities, subkeyCapabilities)
	writeGPGListing(t, bin, listing)
	fixture := newOAuthFixture(t, []int64{9001})
	ui := &fakeInteraction{selected: "77", denyAt: 1}
	plugin := &Plugin{HTTPClient: fixture.client, APIBaseURL: "https://api.github.com"}
	_, err := plugin.Configure(context.Background(), configureContext(project), ui)
	if err == nil || !strings.Contains(err.Error(), "create a GPG key manually") {
		t.Fatalf("Configure() error = %v, want no usable key guidance", err)
	}
	if fixture.deviceCodes != 0 || len(ui.hostCommands) != 0 {
		t.Fatalf("device codes=%d host commands=%#v", fixture.deviceCodes, ui.hostCommands)
	}
	for _, selection := range ui.selections {
		if selection.title == "Choose a GPG key for pass" {
			t.Fatalf("disabled key was offered: %#v", selection.choices)
		}
	}
}

func TestConfigureCanUseEncryptionSubkeyWhenPrimaryIsOfflineStub(t *testing.T) {
	project := gitProject(t, true)
	if err := os.Remove(filepath.Join(os.Getenv("PASSWORD_STORE_DIR"), ".gpg-id")); err != nil {
		t.Fatal(err)
	}
	bin := strings.Split(os.Getenv("PATH"), string(os.PathListSeparator))[0]
	listing := strings.Join([]string{
		colonRecord("sec", "u", "scESC", "#"),
		"fpr:::::::::0123456789012345678901234567890123456789:",
		"uid:u::::0::AABBCCDDEEFF::Offline Primary <offline@example.com>::::::::::0:",
		colonRecord("ssb", "u", "e", ""),
		"fpr:::::::::AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA:",
	}, "\n")
	writeGPGListing(t, bin, listing)
	fixture := newOAuthFixture(t, []int64{9001})
	ui := &fakeInteraction{selected: "77"}
	plugin := &Plugin{HTTPClient: fixture.client, APIBaseURL: "https://api.github.com"}
	if _, err := plugin.Configure(context.Background(), configureContext(project), ui); err != nil {
		t.Fatal(err)
	}
	if len(ui.hostCommands) != 1 || ui.hostCommands[0] != "pass init 0123456789012345678901234567890123456789" {
		t.Fatalf("host commands = %#v", ui.hostCommands)
	}
}

func colonRecord(kind, validity, capabilities, tokenMarker string) string {
	fields := make([]string, 15)
	fields[0], fields[1], fields[2], fields[3], fields[4] = kind, validity, "2048", "1", "ABCDEF"
	fields[5], fields[6], fields[11], fields[14] = "0", "0", capabilities, tokenMarker
	return strings.Join(fields, ":") + ":"
}

func writeGPGListing(t *testing.T, bin, listing string) {
	t.Helper()
	command := "#!/bin/sh\nprintf '%s' '" + strings.ReplaceAll(listing, "'", "'\\''") + "'\n"
	if err := os.WriteFile(filepath.Join(bin, "gpg"), []byte(command), 0700); err != nil {
		t.Fatal(err)
	}
}

func TestConfigureStopsWhenNoGPGKeyCreationIsDeclined(t *testing.T) {
	project := gitProject(t, true)
	if err := os.Remove(filepath.Join(os.Getenv("PASSWORD_STORE_DIR"), ".gpg-id")); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DCH_FAKE_GPG_NO_KEYS", "1")
	fixture := newOAuthFixture(t, []int64{9001})
	ui := &fakeInteraction{denyAt: 1}
	plugin := &Plugin{HTTPClient: fixture.client, APIBaseURL: "https://api.github.com"}
	_, err := plugin.Configure(context.Background(), configureContext(project), ui)
	if err == nil || !strings.Contains(err.Error(), "create a GPG key manually") {
		t.Fatalf("Configure() error = %v, want key creation cancellation", err)
	}
	if fixture.deviceCodes != 0 || len(ui.hostCommands) != 0 {
		t.Fatalf("device codes=%d commands=%#v", fixture.deviceCodes, ui.hostCommands)
	}
}

func TestConfigureCreatesGPGKeyThenInitializesPass(t *testing.T) {
	project := gitProject(t, true)
	if err := os.Remove(filepath.Join(os.Getenv("PASSWORD_STORE_DIR"), ".gpg-id")); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DCH_FAKE_GPG_NO_KEYS", "1")
	fixture := newOAuthFixture(t, []int64{9001})
	ui := &fakeInteraction{selected: "77"}
	ui.onHostCommand = func(command string) {
		if command == "gpg --full-generate-key" {
			t.Setenv("DCH_FAKE_GPG_NO_KEYS", "")
		}
	}
	plugin := &Plugin{HTTPClient: fixture.client, APIBaseURL: "https://api.github.com"}
	if _, err := plugin.Configure(context.Background(), configureContext(project), ui); err != nil {
		t.Fatal(err)
	}
	if len(ui.hostCommands) != 2 || ui.hostCommands[0] != "gpg --full-generate-key" || ui.hostCommands[1] != "pass init 0123456789012345678901234567890123456789" {
		t.Fatalf("host commands = %#v", ui.hostCommands)
	}
}

func TestConfigureStopsWhenInteractiveGPGCreationFails(t *testing.T) {
	project := gitProject(t, true)
	if err := os.Remove(filepath.Join(os.Getenv("PASSWORD_STORE_DIR"), ".gpg-id")); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DCH_FAKE_GPG_NO_KEYS", "1")
	fixture := newOAuthFixture(t, []int64{9001})
	ui := &fakeInteraction{}
	ui.commandErr = errors.New("gpg canceled")
	plugin := &Plugin{HTTPClient: fixture.client, APIBaseURL: "https://api.github.com"}
	_, err := plugin.Configure(context.Background(), configureContext(project), ui)
	if err == nil || !strings.Contains(err.Error(), "key creation failed or was canceled") {
		t.Fatalf("Configure() error = %v", err)
	}
	if fixture.deviceCodes != 0 {
		t.Fatalf("device flow started %d times", fixture.deviceCodes)
	}
}

func TestConfigureRejectsPasswordStoreProbeMismatchBeforeDeviceFlow(t *testing.T) {
	project := gitProject(t, true)
	passPath := writeProbePass(t, "wrong-value", false)
	fixture := newOAuthFixture(t, []int64{9001})
	plugin := &Plugin{HTTPClient: fixture.client, APIBaseURL: "https://api.github.com", PassPath: passPath}
	_, err := plugin.Configure(context.Background(), configureContext(project), &fakeInteraction{selected: "77"})
	if err == nil || !strings.Contains(err.Error(), "readback verification failed") {
		t.Fatalf("Configure() error = %v, want probe mismatch", err)
	}
	if fixture.deviceCodes != 0 {
		t.Fatalf("device flow started %d times", fixture.deviceCodes)
	}
}

func TestConfigureReportsProbeCleanupFailureAndStopsBeforeDeviceFlow(t *testing.T) {
	project := gitProject(t, true)
	passPath := writeProbePass(t, "wrong-value", true)
	fixture := newOAuthFixture(t, []int64{9001})
	plugin := &Plugin{HTTPClient: fixture.client, APIBaseURL: "https://api.github.com", PassPath: passPath}
	_, err := plugin.Configure(context.Background(), configureContext(project), &fakeInteraction{selected: "77"})
	if err == nil || !strings.Contains(err.Error(), "pass rm --") {
		t.Fatalf("Configure() error = %v, want cleanup instruction", err)
	}
	if strings.Contains(err.Error(), "wrong-value") || fixture.deviceCodes != 0 {
		t.Fatalf("error=%v device flow=%d", err, fixture.deviceCodes)
	}
}

func writeProbePass(t *testing.T, shown string, failRemove bool) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "pass")
	remove := "rm -f \"$file\""
	if failRemove {
		remove = "exit 1"
	}
	script := "#!/bin/sh\nfor arg in \"$@\"; do entry=$arg; done\nfile=\"$PASSWORD_STORE_DIR/$entry\"\ncase \"$1\" in\n insert) mkdir -p \"$(dirname \"$file\")\"; cat > \"$file\" ;;\n show) if [ ! -f \"$file\" ]; then echo 'not in the password store' >&2; exit 1; fi; printf '%s\\n' '" + shown + "' ;;\n rm) " + remove + " ;;\n *) exit 1 ;;\nesac\n"
	if err := os.WriteFile(path, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestConfigureDiscoveryUsesTwoDeviceFlowsAndRequiresSameUser(t *testing.T) {
	project := gitProject(t, false)
	fixture := newOAuthFixture(t, []int64{9001, 9001})
	ui := &fakeInteraction{selected: "77"}
	plugin := &Plugin{HTTPClient: fixture.client, APIBaseURL: "https://api.github.com"}
	prepared, err := plugin.Configure(context.Background(), configureContext(project), ui)
	if err != nil {
		t.Fatal(err)
	}
	assertPreparedUserBinding(t, prepared.Config, project, 77)
	if fixture.deviceCodes != 2 || len(fixture.polls) != 2 {
		t.Fatalf("device codes=%d polls=%d", fixture.deviceCodes, len(fixture.polls))
	}
	if len(fixture.deviceRequests) != 2 || fixture.deviceRequests[0].Get("scope") != "offline_access" || fixture.deviceRequests[1].Get("scope") != "offline_access" {
		t.Fatalf("device requests = %#v", fixture.deviceRequests)
	}
	if fixture.polls[0].Get("repository_id") != "" || fixture.polls[1].Get("repository_id") != "77" {
		t.Fatalf("discovery/scoped polls = %#v", fixture.polls)
	}
}

func TestConfigureAnchorsCandidateExpiryToOAuthResponse(t *testing.T) {
	setupHostHookTest(t, true)
	project := gitProject(t, true)
	fixture := newOAuthFixture(t, []int64{9001})
	clock := manualAuthClock{now: time.Now()}
	fixture.clock = clock.read
	fixture.onIssue = func() { clock.advance(5 * time.Second) }
	fixture.onScope = func() { clock.advance(10 * time.Second) }
	ui := &fakeInteraction{selected: "77", onConfirm: func(title string) {
		if title == "Confirm GitHub account" {
			clock.advance(20 * time.Second)
		}
	}}
	passPath, secretPath := writablePass(t)
	plugin := &Plugin{HTTPClient: fixture.client, APIBaseURL: "https://api.github.com", PassPath: passPath, Now: clock.read}
	prepared, err := plugin.Configure(context.Background(), configureContext(project), ui)
	if err != nil {
		t.Fatal(err)
	}
	listener := readyCredentialSocket(t, project)
	defer listener.Close()
	if _, err := prepared.Commit(context.Background(), plugins.HostContext{ProjectPath: project, Config: prepared.Config, Store: &testScopedStore{}}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(secretPath)
	if err != nil {
		t.Fatal(err)
	}
	var set TokenSet
	if err := json.Unmarshal(data, &set); err != nil {
		t.Fatal(err)
	}
	if expected := fixture.issuedAt.Add(time.Hour); !set.AccessTokenExpiresAt.Equal(expected) {
		t.Fatalf("access expiry = %s, want OAuth response expiry %s", set.AccessTokenExpiresAt, expected)
	}
	if expected := fixture.issuedAt.Add(2 * time.Hour); !set.RefreshTokenExpiresAt.Equal(expected) {
		t.Fatalf("refresh expiry = %s, want OAuth response expiry %s", set.RefreshTokenExpiresAt, expected)
	}
}

func TestConfigureRejectsTokenExpiredDuringConfirmation(t *testing.T) {
	project := gitProject(t, true)
	fixture := newOAuthFixture(t, []int64{9001})
	fixture.accessExpiresIn = 30
	clock := manualAuthClock{now: time.Now()}
	fixture.clock = clock.read
	fixture.onIssue = func() { clock.advance(5 * time.Second) }
	fixture.onScope = func() { clock.advance(10 * time.Second) }
	ui := &fakeInteraction{selected: "77", onConfirm: func(title string) {
		if title == "Confirm GitHub account" {
			clock.advance(20 * time.Second)
		}
	}}
	plugin := &Plugin{HTTPClient: fixture.client, APIBaseURL: "https://api.github.com", Now: clock.read}
	if _, err := plugin.Configure(context.Background(), configureContext(project), ui); err == nil || !strings.Contains(err.Error(), "expired") {
		t.Fatalf("Configure() error = %v", err)
	}
}

func TestConfigureRejectsNearExpiryAfterScopeVerification(t *testing.T) {
	project := gitProject(t, true)
	fixture := newOAuthFixture(t, []int64{9001})
	fixture.accessExpiresIn = 300
	clock := manualAuthClock{now: time.Now()}
	fixture.clock = clock.read
	fixture.onIssue = func() { clock.advance(5 * time.Second) }
	fixture.onScope = func() { clock.advance(10 * time.Second) }
	ui := &fakeInteraction{selected: "77", onConfirm: func(title string) {
		if title == "Confirm GitHub account" {
			clock.advance(20 * time.Second)
		}
	}}
	plugin := &Plugin{HTTPClient: fixture.client, APIBaseURL: "https://api.github.com", Now: clock.read}
	if _, err := plugin.Configure(context.Background(), configureContext(project), ui); err == nil || !strings.Contains(err.Error(), "too close") {
		t.Fatalf("Configure() error = %v", err)
	}
}

type manualAuthClock struct{ now time.Time }

func (c *manualAuthClock) read() time.Time         { return c.now }
func (c *manualAuthClock) advance(d time.Duration) { c.now = c.now.Add(d) }

func writablePass(t *testing.T) (string, string) {
	t.Helper()
	passPath, secretPath := filepath.Join(t.TempDir(), "pass"), filepath.Join(t.TempDir(), "secret.json")
	script := "#!/bin/sh\ncase \"$1\" in\nshow) if [ -f \"$DCH_WRITABLE_SECRET\" ]; then cat \"$DCH_WRITABLE_SECRET\"; else echo 'not in the password store' >&2; exit 1; fi ;;\ninsert) cat > \"$DCH_WRITABLE_SECRET\" ;;\nrm) rm -f \"$DCH_WRITABLE_SECRET\" ;;\nesac\n"
	if err := os.WriteFile(passPath, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DCH_WRITABLE_SECRET", secretPath)
	return passPath, secretPath
}

func TestConfigureStopsDiscoveryWhenUserDeclinesAccountBeforeRepositoryListing(t *testing.T) {
	project := gitProject(t, false)
	fixture := newOAuthFixture(t, []int64{9001})
	ui := &fakeInteraction{selected: "77", denyAt: 2}
	plugin := &Plugin{HTTPClient: fixture.client, APIBaseURL: "https://api.github.com"}
	if _, err := plugin.Configure(context.Background(), configureContext(project), ui); err == nil || !strings.Contains(err.Error(), "cancelled") {
		t.Fatalf("Configure() error = %v", err)
	}
	if fixture.repositoryListCalls != 0 {
		t.Fatalf("repository listing calls = %d", fixture.repositoryListCalls)
	}
	if fixture.deviceCodes != 1 || len(fixture.polls) != 1 || fixture.polls[0].Get("repository_id") != "" {
		t.Fatalf("device codes=%d polls=%#v", fixture.deviceCodes, fixture.polls)
	}
}

func TestConfigureRejectsAccountChangeAfterDiscovery(t *testing.T) {
	project := gitProject(t, false)
	fixture := newOAuthFixture(t, []int64{9001, 9002})
	ui := &fakeInteraction{selected: "77"}
	plugin := &Plugin{HTTPClient: fixture.client, APIBaseURL: "https://api.github.com"}
	if _, err := plugin.Configure(context.Background(), configureContext(project), ui); err == nil || !strings.Contains(err.Error(), "account changed") {
		t.Fatalf("Configure() error = %v", err)
	}
}

func configureContext(project string) plugins.ConfigureContext {
	shared, _ := json.Marshal(SharedConfig{ClientID: "Iv1_example"})
	return plugins.ConfigureContext{Host: plugins.HostContext{ProjectPath: project}, SharedConfig: shared}
}

func gitProject(t *testing.T, withRemote bool) string {
	t.Helper()
	setupConfigureHost(t)
	project := t.TempDir()
	command := exec.Command("git", "init", "--quiet", project)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git init: %s %v", output, err)
	}
	if withRemote {
		command = exec.Command("git", "-C", project, "remote", "add", "origin", "https://github.com/octo/repo.git")
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("git remote: %s %v", output, err)
		}
	}
	return filepath.Clean(project)
}

func setupConfigureHost(t *testing.T) {
	t.Helper()
	bin := t.TempDir()
	gpg := "#!/bin/sh\nif [ \"$DCH_FAKE_GPG_NO_KEYS\" = 1 ]; then exit 0; fi\nprintf 'sec:u:2048:1:ABCDEF:0:0:::::scESC:\nfpr:::::::::0123456789012345678901234567890123456789:\nuid:u::::0::AABBCCDDEEFF::Test User <test@example.com>::::::::::0:\nssb:u:2048:1:SUBKEY:0:0:::::e:\nfpr:::::::::AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA:\n'\n"
	pass := "#!/bin/sh\nfor arg in \"$@\"; do entry=$arg; done\nfile=\"$PASSWORD_STORE_DIR/$entry\"\ncase \"$1\" in\n init) exit 0 ;;\n insert) mkdir -p \"$(dirname \"$file\")\"; cat > \"$file\" ;;\n show) if [ ! -f \"$file\" ]; then echo 'not in the password store' >&2; exit 1; fi; cat \"$file\" ;;\n rm) rm -f \"$file\" ;;\n *) exit 1 ;;\nesac\n"
	for name, body := range map[string]string{"gpg": gpg, "pass": pass} {
		if err := os.WriteFile(filepath.Join(bin, name), []byte(body), 0700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	store := t.TempDir()
	if err := os.WriteFile(filepath.Join(store, ".gpg-id"), []byte("0123456789012345678901234567890123456789\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PASSWORD_STORE_DIR", store)
}

func TestSecretKeyParserKeepsPrimaryFingerprintAndEncryptionSubkey(t *testing.T) {
	output := []byte("sec:u:2048:1:PRIMARY:0:0:::::scESC:\nfpr:::::::::0123456789012345678901234567890123456789:\nuid:u::::0::AABBCCDDEEFF::Example User <user@example.com>::::::::::0:\nssb:u:2048:1:SUBKEY:0:0:::::e:\nfpr:::::::::AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA:\n")
	keys := parseSecretKeys(output)
	if len(keys) != 1 || keys[0].fingerprint != "0123456789012345678901234567890123456789" || keys[0].user != "Example User <user@example.com>" {
		t.Fatalf("parsed keys = %#v", keys)
	}
}

func assertPreparedUserBinding(t *testing.T, raw json.RawMessage, project string, repositoryID int64) {
	t.Helper()
	var cfg Config
	if err := json.Unmarshal(raw, &cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.ProjectPath != project || cfg.RepositoryID != repositoryID || cfg.Repository != "octo/repo" || cfg.UserID != 9001 || cfg.Login != "octo-user" || cfg.ClientID != "Iv1_example" {
		t.Fatalf("prepared config = %#v", cfg)
	}
}

type fakeInteraction struct {
	selected       string
	confirms       int
	denyAt         int
	opened         int
	onConfirm      func(string)
	onSelect       func(string)
	hostCommands   []string
	onHostCommand  func(string)
	commandErr     error
	notices        []string
	noticeMessages []string
	selections     []recordedSelection
	inputValue     string
	authMethod     string
	inputValues    map[string]string
	inputs         []recordedInput
}

type recordedSelection struct {
	title        string
	choices      []plugins.Choice
	defaultIndex int
}

type recordedInput struct {
	title  string
	secret bool
}

type legacyInteraction struct{}

func (legacyInteraction) Notify(context.Context, string, string) error { return nil }
func (legacyInteraction) Select(context.Context, string, []plugins.Choice, int) (string, error) {
	return "", nil
}
func (legacyInteraction) Confirm(context.Context, string, string) (bool, error) { return false, nil }
func (legacyInteraction) OpenBrowser(context.Context, string) error             { return nil }

func (f *fakeInteraction) Notify(_ context.Context, title, message string) error {
	f.notices = append(f.notices, title)
	f.noticeMessages = append(f.noticeMessages, message)
	return nil
}
func (f *fakeInteraction) Select(_ context.Context, title string, choices []plugins.Choice, defaultIndex int) (string, error) {
	f.selections = append(f.selections, recordedSelection{title: title, choices: append([]plugins.Choice(nil), choices...), defaultIndex: defaultIndex})
	if f.onSelect != nil {
		f.onSelect(title)
	}
	if title == "Choose GitHub authentication" {
		if f.authMethod != "" {
			return f.authMethod, nil
		}
		return "app", nil
	}
	if strings.Contains(title, "GPG key") {
		return "0123456789012345678901234567890123456789", nil
	}
	return f.selected, nil
}
func (f *fakeInteraction) Confirm(_ context.Context, title, _ string) (bool, error) {
	f.confirms++
	if f.onConfirm != nil {
		f.onConfirm(title)
	}
	return f.confirms != f.denyAt, nil
}
func (f *fakeInteraction) OpenBrowser(context.Context, string) error { f.opened++; return nil }
func (f *fakeInteraction) Input(_ context.Context, title, defaultValue string, secret bool) (string, error) {
	f.inputs = append(f.inputs, recordedInput{title: title, secret: secret})
	if value, ok := f.inputValues[title]; ok {
		return value, nil
	}
	if f.inputValue != "" {
		return f.inputValue, nil
	}
	return defaultValue, nil
}
func (f *fakeInteraction) RunHostCommand(_ context.Context, command string, args []string) error {
	line := command + " " + strings.Join(args, " ")
	f.hostCommands = append(f.hostCommands, line)
	if command == "pass" && len(args) == 2 && args[0] == "init" {
		if err := os.WriteFile(filepath.Join(os.Getenv("PASSWORD_STORE_DIR"), ".gpg-id"), []byte(args[1]+"\n"), 0600); err != nil {
			return err
		}
	}
	if f.onHostCommand != nil {
		f.onHostCommand(line)
	}
	return f.commandErr
}

type oauthFixture struct {
	client              *http.Client
	deviceCodes         int
	deviceRequests      []url.Values
	polls               []url.Values
	userIDs             []int64
	repositoryListCalls int
	issuedAt            time.Time
	clock               func() time.Time
	onIssue             func()
	onScope             func()
	accessExpiresIn     int
}

func newOAuthFixture(t *testing.T, userIDs []int64) *oauthFixture {
	t.Helper()
	fixture := &oauthFixture{userIDs: userIDs, clock: time.Now}
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fixture.handle(w, r) })
	fixture.client = &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)
		return recorder.Result(), nil
	})}
	return fixture
}
func (f *oauthFixture) handle(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	switch {
	case r.URL.Path == "/login/device/code":
		f.deviceCodes++
		data, _ := io.ReadAll(r.Body)
		values, _ := url.ParseQuery(string(data))
		f.deviceRequests = append(f.deviceRequests, values)
		_, _ = io.WriteString(w, `{"device_code":"device","user_code":"ABCD-EFGH","verification_uri":"https://github.com/login/device","interval":1,"expires_in":600}`)
	case r.URL.Path == "/login/oauth/access_token":
		if f.onIssue != nil {
			f.onIssue()
		}
		f.issuedAt = f.clock()
		body, _ := io.ReadAll(r.Body)
		values, _ := url.ParseQuery(string(body))
		f.polls = append(f.polls, values)
		expiresIn := f.accessExpiresIn
		if expiresIn == 0 {
			expiresIn = 3600
		}
		_, _ = fmt.Fprintf(w, `{"access_token":"opaque-access","refresh_token":"opaque-refresh","expires_in":%d,"refresh_token_expires_in":7200,"token_type":"bearer"}`, expiresIn)
	case r.URL.Path == "/repos/octo/repo":
		_, _ = io.WriteString(w, `{"id":77,"full_name":"octo/repo"}`)
	case r.URL.Path == "/user":
		id := int64(9001)
		pollIndex := len(f.polls) - 1
		if pollIndex >= 0 && pollIndex < len(f.userIDs) {
			id = f.userIDs[pollIndex]
		}
		_, _ = fmt.Fprintf(w, `{"id":%d,"login":"octo-user"}`, id)
	case r.URL.Path == "/user/installations":
		f.repositoryListCalls++
		if f.onScope != nil {
			f.onScope()
		}
		_, _ = io.WriteString(w, `{"total_count":1,"installations":[{"id":3}]}`)
	case r.URL.Path == "/user/installations/3/repositories":
		if f.onScope != nil {
			f.onScope()
		}
		_, _ = io.WriteString(w, `{"total_count":1,"repositories":[{"id":77,"full_name":"octo/repo"}]}`)
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) { return f(request) }
