// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package remoteagent

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/openchoreo/openchoreo/internal/remoteconnect"
)

// fakeValueReader answers reads from an in-memory map keyed by "<kind>/<name>/<key>".
type fakeValueReader struct {
	values map[string][]byte
	err    error
	reads  []remoteconnect.SecretGrant
}

func (f *fakeValueReader) read(_ context.Context, grant remoteconnect.SecretGrant) ([]byte, error) {
	f.reads = append(f.reads, grant)
	if f.err != nil {
		return nil, f.err
	}
	v, ok := f.values[grant.SourceKind+"/"+grant.SourceName+"/"+grant.SourceKey]
	if !ok {
		return nil, errors.New("not found")
	}
	return v, nil
}

// serveFetchAgent is serveAgent plus a value reader, and with the agent's own namespace
// set so the namespace guard is exercised rather than skipped.
func serveFetchAgent(t *testing.T, auth streamAuthorizer, values valueReader, namespace string) *remoteconnect.TunnelClient {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })

	cfg := Config{Namespace: namespace}.withDefaults()
	srv := NewServer(cfg, auth, slog.New(slog.NewTextHandler(io.Discard, nil)))
	srv.values = values

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() { _ = srv.Serve(ctx, ln) }()

	conn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	client, err := remoteconnect.NewTunnelClient(conn, "test-capability")
	if err != nil {
		t.Fatalf("handshake: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })
	return client
}

func TestAgentFetchRoundTrip(t *testing.T) {
	const ns = "dp-default-doclet-development"
	key := remoteconnect.SecretGrantKey("doclet-postgres", "password")
	auth := &fakeAuthorizer{targets: map[string]remoteconnect.AuthorizeResponse{
		key: {
			Kind:           remoteconnect.AuthorizeKindSecret,
			AgentNamespace: ns,
			Secret: &remoteconnect.SecretGrant{
				Key: key, AgentNamespace: ns,
				SourceKind: remoteconnect.SourceKindSecret, SourceName: "pg-conn", SourceKey: "password",
			},
		},
	}}
	values := &fakeValueReader{values: map[string][]byte{"Secret/pg-conn/password": []byte("s3cr3t")}}
	client := serveFetchAgent(t, auth, values, ns)

	got, err := client.Fetch(key)
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if string(got) != "s3cr3t" {
		t.Fatalf("got %q, want %q", got, "s3cr3t")
	}
	// The agent must read from its OWN namespace, never one named by the client.
	if len(values.reads) != 1 || values.reads[0].SourceName != "pg-conn" {
		t.Fatalf("unexpected reads: %+v", values.reads)
	}
}

// A binary value must survive the round trip byte-for-byte: a file binding may carry a
// keystore or a DER certificate, and any re-encoding would corrupt it silently.
func TestAgentFetchPreservesBinaryValue(t *testing.T) {
	const ns = "dp-ns"
	key := remoteconnect.SecretGrantKey("res", "keystore")
	want := []byte{0x00, 0xff, 0xfe, 0x01, 0x80, 0x00}
	auth := &fakeAuthorizer{targets: map[string]remoteconnect.AuthorizeResponse{
		key: {
			Kind: remoteconnect.AuthorizeKindSecret, AgentNamespace: ns,
			Secret: &remoteconnect.SecretGrant{
				Key: key, AgentNamespace: ns,
				SourceKind: remoteconnect.SourceKindSecret, SourceName: "ks", SourceKey: "keystore",
			},
		},
	}}
	values := &fakeValueReader{values: map[string][]byte{"Secret/ks/keystore": want}}
	client := serveFetchAgent(t, auth, values, ns)

	got, err := client.Fetch(key)
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if string(got) != string(want) {
		t.Fatalf("binary value not preserved: got %v want %v", got, want)
	}
}

// countingEcho is startEcho plus a connection count, so a test can assert the agent
// never dialed rather than only that the client saw an error.
func countingEcho(t *testing.T) (addr string, conns func() int) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })

	var mu sync.Mutex
	n := 0
	go func() {
		for {
			c, aerr := ln.Accept()
			if aerr != nil {
				return
			}
			mu.Lock()
			n++
			mu.Unlock()
			go func(c net.Conn) { defer c.Close(); _, _ = io.Copy(c, c) }(c)
		}
	}()
	return ln.Addr().String(), func() int {
		mu.Lock()
		defer mu.Unlock()
		return n
	}
}

