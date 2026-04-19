package write

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/auro/devlane/internal/config"
	"github.com/auro/devlane/internal/manifest"
	"github.com/auro/devlane/internal/render"
	"github.com/auro/devlane/internal/util"
)

type RenderedRunCommand struct {
	Name        string
	Description string
	Command     string
}

type PrepareResult struct {
	Messages []string
}

type fileSnapshot struct {
	path    string
	existed bool
	payload []byte
	mode    os.FileMode
}

type writeTarget struct {
	path    string
	existed bool
	mode    os.FileMode
}

type prepareOperation struct {
	messages  []string
	staged    []stagedWrite
	snapshots []fileSnapshot
}

type plannedWrite struct {
	label       string
	path        string
	targetPath  string
	payload     []byte
	mode        os.FileMode
	verifyForUp bool
}

type stagedWrite struct {
	label   string
	path    string
	target  string
	temp    string
	payload []byte
	mode    os.FileMode
}

var renameFile = os.Rename

func Prepare(laneManifest manifest.Manifest, adapter *config.AdapterConfig) (PrepareResult, error) {
	result, _, err := PrepareWithRollback(laneManifest, adapter)
	return result, err
}

func PrepareWithRollback(laneManifest manifest.Manifest, adapter *config.AdapterConfig) (PrepareResult, func() error, error) {
	op, err := newPrepareOperation(laneManifest, adapter)
	if err != nil {
		return PrepareResult{}, nil, err
	}

	result, err := op.Apply()
	if err != nil {
		return PrepareResult{}, nil, err
	}

	return result, op.Rollback, nil
}

func VerifyPreparedOutputs(laneManifest manifest.Manifest, adapter *config.AdapterConfig) error {
	context, err := TemplateContext(laneManifest, adapter)
	if err != nil {
		return err
	}

	writes, _, err := planWrites(laneManifest, adapter, context)
	if err != nil {
		return err
	}

	for _, write := range writes {
		if !write.verifyForUp {
			continue
		}

		info, err := os.Stat(write.path)
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("lane %q is not prepared; run `devlane prepare` first", laneManifest.Lane.Name)
		}
		if err != nil {
			return fmt.Errorf("stat %s %s: %w", write.label, write.path, err)
		}
		if info.IsDir() {
			return fmt.Errorf("%s target must be a file: %s", write.label, write.path)
		}

		existing, err := os.ReadFile(write.path)
		if err != nil {
			return fmt.Errorf("read %s %s: %w", write.label, write.path, err)
		}
		if !equalPreparedPayload(existing, write.payload) {
			return fmt.Errorf("lane %q prepared %s is stale; run `devlane prepare` first", laneManifest.Lane.Name, write.label)
		}
	}

	return nil
}

func newPrepareOperation(laneManifest manifest.Manifest, adapter *config.AdapterConfig) (*prepareOperation, error) {
	context, err := TemplateContext(laneManifest, adapter)
	if err != nil {
		return nil, err
	}

	messages := make([]string, 0, len(laneManifest.Outputs.Generated))

	writes, newMessages, err := planWrites(laneManifest, adapter, context)
	if err != nil {
		return nil, err
	}
	messages = append(messages, newMessages...)

	resolvedWrites, snapshots, err := prepareWriteTargets(writes)
	if err != nil {
		return nil, err
	}

	staged, err := stageWrites(resolvedWrites)
	if err != nil {
		return nil, err
	}

	return &prepareOperation{
		messages:  messages,
		staged:    staged,
		snapshots: snapshots,
	}, nil
}

func (op *prepareOperation) Apply() (PrepareResult, error) {
	defer cleanupTemps(op.staged)

	promoted, err := promoteWrites(op.staged)
	if err != nil {
		rollbackErr := op.Rollback()
		switch {
		case rollbackErr != nil && len(promoted) > 0:
			return PrepareResult{}, fmt.Errorf("%w; rollback promoted local state %s: %v", err, strings.Join(promoted, ", "), rollbackErr)
		case rollbackErr != nil:
			return PrepareResult{}, fmt.Errorf("%w; rollback local prepare state: %v", err, rollbackErr)
		case len(promoted) > 0:
			return PrepareResult{}, fmt.Errorf("%w; rolled back promoted local state: %s", err, strings.Join(promoted, ", "))
		default:
			return PrepareResult{}, err
		}
	}

	return PrepareResult{Messages: op.messages}, nil
}

