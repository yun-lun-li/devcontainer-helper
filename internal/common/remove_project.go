package common

import "fmt"

func RemoveProjectRegistration(path string) error {
	cfg, err := LoadConfig()
	if err != nil {
		return err
	}
	for i, registered := range cfg.Projects {
		if registered == path {
			cfg.Projects = append(cfg.Projects[:i], cfg.Projects[i+1:]...)
			return SaveConfig(cfg)
		}
	}
	return fmt.Errorf("project %s is no longer registered", path)
}