// A fetch key answered with a dial target must not produce a connection. The client
// seeing an error is not enough: what matters is that no TCP session was opened to a
// host the fetch key never authorized.
func TestAgentRefusesFetchKeyAnsweredWithDialTarget(t *testing.T) {
	addr, conns := countingEcho(t)
	host, portStr, _ := net.SplitHostPort(addr)
	port, _ := strconv.Atoi(portStr)

	key := remoteconnect.SecretGrantKey("doclet-postgres", "password")
	auth := &fakeAuthorizer{targets: map[string]remoteconnect.AuthorizeResponse{
		// Kind left empty, i.e. "tcp" — a dial answer to a fetch key.
		key: {Host: host, Port: port, Proto: "tcp", AgentNamespace: "dp-ns"},
	}}
	values := &fakeValueReader{}
	client := serveFetchAgent(t, auth, values, "dp-ns")

	if _, err := client.Fetch(key); err == nil {
		t.Fatal("expected the mismatched kind to be refused")
	}
	if got := conns(); got != 0 {
		t.Errorf("agent dialed %d time(s) for a fetch key", got)
	}
	if len(values.reads) != 0 {
		t.Errorf("a refused stream still read a value: %+v", values.reads)
	}
}

// The mirror image: a dial key answered with a secret grant must be refused rather than
// served. The response carries a usable dial target as well, so without the kind check
// the agent would happily pipe bytes for an answer that says it is a value read — acting
// on an authorization the control plane and the client disagree about.
func TestAgentRefusesDialKeyAnsweredWithSecretGrant(t *testing.T) {
	const ns = "dp-ns"
	addr, conns := countingEcho(t)
	host, portStr, _ := net.SplitHostPort(addr)
	port, _ := strconv.Atoi(portStr)

	auth := &fakeAuthorizer{targets: map[string]remoteconnect.AuthorizeResponse{
		"ep/greeter/http": {
			Kind: remoteconnect.AuthorizeKindSecret, AgentNamespace: ns,
			Host: host, Port: port, Proto: "tcp",
			Secret: &remoteconnect.SecretGrant{
				Key: "ep/greeter/http", AgentNamespace: ns,
				SourceKind: remoteconnect.SourceKindSecret, SourceName: "pg-conn", SourceKey: "password",
			},
		},
	}}
	values := &fakeValueReader{values: map[string][]byte{"Secret/pg-conn/password": []byte("s3cr3t")}}
	client := serveFetchAgent(t, auth, values, ns)

	if _, err := client.OpenStream("ep/greeter/http"); err == nil {
		t.Fatal("expected the mismatched kind to be refused")
	}
	if got := conns(); got != 0 {
		t.Errorf("agent dialed %d time(s) for a kind-mismatched answer", got)
	}
	if len(values.reads) != 0 {
		t.Errorf("a refused dial still read a value: %+v", values.reads)
	}
}

// A grant routed to another agent's namespace must be refused even though the control
// plane authorized it: the agent enforces "act from your own namespace" itself rather
// than trusting the client's choice of which agent to open the stream against.
func TestAgentRefusesFetchForAnotherNamespace(t *testing.T) {
	key := remoteconnect.SecretGrantKey("res", "password")
	auth := &fakeAuthorizer{targets: map[string]remoteconnect.AuthorizeResponse{
		key: {
			Kind: remoteconnect.AuthorizeKindSecret, AgentNamespace: "dp-other-project",
			Secret: &remoteconnect.SecretGrant{
				Key: key, AgentNamespace: "dp-other-project",
				SourceKind: remoteconnect.SourceKindSecret, SourceName: "pg-conn", SourceKey: "password",
			},
		},
	}}
	values := &fakeValueReader{values: map[string][]byte{"Secret/pg-conn/password": []byte("s3cr3t")}}
	client := serveFetchAgent(t, auth, values, "dp-this-project")

	if _, err := client.Fetch(key); err == nil {
		t.Fatal("expected a cross-namespace grant to be refused")
	}
	if len(values.reads) != 0 {
		t.Errorf("a refused stream still read a value: %+v", values.reads)
	}
}

