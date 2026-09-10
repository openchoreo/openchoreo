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

const platformLogsWindow = "startTime=2026-08-14T16:30:00Z&endTime=2026-08-14T17:30:00Z"

func platformLogsHandler(t *testing.T, svc service.PlatformLogsQuerier) *Handler {
	t.Helper()
	return &Handler{
		baseHandler:         baseHandler{logger: noopLogger()},
		platformLogsService: svc,
	}
}

func getPlatformLogs(t *testing.T, h *Handler, query string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/v1alpha1/platform-logs?"+query, nil)
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

func TestValidatePlatformLogsQueryRequest_AppliesDefaults(t *testing.T) {
	t.Parallel()

	req := &types.PlatformLogsQueryRequest{
		StartTime: "2026-08-14T16:30:00Z",
		EndTime:   "2026-08-14T17:30:00Z",
	}
	require.NoError(t, ValidatePlatformLogsQueryRequest(req))
	assert.Equal(t, defaultLimit, req.Limit)
	assert.Equal(t, defaultSortOrder, req.SortOrder)
}

func TestValidatePlatformLogsQueryRequest_Rejects(t *testing.T) {
	t.Parallel()

	base := func() *types.PlatformLogsQueryRequest {
		return &types.PlatformLogsQueryRequest{
			StartTime: "2026-08-14T16:30:00Z",
			EndTime:   "2026-08-14T17:30:00Z",
		}
	}
	manyValues := make([]string, maxPlatformLogsFilterItems+1)
	for i := range manyValues {
		manyValues[i] = fmt.Sprintf("ns-%d", i)
	}

	tests := []struct {
		name    string
		mutate  func(*types.PlatformLogsQueryRequest)
		wantErr string
	}{
		{
			name:    "too many namespaces",
			mutate:  func(r *types.PlatformLogsQueryRequest) { r.Namespaces = manyValues },
			wantErr: "cannot have more than 20 values",
		},
		{
			name: "over-long pod name",
			mutate: func(r *types.PlatformLogsQueryRequest) {
				r.PodNames = []string{strings.Repeat("p", 254)}
			},
			wantErr: "cannot exceed 253 characters",
		},
		{
			name: "duplicate cluster instance",
			mutate: func(r *types.PlatformLogsQueryRequest) {
				r.ClusterInstances = []string{"cluster1", "cluster1"}
			},
			wantErr: "duplicate clusterInstance",
		},
		{
			name: "over-long search phrase",
			mutate: func(r *types.PlatformLogsQueryRequest) {
				r.SearchPhrase = strings.Repeat("x", 257)
			},
			wantErr: "searchPhrase cannot exceed 256 characters",
		},
		{
			name:    "missing time range",
			mutate:  func(r *types.PlatformLogsQueryRequest) { r.StartTime = "" },
			wantErr: "startTime is required",
		},
		{
			name:    "time range beyond the cap",
			mutate:  func(r *types.PlatformLogsQueryRequest) { r.EndTime = "2026-10-14T17:30:00Z" },
			wantErr: "cannot exceed 30 days",
		},
		{
			name:    "unknown log level",
			mutate:  func(r *types.PlatformLogsQueryRequest) { r.LogLevels = []string{"TRACE"} },
			wantErr: "invalid log level",
		},
		{
			name:    "limit above the cap",
			mutate:  func(r *types.PlatformLogsQueryRequest) { r.Limit = 5000 },
			wantErr: "limit cannot exceed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			req := base()
			tt.mutate(req)
			err := ValidatePlatformLogsQueryRequest(req)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

// --- handler ---

func TestGetPlatformLogs_Success(t *testing.T) {
	t.Parallel()

	svc := servicemocks.NewMockPlatformLogsQuerier(t)
	svc.EXPECT().QueryPlatformLogs(mock.Anything, mock.Anything).Return(&types.PlatformLogsResponse{
		Logs: []types.PlatformLog{{
			Timestamp:     "2026-08-14T16:31:00Z",
			Log:           "reconcile failed",
			Level:         "ERROR",
			NamespaceName: "openchoreo-control-plane",
			PodName:       "controller-manager-7f58b689b5-pwsb5",
		}},
		Total:  1,
		TookMs: 4,
	}, nil)

	rr := getPlatformLogs(t, platformLogsHandler(t, svc), platformLogsWindow)

	require.Equal(t, http.StatusOK, rr.Code)
	assert.Contains(t, rr.Body.String(), `"total":1`)
	assert.Contains(t, rr.Body.String(), "reconcile failed")
}

// TestGetPlatformLogs_ParsesFilters pins the query-string contract: multi-value
// parameters are comma-separated - the spec declares style: form, explode: false, so
// the generated binder rejects a repeated parameter with a 400 - and the label
// selector reaches the service as parsed pairs.
func TestGetPlatformLogs_ParsesFilters(t *testing.T) {
	t.Parallel()

	var got *types.PlatformLogsQueryRequest
	svc := servicemocks.NewMockPlatformLogsQuerier(t)
	svc.EXPECT().QueryPlatformLogs(mock.Anything, mock.Anything).
		Run(func(_ context.Context, req *types.PlatformLogsQueryRequest) { got = req }).
		Return(&types.PlatformLogsResponse{}, nil)

	query := platformLogsWindow +
		"&namespace=openchoreo-control-plane,cert-manager" +
		"&podName=pod-a,pod-b" +
		"&clusterInstance=cluster1" +
		"&containerName=manager" +
		"&logLevels=ERROR,WARN" +
		"&labels=openchoreo.dev%2Fplane%3Dcontrolplane" +
		"&searchPhrase=reconcile" +
		"&limit=25&sortOrder=asc"

	rr := getPlatformLogs(t, platformLogsHandler(t, svc), query)
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

// TestGetPlatformLogs_IncludeFacets pins the opt-in: absent means false, so an
// existing caller never starts paying for an aggregation it did not ask for.
func TestGetPlatformLogs_IncludeFacets(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		query string
		want  bool
	}{
		{name: "absent", query: platformLogsWindow, want: false},
		{name: "true", query: platformLogsWindow + "&includeFacets=true", want: true},
		{name: "false", query: platformLogsWindow + "&includeFacets=false", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var got *types.PlatformLogsQueryRequest
			svc := servicemocks.NewMockPlatformLogsQuerier(t)
			svc.EXPECT().QueryPlatformLogs(mock.Anything, mock.Anything).
				Run(func(_ context.Context, req *types.PlatformLogsQueryRequest) { got = req }).
				Return(&types.PlatformLogsResponse{}, nil)

			rr := getPlatformLogs(t, platformLogsHandler(t, svc), tt.query)
			require.Equal(t, http.StatusOK, rr.Code)

			require.NotNil(t, got)
			assert.Equal(t, tt.want, got.IncludeFacets)
		})
	}
}

// TestGetPlatformLogs_SerializesFacets pins the response shape a client reads its
// pickers from - the keys are the filter parameter names - and that an absent
// facets field is omitted rather than serialized as null.
func TestGetPlatformLogs_SerializesFacets(t *testing.T) {
	t.Parallel()

	t.Run("present", func(t *testing.T) {
		t.Parallel()

		svc := servicemocks.NewMockPlatformLogsQuerier(t)
		svc.EXPECT().QueryPlatformLogs(mock.Anything, mock.Anything).Return(&types.PlatformLogsResponse{
			Logs: []types.PlatformLog{},
			Facets: &types.PlatformLogFacets{
				Namespace: []types.PlatformLogFacetValue{
					{Value: "openchoreo-control-plane", Count: 412},
				},
			},
		}, nil)

		rr := getPlatformLogs(t, platformLogsHandler(t, svc),
			platformLogsWindow+"&includeFacets=true")

		require.Equal(t, http.StatusOK, rr.Code)
		assert.Contains(t, rr.Body.String(),
			`"facets":{"namespace":[{"value":"openchoreo-control-plane","count":412}]}`)
	})

	t.Run("absent", func(t *testing.T) {
		t.Parallel()

		svc := servicemocks.NewMockPlatformLogsQuerier(t)
		svc.EXPECT().QueryPlatformLogs(mock.Anything, mock.Anything).
			Return(&types.PlatformLogsResponse{Logs: []types.PlatformLog{}}, nil)

		rr := getPlatformLogs(t, platformLogsHandler(t, svc), platformLogsWindow)

		require.Equal(t, http.StatusOK, rr.Code)
		assert.NotContains(t, rr.Body.String(), "facets")
	})
}

func TestGetPlatformLogs_BadRequests(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		query string
	}{
		{name: "missing time range", query: ""},
		{name: "malformed label selector", query: platformLogsWindow + "&labels=notapair"},
		{
			// style: form, explode: false - a repeated parameter is not the contract.
			name:  "repeated multi-value parameter",
			query: platformLogsWindow + "&podName=pod-a&podName=pod-b",
		},
		{name: "non-numeric limit", query: platformLogsWindow + "&limit=abc"},
		{name: "unknown log level", query: platformLogsWindow + "&logLevels=TRACE"},
		{name: "unknown sort order", query: platformLogsWindow + "&sortOrder=sideways"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			svc := servicemocks.NewMockPlatformLogsQuerier(t)
			rr := getPlatformLogs(t, platformLogsHandler(t, svc), tt.query)
			assert.Equal(t, http.StatusBadRequest, rr.Code)
			svc.AssertNotCalled(t, "QueryPlatformLogs", mock.Anything, mock.Anything)
		})
	}
}

func TestGetPlatformLogs_ErrorMapping(t *testing.T) {
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
			err:      service.ErrPlatformLogsNotSupported,
			wantCode: http.StatusNotImplemented,
			wantBody: types.ErrorCodeV1PlatformLogsNotSupported,
		},
		{
			name:     "retrieval failure",
			err:      fmt.Errorf("%w: boom", service.ErrPlatformLogsRetrieval),
			wantCode: http.StatusInternalServerError,
			wantBody: types.ErrorCodeV1PlatformLogsRetrievalFailed,
		},
		{
			name:     "unclassified failure",
			err:      errors.New("boom"),
			wantCode: http.StatusInternalServerError,
			wantBody: types.ErrorCodeV1PlatformLogsInternalGeneric,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			svc := servicemocks.NewMockPlatformLogsQuerier(t)
			svc.EXPECT().QueryPlatformLogs(mock.Anything, mock.Anything).Return(nil, tt.err)

			rr := getPlatformLogs(t, platformLogsHandler(t, svc), platformLogsWindow)

			assert.Equal(t, tt.wantCode, rr.Code)
			if tt.wantBody != "" {
				assert.Contains(t, rr.Body.String(), tt.wantBody)
			}
		})
	}
}

func TestGetPlatformLogs_ServiceNotInitialized(t *testing.T) {
	t.Parallel()

	rr := getPlatformLogs(t, platformLogsHandler(t, nil), platformLogsWindow)

	assert.Equal(t, http.StatusInternalServerError, rr.Code)
	assert.Contains(t, rr.Body.String(), types.ErrorCodeV1PlatformLogsServiceNotReady)
}
