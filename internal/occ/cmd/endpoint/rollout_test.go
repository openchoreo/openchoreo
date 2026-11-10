// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package endpoint

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"k8s.io/utils/ptr"

	"github.com/openchoreo/openchoreo/internal/occ/resources/client/mocks"
	"github.com/openchoreo/openchoreo/internal/occ/testutil"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
)

const goCandidate = "trips-go"

func stage(index int, percent int32) gen.EndpointRolloutStage {
	return gen.EndpointRolloutStage{Index: index, Backends: []struct {
		Name    string `json:"name"`
		Percent int32  `json:"percent"`
	}{{Name: goCandidate, Percent: percent}}}
}

func rolloutStatus(step int) *gen.EndpointRolloutStatus {
	return &gen.EndpointRolloutStatus{
		Endpoint: "trips-api",
		Rollout: &gen.EndpointRollout{
			Strategy: gen.EndpointRolloutStrategyCanary,
			Stable:   "trips-python",
			Candidates: []struct {
				Name string `json:"name"`
			}{{Name: goCandidate}},
		},
		Stages: &[]gen.EndpointRolloutStage{stage(0, 0), stage(1, 5), stage(2, 25)},
		Environments: []gen.EndpointRolloutEnvironment{{
			Environment:     "development",
			BindingName:     "trips-api-development",
			EndpointRelease: ptr.To("trips-api-1"),
			Step:            &step,
			Percents:        &map[string]int32{goCandidate: 5},
			Ready:           true,
		}},
	}
}

func TestRolloutGet_Tree(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	status := rolloutStatus(1)
	status.Environments[0].Overrides = []gen.EndpointBackendOverride{
		{Name: "trips-python", Weight: ptr.To[int32](95)}, {Name: goCandidate, Weight: ptr.To[int32](5)},
	}
	status.Environments[0].Backends = []gen.ResolvedEndpointBackend{
		{Name: "trips-python", Weight: ptr.To[int32](95), Resolved: true},
		{Name: goCandidate, Weight: ptr.To[int32](5), Resolved: true},
	}
	mc.EXPECT().GetEndpointRollout(mock.Anything, "default", "trips-api").Return(status, nil)
	mc.EXPECT().GetEndpoint(mock.Anything, "default", "trips-api").Return(&gen.Endpoint{Spec: &gen.EndpointSpec{
		BackendRefs: []gen.EndpointBackendRef{{
			Name:  goCandidate,
			Match: &gen.EndpointMatch{Headers: &[]gen.EndpointHeaderMatch{{Name: "x-canary", Value: "true"}}},
		}},
	}}, nil)

	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, New(mc).RolloutGet(RolloutGetParams{
			RolloutParams: RolloutParams{Namespace: "default", EndpointName: "trips-api"},
		}))
	})
	assert.Contains(t, out, "Canary   trips-python → trips-go")
	assert.Contains(t, out, "Testers:    header x-canary: true → trips-go")
	assert.Contains(t, out, "Steps:      5% → 25%")
	assert.Contains(t, out, "1/2")
	assert.Contains(t, out, "trips-go 5  trips-python 95")
	assert.Contains(t, out, "✔ Healthy")
	assert.Contains(t, out, "► 5%  ○ 25%")
}

func TestRolloutStep_SendsExpectedStep(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetEndpointRollout(mock.Anything, "default", "trips-api").Return(rolloutStatus(0), nil)
	mc.EXPECT().StepEndpointRollout(mock.Anything, "default", "trips-api", mock.MatchedBy(func(r gen.EndpointRolloutStepRequest) bool {
		return r.Environment == "development" && r.ExpectedStep != nil && *r.ExpectedStep == 0 && r.To == nil && !*r.DryRun
	})).Return(rolloutStatus(1), nil)

	testutil.CaptureStdout(t, func() {
		require.NoError(t, New(mc).RolloutStep(RolloutStepParams{
			RolloutParams: RolloutParams{Namespace: "default", EndpointName: "trips-api"},
			Environment:   "development",
			ExpectedStep:  ptr.To(0),
		}))
	})
}

