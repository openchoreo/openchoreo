// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package k8sresources

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/models"
	"github.com/openchoreo/openchoreo/internal/resourcetree/protocol"
)

func TestNodeAccumulatorConservativelyMergesDuplicateUIDsInEitherOrder(t *testing.T) {
	full := models.ResourceNode{UID: "same", Object: map[string]any{
		"apiVersion": "v1", "kind": "Secret", "metadata": map[string]any{"name": "shared"}, "data": map[string]any{"key": "value"},
	}}
	restricted := full
	restricted.MetadataOnly = true
	restricted.MatchedBy = matchedByLabelSelector
	restricted.Object = projectMetadata(full.Object)

	for _, nodes := range [][]models.ResourceNode{{full, restricted}, {restricted, full}} {
		acc := newNodeAccumulator(2)
		for _, node := range nodes {
			acc.add(node)
		}
		if assert.Len(t, acc.nodes, 1) {
			assert.True(t, acc.nodes[0].MetadataOnly)
			assert.Equal(t, matchedByLabelSelector, acc.nodes[0].MatchedBy)
			assert.NotContains(t, acc.nodes[0].Object, "data")
		}
	}
}

func TestSafeForClusterScopedParent(t *testing.T) {
	criteria := func(matchLabels map[string]string, namespaces ...string) json.RawMessage {
		raw, err := json.Marshal(protocol.LabelSelectorCriteria{MatchLabels: matchLabels, Namespaces: namespaces})
		assert.NoError(t, err)
		return raw
	}

	tests := []struct {
		name string
		edge *compiledChild
		want bool
	}{
		{
			name: "explicit namespace and name-derived selector",
			edge: &compiledChild{Matcher: protocol.MatcherLabelSelector,
				Criteria: criteria(map[string]string{"owner": protocol.TokenParentName}, "operator-system")},
			want: true,
		},
		{
			name: "selector derives from absent parent namespace",
			edge: &compiledChild{Matcher: protocol.MatcherLabelSelector,
				Criteria: criteria(map[string]string{"owner-ns": protocol.TokenParentNamespace}, "operator-system")},
		},
		{
			name: "selector has no explicit namespace",
			edge: &compiledChild{Matcher: protocol.MatcherLabelSelector,
				Criteria: criteria(map[string]string{"owner": protocol.TokenParentName})},
		},
		{name: "owner reference", edge: &compiledChild{Matcher: protocol.MatcherOwnerRef}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, safeForClusterScopedParent(tt.edge))
		})
	}
}
