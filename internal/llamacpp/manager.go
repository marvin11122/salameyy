package llamacpp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/wiesnermarven-hue/salameyy/internal/gguf/gpu"
	"github.com/wiesnermarven-hue/salameyy/internal/gguf/registry"
)

const maxStateBytes = 1 << 20

type StartOptions struct {
	Command            string
	Args               []string
	Env                map[string]string
	BaseURL            string
	ModelPath          string
	Host               string
	Port               int
	WorkingDir         string
	HealthcheckTimeout time.Duration
	ContextSize        int
	GPULayers          int
	Threads            int
	Parallel           int
	FlashAttention     *bool
	NoMMap             *bool
}

type State struct {
	PID          int       `json:"pid"`
	Command      string    `json:"command"`
	Args         []string  `json:"args"`
	BaseURL      string    `json:"base_url"`
	ModelPath    string    `json:"model_path"`
	LogPath      string    `json:"log_path"`
	StartedAt    time.Time `json:"started_at"`
	WorkingDir   string    `json:"working_dir,omitempty"`
	PIDStartTime uint64    `json:"pid_start_time,omitempty"`
}

type Status struct {
	BaseURL    string
	Managed    bool
	Reachable  bool
	PIDRunning bool
	Models     []ModelInfo
	State      *State
	LastError  error
}

type GPUConfig struct {
	GPUInfo            gpu.GPUInfo
	RecommendedLayers  int
	UseGPU             bool
	AvailableMemoryMB  int64
	EstimatedModelSize float64
}

type Stats struct {
	Uptime       time.Duration `json:"uptime"`
	Requests     int64         `json:"requests"`
	TokensPerSec float64       `json:"tokens_per_sec"`
	TotalTokens  int64         `json:"total_tokens"`
	Managed      bool          `json:"managed"`
	Reachable    bool          `json:"reachable"`
	ModelPath    string        `json:"model_path,omitempty"`
}

type HealthStatus struct {
	*Status
	SystemInfo *gpu.SystemInfo `json:"system_info"`
	GPUConfig  *GPUConfig      `json:"gpu_config,omitempty"`
	Uptime     time.Duration   `json:"uptime"`
}

type Manager struct {
	dataDir   string
	operation sync.Mutex
	mu        sync.Mutex
	processes map[int]*managedProcess
}

type managedProcess struct {
	cmd       *exec.Cmd
	done      chan struct{}
	command   string
	startTime uint64
}

func NewManager(dataDir string) *Manager {
	return &Manager{dataDir: dataDir, processes: make(map[int]*managedProcess)}
}

