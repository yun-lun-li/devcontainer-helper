package console

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"dch/internal/common"
	"dch/internal/githubapp"
	"dch/internal/plugins"
)

func requireExecutable(name string) error {
	if _, err := exec.LookPath(name); err != nil {
		return fmt.Errorf("%s not found in PATH; install it first", name)
	}
	return nil
}

func invokeDevcontainer(args ...string) error {
	return invokeDevcontainerWithCleanup(nil, args...)
}

func invokeDevcontainerCommand(cleanup func(), args ...string) error {
	action := args[0]
	if action == "up" && hasArgument(args, "--remove-existing-container") {
		action = "build"
	}
	title := "devcontainer " + action
	command := "$ devcontainer " + strings.Join(args, " ") + "\n\n"
	cmd := exec.Command("devcontainer", args...)
	cmd.Stdin = os.Stdin
	if args[0] == "exec" || !screenActive {
		screenHeading(title)
		fmt.Print(command)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		err := cmd.Run()
		if cleanup != nil {
			cleanup()
		}
		screenWait()
		if err != nil {
			return fmt.Errorf("devcontainer command failed: %w", err)
		}
		return nil
	}
	err := runDevcontainerWithLogs(cmd, title, command, cleanup)
	if err != nil {
		return fmt.Errorf("devcontainer command failed: %w", err)
	}
	return nil
}

func hasArgument(args []string, want string) bool {
	for _, arg := range args {
		if arg == want {
			return true
		}
	}
	return false
}

func invokeProjectDevcontainer(p Project, args ...string) error {
	return invokeProjectDevcontainerMode(p, false, args...)
}

func invokeProjectDevcontainerQuiet(p Project, args ...string) error {
	return invokeProjectDevcontainerMode(p, true, args...)
}

func invokeProjectDevcontainerMode(p Project, quiet bool, args ...string) error {
	config, err := prepareMergedProjectConfig(p)
	if err != nil {
		return err
	}
	if len(args) > 0 && args[0] == "up" {
		config, err = withStableContainerName(config, p)
		if err != nil {
			return err
		}
	}
	configDir := filepath.Join(p.Path, ".devcontainer")
	tempDir, err := os.MkdirTemp("", "dch-devcontainer-")
	if err != nil {
		return fmt.Errorf("create temporary devcontainer directory: %w", err)
	}
	defer os.RemoveAll(tempDir)
	config, err = rebaseConfigPaths(config, configDir, tempDir)
	if err != nil {
		return err
	}
	mergedPath := filepath.Join(tempDir, "devcontainer.json")
	if err := os.WriteFile(mergedPath, config, 0o600); err != nil {
		return fmt.Errorf("write temporary merged devcontainer config: %w", err)
	}

	args, err = appendDevcontainerConfigArgs(args, p, configDir, mergedPath)
	if err != nil {
		return err
	}
	cleanup := func() { _ = os.RemoveAll(tempDir) }
	if quiet {
		err = invokeDevcontainerQuiet(args...)
		cleanup()
		return err
	}
	return invokeDevcontainerWithCleanup(cleanup, args...)
}

func prepareMergedProjectConfig(p Project) ([]byte, error) {
	config, err := common.MergedDevcontainerConfig(p.Path)
	if err != nil {
		return nil, err
	}
	status, err := plugins.NewManager().Status(context.Background(), p.Path)
	if err != nil {
		return nil, err
	}
	if !status.Enabled {
		return config, nil
	}
	runtimeDir, err := githubapp.RuntimeDir(p.Path)
	if err != nil {
		return nil, err
	}
	if err := validateGitHubAppConfig(config, runtimeDir); err != nil {
		return nil, err
	}
	return config, nil
}

func appendDevcontainerConfigArgs(args []string, p Project, configDir, mergedPath string) ([]string, error) {
	extraArgs := []string{"--config", mergedPath}
	if len(args) == 0 {
		return append(args, extraArgs...), nil
	}
	if args[0] != "up" && args[0] != "exec" {
		return append(args, extraArgs...), nil
	}
	basePath, err := filepath.Abs(filepath.Join(configDir, "devcontainer.json"))
	if err != nil {
		return nil, fmt.Errorf("resolve devcontainer config path: %w", err)
	}
	workspacePath, err := filepath.Abs(p.Path)
	if err != nil {
		return nil, fmt.Errorf("resolve workspace path: %w", err)
	}
	extraArgs = append(extraArgs, "--id-label", "devcontainer.local_folder="+workspacePath, "--id-label", "devcontainer.config_file="+basePath)
	if args[0] == "exec" && len(args) >= 4 && args[1] == "--workspace-folder" {
		return append(args[:3], append(extraArgs, args[3:]...)...), nil
	}
	return append(args, extraArgs...), nil
}

