// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/openchoreo/openchoreo/internal/observer/api/gen"
	"github.com/openchoreo/openchoreo/internal/observer/api/internalgen"
	"github.com/openchoreo/openchoreo/internal/observer/service"
	servicemocks "github.com/openchoreo/openchoreo/internal/observer/service/mocks"
)

// helpers -----------------------------------------------------------------------

const (
	testRuleName = "test-rule"
	testNS       = "test-ns"

	webhookURL = "/api/v1alpha1/alerts/webhook"
)

// validAlertRuleBody returns a minimal valid log-based AlertRuleRequest as a JSON io.Reader.
func validAlertRuleBody(t *testing.T) io.Reader {
	t.Helper()
	uid := "00000000-0000-0000-0000-000000000001"
	query := "ERROR"
	raw := map[string]any{
		"metadata": map[string]any{
			"name":           testRuleName,
			"componentUid":   uid,
			"projectUid":     uid,
			"environmentUid": uid,
			"namespace":      testNS,
		},
		"source": map[string]any{
			"type":  sourceTypeLog,
			"query": query,
		},
		"condition": map[string]any{
			"window":    "5m",
			"interval":  "1m",
			"operator":  "gt",
			"threshold": 1.0,
			"enabled":   true,
		},
	}
	b, err := json.Marshal(raw)
	require.NoError(t, err)
	return bytes.NewReader(b)
}

func newInternalHandler(svc service.AlertRuleService) *InternalHandler {
	return &InternalHandler{
		baseHandler:  baseHandler{logger: noopLogger()},
		alertService: svc,
	}
}

// newInternalServer builds the internal API exactly as cmd/observer does: the
// generated router from observer-internal-api.yaml, the composed middleware
// chain, and both explicit error handlers.
//
// Driving this rather than calling handler methods directly means routing,
// path-parameter binding, body decoding and the handler are all exercised
// together — so a spec change that breaks a route fails a test here instead of
// only in production. It is also what keeps the error-shape guarantees honest:
// the plain-text generated defaults would show up in these assertions.
func newInternalServer(t *testing.T, svc service.AlertRuleService) http.Handler {
	t.Helper()

	logger := noopLogger()
	strict := internalgen.NewStrictHandlerWithOptions(
		newInternalHandler(svc),
		nil,
		internalgen.StrictHTTPServerOptions{
			RequestErrorHandlerFunc:  StrictRequestErrorHandler(logger),
			ResponseErrorHandlerFunc: StrictResponseErrorHandler(logger),
		},
	)

	return internalgen.HandlerWithOptions(strict, internalgen.StdHTTPServerOptions{
		BaseRouter:       http.NewServeMux(),
		Middlewares:      InternalMiddlewares(InternalMiddlewareOptions{Logger: logger}),
		ErrorHandlerFunc: ParamBindingErrorHandler(logger),
	})
}

// do issues a request against the generated internal router.
func do(t *testing.T, svc service.AlertRuleService, method, url string, body io.Reader) *httptest.ResponseRecorder {
	t.Helper()
	rr := httptest.NewRecorder()
	newInternalServer(t, svc).ServeHTTP(rr, httptest.NewRequest(method, url, body))
	return rr
}

func ruleURL(sourceType, ruleName string) string {
	return "/api/v1alpha1/alerts/sources/" + sourceType + "/rules/" + ruleName
}

func rulesURL(sourceType string) string {
	return "/api/v1alpha1/alerts/sources/" + sourceType + "/rules"
}

// routing ------------------------------------------------------------------------

