package githubapp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"dch/internal/plugins"
)

const maxPATLength = 4096

func (p *Plugin) configurePAT(ctx context.Context, configure plugins.ConfigureContext, cfg Config, interaction plugins.Interaction) (plugins.PreparedConfiguration, error) {
	repositoryName, err := choosePATRepository(ctx, cfg.ProjectPath, interaction)
	if err != nil {
		return plugins.PreparedConfiguration{}, err
	}
	if err := notifyPATCreationGuide(ctx, interaction); err != nil {
		return plugins.PreparedConfiguration{}, err
	}
	input := interaction.(plugins.InputInteraction)
	token, err := input.Input(ctx, "Fine-grained personal access token", "", true)
	if err != nil {
		return plugins.PreparedConfiguration{}, err
	}
	if !isFineGrainedPAT(token) {
		return plugins.PreparedConfiguration{}, fmt.Errorf("enter a fine-grained personal access token beginning with github_pat_")
	}
	user, err := p.fetchUser(ctx, token)
	if err != nil {
		return plugins.PreparedConfiguration{}, fmt.Errorf("could not verify GitHub account; check the PAT and organization approval")
	}
	repository, err := lookupRepositoryWithToken(ctx, token, repositoryName, p.repositoryOptions())
	if err != nil {
		return plugins.PreparedConfiguration{}, fmt.Errorf("could not verify repository access; check the PAT permissions and organization approval")
	}
	return p.preparePATConfiguration(ctx, cfg.ProjectPath, repository, user, token, interaction)
}

func choosePATRepository(ctx context.Context, project string, interaction plugins.Interaction) (string, error) {
	remotes, err := ReadGitRemotes(ctx, project, "git")
	if err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		return inputPATRepository(ctx, interaction)
	}
	if len(remotes) == 0 {
		return inputPATRepository(ctx, interaction)
	}
	selected, err := chooseRemote(ctx, interaction, remotes)
	if err != nil {
		return "", err
	}
	return selected.Repository, nil
}

func inputPATRepository(ctx context.Context, interaction plugins.Interaction) (string, error) {
	input := interaction.(plugins.InputInteraction)
	name, err := input.Input(ctx, "GitHub repository (owner/repo)", "", false)
	if err != nil {
		return "", err
	}
	return normalizeRepositoryName(name)
}

func normalizeRepositoryName(name string) (string, error) {
	parts := strings.Split(strings.TrimSpace(name), "/")
	if len(parts) != 2 || !validOwner(parts[0]) || !validRepository(parts[1]) {
		return "", fmt.Errorf("enter a repository as owner/repo")
	}
	return parts[0] + "/" + parts[1], nil
}

func isFineGrainedPAT(token string) bool {
	const prefix = "github_pat_"
	if len(token) > maxPATLength || !strings.HasPrefix(token, prefix) || len(token) == len(prefix) {
		return false
	}
	for _, character := range token[len(prefix):] {
		if !isPATCharacter(character) {
			return false
		}
	}
	return true
}

func isPATCharacter(character rune) bool {
	return character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z' || character >= '0' && character <= '9' || character == '_' || character == '-'
}

func lookupRepositoryWithToken(ctx context.Context, token, fullName string, opts RepositoryOptions) (Repository, error) {
	name, err := normalizeRepositoryName(fullName)
	if err != nil {
		return Repository{}, err
	}
	parts := strings.Split(name, "/")
	repository, err := requestRepository(ctx, http.MethodGet, "/repos/"+parts[0]+"/"+parts[1], token, opts)
	if err != nil || !sameRepositoryName(repository.FullName, name) {
		return Repository{}, ErrRepositoryUnavailable
	}
	return repository, nil
}

func (p *Plugin) preparePATConfiguration(ctx context.Context, project string, repository Repository, user githubUser, token string, interaction plugins.Interaction) (plugins.PreparedConfiguration, error) {
	cfg := Config{AuthMethod: authMethodPAT, ProjectPath: project, UserID: user.ID, Login: user.Login, RepositoryID: repository.ID, Repository: repository.FullName}
	if err := cfg.Validate(); err != nil {
		return plugins.PreparedConfiguration{}, err
	}
	message := fmt.Sprintf("Authorize this project as %s (%d) for %s? Check that the PAT has the access you need. GitHub lets dch check this repository's metadata, but not verify that the PAT is limited to this repository or inspect all its permissions. Public repository metadata access does not confirm that Git or gh can push.", user.Login, user.ID, repository.FullName)
	confirmed, err := interaction.Confirm(ctx, "Confirm GitHub account and repository", message)
	if err != nil {
		return plugins.PreparedConfiguration{}, err
	}
	if !confirmed {
		return plugins.PreparedConfiguration{}, fmt.Errorf("authorization cancelled")
	}
	tokens := TokenSet{Schema: 2, AuthMethod: authMethodPAT, ProjectPath: project, UserID: user.ID, RepositoryID: repository.ID, Repository: repository.FullName, PersonalAccessToken: token}
	raw, err := json.Marshal(cfg)
	if err != nil {
		return plugins.PreparedConfiguration{}, err
	}
	return plugins.PreparedConfiguration{Config: raw, Commit: func(ctx context.Context, host plugins.HostContext) (plugins.Result, error) {
		return p.commitCandidate(ctx, host, cfg, tokens, raw)
	}}, nil
}

func notifyPATCreationGuide(ctx context.Context, interaction plugins.Interaction) error {
	message := "Create a fine-grained token at https://github.com/settings/personal-access-tokens/new. Choose only this repository and grant the minimum permissions for your use. GitHub may require organization approval. Public repository metadata access alone does not grant Git or gh push access. More information: https://docs.github.com/en/authentication/keeping-your-account-and-data-secure/managing-your-personal-access-tokens"
	return interaction.Notify(ctx, "Create a fine-grained personal access token", message)
}
