package console

func buildProject(p Project) error {
	if err := requireExecutable("devcontainer"); err != nil {
		return err
	}
	cacheChoice, err := choose("Build cache", []string{
		"Use build cache",
		"Build without cache",
	})
	if err != nil {
		return err
	}
	args := []string{"up", "--workspace-folder", p.Path, "--remove-existing-container"}
	if cacheChoice == 1 {
		args = append(args, "--build-no-cache")
	}
	return invokeProjectDevcontainer(p, args...)
}
