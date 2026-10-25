package githubapp

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestSecretStoreDeleteIsIdempotentAndPreservesOtherEntries(t *testing.T) {
	store, target, other := newDeleteTestStore(t)
	if err := os.WriteFile(target, []byte(`{"owned":"credential"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(other, []byte("other project"), 0600); err != nil {
		t.Fatal(err)
	}
	for attempt := 0; attempt < 2; attempt++ {
		if err := store.Delete(context.Background()); err != nil {
			t.Fatalf("delete attempt %d: %v", attempt+1, err)
		}
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("credential still exists: %v", err)
	}
	if contents, err := os.ReadFile(other); err != nil || string(contents) != "other project" {
		t.Fatalf("other entry changed: %q, %v", contents, err)
	}
}

func TestSecretStoreDeleteReturnsUnknownPassFailure(t *testing.T) {
	store, target, _ := newDeleteTestStore(t)
	if err := os.WriteFile(target, []byte(`{"owned":"credential"}`), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PASS_RM_ERROR", "1")
	if err := store.Delete(context.Background()); err == nil {
		t.Fatal("masked an unknown pass failure")
	}
	if _, err := os.Stat(target); err != nil {
		t.Fatalf("credential was unexpectedly removed: %v", err)
	}
}

func newDeleteTestStore(t *testing.T) (*SecretStore, string, string) {
	t.Helper()
	dir := t.TempDir()
	pass := filepath.Join(dir, "pass")
	target := filepath.Join(dir, "project-secret")
	other := filepath.Join(dir, "other-secret")
	script := "#!/bin/sh\ncase \"$1\" in\nshow) if [ -f \"$PASS_SECRET\" ]; then cat \"$PASS_SECRET\"; else echo 'Error: not in the password store' >&2; exit 1; fi ;;\nrm) if [ \"$PASS_RM_ERROR\" = 1 ]; then echo 'gpg failure' >&2; exit 1; fi; if [ -f \"$PASS_SECRET\" ]; then rm \"$PASS_SECRET\"; else echo 'Error: not in the password store' >&2; exit 1; fi ;;\n*) exit 2 ;;\nesac\n"
	if err := os.WriteFile(pass, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PASS_SECRET", target)
	return &SecretStore{PassPath: pass, Entry: "owned-entry"}, target, other
}
