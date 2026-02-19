// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package legacyservices

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/openchoreo/openchoreo/api/v1alpha1"
	authz "github.com/openchoreo/openchoreo/internal/authz/core"
	"github.com/openchoreo/openchoreo/internal/controller"
)

func newCCTService(t *testing.T, objects []client.Object, pdp authz.PDP) *ClusterComponentTypeService {
	t.Helper()
	fakeClient := fake.NewClientBuilder().
		WithScheme(newTestScheme(t)).
		WithObjects(objects...).
		Build()
	return &ClusterComponentTypeService{
		k8sClient: fakeClient,
		logger:    slog.Default(),
		authzPDP:  pdp,
	}
}

func TestListClusterComponentTypes(t *testing.T) {
	tests := []struct {
		name      string
		objects   []client.Object
		pdp       authz.PDP
		wantCount int
	}{
		{
			name:      "Empty list returns no items",
			objects:   []client.Object{},
			pdp:       &allowAllPDP{},
			wantCount: 0,
		},
		{
			name: "Single ClusterComponentType returned",
			objects: []client.Object{
				&v1alpha1.ClusterComponentType{
					ObjectMeta: metav1.ObjectMeta{
						Name: "go-service",
						Annotations: map[string]string{
							controller.AnnotationKeyDisplayName: "Go Service",
							controller.AnnotationKeyDescription: "Go microservice template",
						},
					},
					Spec: v1alpha1.ClusterComponentTypeSpec{
						WorkloadType: "deployment",
						Resources:    []v1alpha1.ResourceTemplate{{ID: "deployment"}},
					},
				},
			},
			pdp:       &allowAllPDP{},
			wantCount: 1,
		},
		{
			name: "Multiple ClusterComponentTypes returned",
			objects: []client.Object{
				&v1alpha1.ClusterComponentType{
					ObjectMeta: metav1.ObjectMeta{Name: "go-service"},
					Spec: v1alpha1.ClusterComponentTypeSpec{
						WorkloadType: "deployment",
						Resources:    []v1alpha1.ResourceTemplate{{ID: "deployment"}},
					},
				},
				&v1alpha1.ClusterComponentType{
					ObjectMeta: metav1.ObjectMeta{Name: "python-job"},
					Spec: v1alpha1.ClusterComponentTypeSpec{
						WorkloadType: "job",
						Resources:    []v1alpha1.ResourceTemplate{{ID: "job"}},
					},
				},
			},
			pdp:       &allowAllPDP{},
			wantCount: 2,
		},
		{
			name: "Unauthorized items are filtered out",
			objects: []client.Object{
				&v1alpha1.ClusterComponentType{
					ObjectMeta: metav1.ObjectMeta{Name: "go-service"},
					Spec: v1alpha1.ClusterComponentTypeSpec{
						WorkloadType: "deployment",
						Resources:    []v1alpha1.ResourceTemplate{{ID: "deployment"}},
					},
				},
			},
			pdp:       &denyAllPDP{},
			wantCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := newCCTService(t, tt.objects, tt.pdp)

			result, err := svc.ListClusterComponentTypes(context.Background())
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(result) != tt.wantCount {
				t.Errorf("got %d items, want %d", len(result), tt.wantCount)
			}
		})
	}
}

