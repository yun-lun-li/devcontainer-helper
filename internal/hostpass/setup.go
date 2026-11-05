package hostpass

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"dch/internal/common"
)

// ErrSetupCancelled indicates that host setup was declined.
var ErrSetupCancelled = errors.New("host pass setup cancelled")

// SetupUI provides terminal access and menu selection for interactive host setup.
type SetupUI struct {
	Choose         func(string, []string) (int, error)
	Stdin          io.Reader
	Stdout, Stderr io.Writer
}

// Setup prepares host dependencies, initializes a new password store when needed,
// and stores or verifies the PAT. Existing recipient settings are retained.
func Setup(ctx context.Context, entry string, ui SetupUI) error {
	if err := validateEntry(entry); err != nil {
		return err
	}
	if ui.Choose == nil {
		return fmt.Errorf("host setup requires a menu")
	}
	if err := ui.ensureTools(ctx); err != nil {
		return err
	}
	dir, err := sharedDirectory()
	if err != nil {
		return err
	}
	unlock, err := lockRuntime(ctx, filepath.Join(dir, "setup.lock"), true)
	if err != nil {
		return err
	}
	defer unlock()
	if err := ui.initializeStore(ctx, entry); err != nil {
		return err
	}
	return ensureToken(ctx, entry, ui.Stdin, ui.Stdout, ui.Stderr)
}

func (ui SetupUI) run(ctx context.Context, args ...string) error {
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = ui.Stdin, ui.Stdout, ui.Stderr
	cmd.Env = passEnvironment()
	if err := common.RunHostCommand(cmd); err != nil {
		return fmt.Errorf("%s failed: %w", args[0], err)
	}
	return nil
}

func missingHostTools() []string {
	var missing []string
	for _, name := range []string{"pass", "gpg", "gpg-agent", "pinentry"} {
		if _, err := exec.LookPath(name); err != nil {
			missing = append(missing, name)
		}
	}
	return missing
}

func (ui SetupUI) ensureTools(ctx context.Context) error {
	missing := missingHostTools()
	if len(missing) == 0 {
		return nil
	}
	plan, err := hostInstallPlan()
	if err != nil {
		return err
	}
	var commands []string
	for _, args := range plan {
		commands = append(commands, strings.Join(args, " "))
	}
	title := "Missing host tools: " + strings.Join(missing, ", ") + "\n" + strings.Join(commands, "\n")
	choice, err := ui.Choose(title, []string{"Install host dependencies", "Cancel"})
	if err != nil {
		return err
	}
	if choice != 0 {
		return ErrSetupCancelled
	}
	for _, args := range plan {
		if err := ui.run(ctx, args...); err != nil {
			return err
		}
	}
	if missing := missingHostTools(); len(missing) != 0 {
		return fmt.Errorf("host tools still missing: %s", strings.Join(missing, ", "))
	}
	return nil
}

func hostInstallPlan() ([][]string, error) {
	plans := [][][]string{
		{{"apt-get", "update"}, {"apt-get", "install", "-y", "pass", "gnupg", "pinentry-curses"}},
		{{"dnf", "install", "-y", "pass", "gnupg2", "pinentry-curses"}},
		{{"pacman", "-S", "--needed", "pass", "gnupg", "pinentry"}},
		{{"apk", "add", "pass", "gnupg", "pinentry"}},
	}
	for _, plan := range plans {
		if _, err := exec.LookPath(plan[0][0]); err != nil {
			continue
		}
		return privilegedInstallPlan(plan)
	}
	return nil, fmt.Errorf("install pass, GnuPG, gpg-agent and pinentry on the host; supported installers: apt-get, dnf, pacman, apk")
}

func privilegedInstallPlan(plan [][]string) ([][]string, error) {
	if os.Geteuid() == 0 {
		return plan, nil
	}
	if _, err := exec.LookPath("sudo"); err != nil {
		return nil, fmt.Errorf("install host dependencies as administrator, or install sudo to continue")
	}
	for i, args := range plan {
		plan[i] = append([]string{"sudo"}, args...)
	}
	return plan, nil
}

func passwordStoreDirectory() (string, error) {
	if root := os.Getenv("PASSWORD_STORE_DIR"); root != "" {
		return filepath.Abs(root)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".password-store"), nil
}

func (ui SetupUI) initializeStore(ctx context.Context, entry string) error {
	root, err := passwordStoreDirectory()
	if err != nil {
		return err
	}
	initialized, err := storeHasRecipients(root, entry)
	if err != nil || initialized {
		return err
	}
	if err := checkNewPasswordStore(root); err != nil {
		return err
	}
	key, err := ui.chooseGPGKey(ctx)
	if err != nil {
		return err
	}
	fmt.Fprintln(ui.Stdout, "Initialize host password store:", root)
	return ui.run(ctx, "pass", "init", "--", key)
}

func storeHasRecipients(root, entry string) (bool, error) {
	for dir := filepath.Dir(filepath.Join(root, entry)); ; dir = filepath.Dir(dir) {
		data, err := os.ReadFile(filepath.Join(dir, ".gpg-id"))
		if err == nil {
			if strings.TrimSpace(string(data)) == "" {
				return false, fmt.Errorf("empty pass recipient configuration in %s", dir)
			}
			return true, nil
		}
		if !os.IsNotExist(err) {
			return false, err
		}
		if dir == root {
			return false, nil
		}
	}
}

func checkNewPasswordStore(root string) error {
	return filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if os.IsNotExist(err) && path == root {
			return nil
		}
		if err != nil {
			return err
		}
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".gpg") {
			return fmt.Errorf("password store has existing encrypted entries without recipients for this entry; configure pass manually")
		}
		return nil
	})
}
