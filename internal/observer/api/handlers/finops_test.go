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
	"github.com/openchoreo/openchoreo/internal/observer/types"
)

// newCostsRequest builds a GET costs request with the given raw query string.
// Path values come from the URL -- the generated router binds them.
func newCostsRequest(query string) *http.Request {
	target := "/api/v1alpha1/costs/namespaces/default/environments/production"
	if query != "" {
		target += "?" + query
	}
	return httptest.NewRequest(http.MethodGet, target, nil)
}

func newRecommendationsRequest(query string) *http.Request {
	target := "/api/v1alpha1/costs/namespaces/default/environments/production/recommendations"
	if query != "" {
		target += "?" + query
	}
	return httptest.NewRequest(http.MethodGet, target, nil)
}

const validCostsQuery = "startTime=2026-05-23T10:00:01Z&endTime=2026-05-24T10:00:01Z"

func TestGetComponentCosts_Success(t *testing.T) {
	t.Parallel()

	svc := servicemocks.NewMockFinOpsQuerier(t)
	svc.On("GetComponentCosts", mock.Anything, mock.MatchedBy(func(req *types.CostQueryRequest) bool {
		return assert.ObjectsAreEqual(&types.CostQueryRequest{
			Namespace:   "default",
			Environment: "production",
			Project:     "checkout",
			Component:   "payment-service",
			StartTime:   "2026-05-23T10:00:01Z",
			EndTime:     "2026-05-24T10:00:01Z",
			Granularity: "1d",
		}, req)
	})).Return(map[string]any{"items": []any{}}, nil)

	h := &Handler{
		baseHandler:   baseHandler{logger: noopLogger()},
		finOpsService: svc,
	}

	rr := serve(t, h, newCostsRequest(
		validCostsQuery+"&project=checkout&component=payment-service&granularity=1d"))

	require.Equal(t, http.StatusOK, rr.Code)
	assert.Contains(t, rr.Body.String(), `"items"`)
	// The mock's MatchedBy asserts namespace and environment arrived from the
	// path and the times survived the time.Time round-trip unchanged.
	svc.AssertExpectations(t)
}

// TestGetComponentCosts_MissingTimes covers the other half of the error-shape
// guarantee, and it is the first case that reaches ParamBindingErrorHandler.
//
// startTime and endTime are required non-pointer query parameters, so the
// generated wrapper rejects the request before the handler runs. Without an
// explicit ErrorHandlerFunc that 400 would be plain text from http.Error, unlike
// every other observer error. Pre-migration this case reached
// ValidateCostQueryRequest instead, which produced a domain message -- the
// status is the same, the message is not.
func TestGetComponentCosts_MissingTimes(t *testing.T) {
	t.Parallel()

	h := &Handler{
		baseHandler:   baseHandler{logger: noopLogger()},
		finOpsService: servicemocks.NewMockFinOpsQuerier(t),
	}

	rr := serve(t, h, newCostsRequest(""))

	require.Equal(t, http.StatusBadRequest, rr.Code)
	assertErrorResponseShape(t, rr)
	assert.Contains(t, rr.Body.String(), "INVALID_PARAMETER")
	assert.Contains(t, rr.Body.String(), "startTime")
}

// TestGetComponentCosts_UnparseableTime is also rejected during binding, since
// the generated parameter is a time.Time.
func TestGetComponentCosts_UnparseableTime(t *testing.T) {
	t.Parallel()

	h := &Handler{
		baseHandler:   baseHandler{logger: noopLogger()},
		finOpsService: servicemocks.NewMockFinOpsQuerier(t),
	}

	rr := serve(t, h, newCostsRequest("startTime=not-a-date&endTime=2026-05-24T10:00:01Z"))

	require.Equal(t, http.StatusBadRequest, rr.Code)
	assertErrorResponseShape(t, rr)
}

func TestGetComponentCosts_ComponentWithoutProject(t *testing.T) {
	t.Parallel()

	h := &Handler{
		baseHandler:   baseHandler{logger: noopLogger()},
		finOpsService: servicemocks.NewMockFinOpsQuerier(t),
	}

	rr := serve(t, h, newCostsRequest(validCostsQuery+"&component=payment-service"))

	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestGetComponentCosts_BadGranularity(t *testing.T) {
	t.Parallel()

	h := &Handler{
		baseHandler:   baseHandler{logger: noopLogger()},
		finOpsService: servicemocks.NewMockFinOpsQuerier(t),
	}

	rr := serve(t, h, newCostsRequest(validCostsQuery+"&granularity=daily"))

	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestGetComponentCosts_ServiceNotInitialized(t *testing.T) {
	t.Parallel()

	h := &Handler{baseHandler: baseHandler{logger: noopLogger()}}

	rr := serve(t, h, newCostsRequest(validCostsQuery))

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

	rr := serve(t, h, newCostsRequest(validCostsQuery))

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

	rr := serve(t, h, newCostsRequest(validCostsQuery))

	assert.Equal(t, http.StatusInternalServerError, rr.Code)
}

func TestGetRecommendations_Success(t *testing.T) {
	t.Parallel()

	svc := servicemocks.NewMockFinOpsQuerier(t)
	svc.On("GetRecommendations", mock.Anything, mock.MatchedBy(func(req *types.RecommendationQueryRequest) bool {
		return assert.ObjectsAreEqual(&types.RecommendationQueryRequest{
			Namespace:   "default",
			Environment: "production",
			Project:     "checkout",
			StartTime:   "2026-05-23T10:00:01Z",
			EndTime:     "2026-05-24T10:00:01Z",
		}, req)
	})).Return(map[string]any{"items": []any{}}, nil)

	h := &Handler{
		baseHandler:   baseHandler{logger: noopLogger()},
		finOpsService: svc,
	}

	rr := serve(t, h, newRecommendationsRequest(validCostsQuery+"&project=checkout"))

	require.Equal(t, http.StatusOK, rr.Code)
	assert.Contains(t, rr.Body.String(), `"items"`)
	svc.AssertExpectations(t)
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

	rr := serve(t, h, newRecommendationsRequest(validCostsQuery))

	assert.Equal(t, http.StatusForbidden, rr.Code)
}
