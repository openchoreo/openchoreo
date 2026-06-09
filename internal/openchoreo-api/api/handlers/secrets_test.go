// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"errors"
	"io"
	"log/slog"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/config"
	svcpkg "github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/handlerservices"
	secretsvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/secret"
	secretmocks "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/secret/mocks"
)

const (
	testSecretsNs   = "test-ns"
	testSecretName  = "my-secret"
	testSecretType  = "Opaque"
	testTargetPlane = "default-dp"
)

func newSecretHandler(t *testing.T, enabled bool) (*Handler, *secretmocks.MockService) {
	t.Helper()
	svc := secretmocks.NewMockService(t)
	h := &Handler{
		services: &handlerservices.Services{SecretService: svc},
		logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
		Config:   &config.Config{SecretManagement: config.SecretManagementConfig{Enabled: enabled}},
	}
	return h, svc
}

func sampleSecretCR() *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: testSecretName, Namespace: testSecretsNs},
		Type:       corev1.SecretTypeOpaque,
		Data:       map[string][]byte{"k": []byte("v")},
	}
}

// --- Feature flag (501) ---

func TestSecretHandlers_FeatureDisabledReturns501(t *testing.T) {
	ctx := testContext()

	t.Run("ListSecrets", func(t *testing.T) {
		h, _ := newSecretHandler(t, false)
		resp, err := h.ListSecrets(ctx, gen.ListSecretsRequestObject{NamespaceName: testSecretsNs})
		require.NoError(t, err)
		assert.IsType(t, gen.ListSecrets501JSONResponse{}, resp)
	})

	t.Run("CreateSecret", func(t *testing.T) {
		h, _ := newSecretHandler(t, false)
		resp, err := h.CreateSecret(ctx, gen.CreateSecretRequestObject{NamespaceName: testSecretsNs})
		require.NoError(t, err)
		assert.IsType(t, gen.CreateSecret501JSONResponse{}, resp)
	})

	t.Run("GetSecret", func(t *testing.T) {
		h, _ := newSecretHandler(t, false)
		resp, err := h.GetSecret(ctx, gen.GetSecretRequestObject{NamespaceName: testSecretsNs, SecretName: testSecretName})
		require.NoError(t, err)
		assert.IsType(t, gen.GetSecret501JSONResponse{}, resp)
	})

	t.Run("UpdateSecret", func(t *testing.T) {
		h, _ := newSecretHandler(t, false)
		resp, err := h.UpdateSecret(ctx, gen.UpdateSecretRequestObject{NamespaceName: testSecretsNs, SecretName: testSecretName})
		require.NoError(t, err)
		assert.IsType(t, gen.UpdateSecret501JSONResponse{}, resp)
	})

	t.Run("DeleteSecret", func(t *testing.T) {
		h, _ := newSecretHandler(t, false)
		resp, err := h.DeleteSecret(ctx, gen.DeleteSecretRequestObject{NamespaceName: testSecretsNs, SecretName: testSecretName})
		require.NoError(t, err)
		assert.IsType(t, gen.DeleteSecret501JSONResponse{}, resp)
	})
}

// --- ListSecrets ---

