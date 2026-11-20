// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package services

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ApplyRequestedResourceVersion makes the update of existing conditional on the
// resourceVersion the caller read: with it set, the API server rejects the write
// with a conflict if the resource changed since that read. An empty value keeps
// the unconditional update. A value that can't have come from the API is a
// ValidationError instead of an opaque server error.
func ApplyRequestedResourceVersion(existing metav1.Object, requested string) error {
	if requested == "" {
		return nil
	}
	if v, err := strconv.ParseUint(requested, 10, 64); err != nil || v == 0 {
		return &ValidationError{Msg: fmt.Sprintf("metadata.resourceVersion %q is not a version read from the API", requested)}
	}
	existing.SetResourceVersion(requested)
	return nil
}

// FormatDualScopedResourceName returns the authz-engine identifier for a dual-scoped resource.
// Namespace-scoped resources use "{namespace}/{name}"; cluster-scoped resources use plain "{name}".
func FormatDualScopedResourceName(namespace, name string, isClusterScoped bool) string {
	if isClusterScoped {
		return name
	}
	return namespace + "/" + name
}

// ExtractValidationMessage extracts cause messages from a K8s StatusError, falling back to a generic message to avoid leaking internal details.
func ExtractValidationMessage(err error) string {
	if statusErr, ok := errors.AsType[*apierrors.StatusError](err); ok && statusErr.ErrStatus.Reason == metav1.StatusReasonInvalid && statusErr.ErrStatus.Details != nil {
		var msgs []string
		for _, cause := range statusErr.ErrStatus.Details.Causes {
			if cause.Message == "" {
				continue
			}
			if cause.Field != "" {
				msgs = append(msgs, fmt.Sprintf("%s: %s", cause.Field, cause.Message))
			} else {
				msgs = append(msgs, cause.Message)
			}
		}
		if len(msgs) > 0 {
			return strings.Join(msgs, "; ")
		}
	}
	return "validation failed"
}

// ExtractValidationError wraps a Kubernetes Invalid StatusError as a *ValidationError,
// carrying through the originating HTTP status (typically 422). Returns nil for any
// error that is not a Kubernetes Invalid status, including nil input.
func ExtractValidationError(err error) *ValidationError {
	if err == nil {
		return nil
	}
	statusErr, ok := errors.AsType[*apierrors.StatusError](err)
	if !ok || statusErr.ErrStatus.Reason != metav1.StatusReasonInvalid {
		return nil
	}
	return &ValidationError{
		Msg:        ExtractValidationMessage(err),
		StatusCode: int(statusErr.ErrStatus.Code),
	}
}
