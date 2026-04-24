package portalloc

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/auro/devlane/internal/config"
	"go.yaml.in/yaml/v3"
)

const catalogSchema = 1

const (
	catalogLockTimeout    = 30 * time.Second
	catalogLockRetryDelay = 100 * time.Millisecond
	catalogLockFileMode   = 0o644
	catalogLockWritePerms = 0o755
)

type Lane struct {
	App      string
	RepoPath string
	Name     string
	Mode     string
	Branch   string
	Stable   bool
}

type State struct {
	Port      int
	Allocated bool
	HealthURL *string
}

type hostConfig struct {
	PortRange struct {
		Start int `yaml:"start"`
		End   int `yaml:"end"`
	} `yaml:"port_range"`
	Reserved []int `yaml:"reserved"`
}

type catalog struct {
	Schema      int          `json:"schema"`
	Allocations []allocation `json:"allocations"`
}

type allocation struct {
	App          string `json:"app"`
	Lane         string `json:"lane"`
	Mode         string `json:"mode"`
	Branch       string `json:"branch"`
	Service      string `json:"service"`
	Port         int    `json:"port"`
	RepoPath     string `json:"repoPath"`
	LastPrepared string `json:"lastPrepared"`
}

type PrepareSession struct {
	lock      *catalogLock
	catalog   *catalog
	states    map[string]State
	ready     bool
	published bool
	finished  bool
}

type catalogLock struct {
	file *os.File
}

func Inspect(adapter *config.AdapterConfig, lane Lane) (map[string]State, bool, error) {
	cfg, err := loadHostConfig()
	if err != nil {
		return nil, false, err
	}

	cat, err := loadCatalog()
	if err != nil {
		return nil, false, err
	}

	return resolve(adapter, lane, cfg, cat, false)
}

func HasAllocation(lane Lane) (bool, error) {
	cat, err := loadCatalog()
	if err != nil {
		return false, err
	}

	for _, row := range cat.Allocations {
		if row.App == lane.App && sameRepoPath(row.RepoPath, lane.RepoPath) {
			return true, nil
		}
	}

	return false, nil
}

func BeginPrepare(adapter *config.AdapterConfig, lane Lane) (*PrepareSession, error) {
	cfg, err := loadHostConfig()
	if err != nil {
		return nil, err
	}

	lock, err := acquireCatalogLock(catalogLockTimeout)
	if err != nil {
		return nil, err
	}

	cat, err := loadCatalog()
	if err != nil {
		_ = lock.Close()
		return nil, err
	}

	states, ready, err := resolve(adapter, lane, cfg, cat, true)
	if err != nil {
		_ = lock.Close()
		return nil, err
	}

	return &PrepareSession{
		lock:    lock,
		catalog: cat,
		states:  states,
		ready:   ready,
	}, nil
}

func Prepare(adapter *config.AdapterConfig, lane Lane) (map[string]State, bool, error) {
	session, err := BeginPrepare(adapter, lane)
	if err != nil {
		return nil, false, err
	}
	defer session.Close()

	if err := session.Commit(); err != nil {
		return nil, false, err
	}

	return session.States(), session.Ready(), nil
}

func (s *PrepareSession) States() map[string]State {
	result := make(map[string]State, len(s.states))
	for name, state := range s.states {
		result[name] = state
	}
	return result
}

func (s *PrepareSession) Ready() bool {
	return s.ready
}

func (s *PrepareSession) Commit() error {
	if err := s.Publish(); err != nil {
		return err
	}

	return s.Close()
}

func (s *PrepareSession) Publish() error {
	if s == nil || s.finished {
		return errors.New("prepare session is already closed")
	}
	if s.published {
		return errors.New("prepare session is already published")
	}

	if err := saveCatalog(s.catalog); err != nil {
		return err
	}

	s.published = true
	return nil
}

func (s *PrepareSession) Close() error {
	if s == nil || s.finished {
		return nil
	}

	s.finished = true
	return s.lock.Close()
}

