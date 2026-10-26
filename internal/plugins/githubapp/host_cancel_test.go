package githubapp

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestConfigureReturnsPromptlyWhenCanceledGPGDescendantKeepsPipeOpen(t *testing.T) {
	project := gitProject(t, true)
	root := os.Getenv("PASSWORD_STORE_DIR")
	if err := os.Remove(filepath.Join(root, ".gpg-id")); err != nil {
		t.Fatal(err)
	}
	bin := strings.Split(os.Getenv("PATH"), string(os.PathListSeparator))[0]
	forkingGPG := "#!/bin/sh\n(sleep 1) &\nexit 0\n"
	if err := os.WriteFile(filepath.Join(bin, "gpg"), []byte(forkingGPG), 0700); err != nil {
		t.Fatal(err)
	}
	fixture := newOAuthFixture(t, []int64{9001})
	plugin := &Plugin{HTTPClient: fixture.client, APIBaseURL: "https://api.github.com"}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	started := time.Now()
	_, err := plugin.Configure(ctx, configureContext(project), &fakeInteraction{denyAt: 1})
	elapsed := time.Since(started)
	t.Logf("Configure returned after %s with error %v", elapsed, err)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Configure() error = %v, want deadline exceeded after cancellation", err)
	}
	if elapsed > 500*time.Millisecond {
		t.Fatalf("Configure() took %s after a 50ms caller deadline", elapsed)
	}
	if fixture.deviceCodes != 0 {
		t.Fatalf("Device Flow started %d times after cancellation", fixture.deviceCodes)
	}
}
