//go:build windows

package llamacpp

import (
	"errors"
	"os"
	"os/exec"
	"strings"

	"golang.org/x/sys/windows"
)

func detachCommand(cmd *exec.Cmd) {}

func processRunning(pid int) bool {
	if pid <= 1 {
		return false
	}
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return false
	}
	defer windows.CloseHandle(handle)
	var exitCode uint32
	if err := windows.GetExitCodeProcess(handle, &exitCode); err != nil {
		return false
	}
	return exitCode == 259
}

func signalProcess(pid int, force bool) error {
	if pid <= 1 || pid == os.Getpid() {
		return errors.New("refusing to signal an unsafe process ID")
	}
	_ = force
	access := uint32(windows.PROCESS_QUERY_LIMITED_INFORMATION | windows.PROCESS_TERMINATE)
	handle, err := windows.OpenProcess(access, false, uint32(pid))
	if err != nil {
		if errors.Is(err, windows.ERROR_INVALID_PARAMETER) {
			return nil
		}
		return err
	}
	defer windows.CloseHandle(handle)
	if err := windows.TerminateProcess(handle, 1); err != nil {
		return err
	}
	return nil
}

func processStartTime(int) uint64 {
	return 0
}

func processMatchesState(state *State) bool {
	if state == nil || !processRunning(state.PID) || strings.TrimSpace(state.Command) == "" {
		return false
	}
	return true
}
