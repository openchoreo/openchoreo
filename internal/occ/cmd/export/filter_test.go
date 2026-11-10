// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package export

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNormalizeObject_Namespace(t *testing.T) {
	tests := []struct {
		name              string
		metadataNamespace string
		omitNamespace     bool
		namespace         string
		wantErr           bool
	}{
		{name: "omitted namespace", omitNamespace: true, namespace: "acme"},
		{name: "empty namespace", namespace: "acme"},
		{name: "matching namespace", metadataNamespace: "acme", namespace: "acme"},
		{name: "mismatched namespace", metadataNamespace: "other", namespace: "acme", wantErr: true},
		{name: "no requested namespace", metadataNamespace: "other"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			metadata := map[string]any{"name": "example"}
			if !tt.omitNamespace {
				metadata["namespace"] = tt.metadataNamespace
			}
			object, err := normalizeObject(map[string]any{"metadata": metadata}, "Component", tt.namespace)
			if tt.wantErr {
				require.EqualError(t, err, `metadata.namespace "other" does not match requested namespace "acme"`)
				assert.Nil(t, object)
				return
			}
			require.NoError(t, err)
			gotMetadata := object["metadata"].(map[string]any)
			if tt.namespace == "" {
				assert.NotContains(t, gotMetadata, "namespace")
			} else {
				assert.Equal(t, tt.namespace, gotMetadata["namespace"])
			}
		})
	}
}
