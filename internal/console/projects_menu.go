package console

import (
	"errors"
	"fmt"
	"strings"

	"dch/internal/common"
)

type projectAction string

const (
	actionConnect  projectAction = "Connect"
	actionStart    projectAction = "Start"
	actionStop     projectAction = "Stop"
	actionBuild    projectAction = "Build"
	actionConfig   projectAction = "Show config"
	actionFeatures projectAction = "Features"
	actionRemove   projectAction = "Remove Project"
	actionBack     projectAction = "Back to projects"
)

func projectsMenu() error {
	for {
		projects, err := common.LoadProjects()
		if err != nil {
			return err
		}
		if len(projects) == 0 {
			_, err := choose("Projects\nNo registered projects. Run dch register --path PROJECT_PATH first.", []string{"Quit console"})
			if errors.Is(err, errCancelled) {
				return nil
			}
			return err
		}
		statusErr := common.UpdateStatuses(projects)
		options := make([]string, 0, len(projects)+1)
		for _, p := range projects {
			options = append(options, fmt.Sprintf("%s  [%s]  %s", p.Name, p.State, p.Path))
		}
		options = append(options, "Quit console")
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
		projects, err := common.LoadProjects()
		if err != nil {
			return err
		}
		p, found := findProject(projects, path)
		if !found {
			return nil
		}
		current := []Project{p}
		statusErr := common.UpdateStatuses(current)
		p = current[0]
		actions := availableProjectActions(p, statusErr == nil)
		choice, err := chooseProject(p, actions)
		if errors.Is(err, errCancelled) {
			return nil
		}
		if err != nil {
			return err
		}
		back, err := performProjectAction(p, actions[choice])
		if back {
			return nil
		}
		if err := reportProjectActionError(err); err != nil {
			return err
		}
	}
}

func reportProjectActionError(err error) error {
	if err == nil || errors.Is(err, errCancelled) {
		return nil
	}
	if !screenActive {
		return err
	}
	screenError(err)
	return nil
}

func findProject(projects []Project, path string) (Project, bool) {
	for _, project := range projects {
		if project.Path == path {
			return project, true
		}
	}
	return Project{}, false
}

func performProjectAction(p Project, action projectAction) (bool, error) {
	var err error
	switch action {
	case actionConnect:
		err = connectProject(p)
	case actionStart:
		err = startProject(p)
	case actionStop:
		err = stopProject(p)
	case actionBuild:
		err = buildProject(p)
	case actionConfig:
		err = showProjectConfig(p)
	case actionFeatures:
		err = projectFeatures(p)
	case actionRemove:
		return removeProjectFromMenu(p)
	case actionBack:
		return true, nil
	}
	return false, err
}

func removeProjectFromMenu(p Project) (bool, error) {
	choice, err := choose("Remove project?", []string{
		"Remove project registration  Keep files and containers", "Cancel",
	})
	if err != nil || choice != 0 {
		return false, err
	}
	err = common.RemoveProjectRegistration(p.Path)
	return err == nil, err
}

func chooseProject(p Project, actions []projectAction) (int, error) {
	options := make([]string, len(actions))
	for i, action := range actions {
		switch action {
		case actionConnect:
			options[i] = "↗ Connect  Open a shell in this container"
		case actionStart:
			options[i] = "▶ Start  Create or start the container"
		case actionStop:
			options[i] = "■ Stop  Stop the running container"
		case actionBuild:
			options[i] = "▣ Build  Rebuild container or image"
		case actionConfig:
			options[i] = "⚙ Show config  Preview merged devcontainer settings"
		case actionFeatures:
			options[i] = "⚙ Features  Manage optional project Features"
		case actionRemove:
			options[i] = "✕ Remove Project  Unregister only; keep files and containers"
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
			actions = append(actions, actionConnect, actionStop)
		case strings.HasPrefix(p.State, "STOPPED"), p.State == "NOT CREATED":
			actions = append(actions, actionStart)
		}
		actions = append(actions, actionBuild)
	}
	if p.Valid {
		actions = append(actions, actionFeatures)
	}
	return append(actions, actionConfig, actionRemove, actionBack)
}
