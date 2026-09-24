package llamacpp

import (
	"fmt"
	"math"
	"os"
	"runtime"
)

type Recommendation struct {
	Threads     int
	Parallel    int
	ContextSize int
	AutoStart   bool
	Notes       []string
}

func RecommendOptions(opts StartOptions) Recommendation {
	cpuCount := runtime.NumCPU()
	threads := opts.Threads
	if threads <= 0 {
		threads = min(max(cpuCount-1, 1), 16)
	}
	parallel := opts.Parallel
	if parallel <= 0 {
		switch {
		case cpuCount >= 24:
			parallel = 3
		case cpuCount >= 12:
			parallel = 2
		default:
			parallel = 1
		}
	}
	contextSize := opts.ContextSize
	if contextSize <= 0 {
		contextSize = recommendedContextSize(opts.ModelPath)
	}
	return Recommendation{Threads: threads, Parallel: parallel, ContextSize: contextSize, AutoStart: true, Notes: []string{fmt.Sprintf("cpu=%d -> threads=%d", cpuCount, threads), fmt.Sprintf("parallel=%d chosen for local tool latency", parallel), fmt.Sprintf("ctx=%d tuned from model footprint", contextSize)}}
}

func recommendedContextSize(modelPath string) int {
	if modelPath == "" {
		return 16384
	}
	info, err := os.Stat(modelPath)
	if err != nil || !info.Mode().IsRegular() || info.Size() < 0 {
		return 16384
	}
	sizeGB := float64(info.Size()) / (1024 * 1024 * 1024)
	if math.IsNaN(sizeGB) || math.IsInf(sizeGB, 0) {
		return 8192
	}
	switch {
	case sizeGB >= 24:
		return 8192
	case sizeGB >= 12:
		return 12288
	default:
		return 16384
	}
}
