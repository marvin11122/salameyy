//go:build !cgo || !gocv

package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stderr, "This demo requires CGO, OpenCV, and -tags gocv")
	os.Exit(1)
}
