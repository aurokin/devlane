package gitutil

import (
	"bytes"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

func runGit(cwd string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = cwd

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}

	return strings.TrimSpace(stdout.String()), nil
}

func FindRepoRoot(cwd string) string {
	root, ok := FindRepoRootOK(cwd)
	if !ok {
		return filepath.Clean(cwd)
	}

	return root
}

func FindRepoRootOK(cwd string) (string, bool) {
	root, err := runGit(cwd, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", false
	}

	return filepath.Clean(root), true
}

func CurrentBranch(cwd string) string {
	branch, err := runGit(cwd, "branch", "--show-current")
	if err != nil || branch == "" {
		return "detached"
	}

	return branch
}
