package commands

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"dch/internal/hostpass"
)

func runHostPass(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: dch host-pass prepare|serve|stop --project PATH [--entry NAME]")
	}
	flags := flag.NewFlagSet("host-pass", flag.ContinueOnError)
	project := flags.String("project", "", "project directory")
	entry := flags.String("entry", "dch/github/pat", "host pass entry containing the GitHub PAT")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	if (*project == "" && args[0] != "daemon") || flags.NArg() != 0 {
		return fmt.Errorf("host-pass requires --project PATH and no positional arguments")
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	switch args[0] {
	case "daemon":
		return hostpass.ServeShared(ctx)
	case "prepare":
		return hostpass.Prepare(ctx, *project, *entry, os.Stdin, os.Stdout, os.Stderr)
	case "serve":
		return hostpass.Serve(ctx, *project, *entry)
	case "stop":
		return hostpass.Stop(ctx, *project)
	default:
		return fmt.Errorf("unknown host-pass action %q", args[0])
	}
}
