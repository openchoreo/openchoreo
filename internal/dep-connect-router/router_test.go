// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package depconnectrouter

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"io"
	"math/big"
	"net"
	goruntime "runtime"
	"sync"
	"syscall"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// selfSignedCert mints a cert valid for dnsName, standing in for the cert the control
// plane provisions for a dep-agent (SAN = the agent's SNI).
func selfSignedCert(t *testing.T, dnsName string) (tls.Certificate, *x509.CertPool) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: dnsName},
		DNSNames:              []string{dnsName},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, pub, priv)
	if err != nil {
		t.Fatal(err)
	}
	leaf, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	pool := x509.NewCertPool()
	pool.AddCert(leaf)
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: priv, Leaf: leaf}, pool
}

// startTLSEchoBackend stands in for a dep-agent: it terminates TLS for testSNI and
// echoes. A successful handshake proves the router replayed the ClientHello byte-exact.
func startTLSEchoBackend(t *testing.T) (addr string, pool *x509.CertPool) {
	t.Helper()
	cert, pool := selfSignedCert(t, testSNI)
	ln, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12})
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
	return ln.Addr().String(), pool
}

// recordingDialer captures the backend the router resolved and redirects the dial to a
// reachable test address, since the registry returns in-cluster DNS names.
type recordingDialer struct {
	mu         sync.Mutex
	addrs      []string
	redirectTo string
	failWith   error
}

func (d *recordingDialer) dial(ctx context.Context, network, addr string) (net.Conn, error) {
	d.mu.Lock()
	d.addrs = append(d.addrs, addr)
	failWith, redirect := d.failWith, d.redirectTo
	d.mu.Unlock()
	if failWith != nil {
		return nil, failWith
	}
	return (&net.Dialer{}).DialContext(ctx, network, redirect)
}

func (d *recordingDialer) dialed() []string {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]string(nil), d.addrs...)
}

// startTestRouter serves a router (backed by a registry over the given Services) on an
// ephemeral port and returns its address.
func startTestRouter(t *testing.T, dialer *recordingDialer, services ...*corev1.Service) string {
	t.Helper()
	objs := make([]runtime.Object, 0, len(services))
	for _, s := range services {
		objs = append(objs, s)
	}
	reg, _ := newTestRegistry(objs...)

	r := newRouter(Config{DialTimeout: 2 * time.Second}, reg, testLogger())
	r.dialer = dialer.dial

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() { _ = r.Serve(ctx, ln) }()
	return ln.Addr().String()
}

// TestRouterRoutesBySNIAndSplices covers the core contract: peek SNI, resolve, replay the
// ClientHello, splice. The handshake completing proves passthrough — the router holds no key.
func TestRouterRoutesBySNIAndSplices(t *testing.T) {
	backendAddr, pool := startTLSEchoBackend(t)
	dialer := &recordingDialer{redirectTo: backendAddr}
	routerAddr := startTestRouter(t, dialer, agentService("dp-doclet-development", testSNI, 8443))

	conn, err := tls.Dial("tcp", routerAddr, &tls.Config{ServerName: testSNI, RootCAs: pool, MinVersion: tls.VersionTLS12})
	if err != nil {
		t.Fatalf("TLS handshake through the router failed: %v", err)
	}
	defer conn.Close()

	msg := []byte("bytes-through-the-sni-router")
	if _, err := conn.Write(msg); err != nil {
		t.Fatalf("write: %v", err)
	}
	buf := make([]byte, len(msg))
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	if _, err := io.ReadFull(conn, buf); err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(buf) != string(msg) {
		t.Fatalf("echo mismatch: got %q want %q", buf, msg)
	}

	// The router dialed the backend the registry resolved from the Service, not the
	// address occ connected to.
	if got := dialer.dialed(); len(got) != 1 || got[0] != testBackend {
		t.Fatalf("dialed = %v, want [%s]", got, testBackend)
	}
}

