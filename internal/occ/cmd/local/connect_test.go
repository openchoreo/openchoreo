// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package local

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/openchoreo/openchoreo/internal/depconnect"
)

type fakeResolver struct{ resp *depconnect.ResolveResponse }

func (f *fakeResolver) Resolve(context.Context, depconnect.ResolveRequest) (*depconnect.ResolveResponse, error) {
	return f.resp, nil
}

// fakeTunnel stands in for a yamux session to a dep-agent: each OpenStream dials a
// local echo server, so Connect's local plumbing (listeners, env, per-connection
// stream open) can be exercised without a real dep-agent. The occ -> dep-agent ->
// dependency chain's own hops are covered in their own packages.
type fakeTunnel struct {
	addr   string
	onOpen func(key string)
}

func (f *fakeTunnel) OpenStream(key string) (net.Conn, error) {
	if f.onOpen != nil {
		f.onOpen(key)
	}
	return net.Dial("tcp", f.addr)
}

func (f *fakeTunnel) Close() error { return nil }

// startEchoServer starts a plain TCP echo server, standing in for the far end of a
// dep-connect stream (in production, this is the tunneled dependency).
func startEchoServer(t *testing.T) net.Listener {
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

const testWorkloadYAML = `apiVersion: openchoreo.dev/v1alpha1
kind: Component
metadata:
  name: doclet-document
---
apiVersion: openchoreo.dev/v1alpha1
kind: Workload
metadata:
  name: doclet-document
spec:
  owner:
    projectName: doclet
    componentName: doclet-document
  dependencies:
    endpoints:
      - project: doclet
        component: backend-api
        name: http
        visibility: project
        envBindings:
          address: BACKEND_API_URL
          host: BACKEND_HOST
          port: BACKEND_PORT
`

func writeWorkloadFile(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "workload.yaml")
	if err := os.WriteFile(path, []byte(testWorkloadYAML), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func envToMap(env []string) map[string]string {
	m := map[string]string{}
	for _, kv := range env {
		if k, v, ok := strings.Cut(kv, "="); ok {
			m[k] = v
		}
	}
	return m
}

// TestConnectEndToEnd exercises Dev.Connect with a fake tunnel dialer that connects
// straight to a local echo server, standing in for the real occ -> dep-connect router
// -> dep-agent -> dependency chain. Those hops are covered independently in their own
// packages (depconnect, dep-agent, dep-connect-router, openchoreo-api/handlers); this
// test's job is Dev.Connect's local plumbing: listeners, env rendering, and
// per-connection dialing.
func TestConnectEndToEnd(t *testing.T) {
	echo := startEchoServer(t)

	resp := &depconnect.ResolveResponse{
		Capability: "test-capability",
		Targets: []depconnect.ResolvedTarget{{
			Key:   "ep/backend-api/http",
			Proto: "tcp",
			Endpoint: &depconnect.EndpointRender{
				Scheme:   "http",
				Bindings: depconnect.EndpointEnvBindings{Host: "BACKEND_HOST", Port: "BACKEND_PORT"},
			},
			AgentID: "dp-default-doclet-development",
		}},
		Agents: map[string]depconnect.AgentEndpoint{
			"dp-default-doclet-development": {Endpoint: "router:8443", ServerName: "dp-default-doclet-development.dep-connect"},
		},
	}

	d := New(&fakeResolver{resp: resp})

	var gotKey, gotCapability, gotServerName string
	d.dialTunnel = func(_ context.Context, agent depconnect.AgentEndpoint, capability string) (tunnel, error) {
		gotCapability = capability
		gotServerName = agent.ServerName
		return &fakeTunnel{addr: echo.Addr().String(), onOpen: func(key string) { gotKey = key }}, nil
	}

	var gotEnv map[string]string
	roundTripErr := make(chan error, 1)
	d.runShell = func(_ context.Context, env []string) error {
		gotEnv = envToMap(env)
		roundTripErr <- tunnelRoundTrip(net.JoinHostPort(gotEnv["BACKEND_HOST"], gotEnv["BACKEND_PORT"]), "hello-tunnel")
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := d.Connect(ctx, ConnectParams{WorkloadPaths: []string{writeWorkloadFile(t)}, Namespace: "default", Environment: "development"}, io.Discard); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	if err := <-roundTripErr; err != nil {
		t.Fatalf("round trip through tunnel failed: %v", err)
	}
	if gotEnv["BACKEND_HOST"] != "127.0.0.1" {
		t.Errorf("BACKEND_HOST = %q, want 127.0.0.1", gotEnv["BACKEND_HOST"])
	}
	if _, err := strconv.Atoi(gotEnv["BACKEND_PORT"]); err != nil {
		t.Errorf("BACKEND_PORT not numeric: %q", gotEnv["BACKEND_PORT"])
	}

	if gotKey != "ep/backend-api/http" || gotCapability != "test-capability" {
		t.Errorf("tunnel opened with unexpected args: key=%q capability=%q", gotKey, gotCapability)
	}
	if gotServerName != "dp-default-doclet-development.dep-connect" {
		t.Errorf("tunnel dialed wrong agent: serverName=%q", gotServerName)
	}
}

func tunnelRoundTrip(addr, msg string) error {
	conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		return err
	}
	defer conn.Close()
	if _, err := conn.Write([]byte(msg)); err != nil {
		return err
	}
	buf := make([]byte, len(msg))
	if _, err := io.ReadFull(conn, buf); err != nil {
		return err
	}
	if string(buf) != msg {
		return io.ErrUnexpectedEOF
	}
	return nil
}

const consumerWorkloadYAML = `apiVersion: openchoreo.dev/v1alpha1
kind: Component
metadata:
  name: comp1
---
apiVersion: openchoreo.dev/v1alpha1
kind: Workload
metadata:
  name: comp1
spec:
  owner:
    projectName: demo
    componentName: comp1
  dependencies:
    endpoints:
      - component: comp2
        name: http
        visibility: project
        envBindings:
          address: COMP2_URL
`

const providerWorkloadYAML = `apiVersion: openchoreo.dev/v1alpha1
kind: Component
metadata:
  name: comp2
---
apiVersion: openchoreo.dev/v1alpha1
kind: Workload
metadata:
  name: comp2
spec:
  owner:
    projectName: demo
    componentName: comp2
  endpoints:
    http:
      type: HTTP
      port: 9091
      visibility: [project]
`

func writeWorkloadFileContent(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// erroringResolver fails the test if Resolve is ever called - used to prove a
// cross-linked dependency never reaches the control plane.
type erroringResolver struct{ t *testing.T }

func (r erroringResolver) Resolve(context.Context, depconnect.ResolveRequest) (*depconnect.ResolveResponse, error) {
	r.t.Helper()
	r.t.Fatal("Resolve should not be called when every dependency is locally cross-linked")
	return nil, nil
}

// TestConnectMultiWorkloadLocalLink exercises two workload files where comp1 depends
// on comp2's "http" endpoint and both are passed to Connect - comp2's env binding
// should point straight at a local host:port (default: comp2's own declared port on
// 127.0.0.1) with no tunnel/resolve call involved.
func TestConnectMultiWorkloadLocalLink(t *testing.T) {
	comp1 := writeWorkloadFileContent(t, "comp1.yaml", consumerWorkloadYAML)
	comp2 := writeWorkloadFileContent(t, "comp2.yaml", providerWorkloadYAML)

	d := New(erroringResolver{t: t})
	d.dialTunnel = func(context.Context, depconnect.AgentEndpoint, string) (tunnel, error) {
		t.Fatal("dialTunnel should not be called for a locally-linked dependency")
		return nil, nil
	}

	var gotEnv map[string]string
	d.runShell = func(_ context.Context, env []string) error {
		gotEnv = envToMap(env)
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := d.Connect(ctx, ConnectParams{
		WorkloadPaths: []string{comp1, comp2},
		Namespace:     "default",
		Environment:   "development",
	}, io.Discard); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	if got, want := gotEnv["COMP2_URL"], "http://127.0.0.1:9091"; got != want {
		t.Errorf("COMP2_URL = %q, want %q", got, want)
	}
}

// TestConnectMultiWorkloadLocalLinkOverride exercises --local overriding the default
// local host:port for a cross-linked dependency.
func TestConnectMultiWorkloadLocalLinkOverride(t *testing.T) {
	comp1 := writeWorkloadFileContent(t, "comp1.yaml", consumerWorkloadYAML)
	comp2 := writeWorkloadFileContent(t, "comp2.yaml", providerWorkloadYAML)

	d := New(erroringResolver{t: t})

	var gotEnv map[string]string
	d.runShell = func(_ context.Context, env []string) error {
		gotEnv = envToMap(env)
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	err := d.Connect(ctx, ConnectParams{
		WorkloadPaths:  []string{comp1, comp2},
		Namespace:      "default",
		Environment:    "development",
		LocalOverrides: map[string]LocalTarget{"comp2": {Host: "127.0.0.1", Port: 9999}},
	}, io.Discard)
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}

	if got, want := gotEnv["COMP2_URL"], "http://127.0.0.1:9999"; got != want {
		t.Errorf("COMP2_URL = %q, want %q", got, want)
	}
}

func TestBuildResolveRequestFromWorkloadFile(t *testing.T) {
	wl, err := loadWorkloadFromFile(writeWorkloadFile(t))
	if err != nil {
		t.Fatal(err)
	}
	endpoints := wl.Spec.Dependencies.Endpoints
	req := buildResolveRequest(wl, "default", "development", endpoints)
	if req.Namespace != "default" || req.Project != "doclet" || req.Component != "doclet-document" || req.Environment != "development" {
		t.Fatalf("unexpected identity: %+v", req)
	}
	if len(req.Endpoints) != 1 || req.Endpoints[0].Component != "backend-api" || req.Endpoints[0].Name != "http" {
		t.Fatalf("unexpected endpoints: %+v", req.Endpoints)
	}
	if req.Endpoints[0].EnvBindings.Address != "BACKEND_API_URL" {
		t.Fatalf("unexpected env bindings: %+v", req.Endpoints[0].EnvBindings)
	}
}

// mintExpiredCapability signs a capability that expired an hour ago. Only the exp claim
// matters here: occ reads it unverified, purely to explain the failure.
func mintExpiredCapability(t *testing.T) string {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	tok, err := depconnect.SignCapability(&depconnect.CapabilityClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    "cp",
			Subject:   "user:alice",
			Audience:  jwt.ClaimStrings{depconnect.CapabilityAudience},
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(-time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now().Add(-2 * time.Hour)),
		},
	}, priv, "k1")
	if err != nil {
		t.Fatal(err)
	}
	return tok
}

// syncBuf is a writer safe to read while forward's goroutines are still writing.
type syncBuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuf) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuf) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

