package common

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// MergedDevcontainerConfig returns the project's base configuration merged with
// its optional per-user devcontainer.local.json file.
func MergedDevcontainerConfig(projectPath string) ([]byte, error) {
	configDir := filepath.Join(projectPath, ".devcontainer")
	basePath := filepath.Join(configDir, "devcontainer.json")
	base, err := readJSONCObject(basePath)
	if err != nil {
		return nil, err
	}

	localPath := filepath.Join(configDir, "devcontainer.local.json")
	local, err := readJSONCObject(localPath)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		local = map[string]any{}
	}

	merged := mergeConfigValues(base, local).(map[string]any)
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
	return mergeConfigAt("", base, local)
}

func mergeConfigAt(path string, base, local any) any {
	if isLifecyclePath(path) {
		return local
	}
	baseObject, baseIsObject := base.(map[string]any)
	localObject, localIsObject := local.(map[string]any)
	if baseIsObject && localIsObject {
		return mergeConfigObjects(path, baseObject, localObject)
	}

	baseArray, baseIsArray := base.([]any)
	localArray, localIsArray := local.([]any)
	if baseIsArray && localIsArray {
		return mergeConfigArrays(path, baseArray, localArray)
	}
	return local
}

func mergeConfigObjects(path string, base, local map[string]any) map[string]any {
	merged := make(map[string]any, len(base)+len(local))
	for key, value := range base {
		merged[key] = value
	}
	for key, value := range local {
		if baseValue, exists := base[key]; exists {
			childPath := key
			if path != "" {
				childPath = path + "." + key
			}
			merged[key] = mergeConfigAt(childPath, baseValue, value)
		} else {
			merged[key] = value
		}
	}
	return merged
}

func mergeConfigArrays(path string, base, local []any) []any {
	switch path {
	case "runArgs", "build.options":
		return mergeDockerOptions(base, local, path == "runArgs")
	case "dockerComposeFile":
		return append(append([]any{}, base...), local...)
	case "extensions", "customizations.vscode.extensions", "forwardPorts", "capAdd":
		return uniqueArray(append(append([]any{}, base...), local...))
	default:
		return local
	}
}

func isLifecyclePath(path string) bool {
	switch path {
	case "initializeCommand", "onCreateCommand", "updateContentCommand", "postCreateCommand", "postStartCommand", "postAttachCommand":
		return true
	default:
		return false
	}
}

func uniqueArray(items []any) []any {
	seen := map[string]bool{}
	result := make([]any, 0, len(items))
	for _, item := range items {
		encoded, _ := json.Marshal(item)
		if !seen[string(encoded)] {
			seen[string(encoded)] = true
			result = append(result, item)
		}
	}
	return result
}

type dockerOption struct {
	canonical, kind string
	value           bool
}
type dockerArg struct {
	tokens   []any
	id, kind string
	known    bool
}

func dockerOptions(run bool) map[string]dockerOption {
	table := map[string]dockerOption{}
	add := func(names, canonical, kind string, value bool) {
		for _, name := range strings.Fields(names) {
			table[name] = dockerOption{canonical, kind, value}
		}
	}
	if run {
		add("-a --attach --add-host --blkio-weight-device --gpus --cap-add --cap-drop --device --device-cgroup-rule --device-read-bps --device-read-iops --device-write-bps --device-write-iops --dns --dns-option --dns-search --env-file --expose --group-add --label-file --link-local-ip --mount --network --network-alias --publish -p --security-opt --tmpfs --ulimit --volume -v --volumes-from", "", "append", true)
		add("-e --env -l --label --annotation", "", "key", true)
		add("--log-opt --storage-opt --sysctl", "", "key", true)
		add("--blkio-weight --cgroup-parent --cgroupns --cidfile --cpu-count --cpu-percent --cpu-period --cpu-quota --cpu-rt-period --cpu-rt-runtime -c --cpu-shares --cpus --cpuset-cpus --cpuset-mems --detach-keys --domainname --entrypoint --health-cmd --health-interval --health-retries --health-start-interval --health-start-period --health-timeout -h --hostname --io-maxbandwidth --io-maxiops --ip --ip6 --ipc --isolation --log-driver --mac-address -m --memory --memory-reservation --memory-swap --memory-swappiness --name --oom-score-adj --pid --pids-limit --platform --pull --restart --runtime --shm-size --stop-signal --stop-timeout --umask -u --user --userns --uts --volume-driver -w --workdir", "", "single", true)
		add("-d --detach --init -i --interactive --no-healthcheck --oom-kill-disable --privileged -P --publish-all -q --quiet --read-only --rm --sig-proxy -t --tty --use-api-socket", "", "single", false)
		for short, long := range map[string]string{"-a": "--attach", "-c": "--cpu-shares", "-d": "--detach", "-e": "--env", "-h": "--hostname", "-i": "--interactive", "-l": "--label", "-m": "--memory", "-p": "--publish", "-P": "--publish-all", "-q": "--quiet", "-t": "--tty", "-u": "--user", "-v": "--volume", "-w": "--workdir"} {
			if o, ok := table[long]; ok {
				o.canonical = long
				table[short] = o
			}
		}
	} else {
		add("--build-arg --label --ulimit --add-host --build-context", "", "key", true)
		add("--allow --annotation --attest --cache-from --cache-to --output -o --platform --secret --ssh --tag -t", "", "append", true)
		add("--cgroup-parent --file -f --iidfile --memory --metadata-file --network --progress --provenance --sbom --shm-size --target", "", "single", true)
		table["-f"] = dockerOption{canonical: "--file", kind: "single", value: true}
		add("--no-cache --pull --push --quiet -q --rm --load --check", "", "single", false)
		for short, long := range map[string]string{"-f": "--file", "-t": "--tag", "-q": "--quiet"} {
			if option, ok := table[long]; ok {
				option.canonical = long
				table[short] = option
			}
		}
	}
	return table
}

