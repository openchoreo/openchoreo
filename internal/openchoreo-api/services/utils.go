// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package services

import (
	"errors"
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

// ExtractValidationMessage extracts cause messages from a K8s StatusError, falling back to a generic message to avoid leaking internal details.
func ExtractValidationMessage(err error) string {
	var statusErr *apierrors.StatusError
	if errors.As(err, &statusErr) && statusErr.ErrStatus.Details != nil {
		var msgs []string
		for _, cause := range statusErr.ErrStatus.Details.Causes {
			if cause.Message != "" {
				msgs = append(msgs, cause.Message)
			}
		}
		if len(msgs) > 0 {
			return strings.Join(msgs, "; ")
		}
	}
	return "validation failed"
}
