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
	cfg, err := common.LoadConfig()
	if err != nil {
		return err
	}
	for _, existing := range cfg.Projects {
		if existing == path {
			fmt.Printf("Already registered %q (%s)\n", name, path)
			return nil
		}
	}
	cfg.Projects = append(cfg.Projects, path)
	if err := common.SaveConfig(cfg); err != nil {
		return err
	}
	fmt.Printf("Registered %q (%s)\n", name, path)
	return nil
}
