// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	observerAuthz "github.com/openchoreo/openchoreo/internal/observer/authz"
	"github.com/openchoreo/openchoreo/internal/observer/querylimit"
	"github.com/openchoreo/openchoreo/internal/observer/service"
	servicemocks "github.com/openchoreo/openchoreo/internal/observer/service/mocks"
	"github.com/openchoreo/openchoreo/internal/observer/types"
)

const rawQueryPath = RawTracesQueryPath

// rawQueryRequest builds a raw trace query request with a native backend body.
func rawQueryRequest(contentType, body string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, rawQueryPath, strings.NewReader(body))
	req.Header.Set("Content-Type", contentType)
	return req
}

type rawErrorBody struct {
	Title     string `json:"title"`
	ErrorCode string `json:"errorCode"`
	Message   string `json:"message"`
}

func decodeRawError(t *testing.T, rr *httptest.ResponseRecorder) rawErrorBody {
	t.Helper()
	var body rawErrorBody
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &body), rr.Body.String())
	return body
}

func TestQueryRawTraces_PassesAJSONRequestAndTheResponseThrough(t *testing.T) {
	t.Parallel()

	const request = `{"size":0,"query":{"range":{"startTime":{"gte":1792404839161624162}}}}`
	const response = "{\n  \"took\": 3, \"q\": \"a < b && c > d\",\n  \"hits\": {\"total\": {\"value\": 12345678901234567890}}\n}"
	svc := servicemocks.NewMockTracesQuerier(t)
	svc.On("QueryRawTraces", mock.Anything, mock.MatchedBy(func(req *types.RawTracesQueryRequest) bool {
		// The request reaches the service byte for byte, so numbers keep their precision.
		return req.ContentType == jsonMediaType && string(req.Body) == request
	})).Return(&types.RawTracesQueryResponse{Body: json.RawMessage(response)}, nil)

	h := &Handler{baseHandler: baseHandler{logger: noopLogger()}, tracesService: svc}
	rr := serve(t, h, rawQueryRequest(jsonMediaType, request))

	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
	assert.Equal(t, response, rr.Body.String(), "the response must be the backend's, byte for byte")
	assert.Contains(t, rr.Body.String(), "12345678901234567890", "the response must not lose number precision")
}

func TestQueryRawTraces_PassesATextRequestThrough(t *testing.T) {
	t.Parallel()

	const kql = "AppDependencies | where Success == false | take 10"
	svc := servicemocks.NewMockTracesQuerier(t)
	svc.On("QueryRawTraces", mock.Anything, mock.MatchedBy(func(req *types.RawTracesQueryRequest) bool {
		return req.ContentType == "text/plain" && string(req.Body) == kql
	})).Return(&types.RawTracesQueryResponse{Body: json.RawMessage(`{"tables":[]}`)}, nil)

	h := &Handler{baseHandler: baseHandler{logger: noopLogger()}, tracesService: svc}
	rr := serve(t, h, rawQueryRequest("text/plain", kql))

	assert.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
}

func TestQueryRawTraces_TreatsMediaTypesCaseInsensitively(t *testing.T) {
	t.Parallel()

	tests := []struct {
		contentType string
		want        string
	}{
		{"Application/JSON", jsonMediaType},
		{"APPLICATION/JSON; Charset=UTF-8", jsonMediaType},
		{"Text/Plain", "text/plain"},
		{"TEXT/PLAIN; charset=UTF-8", "text/plain"},
		{"text/plain; Charset=utf-8", "text/plain"},
	}
	for _, tt := range tests {
		t.Run(tt.contentType, func(t *testing.T) {
			t.Parallel()
			svc := servicemocks.NewMockTracesQuerier(t)
			svc.On("QueryRawTraces", mock.Anything, mock.MatchedBy(func(req *types.RawTracesQueryRequest) bool {
				return req.ContentType == tt.want && string(req.Body) == `{"size":0}`
			})).Return(&types.RawTracesQueryResponse{Body: json.RawMessage(`{}`)}, nil)
			h := &Handler{baseHandler: baseHandler{logger: noopLogger()}, tracesService: svc}

			rr := serve(t, h, rawQueryRequest(tt.contentType, `{"size":0}`))

			assert.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
		})
	}
}

