package commands

import (
	"errors"
	"fmt"
	"strings"
)

type projectAction string

const (
	actionConnect    projectAction = "Connect"
	actionStart      projectAction = "Start"
	actionStopDelete projectAction = "Stop & Delete"
	actionBuild      projectAction = "Build"
	actionBack       projectAction = "Back to projects"
)

func projectsMenu() error {
	for {
		projects, err := loadProjects()
		if err != nil {
			return err
		}
		if len(projects) == 0 {
			screenNotice("Projects", "No registered projects. Add one in Manage registrations.")
			return nil
		}
		statusErr := updateStatuses(projects)
		options := make([]string, 0, len(projects)+1)
		for _, p := range projects {
			options = append(options, fmt.Sprintf("%s  [%s]  %s", p.Name, p.State, p.Path))
		}
		options = append(options, "Back to main menu")
		title := "Projects"
		if statusErr != nil {
			title += "  /  Docker unavailable"
		}
		choice, err := choose(title, options)
		if errors.Is(err, errCancelled) || choice == len(projects) {
			return nil
		}
		if err != nil {
			return err
		}
		if err := projectMenu(projects[choice].Path); err != nil {
			return err
		}
	}
}

func projectMenu(path string) error {
	for {
		projects, err := loadProjects()
		if err != nil {
			return err
		}
		var p Project
		found := false
		for _, candidate := range projects {
			if candidate.Path == path {
				p, found = candidate, true
				break
			}
		}
		if !found {
			return nil
		}
		current := []Project{p}
		statusErr := updateStatuses(current)
		p = current[0]
		actions := availableProjectActions(p, statusErr == nil)
		choice, err := chooseProject(p, actions)
		if errors.Is(err, errCancelled) {
			return nil
		}
		if err != nil {
			return err
		}
		switch actions[choice] {
		case actionConnect:
			err = connectProject(p)
		case actionStart:
			err = startProject(p)
		case actionStopDelete:
			err = stopDeleteProject(p)
		case actionBuild:
			err = buildProject(p)
		case actionBack:
			return nil
		}
		if err != nil && !errors.Is(err, errCancelled) {
			if screenActive {
				screenError(err)
			} else {
				return err
			}
		}
	}
}

func chooseProject(p Project, actions []projectAction) (int, error) {
	options := make([]string, len(actions))
	for i, action := range actions {
		switch action {
		case actionConnect:
			options[i] = "↗ Connect  Open a shell in this container"
		case actionStart:
			options[i] = "▶ Start  Create or start the container"
		case actionStopDelete:
			options[i] = "■ Stop & Delete  Stop and remove containers"
		case actionBuild:
			options[i] = "▣ Build  Rebuild container or image"
		case actionBack:
			options[i] = "← Back to projects"
		}
	}
	return chooseView(menuView{project: &p, options: options})
}

func availableProjectActions(p Project, dockerAvailable bool) []projectAction {
	actions := make([]projectAction, 0, 5)
	if p.Valid && dockerAvailable {
		switch {
		case strings.HasPrefix(p.State, "RUNNING"):
			actions = append(actions, actionConnect, actionStopDelete)
		case strings.HasPrefix(p.State, "STOPPED"), p.State == "NOT CREATED":
			actions = append(actions, actionStart)
		}
		actions = append(actions, actionBuild)
	}
	return append(actions, actionBack)
}
