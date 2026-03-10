// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package component

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openchoreo/openchoreo/api/v1alpha1"
)

func TestValidateAllowedTraits(t *testing.T) {
	tests := []struct {
		name          string
		compTraits    []v1alpha1.ComponentTrait
		allowedTraits []v1alpha1.TraitRef
		wantErr       bool
		errContains   string
	}{
		{
			name:       "no component traits is always valid",
			compTraits: nil,
			allowedTraits: []v1alpha1.TraitRef{
				{Kind: v1alpha1.TraitRefKindTrait, Name: "foo"},
			},
			wantErr: false,
		},
		{
			name: "component traits not allowed when allowedTraits is empty",
			compTraits: []v1alpha1.ComponentTrait{
				{Kind: v1alpha1.TraitRefKindTrait, Name: "foo", InstanceName: "foo-1"},
			},
			allowedTraits: nil,
			wantErr:       true,
			errContains:   "no traits are allowed",
		},
		{
			name: "all traits in allowed list",
			compTraits: []v1alpha1.ComponentTrait{
				{Kind: v1alpha1.TraitRefKindTrait, Name: "foo", InstanceName: "foo-1"},
				{Kind: v1alpha1.TraitRefKindClusterTrait, Name: "bar", InstanceName: "bar-1"},
			},
			allowedTraits: []v1alpha1.TraitRef{
				{Kind: v1alpha1.TraitRefKindTrait, Name: "foo"},
				{Kind: v1alpha1.TraitRefKindClusterTrait, Name: "bar"},
			},
			wantErr: false,
		},
		{
			name: "trait not in allowed list",
			compTraits: []v1alpha1.ComponentTrait{
				{Kind: v1alpha1.TraitRefKindTrait, Name: "foo", InstanceName: "foo-1"},
				{Kind: v1alpha1.TraitRefKindTrait, Name: "baz", InstanceName: "baz-1"},
			},
			allowedTraits: []v1alpha1.TraitRef{
				{Kind: v1alpha1.TraitRefKindTrait, Name: "foo"},
			},
			wantErr:     true,
			errContains: "Trait:baz",
		},
		{
			name: "kind mismatch with allowed list",
			compTraits: []v1alpha1.ComponentTrait{
				{Kind: v1alpha1.TraitRefKindClusterTrait, Name: "foo", InstanceName: "foo-1"},
			},
			allowedTraits: []v1alpha1.TraitRef{
				{Kind: v1alpha1.TraitRefKindTrait, Name: "foo"},
			},
			wantErr:     true,
			errContains: "ClusterTrait:foo",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateAllowedTraits(tt.compTraits, tt.allowedTraits)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errContains)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestValidateTraitInstanceNameUniqueness(t *testing.T) {
	tests := []struct {
		name           string
		compTraits     []v1alpha1.ComponentTrait
		embeddedTraits []v1alpha1.ComponentTypeTrait
		wantErr        bool
		errContains    string
	}{
		{
			name:           "no component traits",
			compTraits:     nil,
			embeddedTraits: []v1alpha1.ComponentTypeTrait{{InstanceName: "foo"}},
			wantErr:        false,
		},
		{
			name:           "no embedded traits",
			compTraits:     []v1alpha1.ComponentTrait{{InstanceName: "foo"}},
			embeddedTraits: nil,
			wantErr:        false,
		},
		{
			name: "no collision",
			compTraits: []v1alpha1.ComponentTrait{
				{Name: "a", InstanceName: "a-1"},
			},
			embeddedTraits: []v1alpha1.ComponentTypeTrait{
				{Name: "b", InstanceName: "b-1"},
			},
			wantErr: false,
		},
		{
			name: "instance name collision between component and embedded",
			compTraits: []v1alpha1.ComponentTrait{
				{Name: "a", InstanceName: "shared-name"},
			},
			embeddedTraits: []v1alpha1.ComponentTypeTrait{
				{Name: "b", InstanceName: "shared-name"},
			},
			wantErr:     true,
			errContains: "shared-name",
		},
		{
			name: "multiple collisions between component and embedded",
			compTraits: []v1alpha1.ComponentTrait{
				{Name: "a", InstanceName: "name1"},
				{Name: "b", InstanceName: "name2"},
			},
			embeddedTraits: []v1alpha1.ComponentTypeTrait{
				{Name: "c", InstanceName: "name1"},
				{Name: "d", InstanceName: "name2"},
			},
			wantErr:     true,
			errContains: "collide with embedded traits",
		},
		{
			name: "duplicate instance names within component traits",
			compTraits: []v1alpha1.ComponentTrait{
				{Name: "a", InstanceName: "dup"},
				{Name: "b", InstanceName: "dup"},
			},
			embeddedTraits: nil,
			wantErr:        true,
			errContains:    "in component traits",
		},
		{
			name:       "duplicate instance names within embedded traits",
			compTraits: nil,
			embeddedTraits: []v1alpha1.ComponentTypeTrait{
				{Name: "a", InstanceName: "dup"},
				{Name: "b", InstanceName: "dup"},
			},
			wantErr:     true,
			errContains: "in embedded traits",
		},
		{
			name: "embedded duplicates detected before component trait checks",
			compTraits: []v1alpha1.ComponentTrait{
				{Name: "a", InstanceName: "dup"},
			},
			embeddedTraits: []v1alpha1.ComponentTypeTrait{
				{Name: "b", InstanceName: "dup"},
				{Name: "c", InstanceName: "dup"},
			},
			wantErr:     true,
			errContains: "in embedded traits",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateTraitInstanceNameUniqueness(tt.compTraits, tt.embeddedTraits)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errContains)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestValidateTraitNameKindConsistency(t *testing.T) {
	tests := []struct {
		name           string
		compTraits     []v1alpha1.ComponentTrait
		embeddedTraits []v1alpha1.ComponentTypeTrait
		wantErr        bool
		errContains    string
	}{
		{
			name:           "no component traits",
			compTraits:     nil,
			embeddedTraits: []v1alpha1.ComponentTypeTrait{{Kind: v1alpha1.TraitRefKindTrait, Name: "foo", InstanceName: "foo-1"}},
			wantErr:        false,
		},
		{
			name:           "no embedded traits",
			compTraits:     []v1alpha1.ComponentTrait{{Kind: v1alpha1.TraitRefKindTrait, Name: "foo", InstanceName: "foo-1"}},
			embeddedTraits: nil,
			wantErr:        false,
		},
		{
			name: "same name same kind is valid",
			compTraits: []v1alpha1.ComponentTrait{
				{Kind: v1alpha1.TraitRefKindTrait, Name: "foo", InstanceName: "foo-comp"},
			},
			embeddedTraits: []v1alpha1.ComponentTypeTrait{
				{Kind: v1alpha1.TraitRefKindTrait, Name: "foo", InstanceName: "foo-embedded"},
			},
			wantErr: false,
		},
		{
			name: "different names different kinds is valid",
			compTraits: []v1alpha1.ComponentTrait{
				{Kind: v1alpha1.TraitRefKindClusterTrait, Name: "bar", InstanceName: "bar-1"},
			},
			embeddedTraits: []v1alpha1.ComponentTypeTrait{
				{Kind: v1alpha1.TraitRefKindTrait, Name: "foo", InstanceName: "foo-1"},
			},
			wantErr: false,
		},
		{
			name: "cross-list conflict message includes source",
			compTraits: []v1alpha1.ComponentTrait{
				{Kind: v1alpha1.TraitRefKindClusterTrait, Name: "storage", InstanceName: "storage-comp"},
			},
			embeddedTraits: []v1alpha1.ComponentTypeTrait{
				{Kind: v1alpha1.TraitRefKindTrait, Name: "storage", InstanceName: "storage-embedded"},
			},
			wantErr:     true,
			errContains: "Trait in embedded traits and ClusterTrait in component traits",
		},
		{
			name: "same name different kind is invalid",
			compTraits: []v1alpha1.ComponentTrait{
				{Kind: v1alpha1.TraitRefKindClusterTrait, Name: "foo", InstanceName: "foo-comp"},
			},
			embeddedTraits: []v1alpha1.ComponentTypeTrait{
				{Kind: v1alpha1.TraitRefKindTrait, Name: "foo", InstanceName: "foo-embedded"},
			},
			wantErr:     true,
			errContains: "foo",
		},
		{
			name:       "conflict within embedded traits",
			compTraits: nil,
			embeddedTraits: []v1alpha1.ComponentTypeTrait{
				{Kind: v1alpha1.TraitRefKindTrait, Name: "foo", InstanceName: "foo-1"},
				{Kind: v1alpha1.TraitRefKindClusterTrait, Name: "foo", InstanceName: "foo-2"},
			},
			wantErr:     true,
			errContains: "in embedded traits",
		},
		{
			name: "conflict within component traits",
			compTraits: []v1alpha1.ComponentTrait{
				{Kind: v1alpha1.TraitRefKindTrait, Name: "bar", InstanceName: "bar-1"},
				{Kind: v1alpha1.TraitRefKindClusterTrait, Name: "bar", InstanceName: "bar-2"},
			},
			embeddedTraits: nil,
			wantErr:        true,
			errContains:    "in component traits",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateTraitNameKindConsistency(tt.compTraits, tt.embeddedTraits)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errContains)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
