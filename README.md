# dch — devcontainer helper

`dch` is a terminal app for managing Dev Containers on Linux, WSL2, and macOS. It is a single Go binary with no third-party Go dependencies.

| Feature | Description |
| --- | --- |
| Manage Dev Containers in one place | View the status of multiple projects in a terminal interface, and connect to, start, build, or stop their containers. |
| Apply personal settings | Merge sorted `.devcontainer/devcontainer.local.d/*.json` fragments with the project configuration for previews and container operations, without changing the original configuration file. |
| Project plugins | Configure per-project host services such as GitHub App credentials, then apply their container settings with Build. |

## Prerequisites

- Docker CLI and an accessible Docker daemon
- Official [Dev Container CLI](https://github.com/devcontainers/cli): `npm install -g @devcontainers/cli` (or its official install script)
- A `<project>/.devcontainer/devcontainer.json` file in each project you register

## Install

Install the latest release and the Dev Container CLI with POSIX `sh` (no Node.js installation is needed):

```sh
curl -fsSL https://raw.githubusercontent.com/yun-lun-li/devcontainer-helper/main/install.sh | sh
dch version
```

## Update

```sh
dch update 
```

The installer puts both commands in `~/.local/bin`, creates `~/.dch/config`, and adds the binary directory to the detected shell profile (`~/.bashrc`, `~/.zshrc`, or `~/.profile`). Set `DCH_VERSION` to install a specific release tag, or `DCH_PREFIX` and `DCH_INSTALL_DIR` to change the installation locations.

## Quick start

```bash
dch register --path .          # Register the current directory
dch console                    # Open the Projects console
dch update                     # Update dch to the latest release
dch version                    # Show the installed version
```

`dch register --path PATH` is the only registration command. Relative paths are resolved against the current directory and stored as absolute paths; `~` is also supported. The project must contain `.devcontainer/devcontainer.json`. Registering the same path again is safe and does not create a duplicate.

### Commands

Container operations are available inside the console.

```bash
dch register --path PATH  # Register one project; accepts relative paths
dch list                  # Print projects, status, paths, and container IDs
dch console               # Open the interactive Projects console
dch update                # Update dch to the latest release
dch version               # Show the installed version
dch help                  # Show command help
```

### Configuration file

`~/.dch/config` is JSON without a `.json` extension. It stores registered project paths and enabled plugin settings. Private keys, JWTs, and access tokens are not stored there. dch reads each project's name from the merged `.devcontainer/devcontainer.json` and optional first-level `.devcontainer/devcontainer.local.d/*.json` fragments; comments and trailing commas are supported. If `name` is missing, the directory name is used.

```json
{
  "version": 1,
  "projects": [
    "/home/user/projects/app-one",
    "/home/user/projects/app-two"
  ]
}
```

### Project plugins

Each project has its own plugin configuration and state. **Project → Plugins** lists registered plugins and renders their configuration fields. The manager dispatches Host and Container lifecycle hooks, records state, and reports whether a container needs to be recreated. Plugins that change only host resources can leave container recreation unnecessary.

Built-in plugins implement `internal/plugins.Plugin` in their own subdirectory and are registered in `main.go`. `Definition` provides the configuration fields; validation, lifecycle work, and status belong to the implementation. Container hooks run through the provided executor using `devcontainer exec`. Plugins can also implement the optional `HostService` interface when they need a persistent host process. The generic manager and console do not require GitHub, systemd, or a Unix Socket.

The first implementation is in `internal/plugins/githubapp/`. Adding another built-in plugin requires its implementation and registration, without editing the manager or adding plugin-specific console branches. External plugin downloads and dynamic loading are not supported.

### GitHub App plugin

The built-in GitHub App plugin offers **BYO GitHub App (Recommended)** and **Fine-grained token** authentication. Both require Linux, local Docker, a systemd user service, `pass`, and GPG on the host. For the App option, configure its permissions and installation on GitHub, enable Device Flow, then enter its **Client ID** during project setup. Existing project and shared Client IDs can be reused as defaults. No App ID, installation ID, client secret, or App private key is needed.

Open **Project → Plugins** to enable or reauthorize the plugin and select an authentication method. Before requesting a token, dch checks GPG and `pass`. Missing packages can be installed with apt, dnf, or pacman after you approve the displayed command; sudo and the package manager handle their own prompts. Other systems receive manual installation guidance. dch reuses an initialized password store, or helps you select/create a GPG encryption key and run `pass init`. A temporary secret must pass an encrypt/decrypt round trip and be removed before authorization continues. Installed packages, new keys, and password-store initialization remain if later setup is canceled.

For **BYO GitHub App**, dch reads the project's Git remotes and asks you to confirm the repository; with multiple supported remotes, select one. It obtains the numeric repository ID through the public GitHub API, or through the host's existing `gh` authentication for private repositories. SSH clone access alone does not authenticate REST API requests. If the ID cannot be obtained, dch offers an additional discovery authorization before the repository-scoped authorization. With a resolved ID, only one Device Flow is needed.

The final Device Flow exchanges include `repository_id`. dch verifies the returned token's repository access before saving it and supplying it to the container; an invalid ID or an unverifiable restriction never falls back to a broader token. Confirm the GitHub account shown by dch. Pull requests use this user's identity; Git commit author settings remain unchanged.

For **Fine-grained token**, create a PAT on GitHub with only the intended repository and the permissions your operations require. Paste it into the hidden terminal prompt. dch verifies the account and target repository API access; this does not prove the token is restricted to that repository or has every permission needed for Git or `gh`. Classic PATs are not supported. Each project stores its own PAT in host `pass`; PATs are not refreshed automatically. Reconfigure the plugin when a PAT expires, is revoked, or needs different permissions.

Each project keeps its own encrypted credential entry in host `pass`. App credentials include access/refresh tokens and expiration metadata; PAT credentials include the token and project/account/repository binding. dch derives entry names automatically. It saves a replacement before switching services, restores the previous credential on a failed switch, and cleans up the old entry after a successful switch. Neither refresh tokens nor GPG keys are mounted into the container.

The host service permits graphical pinentry and allows up to two minutes for a credential request, including GPG unlock. A working graphical pinentry and user session are required. dch forwards relevant display/session variables when configuring the service and does not change GPG agent configuration or save a passphrase. If the session changes, configure the service again. On SSH/headless hosts, unlock the same GPG agent in a host terminal and retry. Canceling pinentry or exceeding the deadline fails the credential request; cached tokens may be returned without another unlock prompt.

The container needs Git 2.30 or newer, GitHub CLI (`gh`), curl, and POSIX `sh`. The plugin adds a read-only mount at `/run/dch-github`, a Git credential helper for HTTPS `github.com`, and a `gh` wrapper available from Connect shells. The wrapper requests a token for each invocation and sets `GH_TOKEN` only for that `gh` process. Other Git hosts keep their existing credential helpers; SSH URLs are unchanged. Existing lifecycle hooks are preserved. The socket directory is mode `0700` and the socket is mode `0600`; the container user must be able to connect to it, so a mismatched UID will fail.

Any container process that can access the socket can request that project's credential (including a PAT), and Docker administrators can access the mount. Use the plugin only with containers you trust.

After enabling, Build the project to recreate the container, apply the mount, and initialize the Git helper. This does not force an image rebuild; image building follows the devcontainer configuration and cache. Enabling or disabling the plugin marks an existing container as needing Build. Reauthorization that keeps the adapter and mount unchanged does not add a Build requirement. dch does not rebuild automatically. Start and Connect are blocked for an existing container until Build succeeds. Disabling stops the host service and removes its runtime files immediately; removing a project also disables its plugin before unregistering it.

The systemd user service stays active when dch closes or the container stops. For App authentication, the service caches the access token in host memory and reloads the encrypted credential from `pass` after a restart. It reuses a token while it has more than five minutes left and refreshes it on the next credential request when five minutes or less remain. Refresh rotates and saves the credential pair before publishing the new token. Invalid or revoked authorization requires reauthorization through Project Plugins. No plaintext token files are mounted into the container and no refresh timer is installed.

To check that the host service is alive without requesting a credential, use:

```sh
curl --fail --silent --show-error --max-time 5 --unix-socket "$XDG_RUNTIME_DIR/dch/<project-hash>/credential.sock" http://localhost/health
```

The health endpoint returns only service status. Do not request or print `/credential` when checking service health.

#### Manual Linux verification

1. In **Project → Plugins**, select BYO GitHub App, complete the GPG/pass checks, enter or reuse a Client ID, confirm the remote, and complete Device Flow. Verify the selected repository restriction without printing the token.
2. Close dch and stop the container. Confirm the user service remains active; run the `/health` request above and expect `ok`.
3. Build and Connect. Confirm Git fetch/push and `gh pr create` use the configured repository and expected GitHub identity and permissions.
4. Restart the user service while the container is running, then use Git or `gh` again to confirm the mounted runtime directory exposes the replacement socket.
5. Reauthorize and verify it does not add a Build requirement. Disable the plugin and confirm the service stops immediately, its owned token entry is removed, and Build is required to remove the mount.
6. Repeat with a container user whose UID does not match the host socket owner and confirm connection fails.
7. On apt, dnf, and pacman hosts, test missing-package approval, rejection, install failure, key creation, and password-store initialization. Verify no Device Flow or PAT input occurs when the prerequisite check fails.
8. On a GUI host, test pinentry unlock after the agent cache expires, cancellation, and timeout. On SSH/headless hosts, unlock from the host terminal and retry. Confirm no passphrase or token appears in output.
9. Configure a fine-grained PAT with intended permissions, test Git/gh, replace the PAT, switch between PAT and App, and disable the plugin. Test expired/revoked tokens and failed replacement; verify the previous configuration remains recoverable and other projects' entries remain intact.

These checks require Linux, Docker, systemd, `pass`, and GPG agent access; they are manual integration checks.

Project-specific settings belong in `.devcontainer/devcontainer.local.d/` as first-level `.json` files. dch merges them in filename order; keep local fragments out of version control when they contain machine-specific settings. dch no longer reads `.devcontainer/devcontainer.local.json`; existing files are ignored and never converted automatically.
