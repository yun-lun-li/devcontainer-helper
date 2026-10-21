package commands

func up() error {
	p, err := selectRunnableProject("up")
	if err != nil {
		return err
	}
	return startProject(p)
}

func startProject(p Project) error {
	if err := requireExecutable("devcontainer"); err != nil {
		return err
	}
	return invokeDevcontainer("up", "--workspace-folder", p.Path)
}
