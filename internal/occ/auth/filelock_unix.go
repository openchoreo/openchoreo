// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

//go:build !windows

package auth

import (
	"os"

	"golang.org/x/sys/unix"
)

// lockFD takes an exclusive advisory lock on f, blocking until it is available. The
// lock is released when f is closed.
func lockFD(f *os.File) error {
	return unix.Flock(int(f.Fd()), unix.LOCK_EX)
}
