// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package gitsecret_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/mock"

	authzcore "github.com/openchoreo/openchoreo/internal/authz/core"
	authzmocks "github.com/openchoreo/openchoreo/internal/authz/core/mocks"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/gitsecret"
	gitsecretmocks "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/gitsecret/mocks"
)

const testNamespace = "ns1"

func newTestLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// --- PDP mock helpers ---

func newAllowAllPDP(t *testing.T) *authzmocks.MockPDP {
	t.Helper()
	pdp := authzmocks.NewMockPDP(t)
	pdp.EXPECT().Evaluate(mock.Anything, mock.Anything).
		Return(&authzcore.Decision{Decision: true, Context: &authzcore.DecisionContext{}}, nil).
		Maybe()
	return pdp
}

func newDenyAllPDP(t *testing.T) *authzmocks.MockPDP {
	t.Helper()
	pdp := authzmocks.NewMockPDP(t)
	pdp.EXPECT().Evaluate(mock.Anything, mock.Anything).
		Return(&authzcore.Decision{Decision: false, Context: &authzcore.DecisionContext{}}, nil).
		Maybe()
	return pdp
}

func newErrorPDP(t *testing.T, err error) *authzmocks.MockPDP {
	t.Helper()
	pdp := authzmocks.NewMockPDP(t)
	pdp.EXPECT().Evaluate(mock.Anything, mock.Anything).
		Return(nil, err).
		Maybe()
	return pdp
}

func newSelectivePDP(t *testing.T, allowedIDs map[string]bool) *authzmocks.MockPDP {
	t.Helper()
	pdp := authzmocks.NewMockPDP(t)
	pdp.EXPECT().Evaluate(mock.Anything, mock.Anything).
		RunAndReturn(func(_ context.Context, req *authzcore.EvaluateRequest) (*authzcore.Decision, error) {
			return &authzcore.Decision{
				Decision: allowedIDs[req.Resource.ID],
				Context:  &authzcore.DecisionContext{},
			}, nil
		}).
		Maybe()
	return pdp
}

// newAuthzService creates the authz-wrapped service using mockery-generated mocks.
func newAuthzService(t *testing.T, svcSetup func(*gitsecretmocks.MockService), pdp authzcore.PDP) gitsecret.Service {
	t.Helper()
	mockSvc := gitsecretmocks.NewMockService(t)
	svcSetup(mockSvc)
	return gitsecret.NewAuthzServiceForTest(mockSvc, pdp, newTestLogger())
}

// --- ListGitSecrets authz tests ---

func TestAuthzListGitSecrets_AllowAll(t *testing.T) {
	items := []gitsecret.GitSecretInfo{
		{Name: "secret-1", Namespace: testNamespace},
		{Name: "secret-2", Namespace: testNamespace},
	}
	svc := newAuthzService(t, func(m *gitsecretmocks.MockService) {
		m.EXPECT().ListGitSecrets(mock.Anything, testNamespace).Return(items, nil)
	}, newAllowAllPDP(t))

	result, err := svc.ListGitSecrets(context.Background(), testNamespace)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 2 {
		t.Errorf("expected 2 secrets, got %d", len(result))
	}
}

func TestAuthzListGitSecrets_DenyAll(t *testing.T) {
	items := []gitsecret.GitSecretInfo{
		{Name: "secret-1", Namespace: testNamespace},
		{Name: "secret-2", Namespace: testNamespace},
	}
	svc := newAuthzService(t, func(m *gitsecretmocks.MockService) {
		m.EXPECT().ListGitSecrets(mock.Anything, testNamespace).Return(items, nil)
	}, newDenyAllPDP(t))

	result, err := svc.ListGitSecrets(context.Background(), testNamespace)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("expected 0 secrets, got %d", len(result))
	}
}

func TestAuthzListGitSecrets_Selective(t *testing.T) {
	items := []gitsecret.GitSecretInfo{
		{Name: "allowed-1", Namespace: testNamespace},
		{Name: "denied-1", Namespace: testNamespace},
		{Name: "allowed-2", Namespace: testNamespace},
	}
	pdp := newSelectivePDP(t, map[string]bool{
		"allowed-1": true,
		"allowed-2": true,
	})
	svc := newAuthzService(t, func(m *gitsecretmocks.MockService) {
		m.EXPECT().ListGitSecrets(mock.Anything, testNamespace).Return(items, nil)
	}, pdp)

	result, err := svc.ListGitSecrets(context.Background(), testNamespace)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("expected 2 secrets, got %d", len(result))
	}
	if result[0].Name != "allowed-1" || result[1].Name != "allowed-2" {
		t.Errorf("unexpected result names: %v, %v", result[0].Name, result[1].Name)
	}
}

func TestAuthzListGitSecrets_PDPError(t *testing.T) {
	items := []gitsecret.GitSecretInfo{
		{Name: "secret-1", Namespace: testNamespace},
	}
	pdpErr := errors.New("pdp connection failed")
	svc := newAuthzService(t, func(m *gitsecretmocks.MockService) {
		m.EXPECT().ListGitSecrets(mock.Anything, testNamespace).Return(items, nil)
	}, newErrorPDP(t, pdpErr))

	_, err := svc.ListGitSecrets(context.Background(), testNamespace)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, pdpErr) {
		t.Errorf("expected wrapped pdp error, got %v", err)
	}
}

