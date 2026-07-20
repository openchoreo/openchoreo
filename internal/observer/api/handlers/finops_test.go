// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	observerAuthz "github.com/openchoreo/openchoreo/internal/observer/authz"
	"github.com/openchoreo/openchoreo/internal/observer/service"
	servicemocks "github.com/openchoreo/openchoreo/internal/observer/service/mocks"
)

// newCostsRequest builds a GET costs request with path values populated (as the
// ServeMux would) and the given raw query string.
func newCostsRequest(query string) *http.Request {
	target := "/api/v1alpha1/costs/namespaces/default/environments/production"
	if query != "" {
		target += "?" + query
	}
	req := httptest.NewRequest(http.MethodGet, target, nil)
	req.SetPathValue("namespace", "default")
	req.SetPathValue("environment", "production")
	return req
}

func newRecommendationsRequest(query string) *http.Request {
	target := "/api/v1alpha1/costs/namespaces/default/environments/production/recommendations"
	if query != "" {
		target += "?" + query
	}
	req := httptest.NewRequest(http.MethodGet, target, nil)
	req.SetPathValue("namespace", "default")
	req.SetPathValue("environment", "production")
	return req
}

const validCostsQuery = "startTime=2026-05-23T10:00:01Z&endTime=2026-05-24T10:00:01Z"

func TestGetComponentCosts_Success(t *testing.T) {
	t.Parallel()

	svc := servicemocks.NewMockFinOpsQuerier(t)
	svc.On("GetComponentCosts", mock.Anything, mock.Anything).Return(map[string]any{"items": []any{}}, nil)

	h := &Handler{
		baseHandler:   baseHandler{logger: noopLogger()},
		finOpsService: svc,
	}

	rr := httptest.NewRecorder()
	h.GetComponentCosts(rr, newCostsRequest(validCostsQuery+"&project=p&component=c&granularity=1d"))

	require.Equal(t, http.StatusOK, rr.Code)
	assert.Contains(t, rr.Body.String(), `"items"`)
}

func TestGetComponentCosts_MissingTimes(t *testing.T) {
	t.Parallel()

	h := &Handler{
		baseHandler:   baseHandler{logger: noopLogger()},
		finOpsService: servicemocks.NewMockFinOpsQuerier(t),
	}

	rr := httptest.NewRecorder()
	h.GetComponentCosts(rr, newCostsRequest(""))

	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestGetComponentCosts_ComponentWithoutProject(t *testing.T) {
	t.Parallel()

	h := &Handler{
		baseHandler:   baseHandler{logger: noopLogger()},
		finOpsService: servicemocks.NewMockFinOpsQuerier(t),
	}

	rr := httptest.NewRecorder()
	h.GetComponentCosts(rr, newCostsRequest(validCostsQuery+"&component=c"))

	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestGetComponentCosts_BadGranularity(t *testing.T) {
	t.Parallel()

	h := &Handler{
		baseHandler:   baseHandler{logger: noopLogger()},
		finOpsService: servicemocks.NewMockFinOpsQuerier(t),
	}

	rr := httptest.NewRecorder()
	h.GetComponentCosts(rr, newCostsRequest(validCostsQuery+"&granularity=daily"))

	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestGetComponentCosts_ServiceNotInitialized(t *testing.T) {
	t.Parallel()

	h := &Handler{baseHandler: baseHandler{logger: noopLogger()}}

	rr := httptest.NewRecorder()
	h.GetComponentCosts(rr, newCostsRequest(validCostsQuery))

	assert.Equal(t, http.StatusInternalServerError, rr.Code)
}

func TestGetComponentCosts_Forbidden(t *testing.T) {
	t.Parallel()

	svc := servicemocks.NewMockFinOpsQuerier(t)
	svc.On("GetComponentCosts", mock.Anything, mock.Anything).
		Return(nil, observerAuthz.ErrAuthzForbidden)

	h := &Handler{
		baseHandler:   baseHandler{logger: noopLogger()},
		finOpsService: svc,
	}

	rr := httptest.NewRecorder()
	h.GetComponentCosts(rr, newCostsRequest(validCostsQuery))

	assert.Equal(t, http.StatusForbidden, rr.Code)
}

func TestGetComponentCosts_RetrievalError(t *testing.T) {
	t.Parallel()

	svc := servicemocks.NewMockFinOpsQuerier(t)
	svc.On("GetComponentCosts", mock.Anything, mock.Anything).
		Return(nil, service.ErrFinOpsRetrieval)

	h := &Handler{
		baseHandler:   baseHandler{logger: noopLogger()},
		finOpsService: svc,
	}

	rr := httptest.NewRecorder()
	h.GetComponentCosts(rr, newCostsRequest(validCostsQuery))

	assert.Equal(t, http.StatusInternalServerError, rr.Code)
}

func TestGetRecommendations_Success(t *testing.T) {
	t.Parallel()

	svc := servicemocks.NewMockFinOpsQuerier(t)
	svc.On("GetRecommendations", mock.Anything, mock.Anything).Return(map[string]any{"items": []any{}}, nil)

	h := &Handler{
		baseHandler:   baseHandler{logger: noopLogger()},
		finOpsService: svc,
	}

	rr := httptest.NewRecorder()
	h.GetRecommendations(rr, newRecommendationsRequest(validCostsQuery+"&project=p"))

	require.Equal(t, http.StatusOK, rr.Code)
	assert.Contains(t, rr.Body.String(), `"items"`)
}

func TestGetRecommendations_Forbidden(t *testing.T) {
	t.Parallel()

	svc := servicemocks.NewMockFinOpsQuerier(t)
	svc.On("GetRecommendations", mock.Anything, mock.Anything).
		Return(nil, observerAuthz.ErrAuthzForbidden)

	h := &Handler{
		baseHandler:   baseHandler{logger: noopLogger()},
		finOpsService: svc,
	}

	rr := httptest.NewRecorder()
	h.GetRecommendations(rr, newRecommendationsRequest(validCostsQuery))

	assert.Equal(t, http.StatusForbidden, rr.Code)
}
