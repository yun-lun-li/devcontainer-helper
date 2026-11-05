package hostpass

import (
	"strings"
	"testing"
)

func TestSetupKeysUsePrimaryFingerprintAndEncryptionCapabilities(t *testing.T) {
	keys := parseSetupKeys(setupKeyListing)
	if len(keys) != 1 || keys[0].fingerprint != setupFingerprint || keys[0].identity != "Test User" {
		t.Fatalf("unexpected usable keys: %v", keys)
	}
}

func TestSetupKeysRejectUnusableSecretKeys(t *testing.T) {
	for _, listing := range []string{
		strings.Replace(setupKeyListing, "sec:u:", "sec:r:", 1),
		strings.Replace(setupKeyListing, "sec:u:", "sec:e:", 1),
		strings.Replace(setupKeyListing, "sec:u:", "sec:i:", 1),
		strings.Replace(setupKeyListing, "scESC", "scSC", 1),
		strings.Replace(setupKeyListing, "scESC", "scESCD", 1),
		strings.Replace(setupKeyListing, setupFingerprint, "invalid", 1),
	} {
		if keys := parseSetupKeys(listing); len(keys) != 0 {
			t.Fatalf("accepted unusable key: %v", keys)
		}
	}
}

func TestSetupKeysKeepValidKeysAfterInvalidKey(t *testing.T) {
	invalid := strings.Replace(setupKeyListing, "sec:u:", "sec:r:", 1)
	keys := parseSetupKeys(invalid + setupKeyListing)
	if len(keys) != 1 || keys[0].fingerprint != setupFingerprint {
		t.Fatalf("unexpected keys: %v", keys)
	}
}
