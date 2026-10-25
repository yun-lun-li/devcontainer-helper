package commands

import (
	"fmt"
	"strings"

	"dch/internal/common"
)

func registerProject(rawPath string) error {
	if strings.TrimSpace(rawPath) == "" {
		return fmt.Errorf("--path requires a project directory")
	}
	path, err := common.NormalizePath(rawPath)
	if err != nil {
		return err
	}
	name, err := common.ProjectName(path)
	if err != nil {
		return fmt.Errorf("cannot register %s: %w", path, err)
	}
	alreadyRegistered := false
	if err := common.UpdateConfig(func(cfg *common.Config) error {
		for _, existing := range cfg.Projects {
			if existing == path {
				alreadyRegistered = true
				return nil
			}
		}
		cfg.Projects = append(cfg.Projects, path)
		return nil
	}); err != nil {
		return err
	}
	if alreadyRegistered {
		fmt.Printf("Already registered %q (%s)\n", name, path)
		return nil
	}
	fmt.Printf("Registered %q (%s)\n", name, path)
	return nil
}