func TestRolloutStep_RefusedStepIsReturned(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetEndpointRollout(mock.Anything, "default", "trips-api").Return(rolloutStatus(1), nil)
	mc.EXPECT().StepEndpointRollout(mock.Anything, "default", "trips-api", mock.Anything).
		Return(nil, fmt.Errorf("development is at step 1, not step 0; check its status and retry"))

	err := New(mc).RolloutStep(RolloutStepParams{
		RolloutParams: RolloutParams{Namespace: "default", EndpointName: "trips-api"},
		Environment:   "development",
		ExpectedStep:  ptr.To(0),
	})
	assert.ErrorContains(t, err, "not step 0")
}

func TestRolloutStep_DryRunYAMLPrintsTheBinding(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetEndpointRollout(mock.Anything, "default", "trips-api").Return(rolloutStatus(0), nil)
	status := rolloutStatus(0)
	status.Change = &gen.EndpointRolloutChange{
		Environment: ptr.To("development"),
		After: &[]gen.EndpointBackendOverride{
			{Name: goCandidate, Weight: ptr.To[int32](5)},
			{Name: "trips-python", Weight: ptr.To[int32](95)},
		},
	}
	mc.EXPECT().StepEndpointRollout(mock.Anything, "default", "trips-api", mock.Anything).Return(status, nil)
	mc.EXPECT().GetEndpointReleaseBinding(mock.Anything, "default", "trips-api-development").Return(&gen.EndpointReleaseBinding{
		Metadata: gen.ObjectMeta{Name: "trips-api-development", Namespace: ptr.To("default")},
		Spec:     &gen.EndpointReleaseBindingSpec{Environment: "development"},
	}, nil)

	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, New(mc).RolloutStep(RolloutStepParams{
			RolloutParams: RolloutParams{Namespace: "default", EndpointName: "trips-api", DryRun: true, Output: "yaml"},
			Environment:   "development",
		}))
	})
	assert.Contains(t, out, "kind: EndpointReleaseBinding")
	assert.Contains(t, out, "backendOverrides:")
	assert.Contains(t, out, "weight: 95")
	assert.NotContains(t, out, "status:")
}

func TestRolloutFinish_DryRunYAMLPrintsTheEndpoint(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	status := rolloutStatus(2)
	status.Change = &gen.EndpointRolloutChange{Endpoint: &gen.EndpointSpec{
		BackendRefs: []gen.EndpointBackendRef{{Name: goCandidate, Weight: ptr.To[int32](100)}},
	}}
	mc.EXPECT().FinishEndpointRollout(mock.Anything, "default", "trips-api", mock.MatchedBy(func(r gen.EndpointRolloutFinishRequest) bool {
		return r.Keep == goCandidate && *r.DryRun
	})).Return(status, nil)
	mc.EXPECT().GetEndpoint(mock.Anything, "default", "trips-api").Return(&gen.Endpoint{
		Metadata: gen.ObjectMeta{Name: "trips-api"},
	}, nil)

	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, New(mc).RolloutFinish(RolloutFinishParams{
			RolloutParams: RolloutParams{Namespace: "default", EndpointName: "trips-api", DryRun: true, Output: "yaml"},
			Keep:          goCandidate,
		}))
	})
	assert.Contains(t, out, "kind: Endpoint\n")
	assert.Contains(t, out, "name: trips-go")
}

func TestRollout_RequiredFields(t *testing.T) {
	r := New(mocks.NewMockInterface(t))
	base := RolloutParams{Namespace: "default", EndpointName: "trips-api"}
	assert.Error(t, r.RolloutStep(RolloutStepParams{RolloutParams: base}), "env is required")
	assert.Error(t, r.RolloutAbort(RolloutStepParams{RolloutParams: base}), "env is required")
	assert.Error(t, r.RolloutFinish(RolloutFinishParams{RolloutParams: base}), "keep is required")
	assert.Error(t, r.RolloutStart(RolloutStartParams{RolloutParams: base}), "a file or flags are required")
	assert.Error(t, r.RolloutGet(RolloutGetParams{RolloutParams: RolloutParams{EndpointName: "trips-api"}}), "namespace is required")
	assert.Error(t, r.RolloutStatus(RolloutWaitParams{RolloutParams: base}), "env is required")
}

