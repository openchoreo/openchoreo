// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

//go:build !windows

package component

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"golang.org/x/term"
)

func watchResize(ctx context.Context, ws *wsWriter, fd int) {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGWINCH)
	defer signal.Stop(ch)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ch:
			if w, h, err := term.GetSize(fd); err == nil {
				sendResize(ws, safeUint16(w), safeUint16(h))
			}
		}
	}
}
