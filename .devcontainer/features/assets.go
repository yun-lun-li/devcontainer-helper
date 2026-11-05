// Package featureassets bundles the local Features distributed with dch.
package featureassets

import "embed"

// GitHubCLI contains the GitHub CLI Feature and its installation scripts.
//
//go:embed github-cli/*
var GitHubCLI embed.FS
