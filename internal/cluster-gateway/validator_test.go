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

	assert.Len(t, v.blockedPaths, 2)
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
		name string
		path string
	}{
		{"kube-system secrets", "/api/v1/namespaces/kube-system/secrets"},
		{"cluster-wide service accounts", "/apis/v1/serviceaccounts"},
		{"kube-system secrets subpath", "/api/v1/namespaces/kube-system/secrets/my-secret"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			err := v.ValidateRequest(req, "k8s", tt.path)
			require.Error(t, err)
			var valErr *ValidationError
			require.ErrorAs(t, err, &valErr)
			assert.Equal(t, http.StatusForbidden, valErr.Code)
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
