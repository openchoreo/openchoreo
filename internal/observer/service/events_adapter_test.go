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

// sampleEventsResponse mirrors the logs-adapter events response from the design discussion.
const sampleEventsResponse = `{
  "events": [
    {
      "timestamp": "2026-06-05T03:07:12Z",
      "message": "Saw completed job: github-issue-reporter-development-5e31cab9-29674469, condition: Complete",
      "type": "Normal",
      "reason": "SawCompletedJob",
      "metadata": {
        "componentName": "github-issue-reporter",
        "projectName": "default",
        "environmentName": "development",
        "namespaceName": "default",
        "componentUid": "a022c8af-78c8-4fa2-a9fd-51eb8579ecb2",
        "projectUid": "fc480b7a-d4bb-4638-b39d-66b317f24fe7",
        "environmentUid": "cb6b3d47-f636-4e2d-aaa3-1b2b70283401",
        "objectKind": "CronJob",
        "objectName": "github-issue-reporter-development-5e31cab9",
        "objectNamespace": "dp-default-default-development-f8e58905"
      }
    }
  ],
  "total": 24,
  "tookMs": 57
}`

func TestLogsAdapterGetComponentEvents(t *testing.T) {
	var gotPath, gotMethod string
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotMethod = r.Method
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(sampleEventsResponse))
	}))
	defer srv.Close()

	adapter, err := NewLogsAdapter(LogsAdapterConfig{BaseURL: srv.URL})
	require.NoError(t, err)

	res, err := adapter.GetComponentEvents(context.Background(), observability.ComponentEventsParams{
		Namespace:     "default",
		ProjectID:     "fc480b7a-d4bb-4638-b39d-66b317f24fe7",
		ComponentID:   "a022c8af-78c8-4fa2-a9fd-51eb8579ecb2",
		EnvironmentID: "cb6b3d47-f636-4e2d-aaa3-1b2b70283401",
		StartTime:     time.Date(2026, 6, 5, 2, 58, 31, 0, time.UTC),
		EndTime:       time.Date(2026, 6, 5, 3, 8, 31, 0, time.UTC),
		Limit:         50,
		SortOrder:     "asc",
	})
	require.NoError(t, err)

	assert.Equal(t, http.MethodPost, gotMethod)
	assert.Equal(t, "/api/v1/events/query", gotPath)

	// Request: component scope forwarded with UIDs.
	scope, ok := gotBody["searchScope"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "default", scope["namespace"])
	assert.Equal(t, "fc480b7a-d4bb-4638-b39d-66b317f24fe7", scope["projectUid"])
	assert.Equal(t, "a022c8af-78c8-4fa2-a9fd-51eb8579ecb2", scope["componentUid"])
	assert.Equal(t, "cb6b3d47-f636-4e2d-aaa3-1b2b70283401", scope["environmentUid"])
	assert.EqualValues(t, 50, gotBody["limit"])
	assert.Equal(t, "asc", gotBody["sortOrder"])

	// Response: mapped into observability.EventEntry.
	require.Len(t, res.Events, 1)
	e := res.Events[0]
	assert.Equal(t, "SawCompletedJob", e.Reason)
	assert.Contains(t, e.Message, "Saw completed job")
	assert.Equal(t, "Normal", e.Type)
	assert.Equal(t, "CronJob", e.ObjectKind)
	assert.Equal(t, "github-issue-reporter-development-5e31cab9", e.ObjectName)
	assert.Equal(t, "dp-default-default-development-f8e58905", e.ObjectNamespace)
	assert.Equal(t, "github-issue-reporter", e.ComponentName)
	assert.Equal(t, "a022c8af-78c8-4fa2-a9fd-51eb8579ecb2", e.ComponentID)
	assert.Equal(t, 24, res.TotalCount)
	assert.Equal(t, 57, res.Took)
}

func TestLogsAdapterGetWorkflowEvents(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"events":[],"total":0,"tookMs":3}`))
	}))
	defer srv.Close()

	adapter, err := NewLogsAdapter(LogsAdapterConfig{BaseURL: srv.URL})
	require.NoError(t, err)

	res, err := adapter.GetWorkflowEvents(context.Background(), observability.WorkflowEventsParams{
		Namespace:       "default",
		WorkflowRunName: "run-1",
		TaskName:        "migrate",
		StartTime:       time.Date(2026, 6, 5, 2, 0, 0, 0, time.UTC),
		EndTime:         time.Date(2026, 6, 5, 3, 0, 0, 0, time.UTC),
	})
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Empty(t, res.Events)

	scope, ok := gotBody["searchScope"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "default", scope["namespace"])
	assert.Equal(t, "run-1", scope["workflowRunName"])
	assert.Equal(t, "migrate", scope["taskName"])
}

func TestLogsAdapterGetComponentEventsHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"message":"boom"}`))
	}))
	defer srv.Close()

	adapter, err := NewLogsAdapter(LogsAdapterConfig{BaseURL: srv.URL})
	require.NoError(t, err)

	_, err = adapter.GetComponentEvents(context.Background(), observability.ComponentEventsParams{
		Namespace: "default",
		StartTime: time.Date(2026, 6, 5, 2, 0, 0, 0, time.UTC),
		EndTime:   time.Date(2026, 6, 5, 3, 0, 0, 0, time.UTC),
	})
	require.Error(t, err)
}