func TestQueryRawTraces_RejectsInvalidRequests(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		contentType string
		body        string
		status      int
		errorCode   string
		message     string
	}{
		{"empty JSON body", jsonMediaType, "null", http.StatusBadRequest,
			types.ErrorCodeV1TracesInvalidRequest, "the request body is empty"},
		{"blank text body", "text/plain", "   ", http.StatusBadRequest,
			types.ErrorCodeV1TracesInvalidRequest, "the request body is empty"},
		{"empty body", jsonMediaType, "", http.StatusBadRequest,
			types.ErrorCodeV1TracesInvalidRequest, "the request body is empty"},
		{"malformed JSON", jsonMediaType, `{"query":`, http.StatusBadRequest,
			types.ErrorCodeV1TracesInvalidRequest, "The request body must be a single valid JSON value"},
		{"two JSON values", jsonMediaType, `{"size":0} {"query":{"match_all":{}}}`, http.StatusBadRequest,
			types.ErrorCodeV1TracesInvalidRequest, "The request body must be a single valid JSON value"},
		{"text that isn't UTF-8", "text/plain", "SELECT \xff", http.StatusBadRequest,
			types.ErrorCodeV1TracesInvalidRequest, "The request body is not valid UTF-8"},
		{"a charset other than UTF-8", "text/plain; charset=utf-16", "SELECT 1", http.StatusUnsupportedMediaType,
			types.ErrorCodeV1TracesRawQueryMediaType, "Send the request body as UTF-8"},
		{"unsupported media type", "application/x-www-form-urlencoded", "q=1", http.StatusUnsupportedMediaType,
			types.ErrorCodeV1TracesRawQueryMediaType,
			"Send the tracing backend's native request as application/json or text/plain"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := &Handler{baseHandler: baseHandler{logger: noopLogger()}, tracesService: servicemocks.NewMockTracesQuerier(t)}
			rr := serve(t, h, rawQueryRequest(tt.contentType, tt.body))

			require.Equal(t, tt.status, rr.Code, rr.Body.String())
			if tt.errorCode != "" {
				body := decodeRawError(t, rr)
				assert.Equal(t, tt.errorCode, body.ErrorCode)
				assert.Equal(t, tt.message, body.Message)
			}
		})
	}
}

func TestQueryRawTraces_RejectsAnOversizedRequest(t *testing.T) {
	t.Parallel()

	var got string
	next := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		b, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		got = string(b)
	})
	guard := rawTracesQueryGuard(noopLogger(), 16)(next)

	rr := httptest.NewRecorder()
	guard.ServeHTTP(rr, rawQueryRequest("text/plain", strings.Repeat("x", 16)))
	assert.Equal(t, strings.Repeat("x", 16), got, "a body at the limit must reach the handler intact")

	rr = httptest.NewRecorder()
	guard.ServeHTTP(rr, rawQueryRequest("text/plain", strings.Repeat("x", 17)))
	require.Equal(t, http.StatusRequestEntityTooLarge, rr.Code, rr.Body.String())
	assert.Equal(t, types.ErrorCodeV1TracesRawQueryRequestTooLarge, decodeRawError(t, rr).ErrorCode)
}

func TestQueryRawTraces_AcceptsTheLargestRequestLimit(t *testing.T) {
	t.Parallel()

	var got string
	next := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		b, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		got = string(b)
	})
	rr := httptest.NewRecorder()
	rawTracesQueryGuard(noopLogger(), math.MaxInt64)(next).ServeHTTP(rr, rawQueryRequest("text/plain", "SELECT 1"))

	assert.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
	assert.Equal(t, "SELECT 1", got)
}

func TestQueryRawTraces_ServiceNotInitialized(t *testing.T) {
	t.Parallel()

	h := &Handler{baseHandler: baseHandler{logger: noopLogger()}}
	rr := serve(t, h, rawQueryRequest("text/plain", "SELECT 1"))

	require.Equal(t, http.StatusInternalServerError, rr.Code)
	assert.Equal(t, types.ErrorCodeV1TracesServiceNotReady, decodeRawError(t, rr).ErrorCode)
}

