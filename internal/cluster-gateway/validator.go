// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clustergateway

import (
	"fmt"
	"net/http"
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
			"/api/v1/secrets",         // Cluster-wide secrets
			"/api/v1/serviceaccounts", // Cluster-wide service accounts
		},
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

	if v.isBlockedPath(r.Method, path) {
		return &ValidationError{
			Code:    http.StatusForbidden,
			Message: fmt.Sprintf("Access to path is blocked: %s", path),
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

func (v *RequestValidator) isBlockedPath(method, requestPath string) bool {
	for _, blockedPath := range v.blockedPaths {
		if pathMatchesPrefix(requestPath, blockedPath) {
			return true
		}
	}
	return isServiceAccountTokenRequest(method, requestPath)
}

func pathMatchesPrefix(requestPath, blockedPath string) bool {
	requestPath = strings.TrimRight(requestPath, "/")
	blockedPath = strings.TrimRight(blockedPath, "/")
	return requestPath == blockedPath || strings.HasPrefix(requestPath, blockedPath+"/")
}

func isServiceAccountTokenRequest(method, requestPath string) bool {
	if method != http.MethodPost {
		return false
	}

	segments := strings.Split(strings.Trim(requestPath, "/"), "/")
	if len(segments) != 7 {
		return false
	}

	return segments[0] == "api" &&
		segments[1] == "v1" &&
		segments[2] == "namespaces" &&
		segments[3] != "" &&
		segments[4] == "serviceaccounts" &&
		segments[5] != "" &&
		segments[6] == "token"
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
