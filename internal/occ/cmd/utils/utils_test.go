// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package utils

import (
	"testing"
	"time"
)

func TestFormatAge(t *testing.T) {
	tests := []struct {
		name string
		t    time.Time
		want string
	}{
		{
			name: "zero time",
			t:    time.Time{},
			want: "0m",
		},
		{
			name: "minutes ago",
			t:    time.Now().Add(-5 * time.Minute),
			want: "5m",
		},
		{
			name: "hours ago",
			t:    time.Now().Add(-3 * time.Hour),
			want: "3h",
		},
		{
			name: "days ago",
			t:    time.Now().Add(-48 * time.Hour),
			want: "2d",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatAge(tt.t)
			if got != tt.want {
				t.Errorf("FormatAge() = %q, want %q", got, tt.want)
			}
		})
	}
}