func (op *prepareOperation) Rollback() error {
	var failures []string
	for i := len(op.snapshots) - 1; i >= 0; i-- {
		if err := restoreSnapshot(op.snapshots[i]); err != nil {
			failures = append(failures, err.Error())
		}
	}

	if len(failures) > 0 {
		return errors.New(strings.Join(failures, "; "))
	}

	return nil
}

func RunCommands(laneManifest manifest.Manifest, adapter *config.AdapterConfig) ([]RenderedRunCommand, error) {
	context, err := TemplateContext(laneManifest, adapter)
	if err != nil {
		return nil, err
	}

	rendered := make([]RenderedRunCommand, 0, len(adapter.Runtime.Run.Commands))
	for _, command := range adapter.Runtime.Run.Commands {
		result, renderErr := render.Text(command.Command, context)
		if renderErr != nil {
			return nil, fmt.Errorf("render runtime.run command %q: %w", command.Name, renderErr)
		}

		rendered = append(rendered, RenderedRunCommand{
			Name:        command.Name,
			Description: command.Description,
			Command:     result,
		})
	}

	return rendered, nil
}

func ComputeEnv(laneManifest manifest.Manifest, adapter *config.AdapterConfig) (map[string]string, error) {
	env := map[string]string{
		"DEVLANE_APP":             adapter.App,
		"DEVLANE_APP_SLUG":        util.Slugify(adapter.App, false),
		"DEVLANE_KIND":            adapter.Kind,
		"DEVLANE_BRANCH":          laneManifest.Lane.Branch,
		"DEVLANE_MODE":            laneManifest.Lane.Mode,
		"DEVLANE_LANE":            laneManifest.Lane.Name,
		"DEVLANE_LANE_SLUG":       laneManifest.Lane.Slug,
		"DEVLANE_STABLE":          fmt.Sprintf("%t", laneManifest.Lane.Stable),
		"DEVLANE_REPO_ROOT":       laneManifest.Lane.RepoRoot,
		"DEVLANE_CONFIG":          laneManifest.Lane.ConfigPath,
		"DEVLANE_MANIFEST":        laneManifest.Paths.Manifest,
		"DEVLANE_STATE_ROOT":      laneManifest.Paths.StateRoot,
		"DEVLANE_CACHE_ROOT":      laneManifest.Paths.CacheRoot,
		"DEVLANE_RUNTIME_ROOT":    laneManifest.Paths.RuntimeRoot,
		"DEVLANE_COMPOSE_PROJECT": laneManifest.Network.ProjectName,
		"DEVLANE_PUBLIC_HOST":     derefOrEmpty(laneManifest.Network.PublicHost),
		"DEVLANE_PUBLIC_URL":      derefOrEmpty(laneManifest.Network.PublicURL),
	}

	if laneManifest.Paths.ComposeEnv != nil {
		env["DEVLANE_COMPOSE_ENV"] = *laneManifest.Paths.ComposeEnv
	} else {
		env["DEVLANE_COMPOSE_ENV"] = ""
	}

	renderValues := map[string]string{
		"app":          adapter.App,
		"app_slug":     util.Slugify(adapter.App, false),
		"branch":       laneManifest.Lane.Branch,
		"branch_slug":  util.Slugify(laneManifest.Lane.Branch, false),
		"lane_name":    laneManifest.Lane.Name,
		"lane_slug":    laneManifest.Lane.Slug,
		"mode":         laneManifest.Lane.Mode,
		"public_host":  derefOrEmpty(laneManifest.Network.PublicHost),
		"public_url":   derefOrEmpty(laneManifest.Network.PublicURL),
		"project_name": laneManifest.Network.ProjectName,
		"state_root":   laneManifest.Paths.StateRoot,
		"cache_root":   laneManifest.Paths.CacheRoot,
		"runtime_root": laneManifest.Paths.RuntimeRoot,
	}

	for key, raw := range adapter.Runtime.Env {
		rendered, err := renderEnvValue(raw, renderValues)
		if err != nil {
			return nil, fmt.Errorf("render runtime.env.%s: %w", key, err)
		}
		env[key] = rendered
	}

	for name, port := range laneManifest.Ports {
		if !port.Allocated {
			continue
		}
		key := portEnvKey(name)
		env[key] = strconv.Itoa(port.Port)
	}

	return env, nil
}

