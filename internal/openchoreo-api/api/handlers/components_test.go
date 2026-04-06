// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	authzcore "github.com/openchoreo/openchoreo/internal/authz/core"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
	svcpkg "github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	componentsvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/component"
	componentsvcmocks "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/component/mocks"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/handlerservices"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/testutil"
)

func TestToModelCreateComponentRequest(t *testing.T) {
	tests := []struct {
		name              string
		input             *gen.CreateComponentRequest
		wantNil           bool
		wantComponentType bool
		wantKind          string
		wantCTName        string
	}{
		{
			name:    "Nil input returns nil",
			input:   nil,
			wantNil: true,
		},
		{
			name: "Non-nil input with ComponentType string",
			input: &gen.CreateComponentRequest{
				Name:          "my-comp",
				ComponentType: ptr.To("deployment/web-app"),
			},
			wantNil:           false,
			wantComponentType: true,
			wantKind:          "ComponentType",
			wantCTName:        "deployment/web-app",
		},
		{
			name: "Non-nil input with nil ComponentType",
			input: &gen.CreateComponentRequest{
				Name: "my-comp",
			},
			wantNil:           false,
			wantComponentType: false,
		},
		{
			name: "Non-nil input with empty string ComponentType",
			input: &gen.CreateComponentRequest{
				Name:          "my-comp",
				ComponentType: ptr.To(""),
			},
			wantNil:           false,
			wantComponentType: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := toModelCreateComponentRequest(tt.input)

			if tt.wantNil {
				if result != nil {
					t.Errorf("toModelCreateComponentRequest() = %v, want nil", result)
				}
				return
			}

			if result == nil {
				t.Fatal("toModelCreateComponentRequest() returned nil, want non-nil")
			}

			if tt.wantComponentType {
				if result.ComponentType == nil {
					t.Fatal("ComponentType is nil, want non-nil")
				}
				if result.ComponentType.Kind != tt.wantKind {
					t.Errorf("ComponentType.Kind = %q, want %q", result.ComponentType.Kind, tt.wantKind)
				}
				if result.ComponentType.Name != tt.wantCTName {
					t.Errorf("ComponentType.Name = %q, want %q", result.ComponentType.Name, tt.wantCTName)
				}
			} else {
				if result.ComponentType != nil {
					t.Errorf("ComponentType = %v, want nil", result.ComponentType)
				}
			}
		})
	}
}

func TestToModelTraits(t *testing.T) {
	traitKind := gen.ComponentTraitInputKindTrait
	clusterTraitKind := gen.ComponentTraitInputKindClusterTrait

	tests := []struct {
		name      string
		input     *[]gen.ComponentTraitInput
		wantNil   bool
		wantCount int
		wantKinds []string
		wantNames []string
	}{
		{
			name:    "Nil input returns nil",
			input:   nil,
			wantNil: true,
		},
		{
			name:    "Empty slice returns nil",
			input:   &[]gen.ComponentTraitInput{},
			wantNil: true,
		},
		{
			name: "Traits without kind default to empty string",
			input: &[]gen.ComponentTraitInput{
				{Name: "logging", InstanceName: "app-logging"},
			},
			wantCount: 1,
			wantKinds: []string{""},
			wantNames: []string{"logging"},
		},
		{
			name: "Trait with kind=Trait",
			input: &[]gen.ComponentTraitInput{
				{Name: "logging", InstanceName: "app-logging", Kind: &traitKind},
			},
			wantCount: 1,
			wantKinds: []string{"Trait"},
			wantNames: []string{"logging"},
		},
		{
			name: "Trait with kind=ClusterTrait",
			input: &[]gen.ComponentTraitInput{
				{Name: "global-logger", InstanceName: "my-logger", Kind: &clusterTraitKind},
			},
			wantCount: 1,
			wantKinds: []string{"ClusterTrait"},
			wantNames: []string{"global-logger"},
		},
		{
			name: "Mixed kinds",
			input: &[]gen.ComponentTraitInput{
				{Name: "logging", InstanceName: "app-logging", Kind: &traitKind},
				{Name: "global-logger", InstanceName: "my-logger", Kind: &clusterTraitKind},
				{Name: "autoscaler", InstanceName: "my-scaler"},
			},
			wantCount: 3,
			wantKinds: []string{"Trait", "ClusterTrait", ""},
			wantNames: []string{"logging", "global-logger", "autoscaler"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := toModelTraits(tt.input)

			if tt.wantNil {
				if result != nil {
					t.Errorf("toModelTraits() = %v, want nil", result)
				}
				return
			}

			if len(result) != tt.wantCount {
				t.Fatalf("toModelTraits() returned %d traits, want %d", len(result), tt.wantCount)
			}

			for i, trait := range result {
				if trait.Kind != tt.wantKinds[i] {
					t.Errorf("trait[%d].Kind = %q, want %q", i, trait.Kind, tt.wantKinds[i])
				}
				if trait.Name != tt.wantNames[i] {
					t.Errorf("trait[%d].Name = %q, want %q", i, trait.Name, tt.wantNames[i])
				}
			}
		})
	}
}

