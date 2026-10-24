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
	title := "devcontainer " + args[0]
	command := "$ devcontainer " + strings.Join(args, " ") + "\n\n"
	cmd := exec.Command("devcontainer", args...)
	cmd.Stdin = os.Stdin
	if args[0] == "exec" || !screenActive {
		screenHeading(title)
		fmt.Print(command)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		err := cmd.Run()
		screenWait()
		if err != nil {
			return fmt.Errorf("devcontainer command failed: %w", err)
		}
		return nil
	}
	output, err := cmd.CombinedOutput()
	showLogs(title, append([]byte(command), output...))
	if err != nil {
		return fmt.Errorf("devcontainer command failed: %w", err)
	}
	return nil
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
	// The Dev Container CLI only accepts configuration files with one of these
	// two basenames. Keep the merged file beside the source config so relative
	// paths in the configuration retain their existing meaning.
	mergedPath := filepath.Join(configDir, ".devcontainer.json")
	mergedFile, err := os.OpenFile(mergedPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("create temporary merged devcontainer config: %w", err)
	}
	defer os.Remove(mergedPath)
	if _, err := mergedFile.Write(config); err != nil {
		mergedFile.Close()
		return fmt.Errorf("write temporary merged devcontainer config: %w", err)
	}
	if err := mergedFile.Close(); err != nil {
		return fmt.Errorf("close temporary merged devcontainer config: %w", err)
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
	return invokeDevcontainer(args...)
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
