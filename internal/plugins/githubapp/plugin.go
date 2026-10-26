package githubapp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"dch/internal/plugins"
	oauth "github.com/cli/oauth"
	"github.com/cli/oauth/api"
	"github.com/cli/oauth/device"
)

const pluginID = "github-app"

type Plugin struct {
	HTTPClient *http.Client
	APIBaseURL string
	PassPath   string
	Now        func() time.Time
}

func New() *Plugin { return &Plugin{} }

func (p *Plugin) Definition() plugins.Definition {
	return plugins.Definition{ID: pluginID, Name: "GitHub App", Description: "Provide repository-scoped GitHub user credentials to this project.", ServiceAliases: []string{"_github-app-serve"}, SharedFields: []plugins.Field{{Name: "client_id", Label: "GitHub App Client ID", Required: false, Kind: plugins.FieldText}}, Notices: []string{"GitHub App Device Flow must be enabled for this Client ID.", "Tokens are limited to the selected repository and saved in the host password store."}}
}

func (p *Plugin) ValidateConfig(raw json.RawMessage) error {
	cfg, err := decodeConfig(raw)
	if err != nil {
		return err
	}
	return cfg.Validate()
}
func (p *Plugin) HandleHost(ctx context.Context, host plugins.HostContext, event plugins.Event) (plugins.Result, error) {
	switch event {
	case plugins.EventEnable:
		return p.enable(ctx, host)
	case plugins.EventDisable:
		return p.disable(ctx, host)
	case plugins.EventStart, plugins.EventBuild, plugins.EventConnect:
		return p.ensure(ctx, host)
	default:
		return plugins.Result{}, nil
	}
}
func (p *Plugin) HandleContainer(ctx context.Context, container plugins.ContainerContext, event plugins.Event) (plugins.Result, error) {
	if event != plugins.EventStart && event != plugins.EventBuild && event != plugins.EventConnect {
		return plugins.Result{}, nil
	}
	if _, err := container.Executor.Exec(ctx, AdapterInstallCommand()...); err != nil {
		return plugins.Result{}, fmt.Errorf("install GitHub adapter: %w", err)
	}
	return plugins.Result{}, nil
}
func (p *Plugin) ValidateContainerConfig(_ context.Context, host plugins.ContainerContext, merged map[string]json.RawMessage) error {
	if _, exists := merged["dockerComposeFile"]; exists {
		return fmt.Errorf("GitHub App plugin does not support Docker Compose")
	}
	raw, err := json.Marshal(merged)
	if err != nil {
		return fmt.Errorf("cannot validate GitHub App adapter settings")
	}
	runtimeDir, err := RuntimeDir(host.ProjectPath)
	if err != nil {
		return err
	}
	return validateContainerConfig(raw, runtimeDir)
}
func (p *Plugin) Status(ctx context.Context, host plugins.HostContext) (plugins.PluginStatus, error) {
	status := plugins.PluginStatus{ID: pluginID, Name: "GitHub App", Enabled: len(host.Config) > 0}
	if !status.Enabled {
		return statusWithoutConfiguration(status, host)
	}
	if isLegacyConfig(host.Config) {
		if err := p.quarantineLegacy(ctx, host); err != nil {
			status.Message = "Legacy credential service could not be quarantined safely; authorization is blocked"
			return status, fmt.Errorf("legacy credential service quarantine failed: %w", err)
		}
		status.Message = "Authorization required: legacy installation credentials are no longer used"
		return status, nil
	}
	cfg, err := decodeConfig(host.Config)
	if err != nil {
		return status, err
	}
	if filepath.Clean(cfg.ProjectPath) != filepath.Clean(host.ProjectPath) {
		status.Message = "Authorization required: project binding does not match"
		return status, nil
	}
	state, stateErr := decodeState(host.State)
	if stateErr != nil {
		return status, stateErr
	}
	if containsRecoveryIdentity(state, identityForConfig(cfg)) {
		status.Message = "Authorization required: credential recovery is pending; reauthorize this project"
		return status, nil
	}
	if hasCredentialCleanup(state) {
		status.Message = "Authorization active; previous project credential cleanup is pending"
		return status, nil
	}
	if err := runSystemctl(ctx, "is-active", "--quiet", unitName(host.ProjectPath)); err != nil {
		status.Message = "Authorization or credential service required"
		return status, nil
	}
	account := cfg.Login
	if account == "" {
		account = fmt.Sprintf("GitHub user %d", cfg.UserID)
	}
	status.Message = fmt.Sprintf("Authorized as %s for %s", account, cfg.Repository)
	return status, nil
}