// An agent with no Kubernetes identity says so per stream rather than hanging or
// failing to start: it can still serve tunnels, and the developer needs a reason.
func TestAgentFetchWithoutReaderReportsPlainly(t *testing.T) {
	const ns = "dp-ns"
	key := remoteconnect.SecretGrantKey("res", "password")
	auth := &fakeAuthorizer{targets: map[string]remoteconnect.AuthorizeResponse{
		key: {
			Kind: remoteconnect.AuthorizeKindSecret, AgentNamespace: ns,
			Secret: &remoteconnect.SecretGrant{
				Key: key, AgentNamespace: ns,
				SourceKind: remoteconnect.SourceKindSecret, SourceName: "pg-conn", SourceKey: "password",
			},
		},
	}}
	client := serveFetchAgent(t, auth, nil, ns)

	_, err := client.Fetch(key)
	if err == nil {
		t.Fatal("expected an error when the agent cannot read values")
	}
	if !strings.Contains(err.Error(), "cannot read values") {
		t.Errorf("unhelpful error %q", err)
	}
}

// A read failure must not relay the API server's message, which can name objects and
// fields the developer's terminal has no business carrying.
func TestAgentFetchErrorIsCoarse(t *testing.T) {
	const ns = "dp-ns"
	key := remoteconnect.SecretGrantKey("res", "password")
	auth := &fakeAuthorizer{targets: map[string]remoteconnect.AuthorizeResponse{
		key: {
			Kind: remoteconnect.AuthorizeKindSecret, AgentNamespace: ns,
			Secret: &remoteconnect.SecretGrant{
				Key: key, AgentNamespace: ns,
				SourceKind: remoteconnect.SourceKindSecret, SourceName: "pg-conn", SourceKey: "password",
			},
		},
	}}
	values := &fakeValueReader{err: errors.New(`secrets "pg-conn" is forbidden: User "system:serviceaccount:x" cannot get`)}
	client := serveFetchAgent(t, auth, values, ns)

	_, err := client.Fetch(key)
	if err == nil {
		t.Fatal("expected an error")
	}
	if strings.Contains(err.Error(), "serviceaccount") {
		t.Errorf("API server detail relayed to the client: %q", err)
	}
}

// A value over the frame cap is refused with a reason, not written as an oversized
// frame the peer rejects for length — which would read as a transport fault.
func TestAgentFetchRefusesOversizedValue(t *testing.T) {
	const ns = "dp-ns"
	key := remoteconnect.SecretGrantKey("res", "big")
	auth := &fakeAuthorizer{targets: map[string]remoteconnect.AuthorizeResponse{
		key: {
			Kind: remoteconnect.AuthorizeKindSecret, AgentNamespace: ns,
			Secret: &remoteconnect.SecretGrant{
				Key: key, AgentNamespace: ns,
				SourceKind: remoteconnect.SourceKindSecret, SourceName: "big", SourceKey: "big",
			},
		},
	}}
	values := &fakeValueReader{values: map[string][]byte{
		"Secret/big/big": make([]byte, remoteconnect.MaxSecretValueSize+1),
	}}
	client := serveFetchAgent(t, auth, values, ns)

	_, err := client.Fetch(key)
	if err == nil {
		t.Fatal("expected an oversized value to be refused")
	}
	if !strings.Contains(err.Error(), "too large") {
		t.Errorf("unhelpful error %q", err)
	}
}
