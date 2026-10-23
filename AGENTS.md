# Repository Guidelines

## Project Structure & Module Organization

This repository is a single Go module (`dch`) with `main.go` as the CLI entry point. Shared project metadata, configuration, and Docker status logic live in `internal/common`; command dispatch and non-interactive commands are in `internal/commands`; terminal screens and container actions are in `internal/console`. Tests sit beside the code they cover in `*_test.go` files. There are no generated assets or separate test directories.

## Build, Test, and Development Commands

- `make build` compiles the app to `./dch`.
- `make test` runs all Go tests with `go test ./...`.
- `make install` builds and installs the binary to `$(BINDIR)` (default: `~/.local/bin`). Override `PREFIX` or `BINDIR` for a different location.
- `go run . help` runs the CLI from source and prints command help. Docker and the Dev Container CLI are needed to exercise container operations.

## Coding Style & Naming Conventions

Use standard Go formatting (`gofmt`) and idiomatic Go names: exported identifiers use `MixedCaps`, unexported identifiers use `mixedCaps`, and package names are short and lowercase. Keep shared behavior in `internal/common` rather than duplicating it across commands and console screens. Avoid adding dependencies unless the standard library cannot reasonably meet the need; the module currently has no third-party Go dependencies.

## Testing Guidelines

Use Go's built-in `testing` package. Name test files `*_test.go` and test functions `TestThing`; add or update tests alongside the package behavior being changed. Run `make test` (or `go test ./...`) before submitting. Tests should not require a live Docker daemon unless explicitly designed as integration tests.

## Commit & Pull Request Guidelines

Recent commits use concise imperative subjects, sometimes with a scope such as `fix(codex-feature): ...`. Keep each commit focused and describe the user-visible change. Pull requests should explain the behavior and motivation, list relevant checks (for example, `make test`), and include terminal screenshots or captured output when changing the console UI. Link related issues when applicable.

## Configuration & Safety

The app reads project paths from `~/.dch/config`; project metadata comes from each project's `.devcontainer/devcontainer.json`. Do not commit local configuration, credentials, or machine-specific paths. Container rebuild and removal flows can discard data in a container's writable layer, so preserve the existing confirmation behavior when changing those operations.
