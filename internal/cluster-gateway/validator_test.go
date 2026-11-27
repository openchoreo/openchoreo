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

func TestValidateRequest_BlockedPaths(t *testing.T) {
	v := NewRequestValidator()

	tests := []struct {
		name   string
		method string
		path   string
	}{
		{"cluster-wide secrets list", http.MethodGet, "/api/v1/secrets"},
		{"cluster-wide secrets watch path", http.MethodGet, "/api/v1/watch/secrets"},
		{"cluster-wide secrets trailing slash", http.MethodGet, "/api/v1/secrets/"},
		{"cluster-wide secrets double slash", http.MethodGet, "//api/v1//secrets"},
		{"cluster-wide secrets dot segment", http.MethodGet, "/api/v1/./secrets"},
		{"cluster-wide secrets delete collection", http.MethodDelete, "/api/v1/secrets"},
		{"cluster-wide service accounts list", http.MethodGet, "/api/v1/serviceaccounts"},
		{"cluster-wide service accounts watch path", http.MethodGet, "/api/v1/watch/serviceaccounts"},
		{"kube-system secrets list", http.MethodGet, "/api/v1/namespaces/kube-system/secrets"},
		{"kube-system secret get", http.MethodGet, "/api/v1/namespaces/kube-system/secrets/my-secret"},
		{"kube-system secrets watch path", http.MethodGet, "/api/v1/watch/namespaces/kube-system/secrets"},
		{"kube-system secrets double slash", http.MethodGet, "/api/v1/namespaces/kube-system//secrets"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, "/test", nil)
			err := v.ValidateRequest(req, "k8s", tt.path)
			require.Error(t, err)
			var valErr *ValidationError
			require.ErrorAs(t, err, &valErr)
			assert.Equal(t, http.StatusForbidden, valErr.Code)
		})
	}
}

func TestValidateRequest_BlockedWatchQuery(t *testing.T) {
	v := NewRequestValidator()

	for _, q := range []string{"watch=true", "watch=1", "watch=True&labelSelector=a%3Db"} {
		t.Run(q, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/test?"+q, nil)
			err := v.ValidateRequest(req, "k8s", "/api/v1/secrets")
			require.Error(t, err)
			var valErr *ValidationError
			require.ErrorAs(t, err, &valErr)
			assert.Equal(t, http.StatusForbidden, valErr.Code)
		})
	}
}

func TestValidateRequest_PercentEncodedPath(t *testing.T) {
	v := NewRequestValidator()

	// The gateway decodes the path once and the agent decodes it again, so a
	// leftover escape like %73 reaches the API server as "s".
	for _, p := range []string{
		"/api/v1/namespaces/kube-system/%73ecrets",
		"/api/v1/%73ecrets",
	} {
		t.Run(p, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			err := v.ValidateRequest(req, "k8s", p)
			require.Error(t, err)
			var valErr *ValidationError
			require.ErrorAs(t, err, &valErr)
			assert.Equal(t, http.StatusBadRequest, valErr.Code)
		})
	}
}

func TestValidateRequest_AllowedSensitivePaths(t *testing.T) {
	v := NewRequestValidator()

	// The control plane manages Secrets and ServiceAccounts in plane namespaces
	// through this proxy, so namespaced access must keep working.
	tests := []struct {
		name   string
		method string
		path   string
	}{
		{"namespaced secrets list", http.MethodGet, "/api/v1/namespaces/dp-default/secrets"},
		{"namespaced secret get", http.MethodGet, "/api/v1/namespaces/dp-default/secrets/app-creds"},
		{"namespaced secret create", http.MethodPost, "/api/v1/namespaces/dp-default/secrets"},
		{"namespaced secret delete", http.MethodDelete, "/api/v1/namespaces/dp-default/secrets/app-creds"},
		{"namespaced secrets watch path", http.MethodGet, "/api/v1/watch/namespaces/dp-default/secrets"},
		{"namespaced service account get", http.MethodGet, "/api/v1/namespaces/dp-default/serviceaccounts/app"},
		{"service account token", http.MethodPost, "/api/v1/namespaces/dp-default/serviceaccounts/app/token"},
		{"namespace named secrets", http.MethodGet, "/api/v1/namespaces/secrets/pods"},
		{"kube-system namespace object", http.MethodGet, "/api/v1/namespaces/kube-system"},
		{"kube-system configmaps", http.MethodGet, "/api/v1/namespaces/kube-system/configmaps"},
		{"external secrets CRD", http.MethodGet, "/apis/external-secrets.io/v1/externalsecrets"},
		{"group named secrets", http.MethodGet, "/apis/secrets.example.com/v1/secrets"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, "/test", nil)
			assert.NoError(t, v.ValidateRequest(req, "k8s", tt.path))
		})
	}
}

func TestValidateRequest_NonK8sTargetSkipsK8sRules(t *testing.T) {
	v := NewRequestValidator()

	for _, p := range []string{"/api/v1/secrets", "/logs/app-%2A/_search"} {
		t.Run(p, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			assert.NoError(t, v.ValidateRequest(req, "logs", p))
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
