package common

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
)

type jsonProperty struct {
	name                   string
	start, valueStart, end int
}

func jsonProperties(data []byte) ([]jsonProperty, []byte, error) {
	clean, err := jsoncToJSON(data)
	if err != nil {
		return nil, nil, err
	}
	decoder := json.NewDecoder(bytes.NewReader(clean))
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		return nil, nil, fmt.Errorf("expected a JSON object")
	}
	var properties []jsonProperty
	for decoder.More() {
		before := int(decoder.InputOffset())
		key, err := decoder.Token()
		if err != nil {
			return nil, nil, err
		}
		start := before + bytes.IndexByte(clean[before:int(decoder.InputOffset())], '"')
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			return nil, nil, err
		}
		end := int(decoder.InputOffset())
		properties = append(properties, jsonProperty{key.(string), start, end - len(value), end})
	}
	if _, err := decoder.Token(); err != nil {
		return nil, nil, err
	}
	if !json.Valid(clean) {
		return nil, nil, fmt.Errorf("invalid JSON object")
	}
	return properties, clean, nil
}

// editJSONProperty retains comments and formatting outside the edited value.
func editJSONProperty(data []byte, key string, value []byte) ([]byte, error) {
	properties, clean, err := jsonProperties(data)
	if err != nil {
		return nil, err
	}
	for _, property := range properties {
		if property.name != key {
			continue
		}
		if value != nil {
			return spliceJSON(data, property.valueStart, property.end, value), nil
		}
		return removeJSONProperty(data, clean, property), nil
	}
	if value == nil {
		return data, nil
	}
	encodedKey, _ := json.Marshal(key)
	insert := append(append(encodedKey, ':', ' '), value...)
	if len(properties) > 0 {
		insert = append(insert, ',')
	}
	insert = append([]byte("\n  "), insert...)
	start := bytes.IndexByte(clean, '{') + 1
	return spliceJSON(data, start, start, insert), nil
}

var followingComma = regexp.MustCompile(`(?s)^(?:\s|//[^\r\n]*|/\*.*?\*/)*,`)

func removeJSONProperty(data, clean []byte, property jsonProperty) []byte {
	// Find the comma in the original suffix, including JSONC trailing commas.
	if match := followingComma.FindIndex(data[property.end:]); match != nil {
		comma := property.end + match[1] - 1
		data = spliceJSON(data, comma, comma+1, nil)
		return spliceJSON(data, property.start, property.end, nil)
	}
	data = spliceJSON(data, property.start, property.end, nil)
	previous := property.start - 1
	for previous >= 0 && bytes.ContainsRune([]byte(" \t\r\n"), rune(clean[previous])) {
		previous--
	}
	if previous >= 0 && clean[previous] == ',' {
		data = spliceJSON(data, previous, previous+1, nil)
	}
	return data
}

func spliceJSON(data []byte, start, end int, value []byte) []byte {
	result := append([]byte{}, data[:start]...)
	result = append(result, value...)
	return append(result, data[end:]...)
}

// SetLocalFeature updates one local Feature without changing other settings.
// A nil options value removes the Feature from the local configuration.
func SetLocalFeature(project, feature string, options map[string]any) error {
	base, err := readJSONCObject(filepath.Join(project, ".devcontainer", "devcontainer.json"))
	if err != nil {
		return err
	}
	if features, ok := base["features"].(map[string]any); ok && features[feature] != nil {
		return fmt.Errorf("Feature is configured in devcontainer.json; manage it there instead")
	}
	path := filepath.Join(project, ".devcontainer", "devcontainer.local.json")
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		data, err = []byte("{}\n"), nil
	}
	if err != nil {
		return err
	}
	updated, err := editLocalFeature(data, feature, options)
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	return writeLocalConfig(path, updated)
}

func editLocalFeature(data []byte, feature string, options map[string]any) ([]byte, error) {
	properties, _, err := jsonProperties(data)
	if err != nil {
		return nil, err
	}
	features := []byte("{}")
	for _, property := range properties {
		if property.name == "features" {
			features = data[property.valueStart:property.end]
		}
	}
	var value []byte
	if options != nil {
		value, err = json.Marshal(options)
		if err != nil {
			return nil, err
		}
	}
	features, err = editJSONProperty(features, feature, value)
	if err != nil {
		return nil, fmt.Errorf("features: %w", err)
	}
	return editJSONProperty(data, "features", features)
}

func writeLocalConfig(path string, data []byte) error {
	file, err := os.CreateTemp(filepath.Dir(path), ".dch-local-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if _, err := file.Write(data); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(file.Name(), path)
}
