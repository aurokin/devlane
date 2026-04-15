package render

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var tokenRE = regexp.MustCompile(`\{\{\s*([a-zA-Z0-9_.]+)\s*\}\}`)

func Text(template string, context map[string]any) (string, error) {
	matches := tokenRE.FindAllStringSubmatchIndex(template, -1)
	if len(matches) == 0 {
		return template, nil
	}

	var builder strings.Builder
	last := 0
	for _, match := range matches {
		builder.WriteString(template[last:match[0]])
		path := template[match[2]:match[3]]

		value, err := lookup(context, path)
		if err != nil {
			return "", err
		}

		if value != nil {
			builder.WriteString(fmt.Sprint(value))
		}

		last = match[1]
	}

	builder.WriteString(template[last:])
	return builder.String(), nil
}

func File(templatePath, destinationPath string, context map[string]any) error {
	payload, err := os.ReadFile(templatePath)
	if err != nil {
		return fmt.Errorf("read template: %w", err)
	}

	rendered, err := Text(string(payload), context)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(destinationPath), 0o755); err != nil {
		return fmt.Errorf("create destination parent: %w", err)
	}

	if err := os.WriteFile(destinationPath, []byte(rendered), 0o644); err != nil {
		return fmt.Errorf("write rendered output: %w", err)
	}

	return nil
}

func lookup(mapping map[string]any, path string) (any, error) {
	current := any(mapping)
	for _, part := range strings.Split(path, ".") {
		switch next := current.(type) {
		case map[string]any:
			value, ok := next[part]
			if !ok {
				return nil, fmt.Errorf("undefined template variable %q", path)
			}
			current = value
		case map[string]string:
			value, ok := next[part]
			if !ok {
				return nil, fmt.Errorf("undefined template variable %q", path)
			}
			current = value
		default:
			return nil, fmt.Errorf("template path %q does not resolve to a mapping", path)
		}
	}

	return current, nil
}
