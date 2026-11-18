// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package endpointrollout

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	clusterendpointtypesvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/clusterendpointtype"
	endpointsvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/endpoint"
	endpointreleasesvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/endpointrelease"
	endpointreleasebindingsvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/endpointreleasebinding"
	endpointtypesvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/endpointtype"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/testutil"
	"github.com/openchoreo/openchoreo/pkg/endpointrollout"
)

const (
	ns      = "test-ns"
	project = "test-project"
	epName  = "greeter-api"
	release = "greeter-api-1"
)

func candidate() openchoreov1alpha1.EndpointBackendRef {
	return openchoreov1alpha1.EndpointBackendRef{
		Name:      "v2",
		Kind:      openchoreov1alpha1.EndpointBackendKindComponent,
		Component: &openchoreov1alpha1.EndpointComponentBackend{Name: "greeter-v2", Endpoint: "http"},
		Weight:    ptr(int32(0)),
		Match: &openchoreov1alpha1.EndpointMatch{
			Headers: []openchoreov1alpha1.EndpointHeaderMatch{{Name: "x-canary", Value: "true"}},
		},
	}
}

func canaryPlan() openchoreov1alpha1.EndpointRollout {
	return openchoreov1alpha1.EndpointRollout{
		Strategy:   openchoreov1alpha1.EndpointRolloutCanary,
		Stable:     "stable",
		Candidates: []openchoreov1alpha1.EndpointRolloutCandidate{{Name: "v2"}},
		Canary: &openchoreov1alpha1.EndpointRolloutSteps{Steps: []openchoreov1alpha1.EndpointRolloutStep{
			{Backends: []openchoreov1alpha1.EndpointRolloutStepBackend{{Name: "v2", Percent: 5}}},
			{Backends: []openchoreov1alpha1.EndpointRolloutStepBackend{{Name: "v2", Percent: 100}}},
		}},
	}
}

// fixture is an Endpoint with stable and v2, its release, and a dev binding.
func fixture(t *testing.T, withPlan bool) (Service, client.Client) {
	t.Helper()
	return interceptedFixture(t, withPlan, nil)
}

// interceptedFixture is fixture with funcs run on each call the services
// make; they get the client the fixture's objects are stored in.
func interceptedFixture(t *testing.T, withPlan bool, funcs func(store client.WithWatch) interceptor.Funcs) (Service, client.Client) {
	t.Helper()
	ep := testutil.NewEndpoint(ns, project, epName)
	ep.Spec.BackendRefs = append(ep.Spec.BackendRefs, candidate())
	if withPlan {
		plan := canaryPlan()
		ep.Spec.Rollout = &plan
	}
	rel := testutil.NewEndpointRelease(ns, project, epName, release)
	rel.Spec.BackendRefs = ep.Spec.BackendRefs
	b := testutil.NewEndpointReleaseBinding(ns, project, epName, "dev", epName+"-dev")
	b.Spec.EndpointRelease = release
	b.Status.Backends = reported()
	b.Status.Conditions = reconciled()
	release2 := rel.DeepCopy()
	release2.Name = release + "-next"
	c := testutil.NewFakeClient(ep, rel, release2, b)
	if funcs != nil {
		store := c.(client.WithWatch)
		c = interceptor.NewClient(store, funcs(store))
	}
	logger := testutil.TestLogger()
	return NewService(endpointsvc.NewService(c, logger), endpointreleasesvc.NewService(c, logger),
		endpointreleasebindingsvc.NewService(c, logger), endpointreleasebindingsvc.NewService(c, logger),
		endpointtypesvc.NewService(c, logger),
		clusterendpointtypesvc.NewService(c, logger), logger), c
}

func overridesOf(t *testing.T, c client.Client) map[string]int32 {
	t.Helper()
	b := &openchoreov1alpha1.EndpointReleaseBinding{}
	require.NoError(t, c.Get(context.Background(), client.ObjectKey{Namespace: ns, Name: epName + "-dev"}, b))
	out := map[string]int32{}
	for _, o := range b.Spec.BackendOverrides {
		out[o.Name] = *o.Weight
	}
	return out
}

func TestStart(t *testing.T) {
	ctx := context.Background()
	svc, c := fixture(t, false)

	status, err := svc.Start(ctx, ns, epName, StartRequest{Rollout: canaryPlan()})
	require.NoError(t, err)
	require.NotNil(t, status.Rollout)
	assert.Len(t, status.Stages, 3)
	require.Len(t, status.Environments, 1)
	assert.Equal(t, 0, *status.Environments[0].Step)

	ep := &openchoreov1alpha1.Endpoint{}
	require.NoError(t, c.Get(ctx, client.ObjectKey{Namespace: ns, Name: epName}, ep))
	assert.Equal(t, "stable", ep.Spec.Rollout.Stable)

	_, err = svc.Start(ctx, ns, epName, StartRequest{Rollout: canaryPlan()})
	var conflictErr *ConflictError
	assert.ErrorAs(t, err, &conflictErr, "a second plan is refused")
}

func TestStep(t *testing.T) {
	ctx := context.Background()
	svc, c := fixture(t, true)

	status, err := svc.Step(ctx, ns, epName, StepRequest{Environment: "dev", ExpectedStep: ptr(0)})
	require.NoError(t, err)
	assert.Equal(t, map[string]int32{"stable": 95, "v2": 5}, overridesOf(t, c))
	assert.Equal(t, 1, *status.Environments[0].Step)
	assert.Equal(t, "dev", status.Change.Environment)

	_, err = svc.Step(ctx, ns, epName, StepRequest{Environment: "dev", ExpectedStep: ptr(0)})
	var conflictErr *ConflictError
	require.ErrorAs(t, err, &conflictErr, "a stale expected step is refused")
	assert.Contains(t, conflictErr.Msg, "step 1")

	_, err = svc.Step(ctx, ns, epName, StepRequest{Environment: "dev"})
	require.NoError(t, err)
	assert.Equal(t, map[string]int32{"stable": 0, "v2": 100}, overridesOf(t, c))

	_, err = svc.Step(ctx, ns, epName, StepRequest{Environment: "dev"})
	assert.ErrorAs(t, err, &conflictErr, "nothing is past the last step")
}

func TestStep_DryRunWritesNothing(t *testing.T) {
	svc, c := fixture(t, true)
	status, err := svc.Step(context.Background(), ns, epName, StepRequest{Environment: "dev", DryRun: true})
	require.NoError(t, err)
	assert.Len(t, status.Change.After, 2)
	assert.Empty(t, overridesOf(t, c))
}

func TestStep_UnknownEnvironment(t *testing.T) {
	svc, _ := fixture(t, true)
	_, err := svc.Step(context.Background(), ns, epName, StepRequest{Environment: "prod"})
	assert.True(t, errors.Is(err, ErrEnvironmentNotBound))
}

func TestAbort(t *testing.T) {
	ctx := context.Background()
	svc, c := fixture(t, true)
	_, err := svc.Step(ctx, ns, epName, StepRequest{Environment: "dev", To: ptr(2)})
	require.NoError(t, err)

	_, err = svc.Abort(ctx, ns, epName, AbortRequest{Environment: "dev"})
	require.NoError(t, err)
	assert.Equal(t, map[string]int32{"stable": 100, "v2": 0}, overridesOf(t, c))
}

func TestFinish(t *testing.T) {
	ctx := context.Background()
	svc, c := fixture(t, true)

	b := &openchoreov1alpha1.EndpointReleaseBinding{}
	require.NoError(t, c.Get(ctx, client.ObjectKey{Namespace: ns, Name: epName + "-dev"}, b))
	b.Status.Backends = []openchoreov1alpha1.ResolvedEndpointBackend{{Name: "stable", Weight: 100}, {Name: "v2"}}
	require.NoError(t, c.Update(ctx, b))

	_, err := svc.Finish(ctx, ns, epName, FinishRequest{Keep: "v2"})
	var conflictErr *ConflictError
	require.ErrorAs(t, err, &conflictErr)
	assert.Contains(t, conflictErr.Msg, "stable in dev")

	status, err := svc.Finish(ctx, ns, epName, FinishRequest{Keep: "stable"})
	require.NoError(t, err)
	assert.Nil(t, status.Rollout)

	ep := &openchoreov1alpha1.Endpoint{}
	require.NoError(t, c.Get(ctx, client.ObjectKey{Namespace: ns, Name: epName}, ep))
	assert.Nil(t, ep.Spec.Rollout)
	require.Len(t, ep.Spec.BackendRefs, 1)
	assert.Equal(t, "stable", ep.Spec.BackendRefs[0].Name)
}

// asMirror turns the stored plan into a mirror rollout to v2.
func asMirror(t *testing.T, c client.Client) {
	t.Helper()
	ep := &openchoreov1alpha1.Endpoint{}
	require.NoError(t, c.Get(context.Background(), client.ObjectKey{Namespace: ns, Name: epName}, ep))
	ep.Spec.BackendRefs[1].Match = nil
	ep.Spec.BackendRefs[1].Mirror = &openchoreov1alpha1.EndpointMirror{Percent: 0}
	ep.Spec.Rollout.Strategy = openchoreov1alpha1.EndpointRolloutMirror
	ep.Spec.Rollout.Mirror, ep.Spec.Rollout.Canary = ep.Spec.Rollout.Canary, nil
	require.NoError(t, c.Update(context.Background(), ep))
}

