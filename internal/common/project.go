package common

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Project struct {
	Path, Name, State, ID, ContainerName string
	Valid                                bool
}

// jsoncToJSON blanks comments and trailing commas without modifying string literals.
// The devcontainer.json format permits both (JSON with Comments / JSONC).
func jsoncToJSON(in []byte) ([]byte, error) {
	out := bytes.Clone(in)
	inString, escape := false, false
	for i := 0; i < len(out); i++ {
		c := out[i]
		if inString {
			if escape {
				escape = false
			} else if c == '\\' {
				escape = true
			} else if c == '"' {
				inString = false
			}
			continue
		}
		if c == '"' {
			inString = true
			continue
		}
		if c == '/' && i+1 < len(out) && out[i+1] == '/' {
			for i < len(out) && out[i] != '\n' && out[i] != '\r' {
				out[i] = ' '
				i++
			}
			continue
		}
		if c == '/' && i+1 < len(out) && out[i+1] == '*' {
			start := i
			out[i], out[i+1] = ' ', ' '
			i += 2
			closed := false
			for i+1 < len(out) {
				if out[i] == '*' && out[i+1] == '/' {
					out[i], out[i+1] = ' ', ' '
					i++
					closed = true
					break
				}
				if out[i] != '\n' && out[i] != '\r' {
					out[i] = ' '
				}
				i++
			}
			if !closed {
				return nil, fmt.Errorf("unterminated block comment at offset %d", start)
			}
			continue
		}
	}
	inString, escape = false, false
	for i := 0; i < len(out); i++ {
		if inString {
			if escape {
				escape = false
			} else if out[i] == '\\' {
				escape = true
			} else if out[i] == '"' {
				inString = false
			}
			continue
		}
		if out[i] == '"' {
			inString = true
			continue
		}
		if out[i] != ',' {
			continue
		}
		j := i + 1
		for j < len(out) && (out[j] == ' ' || out[j] == '\t' || out[j] == '\r' || out[j] == '\n') {
			j++
		}
		if j < len(out) && (out[j] == '}' || out[j] == ']') {
			out[i] = ' '
		}
	}
	return out, nil
}

func ProjectName(path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("not a directory: %s", path)
	}
	p := filepath.Join(path, ".devcontainer", "devcontainer.json")
	data, err := os.ReadFile(p)
	if err != nil {
		return "", fmt.Errorf("%s: %w", p, err)
	}
	data, err = jsoncToJSON(data)
	if err != nil {
		return "", fmt.Errorf("%s: %w", p, err)
	}
	var cfg struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return "", fmt.Errorf("invalid devcontainer.json: %w", err)
	}
	if strings.TrimSpace(cfg.Name) == "" {
		return filepath.Base(path), nil
	}
	return cfg.Name, nil
}

func LoadProjects() ([]Project, error) {
	cfg, err := LoadConfig()
	if err != nil {
		return nil, err
	}
	projects := make([]Project, 0, len(cfg.Projects))
	for _, p := range cfg.Projects {
		pr := Project{Path: p, Name: filepath.Base(p), State: "NOT CREATED"}
		if name, err := ProjectName(p); err == nil {
			pr.Name, pr.Valid = name, true
		} else {
			pr.State = "CONFIG MISSING/INVALID"
		}
		projects = append(projects, pr)
	}
	return projects, nil
}