func TestGetClusterComponentType(t *testing.T) {
	tests := []struct {
		name         string
		ctName       string
		objects      []client.Object
		pdp          authz.PDP
		wantErr      bool
		wantNotFound bool
		wantName     string
	}{
		{
			name:   "Existing ClusterComponentType returned",
			ctName: "go-service",
			objects: []client.Object{
				&v1alpha1.ClusterComponentType{
					ObjectMeta: metav1.ObjectMeta{
						Name: "go-service",
						Annotations: map[string]string{
							controller.AnnotationKeyDisplayName: "Go Service",
							controller.AnnotationKeyDescription: "Go microservice",
						},
					},
					Spec: v1alpha1.ClusterComponentTypeSpec{
						WorkloadType: "deployment",
						Resources:    []v1alpha1.ResourceTemplate{{ID: "deployment"}},
					},
				},
			},
			pdp:      &allowAllPDP{},
			wantName: "go-service",
		},
		{
			name:         "Non-existent ClusterComponentType returns not found",
			ctName:       "nonexistent",
			objects:      []client.Object{},
			pdp:          &allowAllPDP{},
			wantErr:      true,
			wantNotFound: true,
		},
		{
			name:   "Unauthorized access returns forbidden",
			ctName: "go-service",
			objects: []client.Object{
				&v1alpha1.ClusterComponentType{
					ObjectMeta: metav1.ObjectMeta{Name: "go-service"},
					Spec: v1alpha1.ClusterComponentTypeSpec{
						WorkloadType: "deployment",
						Resources:    []v1alpha1.ResourceTemplate{{ID: "deployment"}},
					},
				},
			},
			pdp:     &denyAllPDP{},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := newCCTService(t, tt.objects, tt.pdp)

			result, err := svc.GetClusterComponentType(context.Background(), tt.ctName)

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if tt.wantNotFound && !errors.Is(err, ErrClusterComponentTypeNotFound) {
					t.Errorf("expected ErrClusterComponentTypeNotFound, got: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result.Name != tt.wantName {
				t.Errorf("got name %q, want %q", result.Name, tt.wantName)
			}
		})
	}
}

func TestGetClusterComponentTypeSchema(t *testing.T) {
	paramsRaw, _ := json.Marshal(map[string]any{
		"replicas": "integer",
	})

	tests := []struct {
		name         string
		ctName       string
		objects      []client.Object
		pdp          authz.PDP
		wantErr      bool
		wantNotFound bool
		wantSchema   bool
	}{
		{
			name:   "Schema extracted from ClusterComponentType with parameters",
			ctName: "go-service",
			objects: []client.Object{
				&v1alpha1.ClusterComponentType{
					ObjectMeta: metav1.ObjectMeta{Name: "go-service"},
					Spec: v1alpha1.ClusterComponentTypeSpec{
						WorkloadType: "deployment",
						Resources:    []v1alpha1.ResourceTemplate{{ID: "deployment"}},
						Schema: v1alpha1.ComponentTypeSchema{
							Parameters: &runtime.RawExtension{Raw: paramsRaw},
						},
					},
				},
			},
			pdp:        &allowAllPDP{},
			wantSchema: true,
		},
		{
			name:   "Schema from ClusterComponentType without parameters",
			ctName: "empty-ct",
			objects: []client.Object{
				&v1alpha1.ClusterComponentType{
					ObjectMeta: metav1.ObjectMeta{Name: "empty-ct"},
					Spec: v1alpha1.ClusterComponentTypeSpec{
						WorkloadType: "deployment",
						Resources:    []v1alpha1.ResourceTemplate{{ID: "deployment"}},
					},
				},
			},
			pdp:        &allowAllPDP{},
			wantSchema: true,
		},
		{
			name:         "Non-existent ClusterComponentType returns not found",
			ctName:       "nonexistent",
			objects:      []client.Object{},
			pdp:          &allowAllPDP{},
			wantErr:      true,
			wantNotFound: true,
		},
		{
			name:   "Unauthorized access returns error",
			ctName: "go-service",
			objects: []client.Object{
				&v1alpha1.ClusterComponentType{
					ObjectMeta: metav1.ObjectMeta{Name: "go-service"},
					Spec: v1alpha1.ClusterComponentTypeSpec{
						WorkloadType: "deployment",
						Resources:    []v1alpha1.ResourceTemplate{{ID: "deployment"}},
					},
				},
			},
			pdp:     &denyAllPDP{},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := newCCTService(t, tt.objects, tt.pdp)

			result, err := svc.GetClusterComponentTypeSchema(context.Background(), tt.ctName)

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if tt.wantNotFound && !errors.Is(err, ErrClusterComponentTypeNotFound) {
					t.Errorf("expected ErrClusterComponentTypeNotFound, got: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.wantSchema && result == nil {
				t.Fatal("expected non-nil schema, got nil")
			}
		})
	}
}

