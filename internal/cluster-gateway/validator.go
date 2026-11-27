// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clustergateway

import (
	"fmt"
	"net/http"
	"path"
	"strings"
)

const k8sTarget = "k8s"

// Secrets and ServiceAccounts stay reachable inside a namespace on purpose: the
// control plane manages them on the planes through this proxy (#3321). Only the
// cluster-wide collections and kube-system Secrets are denied.
var (
	clusterWideDeniedResources = map[string]bool{
		"secrets":         true,
		"serviceaccounts": true,
	}
	namespacedDeniedResources = map[string]map[string]bool{
		"kube-system": {"secrets": true},
	}
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
		blockedPaths: []string{},
		allowedTargets: map[string]bool{
			k8sTarget:    true,
			"monitoring": true,
			"logs":       true,
		},
	}
}

func (v *RequestValidator) ValidateRequest(r *http.Request, target, reqPath string) error {
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

	if strings.Contains(reqPath, "..") {
		return &ValidationError{
			Code:    http.StatusBadRequest,
			Message: "Path contains directory traversal",
		}
	}

	if strings.Contains(reqPath, "\x00") {
		return &ValidationError{
			Code:    http.StatusBadRequest,
			Message: "Path contains null bytes",
		}
	}

	if target == k8sTarget {
		// The path arrives decoded and the agent decodes it again when it builds the
		// upstream URL, so a leftover escape would reach the API server unchecked.
		if strings.Contains(reqPath, "%") {
			return &ValidationError{
				Code:    http.StatusBadRequest,
				Message: "Path contains percent-encoded characters",
			}
		}

		if isDeniedCoreResourcePath(reqPath) {
			return &ValidationError{
				Code:    http.StatusForbidden,
				Message: fmt.Sprintf("Access to path is blocked: %s", reqPath),
			}
		}
	}

	for _, blockedPath := range v.blockedPaths {
		if strings.Contains(reqPath, blockedPath) {
			return &ValidationError{
				Code:    http.StatusForbidden,
				Message: fmt.Sprintf("Access to path is blocked: %s", reqPath),
			}
		}
	}

	if r.ContentLength > v.maxRequestBodySize {
		return &ValidationError{
			Code:    http.StatusRequestEntityTooLarge,
			Message: fmt.Sprintf("Request body too large: %d bytes (max: %d)", r.ContentLength, v.maxRequestBodySize),
		}
	}

	return nil
}

// isDeniedCoreResourcePath reports whether reqPath addresses a denied core API
// resource. List and watch share a path (watch differs only by query or the
// legacy /watch/ prefix), so a path match covers both.
func isDeniedCoreResourcePath(reqPath string) bool {
	segments := strings.Split(strings.Trim(path.Clean("/"+reqPath), "/"), "/")
	if len(segments) < 3 || segments[0] != "api" || segments[1] != "v1" {
		return false
	}
	rest := segments[2:]
	if rest[0] == "watch" {
		rest = rest[1:]
	}
	if len(rest) == 0 {
		return false
	}
	if clusterWideDeniedResources[rest[0]] {
		return true
	}
	return len(rest) >= 3 && rest[0] == "namespaces" && namespacedDeniedResources[rest[1]][rest[2]]
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