func statusWithoutConfiguration(status plugins.PluginStatus, host plugins.HostContext) (plugins.PluginStatus, error) {
	state, err := decodeState(host.State)
	if err != nil {
		return status, err
	}
	if hasCredentialCleanup(state) {
		status.Message = "Disabled; previous project credential cleanup is pending"
		status.CleanupPending = true
	}
	return status, nil
}

func (p *Plugin) Configure(ctx context.Context, configure plugins.ConfigureContext, interaction plugins.Interaction) (plugins.PreparedConfiguration, error) {
	_, hasInput := interaction.(plugins.InputInteraction)
	hostCommands, hasHostCommands := interaction.(plugins.HostCommandInteraction)
	if !hasInput || !hasHostCommands {
		return plugins.PreparedConfiguration{}, fmt.Errorf("host setup interaction is unavailable")
	}
	method, err := chooseAuthenticationMethod(ctx, interaction)
	if err != nil {
		return plugins.PreparedConfiguration{}, err
	}
	config := Config{AuthMethod: method, ProjectPath: configure.Host.ProjectPath}
	if method == authMethodApp {
		config.ClientID, err = configuredAppClientID(ctx, configure, interaction)
		if err != nil {
			return plugins.PreparedConfiguration{}, err
		}
	}
	if err := p.prepareHost(ctx, config, interaction, hostCommands); err != nil {
		return plugins.PreparedConfiguration{}, err
	}
	if method == authMethodPAT {
		return p.configurePAT(ctx, configure, config, interaction)
	}
	clientID := config.ClientID
	resolution, err := p.chooseRepository(ctx, configure.Host.ProjectPath, clientID, interaction)
	if err != nil {
		return plugins.PreparedConfiguration{}, err
	}
	issued, user, err := p.authorizeRepository(ctx, clientID, resolution.Repository, configure.Host.Config, resolution.DiscoveryUserID, interaction)
	if err != nil {
		return plugins.PreparedConfiguration{}, err
	}
	return p.prepareConfiguration(configure.Host.ProjectPath, clientID, resolution.Repository, user, issued)
}

func configuredAppClientID(ctx context.Context, configure plugins.ConfigureContext, interaction plugins.Interaction) (string, error) {
	defaultID := ""
	if current, err := decodeConfig(configure.Host.Config); err == nil && current.ClientID != "" {
		defaultID = current.ClientID
	} else if shared, err := sharedClient(configure.SharedConfig); err == nil {
		defaultID = shared.ClientID
	}
	input, ok := interaction.(plugins.InputInteraction)
	if !ok {
		return "", fmt.Errorf("secure GitHub App Client ID input is unavailable")
	}
	clientID, err := input.Input(ctx, "GitHub App Client ID (Enter keeps the current value)", defaultID, false)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(clientID) != clientID || !clientIDPattern.MatchString(clientID) {
		return "", fmt.Errorf("GitHub App Client ID is required and must be valid")
	}
	return clientID, nil
}

func chooseAuthenticationMethod(ctx context.Context, interaction plugins.Interaction) (string, error) {
	choices := []plugins.Choice{
		{Value: "app", Label: "BYO GitHub App (Recommended)"},
		{Value: "pat", Label: "Fine-grained personal access token"},
	}
	method, err := interaction.Select(ctx, "Choose GitHub authentication", choices, 0)
	if err != nil {
		return "", err
	}
	if method != "app" && method != "pat" {
		return "", fmt.Errorf("unsupported GitHub authentication method")
	}
	return method, nil
}