func TestFinishDropsTheLeavingMirror(t *testing.T) {
	ctx := context.Background()
	svc, c := fixture(t, true)
	asMirror(t, c)

	b := &openchoreov1alpha1.EndpointReleaseBinding{}
	require.NoError(t, c.Get(ctx, client.ObjectKey{Namespace: ns, Name: epName + "-dev"}, b))
	b.Spec.BackendOverrides = []openchoreov1alpha1.EndpointBackendOverride{
		{Name: "stable", Weight: ptr(int32(100))},
		{Name: "v2", Mirror: &openchoreov1alpha1.EndpointMirror{Percent: 100}},
	}
	require.NoError(t, c.Update(ctx, b))

	_, err := svc.Finish(ctx, ns, epName, FinishRequest{Keep: "stable"})
	require.NoError(t, err)

	require.NoError(t, c.Get(ctx, client.ObjectKey{Namespace: ns, Name: epName + "-dev"}, b))
	assert.Empty(t, b.Spec.BackendOverrides,
		"the mirror share goes, and stable's weight too: it is the only backend with weight, so its 100 changes nothing")
}

func TestNoRollout(t *testing.T) {
	svc, _ := fixture(t, false)
	_, err := svc.Step(context.Background(), ns, epName, StepRequest{Environment: "dev"})
	assert.True(t, errors.Is(err, ErrNoRollout))
}

// afterBindingList runs change on the stored binding once a call has listed
// the bindings n times, as autoDeploy or another user would.
func afterBindingList(
	t *testing.T, n int, change func(*openchoreov1alpha1.EndpointReleaseBinding),
) func(client.WithWatch) interceptor.Funcs {
	return func(store client.WithWatch) interceptor.Funcs {
		lists := 0
		return interceptor.Funcs{
			List: func(ctx context.Context, _ client.WithWatch, list client.ObjectList, opts ...client.ListOption) error {
				if err := store.List(ctx, list, opts...); err != nil {
					return err
				}
				if _, ok := list.(*openchoreov1alpha1.EndpointReleaseBindingList); !ok {
					return nil
				}
				if lists++; lists != n {
					return nil
				}
				b := &openchoreov1alpha1.EndpointReleaseBinding{}
				require.NoError(t, store.Get(ctx, client.ObjectKey{Namespace: ns, Name: epName + "-dev"}, b))
				change(b)
				return store.Update(ctx, b)
			},
		}
	}
}

func TestStep_RetriesOnConflict(t *testing.T) {
	updates := 0
	svc, c := interceptedFixture(t, true, func(store client.WithWatch) interceptor.Funcs {
		return interceptor.Funcs{
			Update: func(ctx context.Context, _ client.WithWatch, obj client.Object, opts ...client.UpdateOption) error {
				if _, ok := obj.(*openchoreov1alpha1.EndpointReleaseBinding); ok {
					if updates++; updates == 1 {
						return apierrors.NewConflict(schema.GroupResource{Resource: "endpointreleasebindings"}, obj.GetName(), errors.New("stale"))
					}
				}
				return store.Update(ctx, obj, opts...)
			},
		}
	})

	_, err := svc.Step(context.Background(), ns, epName, StepRequest{Environment: "dev"})
	require.NoError(t, err)
	assert.Equal(t, 2, updates)
	assert.Equal(t, map[string]int32{"stable": 95, "v2": 5}, overridesOf(t, c))
}

func TestStep_KeepsAConcurrentRelease(t *testing.T) {
	svc, c := interceptedFixture(t, true, afterBindingList(t, 1, func(b *openchoreov1alpha1.EndpointReleaseBinding) {
		b.Spec.EndpointRelease = release + "-next"
	}))

	_, err := svc.Step(context.Background(), ns, epName, StepRequest{Environment: "dev"})
	require.NoError(t, err)
	b := &openchoreov1alpha1.EndpointReleaseBinding{}
	require.NoError(t, c.Get(context.Background(), client.ObjectKey{Namespace: ns, Name: epName + "-dev"}, b))
	assert.Equal(t, release+"-next", b.Spec.EndpointRelease, "the step doesn't undo the promotion")
	assert.Equal(t, map[string]int32{"stable": 95, "v2": 5}, overridesOf(t, c))
}

func TestStep_ExpectStepRecheckedOnRetry(t *testing.T) {
	svc, c := interceptedFixture(t, true, afterBindingList(t, 1, func(b *openchoreov1alpha1.EndpointReleaseBinding) {
		b.Spec.BackendOverrides = []openchoreov1alpha1.EndpointBackendOverride{
			{Name: "stable", Weight: ptr(int32(95))}, {Name: "v2", Weight: ptr(int32(5))},
		}
	}))

	_, err := svc.Step(context.Background(), ns, epName, StepRequest{Environment: "dev", ExpectedStep: ptr(0)})
	var conflictErr *ConflictError
	require.ErrorAs(t, err, &conflictErr)
	assert.Contains(t, conflictErr.Msg, "step 1")
	assert.Equal(t, map[string]int32{"stable": 95, "v2": 5}, overridesOf(t, c), "the other step stands")
}

func TestStart_ConflictBecomes409(t *testing.T) {
	svc, _ := interceptedFixture(t, false, func(store client.WithWatch) interceptor.Funcs {
		return interceptor.Funcs{
			Update: func(_ context.Context, _ client.WithWatch, obj client.Object, _ ...client.UpdateOption) error {
				return apierrors.NewConflict(schema.GroupResource{Resource: "endpoints"}, obj.GetName(), errors.New("stale"))
			},
		}
	})

	_, err := svc.Start(context.Background(), ns, epName, StartRequest{Rollout: canaryPlan()})
	var conflictErr *ConflictError
	require.ErrorAs(t, err, &conflictErr)
	assert.Contains(t, conflictErr.Msg, "changed while saving")
}

func TestStep_ReadsEachReleaseAndTheBindingsOnce(t *testing.T) {
	ctx := context.Background()
	releaseGets, bindingLists := 0, 0
	svc, c := interceptedFixture(t, true, func(store client.WithWatch) interceptor.Funcs {
		return interceptor.Funcs{
			Get: func(ctx context.Context, _ client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
				if _, ok := obj.(*openchoreov1alpha1.EndpointRelease); ok {
					releaseGets++
				}
				return store.Get(ctx, key, obj, opts...)
			},
			List: func(ctx context.Context, _ client.WithWatch, list client.ObjectList, opts ...client.ListOption) error {
				if _, ok := list.(*openchoreov1alpha1.EndpointReleaseBindingList); ok {
					bindingLists++
				}
				return store.List(ctx, list, opts...)
			},
		}
	})
	staging := testutil.NewEndpointReleaseBinding(ns, project, epName, "staging", epName+"-staging")
	staging.Spec.EndpointRelease = release
	staging.Status.Backends = reported()
	staging.Status.Conditions = reconciled()
	require.NoError(t, c.Create(ctx, staging))

	status, err := svc.Step(ctx, ns, epName, StepRequest{Environment: "dev"})
	require.NoError(t, err)
	assert.Len(t, status.Environments, 2)
	assert.Equal(t, 1, releaseGets, "both environments run the same release")
	assert.Equal(t, 1, bindingLists)
	assert.Equal(t, map[string]int32{"stable": 95, "v2": 5}, overridesOf(t, c))
}

// withType stores the fixture Endpoint's type, declaring the given
// capabilities.
func withType(t *testing.T, c client.Client, capabilities ...openchoreov1alpha1.EndpointCapability) {
	t.Helper()
	et := testutil.NewEndpointType(ns, "http-gateway")
	et.Spec.Capabilities = capabilities
	require.NoError(t, c.Create(context.Background(), et))
}

func mirrorStart(percents ...int32) StartRequest {
	steps := make([]openchoreov1alpha1.EndpointRolloutStep, 0, len(percents))
	for _, p := range percents {
		steps = append(steps, openchoreov1alpha1.EndpointRolloutStep{
			Backends: []openchoreov1alpha1.EndpointRolloutStepBackend{{Name: "v2", Percent: p}},
		})
	}
	shadow := candidate()
	shadow.Match = nil
	shadow.Mirror = &openchoreov1alpha1.EndpointMirror{Percent: 0}
	return StartRequest{
		Rollout: openchoreov1alpha1.EndpointRollout{
			Strategy:   openchoreov1alpha1.EndpointRolloutMirror,
			Stable:     "stable",
			Candidates: []openchoreov1alpha1.EndpointRolloutCandidate{{Name: "v2"}},
			Mirror:     &openchoreov1alpha1.EndpointRolloutSteps{Steps: steps},
		},
		Backends: []openchoreov1alpha1.EndpointBackendRef{shadow},
	}
}

func TestStart_RefusesWhatTheTypeCannotRender(t *testing.T) {
	ctx := context.Background()
	svc, c := fixture(t, false)
	withType(t, c, openchoreov1alpha1.EndpointCapabilityMatch)

	_, err := svc.Start(ctx, ns, epName, mirrorStart(100))
	var validationErr *services.ValidationError
	require.ErrorAs(t, err, &validationErr)
	assert.Contains(t, validationErr.Msg, `EndpointType "http-gateway" can't render this rollout`)
	assert.Contains(t, validationErr.Msg, "Mirror capability")

	ep := &openchoreov1alpha1.Endpoint{}
	require.NoError(t, c.Get(ctx, client.ObjectKey{Namespace: ns, Name: epName}, ep))
	assert.Nil(t, ep.Spec.Rollout, "nothing is saved")

	_, err = svc.Start(ctx, ns, epName, StartRequest{Rollout: canaryPlan()})
	assert.NoError(t, err, "a canary whose testers match a header needs only Match")
}

func TestStart_MirrorShareNeedsMirrorPercent(t *testing.T) {
	svc, c := fixture(t, false)
	withType(t, c, openchoreov1alpha1.EndpointCapabilityMirror)

	_, err := svc.Start(context.Background(), ns, epName, mirrorStart(10, 100))
	var validationErr *services.ValidationError
	require.ErrorAs(t, err, &validationErr)
	assert.Contains(t, validationErr.Msg, "MirrorPercent")

	_, err = svc.Start(context.Background(), ns, epName, mirrorStart(100))
	assert.NoError(t, err, "mirroring every request needs only Mirror")
}

