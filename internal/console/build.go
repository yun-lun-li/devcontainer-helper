package console

func buildProject(p Project) error {
	if err := requireExecutable("devcontainer"); err != nil {
		return err
	}
	mode, err := choose("Build mode", []string{
		"Rebuild and RECREATE container (starts it)",
		"Build image only (does NOT replace existing container)",
	})
	if err != nil {
		return err
	}
	useCache, err := confirm("Use build cache?", true)
	if err != nil {
		return err
	}
	if mode == 1 {
		args := []string{"build", "--workspace-folder", p.Path}
		if !useCache {
			args = append(args, "--no-cache")
		}
		return invokeDevcontainer(args...)
	}
	ok, err := confirm("Remove the existing devcontainer (if any) and rebuild from scratch? Unsaved changes inside the container may be lost.", false)
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}
	args := []string{"up", "--workspace-folder", p.Path, "--remove-existing-container"}
	if !useCache {
		args = append(args, "--build-no-cache")
	}
	return invokeDevcontainer(args...)
}
