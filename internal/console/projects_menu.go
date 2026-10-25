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

func projectsMenu(manager *plugins.Manager) error {
	for {
		projects, err := common.LoadProjects()
		if err != nil {
			return err
		}
		project, hasProject, quit, err := selectProject(manager, projects)
		if err != nil {
			return err
		}
		if quit {
			return nil
		}
		if !hasProject {
			continue
		}
		if err := projectMenu(manager, project.Path); err != nil {
			return err
		}
	}
}

func selectProject(manager *plugins.Manager, projects []Project) (Project, bool, bool, error) {
	if len(projects) == 0 {
		quit, err := emptyProjectsMenu(manager)
		return Project{}, false, quit, err
	}
	statusErr := common.UpdateStatuses(projects)
	options, err := projectOptions(manager, projects)
	if err != nil {
		return Project{}, false, false, err
	}
	choice, err := chooseProjectList(append(options, "Settings", "Quit console"), statusErr)
	if errors.Is(err, errCancelled) {
		return Project{}, false, true, nil
	}
	if err != nil {
		return Project{}, false, false, err
	}
	return selectedProject(manager, projects, choice)
}

func emptyProjectsMenu(manager *plugins.Manager) (bool, error) {
	choice, err := choose("Projects", []string{"Settings", "Quit console"})
	if errors.Is(err, errCancelled) || choice == 1 {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	return false, pluginSettingsMenu(manager)
}

func selectedProject(manager *plugins.Manager, projects []Project, choice int) (Project, bool, bool, error) {
	if choice == len(projects)+1 {
		return Project{}, false, true, nil
	}
	if choice == len(projects) {
		return Project{}, false, false, pluginSettingsMenu(manager)
	}
	return projects[choice], true, false, nil
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

func projectMenu(manager *plugins.Manager, path string) error {
	for {
		p, found, statusErr, pluginStatus, err := loadProjectMenuState(manager, path)
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
		done, actionErr := performProjectMenuAction(manager, actions[choice], p)
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

func performProjectMenuAction(manager *plugins.Manager, action projectAction, p Project) (bool, error) {
	if action == actionBack {
		return true, nil
	}
	if action == actionRemove {
		return confirmProjectRemoval(manager, p)
	}
	return false, runProjectAction(manager, action, p)
}

func loadProjectMenuState(manager *plugins.Manager, path string) (Project, bool, error, plugins.Status, error) {
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
	pluginStatus, err := manager.Status(context.Background(), p.Path)
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

func projectOptions(manager *plugins.Manager, projects []Project) ([]string, error) {
	options := make([]string, 0, len(projects)+1)
	for _, project := range projects {
		status, err := manager.Status(context.Background(), project.Path)
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

func runProjectAction(manager *plugins.Manager, action projectAction, p Project) error {
	switch action {
	case actionConnect:
		return connectProject(manager, p)
	case actionStart:
		return startProject(manager, p)
	case actionStop:
		return stopProject(p)
	case actionBuild:
		return buildProject(manager, p)
	case actionPlugins:
		return pluginsMenu(p, manager)
	case actionConfig:
		return showProjectConfig(p)
	}
	return nil
}

func confirmProjectRemoval(manager *plugins.Manager, p Project) (bool, error) {
	choice, err := choose("Remove project?", []string{"Remove project registration  Keep files and containers", "Cancel"})
	if err != nil || choice != 0 {
		return false, err
	}
	err = removeProject(manager, p)
	return err == nil, err
}

func removeProject(manager *plugins.Manager, p Project) error {
	return manager.RemoveProject(context.Background(), p.Path)
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
