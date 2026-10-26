package githubapp

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"dch/internal/plugins"
)

func (p *Plugin) prepareHost(ctx context.Context, project, clientID string, ui plugins.Interaction, hostUI plugins.HostCommandInteraction) error {
	if err := installRequirements(ctx, ui, hostUI); err != nil {
		return err
	}
	root, err := passwordStoreRoot()
	if err != nil {
		return err
	}
	base := secretEntry(Config{ProjectPath: project, ClientID: clientID})
	if err := p.initializePass(ctx, base, root, ui, hostUI); err != nil {
		return err
	}
	return p.verifyPass(ctx, &SecretStore{PassPath: p.PassPath, Entry: base})
}

func installRequirements(ctx context.Context, ui plugins.Interaction, hostUI plugins.HostCommandInteraction) error {
	missing := missingExecutables()
	plan, err := installationPlan(missing)
	if err != nil {
		return err
	}
	if len(plan.args) > 0 {
		if err := confirmHostInstall(ctx, ui, hostUI, plan); err != nil {
			return err
		}
		if missing := missingExecutables(); len(missing) > 0 {
			return fmt.Errorf("installation did not provide %s; install them manually", strings.Join(missing, " and "))
		}
	}
	return nil
}

func confirmHostInstall(ctx context.Context, ui plugins.Interaction, hostUI plugins.HostCommandInteraction, plan packagePlan) error {
	command := plan.manager + " " + strings.Join(plan.args, " ")
	ok, err := ui.Confirm(ctx, "Install host requirements", "dch needs GPG and pass. It will run: "+command+". Administrator privileges are required; the package manager keeps its own confirmation. Host package changes remain installed if setup is canceled.")
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("host setup canceled; install GPG and pass manually")
	}
	if err := hostUI.RunHostCommand(ctx, plan.manager, plan.args); err != nil {
		return fmt.Errorf("host package installation failed; install GPG and pass manually: %w", err)
	}
	return nil
}

func (p *Plugin) initializePass(ctx context.Context, entry, root string, ui plugins.Interaction, hostUI plugins.HostCommandInteraction) error {
	recipient, recipientErr := applicableGPGID(root, entry)
	if errors.Is(recipientErr, os.ErrNotExist) {
		return p.createPassStore(ctx, entry, root, ui, hostUI)
	} else if recipientErr != nil {
		return fmt.Errorf("could not inspect password-store recipient configuration: %w", recipientErr)
	}
	return ui.Notify(ctx, "Using existing pass recipient", "The password store already uses GPG key "+recipient+" for this credential path.")
}

func (p *Plugin) createPassStore(ctx context.Context, entry, root string, ui plugins.Interaction, hostUI plugins.HostCommandInteraction) error {
	key, err := p.selectPassRecipient(ctx, ui, hostUI)
	if err != nil {
		return err
	}
	if _, err := applicableGPGID(root, entry); err == nil {
		return ui.Notify(ctx, "Using existing pass recipient", "The password store was initialized during setup; its current GPG recipient will be used.")
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("could not inspect password-store recipient configuration: %w", err)
	}
	return p.initializeRecipient(ctx, entry, root, key, hostUI)
}

func (p *Plugin) selectPassRecipient(ctx context.Context, ui plugins.Interaction, hostUI plugins.HostCommandInteraction) (string, error) {
	keys, err := p.usableKeys(ctx)
	if err != nil {
		return "", err
	}
	if len(keys) == 0 {
		ok, err := ui.Confirm(ctx, "Create a GPG key", "No usable secret key was found. dch will open GPG's interactive key creation flow. GPG handles the identity and passphrase; dch does not receive the passphrase.")
		if err != nil {
			return "", err
		}
		if !ok {
			return "", fmt.Errorf("host setup canceled; create a GPG key manually")
		}
		if err := hostUI.RunHostCommand(ctx, "gpg", []string{"--full-generate-key"}); err != nil {
			return "", fmt.Errorf("GPG key creation failed or was canceled: %w", err)
		}
		keys, err = p.usableKeys(ctx)
		if err != nil {
			return "", fmt.Errorf("could not list GPG secret keys after creation: %w", err)
		}
	}
	if len(keys) == 0 {
		return "", fmt.Errorf("no usable GPG encryption key found")
	}
	return selectSecretKey(ctx, ui, keys)
}

