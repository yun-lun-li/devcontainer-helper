package hostpass

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strings"

	"dch/internal/common"
)

type setupKey struct{ fingerprint, identity string }

var fingerprintPattern = regexp.MustCompile(`^(?:[A-Fa-f0-9]{40}|[A-Fa-f0-9]{64})$`)

func (ui SetupUI) chooseGPGKey(ctx context.Context) (string, error) {
	keys, err := listSetupKeys(ctx, ui)
	if err != nil {
		return "", err
	}
	options := make([]string, 0, len(keys)+2)
	for _, key := range keys {
		options = append(options, key.identity+"  ["+key.fingerprint+"]")
	}
	options = append(options, "Create a GPG key", "Cancel")
	choice, err := ui.Choose("Choose an encryption key for the host password store", options)
	if err != nil {
		return "", err
	}
	if choice < 0 || choice > len(keys) {
		return "", ErrSetupCancelled
	}
	if choice < len(keys) {
		return keys[choice].fingerprint, nil
	}
	fmt.Fprintln(ui.Stdout, "GPG will ask for your name, email and key passphrase. Keep the default encryption-capable key type.")
	if err := ui.run(ctx, "gpg", "--generate-key"); err != nil {
		return "", err
	}
	keys, err = listSetupKeys(ctx, ui)
	if err != nil {
		return "", err
	}
	return ui.selectGeneratedKey(keys)
}

func (ui SetupUI) selectGeneratedKey(keys []setupKey) (string, error) {
	if len(keys) == 0 {
		return "", fmt.Errorf("no usable encryption secret key found after GPG key generation")
	}
	options := make([]string, 0, len(keys)+1)
	for _, key := range keys {
		options = append(options, key.identity+"  ["+key.fingerprint+"]")
	}
	options = append(options, "Cancel")
	choice, err := ui.Choose("Choose the key for the host password store", options)
	if err != nil {
		return "", err
	}
	if choice < 0 || choice >= len(keys) {
		return "", ErrSetupCancelled
	}
	return keys[choice].fingerprint, nil
}

func listSetupKeys(ctx context.Context, ui SetupUI) ([]setupKey, error) {
	cmd := exec.CommandContext(ctx, "gpg", "--batch", "--with-colons", "--fixed-list-mode", "--list-secret-keys", "--fingerprint")
	var output bytes.Buffer
	cmd.Stdout, cmd.Stderr, cmd.Env = &output, ui.Stderr, passEnvironment()
	if err := common.RunHostCommand(cmd); err != nil {
		return nil, fmt.Errorf("list GPG secret keys: %w", err)
	}
	return parseSetupKeys(output.String()), nil
}

func parseSetupKeys(output string) []setupKey {
	var keys []setupKey
	var key setupKey
	usable, primary := false, false
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Split(line, ":")
		if len(fields) < 10 {
			continue
		}
		switch fields[0] {
		case "sec":
			keys = appendSetupKey(keys, key, usable)
			key = setupKey{}
			usable = usableSecretKey(fields)
			primary = true
		case "ssb":
			primary = false
		case "fpr":
			if primary && key.fingerprint == "" {
				key.fingerprint = fields[9]
			}
		case "uid":
			if key.identity == "" {
				key.identity = fields[9]
			}
		}
	}
	return appendSetupKey(keys, key, usable)
}

func usableSecretKey(fields []string) bool {
	// Uppercase E also covers usable encryption subkeys on a signing primary key.
	return len(fields) > 11 && !strings.ContainsAny(fields[1], "reid") && strings.ContainsAny(fields[11], "eE") && !strings.Contains(fields[11], "D")
}

func appendSetupKey(keys []setupKey, key setupKey, usable bool) []setupKey {
	if usable && fingerprintPattern.MatchString(key.fingerprint) {
		return append(keys, key)
	}
	return keys
}
