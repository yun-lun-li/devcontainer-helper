package console

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"dch/internal/common"
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
	config, err := common.MergedDevcontainerConfig(p.Path)
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

	args = append(args, "--config", mergedPath)
	if args[0] == "up" {
		basePath, err := filepath.Abs(filepath.Join(configDir, "devcontainer.json"))
		if err != nil {
			return fmt.Errorf("resolve devcontainer config path: %w", err)
		}
		workspacePath, err := filepath.Abs(p.Path)
		if err != nil {
			return fmt.Errorf("resolve workspace path: %w", err)
		}
		args = append(args,
			"--id-label", "devcontainer.local_folder="+workspacePath,
			"--id-label", "devcontainer.config_file="+basePath,
		)
	}
	return invokeDevcontainerWithCleanup(func() { _ = os.RemoveAll(tempDir) }, args...)
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
