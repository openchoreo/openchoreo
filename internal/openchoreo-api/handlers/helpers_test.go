// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"net/url"
	"testing"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/models"
)

func TestExtractListParams_ClampLimitAboveMax(t *testing.T) {
	q := url.Values{}
	q.Set("limit", "1000")

	opts, err := extractListParams(q)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if opts.Limit != models.MaxPageLimit {
		t.Fatalf("expected limit=%d, got %d", models.MaxPageLimit, opts.Limit)
	}
}

func TestExtractListParams_RejectsZeroLimit(t *testing.T) {
	q := url.Values{}
	q.Set("limit", "0")

	_, err := extractListParams(q)
	if err == nil {
		t.Fatalf("expected error for limit=0, got nil")
	}
	expectedErr := "limit 0 out of range [1, 512]"
	if err.Error() != expectedErr {
		t.Fatalf("expected error %q, got %v", expectedErr, err)
	}
}

func TestExtractListParams_InvalidLimit(t *testing.T) {
	q := url.Values{}
	q.Set("limit", "nope")

	_, err := extractListParams(q)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
}

func TestExtractListParams_LimitBelowMinErrors(t *testing.T) {
	q := url.Values{}
	q.Set("limit", "-1")

	_, err := extractListParams(q)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
}
