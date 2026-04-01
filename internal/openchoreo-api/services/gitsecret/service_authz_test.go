// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package gitsecret

import (
	"context"
	"errors"
	"testing"

	authzcore "github.com/openchoreo/openchoreo/internal/authz/core"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
)

// --- Mock PDP implementations ---

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

type errorPDP struct {
	err error
}

func (e *errorPDP) Evaluate(_ context.Context, _ *authzcore.EvaluateRequest) (*authzcore.Decision, error) {
	return nil, e.err
}

func (e *errorPDP) BatchEvaluate(_ context.Context, _ *authzcore.BatchEvaluateRequest) (*authzcore.BatchEvaluateResponse, error) {
	return nil, e.err
}

func (e *errorPDP) GetSubjectProfile(_ context.Context, _ *authzcore.ProfileRequest) (*authzcore.UserCapabilitiesResponse, error) {
	return nil, e.err
}

// --- Mock Service implementation ---

type mockGitSecretService struct {
	listResult   []GitSecretInfo
	listErr      error
	createResult *GitSecretInfo
	createErr    error
	deleteErr    error

	listCalls   int
	createCalls int
	deleteCalls int
}

func (m *mockGitSecretService) ListGitSecrets(_ context.Context, _ string) ([]GitSecretInfo, error) {
	m.listCalls++
	return m.listResult, m.listErr
}

func (m *mockGitSecretService) CreateGitSecret(_ context.Context, _ string, _ *CreateGitSecretParams) (*GitSecretInfo, error) {
	m.createCalls++
	return m.createResult, m.createErr
}

func (m *mockGitSecretService) DeleteGitSecret(_ context.Context, _, _ string) error {
	m.deleteCalls++
	return m.deleteErr
}

// newAuthzService creates a gitSecretServiceWithAuthz with the given mock service and PDP.
func newAuthzService(internal Service, pdp authzcore.PDP) *gitSecretServiceWithAuthz {
	return &gitSecretServiceWithAuthz{
		internal: internal,
		authz:    services.NewAuthzChecker(pdp, newTestLogger()),
	}
}

// --- ListGitSecrets authz tests ---

func TestAuthzListGitSecrets_AllowAll(t *testing.T) {
	items := []GitSecretInfo{
		{Name: "secret-1", Namespace: "ns1"},
		{Name: "secret-2", Namespace: "ns1"},
	}
	svc := newAuthzService(&mockGitSecretService{listResult: items}, &allowAllPDP{})

	result, err := svc.ListGitSecrets(context.Background(), "ns1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 2 {
		t.Errorf("expected 2 secrets, got %d", len(result))
	}
}

func TestAuthzListGitSecrets_DenyAll(t *testing.T) {
	items := []GitSecretInfo{
		{Name: "secret-1", Namespace: "ns1"},
		{Name: "secret-2", Namespace: "ns1"},
	}
	mock := &mockGitSecretService{listResult: items}
	svc := newAuthzService(mock, &denyAllPDP{})

	result, err := svc.ListGitSecrets(context.Background(), "ns1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("expected 0 secrets, got %d", len(result))
	}
	if mock.listCalls != 1 {
		t.Errorf("expected listCalls=1 (to fetch items for filtering), got %d", mock.listCalls)
	}
}

func TestAuthzListGitSecrets_Selective(t *testing.T) {
	items := []GitSecretInfo{
		{Name: "allowed-1", Namespace: "ns1"},
		{Name: "denied-1", Namespace: "ns1"},
		{Name: "allowed-2", Namespace: "ns1"},
	}
	pdp := &selectivePDP{allowedIDs: map[string]bool{
		"allowed-1": true,
		"allowed-2": true,
	}}
	svc := newAuthzService(&mockGitSecretService{listResult: items}, pdp)

	result, err := svc.ListGitSecrets(context.Background(), "ns1")
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
	items := []GitSecretInfo{
		{Name: "secret-1", Namespace: "ns1"},
	}
	pdpErr := errors.New("pdp connection failed")
	mock := &mockGitSecretService{listResult: items}
	svc := newAuthzService(mock, &errorPDP{err: pdpErr})

	_, err := svc.ListGitSecrets(context.Background(), "ns1")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, pdpErr) {
		t.Errorf("expected wrapped pdp error, got %v", err)
	}
}

