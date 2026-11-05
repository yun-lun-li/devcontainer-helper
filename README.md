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

### Feature host initialization

Start and Build support Feature host hooks through the tool-specific property
`customizations.dch.initializeCommand` in `devcontainer-feature.json`:

```json
{
  "id": "example",
  "version": "1.0.0",
  "name": "Example",
  "customizations": {
    "dch": {
      "initializeCommand": "sh \"$DCH_FEATURE_DIR/initialize.sh\""
    }
  }
}
```

This is a dch extension, not a standard Feature lifecycle hook. Other Dev Container
tools ignore it; the project's standard `initializeCommand` still works normally.
Only enable Features whose host code you trust.

The Dev Container CLI resolves local, OCI, and archive Features and their dependency
order. dch runs their host hooks in that order before `devcontainer up`, including
Build and subsequent Starts. A failed hook stops the operation. Connect to an
already running container does not rerun initialization. Hooks must be safe to run
repeatedly. They run as your host user, with the workspace as their working directory.

Hooks accept a shell string, an argument array (without shell expansion), or an
object of commands executed in parallel. Parallel commands have no terminal input;
use one string/array hook for interactive setup. Cancellation stops the hook's
process group and aborts startup. Feature options and defaults are available as
`DCH_FEATURE_OPTION_<UPPERCASE_NAME>` environment variables; for example, `passEntry`
becomes `DCH_FEATURE_OPTION_PASSENTRY`. The host environment is otherwise preserved.
`DCH_FEATURE_DIR` points to the resolved Feature directory,
`DCH_WORKSPACE_FOLDER` to the workspace, and `DCH_EXECUTABLE` to the running dch binary.
Do not put secrets in Feature options.

### GitHub CLI with a PAT in host pass

The local Feature at `.devcontainer/features/github-cli` depends on the official
GitHub CLI Feature. It installs a `gh` wrapper and an HTTPS Git credential helper.
A single detached dch process per host user serves all registered projects. Each
project has its own Unix socket bound to exactly one configured `pass` entry. The
container cannot select a different entry in its requests. The password store and GPG keys stay on the
host. The container receives the PAT when it uses Git or `gh`; processes with access
to the socket can retrieve that PAT. Use a PAT with the repository permissions you
intend to grant to the container.

For the first version, use Linux with local Docker and matching host/container
user IDs. Docker Desktop, remote Docker, and different
UIDs are not supported. The container image must provide Git and curl.

In dch, select a project, open **Features**, check **GitHub CLI with host pass**
with Space, and press Enter to apply. Use arrows to move and q/Esc to discard
pending checkbox changes. With redirected input, enter a number to toggle and
Enter or `a` to apply. Enter a host pass entry name (never the PAT itself).
dch then guides host setup before saving the Feature:

1. Check for pass, GPG, gpg-agent and pinentry. If needed, show package installation
   commands and offer to run them, using sudo for non-root users. Supported package
   managers are apt-get, dnf, pacman and apk; other systems require manual installation.
2. Reuse existing pass recipient settings. For a new store, select an existing usable
   encryption secret key or create one through GPG's interactive name, email and
   passphrase prompts. Initialize pass with the selected key. Existing stores without
   applicable recipient settings require manual repair rather than re-encryption.
3. If the entry is missing, prompt through `pass insert` for the PAT with hidden
   input, then verify that pass can decrypt it and that its first line has PAT format.
   An existing entry is verified and retained.