func TestQueryRawTraces_MapsServiceErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		err       error
		status    int
		title     string
		errorCode string
		message   string
	}{
		{"forbidden", observerAuthz.ErrAuthzForbidden, http.StatusForbidden, "forbidden", "", "Access denied"},
		{"unauthorized", observerAuthz.ErrAuthzUnauthorized, http.StatusUnauthorized, "unauthorized", "", "Unauthorized"},
		{"invalid request",
			fmt.Errorf("%w: a request body is required", service.ErrRawTracesInvalidRequest),
			http.StatusBadRequest, "badRequest", types.ErrorCodeV1TracesInvalidRequest, "a request body is required"},
		{"rejected by the backend",
			&service.RawTracesRejectedError{Detail: "invalid OpenObserve SQL: syntax error near FROMM"},
			http.StatusBadRequest, "badRequest", types.ErrorCodeV1TracesRawQueryRejected,
			"invalid OpenObserve SQL: syntax error near FROMM"},
		{"result too large", service.ErrRawTracesResultTooLarge, http.StatusBadRequest, "badRequest",
			types.ErrorCodeV1TracesRawQueryResultTooLarge,
			"The result is larger than the maximum response size; narrow the query or add a limit"},
		{"request too large for the backend", service.ErrRawTracesRequestTooLarge, http.StatusRequestEntityTooLarge,
			"payloadTooLarge", types.ErrorCodeV1TracesRawQueryRequestTooLarge,
			"The request is larger than the tracing backend accepts"},
		{"media type the backend doesn't take", service.ErrRawTracesUnsupportedMediaType,
			http.StatusUnsupportedMediaType, "unsupportedMediaType", types.ErrorCodeV1TracesRawQueryMediaType,
			"The tracing backend does not accept raw queries in this media type"},
		{"not supported", service.ErrRawTracesNotSupported, http.StatusNotImplemented, "notImplemented",
			types.ErrorCodeV1TracesRawQueryNotSupported, "The tracing backend does not support raw queries"},
		{"timeout", service.ErrRawTracesTimeout, http.StatusGatewayTimeout, "gatewayTimeout",
			types.ErrorCodeV1TracesRawQueryTimeout, "The raw query ran past the query timeout"},
		{"retrieval failure", fmt.Errorf("%w: boom", service.ErrTracesRetrieval),
			http.StatusInternalServerError, "internalServerError", types.ErrorCodeV1TracesRetrievalFailed,
			"Failed to run the raw trace query"},
		{"unexpected failure", errors.New("boom"), http.StatusInternalServerError, "internalServerError",
			types.ErrorCodeV1TracesInternalGeneric, "Failed to run the raw trace query"},
		{"canceled by the caller", fmt.Errorf("failed to execute request: %w", context.Canceled),
			http.StatusInternalServerError, "internalServerError",
			types.ErrorCodeV1TracesInternalGeneric, "The raw trace query was canceled"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			svc := servicemocks.NewMockTracesQuerier(t)
			svc.On("QueryRawTraces", mock.Anything, mock.Anything).Return(nil, tt.err)
			h := &Handler{baseHandler: baseHandler{logger: noopLogger()}, tracesService: svc}

			rr := serve(t, h, rawQueryRequest("text/plain", "SELECT 1"))

			require.Equal(t, tt.status, rr.Code, rr.Body.String())
			body := decodeRawError(t, rr)
			assert.Equal(t, tt.title, body.Title)
			assert.Equal(t, tt.errorCode, body.ErrorCode)
			assert.Equal(t, tt.message, body.Message)
		})
	}
}

func TestQueryRawTraces_AnswersALimitedQueryWith429(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		limited    *querylimit.LimitedError
		errorCode  string
		message    string
		retryAfter string
	}{
		{"too many running",
			&querylimit.LimitedError{Reason: querylimit.ReasonConcurrency, RetryAfter: time.Second},
			types.ErrorCodeV1TracesRawQueryConcurrencyLimited,
			"Too many raw trace queries are running; retry later", "1"},
		{"over the caller's rate",
			&querylimit.LimitedError{Reason: querylimit.ReasonRate, RetryAfter: 6 * time.Second},
			types.ErrorCodeV1TracesRawQueryRateLimited,
			"Too many raw trace queries from this caller; retry later", "6"},
		{"retry after rounded up to whole seconds",
			&querylimit.LimitedError{Reason: querylimit.ReasonRate, RetryAfter: 1500 * time.Millisecond},
			types.ErrorCodeV1TracesRawQueryRateLimited,
			"Too many raw trace queries from this caller; retry later", "2"},
		{"retry after at least one second",
			&querylimit.LimitedError{Reason: querylimit.ReasonRate},
			types.ErrorCodeV1TracesRawQueryRateLimited,
			"Too many raw trace queries from this caller; retry later", "1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			svc := servicemocks.NewMockTracesQuerier(t)
			svc.On("QueryRawTraces", mock.Anything, mock.Anything).
				Return(nil, fmt.Errorf("%w: %w", service.ErrRawTracesLimited, tt.limited))
			h := &Handler{baseHandler: baseHandler{logger: noopLogger()}, tracesService: svc}

			rr := serve(t, h, rawQueryRequest("application/json", `{"size":0}`))

			require.Equal(t, http.StatusTooManyRequests, rr.Code, rr.Body.String())
			assert.Equal(t, tt.retryAfter, rr.Header().Get("Retry-After"))
			assert.Equal(t, "application/json", rr.Header().Get("Content-Type"))
			body := decodeRawError(t, rr)
			assert.Equal(t, "tooManyRequests", body.Title)
			assert.Equal(t, tt.errorCode, body.ErrorCode)
			assert.Equal(t, tt.message, body.Message)
		})
	}
}

func TestRequireJSONContentType_LeavesRawQueriesToTheGuard(t *testing.T) {
	t.Parallel()

	reached := false
	next := http.HandlerFunc(func(http.ResponseWriter, *http.Request) { reached = true })
	rr := httptest.NewRecorder()
	RequireJSONContentType(noopLogger())(next).ServeHTTP(rr, rawQueryRequest("text/plain", "SELECT 1"))
	assert.True(t, reached, "a text raw query must reach the raw query guard")

	reached = false
	other := httptest.NewRequest(http.MethodPost, "/api/v1alpha1/traces/query", strings.NewReader("x"))
	other.Header.Set("Content-Type", "text/plain")
	rr = httptest.NewRecorder()
	RequireJSONContentType(noopLogger())(next).ServeHTTP(rr, other)
	assert.False(t, reached, "other routes still require JSON")
	assert.Equal(t, http.StatusBadRequest, rr.Code)
}