func TestAuthzListGitSecrets_InternalError(t *testing.T) {
	internalErr := errors.New("k8s list failed")
	svc := newAuthzService(&mockGitSecretService{listErr: internalErr}, &allowAllPDP{})

	_, err := svc.ListGitSecrets(context.Background(), "ns1")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, internalErr) {
		t.Errorf("expected internal error, got %v", err)
	}
}

func TestAuthzListGitSecrets_EmptyList(t *testing.T) {
	svc := newAuthzService(&mockGitSecretService{listResult: []GitSecretInfo{}}, &allowAllPDP{})

	result, err := svc.ListGitSecrets(context.Background(), "ns1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("expected 0 secrets, got %d", len(result))
	}
}

// --- CreateGitSecret authz tests ---

func TestAuthzCreateGitSecret_Allowed(t *testing.T) {
	expected := &GitSecretInfo{Name: "new-secret", Namespace: "ns1"}
	svc := newAuthzService(&mockGitSecretService{createResult: expected}, &allowAllPDP{})

	result, err := svc.CreateGitSecret(context.Background(), "ns1", &CreateGitSecretParams{
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
	mock := &mockGitSecretService{}
	svc := newAuthzService(mock, &denyAllPDP{})

	_, err := svc.CreateGitSecret(context.Background(), "ns1", &CreateGitSecretParams{
		SecretName: "new-secret",
		SecretType: "basic-auth",
		Token:      "token",
	})
	if !errors.Is(err, services.ErrForbidden) {
		t.Errorf("expected ErrForbidden, got %v", err)
	}
	if mock.createCalls != 0 {
		t.Errorf("expected inner service not to be called, got createCalls=%d", mock.createCalls)
	}
}

func TestAuthzCreateGitSecret_PDPError(t *testing.T) {
	pdpErr := errors.New("pdp unavailable")
	mock := &mockGitSecretService{}
	svc := newAuthzService(mock, &errorPDP{err: pdpErr})

	_, err := svc.CreateGitSecret(context.Background(), "ns1", &CreateGitSecretParams{
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
	if mock.createCalls != 0 {
		t.Errorf("expected inner service not to be called, got createCalls=%d", mock.createCalls)
	}
}

func TestAuthzCreateGitSecret_InternalError(t *testing.T) {
	internalErr := errors.New("internal create failed")
	svc := newAuthzService(&mockGitSecretService{createErr: internalErr}, &allowAllPDP{})

	_, err := svc.CreateGitSecret(context.Background(), "ns1", &CreateGitSecretParams{
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
	svc := newAuthzService(&mockGitSecretService{}, &allowAllPDP{})

	err := svc.DeleteGitSecret(context.Background(), "ns1", "my-secret")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAuthzDeleteGitSecret_Denied(t *testing.T) {
	mock := &mockGitSecretService{}
	svc := newAuthzService(mock, &denyAllPDP{})

	err := svc.DeleteGitSecret(context.Background(), "ns1", "my-secret")
	if !errors.Is(err, services.ErrForbidden) {
		t.Errorf("expected ErrForbidden, got %v", err)
	}
	if mock.deleteCalls != 0 {
		t.Errorf("expected inner service not to be called, got deleteCalls=%d", mock.deleteCalls)
	}
}

func TestAuthzDeleteGitSecret_PDPError(t *testing.T) {
	pdpErr := errors.New("pdp timeout")
	mock := &mockGitSecretService{}
	svc := newAuthzService(mock, &errorPDP{err: pdpErr})

	err := svc.DeleteGitSecret(context.Background(), "ns1", "my-secret")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, pdpErr) {
		t.Errorf("expected wrapped pdp error, got %v", err)
	}
	if mock.deleteCalls != 0 {
		t.Errorf("expected inner service not to be called, got deleteCalls=%d", mock.deleteCalls)
	}
}

func TestAuthzDeleteGitSecret_InternalError(t *testing.T) {
	internalErr := errors.New("internal delete failed")
	svc := newAuthzService(&mockGitSecretService{deleteErr: internalErr}, &allowAllPDP{})

	err := svc.DeleteGitSecret(context.Background(), "ns1", "my-secret")
	if !errors.Is(err, internalErr) {
		t.Errorf("expected internal error, got %v", err)
	}
}
