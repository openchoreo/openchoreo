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
	// blockedPaths holds extra substring blocks added through BlockPath.
	// Secrets and service accounts are not in this list. Those are matched
	// as real core /api/v1 resource segments in pathTouchesSensitiveCoreResource.
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

// pathTouchesSensitiveCoreResource reports whether rawPath is a core v1
// request for secrets or serviceaccounts. Cluster-scoped, namespaced, and
// the serviceaccount token subresource all count. The legacy watch form
// (/api/v1/watch/...) is treated like list/get.
//
// Only the core /api/v1 prefix is matched. CRDs live under /apis/<group>/...
// and must keep working. path.Clean runs first so //, /./ and a trailing
// slash cannot move the resource segment out of the place we check.
// A namespace or object whose name is "secrets" is not a match. The
// resource segment has to be.
func pathTouchesSensitiveCoreResource(rawPath string) bool {
	parts := strings.Split(path.Clean(rawPath), "/")
	for i := 0; i+1 < len(parts); i++ {
		if parts[i] != "api" || parts[i+1] != "v1" {
			continue
		}
		rest := parts[i+2:]
		if len(rest) > 0 && rest[0] == "watch" {
			rest = rest[1:]
		}
		if sensitiveCoreResource(rest) {
			return true
		}
	}
	return false
}

func sensitiveCoreResource(rest []string) bool {
	if len(rest) == 0 {
		return false
	}
	if isSensitiveCoreResource(rest[0]) {
		return true
	}
	// /api/v1/[watch/]namespaces/<ns>/<resource>/...
	if len(rest) >= 3 && rest[0] == "namespaces" {
		return isSensitiveCoreResource(rest[2])
	}
	return false
}

func isSensitiveCoreResource(resource string) bool {
	switch resource {
	case "secrets", "serviceaccounts":
		return true
	default:
		return false
	}
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