func (p *Plugin) chooseRepository(ctx context.Context, project, clientID string, ui plugins.Interaction) (repositoryResolution, error) {
	remotes, err := ReadGitRemotes(ctx, project, "git")
	if err != nil {
		return repositoryResolution{}, err
	}
	var selected *GitRemote
	if len(remotes) > 0 {
		remote, err := chooseRemote(ctx, ui, remotes)
		if err != nil {
			return repositoryResolution{}, err
		}
		selected = &remote
	}
	return p.resolveRepository(ctx, selected, clientID, ui)
}

func (p *Plugin) prepareConfiguration(project, clientID string, repo Repository, user githubUser, issued issuedToken) (plugins.PreparedConfiguration, error) {
	cfg := Config{AuthMethod: authMethodApp, ClientID: clientID, ProjectPath: project, UserID: user.ID, Login: user.Login, RepositoryID: repo.ID, Repository: repo.FullName}
	tokens, err := tokenSet(issued.Pair, cfg, issued.ReceivedAt)
	if err != nil {
		return plugins.PreparedConfiguration{}, err
	}
	now := p.now()
	if !tokens.AccessTokenExpiresAt.After(now.Add(5*time.Minute)) || !tokens.RefreshTokenExpiresAt.After(now) {
		return plugins.PreparedConfiguration{}, fmt.Errorf("GitHub credential expired or is too close to expiry after authorization checks")
	}
	raw, err := json.Marshal(cfg)
	if err != nil {
		return plugins.PreparedConfiguration{}, err
	}
	return plugins.PreparedConfiguration{Config: raw, Commit: func(ctx context.Context, host plugins.HostContext) (plugins.Result, error) {
		return p.commitCandidate(ctx, host, cfg, tokens, raw)
	}}, nil
}

func (p *Plugin) commitCandidate(ctx context.Context, host plugins.HostContext, cfg Config, tokens TokenSet, raw json.RawMessage) (plugins.Result, error) {
	activeIdentity := identityForConfig(cfg)
	candidateState, err := clearRecoveryIdentity(host.State, activeIdentity)
	if err != nil {
		return plugins.Result{}, err
	}
	store := p.secretStore(cfg)
	previousSecret, missing, err := saveCandidate(ctx, store, tokens)
	if err != nil {
		if isCredentialRestoreFailure(err) {
			err = errors.Join(err, recordCredentialRecoveryIdentity(host, activeIdentity))
		}
		return plugins.Result{}, err
	}
	host.Config = raw
	host.State = candidateState
	tracker := &stateTrackingStore{ScopedStore: host.Store}
	host.Store = tracker
	result, enableErr := p.enable(ctx, host)
	if enableErr == nil {
		oldIdentity := credentialIdentity{}
		if old, decodeErr := decodeConfig(host.PreviousConfig); decodeErr == nil {
			oldIdentity = identityForConfig(old)
		}
		return p.cleanupPreviousCredential(ctx, host, result, oldIdentity, activeIdentity)
	}
	restoreCtx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	restoreErr := store.WithLock(restoreCtx, func() error {
		if missing {
			return store.Delete(restoreCtx)
		}
		return store.RestoreRaw(restoreCtx, previousSecret)
	})
	if restoreErr != nil {
		recoveryErr := recordCredentialRecoveryIdentity(host, activeIdentity)
		return plugins.Result{}, errors.Join(fmt.Errorf("%w; previous credential restore failed and authorization is required: %v", enableErr, restoreErr), recoveryErr)
	}
	return plugins.Result{}, enableErr
}

func clearRecoveryIdentity(raw json.RawMessage, identity credentialIdentity) (json.RawMessage, error) {
	state, err := decodeState(raw)
	if err != nil {
		return nil, err
	}
	state = removeCredentialIdentity(state, identity)
	return encodeState(state), nil
}

type credentialRestoreFailure struct{ cause error }

func (failure credentialRestoreFailure) Error() string { return failure.cause.Error() }
func (failure credentialRestoreFailure) Unwrap() error { return failure.cause }

func isCredentialRestoreFailure(err error) bool {
	var failure credentialRestoreFailure
	return errors.As(err, &failure)
}

