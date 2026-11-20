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
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

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

// --- CreateSecret ---

func TestAuthzCreateSecret_Allowed(t *testing.T) {
	expected := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: testSecret, Namespace: testNamespace}}
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

// --- UpdateSecret ---

func TestAuthzUpdateSecret_Allowed(t *testing.T) {
	expected := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: testSecret, Namespace: testNamespace}}
	svc := newAuthzService(t, func(m *secretmocks.MockService) {
		m.EXPECT().UpdateSecret(mock.Anything, testNamespace, testSecret, mock.Anything).Return(expected, nil)
	}, newAllowAllPDP(t))

	got, err := svc.UpdateSecret(context.Background(), testNamespace, testSecret, &secret.UpdateSecretParams{Data: map[string]string{"k": "v"}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Name != testSecret {
		t.Errorf("got Name = %q", got.Name)
	}
}

func TestAuthzUpdateSecret_Denied(t *testing.T) {
	svc := newAuthzService(t, func(_ *secretmocks.MockService) {}, newDenyAllPDP(t))

	_, err := svc.UpdateSecret(context.Background(), testNamespace, testSecret, &secret.UpdateSecretParams{Data: map[string]string{"k": "v"}})
	if !errors.Is(err, services.ErrForbidden) {
		t.Errorf("expected ErrForbidden, got %v", err)
	}
}

// --- GetSecret ---

func TestAuthzGetSecret_Allowed(t *testing.T) {
	expected := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: testSecret, Namespace: testNamespace}}
	svc := newAuthzService(t, func(m *secretmocks.MockService) {
		m.EXPECT().GetSecret(mock.Anything, testNamespace, testSecret).Return(expected, nil)
	}, newAllowAllPDP(t))

	got, err := svc.GetSecret(context.Background(), testNamespace, testSecret)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Name != testSecret {
		t.Errorf("got Name = %q", got.Name)
	}
}

func TestAuthzGetSecret_Denied(t *testing.T) {
	svc := newAuthzService(t, func(_ *secretmocks.MockService) {}, newDenyAllPDP(t))

	_, err := svc.GetSecret(context.Background(), testNamespace, testSecret)
	if !errors.Is(err, services.ErrForbidden) {
		t.Errorf("expected ErrForbidden, got %v", err)
	}
}

// --- ListSecrets ---

// listAllowPDP returns a PDP that allows every per-item check (FilteredList
// calls authz once per item it sees, so we don't bound the call count).
func listAllowPDP(t *testing.T) *authzmocks.MockPDP {
	t.Helper()
	pdp := authzmocks.NewMockPDP(t)
	pdp.EXPECT().Evaluate(mock.Anything, mock.Anything).
		Return(&authzcore.Decision{Decision: true, Context: &authzcore.DecisionContext{}}, nil)
	return pdp
}

// listDenyPDP returns a PDP that denies every per-item check.
func listDenyPDP(t *testing.T) *authzmocks.MockPDP {
	t.Helper()
	pdp := authzmocks.NewMockPDP(t)
	pdp.EXPECT().Evaluate(mock.Anything, mock.Anything).
		Return(&authzcore.Decision{Decision: false, Context: &authzcore.DecisionContext{}}, nil)
	return pdp
}

// --- resource.name attribute tests ---

// newCapturingPDP returns a PDP that allows the request and captures the
// EvaluateRequest so the caller can inspect the Context attributes.
func newCapturingPDP(t *testing.T) (*authzmocks.MockPDP, *authzcore.EvaluateRequest) {
	t.Helper()
	var captured authzcore.EvaluateRequest
	pdp := authzmocks.NewMockPDP(t)
	pdp.EXPECT().Evaluate(mock.Anything, mock.Anything).
		RunAndReturn(func(_ context.Context, req *authzcore.EvaluateRequest) (*authzcore.Decision, error) {
			captured = *req
			return &authzcore.Decision{Decision: true, Context: &authzcore.DecisionContext{}}, nil
		}).Once()
	return pdp, &captured
}

func TestAuthzCreateSecret_PassesNameAttribute(t *testing.T) {
	expected := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: testSecret, Namespace: testNamespace}}
	pdp, captured := newCapturingPDP(t)
	svc := newAuthzService(t, func(m *secretmocks.MockService) {
		m.EXPECT().CreateSecret(mock.Anything, testNamespace, mock.Anything).Return(expected, nil)
	}, pdp)

	_, err := svc.CreateSecret(context.Background(), testNamespace, sampleCreateParams())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if captured.Context.Resource.Name != testSecret {
		t.Errorf("Context.Resource.Name = %q, want %q", captured.Context.Resource.Name, testSecret)
	}
}

