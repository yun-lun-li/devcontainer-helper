package githubapp

import (
	"context"
	"encoding/json"
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

func TestConfigureStopsDiscoveryWhenUserDeclinesAccountBeforeRepositoryListing(t *testing.T) {
	project := gitProject(t, false)
	fixture := newOAuthFixture(t, []int64{9001})
	marker := filepath.Join(t.TempDir(), "pass-writes")
	pass := filepath.Join(t.TempDir(), "pass")
	if err := os.WriteFile(pass, []byte("#!/bin/sh\nprintf called > "+marker+"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	ui := &fakeInteraction{selected: "77", denyAt: 2}
	plugin := &Plugin{HTTPClient: fixture.client, APIBaseURL: "https://api.github.com", PassPath: pass}
	if _, err := plugin.Configure(context.Background(), configureContext(project), ui); err == nil || !strings.Contains(err.Error(), "cancelled") {
		t.Fatalf("Configure() error = %v", err)
	}
	if fixture.repositoryListCalls != 0 {
		t.Fatalf("repository listing calls = %d", fixture.repositoryListCalls)
	}
	if fixture.deviceCodes != 1 || len(fixture.polls) != 1 || fixture.polls[0].Get("repository_id") != "" {
		t.Fatalf("device codes=%d polls=%#v", fixture.deviceCodes, fixture.polls)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("pass was invoked before account confirmation: %v", err)
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
	selected string
	confirms int
	denyAt   int
	opened   int
}

func (f *fakeInteraction) Notify(context.Context, string, string) error { return nil }
func (f *fakeInteraction) Select(_ context.Context, _ string, _ []plugins.Choice, _ int) (string, error) {
	return f.selected, nil
}
func (f *fakeInteraction) Confirm(context.Context, string, string) (bool, error) {
	f.confirms++
	return f.confirms != f.denyAt, nil
}
func (f *fakeInteraction) OpenBrowser(context.Context, string) error { f.opened++; return nil }

type oauthFixture struct {
	client              *http.Client
	deviceCodes         int
	deviceRequests      []url.Values
	polls               []url.Values
	userIDs             []int64
	repositoryListCalls int
}

func newOAuthFixture(t *testing.T, userIDs []int64) *oauthFixture {
	t.Helper()
	fixture := &oauthFixture{userIDs: userIDs}
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
		body, _ := io.ReadAll(r.Body)
		values, _ := url.ParseQuery(string(body))
		f.polls = append(f.polls, values)
		_, _ = io.WriteString(w, `{"access_token":"opaque-access","refresh_token":"opaque-refresh","expires_in":3600,"refresh_token_expires_in":7200,"token_type":"bearer"}`)
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
		_, _ = io.WriteString(w, `{"total_count":1,"installations":[{"id":3}]}`)
	case r.URL.Path == "/user/installations/3/repositories":
		_, _ = io.WriteString(w, `{"total_count":1,"repositories":[{"id":77,"full_name":"octo/repo"}]}`)
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) { return f(request) }
