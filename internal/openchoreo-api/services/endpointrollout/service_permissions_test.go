// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package endpointrollout

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	authzcore "github.com/openchoreo/openchoreo/internal/authz/core"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	clusterendpointtypesvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/clusterendpointtype"
	endpointsvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/endpoint"
	endpointreleasesvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/endpointrelease"
	endpointreleasebindingsvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/endpointreleasebinding"
	endpointtypesvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/endpointtype"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/testutil"
)

// grantedPDP allows exactly the actions it holds.
type grantedPDP map[string]bool

func (p grantedPDP) Evaluate(_ context.Context, req *authzcore.EvaluateRequest) (*authzcore.Decision, error) {
	return &authzcore.Decision{Decision: p[req.Action], Context: &authzcore.DecisionContext{}}, nil
}

func (p grantedPDP) BatchEvaluate(_ context.Context, req *authzcore.BatchEvaluateRequest) (*authzcore.BatchEvaluateResponse, error) {
	decisions := make([]authzcore.Decision, len(req.Requests))
	for i, r := range req.Requests {
		decisions[i] = authzcore.Decision{Decision: p[r.Action]}
	}
	return &authzcore.BatchEvaluateResponse{Decisions: decisions}, nil
}

func (p grantedPDP) GetSubjectProfile(context.Context, *authzcore.ProfileRequest) (*authzcore.UserCapabilitiesResponse, error) {
	return nil, nil
}

func grant(actions ...string) grantedPDP {
	p := grantedPDP{}
	for _, a := range actions {
		p[a] = true
	}
	return p
}

var viewer = []string{
	authzcore.ActionViewEndpoint, authzcore.ActionViewEndpointRelease, authzcore.ActionViewEndpointReleaseBinding,
}

// authorized is the fixture's rollout service as handlers get it, on the
// authorized Endpoint, release and binding services.
func authorized(t *testing.T, pdp authzcore.PDP) (Service, client.Client) {
	t.Helper()
	_, c := fixture(t, true)
	logger := testutil.TestLogger()
	return NewService(endpointsvc.NewServiceWithAuthz(c, pdp, logger), endpointreleasesvc.NewServiceWithAuthz(c, pdp, logger),
		endpointreleasebindingsvc.NewServiceWithAuthz(c, pdp, logger), endpointreleasebindingsvc.NewService(c, logger),
		endpointtypesvc.NewService(c, logger),
		clusterendpointtypesvc.NewService(c, logger), logger), c
}

func TestPermissions_ViewerCanOnlyRead(t *testing.T) {
	ctx := testutil.AuthzContext()
	svc, c := authorized(t, grant(viewer...))

	status, err := svc.Get(ctx, ns, epName)
	require.NoError(t, err)
	assert.Len(t, status.Environments, 1)

	_, err = svc.Step(ctx, ns, epName, StepRequest{Environment: "dev"})
	assert.ErrorIs(t, err, services.ErrForbidden, "step")
	_, err = svc.Abort(ctx, ns, epName, AbortRequest{Environment: "dev"})
	assert.ErrorIs(t, err, services.ErrForbidden, "abort")
	_, err = svc.Finish(ctx, ns, epName, FinishRequest{Keep: "stable"})
	assert.ErrorIs(t, err, services.ErrForbidden, "finish")

	assert.Empty(t, overridesOf(t, c))
	ep := &openchoreov1alpha1.Endpoint{}
	require.NoError(t, c.Get(context.Background(), client.ObjectKey{Namespace: ns, Name: epName}, ep))
	assert.NotNil(t, ep.Spec.Rollout, "the plan is still there")
}

func TestPermissions_ViewerCannotStart(t *testing.T) {
	ctx := testutil.AuthzContext()
	svc, c := authorized(t, grant(viewer...))
	ep := &openchoreov1alpha1.Endpoint{}
	require.NoError(t, c.Get(ctx, client.ObjectKey{Namespace: ns, Name: epName}, ep))
	ep.Spec.Rollout = nil
	require.NoError(t, c.Update(ctx, ep))

	_, err := svc.Start(ctx, ns, epName, StartRequest{Rollout: canaryPlan()})
	assert.ErrorIs(t, err, services.ErrForbidden)
}

// Moving traffic needs update on the environment's binding only; changing
// the plan needs update on the Endpoint.
func TestPermissions_StepperMovesTrafficButNotThePlan(t *testing.T) {
	ctx := testutil.AuthzContext()
	svc, c := authorized(t, grant(append(viewer, authzcore.ActionUpdateEndpointReleaseBinding)...))

	_, err := svc.Step(ctx, ns, epName, StepRequest{Environment: "dev"})
	require.NoError(t, err)
	assert.Equal(t, map[string]int32{"stable": 95, "v2": 5}, overridesOf(t, c))
	_, err = svc.Abort(ctx, ns, epName, AbortRequest{Environment: "dev"})
	require.NoError(t, err)
	assert.Equal(t, map[string]int32{"stable": 100, "v2": 0}, overridesOf(t, c))

	_, err = svc.Finish(ctx, ns, epName, FinishRequest{Keep: "stable"})
	assert.ErrorIs(t, err, services.ErrForbidden)
}

