// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package cmdutil

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRequireFields(t *testing.T) {
	t.Run("no missing fields returns nil", func(t *testing.T) {
		err := RequireFields("get", "component", map[string]string{
			"namespace": "my-ns",
			"name":      "my-comp",
		})
		require.NoError(t, err)
	})

	t.Run("single missing field", func(t *testing.T) {
		err := RequireFields("get", "component", map[string]string{
			"namespace": "",
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "--namespace")
		assert.Contains(t, err.Error(), "occ component get -h")
		assert.NotContains(t, err.Error(), "parameters") // singular form
	})

	t.Run("multiple missing fields are sorted", func(t *testing.T) {
		err := RequireFields("delete", "project", map[string]string{
			"namespace": "",
			"name":      "",
		})
		require.Error(t, err)
		msg := err.Error()
		assert.Contains(t, msg, "--name, --namespace")
		assert.Contains(t, msg, "parameters") // plural form
		assert.Contains(t, msg, "occ project delete -h")
	})

	t.Run("mix of set and missing fields", func(t *testing.T) {
		err := RequireFields("list", "environment", map[string]string{
			"namespace": "ns1",
			"project":   "",
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "--project")
		assert.NotContains(t, err.Error(), "--namespace")
	})

	t.Run("empty map returns nil", func(t *testing.T) {
		err := RequireFields("list", "namespace", map[string]string{})
		require.NoError(t, err)
	})
}
