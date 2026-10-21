package commands

func startProject(p Project) error {
	if err := requireExecutable("devcontainer"); err != nil {
		return err
	}
	return invokeDevcontainer("up", "--workspace-folder", p.Path)
}
