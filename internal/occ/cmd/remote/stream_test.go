// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package remote

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"math/big"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/openchoreo/openchoreo/internal/remoteconnect"
)

// TestDialRemoteAgentTunnelHonoursCancel: canceling mid-retry gives up promptly rather
// than running out the retry window.
func TestDialRemoteAgentTunnelHonoursCancel(t *testing.T) {
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

	agent := remoteconnect.AgentEndpoint{Endpoint: ln.Addr().String(), ServerName: "agent.remote-connect"}
	start := time.Now()
	_, derr := dialRemoteAgentTunnel(ctx, agent, func() string { return "capability" })
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

// agentTLS builds the self-signed server certificate an agent presents, in the shape occ
// pins it: the leaf itself is the trusted root.
func agentTLS(t *testing.T, serverName string) (*tls.Config, string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: serverName},
		DNSNames:              []string{serverName},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	pair, err := tls.X509KeyPair(certPEM, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER}))
	if err != nil {
		t.Fatal(err)
	}
	return &tls.Config{Certificates: []tls.Certificate{pair}, MinVersion: tls.VersionTLS12}, string(certPEM)
}

// TestDialRemoteAgentTunnelDoesNotRetryRefusal: the retry window covers an agent that is
// still coming up, so an agent that answers with a refusal must end the dial at once. The
// accept count is the proof: a retry would open a second connection.
func TestDialRemoteAgentTunnelDoesNotRetryRefusal(t *testing.T) {
	const serverName = "agent.remote-connect"
	const reason = "this remote-agent is already serving its maximum number of sessions"
	tlsCfg, caBundle := agentTLS(t, serverName)

	ln, err := tls.Listen("tcp", "127.0.0.1:0", tlsCfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })

	var mu sync.Mutex
	accepts := 0
	go func() {
		for {
			c, aerr := ln.Accept()
			if aerr != nil {
				return
			}
			mu.Lock()
			accepts++
			mu.Unlock()
			go func(c net.Conn) {
				defer c.Close()
				var hello remoteconnect.Hello
				if remoteconnect.ReadMessage(c, &hello) == nil {
					_ = remoteconnect.WriteMessage(c, remoteconnect.HelloResult{OK: false, Error: reason})
				}
			}(c)
		}
	}()

	agent := remoteconnect.AgentEndpoint{Endpoint: ln.Addr().String(), CABundle: caBundle, ServerName: serverName}
	start := time.Now()
	_, derr := dialRemoteAgentTunnel(context.Background(), agent, func() string { return "capability" })
	elapsed := time.Since(start)

	if derr == nil {
		t.Fatal("expected the dial to fail")
	}
	var rejected *remoteconnect.HandshakeRejectedError
	if !errors.As(derr, &rejected) {
		t.Fatalf("error %v does not wrap *HandshakeRejectedError", derr)
	}
	if !strings.Contains(derr.Error(), reason) {
		t.Errorf("error %q does not carry the refusal reason", derr.Error())
	}
	mu.Lock()
	got := accepts
	mu.Unlock()
	if got != 1 {
		t.Errorf("agent saw %d connections, want 1 (the refusal was retried)", got)
	}
	if elapsed > 5*time.Second {
		t.Errorf("refusal took %v to surface; retry window is %v", elapsed, dialRetryTimeout)
	}
}
