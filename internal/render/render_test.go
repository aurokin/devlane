package render_test

import (
	"testing"

	"github.com/auro/devlane/internal/render"
)

func TestRenderTextUsesDotPaths(t *testing.T) {
	rendered, err := render.Text(
		"lane={{lane.slug}} host={{network.publicHost}}",
		map[string]any{
			"lane": map[string]any{
				"slug": "feature-x",
			},
			"network": map[string]any{
				"publicHost": "feature-x.demoapp.localhost",
			},
		},
	)
	if err != nil {
		t.Fatalf("Text returned error: %v", err)
	}

	if rendered != "lane=feature-x host=feature-x.demoapp.localhost" {
		t.Fatalf("unexpected render result: %q", rendered)
	}
}

func TestRenderTextFailsForUndefinedVariable(t *testing.T) {
	if _, err := render.Text("{{missing.value}}", map[string]any{}); err == nil {
		t.Fatal("expected Text to fail for undefined variable")
	}
}
