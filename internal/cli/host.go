package cli

import (
	"flag"
	"fmt"
	"os"
	"sort"
	"text/tabwriter"

	"github.com/auro/devlane/internal/portalloc"
)

func runHost(args []string) int {
	if len(args) == 0 {
		printHostUsage()
		return 2
	}

	switch args[0] {
	case "status":
		return runHostStatus(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "unknown host subcommand %q\n", args[0])
		printHostUsage()
		return 2
	}
}

func printHostUsage() {
	fmt.Fprintln(os.Stderr, "usage: devlane host <status> [flags]")
}

func runHostStatus(args []string) int {
	fs := flag.NewFlagSet("host status", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() > 0 {
		fmt.Fprintf(os.Stderr, "host status takes no positional arguments\n")
		return 2
	}

	rows, err := portalloc.List()
	if err != nil {
		return exitError(err)
	}

	if len(rows) == 0 {
		fmt.Println("no allocations")
		return 0
	}

	sort.Slice(rows, func(i, j int) bool {
		left := rows[i]
		right := rows[j]
		if left.App != right.App {
			return left.App < right.App
		}
		if left.RepoPath != right.RepoPath {
			return left.RepoPath < right.RepoPath
		}
		return left.Service < right.Service
	})

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "APP\tMODE\tLANE\tSERVICE\tPORT\tREPO PATH")
	for _, row := range rows {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%d\t%s\n",
			row.App, row.Mode, row.Lane, row.Service, row.Port, row.RepoPath)
	}
	if err := w.Flush(); err != nil {
		return exitError(err)
	}
	return 0
}
