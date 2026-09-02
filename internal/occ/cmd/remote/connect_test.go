// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package remote

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"errors"
	"fmt"
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

	"github.com/openchoreo/openchoreo/internal/remoteconnect"
)

type fakeResolver struct {
	resp *remoteconnect.ResolveResponse
}

func (f *fakeResolver) Resolve(context.Context, remoteconnect.ResolveRequest) (*remoteconnect.ResolveResponse, error) {
	return f.resp, nil
}

// fakeTunnel stands in for a yamux session to a remote-agent: each OpenStream dials a
// local echo server, so Connect's local plumbing (listeners, env, per-connection
// stream open) can be exercised without a real remote-agent. The occ -> remote-agent ->
// dependency chain's own hops are covered in their own packages.
type fakeTunnel struct {
	addr   string
	onOpen func(key string)
	// values answers Fetch by key. A key not present is answered as a read failure,
	// which is how the control plane's denial reaches occ.
	values   map[string][]byte
	fetchErr error
	fetched  []string
}

func (f *fakeTunnel) OpenStream(key string) (net.Conn, error) {
	if f.onOpen != nil {
		f.onOpen(key)
	}
	return net.Dial("tcp", f.addr)
}

func (f *fakeTunnel) Fetch(key string) ([]byte, error) {
	f.fetched = append(f.fetched, key)
	if f.fetchErr != nil {
		return nil, f.fetchErr
	}
	v, ok := f.values[key]
	if !ok {
		return nil, errors.New("read failed")
	}
	return v, nil
}

func (f *fakeTunnel) Close() error { return nil }

// startEchoServer starts a plain TCP echo server, standing in for the far end of a
// remote-connect stream (in production, this is the tunneled dependency).
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
    resources:
      - ref: doclet-postgres
        envBindings:
          host: DB_HOST
          port: DB_PORT
          database: DB_NAME
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