func TestStart_TypeWithoutCapabilitiesIsNotChecked(t *testing.T) {
	svc, c := fixture(t, false)
	withType(t, c)
	_, err := svc.Start(context.Background(), ns, epName, mirrorStart(10, 100))
	assert.NoError(t, err)
}

// serve sets what the dev binding reports each backend gets.
func serve(t *testing.T, c client.Client, backends ...openchoreov1alpha1.ResolvedEndpointBackend) {
	t.Helper()
	b := &openchoreov1alpha1.EndpointReleaseBinding{}
	require.NoError(t, c.Get(context.Background(), client.ObjectKey{Namespace: ns, Name: epName + "-dev"}, b))
	b.Status.Backends = backends
	require.NoError(t, c.Update(context.Background(), b))
}

// reported is what a binding on the fixture's release reports once it has
// reconciled: stable and v2 resolved.
func reported() []openchoreov1alpha1.ResolvedEndpointBackend {
	return []openchoreov1alpha1.ResolvedEndpointBackend{deployed("stable", 100), deployed("v2", 0)}
}

// reconciled is the condition a binding reports once the controller has
// resolved the backends of its current spec.
func reconciled() []metav1.Condition {
	return []metav1.Condition{{Type: "BackendsResolved", Status: metav1.ConditionTrue, Reason: "BackendsResolved",
		LastTransitionTime: metav1.Now()}}
}

func deployed(name string, weight int32) openchoreov1alpha1.ResolvedEndpointBackend {
	return openchoreov1alpha1.ResolvedEndpointBackend{Name: name, Weight: weight, Resolved: true}
}

func gone(name string, weight int32) openchoreov1alpha1.ResolvedEndpointBackend {
	return openchoreov1alpha1.ResolvedEndpointBackend{
		Name: name, Weight: weight, Reason: "NotDeployed",
		Message: `component "greeter-v2" is not deployed in environment "dev"`,
	}
}

func TestStep_RefusesTrafficToAnUndeployedBackend(t *testing.T) {
	ctx := context.Background()
	svc, c := fixture(t, true)
	serve(t, c, deployed("stable", 100), gone("v2", 0))

	_, err := svc.Step(ctx, ns, epName, StepRequest{Environment: "dev"})
	var conflictErr *ConflictError
	require.ErrorAs(t, err, &conflictErr)
	assert.Contains(t, conflictErr.Msg, `v2, which isn't deployed in dev (component "greeter-v2" is not deployed`)
	assert.Empty(t, overridesOf(t, c), "nothing is saved")

	_, err = svc.Step(ctx, ns, epName, StepRequest{Environment: "dev", DryRun: true})
	require.ErrorAs(t, err, &conflictErr, "a dry run is refused the same way")

	_, err = svc.Abort(ctx, ns, epName, AbortRequest{Environment: "dev"})
	require.NoError(t, err, "moving traffic off it is allowed")
	assert.Equal(t, map[string]int32{"stable": 100, "v2": 0}, overridesOf(t, c))
}

func TestAbort_RefusedWhenStableIsGone(t *testing.T) {
	ctx := context.Background()
	svc, c := fixture(t, true)
	_, err := svc.Step(ctx, ns, epName, StepRequest{Environment: "dev"})
	require.NoError(t, err)
	serve(t, c, gone("stable", 95), deployed("v2", 5))

	_, err = svc.Abort(ctx, ns, epName, AbortRequest{Environment: "dev"})
	var conflictErr *ConflictError
	require.ErrorAs(t, err, &conflictErr)
	assert.Contains(t, conflictErr.Msg, "stable, which isn't deployed in dev")

	_, err = svc.Step(ctx, ns, epName, StepRequest{Environment: "dev", To: ptr(2)})
	require.NoError(t, err, "the last step moves everything to the candidate")
	assert.Equal(t, map[string]int32{"stable": 0, "v2": 100}, overridesOf(t, c))
}

func TestFinish_AfterTheCandidateIsGone(t *testing.T) {
	ctx := context.Background()
	svc, c := fixture(t, true)
	_, err := svc.Step(ctx, ns, epName, StepRequest{Environment: "dev"})
	require.NoError(t, err)
	serve(t, c, deployed("stable", 95), gone("v2", 5))

	_, err = svc.Finish(ctx, ns, epName, FinishRequest{Keep: "stable"})
	var conflictErr *ConflictError
	require.ErrorAs(t, err, &conflictErr)
	assert.Contains(t, conflictErr.Msg, "v2 in dev still gets traffic")

	_, err = svc.Abort(ctx, ns, epName, AbortRequest{Environment: "dev"})
	require.NoError(t, err)
	serve(t, c, deployed("stable", 100), gone("v2", 0))

	_, err = svc.Finish(ctx, ns, epName, FinishRequest{Keep: "stable"})
	require.NoError(t, err)
	ep := &openchoreov1alpha1.Endpoint{}
	require.NoError(t, c.Get(ctx, client.ObjectKey{Namespace: ns, Name: epName}, ep))
	require.Len(t, ep.Spec.BackendRefs, 1)
	assert.Equal(t, "stable", ep.Spec.BackendRefs[0].Name)
}

func stagingOn(t *testing.T, c client.Client, releaseName string) {
	t.Helper()
	b := testutil.NewEndpointReleaseBinding(ns, project, epName, "staging", epName+"-staging")
	b.Spec.EndpointRelease = releaseName
	b.Status.Backends = reported()
	b.Status.Conditions = reconciled()
	require.NoError(t, c.Create(context.Background(), b))
}

func stagingOverrides(t *testing.T, c client.Client) []openchoreov1alpha1.EndpointBackendOverride {
	t.Helper()
	b := &openchoreov1alpha1.EndpointReleaseBinding{}
	require.NoError(t, c.Get(context.Background(), client.ObjectKey{Namespace: ns, Name: epName + "-staging"}, b))
	return b.Spec.BackendOverrides
}

// A release promoted mid-rollout starts the next environment at 0%, and each
// environment then moves on its own.
func TestStep_EnvironmentsMoveIndependently(t *testing.T) {
	ctx := context.Background()
	svc, c := fixture(t, true)
	stagingOn(t, c, release)

	status, err := svc.Step(ctx, ns, epName, StepRequest{Environment: "dev"})
	require.NoError(t, err)
	steps := map[string]int{}
	for _, e := range status.Environments {
		steps[e.Environment] = *e.Step
	}
	assert.Equal(t, map[string]int{"dev": 1, "staging": 0}, steps)
	assert.Empty(t, stagingOverrides(t, c))

	_, err = svc.Step(ctx, ns, epName, StepRequest{Environment: "staging", ExpectedStep: ptr(0)})
	require.NoError(t, err)
	assert.Len(t, stagingOverrides(t, c), 2)
	assert.Equal(t, map[string]int32{"stable": 95, "v2": 5}, overridesOf(t, c), "dev is unchanged")
}

func TestStep_RefusedAfterARollback(t *testing.T) {
	ctx := context.Background()
	svc, c := fixture(t, true)
	old := testutil.NewEndpointRelease(ns, project, epName, epName+"-0")
	require.NoError(t, c.Create(ctx, old))
	stagingOn(t, c, old.Name)

	_, err := svc.Step(ctx, ns, epName, StepRequest{Environment: "staging"})
	var conflictErr *ConflictError
	require.ErrorAs(t, err, &conflictErr)
	assert.Contains(t, conflictErr.Msg, "which has no backend v2")
}

func TestStart_DryRunChecksThePlan(t *testing.T) {
	svc, _ := fixture(t, false)
	plan := canaryPlan()
	plan.Strategy, plan.Canary, plan.Candidates = openchoreov1alpha1.EndpointRolloutBlueGreen, nil, nil

	_, err := svc.Start(context.Background(), ns, epName, StartRequest{Rollout: plan, DryRun: true})
	var validationErr *services.ValidationError
	require.ErrorAs(t, err, &validationErr)
	assert.Contains(t, validationErr.Msg, "exactly one candidate")

	plan = canaryPlan()
	plan.Stable = "missing"
	_, err = svc.Start(context.Background(), ns, epName, StartRequest{Rollout: plan, DryRun: true})
	require.ErrorAs(t, err, &validationErr)
	assert.Contains(t, validationErr.Msg, "rollout.stable must name a backend")
}

func TestStart_BackendByNameKeepsItsComponent(t *testing.T) {
	ctx := context.Background()
	svc, c := fixture(t, false)
	byName := openchoreov1alpha1.EndpointBackendRef{
		Name: "v2", Kind: openchoreov1alpha1.EndpointBackendKindComponent, Weight: ptr(int32(0)),
	}

	_, err := svc.Start(ctx, ns, epName, StartRequest{Rollout: canaryPlan(), Backends: []openchoreov1alpha1.EndpointBackendRef{byName}})
	require.NoError(t, err)
	ep := &openchoreov1alpha1.Endpoint{}
	require.NoError(t, c.Get(ctx, client.ObjectKey{Namespace: ns, Name: epName}, ep))
	require.NotNil(t, ep.Spec.BackendRefs[1].Component)
	assert.Equal(t, "greeter-v2", ep.Spec.BackendRefs[1].Component.Name)
	assert.Nil(t, ep.Spec.BackendRefs[1].Match, "the rest is taken from the request")
}

func TestStart_RefusesAMatchTheControllerRefuses(t *testing.T) {
	svc, _ := fixture(t, false)
	v2 := candidate()
	v2.Match.Headers = append(v2.Match.Headers, openchoreov1alpha1.EndpointHeaderMatch{Name: "X-Canary", Value: "yes"})

	_, err := svc.Start(context.Background(), ns, epName, StartRequest{Rollout: canaryPlan(), Backends: []openchoreov1alpha1.EndpointBackendRef{v2}})
	var validationErr *services.ValidationError
	require.ErrorAs(t, err, &validationErr)
	assert.Contains(t, validationErr.Msg, `header "X-Canary" appears more than once`)
}

