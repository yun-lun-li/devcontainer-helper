package commands

func up() error {
	p, err := selectRunnableProject("up")
	if err != nil {
		return err
	}
	return invokeDevcontainer("up", "--workspace-folder", p.Path)
}
