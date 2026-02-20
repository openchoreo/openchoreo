// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package legacyservices

import (
	"context"
	"log/slog"
	"os"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/openchoreo/openchoreo/api/v1alpha1"
)

// TestGetEnvironmentObserverURL tests the GetEnvironmentObserverURL method
// covering both DataPlane and ClusterDataPlane resolution paths.
func TestGetEnvironmentObserverURL(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := v1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("Failed to add v1alpha1 to scheme: %v", err)
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	const (
		namespaceName = "test-ns"
		envName       = "development"
		observerURL   = "http://observer.test:8080"
	)

	tests := []struct {
		name            string
		objects         []client.Object
		wantObserverURL string
		wantMessage     string
		wantErr         bool
	}{
		{
			name: "ClusterDataPlane path - success with ClusterObservabilityPlane",
			objects: []client.Object{
				&v1alpha1.Environment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      envName,
						Namespace: namespaceName,
					},
					Spec: v1alpha1.EnvironmentSpec{
						DataPlaneRef: &v1alpha1.DataPlaneRef{
							Kind: v1alpha1.DataPlaneRefKindClusterDataPlane,
							Name: "shared-dp",
						},
					},
				},
				&v1alpha1.ClusterDataPlane{
					ObjectMeta: metav1.ObjectMeta{
						Name: "shared-dp",
					},
					Spec: v1alpha1.ClusterDataPlaneSpec{
						ObservabilityPlaneRef: &v1alpha1.ClusterObservabilityPlaneRef{
							Kind: v1alpha1.ClusterObservabilityPlaneRefKindClusterObservabilityPlane,
							Name: "shared-obs",
						},
					},
				},
				&v1alpha1.ClusterObservabilityPlane{
					ObjectMeta: metav1.ObjectMeta{
						Name: "shared-obs",
					},
					Spec: v1alpha1.ClusterObservabilityPlaneSpec{
						ObserverURL: observerURL,
					},
				},
			},
			wantObserverURL: observerURL,
		},
		{
			name: "ClusterDataPlane path - ClusterObservabilityPlane not found",
			objects: []client.Object{
				&v1alpha1.Environment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      envName,
						Namespace: namespaceName,
					},
					Spec: v1alpha1.EnvironmentSpec{
						DataPlaneRef: &v1alpha1.DataPlaneRef{
							Kind: v1alpha1.DataPlaneRefKindClusterDataPlane,
							Name: "shared-dp",
						},
					},
				},
				&v1alpha1.ClusterDataPlane{
					ObjectMeta: metav1.ObjectMeta{
						Name: "shared-dp",
					},
					Spec: v1alpha1.ClusterDataPlaneSpec{
						ObservabilityPlaneRef: &v1alpha1.ClusterObservabilityPlaneRef{
							Kind: v1alpha1.ClusterObservabilityPlaneRefKindClusterObservabilityPlane,
							Name: "nonexistent-obs",
						},
					},
				},
			},
			wantMessage: "observability-logs have not been configured",
		},
		{
			name: "DataPlane path - success with ObservabilityPlane",
			objects: []client.Object{
				&v1alpha1.Environment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      envName,
						Namespace: namespaceName,
					},
					Spec: v1alpha1.EnvironmentSpec{
						DataPlaneRef: &v1alpha1.DataPlaneRef{
							Kind: v1alpha1.DataPlaneRefKindDataPlane,
							Name: "local-dp",
						},
					},
				},
				&v1alpha1.DataPlane{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "local-dp",
						Namespace: namespaceName,
					},
					Spec: v1alpha1.DataPlaneSpec{
						ObservabilityPlaneRef: &v1alpha1.ObservabilityPlaneRef{
							Kind: v1alpha1.ObservabilityPlaneRefKindObservabilityPlane,
							Name: "local-obs",
						},
					},
				},
				&v1alpha1.ObservabilityPlane{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "local-obs",
						Namespace: namespaceName,
					},
					Spec: v1alpha1.ObservabilityPlaneSpec{
						ObserverURL: observerURL,
					},
				},
			},
			wantObserverURL: observerURL,
		},
		{
			name: "DataPlane path - ObservabilityPlane not found",
			objects: []client.Object{
				&v1alpha1.Environment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      envName,
						Namespace: namespaceName,
					},
					Spec: v1alpha1.EnvironmentSpec{
						DataPlaneRef: &v1alpha1.DataPlaneRef{
							Kind: v1alpha1.DataPlaneRefKindDataPlane,
							Name: "local-dp",
						},
					},
				},
				&v1alpha1.DataPlane{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "local-dp",
						Namespace: namespaceName,
					},
					Spec: v1alpha1.DataPlaneSpec{
						ObservabilityPlaneRef: &v1alpha1.ObservabilityPlaneRef{
							Kind: v1alpha1.ObservabilityPlaneRefKindObservabilityPlane,
							Name: "nonexistent-obs",
						},
					},
				},
			},
			wantMessage: "observability-logs have not been configured",
		},
		{
			name: "ClusterDataPlane not found returns error",
			objects: []client.Object{
				&v1alpha1.Environment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      envName,
						Namespace: namespaceName,
					},
					Spec: v1alpha1.EnvironmentSpec{
						DataPlaneRef: &v1alpha1.DataPlaneRef{
							Kind: v1alpha1.DataPlaneRefKindClusterDataPlane,
							Name: "missing-dp",
						},
					},
				},
				// No ClusterDataPlane object
			},
			wantErr: true,
		},
		{
			name: "DataPlane not found returns error",
			objects: []client.Object{
				&v1alpha1.Environment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      envName,
						Namespace: namespaceName,
					},
					Spec: v1alpha1.EnvironmentSpec{
						DataPlaneRef: &v1alpha1.DataPlaneRef{
							Kind: v1alpha1.DataPlaneRefKindDataPlane,
							Name: "missing-dp",
						},
					},
				},
				// No DataPlane object
			},
			wantErr: true,
		},
		{
			name: "No dataplane reference returns error",
			objects: []client.Object{
				&v1alpha1.Environment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      envName,
						Namespace: namespaceName,
					},
					Spec: v1alpha1.EnvironmentSpec{
						// No DataPlaneRef
					},
				},
			},
			wantErr: true,
		},
		{
			name: "ClusterDataPlane path - empty ObserverURL",
			objects: []client.Object{
				&v1alpha1.Environment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      envName,
						Namespace: namespaceName,
					},
					Spec: v1alpha1.EnvironmentSpec{
						DataPlaneRef: &v1alpha1.DataPlaneRef{
							Kind: v1alpha1.DataPlaneRefKindClusterDataPlane,
							Name: "shared-dp",
						},
					},
				},
				&v1alpha1.ClusterDataPlane{
					ObjectMeta: metav1.ObjectMeta{
						Name: "shared-dp",
					},
					Spec: v1alpha1.ClusterDataPlaneSpec{
						ObservabilityPlaneRef: &v1alpha1.ClusterObservabilityPlaneRef{
							Kind: v1alpha1.ClusterObservabilityPlaneRefKindClusterObservabilityPlane,
							Name: "shared-obs",
						},
					},
				},
				&v1alpha1.ClusterObservabilityPlane{
					ObjectMeta: metav1.ObjectMeta{
						Name: "shared-obs",
					},
					Spec: v1alpha1.ClusterObservabilityPlaneSpec{
						ObserverURL: "", // empty
					},
				},
			},
			wantMessage: "observability-logs have not been configured",
		},
		{
			name: "Unsupported DataPlaneRef kind returns error",
			objects: []client.Object{
				&v1alpha1.Environment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      envName,
						Namespace: namespaceName,
					},
					Spec: v1alpha1.EnvironmentSpec{
						DataPlaneRef: &v1alpha1.DataPlaneRef{
							Kind: "UnknownKind",
							Name: "some-dp",
						},
					},
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(tt.objects...).
				Build()

			service := &EnvironmentService{
				k8sClient: fakeClient,
				logger:    logger,
			}

			resp, err := service.GetEnvironmentObserverURL(context.Background(), namespaceName, envName)

			if tt.wantErr {
				if err == nil {
					t.Errorf("GetEnvironmentObserverURL() expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("GetEnvironmentObserverURL() unexpected error: %v", err)
			}

			if resp == nil {
				t.Fatal("GetEnvironmentObserverURL() returned nil response")
			}

			if tt.wantObserverURL != "" && resp.ObserverURL != tt.wantObserverURL {
				t.Errorf("GetEnvironmentObserverURL() ObserverURL = %q, want %q", resp.ObserverURL, tt.wantObserverURL)
			}

			if tt.wantMessage != "" && resp.Message != tt.wantMessage {
				t.Errorf("GetEnvironmentObserverURL() Message = %q, want %q", resp.Message, tt.wantMessage)
			}
		})
	}
}

