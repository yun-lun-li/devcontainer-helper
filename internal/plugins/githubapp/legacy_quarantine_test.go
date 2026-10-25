package githubapp

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"dch/internal/plugins"
)

func TestStatusStopsOwnedLegacyServiceBeforeCredentialSocketRemainsAvailable(t *testing.T) {
	env, host, socketPath := legacyServiceFixture(t)
	status, err := New().Status(context.Background(), host)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(status.Message, "Authorization required") {
		t.Fatalf("status message = %q", status.Message)
	}
	if err := CheckCredential(context.Background(), socketPath); err == nil {
		t.Fatal("legacy service socket remained available")
	}
	if got := readTestFile(t, filepath.Join(env.root, "systemctl.log")); strings.TrimSpace(got) != "stop" {
		t.Fatalf("systemctl calls = %q", got)
	}
}

func legacyServiceFixture(t *testing.T) (hostHookTestEnv, plugins.HostContext, string) {
	t.Helper()
	env := setupHostHookTest(t, true)
	project := t.TempDir()
	listener := readyCredentialSocket(t, project)
	socketPath := listener.Addr().String()
	script := "#!/bin/sh\nshift\necho \"$1\" >> \"$DCH_TEST_SYSTEMCTL_LOG\"\ncase \"$1\" in\n  is-active) exit 0 ;;\n  stop) rm -f \"$DCH_TEST_SOCKET_PATH\"; exit 0 ;;\n  *) exit 0 ;;\nesac\n"
	binPath := filepath.Join(env.root, "bin", "systemctl")
	if err := os.WriteFile(binPath, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DCH_TEST_SOCKET_PATH", socketPath)
	unit, err := prepareServiceUnit(project, ownershipState{})
	if err != nil {
		t.Fatal(err)
	}
	state := ownershipState{UnitPath: unit.path, UnitDigest: unit.digest}
	legacy := []byte(`{"app_id":12,"installation_id":34,"private_key_pass_entry":"legacy/private-key"}`)
	host := plugins.HostContext{ProjectPath: project, Config: legacy, State: encodeState(state)}
	return env, host, socketPath
}

func TestEnsureQuarantinesLegacyServiceBeforeContainerOperations(t *testing.T) {
	env, host, socketPath := legacyServiceFixture(t)
	_, err := New().HandleHost(context.Background(), host, plugins.EventStart)
	if err == nil || !strings.Contains(err.Error(), "authorization required") {
		t.Fatalf("HandleHost() error = %v", err)
	}
	if err := CheckCredential(context.Background(), socketPath); err == nil {
		t.Fatal("legacy service socket remained available")
	}
	if got := strings.TrimSpace(readTestFile(t, filepath.Join(env.root, "systemctl.log"))); got != "stop" {
		t.Fatalf("systemctl calls = %q", got)
	}
}

func TestServeHostQuarantinesLegacyServiceInsteadOfServingCredentials(t *testing.T) {
	env, host, socketPath := legacyServiceFixture(t)
	err := New().ServeHost(context.Background(), host)
	if err == nil || !strings.Contains(err.Error(), "authorization required") {
		t.Fatalf("ServeHost() error = %v", err)
	}
	if err := CheckCredential(context.Background(), socketPath); err == nil {
		t.Fatal("legacy service socket remained available")
	}
	if got := strings.TrimSpace(readTestFile(t, filepath.Join(env.root, "systemctl.log"))); got != "stop" {
		t.Fatalf("systemctl calls = %q", got)
	}
}

func TestLegacyQuarantineRefusesEditedUnitWithoutStoppingService(t *testing.T) {
	env, host, socketPath := legacyServiceFixture(t)
	state, err := decodeState(host.State)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(state.UnitPath, []byte("foreign unit"), 0600); err != nil {
		t.Fatal(err)
	}
	_, err = New().Status(context.Background(), host)
	if err == nil || !strings.Contains(err.Error(), "quarantine failed") {
		t.Fatalf("Status() error = %v", err)
	}
	assertNoSystemctlCalls(t, env)
	if err := CheckCredential(context.Background(), socketPath); err != nil {
		t.Fatalf("foreign socket was stopped: %v", err)
	}
}

func TestLegacyQuarantineReportsStopFailureWithoutClaimingSuccess(t *testing.T) {
	env, host, socketPath := legacyServiceFixture(t)
	script := "#!/bin/sh\nshift\necho \"$1\" >> \"$DCH_TEST_SYSTEMCTL_LOG\"\n[ \"$1\" = stop ] && exit 1\nexit 0\n"
	if err := os.WriteFile(filepath.Join(env.root, "bin", "systemctl"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	status, err := New().Status(context.Background(), host)
	if err == nil || !strings.Contains(status.Message, "could not be quarantined safely") {
		t.Fatalf("Status() = %#v, %v", status, err)
	}
	if err := CheckCredential(context.Background(), socketPath); err != nil {
		t.Fatalf("test service unexpectedly stopped: %v", err)
	}
}

func TestLegacyQuarantineRefusesActiveServiceWithoutOwnershipMetadata(t *testing.T) {
	for _, state := range []ownershipState{{}, {UnitPath: "/some/unit.service"}, {UnitDigest: "digest"}} {
		t.Run(state.UnitPath+state.UnitDigest, func(t *testing.T) {
			env, host, socketPath := legacyServiceFixture(t)
			host.State = encodeState(state)
			status, err := New().Status(context.Background(), host)
			if err == nil || !strings.Contains(status.Message, "could not be quarantined safely") {
				t.Fatalf("Status() = %#v, %v", status, err)
			}
			if got := strings.TrimSpace(readTestFile(t, filepath.Join(env.root, "systemctl.log"))); got != "is-active" {
				t.Fatalf("systemctl calls = %q", got)
			}
			if err := CheckCredential(context.Background(), socketPath); err != nil {
				t.Fatalf("unverified service was stopped: %v", err)
			}
		})
	}
}

func TestLegacyQuarantineAllowsKnownInactiveServiceWithoutOwnershipMetadata(t *testing.T) {
	env, host, _ := legacyServiceFixture(t)
	host.State = encodeState(ownershipState{})
	script := "#!/bin/sh\nshift\necho \"$1\" >> \"$DCH_TEST_SYSTEMCTL_LOG\"\n[ \"$1\" = is-active ] && { echo inactive; exit 3; }\nexit 0\n"
	if err := os.WriteFile(filepath.Join(env.root, "bin", "systemctl"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	status, err := New().Status(context.Background(), host)
	if err != nil || !strings.Contains(status.Message, "Authorization required") {
		t.Fatalf("Status() = %#v, %v", status, err)
	}
	if got := strings.TrimSpace(readTestFile(t, filepath.Join(env.root, "systemctl.log"))); got != "is-active" {
		t.Fatalf("systemctl calls = %q", got)
	}
}

type quarantineSystemdState struct {
	name     string
	state    string
	exitCode int
	allowed  bool
}

func TestLegacyQuarantineRequiresExactInactiveSystemdState(t *testing.T) {
	cases := []quarantineSystemdState{
		{name: "failed", state: "failed\n", exitCode: 3, allowed: true},
		{name: "unknown transition code", state: "unknown\n", exitCode: 3},
		{name: "activating", state: "activating\n", exitCode: 3},
		{name: "deactivating", state: "deactivating\n", exitCode: 3},
		{name: "refreshing", state: "refreshing\n", exitCode: 3},
		{name: "multiple states", state: "inactive\nactive\n", exitCode: 3},
		{name: "oversized output", state: strings.Repeat("x", 5000), exitCode: 3},
		{name: "not found", state: "unknown\n", exitCode: 4, allowed: true},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			assertLegacyQuarantineSystemdState(t, test)
		})
	}
}

func assertLegacyQuarantineSystemdState(t *testing.T, test quarantineSystemdState) {
	t.Helper()
	env, host, socketPath := legacyServiceFixture(t)
	host.State = encodeState(ownershipState{})
	script := fmt.Sprintf("#!/bin/sh\nshift\necho \"$1\" >> \"$DCH_TEST_SYSTEMCTL_LOG\"\nprintf '%%s' '%s'\nexit %d\n", test.state, test.exitCode)
	if err := os.WriteFile(filepath.Join(env.root, "bin", "systemctl"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	status, err := New().Status(context.Background(), host)
	if test.allowed {
		if err != nil || !strings.Contains(status.Message, "Authorization required") {
			t.Fatalf("Status() = %#v, %v", status, err)
		}
		if got := strings.TrimSpace(readTestFile(t, filepath.Join(env.root, "systemctl.log"))); got != "is-active" {
			t.Fatalf("systemctl calls = %q", got)
		}
		return
	}
	if err == nil || !strings.Contains(status.Message, "could not be quarantined safely") {
		t.Fatalf("Status() = %#v, %v", status, err)
	}
	if err := CheckCredential(context.Background(), socketPath); err != nil {
		t.Fatalf("ambiguous service was stopped: %v", err)
	}
}

func TestLegacyQuarantineRejectsUnknownServiceStateWithoutOwnershipMetadata(t *testing.T) {
	env, host, socketPath := legacyServiceFixture(t)
	host.State = encodeState(ownershipState{})
	script := "#!/bin/sh\nshift\necho \"$1\" >> \"$DCH_TEST_SYSTEMCTL_LOG\"\nexit 1\n"
	if err := os.WriteFile(filepath.Join(env.root, "bin", "systemctl"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	status, err := New().Status(context.Background(), host)
	if err == nil || !strings.Contains(status.Message, "could not be quarantined safely") {
		t.Fatalf("Status() = %#v, %v", status, err)
	}
	if got := strings.TrimSpace(readTestFile(t, filepath.Join(env.root, "systemctl.log"))); got != "is-active" {
		t.Fatalf("systemctl calls = %q", got)
	}
	if err := CheckCredential(context.Background(), socketPath); err != nil {
		t.Fatalf("unverified service was stopped: %v", err)
	}
}

func assertNoSystemctlCalls(t *testing.T, env hostHookTestEnv) {
	t.Helper()
	path := filepath.Join(env.root, "systemctl.log")
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return
	}
	if err != nil {
		t.Fatal(err)
	}
	if len(data) != 0 {
		t.Fatalf("systemctl calls = %q", data)
	}
}
