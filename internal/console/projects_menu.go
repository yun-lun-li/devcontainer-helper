package console

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"dch/internal/common"
	"dch/internal/plugins"
)

type projectAction string

const (
	actionConnect projectAction = "Connect"
	actionStart   projectAction = "Start"
	actionStop    projectAction = "Stop"
	actionBuild   projectAction = "Build"
	actionPlugins projectAction = "Plugins"
	actionConfig  projectAction = "Show config"
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
			return chooseEmptyProjects()
		}
		statusErr := common.UpdateStatuses(projects)
		options, err := projectOptions(projects)
		if err != nil {
			return err
		}
		options = append(options, "Quit console")
		choice, err := chooseProjectList(options, statusErr)
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

func chooseEmptyProjects() error {
	_, err := choose("Projects\nNo registered projects. Run dch register --path PROJECT_PATH first.", []string{"Quit console"})
	if errors.Is(err, errCancelled) {
		return nil
	}
	return err
}

func chooseProjectList(options []string, statusErr error) (int, error) {
	title := "Projects"
	if statusErr != nil {
		title += "  /  Docker unavailable"
	}
	return choose(title, options)
}

func projectMenu(path string) error {
	for {
		p, found, statusErr, pluginStatus, err := loadProjectMenuState(path)
		if err != nil {
			return err
		}
		if !found {
			return nil
		}
		actions := availableProjectActions(p, statusErr == nil, pluginStatus.PendingBuild)
		choice, err := chooseProject(p, actions, pluginStatus.PendingBuild)
		if errors.Is(err, errCancelled) {
			return nil
		}
		if err != nil {
			return err
		}
		done, actionErr := performProjectMenuAction(actions[choice], p)
		if done {
			return nil
		}
		err = actionErr
		if errors.Is(err, errCancelled) {
			continue
		}
		if err := reportProjectActionError(err); err != nil {
			return err
		}
	}
}

func performProjectMenuAction(action projectAction, p Project) (bool, error) {
	if action == actionBack {
		return true, nil
	}
	if action == actionRemove {
		return confirmProjectRemoval(p)
	}
	return false, runProjectAction(action, p)
}

func loadProjectMenuState(path string) (Project, bool, error, plugins.Status, error) {
	projects, err := common.LoadProjects()
	if err != nil {
		return Project{}, false, nil, plugins.Status{}, err
	}
	p, found := findProject(projects, path)
	if !found {
		return Project{}, false, nil, plugins.Status{}, nil
	}
	current := []Project{p}
	statusErr := common.UpdateStatuses(current)
	p = current[0]
	pluginStatus, err := plugins.NewManager().Status(context.Background(), p.Path)
	return p, true, statusErr, pluginStatus, err
}

func reportProjectActionError(err error) error {
	if err == nil || errors.Is(err, errCancelled) {
		return nil
	}
	if screenActive {
		screenError(err)
		return nil
	}
	return err
}

func projectOptions(projects []Project) ([]string, error) {
	options := make([]string, 0, len(projects)+1)
	for _, project := range projects {
		status, err := plugins.NewManager().Status(context.Background(), project.Path)
		if err != nil {
			return nil, err
		}
		state := project.State
		if status.PendingBuild {
			state += " · Build required"
		}
		options = append(options, fmt.Sprintf("%s  [%s]  %s", project.Name, state, project.Path))
	}
	return options, nil
}

func findProject(projects []Project, path string) (Project, bool) {
	for _, project := range projects {
		if project.Path == path {
			return project, true
		}
	}
	return Project{}, false
}

func runProjectAction(action projectAction, p Project) error {
	switch action {
	case actionConnect:
		return connectProject(p)
	case actionStart:
		return startProject(p)
	case actionStop:
		return stopProject(p)
	case actionBuild:
		return buildProject(p)
	case actionPlugins:
		return pluginsMenu(p)
	case actionConfig:
		return showProjectConfig(p)
	}
	return nil
}

func confirmProjectRemoval(p Project) (bool, error) {
	choice, err := choose("Remove project?", []string{"Remove project registration  Keep files and containers", "Cancel"})
	if err != nil || choice != 0 {
		return false, err
	}
	err = removeProject(p)
	return err == nil, err
}

func removeProject(p Project) error {
	path, err := common.NormalizePath(p.Path)
	if err != nil {
		return err
	}
	config, err := common.LoadConfig()
	if err != nil {
		return err
	}
	projectPlugins := config.ProjectPlugins[path]
	if err := common.ValidateProjectPlugins(projectPlugins, map[string]struct{}{"github-app": {}}); err != nil {
		return err
	}
	manager := plugins.NewManager()
	status, err := manager.Status(context.Background(), path)
	if err != nil {
		return err
	}
	if status.Enabled {
		if err := manager.Disable(context.Background(), path); err != nil {
			return fmt.Errorf("disable project plugins before removal: %w", err)
		}
	}
	return common.RemoveProjectRegistration(path)
}

func chooseProject(p Project, actions []projectAction, pendingBuild bool) (int, error) {
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
			if pendingBuild {
				options[i] = "▣ Build required  Apply project plugin changes"
			}
		case actionPlugins:
			options[i] = "⚙ Plugins  Configure project plugins"
		case actionConfig:
			options[i] = "⚙ Show config  Preview merged devcontainer settings"
		case actionRemove:
			options[i] = "✕ Remove Project  Unregister only; keep files and containers"
		case actionBack:
			options[i] = "← Back to projects"
		}
	}
	return chooseView(menuView{project: &p, options: options})
}

func availableProjectActions(p Project, dockerAvailable, pendingBuild bool) []projectAction {
	actions := make([]projectAction, 0, 6)
	if p.Valid && dockerAvailable {
		switch {
		case strings.HasPrefix(p.State, "RUNNING"):
			if !pendingBuild {
				actions = append(actions, actionConnect)
			}
			actions = append(actions, actionStop)
		case strings.HasPrefix(p.State, "STOPPED"), p.State == "NOT CREATED":
			if !pendingBuild || p.State == "NOT CREATED" {
				actions = append(actions, actionStart)
			}
		}
		actions = append(actions, actionBuild)
	}
	return append(actions, actionPlugins, actionConfig, actionRemove, actionBack)
}