// --- ListComponents Handler ---

func newComponentService(t *testing.T, objects []client.Object, pdp authzcore.PDP) componentsvc.Service {
	t.Helper()
	return componentsvc.NewServiceWithAuthz(testutil.NewFakeClient(objects...), pdp, testutil.TestLogger())
}

func newHandlerWithComponentService(svc componentsvc.Service) *Handler {
	return &Handler{
		services: &handlerservices.Services{ComponentService: svc},
		logger:   slog.Default(),
	}
}

func testComponentObj(projectName, name string) *openchoreov1alpha1.Component {
	return &openchoreov1alpha1.Component{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "test-ns"},
		Spec: openchoreov1alpha1.ComponentSpec{
			Owner:         openchoreov1alpha1.ComponentOwner{ProjectName: projectName},
			ComponentType: openchoreov1alpha1.ComponentTypeRef{Name: "deployment/web-app"},
		},
	}
}

func TestListComponentsHandler(t *testing.T) {
	ctx := testContext()

	const (
		ns      = "test-ns"
		projA   = "proj-a"
		projB   = "proj-b"
		compA   = "comp-a"
		compB   = "comp-b"
		pipeDef = "default"
	)

	projectA := &openchoreov1alpha1.Project{
		ObjectMeta: metav1.ObjectMeta{Name: projA, Namespace: ns},
		Spec:       openchoreov1alpha1.ProjectSpec{DeploymentPipelineRef: openchoreov1alpha1.DeploymentPipelineRef{Name: pipeDef}},
	}
	projectB := &openchoreov1alpha1.Project{
		ObjectMeta: metav1.ObjectMeta{Name: projB, Namespace: ns},
		Spec:       openchoreov1alpha1.ProjectSpec{DeploymentPipelineRef: openchoreov1alpha1.DeploymentPipelineRef{Name: pipeDef}},
	}

	t.Run("success - returns items", func(t *testing.T) {
		svc := newComponentService(t, []client.Object{projectA, testComponentObj(projA, compA)}, &allowAllPDP{})
		h := newHandlerWithComponentService(svc)

		resp, err := h.ListComponents(ctx, gen.ListComponentsRequestObject{NamespaceName: ns})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		typed, ok := resp.(gen.ListComponents200JSONResponse)
		if !ok {
			t.Fatalf("unexpected response type: %T", resp)
		}
		if len(typed.Items) != 1 {
			t.Fatalf("expected 1 item, got %d", len(typed.Items))
		}
	})

	t.Run("success - with project filter", func(t *testing.T) {
		objs := []client.Object{projectA, projectB, testComponentObj(projA, compA), testComponentObj(projB, compB)}
		svc := newComponentService(t, objs, &allowAllPDP{})
		h := newHandlerWithComponentService(svc)

		resp, err := h.ListComponents(ctx, gen.ListComponentsRequestObject{
			NamespaceName: ns,
			Params:        gen.ListComponentsParams{Project: ptr.To(projA)},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		typed, ok := resp.(gen.ListComponents200JSONResponse)
		if !ok {
			t.Fatalf("unexpected response type: %T", resp)
		}
		if len(typed.Items) != 1 {
			t.Fatalf("expected 1 item, got %d", len(typed.Items))
		}
	})

	t.Run("project not found returns 404", func(t *testing.T) {
		svc := newComponentService(t, nil, &allowAllPDP{})
		h := newHandlerWithComponentService(svc)

		resp, err := h.ListComponents(ctx, gen.ListComponentsRequestObject{
			NamespaceName: ns,
			Params:        gen.ListComponentsParams{Project: ptr.To("nonexistent")},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if _, ok := resp.(gen.ListComponents404JSONResponse); !ok {
			t.Fatalf("expected 404 response, got %T", resp)
		}
	})

	t.Run("validation error returns 400", func(t *testing.T) {
		svc := newComponentService(t, []client.Object{projectA}, &allowAllPDP{})
		h := newHandlerWithComponentService(svc)

		resp, err := h.ListComponents(ctx, gen.ListComponentsRequestObject{
			NamespaceName: ns,
			Params:        gen.ListComponentsParams{LabelSelector: ptr.To("===invalid")},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if _, ok := resp.(gen.ListComponents400JSONResponse); !ok {
			t.Fatalf("expected 400 response, got %T", resp)
		}
	})

	t.Run("empty list returns 200 with no items", func(t *testing.T) {
		svc := newComponentService(t, []client.Object{projectA}, &allowAllPDP{})
		h := newHandlerWithComponentService(svc)

		resp, err := h.ListComponents(ctx, gen.ListComponentsRequestObject{
			NamespaceName: ns,
			Params:        gen.ListComponentsParams{Project: ptr.To(projA)},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		typed, ok := resp.(gen.ListComponents200JSONResponse)
		if !ok {
			t.Fatalf("unexpected response type: %T", resp)
		}
		if len(typed.Items) != 0 {
			t.Fatalf("expected 0 items, got %d", len(typed.Items))
		}
	})

	t.Run("unauthorized items filtered out", func(t *testing.T) {
		svc := newComponentService(t, []client.Object{projectA, testComponentObj(projA, compA)}, &denyAllPDP{})
		h := newHandlerWithComponentService(svc)

		resp, err := h.ListComponents(ctx, gen.ListComponentsRequestObject{NamespaceName: ns})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		typed, ok := resp.(gen.ListComponents200JSONResponse)
		if !ok {
			t.Fatalf("unexpected response type: %T", resp)
		}
		if len(typed.Items) != 0 {
			t.Fatalf("expected 0 items (authz denied), got %d", len(typed.Items))
		}
	})
}

func TestCreateComponentHandler_NamespaceMismatchReturns400(t *testing.T) {
	ctx := testContext()
	svc := componentsvcmocks.NewMockService(t)
	h := &Handler{
		services: &handlerservices.Services{ComponentService: svc},
		logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	body := gen.Component{
		Metadata: gen.ObjectMeta{
			Name:      "comp-a",
			Namespace: ptr.To("other-ns"),
		},
	}
	resp, err := h.CreateComponent(ctx, gen.CreateComponentRequestObject{
		NamespaceName: "test-ns",
		Body:          &body,
	})
	require.NoError(t, err)
	assert.IsType(t, gen.CreateComponent400JSONResponse{}, resp)
}

func TestUpdateComponentHandler_UsesPathNameAndValidatesNamespace(t *testing.T) {
	ctx := testContext()

	t.Run("uses path name", func(t *testing.T) {
		svc := componentsvcmocks.NewMockService(t)
		svc.EXPECT().UpdateComponent(mock.Anything, mock.Anything, mock.Anything).RunAndReturn(func(_ context.Context, namespace string, component *openchoreov1alpha1.Component) (*openchoreov1alpha1.Component, error) {
			assert.Equal(t, "test-ns", namespace)
			assert.Equal(t, "comp-from-path", component.Name, "path must override body name")
			return component, nil
		})
		h := &Handler{
			services: &handlerservices.Services{ComponentService: svc},
			logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
		}

		body := gen.Component{
			Metadata: gen.ObjectMeta{Name: "comp-from-body"},
		}
		resp, err := h.UpdateComponent(ctx, gen.UpdateComponentRequestObject{
			NamespaceName: "test-ns",
			ComponentName: "comp-from-path",
			Body:          &body,
		})
		require.NoError(t, err)
		typed, ok := resp.(gen.UpdateComponent200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
		assert.Equal(t, "comp-from-path", typed.Metadata.Name)
	})

	t.Run("namespace mismatch returns 400", func(t *testing.T) {
		svc := componentsvcmocks.NewMockService(t)
		h := &Handler{
			services: &handlerservices.Services{ComponentService: svc},
			logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
		}

		body := gen.Component{
			Metadata: gen.ObjectMeta{
				Name:      "comp-from-body",
				Namespace: ptr.To("other-ns"),
			},
		}
		resp, err := h.UpdateComponent(ctx, gen.UpdateComponentRequestObject{
			NamespaceName: "test-ns",
			ComponentName: "comp-from-path",
			Body:          &body,
		})
		require.NoError(t, err)
		assert.IsType(t, gen.UpdateComponent400JSONResponse{}, resp)
	})
}

func TestGenerateReleaseHandler_MapsValidationErrorTo400AndForwardsReleaseName(t *testing.T) {
	ctx := testContext()
	svc := componentsvcmocks.NewMockService(t)
	svc.EXPECT().GenerateRelease(mock.Anything, mock.Anything, mock.Anything, mock.Anything).RunAndReturn(func(_ context.Context, namespace, componentName string, req *componentsvc.GenerateReleaseRequest) (*openchoreov1alpha1.ComponentRelease, error) {
		assert.Equal(t, "test-ns", namespace)
		assert.Equal(t, "comp-a", componentName)
		require.NotNil(t, req)
		assert.Equal(t, "r-1", req.ReleaseName)
		return nil, componentsvc.ErrValidation
	})
	h := &Handler{
		services: &handlerservices.Services{ComponentService: svc},
		logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	rn := "r-1"
	resp, err := h.GenerateRelease(ctx, gen.GenerateReleaseRequestObject{
		NamespaceName: "test-ns",
		ComponentName: "comp-a",
		Body:          &gen.GenerateReleaseRequest{ReleaseName: &rn},
	})
	require.NoError(t, err)
	typed, ok := resp.(gen.GenerateRelease400JSONResponse)
	require.True(t, ok, "expected 400 response, got %T", resp)
	assert.Contains(t, typed.Error, "validation")
}

func TestGetComponentSchemaHandler_MapsErrors(t *testing.T) {
	ctx := testContext()

	tests := []struct {
		name    string
		svcErr  error
		wantTyp any
	}{
		{"forbidden -> 403", svcpkg.ErrForbidden, gen.GetComponentSchema403JSONResponse{}},
		{"component not found -> 404", componentsvc.ErrComponentNotFound, gen.GetComponentSchema404JSONResponse{}},
		{"component type not found -> 404", componentsvc.ErrComponentTypeNotFound, gen.GetComponentSchema404JSONResponse{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := componentsvcmocks.NewMockService(t)
			svc.EXPECT().GetComponentSchema(mock.Anything, mock.Anything, mock.Anything).Return(nil, tt.svcErr)
			h := &Handler{
				services: &handlerservices.Services{ComponentService: svc},
				logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
			}
			resp, err := h.GetComponentSchema(ctx, gen.GetComponentSchemaRequestObject{
				NamespaceName: "test-ns",
				ComponentName: "comp-a",
			})
			require.NoError(t, err)
			assert.IsType(t, tt.wantTyp, resp)
		})
	}
}

func TestUpdateComponentHandler_MapsErrors(t *testing.T) {
	ctx := testContext()

	tests := []struct {
		name    string
		svcErr  error
		wantTyp any
	}{
		{"forbidden -> 403", svcpkg.ErrForbidden, gen.UpdateComponent403JSONResponse{}},
		{"not found -> 404", componentsvc.ErrComponentNotFound, gen.UpdateComponent404JSONResponse{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := componentsvcmocks.NewMockService(t)
			svc.EXPECT().UpdateComponent(mock.Anything, mock.Anything, mock.Anything).Return(nil, tt.svcErr)
			h := &Handler{
				services: &handlerservices.Services{ComponentService: svc},
				logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
			}
			body := gen.Component{Metadata: gen.ObjectMeta{Name: "comp-a"}}
			resp, err := h.UpdateComponent(ctx, gen.UpdateComponentRequestObject{
				NamespaceName: "test-ns",
				ComponentName: "comp-a",
				Body:          &body,
			})
			require.NoError(t, err)
			assert.IsType(t, tt.wantTyp, resp)
		})
	}
}
