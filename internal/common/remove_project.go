package common

import "fmt"

func RemoveProjectRegistration(path string) error {
	cfg, err := LoadConfig()
	if err != nil {
		return err
	}
	for i, registered := range cfg.Projects {
		if registered == path {
			projectPlugins := cfg.ProjectPlugins[path]
			for id := range projectPlugins.Enabled {
				if id != "github-app" {
					return fmt.Errorf("unknown enabled plugin %q", id)
				}
			}
			for id := range projectPlugins.States {
				if id != "github-app" {
					return fmt.Errorf("unknown plugin state %q", id)
				}
			}
			cfg.Projects = append(cfg.Projects[:i], cfg.Projects[i+1:]...)
			delete(cfg.ProjectPlugins, path)
			return SaveConfig(cfg)
		}
	}
	return fmt.Errorf("project %s is no longer registered", path)
}
