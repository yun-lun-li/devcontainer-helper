package console

import (
	"io"
	"path/filepath"
	"strings"
	"testing"
)

type readerFunc func([]byte) (int, error)

func (f readerFunc) Read(p []byte) (int, error) { return f(p) }

func TestReadTTYByteWaitsAfterTemporaryEOF(t *testing.T) {
	reads := 0
	r := readerFunc(func(p []byte) (int, error) {
		reads++
		if reads == 1 {
			return 0, io.EOF
		}
		p[0] = 'q'
		return 1, nil
	})

	b, err := readTTYByte(r, 0, nil)
	if err != nil || b != 'q' || reads != 2 {
		t.Fatalf("got byte %q, error %v, reads %d", b, err, reads)
	}
}

func TestRunWithNoRegisteredProjects(t *testing.T) {
	t.Setenv("DCH_CONFIG", filepath.Join(t.TempDir(), "config"))
	if err := Run(newConsoleTestManager(t)); err != nil {
		t.Fatalf("console returned %v", err)
	}
}

func TestProjectCardShowsColoredStateAndPath(t *testing.T) {
	p := Project{Name: "aol-external-dev-container", State: "RUNNING", Path: "/home/alanli/Projects/aol-svr-external"}
	var colored strings.Builder
	printProjectHeader(&colored, p, true, 76)
	for _, want := range []string{"◆ ", p.Name, cardBlue + "● RUNNING", cardCyan + "🗁 " + p.Path, "ACTIONS"} {
		if !strings.Contains(colored.String(), want) {
			t.Errorf("colored card missing %q", want)
		}
	}
	var plain strings.Builder
	printProjectHeader(&plain, p, false, 40)
	if strings.Contains(plain.String(), "\x1b[") || !strings.Contains(plain.String(), "🗁 ") {
		t.Errorf("plain card should show the folder icon without ANSI colors: %q", plain.String())
	}
}

func TestProjectOptionsHaveDistinctColors(t *testing.T) {
	cases := []struct{ option, color string }{
		{"↗ Connect  Open a shell", cardCyan},
		{"■ Stop  Stop the container", cardAmber},
		{"▣ Build  Rebuild the image", cardPurple},
		{"✕ Remove Project  Unregister only", cardPink},
	}
	for _, tc := range cases {
		if got := projectOption(tc.option, false, true, 76); !strings.HasPrefix(got, tc.color) {
			t.Errorf("%q uses wrong color: %q", tc.option, got)
		}
	}
	if got := projectOption(cases[0].option, true, true, 76); !strings.HasPrefix(got, cardSelect) {
		t.Errorf("selected option lacks highlight: %q", got)
	}
}
