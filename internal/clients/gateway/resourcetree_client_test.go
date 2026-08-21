// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openchoreo/openchoreo/internal/resourcetree/protocol"
)

func newResourceTreeTestClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	server := httptest.NewTLSServer(handler)
	t.Cleanup(server.Close)
	httpClient := server.Client()
	httpClient.Timeout = 10 * time.Second
	return &Client{
		baseURL:            server.URL,
		httpClient:         httpClient,
		maxPodLogBytes:     DefaultMaxPodLogBytes,
		resourceTreeClient: &http.Client{Timeout: DefaultResourceTreeTimeout, Transport: httpClient.Transport},
	}
}

func twoQueryRequest() *protocol.MatchRequest {
	return &protocol.MatchRequest{
		Version: protocol.Version,
		Queries: []protocol.MatchQuery{
			{ID: "q1", Matcher: protocol.MatcherOwnerRef},
			{ID: "q2", Matcher: protocol.MatcherOwnerRef},
		},
	}
}

func writeMatchResponse(t *testing.T, w http.ResponseWriter, resp protocol.MatchResponse) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	require.NoError(t, json.NewEncoder(w).Encode(resp))
}

func TestMatchResourceTreeChildren_Success(t *testing.T) {
	var gotPath, gotMethod, gotContentType string
	var gotBody []byte

	c := newResourceTreeTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotMethod = r.Method
		gotContentType = r.Header.Get("Content-Type")
		gotBody, _ = io.ReadAll(r.Body)
		writeMatchResponse(t, w, protocol.MatchResponse{
			Version: protocol.Version,
			Results: []protocol.MatchResult{
				{ID: "q1", Matches: []protocol.MatchedObject{{ParentUIDs: []string{"u1"}, Object: json.RawMessage(`{"kind":"Pod"}`)}}},
				{ID: "q2", Truncated: true},
			},
		})
	})

	resp, err := c.MatchResourceTreeChildren(context.Background(), "dataplane", "prod", "acme", "prod-dp", twoQueryRequest())
	require.NoError(t, err)
	require.NotNil(t, resp)

	assert.Equal(t, "/api/resource-tree/dataplane/prod/acme/prod-dp/matches", gotPath)
	assert.Equal(t, http.MethodPost, gotMethod)
	assert.Equal(t, "application/json", gotContentType)

	var sent protocol.MatchRequest
	require.NoError(t, json.Unmarshal(gotBody, &sent))
	assert.Equal(t, protocol.Version, sent.Version)
	require.Len(t, sent.Queries, 2)

	assert.Equal(t, protocol.Version, resp.Version)
	require.Len(t, resp.Results, 2)
	assert.Len(t, resp.Results[0].Matches, 1)
	assert.True(t, resp.Results[1].Truncated)
}

func TestMatchResourceTreeChildren_ClusterScopedPlaceholderPassesThrough(t *testing.T) {
	var gotPath string
	c := newResourceTreeTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		writeMatchResponse(t, w, protocol.MatchResponse{Version: protocol.Version})
	})

	_, err := c.MatchResourceTreeChildren(context.Background(), "dataplane", "shared", "_cluster", "shared-dp",
		&protocol.MatchRequest{Version: protocol.Version})
	require.NoError(t, err)
	assert.Equal(t, "/api/resource-tree/dataplane/shared/_cluster/shared-dp/matches", gotPath)
}

// TestMatchResourceTreeChildren_404MapsToSentinel is the version-skew boundary:
// Task 5's handler never answers 404, so any 404 means an old agent (or an old
// gateway with no such route) and the caller may fall back to the legacy walk.
func TestMatchResourceTreeChildren_404MapsToSentinel(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{"unknown target body", "unknown target: resource-tree\n"},
		{"empty body", ""},
		{"go servemux default", "404 page not found\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newResourceTreeTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusNotFound)
				_, _ = io.WriteString(w, tt.body)
			})

			resp, err := c.MatchResourceTreeChildren(context.Background(), "dataplane", "p", "ns", "cr", twoQueryRequest())
			assert.Nil(t, resp)
			require.Error(t, err)
			assert.ErrorIs(t, err, ErrResourceTreeUnsupported)
		})
	}
}

func TestMatchResourceTreeChildren_404CarriesBodyForDiagnosis(t *testing.T) {
	c := newResourceTreeTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, "unknown target: resource-tree")
	})

	_, err := c.MatchResourceTreeChildren(context.Background(), "dataplane", "p", "ns", "cr", twoQueryRequest())
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrResourceTreeUnsupported)
	assert.Contains(t, err.Error(), "unknown target: resource-tree",
		"the diagnostic body text should survive into the error even though the mapping keys off the status code")
}