// TestInternalRouterServesOnlyItsOwnOperations pins that the internal port
// exposes the 5 internal operations and nothing else. Before the split, one spec
// described both ports; a single generated router would have exposed every
// public query endpoint here too.
func TestInternalRouterServesOnlyItsOwnOperations(t *testing.T) {
	t.Parallel()

	srv := newInternalServer(t, servicemocks.NewMockAlertRuleService(t))

	// Public operations must not be reachable on the internal port.
	for _, path := range []string{
		"/api/v1/logs/query",
		"/api/v1/events/query",
		"/api/v1/metrics/query",
		"/api/v1alpha1/traces/query",
		"/api/v1alpha1/incidents/query",
		"/health",
	} {
		rr := httptest.NewRecorder()
		srv.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, path, nil))
		assert.Equal(t, http.StatusNotFound, rr.Code,
			"%s must not be served on the internal port", path)
	}
}

func TestInternalRouterRejectsWrongMethod(t *testing.T) {
	t.Parallel()

	// PATCH is not declared on the alert-rule path, so the generated router
	// never reaches a handler.
	rr := do(t, servicemocks.NewMockAlertRuleService(t),
		http.MethodPatch, ruleURL(sourceTypeLog, "r1"), nil)

	assert.Equal(t, http.StatusMethodNotAllowed, rr.Code)
}

// CreateAlertRule tests ---------------------------------------------------------

func TestCreateAlertRule_InvalidSourceType(t *testing.T) {
	t.Parallel()

	rr := do(t, servicemocks.NewMockAlertRuleService(t),
		http.MethodPost, rulesURL("unknown"), validAlertRuleBody(t))

	require.Equal(t, http.StatusBadRequest, rr.Code)
	assert.Contains(t, rr.Body.String(), "INVALID_SOURCE_TYPE")
}

// TestCreateAlertRule_InvalidJSON also guards the error *shape*. The strict
// handler decodes the body now, so this 400 comes from
// StrictRequestErrorHandler rather than from the handler. Without that
// explicit hook the generated default would return plain text, silently
// breaking the contract every other observer error follows.
func TestCreateAlertRule_InvalidJSON(t *testing.T) {
	t.Parallel()

	rr := do(t, servicemocks.NewMockAlertRuleService(t),
		http.MethodPost, rulesURL(sourceTypeLog), strings.NewReader("{not-json"))

	require.Equal(t, http.StatusBadRequest, rr.Code)
	assert.Contains(t, rr.Body.String(), "INVALID_REQUEST_BODY")
	assertErrorResponseShape(t, rr)
}

func TestCreateAlertRule_ValidationError(t *testing.T) {
	t.Parallel()

	// Missing metadata.name → validation error.
	raw := map[string]any{
		"metadata":  map[string]any{"name": ""},
		"source":    map[string]any{"type": "log"},
		"condition": map[string]any{},
	}
	b, err := json.Marshal(raw)
	require.NoError(t, err)

	rr := do(t, servicemocks.NewMockAlertRuleService(t),
		http.MethodPost, rulesURL(sourceTypeLog), bytes.NewReader(b))

	require.Equal(t, http.StatusBadRequest, rr.Code)
	assert.Contains(t, rr.Body.String(), "VALIDATION_ERROR")
}

func TestCreateAlertRule_SourceTypeMismatch(t *testing.T) {
	t.Parallel()

	// Path says "metric" but body says "log".
	rr := do(t, servicemocks.NewMockAlertRuleService(t),
		http.MethodPost, rulesURL("metric"), validAlertRuleBody(t))

	require.Equal(t, http.StatusBadRequest, rr.Code)
	assert.Contains(t, rr.Body.String(), "SOURCE_TYPE_MISMATCH")
}

func TestCreateAlertRule_AlreadyExists(t *testing.T) {
	t.Parallel()

	svc := servicemocks.NewMockAlertRuleService(t)
	svc.On("CreateAlertRule", mock.Anything, mock.Anything).Return(nil, service.ErrAlertRuleAlreadyExists)

	rr := do(t, svc, http.MethodPost, rulesURL(sourceTypeLog), validAlertRuleBody(t))

	require.Equal(t, http.StatusConflict, rr.Code)
	assert.Contains(t, rr.Body.String(), "ALREADY_EXISTS")
}

