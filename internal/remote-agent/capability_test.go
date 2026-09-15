// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package remoteagent

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"strconv"
	"sync"
	"testing"

	"github.com/openchoreo/openchoreo/internal/remoteconnect"
)

// recordingAuthorizer records the capability each stream presented.
type recordingAuthorizer struct {
	target remoteconnect.AuthorizeResponse
	mu     sync.Mutex
	seen   []string
}

func (r *recordingAuthorizer) authorize(_ context.Context, capability, key string) (*remoteconnect.AuthorizeResponse, error) {
	r.mu.Lock()
	r.seen = append(r.seen, capability)
	r.mu.Unlock()
	if key == "" {
		return nil, fmt.Errorf("no key")
	}
	t := r.target
	return &t, nil
}

func (r *recordingAuthorizer) capabilities() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.seen...)
}

// serveAgentWithCapability starts an agent and connects a client whose capability comes
// from the given provider, so a test can change it mid-session.
func serveAgentWithCapability(t *testing.T, auth streamAuthorizer, capability func() string) *remoteconnect.TunnelClient {
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
	client, err := remoteconnect.NewTunnelClient(conn, capability)
	if err != nil {
		t.Fatalf("handshake: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })
	return client
}

// A renewed capability authorizes the next stream on the existing tunnel, so renewal
// does not have to re-establish it and drop the session's open connections.
func TestRenewedCapabilityTakesEffectWithoutRedialing(t *testing.T) {
	echo := startEcho(t)
	host, portStr, _ := net.SplitHostPort(echo.Addr().String())
	port, _ := strconv.Atoi(portStr)

	auth := &recordingAuthorizer{target: remoteconnect.AuthorizeResponse{
		Kind: remoteconnect.AuthorizeKindTCP, Host: host, Port: port, Proto: "tcp",
	}}

	var mu sync.Mutex
	current := "capability-1"
	client := serveAgentWithCapability(t, auth, func() string {
		mu.Lock()
		defer mu.Unlock()
		return current
	})

	first, err := client.OpenStream("ep/greeter/greeter-svc/http")
	if err != nil {
		t.Fatalf("open first stream: %v", err)
	}
	defer first.Close()

	// Nothing about the tunnel changes, only what the provider returns.
	mu.Lock()
	current = "capability-2"
	mu.Unlock()

	second, err := client.OpenStream("ep/greeter/greeter-svc/http")
	if err != nil {
		t.Fatalf("open second stream after renewal: %v", err)
	}
	defer second.Close()

	got := auth.capabilities()
	want := []string{"capability-1", "capability-2"}
	if len(got) != len(want) {
		t.Fatalf("expected %d authorize calls, got %v", len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("authorize call %d used capability %q, want %q", i, got[i], want[i])
		}
	}

	// A renewal must not disturb connections that are already established.
	if _, werr := first.Write([]byte("ping")); werr != nil {
		t.Fatalf("write on the pre-renewal stream: %v", werr)
	}
	buf := make([]byte, 4)
	if _, rerr := io.ReadFull(first, buf); rerr != nil {
		t.Fatalf("read on the pre-renewal stream: %v", rerr)
	}
	if string(buf) != "ping" {
		t.Errorf("pre-renewal stream echoed %q, want %q", buf, "ping")
	}
}

// The agent holds no capability of its own, so a stream presenting none is refused
// rather than forwarded as an empty token.
func TestStreamWithoutCapabilityRefused(t *testing.T) {
	auth := &recordingAuthorizer{target: remoteconnect.AuthorizeResponse{
		Kind: remoteconnect.AuthorizeKindTCP, Host: "127.0.0.1", Port: 1, Proto: "tcp",
	}}
	client := serveAgentWithCapability(t, auth, func() string { return "" })

	if _, err := client.OpenStream("ep/greeter/greeter-svc/http"); err == nil {
		t.Fatal("expected a stream with no capability to be refused")
	}
	if calls := auth.capabilities(); len(calls) != 0 {
		t.Errorf("agent called authorize for a capability-less stream: %v", calls)
	}
}

// occ selects a tunnel and a capability together; the stream must carry that capability
// even if the provider has since moved on.
func TestOpenStreamWithUsesTheGivenCapability(t *testing.T) {
	echo := startEcho(t)
	host, portStr, _ := net.SplitHostPort(echo.Addr().String())
	port, _ := strconv.Atoi(portStr)

	auth := &recordingAuthorizer{target: remoteconnect.AuthorizeResponse{
		Kind: remoteconnect.AuthorizeKindTCP, Host: host, Port: port, Proto: "tcp",
	}}
	client := serveAgentWithCapability(t, auth, func() string { return "capability-renewed" })

	stream, err := client.OpenStreamWith("ep/greeter/greeter-svc/http", "capability-pinned")
	if err != nil {
		t.Fatalf("open stream: %v", err)
	}
	defer stream.Close()

	auth.mu.Lock()
	defer auth.mu.Unlock()
	if len(auth.seen) != 1 || auth.seen[0] != "capability-pinned" {
		t.Errorf("agent authorized with %v, want the pinned capability", auth.seen)
	}
}