func TestClusterComponentTypeToResponse(t *testing.T) {
	svc := &ClusterComponentTypeService{logger: slog.Default()}

	tests := []struct {
		name              string
		ct                *v1alpha1.ClusterComponentType
		wantName          string
		wantDisplayName   string
		wantDescription   string
		wantWorkloadType  string
		wantWorkflowCount int
		wantTraitCount    int
	}{
		{
			name: "Full ClusterComponentType with all fields",
			ct: &v1alpha1.ClusterComponentType{
				ObjectMeta: metav1.ObjectMeta{
					Name: "go-service",
					Annotations: map[string]string{
						controller.AnnotationKeyDisplayName: "Go Service",
						controller.AnnotationKeyDescription: "A Go microservice template",
					},
				},
				Spec: v1alpha1.ClusterComponentTypeSpec{
					WorkloadType:     "deployment",
					AllowedWorkflows: []string{"docker-build", "buildpack-build"},
					AllowedTraits: []v1alpha1.ClusterTraitRef{
						{Kind: v1alpha1.ClusterTraitRefKindClusterTrait, Name: "autoscaler"},
						{Kind: v1alpha1.ClusterTraitRefKindClusterTrait, Name: "logger"},
					},
					Resources: []v1alpha1.ResourceTemplate{{ID: "deployment"}},
				},
			},
			wantName:          "go-service",
			wantDisplayName:   "Go Service",
			wantDescription:   "A Go microservice template",
			wantWorkloadType:  "deployment",
			wantWorkflowCount: 2,
			wantTraitCount:    2,
		},
		{
			name: "Minimal ClusterComponentType without optional fields",
			ct: &v1alpha1.ClusterComponentType{
				ObjectMeta: metav1.ObjectMeta{Name: "minimal"},
				Spec: v1alpha1.ClusterComponentTypeSpec{
					WorkloadType: "job",
					Resources:    []v1alpha1.ResourceTemplate{{ID: "job"}},
				},
			},
			wantName:          "minimal",
			wantDisplayName:   "",
			wantDescription:   "",
			wantWorkloadType:  "job",
			wantWorkflowCount: 0,
			wantTraitCount:    0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := svc.toComponentTypeResponse(tt.ct)

			if result.Name != tt.wantName {
				t.Errorf("Name = %q, want %q", result.Name, tt.wantName)
			}
			if result.DisplayName != tt.wantDisplayName {
				t.Errorf("DisplayName = %q, want %q", result.DisplayName, tt.wantDisplayName)
			}
			if result.Description != tt.wantDescription {
				t.Errorf("Description = %q, want %q", result.Description, tt.wantDescription)
			}
			if result.WorkloadType != tt.wantWorkloadType {
				t.Errorf("WorkloadType = %q, want %q", result.WorkloadType, tt.wantWorkloadType)
			}
			if len(result.AllowedWorkflows) != tt.wantWorkflowCount {
				t.Errorf("AllowedWorkflows count = %d, want %d", len(result.AllowedWorkflows), tt.wantWorkflowCount)
			}
			if len(result.AllowedTraits) != tt.wantTraitCount {
				t.Errorf("AllowedTraits count = %d, want %d", len(result.AllowedTraits), tt.wantTraitCount)
			}
			for i, trait := range result.AllowedTraits {
				if trait.Kind != string(v1alpha1.ClusterTraitRefKindClusterTrait) {
					t.Errorf("AllowedTraits[%d].Kind = %q, want %q", i, trait.Kind, v1alpha1.ClusterTraitRefKindClusterTrait)
				}
			}
		})
	}
}
