package plugins

type Plugin struct {
	ID   string
	Name string
}

func Builtins() []Plugin {
	return []Plugin{{ID: githubAppID, Name: "GitHub App"}}
}
