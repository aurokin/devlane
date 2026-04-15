package compose_test

import (
	"reflect"
	"testing"

	"github.com/auro/devlane/internal/compose"
	"github.com/auro/devlane/internal/manifest"
)

func TestBuildCommandContainsProjectAndProfiles(t *testing.T) {
	composeEnv := "/tmp/repo/.devlane/compose.env"
	laneManifest := manifest.Manifest{
		Paths: manifest.Paths{
			ComposeEnv: &composeEnv,
		},
		Network: manifest.Network{
			ProjectName: "demoapp_feature-x",
		},
		Compose: manifest.Compose{
			Files:    []string{"/tmp/repo/compose.yaml", "/tmp/repo/compose.devlane.yaml"},
			Profiles: []string{"web"},
		},
	}

	command, err := compose.BuildCommand(laneManifest, "up", []string{"db"})
	if err != nil {
		t.Fatalf("BuildCommand returned error: %v", err)
	}

	expectedPrefix := []string{"docker", "compose", "--env-file", "/tmp/repo/.devlane/compose.env"}
	if !reflect.DeepEqual(command[:4], expectedPrefix) {
		t.Fatalf("unexpected prefix: got %v want %v", command[:4], expectedPrefix)
	}

	if command[len(command)-2] != "up" || command[len(command)-1] != "-d" {
		t.Fatalf("expected trailing up -d, got %v", command[len(command)-2:])
	}
}
