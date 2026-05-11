// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	authzcore "github.com/openchoreo/openchoreo/internal/authz/core"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
	svcpkg "github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	clustercomponenttypesvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/clustercomponenttype"
	cctsvcmocks "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/clustercomponenttype/mocks"
	clustertraitsvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/clustertrait"
	clustertraitmocks "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/clustertrait/mocks"
	clusterworkflowsvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/clusterworkflow"
	clusterworkflowmocks "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/clusterworkflow/mocks"
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

// selectivePDP allows resources whose ID is in the allowedIDs set.
type selectivePDP struct {
	allowedIDs map[string]bool
}

func (s *selectivePDP) Evaluate(_ context.Context, req *authzcore.EvaluateRequest) (*authzcore.Decision, error) {
	return &authzcore.Decision{
		Decision: s.allowedIDs[req.Resource.ID],
		Context:  &authzcore.DecisionContext{},
	}, nil
}

func (s *selectivePDP) BatchEvaluate(_ context.Context, _ *authzcore.BatchEvaluateRequest) (*authzcore.BatchEvaluateResponse, error) {
	return nil, nil
}

func (s *selectivePDP) GetSubjectProfile(_ context.Context, _ *authzcore.ProfileRequest) (*authzcore.UserCapabilitiesResponse, error) {
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
		require.NoError(t, err)
		typed, ok := resp.(gen.ListClusterComponentTypes200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
		assert.Len(t, typed.Items, 1)
	})

	t.Run("filters unauthorized items", func(t *testing.T) {
		svc := newClusterComponentTypeService(t, []client.Object{cct}, &denyAllPDP{})
		h := newHandlerWithClusterComponentTypeService(svc)

		resp, err := h.ListClusterComponentTypes(ctx, gen.ListClusterComponentTypesRequestObject{})
		require.NoError(t, err)
		typed, ok := resp.(gen.ListClusterComponentTypes200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
		assert.Empty(t, typed.Items)
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
			Parameters: &openchoreov1alpha1.SchemaSection{
				OpenAPIV3Schema: &runtime.RawExtension{Raw: paramsRaw},
			},
		},
	}

	t.Run("returns schema when authorized", func(t *testing.T) {
		svc := newClusterComponentTypeService(t, []client.Object{cct}, &allowAllPDP{})
		h := newHandlerWithClusterComponentTypeService(svc)

		resp, err := h.GetClusterComponentTypeSchema(ctx, gen.GetClusterComponentTypeSchemaRequestObject{CctName: "go-service"})
		require.NoError(t, err)
		typed, ok := resp.(gen.GetClusterComponentTypeSchema200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
		assert.NotEmpty(t, typed)
	})

	t.Run("returns 404 when not found", func(t *testing.T) {
		svc := newClusterComponentTypeService(t, nil, &allowAllPDP{})
		h := newHandlerWithClusterComponentTypeService(svc)

		resp, err := h.GetClusterComponentTypeSchema(ctx, gen.GetClusterComponentTypeSchemaRequestObject{CctName: "missing"})
		require.NoError(t, err)
		assert.IsType(t, gen.GetClusterComponentTypeSchema404JSONResponse{}, resp)
	})

	t.Run("returns 403 when forbidden", func(t *testing.T) {
		svc := newClusterComponentTypeService(t, []client.Object{cct}, &denyAllPDP{})
		h := newHandlerWithClusterComponentTypeService(svc)

		resp, err := h.GetClusterComponentTypeSchema(ctx, gen.GetClusterComponentTypeSchemaRequestObject{CctName: "go-service"})
		require.NoError(t, err)
		assert.IsType(t, gen.GetClusterComponentTypeSchema403JSONResponse{}, resp)
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
		require.NoError(t, err)
		typed, ok := resp.(gen.ListClusterTraits200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
		assert.Len(t, typed.Items, 1)
	})

	t.Run("filters unauthorized items", func(t *testing.T) {
		svc := newClusterTraitService(t, []client.Object{ct}, &denyAllPDP{})
		h := newHandlerWithClusterTraitService(svc)

		resp, err := h.ListClusterTraits(ctx, gen.ListClusterTraitsRequestObject{})
		require.NoError(t, err)
		typed, ok := resp.(gen.ListClusterTraits200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
		assert.Empty(t, typed.Items)
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
			Parameters: &openchoreov1alpha1.SchemaSection{
				OpenAPIV3Schema: &runtime.RawExtension{Raw: paramsRaw},
			},
		},
	}

	t.Run("returns schema when authorized", func(t *testing.T) {
		svc := newClusterTraitService(t, []client.Object{ct}, &allowAllPDP{})
		h := newHandlerWithClusterTraitService(svc)

		resp, err := h.GetClusterTraitSchema(ctx, gen.GetClusterTraitSchemaRequestObject{ClusterTraitName: "autoscaler"})
		require.NoError(t, err)
		typed, ok := resp.(gen.GetClusterTraitSchema200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
		assert.NotEmpty(t, typed)
	})

	t.Run("returns 404 when not found", func(t *testing.T) {
		svc := newClusterTraitService(t, nil, &allowAllPDP{})
		h := newHandlerWithClusterTraitService(svc)

		resp, err := h.GetClusterTraitSchema(ctx, gen.GetClusterTraitSchemaRequestObject{ClusterTraitName: "missing"})
		require.NoError(t, err)
		assert.IsType(t, gen.GetClusterTraitSchema404JSONResponse{}, resp)
	})

	t.Run("returns 403 when forbidden", func(t *testing.T) {
		svc := newClusterTraitService(t, []client.Object{ct}, &denyAllPDP{})
		h := newHandlerWithClusterTraitService(svc)

		resp, err := h.GetClusterTraitSchema(ctx, gen.GetClusterTraitSchemaRequestObject{ClusterTraitName: "autoscaler"})
		require.NoError(t, err)
		assert.IsType(t, gen.GetClusterTraitSchema403JSONResponse{}, resp)
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
		require.NoError(t, err)
		typed, ok := resp.(gen.ListClusterWorkflows200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
		assert.Len(t, typed.Items, 1)
	})

	t.Run("filters unauthorized items", func(t *testing.T) {
		svc := newClusterWorkflowService(t, []client.Object{cwf}, &denyAllPDP{})
		h := newHandlerWithClusterWorkflowService(svc)

		resp, err := h.ListClusterWorkflows(ctx, gen.ListClusterWorkflowsRequestObject{})
		require.NoError(t, err)
		typed, ok := resp.(gen.ListClusterWorkflows200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
		assert.Empty(t, typed.Items)
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
		require.NoError(t, err)
		assert.IsType(t, gen.GetClusterWorkflow200JSONResponse{}, resp)
	})

	t.Run("returns 404 when not found", func(t *testing.T) {
		svc := newClusterWorkflowService(t, nil, &allowAllPDP{})
		h := newHandlerWithClusterWorkflowService(svc)

		resp, err := h.GetClusterWorkflow(ctx, gen.GetClusterWorkflowRequestObject{ClusterWorkflowName: "missing"})
		require.NoError(t, err)
		assert.IsType(t, gen.GetClusterWorkflow404JSONResponse{}, resp)
	})

	t.Run("returns 403 when forbidden", func(t *testing.T) {
		svc := newClusterWorkflowService(t, []client.Object{cwf}, &denyAllPDP{})
		h := newHandlerWithClusterWorkflowService(svc)

		resp, err := h.GetClusterWorkflow(ctx, gen.GetClusterWorkflowRequestObject{ClusterWorkflowName: "build-go"})
		require.NoError(t, err)
		assert.IsType(t, gen.GetClusterWorkflow403JSONResponse{}, resp)
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
		require.NoError(t, err)
		assert.IsType(t, gen.CreateClusterWorkflow201JSONResponse{}, resp)
	})

	t.Run("returns 400 when body is nil", func(t *testing.T) {
		svc := newClusterWorkflowService(t, nil, &allowAllPDP{})
		h := newHandlerWithClusterWorkflowService(svc)

		resp, err := h.CreateClusterWorkflow(ctx, gen.CreateClusterWorkflowRequestObject{Body: nil})
		require.NoError(t, err)
		assert.IsType(t, gen.CreateClusterWorkflow400JSONResponse{}, resp)
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
		require.NoError(t, err)
		assert.IsType(t, gen.CreateClusterWorkflow403JSONResponse{}, resp)
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
		require.NoError(t, err)
		assert.IsType(t, gen.DeleteClusterWorkflow204Response{}, resp)
	})

	t.Run("returns 404 when not found", func(t *testing.T) {
		svc := newClusterWorkflowService(t, nil, &allowAllPDP{})
		h := newHandlerWithClusterWorkflowService(svc)

		resp, err := h.DeleteClusterWorkflow(ctx, gen.DeleteClusterWorkflowRequestObject{ClusterWorkflowName: "missing"})
		require.NoError(t, err)
		assert.IsType(t, gen.DeleteClusterWorkflow404JSONResponse{}, resp)
	})

	t.Run("returns 403 when forbidden", func(t *testing.T) {
		svc := newClusterWorkflowService(t, []client.Object{cwf}, &denyAllPDP{})
		h := newHandlerWithClusterWorkflowService(svc)

		resp, err := h.DeleteClusterWorkflow(ctx, gen.DeleteClusterWorkflowRequestObject{ClusterWorkflowName: "build-go"})
		require.NoError(t, err)
		assert.IsType(t, gen.DeleteClusterWorkflow403JSONResponse{}, resp)
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
			Parameters: &openchoreov1alpha1.SchemaSection{
				OpenAPIV3Schema: &runtime.RawExtension{Raw: paramsRaw},
			},
		},
	}

	t.Run("returns schema when authorized", func(t *testing.T) {
		svc := newClusterWorkflowService(t, []client.Object{cwf}, &allowAllPDP{})
		h := newHandlerWithClusterWorkflowService(svc)

		resp, err := h.GetClusterWorkflowSchema(ctx, gen.GetClusterWorkflowSchemaRequestObject{ClusterWorkflowName: "build-go"})
		require.NoError(t, err)
		typed, ok := resp.(gen.GetClusterWorkflowSchema200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
		assert.NotEmpty(t, typed)
	})

	t.Run("returns 404 when not found", func(t *testing.T) {
		svc := newClusterWorkflowService(t, nil, &allowAllPDP{})
		h := newHandlerWithClusterWorkflowService(svc)

		resp, err := h.GetClusterWorkflowSchema(ctx, gen.GetClusterWorkflowSchemaRequestObject{ClusterWorkflowName: "missing"})
		require.NoError(t, err)
		assert.IsType(t, gen.GetClusterWorkflowSchema404JSONResponse{}, resp)
	})

	t.Run("returns 403 when forbidden", func(t *testing.T) {
		svc := newClusterWorkflowService(t, []client.Object{cwf}, &denyAllPDP{})
		h := newHandlerWithClusterWorkflowService(svc)

		resp, err := h.GetClusterWorkflowSchema(ctx, gen.GetClusterWorkflowSchemaRequestObject{ClusterWorkflowName: "build-go"})
		require.NoError(t, err)
		assert.IsType(t, gen.GetClusterWorkflowSchema403JSONResponse{}, resp)
	})
}

func TestUpdateClusterComponentTypeHandler_UsesPathName(t *testing.T) {
	ctx := testContext()
	svc := cctsvcmocks.NewMockService(t)
	svc.EXPECT().UpdateClusterComponentType(mock.Anything, mock.Anything).RunAndReturn(func(_ context.Context, cct *openchoreov1alpha1.ClusterComponentType) (*openchoreov1alpha1.ClusterComponentType, error) {
		assert.Equal(t, "from-path", cct.Name)
		return cct, nil
	})
	h := &Handler{
		services: &handlerservices.Services{ClusterComponentTypeService: svc},
		logger:   slog.Default(),
	}

	body := gen.ClusterComponentType{Metadata: gen.ObjectMeta{Name: "from-body"}}
	resp, err := h.UpdateClusterComponentType(ctx, gen.UpdateClusterComponentTypeRequestObject{
		CctName: "from-path",
		Body:    &body,
	})
	require.NoError(t, err)
	typed, ok := resp.(gen.UpdateClusterComponentType200JSONResponse)
	require.True(t, ok, "expected 200 response, got %T", resp)
	assert.Equal(t, "from-path", typed.Metadata.Name)
}

func TestCreateClusterComponentTypeHandler_MapsErrors(t *testing.T) {
	ctx := testContext()

	tests := []struct {
		name    string
		svcErr  error
		wantTyp any
	}{
		{"forbidden -> 403", svcpkg.ErrForbidden, gen.CreateClusterComponentType403JSONResponse{}},
		{"already exists -> 409", clustercomponenttypesvc.ErrClusterComponentTypeAlreadyExists, gen.CreateClusterComponentType409JSONResponse{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := cctsvcmocks.NewMockService(t)
			svc.EXPECT().CreateClusterComponentType(mock.Anything, mock.Anything).Return(nil, tt.svcErr)
			h := &Handler{
				services: &handlerservices.Services{ClusterComponentTypeService: svc},
				logger:   slog.Default(),
			}
			body := gen.ClusterComponentType{Metadata: gen.ObjectMeta{Name: "cct"}}
			resp, err := h.CreateClusterComponentType(ctx, gen.CreateClusterComponentTypeRequestObject{Body: &body})
			require.NoError(t, err)
			assert.IsType(t, tt.wantTyp, resp)
		})
	}
}

func TestUpdateClusterTraitHandler_UsesPathName(t *testing.T) {
	ctx := testContext()
	svc := clustertraitmocks.NewMockService(t)
	svc.EXPECT().UpdateClusterTrait(mock.Anything, mock.Anything).RunAndReturn(func(_ context.Context, ct *openchoreov1alpha1.ClusterTrait) (*openchoreov1alpha1.ClusterTrait, error) {
		assert.Equal(t, "from-path", ct.Name)
		return ct, nil
	})
	h := &Handler{
		services: &handlerservices.Services{ClusterTraitService: svc},
		logger:   slog.Default(),
	}

	body := gen.ClusterTrait{Metadata: gen.ObjectMeta{Name: "from-body"}}
	resp, err := h.UpdateClusterTrait(ctx, gen.UpdateClusterTraitRequestObject{
		ClusterTraitName: "from-path",
		Body:             &body,
	})
	require.NoError(t, err)
	typed, ok := resp.(gen.UpdateClusterTrait200JSONResponse)
	require.True(t, ok, "expected 200 response, got %T", resp)
	assert.Equal(t, "from-path", typed.Metadata.Name)
}

func TestCreateClusterTraitHandler_MapsErrors(t *testing.T) {
	ctx := testContext()

	tests := []struct {
		name    string
		svcErr  error
		wantTyp any
	}{
		{"forbidden -> 403", svcpkg.ErrForbidden, gen.CreateClusterTrait403JSONResponse{}},
		{"already exists -> 409", clustertraitsvc.ErrClusterTraitAlreadyExists, gen.CreateClusterTrait409JSONResponse{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := clustertraitmocks.NewMockService(t)
			svc.EXPECT().CreateClusterTrait(mock.Anything, mock.Anything).Return(nil, tt.svcErr)
			h := &Handler{
				services: &handlerservices.Services{ClusterTraitService: svc},
				logger:   slog.Default(),
			}
			body := gen.ClusterTrait{Metadata: gen.ObjectMeta{Name: "ct"}}
			resp, err := h.CreateClusterTrait(ctx, gen.CreateClusterTraitRequestObject{Body: &body})
			require.NoError(t, err)
			assert.IsType(t, tt.wantTyp, resp)
		})
	}
}

func TestUpdateClusterComponentTypeHandler_MapsErrors(t *testing.T) {
	ctx := testContext()

	tests := []struct {
		name    string
		svcErr  error
		wantTyp any
	}{
		{"forbidden -> 403", svcpkg.ErrForbidden, gen.UpdateClusterComponentType403JSONResponse{}},
		{"not found -> 404", clustercomponenttypesvc.ErrClusterComponentTypeNotFound, gen.UpdateClusterComponentType404JSONResponse{}},
		{"validation error -> 400", &svcpkg.ValidationError{Msg: "invalid spec"}, gen.UpdateClusterComponentType400JSONResponse{}},
		{"validation 422 -> 422", &svcpkg.ValidationError{Msg: "invalid spec", StatusCode: http.StatusUnprocessableEntity}, gen.UpdateClusterComponentType422JSONResponse{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := cctsvcmocks.NewMockService(t)
			svc.EXPECT().UpdateClusterComponentType(mock.Anything, mock.Anything).Return(nil, tt.svcErr)
			h := &Handler{
				services: &handlerservices.Services{ClusterComponentTypeService: svc},
				logger:   slog.Default(),
			}
			body := gen.ClusterComponentType{Metadata: gen.ObjectMeta{Name: "cct"}}
			resp, err := h.UpdateClusterComponentType(ctx, gen.UpdateClusterComponentTypeRequestObject{CctName: "cct", Body: &body})
			require.NoError(t, err)
			assert.IsType(t, tt.wantTyp, resp)
		})
	}
}

func TestUpdateClusterTraitHandler_MapsErrors(t *testing.T) {
	ctx := testContext()

	tests := []struct {
		name    string
		svcErr  error
		wantTyp any
	}{
		{"forbidden -> 403", svcpkg.ErrForbidden, gen.UpdateClusterTrait403JSONResponse{}},
		{"not found -> 404", clustertraitsvc.ErrClusterTraitNotFound, gen.UpdateClusterTrait404JSONResponse{}},
		{"validation error -> 400", &svcpkg.ValidationError{Msg: "invalid spec"}, gen.UpdateClusterTrait400JSONResponse{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := clustertraitmocks.NewMockService(t)
			svc.EXPECT().UpdateClusterTrait(mock.Anything, mock.Anything).Return(nil, tt.svcErr)
			h := &Handler{
				services: &handlerservices.Services{ClusterTraitService: svc},
				logger:   slog.Default(),
			}
			body := gen.ClusterTrait{Metadata: gen.ObjectMeta{Name: "ct"}}
			resp, err := h.UpdateClusterTrait(ctx, gen.UpdateClusterTraitRequestObject{ClusterTraitName: "ct", Body: &body})
			require.NoError(t, err)
			assert.IsType(t, tt.wantTyp, resp)
		})
	}
}

// --- Additional ClusterWorkflow handler coverage ---

func TestListClusterWorkflowsHandler_MapsErrors(t *testing.T) {
	ctx := testContext()
	tests := []struct {
		name    string
		svcErr  error
		wantTyp any
	}{
		{"validation -> 400", &svcpkg.ValidationError{Msg: "bad request"}, gen.ListClusterWorkflows400JSONResponse{}},
		{"internal -> 500", errors.New("internal server error"), gen.ListClusterWorkflows500JSONResponse{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := clusterworkflowmocks.NewMockService(t)
			svc.EXPECT().ListClusterWorkflows(mock.Anything, mock.Anything).Return(nil, tt.svcErr)
			h := &Handler{
				services: &handlerservices.Services{ClusterWorkflowService: svc},
				logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
			}
			resp, err := h.ListClusterWorkflows(ctx, gen.ListClusterWorkflowsRequestObject{})
			require.NoError(t, err)
			assert.IsType(t, tt.wantTyp, resp)
		})
	}
}

func TestGetClusterWorkflowHandler_InternalError(t *testing.T) {
	ctx := testContext()
	svc := clusterworkflowmocks.NewMockService(t)
	svc.EXPECT().GetClusterWorkflow(mock.Anything, "name").Return(nil, errors.New("internal server error"))
	h := &Handler{
		services: &handlerservices.Services{ClusterWorkflowService: svc},
		logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	resp, err := h.GetClusterWorkflow(ctx, gen.GetClusterWorkflowRequestObject{ClusterWorkflowName: "name"})
	require.NoError(t, err)
	assert.IsType(t, gen.GetClusterWorkflow500JSONResponse{}, resp)
}

func TestCreateClusterWorkflowHandler_MapsErrors(t *testing.T) {
	ctx := testContext()
	body := &gen.ClusterWorkflow{Metadata: gen.ObjectMeta{Name: "cwf"}}

	tests := []struct {
		name    string
		svcErr  error
		wantTyp any
	}{
		{"already exists -> 409", clusterworkflowsvc.ErrClusterWorkflowAlreadyExists, gen.CreateClusterWorkflow409JSONResponse{}},
		{"validation error -> 400", &svcpkg.ValidationError{Msg: "invalid spec"}, gen.CreateClusterWorkflow400JSONResponse{}},
		{"validation 422 -> 422", &svcpkg.ValidationError{Msg: "invalid spec", StatusCode: http.StatusUnprocessableEntity}, gen.CreateClusterWorkflow422JSONResponse{}},
		{"internal -> 500", errors.New("internal server error"), gen.CreateClusterWorkflow500JSONResponse{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := clusterworkflowmocks.NewMockService(t)
			svc.EXPECT().CreateClusterWorkflow(mock.Anything, mock.Anything).Return(nil, tt.svcErr)
			h := &Handler{
				services: &handlerservices.Services{ClusterWorkflowService: svc},
				logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
			}
			resp, err := h.CreateClusterWorkflow(ctx, gen.CreateClusterWorkflowRequestObject{Body: body})
			require.NoError(t, err)
			assert.IsType(t, tt.wantTyp, resp)
		})
	}
}

func TestUpdateClusterWorkflowHandler(t *testing.T) {
	ctx := testContext()
	body := &gen.ClusterWorkflow{Metadata: gen.ObjectMeta{Name: "cwf"}}

	t.Run("uses path name", func(t *testing.T) {
		svc := clusterworkflowmocks.NewMockService(t)
		svc.EXPECT().UpdateClusterWorkflow(mock.Anything, mock.Anything).RunAndReturn(func(_ context.Context, cwf *openchoreov1alpha1.ClusterWorkflow) (*openchoreov1alpha1.ClusterWorkflow, error) {
			assert.Equal(t, "from-path", cwf.Name)
			return cwf, nil
		})
		h := &Handler{
			services: &handlerservices.Services{ClusterWorkflowService: svc},
			logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
		}
		resp, err := h.UpdateClusterWorkflow(ctx, gen.UpdateClusterWorkflowRequestObject{
			ClusterWorkflowName: "from-path",
			Body:                &gen.ClusterWorkflow{Metadata: gen.ObjectMeta{Name: "from-body"}},
		})
		require.NoError(t, err)
		typed, ok := resp.(gen.UpdateClusterWorkflow200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
		assert.Equal(t, "from-path", typed.Metadata.Name)
	})

	t.Run("nil body returns 400", func(t *testing.T) {
		h := &Handler{
			services: &handlerservices.Services{ClusterWorkflowService: clusterworkflowmocks.NewMockService(t)},
			logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
		}
		resp, err := h.UpdateClusterWorkflow(ctx, gen.UpdateClusterWorkflowRequestObject{
			ClusterWorkflowName: "cwf",
			Body:                nil,
		})
		require.NoError(t, err)
		assert.IsType(t, gen.UpdateClusterWorkflow400JSONResponse{}, resp)
	})

	tests := []struct {
		name    string
		svcErr  error
		wantTyp any
	}{
		{"forbidden -> 403", svcpkg.ErrForbidden, gen.UpdateClusterWorkflow403JSONResponse{}},
		{"not found -> 404", clusterworkflowsvc.ErrClusterWorkflowNotFound, gen.UpdateClusterWorkflow404JSONResponse{}},
		{"validation error -> 400", &svcpkg.ValidationError{Msg: "invalid spec"}, gen.UpdateClusterWorkflow400JSONResponse{}},
		{"validation 422 -> 422", &svcpkg.ValidationError{Msg: "invalid spec", StatusCode: http.StatusUnprocessableEntity}, gen.UpdateClusterWorkflow422JSONResponse{}},
		{"internal -> 500", errors.New("internal server error"), gen.UpdateClusterWorkflow500JSONResponse{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := clusterworkflowmocks.NewMockService(t)
			svc.EXPECT().UpdateClusterWorkflow(mock.Anything, mock.Anything).Return(nil, tt.svcErr)
			h := &Handler{
				services: &handlerservices.Services{ClusterWorkflowService: svc},
				logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
			}
			resp, err := h.UpdateClusterWorkflow(ctx, gen.UpdateClusterWorkflowRequestObject{
				ClusterWorkflowName: "cwf",
				Body:                body,
			})
			require.NoError(t, err)
			assert.IsType(t, tt.wantTyp, resp)
		})
	}
}

func TestDeleteClusterWorkflowHandler_InternalError(t *testing.T) {
	ctx := testContext()
	svc := clusterworkflowmocks.NewMockService(t)
	svc.EXPECT().DeleteClusterWorkflow(mock.Anything, "cwf").Return(errors.New("internal server error"))
	h := &Handler{
		services: &handlerservices.Services{ClusterWorkflowService: svc},
		logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	resp, err := h.DeleteClusterWorkflow(ctx, gen.DeleteClusterWorkflowRequestObject{ClusterWorkflowName: "cwf"})
	require.NoError(t, err)
	assert.IsType(t, gen.DeleteClusterWorkflow500JSONResponse{}, resp)
}

func TestGetClusterWorkflowSchemaHandler_InternalError(t *testing.T) {
	ctx := testContext()
	svc := clusterworkflowmocks.NewMockService(t)
	svc.EXPECT().GetClusterWorkflowSchema(mock.Anything, "cwf").Return(nil, errors.New("internal server error"))
	h := &Handler{
		services: &handlerservices.Services{ClusterWorkflowService: svc},
		logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	resp, err := h.GetClusterWorkflowSchema(ctx, gen.GetClusterWorkflowSchemaRequestObject{ClusterWorkflowName: "cwf"})
	require.NoError(t, err)
	assert.IsType(t, gen.GetClusterWorkflowSchema500JSONResponse{}, resp)
}

// --- Additional ClusterComponentType handler coverage ---

func TestGetClusterComponentTypeHandler(t *testing.T) {
	ctx := testContext()
	cct := &openchoreov1alpha1.ClusterComponentType{
		ObjectMeta: metav1.ObjectMeta{Name: "go-service"},
		Spec: openchoreov1alpha1.ClusterComponentTypeSpec{
			WorkloadType: "deployment",
			Resources:    []openchoreov1alpha1.ResourceTemplate{{ID: "deployment"}},
		},
	}

	t.Run("success", func(t *testing.T) {
		svc := newClusterComponentTypeService(t, []client.Object{cct}, &allowAllPDP{})
		h := newHandlerWithClusterComponentTypeService(svc)
		resp, err := h.GetClusterComponentType(ctx, gen.GetClusterComponentTypeRequestObject{CctName: "go-service"})
		require.NoError(t, err)
		typed, ok := resp.(gen.GetClusterComponentType200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
		assert.Equal(t, "go-service", typed.Metadata.Name)
	})

	t.Run("not found returns 404", func(t *testing.T) {
		svc := newClusterComponentTypeService(t, nil, &allowAllPDP{})
		h := newHandlerWithClusterComponentTypeService(svc)
		resp, err := h.GetClusterComponentType(ctx, gen.GetClusterComponentTypeRequestObject{CctName: "missing"})
		require.NoError(t, err)
		assert.IsType(t, gen.GetClusterComponentType404JSONResponse{}, resp)
	})

	t.Run("forbidden returns 403", func(t *testing.T) {
		svc := newClusterComponentTypeService(t, []client.Object{cct}, &denyAllPDP{})
		h := newHandlerWithClusterComponentTypeService(svc)
		resp, err := h.GetClusterComponentType(ctx, gen.GetClusterComponentTypeRequestObject{CctName: "go-service"})
		require.NoError(t, err)
		assert.IsType(t, gen.GetClusterComponentType403JSONResponse{}, resp)
	})

	t.Run("internal error returns 500", func(t *testing.T) {
		svc := cctsvcmocks.NewMockService(t)
		svc.EXPECT().GetClusterComponentType(mock.Anything, "go-service").Return(nil, errors.New("internal server error"))
		h := &Handler{
			services: &handlerservices.Services{ClusterComponentTypeService: svc},
			logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
		}
		resp, err := h.GetClusterComponentType(ctx, gen.GetClusterComponentTypeRequestObject{CctName: "go-service"})
		require.NoError(t, err)
		assert.IsType(t, gen.GetClusterComponentType500JSONResponse{}, resp)
	})
}

func TestDeleteClusterComponentTypeHandler(t *testing.T) {
	ctx := testContext()
	cct := &openchoreov1alpha1.ClusterComponentType{
		ObjectMeta: metav1.ObjectMeta{Name: "go-service"},
		Spec: openchoreov1alpha1.ClusterComponentTypeSpec{
			WorkloadType: "deployment",
			Resources:    []openchoreov1alpha1.ResourceTemplate{{ID: "deployment"}},
		},
	}

	t.Run("success", func(t *testing.T) {
		svc := newClusterComponentTypeService(t, []client.Object{cct}, &allowAllPDP{})
		h := newHandlerWithClusterComponentTypeService(svc)
		resp, err := h.DeleteClusterComponentType(ctx, gen.DeleteClusterComponentTypeRequestObject{CctName: "go-service"})
		require.NoError(t, err)
		assert.IsType(t, gen.DeleteClusterComponentType204Response{}, resp)
	})

	t.Run("not found returns 404", func(t *testing.T) {
		svc := newClusterComponentTypeService(t, nil, &allowAllPDP{})
		h := newHandlerWithClusterComponentTypeService(svc)
		resp, err := h.DeleteClusterComponentType(ctx, gen.DeleteClusterComponentTypeRequestObject{CctName: "missing"})
		require.NoError(t, err)
		assert.IsType(t, gen.DeleteClusterComponentType404JSONResponse{}, resp)
	})

	t.Run("forbidden returns 403", func(t *testing.T) {
		svc := newClusterComponentTypeService(t, []client.Object{cct}, &denyAllPDP{})
		h := newHandlerWithClusterComponentTypeService(svc)
		resp, err := h.DeleteClusterComponentType(ctx, gen.DeleteClusterComponentTypeRequestObject{CctName: "go-service"})
		require.NoError(t, err)
		assert.IsType(t, gen.DeleteClusterComponentType403JSONResponse{}, resp)
	})

	t.Run("internal error returns 500", func(t *testing.T) {
		svc := cctsvcmocks.NewMockService(t)
		svc.EXPECT().DeleteClusterComponentType(mock.Anything, "go-service").Return(errors.New("internal server error"))
		h := &Handler{
			services: &handlerservices.Services{ClusterComponentTypeService: svc},
			logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
		}
		resp, err := h.DeleteClusterComponentType(ctx, gen.DeleteClusterComponentTypeRequestObject{CctName: "go-service"})
		require.NoError(t, err)
		assert.IsType(t, gen.DeleteClusterComponentType500JSONResponse{}, resp)
	})
}

func TestListClusterComponentTypesHandler_MapsErrors(t *testing.T) {
	ctx := testContext()
	tests := []struct {
		name    string
		svcErr  error
		wantTyp any
	}{
		{"validation -> 400", &svcpkg.ValidationError{Msg: "bad request"}, gen.ListClusterComponentTypes400JSONResponse{}},
		{"internal -> 500", errors.New("internal server error"), gen.ListClusterComponentTypes500JSONResponse{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := cctsvcmocks.NewMockService(t)
			svc.EXPECT().ListClusterComponentTypes(mock.Anything, mock.Anything).Return(nil, tt.svcErr)
			h := &Handler{
				services: &handlerservices.Services{ClusterComponentTypeService: svc},
				logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
			}
			resp, err := h.ListClusterComponentTypes(ctx, gen.ListClusterComponentTypesRequestObject{})
			require.NoError(t, err)
			assert.IsType(t, tt.wantTyp, resp)
		})
	}
}

func TestCreateClusterComponentTypeHandler_ValidationError(t *testing.T) {
	ctx := testContext()
	svc := cctsvcmocks.NewMockService(t)
	svc.EXPECT().CreateClusterComponentType(mock.Anything, mock.Anything).Return(nil, &svcpkg.ValidationError{Msg: "bad request"})
	h := &Handler{
		services: &handlerservices.Services{ClusterComponentTypeService: svc},
		logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	body := gen.ClusterComponentType{Metadata: gen.ObjectMeta{Name: "cct"}}
	resp, err := h.CreateClusterComponentType(ctx, gen.CreateClusterComponentTypeRequestObject{Body: &body})
	require.NoError(t, err)
	assert.IsType(t, gen.CreateClusterComponentType400JSONResponse{}, resp)
}

func TestCreateClusterComponentTypeHandler_NilBody(t *testing.T) {
	ctx := testContext()
	h := &Handler{
		services: &handlerservices.Services{ClusterComponentTypeService: cctsvcmocks.NewMockService(t)},
		logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	resp, err := h.CreateClusterComponentType(ctx, gen.CreateClusterComponentTypeRequestObject{Body: nil})
	require.NoError(t, err)
	assert.IsType(t, gen.CreateClusterComponentType400JSONResponse{}, resp)
}

func TestCreateClusterComponentTypeHandler_Success(t *testing.T) {
	ctx := testContext()
	svc := cctsvcmocks.NewMockService(t)
	svc.EXPECT().CreateClusterComponentType(mock.Anything, mock.Anything).RunAndReturn(func(_ context.Context, cct *openchoreov1alpha1.ClusterComponentType) (*openchoreov1alpha1.ClusterComponentType, error) {
		return cct, nil
	})
	h := &Handler{
		services: &handlerservices.Services{ClusterComponentTypeService: svc},
		logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	body := gen.ClusterComponentType{Metadata: gen.ObjectMeta{Name: "cct"}}
	resp, err := h.CreateClusterComponentType(ctx, gen.CreateClusterComponentTypeRequestObject{Body: &body})
	require.NoError(t, err)
	typed, ok := resp.(gen.CreateClusterComponentType201JSONResponse)
	require.True(t, ok, "expected 201 response, got %T", resp)
	assert.Equal(t, "cct", typed.Metadata.Name)
}

func TestCreateClusterComponentTypeHandler_InternalError(t *testing.T) {
	ctx := testContext()
	svc := cctsvcmocks.NewMockService(t)
	svc.EXPECT().CreateClusterComponentType(mock.Anything, mock.Anything).Return(nil, errors.New("internal server error"))
	h := &Handler{
		services: &handlerservices.Services{ClusterComponentTypeService: svc},
		logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	body := gen.ClusterComponentType{Metadata: gen.ObjectMeta{Name: "cct"}}
	resp, err := h.CreateClusterComponentType(ctx, gen.CreateClusterComponentTypeRequestObject{Body: &body})
	require.NoError(t, err)
	assert.IsType(t, gen.CreateClusterComponentType500JSONResponse{}, resp)
}

func TestUpdateClusterComponentTypeHandler_NilBody(t *testing.T) {
	ctx := testContext()
	h := &Handler{
		services: &handlerservices.Services{ClusterComponentTypeService: cctsvcmocks.NewMockService(t)},
		logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	resp, err := h.UpdateClusterComponentType(ctx, gen.UpdateClusterComponentTypeRequestObject{CctName: "cct", Body: nil})
	require.NoError(t, err)
	assert.IsType(t, gen.UpdateClusterComponentType400JSONResponse{}, resp)
}

func TestUpdateClusterComponentTypeHandler_InternalError(t *testing.T) {
	ctx := testContext()
	svc := cctsvcmocks.NewMockService(t)
	svc.EXPECT().UpdateClusterComponentType(mock.Anything, mock.Anything).Return(nil, errors.New("internal server error"))
	h := &Handler{
		services: &handlerservices.Services{ClusterComponentTypeService: svc},
		logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	body := gen.ClusterComponentType{Metadata: gen.ObjectMeta{Name: "cct"}}
	resp, err := h.UpdateClusterComponentType(ctx, gen.UpdateClusterComponentTypeRequestObject{CctName: "cct", Body: &body})
	require.NoError(t, err)
	assert.IsType(t, gen.UpdateClusterComponentType500JSONResponse{}, resp)
}
