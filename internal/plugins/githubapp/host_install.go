package githubapp

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
)

type packagePlan struct {
	manager string
	args    []string
}

func installationPlan(missing []string) (packagePlan, error) {
	return installationPlanFor(missing, runtime.GOOS, linuxDistribution(), os.Geteuid() == 0, commandAvailable("sudo"))
}

func installationPlanFor(missing []string, operatingSystem, distribution string, root, sudoAvailable bool) (packagePlan, error) {
	if len(missing) == 0 {
		return packagePlan{}, nil
	}
	plan, err := packageManagerPlanFor(operatingSystem, distribution)
	if err != nil {
		return packagePlan{}, err
	}
	plan.args = append(plan.args, packageNames(missing, plan.manager)...)
	if !root {
		if !sudoAvailable {
			return packagePlan{}, fmt.Errorf("install gpg and pass manually; sudo is unavailable")
		}
		plan.args = append([]string{plan.manager}, plan.args...)
		plan.manager = "sudo"
	}
	return plan, nil
}

func packageManagerPlanFor(operatingSystem, distribution string) (packagePlan, error) {
	plan := packagePlan{}
	switch operatingSystem {
	case "linux":
		switch distribution {
		case "debian":
			plan = packagePlan{manager: "apt-get", args: []string{"install"}}
		case "fedora":
			plan = packagePlan{manager: "dnf", args: []string{"install"}}
		case "arch":
			plan = packagePlan{manager: "pacman", args: []string{"-S", "--needed"}}
		default:
			return packagePlan{}, fmt.Errorf("install gpg and pass manually; this Linux distribution is not supported")
		}
	default:
		return packagePlan{}, fmt.Errorf("install gpg and pass manually; this operating system is not supported")
	}
	return plan, nil
}

func linuxDistribution() string {
	if fileExists("/etc/debian_version") {
		return "debian"
	}
	if releaseExists("/etc/fedora-release", "/etc/redhat-release") {
		return "fedora"
	}
	if fileExists("/etc/arch-release") {
		return "arch"
	}
	return ""
}

func fileExists(path string) bool       { _, err := os.Stat(path); return err == nil }
func commandAvailable(name string) bool { _, err := exec.LookPath(name); return err == nil }

func packageNames(missing []string, manager string) []string {
	packages := make([]string, 0, len(missing))
	for _, executable := range missing {
		packageName := executable
		if executable == "gpg" {
			if manager == "dnf" {
				packageName = "gnupg2"
			} else {
				packageName = "gnupg"
			}
		}
		packages = append(packages, packageName)
	}
	return packages
}

func releaseExists(paths ...string) bool {
	for _, path := range paths {
		if _, err := os.Stat(path); err == nil {
			return true
		}
	}
	return false
}
