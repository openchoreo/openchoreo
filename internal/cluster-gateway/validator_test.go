// Copyright 2025 The OpenChoreo Authors
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
	require.NotNil(t, v)
	assert.Equal(t, int64(10*1024*1024), v.maxRequestBodySize)
	assert.True(t, v.allowedMethods[http.MethodGet])
	assert.True(t, v.allowedTargets["k8s"])
	assert.Len(t, v.blockedPaths, 3)
}

func TestValidateRequest_Method(t *testing.T) {
	v := NewRequestValidator()

	tests := []struct {
		name      string
		method    string
		expectErr bool
	}{
		{"GET allowed", http.MethodGet, false},
		{"POST allowed", http.MethodPost, false},
		{"PUT allowed", http.MethodPut, false},
		{"PATCH allowed", http.MethodPatch, false},
		{"DELETE allowed", http.MethodDelete, false},
		{"HEAD allowed", http.MethodHead, false},
		{"OPTIONS allowed", http.MethodOptions, false},
		{"CONNECT disallowed", http.MethodConnect, true},
		{"TRACE disallowed", http.MethodTrace, true},
		{"unknown method disallowed", "FOOBAR", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest(tt.method, "/", nil)
			err := v.ValidateRequest(r, "k8s", "/api/v1/pods")
			if tt.expectErr {
				require.Error(t, err)
				var valErr *ValidationError
				require.ErrorAs(t, err, &valErr)
				assert.Equal(t, http.StatusMethodNotAllowed, valErr.Code)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateRequest_Target(t *testing.T) {
	v := NewRequestValidator()

	tests := []struct {
		name      string
		target    string
		expectErr bool
	}{
		{"k8s allowed", "k8s", false},
		{"monitoring allowed", "monitoring", false},
		{"logs allowed", "logs", false},
		{"unknown target blocked", "unknown", true},
		{"empty target blocked", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			err := v.ValidateRequest(r, tt.target, "/api/v1/pods")
			if tt.expectErr {
				require.Error(t, err)
				var valErr *ValidationError
				require.ErrorAs(t, err, &valErr)
				assert.Equal(t, http.StatusForbidden, valErr.Code)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateRequest_BodySize(t *testing.T) {
	v := NewRequestValidator()

	tests := []struct {
		name          string
		contentLength int64
		expectErr     bool
	}{
		{"within limit", 1024, false},
		{"at limit", 10 * 1024 * 1024, false},
		{"over limit", 10*1024*1024 + 1, true},
		{"zero", 0, false},
		{"unknown (-1)", -1, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			r.ContentLength = tt.contentLength
			err := v.ValidateRequest(r, "k8s", "/api/v1/pods")
			if tt.expectErr {
				require.Error(t, err)
				var valErr *ValidationError
				require.ErrorAs(t, err, &valErr)
				assert.Equal(t, http.StatusRequestEntityTooLarge, valErr.Code)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateRequest_DirectoryTraversal(t *testing.T) {
	v := NewRequestValidator()

	tests := []struct {
		name      string
		path      string
		expectErr bool
	}{
		{"normal path", "/api/v1/pods", false},
		{"path with ..", "/api/v1/../secrets", true},
		{"path with .. at start", "/../etc/passwd", true},
		{"path with .. at end", "/api/v1/..", true},
		{"path with single dot", "/api/v1/./pods", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			err := v.ValidateRequest(r, "k8s", tt.path)
			if tt.expectErr {
				require.Error(t, err)
				var valErr *ValidationError
				require.ErrorAs(t, err, &valErr)
				assert.Equal(t, http.StatusBadRequest, valErr.Code)
				assert.Contains(t, valErr.Message, "directory traversal")
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateRequest_NullBytes(t *testing.T) {
	v := NewRequestValidator()

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	err := v.ValidateRequest(r, "k8s", "/api/v1/pods\x00injected")
	require.Error(t, err)
	var valErr *ValidationError
	require.ErrorAs(t, err, &valErr)
	assert.Equal(t, http.StatusBadRequest, valErr.Code)
	assert.Contains(t, valErr.Message, "null bytes")
}

func TestValidateRequest_BlockedPaths(t *testing.T) {
	v := NewRequestValidator()

	tests := []struct {
		name      string
		path      string
		expectErr bool
	}{
		{"kube-system secrets exact", "/api/v1/namespaces/kube-system/secrets", true},
		{"kube-system secrets subpath", "/api/v1/namespaces/kube-system/secrets/token-abc", true},
		{"cluster-wide secrets exact", "/api/v1/secrets", true},
		{"cluster-wide secrets subpath", "/api/v1/secrets/my-secret", true},
		{"cluster-wide serviceaccounts exact", "/apis/v1/serviceaccounts", true},
		{"cluster-wide serviceaccounts subpath", "/apis/v1/serviceaccounts/default", true},
		// Should NOT match paths where blocked path is a substring but not a prefix boundary
		{"secrets-backup not blocked", "/api/v1/secrets-backup", false},
		{"secrets-config not blocked", "/api/v1/namespaces/kube-system/secrets-config", false},
		// Normal paths should pass
		{"pods allowed", "/api/v1/pods", false},
		{"deployments allowed", "/apis/apps/v1/deployments", false},
		{"other namespace secrets", "/api/v1/namespaces/default/secrets", false},
		{"configmaps allowed", "/api/v1/namespaces/kube-system/configmaps", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			err := v.ValidateRequest(r, "k8s", tt.path)
			if tt.expectErr {
				require.Error(t, err)
				var valErr *ValidationError
				require.ErrorAs(t, err, &valErr)
				assert.Equal(t, http.StatusForbidden, valErr.Code)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateRequest_BlockedPathNormalization(t *testing.T) {
	v := NewRequestValidator()

	tests := []struct {
		name      string
		path      string
		expectErr bool
	}{
		{"double slash normalization", "/api/v1//secrets", true},
		{"trailing slash", "/api/v1/secrets/", true},
		{"path with dot segments", "/api/v1/./secrets", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			err := v.ValidateRequest(r, "k8s", tt.path)
			if tt.expectErr {
				require.Error(t, err)
				var valErr *ValidationError
				require.ErrorAs(t, err, &valErr)
				assert.Equal(t, http.StatusForbidden, valErr.Code)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestAllowTarget(t *testing.T) {
	v := NewRequestValidator()

	r := httptest.NewRequest(http.MethodGet, "/", nil)

	// Should fail before allowing
	err := v.ValidateRequest(r, "custom", "/api/v1/pods")
	require.Error(t, err)

	// Allow the target
	v.AllowTarget("custom")

	// Should pass now
	err = v.ValidateRequest(r, "custom", "/api/v1/pods")
	assert.NoError(t, err)
}

func TestBlockPath(t *testing.T) {
	v := NewRequestValidator()

	r := httptest.NewRequest(http.MethodGet, "/", nil)

	// Should pass before blocking
	err := v.ValidateRequest(r, "k8s", "/custom/sensitive")
	require.NoError(t, err)

	// Block the path
	v.BlockPath("/custom/sensitive")

	// Should fail now
	err = v.ValidateRequest(r, "k8s", "/custom/sensitive")
	require.Error(t, err)
	var valErr *ValidationError
	require.ErrorAs(t, err, &valErr)
	assert.Equal(t, http.StatusForbidden, valErr.Code)
}

func TestBlockPath_Normalization(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)

	tests := []struct {
		name        string
		blockedPath string
		requestPath string
		expectErr   bool
	}{
		{"trailing slash in blocked path", "/custom/sensitive/", "/custom/sensitive", true},
		{"double slash in blocked path", "/custom//sensitive", "/custom/sensitive", true},
		{"dot segment in blocked path", "/custom/./sensitive", "/custom/sensitive", true},
		{"subpath of normalized blocked path", "/custom/sensitive/", "/custom/sensitive/data", true},
		{"non-matching path", "/custom/sensitive/", "/custom/other", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vt := NewRequestValidator()
			vt.BlockPath(tt.blockedPath)
			err := vt.ValidateRequest(r, "k8s", tt.requestPath)
			if tt.expectErr {
				require.Error(t, err)
				var valErr *ValidationError
				require.ErrorAs(t, err, &valErr)
				assert.Equal(t, http.StatusForbidden, valErr.Code)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestSetMaxRequestBodySize(t *testing.T) {
	v := NewRequestValidator()
	v.SetMaxRequestBodySize(100)

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.ContentLength = 101

	err := v.ValidateRequest(r, "k8s", "/api/v1/pods")
	require.Error(t, err)
	var valErr *ValidationError
	require.ErrorAs(t, err, &valErr)
	assert.Equal(t, http.StatusRequestEntityTooLarge, valErr.Code)

	// At limit should pass
	r.ContentLength = 100
	err = v.ValidateRequest(r, "k8s", "/api/v1/pods")
	assert.NoError(t, err)
}
