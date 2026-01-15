// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package browser

import (
	"fmt"
	"os/exec"
	"runtime"
)

// Open opens the specified URL in the default browser
func Open(url string) error {
	var cmd string
	var args []string

	switch runtime.GOOS {
	case "darwin":
		cmd = "open"
		args = []string{url}
	case "linux":
		cmd = "xdg-open"
		args = []string{url}
	case "windows":
		cmd = "cmd"
		args = []string{"/c", "start", url}
	default:
		return fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}

	return exec.Command(cmd, args...).Start()
}