func portEnvKey(name string) string {
	base := filepath.Base(filepath.Clean(name))
	slug := util.Slugify(base, true)
	slug = strings.ReplaceAll(slug, "-", "_")
	if slug == "" {
		slug = "PORT"
	}

	return "DEVLANE_PORT_" + strings.ToUpper(slug)
}

func TemplateContext(laneManifest manifest.Manifest, adapter *config.AdapterConfig) (map[string]any, error) {
	env, err := ComputeEnv(laneManifest, adapter)
	if err != nil {
		return nil, err
	}

	generated := make([]map[string]any, 0, len(laneManifest.Outputs.Generated))
	for _, output := range laneManifest.Outputs.Generated {
		generated = append(generated, map[string]any{
			"template":    output.Template,
			"destination": output.Destination,
		})
	}

	ports := make(map[string]any, len(laneManifest.Ports))
	for name, port := range laneManifest.Ports {
		ports[name] = port.Port
	}

	return map[string]any{
		"app":   laneManifest.App,
		"kind":  laneManifest.Kind,
		"ready": laneManifest.Ready,
		"lane": map[string]any{
			"name":       laneManifest.Lane.Name,
			"slug":       laneManifest.Lane.Slug,
			"mode":       laneManifest.Lane.Mode,
			"stable":     laneManifest.Lane.Stable,
			"branch":     laneManifest.Lane.Branch,
			"repoRoot":   laneManifest.Lane.RepoRoot,
			"configPath": laneManifest.Lane.ConfigPath,
		},
		"paths": map[string]any{
			"manifest":    laneManifest.Paths.Manifest,
			"composeEnv":  derefOrNil(laneManifest.Paths.ComposeEnv),
			"stateRoot":   laneManifest.Paths.StateRoot,
			"cacheRoot":   laneManifest.Paths.CacheRoot,
			"runtimeRoot": laneManifest.Paths.RuntimeRoot,
		},
		"network": map[string]any{
			"projectName": laneManifest.Network.ProjectName,
			"publicHost":  derefOrNil(laneManifest.Network.PublicHost),
			"publicUrl":   derefOrNil(laneManifest.Network.PublicURL),
		},
		"compose": map[string]any{
			"files":    append([]string{}, laneManifest.Compose.Files...),
			"profiles": append([]string{}, laneManifest.Compose.Profiles...),
		},
		"ports": ports,
		"outputs": map[string]any{
			"generated": generated,
		},
		"env": env,
	}, nil
}

func planWrites(laneManifest manifest.Manifest, adapter *config.AdapterConfig, context map[string]any) ([]plannedWrite, []string, error) {
	for _, composeFile := range laneManifest.Compose.Files {
		info, err := os.Stat(composeFile)
		if err != nil {
			return nil, nil, fmt.Errorf("compose file missing: %s", composeFile)
		}
		if info.IsDir() {
			return nil, nil, fmt.Errorf("compose file must be a file: %s", composeFile)
		}
	}

	payload, err := json.MarshalIndent(laneManifest, "", "  ")
	if err != nil {
		return nil, nil, fmt.Errorf("marshal manifest: %w", err)
	}

	writes := []plannedWrite{
		{
			label:   "manifest",
			path:    laneManifest.Paths.Manifest,
			payload: append(payload, '\n'),
			mode:    0o644,
		},
	}

	if laneManifest.Paths.ComposeEnv != nil {
		envPayload, err := ComposeEnvPayload(laneManifest, adapter)
		if err != nil {
			return nil, nil, err
		}
		writes = append(writes, plannedWrite{
			label:       "compose env",
			path:        *laneManifest.Paths.ComposeEnv,
			payload:     envPayload,
			mode:        0o644,
			verifyForUp: true,
		})
	}

	generatedWrites, messages, err := generatedWrites(laneManifest, context)
	if err != nil {
		return nil, nil, err
	}
	writes = append(writes, generatedWrites...)

	return writes, messages, nil
}

