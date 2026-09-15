// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	coreconfig "github.com/openchoreo/openchoreo/internal/config"
	observeraudit "github.com/openchoreo/openchoreo/internal/observer/audit"
	observerAuthz "github.com/openchoreo/openchoreo/internal/observer/authz"
	"github.com/openchoreo/openchoreo/internal/observer/service/mocks"
	"github.com/openchoreo/openchoreo/internal/observer/types"
	"github.com/openchoreo/openchoreo/internal/server/middleware/audit"
	"github.com/openchoreo/openchoreo/internal/server/middleware/auth"
	"github.com/openchoreo/openchoreo/pkg/mcp/mcpaudit"
)

const (
	auditStart = "2026-08-14T16:30:00Z"
	auditEnd   = "2026-08-14T17:30:00Z"
)

func TestQueryAuditLogsAppliesDefaults(t *testing.T) {
	var captured *types.AuditLogsQueryRequest
	svc := mocks.NewMockAuditLogsQuerier(t)
	svc.EXPECT().
		QueryAuditLogs(mock.Anything, mock.Anything).
		Run(func(_ context.Context, req *types.AuditLogsQueryRequest) { captured = req }).
		Return(&types.AuditLogsResponse{}, nil).Once()

	h := newTestMCPHandler(t, withAuditLogsService(svc))
	_, err := h.QueryAuditLogs(context.Background(), AuditLogsQueryArgs{
		StartTime: auditStart,
		EndTime:   auditEnd,
	})
	require.NoError(t, err)

	require.NotNil(t, captured)
	// The REST validator's defaults, reached through the tool.
	assert.Equal(t, 100, captured.Limit)
	assert.Equal(t, "desc", captured.SortOrder)
}

// TestQueryAuditLogsRejectsInvalidQueries covers the cases the tool must not
// forward: each would otherwise reach the adapter in a shape the REST endpoint
// answers with a 400.
func TestQueryAuditLogsRejectsInvalidQueries(t *testing.T) {
	tests := []struct {
		name string
		args AuditLogsQueryArgs
		want string
	}{
		{
			name: "unknown category",
			args: AuditLogsQueryArgs{StartTime: auditStart, EndTime: auditEnd, Categories: []string{"mangement"}},
			want: "invalid category",
		},
		{
			name: "unknown result",
			args: AuditLogsQueryArgs{StartTime: auditStart, EndTime: auditEnd, Results: []string{"ok"}},
			want: "invalid result",
		},
		{
			name: "unknown surface",
			args: AuditLogsQueryArgs{StartTime: auditStart, EndTime: auditEnd, Surfaces: []string{"api"}},
			want: "invalid surface",
		},
		{
			name: "end before start",
			args: AuditLogsQueryArgs{StartTime: auditEnd, EndTime: auditStart},
			want: "startTime",
		},
		{
			name: "window wider than 366 days",
			args: AuditLogsQueryArgs{StartTime: "2024-01-01T00:00:00Z", EndTime: "2026-01-01T00:00:00Z"},
			want: "time range",
		},
		{
			name: "too many filter values",
			args: AuditLogsQueryArgs{
				StartTime: auditStart, EndTime: auditEnd,
				Categories: []string{"management", "authorization", "access", "management"},
			},
			want: "cannot have more than",
		},
		{
			name: "duplicate filter value",
			args: AuditLogsQueryArgs{
				StartTime: auditStart, EndTime: auditEnd,
				Producers: []string{"openchoreo-api", "openchoreo-api"},
			},
			want: "duplicate",
		},
		{
			name: "malformed timeline interval",
			args: AuditLogsQueryArgs{
				StartTime: auditStart, EndTime: auditEnd,
				IncludeTimeline: true, TimelineInterval: "15 minutes",
			},
			want: "timelineInterval",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// No expectations set, so mockery fails on cleanup if the service
			// is reached.
			h := newTestMCPHandler(t, withAuditLogsService(mocks.NewMockAuditLogsQuerier(t)))

			_, err := h.QueryAuditLogs(context.Background(), tt.args)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.want)
		})
	}
}

func TestQueryAuditLogsPropagatesServiceError(t *testing.T) {
	wantErr := errors.New("adapter unavailable")
	svc := mocks.NewMockAuditLogsQuerier(t)
	svc.EXPECT().QueryAuditLogs(mock.Anything, mock.Anything).Return(nil, wantErr).Once()

	h := newTestMCPHandler(t, withAuditLogsService(svc))
	_, err := h.QueryAuditLogs(context.Background(), AuditLogsQueryArgs{
		StartTime: auditStart,
		EndTime:   auditEnd,
	})
	require.ErrorIs(t, err, wantErr)
}

// newTestAuditOptions builds the audit options NewHTTPServer takes, from the
// production binding table, writing records to sink.
func newTestAuditOptions(t *testing.T, sink io.Writer) mcpaudit.MiddlewareOptions {
	t.Helper()

	policies, errs := audit.NewPolicySet(coreconfig.NewPath("audit"), audit.Settings{Publish: true}, nil)
	require.Empty(t, errs)
	emitter, err := audit.NewEmitter("observer", policies, audit.NewLogger(sink))
	require.NoError(t, err)

	bindings, err := observeraudit.MCPBindings()
	require.NoError(t, err)

	return mcpaudit.MiddlewareOptions{Emitter: emitter, Bindings: bindings, Enabled: true}
}