// TestRouterDropsUnknownSNI: an SNI with no provisioned agent must be dropped without
// dialing anything.
func TestRouterDropsUnknownSNI(t *testing.T) {
	_, pool := startTLSEchoBackend(t)
	dialer := &recordingDialer{redirectTo: "127.0.0.1:1"}
	routerAddr := startTestRouter(t, dialer, agentService("dp-doclet-development", testSNI, 8443))

	conn, err := tls.Dial("tcp", routerAddr, &tls.Config{ServerName: "nobody.dep-connect", RootCAs: pool, MinVersion: tls.VersionTLS12})
	if err == nil {
		conn.Close()
		t.Fatal("expected the handshake to fail for an unrouted SNI")
	}
	if got := dialer.dialed(); len(got) != 0 {
		t.Fatalf("router dialed on an unknown SNI: %v", got)
	}
}

// TestRouterDropsOnDialFailure: an agent that is registered but unreachable closes the
// client connection rather than hanging it.
func TestRouterDropsOnDialFailure(t *testing.T) {
	_, pool := startTLSEchoBackend(t)
	dialer := &recordingDialer{failWith: errors.New("connection refused")}
	routerAddr := startTestRouter(t, dialer, agentService("dp-doclet-development", testSNI, 8443))

	conn, err := tls.Dial("tcp", routerAddr, &tls.Config{ServerName: testSNI, RootCAs: pool, MinVersion: tls.VersionTLS12})
	if err == nil {
		conn.Close()
		t.Fatal("expected the handshake to fail when the agent dial fails")
	}
	if got := dialer.dialed(); len(got) != 1 {
		t.Fatalf("expected exactly one dial attempt, got %v", got)
	}
}

// TestRouterDropsNonTLSConnection: the router must not forward traffic it cannot read an
// SNI from — a plain HTTP request is dropped, never routed to an agent.
func TestRouterDropsNonTLSConnection(t *testing.T) {
	dialer := &recordingDialer{redirectTo: "127.0.0.1:1"}
	routerAddr := startTestRouter(t, dialer, agentService("dp-doclet-development", testSNI, 8443))

	conn, err := net.Dial("tcp", routerAddr)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if _, err := conn.Write([]byte("GET / HTTP/1.1\r\nHost: x\r\n\r\n")); err != nil {
		t.Fatal(err)
	}
	assertConnClosed(t, conn)
	if got := dialer.dialed(); len(got) != 0 {
		t.Fatalf("router dialed for a non-TLS connection: %v", got)
	}
}

// TestRouterDropsSilentConnection covers the handshake deadline: a client that connects
// and never sends a ClientHello is dropped instead of holding a goroutine forever.
func TestRouterDropsSilentConnection(t *testing.T) {
	reg, _ := newTestRegistry(agentService("dp-doclet-development", testSNI, 8443))
	dialer := &recordingDialer{redirectTo: "127.0.0.1:1"}
	r := newRouter(Config{DialTimeout: time.Second}, reg, testLogger())
	r.dialer = dialer.dial
	r.handshakeTimeout = 100 * time.Millisecond

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() { _ = r.Serve(ctx, ln) }()

	conn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	assertConnClosed(t, conn) // sends nothing; the handshake deadline must fire
	if got := dialer.dialed(); len(got) != 0 {
		t.Fatalf("router dialed for a connection that never sent a ClientHello: %v", got)
	}
}

// TestRouterServeStopsOnContextCancel: canceling the context closes the listener and
// Serve returns nil rather than an accept error.
func TestRouterServeStopsOnContextCancel(t *testing.T) {
	reg, _ := newTestRegistry()
	r := newRouter(Config{}, reg, testLogger())
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() { done <- r.Serve(ctx, ln) }()

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Serve returned %v, want nil on context cancel", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Serve did not return after context cancel")
	}
}

// flakyListener wraps a listener and fails the first failures Accept calls with err,
// standing in for transient accept failures such as fd exhaustion.
type flakyListener struct {
	net.Listener
	mu       sync.Mutex
	failWith error
	failures int
	attempts int
}

