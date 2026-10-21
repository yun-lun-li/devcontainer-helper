package commands

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
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
	return nil
}

func printProjects(projects []Project) {
	info, err := os.Stdout.Stat()
	color := err == nil && info.Mode()&os.ModeCharDevice != 0 && os.Getenv("NO_COLOR") == "" && os.Getenv("TERM") != "dumb"
	renderProjects(os.Stdout, projects, color)
}

func renderProjects(out io.Writer, projects []Project, color bool) {
	var plain bytes.Buffer
	w := tabwriter.NewWriter(&plain, 0, 8, 2, ' ', 0)
	fmt.Fprintln(w, "#\tDEVCONTAINER\tSTATUS\tPROJECT PATH\tCONTAINER ID")
	for i, p := range projects {
		id := p.ID
		if id == "" {
			id = "-"
		}
		fmt.Fprintf(w, "%d\t%s\t%s\t%s\t%s\n", i+1, p.Name, p.State, p.Path, id)
	}
	w.Flush()
	if !color {
		fmt.Fprint(out, plain.String())
		return
	}
	lines := strings.Split(strings.TrimSuffix(plain.String(), "\n"), "\n")
	fmt.Fprintf(out, "\x1b[1;36m%s\x1b[0m\n", lines[0])
	for i, p := range projects {
		row := lines[i+1]
		start := len(strconv.Itoa(i + 1))
		nameOffset := strings.Index(row[start:], p.Name)
		if nameOffset < 0 {
			fmt.Fprintln(out, row)
			continue
		}
		stateOffset := start + nameOffset + len(p.Name)
		stateIndex := strings.Index(row[stateOffset:], p.State)
		if stateIndex < 0 {
			fmt.Fprintln(out, row)
			continue
		}
		stateStart := stateOffset + stateIndex
		stateEnd := stateStart + len(p.State)
		fmt.Fprint(out, row[:stateStart], statusColor(p.State), p.State, "\x1b[0m")
		if p.ID == "" {
			fmt.Fprintln(out, row[stateEnd:])
			continue
		}
		idStart := strings.LastIndex(row, p.ID)
		if idStart < stateEnd {
			fmt.Fprintln(out, row[stateEnd:])
			continue
		}
		fmt.Fprint(out, row[stateEnd:idStart], "\x1b[36m", p.ID, "\x1b[0m", row[idStart+len(p.ID):], "\n")
	}
}

func statusColor(state string) string {
	switch {
	case strings.HasPrefix(state, "RUNNING"):
		return "\x1b[94m"
	case strings.HasPrefix(state, "STOPPED"), state == "UNKNOWN":
		return "\x1b[33m"
	case state == "CONFIG MISSING/INVALID":
		return "\x1b[31m"
	default:
		return "\x1b[90m"
	}
}
