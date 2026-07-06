// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package component

import (
	"fmt"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/componentrelease"
)

func makeCTSnapshot() openchoreov1alpha1.ComponentReleaseComponentType {
	return openchoreov1alpha1.ComponentReleaseComponentType{
		Kind: openchoreov1alpha1.ComponentTypeRefKindComponentType,
		Name: "deployment/test-type",
		Spec: openchoreov1alpha1.ComponentTypeSpec{WorkloadType: "deployment"},
	}
}

func TestComputeReleaseHash(t *testing.T) {
	tests := []struct {
		name           string
		template       *ReleaseSpec
		collisionCount *int32
		expectSame     *ReleaseSpec // if set, should produce same hash
		expectDiff     *ReleaseSpec // if set, should produce different hash
	}{
		{
			name: "basic hash computation",
			template: &ReleaseSpec{
				ComponentType: makeCTSnapshot(),
				ComponentProfile: &openchoreov1alpha1.ComponentProfile{
					Parameters: &runtime.RawExtension{Raw: []byte(`{"replicas": 3}`)},
				},
				Workload: openchoreov1alpha1.WorkloadTemplateSpec{
					Container: openchoreov1alpha1.Container{
						Image: "nginx:1.21",
					},
				},
			},
		},
		{
			name: "same template produces same hash",
			template: &ReleaseSpec{
				ComponentType: makeCTSnapshot(),
				ComponentProfile: &openchoreov1alpha1.ComponentProfile{
					Parameters: &runtime.RawExtension{Raw: []byte(`{"replicas": 3}`)},
				},
				Workload: openchoreov1alpha1.WorkloadTemplateSpec{
					Container: openchoreov1alpha1.Container{
						Image: "nginx:1.21",
					},
				},
			},
			expectSame: &ReleaseSpec{
				ComponentType: makeCTSnapshot(),
				ComponentProfile: &openchoreov1alpha1.ComponentProfile{
					Parameters: &runtime.RawExtension{Raw: []byte(`{"replicas": 3}`)},
				},
				Workload: openchoreov1alpha1.WorkloadTemplateSpec{
					Container: openchoreov1alpha1.Container{
						Image: "nginx:1.21",
					},
				},
			},
		},
		{
			name: "different workload image produces different hash",
			template: &ReleaseSpec{
				ComponentType: makeCTSnapshot(),
				ComponentProfile: &openchoreov1alpha1.ComponentProfile{
					Parameters: &runtime.RawExtension{Raw: []byte(`{"replicas": 3}`)},
				},
				Workload: openchoreov1alpha1.WorkloadTemplateSpec{
					Container: openchoreov1alpha1.Container{
						Image: "nginx:1.21",
					},
				},
			},
			expectDiff: &ReleaseSpec{
				ComponentType: makeCTSnapshot(),
				ComponentProfile: &openchoreov1alpha1.ComponentProfile{
					Parameters: &runtime.RawExtension{Raw: []byte(`{"replicas": 3}`)},
				},
				Workload: openchoreov1alpha1.WorkloadTemplateSpec{
					Container: openchoreov1alpha1.Container{
						Image: "nginx:1.22", // Different image version
					},
				},
			},
		},
		{
			name: "collision count changes hash",
			template: &ReleaseSpec{
				ComponentType: makeCTSnapshot(),
				Workload: openchoreov1alpha1.WorkloadTemplateSpec{
					Container: openchoreov1alpha1.Container{
						Image: "nginx:1.21",
					},
				},
			},
			collisionCount: func() *int32 { v := int32(1); return &v }(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hash1 := ComputeReleaseHash(tt.template, tt.collisionCount)

			// Verify hash is not empty
			if hash1 == "" {
				t.Errorf("ComputeReleaseHash() returned empty hash")
			}

			// Verify hash is consistent (computing twice gives same result)
			hash2 := ComputeReleaseHash(tt.template, tt.collisionCount)
			if hash1 != hash2 {
				t.Errorf("ComputeReleaseHash() not consistent: %s != %s", hash1, hash2)
			}

			// Test expectSame
			if tt.expectSame != nil {
				hashSame := ComputeReleaseHash(tt.expectSame, tt.collisionCount)
				if hash1 != hashSame {
					t.Errorf("Expected same hash for identical templates, got %s != %s", hash1, hashSame)
				}
			}

			// Test expectDiff
			if tt.expectDiff != nil {
				hashDiff := ComputeReleaseHash(tt.expectDiff, tt.collisionCount)
				if hash1 == hashDiff {
					t.Errorf("Expected different hash for different templates, got %s == %s", hash1, hashDiff)
				}
			}
		})
	}
}

