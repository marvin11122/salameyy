package gpu

import (
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const (
	commandOutputLimit = 1 << 20
	commandTimeout     = 3 * time.Second
)

type GPUInfo struct {
	ID          int     `json:"id"`
	Name        string  `json:"name"`
	Driver      string  `json:"driver"`
	MemoryMB    int64   `json:"memory_mb"`
	ComputeCap  string  `json:"compute_capability,omitempty"`
	IsAvailable bool    `json:"is_available"`
	Type        GPUType `json:"type"`
}

type GPUType string

const (
	GPUNvidia  GPUType = "nvidia"
	GPUAmd     GPUType = "amd"
	GPUIntel   GPUType = "intel"
	GPUUnknown GPUType = "unknown"
)

type SystemInfo struct {
	CPUCount int    `json:"cpu_count"`
	CPUModel string `json:"cpu_model"`

	TotalRAMMB int64 `json:"total_ram_mb"`
	FreeRAMMB  int64 `json:"free_ram_mb"`

	GPUs      []GPUInfo `json:"gpus"`
	HasNVIDIA bool      `json:"has_nvidia"`
	HasAMD    bool      `json:"has_amd"`

	OS   string `json:"os"`
	Arch string `json:"arch"`
}

func DetectSystem() (*SystemInfo, error) {
	return DetectSystemContext(context.Background())
}

func DetectSystemContext(ctx context.Context) (*SystemInfo, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	info := &SystemInfo{CPUCount: runtime.NumCPU(), OS: runtime.GOOS, Arch: runtime.GOARCH}
	if model, err := detectCPUModelContext(ctx); err == nil {
		info.CPUModel = model
	} else {
		info.CPUModel = runtime.GOARCH + " CPU"
	}
	if total, free, err := detectMemoryContext(ctx); err == nil {
		info.TotalRAMMB = total
		info.FreeRAMMB = free
	}
	gpus, err := detectGPUsContext(ctx)
	if err == nil {
		info.GPUs = gpus
		for _, gpu := range gpus {
			switch gpu.Type {
			case GPUNvidia:
				info.HasNVIDIA = true
			case GPUAmd:
				info.HasAMD = true
			}
		}
	}
	return info, nil
}

func detectCPUModel() (string, error) {
	return detectCPUModelContext(context.Background())
}

func detectCPUModelContext(ctx context.Context) (string, error) {
	switch runtime.GOOS {
	case "linux":
		data, err := readProcFile("/proc/cpuinfo", commandOutputLimit)
		if err != nil {
			return "", err
		}
		for _, line := range strings.Split(string(data), "\n") {
			if strings.HasPrefix(strings.ToLower(line), "model name") {
				parts := strings.SplitN(line, ":", 2)
				if len(parts) == 2 {
					return strings.TrimSpace(parts[1]), nil
				}
			}
		}
	case "darwin":
		out, err := runCommand(ctx, commandTimeout, "sysctl", "-n", "machdep.cpu.brand_string")
		if err == nil {
			return strings.TrimSpace(string(out)), nil
		}
	}
	return runtime.GOARCH + " CPU", nil
}

func detectMemory() (int64, int64, error) {
	return detectMemoryContext(context.Background())
}

func detectMemoryContext(ctx context.Context) (totalMB, freeMB int64, err error) {
	switch runtime.GOOS {
	case "linux":
		data, err := readProcFile("/proc/meminfo", commandOutputLimit)
		if err != nil {
			return 0, 0, err
		}
		for _, line := range strings.Split(string(data), "\n") {
			fields := strings.Fields(line)
			if len(fields) < 2 {
				continue
			}
			key := strings.TrimSuffix(fields[0], ":")
			value, parseErr := strconv.ParseInt(fields[1], 10, 64)
			if parseErr != nil || value < 0 {
				continue
			}
			value /= 1024
			switch key {
			case "MemTotal":
				totalMB = value
			case "MemAvailable":
				freeMB = value
			}
		}
	case "darwin":
		out, commandErr := runCommand(ctx, commandTimeout, "sysctl", "-n", "hw.memsize")
		if commandErr == nil {
			value, parseErr := strconv.ParseInt(strings.TrimSpace(string(out)), 10, 64)
			if parseErr == nil && value > 0 {
				totalMB = value / (1024 * 1024)
				freeMB = totalMB / 2
			}
		}
	}
	return totalMB, freeMB, nil
}

func detectGPUs() ([]GPUInfo, error) {
	return detectGPUsContext(context.Background())
}

func detectGPUsContext(ctx context.Context) ([]GPUInfo, error) {
	var gpus []GPUInfo
	if nvidia, err := detectNVIDIAGPUsContext(ctx); err == nil {
		gpus = append(gpus, nvidia...)
	}
	if amd, err := detectAMDGPUsContext(ctx); err == nil {
		gpus = append(gpus, amd...)
	}
	return gpus, nil
}

func detectNVIDIAGPUs() ([]GPUInfo, error) {
	return detectNVIDIAGPUsContext(context.Background())
}

func detectNVIDIAGPUsContext(ctx context.Context) ([]GPUInfo, error) {
	if _, err := exec.LookPath("nvidia-smi"); err != nil {
		return nil, fmt.Errorf("nvidia-smi not found")
	}
	args := []string{"--query-gpu=index,name,driver_version,memory.total,compute_capability", "--format=csv,noheader,nounits"}
	out, err := runCommand(ctx, commandTimeout, "nvidia-smi", args...)
	if err != nil {
		out, err = runCommand(ctx, commandTimeout, "nvidia-smi", "--query-gpu=index,name,driver_version,memory.total", "--format=csv,noheader,nounits")
	}
	if err != nil {
		return nil, fmt.Errorf("nvidia-smi query failed: %w", err)
	}
	return parseNVIDIAGPUOutput(out), nil
}

func detectAMDGPUs() ([]GPUInfo, error) {
	return detectAMDGPUsContext(context.Background())
}

func detectAMDGPUsContext(ctx context.Context) ([]GPUInfo, error) {
	if runtime.GOOS != "linux" {
		return nil, fmt.Errorf("AMD detection only supported on Linux")
	}
	data, err := readProcFile("/proc/driver/amdgpu/gpu_info", commandOutputLimit)
	if err != nil {
		return nil, fmt.Errorf("AMD GPU info not found")
	}
	var gpus []GPUInfo
	for _, line := range strings.Split(string(data), "\n") {
		if !strings.HasPrefix(line, "Marketing Name:") {
			continue
		}
		name := strings.TrimSpace(strings.TrimPrefix(line, "Marketing Name:"))
		if name != "" {
			gpus = append(gpus, GPUInfo{ID: len(gpus), Type: GPUAmd, IsAvailable: true, Name: name})
		}
	}
	return gpus, nil
}

func GetRecommendedGPULayers(gpuInfo GPUInfo, modelSizeGB float64) int {
	if gpuInfo.MemoryMB <= 0 || modelSizeGB <= 0 || math.IsNaN(modelSizeGB) || math.IsInf(modelSizeGB, 0) {
		return 0
	}
	availableGB := float64(gpuInfo.MemoryMB) * 0.9 / 1024
	if modelSizeGB < availableGB {
		return -1
	}
	ratio := availableGB / modelSizeGB
	if ratio < 0.1 {
		return 0
	}
	layers := int(ratio * 100)
	if layers < 1 {
		return 1
	}
	return layers
}

func GetBestGPU(gpus []GPUInfo) *GPUInfo {
	var best *GPUInfo
	for i := range gpus {
		gpu := &gpus[i]
		if !gpu.IsAvailable {
			continue
		}
		if best == nil || gpu.MemoryMB > best.MemoryMB || (gpu.MemoryMB == best.MemoryMB && gpu.Type == GPUNvidia && best.Type != GPUNvidia) {
			best = gpu
		}
	}
	return best
}

func FormatMemory(mb int64) string {
	if mb < 0 {
		return "unknown"
	}
	if mb >= 1024 {
		return fmt.Sprintf("%.1f GB", float64(mb)/1024)
	}
	return fmt.Sprintf("%d MB", mb)
}

func (s *SystemInfo) CanRunModel(requiredGB float64) (bool, string) {
	if s == nil {
		return false, "system information is unavailable"
	}
	if requiredGB <= 0 {
		return true, "no memory requirement"
	}
	if math.IsNaN(requiredGB) || math.IsInf(requiredGB, 0) {
		return false, "invalid memory requirement"
	}
	for _, gpu := range s.GPUs {
		if !gpu.IsAvailable {
			continue
		}
		gpuGB := float64(gpu.MemoryMB) / 1024
		if gpuGB >= requiredGB {
			return true, fmt.Sprintf("GPU %d (%s) has %.1f GB, model needs %.1f GB", gpu.ID, gpu.Name, gpuGB, requiredGB)
		}
	}
	ramGB := float64(s.FreeRAMMB) / 1024
	if ramGB >= requiredGB {
		return true, fmt.Sprintf("System has %.1f GB free RAM, model needs %.1f GB", ramGB, requiredGB)
	}
	return false, fmt.Sprintf("Need %.1f GB but only %.1f GB available", requiredGB, ramGB)
}

func (s *SystemInfo) String() string {
	if s == nil {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "CPU: %d cores", s.CPUCount)
	if s.CPUModel != "" {
		fmt.Fprintf(&b, " (%s)", s.CPUModel)
	}
	b.WriteByte('\n')
	fmt.Fprintf(&b, "RAM: %s total, %s free\n", FormatMemory(s.TotalRAMMB), FormatMemory(s.FreeRAMMB))
	if len(s.GPUs) == 0 {
		b.WriteString("GPUs: None detected (CPU only)\n")
		return b.String()
	}
	b.WriteString("GPUs:\n")
	for _, gpu := range s.GPUs {
		fmt.Fprintf(&b, "  [%d] %s (%s) - %s VRAM", gpu.ID, gpu.Name, gpu.Type, FormatMemory(gpu.MemoryMB))
		if gpu.ComputeCap != "" {
			fmt.Fprintf(&b, " (CC %s)", gpu.ComputeCap)
		}
		b.WriteByte('\n')
	}
	return b.String()
}

type limitedBuffer struct {
	buffer []byte
	limit  int
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	if len(b.buffer) >= b.limit {
		return 0, fmt.Errorf("command output exceeds %d bytes", b.limit)
	}
	remaining := b.limit - len(b.buffer)
	if len(p) > remaining {
		b.buffer = append(b.buffer, p[:remaining]...)
		return remaining, fmt.Errorf("command output exceeds %d bytes", b.limit)
	}
	b.buffer = append(b.buffer, p...)
	return len(p), nil
}

func runCommand(ctx context.Context, timeout time.Duration, name string, args ...string) ([]byte, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if timeout <= 0 {
		timeout = commandTimeout
	}
	commandCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	path, err := exec.LookPath(name)
	if err != nil {
		return nil, err
	}
	var output limitedBuffer
	output.limit = commandOutputLimit
	cmd := exec.CommandContext(commandCtx, path, args...)
	cmd.Stdout = &output
	cmd.Stderr = io.Discard
	if err := cmd.Run(); err != nil {
		if errors.Is(commandCtx.Err(), context.DeadlineExceeded) {
			return nil, context.DeadlineExceeded
		}
		return nil, err
	}
	return output.buffer, nil
}

func readProcFile(path string, limit int64) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	return io.ReadAll(io.LimitReader(file, limit))
}

func parseNVIDIAGPUOutput(data []byte) []GPUInfo {
	reader := csv.NewReader(strings.NewReader(string(data)))
	reader.FieldsPerRecord = -1
	var gpus []GPUInfo
	for {
		record, err := reader.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil || len(record) < 4 {
			continue
		}
		id, idErr := strconv.Atoi(strings.TrimSpace(record[0]))
		memory, memoryErr := strconv.ParseInt(strings.TrimSpace(record[3]), 10, 64)
		if idErr != nil || memoryErr != nil || id < 0 || memory < 0 {
			continue
		}
		gpu := GPUInfo{ID: id, Name: strings.TrimSpace(record[1]), Driver: strings.TrimSpace(record[2]), MemoryMB: memory, IsAvailable: true, Type: GPUNvidia}
		if len(record) >= 5 {
			gpu.ComputeCap = strings.TrimSpace(record[4])
		}
		gpus = append(gpus, gpu)
	}
	return gpus
}