func prepareWriteTargets(writes []plannedWrite) ([]plannedWrite, []fileSnapshot, error) {
	resolvedWrites := make([]plannedWrite, 0, len(writes))
	snapshots := make([]fileSnapshot, 0, len(writes))
	for _, write := range writes {
		target, err := inspectWriteTarget(write)
		if err != nil {
			return nil, nil, err
		}

		resolvedWrite := write
		resolvedWrite.targetPath = target.path
		if target.existed {
			resolvedWrite.mode = target.mode
			payload, err := os.ReadFile(target.path)
			if err != nil {
				return nil, nil, fmt.Errorf("read existing %s %s: %w", write.label, write.path, err)
			}
			snapshots = append(snapshots, fileSnapshot{
				path:    target.path,
				existed: true,
				payload: payload,
				mode:    target.mode,
			})
		} else {
			snapshots = append(snapshots, fileSnapshot{path: target.path})
		}
		resolvedWrites = append(resolvedWrites, resolvedWrite)
	}

	return resolvedWrites, snapshots, nil
}

func ComposeEnvPayload(laneManifest manifest.Manifest, adapter *config.AdapterConfig) ([]byte, error) {
	env, err := ComputeEnv(laneManifest, adapter)
	if err != nil {
		return nil, err
	}

	keys := make([]string, 0, len(env))
	for key := range env {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	lines := make([]byte, 0, len(keys)*24)
	for _, key := range keys {
		lines = append(lines, []byte(fmt.Sprintf("%s=%s\n", key, env[key]))...)
	}

	return lines, nil
}

func equalPreparedPayload(left, right []byte) bool {
	return bytes.Equal(left, right)
}

func renderTemplate(templatePath string, context map[string]any) (string, error) {
	payload, err := os.ReadFile(templatePath)
	if err != nil {
		return "", fmt.Errorf("read template: %w", err)
	}

	rendered, err := render.Text(string(payload), context)
	if err != nil {
		return "", err
	}

	return rendered, nil
}

func generatedFileMessages(destinationPath, sidecarPath string) ([]string, error) {
	currentHash, hasCurrent, err := fileHash(destinationPath)
	if err != nil {
		return nil, err
	}
	if !hasCurrent {
		return nil, nil
	}

	sidecarPayload, err := os.ReadFile(sidecarPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []string{fmt.Sprintf("notice: overwriting existing generated file without sidecar hash: %s", destinationPath)}, nil
		}
		return nil, fmt.Errorf("read generated sidecar %s: %w", sidecarPath, err)
	}

	recordedHash := strings.TrimSpace(string(sidecarPayload))
	if recordedHash == "" {
		return nil, fmt.Errorf("generated sidecar is empty: %s", sidecarPath)
	}

	if recordedHash != currentHash {
		return []string{fmt.Sprintf("warning: generated file was modified; overwriting: %s", destinationPath)}, nil
	}

	return nil, nil
}