func TestComputeReleaseHashWithCollision(t *testing.T) {
	template := &ReleaseSpec{
		ComponentType: makeCTSnapshot(),
		Workload: openchoreov1alpha1.WorkloadTemplateSpec{
			Container: openchoreov1alpha1.Container{
				Image: "nginx:1.21",
			},
		},
	}

	// Hash without collision count
	hashNoCollision := ComputeReleaseHash(template, nil)

	// Hash with collision count = 0
	collisionZero := int32(0)
	hashCollisionZero := ComputeReleaseHash(template, &collisionZero)

	// Hash with collision count = 1
	collisionOne := int32(1)
	hashCollisionOne := ComputeReleaseHash(template, &collisionOne)

	// These should all be different due to collision count
	if hashNoCollision == hashCollisionZero {
		t.Errorf("Hash with nil collision count should differ from collision count 0")
	}

	if hashCollisionZero == hashCollisionOne {
		t.Errorf("Hash with collision count 0 should differ from collision count 1")
	}

	if hashNoCollision == hashCollisionOne {
		t.Errorf("Hash with nil collision count should differ from collision count 1")
	}
}

func TestEqualReleaseTemplate(t *testing.T) {
	template1 := &ReleaseSpec{
		ComponentType: makeCTSnapshot(),
		Workload: openchoreov1alpha1.WorkloadTemplateSpec{
			Container: openchoreov1alpha1.Container{
				Image: "nginx:1.21",
			},
		},
	}

	template2 := &ReleaseSpec{
		ComponentType: makeCTSnapshot(),
		Workload: openchoreov1alpha1.WorkloadTemplateSpec{
			Container: openchoreov1alpha1.Container{
				Image: "nginx:1.21",
			},
		},
	}

	template3 := &ReleaseSpec{
		ComponentType: makeCTSnapshot(),
		Workload: openchoreov1alpha1.WorkloadTemplateSpec{
			Container: openchoreov1alpha1.Container{
				Image: "nginx:1.22", // Different image
			},
		},
	}

	tests := []struct {
		name     string
		lhs      *ReleaseSpec
		rhs      *ReleaseSpec
		expected bool
	}{
		{
			name:     "identical templates are equal",
			lhs:      template1,
			rhs:      template2,
			expected: true,
		},
		{
			name:     "different templates are not equal",
			lhs:      template1,
			rhs:      template3,
			expected: false,
		},
		{
			name:     "both nil are equal",
			lhs:      nil,
			rhs:      nil,
			expected: true,
		},
		{
			name:     "one nil are not equal",
			lhs:      template1,
			rhs:      nil,
			expected: false,
		},
		{
			name:     "nil and non-nil are not equal (reversed)",
			lhs:      nil,
			rhs:      template1,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := EqualReleaseTemplate(tt.lhs, tt.rhs)
			if result != tt.expected {
				t.Errorf("EqualReleaseTemplate() = %v, want %v", result, tt.expected)
			}
		})
	}
}

// TestHashOutputExamples demonstrates actual hash output from the FNV-1a algorithm.
// This test always passes and is used to show sample hashes.
func TestHashOutputExamples(t *testing.T) {
	examples := []struct {
		name     string
		template *ReleaseSpec
	}{
		{
			name: "simple deployment",
			template: &ReleaseSpec{
				ComponentType: makeCTSnapshot(),
				Workload: openchoreov1alpha1.WorkloadTemplateSpec{
					Container: openchoreov1alpha1.Container{
						Image: "nginx:1.21",
					},
				},
			},
		},
		{
			name: "deployment with parameters",
			template: &ReleaseSpec{
				ComponentType: makeCTSnapshot(),
				ComponentProfile: &openchoreov1alpha1.ComponentProfile{
					Parameters: &runtime.RawExtension{Raw: []byte(`{"replicas": 3, "port": 8080}`)},
				},
				Workload: openchoreov1alpha1.WorkloadTemplateSpec{
					Container: openchoreov1alpha1.Container{
						Image: "nginx:1.21",
					},
				},
			},
		},
		{
			name: "different image version",
			template: &ReleaseSpec{
				ComponentType: makeCTSnapshot(),
				Workload: openchoreov1alpha1.WorkloadTemplateSpec{
					Container: openchoreov1alpha1.Container{
						Image: "nginx:1.22", // Different version
					},
				},
			},
		},
	}

	fmt.Println("\n=== Sample ComponentRelease Hashes (FNV-1a) ===")
	for _, ex := range examples {
		hash := ComputeReleaseHash(ex.template, nil)
		fmt.Printf("%-30s → %s\n", ex.name, hash)

		// Also show with collision count
		collisionOne := int32(1)
		hashWithCollision := ComputeReleaseHash(ex.template, &collisionOne)
		fmt.Printf("%-30s → %s (with collision count 1)\n", ex.name, hashWithCollision)
	}
	fmt.Println("===============================================")
}

