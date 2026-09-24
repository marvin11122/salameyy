//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package llamacpp

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
)

func detachCommand(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

func processRunning(pid int) bool {
	if pid <= 1 {
		return false
	}
	err := syscall.Kill(pid, 0)
	if err != nil && !errors.Is(err, syscall.EPERM) {
		return false
	}
	if runtime.GOOS == "linux" {
		data, readErr := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
		if readErr == nil {
			if closeParen := strings.LastIndexByte(string(data), ')'); closeParen >= 0 && closeParen+2 < len(data) && data[closeParen+2] == 'Z' {
				return false
			}
		}
	}
	return true
}

func signalProcess(pid int, force bool) error {
	if pid <= 1 || pid == os.Getpid() {
		return errors.New("refusing to signal an unsafe process ID")
	}
	processGroup, err := syscall.Getpgid(pid)
	if err != nil || processGroup != pid {
		return errors.New("refusing to signal a process not owned by its manager")
	}
	signal := syscall.SIGTERM
	if force {
		signal = syscall.SIGKILL
	}
	err = syscall.Kill(pid, signal)
	if errors.Is(err, syscall.ESRCH) {
		return nil
	}
	return err
}

func processStartTime(pid int) uint64 {
	if pid <= 1 || runtime.GOOS != "linux" {
		return 0
	}
	data, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		return 0
	}
	text := string(data)
	closeParen := strings.LastIndexByte(text, ')')
	if closeParen < 0 || closeParen+2 >= len(text) {
		return 0
	}
	fields := strings.Fields(text[closeParen+2:])
	if len(fields) <= 19 {
		return 0
	}
	value, err := strconv.ParseUint(fields[19], 10, 64)
	if err != nil {
		return 0
	}
	return value
}

func processMatchesState(state *State) bool {
	if state == nil || !processRunning(state.PID) {
		return false
	}
	if state.PIDStartTime != 0 && processStartTime(state.PID) != state.PIDStartTime {
		return false
	}
	if strings.TrimSpace(state.Command) == "" {
		return false
	}
	processGroup, groupErr := syscall.Getpgid(state.PID)
	if groupErr != nil || processGroup != state.PID {
		return false
	}
	if runtime.GOOS != "linux" {
		return true
	}
	data, err := os.ReadFile("/proc/" + strconv.Itoa(state.PID) + "/cmdline")
	if err != nil {
		return false
	}
	command := filepath.Clean(state.Command)
	base := filepath.Base(command)
	for _, arg := range strings.Split(string(data), "\x00") {
		arg = filepath.Clean(arg)
		if arg == command || filepath.Base(arg) == base {
			return true
		}
	}
	return false
}
