package compose

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/auro/devlane/internal/manifest"
	"github.com/auro/devlane/internal/util"
)

func BuildCommand(manifest manifest.Manifest, action string, extraProfiles []string) ([]string, error) {
	if len(manifest.Compose.Files) == 0 {
		return nil, fmt.Errorf("adapter does not declare any compose files")
	}

	if manifest.Paths.ComposeEnv == nil {
		return nil, fmt.Errorf("compose env path is missing from manifest")
	}

	command := []string{"docker", "compose", "--env-file", *manifest.Paths.ComposeEnv, "-p", manifest.Network.ProjectName}
	for _, composeFile := range manifest.Compose.Files {
		command = append(command, "-f", composeFile)
	}

	profiles := util.DedupePreserveOrder(append(append([]string{}, manifest.Compose.Profiles...), extraProfiles...))
	for _, profile := range profiles {
		command = append(command, "--profile", profile)
	}

	switch action {
	case "up":
		command = append(command, "up", "-d")
	case "down":
		command = append(command, "down")
	case "status":
		command = append(command, "ps")
	default:
		return nil, fmt.Errorf("unsupported compose action %q", action)
	}

	return command, nil
}

func DockerAvailable() bool {
	_, err := exec.LookPath("docker")
	return err == nil
}

func Run(command []string, cwd string) int {
	cmd := exec.Command(command[0], command[1:]...)
	cmd.Dir = cwd
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return exitErr.ExitCode()
		}

		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	return 0
}