Only successful setup enables the Feature. Cancelling does not save pending Feature
settings; packages, keys or store initialization already completed remain on the host.
PAT values and GPG passphrases are never passed as command arguments or saved in
local configuration. The setup follows [pass initialization](https://www.passwordstore.org/)
and [GPG key generation](https://www.gnupg.org/documentation/manuals/gnupg/OpenPGP-Key-Management.html).

dch bundles the Feature, places it in `.devcontainer/.dch/features/github-cli`, and adds its options
to `.devcontainer/devcontainer.local.json`. Other settings and comments are retained.
Each project has its own toggle and entry setting. The menu is available even when
Docker is unavailable. Run **Build** afterward to apply the change.

```text
demo / Features
  › [ ] GitHub CLI with host pass

  ↑/↓ move · Space toggle · Enter apply · q/Esc discard
```

Uncheck the enabled Feature and apply to disable it. This removes its local setting and
stops the project's host pass service, retaining the encrypted PAT. Run **Build**
to remove the Feature from the container. Keep `devcontainer.local.json` and
`.devcontainer/.dch/` out of version control. Features configured in the base
`devcontainer.json` must be managed there. Feature files remain available for
existing containers. The menu currently manages the bundled GitHub CLI Feature.

For manual setup instead, copy the Feature directory into your project's
`.devcontainer/features/github-cli`, then add this to `devcontainer.json` or
your `devcontainer.local.json`. Initialize host pass/GPG yourself for this path;
Start/Build and `host-pass prepare` do not install packages or create GPG keys:

```json
{
  "features": {
    "./features/github-cli": {
      "passEntry": "dch/github/my-project"
    }
  }
}
```

Store the PAT using host pass's hidden prompt:

```sh
pass insert dch/github/my-project
```

Alternatively, the first dch Start/Build prompts through `pass insert` if the entry
is missing. Its first line must be a GitHub PAT (`github_pat_...` or `ghp_...`). The
Feature accepts an entry name, never a token. dch checks token format; GitHub checks
its permissions, expiration, and organization approval when Git/gh uses it.

Build the container after adding/removing the Feature. No configuration change to
this repository's own development container is required to build or test dch.

The runtime directory is `.devcontainer/.dch/github-auth`; add
`.devcontainer/.dch/` to your project's `.gitignore`. The directory and socket are
private to your host UID. The Feature mounts only that runtime directory. It never
mounts the password store, GPG directory, or plaintext credential files.

The service remains available when dch exits or the container stops. It rereads pass
for each request, so updating the same entry with `pass insert` takes effect without
a rebuild. GPG/pinentry can take up to two minutes; on headless hosts, unlock the same
host GPG agent in a terminal before retrying. Existing inherited GitHub tokens are
overridden for the wrapped `gh` process. SSH Git remotes are unchanged; use HTTPS to
use this helper. The wrapper targets github.com, not GitHub Enterprise.

Stop the service explicitly when removing the Feature or changing its entry:

```sh
dch host-pass stop --project /path/to/project
```

This retains the encrypted PAT. Delete it separately with `pass rm ENTRY` if desired.
After a host restart, use Start in dch or run
`dch host-pass prepare --project /path/to/project --entry ENTRY` before reconnecting.
If the host GPG/password-store environment changes, stop and prepare the service
again. Removing the Feature alone does not stop an existing host process.

`host-pass stop --project PATH` unregisters only that project, cancels its pending
credential requests, and removes its socket. Other projects keep working. Once all
projects are unregistered, the shared process exits after 30 idle seconds. New
registrations and idle shutdown are serialized with a host-level startup lock.
Concurrent credential reads are serialized across projects to limit GPG prompts.

The host-only control socket lives at `$XDG_RUNTIME_DIR/dch-host-pass/control.sock`,
or `~/.dch/dch-host-pass/control.sock` when `XDG_RUNTIME_DIR` is unset. It is never
mounted into containers. Each registration retains its own pass executable and
environment and working directory in memory, supporting different password stores/GPG settings. Restarting
the service clears registrations; run Start or `host-pass prepare` for each project
that needs to reconnect. Existing services from older dch versions remain supported;
stop each project's old service and prepare it again to join the shared process.

Socket isolation assumes containers can access only their own socket directory.
It does not isolate projects from other host processes running as the same user,
or from containers allowed to mount other projects' sockets or control Docker.
