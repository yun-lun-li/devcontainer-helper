package common

import "fmt"

// ValidateProjectPlugins rejects plugin metadata that this version cannot manage.
func ValidateProjectPlugins(project ProjectPlugins, knownIDs map[string]struct{}) error {
	for id := range project.Enabled {
		if _, ok := knownIDs[id]; !ok {
			return fmt.Errorf("unknown enabled plugin %q", id)
		}
	}
	for id := range project.States {
		if _, ok := knownIDs[id]; !ok {
			return fmt.Errorf("unknown plugin state %q", id)
		}
	}
	return nil
}

func RemoveProjectRegistration(path string) error {
	cfg, err := LoadConfig()
	if err != nil {
		return err
	}
	for i, registered := range cfg.Projects {
		if registered == path {
			projectPlugins := cfg.ProjectPlugins[path]
			if err := ValidateProjectPlugins(projectPlugins, map[string]struct{}{"github-app": {}}); err != nil {
				return err
			}
			cfg.Projects = append(cfg.Projects[:i], cfg.Projects[i+1:]...)
			delete(cfg.ProjectPlugins, path)
			return SaveConfig(cfg)
		}
	}
	return fmt.Errorf("project %s is no longer registered", path)
}