// buildSpecForTest builds a ComponentReleaseSpec via componentrelease.BuildSpec with a
// minimal valid input carrying the given traits map. Lives here (not the builder package)
// because the hash helpers below are in this package and importing them there would cycle.
func buildSpecForTest(t *testing.T, traits map[string]openchoreov1alpha1.TraitSpec) *openchoreov1alpha1.ComponentReleaseSpec {
	t.Helper()
	out, err := componentrelease.BuildSpec(componentrelease.BuildInput{
		Component: &openchoreov1alpha1.Component{
			ObjectMeta: metav1.ObjectMeta{Name: "comp"},
			Spec:       openchoreov1alpha1.ComponentSpec{Owner: openchoreov1alpha1.ComponentOwner{ProjectName: "proj"}},
		},
		ComponentType: makeCTSnapshot(),
		Traits:        traits,
		Workload: &openchoreov1alpha1.WorkloadTemplateSpec{
			Container: openchoreov1alpha1.Container{Image: "nginx:1.21"},
		},
	})
	if err != nil {
		t.Fatalf("BuildSpec failed: %v", err)
	}
	return out
}

// hashOf mirrors the component controller: derive a ReleaseSpec then compute its hash.
func hashOf(t *testing.T, spec *openchoreov1alpha1.ComponentReleaseSpec) string {
	t.Helper()
	return ComputeReleaseHash(ReleaseSpecFromComponentReleaseSpec(spec), nil)
}

func TestReleaseHash_ChangesWithPostRenderValidations(t *testing.T) {
	base := buildSpecForTest(t, map[string]openchoreov1alpha1.TraitSpec{"t1": {}})
	withPRV := buildSpecForTest(t, map[string]openchoreov1alpha1.TraitSpec{
		"t1": {PostRenderValidations: []openchoreov1alpha1.PostRenderValidation{{
			Target:  openchoreov1alpha1.PostRenderTarget{PatchTarget: openchoreov1alpha1.PatchTarget{Group: "apps", Version: "v1", Kind: "Deployment"}},
			Rule:    "${resource.spec.replicas == 1}",
			Message: "m",
		}}},
	})
	if hashOf(t, base) == hashOf(t, withPRV) {
		t.Fatalf("expected release hash to change when a trait gains postRenderValidations")
	}
}

// buildSpecWithComponentType builds a release spec from a ComponentType snapshot, so tests can
// vary the ComponentType's own fields (not just traits).
func buildSpecWithComponentType(t *testing.T, ct openchoreov1alpha1.ComponentReleaseComponentType) *openchoreov1alpha1.ComponentReleaseSpec {
	t.Helper()
	out, err := componentrelease.BuildSpec(componentrelease.BuildInput{
		Component: &openchoreov1alpha1.Component{
			ObjectMeta: metav1.ObjectMeta{Name: "comp"},
			Spec:       openchoreov1alpha1.ComponentSpec{Owner: openchoreov1alpha1.ComponentOwner{ProjectName: "proj"}},
		},
		ComponentType: ct,
		Workload: &openchoreov1alpha1.WorkloadTemplateSpec{
			Container: openchoreov1alpha1.Container{Image: "nginx:1.21"},
		},
	})
	if err != nil {
		t.Fatalf("BuildSpec failed: %v", err)
	}
	return out
}

func TestReleaseHash_ChangesWithComponentTypePostRenderValidations(t *testing.T) {
	base := buildSpecWithComponentType(t, makeCTSnapshot())
	withPRV := makeCTSnapshot()
	withPRV.Spec.PostRenderValidations = []openchoreov1alpha1.PostRenderValidation{{
		Target:  openchoreov1alpha1.PostRenderTarget{PatchTarget: openchoreov1alpha1.PatchTarget{Group: "apps", Version: "v1", Kind: "Deployment"}},
		Rule:    "${resource.spec.replicas == 1}",
		Message: "m",
	}}
	if hashOf(t, base) == hashOf(t, buildSpecWithComponentType(t, withPRV)) {
		t.Fatalf("expected release hash to change when the ComponentType gains postRenderValidations")
	}
}
