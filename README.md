# dch — devcontainer helper

`dch` is a single-binary Dev Container helper for Linux, WSL2, and macOS terminals. It uses only the Go standard library.

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

## Usage

```bash
dch config   # Add or remove projects; an empty path selects the current directory
dch list     # Show registered projects and their status, then select one for details
dch build    # Select a project and rebuild its container or build its image only
dch up       # Select a project and run devcontainer up
dch connect  # Select a running project and open a shell with devcontainer exec
dch help     # Show command help
```

Use the arrow keys and Enter to select a menu item. Press `q` or Esc to cancel. When stdin is not a terminal, menus use numbered choices.

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

Removing a project with `dch config` removes only its registration. It does not delete its container, image, or workspace.

### Build modes

| Mode | Command | Result |
| --- | --- | --- |
| Rebuild and recreate (first option) | `devcontainer up --workspace-folder PATH --remove-existing-container --build-no-cache` | Recreates the existing container without using the build cache, then starts it. Requires confirmation. Unsaved changes in the container's writable layer may be lost. |
| Build image only | `devcontainer build --workspace-folder PATH --no-cache` | Builds an image without replacing the running container. A later `up` may not use the newly built image. |

## Status

`dch list` finds containers with the `devcontainer.local_folder` label using `docker ps -a`, then uses `docker inspect` to match the exact workspace path and, when present, `devcontainer.config_file`. It does not infer projects from container names.

Possible states include `RUNNING`, `STOPPED`, `NOT CREATED`, and `CONFIG MISSING/INVALID`. When Docker is unavailable, valid projects show `UNKNOWN` and a warning is printed. If a project has multiple matching containers, the status includes their count.

`dch connect` uses `devcontainer exec`, which applies the Dev Container CLI's `remoteUser` and `remoteEnv` settings. It starts bash when available and falls back to sh. Run `dch up` before connecting to a stopped project.

## Limitations

- Only `.devcontainer/devcontainer.json` at the project root is supported. Multiple configuration files and a root-level `.devcontainer.json` are not supported.
- Status matching uses local absolute paths. A container created through a different symlink or Windows/VS Code path may have a different Docker label and fail to match.
- The official `devcontainer` CLI supports Docker Compose devcontainers, but labels vary by configuration. A Compose service without a matching label may not appear in `dch list`.
- Docker stop/down commands are not implemented.

## Development

`main.go` is the CLI entry point. Command implementations are in `internal/commands`, with one file per user-facing command and separate files for shared configuration, project, Docker, and menu logic.

Run the tests with:

```bash
make test
```
