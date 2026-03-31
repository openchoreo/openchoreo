// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"encoding/json"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	authzcore "github.com/openchoreo/openchoreo/internal/authz/core"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/handlerservices"
	traitsvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/trait"
)

func newTraitService(t *testing.T, objects []client.Object, pdp authzcore.PDP) traitsvc.Service {
	t.Helper()
	fakeClient := fake.NewClientBuilder().
		WithScheme(newTestScheme(t)).
		WithObjects(objects...).
		Build()
	return traitsvc.NewServiceWithAuthz(fakeClient, pdp, slog.Default())
}

func newHandlerWithTraitService(svc traitsvc.Service) *Handler {
	return &Handler{
		services: &handlerservices.Services{TraitService: svc},
		logger:   slog.Default(),
	}
}

func testTraitObj(name string) *openchoreov1alpha1.Trait {
	return &openchoreov1alpha1.Trait{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "test-ns"},
	}
}

// --- ListTraits Handler ---

func TestListTraitsHandler(t *testing.T) {
	ctx := testContext()
	const ns = "test-ns"

	t.Run("success - returns items", func(t *testing.T) {
		svc := newTraitService(t, []client.Object{testTraitObj("trait-1")}, &allowAllPDP{})
		h := newHandlerWithTraitService(svc)

		resp, err := h.ListTraits(ctx, gen.ListTraitsRequestObject{NamespaceName: ns})
		require.NoError(t, err)
		typed, ok := resp.(gen.ListTraits200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
		require.Len(t, typed.Items, 1)
		assert.Equal(t, "trait-1", typed.Items[0].Metadata.Name)
	})

	t.Run("empty list returns 200", func(t *testing.T) {
		svc := newTraitService(t, nil, &allowAllPDP{})
		h := newHandlerWithTraitService(svc)

		resp, err := h.ListTraits(ctx, gen.ListTraitsRequestObject{NamespaceName: ns})
		require.NoError(t, err)
		typed, ok := resp.(gen.ListTraits200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
		assert.Empty(t, typed.Items)
	})

	t.Run("validation error returns 400", func(t *testing.T) {
		svc := newTraitService(t, nil, &allowAllPDP{})
		h := newHandlerWithTraitService(svc)

		resp, err := h.ListTraits(ctx, gen.ListTraitsRequestObject{
			NamespaceName: ns,
			Params:        gen.ListTraitsParams{LabelSelector: ptr.To("===invalid")},
		})
		require.NoError(t, err)
		assert.IsType(t, gen.ListTraits400JSONResponse{}, resp)
	})

	t.Run("unauthorized items filtered out", func(t *testing.T) {
		svc := newTraitService(t, []client.Object{testTraitObj("trait-1")}, &denyAllPDP{})
		h := newHandlerWithTraitService(svc)

		resp, err := h.ListTraits(ctx, gen.ListTraitsRequestObject{NamespaceName: ns})
		require.NoError(t, err)
		typed, ok := resp.(gen.ListTraits200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
		assert.Empty(t, typed.Items)
	})
}

// --- GetTrait Handler ---

func TestGetTraitHandler(t *testing.T) {
	ctx := testContext()
	const ns = "test-ns"

	t.Run("success", func(t *testing.T) {
		svc := newTraitService(t, []client.Object{testTraitObj("trait-1")}, &allowAllPDP{})
		h := newHandlerWithTraitService(svc)

		resp, err := h.GetTrait(ctx, gen.GetTraitRequestObject{NamespaceName: ns, TraitName: "trait-1"})
		require.NoError(t, err)
		typed, ok := resp.(gen.GetTrait200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
		assert.Equal(t, "trait-1", typed.Metadata.Name)
	})

	t.Run("not found returns 404", func(t *testing.T) {
		svc := newTraitService(t, nil, &allowAllPDP{})
		h := newHandlerWithTraitService(svc)

		resp, err := h.GetTrait(ctx, gen.GetTraitRequestObject{NamespaceName: ns, TraitName: "nonexistent"})
		require.NoError(t, err)
		assert.IsType(t, gen.GetTrait404JSONResponse{}, resp)
	})

	t.Run("forbidden returns 403", func(t *testing.T) {
		svc := newTraitService(t, []client.Object{testTraitObj("trait-1")}, &denyAllPDP{})
		h := newHandlerWithTraitService(svc)

		resp, err := h.GetTrait(ctx, gen.GetTraitRequestObject{NamespaceName: ns, TraitName: "trait-1"})
		require.NoError(t, err)
		assert.IsType(t, gen.GetTrait403JSONResponse{}, resp)
	})
}

// --- CreateTrait Handler ---

func TestCreateTraitHandler(t *testing.T) {
	ctx := testContext()
	const ns = "test-ns"

	t.Run("success", func(t *testing.T) {
		svc := newTraitService(t, nil, &allowAllPDP{})
		h := newHandlerWithTraitService(svc)

		resp, err := h.CreateTrait(ctx, gen.CreateTraitRequestObject{
			NamespaceName: ns,
			Body:          &gen.Trait{Metadata: gen.ObjectMeta{Name: "new-trait"}},
		})
		require.NoError(t, err)
		typed, ok := resp.(gen.CreateTrait201JSONResponse)
		require.True(t, ok, "expected 201 response, got %T", resp)
		assert.Equal(t, "new-trait", typed.Metadata.Name)
	})

	t.Run("nil body returns 400", func(t *testing.T) {
		svc := newTraitService(t, nil, &allowAllPDP{})
		h := newHandlerWithTraitService(svc)

		resp, err := h.CreateTrait(ctx, gen.CreateTraitRequestObject{
			NamespaceName: ns,
			Body:          nil,
		})
		require.NoError(t, err)
		assert.IsType(t, gen.CreateTrait400JSONResponse{}, resp)
	})

	t.Run("already exists returns 409", func(t *testing.T) {
		existing := testTraitObj("new-trait")
		svc := newTraitService(t, []client.Object{existing}, &allowAllPDP{})
		h := newHandlerWithTraitService(svc)

		resp, err := h.CreateTrait(ctx, gen.CreateTraitRequestObject{
			NamespaceName: ns,
			Body:          &gen.Trait{Metadata: gen.ObjectMeta{Name: "new-trait"}},
		})
		require.NoError(t, err)
		assert.IsType(t, gen.CreateTrait409JSONResponse{}, resp)
	})

	t.Run("forbidden returns 403", func(t *testing.T) {
		svc := newTraitService(t, nil, &denyAllPDP{})
		h := newHandlerWithTraitService(svc)

		resp, err := h.CreateTrait(ctx, gen.CreateTraitRequestObject{
			NamespaceName: ns,
			Body:          &gen.Trait{Metadata: gen.ObjectMeta{Name: "new-trait"}},
		})
		require.NoError(t, err)
		assert.IsType(t, gen.CreateTrait403JSONResponse{}, resp)
	})
}

// --- UpdateTrait Handler ---

func TestUpdateTraitHandler(t *testing.T) {
	ctx := testContext()
	const ns = "test-ns"

	t.Run("success", func(t *testing.T) {
		svc := newTraitService(t, []client.Object{testTraitObj("trait-1")}, &allowAllPDP{})
		h := newHandlerWithTraitService(svc)

		resp, err := h.UpdateTrait(ctx, gen.UpdateTraitRequestObject{
			NamespaceName: ns,
			TraitName:     "trait-1",
			Body:          &gen.Trait{Metadata: gen.ObjectMeta{Name: "trait-1"}},
		})
		require.NoError(t, err)
		typed, ok := resp.(gen.UpdateTrait200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
		assert.Equal(t, "trait-1", typed.Metadata.Name)
	})

	t.Run("nil body returns 400", func(t *testing.T) {
		svc := newTraitService(t, nil, &allowAllPDP{})
		h := newHandlerWithTraitService(svc)

		resp, err := h.UpdateTrait(ctx, gen.UpdateTraitRequestObject{
			NamespaceName: ns,
			TraitName:     "trait-1",
			Body:          nil,
		})
		require.NoError(t, err)
		assert.IsType(t, gen.UpdateTrait400JSONResponse{}, resp)
	})

	t.Run("not found returns 404", func(t *testing.T) {
		svc := newTraitService(t, nil, &allowAllPDP{})
		h := newHandlerWithTraitService(svc)

		resp, err := h.UpdateTrait(ctx, gen.UpdateTraitRequestObject{
			NamespaceName: ns,
			TraitName:     "nonexistent",
			Body:          &gen.Trait{Metadata: gen.ObjectMeta{Name: "nonexistent"}},
		})
		require.NoError(t, err)
		assert.IsType(t, gen.UpdateTrait404JSONResponse{}, resp)
	})

	t.Run("forbidden returns 403", func(t *testing.T) {
		svc := newTraitService(t, []client.Object{testTraitObj("trait-1")}, &denyAllPDP{})
		h := newHandlerWithTraitService(svc)

		resp, err := h.UpdateTrait(ctx, gen.UpdateTraitRequestObject{
			NamespaceName: ns,
			TraitName:     "trait-1",
			Body:          &gen.Trait{Metadata: gen.ObjectMeta{Name: "trait-1"}},
		})
		require.NoError(t, err)
		assert.IsType(t, gen.UpdateTrait403JSONResponse{}, resp)
	})

	t.Run("URL path name overrides body name", func(t *testing.T) {
		svc := newTraitService(t, []client.Object{testTraitObj("trait-1")}, &allowAllPDP{})
		h := newHandlerWithTraitService(svc)

		resp, err := h.UpdateTrait(ctx, gen.UpdateTraitRequestObject{
			NamespaceName: ns,
			TraitName:     "trait-1",
			Body:          &gen.Trait{Metadata: gen.ObjectMeta{Name: "different-name"}},
		})
		require.NoError(t, err)
		typed, ok := resp.(gen.UpdateTrait200JSONResponse)
		require.True(t, ok, "expected 200 response (URL path name used), got %T", resp)
		assert.Equal(t, "trait-1", typed.Metadata.Name)
	})
}

// --- DeleteTrait Handler ---

func TestDeleteTraitHandler(t *testing.T) {
	ctx := testContext()
	const ns = "test-ns"

	t.Run("success", func(t *testing.T) {
		svc := newTraitService(t, []client.Object{testTraitObj("trait-1")}, &allowAllPDP{})
		h := newHandlerWithTraitService(svc)

		resp, err := h.DeleteTrait(ctx, gen.DeleteTraitRequestObject{NamespaceName: ns, TraitName: "trait-1"})
		require.NoError(t, err)
		assert.IsType(t, gen.DeleteTrait204Response{}, resp)
	})

	t.Run("not found returns 404", func(t *testing.T) {
		svc := newTraitService(t, nil, &allowAllPDP{})
		h := newHandlerWithTraitService(svc)

		resp, err := h.DeleteTrait(ctx, gen.DeleteTraitRequestObject{NamespaceName: ns, TraitName: "nonexistent"})
		require.NoError(t, err)
		assert.IsType(t, gen.DeleteTrait404JSONResponse{}, resp)
	})

	t.Run("forbidden returns 403", func(t *testing.T) {
		svc := newTraitService(t, []client.Object{testTraitObj("trait-1")}, &denyAllPDP{})
		h := newHandlerWithTraitService(svc)

		resp, err := h.DeleteTrait(ctx, gen.DeleteTraitRequestObject{NamespaceName: ns, TraitName: "trait-1"})
		require.NoError(t, err)
		assert.IsType(t, gen.DeleteTrait403JSONResponse{}, resp)
	})
}

// --- GetTraitSchema Handler ---

func TestGetTraitSchemaHandler(t *testing.T) {
	ctx := testContext()
	const ns = "test-ns"

	paramsRaw, _ := json.Marshal(map[string]any{"minReplicas": "integer"})
	traitObj := &openchoreov1alpha1.Trait{
		ObjectMeta: metav1.ObjectMeta{Name: "autoscaler", Namespace: "test-ns"},
		Spec: openchoreov1alpha1.TraitSpec{
			Parameters: &openchoreov1alpha1.SchemaSection{
				OpenAPIV3Schema: &runtime.RawExtension{Raw: paramsRaw},
			},
		},
	}

	t.Run("success", func(t *testing.T) {
		svc := newTraitService(t, []client.Object{traitObj}, &allowAllPDP{})
		h := newHandlerWithTraitService(svc)

		resp, err := h.GetTraitSchema(ctx, gen.GetTraitSchemaRequestObject{
			NamespaceName: ns,
			TraitName:     "autoscaler",
		})
		require.NoError(t, err)
		_, ok := resp.(gen.GetTraitSchema200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
	})

	t.Run("not found returns 404", func(t *testing.T) {
		svc := newTraitService(t, nil, &allowAllPDP{})
		h := newHandlerWithTraitService(svc)

		resp, err := h.GetTraitSchema(ctx, gen.GetTraitSchemaRequestObject{
			NamespaceName: ns,
			TraitName:     "nonexistent",
		})
		require.NoError(t, err)
		assert.IsType(t, gen.GetTraitSchema404JSONResponse{}, resp)
	})

	t.Run("forbidden returns 403", func(t *testing.T) {
		svc := newTraitService(t, []client.Object{traitObj}, &denyAllPDP{})
		h := newHandlerWithTraitService(svc)

		resp, err := h.GetTraitSchema(ctx, gen.GetTraitSchemaRequestObject{
			NamespaceName: ns,
			TraitName:     "autoscaler",
		})
		require.NoError(t, err)
		assert.IsType(t, gen.GetTraitSchema403JSONResponse{}, resp)
	})
}