func (m *Manager) Start(ctx context.Context, opts StartOptions) (*Status, error) {
	if m == nil {
		return nil, fmt.Errorf("manager is nil")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	opts = opts.withDefaults()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := validateStartOptions(opts); err != nil {
		return nil, err
	}
	m.operation.Lock()
	defer m.operation.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if status, err := m.Status(ctx, opts.BaseURL); err == nil && status.Reachable {
		return nil, fmt.Errorf("llama.cpp server already reachable at %s", status.BaseURL)
	}
	if state, err := m.LoadState(); err == nil && state.PID > 0 && processMatchesState(state) {
		return nil, fmt.Errorf("managed llama.cpp process %d is still running", state.PID)
	}
	command, err := resolveCommand(opts.Command)
	if err != nil {
		return nil, err
	}
	logPath := m.logPath()
	if info, statErr := os.Lstat(logPath); statErr == nil && info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("refusing to write llama.cpp log through a symlink: %s", logPath)
	}
	if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
		return nil, fmt.Errorf("create llama.cpp log directory: %w", err)
	}
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open llama.cpp log file: %w", err)
	}
	defer logFile.Close()
	args := buildArgs(opts)
	cmd := exec.Command(command, args...)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.Stdin = nil
	cmd.Dir = opts.WorkingDir
	cmd.Env = mergeEnv(os.Environ(), opts.Env)
	detachCommand(cmd)
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start llama.cpp: %w", err)
	}
	pid := cmd.Process.Pid
	managed := &managedProcess{cmd: cmd, done: make(chan struct{}), command: command, startTime: processStartTime(pid)}
	m.mu.Lock()
	if m.processes == nil {
		m.processes = make(map[int]*managedProcess)
	}
	m.processes[pid] = managed
	m.mu.Unlock()
	go func() {
		_ = cmd.Wait()
		close(managed.done)
		m.mu.Lock()
		if current, ok := m.processes[pid]; ok && current == managed {
			delete(m.processes, pid)
		}
		m.mu.Unlock()
	}()
	state := &State{PID: pid, Command: command, Args: append([]string(nil), args...), BaseURL: opts.BaseURL, ModelPath: opts.ModelPath, LogPath: logPath, StartedAt: time.Now().UTC(), WorkingDir: opts.WorkingDir, PIDStartTime: managed.startTime}
	if err := m.saveState(state); err != nil {
		m.stopManagedProcess(managed, true)
		return nil, err
	}
	readyCtx, cancel := context.WithTimeout(ctx, opts.HealthcheckTimeout)
	defer cancel()
	models, readyErr := WaitUntilReady(readyCtx, opts.BaseURL, 250*time.Millisecond)
	status := &Status{BaseURL: opts.BaseURL, Managed: true, PIDRunning: processRunning(pid), State: cloneState(state)}
	if readyErr != nil {
		status.LastError = readyErr
		m.stopManagedProcess(managed, true)
		_ = os.Remove(m.statePath())
		return status, fmt.Errorf("llama.cpp started with pid %d but did not become ready within %s: %w", pid, opts.HealthcheckTimeout, readyErr)
	}
	status.Reachable = true
	status.Models = models
	return status, nil
}

func (m *Manager) Status(ctx context.Context, baseURL string) (*Status, error) {
	if m == nil {
		return nil, fmt.Errorf("manager is nil")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	state, err := m.LoadState()
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	status := &Status{Managed: state != nil, State: cloneState(state)}
	if state != nil {
		status.BaseURL = state.BaseURL
		status.PIDRunning = processMatchesState(state)
	}
	if strings.TrimSpace(baseURL) != "" {
		status.BaseURL = NormalizeBaseURL(baseURL, "", 0)
	}
	if status.BaseURL == "" {
		status.BaseURL = DefaultBaseURL
	}
	models, queryErr := QueryModels(ctx, status.BaseURL)
	if queryErr != nil {
		status.LastError = queryErr
		return status, nil
	}
	status.Reachable = true
	status.Models = models
	return status, nil
}

func (m *Manager) Stop(ctx context.Context, force bool) (*Status, error) {
	if m == nil {
		return nil, fmt.Errorf("manager is nil")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	m.operation.Lock()
	defer m.operation.Unlock()
	state, err := m.LoadState()
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("no managed llama.cpp runtime found")
		}
		return nil, err
	}
	if state.PID <= 0 {
		return nil, fmt.Errorf("managed llama.cpp state has an invalid process ID")
	}
	if !processRunning(state.PID) {
		if err := os.Remove(m.statePath()); err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("remove llama.cpp state file: %w", err)
		}
		return &Status{BaseURL: state.BaseURL, Managed: false, State: cloneState(state)}, nil
	}
	if !processMatchesState(state) {
		return nil, fmt.Errorf("refusing to signal PID %d because it does not match the managed llama.cpp process", state.PID)
	}
	if err := signalProcess(state.PID, force); err != nil {
		return nil, fmt.Errorf("signal llama.cpp process: %w", err)
	}
	waitCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for processRunning(state.PID) {
		select {
		case <-waitCtx.Done():
			return nil, fmt.Errorf("llama.cpp pid %d did not stop before timeout", state.PID)
		case <-ticker.C:
		}
	}
	if err := os.Remove(m.statePath()); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("remove llama.cpp state file: %w", err)
	}
	return &Status{BaseURL: state.BaseURL, Managed: false, State: cloneState(state)}, nil
}

