package commands

import (
	"strings"
	"testing"
)

func TestHostPassCommandRejectsInvalidInvocations(t *testing.T) {
	cases := []struct {
		args    []string
		message string
	}{
		{[]string{"host-pass"}, "usage:"},
		{[]string{"host-pass", "prepare"}, "requires --project"},
		{[]string{"host-pass", "prepare", "--project", "/tmp/project", "extra"}, "no positional"},
		{[]string{"host-pass", "unknown", "--project", "/tmp/project"}, "unknown host-pass action"},
		{[]string{"host-pass", "serve", "--project", "/tmp/project", "--entry", "../other"}, "relative name"},
	}
	for _, test := range cases {
		err := Run(test.args, nil)
		if err == nil || !strings.Contains(err.Error(), test.message) {
			t.Fatalf("args %v: error = %v", test.args, err)
		}
	}
}