func (l *flakyListener) Accept() (net.Conn, error) {
	l.mu.Lock()
	l.attempts++
	fail := l.failures > 0
	if fail {
		l.failures--
	}
	err := l.failWith
	l.mu.Unlock()
	if fail {
		return nil, err
	}
	return l.Listener.Accept()
}

func (l *flakyListener) acceptAttempts() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.attempts
}

// TestRouterRetriesTransientAcceptErrors: fd exhaustion must not end Serve. Returning
// would exit the process and cut every live tunnel, so Serve backs off and keeps serving.
func TestRouterRetriesTransientAcceptErrors(t *testing.T) {
	backendAddr, pool := startTLSEchoBackend(t)
	dialer := &recordingDialer{redirectTo: backendAddr}
	reg, _ := newTestRegistry(agentService("dp-doclet-development", testSNI, 8443))
	r := newRouter(Config{DialTimeout: 2 * time.Second}, reg, testLogger())
	r.dialer = dialer.dial

	base, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ln := &flakyListener{Listener: base, failWith: syscall.EMFILE, failures: 3}
	t.Cleanup(func() { _ = ln.Close() })

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	done := make(chan error, 1)
	go func() { done <- r.Serve(ctx, ln) }()

	// The router survived the failures if a connection still routes end to end.
	conn, err := tls.DialWithDialer(&net.Dialer{Timeout: 10 * time.Second}, "tcp", base.Addr().String(),
		&tls.Config{ServerName: testSNI, RootCAs: pool, MinVersion: tls.VersionTLS12})
	if err != nil {
		t.Fatalf("TLS handshake after transient accept errors failed: %v", err)
	}
	defer conn.Close()

	select {
	case err := <-done:
		t.Fatalf("Serve returned %v on a transient accept error, want it to keep serving", err)
	default:
	}
	if got := ln.acceptAttempts(); got < 4 {
		t.Fatalf("accept attempts = %d, want at least 4 (3 failures + 1 success)", got)
	}
}

// TestRouterServeStopsOnPermanentAcceptError: an error Accept cannot recover from must
// surface rather than spin.
func TestRouterServeStopsOnPermanentAcceptError(t *testing.T) {
	reg, _ := newTestRegistry()
	r := newRouter(Config{}, reg, testLogger())

	base, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ln := &flakyListener{Listener: base, failWith: syscall.EINVAL, failures: 1}
	t.Cleanup(func() { _ = ln.Close() })

	done := make(chan error, 1)
	go func() { done <- r.Serve(context.Background(), ln) }()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("Serve returned nil on a permanent accept error, want an error")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Serve did not return on a permanent accept error")
	}
}

// TestRouterServeStopsListenerCloserGoroutine: the goroutine that closes ln on cancel
// must exit when Serve returns, not linger for the lifetime of a long-running ctx.
func TestRouterServeStopsListenerCloserGoroutine(t *testing.T) {
	reg, _ := newTestRegistry()
	r := newRouter(Config{}, reg, testLogger())

	base, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ln := &flakyListener{Listener: base, failWith: syscall.EINVAL, failures: 1}
	t.Cleanup(func() { _ = ln.Close() })

	before := goruntime.NumGoroutine()
	if err := r.Serve(context.Background(), ln); err == nil {
		t.Fatal("expected Serve to return an error")
	}

	// The closer goroutine exits via `stopped`; poll since goroutine teardown is async.
	for i := 0; i < 100; i++ {
		if goruntime.NumGoroutine() <= before {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("goroutine count %d did not return to %d after Serve returned", goruntime.NumGoroutine(), before)
}

// assertConnClosed asserts the peer closed the connection (read returns EOF or fails).
func assertConnClosed(t *testing.T, conn net.Conn) {
	t.Helper()
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	if n, err := conn.Read(make([]byte, 1)); err == nil {
		t.Fatalf("expected the router to close the connection, read %d bytes", n)
	}
}
