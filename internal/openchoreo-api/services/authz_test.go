// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package services

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	authz "github.com/openchoreo/openchoreo/internal/authz/core"
	"github.com/openchoreo/openchoreo/internal/server/middleware/auth"
)

// mockPDP is a test double for authz.PDP.
type mockPDP struct {
	evaluateFunc      func(ctx context.Context, req *authz.EvaluateRequest) (*authz.Decision, error)
	batchEvaluateFunc func(ctx context.Context, req *authz.BatchEvaluateRequest) (*authz.BatchEvaluateResponse, error)
}

func (m *mockPDP) Evaluate(ctx context.Context, req *authz.EvaluateRequest) (*authz.Decision, error) {
	return m.evaluateFunc(ctx, req)
}

func (m *mockPDP) BatchEvaluate(ctx context.Context, req *authz.BatchEvaluateRequest) (*authz.BatchEvaluateResponse, error) {
	return m.batchEvaluateFunc(ctx, req)
}

func (m *mockPDP) GetSubjectProfile(ctx context.Context, req *authz.ProfileRequest) (*authz.UserCapabilitiesResponse, error) {
	return nil, nil
}

// ctxWithSubject returns a context with the given SubjectContext set.
func ctxWithSubject(subjectCtx *auth.SubjectContext) context.Context {
	return auth.SetSubjectContext(context.Background(), subjectCtx)
}

// testSubjectContext returns a valid SubjectContext for testing.
func testSubjectContext() *auth.SubjectContext {
	return &auth.SubjectContext{
		ID:                "user-1",
		Type:              "user",
		EntitlementClaim:  "groups",
		EntitlementValues: []string{"org-admins"},
	}
}

// testCheckRequest returns a sample CheckRequest for testing.
func testCheckRequest() CheckRequest {
	return CheckRequest{
		Action:       "project:view",
		ResourceType: "project",
		ResourceID:   "my-project",
		Hierarchy:    authz.ResourceHierarchy{Namespace: "ns-1", Project: "my-project"},
	}
}

func newTestChecker(pdp authz.PDP) *AuthzChecker {
	return NewAuthzChecker(pdp, slog.Default())
}

// ---------------------------------------------------------------------------
// Check tests
// ---------------------------------------------------------------------------

func TestCheck_Allow(t *testing.T) {
	pdp := &mockPDP{
		evaluateFunc: func(_ context.Context, _ *authz.EvaluateRequest) (*authz.Decision, error) {
			return &authz.Decision{Decision: true, Context: &authz.DecisionContext{Reason: "allowed"}}, nil
		},
	}
	checker := newTestChecker(pdp)

	err := checker.Check(ctxWithSubject(testSubjectContext()), testCheckRequest())
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}

func TestCheck_Deny(t *testing.T) {
	pdp := &mockPDP{
		evaluateFunc: func(_ context.Context, _ *authz.EvaluateRequest) (*authz.Decision, error) {
			return &authz.Decision{Decision: false, Context: &authz.DecisionContext{Reason: "denied"}}, nil
		},
	}
	checker := newTestChecker(pdp)

	err := checker.Check(ctxWithSubject(testSubjectContext()), testCheckRequest())
	if !errors.Is(err, ErrForbidden) {
		t.Fatalf("expected ErrForbidden, got %v", err)
	}
}

func TestCheck_EvaluateError(t *testing.T) {
	evalErr := errors.New("pdp unavailable")
	pdp := &mockPDP{
		evaluateFunc: func(_ context.Context, _ *authz.EvaluateRequest) (*authz.Decision, error) {
			return nil, evalErr
		},
	}
	checker := newTestChecker(pdp)

	err := checker.Check(ctxWithSubject(testSubjectContext()), testCheckRequest())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, evalErr) {
		t.Fatalf("expected wrapped evalErr, got %v", err)
	}
}

func TestCheck_MissingSubjectContext(t *testing.T) {
	pdp := &mockPDP{
		evaluateFunc: func(_ context.Context, _ *authz.EvaluateRequest) (*authz.Decision, error) {
			t.Fatal("Evaluate should not be called when subject context is missing")
			return nil, nil
		},
	}
	checker := newTestChecker(pdp)

	// context.Background() has no SubjectContext set.
	err := checker.Check(context.Background(), testCheckRequest())
	if err == nil {
		t.Fatal("expected error for missing subject context, got nil")
	}
	if err.Error() != "failed to get user information from token" {
		t.Fatalf("unexpected error message: %v", err)
	}
}

// ---------------------------------------------------------------------------
// BatchCheck tests
// ---------------------------------------------------------------------------

