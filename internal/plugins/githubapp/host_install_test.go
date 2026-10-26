package githubapp

import (
	"context"
	"reflect"
	"testing"
)

func TestInstallationPlanUsesSupportedPackageCommands(t *testing.T) {
	cases := []struct {
		distro, command string
		args            []string
	}{
		{"debian", "apt-get", []string{"install", "gnupg", "pass"}},
		{"fedora", "dnf", []string{"install", "gnupg2", "pass"}},
		{"arch", "pacman", []string{"-S", "--needed", "gnupg", "pass"}},
	}
	for _, test := range cases {
		t.Run(test.distro, func(t *testing.T) {
			plan, err := installationPlanFor([]string{"gpg", "pass"}, "linux", test.distro, true, false)
			if err != nil {
				t.Fatal(err)
			}
			if plan.manager != test.command || !reflect.DeepEqual(plan.args, test.args) {
				t.Fatalf("plan = %#v; want %s %#v", plan, test.command, test.args)
			}
		})
	}
}

func TestHostInstallRequiresConsentBeforeRunningCommand(t *testing.T) {
	plan := packagePlan{manager: "sudo", args: []string{"apt-get", "install", "gnupg", "pass"}}
	ui := &fakeInteraction{denyAt: 1}
	err := confirmHostInstall(context.Background(), ui, ui, plan)
	if err == nil || len(ui.hostCommands) != 0 {
		t.Fatalf("error=%v commands=%#v", err, ui.hostCommands)
	}
}

func TestHostInstallRunsTheConfirmedArgumentVector(t *testing.T) {
	plan := packagePlan{manager: "sudo", args: []string{"apt-get", "install", "gnupg", "pass"}}
	ui := &fakeInteraction{}
	if err := confirmHostInstall(context.Background(), ui, ui, plan); err != nil {
		t.Fatal(err)
	}
	if len(ui.hostCommands) != 1 || ui.hostCommands[0] != "sudo apt-get install gnupg pass" {
		t.Fatalf("commands = %#v", ui.hostCommands)
	}
}

func TestInstallationPlanRequiresSudoForNonRoot(t *testing.T) {
	if _, err := installationPlanFor([]string{"pass"}, "linux", "debian", false, false); err == nil {
		t.Fatal("installation plan succeeded without sudo")
	}
	plan, err := installationPlanFor([]string{"pass"}, "linux", "debian", false, true)
	if err != nil {
		t.Fatal(err)
	}
	if plan.manager != "sudo" || !reflect.DeepEqual(plan.args, []string{"apt-get", "install", "pass"}) {
		t.Fatalf("sudo plan = %#v", plan)
	}
}
