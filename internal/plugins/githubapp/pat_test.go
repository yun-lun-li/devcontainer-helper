package githubapp

import (
	"strings"
	"testing"
)

func TestFineGrainedPATRejectsValuesLargerThanInputLimit(t *testing.T) {
	token := "github_pat_" + strings.Repeat("a", 4096)
	if isFineGrainedPAT(token) {
		t.Fatal("PAT larger than the input limit was accepted")
	}
}
