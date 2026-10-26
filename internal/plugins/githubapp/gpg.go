package githubapp

import (
	"context"
	"dch/internal/plugins"
	"fmt"
	"strings"
)

type secretKey struct{ fingerprint, user string }
type keyParseState struct {
	key                                 secretKey
	active, canEncrypt, haveFingerprint bool
}

func parseSecretKeys(output []byte) []secretKey {
	var keys []secretKey
	var state keyParseState
	for _, line := range strings.Split(string(output), "\n") {
		parts := strings.Split(line, ":")
		keys, state = consumeKeyRecord(keys, state, parts)
	}
	if completeKey(state) {
		keys = append(keys, state.key)
	}
	return keys
}

func consumeKeyRecord(keys []secretKey, state keyParseState, parts []string) ([]secretKey, keyParseState) {
	if len(parts) < 10 {
		return keys, state
	}
	switch parts[0] {
	case "sec":
		if completeKey(state) {
			keys = append(keys, state.key)
		}
		capabilities := colonField(parts, 11)
		active := usableKeyValidity(colonField(parts, 1)) && !strings.Contains(capabilities, "D")
		state = keyParseState{
			active:     active,
			canEncrypt: active && encryptionCapability(capabilities) && colonField(parts, 14) != "#",
		}
	case "ssb":
		capabilities := colonField(parts, 11)
		state.canEncrypt = state.canEncrypt || (state.active && usableKeyValidity(colonField(parts, 1)) && encryptionCapability(capabilities) && colonField(parts, 14) != "#")
	case "fpr":
		if state.active && !state.haveFingerprint && len(parts[9]) == 40 {
			state.key.fingerprint, state.haveFingerprint = parts[9], true
		}
	case "uid":
		if state.active && state.key.user == "" {
			state.key.user = parts[9]
		}
	}
	return keys, state
}

func colonField(fields []string, index int) string {
	if index >= len(fields) {
		return ""
	}
	return fields[index]
}

func encryptionCapability(capabilities string) bool {
	return strings.Contains(capabilities, "e") && !strings.Contains(capabilities, "D")
}

func completeKey(state keyParseState) bool {
	return state.active && state.canEncrypt && state.haveFingerprint && state.key.user != ""
}

func usableKeyValidity(validity string) bool {
	return validity == "u" || validity == "m" || validity == "f"
}

func selectSecretKey(ctx context.Context, ui plugins.Interaction, keys []secretKey) (string, error) {
	choices := make([]plugins.Choice, len(keys))
	for i, key := range keys {
		choices[i] = plugins.Choice{Value: key.fingerprint, Label: key.user + " — " + key.fingerprint}
	}
	selected, err := ui.Select(ctx, "Choose a GPG key for pass", choices, 0)
	if err != nil {
		return "", err
	}
	for _, key := range keys {
		if selected == key.fingerprint {
			return selected, nil
		}
	}
	return "", fmt.Errorf("selected GPG key is unavailable")
}
