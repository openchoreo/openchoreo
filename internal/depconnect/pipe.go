// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package depconnect

import (
	"io"
	"sync"
)

// closeWriter is implemented by streams that can end their write side without ending
// the read side (*net.TCPConn, *tls.Conn). A yamux stream's Close does the same.
type closeWriter interface {
	CloseWrite() error
}

// Pipe copies bytes in both directions between a and b, ending each direction
// independently so a peer that half-closes still receives the reply, then closes both.
// a and b need only be byte streams (net.Conn, a yamux stream, ...), not the same kind.
func Pipe(a, b io.ReadWriteCloser) {
	var wg sync.WaitGroup
	wg.Add(2)
	cp := func(dst, src io.ReadWriteCloser) {
		defer wg.Done()
		_, _ = io.Copy(dst, src)
		if cw, ok := dst.(closeWriter); ok {
			_ = cw.CloseWrite()
			return
		}
		_ = dst.Close()
	}
	go cp(a, b)
	go cp(b, a)
	wg.Wait()
	_ = a.Close()
	_ = b.Close()
}