func mergeDockerOptions(base, local []any, run bool) []any {
	table := dockerOptions(run)
	b, safe := parseDockerArgs(base, table)
	l, _ := parseDockerArgs(local, table)
	if !safe {
		return append(append([]any{}, base...), local...)
	}
	conflicts := map[string]bool{}
	for _, arg := range l {
		if arg.known && arg.kind != "append" {
			conflicts[arg.id] = true
		}
	}
	result := []any{}
	for _, arg := range b {
		if !arg.known || arg.kind == "append" || !conflicts[arg.id] {
			result = append(result, arg.tokens...)
		}
	}
	return append(result, local...)
}

func parseDockerArgs(items []any, table map[string]dockerOption) ([]dockerArg, bool) {
	result := []dockerArg{}
	for i := 0; i < len(items); {
		if token, ok := items[i].(string); ok {
			if bundle, ok := parseShortBooleanBundle(token, table); ok {
				result = append(result, bundle...)
				i++
				continue
			}
		}
		arg, next, ok := parseDockerArg(items, i, table)
		if !ok {
			return result, false
		}
		result = append(result, arg)
		i = next
	}
	return result, true
}

func parseShortBooleanBundle(token string, table map[string]dockerOption) ([]dockerArg, bool) {
	if len(token) < 3 || token[0] != '-' || token[1] == '-' || strings.Contains(token, "=") {
		return nil, false
	}
	args := make([]dockerArg, 0, len(token)-1)
	for _, flag := range token[1:] {
		name := "-" + string(flag)
		option, ok := table[name]
		if !ok || option.value || option.kind != "single" {
			return nil, false
		}
		args = append(args, dockerArg{id: dockerOptionID(name, option), kind: option.kind, known: true, tokens: []any{name}})
	}
	return args, true
}

func dockerOptionID(name string, option dockerOption) string {
	if option.canonical != "" {
		return option.canonical
	}
	return name
}

func parseDockerArg(items []any, index int, table map[string]dockerOption) (dockerArg, int, bool) {
	token, ok := items[index].(string)
	if !ok || !strings.HasPrefix(token, "-") || token == "-" || token == "--" {
		return dockerArg{}, index, false
	}
	name, value, inline := strings.Cut(token, "=")
	option, ok := table[name]
	if !ok && strings.HasPrefix(token, "-") && !strings.HasPrefix(token, "--") && len(token) > 2 {
		name = token[:2]
		option, ok = table[name]
		if ok && option.value {
			value = strings.TrimPrefix(token[2:], "=")
			inline = true
		} else {
			ok = false
		}
	}
	if !ok {
		return dockerArg{}, index, false
	}
	id := dockerOptionID(name, option)
	arg := dockerArg{id: id, kind: option.kind, known: true, tokens: []any{items[index]}}
	if option.value && !inline {
		if index+1 >= len(items) {
			return dockerArg{}, index, false
		}
		index++
		value = fmt.Sprint(items[index])
		arg.tokens = append(arg.tokens, items[index])
	}
	if option.kind == "key" {
		key, _, _ := strings.Cut(value, "=")
		arg.id += ":" + key
	}
	return arg, index + 1, true
}
