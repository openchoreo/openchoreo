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

// httpResolver calls the control plane's remote-connect resolve endpoint.
type httpResolver struct {
	baseURL string
	token   string
	client  *http.Client
}

const resolvePath = "/api/v1/remote-connect:resolve"

// newHTTPResolver builds a resolver targeting the current control plane, refreshing
// the credential token if needed (mirrors internal/occ/cmd/component/exec.go).
func newHTTPResolver() (Resolver, error) {
	cp, err := config.GetCurrentControlPlane()
	if err != nil {
		return nil, err
	}
	cred, err := config.GetCurrentCredential()
	if err != nil {
		return nil, err
	}
	token := cred.Token
	if token != "" && auth.IsTokenExpired(token) {
		if refreshed, rerr := auth.RefreshToken(); rerr == nil {
			token = refreshed
		}
	}
	return &httpResolver{
		baseURL: strings.TrimRight(cp.URL, "/"),
		token:   token,
		client:  &http.Client{Timeout: 30 * time.Second},
	}, nil
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
	if h.token != "" {
		httpReq.Header.Set("Authorization", "Bearer "+h.token)
	}

	resp, err := h.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("call resolve endpoint: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("resolve failed: %s: %s", resp.Status, strings.TrimSpace(string(msg)))
	}

	var out remoteconnect.ResolveResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("decode resolve response: %w", err)
	}
	return &out, nil
}