func TestListSecretsHandler(t *testing.T) {
	ctx := testContext()

	t.Run("success returns 200 with items and pagination", func(t *testing.T) {
		h, svc := newSecretHandler(t, true)
		svc.EXPECT().ListSecrets(mock.Anything, testSecretsNs, mock.Anything).
			Return(&svcpkg.ListResult[corev1.Secret]{Items: []corev1.Secret{*sampleSecretCR()}}, nil)

		resp, err := h.ListSecrets(ctx, gen.ListSecretsRequestObject{NamespaceName: testSecretsNs})
		require.NoError(t, err)
		typed, ok := resp.(gen.ListSecrets200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
		require.Len(t, typed.Items, 1)
		assert.Equal(t, testSecretName, typed.Items[0].Metadata.Name)
	})

	t.Run("forbidden returns 403", func(t *testing.T) {
		h, svc := newSecretHandler(t, true)
		svc.EXPECT().ListSecrets(mock.Anything, testSecretsNs, mock.Anything).
			Return(nil, svcpkg.ErrForbidden)

		resp, err := h.ListSecrets(ctx, gen.ListSecretsRequestObject{NamespaceName: testSecretsNs})
		require.NoError(t, err)
		assert.IsType(t, gen.ListSecrets403JSONResponse{}, resp)
	})

	t.Run("validation error returns 400", func(t *testing.T) {
		h, svc := newSecretHandler(t, true)
		svc.EXPECT().ListSecrets(mock.Anything, testSecretsNs, mock.Anything).
			Return(nil, &svcpkg.ValidationError{StatusCode: http.StatusBadRequest, Msg: "bad cursor"})

		resp, err := h.ListSecrets(ctx, gen.ListSecretsRequestObject{NamespaceName: testSecretsNs})
		require.NoError(t, err)
		assert.IsType(t, gen.ListSecrets400JSONResponse{}, resp)
	})

	t.Run("unknown error returns 500", func(t *testing.T) {
		h, svc := newSecretHandler(t, true)
		svc.EXPECT().ListSecrets(mock.Anything, testSecretsNs, mock.Anything).
			Return(nil, errors.New("boom"))

		resp, err := h.ListSecrets(ctx, gen.ListSecretsRequestObject{NamespaceName: testSecretsNs})
		require.NoError(t, err)
		assert.IsType(t, gen.ListSecrets500JSONResponse{}, resp)
	})
}

// --- CreateSecret ---

func TestCreateSecretHandler(t *testing.T) {
	ctx := testContext()

	validBody := &gen.CreateSecretRequest{
		SecretName: testSecretName,
		SecretType: testSecretType,
		TargetPlane: gen.TargetPlaneRef{
			Kind: gen.TargetPlaneRefKind("DataPlane"),
			Name: testTargetPlane,
		},
		Data: map[string]string{"k": "v"},
	}

	t.Run("success returns 201", func(t *testing.T) {
		h, svc := newSecretHandler(t, true)
		svc.EXPECT().CreateSecret(mock.Anything, testSecretsNs, mock.Anything).
			Return(sampleSecretCR(), nil)

		resp, err := h.CreateSecret(ctx, gen.CreateSecretRequestObject{
			NamespaceName: testSecretsNs,
			Body:          validBody,
		})
		require.NoError(t, err)
		typed, ok := resp.(gen.CreateSecret201JSONResponse)
		require.True(t, ok, "expected 201 response, got %T", resp)
		assert.Equal(t, testSecretName, typed.Metadata.Name)
	})

	t.Run("nil body returns 400", func(t *testing.T) {
		h, _ := newSecretHandler(t, true)
		resp, err := h.CreateSecret(ctx, gen.CreateSecretRequestObject{NamespaceName: testSecretsNs, Body: nil})
		require.NoError(t, err)
		assert.IsType(t, gen.CreateSecret400JSONResponse{}, resp)
	})

	t.Run("already exists returns 409", func(t *testing.T) {
		h, svc := newSecretHandler(t, true)
		svc.EXPECT().CreateSecret(mock.Anything, testSecretsNs, mock.Anything).
			Return(nil, secretsvc.ErrSecretAlreadyExists)

		resp, err := h.CreateSecret(ctx, gen.CreateSecretRequestObject{NamespaceName: testSecretsNs, Body: validBody})
		require.NoError(t, err)
		assert.IsType(t, gen.CreateSecret409JSONResponse{}, resp)
	})

	t.Run("plane not found returns 422", func(t *testing.T) {
		h, svc := newSecretHandler(t, true)
		svc.EXPECT().CreateSecret(mock.Anything, testSecretsNs, mock.Anything).
			Return(nil, secretsvc.ErrPlaneNotFound)

		resp, err := h.CreateSecret(ctx, gen.CreateSecretRequestObject{NamespaceName: testSecretsNs, Body: validBody})
		require.NoError(t, err)
		assert.IsType(t, gen.CreateSecret422JSONResponse{}, resp)
	})

	t.Run("secret store not configured returns 400", func(t *testing.T) {
		h, svc := newSecretHandler(t, true)
		svc.EXPECT().CreateSecret(mock.Anything, testSecretsNs, mock.Anything).
			Return(nil, secretsvc.ErrSecretStoreNotConfigured)

		resp, err := h.CreateSecret(ctx, gen.CreateSecretRequestObject{NamespaceName: testSecretsNs, Body: validBody})
		require.NoError(t, err)
		assert.IsType(t, gen.CreateSecret400JSONResponse{}, resp)
	})

	t.Run("forbidden returns 403", func(t *testing.T) {
		h, svc := newSecretHandler(t, true)
		svc.EXPECT().CreateSecret(mock.Anything, testSecretsNs, mock.Anything).
			Return(nil, svcpkg.ErrForbidden)

		resp, err := h.CreateSecret(ctx, gen.CreateSecretRequestObject{NamespaceName: testSecretsNs, Body: validBody})
		require.NoError(t, err)
		assert.IsType(t, gen.CreateSecret403JSONResponse{}, resp)
	})

	t.Run("validation 422 returns 422", func(t *testing.T) {
		h, svc := newSecretHandler(t, true)
		svc.EXPECT().CreateSecret(mock.Anything, testSecretsNs, mock.Anything).
			Return(nil, &svcpkg.ValidationError{StatusCode: http.StatusUnprocessableEntity, Msg: "missing key"})

		resp, err := h.CreateSecret(ctx, gen.CreateSecretRequestObject{NamespaceName: testSecretsNs, Body: validBody})
		require.NoError(t, err)
		assert.IsType(t, gen.CreateSecret422JSONResponse{}, resp)
	})

	t.Run("validation 400 returns 400", func(t *testing.T) {
		h, svc := newSecretHandler(t, true)
		svc.EXPECT().CreateSecret(mock.Anything, testSecretsNs, mock.Anything).
			Return(nil, &svcpkg.ValidationError{StatusCode: http.StatusBadRequest, Msg: "bad input"})

		resp, err := h.CreateSecret(ctx, gen.CreateSecretRequestObject{NamespaceName: testSecretsNs, Body: validBody})
		require.NoError(t, err)
		assert.IsType(t, gen.CreateSecret400JSONResponse{}, resp)
	})

	t.Run("unknown error returns 500", func(t *testing.T) {
		h, svc := newSecretHandler(t, true)
		svc.EXPECT().CreateSecret(mock.Anything, testSecretsNs, mock.Anything).
			Return(nil, errors.New("boom"))

		resp, err := h.CreateSecret(ctx, gen.CreateSecretRequestObject{NamespaceName: testSecretsNs, Body: validBody})
		require.NoError(t, err)
		assert.IsType(t, gen.CreateSecret500JSONResponse{}, resp)
	})
}

// --- GetSecret ---

func TestGetSecretHandler(t *testing.T) {
	ctx := testContext()

	t.Run("success returns 200", func(t *testing.T) {
		h, svc := newSecretHandler(t, true)
		svc.EXPECT().GetSecret(mock.Anything, testSecretsNs, testSecretName).Return(sampleSecretCR(), nil)

		resp, err := h.GetSecret(ctx, gen.GetSecretRequestObject{NamespaceName: testSecretsNs, SecretName: testSecretName})
		require.NoError(t, err)
		typed, ok := resp.(gen.GetSecret200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
		assert.Equal(t, testSecretName, typed.Metadata.Name)
	})

	t.Run("not found returns 404", func(t *testing.T) {
		h, svc := newSecretHandler(t, true)
		svc.EXPECT().GetSecret(mock.Anything, testSecretsNs, testSecretName).Return(nil, secretsvc.ErrSecretNotFound)

		resp, err := h.GetSecret(ctx, gen.GetSecretRequestObject{NamespaceName: testSecretsNs, SecretName: testSecretName})
		require.NoError(t, err)
		assert.IsType(t, gen.GetSecret404JSONResponse{}, resp)
	})

	t.Run("forbidden returns 403", func(t *testing.T) {
		h, svc := newSecretHandler(t, true)
		svc.EXPECT().GetSecret(mock.Anything, testSecretsNs, testSecretName).Return(nil, svcpkg.ErrForbidden)

		resp, err := h.GetSecret(ctx, gen.GetSecretRequestObject{NamespaceName: testSecretsNs, SecretName: testSecretName})
		require.NoError(t, err)
		assert.IsType(t, gen.GetSecret403JSONResponse{}, resp)
	})

	t.Run("unknown error returns 500", func(t *testing.T) {
		h, svc := newSecretHandler(t, true)
		svc.EXPECT().GetSecret(mock.Anything, testSecretsNs, testSecretName).Return(nil, errors.New("boom"))

		resp, err := h.GetSecret(ctx, gen.GetSecretRequestObject{NamespaceName: testSecretsNs, SecretName: testSecretName})
		require.NoError(t, err)
		assert.IsType(t, gen.GetSecret500JSONResponse{}, resp)
	})
}

// --- UpdateSecret ---

func TestUpdateSecretHandler(t *testing.T) {
	ctx := testContext()
	validBody := &gen.UpdateSecretRequest{Data: map[string]string{"k": "v2"}}

	t.Run("success returns 200", func(t *testing.T) {
		h, svc := newSecretHandler(t, true)
		svc.EXPECT().UpdateSecret(mock.Anything, testSecretsNs, testSecretName, mock.Anything).
			Return(sampleSecretCR(), nil)

		resp, err := h.UpdateSecret(ctx, gen.UpdateSecretRequestObject{
			NamespaceName: testSecretsNs, SecretName: testSecretName, Body: validBody,
		})
		require.NoError(t, err)
		typed, ok := resp.(gen.UpdateSecret200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
		assert.Equal(t, testSecretName, typed.Metadata.Name)
	})

	t.Run("nil body returns 400", func(t *testing.T) {
		h, _ := newSecretHandler(t, true)
		resp, err := h.UpdateSecret(ctx, gen.UpdateSecretRequestObject{
			NamespaceName: testSecretsNs, SecretName: testSecretName, Body: nil,
		})
		require.NoError(t, err)
		assert.IsType(t, gen.UpdateSecret400JSONResponse{}, resp)
	})

	t.Run("not found returns 404", func(t *testing.T) {
		h, svc := newSecretHandler(t, true)
		svc.EXPECT().UpdateSecret(mock.Anything, testSecretsNs, testSecretName, mock.Anything).
			Return(nil, secretsvc.ErrSecretNotFound)

		resp, err := h.UpdateSecret(ctx, gen.UpdateSecretRequestObject{
			NamespaceName: testSecretsNs, SecretName: testSecretName, Body: validBody,
		})
		require.NoError(t, err)
		assert.IsType(t, gen.UpdateSecret404JSONResponse{}, resp)
	})

	t.Run("plane not found returns 422", func(t *testing.T) {
		h, svc := newSecretHandler(t, true)
		svc.EXPECT().UpdateSecret(mock.Anything, testSecretsNs, testSecretName, mock.Anything).
			Return(nil, secretsvc.ErrPlaneNotFound)

		resp, err := h.UpdateSecret(ctx, gen.UpdateSecretRequestObject{
			NamespaceName: testSecretsNs, SecretName: testSecretName, Body: validBody,
		})
		require.NoError(t, err)
		assert.IsType(t, gen.UpdateSecret422JSONResponse{}, resp)
	})

	t.Run("forbidden returns 403", func(t *testing.T) {
		h, svc := newSecretHandler(t, true)
		svc.EXPECT().UpdateSecret(mock.Anything, testSecretsNs, testSecretName, mock.Anything).
			Return(nil, svcpkg.ErrForbidden)

		resp, err := h.UpdateSecret(ctx, gen.UpdateSecretRequestObject{
			NamespaceName: testSecretsNs, SecretName: testSecretName, Body: validBody,
		})
		require.NoError(t, err)
		assert.IsType(t, gen.UpdateSecret403JSONResponse{}, resp)
	})

	t.Run("validation 422 returns 422", func(t *testing.T) {
		h, svc := newSecretHandler(t, true)
		svc.EXPECT().UpdateSecret(mock.Anything, testSecretsNs, testSecretName, mock.Anything).
			Return(nil, &svcpkg.ValidationError{StatusCode: http.StatusUnprocessableEntity, Msg: "missing key"})

		resp, err := h.UpdateSecret(ctx, gen.UpdateSecretRequestObject{
			NamespaceName: testSecretsNs, SecretName: testSecretName, Body: validBody,
		})
		require.NoError(t, err)
		assert.IsType(t, gen.UpdateSecret422JSONResponse{}, resp)
	})

	t.Run("unknown error returns 500", func(t *testing.T) {
		h, svc := newSecretHandler(t, true)
		svc.EXPECT().UpdateSecret(mock.Anything, testSecretsNs, testSecretName, mock.Anything).
			Return(nil, errors.New("boom"))

		resp, err := h.UpdateSecret(ctx, gen.UpdateSecretRequestObject{
			NamespaceName: testSecretsNs, SecretName: testSecretName, Body: validBody,
		})
		require.NoError(t, err)
		assert.IsType(t, gen.UpdateSecret500JSONResponse{}, resp)
	})
}

// --- DeleteSecret ---

func TestDeleteSecretHandler(t *testing.T) {
	ctx := testContext()

	t.Run("success returns 204", func(t *testing.T) {
		h, svc := newSecretHandler(t, true)
		svc.EXPECT().DeleteSecret(mock.Anything, testSecretsNs, testSecretName).Return(nil)

		resp, err := h.DeleteSecret(ctx, gen.DeleteSecretRequestObject{NamespaceName: testSecretsNs, SecretName: testSecretName})
		require.NoError(t, err)
		assert.IsType(t, gen.DeleteSecret204Response{}, resp)
	})

	t.Run("not found returns 404", func(t *testing.T) {
		h, svc := newSecretHandler(t, true)
		svc.EXPECT().DeleteSecret(mock.Anything, testSecretsNs, testSecretName).Return(secretsvc.ErrSecretNotFound)

		resp, err := h.DeleteSecret(ctx, gen.DeleteSecretRequestObject{NamespaceName: testSecretsNs, SecretName: testSecretName})
		require.NoError(t, err)
		assert.IsType(t, gen.DeleteSecret404JSONResponse{}, resp)
	})

	t.Run("plane not found returns 422", func(t *testing.T) {
		h, svc := newSecretHandler(t, true)
		svc.EXPECT().DeleteSecret(mock.Anything, testSecretsNs, testSecretName).Return(secretsvc.ErrPlaneNotFound)

		resp, err := h.DeleteSecret(ctx, gen.DeleteSecretRequestObject{NamespaceName: testSecretsNs, SecretName: testSecretName})
		require.NoError(t, err)
		assert.IsType(t, gen.DeleteSecret422JSONResponse{}, resp)
	})

	t.Run("forbidden returns 403", func(t *testing.T) {
		h, svc := newSecretHandler(t, true)
		svc.EXPECT().DeleteSecret(mock.Anything, testSecretsNs, testSecretName).Return(svcpkg.ErrForbidden)

		resp, err := h.DeleteSecret(ctx, gen.DeleteSecretRequestObject{NamespaceName: testSecretsNs, SecretName: testSecretName})
		require.NoError(t, err)
		assert.IsType(t, gen.DeleteSecret403JSONResponse{}, resp)
	})

	t.Run("unknown error returns 500", func(t *testing.T) {
		h, svc := newSecretHandler(t, true)
		svc.EXPECT().DeleteSecret(mock.Anything, testSecretsNs, testSecretName).Return(errors.New("boom"))

		resp, err := h.DeleteSecret(ctx, gen.DeleteSecretRequestObject{NamespaceName: testSecretsNs, SecretName: testSecretName})
		require.NoError(t, err)
		assert.IsType(t, gen.DeleteSecret500JSONResponse{}, resp)
	})
}