func stepTo(to int) any {
	return mock.MatchedBy(func(r gen.EndpointRolloutStepRequest) bool { return r.To != nil && *r.To == to })
}

func viewOf(t *testing.T, status *gen.EndpointRolloutStatus, refs []gen.EndpointBackendRef) string {
	t.Helper()
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetEndpointRollout(mock.Anything, "default", "trips-api").Return(status, nil)
	mc.EXPECT().GetEndpoint(mock.Anything, "default", "trips-api").Return(&gen.Endpoint{
		Spec: &gen.EndpointSpec{BackendRefs: refs},
	}, nil)
	return testutil.CaptureStdout(t, func() {
		require.NoError(t, New(mc).RolloutGet(RolloutGetParams{
			RolloutParams: RolloutParams{Namespace: "default", EndpointName: "trips-api"},
		}))
	})
}

func TestRolloutGet_BlueGreen(t *testing.T) {
	status := rolloutStatus(0)
	status.Rollout.Strategy = gen.EndpointRolloutStrategyBlueGreen
	status.Stages = &[]gen.EndpointRolloutStage{stage(0, 0), stage(1, 100)}
	out := viewOf(t, status, []gen.EndpointBackendRef{{
		Name:  goCandidate,
		Match: &gen.EndpointMatch{Path: &gen.EndpointPathMatch{Value: "/next"}},
	}})
	assert.Contains(t, out, "Blue/green   trips-python → trips-go")
	assert.Contains(t, out, "Steps:      preview → switched")
	assert.Contains(t, out, "Preview:    path /next → trips-go")
	assert.Contains(t, out, "► preview  ○ switched")
	assert.NotContains(t, out, "0/1")
}

func TestRolloutGet_ABTest(t *testing.T) {
	status := rolloutStatus(0)
	status.Rollout.Strategy = gen.EndpointRolloutStrategyAbTest
	status.Stages = &[]gen.EndpointRolloutStage{stage(0, 0)}
	out := viewOf(t, status, []gen.EndpointBackendRef{{
		Name: goCandidate,
		Match: &gen.EndpointMatch{
			Cookies: &[]gen.EndpointCookieMatch{{Name: "exp", Value: "b"}},
			Headers: &[]gen.EndpointHeaderMatch{{Name: "x-client", Value: "ios"}},
		},
	}})
	assert.Contains(t, out, "A/B test   trips-python → trips-go")
	assert.Contains(t, out, "Group:      cookie exp=b + header x-client: ios → trips-go")
	assert.Contains(t, out, "experiment")
	assert.Contains(t, out, "finish trips-api --keep <winner>")
}

func TestRolloutGet_Mirror(t *testing.T) {
	status := rolloutStatus(1)
	status.Rollout.Strategy = gen.EndpointRolloutStrategyMirror
	status.Environments[0].Overrides = []gen.EndpointBackendOverride{
		{Name: goCandidate, Mirror: &gen.EndpointMirror{Percent: 5}},
	}
	out := viewOf(t, status, []gen.EndpointBackendRef{{Name: goCandidate, Mirror: &gen.EndpointMirror{Percent: 0}}})
	assert.Contains(t, out, "Mirror   trips-python → trips-go")
	assert.Contains(t, out, "Copies:     5% → 25%")
	assert.Contains(t, out, "copying 1/2")
	assert.Contains(t, out, "trips-go copy 5%")
}

func TestRolloutStatus_WaitsForReady(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	progressing := rolloutStatus(1)
	progressing.Environments[0].Ready = false
	mc.EXPECT().GetEndpointRollout(mock.Anything, "default", "trips-api").Return(progressing, nil).Once()
	mc.EXPECT().GetEndpointRollout(mock.Anything, "default", "trips-api").Return(rolloutStatus(1), nil).Once()

	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, New(mc).RolloutStatus(RolloutWaitParams{
			RolloutParams: RolloutParams{Namespace: "default", EndpointName: "trips-api"},
			Environment:   "development",
			Timeout:       time.Minute,
			Interval:      time.Millisecond,
		}))
	})
	assert.Contains(t, out, "development: at step 1, ✔ Healthy")
}

