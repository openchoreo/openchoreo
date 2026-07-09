// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package typed

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/pkg/fsindex/index"
)

func makeComponentTypeEntry(t *testing.T, ct *v1alpha1.ComponentType) *index.ResourceEntry {
	t.Helper()
	raw, err := runtime.DefaultUnstructuredConverter.ToUnstructured(ct)
	require.NoError(t, err)
	obj := &unstructured.Unstructured{Object: raw}
	obj.SetGroupVersionKind(v1alpha1.GroupVersion.WithKind("ComponentType"))
	return &index.ResourceEntry{Resource: obj}
}

func TestNewComponentType(t *testing.T) {
	tests := []struct {
		name    string
		entry   *index.ResourceEntry
		wantErr bool
	}{
		{
			name: "valid entry",
			entry: makeComponentTypeEntry(t, &v1alpha1.ComponentType{
				Spec: v1alpha1.ComponentTypeSpec{
					WorkloadType: "deployment",
				},
			}),
		},
		{
			name:    "nil resource entry",
			entry:   nil,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ct, err := NewComponentType(tt.entry)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, ct)
		})
	}
}

func TestComponentTypeWorkloadType(t *testing.T) {
	tests := []struct {
		name         string
		workloadType string
		want         string
	}{
		{
			name:         "present",
			workloadType: "deployment",
			want:         "deployment",
		},
		{
			name:         "empty",
			workloadType: "",
			want:         "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ct := &ComponentType{
				ComponentType: &v1alpha1.ComponentType{
					Spec: v1alpha1.ComponentTypeSpec{
						WorkloadType: tt.workloadType,
					},
				},
			}
			assert.Equal(t, tt.want, ct.WorkloadType())
		})
	}
}
