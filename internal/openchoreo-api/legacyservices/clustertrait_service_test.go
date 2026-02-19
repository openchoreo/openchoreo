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

func newCTService(t *testing.T, objects []client.Object, pdp authz.PDP) *ClusterTraitService {
	t.Helper()
	fakeClient := fake.NewClientBuilder().
		WithScheme(newTestScheme(t)).
		WithObjects(objects...).
		Build()
	return &ClusterTraitService{
		k8sClient: fakeClient,
		logger:    slog.Default(),
		authzPDP:  pdp,
	}
}

func TestListClusterTraits(t *testing.T) {
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
			name: "Single ClusterTrait returned",
			objects: []client.Object{
				&v1alpha1.ClusterTrait{
					ObjectMeta: metav1.ObjectMeta{
						Name: "autoscaler",
						Annotations: map[string]string{
							controller.AnnotationKeyDisplayName: "Auto Scaler",
							controller.AnnotationKeyDescription: "Enables HPA",
						},
					},
					Spec: v1alpha1.ClusterTraitSpec{},
				},
			},
			pdp:       &allowAllPDP{},
			wantCount: 1,
		},
		{
			name: "Multiple ClusterTraits returned",
			objects: []client.Object{
				&v1alpha1.ClusterTrait{
					ObjectMeta: metav1.ObjectMeta{Name: "autoscaler"},
					Spec:       v1alpha1.ClusterTraitSpec{},
				},
				&v1alpha1.ClusterTrait{
					ObjectMeta: metav1.ObjectMeta{Name: "logger"},
					Spec:       v1alpha1.ClusterTraitSpec{},
				},
				&v1alpha1.ClusterTrait{
					ObjectMeta: metav1.ObjectMeta{Name: "ingress"},
					Spec:       v1alpha1.ClusterTraitSpec{},
				},
			},
			pdp:       &allowAllPDP{},
			wantCount: 3,
		},
		{
			name: "Unauthorized items are filtered out",
			objects: []client.Object{
				&v1alpha1.ClusterTrait{
					ObjectMeta: metav1.ObjectMeta{Name: "autoscaler"},
					Spec:       v1alpha1.ClusterTraitSpec{},
				},
			},
			pdp:       &denyAllPDP{},
			wantCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := newCTService(t, tt.objects, tt.pdp)

			result, err := svc.ListClusterTraits(context.Background())
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(result) != tt.wantCount {
				t.Errorf("got %d items, want %d", len(result), tt.wantCount)
			}
		})
	}
}

