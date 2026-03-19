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

func TestFromEntry(t *testing.T) {
	tests := []struct {
		name    string
		entry   *index.ResourceEntry
		wantErr bool
		wantNS  string
	}{
		{
			name: "valid component type entry",
			entry: func() *index.ResourceEntry {
				raw, _ := runtime.DefaultUnstructuredConverter.ToUnstructured(&v1alpha1.ComponentType{
					Spec: v1alpha1.ComponentTypeSpec{
						WorkloadType: "deployment",
					},
				})
				obj := &unstructured.Unstructured{Object: raw}
				obj.SetGroupVersionKind(v1alpha1.GroupVersion.WithKind("ComponentType"))
				obj.SetNamespace("test-ns")
				return &index.ResourceEntry{Resource: obj}
			}(),
			wantNS: "test-ns",
		},
		{
			name:    "nil entry",
			entry:   nil,
			wantErr: true,
		},
		{
			name: "nil resource in entry",
			entry: &index.ResourceEntry{
				Resource: nil,
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := FromEntry[v1alpha1.ComponentType](tt.entry)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, result)
			assert.Equal(t, tt.wantNS, result.GetNamespace())
		})
	}
}

func TestRawExtensionToMap(t *testing.T) {
	tests := []struct {
		name    string
		raw     *runtime.RawExtension
		wantNil bool
		wantKey string
	}{
		{
			name:    "valid JSON",
			raw:     &runtime.RawExtension{Raw: []byte(`{"type":"object","properties":{"port":{"type":"integer"}}}`)},
			wantKey: "type",
		},
		{
			name:    "empty raw bytes",
			raw:     &runtime.RawExtension{Raw: nil},
			wantNil: true,
		},
		{
			name:    "nil extension",
			raw:     nil,
			wantNil: true,
		},
		{
			name:    "invalid JSON",
			raw:     &runtime.RawExtension{Raw: []byte(`not-json`)},
			wantNil: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := rawExtensionToMap(tt.raw)
			if tt.wantNil {
				assert.Nil(t, result)
				return
			}
			require.NotNil(t, result)
			assert.Contains(t, result, tt.wantKey)
		})
	}
}
