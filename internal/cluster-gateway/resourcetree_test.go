// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clustergateway

import (
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openchoreo/openchoreo/internal/cluster-agent/messaging"
	"github.com/openchoreo/openchoreo/internal/resourcetree/protocol"
)

// capturedTunnelSend records what handleResourceTreeMatches asked the tunnel to
// deliver, so a test can assert the constructed HTTPTunnelRequest without a
// live agent connection.
type capturedTunnelSend struct {
	planeIdentifier string
	crKey           string
	req             *messaging.HTTPTunnelRequest
	timeout         time.Duration
	calls           int
}

// stubResourceTreeSend replaces the package-level tunnel seam and restores it
// on cleanup, mirroring stubGetAgentConnectionForWirelogs.
func stubResourceTreeSend(t *testing.T, resp *messaging.HTTPTunnelResponse, err error) *capturedTunnelSend {
	t.Helper()
	captured := &capturedTunnelSend{}
	prev := sendResourceTreeMatchRequest
	sendResourceTreeMatchRequest = func(
		_ *Server, planeIdentifier, crKey string,
		req *messaging.HTTPTunnelRequest, timeout time.Duration,
	) (*messaging.HTTPTunnelResponse, error) {
		captured.planeIdentifier = planeIdentifier
		captured.crKey = crKey
		captured.req = req
		captured.timeout = timeout
		captured.calls++
		return resp, err
	}
	t.Cleanup(func() { sendResourceTreeMatchRequest = prev })
	return captured
}

func newResourceTreeTestServer() *Server {
	return &Server{logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
}

func newMatchesRequest(path, body string) *http.Request {
	r := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	return r
}

func TestHandleResourceTreeMatches_RejectsNonPOST(t *testing.T) {
	captured := stubResourceTreeSend(t, nil, nil)
	s := newResourceTreeTestServer()
	rec := httptest.NewRecorder()

	r := httptest.NewRequest(http.MethodGet, "/api/resource-tree/dataplane/p1/ns1/cr1/matches", nil)
	s.handleResourceTreeMatches(rec, r)

	assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)
	assert.Zero(t, captured.calls, "a rejected method must never reach the tunnel")
}

func TestHandleResourceTreeMatches_RejectsMalformedPath(t *testing.T) {
	tests := []struct {
		name string
		path string
	}{
		{"missing matches suffix", "/api/resource-tree/dataplane/p1/ns1/cr1"},
		{"too few segments", "/api/resource-tree/dataplane/p1/matches"},
		{"too many segments", "/api/resource-tree/dataplane/p1/ns1/cr1/extra/matches"},
		{"wrong final segment", "/api/resource-tree/dataplane/p1/ns1/cr1/children"},
		{"empty plane type", "/api/resource-tree//p1/ns1/cr1/matches"},
		{"empty plane id", "/api/resource-tree/dataplane//ns1/cr1/matches"},
		{"empty cr namespace", "/api/resource-tree/dataplane/p1//cr1/matches"},
		{"empty cr name", "/api/resource-tree/dataplane/p1/ns1//matches"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			captured := stubResourceTreeSend(t, nil, nil)
			s := newResourceTreeTestServer()
			rec := httptest.NewRecorder()

			s.handleResourceTreeMatches(rec, newMatchesRequest(tt.path, `{}`))

			assert.Equal(t, http.StatusBadRequest, rec.Code)
			assert.Contains(t, rec.Body.String(), "invalid resource-tree URL")
			assert.Zero(t, captured.calls, "a rejected path must never reach the tunnel")
		})
	}
}

func TestHandleResourceTreeMatches_ForwardsTunnelRequest(t *testing.T) {
	body := `{"version":"v1","queries":[{"id":"q1"}]}`
	captured := stubResourceTreeSend(t, &messaging.HTTPTunnelResponse{
		StatusCode: http.StatusOK,
		Headers:    map[string][]string{"Content-Type": {"application/json"}},
		Body:       []byte(`{"version":"v1","results":[]}`),
	}, nil)

	s := newResourceTreeTestServer()
	rec := httptest.NewRecorder()
	r := newMatchesRequest("/api/resource-tree/dataplane/prod-cluster/acme/prod-dp/matches", body)
	r.Header.Set("X-Request-ID", "rt-req-1")

	s.handleResourceTreeMatches(rec, r)

	require.Equal(t, 1, captured.calls)
	assert.Equal(t, "dataplane/prod-cluster", captured.planeIdentifier)
	assert.Equal(t, "acme/prod-dp", captured.crKey)
	require.NotNil(t, captured.req)
	assert.Equal(t, protocol.Target, captured.req.Target)
	assert.Equal(t, http.MethodPost, captured.req.Method)
	assert.Equal(t, protocol.PathMatches, captured.req.Path)
	assert.Equal(t, body, string(captured.req.Body))
	assert.Equal(t, "rt-req-1", captured.req.GatewayRequestID)
	assert.Equal(t, resourceTreeTunnelTimeout, captured.timeout)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))
	assert.Equal(t, `{"version":"v1","results":[]}`, rec.Body.String())
}

