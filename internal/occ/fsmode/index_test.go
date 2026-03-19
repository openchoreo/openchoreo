// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package fsmode

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/openchoreo/openchoreo/pkg/fsindex/index"
)

func TestExtractOwnerRef(t *testing.T) {
	tests := []struct {
		name          string
		entry         *index.ResourceEntry
		wantNil       bool
		wantProject   string
		wantComponent string
	}{
		{
			name: "component kind uses metadata.name as componentName",
			entry: &index.ResourceEntry{
				Resource: &unstructured.Unstructured{
					Object: map[string]any{
						"apiVersion": "openchoreo.dev/v1alpha1",
						"kind":       "Component",
						"metadata": map[string]any{
							"name": "my-component",
						},
						"spec": map[string]any{
							"owner": map[string]any{
								"projectName": "my-project",
							},
						},
					},
				},
			},
			wantProject:   "my-project",
			wantComponent: "my-component",
		},
		{
			name: "component release kind uses spec.owner for both fields",
			entry: &index.ResourceEntry{
				Resource: &unstructured.Unstructured{
					Object: map[string]any{
						"apiVersion": "openchoreo.dev/v1alpha1",
						"kind":       "ComponentRelease",
						"metadata": map[string]any{
							"name": "release-1",
						},
						"spec": map[string]any{
							"owner": map[string]any{
								"projectName":   "proj-a",
								"componentName": "comp-b",
							},
						},
					},
				},
			},
			wantProject:   "proj-a",
			wantComponent: "comp-b",
		},
		{
			name:    "nil entry",
			entry:   nil,
			wantNil: true,
		},
		{
			name: "missing owner in spec",
			entry: &index.ResourceEntry{
				Resource: &unstructured.Unstructured{
					Object: map[string]any{
						"apiVersion": "openchoreo.dev/v1alpha1",
						"kind":       "Workload",
						"metadata": map[string]any{
							"name": "wl-1",
						},
						"spec": map[string]any{},
					},
				},
			},
			wantNil: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ref := ExtractOwnerRef(tt.entry)
			if tt.wantNil {
				assert.Nil(t, ref)
				return
			}
			require.NotNil(t, ref)
			assert.Equal(t, tt.wantProject, ref.ProjectName)
			assert.Equal(t, tt.wantComponent, ref.ComponentName)
		})
	}
}