func TestRolloutStatus_FailsAfterTheTimeout(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	progressing := rolloutStatus(1)
	progressing.Environments[0].Ready = false
	mc.EXPECT().GetEndpointRollout(mock.Anything, "default", "trips-api").Return(progressing, nil)

	err := New(mc).RolloutStatus(RolloutWaitParams{
		RolloutParams: RolloutParams{Namespace: "default", EndpointName: "trips-api"},
		Environment:   "development",
		Interval:      time.Millisecond,
	})
	assert.ErrorContains(t, err, "is not serving its latest change")
}

func TestRolloutStatus_RefusesReadyAfterTheTimeout(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	progressing := rolloutStatus(1)
	progressing.Environments[0].Ready = false
	mc.EXPECT().GetEndpointRollout(mock.Anything, "default", "trips-api").Return(progressing, nil).Once()
	mc.EXPECT().GetEndpointRollout(mock.Anything, "default", "trips-api").Return(rolloutStatus(1), nil).Maybe()

	err := New(mc).RolloutStatus(RolloutWaitParams{
		RolloutParams: RolloutParams{Namespace: "default", EndpointName: "trips-api"},
		Environment:   "development",
		Timeout:       time.Millisecond,
		Interval:      20 * time.Millisecond,
	})
	assert.ErrorContains(t, err, "is not serving its latest change")
}

func TestRolloutSet_MergesIntoTheBindingItWrites(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	// The status read is older: another client has since set legacy=10.
	mc.EXPECT().GetEndpointRollout(mock.Anything, "default", "trips-api").Return(rolloutStatus(1), nil)
	mc.EXPECT().GetEndpointReleaseBinding(mock.Anything, "default", "trips-api-development").Return(&gen.EndpointReleaseBinding{
		Metadata: gen.ObjectMeta{Name: "trips-api-development"},
		Spec: &gen.EndpointReleaseBindingSpec{Environment: "development", BackendOverrides: &[]gen.EndpointBackendOverride{
			{Name: "legacy", Weight: ptr.To[int32](10)},
			{Name: goCandidate, Weight: ptr.To[int32](5)},
		}},
	}, nil)
	written := &gen.EndpointReleaseBinding{
		Metadata: gen.ObjectMeta{Name: "trips-api-development"},
		Spec:     &gen.EndpointReleaseBindingSpec{Environment: "development"},
	}
	mc.EXPECT().UpdateEndpointReleaseBinding(mock.Anything, "default", "trips-api-development",
		mock.MatchedBy(func(b gen.EndpointReleaseBinding) bool {
			o := *b.Spec.BackendOverrides
			return len(o) == 2 && o[0].Name == "legacy" && *o[0].Weight == 10 && o[1].Name == goCandidate && *o[1].Weight == 40
		})).Return(written, nil)

	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, New(mc).RolloutSet(RolloutSetParams{
			RolloutParams: RolloutParams{Namespace: "default", EndpointName: "trips-api", Output: "yaml"},
			Environment:   "development",
			Weights:       map[string]int32{goCandidate: 40},
		}))
	})
	assert.Contains(t, out, "kind: EndpointReleaseBinding", "-o yaml prints the binding written")
}

func TestRolloutSet_WritesTheBindingWeights(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetEndpointRollout(mock.Anything, "default", "trips-api").Return(rolloutStatus(1), nil)
	mc.EXPECT().GetEndpointReleaseBinding(mock.Anything, "default", "trips-api-development").Return(&gen.EndpointReleaseBinding{
		Metadata: gen.ObjectMeta{Name: "trips-api-development"},
		Spec:     &gen.EndpointReleaseBindingSpec{Environment: "development"},
	}, nil)
	mc.EXPECT().UpdateEndpointReleaseBinding(mock.Anything, "default", "trips-api-development",
		mock.MatchedBy(func(b gen.EndpointReleaseBinding) bool {
			o := *b.Spec.BackendOverrides
			return len(o) == 2 && o[0].Name == goCandidate && *o[0].Weight == 30 && *o[1].Weight == 70
		})).Return(&gen.EndpointReleaseBinding{}, nil)

	testutil.CaptureStdout(t, func() {
		require.NoError(t, New(mc).RolloutSet(RolloutSetParams{
			RolloutParams: RolloutParams{Namespace: "default", EndpointName: "trips-api"},
			Environment:   "development",
			Weights:       map[string]int32{"trips-python": 70, goCandidate: 30},
		}))
	})
}

