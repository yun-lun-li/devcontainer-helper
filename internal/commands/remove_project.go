package commands

import "fmt"

func removeProjectRegistration(p Project) (bool, error) {
	confirmed, err := confirm("Remove "+p.Name+" from dch? Project files and containers will be kept.", false)
	if err != nil || !confirmed {
		return false, err
	}
	cfg, err := loadConfig()
	if err != nil {
		return false, err
	}
	for i, path := range cfg.Projects {
		if path == p.Path {
			cfg.Projects = append(cfg.Projects[:i], cfg.Projects[i+1:]...)
			if err := saveConfig(cfg); err != nil {
				return false, err
			}
			screenNotice("Project removed", "Registration removed. Project files and containers were kept.")
			return true, nil
		}
	}
	return false, fmt.Errorf("project %s is no longer registered", p.Path)
}
