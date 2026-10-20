package commands

import (
	"fmt"
	"os"
	"text/tabwriter"
)

func list() error {
	projects, err := projectsForCommand()
	if err != nil {
		return err
	}
	if err := updateStatuses(projects); err != nil {
		fmt.Fprintln(os.Stderr, "Warning: cannot query Docker status:", err)
	}
	printProjects(projects)
	p, err := selectProject("list (details)", projects)
	if err != nil {
		return err
	}
	fmt.Printf("\nName: %s\nPath: %s\nStatus: %s\n", p.Name, p.Path, p.State)
	if p.ID != "" {
		fmt.Printf("Container ID: %s\nContainer: %s\n", p.ID, p.ContainerName)
	}
	return nil
}

func printProjects(projects []Project) {
	w := tabwriter.NewWriter(os.Stdout, 0, 8, 2, ' ', 0)
	fmt.Fprintln(w, "#\tDEVCONTAINER\tSTATUS\tPROJECT PATH")
	for i, p := range projects {
		fmt.Fprintf(w, "%d\t%s\t%s\t%s\n", i+1, p.Name, p.State, p.Path)
	}
	w.Flush()
}

func selectProject(command string, projects []Project) (Project, error) {
	options := make([]string, len(projects))
	for i, p := range projects {
		options[i] = fmt.Sprintf("%s  [%s]  %s", p.Name, p.State, p.Path)
	}
	i, err := choose("Select project for dch "+command, options)
	if err != nil {
		return Project{}, err
	}
	return projects[i], nil
}
