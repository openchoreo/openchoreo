// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package depagent

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/openchoreo/openchoreo/internal/depconnect"
)

// fakeAuthorizer stands in for the control-plane authorize callback: it maps target
// keys to concrete dial targets, or refuses unknown keys.
type fakeAuthorizer struct {
	targets map[string]depconnect.AuthorizeResponse
	mu      sync.Mutex
	calls   []string
}

func (f *fakeAuthorizer) authorize(_ context.Context, _ /* capability */, key string) (*depconnect.AuthorizeResponse, error) {
	// handleStream runs one goroutine per stream, so recording must be guarded even
	// though today's tests open streams one at a time.
	f.mu.Lock()
	f.calls = append(f.calls, key)
	f.mu.Unlock()
	t, ok := f.targets[key]
	if !ok {
		return nil, fmt.Errorf("not authorized")
	}
	return &t, nil
}

func startEcho(t *testing.T) net.Listener {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) { defer c.Close(); _, _ = io.Copy(c, c) }(c)
		}
	}()
	return ln
}

// serveAgent starts a Server on a plain (non-TLS) listener with the given authorizer
// and returns the client-side TunnelClient connected to it.
func serveAgent(t *testing.T, auth streamAuthorizer) *depconnect.TunnelClient {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })

	srv := NewServer(Config{}.withDefaults(), auth, slog.New(slog.NewTextHandler(io.Discard, nil)))
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() { _ = srv.Serve(ctx, ln) }()

	conn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	client, err := depconnect.NewTunnelClient(conn, "test-capability")
	if err != nil {
		t.Fatalf("handshake: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })
	return client
}

func TestAgentStreamRoundTrip(t *testing.T) {
	echo := startEcho(t)
	host, portStr, _ := net.SplitHostPort(echo.Addr().String())
	port, _ := strconv.Atoi(portStr)

	auth := &fakeAuthorizer{targets: map[string]depconnect.AuthorizeResponse{
		"ep/greeter/http": {Host: host, Port: port, Proto: "tcp"},
	}}
	client := serveAgent(t, auth)

	stream, err := client.OpenStream("ep/greeter/http")
	if err != nil {
		t.Fatalf("open stream: %v", err)
	}
	defer stream.Close()

	msg := []byte("hello-through-the-agent")
	if _, err := stream.Write(msg); err != nil {
		t.Fatalf("write: %v", err)
	}
	buf := make([]byte, len(msg))
	_ = stream.SetReadDeadline(time.Now().Add(5 * time.Second))
	if _, err := io.ReadFull(stream, buf); err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(buf) != string(msg) {
		t.Fatalf("echo mismatch: got %q want %q", buf, msg)
	}
	if got := auth.recorded(); len(got) != 1 || got[0] != "ep/greeter/http" {
		t.Fatalf("authorize not called per stream: %v", got)
	}
}

// fakeHeartbeater records the (capability, namespace) of each liveness refresh.
type fakeHeartbeater struct {
	ch chan [2]string
}

func (f *fakeHeartbeater) heartbeat(_ context.Context, capability, namespace string) error {
	select {
	case f.ch <- [2]string{capability, namespace}:
	default:
	}
	return nil
}

// TestAgentHeartbeatsWhileSessionLive proves the agent refreshes liveness only while it
// has a live session — the mechanism that keeps a paused/idle session from being reaped
// (it does not depend on any new streams being opened).
func TestAgentHeartbeatsWhileSessionLive(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })

	hb := &fakeHeartbeater{ch: make(chan [2]string, 8)}
	cfg := Config{HeartbeatInterval: 20 * time.Millisecond, Namespace: "dp-ns"}.withDefaults()
	srv := NewServer(cfg, &fakeAuthorizer{targets: map[string]depconnect.AuthorizeResponse{}}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	srv.hb = hb

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() { _ = srv.Serve(ctx, ln) }()

	// No session yet: no heartbeats (nothing to keep alive), even with no open streams.
	select {
	case c := <-hb.ch:
		t.Fatalf("heartbeat fired with no live session: %v", c)
	case <-time.After(120 * time.Millisecond):
	}

	// Open a session (no streams). The heartbeat must still fire — this is the paused-
	// at-a-breakpoint case: a live tunnel with zero open connections.
	conn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	client, err := depconnect.NewTunnelClient(conn, "test-capability")
	if err != nil {
		t.Fatalf("handshake: %v", err)
	}

	select {
	case c := <-hb.ch:
		if c[0] != "test-capability" || c[1] != "dp-ns" {
			t.Fatalf("heartbeat sent unexpected args: capability=%q namespace=%q", c[0], c[1])
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no heartbeat while a session was live")
	}

	// Close the session; once it drains, heartbeats must stop.
	_ = client.Close()
	// Allow in-flight ticks tied to the just-closed session to drain.
	time.Sleep(150 * time.Millisecond)
	for len(hb.ch) > 0 {
		<-hb.ch
	}
	select {
	case c := <-hb.ch:
		t.Fatalf("heartbeat fired after the last session closed: %v", c)
	case <-time.After(150 * time.Millisecond):
	}
}

func TestAgentStreamUnauthorizedKeyRejected(t *testing.T) {
	auth := &fakeAuthorizer{targets: map[string]depconnect.AuthorizeResponse{}}
	client := serveAgent(t, auth)

	if _, err := client.OpenStream("ep/not-authorized"); err == nil {
		t.Fatal("expected OpenStream to be rejected for an unauthorized key")
	}
}

func TestAgentStreamDialFailureRejected(t *testing.T) {
	// Authorized key, but the target points at a closed port so the dial fails.
	closed, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := closed.Addr().(*net.TCPAddr)
	_ = closed.Close()

	auth := &fakeAuthorizer{targets: map[string]depconnect.AuthorizeResponse{
		"ep/greeter/http": {Host: "127.0.0.1", Port: addr.Port, Proto: "tcp"},
	}}
	client := serveAgent(t, auth)

	if _, err := client.OpenStream("ep/greeter/http"); err == nil {
		t.Fatal("expected OpenStream to fail when the upstream dial fails")
	}
}

func (f *fakeAuthorizer) recorded() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.calls...)
}

// TestAgentRefusesTargetForAnotherNamespace: the client chooses which agent to open a
// stream against, while the capability says which namespace should serve the target.
// The agent must refuse the mismatch instead of dialing from the wrong namespace.
func TestAgentRefusesTargetForAnotherNamespace(t *testing.T) {
	echo := startEcho(t)
	host, portStr, _ := net.SplitHostPort(echo.Addr().String())
	port, _ := strconv.Atoi(portStr)

	auth := &fakeAuthorizer{targets: map[string]depconnect.AuthorizeResponse{
		"ep/finance/ledger/http": {
			Host: host, Port: port, Proto: "tcp", AgentNamespace: "dp-default-finance-development",
		},
	}}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })

	// This agent serves a different namespace than the target names.
	cfg := Config{Namespace: "dp-default-marketing-development"}.withDefaults()
	srv := NewServer(cfg, auth, slog.New(slog.NewTextHandler(io.Discard, nil)))
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() { _ = srv.Serve(ctx, ln) }()

	conn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	client, err := depconnect.NewTunnelClient(conn, "test-capability")
	if err != nil {
		t.Fatalf("handshake: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })

	if _, err := client.OpenStream("ep/finance/ledger/http"); err == nil {
		t.Fatal("agent served a target belonging to another namespace")
	}
}
