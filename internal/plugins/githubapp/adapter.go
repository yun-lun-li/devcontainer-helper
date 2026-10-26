package githubapp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

const containerSocketDir = "/run/dch-github"

// AdapterFragment returns the devcontainer settings required by the GitHub App adapter.
func AdapterFragment(socketDir string) ([]byte, error) {
	if !filepath.IsAbs(socketDir) || socketDir == "/" || strings.ContainsAny(socketDir, ",\r\n") || strings.Contains(socketDir, "${") {
		return nil, fmt.Errorf("invalid GitHub App runtime directory")
	}
	fragment := struct {
		Mounts    []string          `json:"mounts"`
		RemoteEnv map[string]string `json:"remoteEnv"`
	}{
		Mounts:    []string{"source=" + socketDir + ",target=" + containerSocketDir + ",type=bind,readonly"},
		RemoteEnv: map[string]string{"PATH": containerSocketDir + "/bin:${containerEnv:PATH}"},
	}
	var out bytes.Buffer
	encoder := json.NewEncoder(&out)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(fragment); err != nil {
		return nil, fmt.Errorf("encode GitHub App adapter fragment: %w", err)
	}
	return out.Bytes(), nil
}

// WriteAdapterScripts securely writes the non-secret container adapter scripts under socketDir/bin.
func WriteAdapterScripts(socketDir string) error {
	if err := validateRuntimeDir(socketDir); err != nil {
		return err
	}
	binDir := filepath.Join(socketDir, "bin")
	if err := os.Mkdir(binDir, 0700); err != nil && !os.IsExist(err) {
		return fmt.Errorf("cannot create adapter scripts directory")
	}
	if err := validateOwnedDir(binDir, 0700); err != nil {
		return err
	}
	for name, content := range adapterScripts() {
		path := filepath.Join(binDir, name)
		if err := writePrivateScript(path, []byte(content)); err != nil {
			return fmt.Errorf("cannot install adapter script %s", name)
		}
	}
	if err := writePrivateFile(filepath.Join(socketDir, "gitconfig"), []byte(adapterGitConfig), 0600); err != nil {
		return fmt.Errorf("cannot install adapter Git config")
	}
	return nil
}

// AdapterInstallCommand returns the command run in the container after startup.
func AdapterInstallCommand() []string {
	return []string{"/bin/sh", "-c", adapterInstallScript}
}

func validateRuntimeDir(path string) error {
	if !filepath.IsAbs(path) || path == "/" || strings.ContainsAny(path, ",\r\n") || strings.Contains(path, "${") {
		return fmt.Errorf("invalid GitHub App runtime directory")
	}
	return validateOwnedDir(path, 0700)
}

func validateOwnedDir(path string, mode fs.FileMode) error {
	info, err := os.Lstat(path)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || !ownedByCurrentUser(info) || info.Mode().Perm() != mode.Perm() {
		return fmt.Errorf("GitHub App adapter directory is unsafe")
	}
	return nil
}

func writePrivateScript(path string, content []byte) error {
	return writePrivateFile(path, content, 0700)
}

func writePrivateFile(path string, content []byte, mode fs.FileMode) error {
	if info, err := os.Lstat(path); err == nil {
		if !info.Mode().IsRegular() || !ownedByCurrentUser(info) || info.Mode().Perm() != mode.Perm() {
			return fmt.Errorf("unsafe existing adapter script")
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".dch-adapter-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if err := tmp.Chmod(mode); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(content); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}

func adapterScripts() map[string]string {
	return map[string]string{
		"git-credential-dch-github": gitCredentialScript,
		"gh":                        ghWrapperScript,
	}
}

const gitCredentialScript = `#!/bin/sh
set -eu
operation=${1-}
[ "$operation" = get ] || exit 0
protocol=
host=
while IFS= read -r line; do
	case "$line" in
		protocol=*) protocol=${line#protocol=} ;;
		host=*) host=${line#host=} ;;
	esac
done
[ "$protocol" = https ] && [ "$host" = github.com ] || exit 0
token=$(curl --disable --noproxy '*' --unix-socket /run/dch-github/credential.sock --fail --silent --show-error --max-time 125 http://localhost/credential) || { echo 'dch: credential unavailable; on a headless host, reconfigure GitHub authorization in host Project Plugins or unlock the host GPG agent in a terminal and retry' >&2; exit 1; }
[ -n "$token" ] || exit 1
printf 'username=x-access-token\npassword=%s\n\n' "$token"
`

const ghWrapperScript = `#!/bin/sh
set -eu
adapter_bin=/run/dch-github/bin
rest=${PATH-}
clean_path=
while :; do
	case "$rest" in
		*:*) part=${rest%%:*}; rest=${rest#*:}; more=yes ;;
		*) part=$rest; rest=; more=no ;;
	esac
	if [ "$part" != "$adapter_bin" ]; then
		if [ -n "$clean_path" ]; then clean_path=$clean_path:$part; else clean_path=$part; fi
	fi
	[ "$more" = yes ] || break
done
PATH=$clean_path
export PATH
real_gh=$(command -v gh) || { echo 'dch: gh is required' >&2; exit 127; }
token=$(curl --disable --noproxy '*' --unix-socket /run/dch-github/credential.sock --fail --silent --show-error --max-time 125 http://localhost/credential) || { echo 'dch: GitHub credential unavailable; on a headless host, reconfigure GitHub authorization in host Project Plugins or unlock the host GPG agent in a terminal and retry' >&2; exit 1; }
[ -n "$token" ] || { echo 'dch: GitHub credential unavailable; on a headless host, reconfigure GitHub authorization in host Project Plugins or unlock the host GPG agent in a terminal and retry' >&2; exit 1; }
GH_TOKEN=$token
export GH_TOKEN
exec "$real_gh" "$@"
`

const adapterGitConfig = `[credential "https://github.com"]
	helper =
	helper = /run/dch-github/bin/git-credential-dch-github
`

const adapterInstallScript = `set -eu
adapter_bin=/run/dch-github/bin
rest=${PATH-}
clean_path=
while :; do
	case "$rest" in
		*:*) part=${rest%%:*}; rest=${rest#*:}; more=yes ;;
		*) part=$rest; rest=; more=no ;;
	esac
	if [ "$part" != "$adapter_bin" ]; then
		if [ -n "$clean_path" ]; then clean_path=$clean_path:$part; else clean_path=$part; fi
	fi
	[ "$more" = yes ] || break
done
PATH=$clean_path
export PATH
for dependency in git gh curl sh; do
	command -v "$dependency" >/dev/null 2>&1 || { echo "dch: required command missing: $dependency" >&2; exit 1; }
done
[ -x /run/dch-github ] || { echo 'dch: cannot access GitHub runtime directory; verify the bind mount and align the container UID with the host user' >&2; exit 1; }
[ -S /run/dch-github/credential.sock ] || { echo 'dch: credential socket unavailable; verify the runtime mount and align the container UID with the host user' >&2; exit 1; }
[ -w /run/dch-github/credential.sock ] || { echo 'dch: no permission to connect to the credential socket; align the container UID with the host user' >&2; exit 1; }
curl --disable --noproxy '*' --unix-socket /run/dch-github/credential.sock --fail --silent --show-error --max-time 20 http://localhost/health >/dev/null || { echo 'dch: credential service unavailable or socket permission denied; check the host service and align the container UID with the host user' >&2; exit 1; }
include=/run/dch-github/gitconfig
if ! git config --global --fixed-value --get-all include.path "$include" >/dev/null 2>&1; then
	git config --global --add include.path "$include"
fi
`
