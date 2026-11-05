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

const setupFingerprint = "0123456789ABCDEF0123456789ABCDEF01234567"
const setupKeyListing = "sec:u:255:22:ID:0:0::u:::scESC:\nfpr:::::::::" + setupFingerprint + ":\nuid:u::::::::Test User:\nssb:u:255:18:SUB:0:0:::::e:\nfpr:::::::::AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA:\n"

func setupFixture(t *testing.T, keys string) string {
	t.Helper()
	root := sharedTestRoot(t)
	bin := filepath.Join(root, "bin")
	store := filepath.Join(root, "store")
	for _, dir := range []string{bin, store} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	writeSetupFile(t, filepath.Join(root, "keys"), keys, 0600)
	writeSetupFile(t, filepath.Join(root, "new-keys"), setupKeyListing, 0600)
	writeSetupFile(t, filepath.Join(bin, "gpg"), `#!/bin/sh
printf '%s\n' "gpg $*" >> "$SETUP_LOG"
case "$1" in
 --generate-key) /bin/cp "$SETUP_NEW_KEYS" "$SETUP_KEYS" ;;
 *) /bin/cat "$SETUP_KEYS" ;;
esac
`, 0700)
	writeSetupFile(t, filepath.Join(bin, "pass"), `#!/bin/sh
printf '%s\n' "pass $*" >> "$SETUP_LOG"
case "$1" in
 init) [ -z "$SETUP_FAIL_INIT" ] || exit 1
       printf '%s' "$3" > "$PASSWORD_STORE_DIR/.gpg-id" ;;
 insert) IFS= read -r token
         printf '%s' "$token" > "$PASSWORD_STORE_DIR/$3.gpg" ;;
 show) /bin/cat "$PASSWORD_STORE_DIR/$3.gpg" ;;
 *) exit 1 ;;
esac
`, 0700)
	for _, tool := range []string{"gpg-agent", "pinentry", "sudo"} {
		writeSetupFile(t, filepath.Join(bin, tool), "#!/bin/sh\nexec \"$@\"\n", 0700)
	}
	t.Setenv("PATH", bin)
	t.Setenv("PASSWORD_STORE_DIR", store)
	t.Setenv("SETUP_LOG", filepath.Join(root, "log"))
	t.Setenv("SETUP_KEYS", filepath.Join(root, "keys"))
	t.Setenv("SETUP_NEW_KEYS", filepath.Join(root, "new-keys"))
	return root
}

func writeSetupFile(t *testing.T, path, text string, mode os.FileMode) {
	t.Helper()
	if err := os.WriteFile(path, []byte(text), mode); err != nil {
		t.Fatal(err)
	}
}

func setupTestUI(t *testing.T, answers []int, stdin string, output *bytes.Buffer) SetupUI {
	t.Helper()
	path := filepath.Join(t.TempDir(), "terminal-input")
	writeSetupFile(t, path, stdin, 0600)
	input, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { input.Close() })
	return SetupUI{Stdin: input, Stdout: output, Stderr: output,
		Choose: func(title string, options []string) (int, error) {
			if len(answers) == 0 {
				t.Fatalf("unexpected setup menu: %s %v", title, options)
			}
			answer := answers[0]
			answers = answers[1:]
			return answer, nil
		},
	}
}

