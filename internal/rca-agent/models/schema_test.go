// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package models

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSchemaLoading(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		loader func() (map[string]any, error)
	}{
		{"RCA report schema", RCAReportSchema},
		{"chat response schema", ChatResponseSchema},
		{"remediation result schema", RemediationResultSchema},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			schema, err := tt.loader()
			require.NoError(t, err)
			assert.NotEmpty(t, schema)
			assert.Contains(t, schema, "type")
		})
	}
}

func TestSchemaFromYAML(t *testing.T) {
	t.Parallel()

	t.Run("valid YAML", func(t *testing.T) {
		t.Parallel()
		m, err := SchemaFromYAML([]byte("type: object\nproperties:\n  name:\n    type: string"))
		require.NoError(t, err)
		assert.Equal(t, "object", m["type"])
	})

	t.Run("invalid YAML", func(t *testing.T) {
		t.Parallel()
		_, err := SchemaFromYAML([]byte(":::invalid"))
		require.Error(t, err)
	})

	t.Run("empty YAML", func(t *testing.T) {
		t.Parallel()
		m, err := SchemaFromYAML([]byte(""))
		require.NoError(t, err)
		assert.Nil(t, m)
	})
}