func TestAuthzGetSecret_PassesNameAttribute(t *testing.T) {
	expected := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: testSecret, Namespace: testNamespace}}
	pdp, captured := newCapturingPDP(t)
	svc := newAuthzService(t, func(m *secretmocks.MockService) {
		m.EXPECT().GetSecret(mock.Anything, testNamespace, testSecret).Return(expected, nil)
	}, pdp)

	_, err := svc.GetSecret(context.Background(), testNamespace, testSecret)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if captured.Context.Resource.Name != testSecret {
		t.Errorf("Context.Resource.Name = %q, want %q", captured.Context.Resource.Name, testSecret)
	}
}

func TestAuthzUpdateSecret_PassesNameAttribute(t *testing.T) {
	expected := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: testSecret, Namespace: testNamespace}}
	pdp, captured := newCapturingPDP(t)
	svc := newAuthzService(t, func(m *secretmocks.MockService) {
		m.EXPECT().UpdateSecret(mock.Anything, testNamespace, testSecret, mock.Anything).Return(expected, nil)
	}, pdp)

	_, err := svc.UpdateSecret(context.Background(), testNamespace, testSecret, &secret.UpdateSecretParams{Data: map[string]string{"k": "v"}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if captured.Context.Resource.Name != testSecret {
		t.Errorf("Context.Resource.Name = %q, want %q", captured.Context.Resource.Name, testSecret)
	}
}

func TestAuthzDeleteSecret_PassesNameAttribute(t *testing.T) {
	pdp, captured := newCapturingPDP(t)
	svc := newAuthzService(t, func(m *secretmocks.MockService) {
		m.EXPECT().DeleteSecret(mock.Anything, testNamespace, testSecret).Return(nil)
	}, pdp)

	if err := svc.DeleteSecret(context.Background(), testNamespace, testSecret); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if captured.Context.Resource.Name != testSecret {
		t.Errorf("Context.Resource.Name = %q, want %q", captured.Context.Resource.Name, testSecret)
	}
}

func TestAuthzListSecrets_PassesNameAttribute(t *testing.T) {
	inner := &services.ListResult[corev1.Secret]{
		Items: []corev1.Secret{
			{ObjectMeta: metav1.ObjectMeta{Name: "sec-alpha", Namespace: testNamespace}},
			{ObjectMeta: metav1.ObjectMeta{Name: "sec-beta", Namespace: testNamespace}},
		},
	}
	var capturedNames []string
	pdp := authzmocks.NewMockPDP(t)
	pdp.EXPECT().Evaluate(mock.Anything, mock.Anything).
		RunAndReturn(func(_ context.Context, req *authzcore.EvaluateRequest) (*authzcore.Decision, error) {
			capturedNames = append(capturedNames, req.Context.Resource.Name)
			return &authzcore.Decision{Decision: true, Context: &authzcore.DecisionContext{}}, nil
		}).Times(2)

	svc := newAuthzService(t, func(m *secretmocks.MockService) {
		m.EXPECT().ListSecrets(mock.Anything, testNamespace, mock.Anything).Return(inner, nil)
	}, pdp)

	got, err := svc.ListSecrets(context.Background(), testNamespace, services.ListOptions{Limit: 10})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got.Items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(got.Items))
	}
	require.Equal(t, []string{"sec-alpha", "sec-beta"}, capturedNames)
}