func TestFinish_RefusedWhileAStepIsBeingApplied(t *testing.T) {
	ctx := context.Background()
	svc, c := fixture(t, true)
	_, err := svc.Step(ctx, ns, epName, StepRequest{Environment: "dev"})
	require.NoError(t, err)
	serve(t, c, deployed("stable", 100), deployed("v2", 0))

	_, err = svc.Finish(ctx, ns, epName, FinishRequest{Keep: "stable"})
	var conflictErr *ConflictError
	require.ErrorAs(t, err, &conflictErr, "the binding asks for 5% even though it still routes 0%")
	assert.Contains(t, conflictErr.Msg, "v2 in dev still gets traffic")
}

func TestFinish_SavedWhenAMirrorCantBeCleared(t *testing.T) {
	ctx := context.Background()
	denied := false
	svc, c := interceptedFixture(t, true, func(store client.WithWatch) interceptor.Funcs {
		return interceptor.Funcs{
			Update: func(ctx context.Context, _ client.WithWatch, obj client.Object, opts ...client.UpdateOption) error {
				if _, ok := obj.(*openchoreov1alpha1.EndpointReleaseBinding); ok && denied {
					return apierrors.NewForbidden(schema.GroupResource{Resource: "endpointreleasebindings"}, obj.GetName(), errors.New("denied"))
				}
				return store.Update(ctx, obj, opts...)
			},
		}
	})
	asMirror(t, c)
	b := &openchoreov1alpha1.EndpointReleaseBinding{}
	require.NoError(t, c.Get(ctx, client.ObjectKey{Namespace: ns, Name: epName + "-dev"}, b))
	b.Spec.BackendOverrides = []openchoreov1alpha1.EndpointBackendOverride{
		{Name: "v2", Mirror: &openchoreov1alpha1.EndpointMirror{Percent: 50}},
	}
	require.NoError(t, c.Update(ctx, b))
	denied = true

	status, err := svc.Finish(ctx, ns, epName, FinishRequest{Keep: "stable"})
	require.NoError(t, err)
	assert.Nil(t, status.Rollout)
}

func TestStep_ABTestHasNoSteps(t *testing.T) {
	ctx := context.Background()
	svc, c := fixture(t, true)
	ep := &openchoreov1alpha1.Endpoint{}
	require.NoError(t, c.Get(ctx, client.ObjectKey{Namespace: ns, Name: epName}, ep))
	ep.Spec.Rollout.Strategy, ep.Spec.Rollout.Canary = openchoreov1alpha1.EndpointRolloutABTest, nil
	require.NoError(t, c.Update(ctx, ep))

	_, err := svc.Step(ctx, ns, epName, StepRequest{Environment: "dev"})
	var conflictErr *ConflictError
	require.ErrorAs(t, err, &conflictErr)
	assert.Contains(t, conflictErr.Msg, "an A/B test has no steps")

	_, err = svc.Abort(ctx, ns, epName, AbortRequest{Environment: "dev"})
	assert.NoError(t, err, "abort still returns weights set by hand to stable")
}

func TestStep_EnvironmentWithoutARelease(t *testing.T) {
	svc, c := fixture(t, true)
	stagingOn(t, c, "")

	_, err := svc.Step(context.Background(), ns, epName, StepRequest{Environment: "staging"})
	var conflictErr *ConflictError
	require.ErrorAs(t, err, &conflictErr)
	assert.Contains(t, conflictErr.Msg, "staging has no release yet")
}

func TestFinish_StartsOverWhenAStepLandsBeforeTheWrite(t *testing.T) {
	ctx := context.Background()
	// Finish lists the bindings twice to check it sees them all, then reads
	// them for its checks, which find v2 at 0. A step to 5% is saved right
	// after, before the Endpoint is, so the recheck starts finish over and it
	// is refused.
	svc, c := interceptedFixture(t, true, afterBindingList(t, 3, func(b *openchoreov1alpha1.EndpointReleaseBinding) {
		b.Spec.BackendOverrides = []openchoreov1alpha1.EndpointBackendOverride{
			{Name: "stable", Weight: ptr(int32(95))}, {Name: "v2", Weight: ptr(int32(5))},
		}
	}))
	serve(t, c, deployed("stable", 100), deployed("v2", 0))

	_, err := svc.Finish(ctx, ns, epName, FinishRequest{Keep: "stable"})
	var conflictErr *ConflictError
	require.ErrorAs(t, err, &conflictErr)
	assert.Contains(t, conflictErr.Msg, "v2 in dev still gets traffic")
	ep := &openchoreov1alpha1.Endpoint{}
	require.NoError(t, c.Get(ctx, client.ObjectKey{Namespace: ns, Name: epName}, ep))
	assert.NotNil(t, ep.Spec.Rollout, "the Endpoint keeps the rollout")
}

func TestStep_RefusedOnAReleaseThatRoutesElsewhere(t *testing.T) {
	ctx := context.Background()
	svc, c := fixture(t, true)
	ep := &openchoreov1alpha1.Endpoint{}
	require.NoError(t, c.Get(ctx, client.ObjectKey{Namespace: ns, Name: epName}, ep))
	ep.Spec.BackendRefs[1].Component = &openchoreov1alpha1.EndpointComponentBackend{
		Name: "greeter-v2", Endpoint: "http", ReleaseLine: "beta",
	}
	require.NoError(t, c.Update(ctx, ep))

	_, err := svc.Step(ctx, ns, epName, StepRequest{Environment: "dev"})
	var conflictErr *ConflictError
	require.ErrorAs(t, err, &conflictErr)
	assert.Contains(t, conflictErr.Msg, "v2 routes to greeter-v2/http there, not greeter-v2@beta/http")
	assert.Contains(t, conflictErr.Msg, "deploy the latest release there first")

	_, err = svc.Abort(ctx, ns, epName, AbortRequest{Environment: "dev"})
	require.NoError(t, err, "going back to step 0 only takes traffic off the candidate")
}

func TestStart_DropsAStaleOverrideOfTheCandidate(t *testing.T) {
	ctx := context.Background()
	svc, c := fixture(t, false)
	// dev runs a release from before v2, with a weight an earlier rollout of a
	// backend called v2 left behind, and its own weight for stable.
	rel := &openchoreov1alpha1.EndpointRelease{}
	require.NoError(t, c.Get(ctx, client.ObjectKey{Namespace: ns, Name: release}, rel))
	rel.Spec.BackendRefs = rel.Spec.BackendRefs[:1]
	require.NoError(t, c.Update(ctx, rel))
	b := &openchoreov1alpha1.EndpointReleaseBinding{}
	require.NoError(t, c.Get(ctx, client.ObjectKey{Namespace: ns, Name: epName + "-dev"}, b))
	b.Spec.BackendOverrides = []openchoreov1alpha1.EndpointBackendOverride{
		{Name: "stable", Weight: ptr(int32(7))}, {Name: "v2", Weight: ptr(int32(50))},
	}
	require.NoError(t, c.Update(ctx, b))

	_, err := svc.Start(ctx, ns, epName, StartRequest{Rollout: canaryPlan()})
	require.NoError(t, err)
	assert.Equal(t, map[string]int32{"stable": 7}, overridesOf(t, c), "only the stale candidate weight goes")
}

func TestStart_DryRunLeavesOverrides(t *testing.T) {
	ctx := context.Background()
	svc, c := fixture(t, false)
	b := &openchoreov1alpha1.EndpointReleaseBinding{}
	require.NoError(t, c.Get(ctx, client.ObjectKey{Namespace: ns, Name: epName + "-dev"}, b))
	b.Spec.EndpointRelease = ""
	b.Spec.BackendOverrides = []openchoreov1alpha1.EndpointBackendOverride{{Name: "v2", Weight: ptr(int32(50))}}
	require.NoError(t, c.Update(ctx, b))

	_, err := svc.Start(ctx, ns, epName, StartRequest{Rollout: canaryPlan(), DryRun: true})
	require.NoError(t, err)
	assert.Equal(t, map[string]int32{"v2": 50}, overridesOf(t, c))
}

func TestStart_BackendByNameKeepsItsReleaseLine(t *testing.T) {
	ctx := context.Background()
	svc, c := fixture(t, false)
	ep := &openchoreov1alpha1.Endpoint{}
	require.NoError(t, c.Get(ctx, client.ObjectKey{Namespace: ns, Name: epName}, ep))
	ep.Spec.BackendRefs[1].Component.ReleaseLine = "beta"
	require.NoError(t, c.Update(ctx, ep))
	byName := openchoreov1alpha1.EndpointBackendRef{Name: "v2", Weight: ptr(int32(0))}

	_, err := svc.Start(ctx, ns, epName, StartRequest{Rollout: canaryPlan(), Backends: []openchoreov1alpha1.EndpointBackendRef{byName}})
	require.NoError(t, err)
	require.NoError(t, c.Get(ctx, client.ObjectKey{Namespace: ns, Name: epName}, ep))
	assert.Equal(t, "beta", ep.Spec.BackendRefs[1].Component.ReleaseLine)
}

