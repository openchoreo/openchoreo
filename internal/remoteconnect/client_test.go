// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package remoteconnect

import (
	"errors"
	"net"
	"strings"
	"testing"
)

// A refusal is reported as HandshakeRejectedError so callers can tell the agent's answer
// apart from a connection that has not come up yet.
func TestNewTunnelClientReportsRefusalAsTypedError(t *testing.T) {
	client, server := net.Pipe()
	t.Cleanup(func() { _ = client.Close(); _ = server.Close() })

	const reason = "this remote-agent is already serving its maximum number of sessions"
	go func() {
		var hello Hello
		_ = ReadMessage(server, &hello)
		_ = WriteMessage(server, HelloResult{OK: false, Error: reason})
	}()

	_, err := NewTunnelClient(client, func() string { return "capability" })
	if err == nil {
		t.Fatal("expected the handshake to be rejected")
	}
	var rejected *HandshakeRejectedError
	if !errors.As(err, &rejected) {
		t.Fatalf("error %v is not a *HandshakeRejectedError", err)
	}
	if rejected.Reason != reason {
		t.Errorf("reason = %q, want %q", rejected.Reason, reason)
	}
	if !strings.Contains(err.Error(), reason) {
		t.Errorf("error text %q does not carry the reason", err.Error())
	}
}
