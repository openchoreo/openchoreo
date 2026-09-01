// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package remoteagentrouter

import (
	"crypto/tls"
	"net"
	"testing"
	"time"
)

// TestReadClientHelloSNI feeds a real crypto/tls ClientHello through readClientHelloSNI
// and checks the SNI is extracted and the raw bytes are preserved for replay.
func TestReadClientHelloSNI(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	// A real TLS client writes its ClientHello (with ServerName) into the pipe.
	go func() {
		c := tls.Client(clientConn, &tls.Config{ServerName: "myproj-dev.remote-connect", InsecureSkipVerify: true}) //nolint:gosec // test
		_ = c.Handshake()                                                                                           // will fail (no server); we only need the ClientHello
	}()

	_ = serverConn.SetReadDeadline(time.Now().Add(5 * time.Second))
	sni, raw, err := readClientHelloSNI(serverConn)
	if err != nil {
		t.Fatalf("readClientHelloSNI: %v", err)
	}
	if sni != "myproj-dev.remote-connect" {
		t.Fatalf("sni = %q, want myproj-dev.remote-connect", sni)
	}
	if len(raw) < 5 || raw[0] != 0x16 {
		t.Fatalf("raw does not look like a TLS handshake record: %v", raw[:min(5, len(raw))])
	}
}

func TestParseClientHelloSNINoExtension(t *testing.T) {
	// A ClientHello body with no extensions -> errNoSNI, not a panic.
	body := []byte{0x01, 0x00, 0x00, 0x00} // ClientHello header, zero-length (truncated)
	if _, err := parseClientHelloSNI(body); err == nil {
		t.Fatal("expected an error for a ClientHello with no SNI")
	}
}