// TestMatchResourceTreeChildren_NonSentinelErrors asserts the OTHER direction of
// the boundary: everything that is not a 404 must stay a hard error, because
// the caller falls back to the slow legacy walk only on the sentinel.
func TestMatchResourceTreeChildren_NonSentinelErrors(t *testing.T) {
	tests := []struct {
		name    string
		handler http.HandlerFunc
	}{
		{
			name: "500 from gateway",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = io.WriteString(w, "proxy request failed")
			},
		},
		{
			name: "403 not authorized for CR",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusForbidden)
				_, _ = io.WriteString(w, "Forbidden: Agent not authorized for CR ns/cr")
			},
		},
		{
			name: "502 tunnel failure",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusBadGateway)
				_, _ = io.WriteString(w, "proxy request failed: HTTP tunnel request timeout")
			},
		},
		{
			name: "400 too many queries",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = io.WriteString(w, "too many queries")
			},
		},
		{
			name: "garbage body with 200",
			handler: func(w http.ResponseWriter, r *http.Request) {
				_, _ = io.WriteString(w, "this is not json")
			},
		},
		{
			name: "truncated json",
			handler: func(w http.ResponseWriter, r *http.Request) {
				_, _ = io.WriteString(w, `{"version":"v1","results":[`)
			},
		},
		{
			name: "empty body with 200",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			},
		},
		{
			name: "response version mismatch",
			handler: func(w http.ResponseWriter, r *http.Request) {
				writeMatchResponse(t, w, protocol.MatchResponse{
					Version: "v2",
					Results: []protocol.MatchResult{{ID: "q1"}, {ID: "q2"}},
				})
			},
		},
		{
			name: "missing result id",
			handler: func(w http.ResponseWriter, r *http.Request) {
				writeMatchResponse(t, w, protocol.MatchResponse{
					Version: protocol.Version,
					Results: []protocol.MatchResult{{ID: "q1"}},
				})
			},
		},
		{
			// Every query ID is answered, so only the duplicate check can catch
			// this — a shape like {q1, q1} would trip the missing-ID check
			// instead and would still pass with the duplicate check removed.
			name: "duplicate result id with otherwise complete coverage",
			handler: func(w http.ResponseWriter, r *http.Request) {
				writeMatchResponse(t, w, protocol.MatchResponse{
					Version: protocol.Version,
					Results: []protocol.MatchResult{{ID: "q1"}, {ID: "q2"}, {ID: "q2"}},
				})
			},
		},
		{
			name: "duplicate result id leaving a query unanswered",
			handler: func(w http.ResponseWriter, r *http.Request) {
				writeMatchResponse(t, w, protocol.MatchResponse{
					Version: protocol.Version,
					Results: []protocol.MatchResult{{ID: "q1"}, {ID: "q1"}},
				})
			},
		},
		{
			// Likewise: q1 and q2 are both answered, so only the unexpected-ID
			// check rejects the stray q3.
			name: "unexpected result id",
			handler: func(w http.ResponseWriter, r *http.Request) {
				writeMatchResponse(t, w, protocol.MatchResponse{
					Version: protocol.Version,
					Results: []protocol.MatchResult{{ID: "q1"}, {ID: "q2"}, {ID: "q3"}},
				})
			},
		},
		{
			// A non-nil error with an empty code must be rejected, not downgraded
			// to a generic discovery error that loses the classification.
			name: "per-result error with empty code",
			handler: func(w http.ResponseWriter, r *http.Request) {
				writeMatchResponse(t, w, protocol.MatchResponse{
					Version: protocol.Version,
					Results: []protocol.MatchResult{
						{ID: "q1"},
						{ID: "q2", Error: &protocol.MatchError{Code: "", Message: "boom"}},
					},
				})
			},
		},
		{
			name: "per-result error with unknown code",
			handler: func(w http.ResponseWriter, r *http.Request) {
				writeMatchResponse(t, w, protocol.MatchResponse{
					Version: protocol.Version,
					Results: []protocol.MatchResult{
						{ID: "q1"},
						{ID: "q2", Error: &protocol.MatchError{Code: "Teapot", Message: "boom"}},
					},
				})
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newResourceTreeTestClient(t, tt.handler)

			resp, err := c.MatchResourceTreeChildren(context.Background(), "dataplane", "p", "ns", "cr", twoQueryRequest())
			assert.Nil(t, resp)
			require.Error(t, err)
			assert.NotErrorIs(t, err, ErrResourceTreeUnsupported,
				"only a 404 may downgrade the caller to the legacy walk")
		})
	}
}

