package commands

func buildProject(p Project) error {
	if err := requireExecutable("devcontainer"); err != nil {
		return err
	}
	mode, err := choose("Build mode", []string{
		"Rebuild and RECREATE container (no cache; starts it)",
		"Build image only (no cache; does NOT replace existing container)",
	})
	if err != nil {
		return err
	}
	if mode == 1 {
		return invokeDevcontainer("build", "--workspace-folder", p.Path, "--no-cache")
	}
	ok, err := confirm("Remove the existing devcontainer (if any) and rebuild from scratch? Unsaved changes inside the container may be lost.", false)
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}
	return invokeDevcontainer("up", "--workspace-folder", p.Path, "--remove-existing-container", "--build-no-cache")
}
