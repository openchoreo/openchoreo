// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

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
