// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

type mockIndexer struct {
	client.FieldIndexer
	indexFuncs map[string]client.IndexerFunc
}

func (m *mockIndexer) IndexField(ctx context.Context, obj client.Object, field string, extractValue client.IndexerFunc) error {
	if m.indexFuncs == nil {
		m.indexFuncs = make(map[string]client.IndexerFunc)
	}
	m.indexFuncs[field] = extractValue
	return nil
}

type mockManager struct {
	manager.Manager
	indexer *mockIndexer
}

func (m *mockManager) GetFieldIndexer() client.FieldIndexer {
	return m.indexer
}

func TestResourceOwnerProjectNameIndexer(t *testing.T) {
	mgr := &mockManager{indexer: &mockIndexer{}}
	err := SetupSharedIndexes(context.Background(), mgr)
	require.NoError(t, err)

	idxFunc := mgr.indexer.indexFuncs[IndexKeyResourceOwnerProjectName]
	require.NotNil(t, idxFunc)

	tests := []struct {
		name     string
		resource *openchoreov1alpha1.Resource
		expected []string
	}{
		{
			name: "with project name",
			resource: &openchoreov1alpha1.Resource{
				Spec: openchoreov1alpha1.ResourceSpec{
					Owner: openchoreov1alpha1.ResourceOwner{
						ProjectName: "test-project",
					},
				},
			},
			expected: []string{"test-project"},
		},
		{
			name: "without project name",
			resource: &openchoreov1alpha1.Resource{
				Spec: openchoreov1alpha1.ResourceSpec{
					Owner: openchoreov1alpha1.ResourceOwner{},
				},
			},
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := idxFunc(tt.resource)
			assert.Equal(t, tt.expected, res)
		})
	}
}

func TestIndexResourceReleaseBindingOwnerEnv(t *testing.T) {
	t.Run("returns_composite_key_when_all_fields_set", func(t *testing.T) {
		rrb := &openchoreov1alpha1.ResourceReleaseBinding{
			Spec: openchoreov1alpha1.ResourceReleaseBindingSpec{
				Owner: openchoreov1alpha1.ResourceReleaseBindingOwner{
					ProjectName:  "proj1",
					ResourceName: "orders-db",
				},
				Environment: "prod",
			},
		}
		got := IndexResourceReleaseBindingOwnerEnv(rrb)
		require.Len(t, got, 1)
		assert.Equal(t, "proj1/orders-db/prod", got[0])
	})

	cases := []struct {
		name string
		spec openchoreov1alpha1.ResourceReleaseBindingSpec
	}{
		{
			name: "empty_project_returns_nil",
			spec: openchoreov1alpha1.ResourceReleaseBindingSpec{
				Owner:       openchoreov1alpha1.ResourceReleaseBindingOwner{ResourceName: "db"},
				Environment: "prod",
			},
		},
		{
			name: "empty_resource_returns_nil",
			spec: openchoreov1alpha1.ResourceReleaseBindingSpec{
				Owner:       openchoreov1alpha1.ResourceReleaseBindingOwner{ProjectName: "p"},
				Environment: "prod",
			},
		},
		{
			name: "empty_environment_returns_nil",
			spec: openchoreov1alpha1.ResourceReleaseBindingSpec{
				Owner: openchoreov1alpha1.ResourceReleaseBindingOwner{ProjectName: "p", ResourceName: "db"},
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rrb := &openchoreov1alpha1.ResourceReleaseBinding{Spec: tc.spec}
			got := IndexResourceReleaseBindingOwnerEnv(rrb)
			assert.Nil(t, got, "expected nil index key when a required field is empty")
		})
	}
}
