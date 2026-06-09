// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"errors"
	"fmt"
	"io"
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

func validEventsRequestBody(t *testing.T) io.Reader {
	t.Helper()
	return strings.NewReader(`{
		"startTime": "2026-06-05T02:58:31Z",
		"endTime": "2026-06-05T03:08:31Z",
		"limit": 50,
		"sortOrder": "asc",
		"searchScope": {
			"namespace": "default",
			"project": "default",
			"component": "github-issue-reporter",
			"environment": "development"
		}
	}`)
}

func TestQueryEvents_Success(t *testing.T) {
	t.Parallel()

	svc := servicemocks.NewMockEventsQuerier(t)
	svc.On("QueryEvents", mock.Anything, mock.Anything).Return(&types.EventsQueryResponse{
		Events: []types.EventEntry{{Timestamp: "2026-06-05T03:07:12Z", Message: "done", Type: "Normal", Reason: "SawCompletedJob"}},
		Total:  1,
		TookMs: 5,
	}, nil)

	h := &Handler{
		baseHandler:   baseHandler{logger: noopLogger()},
		eventsService: svc,
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/events/query", validEventsRequestBody(t))
	rr := httptest.NewRecorder()

	h.QueryEvents(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	assert.Contains(t, rr.Body.String(), `"total":1`)
	assert.Contains(t, rr.Body.String(), "SawCompletedJob")
}

func TestQueryEvents_InvalidBody(t *testing.T) {
	t.Parallel()

	h := &Handler{
		baseHandler:   baseHandler{logger: noopLogger()},
		eventsService: servicemocks.NewMockEventsQuerier(t),
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/events/query", strings.NewReader("not-json"))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	h.QueryEvents(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestQueryEvents_ValidationError(t *testing.T) {
	t.Parallel()

	h := &Handler{
		baseHandler:   baseHandler{logger: noopLogger()},
		eventsService: servicemocks.NewMockEventsQuerier(t),
	}

	// Missing searchScope → validation failure.
	body := `{"startTime":"2026-06-05T02:58:31Z","endTime":"2026-06-05T03:08:31Z"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/events/query", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	h.QueryEvents(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestQueryEvents_ServiceNotInitialized(t *testing.T) {
	t.Parallel()

	h := &Handler{
		baseHandler:   baseHandler{logger: noopLogger()},
		eventsService: nil,
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/events/query", validEventsRequestBody(t))
	rr := httptest.NewRecorder()

	h.QueryEvents(rr, req)

	require.Equal(t, http.StatusInternalServerError, rr.Code)
	assert.Contains(t, rr.Body.String(), types.ErrorCodeV1EventsServiceNotReady)
}

func TestQueryEvents_AuthzForbidden(t *testing.T) {
	t.Parallel()

	svc := servicemocks.NewMockEventsQuerier(t)
	svc.On("QueryEvents", mock.Anything, mock.Anything).Return(nil, observerAuthz.ErrAuthzForbidden)

	h := &Handler{
		baseHandler:   baseHandler{logger: noopLogger()},
		eventsService: svc,
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/events/query", validEventsRequestBody(t))
	rr := httptest.NewRecorder()

	h.QueryEvents(rr, req)

	assert.Equal(t, http.StatusForbidden, rr.Code)
}

func TestQueryEvents_AuthzUnauthorized(t *testing.T) {
	t.Parallel()

	svc := servicemocks.NewMockEventsQuerier(t)
	svc.On("QueryEvents", mock.Anything, mock.Anything).Return(nil, observerAuthz.ErrAuthzUnauthorized)

	h := &Handler{
		baseHandler:   baseHandler{logger: noopLogger()},
		eventsService: svc,
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/events/query", validEventsRequestBody(t))
	rr := httptest.NewRecorder()

	h.QueryEvents(rr, req)

	assert.Equal(t, http.StatusUnauthorized, rr.Code)
}

func TestQueryEvents_ResolveSearchScopeError(t *testing.T) {
	t.Parallel()

	svc := servicemocks.NewMockEventsQuerier(t)
	svc.On("QueryEvents", mock.Anything, mock.Anything).
		Return(nil, fmt.Errorf("%w: resolver failed", service.ErrEventsResolveSearchScope))

	h := &Handler{
		baseHandler:   baseHandler{logger: noopLogger()},
		eventsService: svc,
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/events/query", validEventsRequestBody(t))
	rr := httptest.NewRecorder()

	h.QueryEvents(rr, req)

	require.Equal(t, http.StatusInternalServerError, rr.Code)
	assert.Contains(t, rr.Body.String(), types.ErrorCodeV1EventsResolverFailed)
}

func TestQueryEvents_RetrievalError(t *testing.T) {
	t.Parallel()

	svc := servicemocks.NewMockEventsQuerier(t)
	svc.On("QueryEvents", mock.Anything, mock.Anything).
		Return(nil, fmt.Errorf("%w: backend error", service.ErrEventsRetrieval))

	h := &Handler{
		baseHandler:   baseHandler{logger: noopLogger()},
		eventsService: svc,
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/events/query", validEventsRequestBody(t))
	rr := httptest.NewRecorder()

	h.QueryEvents(rr, req)

	require.Equal(t, http.StatusInternalServerError, rr.Code)
	assert.Contains(t, rr.Body.String(), types.ErrorCodeV1EventsRetrievalFailed)
}

func TestQueryEvents_GenericError(t *testing.T) {
	t.Parallel()

	svc := servicemocks.NewMockEventsQuerier(t)
	svc.On("QueryEvents", mock.Anything, mock.Anything).Return(nil, errors.New("unexpected error"))

	h := &Handler{
		baseHandler:   baseHandler{logger: noopLogger()},
		eventsService: svc,
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/events/query", validEventsRequestBody(t))
	rr := httptest.NewRecorder()

	h.QueryEvents(rr, req)

	require.Equal(t, http.StatusInternalServerError, rr.Code)
	assert.Contains(t, rr.Body.String(), types.ErrorCodeV1EventsInternalGeneric)
}