// TestConnectEndToEnd exercises Remote.Connect with a fake tunnel dialer that connects
// straight to a local echo server, standing in for the real occ -> remote-connect router
// -> remote-agent -> dependency chain. Those hops are covered independently in their own
// packages (remoteconnect, remote-agent, remote-agent-router, openchoreo-api/handlers); this
// test's job is Remote.Connect's local plumbing: listeners, env rendering, and
// per-connection dialing.
func TestConnectEndToEnd(t *testing.T) {
	echo := startEchoServer(t)

	resp := &remoteconnect.ResolveResponse{
		Capability: "test-capability",
		Targets: []remoteconnect.ResolvedTarget{{
			Key:   remoteconnect.ResourceTargetKey("doclet-postgres", "client"),
			Proto: "tcp",
			Resource: &remoteconnect.ResourceRender{
				Ref:      "doclet-postgres",
				Endpoint: "client",
				HostEnv:  "DB_HOST",
				PortEnv:  "DB_PORT",
			},
			AgentID: "dp-default-doclet-development",
		}},
		Resources: []remoteconnect.ResourceBindings{{
			Ref:       "doclet-postgres",
			StaticEnv: map[string]string{"DB_NAME": "doclet"},
		}},
		Agents: map[string]remoteconnect.AgentEndpoint{
			"dp-default-doclet-development": {Endpoint: "router:8443", ServerName: "dp-default-doclet-development.remote-connect"},
		},
	}

	d := New(&fakeResolver{resp: resp})

	var gotKey, gotCapability, gotServerName string
	d.dialTunnel = func(_ context.Context, agent remoteconnect.AgentEndpoint, capability string) (tunnel, error) {
		gotCapability = capability
		gotServerName = agent.ServerName
		return &fakeTunnel{addr: echo.Addr().String(), onOpen: func(key string) { gotKey = key }}, nil
	}

	var gotEnv map[string]string
	roundTripErr := make(chan error, 1)
	d.runShell = func(_ context.Context, env []string) error {
		gotEnv = envToMap(env)
		roundTripErr <- tunnelRoundTrip(net.JoinHostPort(gotEnv["DB_HOST"], gotEnv["DB_PORT"]), "hello-tunnel")
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
	if gotEnv["DB_HOST"] != "127.0.0.1" {
		t.Errorf("DB_HOST = %q, want 127.0.0.1", gotEnv["DB_HOST"])
	}
	if gotEnv["DB_NAME"] != "doclet" {
		t.Errorf("DB_NAME = %q, want doclet", gotEnv["DB_NAME"])
	}
	if _, err := strconv.Atoi(gotEnv["DB_PORT"]); err != nil {
		t.Errorf("DB_PORT not numeric: %q", gotEnv["DB_PORT"])
	}

	if gotKey != "res/doclet-postgres/client" || gotCapability != "test-capability" {
		t.Errorf("tunnel opened with unexpected args: key=%q capability=%q", gotKey, gotCapability)
	}
	if gotServerName != "dp-default-doclet-development.remote-connect" {
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

func (r erroringResolver) Resolve(context.Context, remoteconnect.ResolveRequest) (*remoteconnect.ResolveResponse, error) {
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
	d.dialTunnel = func(context.Context, remoteconnect.AgentEndpoint, string) (tunnel, error) {
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
	req := buildResolveRequest(wl, "default", "development", nil)
	if req.Namespace != "default" || req.Project != "doclet" || req.Component != "doclet-document" || req.Environment != "development" {
		t.Fatalf("unexpected identity: %+v", req)
	}
	if len(req.Resources) != 1 || req.Resources[0].Ref != "doclet-postgres" {
		t.Fatalf("unexpected resources: %+v", req.Resources)
	}
	if req.Resources[0].EnvBindings["host"] != "DB_HOST" {
		t.Fatalf("unexpected env bindings: %+v", req.Resources[0].EnvBindings)
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
	tok, err := remoteconnect.SignCapability(&remoteconnect.CapabilityClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    "cp",
			Subject:   "user:alice",
			Audience:  jwt.ClaimStrings{remoteconnect.CapabilityAudience},
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
	if !strings.Contains(got, "session expired") || !strings.Contains(got, "occ remote") {
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
		return nil, errors.New("dial remote-agent: connection refused")
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

// A composed connection URL must be re-pointed at the tunnel, so an app that reads
// only the URL works from the developer's machine.
func TestRewriteAddrsComposedValues(t *testing.T) {
	swaps := []addrSwap{newAddrSwap("cache.ns.svc.cluster.local:6379", "55445")}
	env := map[string]string{
		"URLOUT_URL": "redis://:urlonlycase@cache.ns.svc.cluster.local:6379",
		"CONNSTR":    "cache.ns.svc.cluster.local:6379,password=pw",
		"BUCKET":     "assets",
	}
	var out bytes.Buffer
	got := rewriteAddrs(env, swaps, "tc-urlout", &out)

	if got["URLOUT_URL"] != "redis://:urlonlycase@127.0.0.1:55445" {
		t.Errorf("URL not re-pointed: %q", got["URLOUT_URL"])
	}
	// Not a URL at all -- substitution has to work on the raw pair, not via a parser.
	if got["CONNSTR"] != "127.0.0.1:55445,password=pw" {
		t.Errorf("connection string not re-pointed: %q", got["CONNSTR"])
	}
	if got["BUCKET"] != "assets" {
		t.Errorf("unrelated value modified: %q", got["BUCKET"])
	}
	if out.Len() != 0 {
		t.Errorf("unexpected warning: %q", out.String())
	}
}

// Only the full host:port pair is replaced. A value naming the host with a different
// port stays as resolved and is reported, rather than being silently rewritten.
func TestRewriteAddrsLeavesOtherPortsAlone(t *testing.T) {
	swaps := []addrSwap{newAddrSwap("pg.ns.svc.cluster.local:5432", "60001")}
	env := map[string]string{"ADMIN": "http://pg.ns.svc.cluster.local:8080/admin"}
	var out bytes.Buffer
	got := rewriteAddrs(env, swaps, "tc-pg", &out)

	if got["ADMIN"] != "http://pg.ns.svc.cluster.local:8080/admin" {
		t.Errorf("value on another port should be untouched: %q", got["ADMIN"])
	}
	if !strings.Contains(out.String(), "still points at pg.ns.svc.cluster.local") {
		t.Errorf("expected a warning about the unreachable address, got %q", out.String())
	}
}

// A bare host must never be rewritten -- doing so would break TLS verification for a
// value that is a server name rather than a dial target.
func TestRewriteAddrsNeverRewritesBareHost(t *testing.T) {
	swaps := []addrSwap{newAddrSwap("pg.ns.svc.cluster.local:5432", "60001")}
	env := map[string]string{"SSL_SERVERNAME": "pg.ns.svc.cluster.local"}
	var out bytes.Buffer
	got := rewriteAddrs(env, swaps, "tc-pg", &out)

	if got["SSL_SERVERNAME"] != "pg.ns.svc.cluster.local" {
		t.Errorf("bare host was rewritten: %q", got["SSL_SERVERNAME"])
	}
}

// A value holding the host and the port apart cannot be matched as a pair, so each is
// substituted on its own. This is the shape a resource type takes when its only output
// is a driver connection string: there are no discrete host and port outputs to name.
func TestRewriteAddrsSplitHostAndPort(t *testing.T) {
	swaps := []addrSwap{newAddrSwap("cache.ns.svc.cluster.local:6379", "64168")}
	env := map[string]string{
		"CONNSTR": "host=cache.ns.svc.cluster.local,port=6379,password=mys3cret",
	}
	var out bytes.Buffer
	got := rewriteAddrs(env, swaps, "tc-split", &out)

	if want := "host=127.0.0.1,port=64168,password=mys3cret"; got["CONNSTR"] != want {
		t.Errorf("CONNSTR = %q, want %q", got["CONNSTR"], want)
	}
	// Substituting a bare host is the weaker inference, so it is never silent.
	if !strings.Contains(out.String(), "CONNSTR had its host and port re-pointed separately") {
		t.Errorf("expected the split rewrite to be reported, got %q", out.String())
	}
}

// Endpoints of one resource share a host, so a split rewrite must be decided against
// the original value -- otherwise the first endpoint's rewrite hides the host from the
// second and its port is left pointing into the cluster.
func TestRewriteAddrsSplitAcrossTwoEndpoints(t *testing.T) {
	swaps := []addrSwap{
		newAddrSwap("nats.ns.svc.cluster.local:4222", "64168"),
		newAddrSwap("nats.ns.svc.cluster.local:8222", "64169"),
	}
	env := map[string]string{
		"CONNSTR": "host=nats.ns.svc.cluster.local,port=4222,monitor=8222",
	}
	var out bytes.Buffer
	got := rewriteAddrs(env, swaps, "tc-broker", &out)

	if want := "host=127.0.0.1,port=64168,monitor=64169"; got["CONNSTR"] != want {
		t.Errorf("CONNSTR = %q, want %q", got["CONNSTR"], want)
	}
}

// The split path needs the port as a standalone token. A digit run that merely contains
// it, or a hostname that ends in it, is not this endpoint's port.
func TestRewriteAddrsSplitRequiresDelimitedPort(t *testing.T) {
	swaps := []addrSwap{newAddrSwap("cache.ns.svc.cluster.local:6379", "64168")}
	env := map[string]string{
		"LONGER": "host=cache.ns.svc.cluster.local,port=63790",
		"SUFFIX": "host=cache.ns.svc.cluster.local,peer=cache6379.ns",
	}
	var out bytes.Buffer
	got := rewriteAddrs(env, swaps, "tc-split", &out)

	for k, v := range env {
		if got[k] != v {
			t.Errorf("%s should be untouched: %q", k, got[k])
		}
	}
	if !strings.Contains(out.String(), "still points at cache.ns.svc.cluster.local") {
		t.Errorf("expected the unreachable-address warning, got %q", out.String())
	}
}

// A value carrying the fused pair must not also take the split path: the host may
// appear a second time as a server name, which a bare-host rewrite would corrupt.
func TestRewriteAddrsFusedPairSuppressesSplit(t *testing.T) {
	swaps := []addrSwap{newAddrSwap("pg.ns.svc.cluster.local:5432", "60001")}
	env := map[string]string{
		"DSN": "postgres://pg.ns.svc.cluster.local:5432/db?sslmode=verify-full&host=pg.ns.svc.cluster.local",
	}
	var out bytes.Buffer
	got := rewriteAddrs(env, swaps, "tc-pg", &out)

	want := "postgres://127.0.0.1:60001/db?sslmode=verify-full&host=pg.ns.svc.cluster.local"
	if got["DSN"] != want {
		t.Errorf("DSN = %q, want %q", got["DSN"], want)
	}
}

// httpEndpointName is the shared "http" literal used as both an endpoint name and a
// URL scheme across these tests.
const httpEndpointName = "http"

// testEndpointWorkloadYAML declares an endpoint dependency on a component in another
// project, which must be resolved remotely and tunneled.
const testEndpointWorkloadYAML = `apiVersion: openchoreo.dev/v1alpha1
kind: Workload
metadata:
  name: doclet-document
spec:
  owner:
    projectName: doclet
    componentName: doclet-document
  dependencies:
    endpoints:
      - project: finance
        component: ledger-svc
        name: http
        visibility: namespace
        envBindings:
          address: LEDGER_URL
          host: LEDGER_HOST
          port: LEDGER_PORT
`

// TestConnectEndToEndEndpointDependency is the endpoint-dependency counterpart of
// TestConnectEndToEnd: a remote endpoint dep must open a tunnel and render its address
// bindings against the local listener. Kept as its own test because the shared workload
// fixture declares a resource dependency instead, which would otherwise leave the
// endpoint path exercised only by unit tests of the pieces.
func TestConnectEndToEndEndpointDependency(t *testing.T) {
	echo := startEchoServer(t)

	resp := &remoteconnect.ResolveResponse{
		Capability: "test-capability",
		Targets: []remoteconnect.ResolvedTarget{{
			Key:   remoteconnect.EndpointTargetKey("finance", "ledger-svc", httpEndpointName),
			Proto: "tcp",
			Endpoint: &remoteconnect.EndpointRender{
				Scheme:   httpEndpointName,
				BasePath: "/api",
				Bindings: remoteconnect.EndpointEnvBindings{
					Address: "LEDGER_URL", Host: "LEDGER_HOST", Port: "LEDGER_PORT",
				},
			},
			AgentID: "dp-default-finance-development",
		}},
		Agents: map[string]remoteconnect.AgentEndpoint{
			"dp-default-finance-development": {
				Endpoint: "router:8443", ServerName: "dp-default-finance-development.remote-connect",
			},
		},
	}

	d := New(&fakeResolver{resp: resp})
	var gotKey string
	d.dialTunnel = func(_ context.Context, _ remoteconnect.AgentEndpoint, _ string) (tunnel, error) {
		return &fakeTunnel{addr: echo.Addr().String(), onOpen: func(key string) { gotKey = key }}, nil
	}

	var gotEnv map[string]string
	roundTripErr := make(chan error, 1)
	d.runShell = func(_ context.Context, env []string) error {
		gotEnv = envToMap(env)
		roundTripErr <- tunnelRoundTrip(net.JoinHostPort(gotEnv["LEDGER_HOST"], gotEnv["LEDGER_PORT"]), "hello-endpoint")
		return nil
	}

	path := writeWorkloadFileContent(t, "workload.yaml", testEndpointWorkloadYAML)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := d.Connect(ctx, ConnectParams{WorkloadPaths: []string{path}, Namespace: "default", Environment: "development"}, io.Discard); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	if err := <-roundTripErr; err != nil {
		t.Fatalf("round trip through tunnel failed: %v", err)
	}

	if gotKey != "ep/finance/ledger-svc/http" {
		t.Errorf("tunnel opened for %q, want ep/finance/ledger-svc/http", gotKey)
	}
	if gotEnv["LEDGER_HOST"] != localHost {
		t.Errorf("LEDGER_HOST = %q, want %s", gotEnv["LEDGER_HOST"], localHost)
	}
	// The composed address must carry the local port and the provider's base path.
	want := httpEndpointName + "://" + net.JoinHostPort(localHost, gotEnv["LEDGER_PORT"]) + "/api"
	if gotEnv["LEDGER_URL"] != want {
		t.Errorf("LEDGER_URL = %q, want %q", gotEnv["LEDGER_URL"], want)
	}
}

func TestBuildResolveRequestIncludesEndpointDependencies(t *testing.T) {
	wl, err := loadWorkloadFromFile(writeWorkloadFileContent(t, "workload.yaml", testEndpointWorkloadYAML))
	if err != nil {
		t.Fatal(err)
	}
	req := buildResolveRequest(wl, "default", "development", wl.Spec.Dependencies.Endpoints)
	if len(req.Endpoints) != 1 {
		t.Fatalf("unexpected endpoints: %+v", req.Endpoints)
	}
	got := req.Endpoints[0]
	if got.Project != "finance" || got.Component != "ledger-svc" || got.Name != httpEndpointName || got.Visibility != "namespace" {
		t.Fatalf("unexpected endpoint dep: %+v", got)
	}
	if got.EnvBindings.Address != "LEDGER_URL" || got.EnvBindings.Host != "LEDGER_HOST" {
		t.Fatalf("unexpected env bindings: %+v", got.EnvBindings)
	}
}

// A resource output mounted as a file has no local equivalent -- the value lives on the
// A file binding whose value cannot be fetched is named with the reason, not dropped.
// A missing mount usually presents as a bug in the app, so silence is the worst answer.
func TestFetchBindingsReportsUnfetchableFile(t *testing.T) {
	resp := &remoteconnect.ResolveResponse{
		Resources: []remoteconnect.ResourceBindings{{
			Ref:          "doclet-postgres",
			FetchFile:    map[string]string{"/etc/tls/ca.crt": "sec/doclet-postgres/caCert"},
			FetchAgentID: "dp-a",
		}},
	}
	tn := &fakeTunnel{fetchErr: errors.New("read failed")}
	store := newFileStore()
	t.Cleanup(store.cleanup)

	var buf bytes.Buffer
	fetchBindings(map[string]string{}, map[string]bool{}, &buf, resp,
		map[string]tunnel{"dp-a": tn}, store, nil, false)

	out := buf.String()
	for _, want := range []string{"res/doclet-postgres", "/etc/tls/ca.crt", "read failed"} {
		if !strings.Contains(out, want) {
			t.Errorf("report missing %q; got:\n%s", want, out)
		}
	}
}

// --no-secrets fetches nothing at all: not one stream is opened, and every binding is
// reported as skipped. The point of the flag is that no credential enters the process,
// so a fetch that still happened would defeat it entirely.
func TestFetchBindingsNoSecretsFetchesNothing(t *testing.T) {
	resp := &remoteconnect.ResolveResponse{
		Resources: []remoteconnect.ResourceBindings{{
			Ref:          "doclet-postgres",
			FetchEnv:     map[string]string{"DB_PASSWORD": "sec/doclet-postgres/password"},
			FetchFile:    map[string]string{"/etc/tls/ca.crt": "sec/doclet-postgres/caCert"},
			FetchAgentID: "dp-a",
		}},
	}
	tn := &fakeTunnel{values: map[string][]byte{
		"sec/doclet-postgres/password": []byte("s3cr3t"),
		"sec/doclet-postgres/caCert":   []byte("PEM"),
	}}
	store := newFileStore()
	t.Cleanup(store.cleanup)

	overrides := map[string]string{}
	var buf bytes.Buffer
	fetchBindings(overrides, map[string]bool{}, &buf, resp,
		map[string]tunnel{"dp-a": tn}, store, nil, true)

	if len(tn.fetched) != 0 {
		t.Fatalf("--no-secrets still fetched %v", tn.fetched)
	}
	if _, set := overrides["DB_PASSWORD"]; set {
		t.Fatalf("--no-secrets still set DB_PASSWORD")
	}
	if !strings.Contains(buf.String(), "--no-secrets") {
		t.Errorf("skipped bindings not attributed to the flag:\n%s", buf.String())
	}
}

// A fetched value reaches the environment but never the terminal. occ's output is the
// developer's scrollback and often a pasted bug report, so printing the value there
// would undo the design's whole point of keeping it off the control plane.
func TestFetchBindingsNeverPrintsTheValue(t *testing.T) {
	const secret = "pa55w0rd-do-not-print"
	resp := &remoteconnect.ResolveResponse{
		Resources: []remoteconnect.ResourceBindings{{
			Ref:          "doclet-postgres",
			FetchEnv:     map[string]string{"DB_PASSWORD": "sec/doclet-postgres/password"},
			FetchAgentID: "dp-a",
		}},
	}
	tn := &fakeTunnel{values: map[string][]byte{"sec/doclet-postgres/password": []byte(secret)}}
	store := newFileStore()
	t.Cleanup(store.cleanup)

	overrides := map[string]string{}
	sensitive := map[string]bool{}
	var buf bytes.Buffer
	fetchBindings(overrides, sensitive, &buf, resp, map[string]tunnel{"dp-a": tn}, store, nil, false)

	if overrides["DB_PASSWORD"] != secret {
		t.Fatalf("value did not reach the env: %q", overrides["DB_PASSWORD"])
	}
	if !sensitive["DB_PASSWORD"] {
		t.Errorf("fetched value not marked sensitive; --print-env would leak it")
	}
	if strings.Contains(buf.String(), secret) {
		t.Errorf("value printed to output:\n%s", buf.String())
	}

	// And --print-env redacts it.
	var envBuf bytes.Buffer
	printEnvBindings(&envBuf, overrides, sensitive)
	if strings.Contains(envBuf.String(), secret) {
		t.Errorf("--print-env leaked the value:\n%s", envBuf.String())
	}
	if !strings.Contains(envBuf.String(), "DB_PASSWORD") {
		t.Errorf("--print-env hid the binding entirely:\n%s", envBuf.String())
	}
}

// A split rewrite must not consume a port an earlier endpoint's rewrite just produced.
// Ephemeral local ports are OS-assigned and can coincide with another endpoint's
// in-cluster port; substituting into that would silently undo the first rewrite.
func TestRewriteAddrsSkipsPortThatIsAnotherEndpointsLocalPort(t *testing.T) {
	swaps := []addrSwap{
		newAddrSwap("pg.ns.svc:5432", "40001"),
		// This endpoint's in-cluster port is the first endpoint's local port.
		newAddrSwap("pg.ns.svc:40001", "50002"),
	}
	env := map[string]string{"DSN": "host=pg.ns.svc,port=5432,fallbackPort=40001"}
	var buf bytes.Buffer
	got := rewriteAddrs(env, swaps, "pg", &buf)

	// The first endpoint's rewrite stands: its local port is not re-substituted.
	if !strings.Contains(got["DSN"], "port=40001") {
		t.Errorf("first rewrite was clobbered: %q", got["DSN"])
	}
	if !strings.Contains(got["DSN"], localHost) {
		t.Errorf("host was not re-pointed: %q", got["DSN"])
	}
	if strings.Contains(got["DSN"], "port=50002") {
		t.Errorf("second endpoint consumed the first's local port: %q", got["DSN"])
	}
}

// applyResourceBindings reports a resource nothing was tunneled for, naming what the
// values actually are — a binding pinned to a ResourceRelease cut before its type
// declared endpoints exports in-cluster addresses this machine cannot resolve, and
// saying nothing there reads as success.
func TestApplyResourceBindingsReportsUntunneledResource(t *testing.T) {
	resp := &remoteconnect.ResolveResponse{
		Resources: []remoteconnect.ResourceBindings{{
			Ref:              "doclet-postgres",
			StaticEnv:        map[string]string{"DB_HOST": "pg.ns.svc.cluster.local", "DB_PORT": "5432"},
			OmittedSecretEnv: []remoteconnect.OmittedBinding{{Target: "DB_PASSWORD", Reason: "secret-backed"}},
		}},
	}
	overrides := map[string]string{}
	var buf bytes.Buffer
	applyResourceBindings(overrides, &buf, resp, nil)

	if overrides["DB_HOST"] != "pg.ns.svc.cluster.local" {
		t.Errorf("static bindings should pass through unchanged, got %+v", overrides)
	}
	out := buf.String()
	for _, want := range []string{"res/doclet-postgres", "no endpoint tunneled", "as published in-cluster", "DB_PASSWORD"} {
		if !strings.Contains(out, want) {
			t.Errorf("report missing %q; got:\n%s", want, out)
		}
	}
}

// A resource that publishes its connection URL through a Secret must have the tunneled
// address substituted in the FETCHED value, not just in the values the control plane
// resolved. Before value resolution this binding was omitted with a warning; resolving it
// without re-pointing would hand the app the in-cluster address instead — reachable from
// the cluster, not from the developer's machine, and silent about it.
func TestFetchBindingsRepointsTunneledAddressInFetchedValue(t *testing.T) {
	const remote = "pg.dp-ns.svc.cluster.local:5432"
	resp := &remoteconnect.ResolveResponse{
		Resources: []remoteconnect.ResourceBindings{{
			Ref:          "local-dev-postgres",
			FetchEnv:     map[string]string{"DB_URL": "sec/local-dev-postgres/url"},
			FetchAgentID: "dp-a",
		}},
	}
	tn := &fakeTunnel{values: map[string][]byte{
		"sec/local-dev-postgres/url": []byte("postgres://app:pw@" + remote + "/doclet"),
	}}
	store := newFileStore()
	t.Cleanup(store.cleanup)

	localAddrs := map[string][]addrSwap{
		"local-dev-postgres": {newAddrSwap(remote, "40001")},
	}
	overrides := map[string]string{}
	var buf bytes.Buffer
	fetchBindings(overrides, map[string]bool{}, &buf, resp,
		map[string]tunnel{"dp-a": tn}, store, localAddrs, false)

	got := overrides["DB_URL"]
	if strings.Contains(got, remote) {
		t.Errorf("fetched URL still points into the cluster: %q", got)
	}
	if !strings.Contains(got, localHost+":40001") {
		t.Errorf("fetched URL was not re-pointed at the tunnel: %q", got)
	}
	// The credential inside the URL must survive the rewrite untouched.
	if !strings.Contains(got, "app:pw@") {
		t.Errorf("rewrite mangled the value: %q", got)
	}
	if strings.Contains(buf.String(), "pw@") {
		t.Errorf("value printed to output:\n%s", buf.String())
	}
}

// A failed dial leaves the attempt announced and nothing claiming success.
func TestConnectAnnouncesAttemptNotSuccessWhenDialFails(t *testing.T) {
	resp := &remoteconnect.ResolveResponse{
		Capability: "cap",
		Targets: []remoteconnect.ResolvedTarget{{
			Key:   remoteconnect.EndpointTargetKey("finance", "ledger-svc", httpEndpointName),
			Proto: "tcp",
			Endpoint: &remoteconnect.EndpointRender{
				Scheme: httpEndpointName,
				Bindings: remoteconnect.EndpointEnvBindings{
					Address: "LEDGER_URL", Host: "LEDGER_HOST", Port: "LEDGER_PORT",
				},
			},
			AgentID: "dp-default-finance-development",
		}},
		Agents: map[string]remoteconnect.AgentEndpoint{
			"dp-default-finance-development": {
				Endpoint: "router:8443", ServerName: "dp-default-finance-development.remote-connect",
			},
		},
	}

	d := New(&fakeResolver{resp: resp})
	dialErr := errors.New("dial remote-agent 127.0.0.1:30443: EOF")
	d.dialTunnel = func(context.Context, remoteconnect.AgentEndpoint, string) (tunnel, error) {
		return nil, dialErr
	}
	d.runShell = func(context.Context, []string) error {
		t.Fatal("runShell must not run when the dial failed")
		return nil
	}

	path := writeWorkloadFileContent(t, "workload.yaml", testEndpointWorkloadYAML)
	var out bytes.Buffer
	err := d.Connect(context.Background(), ConnectParams{
		WorkloadPaths: []string{path}, Namespace: "default", Environment: "development",
	}, &out)

	if !errors.Is(err, dialErr) {
		t.Fatalf("expected the dial error to surface, got %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "Connecting to doclet/doclet-document (development)...") {
		t.Fatalf("expected the attempt to be announced, got %q", got)
	}
	if strings.Contains(got, "Connected") {
		t.Fatalf("output claims a connection that was never established: %q", got)
	}
	if strings.Contains(got, "->") {
		t.Fatalf("expected no tunnel lines when the dial failed, got %q", got)
	}
}

// One announcement per workload, followed by its tunnel lines.
func TestConnectAnnouncesEachWorkloadOnce(t *testing.T) {
	echo := startEchoServer(t)

	resp := &remoteconnect.ResolveResponse{
		Capability: "cap",
		Targets: []remoteconnect.ResolvedTarget{{
			Key:   remoteconnect.EndpointTargetKey("finance", "ledger-svc", httpEndpointName),
			Proto: "tcp",
			Endpoint: &remoteconnect.EndpointRender{
				Scheme: httpEndpointName,
				Bindings: remoteconnect.EndpointEnvBindings{
					Address: "LEDGER_URL", Host: "LEDGER_HOST", Port: "LEDGER_PORT",
				},
			},
			AgentID: "dp-default-finance-development",
		}},
		Agents: map[string]remoteconnect.AgentEndpoint{
			"dp-default-finance-development": {
				Endpoint: "router:8443", ServerName: "dp-default-finance-development.remote-connect",
			},
		},
	}

	d := New(&fakeResolver{resp: resp})
	d.dialTunnel = func(context.Context, remoteconnect.AgentEndpoint, string) (tunnel, error) {
		return &fakeTunnel{addr: echo.Addr().String()}, nil
	}
	d.runShell = func(context.Context, []string) error { return nil }

	path := writeWorkloadFileContent(t, "workload.yaml", testEndpointWorkloadYAML)
	var out bytes.Buffer
	if err := d.Connect(context.Background(), ConnectParams{
		WorkloadPaths: []string{path}, Namespace: "default", Environment: "development",
	}, &out); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	got := out.String()
	if n := strings.Count(got, "Connecting to "); n != 1 {
		t.Fatalf("expected exactly one announcement, got %d in %q", n, got)
	}
	if !strings.Contains(got, "->") {
		t.Fatalf("expected a tunnel line confirming the connection, got %q", got)
	}
}

// Fetches route to the agent the bindings name, not to whichever tunnel is open.
func TestConnectFetchesRouteToNamedAgentAcrossMultipleAgents(t *testing.T) {
	echo := startEchoServer(t)

	const endpointAgent = "dp-default-finance-development"
	const resourceAgent = "dp-default-doclet-development"

	resp := &remoteconnect.ResolveResponse{
		Capability: "cap",
		// The only target belongs to the endpoint's agent, in another project.
		Targets: []remoteconnect.ResolvedTarget{{
			Key:   remoteconnect.EndpointTargetKey("finance", "ledger-svc", httpEndpointName),
			Proto: "tcp",
			Endpoint: &remoteconnect.EndpointRender{
				Scheme:   httpEndpointName,
				Bindings: remoteconnect.EndpointEnvBindings{Host: "LEDGER_HOST", Port: "LEDGER_PORT"},
			},
			AgentID: endpointAgent,
		}},
		// The resource has values but nothing to dial, so it contributes no target.
		Resources: []remoteconnect.ResourceBindings{{
			Ref:          "bucket",
			FetchEnv:     map[string]string{"BUCKET_TOKEN": "grant-token"},
			FetchAgentID: resourceAgent,
		}},
		Agents: map[string]remoteconnect.AgentEndpoint{
			endpointAgent: {Endpoint: "router:8443", ServerName: endpointAgent + ".remote-connect"},
			resourceAgent: {Endpoint: "router:8443", ServerName: resourceAgent + ".remote-connect"},
		},
	}

	d := New(&fakeResolver{resp: resp})
	tunnels := map[string]*fakeTunnel{
		endpointAgent: {addr: echo.Addr().String()},
		resourceAgent: {addr: echo.Addr().String(), values: map[string][]byte{"grant-token": []byte("s3cret")}},
	}
	d.dialTunnel = func(_ context.Context, agent remoteconnect.AgentEndpoint, _ string) (tunnel, error) {
		for id, tn := range tunnels {
			if agent.ServerName == id+".remote-connect" {
				return tn, nil
			}
		}
		return nil, fmt.Errorf("unexpected agent %q", agent.ServerName)
	}

	var gotEnv map[string]string
	d.runShell = func(_ context.Context, env []string) error {
		gotEnv = envToMap(env)
		return nil
	}

	path := writeWorkloadFileContent(t, "workload.yaml", testEndpointWorkloadYAML)
	var out bytes.Buffer
	if err := d.Connect(context.Background(), ConnectParams{
		WorkloadPaths: []string{path}, Namespace: "default", Environment: "development",
	}, &out); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	if got := gotEnv["BUCKET_TOKEN"]; got != "s3cret" {
		t.Fatalf("BUCKET_TOKEN = %q, want the value fetched from %s; output:\n%s", got, resourceAgent, out.String())
	}
	if n := len(tunnels[endpointAgent].fetched); n != 0 {
		t.Fatalf("fetched %d key(s) from the endpoint's agent, want 0: %v", n, tunnels[endpointAgent].fetched)
	}
}

// Two mount paths that flatten to the same name get distinct local files.
func TestFileStoreKeepsFlatteningCollisionsDistinct(t *testing.T) {
	store := newFileStore()
	t.Cleanup(store.cleanup)

	// Both flatten to "etc_tls_certs_ca.pem".
	first, err := store.write("/etc/tls_certs/ca.pem", []byte("FIRST"))
	if err != nil {
		t.Fatalf("write first: %v", err)
	}
	second, err := store.write("/etc/tls/certs/ca.pem", []byte("SECOND"))
	if err != nil {
		t.Fatalf("write second: %v", err)
	}

	if first == second {
		t.Fatalf("both mount paths resolved to one local file: %s", first)
	}
	for path, want := range map[string]string{first: "FIRST", second: "SECOND"} {
		got, rerr := os.ReadFile(path)
		if rerr != nil {
			t.Fatalf("read %s: %v", path, rerr)
		}
		if string(got) != want {
			t.Fatalf("%s holds %q, want %q", path, got, want)
		}
	}
}