func (m *Manager) LoadState() (*State, error) {
	if m == nil {
		return nil, fmt.Errorf("manager is nil")
	}
	data, err := readBoundedFile(m.statePath(), maxStateBytes)
	if err != nil {
		return nil, err
	}
	var state State
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	if err := decoder.Decode(&state); err != nil {
		return nil, fmt.Errorf("decode llama.cpp state: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("decode llama.cpp state: multiple JSON values")
		}
		return nil, fmt.Errorf("decode llama.cpp state: %w", err)
	}
	if state.PID <= 1 {
		return nil, fmt.Errorf("invalid llama.cpp process ID %d", state.PID)
	}
	if len(state.Args) > 4096 {
		return nil, fmt.Errorf("llama.cpp state contains too many arguments")
	}
	argumentBytes := 0
	for _, arg := range state.Args {
		argumentBytes += len(arg)
		if argumentBytes > 1<<20 {
			return nil, fmt.Errorf("llama.cpp state arguments are too large")
		}
	}
	if len(state.Command) > 4096 || len(state.BaseURL) > 4096 || len(state.ModelPath) > 1<<20 || len(state.LogPath) > 1<<20 || len(state.WorkingDir) > 1<<20 {
		return nil, fmt.Errorf("llama.cpp state fields are too large")
	}
	return &state, nil
}

func (m *Manager) statePath() string {
	return filepath.Join(m.dataDir, "llama.cpp.json")
}

func (m *Manager) logPath() string {
	return filepath.Join(m.dataDir, "logs", "llama.cpp.log")
}

func (m *Manager) saveState(state *State) error {
	if state == nil {
		return fmt.Errorf("state is nil")
	}
	if err := os.MkdirAll(filepath.Dir(m.statePath()), 0o755); err != nil {
		return fmt.Errorf("create llama.cpp state directory: %w", err)
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("encode llama.cpp state: %w", err)
	}
	temporary, err := os.CreateTemp(filepath.Dir(m.statePath()), ".llama.cpp-*.tmp")
	if err != nil {
		return fmt.Errorf("create llama.cpp state temporary file: %w", err)
	}
	name := temporary.Name()
	defer os.Remove(name)
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("set llama.cpp state permissions: %w", err)
	}
	if _, err := temporary.Write(data); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("write llama.cpp state: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("sync llama.cpp state: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close llama.cpp state: %w", err)
	}
	if err := os.Rename(name, m.statePath()); err != nil {
		return fmt.Errorf("replace llama.cpp state: %w", err)
	}
	return nil
}

func (m *Manager) stopManagedProcess(process *managedProcess, force bool) {
	if process == nil || process.cmd == nil || process.cmd.Process == nil {
		return
	}
	pid := process.cmd.Process.Pid
	if processMatchesState(&State{PID: pid, Command: process.command, PIDStartTime: process.startTime}) {
		_ = signalProcess(pid, force)
	}
	select {
	case <-process.done:
	case <-time.After(5 * time.Second):
		if force && processMatchesState(&State{PID: pid, Command: process.command, PIDStartTime: process.startTime}) {
			_ = signalProcess(pid, true)
		}
	}
}

func buildArgs(opts StartOptions) []string {
	args := []string{"-m", opts.ModelPath, "--host", opts.Host, "--port", strconv.Itoa(opts.Port)}
	if opts.ContextSize > 0 {
		args = append(args, "-c", strconv.Itoa(opts.ContextSize))
	}
	if opts.GPULayers != 0 {
		args = append(args, "-ngl", strconv.Itoa(opts.GPULayers))
	}
	if opts.Threads > 0 {
		args = append(args, "-t", strconv.Itoa(opts.Threads))
	}
	if opts.Parallel > 0 {
		args = append(args, "-np", strconv.Itoa(opts.Parallel))
	}
	if opts.FlashAttention != nil && *opts.FlashAttention {
		args = append(args, "-fa")
	}
	if opts.NoMMap != nil && *opts.NoMMap {
		args = append(args, "--no-mmap")
	}
	return append(args, opts.Args...)
}