// auditRecordsFrom returns the AUDIT-LOG lines the emitter wrote to buf.
func auditRecordsFrom(t *testing.T, buf *bytes.Buffer) []map[string]any {
	t.Helper()

	var records []map[string]any
	for line := range strings.SplitSeq(buf.String(), "\n") {
		if !strings.Contains(line, `"msg":"AUDIT-LOG"`) {
			continue
		}
		var record map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &record))
		records = append(records, record)
	}
	return records
}

// TestMCPAuditWiring drives real tools/call requests through a real MCP client
// against the production constructor. What needs guarding is that
// NewHTTPServer installs the middleware at all: a server assembled without it
// serves query_audit_logs perfectly well and leaves no trace of the read.
func TestMCPAuditWiring(t *testing.T) {
	setup := func(t *testing.T, buf *bytes.Buffer) (*mcpsdk.ClientSession, *testServices) {
		t.Helper()

		svcs := newTestServices()
		mcpHandler, err := buildMCPHandler(svcs)
		require.NoError(t, err)

		httpHandler, err := NewHTTPServer(mcpHandler, newTestAuditOptions(t, buf))
		require.NoError(t, err)

		server := httptest.NewServer(withTestSubject(httpHandler))
		t.Cleanup(server.Close)

		client := mcpsdk.NewClient(&mcpsdk.Implementation{Name: "audit-wiring-test"}, nil)
		session, err := client.Connect(t.Context(),
			&mcpsdk.StreamableClientTransport{Endpoint: server.URL, HTTPClient: server.Client()}, nil)
		require.NoError(t, err)
		t.Cleanup(func() { _ = session.Close() })

		return session, svcs
	}

	t.Run("query_audit_logs emits one record", func(t *testing.T) {
		var buf bytes.Buffer
		session, _ := setup(t, &buf)

		result, err := session.CallTool(t.Context(), &mcpsdk.CallToolParams{
			Name:      "query_audit_logs",
			Arguments: map[string]any{"start_time": auditStart, "end_time": auditEnd},
		})
		require.NoError(t, err)
		require.False(t, result.IsError)

		records := auditRecordsFrom(t, &buf)
		require.Len(t, records, 1)
		assert.Equal(t, "read_audit_log", records[0]["action"])
		assert.Equal(t, "access", records[0]["category"])
		assert.Equal(t, "success", records[0]["result"])
		assert.Equal(t, "mcp", records[0]["surface"])
		assert.Equal(t, "QueryAuditLogs", records[0]["operation_id"])
	})

	t.Run("a failed read is still recorded", func(t *testing.T) {
		var buf bytes.Buffer
		session, svcs := setup(t, &buf)
		svcs.auditLogs.err = errors.New("adapter unavailable")

		result, err := session.CallTool(t.Context(), &mcpsdk.CallToolParams{
			Name:      "query_audit_logs",
			Arguments: map[string]any{"start_time": auditStart, "end_time": auditEnd},
		})
		require.NoError(t, err)
		require.True(t, result.IsError)

		records := auditRecordsFrom(t, &buf)
		require.Len(t, records, 1)
		assert.Equal(t, "failure", records[0]["result"])
	})

	// A refused read must stay distinguishable from an adapter that was down.
	// See recordAuthzResult for why that takes work on this surface.
	for _, tc := range []struct {
		name string
		err  error
		want string
	}{
		{"denied", observerAuthz.ErrAuthzForbidden, "denied"},
		{"unauthenticated", observerAuthz.ErrAuthzUnauthorized, "unauthenticated"},
	} {
		t.Run("a refused read is recorded as "+tc.want, func(t *testing.T) {
			var buf bytes.Buffer
			session, svcs := setup(t, &buf)
			svcs.auditLogs.err = tc.err

			result, err := session.CallTool(t.Context(), &mcpsdk.CallToolParams{
				Name:      "query_audit_logs",
				Arguments: map[string]any{"start_time": auditStart, "end_time": auditEnd},
			})
			require.NoError(t, err)
			require.True(t, result.IsError)

			records := auditRecordsFrom(t, &buf)
			require.Len(t, records, 1)
			assert.Equal(t, tc.want, records[0]["result"])
		})
	}

	// The cap is declared in the schema, so the SDK refuses the call before the
	// handler runs. The attempt must still be recorded.
	t.Run("an over-large limit is rejected and still audited", func(t *testing.T) {
		var buf bytes.Buffer
		session, _ := setup(t, &buf)

		_, err := session.CallTool(t.Context(), &mcpsdk.CallToolParams{
			Name:      "query_audit_logs",
			Arguments: map[string]any{"start_time": auditStart, "end_time": auditEnd, "limit": 5000},
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "limit", "the error should name the offending argument")

		records := auditRecordsFrom(t, &buf)
		require.Len(t, records, 1)
		assert.Equal(t, "failure", records[0]["result"])
	})

	t.Run("an unbound tool emits nothing", func(t *testing.T) {
		var buf bytes.Buffer
		session, _ := setup(t, &buf)

		_, err := session.CallTool(t.Context(), &mcpsdk.CallToolParams{
			Name: "query_platform_logs",
			Arguments: map[string]any{
				"start_time": auditStart,
				"end_time":   auditEnd,
			},
		})
		require.NoError(t, err)

		assert.Empty(t, auditRecordsFrom(t, &buf),
			"reading platform logs is unaudited on both surfaces — see RESTExemptions")
	})
}

func withTestSubject(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := auth.SetSubjectContext(r.Context(), &auth.SubjectContext{ID: "test-user", Type: "user"})
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
