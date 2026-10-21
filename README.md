# dch — devcontainer helper

`dch` is a terminal app for managing Dev Containers on Linux, WSL2, and macOS. It is a single Go binary with no third-party Go dependencies.

## Prerequisites

- Docker CLI and an accessible Docker daemon
- Official [Dev Container CLI](https://github.com/devcontainers/cli): `npm install -g @devcontainers/cli` (or its official install script)
- Go 1.21 or later to build from source (not required for a prebuilt binary)
- A `<project>/.devcontainer/devcontainer.json` file in each project you register

## Install

```bash
make install
# Ensure ~/.local/bin is on PATH:
export PATH="$HOME/.local/bin:$PATH"
```

To install elsewhere, run `make install PREFIX=/your/prefix`. The executable is installed in `PREFIX/bin`.

## Quick start

```bash
dch
```

The main menu has **Projects**, **Manage registrations**, and **Quit**. To add your first project, open **Manage registrations → Add project** and enter its directory. Press Enter at the path prompt to register the current directory. The project must contain `.devcontainer/devcontainer.json`.

Open **Projects** to see every registered project, its container state, and its path. Select a project to choose an action:

| Project state | Available actions |
| --- | --- |
| `RUNNING` | **Connect**, **Stop**, **Build** |
| `STOPPED` or `NOT CREATED` | **Start**, **Build** |
| `UNKNOWN` or `CONFIG MISSING/INVALID` | No container actions until Docker or the configuration is fixed |

**Connect** opens a shell through `devcontainer exec`, which applies the Dev Container CLI's `remoteUser` and `remoteEnv` settings. It starts bash when available and falls back to sh. **Start** runs `devcontainer up`. The project and action screens refresh Docker status after an operation.

**Stop** stops the project's running containers, then asks whether to delete all containers matching its workspace and Dev Container configuration labels. Answer No to keep them in the `STOPPED` state so **Start** can run them again. Answer Yes to remove them; changes stored only in a container's writable layer are lost. The project registration, workspace files, and images are kept. **Manage registrations → Remove project** only unregisters the path; it does not remove any container or file.

Use ↑/↓ or `j`/`k` to move, Enter to select, and `q` or Esc to go back. The interface redraws in one terminal screen and hides the cursor while a menu is open. The cursor reappears for prompts and shells, and quitting restores the original terminal view. Set `NO_COLOR=1` to disable colors. When stdin is redirected, menus use numbered choices instead.

### Direct commands

You can also run these commands directly. `config`, `build`, `up`, and `connect` still prompt for selections; `list` and `help` print output without a menu.

```bash
dch config   # Add or remove project paths
dch list     # Show projects, status, paths, and container IDs
dch build    # Select a project and build or recreate it
dch up       # Select a project and run devcontainer up
dch connect  # Select a running project and open a shell
dch help     # Show command help
```

### Configuration file

`~/.dch/config` is JSON without a `.json` extension. It stores **only project paths**. `dch` reads each project's name from `.devcontainer/devcontainer.json` every time; comments and trailing commas in that file are supported. If `name` is missing, the directory name is used.

```json
{
  "version": 1,
  "projects": [
    "/home/user/projects/app-one",
    "/home/user/projects/app-two"
  ]
}
```

### Build modes

| Mode | Command | Result |
| --- | --- | --- |
| Rebuild and recreate (first option) | `devcontainer up --workspace-folder PATH --remove-existing-container --build-no-cache` | Recreates the existing container without using the build cache, then starts it. Requires confirmation. Unsaved changes in the container's writable layer may be lost. |
| Build image only | `devcontainer build --workspace-folder PATH --no-cache` | Builds an image without replacing the running container. A later `up` may not use the newly built image. |

## Status

`dch` finds containers with the `devcontainer.local_folder` label using `docker ps -a`, then uses `docker inspect` to match the exact workspace path and, when present, `devcontainer.config_file`. `dch list` shows the matched container ID, or `-` when there is no matching container. It does not infer projects from container names.

On a terminal, the table colors its header, statuses, and container IDs. Redirected output stays plain text. Set `NO_COLOR=1` to disable colors.

Possible states include `RUNNING`, `STOPPED`, `NOT CREATED`, and `CONFIG MISSING/INVALID`. When Docker is unavailable, valid projects show `UNKNOWN`; the Projects screen indicates that Docker is unavailable, while `dch list` prints a warning. If a project has multiple matching containers, the status includes their count.

## Limitations

- Only `.devcontainer/devcontainer.json` at the project root is supported. Multiple configuration files and a root-level `.devcontainer.json` are not supported.
- Status matching uses local absolute paths. A container created through a different symlink or Windows/VS Code path may have a different Docker label and fail to match.
- The official `devcontainer` CLI supports Docker Compose devcontainers, but labels vary by configuration. A Compose service without a matching label may not appear in `dch list`.

## Development

`main.go` is the CLI entry point. Command implementations and terminal UI code are in `internal/commands`.

Run the tests with:

```bash
make test
```