func TestCreateAlertRule_ServiceError(t *testing.T) {
	t.Parallel()

	svc := servicemocks.NewMockAlertRuleService(t)
	svc.On("CreateAlertRule", mock.Anything, mock.Anything).Return(nil, errors.New("backend failure"))

	rr := do(t, svc, http.MethodPost, rulesURL(sourceTypeLog), validAlertRuleBody(t))

	require.Equal(t, http.StatusInternalServerError, rr.Code)
	assert.Contains(t, rr.Body.String(), "CREATE_FAILED")
}

func TestCreateAlertRule_Success(t *testing.T) {
	t.Parallel()

	action := gen.AlertingRuleSyncResponseAction("created")
	svc := servicemocks.NewMockAlertRuleService(t)
	svc.On("CreateAlertRule", mock.Anything, mock.Anything).Return(&gen.AlertingRuleSyncResponse{Action: &action}, nil)

	rr := do(t, svc, http.MethodPost, rulesURL(sourceTypeLog), validAlertRuleBody(t))

	require.Equal(t, http.StatusCreated, rr.Code)
	assert.Contains(t, rr.Body.String(), "created")
}

// GetAlertRule tests ------------------------------------------------------------

func TestGetAlertRule_InvalidSourceType(t *testing.T) {
	t.Parallel()

	rr := do(t, servicemocks.NewMockAlertRuleService(t),
		http.MethodGet, ruleURL("bad", "r1"), nil)

	require.Equal(t, http.StatusBadRequest, rr.Code)
	assert.Contains(t, rr.Body.String(), "INVALID_SOURCE_TYPE")
}

// TestGetAlertRule_EmptyRuleName records a deliberate behavior change.
//
// Before the migration the route was one hand-registered pattern and the
// handler validated ruleName itself, returning 400 INVALID_RULE_NAME for an
// empty value. Under the generated router, `{ruleName}` matches only a
// non-empty path segment, so a trailing-slash request never reaches the handler
// and the mux returns 404.
//
// 404 is the better answer — there is no such resource — and the handler keeps
// its defensive check for callers that invoke it directly. Recorded here so the
// change is visible rather than discovered.
func TestGetAlertRule_EmptyRuleName(t *testing.T) {
	t.Parallel()

	rr := do(t, servicemocks.NewMockAlertRuleService(t),
		http.MethodGet, "/api/v1alpha1/alerts/sources/log/rules/", nil)

	assert.Equal(t, http.StatusNotFound, rr.Code)
}

func TestGetAlertRule_NotFound(t *testing.T) {
	t.Parallel()

	svc := servicemocks.NewMockAlertRuleService(t)
	svc.On("GetAlertRule", mock.Anything, mock.Anything, mock.Anything).Return(nil, service.ErrAlertRuleNotFound)

	rr := do(t, svc, http.MethodGet, ruleURL(sourceTypeLog, "r1"), nil)

	require.Equal(t, http.StatusNotFound, rr.Code)
	assert.Contains(t, rr.Body.String(), "NOT_FOUND")
}

func TestGetAlertRule_ServiceError(t *testing.T) {
	t.Parallel()

	svc := servicemocks.NewMockAlertRuleService(t)
	svc.On("GetAlertRule", mock.Anything, mock.Anything, mock.Anything).Return(nil, errors.New("db error"))

	rr := do(t, svc, http.MethodGet, ruleURL(sourceTypeLog, "r1"), nil)

	require.Equal(t, http.StatusInternalServerError, rr.Code)
	assert.Contains(t, rr.Body.String(), "GET_FAILED")
}

func TestGetAlertRule_Success(t *testing.T) {
	t.Parallel()

	svc := servicemocks.NewMockAlertRuleService(t)
	svc.On("GetAlertRule", mock.Anything, mock.Anything, mock.Anything).Return(&gen.AlertRuleResponse{}, nil)

	rr := do(t, svc, http.MethodGet, ruleURL(sourceTypeLog, "r1"), nil)

	assert.Equal(t, http.StatusOK, rr.Code)
}

