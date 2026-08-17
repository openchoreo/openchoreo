// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package local

import (
	"context"
	"errors"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/openchoreo/openchoreo/internal/depconnect"
)

// TestDialDepAgentTunnelHonoursCancel: canceling mid-retry gives up promptly rather
// than running out the retry window.
func TestDialDepAgentTunnelHonoursCancel(t *testing.T) {
	// Accepts but never completes TLS, so the dial fails and the retry loop engages.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	var mu sync.Mutex
	var accepted []net.Conn
	go func() {
		for {
			c, aerr := ln.Accept()
			if aerr != nil {
				return
			}
			mu.Lock()
			accepted = append(accepted, c)
			mu.Unlock()
		}
	}()
	t.Cleanup(func() {
		mu.Lock()
		defer mu.Unlock()
		for _, c := range accepted {
			_ = c.Close()
		}
	})

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(200 * time.Millisecond)
		cancel()
	}()

	agent := depconnect.AgentEndpoint{Endpoint: ln.Addr().String(), ServerName: "agent.dep-connect"}
	start := time.Now()
	_, derr := dialDepAgentTunnel(ctx, agent, "capability")
	elapsed := time.Since(start)

	if derr == nil {
		t.Fatal("expected the dial to fail")
	}
	if !errors.Is(derr, context.Canceled) {
		t.Fatalf("error = %v, want it to wrap context.Canceled", derr)
	}
	if elapsed > 10*time.Second {
		t.Fatalf("dial ignored cancellation for %v; retry window is %v", elapsed, dialRetryTimeout)
	}
}