func resolve(adapter *config.AdapterConfig, lane Lane, cfg hostConfig, cat *catalog, commit bool) (map[string]State, bool, error) {
	states := make(map[string]State, len(adapter.Ports))
	pruneUndeclaredAllocations(cat, lane, declaredServices(adapter))
	if len(adapter.Ports) == 0 {
		return states, true, nil
	}

	reserved := make(map[int]struct{}, len(cfg.Reserved)+len(adapter.Reserved))
	for _, port := range cfg.Reserved {
		reserved[port] = struct{}{}
	}
	for _, port := range adapter.Reserved {
		reserved[port] = struct{}{}
	}

	held := make(map[int]allocation, len(cat.Allocations))
	byService := make(map[string]*allocation, len(adapter.Ports))
	for i := range cat.Allocations {
		row := &cat.Allocations[i]
		if row.App == lane.App && sameRepoPath(row.RepoPath, lane.RepoPath) {
			byService[row.Service] = row
		}
		held[row.Port] = *row
	}

	now := time.Now().UTC().Format(time.RFC3339)
	claimed := make(map[int]allocation, len(adapter.Ports))

	ready := true
	for _, portConfig := range adapter.Ports {
		if existing := byService[portConfig.Name]; existing != nil && reuseExistingAllocation(existing, portConfig, lane) {
			states[portConfig.Name] = State{
				Port:      existing.Port,
				Allocated: true,
				HealthURL: healthURL(existing.Port, portConfig.HealthPath),
			}
			claimed[existing.Port] = *existing
			if commit {
				existing.Lane = lane.Name
				existing.Mode = lane.Mode
				existing.Branch = lane.Branch
				existing.LastPrepared = now
			}
			continue
		}

		existing := byService[portConfig.Name]
		if existing != nil {
			delete(held, existing.Port)
		}

		port, err := choosePort(portConfig, lane, cfg, reserved, held, claimed)
		if err != nil {
			return nil, false, err
		}

		state := State{
			Port:      port,
			Allocated: commit,
			HealthURL: healthURL(port, portConfig.HealthPath),
		}
		states[portConfig.Name] = state
		if !state.Allocated {
			ready = false
		}

		row := allocation{
			App:          lane.App,
			Lane:         lane.Name,
			Mode:         lane.Mode,
			Branch:       lane.Branch,
			Service:      portConfig.Name,
			Port:         port,
			RepoPath:     lane.RepoPath,
			LastPrepared: now,
		}
		claimed[port] = row
		if commit {
			if existing != nil {
				existing.Lane = lane.Name
				existing.Mode = lane.Mode
				existing.Branch = lane.Branch
				existing.Port = port
				existing.LastPrepared = now
			} else {
				cat.Allocations = append(cat.Allocations, row)
			}
		}
	}

	sortCatalog(cat)
	return states, ready, nil
}

func reuseExistingAllocation(existing *allocation, portConfig config.PortConfig, lane Lane) bool {
	if existing == nil {
		return false
	}
	if !lane.Stable {
		return true
	}
	return existing.Port == stablePort(portConfig)
}

func choosePort(portConfig config.PortConfig, lane Lane, cfg hostConfig, reserved map[int]struct{}, held, claimed map[int]allocation) (int, error) {
	if lane.Stable {
		return chooseStablePort(portConfig, reserved, held, claimed)
	}

	for _, candidate := range devCandidates(portConfig, cfg) {
		ok, err := candidateAvailable(candidate, reserved, held, claimed)
		if err != nil {
			continue
		}
		if ok {
			return candidate, nil
		}
	}

	return 0, fmt.Errorf("no available port found for service %q", portConfig.Name)
}

func chooseStablePort(portConfig config.PortConfig, reserved map[int]struct{}, held, claimed map[int]allocation) (int, error) {
	fixture := stablePort(portConfig)
	ok, err := candidateAvailable(fixture, reserved, held, claimed)
	if err != nil {
		return 0, fmt.Errorf("stable port %d for service %q is unavailable: %w", fixture, portConfig.Name, err)
	}
	if !ok {
		return 0, fmt.Errorf("stable port %d for service %q is unavailable", fixture, portConfig.Name)
	}
	return fixture, nil
}

func candidateAvailable(port int, reserved map[int]struct{}, held, claimed map[int]allocation) (bool, error) {
	if !isAvailable(port, reserved, held, claimed) {
		return false, nil
	}
	if err := probePort(port); err != nil {
		return false, err
	}
	return true, nil
}

