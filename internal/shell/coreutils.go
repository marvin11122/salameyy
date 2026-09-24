package shell

import (
	"os"
	"runtime"
	"strconv"
)

var useGoCoreUtils bool

func init() {
	value := os.Getenv("SALAMEY_CORE_UTILS")
	if value == "" {
		value = os.Getenv("CRUSH_CORE_UTILS")
	}
	if v, err := strconv.ParseBool(value); err == nil {
		useGoCoreUtils = v
	} else {
		useGoCoreUtils = runtime.GOOS == "windows"
	}
}
