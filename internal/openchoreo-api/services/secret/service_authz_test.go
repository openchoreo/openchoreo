// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package secret_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/mock"
	corev1 "k8s.io/api/core/v1"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	authzcore "github.com/openchoreo/openchoreo/internal/authz/core"
	authzmocks "github.com/openchoreo/openchoreo/internal/authz/core/mocks"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/secret"
	secretmocks "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/secret/mocks"
)

const (
	testNamespace = "ns1"
	testSecret    = "my-secret"
)

func newTestLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func newAllowAllPDP(t *testing.T) *authzmocks.MockPDP {
	t.Helper()
	pdp := authzmocks.NewMockPDP(t)
	pdp.EXPECT().Evaluate(mock.Anything, mock.Anything).
		Return(&authzcore.Decision{Decision: true, Context: &authzcore.DecisionContext{}}, nil).
		Once()
	return pdp
}

func newDenyAllPDP(t *testing.T) *authzmocks.MockPDP {
	t.Helper()
	pdp := authzmocks.NewMockPDP(t)
	pdp.EXPECT().Evaluate(mock.Anything, mock.Anything).
		Return(&authzcore.Decision{Decision: false, Context: &authzcore.DecisionContext{}}, nil).
		Once()
	return pdp
}

func newErrorPDP(t *testing.T, err error) *authzmocks.MockPDP {
	t.Helper()
	pdp := authzmocks.NewMockPDP(t)
	pdp.EXPECT().Evaluate(mock.Anything, mock.Anything).
		Return(nil, err).
		Once()
	return pdp
}

func newAuthzService(t *testing.T, svcSetup func(*secretmocks.MockService), pdp authzcore.PDP) secret.Service {
	t.Helper()
	mockSvc := secretmocks.NewMockService(t)
	svcSetup(mockSvc)
	return secret.NewAuthzServiceForTest(mockSvc, pdp, newTestLogger())
}

func sampleCreateParams() *secret.CreateSecretParams {
	return &secret.CreateSecretParams{
		SecretName: testSecret,
		SecretType: corev1.SecretTypeOpaque,
		TargetPlane: openchoreov1alpha1.TargetPlaneRef{
			Kind: "DataPlane",
			Name: "default",
		},
		Data: map[string]string{"key": "value"},
	}
}

func sampleUpdateParams() *secret.UpdateSecretParams {
	return &secret.UpdateSecretParams{Data: map[string]string{"key": "value"}}
}

// --- CreateSecret ---

func TestAuthzCreateSecret_Allowed(t *testing.T) {
	expected := &secret.SecretInfo{Name: testSecret, Namespace: testNamespace}
	svc := newAuthzService(t, func(m *secretmocks.MockService) {
		m.EXPECT().CreateSecret(mock.Anything, testNamespace, mock.Anything).Return(expected, nil)
	}, newAllowAllPDP(t))

	result, err := svc.CreateSecret(context.Background(), testNamespace, sampleCreateParams())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Name != expected.Name {
		t.Errorf("result Name = %q, want %q", result.Name, expected.Name)
	}
}

func TestAuthzCreateSecret_Denied(t *testing.T) {
	svc := newAuthzService(t, func(_ *secretmocks.MockService) {
		// Inner service should not be called.
	}, newDenyAllPDP(t))

	_, err := svc.CreateSecret(context.Background(), testNamespace, sampleCreateParams())
	if !errors.Is(err, services.ErrForbidden) {
		t.Errorf("expected ErrForbidden, got %v", err)
	}
}

func TestAuthzCreateSecret_PDPError(t *testing.T) {
	pdpErr := errors.New("pdp unavailable")
	svc := newAuthzService(t, func(_ *secretmocks.MockService) {}, newErrorPDP(t, pdpErr))

	_, err := svc.CreateSecret(context.Background(), testNamespace, sampleCreateParams())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, pdpErr) {
		t.Errorf("expected wrapped pdp error, got %v", err)
	}
}

