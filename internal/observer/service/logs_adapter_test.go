// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openchoreo/openchoreo/pkg/observability"
)

func TestLogsAdapter_ContainerNameRequestSerialization(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		containerName string
		workflow      bool
	}{
		{name: "component container filter", containerName: "sidecar"},
		{name: "component without filter"},
		{name: "workflow container filter", containerName: "worker", workflow: true},
		{name: "workflow without filter", workflow: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var gotBody map[string]any
			var gotPath string
			var decodeErr error
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotPath = r.URL.Path
				decodeErr = json.NewDecoder(r.Body).Decode(&gotBody)
				w.Header().Set("Content-Type", "application/json")
				if _, err := w.Write([]byte(`{"logs":[],"total":0,"tookMs":1}`)); err != nil {
					return
				}
			}))
			defer server.Close()

			adapter, err := NewLogsAdapter(LogsAdapterConfig{BaseURL: server.URL, Timeout: time.Second})
			require.NoError(t, err)

			if tt.workflow {
				_, err = adapter.GetWorkflowLogs(context.Background(), observability.WorkflowLogsParams{
					Namespace: "ns", ContainerName: tt.containerName,
				})
			} else {
				_, err = adapter.GetComponentApplicationLogs(context.Background(), observability.ComponentApplicationLogsParams{
					Namespace: "ns", ContainerName: tt.containerName,
				})
			}
			require.NoError(t, err)

			assert.Equal(t, "/api/v1/logs/query", gotPath)
			require.NoError(t, decodeErr)
			if tt.containerName == "" {
				assert.NotContains(t, gotBody, "containerName")
			} else {
				assert.Equal(t, tt.containerName, gotBody["containerName"])
			}
		})
	}
}