func TestFinish_RefusedWhileABindingCantRender(t *testing.T) {
	ctx := context.Background()
	svc, c := fixture(t, true)
	// Both overrides at 0: the controller keeps the old route, stable at 100,
	// but reports the weights it was asked for.
	b := &openchoreov1alpha1.EndpointReleaseBinding{}
	require.NoError(t, c.Get(ctx, client.ObjectKey{Namespace: ns, Name: epName + "-dev"}, b))
	b.Spec.BackendOverrides = []openchoreov1alpha1.EndpointBackendOverride{
		{Name: "stable", Weight: ptr(int32(0))}, {Name: "v2", Weight: ptr(int32(0))},
	}
	b.Status.Backends = []openchoreov1alpha1.ResolvedEndpointBackend{deployed("stable", 0), deployed("v2", 0)}
	b.Status.Conditions = []metav1.Condition{{
		Type: "BackendsResolved", Status: metav1.ConditionFalse, Reason: "NoWeightedBackend",
		Message: `No backend has a weight above 0 in environment "dev"`, LastTransitionTime: metav1.Now(),
	}}
	require.NoError(t, c.Update(ctx, b))

	_, err := svc.Finish(ctx, ns, epName, FinishRequest{Keep: "v2"})
	var conflictErr *ConflictError
	require.ErrorAs(t, err, &conflictErr)
	assert.Contains(t, conflictErr.Msg, "dev isn't routing its latest change (No backend has a weight above 0")
	ep := &openchoreov1alpha1.Endpoint{}
	require.NoError(t, c.Get(ctx, client.ObjectKey{Namespace: ns, Name: epName}, ep))
	assert.Len(t, ep.Spec.BackendRefs, 2, "stable stays while it may still get traffic")
}

func TestFinish_RefusedWhileABindingFailsToRender(t *testing.T) {
	ctx := context.Background()
	svc, c := fixture(t, true)
	// The step to v2 resolved, but rendering it failed: the controller keeps
	// the old route, stable at 100, and reports the weights it was asked for.
	b := &openchoreov1alpha1.EndpointReleaseBinding{}
	require.NoError(t, c.Get(ctx, client.ObjectKey{Namespace: ns, Name: epName + "-dev"}, b))
	b.Spec.BackendOverrides = []openchoreov1alpha1.EndpointBackendOverride{
		{Name: "stable", Weight: ptr(int32(0))}, {Name: "v2", Weight: ptr(int32(100))},
	}
	b.Status.Backends = []openchoreov1alpha1.ResolvedEndpointBackend{deployed("stable", 0), deployed("v2", 100)}
	b.Status.Conditions = []metav1.Condition{
		{Type: "BackendsResolved", Status: metav1.ConditionTrue, Reason: "BackendsResolved", LastTransitionTime: metav1.Now()},
		{Type: "Synced", Status: metav1.ConditionFalse, Reason: "RenderingFailed",
			Message: "template evaluation failed", LastTransitionTime: metav1.Now()},
	}
	require.NoError(t, c.Update(ctx, b))

	_, err := svc.Finish(ctx, ns, epName, FinishRequest{Keep: "v2"})
	var conflictErr *ConflictError
	require.ErrorAs(t, err, &conflictErr)
	assert.Contains(t, conflictErr.Msg, "dev isn't routing its latest change (template evaluation failed)")
}

func TestFinish_NotBlockedByAnEnvironmentWithoutARelease(t *testing.T) {
	ctx := context.Background()
	svc, c := fixture(t, true)
	stagingOn(t, c, "")
	b := &openchoreov1alpha1.EndpointReleaseBinding{}
	require.NoError(t, c.Get(ctx, client.ObjectKey{Namespace: ns, Name: epName + "-staging"}, b))
	b.Status.Conditions = []metav1.Condition{{Type: "Synced", Status: metav1.ConditionFalse,
		Reason: "EndpointReleaseNotSet", Message: "spec.endpointRelease is unset", LastTransitionTime: metav1.Now()}}
	require.NoError(t, c.Update(ctx, b))

	_, err := svc.Finish(ctx, ns, epName, FinishRequest{Keep: "stable"})
	assert.NoError(t, err, "staging routes nothing yet")
}

// TestStep_RefusedWhereTheWeightsCantTakeIt covers environments whose own
// weights leave a step nothing exact to set: stable turned off next to
// legacy, or legacy so heavy that stable's share can't be split.
func TestStep_RefusedWhereTheWeightsCantTakeIt(t *testing.T) {
	cases := map[string]struct {
		overrides []openchoreov1alpha1.EndpointBackendOverride
		want      string
	}{
		"the rollout turned off": {
			[]openchoreov1alpha1.EndpointBackendOverride{{Name: "stable", Weight: ptr(int32(0))}},
			"in dev, stable and the candidates get no traffic",
		},
		"too little left for the step": {
			[]openchoreov1alpha1.EndpointBackendOverride{
				{Name: "stable", Weight: ptr(int32(1))}, {Name: "legacy", Weight: ptr(int32(999_999))},
			},
			"in dev, the other backends' weights leave stable and the candidates too little traffic",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			svc, c := fixture(t, true)
			ep := &openchoreov1alpha1.Endpoint{}
			require.NoError(t, c.Get(ctx, client.ObjectKey{Namespace: ns, Name: epName}, ep))
			legacy := candidate()
			legacy.Name, legacy.Match, legacy.Weight = "legacy", nil, ptr(int32(40))
			ep.Spec.BackendRefs = append(ep.Spec.BackendRefs, legacy)
			require.NoError(t, c.Update(ctx, ep))
			rel := &openchoreov1alpha1.EndpointRelease{}
			require.NoError(t, c.Get(ctx, client.ObjectKey{Namespace: ns, Name: release}, rel))
			rel.Spec.BackendRefs = ep.Spec.BackendRefs
			require.NoError(t, c.Update(ctx, rel))
			b := &openchoreov1alpha1.EndpointReleaseBinding{}
			require.NoError(t, c.Get(ctx, client.ObjectKey{Namespace: ns, Name: epName + "-dev"}, b))
			b.Spec.BackendOverrides = tc.overrides
			require.NoError(t, c.Update(ctx, b))

			_, err := svc.Step(ctx, ns, epName, StepRequest{Environment: "dev"})
			var conflictErr *ConflictError
			require.ErrorAs(t, err, &conflictErr)
			assert.Contains(t, conflictErr.Msg, tc.want)
		})
	}
}

// A plan edited while a step runs isn't stepped by the old one: the save is
// refused and the step starts over from the new plan.
func TestStep_FollowsAPlanEditedJustBeforeTheSave(t *testing.T) {
	edited := false
	svc, c := interceptedFixture(t, true, func(store client.WithWatch) interceptor.Funcs {
		return interceptor.Funcs{
			Get: func(ctx context.Context, _ client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
				if _, ok := obj.(*openchoreov1alpha1.EndpointReleaseBinding); ok && !edited {
					edited = true
					ep := &openchoreov1alpha1.Endpoint{}
					require.NoError(t, store.Get(ctx, client.ObjectKey{Namespace: ns, Name: epName}, ep))
					ep.Spec.Rollout.Canary.Steps[0].Backends[0].Percent = 10
					require.NoError(t, store.Update(ctx, ep))
				}
				return store.Get(ctx, key, obj, opts...)
			},
		}
	})

	status, err := svc.Step(context.Background(), ns, epName, StepRequest{Environment: "dev"})
	require.NoError(t, err)
	assert.Equal(t, map[string]int32{"stable": 90, "v2": 10}, overridesOf(t, c), "the step follows the edited plan")
	assert.Equal(t, 1, *status.Environments[0].Step, "and reports its stage against it")
}

// A promotion that lands after the step checked the binding, right before
// it is saved, isn't overwritten: the save is refused and the step starts
// over from it.
func TestStep_KeepsAReleasePinnedJustBeforeTheSave(t *testing.T) {
	gets := 0
	svc, c := interceptedFixture(t, true, func(store client.WithWatch) interceptor.Funcs {
		return interceptor.Funcs{
			Get: func(ctx context.Context, _ client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
				if _, ok := obj.(*openchoreov1alpha1.EndpointReleaseBinding); ok {
					if gets++; gets == 2 {
						b := &openchoreov1alpha1.EndpointReleaseBinding{}
						require.NoError(t, store.Get(ctx, key, b))
						b.Spec.EndpointRelease = release + "-next"
						require.NoError(t, store.Update(ctx, b))
					}
				}
				return store.Get(ctx, key, obj, opts...)
			},
		}
	})

	_, err := svc.Step(context.Background(), ns, epName, StepRequest{Environment: "dev"})
	require.NoError(t, err)
	b := &openchoreov1alpha1.EndpointReleaseBinding{}
	require.NoError(t, c.Get(context.Background(), client.ObjectKey{Namespace: ns, Name: epName + "-dev"}, b))
	assert.Equal(t, release+"-next", b.Spec.EndpointRelease, "the step doesn't undo the promotion")
	assert.Equal(t, map[string]int32{"stable": 95, "v2": 5}, overridesOf(t, c))
}

func TestFinish_KeepsAMirrorShareThePinnedReleaseStillHas(t *testing.T) {
	ctx := context.Background()
	svc, c := fixture(t, true)
	asMirror(t, c)
	// dev runs a release that copies every request to v2; staging runs one
	// without v2.
	pinned := &openchoreov1alpha1.EndpointRelease{}
	require.NoError(t, c.Get(ctx, client.ObjectKey{Namespace: ns, Name: release}, pinned))
	pinned.Spec.BackendRefs[1].Match = nil
	pinned.Spec.BackendRefs[1].Mirror = &openchoreov1alpha1.EndpointMirror{Percent: 100}
	require.NoError(t, c.Update(ctx, pinned))
	next := &openchoreov1alpha1.EndpointRelease{}
	require.NoError(t, c.Get(ctx, client.ObjectKey{Namespace: ns, Name: release + "-next"}, next))
	next.Spec.BackendRefs = next.Spec.BackendRefs[:1]
	require.NoError(t, c.Update(ctx, next))
	stagingOn(t, c, release+"-next")

	off := []openchoreov1alpha1.EndpointBackendOverride{{Name: "v2", Mirror: &openchoreov1alpha1.EndpointMirror{Percent: 0}}}
	for _, name := range []string{epName + "-dev", epName + "-staging"} {
		b := &openchoreov1alpha1.EndpointReleaseBinding{}
		require.NoError(t, c.Get(ctx, client.ObjectKey{Namespace: ns, Name: name}, b))
		b.Spec.BackendOverrides = off
		require.NoError(t, c.Update(ctx, b))
	}

	_, err := svc.Finish(ctx, ns, epName, FinishRequest{Keep: "stable"})
	require.NoError(t, err)

	b := &openchoreov1alpha1.EndpointReleaseBinding{}
	require.NoError(t, c.Get(ctx, client.ObjectKey{Namespace: ns, Name: epName + "-dev"}, b))
	assert.Equal(t, off, b.Spec.BackendOverrides, "the 0% holds the pinned release's 100% down")
	assert.Empty(t, stagingOverrides(t, c), "a release without v2 has nothing for it to hold")
}

