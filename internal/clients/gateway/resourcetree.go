// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"

	"github.com/openchoreo/openchoreo/internal/resourcetree/protocol"
)

// ErrResourceTreeUnsupported reports that the far side cannot answer resource
// tree match queries: either the cluster agent predates the resource-tree
// target, or the gateway predates this endpoint. Callers treat it as the one
// condition that permits falling back to the legacy list-and-filter traversal;
// every other error is a hard discovery failure with no fallback.
var ErrResourceTreeUnsupported = errors.New("cluster agent does not support resource-tree matching")

const (
	// DefaultResourceTreeTimeout is the wall clock a match batch gets. It is the
	// top rung of protocol's timeout ladder, which owns the ordering against the
	// gateway's tunnel timeout and the agent's own budget.
	DefaultResourceTreeTimeout = protocol.ClientRequestTimeout

	// maxResourceTreeResponseBytes caps the decoded match response. The agent
	// caps its own payload below this, so exceeding it means something other
	// than a match response is on the wire.
	maxResourceTreeResponseBytes int64 = 10 * 1024 * 1024 // 10MB

	// errorBodySnippetBytes bounds how much of an error response body is
	// quoted back in the returned error.
	errorBodySnippetBytes = 512
)

// MatchResourceTreeChildren asks the cluster agent behind the gateway to
// resolve one batch of child match queries.
//
// A 404 — and only a 404 — maps to ErrResourceTreeUnsupported. The agent's
// match handler never answers 404 (it reports per-query failures inside a 200
// response), and the gateway endpoint never raises one either, so a 404 can
// only come from an agent whose router does not know the resource-tree target
// or from a gateway with no such route. The response body is folded into the
// error for diagnosis but is never required for the mapping to hold.
//
// Everything else — 5xx, 403, timeouts, unreadable bodies, a version the
// request did not ask for, results that do not line up with the queries sent —
// is a plain error. Callers must not fall back on those.
func (c *Client) MatchResourceTreeChildren(
	ctx context.Context,
	planeType, planeID, crNamespace, crName string,
	req *protocol.MatchRequest,
) (*protocol.MatchResponse, error) {
	if req == nil {
		return nil, fmt.Errorf("resource-tree match request is required")
	}
	queryIDs, err := requestQueryIDs(req)
	if err != nil {
		return nil, err
	}

	payload, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal resource-tree match request: %w", err)
	}

	matchURL := c.baseURL + protocol.BuildGatewayMatchesPath(planeType, planeID, crNamespace, crName)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, matchURL, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("failed to create resource-tree match request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")

	resp, err := c.resourceTreeHTTPClient().Do(httpReq)
	if err != nil {
		return nil, &TransientError{
			Message: "failed to send resource-tree match request",
			Err:     err,
		}
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResourceTreeResponseBytes+1))
	if err != nil {
		return nil, fmt.Errorf("failed to read resource-tree match response: %w", err)
	}
	if int64(len(body)) > maxResourceTreeResponseBytes {
		return nil, fmt.Errorf("resource-tree match response is too large, max is %d bytes", maxResourceTreeResponseBytes)
	}

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("%w (gateway returned 404: %s)", ErrResourceTreeUnsupported, bodySnippet(body))
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("resource-tree match request failed: %w (body: %s)",
			classifyHTTPError(resp.StatusCode), bodySnippet(body))
	}

	var matchResp protocol.MatchResponse
	if err := json.Unmarshal(body, &matchResp); err != nil {
		return nil, fmt.Errorf("failed to decode resource-tree match response: %w", err)
	}

	if err := verifyMatchResponse(req.Version, queryIDs, &matchResp); err != nil {
		return nil, err
	}

	return &matchResp, nil
}

// resourceTreeHTTPClient returns the dedicated client this endpoint needs.
// Clients built through NewClientWithConfig always have one; the fallback only
// covers hand-assembled Client values.
func (c *Client) resourceTreeHTTPClient() *http.Client {
	if c.resourceTreeClient != nil {
		return c.resourceTreeClient
	}
	return c.httpClient
}

// requestQueryIDs collects the query IDs a response must answer. Duplicate IDs
// are rejected here rather than sent: the agent answers per query, so a
// duplicated ID makes exact coverage unverifiable and the caller could not tell
// which result belongs to which query.
func requestQueryIDs(req *protocol.MatchRequest) ([]string, error) {
	ids := make([]string, 0, len(req.Queries))
	seen := make(map[string]struct{}, len(req.Queries))
	for _, q := range req.Queries {
		if _, dup := seen[q.ID]; dup {
			return nil, fmt.Errorf("duplicate query id %q in resource-tree match request", q.ID)
		}
		seen[q.ID] = struct{}{}
		ids = append(ids, q.ID)
	}
	return ids, nil
}

// verifyMatchResponse holds the agent to the contract before any result is
// handed to a caller: the version must be the one asked for, and the results
// must cover the queries sent exactly — no missing, duplicate, or unexpected
// IDs. Each of these is a plain error, never ErrResourceTreeUnsupported.
func verifyMatchResponse(wantVersion string, queryIDs []string, resp *protocol.MatchResponse) error {
	if resp.Version != wantVersion {
		return fmt.Errorf("resource-tree match response version %q does not match the requested version %q",
			resp.Version, wantVersion)
	}

	want := make(map[string]bool, len(queryIDs))
	for _, id := range queryIDs {
		want[id] = false
	}

	for _, result := range resp.Results {
		answered, expected := want[result.ID]
		if !expected {
			return fmt.Errorf("resource-tree match response contains result for unknown query id %q", result.ID)
		}
		if answered {
			return fmt.Errorf("resource-tree match response contains duplicate result for query id %q", result.ID)
		}
		// A non-nil error must carry a code this protocol version defines.
		// Accepting an empty or unrecognized code would silently downgrade an
		// actionable classification (e.g. Forbidden) to a generic discovery error.
		if result.Error != nil && !protocol.IsKnownErrorCode(result.Error.Code) {
			return fmt.Errorf("resource-tree match response for query id %q carries an unrecognized error code %q",
				result.ID, result.Error.Code)
		}
		want[result.ID] = true
	}

	missing := make([]string, 0)
	for id, answered := range want {
		if !answered {
			missing = append(missing, id)
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return fmt.Errorf("resource-tree match response is missing results for query ids: %s",
			strings.Join(missing, ", "))
	}

	return nil
}

// bodySnippet renders an error response body for a log line: bounded in length
// and stripped of the trailing newline http.Error appends.
func bodySnippet(body []byte) string {
	if len(body) == 0 {
		return "<empty>"
	}
	if int64(len(body)) > errorBodySnippetBytes {
		return strings.TrimSpace(string(body[:errorBodySnippetBytes])) + "…(truncated)"
	}
	return strings.TrimSpace(string(body))
}
