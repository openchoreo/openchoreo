// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	authzcore "github.com/openchoreo/openchoreo/internal/authz/core"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/handlerservices"
	secretreferencesvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/secretreference"
)

func newSecretReferenceService(
	t *testing.T, objects []client.Object, pdp authzcore.PDP,
) secretreferencesvc.Service {
	t.Helper()
	fakeClient := fake.NewClientBuilder().
		WithScheme(newTestScheme(t)).
		WithObjects(objects...).
		Build()
	return secretreferencesvc.NewServiceWithAuthz(fakeClient, pdp, slog.Default())
}

func newHandlerWithSecretReferenceService(svc secretreferencesvc.Service) *Handler {
	return &Handler{
		services: &handlerservices.Services{SecretReferenceService: svc},
		logger:   slog.Default(),
	}
}

func testSecretReferenceObj(name string) *openchoreov1alpha1.SecretReference {
	return &openchoreov1alpha1.SecretReference{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "test-ns",
		},
	}
}

// --- ListSecretReferences Handler ---

func TestListSecretReferencesHandler(t *testing.T) {
	ctx := testContext()
	const ns = "test-ns"

	t.Run("success - returns items", func(t *testing.T) {
		svc := newSecretReferenceService(t, []client.Object{testSecretReferenceObj("sr-1")}, &allowAllPDP{})
		h := newHandlerWithSecretReferenceService(svc)

		resp, err := h.ListSecretReferences(ctx, gen.ListSecretReferencesRequestObject{NamespaceName: ns})
		require.NoError(t, err)
		typed, ok := resp.(gen.ListSecretReferences200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
		require.Len(t, typed.Items, 1)
		assert.Equal(t, "sr-1", typed.Items[0].Metadata.Name)
	})

	t.Run("validation error returns 400", func(t *testing.T) {
		svc := newSecretReferenceService(t, nil, &allowAllPDP{})
		h := newHandlerWithSecretReferenceService(svc)

		resp, err := h.ListSecretReferences(ctx, gen.ListSecretReferencesRequestObject{
			NamespaceName: ns,
			Params:        gen.ListSecretReferencesParams{LabelSelector: ptr.To("===invalid")},
		})
		require.NoError(t, err)
		assert.IsType(t, gen.ListSecretReferences400JSONResponse{}, resp)
	})

	t.Run("empty list returns 200", func(t *testing.T) {
		svc := newSecretReferenceService(t, nil, &allowAllPDP{})
		h := newHandlerWithSecretReferenceService(svc)

		resp, err := h.ListSecretReferences(ctx, gen.ListSecretReferencesRequestObject{NamespaceName: ns})
		require.NoError(t, err)
		typed, ok := resp.(gen.ListSecretReferences200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
		assert.Empty(t, typed.Items)
	})
}

// --- GetSecretReference Handler ---

func TestGetSecretReferenceHandler(t *testing.T) {
	ctx := testContext()
	const ns = "test-ns"

	t.Run("success", func(t *testing.T) {
		svc := newSecretReferenceService(t, []client.Object{testSecretReferenceObj("sr-1")}, &allowAllPDP{})
		h := newHandlerWithSecretReferenceService(svc)

		resp, err := h.GetSecretReference(ctx, gen.GetSecretReferenceRequestObject{
			NamespaceName:       ns,
			SecretReferenceName: "sr-1",
		})
		require.NoError(t, err)
		typed, ok := resp.(gen.GetSecretReference200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
		assert.Equal(t, "sr-1", typed.Metadata.Name)
	})

	t.Run("not found returns 404", func(t *testing.T) {
		svc := newSecretReferenceService(t, nil, &allowAllPDP{})
		h := newHandlerWithSecretReferenceService(svc)

		resp, err := h.GetSecretReference(ctx, gen.GetSecretReferenceRequestObject{
			NamespaceName:       ns,
			SecretReferenceName: "nonexistent",
		})
		require.NoError(t, err)
		assert.IsType(t, gen.GetSecretReference404JSONResponse{}, resp)
	})

	t.Run("forbidden returns 403", func(t *testing.T) {
		svc := newSecretReferenceService(t, []client.Object{testSecretReferenceObj("sr-1")}, &denyAllPDP{})
		h := newHandlerWithSecretReferenceService(svc)

		resp, err := h.GetSecretReference(ctx, gen.GetSecretReferenceRequestObject{
			NamespaceName:       ns,
			SecretReferenceName: "sr-1",
		})
		require.NoError(t, err)
		assert.IsType(t, gen.GetSecretReference403JSONResponse{}, resp)
	})
}

// --- CreateSecretReference Handler ---

func TestCreateSecretReferenceHandler(t *testing.T) {
	ctx := testContext()
	const ns = "test-ns"

	validBody := &gen.SecretReference{
		Metadata: gen.ObjectMeta{Name: "new-sr"},
	}

	t.Run("success", func(t *testing.T) {
		svc := newSecretReferenceService(t, nil, &allowAllPDP{})
		h := newHandlerWithSecretReferenceService(svc)

		resp, err := h.CreateSecretReference(ctx, gen.CreateSecretReferenceRequestObject{
			NamespaceName: ns,
			Body:          validBody,
		})
		require.NoError(t, err)
		typed, ok := resp.(gen.CreateSecretReference201JSONResponse)
		require.True(t, ok, "expected 201 response, got %T", resp)
		assert.Equal(t, "new-sr", typed.Metadata.Name)
	})

	t.Run("nil body returns 400", func(t *testing.T) {
		svc := newSecretReferenceService(t, nil, &allowAllPDP{})
		h := newHandlerWithSecretReferenceService(svc)

		resp, err := h.CreateSecretReference(ctx, gen.CreateSecretReferenceRequestObject{
			NamespaceName: ns,
			Body:          nil,
		})
		require.NoError(t, err)
		assert.IsType(t, gen.CreateSecretReference400JSONResponse{}, resp)
	})

	t.Run("already exists returns 409", func(t *testing.T) {
		existing := testSecretReferenceObj("new-sr")
		svc := newSecretReferenceService(t, []client.Object{existing}, &allowAllPDP{})
		h := newHandlerWithSecretReferenceService(svc)

		resp, err := h.CreateSecretReference(ctx, gen.CreateSecretReferenceRequestObject{
			NamespaceName: ns,
			Body:          validBody,
		})
		require.NoError(t, err)
		assert.IsType(t, gen.CreateSecretReference409JSONResponse{}, resp)
	})

	t.Run("forbidden returns 403", func(t *testing.T) {
		svc := newSecretReferenceService(t, nil, &denyAllPDP{})
		h := newHandlerWithSecretReferenceService(svc)

		resp, err := h.CreateSecretReference(ctx, gen.CreateSecretReferenceRequestObject{
			NamespaceName: ns,
			Body:          validBody,
		})
		require.NoError(t, err)
		assert.IsType(t, gen.CreateSecretReference403JSONResponse{}, resp)
	})
}

// --- UpdateSecretReference Handler ---

func TestUpdateSecretReferenceHandler(t *testing.T) {
	ctx := testContext()
	const ns = "test-ns"

	t.Run("success", func(t *testing.T) {
		svc := newSecretReferenceService(t, []client.Object{testSecretReferenceObj("sr-1")}, &allowAllPDP{})
		h := newHandlerWithSecretReferenceService(svc)

		resp, err := h.UpdateSecretReference(ctx, gen.UpdateSecretReferenceRequestObject{
			NamespaceName:       ns,
			SecretReferenceName: "sr-1",
			Body:                &gen.SecretReference{Metadata: gen.ObjectMeta{Name: "sr-1"}},
		})
		require.NoError(t, err)
		typed, ok := resp.(gen.UpdateSecretReference200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
		assert.Equal(t, "sr-1", typed.Metadata.Name)
	})

	t.Run("nil body returns 400", func(t *testing.T) {
		svc := newSecretReferenceService(t, nil, &allowAllPDP{})
		h := newHandlerWithSecretReferenceService(svc)

		resp, err := h.UpdateSecretReference(ctx, gen.UpdateSecretReferenceRequestObject{
			NamespaceName:       ns,
			SecretReferenceName: "sr-1",
			Body:                nil,
		})
		require.NoError(t, err)
		assert.IsType(t, gen.UpdateSecretReference400JSONResponse{}, resp)
	})

	t.Run("not found returns 404", func(t *testing.T) {
		svc := newSecretReferenceService(t, nil, &allowAllPDP{})
		h := newHandlerWithSecretReferenceService(svc)

		resp, err := h.UpdateSecretReference(ctx, gen.UpdateSecretReferenceRequestObject{
			NamespaceName:       ns,
			SecretReferenceName: "nonexistent",
			Body:                &gen.SecretReference{Metadata: gen.ObjectMeta{Name: "nonexistent"}},
		})
		require.NoError(t, err)
		assert.IsType(t, gen.UpdateSecretReference404JSONResponse{}, resp)
	})

	t.Run("forbidden returns 403", func(t *testing.T) {
		svc := newSecretReferenceService(t, []client.Object{testSecretReferenceObj("sr-1")}, &denyAllPDP{})
		h := newHandlerWithSecretReferenceService(svc)

		resp, err := h.UpdateSecretReference(ctx, gen.UpdateSecretReferenceRequestObject{
			NamespaceName:       ns,
			SecretReferenceName: "sr-1",
			Body:                &gen.SecretReference{Metadata: gen.ObjectMeta{Name: "sr-1"}},
		})
		require.NoError(t, err)
		assert.IsType(t, gen.UpdateSecretReference403JSONResponse{}, resp)
	})

	t.Run("URL path name overrides body name", func(t *testing.T) {
		svc := newSecretReferenceService(t, []client.Object{testSecretReferenceObj("sr-1")}, &allowAllPDP{})
		h := newHandlerWithSecretReferenceService(svc)

		resp, err := h.UpdateSecretReference(ctx, gen.UpdateSecretReferenceRequestObject{
			NamespaceName:       ns,
			SecretReferenceName: "sr-1",
			Body:                &gen.SecretReference{Metadata: gen.ObjectMeta{Name: "different-name"}},
		})
		require.NoError(t, err)
		typed, ok := resp.(gen.UpdateSecretReference200JSONResponse)
		require.True(t, ok, "expected 200 response (URL path name used), got %T", resp)
		assert.Equal(t, "sr-1", typed.Metadata.Name)
	})
}

// --- DeleteSecretReference Handler ---

func TestDeleteSecretReferenceHandler(t *testing.T) {
	ctx := testContext()
	const ns = "test-ns"

	t.Run("success", func(t *testing.T) {
		svc := newSecretReferenceService(t, []client.Object{testSecretReferenceObj("sr-1")}, &allowAllPDP{})
		h := newHandlerWithSecretReferenceService(svc)

		resp, err := h.DeleteSecretReference(ctx, gen.DeleteSecretReferenceRequestObject{
			NamespaceName:       ns,
			SecretReferenceName: "sr-1",
		})
		require.NoError(t, err)
		assert.IsType(t, gen.DeleteSecretReference204Response{}, resp)
	})

	t.Run("not found returns 404", func(t *testing.T) {
		svc := newSecretReferenceService(t, nil, &allowAllPDP{})
		h := newHandlerWithSecretReferenceService(svc)

		resp, err := h.DeleteSecretReference(ctx, gen.DeleteSecretReferenceRequestObject{
			NamespaceName:       ns,
			SecretReferenceName: "nonexistent",
		})
		require.NoError(t, err)
		assert.IsType(t, gen.DeleteSecretReference404JSONResponse{}, resp)
	})

	t.Run("forbidden returns 403", func(t *testing.T) {
		svc := newSecretReferenceService(t, []client.Object{testSecretReferenceObj("sr-1")}, &denyAllPDP{})
		h := newHandlerWithSecretReferenceService(svc)

		resp, err := h.DeleteSecretReference(ctx, gen.DeleteSecretReferenceRequestObject{
			NamespaceName:       ns,
			SecretReferenceName: "sr-1",
		})
		require.NoError(t, err)
		assert.IsType(t, gen.DeleteSecretReference403JSONResponse{}, resp)
	})
}
