package console

func startProject(p Project) error {
	if err := requireExecutable("devcontainer"); err != nil {
		return err
	}
	return invokeProjectDevcontainer(p, "up", "--workspace-folder", p.Path)
}