func TestAuthzCreateSecret_InternalError(t *testing.T) {
	internalErr := errors.New("internal create failed")
	svc := newAuthzService(t, func(m *secretmocks.MockService) {
		m.EXPECT().CreateSecret(mock.Anything, testNamespace, mock.Anything).Return(nil, internalErr)
	}, newAllowAllPDP(t))

	_, err := svc.CreateSecret(context.Background(), testNamespace, sampleCreateParams())
	if !errors.Is(err, internalErr) {
		t.Errorf("expected internal error, got %v", err)
	}
}

// --- UpdateSecret ---

func TestAuthzUpdateSecret_Allowed(t *testing.T) {
	expected := &secret.SecretInfo{Name: testSecret, Namespace: testNamespace}
	svc := newAuthzService(t, func(m *secretmocks.MockService) {
		m.EXPECT().UpdateSecret(mock.Anything, testNamespace, testSecret, mock.Anything).Return(expected, nil)
	}, newAllowAllPDP(t))

	result, err := svc.UpdateSecret(context.Background(), testNamespace, testSecret, sampleUpdateParams())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Name != expected.Name {
		t.Errorf("result Name = %q, want %q", result.Name, expected.Name)
	}
}

func TestAuthzUpdateSecret_Denied(t *testing.T) {
	svc := newAuthzService(t, func(_ *secretmocks.MockService) {}, newDenyAllPDP(t))

	_, err := svc.UpdateSecret(context.Background(), testNamespace, testSecret, sampleUpdateParams())
	if !errors.Is(err, services.ErrForbidden) {
		t.Errorf("expected ErrForbidden, got %v", err)
	}
}

func TestAuthzUpdateSecret_PDPError(t *testing.T) {
	pdpErr := errors.New("pdp timeout")
	svc := newAuthzService(t, func(_ *secretmocks.MockService) {}, newErrorPDP(t, pdpErr))

	_, err := svc.UpdateSecret(context.Background(), testNamespace, testSecret, sampleUpdateParams())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, pdpErr) {
		t.Errorf("expected wrapped pdp error, got %v", err)
	}
}

func TestAuthzUpdateSecret_InternalError(t *testing.T) {
	internalErr := errors.New("internal update failed")
	svc := newAuthzService(t, func(m *secretmocks.MockService) {
		m.EXPECT().UpdateSecret(mock.Anything, testNamespace, testSecret, mock.Anything).Return(nil, internalErr)
	}, newAllowAllPDP(t))

	_, err := svc.UpdateSecret(context.Background(), testNamespace, testSecret, sampleUpdateParams())
	if !errors.Is(err, internalErr) {
		t.Errorf("expected internal error, got %v", err)
	}
}

// --- DeleteSecret ---

func TestAuthzDeleteSecret_Allowed(t *testing.T) {
	svc := newAuthzService(t, func(m *secretmocks.MockService) {
		m.EXPECT().DeleteSecret(mock.Anything, testNamespace, testSecret).Return(nil)
	}, newAllowAllPDP(t))

	if err := svc.DeleteSecret(context.Background(), testNamespace, testSecret); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAuthzDeleteSecret_Denied(t *testing.T) {
	svc := newAuthzService(t, func(_ *secretmocks.MockService) {}, newDenyAllPDP(t))

	err := svc.DeleteSecret(context.Background(), testNamespace, testSecret)
	if !errors.Is(err, services.ErrForbidden) {
		t.Errorf("expected ErrForbidden, got %v", err)
	}
}

func TestAuthzDeleteSecret_PDPError(t *testing.T) {
	pdpErr := errors.New("pdp connection failed")
	svc := newAuthzService(t, func(_ *secretmocks.MockService) {}, newErrorPDP(t, pdpErr))

	err := svc.DeleteSecret(context.Background(), testNamespace, testSecret)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, pdpErr) {
		t.Errorf("expected wrapped pdp error, got %v", err)
	}
}

func TestAuthzDeleteSecret_InternalError(t *testing.T) {
	internalErr := errors.New("internal delete failed")
	svc := newAuthzService(t, func(m *secretmocks.MockService) {
		m.EXPECT().DeleteSecret(mock.Anything, testNamespace, testSecret).Return(internalErr)
	}, newAllowAllPDP(t))

	err := svc.DeleteSecret(context.Background(), testNamespace, testSecret)
	if !errors.Is(err, internalErr) {
		t.Errorf("expected internal error, got %v", err)
	}
}
