package hostpass

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func removeSetupDependencies(t *testing.T, root string) {
	t.Helper()
	for _, name := range []string{"pass", "gpg", "gpg-agent", "pinentry"} {
		if err := os.Remove(filepath.Join(root, "bin", name)); err != nil {
			t.Fatal(err)
		}
	}
}

func TestHostDependencyInstallRequiresSelection(t *testing.T) {
	root := setupFixture(t, "")
	removeSetupDependencies(t, root)
	marker := filepath.Join(root, "installer-ran")
	t.Setenv("SETUP_INSTALL_MARKER", marker)
	writeSetupFile(t, filepath.Join(root, "bin", "apt-get"), "#!/bin/sh\n/bin/touch \"$SETUP_INSTALL_MARKER\"\n", 0700)
	var output bytes.Buffer
	err := Setup(context.Background(), "token", setupTestUI(t, []int{1}, "", &output))
	if !errors.Is(err, ErrSetupCancelled) {
		t.Fatalf("got %v", err)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatal("installed packages after cancellation")
	}
}

func TestHostDependencyInstallFailureStopsSetup(t *testing.T) {
	root := setupFixture(t, "")
	removeSetupDependencies(t, root)
	writeSetupFile(t, filepath.Join(root, "bin", "apt-get"), "#!/bin/sh\nexit 1\n", 0700)
	var output bytes.Buffer
	err := Setup(context.Background(), "token", setupTestUI(t, []int{0}, "", &output))
	if err == nil {
		t.Fatal("ignored package installation failure")
	}
	if _, err := os.Stat(filepath.Join(root, "store", ".gpg-id")); !os.IsNotExist(err) {
		t.Fatal("initialized store after installation failed")
	}
}

func TestHostDependencyInstallerRechecksTools(t *testing.T) {
	root := setupFixture(t, "")
	removeSetupDependencies(t, root)
	writeSetupFile(t, filepath.Join(root, "bin", "apk"), "#!/bin/sh\nexit 0\n", 0700)
	var output bytes.Buffer
	err := Setup(context.Background(), "token", setupTestUI(t, []int{0}, "", &output))
	if err == nil || !strings.Contains(err.Error(), "still missing") {
		t.Fatalf("got %v", err)
	}
}

func TestHostDependencyInstallerContinuesAfterSuccessfulInstall(t *testing.T) {
	root := setupFixture(t, setupKeyListing)
	templates := filepath.Join(root, "templates")
	if err := os.Rename(filepath.Join(root, "bin"), templates); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "bin"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SETUP_TEMPLATES", templates)
	t.Setenv("SETUP_BIN", filepath.Join(root, "bin"))
	writeSetupFile(t, filepath.Join(root, "bin", "sudo"), "#!/bin/sh\nexec \"$@\"\n", 0700)
	writeSetupFile(t, filepath.Join(root, "bin", "apk"), `#!/bin/sh
for name in pass gpg gpg-agent pinentry; do
 /bin/cp "$SETUP_TEMPLATES/$name" "$SETUP_BIN/$name"
done
`, 0700)
	var output bytes.Buffer
	if err := Setup(context.Background(), "token", setupTestUI(t, []int{0, 0}, "ghp_TEST_ONLY\n", &output)); err != nil {
		t.Fatal(err)
	}
	assertSetupFile(t, filepath.Join(root, "store", "token.gpg"), "ghp_TEST_ONLY")
}

func TestSupportedHostInstallerPlans(t *testing.T) {
	for _, manager := range []string{"apt-get", "dnf", "pacman", "apk"} {
		t.Run(manager, func(t *testing.T) {
			root := setupFixture(t, "")
			writeSetupFile(t, filepath.Join(root, "bin", manager), "#!/bin/sh\nexit 0\n", 0700)
			plan, err := hostInstallPlan()
			if err != nil {
				t.Fatal(err)
			}
			last := strings.Join(plan[len(plan)-1], " ")
			if !strings.Contains(last, manager) || !strings.Contains(last, "pass") || !strings.Contains(last, "pinentry") {
				t.Fatalf("unexpected install plan: %v", plan)
			}
		})
	}
}
