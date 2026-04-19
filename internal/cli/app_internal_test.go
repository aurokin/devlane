package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/auro/devlane/internal/portalloc"
	"github.com/auro/devlane/internal/testutil"
)

func TestPrepareRollsBackLocalStateWhenCatalogPublishFails(t *testing.T) {
	repo := testutil.InitDemoRepo(t)
	sharedConfig := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", sharedConfig)

	originalPublish := publishPrepareSession
	publishPrepareSession = func(*portalloc.PrepareSession) error {
		return errors.New("publish catalog: forced failure")
	}
	defer func() {
		publishPrepareSession = originalPublish
	}()

	code, _, stderr := runCLIInternal(t, []string{
		"prepare",
		"--cwd", repo,
		"--config", filepath.Join(repo, "devlane.yaml"),
	})
	if code != 1 {
		t.Fatalf("expected exit code 1, got %d\nstderr:\n%s", code, stderr)
	}
	if !strings.Contains(stderr, "forced failure") {
		t.Fatalf("expected forced commit failure, got stderr:\n%s", stderr)
	}

	for _, path := range []string{
		filepath.Join(repo, ".devlane", "manifest.json"),
		filepath.Join(repo, ".devlane", "compose.env"),
		filepath.Join(repo, ".devlane", "generated", "app.env"),
	} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("expected rollback to remove %s, stat err=%v", path, err)
		}
	}

	assertCatalogAllocationsInternal(t, filepath.Join(sharedConfig, "devlane", "catalog.json"), 0)
}

func TestPrepareKeepsLocalStateWhenCatalogLockReleaseFailsAfterPublish(t *testing.T) {
	repo := testutil.InitDemoRepo(t)
	sharedConfig := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", sharedConfig)

	originalClose := closePrepareSession
	closePrepareSession = func(*portalloc.PrepareSession) error {
		return errors.New("release catalog lock: forced failure")
	}
	defer func() {
		closePrepareSession = originalClose
	}()

	code, _, stderr := runCLIInternal(t, []string{
		"prepare",
		"--cwd", repo,
		"--config", filepath.Join(repo, "devlane.yaml"),
	})
	if code != 1 {
		t.Fatalf("expected exit code 1, got %d\nstderr:\n%s", code, stderr)
	}
	if !strings.Contains(stderr, "published catalog state") {
		t.Fatalf("expected publish-success guidance, got stderr:\n%s", stderr)
	}
	if !strings.Contains(stderr, "forced failure") {
		t.Fatalf("expected forced close failure, got stderr:\n%s", stderr)
	}

	for _, path := range []string{
		filepath.Join(repo, ".devlane", "manifest.json"),
		filepath.Join(repo, ".devlane", "compose.env"),
		filepath.Join(repo, ".devlane", "generated", "app.env"),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected local prepare state to remain at %s, stat err=%v", path, err)
		}
	}

	assertCatalogAllocationsInternal(t, filepath.Join(sharedConfig, "devlane", "catalog.json"), 1)
}

func runCLIInternal(t *testing.T, args []string) (int, string, string) {
	t.Helper()

	originalStdout := os.Stdout
	originalStderr := os.Stderr
	originalStdin := os.Stdin

	stdoutReader, stdoutWriter, err := os.Pipe()
	if err != nil {
		t.Fatalf("stdout pipe: %v", err)
	}
	stderrReader, stderrWriter, err := os.Pipe()
	if err != nil {
		t.Fatalf("stderr pipe: %v", err)
	}
	stdinReader, stdinWriter, err := os.Pipe()
	if err != nil {
		t.Fatalf("stdin pipe: %v", err)
	}
	_ = stdinWriter.Close()

	os.Stdout = stdoutWriter
	os.Stderr = stderrWriter
	os.Stdin = stdinReader

	code := Run(args)

	_ = stdoutWriter.Close()
	_ = stderrWriter.Close()
	os.Stdout = originalStdout
	os.Stderr = originalStderr
	os.Stdin = originalStdin

	return code, readPipeInternal(t, stdoutReader), readPipeInternal(t, stderrReader)
}

func readPipeInternal(t *testing.T, reader *os.File) string {
	t.Helper()

	var buffer bytes.Buffer
	if _, err := buffer.ReadFrom(reader); err != nil {
		t.Fatalf("read pipe: %v", err)
	}
	return buffer.String()
}

func assertCatalogAllocationsInternal(t *testing.T, path string, want int) {
	t.Helper()

	payload, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		if want == 0 {
			return
		}
		t.Fatalf("expected catalog at %s", path)
	}
	if err != nil {
		t.Fatalf("read catalog: %v", err)
	}

	var stored struct {
		Allocations []any `json:"allocations"`
	}
	if err := json.Unmarshal(payload, &stored); err != nil {
		t.Fatalf("decode catalog: %v", err)
	}
	if len(stored.Allocations) != want {
		t.Fatalf("expected %d catalog allocations, got %d", want, len(stored.Allocations))
	}
}
