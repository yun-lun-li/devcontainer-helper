# dch — devcontainer helper

`dch` is a terminal app for managing Dev Containers on Linux, WSL2, and macOS. It is a single Go binary with no third-party Go dependencies.

| Feature | Description |
| --- | --- |
| Manage Dev Containers in one place | View the status of multiple projects in a terminal interface, and connect to, start, build, or stop their containers. |
| Apply personal settings | Merge `devcontainer.local.json` with the project configuration for previews and container operations, without changing the original configuration file. |

## Prerequisites

- Docker CLI and an accessible Docker daemon
- Official [Dev Container CLI](https://github.com/devcontainers/cli): `npm install -g @devcontainers/cli` (or its official install script)
- A `<project>/.devcontainer/devcontainer.json` file in each project you register

## Install

Install the latest release and the Dev Container CLI with POSIX `sh` (no Node.js installation is needed):

```sh
curl -fsSL https://raw.githubusercontent.com/yun-lun-li/devcontainer-helper/main/install.sh | sh
dch version

## Update
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

When `dch` opens the interactive console, it checks GitHub for the latest release. The check runs only for release builds in an interactive terminal; source builds (`dev`) and non-interactive runs skip it. The console reports when the installed version is already up to date.

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

`~/.dch/config` is JSON without a `.json` extension. It stores **only project paths**. dch reads each project's name from the merged `.devcontainer/devcontainer.json` and optional `.devcontainer/devcontainer.local.json`; comments and trailing commas are supported in both. If `name` is missing, the directory name is used.

```json
{
  "version": 1,
  "projects": [
    "/home/user/projects/app-one",
    "/home/user/projects/app-two"
  ]
}
```
