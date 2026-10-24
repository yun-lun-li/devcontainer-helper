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

### GitHub App plugin

The built-in GitHub App plugin is Linux-only and gives one project access to one selected GitHub repository. It requires local Docker, a systemd user service, `pass`, and a GPG agent on the host. Create and install the GitHub App, then store its RSA private key in the host's `pass` store. The key entry must be readable by the host user running the dch systemd user service.

Before enabling the plugin, unlock the host GPG agent without printing the key:

```sh
pass show <entry> >/dev/null
```

The GPG agent must be able to decrypt without opening a prompt. dch invokes `pass` non-interactively; if the GPG agent's cached unlock expires, unlock the entry again before the next token refresh. The private key remains on the host and is never mounted into the container.

Open **Project → Plugins** to configure and enable the plugin. Enter the App ID, installation ID, pass entry, repository (`owner/repository`), and choose `read`, `write`, or disabled for `contents` and `pull_requests`. dch verifies the service and credential before enabling. The resulting installation token is limited to that repository and selected permissions.

The container needs Git 2.30 or newer, GitHub CLI (`gh`), curl, and POSIX `sh`. The plugin adds a read-only mount at `/run/dch-github`, a Git credential helper for HTTPS `github.com`, and a `gh` wrapper available from Connect shells. The wrapper requests a token for each invocation and sets `GH_TOKEN` only for that `gh` process. Other Git hosts keep their existing credential helpers; SSH URLs are unchanged. Existing lifecycle hooks are preserved. The socket directory is mode `0700` and the socket is mode `0600`; the container user must be able to connect to it, so a mismatched UID will fail.

Any container process that can access the socket can request that project's short-lived token, and Docker administrators can access the mount. Use the plugin only with containers you trust.

After enabling, Build the project to apply the mount and initialize the Git helper. Configuration changes and disabling the plugin also mark an existing container as needing Build. dch does not rebuild automatically. Start and Connect are blocked for an existing container until Build succeeds. Disabling stops the host service and removes its runtime files immediately; removing a project also disables its plugin before unregistering it.

The systemd user service stays active when dch closes or the container stops. Tokens are kept in host memory only. The service reuses a token while it has more than five minutes left and refreshes it on the next credential request when five minutes or less remain. It does not write token files or run a refresh timer.

To check that the host service is alive without requesting a credential, use:

```sh
curl --fail --silent --show-error --max-time 5 --unix-socket "$XDG_RUNTIME_DIR/dch/<project-hash>/credential.sock" http://localhost/health
```

The health endpoint returns only service status. Do not request or print `/credential` when checking service health.

#### Manual Linux verification

1. In **Project → Plugins**, enable the GitHub App and confirm dch verifies the service and credential without printing the token.
2. Close dch and stop the container. Confirm the user service remains active; run the `/health` request above and expect `ok`.
3. Build and Connect. Confirm Git fetch/push and `gh pr create` use the configured repository and expected GitHub identity and permissions.
4. Restart the user service while the container is running, then use Git or `gh` again to confirm the mounted runtime directory exposes the replacement socket.
5. Change the plugin settings or disable it. Confirm the existing container shows **Build required**, Start and Connect are blocked, and disabling stops the service immediately. Build to apply or remove the mount.
6. Repeat with a container user whose UID does not match the host socket owner and confirm connection fails. Also try enabling while the GPG agent cannot decrypt non-interactively and confirm activation fails without exposing key or token contents.

These checks require Linux, Docker, systemd, `pass`, and GPG agent access; they are manual integration checks.

Project-specific settings belong in `.devcontainer/devcontainer.local.d/` as first-level `.json` files. dch merges them in filename order; keep local fragments out of version control when they contain machine-specific settings. dch no longer reads `.devcontainer/devcontainer.local.json`; existing files are ignored and never converted automatically.