func stageWrites(writes []plannedWrite) ([]stagedWrite, error) {
	staged := make([]stagedWrite, 0, len(writes))
	for _, write := range writes {
		targetPath := write.targetPath
		if targetPath == "" {
			targetPath = write.path
		}

		if err := util.EnsureParent(targetPath); err != nil {
			return nil, fmt.Errorf("create parent for %s: %w", write.path, err)
		}

		tempFile, err := os.CreateTemp(filepath.Dir(targetPath), ".devlane-*")
		if err != nil {
			return nil, fmt.Errorf("stage %s: %w", write.path, err)
		}

		tempPath := tempFile.Name()
		if _, err := tempFile.Write(write.payload); err != nil {
			_ = tempFile.Close()
			_ = os.Remove(tempPath)
			return nil, fmt.Errorf("stage %s: %w", write.path, err)
		}

		if err := tempFile.Chmod(write.mode); err != nil {
			_ = tempFile.Close()
			_ = os.Remove(tempPath)
			return nil, fmt.Errorf("stage %s: %w", write.path, err)
		}

		if err := tempFile.Close(); err != nil {
			_ = os.Remove(tempPath)
			return nil, fmt.Errorf("stage %s: %w", write.path, err)
		}

		staged = append(staged, stagedWrite{
			label:   write.label,
			path:    write.path,
			target:  targetPath,
			temp:    tempPath,
			payload: write.payload,
			mode:    write.mode,
		})
	}

	return staged, nil
}

func promoteWrites(staged []stagedWrite) ([]string, error) {
	promoted := make([]string, 0, len(staged))
	for _, write := range staged {
		if err := renameFile(write.temp, write.target); err != nil {
			return promoted, fmt.Errorf("promote %s %s: %w", write.label, write.path, err)
		}
		promoted = append(promoted, write.path)
	}

	return promoted, nil
}

func cleanupTemps(staged []stagedWrite) {
	for _, write := range staged {
		_ = os.Remove(write.temp)
	}
}

func restoreSnapshot(snapshot fileSnapshot) error {
	if !snapshot.existed {
		if err := os.Remove(snapshot.path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove %s: %w", snapshot.path, err)
		}
		return nil
	}

	if err := util.EnsureParent(snapshot.path); err != nil {
		return fmt.Errorf("create parent for %s: %w", snapshot.path, err)
	}

	tempFile, err := os.CreateTemp(filepath.Dir(snapshot.path), ".devlane-restore-*")
	if err != nil {
		return fmt.Errorf("restore %s: %w", snapshot.path, err)
	}

	tempPath := tempFile.Name()
	defer os.Remove(tempPath)

	if _, err := tempFile.Write(snapshot.payload); err != nil {
		_ = tempFile.Close()
		return fmt.Errorf("restore %s: %w", snapshot.path, err)
	}
	if err := tempFile.Chmod(snapshot.mode); err != nil {
		_ = tempFile.Close()
		return fmt.Errorf("restore %s: %w", snapshot.path, err)
	}
	if err := tempFile.Close(); err != nil {
		return fmt.Errorf("restore %s: %w", snapshot.path, err)
	}
	if err := os.Rename(tempPath, snapshot.path); err != nil {
		return fmt.Errorf("restore %s: %w", snapshot.path, err)
	}

	return nil
}

func inspectWriteTarget(write plannedWrite) (writeTarget, error) {
	info, err := os.Lstat(write.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return writeTarget{path: write.path}, nil
		}
		return writeTarget{}, fmt.Errorf("stat %s %s: %w", write.label, write.path, err)
	}

	if info.Mode()&os.ModeSymlink != 0 {
		targetPath, err := resolveSymlinkTarget(write.path)
		if err != nil {
			return writeTarget{}, fmt.Errorf("resolve %s symlink %s: %w", write.label, write.path, err)
		}

		targetInfo, err := os.Stat(targetPath)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return writeTarget{}, fmt.Errorf("%s symlink target must already exist: %s -> %s", write.label, write.path, targetPath)
			}
			return writeTarget{}, fmt.Errorf("stat %s target %s: %w", write.label, write.path, err)
		}
		if targetInfo.IsDir() {
			return writeTarget{}, fmt.Errorf("%s target must be a file: %s", write.label, write.path)
		}

		return writeTarget{
			path:    targetPath,
			existed: true,
			mode:    targetInfo.Mode().Perm(),
		}, nil
	}

	if info.IsDir() {
		return writeTarget{}, fmt.Errorf("%s target must be a file: %s", write.label, write.path)
	}

	return writeTarget{
		path:    write.path,
		existed: true,
		mode:    info.Mode().Perm(),
	}, nil
}