func TestAuthzSecret_NameConditionEvaluation(t *testing.T) {
	const allowedSecretName = "matching-secret"
	const deniedSecretName = "non-matching-secret"

	// conditionalPDP simulates a PDP evaluating a CEL condition: `resource.name == "matching-secret"`
	conditionalPDP := func(t *testing.T) *authzmocks.MockPDP {
		pdp := authzmocks.NewMockPDP(t)
		pdp.EXPECT().Evaluate(mock.Anything, mock.Anything).
			RunAndReturn(func(_ context.Context, req *authzcore.EvaluateRequest) (*authzcore.Decision, error) {
				allowed := req.Context.Resource.Name == allowedSecretName
				return &authzcore.Decision{Decision: allowed, Context: &authzcore.DecisionContext{}}, nil
			})
		return pdp
	}

	t.Run("matching name allowed", func(t *testing.T) {
		expected := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: allowedSecretName, Namespace: testNamespace}}
		svc := newAuthzService(t, func(m *secretmocks.MockService) {
			m.EXPECT().GetSecret(mock.Anything, testNamespace, allowedSecretName).Return(expected, nil)
			m.EXPECT().DeleteSecret(mock.Anything, testNamespace, allowedSecretName).Return(nil)
			m.EXPECT().UpdateSecret(mock.Anything, testNamespace, allowedSecretName, mock.Anything).Return(expected, nil)
		}, conditionalPDP(t))

		// Get
		got, err := svc.GetSecret(context.Background(), testNamespace, allowedSecretName)
		require.NoError(t, err)
		require.Equal(t, allowedSecretName, got.Name)

		// Update
		got, err = svc.UpdateSecret(context.Background(), testNamespace, allowedSecretName, &secret.UpdateSecretParams{Data: map[string]string{"k": "v"}})
		require.NoError(t, err)
		require.Equal(t, allowedSecretName, got.Name)

		// Delete
		err = svc.DeleteSecret(context.Background(), testNamespace, allowedSecretName)
		require.NoError(t, err)
	})

	t.Run("non-matching name denied", func(t *testing.T) {
		svc := newAuthzService(t, func(_ *secretmocks.MockService) {}, conditionalPDP(t))

		// Get
		_, err := svc.GetSecret(context.Background(), testNamespace, deniedSecretName)
		require.ErrorIs(t, err, services.ErrForbidden)

		// Update
		_, err = svc.UpdateSecret(context.Background(), testNamespace, deniedSecretName, &secret.UpdateSecretParams{Data: map[string]string{"k": "v"}})
		require.ErrorIs(t, err, services.ErrForbidden)

		// Delete
		err = svc.DeleteSecret(context.Background(), testNamespace, deniedSecretName)
		require.ErrorIs(t, err, services.ErrForbidden)
	})

	t.Run("list filtering with name condition", func(t *testing.T) {
		inner := &services.ListResult[corev1.Secret]{
			Items: []corev1.Secret{
				{ObjectMeta: metav1.ObjectMeta{Name: allowedSecretName, Namespace: testNamespace}},
				{ObjectMeta: metav1.ObjectMeta{Name: deniedSecretName, Namespace: testNamespace}},
			},
		}
		svc := newAuthzService(t, func(m *secretmocks.MockService) {
			m.EXPECT().ListSecrets(mock.Anything, testNamespace, mock.Anything).Return(inner, nil)
		}, conditionalPDP(t))

		got, err := svc.ListSecrets(context.Background(), testNamespace, services.ListOptions{Limit: 10})
		require.NoError(t, err)
		require.Len(t, got.Items, 1)
		require.Equal(t, allowedSecretName, got.Items[0].Name)
	})

	t.Run("empty or unset condition behavior allows all", func(t *testing.T) {
		// When no condition is set on the role, the PDP allows all secrets unconditionally
		inner := &services.ListResult[corev1.Secret]{
			Items: []corev1.Secret{
				{ObjectMeta: metav1.ObjectMeta{Name: allowedSecretName, Namespace: testNamespace}},
				{ObjectMeta: metav1.ObjectMeta{Name: deniedSecretName, Namespace: testNamespace}},
			},
		}
		svc := newAuthzService(t, func(m *secretmocks.MockService) {
			m.EXPECT().ListSecrets(mock.Anything, testNamespace, mock.Anything).Return(inner, nil)
		}, listAllowPDP(t))

		got, err := svc.ListSecrets(context.Background(), testNamespace, services.ListOptions{Limit: 10})
		require.NoError(t, err)
		require.Len(t, got.Items, 2)
	})
}

func TestAuthzListSecrets_Allowed(t *testing.T) {
	expected := &services.ListResult[corev1.Secret]{
		Items: []corev1.Secret{{ObjectMeta: metav1.ObjectMeta{Name: testSecret, Namespace: testNamespace}}},
	}
	svc := newAuthzService(t, func(m *secretmocks.MockService) {
		m.EXPECT().ListSecrets(mock.Anything, testNamespace, mock.Anything).Return(expected, nil)
	}, listAllowPDP(t))

	got, err := svc.ListSecrets(context.Background(), testNamespace, services.ListOptions{Limit: 10})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got.Items) != 1 {
		t.Errorf("expected 1 item, got %d", len(got.Items))
	}
}

// When the per-item PDP denies every item, FilteredList drops them silently
// and returns an empty page rather than ErrForbidden.
func TestAuthzListSecrets_Denied(t *testing.T) {
	inner := &services.ListResult[corev1.Secret]{
		Items: []corev1.Secret{{ObjectMeta: metav1.ObjectMeta{Name: testSecret, Namespace: testNamespace}}},
	}
	svc := newAuthzService(t, func(m *secretmocks.MockService) {
		m.EXPECT().ListSecrets(mock.Anything, testNamespace, mock.Anything).Return(inner, nil)
	}, listDenyPDP(t))

	got, err := svc.ListSecrets(context.Background(), testNamespace, services.ListOptions{Limit: 10})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got.Items) != 0 {
		t.Errorf("expected denied items to be dropped, got %d", len(got.Items))
	}
}
