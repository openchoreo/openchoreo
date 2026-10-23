// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/openchoreo/openchoreo/internal/observer/service"
	servicemocks "github.com/openchoreo/openchoreo/internal/observer/service/mocks"
)

// A search scope naming a resource that doesn't exist, such as a deleted release line, is a
// client error on every query rather than a server failure.
func TestQueries_ScopeNotFound(t *testing.T) {
	t.Parallel()

	notFound := func(subsystem error) error {
		return fmt.Errorf("%w: failed to resolve release line: %w", subsystem,
			fmt.Errorf("%w: /api/v1/namespaces/ns/components/comp/releaselines/ghost", service.ErrResourceNotFound))
	}
	base := baseHandler{logger: noopLogger()}

	tests := map[string]func(t *testing.T) (*Handler, *http.Request){
		"logs": func(t *testing.T) (*Handler, *http.Request) {
			svc := servicemocks.NewMockLogsQuerier(t)
			svc.On("QueryLogs", mock.Anything, mock.Anything).Return(nil, notFound(service.ErrLogsResolveSearchScope))
			return &Handler{baseHandler: base, logsService: svc},
				httptest.NewRequest(http.MethodPost, "/api/v1/logs/query", validLogsRequestBody(t))
		},
		"events": func(t *testing.T) (*Handler, *http.Request) {
			svc := servicemocks.NewMockEventsQuerier(t)
			svc.On("QueryEvents", mock.Anything, mock.Anything).Return(nil, notFound(service.ErrEventsResolveSearchScope))
			return &Handler{baseHandler: base, eventsService: svc},
				httptest.NewRequest(http.MethodPost, "/api/v1/events/query", validEventsRequestBody(t))
		},
		"metrics": func(t *testing.T) (*Handler, *http.Request) {
			svc := servicemocks.NewMockMetricsQuerier(t)
			svc.On("QueryMetrics", mock.Anything, mock.Anything).Return(nil, notFound(service.ErrMetricsResolveSearchScope))
			return &Handler{baseHandler: base, metricsService: svc},
				httptest.NewRequest(http.MethodPost, "/api/v1/metrics/query", validMetricsRequestBody(t))
		},
		"runtime topology": func(t *testing.T) (*Handler, *http.Request) {
			svc := servicemocks.NewMockMetricsQuerier(t)
			svc.On("QueryRuntimeTopology", mock.Anything, mock.Anything).
				Return(nil, notFound(service.ErrRuntimeTopologyResolveSearchScope))
			return &Handler{baseHandler: base, metricsService: svc},
				httptest.NewRequest(http.MethodPost, "/api/v1alpha1/metrics/runtime-topology", validRuntimeTopologyRequestBody(t))
		},
		"traces": func(t *testing.T) (*Handler, *http.Request) {
			svc := servicemocks.NewMockTracesQuerier(t)
			svc.On("QueryTraces", mock.Anything, mock.Anything).Return(nil, notFound(service.ErrTracesResolveSearchScope))
			return &Handler{baseHandler: base, tracesService: svc},
				httptest.NewRequest(http.MethodPost, "/api/v1alpha1/traces/query", validTracesRequestBody(t))
		},
		"costs": func(t *testing.T) (*Handler, *http.Request) {
			svc := servicemocks.NewMockFinOpsQuerier(t)
			svc.On("GetComponentCosts", mock.Anything, mock.Anything).Return(nil, notFound(service.ErrFinOpsResolveScope))
			return &Handler{baseHandler: base, finOpsService: svc}, newCostsRequest(validCostsQuery)
		},
	}
	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			h, req := setup(t)
			rr := serve(t, h, req)
			require.Equal(t, http.StatusBadRequest, rr.Code, rr.Body.String())
			assert.Contains(t, rr.Body.String(), scopeNotFoundCode)
		})
	}
}