func resolveSymlinkTarget(path string) (string, error) {
	current := filepath.Clean(path)
	seen := map[string]struct{}{}

	for {
		if _, ok := seen[current]; ok {
			return "", fmt.Errorf("symlink cycle at %s", current)
		}
		seen[current] = struct{}{}

		info, err := os.Lstat(current)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return current, nil
			}
			return "", err
		}
		if info.Mode()&os.ModeSymlink == 0 {
			return current, nil
		}

		target, err := os.Readlink(current)
		if err != nil {
			return "", err
		}
		if !filepath.IsAbs(target) {
			target = filepath.Join(filepath.Dir(current), target)
		}
		current = filepath.Clean(target)
	}
}

func generatedWrites(laneManifest manifest.Manifest, context map[string]any) ([]plannedWrite, []string, error) {
	outputs := append([]manifest.GeneratedOutput{}, laneManifest.Outputs.Generated...)
	sort.Slice(outputs, func(i, j int) bool {
		return outputs[i].Destination < outputs[j].Destination
	})

	writes := make([]plannedWrite, 0, len(outputs)*2)
	messages := make([]string, 0, len(outputs))
	for _, output := range outputs {
		if !util.IsWithin(laneManifest.Lane.RepoRoot, output.Destination) {
			return nil, nil, fmt.Errorf("generated destination must stay within repo root: %s", output.Destination)
		}

		if !util.IsWithin(laneManifest.Lane.RepoRoot, output.Template) {
			return nil, nil, fmt.Errorf("generated template must stay within repo root: %s", output.Template)
		}

		rendered, err := renderTemplate(output.Template, context)
		if err != nil {
			return nil, nil, fmt.Errorf("render %s: %w", output.Destination, err)
		}

		sidecarPath, err := generatedSidecarPath(laneManifest.Lane.RepoRoot, output.Destination)
		if err != nil {
			return nil, nil, err
		}

		newMessages, err := generatedFileMessages(output.Destination, sidecarPath)
		if err != nil {
			return nil, nil, err
		}
		messages = append(messages, newMessages...)

		payload := []byte(rendered)
		writes = append(writes,
			plannedWrite{
				label:       fmt.Sprintf("generated output %s", output.Destination),
				path:        output.Destination,
				payload:     payload,
				mode:        0o644,
				verifyForUp: true,
			},
			plannedWrite{
				label:   "generated sidecar",
				path:    sidecarPath,
				payload: []byte(payloadHash(payload) + "\n"),
				mode:    0o644,
			},
		)
	}

	return writes, messages, nil
}

func generatedSidecarPath(repoRoot, destinationPath string) (string, error) {
	relative, err := filepath.Rel(repoRoot, destinationPath)
	if err != nil {
		return "", fmt.Errorf("derive generated sidecar path for %s: %w", destinationPath, err)
	}

	hash := sha256.Sum256([]byte(filepath.ToSlash(relative)))
	fileName := hex.EncodeToString(hash[:]) + ".sha256"
	return filepath.Join(repoRoot, ".devlane", "generated-hashes", fileName), nil
}

func fileHash(path string) (string, bool, error) {
	payload, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", false, nil
		}
		return "", false, fmt.Errorf("read %s: %w", path, err)
	}

	return payloadHash(payload), true, nil
}

func payloadHash(payload []byte) string {
	hash := sha256.Sum256(payload)
	return hex.EncodeToString(hash[:])
}

func renderEnvValue(raw any, values map[string]string) (string, error) {
	switch value := raw.(type) {
	case nil:
		return "", nil
	case bool:
		return fmt.Sprintf("%t", value), nil
	case int, int32, int64, float32, float64:
		return fmt.Sprint(value), nil
	case string:
		return util.RenderBracedPattern(value, values)
	default:
		return fmt.Sprint(value), nil
	}
}

func derefOrEmpty(value *string) string {
	if value == nil {
		return ""
	}

	return *value
}

func derefOrNil(value *string) any {
	if value == nil {
		return nil
	}

	return *value
}
