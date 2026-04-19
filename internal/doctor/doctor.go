package doctor

import (
	"fmt"
	"os"

	"github.com/auro/devlane/internal/compose"
	"github.com/auro/devlane/internal/config"
)

type Result struct {
	Messages []string
	Failed   bool
}

func Run(adapter *config.AdapterConfig, composeFiles []string, configPath string) Result {
	messages := make([]string, 0, 8)
	failed := false

	messages = append(messages, fmt.Sprintf("ok: config exists at %s", configPath))

	if adapter.HasCompose() {
		messages = append(messages, "ok: adapter declares compose files")

		if err := compose.DockerComposeAvailable(); err == nil {
			messages = append(messages, "ok: docker compose available")
		} else {
			messages = append(messages, fmt.Sprintf("fail: docker compose unavailable: %v", err))
			failed = true
		}

		for _, composeFile := range composeFiles {
			info, err := os.Stat(composeFile)
			if err != nil {
				messages = append(messages, fmt.Sprintf("fail: compose file missing: %s", composeFile))
				failed = true
				continue
			}
			if info.IsDir() {
				messages = append(messages, fmt.Sprintf("fail: compose file is a directory: %s", composeFile))
				failed = true
				continue
			}

			messages = append(messages, fmt.Sprintf("ok: compose file exists: %s", composeFile))
		}
	} else {
		messages = append(messages, "info: adapter does not declare compose files")
	}

	if len(adapter.Outputs.Generated) > 0 {
		messages = append(messages, fmt.Sprintf("ok: adapter declares %d generated output(s)", len(adapter.Outputs.Generated)))
	} else {
		messages = append(messages, "info: adapter declares no generated outputs")
	}

	if adapter.HasRunCommands() {
		messages = append(messages, fmt.Sprintf("ok: adapter declares %d bare-metal run command(s)", len(adapter.Runtime.Run.Commands)))
	} else {
		messages = append(messages, "info: adapter declares no bare-metal run commands")
	}

	return Result{
		Messages: messages,
		Failed:   failed,
	}
}