func TestSetupInitializesStoreAndInsertsPATWithoutPrintingIt(t *testing.T) {
	root := setupFixture(t, setupKeyListing)
	var output bytes.Buffer
	if err := Setup(context.Background(), "token", setupTestUI(t, []int{0}, "ghp_TEST_ONLY\n", &output)); err != nil {
		t.Fatal(err)
	}
	assertSetupFile(t, filepath.Join(root, "store", ".gpg-id"), setupFingerprint)
	assertSetupFile(t, filepath.Join(root, "store", "token.gpg"), "ghp_TEST_ONLY")
	if strings.Contains(output.String(), "ghp_TEST_ONLY") {
		t.Fatal("setup printed PAT")
	}
	log, err := os.ReadFile(filepath.Join(root, "log"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(log), "ghp_TEST_ONLY") {
		t.Fatal("PAT passed as command argument")
	}
}

func assertSetupFile(t *testing.T, path, want string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != want {
		t.Fatalf("unexpected contents in %s", path)
	}
}

func TestSetupCreatesGPGKeyWhenNoneExists(t *testing.T) {
	root := setupFixture(t, "")
	var output bytes.Buffer
	if err := Setup(context.Background(), "token", setupTestUI(t, []int{0, 0}, "ghp_TEST_ONLY\n", &output)); err != nil {
		t.Fatal(err)
	}
	log, err := os.ReadFile(filepath.Join(root, "log"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(log), "gpg --generate-key") {
		t.Fatal("did not delegate key generation to GPG")
	}
	assertSetupFile(t, filepath.Join(root, "store", ".gpg-id"), setupFingerprint)
}

func TestSetupReusesExistingStoreAndPAT(t *testing.T) {
	root := setupFixture(t, "")
	writeSetupFile(t, filepath.Join(root, "store", ".gpg-id"), "existing-recipient", 0600)
	writeSetupFile(t, filepath.Join(root, "store", "token.gpg"), "ghp_EXISTING", 0600)
	var output bytes.Buffer
	if err := Setup(context.Background(), "token", setupTestUI(t, nil, "", &output)); err != nil {
		t.Fatal(err)
	}
	assertSetupFile(t, filepath.Join(root, "store", ".gpg-id"), "existing-recipient")
	assertSetupFile(t, filepath.Join(root, "log"), "pass show -- token\n")
}

func TestSetupRejectsInvalidPATAndDoesNotExposeIt(t *testing.T) {
	setupFixture(t, setupKeyListing)
	var output bytes.Buffer
	err := Setup(context.Background(), "token", setupTestUI(t, []int{0}, "invalid-secret\n", &output))
	if err == nil {
		t.Fatal("accepted invalid PAT")
	}
	if strings.Contains(output.String()+err.Error(), "invalid-secret") {
		t.Fatal("exposed invalid PAT")
	}
}

func TestSetupCancelBeforeKeyGenerationDoesNotInitializeStore(t *testing.T) {
	root := setupFixture(t, "")
	var output bytes.Buffer
	err := Setup(context.Background(), "token", setupTestUI(t, []int{1}, "", &output))
	if !errors.Is(err, ErrSetupCancelled) {
		t.Fatalf("got %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "store", ".gpg-id")); !os.IsNotExist(err) {
		t.Fatal("initialized store after cancellation")
	}
}

func TestSetupDoesNotReplaceOrReencryptUnconfiguredStore(t *testing.T) {
	root := setupFixture(t, setupKeyListing)
	writeSetupFile(t, filepath.Join(root, "store", "old.gpg"), "existing", 0600)
	var output bytes.Buffer
	if err := Setup(context.Background(), "token", setupTestUI(t, nil, "", &output)); err == nil {
		t.Fatal("reinitialized store with existing entries")
	}
	assertSetupFile(t, filepath.Join(root, "store", "old.gpg"), "existing")
}

func TestSetupInitFailureDoesNotPromptForPAT(t *testing.T) {
	root := setupFixture(t, setupKeyListing)
	t.Setenv("SETUP_FAIL_INIT", "1")
	var output bytes.Buffer
	if err := Setup(context.Background(), "token", setupTestUI(t, []int{0}, "ghp_TEST_ONLY\n", &output)); err == nil {
		t.Fatal("ignored pass init failure")
	}
	if _, err := os.Stat(filepath.Join(root, "store", "token.gpg")); !os.IsNotExist(err) {
		t.Fatal("inserted PAT after failed initialization")
	}
}

func TestSetupRetainsSubstoreRecipients(t *testing.T) {
	root := setupFixture(t, "")
	substore := filepath.Join(root, "store", "project")
	if err := os.Mkdir(substore, 0700); err != nil {
		t.Fatal(err)
	}
	writeSetupFile(t, filepath.Join(substore, ".gpg-id"), "substore-recipient", 0600)
	writeSetupFile(t, filepath.Join(substore, "token.gpg"), "ghp_EXISTING", 0600)
	var output bytes.Buffer
	if err := Setup(context.Background(), "project/token", setupTestUI(t, nil, "", &output)); err != nil {
		t.Fatal(err)
	}
	assertSetupFile(t, filepath.Join(substore, ".gpg-id"), "substore-recipient")
	if _, err := os.Stat(filepath.Join(root, "store", ".gpg-id")); !os.IsNotExist(err) {
		t.Fatal("initialized root over substore")
	}
}