// TestMatchResourceTreeChildren_NonOKStatusWithVerifiableBody closes a hole in
// the table above: every non-200 fixture there carries a body that is not a
// match response, so the DECODE guard rejects them and the status check could be
// deleted without a single failure. This 500 carries a complete, valid
// MatchResponse — right version, both query IDs answered exactly once — so
// nothing downstream of the status check has grounds to reject it.
func TestMatchResourceTreeChildren_NonOKStatusWithVerifiableBody(t *testing.T) {
	c := newResourceTreeTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		require.NoError(t, json.NewEncoder(w).Encode(protocol.MatchResponse{
			Version: protocol.Version,
			Results: []protocol.MatchResult{{ID: "q1"}, {ID: "q2"}},
		}))
	})

	resp, err := c.MatchResourceTreeChildren(context.Background(), "dataplane", "p", "ns", "cr", twoQueryRequest())
	assert.Nil(t, resp)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "resource-tree match request failed",
		"the status check, not the decode guard, must be what rejects this")
	assert.NotErrorIs(t, err, ErrResourceTreeUnsupported)
}

// TestMatchResourceTreeChildren_DecodeErrorWithVerifiableShape closes the
// matching hole for the decode guard: the garbage-body fixtures above leave
// matchResp zero-valued, so verifyMatchResponse rejects the empty version and
// the decode error is never the reason the call fails. Here only `truncated` has
// the wrong type — encoding/json records that error but keeps decoding, so the
// version and both query IDs land correctly and the response would verify if the
// decode error were ignored.
func TestMatchResourceTreeChildren_DecodeErrorWithVerifiableShape(t *testing.T) {
	body := fmt.Sprintf(`{"version":%q,"results":[{"id":"q1","truncated":"not-a-bool"},{"id":"q2"}]}`, protocol.Version)

	c := newResourceTreeTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, body)
	})

	resp, err := c.MatchResourceTreeChildren(context.Background(), "dataplane", "p", "ns", "cr", twoQueryRequest())
	assert.Nil(t, resp)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to decode resource-tree match response",
		"the decode guard, not the coverage check, must be what rejects this")
	assert.NotErrorIs(t, err, ErrResourceTreeUnsupported)
}

// stallUntilDisconnect blocks the handler until the caller goes away. The body
// is drained first so the server's background reader can observe the close and
// cancel the request context; the timer only bounds how long httptest's Close
// waits if it does not.
func stallUntilDisconnect(w http.ResponseWriter, r *http.Request) {
	_, _ = io.Copy(io.Discard, r.Body)
	select {
	case <-r.Context().Done():
	case <-time.After(2 * time.Second):
	}
}

func TestMatchResourceTreeChildren_TimeoutIsNotTheSentinel(t *testing.T) {
	c := newResourceTreeTestClient(t, stallUntilDisconnect)
	// The dedicated timeout is a field precisely so a test can shrink it.
	c.resourceTreeClient.Timeout = 100 * time.Millisecond

	resp, err := c.MatchResourceTreeChildren(context.Background(), "dataplane", "p", "ns", "cr", twoQueryRequest())
	assert.Nil(t, resp)
	require.Error(t, err)
	assert.NotErrorIs(t, err, ErrResourceTreeUnsupported)
	assert.True(t, IsTransientError(err), "a timeout should be reported as transient, not as version skew")
}

func TestMatchResourceTreeChildren_ContextCancellation(t *testing.T) {
	c := newResourceTreeTestClient(t, stallUntilDisconnect)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	resp, err := c.MatchResourceTreeChildren(ctx, "dataplane", "p", "ns", "cr", twoQueryRequest())
	assert.Nil(t, resp)
	require.Error(t, err)
	assert.NotErrorIs(t, err, ErrResourceTreeUnsupported)
}

func TestMatchResourceTreeChildren_OversizedResponseRejected(t *testing.T) {
	c := newResourceTreeTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"version":"v1","results":[{"id":"q1","matches":[{"parentUIDs":["u"],"object":"`)
		_, _ = io.WriteString(w, strings.Repeat("a", int(maxResourceTreeResponseBytes)+1))
		_, _ = io.WriteString(w, `"}]}]}`)
	})

	resp, err := c.MatchResourceTreeChildren(context.Background(), "dataplane", "p", "ns", "cr", twoQueryRequest())
	assert.Nil(t, resp)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "too large")
	assert.NotErrorIs(t, err, ErrResourceTreeUnsupported)
}

func TestMatchResourceTreeChildren_RejectsBadRequestBeforeSending(t *testing.T) {
	tests := []struct {
		name    string
		req     *protocol.MatchRequest
		wantMsg string
	}{
		{"nil request", nil, "request is required"},
		{
			name: "duplicate query ids",
			req: &protocol.MatchRequest{
				Version: protocol.Version,
				Queries: []protocol.MatchQuery{{ID: "q1"}, {ID: "q1"}},
			},
			wantMsg: "duplicate query id",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var called int
			c := newResourceTreeTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				called++
				writeMatchResponse(t, w, protocol.MatchResponse{Version: protocol.Version})
			})

			resp, err := c.MatchResourceTreeChildren(context.Background(), "dataplane", "p", "ns", "cr", tt.req)
			assert.Nil(t, resp)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantMsg)
			assert.NotErrorIs(t, err, ErrResourceTreeUnsupported)
			assert.Zero(t, called, "a request that can never verify must not be sent")
		})
	}
}