func devCandidates(portConfig config.PortConfig, cfg hostConfig) []int {
	candidates := make([]int, 0, (cfg.PortRange.End-cfg.PortRange.Start)+2)
	seen := make(map[int]struct{})

	appendCandidate := func(port int) {
		if port <= 0 {
			return
		}
		if _, ok := seen[port]; ok {
			return
		}
		seen[port] = struct{}{}
		candidates = append(candidates, port)
	}

	appendCandidate(portConfig.Default)

	if poolHintWithinRange(portConfig.PoolHint, cfg) {
		for port := portConfig.PoolHint[0]; port <= portConfig.PoolHint[1]; port++ {
			appendCandidate(port)
		}
	}

	for port := cfg.PortRange.Start; port <= cfg.PortRange.End; port++ {
		appendCandidate(port)
	}

	return candidates
}

func isAvailable(port int, reserved map[int]struct{}, held, claimed map[int]allocation) bool {
	if _, ok := reserved[port]; ok {
		return false
	}
	if _, ok := held[port]; ok {
		return false
	}
	if _, ok := claimed[port]; ok {
		return false
	}
	return true
}

func stablePort(portConfig config.PortConfig) int {
	if portConfig.StablePort != nil {
		return *portConfig.StablePort
	}
	return portConfig.Default
}

func poolHintWithinRange(poolHint []int, cfg hostConfig) bool {
	return len(poolHint) == 2 && poolHint[0] >= cfg.PortRange.Start && poolHint[1] <= cfg.PortRange.End
}

func healthURL(port int, path string) *string {
	if strings.TrimSpace(path) == "" {
		return nil
	}
	value := fmt.Sprintf("http://localhost:%d%s", port, path)
	return &value
}

func declaredServices(adapter *config.AdapterConfig) map[string]struct{} {
	declared := make(map[string]struct{}, len(adapter.Ports))
	for _, portConfig := range adapter.Ports {
		declared[portConfig.Name] = struct{}{}
	}
	return declared
}

func pruneUndeclaredAllocations(cat *catalog, lane Lane, declared map[string]struct{}) {
	if cat == nil || len(cat.Allocations) == 0 {
		return
	}

	kept := cat.Allocations[:0]
	for _, row := range cat.Allocations {
		if row.App == lane.App && sameRepoPath(row.RepoPath, lane.RepoPath) {
			if _, ok := declared[row.Service]; !ok {
				continue
			}
		}
		kept = append(kept, row)
	}
	cat.Allocations = kept
}

func sameRepoPath(left, right string) bool {
	if filepath.Clean(left) == filepath.Clean(right) {
		return true
	}

	leftResolved, leftErr := filepath.EvalSymlinks(left)
	rightResolved, rightErr := filepath.EvalSymlinks(right)
	if leftErr != nil || rightErr != nil {
		return false
	}

	return filepath.Clean(leftResolved) == filepath.Clean(rightResolved)
}

func loadHostConfig() (hostConfig, error) {
	cfg := hostConfig{}
	cfg.PortRange.Start = 3000
	cfg.PortRange.End = 9999
	cfg.Reserved = []int{22, 80, 443, 5432, 6379}

	path, err := configPath()
	if err != nil {
		return hostConfig{}, err
	}

	payload, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return cfg, nil
	}
	if err != nil {
		return hostConfig{}, fmt.Errorf("read host config: %w", err)
	}

	if err := yaml.Unmarshal(payload, &cfg); err != nil {
		return hostConfig{}, fmt.Errorf("decode host config: %w", err)
	}
	if cfg.PortRange.Start <= 0 || cfg.PortRange.End <= 0 || cfg.PortRange.Start > cfg.PortRange.End {
		return hostConfig{}, errors.New("host config port_range.start must be <= end")
	}
	slices.Sort(cfg.Reserved)
	cfg.Reserved = slices.Compact(cfg.Reserved)
	return cfg, nil
}