func TestRolloutStart_FromFlags(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetEndpoint(mock.Anything, "default", "trips-api").Return(&gen.Endpoint{
		Spec: &gen.EndpointSpec{BackendRefs: []gen.EndpointBackendRef{
			{Name: "trips-python", Weight: ptr.To[int32](100)},
			{Name: goCandidate, Weight: ptr.To[int32](0)},
		}},
	}, nil)
	mc.EXPECT().StartEndpointRollout(mock.Anything, "default", "trips-api", mock.MatchedBy(func(r gen.EndpointRolloutStartRequest) bool {
		steps := r.Rollout.Canary.Steps
		backends := *r.Backends
		return r.Rollout.Stable == "trips-python" && len(steps) == 3 && steps[1].Backends[0].Percent == 25 &&
			len(backends) == 1 && (*backends[0].Match.Headers)[0].Name == "x-canary"
	})).Return(rolloutStatus(0), nil)

	testutil.CaptureStdout(t, func() {
		require.NoError(t, New(mc).RolloutStart(RolloutStartParams{
			RolloutParams: RolloutParams{Namespace: "default", EndpointName: "trips-api"},
			Plan: RolloutPlanFlags{
				Strategy: "canary", Stable: "trips-python", Candidates: []string{goCandidate},
				Steps: []int32{5, 25, 100}, Header: "x-canary=true",
			},
		}))
	})
}

func TestRolloutList_OnlyEndpointsWithAPlan(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	planned := endpointInstance("trips-api", "ClusterEndpointType", "http")
	planned.Spec.Rollout = rolloutStatus(0).Rollout
	mc.EXPECT().ListEndpoints(mock.Anything, "default", mock.Anything).Return(&gen.EndpointList{
		Items: []gen.Endpoint{planned, endpointInstance("orders-api", "ClusterEndpointType", "http")},
	}, nil)
	mc.EXPECT().GetEndpointRollout(mock.Anything, "default", "trips-api").Return(rolloutStatus(1), nil)

	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, New(mc).RolloutList(ListParams{Namespace: "default"}))
	})
	assert.Contains(t, out, "trips-api")
	assert.Contains(t, out, "development 1/2")
	assert.NotContains(t, out, "orders-api")
}

func TestRolloutStart_AddsANewCandidate(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetEndpoint(mock.Anything, "default", "trips-api").Return(&gen.Endpoint{
		Spec: &gen.EndpointSpec{BackendRefs: []gen.EndpointBackendRef{{
			Name:      "test",
			Component: &gen.EndpointComponentBackend{Name: "test", Endpoint: "http"},
			Weight:    ptr.To[int32](100),
		}}},
	}, nil)
	mc.EXPECT().StartEndpointRollout(mock.Anything, "default", "trips-api", mock.MatchedBy(func(r gen.EndpointRolloutStartRequest) bool {
		b := (*r.Backends)[0]
		return r.Rollout.Candidates[0].Name == "shadow" && b.Name == "shadow" &&
			b.Component.Name == goCandidate && b.Component.Endpoint == "http" &&
			*b.Weight == 0 && b.Mirror != nil && b.Match == nil && r.Rollout.Mirror != nil
	})).Return(rolloutStatus(0), nil)

	testutil.CaptureStdout(t, func() {
		require.NoError(t, New(mc).RolloutStart(RolloutStartParams{
			RolloutParams: RolloutParams{Namespace: "default", EndpointName: "trips-api"},
			Plan: RolloutPlanFlags{
				Strategy: "mirror", Stable: "test", Candidates: []string{"shadow=trips-go"}, Steps: []int32{10, 100},
			},
		}))
	})
}

func TestRolloutStart_SaysHowToAddAMissingCandidate(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetEndpoint(mock.Anything, "default", "trips-api").Return(&gen.Endpoint{
		Spec: &gen.EndpointSpec{BackendRefs: []gen.EndpointBackendRef{{Name: "test"}}},
	}, nil)

	err := New(mc).RolloutStart(RolloutStartParams{
		RolloutParams: RolloutParams{Namespace: "default", EndpointName: "trips-api"},
		Plan:          RolloutPlanFlags{Strategy: "canary", Stable: "test", Candidates: []string{"canary"}},
	})
	assert.ErrorContains(t, err, "--candidate canary=COMPONENT")
}

