package util

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

var bracePatternRE = regexp.MustCompile(`\{([a-z_]+)\}`)

func Slugify(value string, allowUnderscore bool) string {
	text := strings.ToLower(strings.TrimSpace(value))
	if allowUnderscore {
		text = regexp.MustCompile(`[^a-z0-9_-]+`).ReplaceAllString(text, "-")
		text = regexp.MustCompile(`-{2,}`).ReplaceAllString(text, "-")
		text = regexp.MustCompile(`_{2,}`).ReplaceAllString(text, "_")
		return strings.Trim(text, "-_")
	}

	text = regexp.MustCompile(`[^a-z0-9]+`).ReplaceAllString(text, "-")
	text = regexp.MustCompile(`-{2,}`).ReplaceAllString(text, "-")
	return strings.Trim(text, "-")
}

func ResolvePath(base, raw string) string {
	path := filepath.Clean(raw)
	if filepath.IsAbs(path) {
		return path
	}

	return filepath.Clean(filepath.Join(base, path))
}

func EnsureParent(path string) error {
	return os.MkdirAll(filepath.Dir(path), 0o755)
}

func DedupePreserveOrder(items []string) []string {
	seen := make(map[string]struct{}, len(items))
	result := make([]string, 0, len(items))
	for _, item := range items {
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		result = append(result, item)
	}

	return result
}

func RenderBracedPattern(pattern string, values map[string]string) (string, error) {
	var renderErr error
	rendered := bracePatternRE.ReplaceAllStringFunc(pattern, func(raw string) string {
		if renderErr != nil {
			return raw
		}

		match := bracePatternRE.FindStringSubmatch(raw)
		key := match[1]
		value, ok := values[key]
		if !ok {
			renderErr = fmt.Errorf("unknown placeholder %q in pattern %q", key, pattern)
			return raw
		}

		return value
	})
	if renderErr != nil {
		return "", renderErr
	}

	if bracePatternRE.MatchString(rendered) {
		return "", fmt.Errorf("unresolved placeholder in pattern %q", pattern)
	}

	return rendered, nil
}

func IsWithin(base, target string) bool {
	baseClean := filepath.Clean(base)
	targetClean := filepath.Clean(target)

	relative, err := filepath.Rel(baseClean, targetClean)
	if err != nil {
		return false
	}

	return relative == "." || (!strings.HasPrefix(relative, "..") && !slices.Contains([]string{"..", ""}, relative))
}
