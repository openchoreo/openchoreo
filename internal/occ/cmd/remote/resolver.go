// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package remote

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/openchoreo/openchoreo/internal/occ/auth"
	"github.com/openchoreo/openchoreo/internal/occ/cmd/config"
	"github.com/openchoreo/openchoreo/internal/remoteconnect"
)

// Resolver turns a workload's declared dependencies into concrete connection targets
// plus a signed capability.
type Resolver interface {
	Resolve(ctx context.Context, req remoteconnect.ResolveRequest) (*remoteconnect.ResolveResponse, error)
}

// httpResolver calls the control plane's remote-connect resolve endpoint. The control
// plane and credential are pinned at construction, so a session keeps renewing against
// the ones it started with even if the active context changes.
type httpResolver struct {
	baseURL         string
	controlPlaneURL string
	credentialName  string
	client          *http.Client
}

// resolveStatusError carries the HTTP status of a failed resolve, so a renewing session
// can tell a permanent refusal from a transient failure worth retrying.
type resolveStatusError struct {
	status int
	body   string
}

func (e *resolveStatusError) Error() string {
	if e.body == "" {
		return fmt.Sprintf("resolve failed with status %d", e.status)
	}
	return fmt.Sprintf("resolve failed: %d: %s", e.status, e.body)
}

// terminal reports whether retrying this failure is pointless: 401 the login can no
// longer be refreshed, 403 the control plane refused on policy grounds.
func (e *resolveStatusError) terminal() bool {
	return e.status == http.StatusUnauthorized || e.status == http.StatusForbidden
}

const resolvePath = "/api/v1/remote-connect:resolve"

// newHTTPResolver builds a resolver targeting the current control plane. The credential
// is resolved here so a missing login fails early, but its token is read per call: a
// renewing session outlives its access token.
func newHTTPResolver() (Resolver, error) {
	// One snapshot, so the control plane and credential cannot come from different
	// contexts if the active one changes underneath.
	cfg, err := config.LoadStoredConfig()
	if err != nil {
		return nil, err
	}
	cp, err := cfg.ActiveControlPlane()
	if err != nil {
		return nil, err
	}
	cred, err := cfg.ActiveCredential()
	if err != nil {
		return nil, err
	}
	return &httpResolver{
		baseURL:         strings.TrimRight(cp.URL, "/"),
		controlPlaneURL: cp.URL,
		credentialName:  cred.Name,
		client:          &http.Client{Timeout: 30 * time.Second},
	}, nil
}

// callToken returns the bearer token for one resolve, refreshing an expired one first.
// A refresh failure is returned rather than sending a stale token, whose 401 a renewing
// session would read as permanent.
func (h *httpResolver) callToken() (string, error) {
	cfg, err := config.LoadStoredConfig()
	if err != nil {
		return "", err
	}
	cred, err := cfg.CredentialByName(h.credentialName)
	if err != nil {
		return "", err
	}
	if cred.Token != "" && !auth.IsTokenExpired(cred.Token) {
		return cred.Token, nil
	}
	if !auth.CanRefresh(cred) {
		return cred.Token, nil
	}
	return auth.RefreshCredential(h.credentialName, h.controlPlaneURL)
}

func (h *httpResolver) Resolve(ctx context.Context, req remoteconnect.ResolveRequest) (*remoteconnect.ResolveResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal resolve request: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, h.baseURL+resolvePath, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	token, err := h.callToken()
	if err != nil {
		return nil, fmt.Errorf("refresh access token: %w", err)
	}
	if token != "" {
		httpReq.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := h.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("call resolve endpoint: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, &resolveStatusError{status: resp.StatusCode, body: strings.TrimSpace(string(msg))}
	}

	var out remoteconnect.ResolveResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("decode resolve response: %w", err)
	}
	return &out, nil
}