func loadCatalog() (*catalog, error) {
	path, err := catalogPath()
	if err != nil {
		return nil, err
	}

	payload, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return &catalog{Schema: catalogSchema, Allocations: []allocation{}}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read catalog: %w", err)
	}

	var cat catalog
	if err := json.Unmarshal(payload, &cat); err != nil {
		return nil, fmt.Errorf("decode catalog: %w", err)
	}
	if cat.Schema == 0 {
		cat.Schema = catalogSchema
	}
	if cat.Schema != catalogSchema {
		return nil, fmt.Errorf("unsupported catalog schema %d", cat.Schema)
	}
	if cat.Allocations == nil {
		cat.Allocations = []allocation{}
	}
	normalizeLegacyAllocations(&cat)
	sortCatalog(&cat)
	return &cat, nil
}

func saveCatalog(cat *catalog) error {
	dir, err := configDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}

	sortCatalog(cat)
	payload, err := json.MarshalIndent(cat, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal catalog: %w", err)
	}
	payload = append(payload, '\n')

	path, err := catalogPath()
	if err != nil {
		return err
	}
	temp, err := os.CreateTemp(dir, "catalog-*.json")
	if err != nil {
		return fmt.Errorf("create temp catalog: %w", err)
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)

	if _, err := temp.Write(payload); err != nil {
		temp.Close()
		return fmt.Errorf("write temp catalog: %w", err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("close temp catalog: %w", err)
	}
	if err := os.Rename(tempPath, path); err != nil {
		return fmt.Errorf("publish catalog: %w", err)
	}
	return nil
}

func sortCatalog(cat *catalog) {
	sort.Slice(cat.Allocations, func(i, j int) bool {
		left := cat.Allocations[i]
		right := cat.Allocations[j]
		if left.App != right.App {
			return left.App < right.App
		}
		if left.RepoPath != right.RepoPath {
			return left.RepoPath < right.RepoPath
		}
		return left.Service < right.Service
	})
}

func normalizeLegacyAllocations(cat *catalog) {
	for i := range cat.Allocations {
		row := &cat.Allocations[i]
		if strings.TrimSpace(row.Mode) == "" {
			row.Mode = defaultLegacyMode(row)
		}
		if strings.TrimSpace(row.Branch) == "" {
			row.Branch = defaultLegacyBranch(row)
		}
	}
}

func defaultLegacyMode(row *allocation) string {
	if strings.EqualFold(strings.TrimSpace(row.Lane), "stable") {
		return "stable"
	}
	return "dev"
}

func defaultLegacyBranch(row *allocation) string {
	if lane := strings.TrimSpace(row.Lane); lane != "" {
		return lane
	}
	return filepath.Base(filepath.Clean(row.RepoPath))
}

func configDir() (string, error) {
	if xdgConfigHome := strings.TrimSpace(os.Getenv("XDG_CONFIG_HOME")); xdgConfigHome != "" {
		if filepath.IsAbs(xdgConfigHome) {
			return filepath.Join(xdgConfigHome, "devlane"), nil
		}
		root, err := defaultUserConfigDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(root, "devlane"), nil
	}

	root, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolve user config dir: %w", err)
	}
	return filepath.Join(root, "devlane"), nil
}

func defaultUserConfigDir() (string, error) {
	if runtime.GOOS == "darwin" || runtime.GOOS == "windows" {
		root, err := os.UserConfigDir()
		if err != nil {
			return "", fmt.Errorf("resolve user config dir: %w", err)
		}
		return root, nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve user home dir: %w", err)
	}
	return filepath.Join(home, ".config"), nil
}

func configPath() (string, error) {
	dir, err := configDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.yaml"), nil
}

func catalogPath() (string, error) {
	dir, err := configDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "catalog.json"), nil
}

func readLockHolder(path string) string {
	payload, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(payload))
}

// Probe returns nil when the port appears bindable on this host.
func Probe(port int) error {
	return probePort(port)
}

func probePort(port int) error {
	if err := listenAndClose("tcp4", fmt.Sprintf("0.0.0.0:%d", port)); err != nil {
		return fmt.Errorf("ipv4 bind failed: %w", err)
	}

	if err := listenTCP6(port); err != nil {
		if isIPv6Unsupported(err) {
			return nil
		}
		return fmt.Errorf("ipv6 bind failed: %w", err)
	}

	return nil
}

func listenAndClose(network, address string) error {
	listener, err := net.Listen(network, address)
	if err != nil {
		return err
	}
	return listener.Close()
}
