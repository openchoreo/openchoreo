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
	blockedPaths       []string
	allowedTargets     map[string]bool
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
		blockedPaths: []string{
			"/api/v1/namespaces/kube-system/secrets",
			"/api/v1/secrets",          // Without namespace - cluster-wide
			"/apis/v1/serviceaccounts", // Cluster-wide service accounts
		},
		allowedTargets: map[string]bool{
			"k8s":        true,
			"monitoring": true,
			"logs":       true,
		},
	}
}

func (v *RequestValidator) ValidateRequest(r *http.Request, target, requestPath string) error {
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

	if r.ContentLength > v.maxRequestBodySize {
		return &ValidationError{
			Code:    http.StatusRequestEntityTooLarge,
			Message: fmt.Sprintf("Request body too large: %d bytes (max: %d)", r.ContentLength, v.maxRequestBodySize),
		}
	}

	if strings.Contains(requestPath, "..") {
		return &ValidationError{
			Code:    http.StatusBadRequest,
			Message: "Path contains directory traversal",
		}
	}

	if strings.Contains(requestPath, "\x00") {
		return &ValidationError{
			Code:    http.StatusBadRequest,
			Message: "Path contains null bytes",
		}
	}

	// Normalize the path before checking blocked paths to prevent bypass via
	// double slashes or other path irregularities
	cleanPath := path.Clean(requestPath)
	for _, blockedPath := range v.blockedPaths {
		// Normalize both sides so that irregularities in either the request
		// path or the configured blocked path cannot cause a bypass.
		cleanBlocked := path.Clean(blockedPath)
		if cleanPath == cleanBlocked || strings.HasPrefix(cleanPath, cleanBlocked+"/") {
			return &ValidationError{
				Code:    http.StatusForbidden,
				Message: fmt.Sprintf("Access to path is blocked: %s", requestPath),
			}
		}
	}

	return nil
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
