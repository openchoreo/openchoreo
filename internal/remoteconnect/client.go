// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package remoteconnect

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net"

	"github.com/hashicorp/yamux"
)

// TunnelClient is the occ-side endpoint of a remote-connect tunnel: it performs the
// capability handshake over a single connection to a project+env remote-agent and
// multiplexes one stream per accepted local connection with yamux.
type TunnelClient struct {
	conn    net.Conn
	session *yamux.Session
}

// NewTunnelClient runs the Hello/HelloResult handshake over an already-established
// connection (TLS in production; plain in tests) and layers a yamux client session.
func NewTunnelClient(conn net.Conn, capability string) (*TunnelClient, error) {
	if err := WriteMessage(conn, Hello{ProtocolVersion: ProtocolVersion, Capability: capability}); err != nil {
		return nil, fmt.Errorf("remoteconnect: send hello: %w", err)
	}
	var res HelloResult
	if err := ReadMessage(conn, &res); err != nil {
		return nil, fmt.Errorf("remoteconnect: read hello result: %w", err)
	}
	if !res.OK {
		return nil, fmt.Errorf("remoteconnect: tunnel handshake rejected: %s", res.Error)
	}

	ycfg := yamux.DefaultConfig()
	ycfg.LogOutput = io.Discard // quiet yamux; occ surfaces its own errors
	session, err := yamux.Client(conn, ycfg)
	if err != nil {
		return nil, fmt.Errorf("remoteconnect: yamux client: %w", err)
	}
	return &TunnelClient{conn: conn, session: session}, nil
}

// DialTunnel dials the remote-agent endpoint over TLS and constructs a TunnelClient.
// The agent presents its own self-signed certificate rather than one from a gateway
// CA, so occ pins caBundle and verifies against serverName (a fixed SAN baked into
// the cert, decoupled from the runtime L4 address, which is unknown at cert-generation
// time). If caBundle is empty, the system roots and endpoint host are used.
func DialTunnel(ctx context.Context, endpoint, caBundle, serverName, capability string) (*TunnelClient, error) {
	tlsCfg := &tls.Config{MinVersion: tls.VersionTLS12}
	if caBundle != "" {
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM([]byte(caBundle)) {
			return nil, errors.New("remoteconnect: invalid agent CA bundle")
		}
		tlsCfg.RootCAs = pool
	}
	if serverName != "" {
		tlsCfg.ServerName = serverName
	}
	conn, err := (&tls.Dialer{Config: tlsCfg}).DialContext(ctx, "tcp", endpoint)
	if err != nil {
		return nil, fmt.Errorf("remoteconnect: dial remote-agent %s: %w", endpoint, err)
	}
	c, err := NewTunnelClient(conn, capability)
	if err != nil {
		_ = conn.Close()
		return nil, err
	}
	return c, nil
}

// OpenStream opens a multiplexed stream and requests the target identified by key.
// The returned net.Conn is a transparent byte pipe to the dialed dependency.
func (c *TunnelClient) OpenStream(key string) (net.Conn, error) {
	stream, err := c.session.OpenStream()
	if err != nil {
		return nil, fmt.Errorf("remoteconnect: open stream: %w", err)
	}
	if err := WriteMessage(stream, StreamOpen{Key: key}); err != nil {
		_ = stream.Close()
		return nil, err
	}
	var res StreamResult
	if err := ReadMessage(stream, &res); err != nil {
		_ = stream.Close()
		return nil, err
	}
	if !res.OK {
		_ = stream.Close()
		return nil, fmt.Errorf("remoteconnect: stream to %q rejected: %s", key, res.Error)
	}
	return stream, nil
}

// Fetch opens a stream for a value-fetch key and returns the value the remote-agent read
// from its own namespace. Unlike OpenStream this consumes the whole exchange: a fetch
// stream carries exactly one reply and is closed here, so no byte pipe is handed back.
//
// The value is returned to the caller and never logged, written to disk by this package,
// or included in an error — an error mentioning the value would put secret material into
// occ's terminal output.
func (c *TunnelClient) Fetch(key string) ([]byte, error) {
	if !IsSecretGrantKey(key) {
		return nil, fmt.Errorf("remoteconnect: %q is not a value-fetch key", key)
	}
	stream, err := c.session.OpenStream()
	if err != nil {
		return nil, fmt.Errorf("remoteconnect: open stream: %w", err)
	}
	defer stream.Close()

	if err := WriteMessage(stream, StreamOpen{Key: key}); err != nil {
		return nil, err
	}
	var res SecretResult
	if err := ReadMessage(stream, &res); err != nil {
		return nil, err
	}
	if !res.OK {
		return nil, fmt.Errorf("%s", res.Error)
	}
	return res.Value, nil
}

// Close tears down the session and underlying connection.
func (c *TunnelClient) Close() error {
	if c.session != nil {
		_ = c.session.Close()
	}
	return c.conn.Close()
}
