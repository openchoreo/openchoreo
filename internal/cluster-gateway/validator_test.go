// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clustergateway

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewRequestValidator(t *testing.T) {
	v := NewRequestValidator()

	assert.Equal(t, int64(10*1024*1024), v.maxRequestBodySize)

	expectedMethods := []string{
		http.MethodGet, http.MethodPost, http.MethodPut,
		http.MethodPatch, http.MethodDelete, http.MethodHead, http.MethodOptions,
	}
	for _, m := range expectedMethods {
		assert.True(t, v.allowedMethods[m], "method %s should be allowed", m)
	}

	expectedTargets := []string{"k8s", "monitoring", "logs"}
	for _, tgt := range expectedTargets {
		assert.True(t, v.allowedTargets[tgt], "target %s should be allowed", tgt)
	}

	// Secrets and service accounts are matched as core /api/v1 segments,
	// not as entries in this extra substring list.
	assert.Empty(t, v.blockedPaths)
}

func TestValidateRequest_AllowedMethods(t *testing.T) {
	v := NewRequestValidator()

	allowed := []string{
		http.MethodGet, http.MethodPost, http.MethodPut,
		http.MethodPatch, http.MethodDelete, http.MethodHead, http.MethodOptions,
	}
	for _, method := range allowed {
		t.Run("allowed_"+method, func(t *testing.T) {
			req := httptest.NewRequest(method, "/test", nil)
			err := v.ValidateRequest(req, "k8s", "/api/v1/pods")
			assert.NoError(t, err)
		})
	}

	disallowed := []string{"TRACE", "CONNECT", "CUSTOM"}
	for _, method := range disallowed {
		t.Run("disallowed_"+method, func(t *testing.T) {
			req := httptest.NewRequest(method, "/test", nil)
			err := v.ValidateRequest(req, "k8s", "/api/v1/pods")
			require.Error(t, err)
			var valErr *ValidationError
			require.ErrorAs(t, err, &valErr)
			assert.Equal(t, http.StatusMethodNotAllowed, valErr.Code)
		})
	}
}

func TestValidateRequest_SensitiveCoreResources(t *testing.T) {
	v := NewRequestValidator()

	blocked := []struct {
		name   string
		method string
		path   string
	}{
		{"namespaced secrets list", http.MethodGet, "/api/v1/namespaces/default/secrets"},
		{"control plane secret get", http.MethodGet, "/api/v1/namespaces/openchoreo-control-plane/secrets/my-secret"},
		{"kube-system secrets", http.MethodGet, "/api/v1/namespaces/kube-system/secrets"},
		{"kube-system secret get", http.MethodGet, "/api/v1/namespaces/kube-system/secrets/my-secret"},
		{"post secret", http.MethodPost, "/api/v1/namespaces/default/secrets"},
		{"delete secret", http.MethodDelete, "/api/v1/namespaces/default/secrets/my-secret"},
		{"cluster serviceaccounts list", http.MethodGet, "/api/v1/serviceaccounts"},
		{"namespaced serviceaccounts list", http.MethodGet, "/api/v1/namespaces/default/serviceaccounts"},
		{"serviceaccount get", http.MethodGet, "/api/v1/namespaces/default/serviceaccounts/default"},
		{"serviceaccount token", http.MethodPost, "/api/v1/namespaces/default/serviceaccounts/default/token"},
		{"watch namespaced secrets", http.MethodGet, "/api/v1/watch/namespaces/default/secrets"},
		{"watch cluster secrets", http.MethodGet, "/api/v1/watch/secrets"},
		{"watch serviceaccounts", http.MethodGet, "/api/v1/watch/namespaces/default/serviceaccounts"},
		{"double slash", http.MethodGet, "/api//v1/namespaces/default/secrets"},
		{"dot segment", http.MethodGet, "/api/./v1/namespaces/default/secrets"},
		{"trailing slash", http.MethodGet, "/api/v1/namespaces/default/secrets/"},
		{"leading double slash", http.MethodGet, "//api/v1/namespaces/default/secrets"},
	}

	for _, tt := range blocked {
		t.Run("blocked_"+tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, "/test", nil)
			err := v.ValidateRequest(req, "k8s", tt.path)
			require.Error(t, err)
			var valErr *ValidationError
			require.ErrorAs(t, err, &valErr)
			assert.Equal(t, http.StatusForbidden, valErr.Code)
		})
	}

	// These look related if you match on the word "secret" or the old
	// /apis/v1 string. They are not core secret or ServiceAccount calls.
	allowed := []struct {
		name string
		path string
	}{
		{"pods", "/api/v1/namespaces/default/pods"},
		{"pod named secrets", "/api/v1/namespaces/default/pods/secrets"},
		{"namespace named secrets", "/api/v1/namespaces/secrets/pods"},
		{"events", "/api/v1/namespaces/default/events"},
		{"watch pods", "/api/v1/watch/namespaces/default/pods"},
		{"external secret crd", "/apis/external-secrets.io/v1beta1/namespaces/default/externalsecrets"},
		{"external secret crd get", "/apis/external-secrets.io/v1beta1/namespaces/default/externalsecrets/db"},
		{"legacy wrong sa string", "/apis/v1/serviceaccounts"},
	}

	for _, tt := range allowed {
		t.Run("allowed_"+tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			err := v.ValidateRequest(req, "k8s", tt.path)
			assert.NoError(t, err)
		})
	}
}

