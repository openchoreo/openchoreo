// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFormatValueOrPlaceholder(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  string
	}{
		{
			name:  "empty returns placeholder",
			value: "",
			want:  "-",
		},
		{
			name:  "non-empty returns value",
			value: "hello",
			want:  "hello",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatValueOrPlaceholder(tt.value)
			assert.Equal(t, tt.want, got)
		})
	}
}
