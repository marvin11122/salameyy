//go:build !windows && !aix && !darwin && !dragonfly && !freebsd && !linux && !netbsd && !openbsd && !solaris

package llamacpp

import (
	"errors"
	"os/exec"
)

func detachCommand(*exec.Cmd) {}

func processRunning(int) bool {
	return false
}

func signalProcess(int, bool) error {
	return errors.New("process control is unsupported on this platform")
}

func processStartTime(int) uint64 {
	return 0
}

func processMatchesState(*State) bool {
	return false
}
