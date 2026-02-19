// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package mcphandlers

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
	"github.com/openchoreo/openchoreo/internal/controller"
	services "github.com/openchoreo/openchoreo/internal/openchoreo-api/legacyservices"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/models"
)

func newCTHandler(t *testing.T, objects []client.Object) *MCPHandler {
	t.Helper()
	fakeClient := fake.NewClientBuilder().
		WithScheme(newTestScheme(t)).
		WithObjects(objects...).
		Build()
	return &MCPHandler{
		Services: &services.Services{
			ClusterTraitService: services.NewClusterTraitService(
				fakeClient, slog.Default(), &allowAllPDP{},
			),
		},
	}
}

func TestMCPListClusterTraits(t *testing.T) {
	tests := []struct {
		name      string
		objects   []client.Object
		wantCount int
		wantErr   bool
	}{
		{
			name:      "Empty list returns empty response",
			objects:   []client.Object{},
			wantCount: 0,
		},
		{
			name: "Single ClusterTrait returned in response",
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
					ObjectMeta: metav1.ObjectMeta{Name: "ingress"},
					Spec:       v1alpha1.ClusterTraitSpec{},
				},
				&v1alpha1.ClusterTrait{
					ObjectMeta: metav1.ObjectMeta{Name: "logger"},
					Spec:       v1alpha1.ClusterTraitSpec{},
				},
			},
			wantCount: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newCTHandler(t, tt.objects)

			result, err := h.ListClusterTraits(context.Background())

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			resp, ok := result.(ListClusterTraitsResponse)
			if !ok {
				t.Fatalf("expected ListClusterTraitsResponse, got %T", result)
			}
			if len(resp.ClusterTraits) != tt.wantCount {
				t.Errorf("got %d items, want %d", len(resp.ClusterTraits), tt.wantCount)
			}
		})
	}
}

func TestMCPListClusterTraits_ResponseFields(t *testing.T) {
	h := newCTHandler(t, []client.Object{
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
	})

	result, err := h.ListClusterTraits(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	resp := result.(ListClusterTraitsResponse)
	if len(resp.ClusterTraits) != 1 {
		t.Fatalf("expected 1 trait, got %d", len(resp.ClusterTraits))
	}

	trait := resp.ClusterTraits[0]
	if trait.Name != "autoscaler" {
		t.Errorf("Name = %q, want %q", trait.Name, "autoscaler")
	}
	if trait.DisplayName != "Auto Scaler" {
		t.Errorf("DisplayName = %q, want %q", trait.DisplayName, "Auto Scaler")
	}
	if trait.Description != "Enables HPA" {
		t.Errorf("Description = %q, want %q", trait.Description, "Enables HPA")
	}
}

func TestMCPGetClusterTrait(t *testing.T) {
	tests := []struct {
		name     string
		ctName   string
		objects  []client.Object
		wantErr  bool
		wantName string
	}{
		{
			name:   "Existing ClusterTrait returned",
			ctName: "autoscaler",
			objects: []client.Object{
				&v1alpha1.ClusterTrait{
					ObjectMeta: metav1.ObjectMeta{
						Name: "autoscaler",
						Annotations: map[string]string{
							controller.AnnotationKeyDisplayName: "Auto Scaler",
						},
					},
					Spec: v1alpha1.ClusterTraitSpec{},
				},
			},
			wantName: "autoscaler",
		},
		{
			name:    "Non-existent ClusterTrait returns wrapped error",
			ctName:  "nonexistent",
			objects: []client.Object{},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newCTHandler(t, tt.objects)

			result, err := h.GetClusterTrait(context.Background(), tt.ctName)

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if !errors.Is(err, services.ErrClusterTraitNotFound) {
					t.Fatalf("expected ErrClusterTraitNotFound, got: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			resp, ok := result.(*models.TraitResponse)
			if !ok {
				t.Fatalf("expected *models.TraitResponse, got %T", result)
			}
			if resp.Name != tt.wantName {
				t.Errorf("got name %q, want %q", resp.Name, tt.wantName)
			}
		})
	}
}

func TestMCPGetClusterTraitSchema(t *testing.T) {
	paramsRaw, _ := json.Marshal(map[string]any{
		"minReplicas": "integer",
		"maxReplicas": "integer",
	})

	tests := []struct {
		name       string
		ctName     string
		objects    []client.Object
		wantErr    bool
		wantSchema bool
	}{
		{
			name:   "Schema returned for existing ClusterTrait",
			ctName: "autoscaler",
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
			wantSchema: true,
		},
		{
			name:   "Schema returned for ClusterTrait without parameters",
			ctName: "empty-trait",
			objects: []client.Object{
				&v1alpha1.ClusterTrait{
					ObjectMeta: metav1.ObjectMeta{Name: "empty-trait"},
					Spec:       v1alpha1.ClusterTraitSpec{},
				},
			},
			wantSchema: true,
		},
		{
			name:    "Non-existent ClusterTrait returns wrapped error",
			ctName:  "nonexistent",
			objects: []client.Object{},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newCTHandler(t, tt.objects)

			result, err := h.GetClusterTraitSchema(context.Background(), tt.ctName)

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if !errors.Is(err, services.ErrClusterTraitNotFound) {
					t.Fatalf("expected ErrClusterTraitNotFound, got: %v", err)
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
