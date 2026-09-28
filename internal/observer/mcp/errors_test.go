// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package mcp

import (
	"context"
	"errors"
	"fmt"
	"testing"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	observerAuthz "github.com/openchoreo/openchoreo/internal/observer/authz"
	"github.com/openchoreo/openchoreo/internal/observer/service"
)

func TestToolError(t *testing.T) {
	h := newTestMCPHandler(t)

	tests := []struct {
		name    string
		err     error
		want    string
		notWant []string
	}{
		{
			name: "a caller mistake keeps its message",
			err:  fmt.Errorf("invalid start_time: %w", invalidArgument(errors.New("bad format"))),
			want: "invalid start_time: bad format",
		},
		{
			name: "a service's invalid-request message is caller-safe",
			err:  fmt.Errorf("%w: searchScope.project is required", service.ErrRuntimeTopologyInvalidRequest),
			want: "invalid runtime topology request: searchScope.project is required",
		},
		{
			name: "forbidden",
			err:  fmt.Errorf("check failed: %w", observerAuthz.ErrAuthzForbidden),
			want: "access denied",
		},
		{
			name: "unauthorized",
			err:  observerAuthz.ErrAuthzUnauthorized,
			want: "unauthorized",
		},
		{
			name:    "authorization backend down",
			err:     fmt.Errorf("%w: dial tcp 10.0.0.7:8443: connection refused", observerAuthz.ErrAuthzServiceUnavailable),
			want:    "authorization service temporarily unavailable",
			notWant: []string{"10.0.0.7"},
		},
		{
			// The shape reported in #4805: delivery insights borrows the alerts
			// sentinel, so the raw text names the wrong subsystem.
			name: "an unknown scope resource does not name another subsystem",
			err: fmt.Errorf("%w: %s %q not found: %w",
				service.ErrAlertsResolveSearchScope, "project", "nope", service.ErrScopeNotFound),
			want:    "one or more resources in the search scope were not found",
			notWant: []string{"alerts"},
		},
		{
			name:    "a resolver not-found is reported the same way",
			err:     fmt.Errorf("%w: %w: component/ns/proj/comp", service.ErrLogsResolveSearchScope, service.ErrResourceNotFound),
			want:    "one or more resources in the search scope were not found",
			notWant: []string{"component/ns/proj/comp"},
		},
		{
			name: "a scope resolution failure hides its cause",
			err: fmt.Errorf("%w: failed to resolve %s %q: %w",
				service.ErrAlertsResolveSearchScope, "project", "p", service.ErrScopeResolutionFailed),
			want:    "failed to resolve search scope",
			notWant: []string{"alerts"},
		},
		{
			name: "span not found",
			err:  fmt.Errorf("%w: span abc", service.ErrSpanNotFound),
			want: "span not found",
		},
		{
			name:    "an unsupported capability returns only the sentinel",
			err:     fmt.Errorf("adapter at http://logs.internal:9098: %w", service.ErrAuditLogsNotSupported),
			want:    service.ErrAuditLogsNotSupported.Error(),
			notWant: []string{"logs.internal"},
		},
		{
			name: "an adapter failure does not leak its response body",
			err: fmt.Errorf("%w: metrics adapter returned HTTP %d: %s",
				service.ErrMetricsRetrieval, 500, `{"trace":"prometheus.monitoring.svc:9090"}`),
			want:    "query_http_metrics failed due to an internal error",
			notWant: []string{"prometheus", "HTTP 500"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := h.toolError("query_http_metrics", tt.err)
			require.Error(t, got)
			assert.Equal(t, tt.want, got.Error())
			for _, s := range tt.notWant {
				assert.NotContains(t, got.Error(), s)
			}
		})
	}

	t.Run("nil stays nil", func(t *testing.T) {
		assert.NoError(t, h.toolError("query_http_metrics", nil))
	})
}

// TestToolErrorsOverMCP checks the mapping is what a client actually receives,
// since the SDK is what turns a handler's error into the tool result.
func TestToolErrorsOverMCP(t *testing.T) {
	ctx := context.Background()

	call := func(t *testing.T, svcs *testServices, tool string, args map[string]any) string {
		t.Helper()
		handler, err := buildMCPHandler(svcs)
		require.NoError(t, err)

		server := mcpsdk.NewServer(&mcpsdk.Implementation{Name: "test", Version: "1.0.0"}, nil)
		registerTools(server, handler)
		clientTransport, serverTransport := mcpsdk.NewInMemoryTransports()
		_, err = server.Connect(ctx, serverTransport, nil)
		require.NoError(t, err)
		session, err := mcpsdk.NewClient(&mcpsdk.Implementation{Name: "client", Version: "1.0.0"}, nil).
			Connect(ctx, clientTransport, nil)
		require.NoError(t, err)
		t.Cleanup(func() { _ = session.Close() })

		result, err := session.CallTool(ctx, &mcpsdk.CallToolParams{Name: tool, Arguments: args})
		require.NoError(t, err)
		require.True(t, result.IsError)
		require.Len(t, result.Content, 1)
		text, ok := result.Content[0].(*mcpsdk.TextContent)
		require.True(t, ok)
		return text.Text
	}

	t.Run("an unknown project on query_dora_metrics", func(t *testing.T) {
		svcs := newTestServices()
		svcs.insights.queryDoraMetricsErr = fmt.Errorf("%w: %s %q not found: %w",
			service.ErrAlertsResolveSearchScope, "project", "nope", service.ErrScopeNotFound)

		got := call(t, svcs, "query_dora_metrics", map[string]any{
			"namespace":  testNamespace,
			"project":    "nope",
			"start_time": testStartTime,
			"end_time":   testEndTime,
		})
		assert.Equal(t, "one or more resources in the search scope were not found", got)
	})

	t.Run("a backend failure on query_component_logs", func(t *testing.T) {
		svcs := newTestServices()
		svcs.logs.err = fmt.Errorf("%w: opensearch: dial tcp 10.0.0.9:9200: i/o timeout", service.ErrLogsRetrieval)

		got := call(t, svcs, "query_component_logs", map[string]any{
			"namespace":  testNamespace,
			"start_time": testStartTime,
			"end_time":   testEndTime,
		})
		assert.Equal(t, "query_component_logs failed due to an internal error", got)
	})

	t.Run("a malformed time still tells the caller what to fix", func(t *testing.T) {
		got := call(t, newTestServices(), "query_component_logs", map[string]any{
			"namespace":  testNamespace,
			"start_time": "yesterday",
			"end_time":   testEndTime,
		})
		assert.Contains(t, got, "invalid start_time")
	})

	t.Run("a scope argument error still tells the caller what to fix", func(t *testing.T) {
		got := call(t, newTestServices(), "query_resource_metrics", map[string]any{
			"namespace":  testNamespace,
			"component":  testComponent,
			"start_time": testStartTime,
			"end_time":   testEndTime,
		})
		assert.Equal(t, "project is required when component is provided", got)
	})
}