// TestGetAlertRule_PassesRuleNameFromPath proves ruleName binds from the path
// and reaches the service unchanged.
func TestGetAlertRule_PassesRuleNameFromPath(t *testing.T) {
	t.Parallel()

	svc := servicemocks.NewMockAlertRuleService(t)
	svc.On("GetAlertRule", mock.Anything, "my-rule", sourceTypeLog).
		Return(&gen.AlertRuleResponse{}, nil)

	rr := do(t, svc, http.MethodGet, ruleURL(sourceTypeLog, "my-rule"), nil)

	require.Equal(t, http.StatusOK, rr.Code)
	svc.AssertExpectations(t)
}

// UpdateAlertRule tests ---------------------------------------------------------

func TestUpdateAlertRule_InvalidSourceType(t *testing.T) {
	t.Parallel()

	rr := do(t, servicemocks.NewMockAlertRuleService(t),
		http.MethodPut, ruleURL("bad", "r1"), validAlertRuleBody(t))

	require.Equal(t, http.StatusBadRequest, rr.Code)
	assert.Contains(t, rr.Body.String(), "INVALID_SOURCE_TYPE")
}

// TestUpdateAlertRule_EmptyRuleName — see TestGetAlertRule_EmptyRuleName for why
// this is now a routing 404 rather than a handler 400.
func TestUpdateAlertRule_EmptyRuleName(t *testing.T) {
	t.Parallel()

	rr := do(t, servicemocks.NewMockAlertRuleService(t),
		http.MethodPut, "/api/v1alpha1/alerts/sources/log/rules/", validAlertRuleBody(t))

	assert.Equal(t, http.StatusNotFound, rr.Code)
}

func TestUpdateAlertRule_SourceTypeMismatch(t *testing.T) {
	t.Parallel()

	// Path says "metric", body has "log".
	rr := do(t, servicemocks.NewMockAlertRuleService(t),
		http.MethodPut, ruleURL("metric", "r1"), validAlertRuleBody(t))

	require.Equal(t, http.StatusBadRequest, rr.Code)
	assert.Contains(t, rr.Body.String(), "SOURCE_TYPE_MISMATCH")
}

// TestUpdateAlertRule_NotFound covers the 404 the pre-split spec never declared.
// The handler has always returned it; observer-internal-api.yaml now documents
// it, which is what lets the strict interface express it at all.
func TestUpdateAlertRule_NotFound(t *testing.T) {
	t.Parallel()

	svc := servicemocks.NewMockAlertRuleService(t)
	svc.On("UpdateAlertRule", mock.Anything, mock.Anything, mock.Anything).Return(nil, service.ErrAlertRuleNotFound)

	rr := do(t, svc, http.MethodPut, ruleURL(sourceTypeLog, "r1"), validAlertRuleBody(t))

	require.Equal(t, http.StatusNotFound, rr.Code)
	assert.Contains(t, rr.Body.String(), "NOT_FOUND")
}

func TestUpdateAlertRule_ServiceError(t *testing.T) {
	t.Parallel()

	svc := servicemocks.NewMockAlertRuleService(t)
	svc.On("UpdateAlertRule", mock.Anything, mock.Anything, mock.Anything).Return(nil, errors.New("backend error"))

	rr := do(t, svc, http.MethodPut, ruleURL(sourceTypeLog, "r1"), validAlertRuleBody(t))

	require.Equal(t, http.StatusInternalServerError, rr.Code)
	assert.Contains(t, rr.Body.String(), "UPDATE_FAILED")
}