func resolveCommand(command string) (string, error) {
	command = strings.TrimSpace(command)
	candidates := []string{command}
	if command == "" {
		candidates = []string{"llama-server", "llama.cpp/server"}
	}
	for _, candidate := range candidates {
		if candidate == "" || strings.ContainsAny(candidate, "\x00\r\n") {
			continue
		}
		path, err := exec.LookPath(candidate)
		if err != nil {
			continue
		}
		info, statErr := os.Stat(path)
		if statErr != nil || info.IsDir() || (runtime.GOOS != "windows" && info.Mode().Perm()&0o111 == 0) {
			continue
		}
		return path, nil
	}
	if command != "" {
		return "", fmt.Errorf("unable to find executable llama.cpp command %q", command)
	}
	return "", fmt.Errorf("unable to find llama.cpp server binary in PATH; tried llama-server and llama.cpp/server")
}

func mergeEnv(base []string, overrides map[string]string) []string {
	if len(overrides) == 0 {
		return append([]string(nil), base...)
	}
	values := make(map[string]string, len(base)+len(overrides))
	for _, item := range base {
		key, value, ok := strings.Cut(item, "=")
		if ok && validEnvKey(key) && !strings.ContainsRune(value, '\x00') {
			values[key] = value
		}
	}
	for key, value := range overrides {
		if validEnvKey(key) && !strings.ContainsRune(value, '\x00') {
			values[key] = value
		}
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := make([]string, 0, len(keys))
	for _, key := range keys {
		result = append(result, key+"="+values[key])
	}
	return result
}

func (o StartOptions) withDefaults() StartOptions {
	o.Host = strings.TrimSpace(o.Host)
	if o.Host == "" {
		o.Host = "127.0.0.1"
	}
	if o.Port == 0 {
		o.Port = 8080
	}
	o.BaseURL = NormalizeBaseURL(o.BaseURL, o.Host, o.Port)
	if o.HealthcheckTimeout <= 0 {
		o.HealthcheckTimeout = 20 * time.Second
	}
	o.WorkingDir = strings.TrimSpace(o.WorkingDir)
	return o
}

func (m *Manager) StartWithModel(ctx context.Context, entry *registry.ModelEntry) (*Status, error) {
	if m == nil {
		return nil, fmt.Errorf("manager is nil")
	}
	if entry == nil || strings.TrimSpace(entry.Path) == "" {
		return nil, fmt.Errorf("model entry is nil or has an empty path")
	}
	if _, err := os.Stat(entry.Path); err != nil {
		return nil, fmt.Errorf("model file not accessible: %w", err)
	}
	gpuConfig := m.GetGPUConfigContext(ctx, entry.GetFileSizeGB())
	opts := StartOptions{ModelPath: entry.Path, GPULayers: gpuConfig.RecommendedLayers}
	if entry.Metadata != nil && entry.Metadata.ContextLength > 0 && entry.Metadata.ContextLength <= uint64(^uint(0)>>1) {
		opts.ContextSize = int(entry.Metadata.ContextLength)
	}
	return m.Start(ctx, opts)
}

func (m *Manager) GetGPUConfig(modelSizeGB float64) GPUConfig {
	return m.GetGPUConfigContext(context.Background(), modelSizeGB)
}

func (m *Manager) GetGPUConfigContext(ctx context.Context, modelSizeGB float64) GPUConfig {
	if ctx == nil {
		ctx = context.Background()
	}
	systemInfo, err := gpu.DetectSystemContext(ctx)
	if err != nil {
		return GPUConfig{}
	}
	bestGPU := gpu.GetBestGPU(systemInfo.GPUs)
	if bestGPU == nil {
		return GPUConfig{}
	}
	layers := gpu.GetRecommendedGPULayers(*bestGPU, modelSizeGB)
	return GPUConfig{GPUInfo: *bestGPU, RecommendedLayers: layers, UseGPU: layers != 0, AvailableMemoryMB: bestGPU.MemoryMB, EstimatedModelSize: modelSizeGB}
}

func (m *Manager) HotSwap(modelPath string) error {
	return m.HotSwapContext(context.Background(), modelPath)
}

func (m *Manager) HotSwapContext(ctx context.Context, modelPath string) error {
	if m == nil {
		return fmt.Errorf("manager is nil")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	m.operation.Lock()
	defer m.operation.Unlock()
	state, err := m.LoadState()
	if err != nil {
		return err
	}
	if !processMatchesState(state) {
		return fmt.Errorf("llama.cpp server not running")
	}
	info, err := os.Stat(modelPath)
	if err != nil || !info.Mode().IsRegular() {
		if err == nil {
			err = fmt.Errorf("not a regular file")
		}
		return fmt.Errorf("model file not accessible: %w", err)
	}
	payload, err := json.Marshal(map[string]string{"model": modelPath})
	if err != nil {
		return fmt.Errorf("encode hot-swap request: %w", err)
	}
	endpoint := strings.TrimRight(state.BaseURL, "/") + "/api/model"
	requestCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(requestCtx, http.MethodPost, endpoint, strings.NewReader(string(payload)))
	if err != nil {
		return fmt.Errorf("create hot-swap request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
	if err != nil {
		return fmt.Errorf("send hot-swap request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("hot-swap failed with status %s", resp.Status)
	}
	state.ModelPath = modelPath
	return m.saveState(state)
}

func (m *Manager) GetStats() (*Stats, error) {
	return m.GetStatsContext(context.Background())
}

func (m *Manager) GetStatsContext(ctx context.Context) (*Stats, error) {
	if m == nil {
		return nil, fmt.Errorf("manager is nil")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	state, err := m.LoadState()
	if err != nil {
		return nil, err
	}
	status, err := m.Status(ctx, state.BaseURL)
	if err != nil {
		return nil, err
	}
	stats := &Stats{Managed: status.Managed, Reachable: status.Reachable, ModelPath: state.ModelPath}
	if !state.StartedAt.IsZero() {
		stats.Uptime = time.Since(state.StartedAt)
	}
	if !status.Reachable {
		return stats, nil
	}
	endpoint := strings.TrimRight(state.BaseURL, "/") + "/stats"
	requestCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(requestCtx, http.MethodGet, endpoint, nil)
	if err != nil {
		return stats, nil
	}
	resp, err := (&http.Client{Timeout: 2 * time.Second}).Do(req)
	if err != nil {
		return stats, nil
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return stats, nil
	}
	var payload struct {
		Requests     int64   `json:"requests"`
		TokensPerSec float64 `json:"tokens_per_sec"`
		TotalTokens  int64   `json:"total_tokens"`
	}
	if json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&payload) == nil {
		stats.Requests = payload.Requests
		stats.TokensPerSec = payload.TokensPerSec
		stats.TotalTokens = payload.TotalTokens
	}
	return stats, nil
}

func (m *Manager) HealthCheck() (*HealthStatus, error) {
	return m.HealthCheckContext(context.Background())
}

func (m *Manager) HealthCheckContext(ctx context.Context) (*HealthStatus, error) {
	if m == nil {
		return nil, fmt.Errorf("manager is nil")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	state, err := m.LoadState()
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	var status *Status
	if state != nil {
		status, err = m.Status(ctx, state.BaseURL)
		if err != nil {
			return nil, err
		}
	} else {
		status = &Status{Managed: false, BaseURL: DefaultBaseURL}
	}
	health := &HealthStatus{Status: status}
	if systemInfo, detectErr := gpu.DetectSystemContext(ctx); detectErr == nil {
		health.SystemInfo = systemInfo
	}
	if state != nil && state.ModelPath != "" {
		if entryMetadata, parseErr := parseModelSize(state.ModelPath); parseErr == nil {
			config := m.GetGPUConfigContext(ctx, entryMetadata)
			health.GPUConfig = &config
		}
	}
	if state != nil && !state.StartedAt.IsZero() {
		health.Uptime = time.Since(state.StartedAt)
	}
	return health, nil
}

func parseModelSize(path string) (float64, error) {
	info, err := os.Stat(path)
	if err != nil {
		return 0, err
	}
	if !info.Mode().IsRegular() {
		return 0, fmt.Errorf("not a regular file")
	}
	return float64(info.Size()) / (1024 * 1024 * 1024), nil
}

func validateStartOptions(opts StartOptions) error {
	if strings.TrimSpace(opts.ModelPath) == "" {
		return fmt.Errorf("llama.cpp model path is required")
	}
	info, err := os.Stat(opts.ModelPath)
	if err != nil {
		return fmt.Errorf("llama.cpp model path %q is not readable: %w", opts.ModelPath, err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("llama.cpp model path %q is not a regular file", opts.ModelPath)
	}
	if opts.Port < 1 || opts.Port > 65535 {
		return fmt.Errorf("llama.cpp port must be between 1 and 65535")
	}
	if strings.ContainsAny(opts.Host, "\x00\r\n\t ") {
		return fmt.Errorf("invalid llama.cpp host")
	}
	if opts.WorkingDir != "" {
		workingInfo, workingErr := os.Stat(opts.WorkingDir)
		if workingErr != nil || !workingInfo.IsDir() {
			if workingErr == nil {
				workingErr = fmt.Errorf("not a directory")
			}
			return fmt.Errorf("invalid llama.cpp working directory: %w", workingErr)
		}
	}
	if opts.ContextSize < 0 || opts.GPULayers < -1 || opts.Threads < 0 || opts.Parallel < 0 {
		return fmt.Errorf("llama.cpp tuning values must be non-negative and GPU layers must be at least -1")
	}
	if len(opts.Args) > 4096 {
		return fmt.Errorf("too many llama.cpp arguments")
	}
	argumentBytes := 0
	for _, arg := range opts.Args {
		argumentBytes += len(arg)
		if argumentBytes > 1<<20 {
			return fmt.Errorf("llama.cpp arguments are too large")
		}
	}
	if strings.TrimSpace(opts.Command) != "" && strings.ContainsAny(opts.Command, "\x00\r\n") {
		return fmt.Errorf("invalid llama.cpp command")
	}
	if parsed, parseErr := url.Parse(opts.BaseURL); parseErr != nil || parsed.Scheme == "" || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return fmt.Errorf("invalid llama.cpp base URL")
	}
	for _, arg := range opts.Args {
		if strings.IndexFunc(arg, func(r rune) bool { return r < 0x20 || r == 0x7f }) >= 0 {
			return fmt.Errorf("invalid llama.cpp argument")
		}
		if isReservedArgument(arg) {
			return fmt.Errorf("llama.cpp argument %q conflicts with managed options", arg)
		}
	}
	return nil
}

func isReservedArgument(arg string) bool {
	base := arg
	if index := strings.IndexByte(base, '='); index >= 0 {
		base = base[:index]
	}
	switch base {
	case "-m", "--model", "--host", "--port", "-c", "--ctx-size", "-ngl", "--n-gpu-layers", "--gpu-layers", "-t", "--threads", "-np", "--parallel", "-fa", "--flash-attn", "--no-mmap":
		return true
	}
	if strings.HasPrefix(arg, "--") {
		return false
	}
	for _, prefix := range []string{"-m", "-c", "-t", "-np"} {
		if strings.HasPrefix(arg, prefix) && len(arg) > len(prefix) {
			return true
		}
	}
	return strings.HasPrefix(arg, "-ngl") && len(arg) > len("-ngl")
}

func validEnvKey(key string) bool {
	if key == "" || strings.ContainsAny(key, "=\x00\r\n") {
		return false
	}
	return true
}

func readBoundedFile(path string, limit int64) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("state path is not a regular file: %s", path)
	}
	if info.Size() > limit {
		return nil, fmt.Errorf("state file is too large: %d bytes", info.Size())
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("state file is too large")
	}
	return data, nil
}

func cloneState(state *State) *State {
	if state == nil {
		return nil
	}
	copy := *state
	copy.Args = append([]string(nil), state.Args...)
	return &copy
}
