package console

import (
	"errors"
	"fmt"
	"strings"

	"dch/internal/common"
)

type projectAction string

const (
	actionConnect projectAction = "Connect"
	actionStart   projectAction = "Start"
	actionStop    projectAction = "Stop"
	actionBuild   projectAction = "Build"
	actionRemove  projectAction = "Remove Project"
	actionBack    projectAction = "Back to projects"
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
		switch actions[choice] {
		case actionConnect:
			err = connectProject(p)
		case actionStart:
			err = startProject(p)
		case actionStop:
			err = stopProject(p)
		case actionBuild:
			err = buildProject(p)
		case actionRemove:
			err = common.RemoveProjectRegistration(p.Path)
			if err == nil {
				return nil
			}
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
		case actionStop:
			options[i] = "■ Stop  Stop the running container"
		case actionBuild:
			options[i] = "▣ Build  Rebuild container or image"
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
	return append(actions, actionRemove, actionBack)
}
