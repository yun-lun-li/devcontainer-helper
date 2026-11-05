package console

import (
	"errors"
	"os"
	"strings"
	"testing"
)

func TestCheckboxKeysToggleWithoutApplying(t *testing.T) {
	menu := checkboxMenu{labels: []string{"first", "second"}, checked: []bool{false, true}}
	for _, key := range []byte{' ', 'j', ' ', 'k'} {
		apply, err := menu.handleKey(key)
		if apply || err != nil {
			t.Fatalf("key %q applied unexpectedly: %v", key, err)
		}
	}
	if !menu.checked[0] || menu.checked[1] {
		t.Fatalf("wrong checkbox values: %v", menu.checked)
	}
	apply, err := menu.handleKey('\n')
	if !apply || err != nil {
		t.Fatalf("Enter did not apply: %v", err)
	}
	_, err = menu.handleKey('q')
	if !errors.Is(err, errCancelled) {
		t.Fatalf("q did not cancel: %v", err)
	}
}

func TestCheckboxEscapeSequences(t *testing.T) {
	for _, test := range []struct {
		input string
		want  byte
	}{
		{"\x1b[A", 'k'}, {"\x1b[B", 'j'}, {"\x1bOA", 'k'}, {"\x1b", 'q'}, {" ", ' '},
	} {
		key, err := checkboxKey(strings.NewReader(test.input), make(chan os.Signal))
		if err != nil || key != test.want {
			t.Fatalf("%q: got %q (%v)", test.input, key, err)
		}
	}
}