func TestMatchResourceTreeChildren_EmptyQuerySetVerifies(t *testing.T) {
	c := newResourceTreeTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		writeMatchResponse(t, w, protocol.MatchResponse{Version: protocol.Version})
	})

	resp, err := c.MatchResourceTreeChildren(context.Background(), "dataplane", "p", "ns", "cr",
		&protocol.MatchRequest{Version: protocol.Version})
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Empty(t, resp.Results)
}

// TestMatchResourceTreeChildren_PerResultErrorsSurvive proves the client does
// not confuse a per-query MatchError with a transport failure: an agent that
// answers 200 with error-bearing results is a successful call.
func TestMatchResourceTreeChildren_PerResultErrorsSurvive(t *testing.T) {
	c := newResourceTreeTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		writeMatchResponse(t, w, protocol.MatchResponse{
			Version: protocol.Version,
			Results: []protocol.MatchResult{
				{ID: "q1", Error: &protocol.MatchError{Code: protocol.CodeForbidden, Message: "pods is forbidden"}},
				{ID: "q2"},
			},
		})
	})

	resp, err := c.MatchResourceTreeChildren(context.Background(), "dataplane", "p", "ns", "cr", twoQueryRequest())
	require.NoError(t, err)
	require.Len(t, resp.Results, 2)
	require.NotNil(t, resp.Results[0].Error)
	assert.Equal(t, protocol.CodeForbidden, resp.Results[0].Error.Code)
}

// TestNewClientWithConfig_ResourceTreeClientIsDedicated pins ambiguity
// resolution 1: the resource-tree calls get 30s while every other endpoint
// keeps the 10s default, and both share one transport (and therefore one TLS
// config and connection pool).
func TestNewClientWithConfig_ResourceTreeClientIsDedicated(t *testing.T) {
	c, err := NewClientWithConfig(&Config{BaseURL: "https://gateway.local"})
	require.NoError(t, err)

	assert.Equal(t, 10*time.Second, c.httpClient.Timeout,
		"the shared client's default must stay at 10s for existing endpoints")
	assert.Equal(t, DefaultResourceTreeTimeout, c.resourceTreeClient.Timeout)
	assert.Equal(t, 30*time.Second, DefaultResourceTreeTimeout)
	assert.NotSame(t, c.httpClient, c.resourceTreeClient, "the resource-tree client must be a separate http.Client")
	assert.Same(t, c.httpClient.Transport, c.resourceTreeClient.Transport,
		"both clients must share one transport so TLS config and connection pooling are not duplicated")
}

func TestNewClientWithConfig_ExplicitTimeoutDoesNotChangeResourceTreeTimeout(t *testing.T) {
	c, err := NewClientWithConfig(&Config{BaseURL: "https://gateway.local", Timeout: 2 * time.Second})
	require.NoError(t, err)

	assert.Equal(t, 2*time.Second, c.httpClient.Timeout)
	assert.Equal(t, DefaultResourceTreeTimeout, c.resourceTreeClient.Timeout,
		"a caller-supplied timeout tunes the shared client only")
}

func TestMatchResourceTreeChildren_MissingIDsNamedInError(t *testing.T) {
	c := newResourceTreeTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		writeMatchResponse(t, w, protocol.MatchResponse{
			Version: protocol.Version,
			Results: []protocol.MatchResult{{ID: "q2"}},
		})
	})

	_, err := c.MatchResourceTreeChildren(context.Background(), "dataplane", "p", "ns", "cr", twoQueryRequest())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "q1", "the error should name which query went unanswered")
}

func TestMatchResourceTreeChildren_NetworkFailureIsTransient(t *testing.T) {
	c := &Client{
		baseURL:            "https://127.0.0.1:1",
		httpClient:         http.DefaultClient,
		resourceTreeClient: &http.Client{Timeout: time.Second},
	}

	resp, err := c.MatchResourceTreeChildren(context.Background(), "dataplane", "p", "ns", "cr", twoQueryRequest())
	assert.Nil(t, resp)
	require.Error(t, err)
	assert.True(t, IsTransientError(err))
	assert.NotErrorIs(t, err, ErrResourceTreeUnsupported)
	assert.Contains(t, err.Error(), "resource-tree", "the error should name the operation that failed")
}
