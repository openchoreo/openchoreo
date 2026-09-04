// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package audit

import (
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestRequestIDFromHeader(t *testing.T) {
	h := http.Header{}
	h.Set("X-Request-ID", "018f1e4a-6b3a-7c3a-8b3a-6b3a7c3a8b3a")
	if got := RequestIDFromHeader(h); got != "018f1e4a-6b3a-7c3a-8b3a-6b3a7c3a8b3a" {
		t.Errorf("RequestIDFromHeader(valid UUID) = %q, want the header value unchanged", got)
	}

	if got := RequestIDFromHeader(http.Header{}); got == "" {
		t.Error("RequestIDFromHeader(no header) = empty, want a generated UUID")
	}
}

// TestRequestIDFromHeader_RejectsNonUUID guards that a client-chosen value
// which doesn't parse as a UUID must not reach Event.RequestID verbatim —
// it's replaced with a generated UUID and counted via RequestIDRejections.
func TestRequestIDFromHeader_RejectsNonUUID(t *testing.T) {
	before := RequestIDRejections()

	h := http.Header{}
	h.Set("X-Request-ID", "req-123")

	got := RequestIDFromHeader(h)
	if got == "req-123" {
		t.Error("RequestIDFromHeader(non-UUID) returned the client value verbatim, want a generated UUID")
	}
	if _, err := uuid.Parse(got); err != nil {
		t.Errorf("RequestIDFromHeader(non-UUID) = %q, want a valid generated UUID: %v", got, err)
	}

	if after := RequestIDRejections(); after != before+1 {
		t.Errorf("RequestIDRejections() = %d, want %d (exactly one new rejection)", after, before+1)
	}
}

// TestRequestIDFromHeader_BoundsOversizedValue guards against a client
// inflating every audit record for its request by sending an oversized
// X-Request-ID — an oversized value can never parse as a UUID, so it's
// rejected the same way any other malformed value is.
func TestRequestIDFromHeader_BoundsOversizedValue(t *testing.T) {
	h := http.Header{}
	oversized := strings.Repeat("a", 129)
	h.Set("X-Request-ID", oversized)

	got := RequestIDFromHeader(h)
	if got == oversized {
		t.Error("RequestIDFromHeader(oversized) returned the oversized value verbatim, want a generated UUID")
	}
	if _, err := uuid.Parse(got); err != nil {
		t.Errorf("RequestIDFromHeader(oversized) = %q, want a valid generated UUID: %v", got, err)
	}
}

// TestRequestIDFromHeader_AbsentHeaderDoesNotCountAsRejection guards against
// over-counting: a client that sends no X-Request-ID at all hasn't sent a
// malformed one, so RequestIDRejections must not increment for it.
func TestRequestIDFromHeader_AbsentHeaderDoesNotCountAsRejection(t *testing.T) {
	before := RequestIDRejections()
	RequestIDFromHeader(http.Header{})
	if after := RequestIDRejections(); after != before {
		t.Errorf("RequestIDRejections() = %d, want unchanged at %d for an absent header", after, before)
	}
}

func TestSourceIPFromHeader(t *testing.T) {
	tests := []struct {
		name string
		h    http.Header
		want string
	}{
		{name: "no headers", h: http.Header{}, want: ""},
		{
			name: "X-Forwarded-For single IP",
			h:    headerWith("X-Forwarded-For", "203.0.113.1"),
			want: "203.0.113.1",
		},
		{
			name: "X-Forwarded-For multiple IPs takes the first",
			h:    headerWith("X-Forwarded-For", "203.0.113.1, 10.0.0.1, 10.0.0.2"),
			want: "203.0.113.1",
		},
		{
			name: "X-Real-IP used when X-Forwarded-For absent",
			h:    headerWith("X-Real-IP", "203.0.113.9"),
			want: "203.0.113.9",
		},
		{
			name: "X-Forwarded-For takes precedence over X-Real-IP",
			h: func() http.Header {
				h := headerWith("X-Forwarded-For", "203.0.113.1")
				h.Set("X-Real-IP", "203.0.113.9")
				return h
			}(),
			want: "203.0.113.1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := SourceIPFromHeader(tt.h); got != tt.want {
				t.Errorf("SourceIPFromHeader() = %q, want %q", got, tt.want)
			}
		})
	}
}

func headerWith(key, value string) http.Header {
	h := http.Header{}
	h.Set(key, value)
	return h
}
