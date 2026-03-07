// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"
	"encoding/json"
	"log/slog"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	authzcore "github.com/openchoreo/openchoreo/internal/authz/core"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
	clustercomponenttypesvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/clustercomponenttype"
	clustertraitsvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/clustertrait"
	clusterworkflowsvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/clusterworkflow"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/handlerservices"
	"github.com/openchoreo/openchoreo/internal/server/middleware/auth"
)

// denyAllPDP is a PDP stub that always denies authorization.
type denyAllPDP struct{}

func (d *denyAllPDP) Evaluate(_ context.Context, _ *authzcore.EvaluateRequest) (*authzcore.Decision, error) {
	return &authzcore.Decision{Decision: false, Context: &authzcore.DecisionContext{}}, nil
}

func (d *denyAllPDP) BatchEvaluate(_ context.Context, _ *authzcore.BatchEvaluateRequest) (*authzcore.BatchEvaluateResponse, error) {
	return nil, nil
}

func (d *denyAllPDP) GetSubjectProfile(_ context.Context, _ *authzcore.ProfileRequest) (*authzcore.UserCapabilitiesResponse, error) {
	return nil, nil
}

// allowAllPDP is a PDP stub that always allows authorization.
type allowAllPDP struct{}

func (a *allowAllPDP) Evaluate(_ context.Context, _ *authzcore.EvaluateRequest) (*authzcore.Decision, error) {
	return &authzcore.Decision{Decision: true, Context: &authzcore.DecisionContext{}}, nil
}

func (a *allowAllPDP) BatchEvaluate(_ context.Context, _ *authzcore.BatchEvaluateRequest) (*authzcore.BatchEvaluateResponse, error) {
	return nil, nil
}

func (a *allowAllPDP) GetSubjectProfile(_ context.Context, _ *authzcore.ProfileRequest) (*authzcore.UserCapabilitiesResponse, error) {
	return nil, nil
}

func newTestScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := openchoreov1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add scheme: %v", err)
	}
	return scheme
}

func newClusterComponentTypeService(t *testing.T, objects []client.Object, pdp authzcore.PDP) clustercomponenttypesvc.Service {
	t.Helper()
	fakeClient := fake.NewClientBuilder().
		WithScheme(newTestScheme(t)).
		WithObjects(objects...).
		Build()
	return clustercomponenttypesvc.NewServiceWithAuthz(fakeClient, pdp, slog.Default())
}

func newClusterTraitService(t *testing.T, objects []client.Object, pdp authzcore.PDP) clustertraitsvc.Service {
	t.Helper()
	fakeClient := fake.NewClientBuilder().
		WithScheme(newTestScheme(t)).
		WithObjects(objects...).
		Build()
	return clustertraitsvc.NewServiceWithAuthz(fakeClient, pdp, slog.Default())
}

func newHandlerWithClusterTraitService(ctSvc clustertraitsvc.Service) *Handler {
	return &Handler{
		services: &handlerservices.Services{ClusterTraitService: ctSvc},
		logger:   slog.Default(),
	}
}

func newHandlerWithClusterComponentTypeService(cctSvc clustercomponenttypesvc.Service) *Handler {
	return &Handler{
		services: &handlerservices.Services{ClusterComponentTypeService: cctSvc},
		logger:   slog.Default(),
	}
}

// testContext returns a context with a dummy authenticated subject for authz checks.
func testContext() context.Context {
	return auth.SetSubjectContext(context.Background(), &auth.SubjectContext{
		ID:   "test-user",
		Type: "user",
	})
}

