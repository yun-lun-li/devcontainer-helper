package commands

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

func configure() error {
	for {
		choice, err := choose("Manage projects", []string{"View projects", "Add project", "Remove project", "Back to main menu"})
		if err != nil {
			return err
		}
		switch choice {
		case 0:
			if err := list(); err != nil {
				fmt.Fprintln(os.Stderr, "Error:", err)
			}
		case 1:
			cwd, _ := os.Getwd()
			fmt.Printf("Project directory [Enter = %s]: ", cwd)
			line, err := input.ReadString('\n')
			if err != nil && !errors.Is(err, io.EOF) {
				return err
			}
			path, err := normalizePath(strings.TrimSpace(line))
			if err != nil {
				return err
			}
			name, err := projectName(path)
			if err != nil {
				fmt.Fprintln(os.Stderr, "Cannot register:", err)
				continue
			}
			cfg, err := loadConfig()
			if err != nil {
				return err
			}
			found := false
			for _, p := range cfg.Projects {
				if p == path {
					found = true
					break
				}
			}
			if found {
				fmt.Println("Already registered:", path)
				continue
			}
			cfg.Projects = append(cfg.Projects, path)
			if err := saveConfig(cfg); err != nil {
				return err
			}
			fmt.Printf("Registered %q (%s)\n", name, path)
		case 2:
			cfg, err := loadConfig()
			if err != nil {
				return err
			}
			if len(cfg.Projects) == 0 {
				fmt.Println("No registered projects.")
				continue
			}
			opts := make([]string, 0, len(cfg.Projects))
			for _, p := range cfg.Projects {
				name, err := projectName(p)
				if err != nil {
					name = "[missing/invalid]"
				}
				opts = append(opts, fmt.Sprintf("%s — %s", name, p))
			}
			index, err := choose("Remove a registration (not the project itself)", opts)
			if errors.Is(err, errCancelled) {
				continue
			}
			if err != nil {
				return err
			}
			var confirmed bool
			confirmed, err = confirm("Remove "+cfg.Projects[index]+" from dch?", false)
			if err != nil {
				return err
			}
			if !confirmed {
				continue
			}
			cfg.Projects = append(cfg.Projects[:index], cfg.Projects[index+1:]...)
			if err := saveConfig(cfg); err != nil {
				return err
			}
			fmt.Println("Removed registration. No project files or containers were deleted.")
		case 3:
			return nil
		}
	}
}
