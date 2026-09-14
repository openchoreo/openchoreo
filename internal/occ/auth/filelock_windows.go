// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

//go:build windows

package auth

import (
	"os"

	"golang.org/x/sys/windows"
)

// lockFD takes an exclusive lock on f, blocking until it is available. The lock is
// released when f is closed.
func lockFD(f *os.File) error {
	return windows.LockFileEx(windows.Handle(f.Fd()), windows.LOCKFILE_EXCLUSIVE_LOCK,
		0, 1, 0, new(windows.Overlapped))
}
