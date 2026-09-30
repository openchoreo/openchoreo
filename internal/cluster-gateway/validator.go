// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clustergateway

import (
	"fmt"
	"net/http"
	"path"
	"strings"
)

type RequestValidator struct {
	maxRequestBodySize int64
	allowedMethods     map[string]bool
	// blockedPaths are optional extra substrings blocked by BlockPath.
	// Sensitive core resources (secrets, serviceaccounts) are blocked via
	// pathTouchesSensitiveCoreResource so real /api/v1 paths are matched.
	blockedPaths   []string
	allowedTargets map[string]bool
}

type ValidationError struct {
	Code    int
	Message string
}

func (e *ValidationError) Error() string {
	return e.Message
}

// sensitiveCoreResources are core/v1 resource names that must not be reachable
// through the k8s HTTP proxy (any namespace, including subresources like token).
var sensitiveCoreResources = map[string]struct{}{
	"secrets":         {},
	"serviceaccounts": {},
}

func NewRequestValidator() *RequestValidator {
	return &RequestValidator{
		maxRequestBodySize: 10 * 1024 * 1024, // 10MB default
		allowedMethods: map[string]bool{
			http.MethodGet:     true,
			http.MethodPost:    true,
			http.MethodPut:     true,
			http.MethodPatch:   true,
			http.MethodDelete:  true,
			http.MethodHead:    true,
			http.MethodOptions: true,
		},
		blockedPaths: nil,
		allowedTargets: map[string]bool{
			"k8s":        true,
			"monitoring": true,
			"logs":       true,
		},
	}
}

func (v *RequestValidator) ValidateRequest(r *http.Request, target, path string) error {
	if !v.allowedMethods[r.Method] {
		return &ValidationError{
			Code:    http.StatusMethodNotAllowed,
			Message: fmt.Sprintf("HTTP method not allowed: %s", r.Method),
		}
	}

	if !v.allowedTargets[target] {
		return &ValidationError{
			Code:    http.StatusForbidden,
			Message: fmt.Sprintf("Target not allowed: %s", target),
		}
	}

	if pathTouchesSensitiveCoreResource(path) {
		return &ValidationError{
			Code:    http.StatusForbidden,
			Message: fmt.Sprintf("Access to path is blocked: %s", path),
		}
	}

	for _, blockedPath := range v.blockedPaths {
		if strings.Contains(path, blockedPath) {
			return &ValidationError{
				Code:    http.StatusForbidden,
				Message: fmt.Sprintf("Access to path is blocked: %s", path),
			}
		}
	}

	if r.ContentLength > v.maxRequestBodySize {
		return &ValidationError{
			Code:    http.StatusRequestEntityTooLarge,
			Message: fmt.Sprintf("Request body too large: %d bytes (max: %d)", r.ContentLength, v.maxRequestBodySize),
		}
	}

	if strings.Contains(path, "..") {
		return &ValidationError{
			Code:    http.StatusBadRequest,
			Message: "Path contains directory traversal",
		}
	}

	if strings.Contains(path, "\x00") {
		return &ValidationError{
			Code:    http.StatusBadRequest,
			Message: "Path contains null bytes",
		}
	}

	return nil
}

// pathTouchesSensitiveCoreResource reports whether rawPath is a core/v1 request for
// secrets or serviceaccounts (cluster or namespaced, including subresources).
// Only /api/v1/... is matched so CRDs under /apis/... are not false-positive blocked.
//
// path.Clean collapses //, /./, and trailing slashes. An optional "watch" segment
// after /api/v1 is skipped so /api/v1/watch/... is handled the same as list/get.
func pathTouchesSensitiveCoreResource(rawPath string) bool {
	parts := strings.Split(path.Clean(rawPath), "/")
	for i := 0; i < len(parts); i++ {
		if parts[i] != "api" {
			continue
		}
		if i+1 >= len(parts) || parts[i+1] != "v1" {
			continue
		}
		// Resource index is right after /api/v1, or after optional /api/v1/watch.
		resourceIdx := i + 2
		if resourceIdx < len(parts) && parts[resourceIdx] == "watch" {
			resourceIdx++
		}
		// /api/v1/[watch/]<resource>/...
		if resourceIdx < len(parts) {
			if _, ok := sensitiveCoreResources[parts[resourceIdx]]; ok {
				return true
			}
		}
		// /api/v1/[watch/]namespaces/<ns>/<resource>/...
		if resourceIdx+2 < len(parts) && parts[resourceIdx] == "namespaces" {
			if _, ok := sensitiveCoreResources[parts[resourceIdx+2]]; ok {
				return true
			}
		}
	}
	return false
}

func (v *RequestValidator) AllowTarget(target string) {
	v.allowedTargets[target] = true
}

func (v *RequestValidator) BlockPath(path string) {
	v.blockedPaths = append(v.blockedPaths, path)
}

func (v *RequestValidator) SetMaxRequestBodySize(size int64) {
	v.maxRequestBodySize = size
}
