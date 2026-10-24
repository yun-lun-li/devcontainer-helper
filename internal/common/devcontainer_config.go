package common

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// MergedDevcontainerConfig returns the project's base configuration merged with
// its optional first-level JSON fragments from devcontainer.local.d, in filename order.
func MergedDevcontainerConfig(projectPath string) ([]byte, error) {
	configDir := filepath.Join(projectPath, ".devcontainer")
	basePath := filepath.Join(configDir, "devcontainer.json")
	base, err := readJSONCObject(basePath)
	if err != nil {
		return nil, err
	}

	merged := base
	localDir := filepath.Join(configDir, "devcontainer.local.d")
	entries, err := os.ReadDir(localDir)
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("%s: %w", localDir, err)
	}
	for _, entry := range entries {
		if filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		path := filepath.Join(localDir, entry.Name())
		info, err := entry.Info()
		if err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		if !info.Mode().IsRegular() {
			continue
		}
		fragment, err := readJSONCObject(path)
		if err != nil {
			return nil, err
		}
		merged = mergeConfigValues(merged, fragment).(map[string]any)
	}

	var out bytes.Buffer
	encoder := json.NewEncoder(&out)
	encoder.SetIndent("", "  ")
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(merged); err != nil {
		return nil, fmt.Errorf("encode merged devcontainer config: %w", err)
	}
	return out.Bytes(), nil
}

func readJSONCObject(path string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	data, err = jsoncToJSON(data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, fmt.Errorf("%s: invalid JSON: %w", path, err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("%s: invalid JSON: multiple values", path)
		}
		return nil, fmt.Errorf("%s: invalid JSON: %w", path, err)
	}
	object, ok := value.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%s: expected a JSON object", path)
	}
	return object, nil
}

func mergeConfigValues(base, local any) any {
	baseObject, baseIsObject := base.(map[string]any)
	localObject, localIsObject := local.(map[string]any)
	if baseIsObject && localIsObject {
		merged := make(map[string]any, len(baseObject)+len(localObject))
		for key, value := range baseObject {
			merged[key] = value
		}
		for key, value := range localObject {
			if baseValue, exists := baseObject[key]; exists {
				merged[key] = mergeConfigValues(baseValue, value)
			} else {
				merged[key] = value
			}
		}
		return merged
	}

	baseArray, baseIsArray := base.([]any)
	localArray, localIsArray := local.([]any)
	if baseIsArray && localIsArray {
		merged := make([]any, 0, len(baseArray)+len(localArray))
		seen := make(map[string]struct{}, len(baseArray)+len(localArray))
		for _, item := range append(append([]any{}, baseArray...), localArray...) {
			encoded, err := json.Marshal(item)
			if err != nil {
				// Values decoded from JSON are always marshalable.
				continue
			}
			key := string(encoded)
			if _, exists := seen[key]; exists {
				continue
			}
			seen[key] = struct{}{}
			merged = append(merged, item)
		}
		return merged
	}
	return local
}
