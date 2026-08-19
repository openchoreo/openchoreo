// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package audit

import (
	"net/http"
	"strings"
	"testing"
)

func TestRequestIDFromHeader(t *testing.T) {
	h := http.Header{}
	h.Set("X-Request-ID", "req-123")
	if got := RequestIDFromHeader(h); got != "req-123" {
		t.Errorf("RequestIDFromHeader(with header) = %q, want req-123", got)
	}

	if got := RequestIDFromHeader(http.Header{}); got == "" {
		t.Error("RequestIDFromHeader(no header) = empty, want a generated UUID")
	}
}

// TestRequestIDFromHeader_BoundsOversizedValue guards against a client
// inflating every audit record for its request by sending an oversized
// X-Request-ID — the value is attacker-controlled and reaches Event.RequestID
// verbatim otherwise.
func TestRequestIDFromHeader_BoundsOversizedValue(t *testing.T) {
	h := http.Header{}
	h.Set("X-Request-ID", strings.Repeat("a", maxRequestIDLen+1))

	got := RequestIDFromHeader(h)
	if len(got) > maxRequestIDLen {
		t.Errorf("RequestIDFromHeader(oversized) = %q (len %d), want a generated value within maxRequestIDLen", got, len(got))
	}
	if got == strings.Repeat("a", maxRequestIDLen+1) {
		t.Error("RequestIDFromHeader(oversized) returned the oversized value verbatim, want a generated UUID")
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
