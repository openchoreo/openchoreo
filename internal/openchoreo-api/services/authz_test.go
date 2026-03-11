// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package services

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/require"

	disabledAuthz "github.com/openchoreo/openchoreo/internal/authz"
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
	if m.batchEvaluateFunc == nil {
		return &authz.BatchEvaluateResponse{}, nil
	}
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

func TestCheck(t *testing.T) {
	evalErr := errors.New("pdp unavailable")

	tests := []struct {
		name     string
		decision *authz.Decision
		evalErr  error
		checkErr func(t *testing.T, err error)
	}{
		{
			name: "allow",
			decision: &authz.Decision{
				Decision: true,
				Context:  &authz.DecisionContext{Reason: "allowed"},
			},
			checkErr: func(t *testing.T, err error) {
				require.NoError(t, err)
			},
		},
		{
			name: "deny",
			decision: &authz.Decision{
				Decision: false,
				Context:  &authz.DecisionContext{Reason: "denied"},
			},
			checkErr: func(t *testing.T, err error) {
				require.ErrorIs(t, err, ErrForbidden)
			},
		},
		{
			name:    "evaluate error",
			evalErr: evalErr,
			checkErr: func(t *testing.T, err error) {
				require.ErrorIs(t, err, evalErr)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pdp := &mockPDP{
				evaluateFunc: func(_ context.Context, _ *authz.EvaluateRequest) (*authz.Decision, error) {
					if tt.evalErr != nil {
						return nil, tt.evalErr
					}
					return tt.decision, nil
				},
			}
			checker := newTestChecker(pdp)

			err := checker.Check(ctxWithSubject(testSubjectContext()), testCheckRequest())
			tt.checkErr(t, err)
		})
	}
}

func TestCheck_NilSubject_DisabledAuthz(t *testing.T) {
	checker := NewAuthzChecker(disabledAuthz.NewDisabledAuthorizer(slog.Default()), slog.Default())

	// context.Background() has no SubjectContext — disabled authorizer should still allow.
	err := checker.Check(context.Background(), testCheckRequest())
	require.NoError(t, err, "expected nil error with disabled authz")
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
	require.NoError(t, err)
	require.Nil(t, results)
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
	require.NoError(t, err)
	require.Len(t, results, 2)
	for i, r := range results {
		require.Truef(t, r, "expected results[%d] to be true", i)
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
	require.NoError(t, err)
	expected := []bool{true, false, true}
	require.Equal(t, expected, results)
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
	require.Error(t, err)
	require.ErrorIs(t, err, batchErr)
}

func TestBatchCheck_NilSubject_DisabledAuthz(t *testing.T) {
	checker := NewAuthzChecker(disabledAuthz.NewDisabledAuthorizer(slog.Default()), slog.Default())

	// context.Background() has no SubjectContext — disabled authorizer should still allow.
	results, err := checker.BatchCheck(context.Background(), []CheckRequest{testCheckRequest()})
	require.NoError(t, err, "expected nil error with disabled authz")
	require.Equal(t, []bool{true}, results)
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
	require.NoError(t, err)
	require.Len(t, results, 1)
	require.False(t, results[0], "expected results[0] to be false")
}
