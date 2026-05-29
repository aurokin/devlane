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

// IsValidBranchName reports whether name is a well-formed local branch name.
// It defers to `git check-ref-format`, the same validation git itself applies,
// rather than reimplementing the ref-name rules.
func IsValidBranchName(repoDir, name string) bool {
	_, err := runGit(repoDir, "check-ref-format", "refs/heads/"+name)
	return err == nil
}

// BranchExists reports whether a local branch named branch already exists.
func BranchExists(repoDir, branch string) bool {
	_, err := runGit(repoDir, "rev-parse", "--verify", "--quiet", "refs/heads/"+branch)
	return err == nil
}

// AddWorktree creates a new worktree at path on a new branch created from
// commitish (typically "HEAD"). It fails if the branch or path already exists,
// surfacing git's own error.
func AddWorktree(repoDir, path, branch, commitish string) error {
	_, err := runGit(repoDir, "worktree", "add", "-b", branch, path, commitish)
	return err
}

// RemoveWorktree removes the worktree at path. Without force, git refuses when
// the worktree has uncommitted or untracked changes; force discards them.
func RemoveWorktree(repoDir, path string, force bool) error {
	args := []string{"worktree", "remove"}
	if force {
		args = append(args, "--force")
	}
	args = append(args, path)
	_, err := runGit(repoDir, args...)
	return err
}
