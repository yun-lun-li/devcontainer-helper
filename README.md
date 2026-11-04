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

### Dev Container config merge behavior

`devcontainer.local.json` merges recursively with `devcontainer.json`. Ordinary values from the local file replace base values. `runArgs` and `build.options` are merged as Docker option sequences: known single-value options use the local occurrence, keyed `--env`/`--label`/`--build-arg`/`--annotation` values override only the same key, and repeatable options remain additive. Conflict removal requires the entire base sequence to parse safely; if any base fragment is unknown or ambiguous, dch preserves the whole base sequence and appends the local sequence. For local options, dch applies conflicts from the recognized safe prefix, stops parsing at the first unknown or ambiguous fragment, and appends the full local sequence as written. The opaque suffix is not interpreted or used for conflict removal. With no local file, duplicate base tokens remain unchanged.

Lifecycle commands (`initializeCommand`, `onCreateCommand`, `updateContentCommand`, `postCreateCommand`, `postStartCommand`, and `postAttachCommand`) are replaced as whole values. `dockerComposeFile` arrays append in order. `mounts` keeps base entries whose targets are absent from local, then appends local entries in order. Local mounts replace the entire base mount at the same target, including across string and object forms; string targets accept `target`, `dst`, and `destination`. Entries without a readable target and duplicates within each input are preserved. The known set arrays `customizations.vscode.extensions`, `forwardPorts`, `capAdd`, and deprecated root `extensions` remove duplicates; the [VS Code Dev Container schema](https://github.com/microsoft/vscode/blob/main/extensions/configuration-editing/schemas/devContainer.vscode.schema.json) marks root `extensions` deprecated in favor of `customizations.vscode.extensions`. Other arrays use the local array as a whole.

The option classifications follow the Docker CLI `run` reference and flag registrations, and the Buildx `build` reference and flag registrations: [Docker run options](https://docs.docker.com/reference/cli/docker/container/run/), [Docker CLI run registration](https://github.com/docker/cli/blob/master/cli/command/container/run.go) and [container option registrations](https://github.com/docker/cli/blob/master/cli/command/container/opts.go), [Buildx build options](https://docs.docker.com/reference/cli/docker/buildx/build/), and [Buildx build registration](https://github.com/docker/buildx/blob/master/commands/build.go). The set-array names follow the [Dev Container base schema](https://github.com/devcontainers/spec/blob/main/schemas/devContainer.base.schema.json) and supporting tool reference. These are moving `master`/current-documentation sources; dch has no Docker SDK dependency and its option tables can lag newly added or changed upstream flags. Unknown flags therefore remain opaque until the tables are updated.
