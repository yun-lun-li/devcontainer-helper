package commands

import "fmt"

func removeProjectRegistration(path string) error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	for i, registered := range cfg.Projects {
		if registered == path {
			cfg.Projects = append(cfg.Projects[:i], cfg.Projects[i+1:]...)
			return saveConfig(cfg)
		}
	}
	return fmt.Errorf("project %s is no longer registered", path)
}
