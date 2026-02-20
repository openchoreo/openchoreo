// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package component

import (
	"fmt"
	"testing"

	"k8s.io/apimachinery/pkg/runtime"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

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
				ComponentType: openchoreov1alpha1.ComponentTypeSpec{
					WorkloadType: "deployment",
				},
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
				ComponentType: openchoreov1alpha1.ComponentTypeSpec{
					WorkloadType: "deployment",
				},
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
				ComponentType: openchoreov1alpha1.ComponentTypeSpec{
					WorkloadType: "deployment",
				},
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
				ComponentType: openchoreov1alpha1.ComponentTypeSpec{
					WorkloadType: "deployment",
				},
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
				ComponentType: openchoreov1alpha1.ComponentTypeSpec{
					WorkloadType: "deployment",
				},
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
				ComponentType: openchoreov1alpha1.ComponentTypeSpec{
					WorkloadType: "deployment",
				},
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
		ComponentType: openchoreov1alpha1.ComponentTypeSpec{
			WorkloadType: "deployment",
		},
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
		ComponentType: openchoreov1alpha1.ComponentTypeSpec{
			WorkloadType: "deployment",
		},
		Workload: openchoreov1alpha1.WorkloadTemplateSpec{
			Container: openchoreov1alpha1.Container{
				Image: "nginx:1.21",
			},
		},
	}

	template2 := &ReleaseSpec{
		ComponentType: openchoreov1alpha1.ComponentTypeSpec{
			WorkloadType: "deployment",
		},
		Workload: openchoreov1alpha1.WorkloadTemplateSpec{
			Container: openchoreov1alpha1.Container{
				Image: "nginx:1.21",
			},
		},
	}

	template3 := &ReleaseSpec{
		ComponentType: openchoreov1alpha1.ComponentTypeSpec{
			WorkloadType: "deployment",
		},
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
				ComponentType: openchoreov1alpha1.ComponentTypeSpec{
					WorkloadType: "deployment",
				},
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
				ComponentType: openchoreov1alpha1.ComponentTypeSpec{
					WorkloadType: "deployment",
				},
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
				ComponentType: openchoreov1alpha1.ComponentTypeSpec{
					WorkloadType: "deployment",
				},
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

// TODO: Add tests for BuildReleaseSpec once implemented
