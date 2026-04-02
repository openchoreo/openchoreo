// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package testutil

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/require"

	authzcore "github.com/openchoreo/openchoreo/internal/authz/core"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	"github.com/openchoreo/openchoreo/internal/server/middleware/auth"
)

// CapturingPDP is a mock authz.PDP that records every EvaluateRequest and
// returns a configurable decision. Use AllowPDP / DenyPDP / ErrorPDP to
// construct common variants.
type CapturingPDP struct {
	Captured []*authzcore.EvaluateRequest
	Decision bool
	Err      error
}

func (p *CapturingPDP) Evaluate(_ context.Context, req *authzcore.EvaluateRequest) (*authzcore.Decision, error) {
	p.Captured = append(p.Captured, req)
	if p.Err != nil {
		return nil, p.Err
	}
	return &authzcore.Decision{Decision: p.Decision, Context: &authzcore.DecisionContext{}}, nil
}

func (p *CapturingPDP) BatchEvaluate(_ context.Context, req *authzcore.BatchEvaluateRequest) (*authzcore.BatchEvaluateResponse, error) {
	decisions := make([]authzcore.Decision, len(req.Requests))
	for i, r := range req.Requests {
		p.Captured = append(p.Captured, &r)
		decisions[i] = authzcore.Decision{Decision: p.Decision}
	}
	if p.Err != nil {
		return nil, p.Err
	}
	return &authzcore.BatchEvaluateResponse{Decisions: decisions}, nil
}

func (p *CapturingPDP) GetSubjectProfile(_ context.Context, _ *authzcore.ProfileRequest) (*authzcore.UserCapabilitiesResponse, error) {
	return nil, nil
}

// AllowPDP returns a CapturingPDP that approves every request.
func AllowPDP() *CapturingPDP { return &CapturingPDP{Decision: true} }

// DenyPDP returns a CapturingPDP that denies every request.
func DenyPDP() *CapturingPDP { return &CapturingPDP{Decision: false} }

// ErrorPDP returns a CapturingPDP that returns err from every Evaluate call.
func ErrorPDP(err error) *CapturingPDP { return &CapturingPDP{Err: err} }

// AuthzContext returns a context carrying a valid SubjectContext for use in
// tests that exercise the authz wrappers.
func AuthzContext() context.Context {
	return auth.SetSubjectContext(context.Background(), &auth.SubjectContext{
		ID:                "user-1",
		Type:              "user",
		EntitlementClaim:  "groups",
		EntitlementValues: []string{"org-admins"},
	})
}

// NewTestAuthzChecker creates an AuthzChecker backed by pdp and a discard logger.
func NewTestAuthzChecker(pdp authzcore.PDP) *services.AuthzChecker {
	return services.NewAuthzChecker(pdp, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

// RequireEvalRequest asserts all fields of a captured EvaluateRequest.
func RequireEvalRequest(t *testing.T, req *authzcore.EvaluateRequest, action, resourceType, resourceID string, hierarchy authzcore.ResourceHierarchy) {
	t.Helper()
	require.Equal(t, action, req.Action, "action mismatch")
	require.Equal(t, resourceType, req.Resource.Type, "resourceType mismatch")
	require.Equal(t, resourceID, req.Resource.ID, "resourceID mismatch")
	require.Equal(t, hierarchy, req.Resource.Hierarchy, "hierarchy mismatch")
}
