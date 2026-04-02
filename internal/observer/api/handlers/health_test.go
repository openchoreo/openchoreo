// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	servicemocks "github.com/openchoreo/openchoreo/internal/observer/service/mocks"
)

func TestHealth_Healthy(t *testing.T) {
	t.Parallel()

	svc := servicemocks.NewMockHealthChecker(t)
	svc.On("Check", mock.Anything).Return(nil)

	h := &Handler{
		baseHandler:   baseHandler{logger: noopLogger()},
		healthService: svc,
	}

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rr := httptest.NewRecorder()

	h.Health(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	assert.Contains(t, rr.Body.String(), `"status":"healthy"`)
}

func TestHealth_Unhealthy(t *testing.T) {
	t.Parallel()

	svc := servicemocks.NewMockHealthChecker(t)
	svc.On("Check", mock.Anything).Return(errors.New("backend unavailable"))

	h := &Handler{
		baseHandler:   baseHandler{logger: noopLogger()},
		healthService: svc,
	}

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rr := httptest.NewRecorder()

	h.Health(rr, req)

	require.Equal(t, http.StatusServiceUnavailable, rr.Code)
	assert.Contains(t, rr.Body.String(), `"status":"unhealthy"`)
}