func TestHandleResourceTreeMatches_ClusterScopedNamespacePlaceholder(t *testing.T) {
	captured := stubResourceTreeSend(t, &messaging.HTTPTunnelResponse{
		StatusCode: http.StatusOK,
		Body:       []byte(`{}`),
	}, nil)

	s := newResourceTreeTestServer()
	rec := httptest.NewRecorder()

	s.handleResourceTreeMatches(rec,
		newMatchesRequest("/api/resource-tree/dataplane/shared/"+crNamespaceClusterPlaceholder+"/shared-dp/matches", `{}`))

	require.Equal(t, 1, captured.calls)
	assert.Equal(t, "/shared-dp", captured.crKey,
		"the _cluster placeholder must map to the empty namespace used by cluster-scoped CR keys")
}

// TestHandleResourceTreeMatches_TunnelErrorBecomesHTTPResponse pins the
// version-skew path: an agent with no resource-tree target answers with a
// tunnel Error and an EMPTY Body, so the gateway must synthesize the body from
// Error.Message or the 404 signal never reaches the client.
func TestHandleResourceTreeMatches_TunnelErrorBecomesHTTPResponse(t *testing.T) {
	stubResourceTreeSend(t, &messaging.HTTPTunnelResponse{
		StatusCode: http.StatusNotFound,
		Error: &messaging.ErrorDetails{
			Code:    http.StatusNotFound,
			Message: "unknown target: resource-tree",
		},
	}, nil)

	s := newResourceTreeTestServer()
	rec := httptest.NewRecorder()

	s.handleResourceTreeMatches(rec, newMatchesRequest("/api/resource-tree/dataplane/p1/ns1/cr1/matches", `{}`))

	assert.Equal(t, http.StatusNotFound, rec.Code)
	assert.Contains(t, rec.Header().Get("Content-Type"), "text/plain")
	assert.Contains(t, rec.Body.String(), "unknown target: resource-tree")
}

func TestHandleResourceTreeMatches_TunnelErrorWithUnusableStatusCode(t *testing.T) {
	stubResourceTreeSend(t, &messaging.HTTPTunnelResponse{
		StatusCode: 0,
		Error:      &messaging.ErrorDetails{Message: "agent exploded"},
	}, nil)

	s := newResourceTreeTestServer()
	rec := httptest.NewRecorder()

	s.handleResourceTreeMatches(rec, newMatchesRequest("/api/resource-tree/dataplane/p1/ns1/cr1/matches", `{}`))

	assert.Equal(t, http.StatusBadGateway, rec.Code,
		"a status code outside the HTTP range must never be written verbatim")
	assert.Contains(t, rec.Body.String(), "agent exploded")
}

func TestHandleResourceTreeMatches_UnauthorizedForCR(t *testing.T) {
	stubResourceTreeSend(t, nil, errors.New("no agents authorized for CR ns1/cr1"))

	s := newResourceTreeTestServer()
	rec := httptest.NewRecorder()

	s.handleResourceTreeMatches(rec, newMatchesRequest("/api/resource-tree/dataplane/p1/ns1/cr1/matches", `{}`))

	assert.Equal(t, http.StatusForbidden, rec.Code)
	assert.NotEqual(t, http.StatusNotFound, rec.Code,
		"routing failures must never masquerade as the version-skew 404")
}

func TestHandleResourceTreeMatches_TunnelSendFailure(t *testing.T) {
	stubResourceTreeSend(t, nil, errors.New("HTTP tunnel request timeout"))

	s := newResourceTreeTestServer()
	rec := httptest.NewRecorder()

	s.handleResourceTreeMatches(rec, newMatchesRequest("/api/resource-tree/dataplane/p1/ns1/cr1/matches", `{}`))

	assert.Equal(t, http.StatusBadGateway, rec.Code)
	assert.Contains(t, rec.Body.String(), "timeout")
}

func TestHandleResourceTreeMatches_BodyTooLarge(t *testing.T) {
	captured := stubResourceTreeSend(t, nil, nil)
	s := newResourceTreeTestServer()
	rec := httptest.NewRecorder()

	oversized := strings.Repeat("a", int(maxResourceTreeRequestBytes)+1)
	s.handleResourceTreeMatches(rec, newMatchesRequest("/api/resource-tree/dataplane/p1/ns1/cr1/matches", oversized))

	assert.Equal(t, http.StatusRequestEntityTooLarge, rec.Code)
	assert.Zero(t, captured.calls, "an oversized body must never reach the tunnel")
}

func TestHandleResourceTreeMatches_AcceptsBodyAtTheCap(t *testing.T) {
	captured := stubResourceTreeSend(t, &messaging.HTTPTunnelResponse{
		StatusCode: http.StatusOK,
		Body:       []byte(`{}`),
	}, nil)
	s := newResourceTreeTestServer()
	rec := httptest.NewRecorder()

	atCap := strings.Repeat("a", int(maxResourceTreeRequestBytes))
	s.handleResourceTreeMatches(rec, newMatchesRequest("/api/resource-tree/dataplane/p1/ns1/cr1/matches", atCap))

	require.Equal(t, 1, captured.calls, "a body exactly at the cap must be forwarded")
	assert.Len(t, captured.req.Body, int(maxResourceTreeRequestBytes))
	assert.Equal(t, http.StatusOK, rec.Code)
}
