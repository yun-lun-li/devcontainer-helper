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
			_, err := choose("Projects\nNo registered projects. Run dch register --path PROJECT_PATH first.", []string{"Quit console"})
			if errors.Is(err, errCancelled) {
				return nil
			}
			return err
		}
		statusErr := common.UpdateStatuses(projects)
		options := make([]string, 0, len(projects)+1)
		for _, p := range projects {
			status, statusErr := plugins.NewManager().Status(context.Background(), p.Path)
			if statusErr != nil {
				return statusErr
			}
			state := p.State
			if status.PendingBuild {
				state += " · Build required"
			}
			options = append(options, fmt.Sprintf("%s  [%s]  %s", p.Name, state, p.Path))
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
		pluginStatus, err := plugins.NewManager().Status(context.Background(), p.Path)
		if err != nil {
			return err
		}
		actions := availableProjectActions(p, statusErr == nil, pluginStatus.PendingBuild)
		choice, err := chooseProject(p, actions, pluginStatus.PendingBuild)
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
		case actionPlugins:
			err = pluginsMenu(p)
		case actionConfig:
			err = showProjectConfig(p)
		case actionRemove:
			removeChoice, chooseErr := choose("Remove project?", []string{
				"Remove project registration  Keep files and containers",
				"Cancel",
			})
			if chooseErr != nil {
				err = chooseErr
			} else if removeChoice == 0 {
				err = removeProject(p)
				if err == nil {
					return nil
				}
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
