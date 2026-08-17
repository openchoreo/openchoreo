// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

//go:build unix

package local

import (
	"os"
	"os/exec"
	"os/signal"
	"syscall"

	"golang.org/x/sys/unix"
	"golang.org/x/term"
)

// setSubshellProcessGroup puts the subshell into a new process group and, when stdin is
// a terminal, makes it that terminal's FOREGROUND process group (Foreground implies
// Setpgid). Without this the subshell would be a background group and block on SIGTTIN
// the instant the interactive shell read the TTY — the user could type but the shell
// would never receive input.
func setSubshellProcessGroup(cmd *exec.Cmd) {
	attr := &syscall.SysProcAttr{Setpgid: true}
	if f, ok := cmd.Stdin.(*os.File); ok && term.IsTerminal(int(f.Fd())) {
		attr.Foreground = true
		attr.Ctty = int(f.Fd())
	}
	cmd.SysProcAttr = attr
}

// restoreTerminalForeground reclaims the controlling terminal's foreground process group
// for occ after the subshell (which held the foreground) exits. occ is now a background
// group, so the TIOCSPGRP ioctl would itself raise SIGTTOU and stop occ — we ignore
// SIGTTOU across the call. No-op when stdin is not a terminal.
func restoreTerminalForeground() {
	fd := int(os.Stdin.Fd())
	if !term.IsTerminal(fd) {
		return
	}
	signal.Ignore(syscall.SIGTTOU)
	defer signal.Reset(syscall.SIGTTOU)
	_ = unix.IoctlSetPointerInt(fd, unix.TIOCSPGRP, syscall.Getpgrp())
}
