package console

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path"
	"strings"
	"time"

	"dch/internal/githubapp"
	"dch/internal/plugins"
)

func prepareProjectPlugin(p Project) error {
	manager := plugins.NewManager()
	if err := requireLocalDocker(); err != nil {
		return err
	}
	if err := manager.Ensure(context.Background(), p.Path); err != nil {
		return fmt.Errorf("prepare GitHub App credential service: %w", err)
	}
	return nil
}

func requireLocalDocker() error {
	contextName := os.Getenv("DOCKER_CONTEXT")
	if host := os.Getenv("DOCKER_HOST"); host != "" && contextName == "" {
		endpoint, err := url.Parse(host)
		if err != nil || endpoint.Scheme != "unix" {
			return fmt.Errorf("GitHub App projects require a local Docker Unix socket")
		}
		return nil
	}
	args := []string{"context", "inspect"}
	if contextName != "" {
		args = append(args, contextName)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "docker", args...)
	var output boundedOutput
	cmd.Stdout = &output
	if err := cmd.Run(); err != nil || output.overflow {
		return fmt.Errorf("cannot verify local Docker context")
	}
	data := output.Bytes()
	var contexts []struct {
		Endpoints map[string]struct {
			Host string `json:"Host"`
		} `json:"Endpoints"`
	}
	if json.Unmarshal(data, &contexts) != nil || len(contexts) == 0 {
		return fmt.Errorf("cannot verify local Docker context")
	}
	for _, dockerContext := range contexts {
		endpoint := dockerContext.Endpoints["docker"]
		parsed, err := url.Parse(endpoint.Host)
		if err != nil || parsed.Scheme != "unix" {
			return fmt.Errorf("GitHub App projects require a local Docker Unix socket")
		}
	}
	return nil
}

type adapterSettings struct {
	Mounts    []string          `json:"mounts"`
	RemoteEnv map[string]string `json:"remoteEnv"`
}

func validateGitHubAppConfig(data []byte, socketDir string) error {
	required, err := requiredAdapterSettings(socketDir)
	if err != nil {
		return err
	}
	var config map[string]json.RawMessage
	if err := json.Unmarshal(data, &config); err != nil {
		return fmt.Errorf("cannot validate GitHub App adapter settings")
	}
	if _, exists := config["dockerComposeFile"]; exists {
		return fmt.Errorf("GitHub App plugin does not support Docker Compose")
	}
	if err := validateAdapterMounts(config["mounts"], required.Mounts[0]); err != nil {
		return err
	}
	if !hasAdapterPath(config["remoteEnv"], required.RemoteEnv["PATH"]) {
		return fmt.Errorf("GitHub App adapter PATH was removed or overridden")
	}
	return nil
}

func requiredAdapterSettings(socketDir string) (adapterSettings, error) {
	var required adapterSettings
	expected, err := githubapp.AdapterFragment(socketDir)
	if err != nil {
		return required, err
	}
	if err := json.Unmarshal(expected, &required); err != nil {
		return required, fmt.Errorf("cannot validate GitHub App adapter settings")
	}
	return required, nil
}

func validateAdapterMounts(raw json.RawMessage, expectedMount string) error {
	var mounts []json.RawMessage
	if len(raw) == 0 || json.Unmarshal(raw, &mounts) != nil {
		return fmt.Errorf("GitHub App socket mount was removed or overridden")
	}
	foundMount := false
	for _, raw := range mounts {
		valid, exact := inspectAdapterMount(raw, expectedMount)
		if !valid {
			return fmt.Errorf("GitHub App socket mount was overridden")
		}
		foundMount = foundMount || exact
	}
	if !foundMount {
		return fmt.Errorf("GitHub App socket mount was removed or overridden")
	}
	return nil
}

func inspectAdapterMount(raw json.RawMessage, expected string) (bool, bool) {
	var mountString string
	if json.Unmarshal(raw, &mountString) == nil {
		return validAdapterMountString(mountString, expected)
	}
	var mountObject map[string]any
	if json.Unmarshal(raw, &mountObject) != nil {
		return true, false
	}
	for _, key := range []string{"target", "dst", "destination"} {
		if target, ok := mountObject[key].(string); ok && overlapsAdapterMount(target) {
			return false, false
		}
	}
	return true, false
}

func validAdapterMountString(mount, expected string) (bool, bool) {
	if mount == expected {
		return true, true
	}
	for _, field := range strings.Split(mount, ",") {
		for _, key := range []string{"target", "dst", "destination"} {
			if strings.HasPrefix(field, key+"=") && overlapsAdapterMount(strings.TrimPrefix(field, key+"=")) {
				return false, false
			}
		}
	}
	return true, false
}

func hasAdapterPath(raw json.RawMessage, expected string) bool {
	var remoteEnv map[string]any
	return len(raw) > 0 && json.Unmarshal(raw, &remoteEnv) == nil && remoteEnv["PATH"] == expected
}

func overlapsAdapterMount(target string) bool {
	target = path.Clean(target)
	return target == "/run/dch-github" || strings.HasPrefix(target, "/run/dch-github/")
}

type boundedOutput struct {
	data     []byte
	overflow bool
}

func (output *boundedOutput) Write(data []byte) (int, error) {
	const max = 64 * 1024
	if len(output.data)+len(data) > max {
		output.overflow = true
		return 0, fmt.Errorf("command output too large")
	}
	output.data = append(output.data, data...)
	return len(data), nil
}
func (output *boundedOutput) Bytes() []byte { return output.data }