func invokeDevcontainerQuiet(args ...string) error {
	if len(args) == 0 {
		return fmt.Errorf("devcontainer command is required")
	}
	screenHeading("Configure GitHub App authentication")
	cmd := exec.Command("devcontainer", args...)
	var output diagnosticCapture
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, &output, &output
	if err := cmd.Run(); err != nil {
		detail := strings.TrimSpace(output.String())
		if output.truncated {
			detail += " (diagnostic output truncated)"
		}
		if detail != "" {
			return fmt.Errorf("GitHub App container setup failed: %s", detail)
		}
		return fmt.Errorf("GitHub App container setup failed: %w", err)
	}
	return nil
}

type diagnosticCapture struct {
	mu        sync.Mutex
	buffer    bytes.Buffer
	truncated bool
}

func (capture *diagnosticCapture) Write(data []byte) (int, error) {
	capture.mu.Lock()
	defer capture.mu.Unlock()
	const maxDiagnosticBytes = 4096
	remaining := maxDiagnosticBytes - capture.buffer.Len()
	if remaining > 0 {
		if len(data) > remaining {
			_, _ = capture.buffer.Write(data[:remaining])
			capture.truncated = true
		} else {
			_, _ = capture.buffer.Write(data)
		}
	} else {
		capture.truncated = true
	}
	return len(data), nil
}
func (capture *diagnosticCapture) String() string {
	capture.mu.Lock()
	defer capture.mu.Unlock()
	return capture.buffer.String()
}

func invokeDevcontainerWithCleanup(cleanup func(), args ...string) error {
	return invokeDevcontainerCommand(cleanup, args...)
}

func rebaseConfigPaths(config []byte, sourceDir, targetDir string) ([]byte, error) {
	var settings map[string]any
	if err := json.Unmarshal(config, &settings); err != nil {
		return nil, fmt.Errorf("decode merged devcontainer config: %w", err)
	}
	rebase := func(path string) string {
		if path == "" || filepath.IsAbs(path) || strings.Contains(path, "${") || strings.Contains(path, "$LOCAL_") {
			return path
		}
		absolute := filepath.Join(sourceDir, filepath.FromSlash(path))
		relative, err := filepath.Rel(targetDir, absolute)
		if err != nil {
			return path
		}
		return filepath.ToSlash(relative)
	}
	if build, ok := settings["build"].(map[string]any); ok {
		for _, key := range []string{"dockerfile", "context"} {
			if path, ok := build[key].(string); ok {
				build[key] = rebase(path)
			}
		}
	}
	switch compose := settings["dockerComposeFile"].(type) {
	case string:
		settings["dockerComposeFile"] = rebase(compose)
	case []any:
		for i, value := range compose {
			if path, ok := value.(string); ok {
				compose[i] = rebase(path)
			}
		}
	}
	if features, ok := settings["features"].(map[string]any); ok {
		rebased := make(map[string]any, len(features))
		for feature, options := range features {
			if strings.HasPrefix(feature, "./") || strings.HasPrefix(feature, "../") {
				feature = rebase(feature)
			}
			rebased[feature] = options
		}
		settings["features"] = rebased
	}
	updated, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode merged devcontainer config: %w", err)
	}
	return updated, nil
}

func withStableContainerName(config []byte, p Project) ([]byte, error) {
	var settings map[string]json.RawMessage
	if err := json.Unmarshal(config, &settings); err != nil {
		return nil, fmt.Errorf("decode merged devcontainer config: %w", err)
	}

	var runArgs []string
	if raw, ok := settings["runArgs"]; ok {
		if err := json.Unmarshal(raw, &runArgs); err != nil {
			return nil, fmt.Errorf("decode devcontainer runArgs: %w", err)
		}
	}
	for _, arg := range runArgs {
		if arg == "--name" || strings.HasPrefix(arg, "--name=") {
			return config, nil
		}
	}

	// Use Docker's runArgs because the Dev Container CLI has no released
	// devcontainer.json property for setting a container name.
	runArgs = append(runArgs, "--name", stableContainerName(p))
	encodedRunArgs, err := json.Marshal(runArgs)
	if err != nil {
		return nil, fmt.Errorf("encode devcontainer runArgs: %w", err)
	}
	settings["runArgs"] = encodedRunArgs
	updated, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode merged devcontainer config: %w", err)
	}
	return updated, nil
}

func stableContainerName(p Project) string {
	path, err := filepath.Abs(p.Path)
	if err != nil {
		path = p.Path
	}
	hash := sha256.Sum256([]byte(filepath.Clean(path)))

	var slug strings.Builder
	lastHyphen := false
	for _, r := range strings.ToLower(p.Name) {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '_' || r == '.' {
			slug.WriteRune(r)
			lastHyphen = false
		} else if !lastHyphen {
			slug.WriteByte('-')
			lastHyphen = true
		}
	}
	base := strings.Trim(slug.String(), "-_.")
	if base == "" {
		base = "project"
	}
	return base + "-" + hex.EncodeToString(hash[:4])
}
