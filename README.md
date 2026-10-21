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
dch register --path .          # Register the current directory
dch console                    # Open the Projects console
```

`dch register --path PATH` is the only registration command. Relative paths are resolved against the current directory and stored as absolute paths; `~` is also supported. The project must contain `.devcontainer/devcontainer.json`. Registering the same path again is safe and does not create a duplicate.

`dch console` opens directly on the Projects list. Running `dch` without arguments opens the same console. Each row shows a registered project's name, container state, and path. Select a project to see its colored status card, path with a folder icon, and available actions:

| Project state | Available actions |
| --- | --- |
| `RUNNING` | **Connect**, **Stop**, **Build**, **Remove Project** |
| `STOPPED` or `NOT CREATED` | **Start**, **Build**, **Remove Project** |
| `UNKNOWN` or `CONFIG MISSING/INVALID` | **Remove Project**; container actions require Docker and a valid configuration |

**Connect** opens a shell through `devcontainer exec`, which applies the Dev Container CLI's `remoteUser` and `remoteEnv` settings. It starts bash when available and falls back to sh. **Start** runs `devcontainer up`. The project and action screens refresh Docker status after an operation.

**Stop** stops the project's running containers, then asks whether to delete any matching containers that remain. Answer No to keep them in the `STOPPED` state so **Start** can run them again. Answer Yes to remove them; changes stored only in a container's writable layer are lost. If Docker has already removed a container after stopping it, there is nothing further to delete. **Remove Project** immediately unregisters the path without another prompt. It never deletes project files, containers, or images.

Use ↑/↓ or `j`/`k` to move, Enter to select, and `q` or Esc to go back. The interface redraws in one terminal screen and hides the cursor while a menu is open. The cursor reappears for prompts and shells, and quitting restores the original terminal view. Set `NO_COLOR=1` to disable colors. When stdin is redirected, menus use numbered choices instead.

### Commands

The command line has three primary commands. Container operations are available inside the console.

```bash
dch register --path PATH  # Register one project; accepts relative paths
dch list                  # Print projects, status, paths, and container IDs
dch console               # Open the interactive Projects console
dch help                  # Show command help
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

### Source layout

- `internal/commands` handles CLI dispatch, registration, and `dch list` output.
- `internal/console` handles the full-screen interface and container actions.
- `internal/common` holds project configuration, metadata, and Docker status logic shared by both.

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
