# GitHub CLI with host pass

This Feature installs the official GitHub CLI dependency and connects `gh` and
HTTPS Git operations on github.com to a PAT held in **host** pass.

Use dch Start/Build to run `customizations.dch.initializeCommand`. Other tools do
not run this host extension automatically. See the repository README's
"GitHub CLI with a PAT in host pass" section for setup, scope, and cleanup.

`passEntry` defaults to `dch/github/pat`. It is an entry name, never a token.
The container must already have Git and curl; host pass must already be initialized.
The supported first version uses Linux, local Docker, and matching host/container
UIDs. The socket exposes the configured PAT to processes in this trusted container.