func recordCredentialRecoveryIdentity(host plugins.HostContext, identity credentialIdentity) error {
	if !identity.valid() {
		return fmt.Errorf("credential recovery metadata cannot be recorded: invalid identity")
	}
	baseState := host.State
	if tracker, ok := host.Store.(*stateTrackingStore); ok && tracker.saved {
		baseState = tracker.latestState
	}
	state, err := decodeState(baseState)
	if err != nil {
		return fmt.Errorf("credential recovery metadata cannot be recorded: %w", err)
	}
	state = addCredentialIdentity(state, identity)
	if err := host.Store.SaveState(encodeState(state)); err != nil {
		return fmt.Errorf("credential rollback failed and recovery metadata could not be saved: %w", err)
	}
	return nil
}

type stateTrackingStore struct {
	plugins.ScopedStore
	latestState json.RawMessage
	saved       bool
}

func (store *stateTrackingStore) SaveState(raw json.RawMessage) error {
	if err := store.ScopedStore.SaveState(raw); err != nil {
		return err
	}
	store.latestState = append(json.RawMessage(nil), raw...)
	store.saved = true
	return nil
}

func saveCandidate(ctx context.Context, store *SecretStore, tokens TokenSet) ([]byte, bool, error) {
	var previousSecret []byte
	var missing bool
	err := store.WithLock(ctx, func() error {
		var err error
		previousSecret, missing, err = saveCandidateLocked(ctx, store, tokens)
		return err
	})
	return previousSecret, missing, err
}

func saveCandidateLocked(ctx context.Context, store *SecretStore, tokens TokenSet) ([]byte, bool, error) {
	previous, err := store.Snapshot(ctx)
	missing := errors.Is(err, ErrSecretMissing)
	if err != nil && !missing {
		return nil, false, err
	}
	if err := store.Write(ctx, tokens); err == nil {
		return previous, missing, nil
	} else {
		writeErr := err
		rollbackCtx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
		defer cancel()
		var rollbackErr error
		if missing {
			rollbackErr = store.Delete(rollbackCtx)
		} else {
			rollbackErr = store.RestoreRaw(rollbackCtx, previous)
		}
		if rollbackErr != nil {
			return previous, missing, credentialRestoreFailure{cause: fmt.Errorf("%w; previous credential restore failed and authorization is required: %w", writeErr, rollbackErr)}
		}
		return previous, missing, writeErr
	}
}

func (p *Plugin) cleanupPreviousCredential(ctx context.Context, host plugins.HostContext, result plugins.Result, oldIdentity, activeIdentity credentialIdentity) (plugins.Result, error) {
	state, err := decodeState(result.State)
	if err != nil {
		return plugins.Result{}, err
	}
	if oldIdentity.valid() && oldIdentity != activeIdentity {
		state = addCredentialIdentity(state, oldIdentity)
	}
	state = removeCredentialIdentity(state, activeIdentity)
	if err := host.Store.SaveState(encodeState(state)); err != nil {
		return plugins.Result{}, err
	}
	state, _ = p.retryCredentialCleanupFor(ctx, host, state, activeIdentity)
	if err := host.Store.SaveState(encodeState(state)); err != nil {
		return plugins.Result{}, err
	}
	result.State = encodeState(state)
	return result, nil
}

func (p *Plugin) cleanupPreviousClient(ctx context.Context, host plugins.HostContext, result plugins.Result, oldClientID, activeClientID string) (plugins.Result, error) {
	old := credentialIdentity{}
	if oldClientID != "" {
		old = credentialIdentity{Method: authMethodApp, Key: oldClientID}
	}
	return p.cleanupPreviousCredential(ctx, host, result, old, credentialIdentity{Method: authMethodApp, Key: activeClientID})
}

func addCredentialIdentity(state ownershipState, identity credentialIdentity) ownershipState {
	if !identity.valid() {
		return state
	}
	if identity.Method == authMethodApp {
		if !containsClientID(state.CredentialCleanup, identity.Key) {
			state.CredentialCleanup = append(state.CredentialCleanup, identity.Key)
		}
		return state
	}
	for _, previous := range state.CredentialCleanupIdentities {
		if previous == identity {
			return state
		}
	}
	state.CredentialCleanupIdentities = append(state.CredentialCleanupIdentities, identity)
	return state
}

