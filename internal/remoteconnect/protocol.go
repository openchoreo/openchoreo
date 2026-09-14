// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package remoteconnect

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// ProtocolVersion is the wire-protocol version of the occ <-> remote-agent tunnel. occ
// sends it in Hello so the agent can reject incompatible clients.
const ProtocolVersion = 1

// maxMessageSize bounds a single control message (handshake / stream-open / fetch
// result). It is a guard against a malformed or hostile length prefix. Handshake and
// stream-open messages are tiny; the ceiling is set by SecretResult, whose value may be
// a whole Kubernetes Secret key. Kubernetes caps a Secret at 1 MiB and JSON base64
// inflates by 4/3, so 2 MiB clears a maximal value with room for the envelope.
const maxMessageSize = 2 << 20 // 2 MiB

// MaxSecretValueSize bounds the value a fetch stream will return, chosen so the encoded
// SecretResult always fits maxMessageSize. The agent refuses a larger value with an
// error rather than writing a frame the peer will reject, which would look like a
// transport fault instead of an oversized secret.
const MaxSecretValueSize = 1 << 20 // 1 MiB

// Hello is the first message occ sends on a freshly dialed (TLS) connection, before
// yamux is layered on. It presents the CP-signed capability, which the remote-agent
// stores for the lifetime of the session and replays to the control plane's
// authorize endpoint on each StreamOpen (see stream.go).
type Hello struct {
	ProtocolVersion int `json:"protocolVersion"`
	// Capability is the compact JWT minted by the control plane's resolve endpoint
	// (see capability.go).
	Capability string `json:"capability"`
}

// HelloResult is the agent's reply to Hello. On OK, both sides layer yamux over the
// connection; otherwise the agent closes the connection.
type HelloResult struct {
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
}

// StreamOpen is the first message on each yamux stream. Key identifies which of the
// capability's authorized targets to dial; occ never sends a free-form host. The
// agent resolves Key to a concrete host:port by calling the control plane's
// authorize endpoint with the session capability.
type StreamOpen struct {
	Key string `json:"key"`
}

// StreamResult is the agent's reply to StreamOpen. After OK, the stream is a raw
// bidirectional byte pipe to the dialed target.
type StreamResult struct {
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
}

// SecretResult is the agent's reply on a fetch stream, and the only message on it: the
// agent reads the authorized key from the Kubernetes API and writes exactly one of
// these, then closes. Nothing is piped, so the stream is not a byte channel.
type SecretResult struct {
	OK bool `json:"ok"`
	// Value is the raw bytes of the referenced key (base64 in JSON). It is []byte and
	// not string because a Secret key may legitimately hold binary content — a
	// keystore, a DER certificate — and a file binding must reproduce it byte-exactly.
	Value []byte `json:"value,omitempty"`
	// Error is a short, non-value-bearing reason when OK is false.
	Error string `json:"error,omitempty"`
}

// ErrMessageTooLarge is returned by ReadMessage when the length prefix exceeds
// maxMessageSize.
var ErrMessageTooLarge = errors.New("remoteconnect: control message exceeds max size")

// WriteMessage writes v as a length-prefixed JSON control message: a 4-byte
// big-endian unsigned length followed by the JSON body.
func WriteMessage(w io.Writer, v any) error {
	body, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("remoteconnect: marshal control message: %w", err)
	}
	if len(body) > maxMessageSize {
		return ErrMessageTooLarge
	}
	var hdr [4]byte
	// len(body) is bounded by the maxMessageSize check above, so it fits in uint32.
	binary.BigEndian.PutUint32(hdr[:], uint32(len(body))) //nolint:gosec // length checked against maxMessageSize above
	if _, err := w.Write(hdr[:]); err != nil {
		return err
	}
	_, err = w.Write(body)
	return err
}

// ReadMessage reads a length-prefixed JSON control message written by WriteMessage
// and decodes it into v.
func ReadMessage(r io.Reader, v any) error {
	var hdr [4]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return err
	}
	n := binary.BigEndian.Uint32(hdr[:])
	if n > maxMessageSize {
		return ErrMessageTooLarge
	}
	body := make([]byte, n)
	if _, err := io.ReadFull(r, body); err != nil {
		return err
	}
	return json.Unmarshal(body, v)
}
