package commands

import (
	"fmt"
	"strings"
)

func registerProject(rawPath string) error {
	if strings.TrimSpace(rawPath) == "" {
		return fmt.Errorf("--path requires a project directory")
	}
	path, err := normalizePath(rawPath)
	if err != nil {
		return err
	}
	name, err := projectName(path)
	if err != nil {
		return fmt.Errorf("cannot register %s: %w", path, err)
	}
	cfg, err := loadConfig()
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
	if err := saveConfig(cfg); err != nil {
		return err
	}
	fmt.Printf("Registered %q (%s)\n", name, path)
	return nil
}