func removeCredentialIdentity(state ownershipState, identity credentialIdentity) ownershipState {
	if identity.Method == authMethodApp {
		state.CredentialCleanup = removeClientID(state.CredentialCleanup, identity.Key)
	}
	remaining := make([]credentialIdentity, 0, len(state.CredentialCleanupIdentities))
	for _, pending := range state.CredentialCleanupIdentities {
		if pending != identity {
			remaining = append(remaining, pending)
		}
	}
	state.CredentialCleanupIdentities = remaining
	return state
}

func removeClientID(values []string, clientID string) []string {
	remaining := make([]string, 0, len(values))
	for _, value := range values {
		if value != clientID {
			remaining = append(remaining, value)
		}
	}
	return remaining
}
func containsClientID(values []string, clientID string) bool {
	for _, value := range values {
		if value == clientID {
			return true
		}
	}
	return false
}

func sharedClient(raw json.RawMessage) (SharedConfig, error) {
	var cfg SharedConfig
	if json.Unmarshal(raw, &cfg) != nil || cfg.ClientID == "" {
		return cfg, fmt.Errorf("configure the GitHub App Client ID in Settings → Plugins first")
	}
	return cfg, nil
}

type repositoryResolution struct {
	Repository      Repository
	DiscoveryUserID int64
}

func (p *Plugin) resolveRepository(ctx context.Context, selected *GitRemote, clientID string, ui plugins.Interaction) (repositoryResolution, error) {
	if selected != nil {
		owner, name, ok := strings.Cut(selected.Repository, "/")
		if !ok {
			return repositoryResolution{}, fmt.Errorf("selected Git remote is invalid")
		}
		repo, err := LookupRepository(ctx, owner, name, p.repositoryOptions())
		if err == nil && repo.ID > 0 && strings.EqualFold(repo.FullName, selected.Repository) {
			return repositoryResolution{Repository: repo}, nil
		}
		proceed, confirmErr := ui.Confirm(ctx, "Additional GitHub authorization", "The repository ID could not be read from public GitHub or host gh. A discovery authorization needs broader repository read access. Continue?")
		if confirmErr != nil {
			return repositoryResolution{}, confirmErr
		}
		if !proceed {
			return repositoryResolution{}, fmt.Errorf("authorization cancelled")
		}
		return p.discoverRepository(ctx, clientID, selected.Repository, ui)
	}
	proceed, err := ui.Confirm(ctx, "Repository discovery", "No supported GitHub remote was found. Use an additional discovery authorization to choose a repository?")
	if err != nil {
		return repositoryResolution{}, err
	}
	if !proceed {
		return repositoryResolution{}, fmt.Errorf("authorization cancelled")
	}
	return p.discoverRepository(ctx, clientID, "", ui)
}

func (p *Plugin) discoverRepository(ctx context.Context, clientID, remoteName string, ui plugins.Interaction) (repositoryResolution, error) {
	if err := ui.Notify(ctx, "GitHub discovery", "This step uses one additional authorization to list repositories available to your GitHub account."); err != nil {
		return repositoryResolution{}, err
	}
	issued, err := p.requestDeviceToken(ctx, clientID, 0, ui)
	if err != nil {
		return repositoryResolution{}, err
	}
	discoveryUser, err := p.fetchUser(ctx, issued.Pair.Token)
	if err != nil {
		return repositoryResolution{}, err
	}
	if err := confirmDiscoveryUser(ctx, ui, discoveryUser); err != nil {
		return repositoryResolution{}, err
	}
	repos, err := ListAccessibleRepositories(ctx, issued.Pair.Token, p.repositoryOptions())
	if err != nil {
		return repositoryResolution{}, fmt.Errorf("could not completely verify repositories available to this account")
	}
	choices := make([]plugins.Choice, 0, len(repos))
	matches := make(map[string]Repository, len(repos))
	for _, repo := range repos {
		if remoteName != "" && !strings.EqualFold(repo.FullName, remoteName) {
			continue
		}
		key := fmt.Sprint(repo.ID)
		choices = append(choices, plugins.Choice{Value: key, Label: repo.FullName})
		matches[key] = repo
	}
	if len(choices) == 0 {
		return repositoryResolution{}, fmt.Errorf("selected repository was not found in the discovery authorization")
	}
	chosen, err := ui.Select(ctx, "Choose GitHub repository", choices, 0)
	if err != nil {
		return repositoryResolution{}, err
	}
	repo, ok := matches[chosen]
	if !ok {
		return repositoryResolution{}, fmt.Errorf("invalid repository selection")
	}
	return repositoryResolution{Repository: repo, DiscoveryUserID: discoveryUser.ID}, nil
}