func TestListClusterComponentTypesHandler(t *testing.T) {
	ctx := testContext()
	cct := &openchoreov1alpha1.ClusterComponentType{
		ObjectMeta: metav1.ObjectMeta{Name: "go-service"},
		Spec: openchoreov1alpha1.ClusterComponentTypeSpec{
			WorkloadType: "deployment",
			Resources:    []openchoreov1alpha1.ResourceTemplate{{ID: "deployment"}},
		},
	}

	t.Run("returns items when authorized", func(t *testing.T) {
		svc := newClusterComponentTypeService(t, []client.Object{cct}, &allowAllPDP{})
		h := newHandlerWithClusterComponentTypeService(svc)

		resp, err := h.ListClusterComponentTypes(ctx, gen.ListClusterComponentTypesRequestObject{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		typed, ok := resp.(gen.ListClusterComponentTypes200JSONResponse)
		if !ok {
			t.Fatalf("unexpected response type: %T", resp)
		}
		if len(typed.Items) != 1 {
			t.Fatalf("expected 1 item, got %d", len(typed.Items))
		}
	})

	t.Run("filters unauthorized items", func(t *testing.T) {
		svc := newClusterComponentTypeService(t, []client.Object{cct}, &denyAllPDP{})
		h := newHandlerWithClusterComponentTypeService(svc)

		resp, err := h.ListClusterComponentTypes(ctx, gen.ListClusterComponentTypesRequestObject{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		typed, ok := resp.(gen.ListClusterComponentTypes200JSONResponse)
		if !ok {
			t.Fatalf("unexpected response type: %T", resp)
		}
		if len(typed.Items) != 0 {
			t.Fatalf("expected 0 items, got %d", len(typed.Items))
		}
	})
}

func TestGetClusterComponentTypeSchemaHandler(t *testing.T) {
	ctx := testContext()
	paramsRaw, _ := json.Marshal(map[string]any{
		"replicas": "integer",
	})

	cct := &openchoreov1alpha1.ClusterComponentType{
		ObjectMeta: metav1.ObjectMeta{Name: "go-service"},
		Spec: openchoreov1alpha1.ClusterComponentTypeSpec{
			WorkloadType: "deployment",
			Resources:    []openchoreov1alpha1.ResourceTemplate{{ID: "deployment"}},
			Schema: openchoreov1alpha1.ComponentTypeSchema{
				OCSchema: &openchoreov1alpha1.ComponentTypeOCSchema{
					Parameters: &runtime.RawExtension{Raw: paramsRaw},
				},
			},
		},
	}

	t.Run("returns schema when authorized", func(t *testing.T) {
		svc := newClusterComponentTypeService(t, []client.Object{cct}, &allowAllPDP{})
		h := newHandlerWithClusterComponentTypeService(svc)

		resp, err := h.GetClusterComponentTypeSchema(ctx, gen.GetClusterComponentTypeSchemaRequestObject{CctName: "go-service"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		typed, ok := resp.(gen.GetClusterComponentTypeSchema200JSONResponse)
		if !ok {
			t.Fatalf("unexpected response type: %T", resp)
		}
		if len(typed) == 0 {
			t.Fatalf("expected non-empty schema response")
		}
	})

	t.Run("returns 404 when not found", func(t *testing.T) {
		svc := newClusterComponentTypeService(t, nil, &allowAllPDP{})
		h := newHandlerWithClusterComponentTypeService(svc)

		resp, err := h.GetClusterComponentTypeSchema(ctx, gen.GetClusterComponentTypeSchemaRequestObject{CctName: "missing"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if _, ok := resp.(gen.GetClusterComponentTypeSchema404JSONResponse); !ok {
			t.Fatalf("unexpected response type: %T", resp)
		}
	})

	t.Run("returns 403 when forbidden", func(t *testing.T) {
		svc := newClusterComponentTypeService(t, []client.Object{cct}, &denyAllPDP{})
		h := newHandlerWithClusterComponentTypeService(svc)

		resp, err := h.GetClusterComponentTypeSchema(ctx, gen.GetClusterComponentTypeSchemaRequestObject{CctName: "go-service"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if _, ok := resp.(gen.GetClusterComponentTypeSchema403JSONResponse); !ok {
			t.Fatalf("unexpected response type: %T", resp)
		}
	})
}

func TestListClusterTraitsHandler(t *testing.T) {
	ctx := testContext()
	ct := &openchoreov1alpha1.ClusterTrait{
		ObjectMeta: metav1.ObjectMeta{Name: "autoscaler"},
		Spec:       openchoreov1alpha1.ClusterTraitSpec{},
	}

	t.Run("returns items when authorized", func(t *testing.T) {
		svc := newClusterTraitService(t, []client.Object{ct}, &allowAllPDP{})
		h := newHandlerWithClusterTraitService(svc)

		resp, err := h.ListClusterTraits(ctx, gen.ListClusterTraitsRequestObject{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		typed, ok := resp.(gen.ListClusterTraits200JSONResponse)
		if !ok {
			t.Fatalf("unexpected response type: %T", resp)
		}
		if len(typed.Items) != 1 {
			t.Fatalf("expected 1 item, got %d", len(typed.Items))
		}
	})

	t.Run("filters unauthorized items", func(t *testing.T) {
		svc := newClusterTraitService(t, []client.Object{ct}, &denyAllPDP{})
		h := newHandlerWithClusterTraitService(svc)

		resp, err := h.ListClusterTraits(ctx, gen.ListClusterTraitsRequestObject{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		typed, ok := resp.(gen.ListClusterTraits200JSONResponse)
		if !ok {
			t.Fatalf("unexpected response type: %T", resp)
		}
		if len(typed.Items) != 0 {
			t.Fatalf("expected 0 items, got %d", len(typed.Items))
		}
	})
}

func TestGetClusterTraitSchemaHandler(t *testing.T) {
	ctx := testContext()
	paramsRaw, _ := json.Marshal(map[string]any{
		"minReplicas": "integer",
	})

	ct := &openchoreov1alpha1.ClusterTrait{
		ObjectMeta: metav1.ObjectMeta{Name: "autoscaler"},
		Spec: openchoreov1alpha1.ClusterTraitSpec{
			Schema: openchoreov1alpha1.TraitSchema{
				OCSchema: &openchoreov1alpha1.TraitOCSchema{
					Parameters: &runtime.RawExtension{Raw: paramsRaw},
				},
			},
		},
	}

	t.Run("returns schema when authorized", func(t *testing.T) {
		svc := newClusterTraitService(t, []client.Object{ct}, &allowAllPDP{})
		h := newHandlerWithClusterTraitService(svc)

		resp, err := h.GetClusterTraitSchema(ctx, gen.GetClusterTraitSchemaRequestObject{ClusterTraitName: "autoscaler"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		typed, ok := resp.(gen.GetClusterTraitSchema200JSONResponse)
		if !ok {
			t.Fatalf("unexpected response type: %T", resp)
		}
		if len(typed) == 0 {
			t.Fatalf("expected non-empty schema response")
		}
	})

	t.Run("returns 404 when not found", func(t *testing.T) {
		svc := newClusterTraitService(t, nil, &allowAllPDP{})
		h := newHandlerWithClusterTraitService(svc)

		resp, err := h.GetClusterTraitSchema(ctx, gen.GetClusterTraitSchemaRequestObject{ClusterTraitName: "missing"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if _, ok := resp.(gen.GetClusterTraitSchema404JSONResponse); !ok {
			t.Fatalf("unexpected response type: %T", resp)
		}
	})

	t.Run("returns 403 when forbidden", func(t *testing.T) {
		svc := newClusterTraitService(t, []client.Object{ct}, &denyAllPDP{})
		h := newHandlerWithClusterTraitService(svc)

		resp, err := h.GetClusterTraitSchema(ctx, gen.GetClusterTraitSchemaRequestObject{ClusterTraitName: "autoscaler"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if _, ok := resp.(gen.GetClusterTraitSchema403JSONResponse); !ok {
			t.Fatalf("unexpected response type: %T", resp)
		}
	})
}

// --- ClusterWorkflow handler helpers and tests ---

func newClusterWorkflowService(t *testing.T, objects []client.Object, pdp authzcore.PDP) clusterworkflowsvc.Service {
	t.Helper()
	fakeClient := fake.NewClientBuilder().
		WithScheme(newTestScheme(t)).
		WithObjects(objects...).
		Build()
	return clusterworkflowsvc.NewServiceWithAuthz(fakeClient, pdp, slog.Default())
}

func newHandlerWithClusterWorkflowService(cwfSvc clusterworkflowsvc.Service) *Handler {
	return &Handler{
		services: &handlerservices.Services{ClusterWorkflowService: cwfSvc},
		logger:   slog.Default(),
	}
}

func TestListClusterWorkflowsHandler(t *testing.T) {
	ctx := testContext()
	cwf := &openchoreov1alpha1.ClusterWorkflow{
		ObjectMeta: metav1.ObjectMeta{Name: "build-go"},
		Spec: openchoreov1alpha1.ClusterWorkflowSpec{
			RunTemplate: &runtime.RawExtension{Raw: []byte(`{"kind":"Workflow"}`)},
		},
	}

	t.Run("returns items when authorized", func(t *testing.T) {
		svc := newClusterWorkflowService(t, []client.Object{cwf}, &allowAllPDP{})
		h := newHandlerWithClusterWorkflowService(svc)

		resp, err := h.ListClusterWorkflows(ctx, gen.ListClusterWorkflowsRequestObject{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		typed, ok := resp.(gen.ListClusterWorkflows200JSONResponse)
		if !ok {
			t.Fatalf("unexpected response type: %T", resp)
		}
		if len(typed.Items) != 1 {
			t.Fatalf("expected 1 item, got %d", len(typed.Items))
		}
	})

	t.Run("filters unauthorized items", func(t *testing.T) {
		svc := newClusterWorkflowService(t, []client.Object{cwf}, &denyAllPDP{})
		h := newHandlerWithClusterWorkflowService(svc)

		resp, err := h.ListClusterWorkflows(ctx, gen.ListClusterWorkflowsRequestObject{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		typed, ok := resp.(gen.ListClusterWorkflows200JSONResponse)
		if !ok {
			t.Fatalf("unexpected response type: %T", resp)
		}
		if len(typed.Items) != 0 {
			t.Fatalf("expected 0 items, got %d", len(typed.Items))
		}
	})
}

func TestGetClusterWorkflowHandler(t *testing.T) {
	ctx := testContext()
	cwf := &openchoreov1alpha1.ClusterWorkflow{
		ObjectMeta: metav1.ObjectMeta{Name: "build-go"},
		Spec: openchoreov1alpha1.ClusterWorkflowSpec{
			RunTemplate: &runtime.RawExtension{Raw: []byte(`{"kind":"Workflow"}`)},
		},
	}

	t.Run("returns workflow when authorized", func(t *testing.T) {
		svc := newClusterWorkflowService(t, []client.Object{cwf}, &allowAllPDP{})
		h := newHandlerWithClusterWorkflowService(svc)

		resp, err := h.GetClusterWorkflow(ctx, gen.GetClusterWorkflowRequestObject{ClusterWorkflowName: "build-go"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if _, ok := resp.(gen.GetClusterWorkflow200JSONResponse); !ok {
			t.Fatalf("unexpected response type: %T", resp)
		}
	})

	t.Run("returns 404 when not found", func(t *testing.T) {
		svc := newClusterWorkflowService(t, nil, &allowAllPDP{})
		h := newHandlerWithClusterWorkflowService(svc)

		resp, err := h.GetClusterWorkflow(ctx, gen.GetClusterWorkflowRequestObject{ClusterWorkflowName: "missing"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if _, ok := resp.(gen.GetClusterWorkflow404JSONResponse); !ok {
			t.Fatalf("unexpected response type: %T", resp)
		}
	})

	t.Run("returns 403 when forbidden", func(t *testing.T) {
		svc := newClusterWorkflowService(t, []client.Object{cwf}, &denyAllPDP{})
		h := newHandlerWithClusterWorkflowService(svc)

		resp, err := h.GetClusterWorkflow(ctx, gen.GetClusterWorkflowRequestObject{ClusterWorkflowName: "build-go"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if _, ok := resp.(gen.GetClusterWorkflow403JSONResponse); !ok {
			t.Fatalf("unexpected response type: %T", resp)
		}
	})
}

func TestCreateClusterWorkflowHandler(t *testing.T) {
	ctx := testContext()

	t.Run("creates successfully when authorized", func(t *testing.T) {
		svc := newClusterWorkflowService(t, nil, &allowAllPDP{})
		h := newHandlerWithClusterWorkflowService(svc)

		body := gen.ClusterWorkflow{}
		bodyBytes, _ := json.Marshal(map[string]any{
			"apiVersion": "openchoreo.dev/v1alpha1",
			"kind":       "ClusterWorkflow",
			"metadata":   map[string]any{"name": "build-go"},
			"spec": map[string]any{
				"runTemplate": map[string]any{"kind": "Workflow"},
			},
		})
		_ = json.Unmarshal(bodyBytes, &body)

		resp, err := h.CreateClusterWorkflow(ctx, gen.CreateClusterWorkflowRequestObject{Body: &body})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if _, ok := resp.(gen.CreateClusterWorkflow201JSONResponse); !ok {
			t.Fatalf("unexpected response type: %T", resp)
		}
	})

	t.Run("returns 400 when body is nil", func(t *testing.T) {
		svc := newClusterWorkflowService(t, nil, &allowAllPDP{})
		h := newHandlerWithClusterWorkflowService(svc)

		resp, err := h.CreateClusterWorkflow(ctx, gen.CreateClusterWorkflowRequestObject{Body: nil})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if _, ok := resp.(gen.CreateClusterWorkflow400JSONResponse); !ok {
			t.Fatalf("unexpected response type: %T", resp)
		}
	})

	t.Run("returns 403 when forbidden", func(t *testing.T) {
		svc := newClusterWorkflowService(t, nil, &denyAllPDP{})
		h := newHandlerWithClusterWorkflowService(svc)

		body := gen.ClusterWorkflow{}
		bodyBytes, _ := json.Marshal(map[string]any{
			"apiVersion": "openchoreo.dev/v1alpha1",
			"kind":       "ClusterWorkflow",
			"metadata":   map[string]any{"name": "build-go"},
			"spec": map[string]any{
				"runTemplate": map[string]any{"kind": "Workflow"},
			},
		})
		_ = json.Unmarshal(bodyBytes, &body)

		resp, err := h.CreateClusterWorkflow(ctx, gen.CreateClusterWorkflowRequestObject{Body: &body})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if _, ok := resp.(gen.CreateClusterWorkflow403JSONResponse); !ok {
			t.Fatalf("unexpected response type: %T", resp)
		}
	})
}

func TestDeleteClusterWorkflowHandler(t *testing.T) {
	ctx := testContext()
	cwf := &openchoreov1alpha1.ClusterWorkflow{
		ObjectMeta: metav1.ObjectMeta{Name: "build-go"},
		Spec: openchoreov1alpha1.ClusterWorkflowSpec{
			RunTemplate: &runtime.RawExtension{Raw: []byte(`{"kind":"Workflow"}`)},
		},
	}

	t.Run("deletes successfully when authorized", func(t *testing.T) {
		svc := newClusterWorkflowService(t, []client.Object{cwf}, &allowAllPDP{})
		h := newHandlerWithClusterWorkflowService(svc)

		resp, err := h.DeleteClusterWorkflow(ctx, gen.DeleteClusterWorkflowRequestObject{ClusterWorkflowName: "build-go"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if _, ok := resp.(gen.DeleteClusterWorkflow204Response); !ok {
			t.Fatalf("unexpected response type: %T", resp)
		}
	})

	t.Run("returns 404 when not found", func(t *testing.T) {
		svc := newClusterWorkflowService(t, nil, &allowAllPDP{})
		h := newHandlerWithClusterWorkflowService(svc)

		resp, err := h.DeleteClusterWorkflow(ctx, gen.DeleteClusterWorkflowRequestObject{ClusterWorkflowName: "missing"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if _, ok := resp.(gen.DeleteClusterWorkflow404JSONResponse); !ok {
			t.Fatalf("unexpected response type: %T", resp)
		}
	})

	t.Run("returns 403 when forbidden", func(t *testing.T) {
		svc := newClusterWorkflowService(t, []client.Object{cwf}, &denyAllPDP{})
		h := newHandlerWithClusterWorkflowService(svc)

		resp, err := h.DeleteClusterWorkflow(ctx, gen.DeleteClusterWorkflowRequestObject{ClusterWorkflowName: "build-go"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if _, ok := resp.(gen.DeleteClusterWorkflow403JSONResponse); !ok {
			t.Fatalf("unexpected response type: %T", resp)
		}
	})
}

func TestGetClusterWorkflowSchemaHandler(t *testing.T) {
	ctx := testContext()
	paramsRaw, _ := json.Marshal(map[string]any{
		"dockerContext": "string",
	})

	cwf := &openchoreov1alpha1.ClusterWorkflow{
		ObjectMeta: metav1.ObjectMeta{Name: "build-go"},
		Spec: openchoreov1alpha1.ClusterWorkflowSpec{
			RunTemplate: &runtime.RawExtension{Raw: []byte(`{"kind":"Workflow"}`)},
			Schema: &openchoreov1alpha1.WorkflowSchema{
				OCSchema: &openchoreov1alpha1.WorkflowOCSchema{
					Parameters: &runtime.RawExtension{Raw: paramsRaw},
				},
			},
		},
	}

	t.Run("returns schema when authorized", func(t *testing.T) {
		svc := newClusterWorkflowService(t, []client.Object{cwf}, &allowAllPDP{})
		h := newHandlerWithClusterWorkflowService(svc)

		resp, err := h.GetClusterWorkflowSchema(ctx, gen.GetClusterWorkflowSchemaRequestObject{ClusterWorkflowName: "build-go"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		typed, ok := resp.(gen.GetClusterWorkflowSchema200JSONResponse)
		if !ok {
			t.Fatalf("unexpected response type: %T", resp)
		}
		if len(typed) == 0 {
			t.Fatalf("expected non-empty schema response")
		}
	})

	t.Run("returns 404 when not found", func(t *testing.T) {
		svc := newClusterWorkflowService(t, nil, &allowAllPDP{})
		h := newHandlerWithClusterWorkflowService(svc)

		resp, err := h.GetClusterWorkflowSchema(ctx, gen.GetClusterWorkflowSchemaRequestObject{ClusterWorkflowName: "missing"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if _, ok := resp.(gen.GetClusterWorkflowSchema404JSONResponse); !ok {
			t.Fatalf("unexpected response type: %T", resp)
		}
	})

	t.Run("returns 403 when forbidden", func(t *testing.T) {
		svc := newClusterWorkflowService(t, []client.Object{cwf}, &denyAllPDP{})
		h := newHandlerWithClusterWorkflowService(svc)

		resp, err := h.GetClusterWorkflowSchema(ctx, gen.GetClusterWorkflowSchemaRequestObject{ClusterWorkflowName: "build-go"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if _, ok := resp.(gen.GetClusterWorkflowSchema403JSONResponse); !ok {
			t.Fatalf("unexpected response type: %T", resp)
		}
	})
}