func TestUpdateAlertRule_Success(t *testing.T) {
	t.Parallel()

	action := gen.AlertingRuleSyncResponseAction("updated")
	svc := servicemocks.NewMockAlertRuleService(t)
	svc.On("UpdateAlertRule", mock.Anything, mock.Anything, mock.Anything).Return(&gen.AlertingRuleSyncResponse{Action: &action}, nil)

	rr := do(t, svc, http.MethodPut, ruleURL(sourceTypeLog, "r1"), validAlertRuleBody(t))

	require.Equal(t, http.StatusOK, rr.Code)
	assert.Contains(t, rr.Body.String(), "updated")
}

// DeleteAlertRule tests ---------------------------------------------------------

func TestDeleteAlertRule_InvalidSourceType(t *testing.T) {
	t.Parallel()

	rr := do(t, servicemocks.NewMockAlertRuleService(t),
		http.MethodDelete, ruleURL("bad", "r1"), nil)

	require.Equal(t, http.StatusBadRequest, rr.Code)
	assert.Contains(t, rr.Body.String(), "INVALID_SOURCE_TYPE")
}

// TestDeleteAlertRule_EmptyRuleName — see TestGetAlertRule_EmptyRuleName.
func TestDeleteAlertRule_EmptyRuleName(t *testing.T) {
	t.Parallel()

	rr := do(t, servicemocks.NewMockAlertRuleService(t),
		http.MethodDelete, "/api/v1alpha1/alerts/sources/log/rules/", nil)

	assert.Equal(t, http.StatusNotFound, rr.Code)
}

func TestDeleteAlertRule_NotFound(t *testing.T) {
	t.Parallel()

	svc := servicemocks.NewMockAlertRuleService(t)
	svc.On("DeleteAlertRule", mock.Anything, mock.Anything, mock.Anything).Return(nil, service.ErrAlertRuleNotFound)

	rr := do(t, svc, http.MethodDelete, ruleURL(sourceTypeLog, "r1"), nil)

	require.Equal(t, http.StatusNotFound, rr.Code)
	assert.Contains(t, rr.Body.String(), "NOT_FOUND")
}

func TestDeleteAlertRule_ServiceError(t *testing.T) {
	t.Parallel()

	svc := servicemocks.NewMockAlertRuleService(t)
	svc.On("DeleteAlertRule", mock.Anything, mock.Anything, mock.Anything).Return(nil, errors.New("backend error"))

	rr := do(t, svc, http.MethodDelete, ruleURL(sourceTypeLog, "r1"), nil)

	require.Equal(t, http.StatusInternalServerError, rr.Code)
	assert.Contains(t, rr.Body.String(), "DELETE_FAILED")
}

func TestDeleteAlertRule_Success(t *testing.T) {
	t.Parallel()

	action := gen.AlertingRuleSyncResponseAction("deleted")
	svc := servicemocks.NewMockAlertRuleService(t)
	svc.On("DeleteAlertRule", mock.Anything, mock.Anything, mock.Anything).Return(&gen.AlertingRuleSyncResponse{Action: &action}, nil)

	rr := do(t, svc, http.MethodDelete, ruleURL(sourceTypeLog, "r1"), nil)

	require.Equal(t, http.StatusOK, rr.Code)
	assert.Contains(t, rr.Body.String(), "deleted")
}

// HandleAlertWebhook tests -------------------------------------------------------

func TestHandleAlertWebhook_InvalidJSON(t *testing.T) {
	t.Parallel()

	rr := do(t, servicemocks.NewMockAlertRuleService(t),
		http.MethodPost, webhookURL, strings.NewReader("{bad"))

	require.Equal(t, http.StatusBadRequest, rr.Code)
	assert.Contains(t, rr.Body.String(), "INVALID_REQUEST_BODY")
	assertErrorResponseShape(t, rr)
}

func TestHandleAlertWebhook_MissingRuleName(t *testing.T) {
	t.Parallel()

	ns := testNS
	b, err := json.Marshal(gen.AlertWebhookRequest{RuleNamespace: &ns})
	require.NoError(t, err)

	rr := do(t, servicemocks.NewMockAlertRuleService(t),
		http.MethodPost, webhookURL, bytes.NewReader(b))

	require.Equal(t, http.StatusBadRequest, rr.Code)
	assert.Contains(t, rr.Body.String(), "MISSING_RULE_NAME")
}

