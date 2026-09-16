// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package remoteagent

import (
	"context"
	"io"
	"log/slog"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/openchoreo/openchoreo/internal/remoteconnect"
)

// startAgent serves cfg on a fresh plain listener and returns its address.
func startAgent(t *testing.T, cfg Config, auth streamAuthorizer) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	srv := NewServer(cfg, auth, slog.New(slog.NewTextHandler(io.Discard, nil)))
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() { _ = srv.Serve(ctx, ln) }()
	return ln.Addr().String()
}

// dialAgent runs the tunnel handshake against addr, returning the client or the
// refusal the agent answered Hello with.
func dialAgent(t *testing.T, addr string) (*remoteconnect.TunnelClient, error) {
	t.Helper()
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	client, err := remoteconnect.NewTunnelClient(conn, func() string { return testCapability })
	if err != nil {
		_ = conn.Close()
		return nil, err
	}
	t.Cleanup(func() { _ = client.Close() })
	return client, nil
}

// The agent stops admitting sessions at its cap, and says why.
func TestAgentRefusesSessionsBeyondLimit(t *testing.T) {
	addr := startAgent(t, Config{MaxSessions: 1}.withDefaults(), &fakeAuthorizer{})

	if _, err := dialAgent(t, addr); err != nil {
		t.Fatalf("first session refused: %v", err)
	}

	_, err := dialAgent(t, addr)
	if err == nil {
		t.Fatal("agent admitted a session beyond its limit")
	}
	if !strings.Contains(err.Error(), "maximum number of sessions") {
		t.Fatalf("refusal does not say why: %v", err)
	}
}

// A session that ends returns its slot, so the cap throttles concurrency rather than
// closing the agent to new sessions for good.
func TestAgentAdmitsSessionAfterSlotFrees(t *testing.T) {
	addr := startAgent(t, Config{MaxSessions: 1}.withDefaults(), &fakeAuthorizer{})

	first, err := dialAgent(t, addr)
	if err != nil {
		t.Fatalf("first session refused: %v", err)
	}
	_ = first.Close()

	// The slot frees once the agent notices the closed connection, so retry rather
	// than race it.
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := dialAgent(t, addr); err == nil {
			return
		} else if time.Now().After(deadline) {
			t.Fatalf("slot never freed after the session closed: %v", err)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// A peer that connects and never sends Hello holds no session slot.
func TestAgentStalledPeerHoldsNoSlot(t *testing.T) {
	addr := startAgent(t, Config{MaxSessions: 1}.withDefaults(), &fakeAuthorizer{})

	stalled, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = stalled.Close() })

	if _, err := dialAgent(t, addr); err != nil {
		t.Fatalf("stalled peer consumed the session slot: %v", err)
	}
}

// Past the configured rate the agent refuses the stream, and does not spend the
// authorize call it refused.
func TestAgentRateLimitsAuthorizeCalls(t *testing.T) {
	echo := startEcho(t)
	host, portStr, _ := net.SplitHostPort(echo.Addr().String())
	port, _ := strconv.Atoi(portStr)

	auth := &fakeAuthorizer{targets: map[string]remoteconnect.AuthorizeResponse{
		"ep/greeter/http": {Host: host, Port: port, Proto: "tcp"},
	}}
	// One call per second with a bucket of one: the second stream would wait a second
	// for a token, well past the authorize budget.
	cfg := Config{
		AuthorizeRatePerSecond: 1,
		AuthorizeBurst:         1,
		AuthorizeTimeout:       100 * time.Millisecond,
	}.withDefaults()
	client, err := dialAgent(t, startAgent(t, cfg, auth))
	if err != nil {
		t.Fatalf("handshake: %v", err)
	}

	stream, err := client.OpenStream("ep/greeter/http")
	if err != nil {
		t.Fatalf("first stream refused: %v", err)
	}
	defer stream.Close()

	_, err = client.OpenStream("ep/greeter/http")
	if err == nil {
		t.Fatal("agent authorized a stream past its rate limit")
	}
	if !strings.Contains(err.Error(), "too many requests") {
		t.Fatalf("refusal does not name the rate limit: %v", err)
	}
	if got := auth.recorded(); len(got) != 1 {
		t.Fatalf("rate-limited stream still called the control plane: %v", got)
	}
}

// A burst within the bucket passes through untouched.
func TestAgentAllowsAuthorizeBurst(t *testing.T) {
	echo := startEcho(t)
	host, portStr, _ := net.SplitHostPort(echo.Addr().String())
	port, _ := strconv.Atoi(portStr)

	auth := &fakeAuthorizer{targets: map[string]remoteconnect.AuthorizeResponse{
		"ep/greeter/http": {Host: host, Port: port, Proto: "tcp"},
	}}
	cfg := Config{
		AuthorizeRatePerSecond: 1, // slow enough that only the burst can serve these
		AuthorizeBurst:         3,
		AuthorizeTimeout:       100 * time.Millisecond,
	}.withDefaults()
	client, err := dialAgent(t, startAgent(t, cfg, auth))
	if err != nil {
		t.Fatalf("handshake: %v", err)
	}

	for i := range 3 {
		stream, err := client.OpenStream("ep/greeter/http")
		if err != nil {
			t.Fatalf("stream %d within the burst refused: %v", i, err)
		}
		t.Cleanup(func() { _ = stream.Close() })
	}
}

func TestSessionTrackerCap(t *testing.T) {
	tr := newSessionTracker(2)
	first, ok := tr.add()
	if !ok {
		t.Fatal("first session refused")
	}
	if _, ok := tr.add(); !ok {
		t.Fatal("second session refused")
	}
	if _, ok := tr.add(); ok {
		t.Fatal("third session admitted past the cap")
	}
	tr.remove(first)
	if _, ok := tr.add(); !ok {
		t.Fatal("session refused after a slot freed")
	}
}

func TestSessionTrackerUnlimited(t *testing.T) {
	tr := newSessionTracker(0)
	for i := range 100 {
		if _, ok := tr.add(); !ok {
			t.Fatalf("session %d refused by an unlimited tracker", i)
		}
	}
}

func TestNewAuthorizeLimiter(t *testing.T) {
	if l := newAuthorizeLimiter(Config{AuthorizeRatePerSecond: 0, AuthorizeBurst: 10}); l != nil {
		t.Fatal("a zero rate must leave authorize calls unlimited")
	}
	// The burst is floored to 1; a zero-burst limiter would refuse every call.
	l := newAuthorizeLimiter(Config{AuthorizeRatePerSecond: 5})
	if l == nil {
		t.Fatal("a configured rate must produce a limiter")
	}
	if got := l.Burst(); got != 1 {
		t.Fatalf("burst floor: got %d want 1", got)
	}
}
