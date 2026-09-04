// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	observerAuthz "github.com/openchoreo/openchoreo/internal/observer/authz"
	"github.com/openchoreo/openchoreo/internal/observer/service"
	servicemocks "github.com/openchoreo/openchoreo/internal/observer/service/mocks"
	"github.com/openchoreo/openchoreo/internal/observer/types"
)

const clusterLogsWindow = "startTime=2026-08-14T16:30:00Z&endTime=2026-08-14T17:30:00Z"

func clusterLogsHandler(t *testing.T, svc service.ClusterLogsQuerier) *Handler {
	t.Helper()
	return &Handler{
		baseHandler:        baseHandler{logger: noopLogger()},
		clusterLogsService: svc,
	}
}

func getClusterLogs(t *testing.T, h *Handler, query string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/v1alpha1/cluster-logs?"+query, nil)
	return serve(t, h, req)
}

// --- label selector parsing ---

func TestParseLabelSelector(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		selector string
		want     map[string]string
		wantErr  string
	}{
		{name: "empty", selector: "", want: nil},
		{
			name:     "single pair",
			selector: "openchoreo.dev/plane=controlplane",
			want:     map[string]string{"openchoreo.dev/plane": "controlplane"},
		},
		{
			name:     "comma means AND",
			selector: "openchoreo.dev/plane=dataplane,openchoreo.dev/plane-id=prod",
			want: map[string]string{
				"openchoreo.dev/plane":    "dataplane",
				"openchoreo.dev/plane-id": "prod",
			},
		},
		{
			name:     "surrounding whitespace is trimmed",
			selector: " app.kubernetes.io/name = openbao , tier=infra ",
			want:     map[string]string{"app.kubernetes.io/name": "openbao", "tier": "infra"},
		},
		{
			name:     "empty value is a legitimate selector",
			selector: "openchoreo.dev/plane=",
			want:     map[string]string{"openchoreo.dev/plane": ""},
		},
		{
			name:     "repeating the same pair is not a conflict",
			selector: "tier=infra,tier=infra",
			want:     map[string]string{"tier": "infra"},
		},
		{name: "missing equals", selector: "openchoreo.dev/plane", wantErr: "expected key=value"},
		{name: "empty key", selector: "=controlplane", wantErr: "key must not be empty"},
		{name: "inequality is rejected", selector: "tier!=infra", wantErr: "only equality selectors"},
		{name: "double equals is rejected", selector: "tier==infra", wantErr: "only equality selectors"},
		{
			name:     "conflicting values for one key",
			selector: "tier=infra,tier=apps",
			wantErr:  "conflicting values",
		},
		{
			name:     "over-long selector",
			selector: "k=" + strings.Repeat("v", 300),
			wantErr:  "cannot exceed 256 characters",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := ParseLabelSelector(tt.selector)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

// --- request validation ---

func TestValidateClusterLogsQueryRequest_AppliesDefaults(t *testing.T) {
	t.Parallel()

	req := &types.ClusterLogsQueryRequest{
		StartTime: "2026-08-14T16:30:00Z",
		EndTime:   "2026-08-14T17:30:00Z",
	}
	require.NoError(t, ValidateClusterLogsQueryRequest(req))
	assert.Equal(t, defaultLimit, req.Limit)
	assert.Equal(t, defaultSortOrder, req.SortOrder)
}

func TestValidateClusterLogsQueryRequest_Rejects(t *testing.T) {
	t.Parallel()

	base := func() *types.ClusterLogsQueryRequest {
		return &types.ClusterLogsQueryRequest{
			StartTime: "2026-08-14T16:30:00Z",
			EndTime:   "2026-08-14T17:30:00Z",
		}
	}
	manyValues := make([]string, maxClusterLogsFilterItems+1)
	for i := range manyValues {
		manyValues[i] = fmt.Sprintf("ns-%d", i)
	}

	tests := []struct {
		name    string
		mutate  func(*types.ClusterLogsQueryRequest)
		wantErr string
	}{
		{
			name:    "too many namespaces",
			mutate:  func(r *types.ClusterLogsQueryRequest) { r.Namespaces = manyValues },
			wantErr: "cannot have more than 20 values",
		},
		{
			name: "over-long pod name",
			mutate: func(r *types.ClusterLogsQueryRequest) {
				r.PodNames = []string{strings.Repeat("p", 254)}
			},
			wantErr: "cannot exceed 253 characters",
		},
		{
			name: "duplicate cluster instance",
			mutate: func(r *types.ClusterLogsQueryRequest) {
				r.ClusterInstances = []string{"cluster1", "cluster1"}
			},
			wantErr: "duplicate clusterInstance",
		},
		{
			name: "over-long search phrase",
			mutate: func(r *types.ClusterLogsQueryRequest) {
				r.SearchPhrase = strings.Repeat("x", 257)
			},
			wantErr: "searchPhrase cannot exceed 256 characters",
		},
		{
			name:    "missing time range",
			mutate:  func(r *types.ClusterLogsQueryRequest) { r.StartTime = "" },
			wantErr: "startTime is required",
		},
		{
			name:    "time range beyond the cap",
			mutate:  func(r *types.ClusterLogsQueryRequest) { r.EndTime = "2026-10-14T17:30:00Z" },
			wantErr: "cannot exceed 30 days",
		},
		{
			name:    "unknown log level",
			mutate:  func(r *types.ClusterLogsQueryRequest) { r.LogLevels = []string{"TRACE"} },
			wantErr: "invalid log level",
		},
		{
			name:    "limit above the cap",
			mutate:  func(r *types.ClusterLogsQueryRequest) { r.Limit = 5000 },
			wantErr: "limit cannot exceed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			req := base()
			tt.mutate(req)
			err := ValidateClusterLogsQueryRequest(req)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

// --- handler ---

func TestGetClusterLogs_Success(t *testing.T) {
	t.Parallel()

	svc := servicemocks.NewMockClusterLogsQuerier(t)
	svc.EXPECT().QueryClusterLogs(mock.Anything, mock.Anything).Return(&types.ClusterLogsResponse{
		Logs: []types.ClusterLog{{
			Timestamp:     "2026-08-14T16:31:00Z",
			Log:           "reconcile failed",
			Level:         "ERROR",
			NamespaceName: "openchoreo-control-plane",
			PodName:       "controller-manager-7f58b689b5-pwsb5",
		}},
		Total:  1,
		TookMs: 4,
	}, nil)

	rr := getClusterLogs(t, clusterLogsHandler(t, svc), clusterLogsWindow)

	require.Equal(t, http.StatusOK, rr.Code)
	assert.Contains(t, rr.Body.String(), `"total":1`)
	assert.Contains(t, rr.Body.String(), "reconcile failed")
}

// TestGetClusterLogs_ParsesFilters pins the query-string contract: multi-value
// parameters are comma-separated - the spec declares style: form, explode: false, so
// the generated binder rejects a repeated parameter with a 400 - and the label
// selector reaches the service as parsed pairs.
func TestGetClusterLogs_ParsesFilters(t *testing.T) {
	t.Parallel()

	var got *types.ClusterLogsQueryRequest
	svc := servicemocks.NewMockClusterLogsQuerier(t)
	svc.EXPECT().QueryClusterLogs(mock.Anything, mock.Anything).
		Run(func(_ context.Context, req *types.ClusterLogsQueryRequest) { got = req }).
		Return(&types.ClusterLogsResponse{}, nil)

	query := clusterLogsWindow +
		"&namespace=openchoreo-control-plane,cert-manager" +
		"&podName=pod-a,pod-b" +
		"&clusterInstance=cluster1" +
		"&containerName=manager" +
		"&logLevels=ERROR,WARN" +
		"&labels=openchoreo.dev%2Fplane%3Dcontrolplane" +
		"&searchPhrase=reconcile" +
		"&limit=25&sortOrder=asc"

	rr := getClusterLogs(t, clusterLogsHandler(t, svc), query)
	require.Equal(t, http.StatusOK, rr.Code)

	require.NotNil(t, got)
	assert.Equal(t, []string{"openchoreo-control-plane", "cert-manager"}, got.Namespaces)
	assert.Equal(t, []string{"pod-a", "pod-b"}, got.PodNames)
	assert.Equal(t, []string{"cluster1"}, got.ClusterInstances)
	assert.Equal(t, []string{"manager"}, got.ContainerNames)
	assert.Equal(t, []string{"ERROR", "WARN"}, got.LogLevels)
	assert.Equal(t, map[string]string{"openchoreo.dev/plane": "controlplane"}, got.Labels)
	assert.Equal(t, "reconcile", got.SearchPhrase)
	assert.Equal(t, 25, got.Limit)
	assert.Equal(t, "asc", got.SortOrder)
}

func TestGetClusterLogs_BadRequests(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		query string
	}{
		{name: "missing time range", query: ""},
		{name: "malformed label selector", query: clusterLogsWindow + "&labels=notapair"},
		{
			// style: form, explode: false - a repeated parameter is not the contract.
			name:  "repeated multi-value parameter",
			query: clusterLogsWindow + "&podName=pod-a&podName=pod-b",
		},
		{name: "non-numeric limit", query: clusterLogsWindow + "&limit=abc"},
		{name: "unknown log level", query: clusterLogsWindow + "&logLevels=TRACE"},
		{name: "unknown sort order", query: clusterLogsWindow + "&sortOrder=sideways"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			svc := servicemocks.NewMockClusterLogsQuerier(t)
			rr := getClusterLogs(t, clusterLogsHandler(t, svc), tt.query)
			assert.Equal(t, http.StatusBadRequest, rr.Code)
			svc.AssertNotCalled(t, "QueryClusterLogs", mock.Anything, mock.Anything)
		})
	}
}

func TestGetClusterLogs_ErrorMapping(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		err      error
		wantCode int
		wantBody string
	}{
		{
			name:     "forbidden",
			err:      observerAuthz.ErrAuthzForbidden,
			wantCode: http.StatusForbidden,
		},
		{
			name:     "unauthorized",
			err:      observerAuthz.ErrAuthzUnauthorized,
			wantCode: http.StatusUnauthorized,
		},
		{
			name:     "adapter does not implement the endpoint",
			err:      service.ErrClusterLogsNotSupported,
			wantCode: http.StatusNotImplemented,
			wantBody: types.ErrorCodeV1ClusterLogsNotSupported,
		},
		{
			name:     "retrieval failure",
			err:      fmt.Errorf("%w: boom", service.ErrClusterLogsRetrieval),
			wantCode: http.StatusInternalServerError,
			wantBody: types.ErrorCodeV1ClusterLogsRetrievalFailed,
		},
		{
			name:     "unclassified failure",
			err:      errors.New("boom"),
			wantCode: http.StatusInternalServerError,
			wantBody: types.ErrorCodeV1ClusterLogsInternalGeneric,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			svc := servicemocks.NewMockClusterLogsQuerier(t)
			svc.EXPECT().QueryClusterLogs(mock.Anything, mock.Anything).Return(nil, tt.err)

			rr := getClusterLogs(t, clusterLogsHandler(t, svc), clusterLogsWindow)

			assert.Equal(t, tt.wantCode, rr.Code)
			if tt.wantBody != "" {
				assert.Contains(t, rr.Body.String(), tt.wantBody)
			}
		})
	}
}

func TestGetClusterLogs_ServiceNotInitialized(t *testing.T) {
	t.Parallel()

	rr := getClusterLogs(t, clusterLogsHandler(t, nil), clusterLogsWindow)

	assert.Equal(t, http.StatusInternalServerError, rr.Code)
	assert.Contains(t, rr.Body.String(), types.ErrorCodeV1ClusterLogsServiceNotReady)
}