func TestFinish_ABTestKeepingTheCandidateClearsItsAbortedWeight(t *testing.T) {
	ctx := context.Background()
	svc, c := fixture(t, true)
	asABTest(t, c)
	_, err := svc.Abort(ctx, ns, epName, AbortRequest{Environment: "dev"})
	require.NoError(t, err)
	// staging was shipped to everyone by hand.
	stagingOn(t, c, release)
	shipped := []openchoreov1alpha1.EndpointBackendOverride{
		{Name: "stable", Weight: ptr(int32(0))}, {Name: "v2", Weight: ptr(int32(100))},
	}
	b := &openchoreov1alpha1.EndpointReleaseBinding{}
	require.NoError(t, c.Get(ctx, client.ObjectKey{Namespace: ns, Name: epName + "-staging"}, b))
	b.Spec.BackendOverrides = shipped
	require.NoError(t, c.Update(ctx, b))

	_, err = svc.Finish(ctx, ns, epName, FinishRequest{Keep: "v2", DryRun: true})
	require.NoError(t, err)
	assert.Equal(t, map[string]int32{"stable": 100, "v2": 0}, overridesOf(t, c), "a dry run changes nothing")

	_, err = svc.Finish(ctx, ns, epName, FinishRequest{Keep: "v2"})
	require.NoError(t, err)
	assert.Equal(t, map[string]int32{"stable": 100}, overridesOf(t, c),
		"v2 takes stable's weight in the new release; stable's stays for the pinned one")
	assert.Equal(t, shipped, stagingOverrides(t, c), "what staging serves stays")
}

// asABTest turns the stored plan into an A/B test of v2, matched by a cookie
// on the Endpoint and both releases.
func asABTest(t *testing.T, c client.Client) {
	t.Helper()
	ctx := context.Background()
	cookie := &openchoreov1alpha1.EndpointMatch{
		Cookies: []openchoreov1alpha1.EndpointCookieMatch{{Name: "variant", Value: "v2"}},
	}
	ep := &openchoreov1alpha1.Endpoint{}
	require.NoError(t, c.Get(ctx, client.ObjectKey{Namespace: ns, Name: epName}, ep))
	ep.Spec.Rollout.Strategy, ep.Spec.Rollout.Canary = openchoreov1alpha1.EndpointRolloutABTest, nil
	ep.Spec.BackendRefs[1].Match = cookie
	require.NoError(t, endpointrollout.Validate(ep.Spec.Rollout, ep.Spec.BackendRefs))
	require.NoError(t, c.Update(ctx, ep))
	for _, name := range []string{release, release + "-next"} {
		rel := &openchoreov1alpha1.EndpointRelease{}
		require.NoError(t, c.Get(ctx, client.ObjectKey{Namespace: ns, Name: name}, rel))
		rel.Spec.BackendRefs[1].Match = cookie
		require.NoError(t, c.Update(ctx, rel))
	}
}

func TestFinish_ABTestKeepingStableRefusedWhereTheCandidateWasShipped(t *testing.T) {
	ctx := context.Background()
	svc, c := fixture(t, true)
	asABTest(t, c)
	// staging was shipped to everyone by hand.
	stagingOn(t, c, release)
	b := &openchoreov1alpha1.EndpointReleaseBinding{}
	require.NoError(t, c.Get(ctx, client.ObjectKey{Namespace: ns, Name: epName + "-staging"}, b))
	b.Spec.BackendOverrides = []openchoreov1alpha1.EndpointBackendOverride{
		{Name: "stable", Weight: ptr(int32(0))}, {Name: "v2", Weight: ptr(int32(100))},
	}
	require.NoError(t, c.Update(ctx, b))

	for _, dryRun := range []bool{true, false} {
		_, err := svc.Finish(ctx, ns, epName, FinishRequest{Keep: "stable", DryRun: dryRun})
		var conflictErr *ConflictError
		require.ErrorAs(t, err, &conflictErr)
		assert.Contains(t, conflictErr.Msg, "v2 serves everyone in staging, where stable gets no traffic; abort there first")
	}
	ep := &openchoreov1alpha1.Endpoint{}
	require.NoError(t, c.Get(ctx, client.ObjectKey{Namespace: ns, Name: epName}, ep))
	assert.NotNil(t, ep.Spec.Rollout, "nothing is finished")

	_, err := svc.Abort(ctx, ns, epName, AbortRequest{Environment: "staging"})
	require.NoError(t, err)
	_, err = svc.Finish(ctx, ns, epName, FinishRequest{Keep: "stable"})
	assert.NoError(t, err, "after the abort stable has staging's traffic again")
}

func TestFinish_ABTestRefusedWhileABindingCantRender(t *testing.T) {
	ctx := context.Background()
	svc, c := fixture(t, true)
	asABTest(t, c)
	b := &openchoreov1alpha1.EndpointReleaseBinding{}
	require.NoError(t, c.Get(ctx, client.ObjectKey{Namespace: ns, Name: epName + "-dev"}, b))
	b.Status.Conditions = []metav1.Condition{{Type: "Synced", Status: metav1.ConditionFalse,
		Reason: "RenderingFailed", Message: "template evaluation failed", LastTransitionTime: metav1.Now()}}
	require.NoError(t, c.Update(ctx, b))

	_, err := svc.Finish(ctx, ns, epName, FinishRequest{Keep: "v2"})
	var conflictErr *ConflictError
	require.ErrorAs(t, err, &conflictErr)
	assert.Contains(t, conflictErr.Msg, "dev isn't routing its latest change")
}

// pinnedWithWeight pins staging to a release where v2 has weight 100, as one
// cut before the rollout could have it, with an override holding it at 0.
func pinnedWithWeight(t *testing.T, c client.Client) {
	t.Helper()
	ctx := context.Background()
	old := &openchoreov1alpha1.EndpointRelease{}
	require.NoError(t, c.Get(ctx, client.ObjectKey{Namespace: ns, Name: release + "-next"}, old))
	old.Spec.BackendRefs[1].Weight = ptr(int32(100))
	require.NoError(t, c.Update(ctx, old))
	stagingOn(t, c, release+"-next")
	b := &openchoreov1alpha1.EndpointReleaseBinding{}
	require.NoError(t, c.Get(ctx, client.ObjectKey{Namespace: ns, Name: epName + "-staging"}, b))
	b.Spec.BackendOverrides = []openchoreov1alpha1.EndpointBackendOverride{{Name: "v2", Weight: ptr(int32(0))}}
	require.NoError(t, c.Update(ctx, b))
}

func TestFinish_ABTestKeepingTheCandidateRefusedWhereThePinnedReleaseWeighsIt(t *testing.T) {
	ctx := context.Background()
	svc, c := fixture(t, true)
	asABTest(t, c)
	pinnedWithWeight(t, c)

	for _, dryRun := range []bool{true, false} {
		_, err := svc.Finish(ctx, ns, epName, FinishRequest{Keep: "v2", DryRun: dryRun})
		var conflictErr *ConflictError
		require.ErrorAs(t, err, &conflictErr)
		assert.Contains(t, conflictErr.Msg, "staging runs release "+release+"-next, where v2 has weight 100")
	}
	assert.Equal(t, []openchoreov1alpha1.EndpointBackendOverride{{Name: "v2", Weight: ptr(int32(0))}},
		stagingOverrides(t, c), "the 0 still holds v2 off staging")
	ep := &openchoreov1alpha1.Endpoint{}
	require.NoError(t, c.Get(ctx, client.ObjectKey{Namespace: ns, Name: epName}, ep))
	assert.NotNil(t, ep.Spec.Rollout, "nothing is finished")
}

// endpointSaves denies the Endpoint's saves while deny is set.
type endpointSaves struct{ deny, denied bool }

func (e *endpointSaves) funcs(store client.WithWatch) interceptor.Funcs {
	return interceptor.Funcs{
		Update: func(ctx context.Context, _ client.WithWatch, obj client.Object, opts ...client.UpdateOption) error {
			if _, ok := obj.(*openchoreov1alpha1.Endpoint); ok && e.deny {
				e.denied = true
				return apierrors.NewForbidden(schema.GroupResource{Resource: "endpoints"}, obj.GetName(), errors.New("denied"))
			}
			return store.Update(ctx, obj, opts...)
		},
	}
}

// wasDenied checks a call failed on the denied save, not on a check before it.
func (e *endpointSaves) wasDenied(t *testing.T, err error) {
	t.Helper()
	require.True(t, e.denied, "the call reached the Endpoint save")
	require.True(t, apierrors.IsForbidden(err), "the call returns the save's error, got %v", err)
}

func TestFinish_ABTestFailedSaveLeavesRoutingAsItWas(t *testing.T) {
	ctx := context.Background()
	saves := &endpointSaves{}
	svc, c := interceptedFixture(t, true, saves.funcs)
	asABTest(t, c)
	_, err := svc.Abort(ctx, ns, epName, AbortRequest{Environment: "dev"})
	require.NoError(t, err)
	saves.deny = true

	_, err = svc.Finish(ctx, ns, epName, FinishRequest{Keep: "v2"})
	saves.wasDenied(t, err)
	assert.Equal(t, map[string]int32{"stable": 100}, overridesOf(t, c),
		"v2's 0 is gone, but dev's release gives v2 0 too, so dev routes as before")
	ep := &openchoreov1alpha1.Endpoint{}
	require.NoError(t, c.Get(ctx, client.ObjectKey{Namespace: ns, Name: epName}, ep))
	assert.NotNil(t, ep.Spec.Rollout, "the rollout is still planned")
}

