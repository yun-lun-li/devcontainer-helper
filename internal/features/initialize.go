// Package features runs dch host initialization hooks contributed by Dev Container Features.
package features

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strings"

	"dch/internal/common"
)

type resolvedFeature struct {
	ID        string          `json:"id"`
	CachePath string          `json:"cachePath"`
	Value     json.RawMessage `json:"value"`
	Options   map[string]struct {
		Default json.RawMessage `json:"default"`
	} `json:"options"`
	Customizations struct {
		DCH struct {
			InitializeCommand json.RawMessage `json:"initializeCommand"`
		} `json:"dch"`
	} `json:"customizations"`
}

type resolvedConfiguration struct {
	FeaturesConfiguration *struct {
		FeatureSets []struct {
			Features []resolvedFeature `json:"features"`
		} `json:"featureSets"`
	} `json:"featuresConfiguration"`
}

// Initialize resolves Features through the Dev Container CLI and runs their host
// hooks in installation order. It leaves the project's initializeCommand to the CLI.
func Initialize(ctx context.Context, workspace, configPath string, stdin io.Reader, stdout, stderr io.Writer) error {
	cmd := exec.CommandContext(ctx, "devcontainer", "read-configuration", "--workspace-folder", workspace, "--config", configPath, "--include-features-configuration")
	cmd.Dir, cmd.Stderr = workspace, stderr
	var output limitedBuffer
	cmd.Stdout = &output
	if err := common.RunHostCommand(cmd); err != nil {
		return fmt.Errorf("resolve Feature host hooks: %w", err)
	}
	var resolved resolvedConfiguration
	if err := json.Unmarshal(output.data, &resolved); err != nil {
		return fmt.Errorf("decode Feature configuration: %w", err)
	}
	if resolved.FeaturesConfiguration == nil {
		return fmt.Errorf("Dev Container CLI did not return featuresConfiguration")
	}
	for _, set := range resolved.FeaturesConfiguration.FeatureSets {
		for _, feature := range set.Features {
			if err := initializeFeature(ctx, workspace, feature, stdin, stdout, stderr); err != nil {
				return fmt.Errorf("Feature %q host initialization: %w", feature.ID, err)
			}
		}
	}
	return nil
}

func initializeFeature(ctx context.Context, workspace string, feature resolvedFeature, stdin io.Reader, stdout, stderr io.Writer) error {
	raw := feature.Customizations.DCH.InitializeCommand
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	env, err := featureEnvironment(workspace, feature)
	if err != nil {
		return err
	}
	return runHook(ctx, workspace, raw, env, stdin, stdout, stderr)
}

var optionName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

func featureEnvironment(workspace string, feature resolvedFeature) ([]string, error) {
	executable, err := os.Executable()
	if err != nil {
		return nil, err
	}
	values := map[string]json.RawMessage{}
	for name, option := range feature.Options {
		values[name] = option.Default
	}
	if len(feature.Value) > 0 && string(feature.Value) != "true" {
		overrides, err := optionOverrides(feature.Value)
		if err != nil {
			return nil, err
		}
		for name, value := range overrides {
			values[name] = value
		}
	}
	env := cleanEnvironment()
	env = append(env, "DCH_FEATURE_DIR="+feature.CachePath, "DCH_WORKSPACE_FOLDER="+workspace, "DCH_EXECUTABLE="+executable)
	names := make([]string, 0, len(values))
	for name := range values {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if !optionName.MatchString(name) {
			return nil, fmt.Errorf("invalid Feature option name %q", name)
		}
		value, err := optionText(values[name])
		if err != nil {
			return nil, fmt.Errorf("Feature option %q: %w", name, err)
		}
		env = append(env, "DCH_FEATURE_OPTION_"+strings.ToUpper(name)+"="+value)
	}
	return env, nil
}

func optionOverrides(raw json.RawMessage) (map[string]json.RawMessage, error) {
	var version string
	if err := json.Unmarshal(raw, &version); err == nil {
		return map[string]json.RawMessage{"version": raw}, nil
	}
	var values map[string]json.RawMessage
	if err := json.Unmarshal(raw, &values); err != nil {
		return nil, fmt.Errorf("Feature options must be an object or version string: %w", err)
	}
	return values, nil
}

func cleanEnvironment() []string {
	var env []string
	for _, value := range os.Environ() {
		if !strings.HasPrefix(value, "DCH_FEATURE_") && !strings.HasPrefix(value, "DCH_WORKSPACE_FOLDER=") && !strings.HasPrefix(value, "DCH_EXECUTABLE=") {
			env = append(env, value)
		}
	}
	return env
}

func optionText(raw json.RawMessage) (string, error) {
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", err
	}
	switch value := value.(type) {
	case string:
		return value, nil
	case bool:
		return fmt.Sprint(value), nil
	default:
		return "", fmt.Errorf("expected a string or boolean")
	}
}

func runHook(ctx context.Context, workspace string, raw json.RawMessage, env []string, stdin io.Reader, stdout, stderr io.Writer) error {
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return err
	}
	switch value := value.(type) {
	case string:
		return runCommand(ctx, workspace, []string{"/bin/sh", "-c", value}, env, stdin, stdout, stderr)
	case []any:
		args, err := commandArguments(value)
		if err != nil {
			return err
		}
		return runCommand(ctx, workspace, args, env, stdin, stdout, stderr)
	case map[string]any:
		return runParallel(ctx, workspace, value, env, stdout, stderr)
	default:
		return fmt.Errorf("initializeCommand must be a string, argument array, or object")
	}
}

func commandArguments(values []any) ([]string, error) {
	if len(values) == 0 {
		return nil, fmt.Errorf("empty command array")
	}
	args := make([]string, len(values))
	for i, value := range values {
		text, ok := value.(string)
		if !ok {
			return nil, fmt.Errorf("command arguments must be strings")
		}
		args[i] = text
	}
	return args, nil
}

func runCommand(ctx context.Context, workspace string, args, env []string, stdin io.Reader, stdout, stderr io.Writer) error {
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	cmd.Dir, cmd.Env, cmd.Stdin, cmd.Stdout, cmd.Stderr = workspace, env, stdin, stdout, stderr
	return common.RunHostCommand(cmd)
}

type limitedBuffer struct{ data []byte }

func (buffer *limitedBuffer) Write(data []byte) (int, error) {
	if len(buffer.data)+len(data) > 8<<20 {
		return 0, fmt.Errorf("Feature configuration exceeds 8 MiB")
	}
	buffer.data = append(buffer.data, data...)
	return len(data), nil
}