func (p *Plugin) initializeRecipient(ctx context.Context, entry, root, key string, hostUI plugins.HostCommandInteraction) error {
	if err := p.runHostCommand(ctx, hostUI, "pass", "init", key); err != nil {
		return fmt.Errorf("pass initialization failed: %w", err)
	}
	recipient, err := applicableGPGID(root, entry)
	if err != nil || recipient != key {
		return fmt.Errorf("pass initialization did not set the selected GPG recipient; inspect the password store before retrying")
	}
	return nil
}

func (p *Plugin) usableKeys(ctx context.Context) ([]secretKey, error) {
	output, err := p.runCommand(ctx, "gpg", "--batch", "--with-colons", "--list-secret-keys")
	if err != nil {
		return nil, fmt.Errorf("could not list GPG secret keys: %w", err)
	}
	return parseSecretKeys(output), nil
}

func passwordStoreRoot() (string, error) {
	if root := os.Getenv("PASSWORD_STORE_DIR"); root != "" {
		return filepath.Abs(root)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".password-store"), nil
}

func applicableGPGID(root, entry string) (string, error) {
	dir := filepath.Join(root, filepath.Clean(entry))
	for dir = filepath.Dir(dir); ; dir = filepath.Dir(dir) {
		data, err := os.ReadFile(filepath.Join(dir, ".gpg-id"))
		if err == nil {
			recipient := strings.TrimSpace(string(data))
			if recipient == "" {
				return "", fmt.Errorf("password-store recipient file is empty")
			}
			return recipient, nil
		}
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
		if dir == root || filepath.Dir(dir) == dir {
			return "", os.ErrNotExist
		}
	}
}

func missingExecutables() []string {
	var missing []string
	for _, name := range []string{"gpg", "pass"} {
		if _, err := exec.LookPath(name); err != nil {
			missing = append(missing, name)
		}
	}
	return missing
}

func (p *Plugin) runCommand(ctx context.Context, name string, args ...string) ([]byte, error) {
	commandCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	command := exec.CommandContext(commandCtx, name, args...)
	var output hostSetupOutput
	command.Stdout, command.Stderr = &output, &output
	err := command.Run()
	if output.exceeded {
		return nil, fmt.Errorf("host command output exceeded limit")
	}
	return output.data, err
}

type hostSetupOutput struct {
	data     []byte
	exceeded bool
}

func (output *hostSetupOutput) Write(data []byte) (int, error) {
	if len(output.data)+len(data) > 1<<20 {
		output.exceeded = true
		return 0, fmt.Errorf("output limit exceeded")
	}
	output.data = append(output.data, data...)
	return len(data), nil
}

func (p *Plugin) runHostCommand(ctx context.Context, ui plugins.HostCommandInteraction, name string, args ...string) error {
	return ui.RunHostCommand(ctx, name, args)
}

func (p *Plugin) verifyPass(ctx context.Context, store *SecretStore) error {
	var random [24]byte
	if _, err := rand.Read(random[:]); err != nil {
		return fmt.Errorf("could not create password-store probe")
	}
	entry := filepath.Join(filepath.Dir(store.Entry), ".setup-probe-"+hex.EncodeToString(random[:8]))
	value := hex.EncodeToString(random[:])
	cleanup := func() error {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_, stderr, err := store.runCaptured(cleanupCtx, nil, "rm", "--force", entry)
		if err != nil {
			return fmt.Errorf("could not remove host setup probe %q; retry with pass rm -- %s", entry, entry)
		}
		_, stderr, err = store.runCaptured(cleanupCtx, nil, "show", entry)
		if err == nil || !strings.Contains(strings.ToLower(string(stderr)), "not in the password store") {
			return fmt.Errorf("host setup probe %q remains; retry with pass rm -- %s", entry, entry)
		}
		return nil
	}
	if _, _, err := store.runCaptured(ctx, []byte(value+"\n"), "insert", "--multiline", "--force", entry); err != nil {
		if cleanupErr := cleanup(); cleanupErr != nil {
			return cleanupErr
		}
		return fmt.Errorf("password-store write probe failed")
	}
	read, _, err := store.runCaptured(ctx, nil, "show", entry)
	if err != nil || strings.TrimSpace(string(read)) != value {
		if cleanupErr := cleanup(); cleanupErr != nil {
			return cleanupErr
		}
		return errors.New("password-store readback verification failed")
	}
	return cleanup()
}