// TestForwardReportsExpiredSession: once the capability expires the agent rejects every
// new stream. Swallowing that left the app with a bare connection reset and no clue, so
// the first failure per dependency must say what happened and how to recover.
func TestForwardReportsExpiredSession(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })

	out := &syncBuf{}
	reporter := newStreamErrorReporter(out, mintExpiredCapability(t))
	openErr := errors.New("stream rejected: not authorized")
	reported := make(chan struct{}, 2)
	report := func(k string, e error) { reporter.report(k, e); reported <- struct{}{} }
	go forward(ln, "ep/finance/ledger-svc/http", func() (net.Conn, error) { return nil, openErr }, report) //nolint:unparam // always-error open is the point

	// Two connections: the message must appear once, not per connection.
	for range 2 {
		c, derr := net.Dial("tcp", ln.Addr().String())
		if derr != nil {
			t.Fatal(derr)
		}
		<-reported
		_ = c.Close()
	}

	got := out.String()
	if !strings.Contains(got, "ep/finance/ledger-svc/http") {
		t.Errorf("output does not name the dependency: %q", got)
	}
	if !strings.Contains(got, "session expired") || !strings.Contains(got, "occ local") {
		t.Errorf("expired session must be explained with a remedy, got: %q", got)
	}
	if n := strings.Count(got, "ep/finance/ledger-svc/http"); n != 1 {
		t.Errorf("reported %d times, want once per dependency", n)
	}
}

// TestForwardReportsNonExpiryFailure: a failure that is not expiry surfaces the
// underlying error rather than blaming the session.
func TestForwardReportsNonExpiryFailure(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })

	out := &syncBuf{}
	reporter := newStreamErrorReporter(out, "not-a-jwt") // no expiry available
	reported := make(chan struct{}, 1)
	report := func(k string, e error) { reporter.report(k, e); reported <- struct{}{} }
	go forward(ln, "ep/doclet/backend-api/http", func() (net.Conn, error) {
		return nil, errors.New("dial dep-agent: connection refused")
	}, report)

	c, derr := net.Dial("tcp", ln.Addr().String())
	if derr != nil {
		t.Fatal(derr)
	}
	<-reported
	_ = c.Close()

	got := out.String()
	if !strings.Contains(got, "connection refused") {
		t.Errorf("underlying error not surfaced: %q", got)
	}
	if strings.Contains(got, "session expired") {
		t.Errorf("misreported a dial failure as expiry: %q", got)
	}
}