func TestBatchCheck_EmptyRequests(t *testing.T) {
	pdp := &mockPDP{
		batchEvaluateFunc: func(_ context.Context, _ *authz.BatchEvaluateRequest) (*authz.BatchEvaluateResponse, error) {
			t.Fatal("BatchEvaluate should not be called for empty requests")
			return nil, nil
		},
	}
	checker := newTestChecker(pdp)

	results, err := checker.BatchCheck(ctxWithSubject(testSubjectContext()), []CheckRequest{})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if results != nil {
		t.Fatalf("expected nil results, got %v", results)
	}
}

func TestBatchCheck_AllAllowed(t *testing.T) {
	pdp := &mockPDP{
		batchEvaluateFunc: func(_ context.Context, req *authz.BatchEvaluateRequest) (*authz.BatchEvaluateResponse, error) {
			decisions := make([]authz.Decision, len(req.Requests))
			for i := range decisions {
				decisions[i] = authz.Decision{Decision: true}
			}
			return &authz.BatchEvaluateResponse{Decisions: decisions}, nil
		},
	}
	checker := newTestChecker(pdp)

	requests := []CheckRequest{testCheckRequest(), testCheckRequest()}
	results, err := checker.BatchCheck(ctxWithSubject(testSubjectContext()), requests)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	for i, r := range results {
		if !r {
			t.Errorf("expected results[%d] to be true", i)
		}
	}
}

func TestBatchCheck_MixedDecisions(t *testing.T) {
	pdp := &mockPDP{
		batchEvaluateFunc: func(_ context.Context, _ *authz.BatchEvaluateRequest) (*authz.BatchEvaluateResponse, error) {
			return &authz.BatchEvaluateResponse{
				Decisions: []authz.Decision{
					{Decision: true},
					{Decision: false},
					{Decision: true},
				},
			}, nil
		},
	}
	checker := newTestChecker(pdp)

	requests := []CheckRequest{testCheckRequest(), testCheckRequest(), testCheckRequest()}
	results, err := checker.BatchCheck(ctxWithSubject(testSubjectContext()), requests)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	expected := []bool{true, false, true}
	if len(results) != len(expected) {
		t.Fatalf("expected %d results, got %d", len(expected), len(results))
	}
	for i, want := range expected {
		if results[i] != want {
			t.Errorf("results[%d] = %v, want %v", i, results[i], want)
		}
	}
}

func TestBatchCheck_EvaluateError(t *testing.T) {
	batchErr := errors.New("batch pdp error")
	pdp := &mockPDP{
		batchEvaluateFunc: func(_ context.Context, _ *authz.BatchEvaluateRequest) (*authz.BatchEvaluateResponse, error) {
			return nil, batchErr
		},
	}
	checker := newTestChecker(pdp)

	_, err := checker.BatchCheck(ctxWithSubject(testSubjectContext()), []CheckRequest{testCheckRequest()})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, batchErr) {
		t.Fatalf("expected wrapped batchErr, got %v", err)
	}
}

func TestBatchCheck_MissingSubjectContext(t *testing.T) {
	pdp := &mockPDP{
		batchEvaluateFunc: func(_ context.Context, _ *authz.BatchEvaluateRequest) (*authz.BatchEvaluateResponse, error) {
			t.Fatal("BatchEvaluate should not be called when subject context is missing")
			return nil, nil
		},
	}
	checker := newTestChecker(pdp)

	_, err := checker.BatchCheck(context.Background(), []CheckRequest{testCheckRequest()})
	if err == nil {
		t.Fatal("expected error for missing subject context, got nil")
	}
	if err.Error() != "failed to get user information from token" {
		t.Fatalf("unexpected error message: %v", err)
	}
}

func TestBatchCheck_NilSubjectContext(t *testing.T) {
	pdp := &mockPDP{
		batchEvaluateFunc: func(_ context.Context, _ *authz.BatchEvaluateRequest) (*authz.BatchEvaluateResponse, error) {
			t.Fatal("BatchEvaluate should not be called when subject context is nil")
			return nil, nil
		},
	}
	checker := newTestChecker(pdp)

	_, err := checker.BatchCheck(ctxWithSubject(nil), []CheckRequest{testCheckRequest()})
	if err == nil {
		t.Fatal("expected error for nil subject context, got nil")
	}
}

func TestBatchCheck_SingleRequest(t *testing.T) {
	pdp := &mockPDP{
		batchEvaluateFunc: func(_ context.Context, _ *authz.BatchEvaluateRequest) (*authz.BatchEvaluateResponse, error) {
			return &authz.BatchEvaluateResponse{
				Decisions: []authz.Decision{{Decision: false}},
			}, nil
		},
	}
	checker := newTestChecker(pdp)

	results, err := checker.BatchCheck(ctxWithSubject(testSubjectContext()), []CheckRequest{testCheckRequest()})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0] {
		t.Error("expected results[0] to be false")
	}
}
