// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package depagent

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/openchoreo/openchoreo/internal/depconnect"
)

// authorizer calls the control plane's authorize endpoint to turn a (capability,
// key) pair into a concrete dial target. The dep-agent trusts the control plane to
// be the sole authority on which targets a capability may reach; it never verifies
// the capability itself (it holds no signing key).
type authorizer struct {
	url          string
	heartbeatURL string
	client       *http.Client
}

// newAuthorizer builds an authorizer for url (and, optionally, heartbeatURL),
// optionally pinning caBundlePath for the control plane's TLS. timeout bounds each call.
func newAuthorizer(url, heartbeatURL, caBundlePath string, insecure bool, timeout time.Duration) (*authorizer, error) {
	tlsCfg := &tls.Config{MinVersion: tls.VersionTLS12, InsecureSkipVerify: insecure} //nolint:gosec // insecure is opt-in for dev
	if caBundlePath != "" {
		pem, err := os.ReadFile(caBundlePath)
		if err != nil {
			return nil, fmt.Errorf("dep-agent: read authorize CA bundle: %w", err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(pem) {
			return nil, fmt.Errorf("dep-agent: invalid authorize CA bundle at %s", caBundlePath)
		}
		tlsCfg.RootCAs = pool
	}
	return &authorizer{
		url:          url,
		heartbeatURL: heartbeatURL,
		client: &http.Client{
			Timeout:   timeout,
			Transport: &http.Transport{TLSClientConfig: tlsCfg},
		},
	}, nil
}

// heartbeat POSTs a liveness refresh for agentNamespace, presenting a capability the
// agent currently holds. Best-effort: a non-2xx response is returned as an error for
// the caller to log; it does not affect any live stream.
func (a *authorizer) heartbeat(ctx context.Context, capability, agentNamespace string) error {
	body, err := json.Marshal(depconnect.HeartbeatRequest{Capability: capability, AgentNamespace: agentNamespace})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.heartbeatURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := a.client.Do(req)
	if err != nil {
		return fmt.Errorf("dep-agent: heartbeat call: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode/100 != 2 {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("heartbeat rejected (HTTP %d): %s", resp.StatusCode, bytes.TrimSpace(msg))
	}
	return nil
}

// authorize asks the control plane for the dial target authorized by (capability,
// key). A non-2xx response (invalid/expired capability, unknown key) is returned as
// an error and the stream is refused.
func (a *authorizer) authorize(ctx context.Context, capability, key string) (*depconnect.AuthorizeResponse, error) {
	body, err := json.Marshal(depconnect.AuthorizeRequest{Capability: capability, Key: key})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := a.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("dep-agent: authorize call: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("authorize rejected (HTTP %d): %s", resp.StatusCode, bytes.TrimSpace(msg))
	}

	var out depconnect.AuthorizeResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("dep-agent: decode authorize response: %w", err)
	}
	return &out, nil
}
