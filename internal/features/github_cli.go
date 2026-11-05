package features

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	featureassets "dch/.devcontainer/features"
	"dch/internal/common"
	"dch/internal/hostpass"
)

// GitHubCLIKey is the project-local path of the bundled Feature.
const GitHubCLIKey = "./.dch/features/github-cli"

// GitHubCLISettings returns whether the managed Feature is enabled and its pass entry.
func GitHubCLISettings(project string) (bool, string, error) {
	data, err := common.MergedDevcontainerConfig(project)
	if err != nil {
		return false, "", err
	}
	var config struct {
		Features map[string]json.RawMessage `json:"features"`
	}
	if err := json.Unmarshal(data, &config); err != nil {
		return false, "", err
	}
	if _, exists := config.Features["./features/github-cli"]; exists {
		return false, "", fmt.Errorf("GitHub CLI host pass is configured manually at ./features/github-cli; remove that entry before managing it here")
	}
	raw, enabled := config.Features[GitHubCLIKey]
	entry := "dch/github/pat"
	if enabled {
		var options struct {
			PassEntry string `json:"passEntry"`
		}
		if err := json.Unmarshal(raw, &options); err != nil {
			return false, "", err
		}
		if options.PassEntry != "" {
			entry = options.PassEntry
		}
	}
	return enabled, entry, nil
}

// EnableGitHubCLI installs the bundled assets and enables them in local settings.
func EnableGitHubCLI(project, entry string) error {
	if err := hostpass.ValidateEntry(entry); err != nil {
		return err
	}
	if _, _, err := GitHubCLISettings(project); err != nil {
		return err
	}
	dir := filepath.Join(project, ".devcontainer", ".dch", "features", "github-cli")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	files, err := featureassets.GitHubCLI.ReadDir("github-cli")
	if err != nil {
		return err
	}
	for _, file := range files {
		data, err := featureassets.GitHubCLI.ReadFile("github-cli/" + file.Name())
		if err != nil {
			return err
		}
		if err := writeBundledFeatureFile(filepath.Join(dir, file.Name()), data); err != nil {
			return err
		}
	}
	return common.SetLocalFeature(project, GitHubCLIKey, map[string]any{"passEntry": entry})
}

func writeBundledFeatureFile(path string, data []byte) error {
	file, err := os.CreateTemp(filepath.Dir(path), ".dch-feature-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if _, err := file.Write(data); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	if err := os.Chmod(file.Name(), 0755); err != nil {
		return err
	}
	return os.Rename(file.Name(), path)
}

// DisableGitHubCLI removes the managed local setting, retaining assets for existing containers.
func DisableGitHubCLI(project string) error {
	return common.SetLocalFeature(project, GitHubCLIKey, nil)
}
