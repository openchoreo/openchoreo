// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

//go:build !unix

package local

import (
	"os/exec"
)

// setSubshellProcessGroup is a no-op on platforms without POSIX process groups.
func setSubshellProcessGroup(_ *exec.Cmd) {}

// restoreTerminalForeground is a no-op on platforms without POSIX job control.
func restoreTerminalForeground() {}