func TestHandleAlertWebhook_MissingRuleNamespace(t *testing.T) {
	t.Parallel()

	name := testRuleName
	b, err := json.Marshal(gen.AlertWebhookRequest{RuleName: &name})
	require.NoError(t, err)

	rr := do(t, servicemocks.NewMockAlertRuleService(t),
		http.MethodPost, webhookURL, bytes.NewReader(b))

	require.Equal(t, http.StatusBadRequest, rr.Code)
	assert.Contains(t, rr.Body.String(), "MISSING_RULE_NAMESPACE")
}

func TestHandleAlertWebhook_ServiceError(t *testing.T) {
	t.Parallel()

	svc := servicemocks.NewMockAlertRuleService(t)
	svc.On("HandleAlertWebhook", mock.Anything, mock.Anything).Return(nil, errors.New("processing failed"))

	name, ns := testRuleName, testNS
	b, err := json.Marshal(gen.AlertWebhookRequest{RuleName: &name, RuleNamespace: &ns})
	require.NoError(t, err)

	rr := do(t, svc, http.MethodPost, webhookURL, bytes.NewReader(b))

	require.Equal(t, http.StatusInternalServerError, rr.Code)
	assert.Contains(t, rr.Body.String(), "WEBHOOK_FAILED")
}

func TestHandleAlertWebhook_Success(t *testing.T) {
	t.Parallel()

	msg := "processed"
	status := gen.AlertWebhookResponseStatus("ok")
	svc := servicemocks.NewMockAlertRuleService(t)
	svc.On("HandleAlertWebhook", mock.Anything, mock.Anything).
		Return(&gen.AlertWebhookResponse{Message: &msg, Status: &status}, nil)

	name, ns := testRuleName, testNS
	b, err := json.Marshal(gen.AlertWebhookRequest{RuleName: &name, RuleNamespace: &ns})
	require.NoError(t, err)

	rr := do(t, svc, http.MethodPost, webhookURL, bytes.NewReader(b))

	require.Equal(t, http.StatusOK, rr.Code)
	assert.Contains(t, rr.Body.String(), "processed")
}

// Budget source type tests --------------------------------------------------

func TestCreateAlertRule_Budget_Success(t *testing.T) {
	t.Parallel()

	action := gen.AlertingRuleSyncResponseAction("created")
	svc := servicemocks.NewMockAlertRuleService(t)
	svc.On("CreateAlertRule", mock.Anything, mock.Anything).Return(&gen.AlertingRuleSyncResponse{Action: &action}, nil)

	// Budget alert: no query or metric needed, just threshold and window.
	uid := "00000000-0000-0000-0000-000000000001"
	raw := map[string]any{
		"metadata": map[string]any{
			"name":           testRuleName,
			"componentUid":   uid,
			"projectUid":     uid,
			"environmentUid": uid,
			"namespace":      testNS,
		},
		"source": map[string]any{
			"type": sourceTypeBudget,
		},
		"condition": map[string]any{
			"window":    "24h",
			"interval":  "1h",
			"operator":  "gt",
			"threshold": 5.0,
			"enabled":   true,
		},
	}
	b, err := json.Marshal(raw)
	require.NoError(t, err)

	rr := do(t, svc, http.MethodPost, rulesURL(sourceTypeBudget), bytes.NewReader(b))

	require.Equal(t, http.StatusCreated, rr.Code)
	assert.Contains(t, rr.Body.String(), "created")
}

func TestGetAlertRule_Budget_Success(t *testing.T) {
	t.Parallel()

	svc := servicemocks.NewMockAlertRuleService(t)
	svc.On("GetAlertRule", mock.Anything, mock.Anything, mock.Anything).Return(&gen.AlertRuleResponse{}, nil)

	rr := do(t, svc, http.MethodGet, ruleURL(sourceTypeBudget, "r1"), nil)

	assert.Equal(t, http.StatusOK, rr.Code)
}

