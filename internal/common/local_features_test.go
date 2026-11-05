package common

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestEditLocalFeaturePreservesSettingsAndComments(t *testing.T) {
	for _, original := range []string{
		`{ /* keep root */ "name":"demo", "features": { /* keep feature */ "other": {}, }, }`,
		`{ /* keep root */ "name":"demo", "features": { "managed": {}, /* keep feature */ "other": {} } }`,
		`{ /* keep root */ "name":"demo", "features": { /* keep feature */ "other": {}, "managed": {}, } }`,
	} {
		enabled, err := editLocalFeature([]byte(original), "managed", map[string]any{"passEntry": "project/token"})
		if err != nil {
			t.Fatal(err)
		}
		disabled, err := editLocalFeature(enabled, "managed", nil)
		if err != nil {
			t.Fatal(err)
		}
		assertLocalFeatureResult(t, disabled)
	}
}

func assertLocalFeatureResult(t *testing.T, data []byte) {
	t.Helper()
	clean, err := jsoncToJSON(data)
	if err != nil {
		t.Fatal(err)
	}
	var config struct {
		Name     string
		Features map[string]any
	}
	if err := json.Unmarshal(clean, &config); err != nil {
		t.Fatalf("%s: %v", data, err)
	}
	if config.Name != "demo" || len(config.Features) != 1 || config.Features["other"] == nil {
		t.Fatalf("unexpected settings: %s", data)
	}
	for _, comment := range []string{"/* keep root */", "/* keep feature */"} {
		if !strings.Contains(string(data), comment) {
			t.Fatalf("lost comment: %s", data)
		}
	}
}

func TestRemoveOnlyFeatureWithTrailingComma(t *testing.T) {
	for _, original := range []string{
		`{"features":{"managed":{},}}`,
		"{\"features\":{\"managed\":{} /* keep, comment */ , // keep line\n}}",
		`{"features":{"managed":{}}}`,
	} {
		data, err := editLocalFeature([]byte(original), "managed", nil)
		if err != nil {
			t.Fatal(err)
		}
		clean, err := jsoncToJSON(data)
		if err != nil || !json.Valid(clean) {
			t.Fatalf("invalid result: %s (%v)", data, err)
		}
		if strings.Contains(string(data), "managed") {
			t.Fatalf("feature remains: %s", data)
		}
	}
}

func TestEditLocalFeatureRejectsInvalidConfiguration(t *testing.T) {
	for _, original := range []string{`{"features":null}`, `{"features":[]}`, `{"features":{},`, `{} {}`} {
		if _, err := editLocalFeature([]byte(original), "managed", map[string]any{}); err == nil {
			t.Fatalf("accepted %s", original)
		}
	}
}
