package hostpass_test

import (
	"context"
	"testing"

	"dch/internal/hostpass"
)

func TestStopIfRunningHandlesActiveAndAbsentService(t *testing.T) {
	project, _ := setupHost(t)
	if err := hostpass.StopIfRunning(context.Background(), project); err != nil {
		t.Fatal(err)
	}
	cancel, done := startService(t, project, "github/project")
	defer cancel()
	if err := hostpass.StopIfRunning(context.Background(), project); err != nil {
		t.Fatal(err)
	}
	awaitService(t, done)
	if err := hostpass.StopIfRunning(context.Background(), project); err != nil {
		t.Fatal(err)
	}
}