func TestFinish_ABTestRefusedOnASplitSetByHand(t *testing.T) {
	ctx := context.Background()
	svc, c := fixture(t, true)
	asABTest(t, c)
	withLegacy(t, c)
	// dev splits by hand: v2 has weight there besides its cookie's requests.
	split := []openchoreov1alpha1.EndpointBackendOverride{
		{Name: "stable", Weight: ptr(int32(95))}, {Name: "v2", Weight: ptr(int32(5))},
	}
	b := &openchoreov1alpha1.EndpointReleaseBinding{}
	require.NoError(t, c.Get(ctx, client.ObjectKey{Namespace: ns, Name: epName + "-dev"}, b))
	b.Spec.BackendOverrides = split
	require.NoError(t, c.Update(ctx, b))

	// Keeping v2, its 5 would mask the 100 the finished release gives it, so
	// it would get 5% instead of the pair's half against legacy.
	for _, keep := range []string{"v2", "stable"} {
		_, err := svc.Finish(ctx, ns, epName, FinishRequest{Keep: keep})
		var conflictErr *ConflictError
		require.ErrorAs(t, err, &conflictErr)
		assert.Contains(t, conflictErr.Msg, "stable and v2 both have weight in dev, set by hand")
	}
	assert.Equal(t, split, devOverrides(t, c))
	ep := &openchoreov1alpha1.Endpoint{}
	require.NoError(t, c.Get(ctx, client.ObjectKey{Namespace: ns, Name: epName}, ep))
	assert.NotNil(t, ep.Spec.Rollout, "nothing is finished")

	_, err := svc.Abort(ctx, ns, epName, AbortRequest{Environment: "dev"})
	require.NoError(t, err)
	_, err = svc.Finish(ctx, ns, epName, FinishRequest{Keep: "v2"})
	var conflictErr *ConflictError
	require.ErrorAs(t, err, &conflictErr, "the abort wrote stable's weight next to legacy's, not the 1 the release gives v2")
	assert.Contains(t, conflictErr.Msg, "move all of stable's weight to v2 there first")

	// dev ships v2 by hand.
	w := overridesOf(t, c)
	require.NoError(t, c.Get(ctx, client.ObjectKey{Namespace: ns, Name: epName + "-dev"}, b))
	b.Spec.BackendOverrides = []openchoreov1alpha1.EndpointBackendOverride{
		{Name: "stable", Weight: ptr(int32(0))}, {Name: "v2", Weight: ptr(w["stable"])}, {Name: "legacy", Weight: ptr(w["legacy"])},
	}
	require.NoError(t, c.Update(ctx, b))
	_, err = svc.Finish(ctx, ns, epName, FinishRequest{Keep: "v2"})
	assert.NoError(t, err, "shipped, v2's own weight carries over")
}

func TestStep_RefusedWhereThePinnedTypeCantRenderTheShare(t *testing.T) {
	ctx := context.Background()
	svc, c := fixture(t, true)
	asMirror(t, c)
	// dev's release mirrors to v2, but its type snapshot declares Mirror
	// without MirrorPercent, so only 0% and 100% render.
	rel := &openchoreov1alpha1.EndpointRelease{}
	require.NoError(t, c.Get(ctx, client.ObjectKey{Namespace: ns, Name: release}, rel))
	rel.Spec.BackendRefs[1].Match = nil
	rel.Spec.BackendRefs[1].Mirror = &openchoreov1alpha1.EndpointMirror{Percent: 0}
	rel.Spec.EndpointType.Spec.Capabilities = []openchoreov1alpha1.EndpointCapability{openchoreov1alpha1.EndpointCapabilityMirror}
	require.NoError(t, c.Update(ctx, rel))
	before := &openchoreov1alpha1.EndpointReleaseBinding{}
	require.NoError(t, c.Get(ctx, client.ObjectKey{Namespace: ns, Name: epName + "-dev"}, before))

	_, err := svc.Step(ctx, ns, epName, StepRequest{Environment: "dev"})
	var conflictErr *ConflictError
	require.ErrorAs(t, err, &conflictErr)
	assert.Contains(t, conflictErr.Msg, "in dev, release "+release+"'s EndpointType can't render this step")
	assert.Contains(t, conflictErr.Msg, "MirrorPercent")
	after := &openchoreov1alpha1.EndpointReleaseBinding{}
	require.NoError(t, c.Get(ctx, client.ObjectKey{Namespace: ns, Name: epName + "-dev"}, after))
	assert.Equal(t, before.ResourceVersion, after.ResourceVersion, "the binding isn't written")

	_, err = svc.Step(ctx, ns, epName, StepRequest{Environment: "dev", To: ptr(2)})
	assert.NoError(t, err, "copying every request needs only Mirror")
}

func TestStep_ReportsAStepThatWasSaved(t *testing.T) {
	ctx := context.Background()
	written := false
	svc, c := interceptedFixture(t, true, func(store client.WithWatch) interceptor.Funcs {
		return interceptor.Funcs{
			Get: func(ctx context.Context, _ client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
				if _, ok := obj.(*openchoreov1alpha1.EndpointRelease); ok && written {
					return errors.New("the API server went away")
				}
				return store.Get(ctx, key, obj, opts...)
			},
			Update: func(ctx context.Context, _ client.WithWatch, obj client.Object, opts ...client.UpdateOption) error {
				err := store.Update(ctx, obj, opts...)
				_, binding := obj.(*openchoreov1alpha1.EndpointReleaseBinding)
				written = written || (binding && err == nil)
				return err
			},
		}
	})
	// staging runs a release no read has needed before the status.
	stagingOn(t, c, release+"-next")

	status, err := svc.Step(ctx, ns, epName, StepRequest{Environment: "dev"})
	require.NoError(t, err, "the step was saved, so it reports success")
	assert.Len(t, status.Environments, 2)
	assert.Equal(t, map[string]int32{"stable": 95, "v2": 5}, overridesOf(t, c))
}

// withLegacy gives the Endpoint and its release a third backend, legacy, at
// weight 40, outside the rollout.
func withLegacy(t *testing.T, c client.Client) {
	t.Helper()
	ctx := context.Background()
	ep := &openchoreov1alpha1.Endpoint{}
	require.NoError(t, c.Get(ctx, client.ObjectKey{Namespace: ns, Name: epName}, ep))
	legacy := candidate()
	legacy.Name, legacy.Match, legacy.Weight = "legacy", nil, ptr(int32(40))
	ep.Spec.BackendRefs = append(ep.Spec.BackendRefs, legacy)
	require.NoError(t, c.Update(ctx, ep))
	rel := &openchoreov1alpha1.EndpointRelease{}
	require.NoError(t, c.Get(ctx, client.ObjectKey{Namespace: ns, Name: release}, rel))
	rel.Spec.BackendRefs = ep.Spec.BackendRefs
	require.NoError(t, c.Update(ctx, rel))
}

func devOverrides(t *testing.T, c client.Client) []openchoreov1alpha1.EndpointBackendOverride {
	t.Helper()
	b := &openchoreov1alpha1.EndpointReleaseBinding{}
	require.NoError(t, c.Get(context.Background(), client.ObjectKey{Namespace: ns, Name: epName + "-dev"}, b))
	return b.Spec.BackendOverrides
}

func TestFinish_AbortedCanaryLeavesNoOverrides(t *testing.T) {
	ctx := context.Background()
	svc, c := fixture(t, true)
	_, err := svc.Step(ctx, ns, epName, StepRequest{Environment: "dev"})
	require.NoError(t, err)
	_, err = svc.Abort(ctx, ns, epName, AbortRequest{Environment: "dev"})
	require.NoError(t, err)
	require.Equal(t, map[string]int32{"stable": 100, "v2": 0}, overridesOf(t, c))

	_, err = svc.Finish(ctx, ns, epName, FinishRequest{Keep: "stable", DryRun: true})
	require.NoError(t, err)
	assert.Len(t, devOverrides(t, c), 2, "a dry run changes nothing")

	_, err = svc.Finish(ctx, ns, epName, FinishRequest{Keep: "stable"})
	require.NoError(t, err)
	assert.Empty(t, devOverrides(t, c),
		"v2's 0 is its release weight too, and stable is then the only backend with weight")
}

func TestFinish_KeepsTheSplitWithAThirdBackend(t *testing.T) {
	ctx := context.Background()
	svc, c := fixture(t, true)
	withLegacy(t, c)
	_, err := svc.Step(ctx, ns, epName, StepRequest{Environment: "dev"})
	require.NoError(t, err)
	_, err = svc.Abort(ctx, ns, epName, AbortRequest{Environment: "dev"})
	require.NoError(t, err)
	before := overridesOf(t, c)

	_, err = svc.Finish(ctx, ns, epName, FinishRequest{Keep: "stable"})
	require.NoError(t, err)
	after := overridesOf(t, c)
	assert.NotContains(t, after, "v2", "v2's 0 goes")
	assert.Equal(t, before["stable"], after["stable"], "stable's weight stays: it sets its share against legacy")
	assert.Equal(t, before["legacy"], after["legacy"])
}

func TestFinish_KeepingTheCandidateKeepsWhatThePinnedReleaseNeeds(t *testing.T) {
	ctx := context.Background()
	svc, c := fixture(t, true)
	_, err := svc.Step(ctx, ns, epName, StepRequest{Environment: "dev", To: ptr(2)})
	require.NoError(t, err)
	require.Equal(t, map[string]int32{"stable": 0, "v2": 100}, overridesOf(t, c))
	serve(t, c, deployed("stable", 0), deployed("v2", 100))

	_, err = svc.Finish(ctx, ns, epName, FinishRequest{Keep: "v2"})
	require.NoError(t, err)
	assert.Equal(t, map[string]int32{"stable": 0, "v2": 100}, overridesOf(t, c),
		"dev still runs the rollout's release, where stable's 0 and v2's 100 are what route it")
}

