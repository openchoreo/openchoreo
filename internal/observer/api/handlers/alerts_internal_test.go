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
	"github.com/openchoreo/openchoreo/internal/observer/service"
	servicemocks "github.com/openchoreo/openchoreo/internal/observer/service/mocks"
)

// helpers -----------------------------------------------------------------------

const (
	testRuleName = "test-rule"
	testNS       = "test-ns"
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

// CreateAlertRule tests ---------------------------------------------------------

func TestCreateAlertRule_InvalidSourceType(t *testing.T) {
	t.Parallel()

	h := newInternalHandler(servicemocks.NewMockAlertRuleService(t))
	req := httptest.NewRequest(http.MethodPost, "/api/v1alpha1/alerts/sources/unknown/rules",
		validAlertRuleBody(t))
	req.SetPathValue("sourceType", "unknown")
	rr := httptest.NewRecorder()

	h.CreateAlertRule(rr, req)

	require.Equal(t, http.StatusBadRequest, rr.Code)
	assert.Contains(t, rr.Body.String(), "INVALID_SOURCE_TYPE")
}

func TestCreateAlertRule_InvalidJSON(t *testing.T) {
	t.Parallel()

	h := newInternalHandler(servicemocks.NewMockAlertRuleService(t))
	req := httptest.NewRequest(http.MethodPost, "/api/v1alpha1/alerts/sources/log/rules",
		strings.NewReader("{not-json"))
	req.SetPathValue("sourceType", "log")
	rr := httptest.NewRecorder()

	h.CreateAlertRule(rr, req)

	require.Equal(t, http.StatusBadRequest, rr.Code)
	assert.Contains(t, rr.Body.String(), "INVALID_REQUEST_BODY")
}

func TestCreateAlertRule_ValidationError(t *testing.T) {
	t.Parallel()

	h := newInternalHandler(servicemocks.NewMockAlertRuleService(t))
	// Missing metadata.name → validation error.
	raw := map[string]any{
		"metadata": map[string]any{
			"name": "",
		},
		"source":    map[string]any{"type": "log"},
		"condition": map[string]any{},
	}
	b, _ := json.Marshal(raw)
	req := httptest.NewRequest(http.MethodPost, "/api/v1alpha1/alerts/sources/log/rules",
		bytes.NewReader(b))
	req.SetPathValue("sourceType", "log")
	rr := httptest.NewRecorder()

	h.CreateAlertRule(rr, req)

	require.Equal(t, http.StatusBadRequest, rr.Code)
	assert.Contains(t, rr.Body.String(), "VALIDATION_ERROR")
}

func TestCreateAlertRule_SourceTypeMismatch(t *testing.T) {
	t.Parallel()

	h := newInternalHandler(servicemocks.NewMockAlertRuleService(t))
	// Path says "metric" but body says "log".
	req := httptest.NewRequest(http.MethodPost, "/api/v1alpha1/alerts/sources/metric/rules",
		validAlertRuleBody(t))
	req.SetPathValue("sourceType", "metric")
	rr := httptest.NewRecorder()

	h.CreateAlertRule(rr, req)

	require.Equal(t, http.StatusBadRequest, rr.Code)
	assert.Contains(t, rr.Body.String(), "SOURCE_TYPE_MISMATCH")
}

func TestCreateAlertRule_AlreadyExists(t *testing.T) {
	t.Parallel()

	svc := servicemocks.NewMockAlertRuleService(t)
	svc.On("CreateAlertRule", mock.Anything, mock.Anything).Return(nil, service.ErrAlertRuleAlreadyExists)

	h := newInternalHandler(svc)
	req := httptest.NewRequest(http.MethodPost, "/api/v1alpha1/alerts/sources/log/rules",
		validAlertRuleBody(t))
	req.SetPathValue("sourceType", "log")
	rr := httptest.NewRecorder()

	h.CreateAlertRule(rr, req)

	require.Equal(t, http.StatusConflict, rr.Code)
	assert.Contains(t, rr.Body.String(), "ALREADY_EXISTS")
}

func TestCreateAlertRule_ServiceError(t *testing.T) {
	t.Parallel()

	svc := servicemocks.NewMockAlertRuleService(t)
	svc.On("CreateAlertRule", mock.Anything, mock.Anything).Return(nil, errors.New("backend failure"))

	h := newInternalHandler(svc)
	req := httptest.NewRequest(http.MethodPost, "/api/v1alpha1/alerts/sources/log/rules",
		validAlertRuleBody(t))
	req.SetPathValue("sourceType", "log")
	rr := httptest.NewRecorder()

	h.CreateAlertRule(rr, req)

	require.Equal(t, http.StatusInternalServerError, rr.Code)
	assert.Contains(t, rr.Body.String(), "CREATE_FAILED")
}

func TestCreateAlertRule_Success(t *testing.T) {
	t.Parallel()

	action := gen.AlertingRuleSyncResponseAction("created")
	svc := servicemocks.NewMockAlertRuleService(t)
	svc.On("CreateAlertRule", mock.Anything, mock.Anything).Return(&gen.AlertingRuleSyncResponse{Action: &action}, nil)

	h := newInternalHandler(svc)
	req := httptest.NewRequest(http.MethodPost, "/api/v1alpha1/alerts/sources/log/rules",
		validAlertRuleBody(t))
	req.SetPathValue("sourceType", "log")
	rr := httptest.NewRecorder()

	h.CreateAlertRule(rr, req)

	require.Equal(t, http.StatusCreated, rr.Code)
	assert.Contains(t, rr.Body.String(), "created")
}

// GetAlertRule tests ------------------------------------------------------------

func TestGetAlertRule_InvalidSourceType(t *testing.T) {
	t.Parallel()

	h := newInternalHandler(servicemocks.NewMockAlertRuleService(t))
	req := httptest.NewRequest(http.MethodGet, "/api/v1alpha1/alerts/sources/bad/rules/r1", nil)
	req.SetPathValue("sourceType", "bad")
	req.SetPathValue("ruleName", "r1")
	rr := httptest.NewRecorder()

	h.GetAlertRule(rr, req)

	require.Equal(t, http.StatusBadRequest, rr.Code)
	assert.Contains(t, rr.Body.String(), "INVALID_SOURCE_TYPE")
}

func TestGetAlertRule_EmptyRuleName(t *testing.T) {
	t.Parallel()

	h := newInternalHandler(servicemocks.NewMockAlertRuleService(t))
	req := httptest.NewRequest(http.MethodGet, "/api/v1alpha1/alerts/sources/log/rules/", nil)
	req.SetPathValue("sourceType", "log")
	req.SetPathValue("ruleName", "")
	rr := httptest.NewRecorder()

	h.GetAlertRule(rr, req)

	require.Equal(t, http.StatusBadRequest, rr.Code)
	assert.Contains(t, rr.Body.String(), "INVALID_RULE_NAME")
}

func TestGetAlertRule_NotFound(t *testing.T) {
	t.Parallel()

	svc := servicemocks.NewMockAlertRuleService(t)
	svc.On("GetAlertRule", mock.Anything, mock.Anything, mock.Anything).Return(nil, service.ErrAlertRuleNotFound)

	h := newInternalHandler(svc)
	req := httptest.NewRequest(http.MethodGet, "/api/v1alpha1/alerts/sources/log/rules/r1", nil)
	req.SetPathValue("sourceType", "log")
	req.SetPathValue("ruleName", "r1")
	rr := httptest.NewRecorder()

	h.GetAlertRule(rr, req)

	require.Equal(t, http.StatusNotFound, rr.Code)
	assert.Contains(t, rr.Body.String(), "NOT_FOUND")
}

func TestGetAlertRule_ServiceError(t *testing.T) {
	t.Parallel()

	svc := servicemocks.NewMockAlertRuleService(t)
	svc.On("GetAlertRule", mock.Anything, mock.Anything, mock.Anything).Return(nil, errors.New("db error"))

	h := newInternalHandler(svc)
	req := httptest.NewRequest(http.MethodGet, "/api/v1alpha1/alerts/sources/log/rules/r1", nil)
	req.SetPathValue("sourceType", "log")
	req.SetPathValue("ruleName", "r1")
	rr := httptest.NewRecorder()

	h.GetAlertRule(rr, req)

	require.Equal(t, http.StatusInternalServerError, rr.Code)
	assert.Contains(t, rr.Body.String(), "GET_FAILED")
}

func TestGetAlertRule_Success(t *testing.T) {
	t.Parallel()

	svc := servicemocks.NewMockAlertRuleService(t)
	svc.On("GetAlertRule", mock.Anything, mock.Anything, mock.Anything).Return(&gen.AlertRuleResponse{}, nil)

	h := newInternalHandler(svc)
	req := httptest.NewRequest(http.MethodGet, "/api/v1alpha1/alerts/sources/log/rules/r1", nil)
	req.SetPathValue("sourceType", "log")
	req.SetPathValue("ruleName", "r1")
	rr := httptest.NewRecorder()

	h.GetAlertRule(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
}

// UpdateAlertRule tests ---------------------------------------------------------

func TestUpdateAlertRule_InvalidSourceType(t *testing.T) {
	t.Parallel()

	h := newInternalHandler(servicemocks.NewMockAlertRuleService(t))
	req := httptest.NewRequest(http.MethodPut, "/api/v1alpha1/alerts/sources/bad/rules/r1",
		validAlertRuleBody(t))
	req.SetPathValue("sourceType", "bad")
	req.SetPathValue("ruleName", "r1")
	rr := httptest.NewRecorder()

	h.UpdateAlertRule(rr, req)

	require.Equal(t, http.StatusBadRequest, rr.Code)
	assert.Contains(t, rr.Body.String(), "INVALID_SOURCE_TYPE")
}

func TestUpdateAlertRule_EmptyRuleName(t *testing.T) {
	t.Parallel()

	h := newInternalHandler(servicemocks.NewMockAlertRuleService(t))
	req := httptest.NewRequest(http.MethodPut, "/api/v1alpha1/alerts/sources/log/rules/",
		validAlertRuleBody(t))
	req.SetPathValue("sourceType", "log")
	req.SetPathValue("ruleName", "")
	rr := httptest.NewRecorder()

	h.UpdateAlertRule(rr, req)

	require.Equal(t, http.StatusBadRequest, rr.Code)
	assert.Contains(t, rr.Body.String(), "INVALID_RULE_NAME")
}

func TestUpdateAlertRule_SourceTypeMismatch(t *testing.T) {
	t.Parallel()

	h := newInternalHandler(servicemocks.NewMockAlertRuleService(t))
	// Path says "metric", body has "log".
	req := httptest.NewRequest(http.MethodPut, "/api/v1alpha1/alerts/sources/metric/rules/r1",
		validAlertRuleBody(t))
	req.SetPathValue("sourceType", "metric")
	req.SetPathValue("ruleName", "r1")
	rr := httptest.NewRecorder()

	h.UpdateAlertRule(rr, req)

	require.Equal(t, http.StatusBadRequest, rr.Code)
	assert.Contains(t, rr.Body.String(), "SOURCE_TYPE_MISMATCH")
}

func TestUpdateAlertRule_NotFound(t *testing.T) {
	t.Parallel()

	svc := servicemocks.NewMockAlertRuleService(t)
	svc.On("UpdateAlertRule", mock.Anything, mock.Anything, mock.Anything).Return(nil, service.ErrAlertRuleNotFound)

	h := newInternalHandler(svc)
	req := httptest.NewRequest(http.MethodPut, "/api/v1alpha1/alerts/sources/log/rules/r1",
		validAlertRuleBody(t))
	req.SetPathValue("sourceType", "log")
	req.SetPathValue("ruleName", "r1")
	rr := httptest.NewRecorder()

	h.UpdateAlertRule(rr, req)

	require.Equal(t, http.StatusNotFound, rr.Code)
	assert.Contains(t, rr.Body.String(), "NOT_FOUND")
}

func TestUpdateAlertRule_ServiceError(t *testing.T) {
	t.Parallel()

	svc := servicemocks.NewMockAlertRuleService(t)
	svc.On("UpdateAlertRule", mock.Anything, mock.Anything, mock.Anything).Return(nil, errors.New("backend error"))

	h := newInternalHandler(svc)
	req := httptest.NewRequest(http.MethodPut, "/api/v1alpha1/alerts/sources/log/rules/r1",
		validAlertRuleBody(t))
	req.SetPathValue("sourceType", "log")
	req.SetPathValue("ruleName", "r1")
	rr := httptest.NewRecorder()

	h.UpdateAlertRule(rr, req)

	require.Equal(t, http.StatusInternalServerError, rr.Code)
	assert.Contains(t, rr.Body.String(), "UPDATE_FAILED")
}

func TestUpdateAlertRule_Success(t *testing.T) {
	t.Parallel()

	action := gen.AlertingRuleSyncResponseAction("updated")
	svc := servicemocks.NewMockAlertRuleService(t)
	svc.On("UpdateAlertRule", mock.Anything, mock.Anything, mock.Anything).Return(&gen.AlertingRuleSyncResponse{Action: &action}, nil)

	h := newInternalHandler(svc)
	req := httptest.NewRequest(http.MethodPut, "/api/v1alpha1/alerts/sources/log/rules/r1",
		validAlertRuleBody(t))
	req.SetPathValue("sourceType", "log")
	req.SetPathValue("ruleName", "r1")
	rr := httptest.NewRecorder()

	h.UpdateAlertRule(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	assert.Contains(t, rr.Body.String(), "updated")
}

// DeleteAlertRule tests ---------------------------------------------------------

func TestDeleteAlertRule_InvalidSourceType(t *testing.T) {
	t.Parallel()

	h := newInternalHandler(servicemocks.NewMockAlertRuleService(t))
	req := httptest.NewRequest(http.MethodDelete, "/api/v1alpha1/alerts/sources/bad/rules/r1", nil)
	req.SetPathValue("sourceType", "bad")
	req.SetPathValue("ruleName", "r1")
	rr := httptest.NewRecorder()

	h.DeleteAlertRule(rr, req)

	require.Equal(t, http.StatusBadRequest, rr.Code)
	assert.Contains(t, rr.Body.String(), "INVALID_SOURCE_TYPE")
}

func TestDeleteAlertRule_EmptyRuleName(t *testing.T) {
	t.Parallel()

	h := newInternalHandler(servicemocks.NewMockAlertRuleService(t))
	req := httptest.NewRequest(http.MethodDelete, "/api/v1alpha1/alerts/sources/log/rules/", nil)
	req.SetPathValue("sourceType", "log")
	req.SetPathValue("ruleName", "")
	rr := httptest.NewRecorder()

	h.DeleteAlertRule(rr, req)

	require.Equal(t, http.StatusBadRequest, rr.Code)
	assert.Contains(t, rr.Body.String(), "INVALID_RULE_NAME")
}

func TestDeleteAlertRule_NotFound(t *testing.T) {
	t.Parallel()

	svc := servicemocks.NewMockAlertRuleService(t)
	svc.On("DeleteAlertRule", mock.Anything, mock.Anything, mock.Anything).Return(nil, service.ErrAlertRuleNotFound)

	h := newInternalHandler(svc)
	req := httptest.NewRequest(http.MethodDelete, "/api/v1alpha1/alerts/sources/log/rules/r1", nil)
	req.SetPathValue("sourceType", "log")
	req.SetPathValue("ruleName", "r1")
	rr := httptest.NewRecorder()

	h.DeleteAlertRule(rr, req)

	require.Equal(t, http.StatusNotFound, rr.Code)
	assert.Contains(t, rr.Body.String(), "NOT_FOUND")
}

func TestDeleteAlertRule_ServiceError(t *testing.T) {
	t.Parallel()

	svc := servicemocks.NewMockAlertRuleService(t)
	svc.On("DeleteAlertRule", mock.Anything, mock.Anything, mock.Anything).Return(nil, errors.New("backend error"))

	h := newInternalHandler(svc)
	req := httptest.NewRequest(http.MethodDelete, "/api/v1alpha1/alerts/sources/log/rules/r1", nil)
	req.SetPathValue("sourceType", "log")
	req.SetPathValue("ruleName", "r1")
	rr := httptest.NewRecorder()

	h.DeleteAlertRule(rr, req)

	require.Equal(t, http.StatusInternalServerError, rr.Code)
	assert.Contains(t, rr.Body.String(), "DELETE_FAILED")
}

func TestDeleteAlertRule_Success(t *testing.T) {
	t.Parallel()

	action := gen.AlertingRuleSyncResponseAction("deleted")
	svc := servicemocks.NewMockAlertRuleService(t)
	svc.On("DeleteAlertRule", mock.Anything, mock.Anything, mock.Anything).Return(&gen.AlertingRuleSyncResponse{Action: &action}, nil)

	h := newInternalHandler(svc)
	req := httptest.NewRequest(http.MethodDelete, "/api/v1alpha1/alerts/sources/log/rules/r1", nil)
	req.SetPathValue("sourceType", "log")
	req.SetPathValue("ruleName", "r1")
	rr := httptest.NewRecorder()

	h.DeleteAlertRule(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	assert.Contains(t, rr.Body.String(), "deleted")
}

// HandleAlertWebhook tests -------------------------------------------------------

func TestHandleAlertWebhook_InvalidJSON(t *testing.T) {
	t.Parallel()

	h := newInternalHandler(servicemocks.NewMockAlertRuleService(t))
	req := httptest.NewRequest(http.MethodPost, "/api/v1alpha1/alerts/webhook",
		strings.NewReader("{bad"))
	rr := httptest.NewRecorder()

	h.HandleAlertWebhook(rr, req)

	require.Equal(t, http.StatusBadRequest, rr.Code)
	assert.Contains(t, rr.Body.String(), "INVALID_REQUEST_BODY")
}

func TestHandleAlertWebhook_MissingRuleName(t *testing.T) {
	t.Parallel()

	h := newInternalHandler(servicemocks.NewMockAlertRuleService(t))
	ns := testNS
	raw := gen.AlertWebhookRequest{RuleNamespace: &ns}
	b, _ := json.Marshal(raw)
	req := httptest.NewRequest(http.MethodPost, "/api/v1alpha1/alerts/webhook", bytes.NewReader(b))
	rr := httptest.NewRecorder()

	h.HandleAlertWebhook(rr, req)

	require.Equal(t, http.StatusBadRequest, rr.Code)
	assert.Contains(t, rr.Body.String(), "MISSING_RULE_NAME")
}

func TestHandleAlertWebhook_MissingRuleNamespace(t *testing.T) {
	t.Parallel()

	h := newInternalHandler(servicemocks.NewMockAlertRuleService(t))
	name := testRuleName
	raw := gen.AlertWebhookRequest{RuleName: &name}
	b, _ := json.Marshal(raw)
	req := httptest.NewRequest(http.MethodPost, "/api/v1alpha1/alerts/webhook", bytes.NewReader(b))
	rr := httptest.NewRecorder()

	h.HandleAlertWebhook(rr, req)

	require.Equal(t, http.StatusBadRequest, rr.Code)
	assert.Contains(t, rr.Body.String(), "MISSING_RULE_NAMESPACE")
}

func TestHandleAlertWebhook_ServiceError(t *testing.T) {
	t.Parallel()

	svc := servicemocks.NewMockAlertRuleService(t)
	svc.On("HandleAlertWebhook", mock.Anything, mock.Anything).Return(nil, errors.New("processing failed"))

	h := newInternalHandler(svc)
	name, ns := testRuleName, testNS
	raw := gen.AlertWebhookRequest{RuleName: &name, RuleNamespace: &ns}
	b, _ := json.Marshal(raw)
	req := httptest.NewRequest(http.MethodPost, "/api/v1alpha1/alerts/webhook", bytes.NewReader(b))
	rr := httptest.NewRecorder()

	h.HandleAlertWebhook(rr, req)

	require.Equal(t, http.StatusInternalServerError, rr.Code)
	assert.Contains(t, rr.Body.String(), "WEBHOOK_FAILED")
}

func TestHandleAlertWebhook_Success(t *testing.T) {
	t.Parallel()

	msg := "processed"
	status := gen.AlertWebhookResponseStatus("ok")
	svc := servicemocks.NewMockAlertRuleService(t)
	svc.On("HandleAlertWebhook", mock.Anything, mock.Anything).Return(&gen.AlertWebhookResponse{Message: &msg, Status: &status}, nil)

	h := newInternalHandler(svc)
	name, ns := testRuleName, testNS
	raw := gen.AlertWebhookRequest{RuleName: &name, RuleNamespace: &ns}
	b, _ := json.Marshal(raw)
	req := httptest.NewRequest(http.MethodPost, "/api/v1alpha1/alerts/webhook", bytes.NewReader(b))
	rr := httptest.NewRecorder()

	h.HandleAlertWebhook(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	assert.Contains(t, rr.Body.String(), "processed")
}

// Budget source type tests --------------------------------------------------

func TestCreateAlertRule_Budget_Success(t *testing.T) {
	t.Parallel()

	action := gen.AlertingRuleSyncResponseAction("created")
	svc := servicemocks.NewMockAlertRuleService(t)
	svc.On("CreateAlertRule", mock.Anything, mock.Anything).Return(&gen.AlertingRuleSyncResponse{Action: &action}, nil)

	h := newInternalHandler(svc)

	// Budget alert: no query or metric needed, just threshold and window
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

	req := httptest.NewRequest(http.MethodPost, "/api/v1alpha1/alerts/sources/budget/rules",
		bytes.NewReader(b))
	req.SetPathValue("sourceType", sourceTypeBudget)
	rr := httptest.NewRecorder()

	h.CreateAlertRule(rr, req)

	require.Equal(t, http.StatusCreated, rr.Code)
	assert.Contains(t, rr.Body.String(), "created")
}

func TestGetAlertRule_Budget_Success(t *testing.T) {
	t.Parallel()

	svc := servicemocks.NewMockAlertRuleService(t)
	svc.On("GetAlertRule", mock.Anything, mock.Anything, mock.Anything).Return(&gen.AlertRuleResponse{}, nil)

	h := newInternalHandler(svc)
	req := httptest.NewRequest(http.MethodGet, "/api/v1alpha1/alerts/sources/budget/rules/r1", nil)
	req.SetPathValue("sourceType", sourceTypeBudget)
	req.SetPathValue("ruleName", "r1")
	rr := httptest.NewRecorder()

	h.GetAlertRule(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
}