func TestGetClusterTrait(t *testing.T) {
	tests := []struct {
		name         string
		traitName    string
		objects      []client.Object
		pdp          authz.PDP
		wantErr      bool
		wantNotFound bool
		wantName     string
	}{
		{
			name:      "Existing ClusterTrait returned",
			traitName: "autoscaler",
			objects: []client.Object{
				&v1alpha1.ClusterTrait{
					ObjectMeta: metav1.ObjectMeta{
						Name: "autoscaler",
						Annotations: map[string]string{
							controller.AnnotationKeyDisplayName: "Auto Scaler",
							controller.AnnotationKeyDescription: "Enables horizontal pod autoscaling",
						},
					},
					Spec: v1alpha1.ClusterTraitSpec{},
				},
			},
			pdp:      &allowAllPDP{},
			wantName: "autoscaler",
		},
		{
			name:         "Non-existent ClusterTrait returns not found",
			traitName:    "nonexistent",
			objects:      []client.Object{},
			pdp:          &allowAllPDP{},
			wantErr:      true,
			wantNotFound: true,
		},
		{
			name:      "Unauthorized access returns forbidden",
			traitName: "autoscaler",
			objects: []client.Object{
				&v1alpha1.ClusterTrait{
					ObjectMeta: metav1.ObjectMeta{Name: "autoscaler"},
					Spec:       v1alpha1.ClusterTraitSpec{},
				},
			},
			pdp:     &denyAllPDP{},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := newCTService(t, tt.objects, tt.pdp)

			result, err := svc.GetClusterTrait(context.Background(), tt.traitName)

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if tt.wantNotFound && !errors.Is(err, ErrClusterTraitNotFound) {
					t.Errorf("expected ErrClusterTraitNotFound, got: %v", err)
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

func TestGetClusterTraitSchema(t *testing.T) {
	paramsRaw, _ := json.Marshal(map[string]any{
		"minReplicas": "integer",
		"maxReplicas": "integer",
	})

	tests := []struct {
		name         string
		traitName    string
		objects      []client.Object
		pdp          authz.PDP
		wantErr      bool
		wantNotFound bool
		wantSchema   bool
	}{
		{
			name:      "Schema extracted from ClusterTrait with parameters",
			traitName: "autoscaler",
			objects: []client.Object{
				&v1alpha1.ClusterTrait{
					ObjectMeta: metav1.ObjectMeta{Name: "autoscaler"},
					Spec: v1alpha1.ClusterTraitSpec{
						Schema: v1alpha1.TraitSchema{
							Parameters: &runtime.RawExtension{Raw: paramsRaw},
						},
					},
				},
			},
			pdp:        &allowAllPDP{},
			wantSchema: true,
		},
		{
			name:      "Schema from ClusterTrait without parameters",
			traitName: "empty-trait",
			objects: []client.Object{
				&v1alpha1.ClusterTrait{
					ObjectMeta: metav1.ObjectMeta{Name: "empty-trait"},
					Spec:       v1alpha1.ClusterTraitSpec{},
				},
			},
			pdp:        &allowAllPDP{},
			wantSchema: true,
		},
		{
			name:         "Non-existent ClusterTrait returns not found",
			traitName:    "nonexistent",
			objects:      []client.Object{},
			pdp:          &allowAllPDP{},
			wantErr:      true,
			wantNotFound: true,
		},
		{
			name:      "Unauthorized access returns error",
			traitName: "autoscaler",
			objects: []client.Object{
				&v1alpha1.ClusterTrait{
					ObjectMeta: metav1.ObjectMeta{Name: "autoscaler"},
					Spec:       v1alpha1.ClusterTraitSpec{},
				},
			},
			pdp:     &denyAllPDP{},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := newCTService(t, tt.objects, tt.pdp)

			result, err := svc.GetClusterTraitSchema(context.Background(), tt.traitName)

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if tt.wantNotFound && !errors.Is(err, ErrClusterTraitNotFound) {
					t.Errorf("expected ErrClusterTraitNotFound, got: %v", err)
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

func TestClusterTraitToResponse(t *testing.T) {
	svc := &ClusterTraitService{logger: slog.Default()}

	tests := []struct {
		name            string
		trait           *v1alpha1.ClusterTrait
		wantName        string
		wantDisplayName string
		wantDescription string
	}{
		{
			name: "Full ClusterTrait with all annotations",
			trait: &v1alpha1.ClusterTrait{
				ObjectMeta: metav1.ObjectMeta{
					Name: "autoscaler",
					Annotations: map[string]string{
						controller.AnnotationKeyDisplayName: "Auto Scaler",
						controller.AnnotationKeyDescription: "Enables horizontal pod autoscaling",
					},
				},
			},
			wantName:        "autoscaler",
			wantDisplayName: "Auto Scaler",
			wantDescription: "Enables horizontal pod autoscaling",
		},
		{
			name: "ClusterTrait without annotations",
			trait: &v1alpha1.ClusterTrait{
				ObjectMeta: metav1.ObjectMeta{Name: "minimal"},
			},
			wantName:        "minimal",
			wantDisplayName: "",
			wantDescription: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := svc.toTraitResponse(tt.trait)

			if result.Name != tt.wantName {
				t.Errorf("Name = %q, want %q", result.Name, tt.wantName)
			}
			if result.DisplayName != tt.wantDisplayName {
				t.Errorf("DisplayName = %q, want %q", result.DisplayName, tt.wantDisplayName)
			}
			if result.Description != tt.wantDescription {
				t.Errorf("Description = %q, want %q", result.Description, tt.wantDescription)
			}
		})
	}
}
