// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package bindingrevision

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

func TestSpecChanges_KeepsLargeIntegersExact(t *testing.T) {
	spec := func(config string) openchoreov1alpha1.ReleaseBindingSpec {
		return openchoreov1alpha1.ReleaseBindingSpec{
			Environment: "dev", ReleaseName: "api-a",
			ComponentTypeEnvironmentConfigs: &runtime.RawExtension{Raw: []byte(config)},
		}
	}

	t.Run("integers float64 cannot tell apart are a change", func(t *testing.T) {
		changes, err := SpecChanges(spec(`{"id":9007199254740992}`), spec(`{"id":9007199254740993}`))
		require.NoError(t, err)
		require.Len(t, changes, 1)
		assert.Equal(t, "componentTypeEnvironmentConfigs", changes[0].Field)
	})

	t.Run("a sibling change reports the large integer as written", func(t *testing.T) {
		changes, err := SpecChanges(spec(`{"id":9007199254740993,"replicas":1}`), spec(`{"id":9007199254740993,"replicas":2}`))
		require.NoError(t, err)
		require.Len(t, changes, 1)
		current, err := json.Marshal(changes[0].Current)
		require.NoError(t, err)
		target, err := json.Marshal(changes[0].Target)
		require.NoError(t, err)
		assert.JSONEq(t, `{"id":9007199254740993,"replicas":1}`, string(current))
		assert.Contains(t, string(current), "9007199254740993")
		assert.Contains(t, string(target), "9007199254740993")
	})

	t.Run("an unchanged spec has no changes", func(t *testing.T) {
		changes, err := SpecChanges(spec(`{"id":9007199254740993}`), spec(`{"id":9007199254740993}`))
		require.NoError(t, err)
		assert.Empty(t, changes)
	})
}