// nil-response guard -------------------------------------------------------------

// TestInternalHandlers_NilServiceResponse covers the errNilServiceResponse
// branch in all five operations.
//
// A service returning (nil, nil) violates the AlertRuleService contract, so this
// is unreachable in practice — but the strict response types are value types, so
// without the guard each of these would panic on a nil dereference. Five
// branches deserve one test; before this, all five were uncovered.
func TestInternalHandlers_NilServiceResponse(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		mockCall  string
		method    string
		url       string
		body      func(*testing.T) io.Reader
		errorCode string
	}{
		{
			name:      "CreateAlertRule",
			mockCall:  "CreateAlertRule",
			method:    http.MethodPost,
			url:       rulesURL(sourceTypeLog),
			body:      validAlertRuleBody,
			errorCode: "CREATE_FAILED",
		},
		{
			name:      "GetAlertRule",
			mockCall:  "GetAlertRule",
			method:    http.MethodGet,
			url:       ruleURL(sourceTypeLog, "r1"),
			errorCode: "GET_FAILED",
		},
		{
			name:      "UpdateAlertRule",
			mockCall:  "UpdateAlertRule",
			method:    http.MethodPut,
			url:       ruleURL(sourceTypeLog, "r1"),
			body:      validAlertRuleBody,
			errorCode: "UPDATE_FAILED",
		},
		{
			name:      "DeleteAlertRule",
			mockCall:  "DeleteAlertRule",
			method:    http.MethodDelete,
			url:       ruleURL(sourceTypeLog, "r1"),
			errorCode: "DELETE_FAILED",
		},
		{
			name:     "HandleAlertWebhook",
			mockCall: "HandleAlertWebhook",
			method:   http.MethodPost,
			url:      webhookURL,
			body: func(t *testing.T) io.Reader {
				t.Helper()
				name, ns := testRuleName, testNS
				b, err := json.Marshal(gen.AlertWebhookRequest{RuleName: &name, RuleNamespace: &ns})
				require.NoError(t, err)
				return bytes.NewReader(b)
			},
			errorCode: "WEBHOOK_FAILED",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			svc := servicemocks.NewMockAlertRuleService(t)
			// Return (nil, nil): no response, no error.
			svc.On(tc.mockCall, mock.Anything, mock.Anything, mock.Anything).
				Maybe().Return(nil, nil)
			svc.On(tc.mockCall, mock.Anything, mock.Anything).
				Maybe().Return(nil, nil)

			var body io.Reader
			if tc.body != nil {
				body = tc.body(t)
			}

			rr := do(t, svc, tc.method, tc.url, body)

			require.Equal(t, http.StatusInternalServerError, rr.Code,
				"a nil service response must be a 500, not a panic")
			assert.Contains(t, rr.Body.String(), tc.errorCode)
			assertErrorResponseShape(t, rr)
		})
	}
}

// error shape --------------------------------------------------------------------

// assertErrorResponseShape checks a response body is the gen.ErrorResponse JSON
// object rather than plain text. This is what distinguishes a properly wired
// error handler from the generated default, and no test asserted it before the
// migration — which is exactly why the regression would have shipped silently.
func assertErrorResponseShape(t *testing.T, rr *httptest.ResponseRecorder) {
	t.Helper()

	assert.Equal(t, "application/json", rr.Header().Get("Content-Type"))

	var payload gen.ErrorResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &payload),
		"error body must be gen.ErrorResponse JSON, got: %s", rr.Body.String())
	require.NotNil(t, payload.ErrorCode, "errorCode must be set")
	require.NotNil(t, payload.Message, "message must be set")
	require.NotNil(t, payload.Title, "title must be set")
}