func (p *Plugin) authorizeRepository(ctx context.Context, clientID string, repo Repository, previous json.RawMessage, discoveryUserID int64, ui plugins.Interaction) (issuedToken, githubUser, error) {
	issued, err := p.requestDeviceToken(ctx, clientID, repo.ID, ui)
	if err != nil {
		return issuedToken{}, githubUser{}, err
	}
	user, err := p.fetchUser(ctx, issued.Pair.Token)
	if err != nil {
		return issuedToken{}, user, err
	}
	if discoveryUserID > 0 && user.ID != discoveryUserID {
		return issuedToken{}, user, fmt.Errorf("GitHub account changed between discovery and repository authorization; start again")
	}
	if err := confirmAuthorizedUser(ctx, ui, user, previous); err != nil {
		return issuedToken{}, user, err
	}
	if err := VerifyRepositoryScope(ctx, issued.Pair.Token, repo, p.repositoryOptions()); err != nil {
		return issuedToken{}, user, fmt.Errorf("GitHub did not confirm access limited to the selected repository")
	}
	return issued, user, nil
}

type issuedToken struct {
	Pair       *api.AccessToken
	ReceivedAt time.Time
}

func (p *Plugin) requestDeviceToken(ctx context.Context, clientID string, repositoryID int64, ui plugins.Interaction) (issuedToken, error) {
	host, err := oauth.NewGitHubHost("https://github.com")
	if err != nil {
		return issuedToken{}, fmt.Errorf("GitHub device authorization is unavailable")
	}
	form := contextFormClient{ctx: ctx, client: p.oauthClient(), repositoryID: repositoryID}
	code, err := device.RequestCode(form, host.DeviceCodeURL, clientID, []string{"offline_access"})
	if err != nil {
		return issuedToken{}, fmt.Errorf("GitHub device authorization could not start: %w", err)
	}
	if !validDeviceVerificationURL(code.VerificationURI) {
		return issuedToken{}, fmt.Errorf("GitHub returned an unsafe verification URL")
	}
	if err := ui.Notify(ctx, "GitHub authorization", fmt.Sprintf("Enter code %s at %s", code.UserCode, code.VerificationURI)); err != nil {
		return issuedToken{}, err
	}
	if err := ui.OpenBrowser(ctx, code.VerificationURI); err != nil {
		if err := ui.Notify(ctx, "Open GitHub manually", code.VerificationURI); err != nil {
			return issuedToken{}, err
		}
	}
	pair, err := device.Wait(ctx, form, host.TokenURL, device.WaitOptions{ClientID: clientID, DeviceCode: code})
	if err != nil {
		return issuedToken{}, fmt.Errorf("GitHub authorization did not complete")
	}
	return issuedToken{Pair: pair, ReceivedAt: p.now()}, nil
}

func confirmDiscoveryUser(ctx context.Context, ui plugins.Interaction, user githubUser) error {
	account := user.Login
	if account == "" {
		account = fmt.Sprintf("user %d", user.ID)
	}
	ok, err := ui.Confirm(ctx, "Confirm discovery account", fmt.Sprintf("List repositories available to %s (%d)?", account, user.ID))
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("discovery authorization cancelled")
	}
	return nil
}

func confirmAuthorizedUser(ctx context.Context, ui plugins.Interaction, user githubUser, previous json.RawMessage) error {
	userID := user.ID
	account := user.Login
	if account == "" {
		account = fmt.Sprintf("user %d", userID)
	}
	if len(previous) > 0 {
		old, err := decodeConfig(previous)
		if err == nil && old.UserID > 0 && old.UserID != userID {
			ok, err := ui.Confirm(ctx, "Switch GitHub account", fmt.Sprintf("This project is currently authorized as user %d. Switch to %s (%d)?", old.UserID, account, userID))
			if err != nil {
				return err
			}
			if !ok {
				return fmt.Errorf("account switch cancelled")
			}
		}
	}
	ok, err := ui.Confirm(ctx, "Confirm GitHub account", fmt.Sprintf("Authorize this project as %s (%d)?", account, userID))
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("authorization cancelled")
	}
	return nil
}