func TestValidateRequest_AllowedTargets(t *testing.T) {
	v := NewRequestValidator()

	for _, target := range []string{"k8s", "monitoring", "logs"} {
		t.Run("allowed_"+target, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			err := v.ValidateRequest(req, target, "/api/v1/pods")
			assert.NoError(t, err)
		})
	}

	for _, target := range []string{"unknown", "database", ""} {
		t.Run("disallowed_"+target, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			err := v.ValidateRequest(req, target, "/api/v1/pods")
			require.Error(t, err)
			var valErr *ValidationError
			require.ErrorAs(t, err, &valErr)
			assert.Equal(t, http.StatusForbidden, valErr.Code)
		})
	}
}

func TestValidateRequest_BodySizeLimit(t *testing.T) {
	v := NewRequestValidator()

	t.Run("exceeds limit", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/test", nil)
		req.ContentLength = 10*1024*1024 + 1
		err := v.ValidateRequest(req, "k8s", "/api/v1/pods")
		require.Error(t, err)
		var valErr *ValidationError
		require.ErrorAs(t, err, &valErr)
		assert.Equal(t, http.StatusRequestEntityTooLarge, valErr.Code)
	})

	t.Run("at limit", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/test", nil)
		req.ContentLength = 10 * 1024 * 1024
		err := v.ValidateRequest(req, "k8s", "/api/v1/pods")
		assert.NoError(t, err)
	})

	t.Run("below limit", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/test", nil)
		req.ContentLength = 1024
		err := v.ValidateRequest(req, "k8s", "/api/v1/pods")
		assert.NoError(t, err)
	})
}

func TestValidateRequest_PathTraversal(t *testing.T) {
	v := NewRequestValidator()

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	err := v.ValidateRequest(req, "k8s", "/api/v1/../secrets")
	require.Error(t, err)
	var valErr *ValidationError
	require.ErrorAs(t, err, &valErr)
	assert.Equal(t, http.StatusBadRequest, valErr.Code)
	assert.Contains(t, valErr.Message, "directory traversal")
}

func TestValidateRequest_NullBytes(t *testing.T) {
	v := NewRequestValidator()

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	err := v.ValidateRequest(req, "k8s", "/api/v1/pods\x00malicious")
	require.Error(t, err)
	var valErr *ValidationError
	require.ErrorAs(t, err, &valErr)
	assert.Equal(t, http.StatusBadRequest, valErr.Code)
	assert.Contains(t, valErr.Message, "null bytes")
}

func TestValidateRequest_ValidRequest(t *testing.T) {
	v := NewRequestValidator()

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	err := v.ValidateRequest(req, "k8s", "/api/v1/namespaces/default/pods")
	assert.NoError(t, err)
}

func TestValidationError_Error(t *testing.T) {
	err := &ValidationError{
		Code:    http.StatusForbidden,
		Message: "access denied",
	}
	assert.Equal(t, "access denied", err.Error())
}

func TestAllowTarget(t *testing.T) {
	v := NewRequestValidator()

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	err := v.ValidateRequest(req, "custom-target", "/api/v1/pods")
	assert.Error(t, err)

	v.AllowTarget("custom-target")

	err = v.ValidateRequest(req, "custom-target", "/api/v1/pods")
	assert.NoError(t, err)
}

func TestBlockPath(t *testing.T) {
	v := NewRequestValidator()

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	path := "/api/v1/namespaces/default/configmaps"
	err := v.ValidateRequest(req, "k8s", path)
	assert.NoError(t, err)

	v.BlockPath(path)

	err = v.ValidateRequest(req, "k8s", path)
	require.Error(t, err)
	var valErr *ValidationError
	require.ErrorAs(t, err, &valErr)
	assert.Equal(t, http.StatusForbidden, valErr.Code)
}

func TestSetMaxRequestBodySize(t *testing.T) {
	v := NewRequestValidator()
	v.SetMaxRequestBodySize(1024)

	req := httptest.NewRequest(http.MethodPost, "/test", nil)
	req.ContentLength = 2048
	err := v.ValidateRequest(req, "k8s", "/api/v1/pods")
	require.Error(t, err)
	var valErr *ValidationError
	require.ErrorAs(t, err, &valErr)
	assert.Equal(t, http.StatusRequestEntityTooLarge, valErr.Code)

	req.ContentLength = 1024
	err = v.ValidateRequest(req, "k8s", "/api/v1/pods")
	assert.NoError(t, err)
}
