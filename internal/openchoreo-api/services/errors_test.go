// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package services

import (
	"errors"
	"io"
	"log/slog"
	"testing"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestHandleListError_InvalidContinue_FromCauses(t *testing.T) {
	status := metav1.Status{
		Code:    400,
		Message: "invalid continue token",
		Reason:  metav1.StatusReasonInvalid,
		Details: &metav1.StatusDetails{
			Causes: []metav1.StatusCause{{Field: "continue", Message: "invalid continue token"}},
		},
	}
	err := &apierrors.StatusError{ErrStatus: status}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	var ei error = err
	if statusErr, ok := ei.(apierrors.APIStatus); !ok {
		t.Fatalf("expected APIStatus error type, got: %T", err)
	} else {
		t.Logf("status details: %+v", statusErr.Status())
	}

	got := HandleListError(err, logger, "token", "resources")
	if !errors.Is(got, ErrInvalidContinueToken) {
		t.Fatalf("expected ErrInvalidContinueToken, got %v", got)
	}
}

func TestHandleListError_InvalidContinue_StringFallback(t *testing.T) {
	err := apierrors.NewBadRequest("invalid value for continue token 'abc'")
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	got := HandleListError(err, logger, "abc", "resources")
	if !errors.Is(got, ErrInvalidContinueToken) {
		t.Fatalf("expected ErrInvalidContinueToken via fallback, got %v", got)
	}
}

func TestHandleListError_OtherBadRequest(t *testing.T) {
	err := apierrors.NewBadRequest("something else is wrong")
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	got := HandleListError(err, logger, "token", "resources")
	if errors.Is(got, ErrInvalidContinueToken) {
		t.Fatalf("did not expect ErrInvalidContinueToken for unrelated bad request")
	}
}