// hidesStaging grants every action except on the staging environment's
// bindings, as an environment-scoped role does.
type hidesStaging struct{}

func (hidesStaging) Evaluate(_ context.Context, req *authzcore.EvaluateRequest) (*authzcore.Decision, error) {
	return &authzcore.Decision{Decision: !strings.HasSuffix(req.Context.Resource.Environment, "staging"),
		Context: &authzcore.DecisionContext{}}, nil
}

func (hidesStaging) BatchEvaluate(_ context.Context, req *authzcore.BatchEvaluateRequest) (*authzcore.BatchEvaluateResponse, error) {
	decisions := make([]authzcore.Decision, len(req.Requests))
	for i, r := range req.Requests {
		decisions[i] = authzcore.Decision{Decision: !strings.HasSuffix(r.Context.Resource.Environment, "staging")}
	}
	return &authzcore.BatchEvaluateResponse{Decisions: decisions}, nil
}

func (hidesStaging) GetSubjectProfile(context.Context, *authzcore.ProfileRequest) (*authzcore.UserCapabilitiesResponse, error) {
	return nil, nil
}

func TestPermissions_PlanChangesNeedEveryEnvironment(t *testing.T) {
	ctx := testutil.AuthzContext()
	svc, c := authorized(t, hidesStaging{})
	stagingOn(t, c, release)

	status, err := svc.Get(ctx, ns, epName)
	require.NoError(t, err)
	assert.Len(t, status.Environments, 1, "staging is hidden")
	_, err = svc.Step(ctx, ns, epName, StepRequest{Environment: "dev", DryRun: true})
	require.NoError(t, err, "a step in a visible environment needs only that one")

	hidden := func(t *testing.T, err error) {
		t.Helper()
		var conflictErr *ConflictError
		require.ErrorAs(t, err, &conflictErr)
		assert.Contains(t, conflictErr.Msg, "1 of this Endpoint's environments aren't visible to you")
		assert.NotContains(t, conflictErr.Msg, "staging", "the hidden environment isn't named")
	}
	_, err = svc.Finish(ctx, ns, epName, FinishRequest{Keep: "stable"})
	hidden(t, err)

	ep := &openchoreov1alpha1.Endpoint{}
	require.NoError(t, c.Get(ctx, client.ObjectKey{Namespace: ns, Name: epName}, ep))
	assert.NotNil(t, ep.Spec.Rollout, "nothing is finished")
	ep.Spec.Rollout = nil
	require.NoError(t, c.Update(ctx, ep))
	_, err = svc.Start(ctx, ns, epName, StartRequest{Rollout: canaryPlan()})
	hidden(t, err)
}

// A binding the caller can't view, created after the first visibility check,
// is caught before the Endpoint is saved.
func TestPermissions_FinishRechecksEveryEnvironmentBeforeTheSave(t *testing.T) {
	ctx := testutil.AuthzContext()
	lists := 0
	_, c := interceptedFixture(t, true, func(store client.WithWatch) interceptor.Funcs {
		return interceptor.Funcs{
			List: func(ctx context.Context, _ client.WithWatch, list client.ObjectList, opts ...client.ListOption) error {
				if _, ok := list.(*openchoreov1alpha1.EndpointReleaseBindingList); ok {
					if lists++; lists == 3 {
						staging := testutil.NewEndpointReleaseBinding(ns, project, epName, "staging", epName+"-staging")
						staging.Spec.EndpointRelease = release
						require.NoError(t, store.Create(ctx, staging))
					}
				}
				return store.List(ctx, list, opts...)
			},
		}
	})
	logger := testutil.TestLogger()
	pdp := hidesStaging{}
	svc := NewService(endpointsvc.NewServiceWithAuthz(c, pdp, logger), endpointreleasesvc.NewServiceWithAuthz(c, pdp, logger),
		endpointreleasebindingsvc.NewServiceWithAuthz(c, pdp, logger), endpointreleasebindingsvc.NewService(c, logger),
		endpointtypesvc.NewService(c, logger), clusterendpointtypesvc.NewService(c, logger), logger)

	_, err := svc.Finish(ctx, ns, epName, FinishRequest{Keep: "stable"})
	var conflictErr *ConflictError
	require.ErrorAs(t, err, &conflictErr)
	assert.Contains(t, conflictErr.Msg, "1 of this Endpoint's environments aren't visible to you")
	ep := &openchoreov1alpha1.Endpoint{}
	require.NoError(t, c.Get(ctx, client.ObjectKey{Namespace: ns, Name: epName}, ep))
	assert.NotNil(t, ep.Spec.Rollout, "nothing is finished")
}