func TestStep_RefusedWhereTheBindingDoesntReportTheBackend(t *testing.T) {
	ctx := context.Background()
	svc, c := fixture(t, true)
	// dev hasn't reconciled the release with v2 yet.
	serve(t, c, deployed("stable", 100))

	_, err := svc.Step(ctx, ns, epName, StepRequest{Environment: "dev"})
	var conflictErr *ConflictError
	require.ErrorAs(t, err, &conflictErr)
	assert.Contains(t, conflictErr.Msg, "v2, which dev doesn't report yet")
	assert.Empty(t, overridesOf(t, c))

	_, err = svc.Abort(ctx, ns, epName, AbortRequest{Environment: "dev"})
	assert.NoError(t, err, "a step that sends v2 nothing still goes")
}

func TestFinish_KeepingTheCandidateRefusedWhereItIsntDeployed(t *testing.T) {
	ctx := context.Background()
	svc, c := fixture(t, true)
	// An A/B test may finish keeping v2 while stable still has traffic, so
	// nothing else checks v2 can take it.
	asABTest(t, c)
	stagingOn(t, c, release)
	b := &openchoreov1alpha1.EndpointReleaseBinding{}
	require.NoError(t, c.Get(ctx, client.ObjectKey{Namespace: ns, Name: epName + "-staging"}, b))
	b.Status.Backends = []openchoreov1alpha1.ResolvedEndpointBackend{deployed("stable", 100), gone("v2", 0)}
	require.NoError(t, c.Update(ctx, b))

	for _, dryRun := range []bool{true, false} {
		_, err := svc.Finish(ctx, ns, epName, FinishRequest{Keep: "v2", DryRun: dryRun})
		var conflictErr *ConflictError
		require.ErrorAs(t, err, &conflictErr)
		assert.Contains(t, conflictErr.Msg, "v2 isn't deployed in staging")
	}

	_, err := svc.Finish(ctx, ns, epName, FinishRequest{Keep: "stable"})
	assert.NoError(t, err, "v2 not being deployed doesn't stop it leaving")
}

func TestStart_ReturnsTheSavedEndpointWhenItsStatusCantBeRead(t *testing.T) {
	saved := false
	svc, c := interceptedFixture(t, false, func(store client.WithWatch) interceptor.Funcs {
		return interceptor.Funcs{
			Update: func(ctx context.Context, _ client.WithWatch, obj client.Object, opts ...client.UpdateOption) error {
				err := store.Update(ctx, obj, opts...)
				_, saved = obj.(*openchoreov1alpha1.Endpoint)
				return err
			},
			List: func(ctx context.Context, _ client.WithWatch, list client.ObjectList, opts ...client.ListOption) error {
				if saved {
					return errors.New("list failed")
				}
				return store.List(ctx, list, opts...)
			},
		}
	})

	status, err := svc.Start(context.Background(), ns, epName, StartRequest{Rollout: canaryPlan()})
	require.NoError(t, err, "the plan is saved, so the call succeeded")
	require.NotNil(t, status.Change)
	require.NotNil(t, status.Change.Endpoint)
	assert.Equal(t, "stable", status.Change.Endpoint.Rollout.Stable, "the saved Endpoint, for occ -o yaml")
	ep := &openchoreov1alpha1.Endpoint{}
	require.NoError(t, c.Get(context.Background(), client.ObjectKey{Namespace: ns, Name: epName}, ep))
	assert.NotNil(t, ep.Spec.Rollout)
}

func TestFinish_KeepsStablesZeroWhereNothingHasWeight(t *testing.T) {
	ctx := context.Background()
	svc, c := fixture(t, true)
	// dev was switched off by hand; its route stays as it was until a
	// backend gets weight again.
	b := &openchoreov1alpha1.EndpointReleaseBinding{}
	require.NoError(t, c.Get(ctx, client.ObjectKey{Namespace: ns, Name: epName + "-dev"}, b))
	b.Spec.BackendOverrides = []openchoreov1alpha1.EndpointBackendOverride{
		{Name: "stable", Weight: ptr(int32(0))}, {Name: "v2", Weight: ptr(int32(0))},
	}
	require.NoError(t, c.Update(ctx, b))
	serve(t, c, deployed("stable", 0), deployed("v2", 0))

	_, err := svc.Finish(ctx, ns, epName, FinishRequest{Keep: "stable"})
	require.NoError(t, err)
	assert.Equal(t, []openchoreov1alpha1.EndpointBackendOverride{{Name: "stable", Weight: ptr(int32(0))}}, devOverrides(t, c),
		"dropping stable's 0 would send it traffic dev doesn't route now")
}

func TestStart_RefusedWhereTheCandidateAlreadyGetsTraffic(t *testing.T) {
	ctx := context.Background()
	svc, c := fixture(t, false)
	b := &openchoreov1alpha1.EndpointReleaseBinding{}
	require.NoError(t, c.Get(ctx, client.ObjectKey{Namespace: ns, Name: epName + "-dev"}, b))
	b.Spec.BackendOverrides = []openchoreov1alpha1.EndpointBackendOverride{{Name: "v2", Weight: ptr(int32(10))}}
	require.NoError(t, c.Update(ctx, b))

	for _, dryRun := range []bool{true, false} {
		_, err := svc.Start(ctx, ns, epName, StartRequest{Rollout: canaryPlan(), DryRun: dryRun})
		var conflictErr *ConflictError
		require.ErrorAs(t, err, &conflictErr)
		assert.Contains(t, conflictErr.Msg, "v2 already gets traffic in dev")
	}
	assert.Equal(t, map[string]int32{"v2": 10}, overridesOf(t, c), "the override isn't removed behind the caller's back")

	require.NoError(t, c.Get(ctx, client.ObjectKey{Namespace: ns, Name: epName + "-dev"}, b))
	b.Spec.BackendOverrides = nil
	require.NoError(t, c.Update(ctx, b))
	_, err := svc.Start(ctx, ns, epName, StartRequest{Rollout: canaryPlan()})
	assert.NoError(t, err, "v2's header match alone doesn't count")
}

func TestStep_RefusedWhereStableIsUnresolvedByItsReleaseWeight(t *testing.T) {
	ctx := context.Background()
	svc, c := fixture(t, true)
	asMirror(t, c)
	rel := &openchoreov1alpha1.EndpointRelease{}
	require.NoError(t, c.Get(ctx, client.ObjectKey{Namespace: ns, Name: release}, rel))
	rel.Spec.BackendRefs[1].Match = nil
	rel.Spec.BackendRefs[1].Mirror = &openchoreov1alpha1.EndpointMirror{Percent: 0}
	require.NoError(t, c.Update(ctx, rel))
	serve(t, c, gone("stable", 1), deployed("v2", 0))

	_, err := svc.Step(ctx, ns, epName, StepRequest{Environment: "dev"})
	var conflictErr *ConflictError
	require.ErrorAs(t, err, &conflictErr, "the mirror step sets only v2's share; stable keeps the release's weight")
	assert.Contains(t, conflictErr.Msg, "stable, which isn't deployed in dev")
}

func TestStep_WaitsForAReportOfTheCurrentSpec(t *testing.T) {
	ctx := context.Background()
	svc, c := fixture(t, true)
	// dev was pinned again; its status is still the one for the old pin.
	b := &openchoreov1alpha1.EndpointReleaseBinding{}
	require.NoError(t, c.Get(ctx, client.ObjectKey{Namespace: ns, Name: epName + "-dev"}, b))
	b.Generation = 2
	b.Status.Conditions[0].ObservedGeneration = 1
	require.NoError(t, c.Update(ctx, b))

	_, err := svc.Step(ctx, ns, epName, StepRequest{Environment: "dev"})
	var conflictErr *ConflictError
	require.ErrorAs(t, err, &conflictErr)
	assert.Contains(t, conflictErr.Msg, "v2, which dev doesn't report yet")

	_, err = svc.Abort(ctx, ns, epName, AbortRequest{Environment: "dev"})
	assert.NoError(t, err, "back to step 0 doesn't wait for the report")
}

func TestStep_RefusedWhereAnEnvironmentHasTwoBindings(t *testing.T) {
	ctx := context.Background()
	svc, c := fixture(t, true)
	twin := testutil.NewEndpointReleaseBinding(ns, project, epName, "dev", epName+"-dev-2")
	twin.Spec.EndpointRelease = release
	require.NoError(t, c.Create(ctx, twin))

	_, err := svc.Step(ctx, ns, epName, StepRequest{Environment: "dev"})
	var conflictErr *ConflictError
	require.ErrorAs(t, err, &conflictErr)
	assert.Contains(t, conflictErr.Msg, "has 2 bindings in dev ("+epName+"-dev, "+epName+"-dev-2); remove all but one")
	assert.Empty(t, overridesOf(t, c))
}

func TestStart_DryRunShowsTheOwnerTheSaveKeeps(t *testing.T) {
	ctx := context.Background()
	svc, c := fixture(t, false)
	ep := &openchoreov1alpha1.Endpoint{}
	require.NoError(t, c.Get(ctx, client.ObjectKey{Namespace: ns, Name: epName}, ep))
	ep.Spec.Owner.ComponentName = "greeter"
	ep.Spec.BackendRefs = ep.Spec.BackendRefs[:1]
	require.NoError(t, c.Update(ctx, ep))

	status, err := svc.Start(ctx, ns, epName, StartRequest{Rollout: canaryPlan(), Backends: []openchoreov1alpha1.EndpointBackendRef{candidate()},
		DryRun: true})
	require.NoError(t, err)
	assert.Empty(t, status.Change.Endpoint.Owner.ComponentName, "v2 routes to greeter-v2, so the project owns the Endpoint")
}
