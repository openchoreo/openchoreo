// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package k8sresource

import (
	"errors"
	"strings"
)

// blockedPathSegments contains path segments that are blocked from proxying for security.
// These are matched as exact URL path segments to avoid false positives.
// NOTE: exec/attach/portforward are POST-only in the K8s API, but we block them
// defensively since WebSocket upgrades can bypass method restrictions.
// TODO: Consider switching to an allowlist approach for stricter control.
var blockedPathSegments = []string{
	"secrets",
	"serviceaccounts",
	"exec",
	"attach",
	"portforward",
}

// blockedNamespaces contains namespaces that are blocked from proxying for security.
var blockedNamespaces = []string{
	"kube-system",
	"kube-public",
	"kube-node-lease",
}

// ValidateK8sPath checks that the K8s API path is safe to proxy.
func ValidateK8sPath(path string) error {
	// Block directory traversal
	if strings.Contains(path, "..") {
		return errors.New("path traversal is not allowed")
	}

	// Block null bytes
	if strings.ContainsRune(path, '\x00') {
		return errors.New("invalid path")
	}

	// Split into segments for exact matching to avoid false positives
	// (e.g., a pod named "my-secrets-pod" should not be blocked)
	segments := strings.Split(strings.ToLower(path), "/")

	// Block access to sensitive resource types (exact segment match)
	for _, seg := range segments {
		for _, blocked := range blockedPathSegments {
			if seg == blocked {
				return errors.New("access to this resource type is not allowed")
			}
		}
	}

	// Block access to sensitive namespaces
	for i, seg := range segments {
		if seg == "namespaces" && i+1 < len(segments) {
			for _, ns := range blockedNamespaces {
				if segments[i+1] == ns {
					return errors.New("access to this namespace is not allowed")
				}
			}
		}
	}

	return nil
}
