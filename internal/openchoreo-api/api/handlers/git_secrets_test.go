// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
	svcpkg "github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	gitsecretsvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/gitsecret"
	gitsecretmocks "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/gitsecret/mocks"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/handlerservices"
)

func TestListGitSecretsHandler_OnlySetsWorkflowPlanePointersWhenPresent(t *testing.T) {
	ctx := testContext()
	svc := gitsecretmocks.NewMockService(t)
	svc.EXPECT().ListGitSecrets(mock.Anything, "ns1").Return([]gitsecretsvc.GitSecretInfo{
		{Name: "s1", Namespace: "ns1", WorkflowPlaneKind: "WorkflowPlane", WorkflowPlaneName: "wp1"},
		{Name: "s2", Namespace: "ns1"},
	}, nil)
	h := &Handler{
		services: &handlerservices.Services{GitSecretService: svc},
		logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	resp, err := h.ListGitSecrets(ctx, gen.ListGitSecretsRequestObject{NamespaceName: "ns1"})
	require.NoError(t, err)
	typed, ok := resp.(gen.ListGitSecrets200JSONResponse)
	require.True(t, ok, "expected 200 response, got %T", resp)
	require.Len(t, typed.Items, 2)

	require.NotNil(t, typed.Items[0].WorkflowPlaneKind)
	require.NotNil(t, typed.Items[0].WorkflowPlaneName)
	assert.Equal(t, "WorkflowPlane", *typed.Items[0].WorkflowPlaneKind)
	assert.Equal(t, "wp1", *typed.Items[0].WorkflowPlaneName)
	assert.Nil(t, typed.Items[1].WorkflowPlaneKind)
	assert.Nil(t, typed.Items[1].WorkflowPlaneName)
}

func TestCreateGitSecretHandler_ForwardsOptionalFields(t *testing.T) {
	ctx := testContext()
	var captured *gitsecretsvc.CreateGitSecretParams
	svc := gitsecretmocks.NewMockService(t)
	svc.EXPECT().CreateGitSecret(mock.Anything, "test-ns", mock.Anything).
		RunAndReturn(func(_ context.Context, namespace string, req *gitsecretsvc.CreateGitSecretParams) (*gitsecretsvc.GitSecretInfo, error) {
			captured = req
			return &gitsecretsvc.GitSecretInfo{
				Name:              req.SecretName,
				Namespace:         namespace,
				WorkflowPlaneKind: req.WorkflowPlaneKind,
				WorkflowPlaneName: req.WorkflowPlaneName,
			}, nil
		})
	h := &Handler{
		services: &handlerservices.Services{GitSecretService: svc},
		logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	username := "u"
	token := "t"
	sshKey := "k"
	sshKeyID := "kid"
	body := gen.CreateGitSecretRequest{
		SecretName:        "s1",
		SecretType:        gen.BasicAuth,
		WorkflowPlaneKind: gen.CreateGitSecretRequestWorkflowPlaneKindWorkflowPlane,
		WorkflowPlaneName: "wp1",
		Username:          &username,
		Token:             &token,
		SshKey:            &sshKey,
		SshKeyId:          &sshKeyID,
	}

	resp, err := h.CreateGitSecret(ctx, gen.CreateGitSecretRequestObject{
		NamespaceName: "test-ns",
		Body:          &body,
	})
	require.NoError(t, err)
	typed, ok := resp.(gen.CreateGitSecret201JSONResponse)
	require.True(t, ok, "expected 201 response, got %T", resp)
	require.NotNil(t, typed.Name)
	assert.Equal(t, "s1", *typed.Name)
	require.NotNil(t, typed.WorkflowPlaneKind)
	assert.Equal(t, "WorkflowPlane", *typed.WorkflowPlaneKind)
	require.NotNil(t, typed.WorkflowPlaneName)
	assert.Equal(t, "wp1", *typed.WorkflowPlaneName)

	require.NotNil(t, captured)
	assert.Equal(t, "s1", captured.SecretName)
	assert.Equal(t, "basic-auth", captured.SecretType)
	assert.Equal(t, "WorkflowPlane", captured.WorkflowPlaneKind)
	assert.Equal(t, "wp1", captured.WorkflowPlaneName)
	assert.Equal(t, username, captured.Username)
	assert.Equal(t, token, captured.Token)
	assert.Equal(t, sshKey, captured.SSHKey)
	assert.Equal(t, sshKeyID, captured.SSHKeyID)
}

func TestMapCreateGitSecretError(t *testing.T) {
	h := &Handler{logger: slog.New(slog.NewTextHandler(io.Discard, nil))}

	resp, err := mapCreateGitSecretError(h, svcpkg.ErrForbidden)
	require.NoError(t, err)
	assert.IsType(t, gen.CreateGitSecret403JSONResponse{}, resp)

	resp, err = mapCreateGitSecretError(h, gitsecretsvc.ErrGitSecretAlreadyExists)
	require.NoError(t, err)
	assert.IsType(t, gen.CreateGitSecret409JSONResponse{}, resp)

	resp, err = mapCreateGitSecretError(h, gitsecretsvc.ErrInvalidSecretType)
	require.NoError(t, err)
	assert.IsType(t, gen.CreateGitSecret400JSONResponse{}, resp)

	resp, err = mapCreateGitSecretError(h, gitsecretsvc.ErrWorkflowPlaneNotFound)
	require.NoError(t, err)
	assert.IsType(t, gen.CreateGitSecret400JSONResponse{}, resp)

	resp, err = mapCreateGitSecretError(h, gitsecretsvc.ErrSecretStoreNotConfigured)
	require.NoError(t, err)
	assert.IsType(t, gen.CreateGitSecret400JSONResponse{}, resp)

	resp, err = mapCreateGitSecretError(h, &svcpkg.ValidationError{Msg: "token is required"})
	require.NoError(t, err)
	assert.IsType(t, gen.CreateGitSecret400JSONResponse{}, resp)
}

func TestMapDeleteGitSecretError(t *testing.T) {
	h := &Handler{logger: slog.New(slog.NewTextHandler(io.Discard, nil))}

	resp, err := mapDeleteGitSecretError(h, svcpkg.ErrForbidden)
	require.NoError(t, err)
	assert.IsType(t, gen.DeleteGitSecret403JSONResponse{}, resp)

	resp, err = mapDeleteGitSecretError(h, gitsecretsvc.ErrGitSecretNotFound)
	require.NoError(t, err)
	assert.IsType(t, gen.DeleteGitSecret404JSONResponse{}, resp)

	resp, err = mapDeleteGitSecretError(h, gitsecretsvc.ErrWorkflowPlaneNotFound)
	require.NoError(t, err)
	assert.IsType(t, gen.DeleteGitSecret500JSONResponse{}, resp)
}

func TestListGitSecretsHandler_ServiceErrorReturns500(t *testing.T) {
	ctx := testContext()
	svc := gitsecretmocks.NewMockService(t)
	svc.EXPECT().ListGitSecrets(mock.Anything, "ns1").Return(nil, errors.New("storage unavailable"))
	h := &Handler{
		services: &handlerservices.Services{GitSecretService: svc},
		logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	resp, err := h.ListGitSecrets(ctx, gen.ListGitSecretsRequestObject{NamespaceName: "ns1"})
	require.NoError(t, err)
	assert.IsType(t, gen.ListGitSecrets500JSONResponse{}, resp)
}

func TestCreateGitSecretHandler_NilBodyReturns400(t *testing.T) {
	ctx := testContext()
	svc := gitsecretmocks.NewMockService(t)
	h := &Handler{
		services: &handlerservices.Services{GitSecretService: svc},
		logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	resp, err := h.CreateGitSecret(ctx, gen.CreateGitSecretRequestObject{NamespaceName: "ns1", Body: nil})
	require.NoError(t, err)
	assert.IsType(t, gen.CreateGitSecret400JSONResponse{}, resp)
}

func TestDeleteGitSecretHandler_SuccessReturns204(t *testing.T) {
	ctx := testContext()
	svc := gitsecretmocks.NewMockService(t)
	svc.EXPECT().DeleteGitSecret(mock.Anything, "ns1", "s1").Return(nil)
	h := &Handler{
		services: &handlerservices.Services{GitSecretService: svc},
		logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	resp, err := h.DeleteGitSecret(ctx, gen.DeleteGitSecretRequestObject{NamespaceName: "ns1", GitSecretName: "s1"})
	require.NoError(t, err)
	assert.IsType(t, gen.DeleteGitSecret204Response{}, resp)
}