func TestRolloutStart_ABTestNeedsACookie(t *testing.T) {
	err := New(mocks.NewMockInterface(t)).RolloutStart(RolloutStartParams{
		RolloutParams: RolloutParams{Namespace: "default", EndpointName: "trips-api"},
		Plan:          RolloutPlanFlags{Strategy: "abTest", Stable: "test", Candidates: []string{"b"}},
	})
	assert.ErrorContains(t, err, "--cookie")
}

func TestRolloutStart_OneCandidate(t *testing.T) {
	err := New(mocks.NewMockInterface(t)).RolloutStart(RolloutStartParams{
		RolloutParams: RolloutParams{Namespace: "default", EndpointName: "trips-api"},
		Plan:          RolloutPlanFlags{Strategy: "abTest", Stable: "test", Candidates: []string{"b", "c"}},
	})
	assert.ErrorContains(t, err, "a rollout has one candidate")
}

func devStepParams() RolloutStepParams {
	return RolloutStepParams{
		RolloutParams: RolloutParams{Namespace: "default", EndpointName: "trips-api"},
		Environment:   "development",
	}
}

func TestRolloutStep_Last(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetEndpointRollout(mock.Anything, "default", "trips-api").Return(rolloutStatus(1), nil)
	mc.EXPECT().StepEndpointRollout(mock.Anything, "default", "trips-api", stepTo(2)).Return(rolloutStatus(2), nil)

	params := devStepParams()
	params.Last = true
	testutil.CaptureStdout(t, func() { require.NoError(t, New(mc).RolloutStep(params)) })
}

func TestRolloutStep_BackGoesOneStepBack(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetEndpointRollout(mock.Anything, "default", "trips-api").Return(rolloutStatus(2), nil)
	mc.EXPECT().StepEndpointRollout(mock.Anything, "default", "trips-api", mock.MatchedBy(func(r gen.EndpointRolloutStepRequest) bool {
		return *r.To == 1 && *r.ExpectedStep == 2
	})).Return(rolloutStatus(1), nil)

	params := devStepParams()
	params.Back = true
	testutil.CaptureStdout(t, func() { require.NoError(t, New(mc).RolloutStep(params)) })
}

func TestRolloutStep_BackRefusedAtTheStart(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetEndpointRollout(mock.Anything, "default", "trips-api").Return(rolloutStatus(0), nil)

	params := devStepParams()
	params.Back = true
	assert.ErrorContains(t, New(mc).RolloutStep(params), "no step to go back to")
}

func TestRolloutStep_OneWayToPickTheStep(t *testing.T) {
	params := devStepParams()
	params.Back, params.Last = true, true
	assert.ErrorContains(t, New(mocks.NewMockInterface(t)).RolloutStep(params), "only one of --to, --back and --last")
}

func TestRolloutStep_RefusesAnABTest(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	status := rolloutStatus(0)
	status.Rollout.Strategy = gen.EndpointRolloutStrategyAbTest
	mc.EXPECT().GetEndpointRollout(mock.Anything, "default", "trips-api").Return(status, nil)

	assert.ErrorContains(t, New(mc).RolloutStep(devStepParams()), "--keep <winner>")
}

func TestRolloutStart_MirrorRefusesAMatch(t *testing.T) {
	err := New(mocks.NewMockInterface(t)).RolloutStart(RolloutStartParams{
		RolloutParams: RolloutParams{Namespace: "default", EndpointName: "trips-api"},
		Plan:          RolloutPlanFlags{Strategy: "mirror", Stable: "test", Candidates: []string{"b"}, Header: "x-canary=true"},
	})
	assert.ErrorContains(t, err, "a mirror rollout copies requests and routes none by match")
}

func TestRolloutGet_NamesTheEnvironmentItIsNotDeployedTo(t *testing.T) {
	var out strings.Builder
	printRolloutTree(&out, rolloutStatus(0), nil, "production")
	assert.Contains(t, out.String(), "Not deployed to production")
}