func TestAuthzListGitSecrets_InternalError(t *testing.T) {
	internalErr := errors.New("k8s list failed")
	svc := newAuthzService(t, func(m *gitsecretmocks.MockService) {
		m.EXPECT().ListGitSecrets(mock.Anything, testNamespace).Return(nil, internalErr)
	}, newAllowAllPDP(t))

	_, err := svc.ListGitSecrets(context.Background(), testNamespace)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, internalErr) {
		t.Errorf("expected internal error, got %v", err)
	}
}

func TestAuthzListGitSecrets_EmptyList(t *testing.T) {
	svc := newAuthzService(t, func(m *gitsecretmocks.MockService) {
		m.EXPECT().ListGitSecrets(mock.Anything, testNamespace).Return([]gitsecret.GitSecretInfo{}, nil)
	}, newAllowAllPDP(t))

	result, err := svc.ListGitSecrets(context.Background(), testNamespace)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("expected 0 secrets, got %d", len(result))
	}
}

// --- CreateGitSecret authz tests ---

func TestAuthzCreateGitSecret_Allowed(t *testing.T) {
	expected := &gitsecret.GitSecretInfo{Name: "new-secret", Namespace: testNamespace}
	svc := newAuthzService(t, func(m *gitsecretmocks.MockService) {
		m.EXPECT().CreateGitSecret(mock.Anything, testNamespace, mock.Anything).Return(expected, nil)
	}, newAllowAllPDP(t))

	result, err := svc.CreateGitSecret(context.Background(), testNamespace, &gitsecret.CreateGitSecretParams{
		SecretName: "new-secret",
		SecretType: "basic-auth",
		Token:      "token",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Name != expected.Name {
		t.Errorf("result Name = %q, want %q", result.Name, expected.Name)
	}
}

func TestAuthzCreateGitSecret_Denied(t *testing.T) {
	svc := newAuthzService(t, func(_ *gitsecretmocks.MockService) {
		// No expectations — inner service should not be called.
	}, newDenyAllPDP(t))

	_, err := svc.CreateGitSecret(context.Background(), testNamespace, &gitsecret.CreateGitSecretParams{
		SecretName: "new-secret",
		SecretType: "basic-auth",
		Token:      "token",
	})
	if !errors.Is(err, services.ErrForbidden) {
		t.Errorf("expected ErrForbidden, got %v", err)
	}
}

func TestAuthzCreateGitSecret_PDPError(t *testing.T) {
	pdpErr := errors.New("pdp unavailable")
	svc := newAuthzService(t, func(_ *gitsecretmocks.MockService) {
		// No expectations — inner service should not be called.
	}, newErrorPDP(t, pdpErr))

	_, err := svc.CreateGitSecret(context.Background(), testNamespace, &gitsecret.CreateGitSecretParams{
		SecretName: "new-secret",
		SecretType: "basic-auth",
		Token:      "token",
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, pdpErr) {
		t.Errorf("expected wrapped pdp error, got %v", err)
	}
}

func TestAuthzCreateGitSecret_InternalError(t *testing.T) {
	internalErr := errors.New("internal create failed")
	svc := newAuthzService(t, func(m *gitsecretmocks.MockService) {
		m.EXPECT().CreateGitSecret(mock.Anything, testNamespace, mock.Anything).Return(nil, internalErr)
	}, newAllowAllPDP(t))

	_, err := svc.CreateGitSecret(context.Background(), testNamespace, &gitsecret.CreateGitSecretParams{
		SecretName: "new-secret",
		SecretType: "basic-auth",
		Token:      "token",
	})
	if !errors.Is(err, internalErr) {
		t.Errorf("expected internal error, got %v", err)
	}
}

// --- DeleteGitSecret authz tests ---

func TestAuthzDeleteGitSecret_Allowed(t *testing.T) {
	svc := newAuthzService(t, func(m *gitsecretmocks.MockService) {
		m.EXPECT().DeleteGitSecret(mock.Anything, testNamespace, "my-secret").Return(nil)
	}, newAllowAllPDP(t))

	err := svc.DeleteGitSecret(context.Background(), testNamespace, "my-secret")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAuthzDeleteGitSecret_Denied(t *testing.T) {
	svc := newAuthzService(t, func(_ *gitsecretmocks.MockService) {
		// No expectations — inner service should not be called.
	}, newDenyAllPDP(t))

	err := svc.DeleteGitSecret(context.Background(), testNamespace, "my-secret")
	if !errors.Is(err, services.ErrForbidden) {
		t.Errorf("expected ErrForbidden, got %v", err)
	}
}

func TestAuthzDeleteGitSecret_PDPError(t *testing.T) {
	pdpErr := errors.New("pdp timeout")
	svc := newAuthzService(t, func(_ *gitsecretmocks.MockService) {
		// No expectations — inner service should not be called.
	}, newErrorPDP(t, pdpErr))

	err := svc.DeleteGitSecret(context.Background(), testNamespace, "my-secret")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, pdpErr) {
		t.Errorf("expected wrapped pdp error, got %v", err)
	}
}

func TestAuthzDeleteGitSecret_InternalError(t *testing.T) {
	internalErr := errors.New("internal delete failed")
	svc := newAuthzService(t, func(m *gitsecretmocks.MockService) {
		m.EXPECT().DeleteGitSecret(mock.Anything, testNamespace, "my-secret").Return(internalErr)
	}, newAllowAllPDP(t))

	err := svc.DeleteGitSecret(context.Background(), testNamespace, "my-secret")
	if !errors.Is(err, internalErr) {
		t.Errorf("expected internal error, got %v", err)
	}
}