// TestGetRCAAgentURL tests the GetRCAAgentURL method
// covering both DataPlane and ClusterDataPlane resolution paths.
func TestGetRCAAgentURL(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := v1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("Failed to add v1alpha1 to scheme: %v", err)
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	const (
		namespaceName = "test-ns"
		envName       = "development"
		rcaAgentURL   = "http://rca-agent.test:8080"
	)

	tests := []struct {
		name        string
		objects     []client.Object
		wantRCAURL  string
		wantMessage string
		wantErr     bool
	}{
		{
			name: "ClusterDataPlane path - success with ClusterObservabilityPlane",
			objects: []client.Object{
				&v1alpha1.Environment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      envName,
						Namespace: namespaceName,
					},
					Spec: v1alpha1.EnvironmentSpec{
						DataPlaneRef: &v1alpha1.DataPlaneRef{
							Kind: v1alpha1.DataPlaneRefKindClusterDataPlane,
							Name: "shared-dp",
						},
					},
				},
				&v1alpha1.ClusterDataPlane{
					ObjectMeta: metav1.ObjectMeta{
						Name: "shared-dp",
					},
					Spec: v1alpha1.ClusterDataPlaneSpec{
						ObservabilityPlaneRef: &v1alpha1.ClusterObservabilityPlaneRef{
							Kind: v1alpha1.ClusterObservabilityPlaneRefKindClusterObservabilityPlane,
							Name: "shared-obs",
						},
					},
				},
				&v1alpha1.ClusterObservabilityPlane{
					ObjectMeta: metav1.ObjectMeta{
						Name: "shared-obs",
					},
					Spec: v1alpha1.ClusterObservabilityPlaneSpec{
						RCAAgentURL: rcaAgentURL,
					},
				},
			},
			wantRCAURL: rcaAgentURL,
		},
		{
			name: "ClusterDataPlane path - ClusterObservabilityPlane not found",
			objects: []client.Object{
				&v1alpha1.Environment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      envName,
						Namespace: namespaceName,
					},
					Spec: v1alpha1.EnvironmentSpec{
						DataPlaneRef: &v1alpha1.DataPlaneRef{
							Kind: v1alpha1.DataPlaneRefKindClusterDataPlane,
							Name: "shared-dp",
						},
					},
				},
				&v1alpha1.ClusterDataPlane{
					ObjectMeta: metav1.ObjectMeta{
						Name: "shared-dp",
					},
					Spec: v1alpha1.ClusterDataPlaneSpec{
						ObservabilityPlaneRef: &v1alpha1.ClusterObservabilityPlaneRef{
							Kind: v1alpha1.ClusterObservabilityPlaneRefKindClusterObservabilityPlane,
							Name: "nonexistent-obs",
						},
					},
				},
			},
			wantMessage: "ObservabilityPlaneRef has not been configured",
		},
		{
			name: "DataPlane path - success with ObservabilityPlane",
			objects: []client.Object{
				&v1alpha1.Environment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      envName,
						Namespace: namespaceName,
					},
					Spec: v1alpha1.EnvironmentSpec{
						DataPlaneRef: &v1alpha1.DataPlaneRef{
							Kind: v1alpha1.DataPlaneRefKindDataPlane,
							Name: "local-dp",
						},
					},
				},
				&v1alpha1.DataPlane{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "local-dp",
						Namespace: namespaceName,
					},
					Spec: v1alpha1.DataPlaneSpec{
						ObservabilityPlaneRef: &v1alpha1.ObservabilityPlaneRef{
							Kind: v1alpha1.ObservabilityPlaneRefKindObservabilityPlane,
							Name: "local-obs",
						},
					},
				},
				&v1alpha1.ObservabilityPlane{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "local-obs",
						Namespace: namespaceName,
					},
					Spec: v1alpha1.ObservabilityPlaneSpec{
						RCAAgentURL: rcaAgentURL,
					},
				},
			},
			wantRCAURL: rcaAgentURL,
		},
		{
			name: "DataPlane path - ObservabilityPlane not found",
			objects: []client.Object{
				&v1alpha1.Environment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      envName,
						Namespace: namespaceName,
					},
					Spec: v1alpha1.EnvironmentSpec{
						DataPlaneRef: &v1alpha1.DataPlaneRef{
							Kind: v1alpha1.DataPlaneRefKindDataPlane,
							Name: "local-dp",
						},
					},
				},
				&v1alpha1.DataPlane{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "local-dp",
						Namespace: namespaceName,
					},
					Spec: v1alpha1.DataPlaneSpec{
						ObservabilityPlaneRef: &v1alpha1.ObservabilityPlaneRef{
							Kind: v1alpha1.ObservabilityPlaneRefKindObservabilityPlane,
							Name: "nonexistent-obs",
						},
					},
				},
			},
			wantMessage: "ObservabilityPlaneRef has not been configured",
		},
		{
			name: "ClusterDataPlane not found returns error",
			objects: []client.Object{
				&v1alpha1.Environment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      envName,
						Namespace: namespaceName,
					},
					Spec: v1alpha1.EnvironmentSpec{
						DataPlaneRef: &v1alpha1.DataPlaneRef{
							Kind: v1alpha1.DataPlaneRefKindClusterDataPlane,
							Name: "missing-dp",
						},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "DataPlane not found returns error",
			objects: []client.Object{
				&v1alpha1.Environment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      envName,
						Namespace: namespaceName,
					},
					Spec: v1alpha1.EnvironmentSpec{
						DataPlaneRef: &v1alpha1.DataPlaneRef{
							Kind: v1alpha1.DataPlaneRefKindDataPlane,
							Name: "missing-dp",
						},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "No dataplane reference returns error",
			objects: []client.Object{
				&v1alpha1.Environment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      envName,
						Namespace: namespaceName,
					},
					Spec: v1alpha1.EnvironmentSpec{},
				},
			},
			wantErr: true,
		},
		{
			name: "ClusterDataPlane path - empty RCAAgentURL",
			objects: []client.Object{
				&v1alpha1.Environment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      envName,
						Namespace: namespaceName,
					},
					Spec: v1alpha1.EnvironmentSpec{
						DataPlaneRef: &v1alpha1.DataPlaneRef{
							Kind: v1alpha1.DataPlaneRefKindClusterDataPlane,
							Name: "shared-dp",
						},
					},
				},
				&v1alpha1.ClusterDataPlane{
					ObjectMeta: metav1.ObjectMeta{
						Name: "shared-dp",
					},
					Spec: v1alpha1.ClusterDataPlaneSpec{
						ObservabilityPlaneRef: &v1alpha1.ClusterObservabilityPlaneRef{
							Kind: v1alpha1.ClusterObservabilityPlaneRefKindClusterObservabilityPlane,
							Name: "shared-obs",
						},
					},
				},
				&v1alpha1.ClusterObservabilityPlane{
					ObjectMeta: metav1.ObjectMeta{
						Name: "shared-obs",
					},
					Spec: v1alpha1.ClusterObservabilityPlaneSpec{
						RCAAgentURL: "", // empty
					},
				},
			},
			wantMessage: "RCAAgentURL has not been configured",
		},
		{
			name: "Unsupported DataPlaneRef kind returns error",
			objects: []client.Object{
				&v1alpha1.Environment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      envName,
						Namespace: namespaceName,
					},
					Spec: v1alpha1.EnvironmentSpec{
						DataPlaneRef: &v1alpha1.DataPlaneRef{
							Kind: "UnknownKind",
							Name: "some-dp",
						},
					},
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(tt.objects...).
				Build()

			service := &EnvironmentService{
				k8sClient: fakeClient,
				logger:    logger,
			}

			resp, err := service.GetRCAAgentURL(context.Background(), namespaceName, envName)

			if tt.wantErr {
				if err == nil {
					t.Errorf("GetRCAAgentURL() expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("GetRCAAgentURL() unexpected error: %v", err)
			}

			if resp == nil {
				t.Fatal("GetRCAAgentURL() returned nil response")
			}

			if tt.wantRCAURL != "" && resp.RCAAgentURL != tt.wantRCAURL {
				t.Errorf("GetRCAAgentURL() RCAAgentURL = %q, want %q", resp.RCAAgentURL, tt.wantRCAURL)
			}

			if tt.wantMessage != "" && resp.Message != tt.wantMessage {
				t.Errorf("GetRCAAgentURL() Message = %q, want %q", resp.Message, tt.wantMessage)
			}
		})
	}
}
