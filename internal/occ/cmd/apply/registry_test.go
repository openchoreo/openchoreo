// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package apply

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSupportedKinds(t *testing.T) {
	kinds := supportedKinds()
	require.NotEmpty(t, kinds)
	assert.IsNonDecreasing(t, kinds)
}

func TestResourceFamilyKindsRegistered(t *testing.T) {
	reg := getResourceRegistry()
	tests := []struct {
		kind       string
		scope      resourceScope
		capability applyCapability
		hasUpdate  bool
	}{
		{"ClusterResourceType", scopeCluster, capCreateAndUpdate, true},
		{"ResourceType", scopeNamespaced, capCreateAndUpdate, true},
		{"Resource", scopeNamespaced, capCreateAndUpdate, true},
		{"ResourceRelease", scopeNamespaced, capCreateOnly, false},
		{"ResourceReleaseBinding", scopeNamespaced, capCreateAndUpdate, true},
	}
	for _, tt := range tests {
		t.Run(tt.kind, func(t *testing.T) {
			entry, ok := reg[tt.kind]
			require.True(t, ok, "kind %q not registered", tt.kind)
			assert.Equal(t, tt.scope, entry.scope)
			assert.Equal(t, tt.capability, entry.capability)
			assert.NotNil(t, entry.get)
			assert.NotNil(t, entry.create)
			if tt.hasUpdate {
				assert.NotNil(t, entry.update)
			} else {
				assert.Nil(t, entry.update)
			}
		})
	}
}
