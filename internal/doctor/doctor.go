package doctor

import (
	"fmt"

	"github.com/auro/devlane/internal/compose"
	"github.com/auro/devlane/internal/config"
)

func Run(adapter *config.AdapterConfig, configPath string) []string {
	messages := make([]string, 0, 4)

	messages = append(messages, fmt.Sprintf("ok: config exists at %s", configPath))

	if adapter.HasCompose() {
		messages = append(messages, "ok: adapter declares compose files")
		if compose.DockerAvailable() {
			messages = append(messages, "ok: docker binary found")
		} else {
			messages = append(messages, "warn: docker binary not found")
		}
	} else {
		messages = append(messages, "info: adapter does not declare compose files")
	}

	if len(adapter.Outputs.Generated) > 0 {
		messages = append(messages, fmt.Sprintf("ok: adapter declares %d generated output(s)", len(adapter.Outputs.Generated)))
	} else {
		messages = append(messages, "warn: adapter declares no generated outputs")
	}

	if adapter.HasRunCommands() {
		messages = append(messages, fmt.Sprintf("ok: adapter declares %d bare-metal run command(s)", len(adapter.Runtime.Run.Commands)))
	}

	return messages
}
