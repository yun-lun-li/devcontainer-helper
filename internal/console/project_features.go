package console

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"dch/internal/features"
	"dch/internal/hostpass"
)

func projectFeatures(p Project) error {
	for {
		enabled, entry, err := features.GitHubCLISettings(p.Path)
		if err != nil {
			return err
		}
		label := "GitHub CLI with host pass"
		if enabled {
			label += "  (" + entry + ")"
		}
		_, err = chooseCheckboxes(p.Name+" / Features", []string{label}, []bool{enabled})
		if errors.Is(err, errCancelled) {
			return nil
		}
		if err != nil {
			return err
		}
		if err := toggleGitHubCLI(p, enabled, entry); err != nil {
			if errors.Is(err, errCancelled) {
				continue
			}
			return err
		}
		screenNotice("Features", "Saved devcontainer.local.json. Use Build to apply the change.\nKeep devcontainer.local.json and .devcontainer/.dch/ out of version control.")
	}
}

func toggleGitHubCLI(p Project, enabled bool, entry string) error {
	if enabled {
		if err := features.DisableGitHubCLI(p.Path); err != nil {
			return err
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := hostpass.StopIfRunning(ctx, p.Path); err != nil {
			return fmt.Errorf("Feature disabled in local config, but stopping its host service failed: %w", err)
		}
		return nil
	}
	value, err := promptPassEntry(entry)
	if err != nil {
		return err
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	ui := hostpass.SetupUI{Choose: chooseHostSetup, Stdin: os.Stdin, Stdout: os.Stdout, Stderr: os.Stderr}
	if err := hostpass.Setup(ctx, value, ui); err != nil {
		if errors.Is(err, hostpass.ErrSetupCancelled) {
			return errCancelled
		}
		return err
	}
	return features.EnableGitHubCLI(p.Path, value)
}

func chooseHostSetup(title string, options []string) (int, error) {
	choice, err := choose(title, options)
	screenHeading("GitHub CLI host setup")
	return choice, err
}

func promptPassEntry(defaultEntry string) (string, error) {
	screenHeading("GitHub CLI with host pass")
	fmt.Println("Requires Linux, local Docker and matching host/container UIDs. Host pass/GPG setup follows.")
	fmt.Println("Git and curl must be available in the container. The container can retrieve this PAT.")
	fmt.Printf("Host pass entry [%s] (entry name, never paste a PAT; q to cancel): ", defaultEntry)
	line, err := input.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	line = strings.TrimSpace(line)
	if line == "q" || (line == "" && errors.Is(err, io.EOF)) {
		return "", errCancelled
	}
	if line == "" {
		line = defaultEntry
	}
	if strings.HasPrefix(line, "ghp_") || strings.HasPrefix(line, "github_pat_") {
		return "", fmt.Errorf("enter a pass entry name, not a PAT")
	}
	if err := hostpass.ValidateEntry(line); err != nil {
		return "", err
	}
	return line, nil
}