func chooseRemote(ctx context.Context, interaction plugins.Interaction, remotes []GitRemote) (GitRemote, error) {
	if len(remotes) == 0 {
		return GitRemote{}, fmt.Errorf("no supported GitHub remote; configure a GitHub remote before authorization")
	}
	if len(remotes) == 1 {
		ok, err := interaction.Confirm(ctx, "GitHub repository", fmt.Sprintf("Authorize %s (%s)?", remotes[0].Name, remotes[0].Repository))
		if err != nil {
			return GitRemote{}, err
		}
		if !ok {
			return GitRemote{}, fmt.Errorf("authorization cancelled")
		}
		return remotes[0], nil
	}
	choices := make([]plugins.Choice, 0, len(remotes))
	defaultIndex := 0
	for i, r := range remotes {
		choices = append(choices, plugins.Choice{Value: r.Name, Label: r.Name + " — " + r.Repository})
		if r.Name == "origin" {
			defaultIndex = i
		}
	}
	name, err := interaction.Select(ctx, "Choose GitHub remote", choices, defaultIndex)
	if err != nil {
		return GitRemote{}, err
	}
	return SelectRemote(remotes, name)
}
func (p *Plugin) repositoryOptions() RepositoryOptions {
	opts := RepositoryOptions{HTTPClient: p.oauthClient(), APIBaseURL: p.APIBaseURL}
	if opts.APIBaseURL == "" {
		opts.APIBaseURL = "https://api.github.com"
	}
	return opts
}
func (p *Plugin) oauthClient() *http.Client {
	if p.HTTPClient != nil {
		return p.HTTPClient
	}
	return &http.Client{Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
}
func (p *Plugin) now() time.Time {
	if p.Now != nil {
		return p.Now()
	}
	return time.Now()
}
func (p *Plugin) fetchUser(ctx context.Context, token string) (githubUser, error) {
	apiBase := p.APIBaseURL
	if apiBase == "" {
		apiBase = "https://api.github.com"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(apiBase, "/")+"/user", nil)
	if err != nil {
		return githubUser{}, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := p.oauthClient().Do(req)
	if err != nil {
		return githubUser{}, fmt.Errorf("could not verify GitHub account")
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	var user githubUser
	if err != nil || len(body) > maxResponseBytes || resp.StatusCode != http.StatusOK || json.Unmarshal(body, &user) != nil || user.ID <= 0 {
		return githubUser{}, fmt.Errorf("could not verify GitHub account")
	}
	return user, nil
}

type githubUser struct {
	ID    int64  `json:"id"`
	Login string `json:"login"`
}

func tokenSet(pair *api.AccessToken, cfg Config, now time.Time) (TokenSet, error) {
	if pair == nil || pair.Token == "" || pair.RefreshToken == "" || pair.ExpiresIn <= 0 || pair.RefreshTokenExpiresIn <= 0 {
		return TokenSet{}, fmt.Errorf("GitHub did not issue expiring access and refresh tokens")
	}
	return TokenSet{Schema: 1, ClientID: cfg.ClientID, ProjectPath: cfg.ProjectPath, UserID: cfg.UserID, RepositoryID: cfg.RepositoryID, Repository: cfg.Repository, AccessToken: pair.Token, RefreshToken: pair.RefreshToken, AccessTokenExpiresAt: now.Add(pair.ExpiresIn), RefreshTokenExpiresAt: now.Add(pair.RefreshTokenExpiresIn)}, nil
}
func (p *Plugin) secretStore(cfg Config) *SecretStore {
	return &SecretStore{PassPath: p.PassPath, Entry: secretEntry(cfg)}
}

func validDeviceVerificationURL(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && u.Scheme == "https" && u.Host == "github.com" && u.User == nil && u.Path == "/login/device" && u.RawQuery == "" && u.Fragment == ""
}
