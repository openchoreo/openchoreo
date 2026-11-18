// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package endpointrollout

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

func backend(name string, weight int32) openchoreov1alpha1.EndpointBackendRef {
	return openchoreov1alpha1.EndpointBackendRef{
		Name:      name,
		Kind:      openchoreov1alpha1.EndpointBackendKindComponent,
		Component: &openchoreov1alpha1.EndpointComponentBackend{Name: name, Endpoint: "http"},
		Weight:    ptr(weight),
	}
}

func steps(name string, percents ...int32) *openchoreov1alpha1.EndpointRolloutSteps {
	out := &openchoreov1alpha1.EndpointRolloutSteps{}
	for _, p := range percents {
		out.Steps = append(out.Steps, openchoreov1alpha1.EndpointRolloutStep{
			Backends: []openchoreov1alpha1.EndpointRolloutStepBackend{{Name: name, Percent: p}},
		})
	}
	return out
}

func canary() *openchoreov1alpha1.EndpointRollout {
	return &openchoreov1alpha1.EndpointRollout{
		Strategy:   openchoreov1alpha1.EndpointRolloutCanary,
		Stable:     "stable",
		Candidates: []openchoreov1alpha1.EndpointRolloutCandidate{{Name: "v2"}},
		Canary:     steps("v2", 5, 25, 100),
	}
}

func weightsOf(overrides []openchoreov1alpha1.EndpointBackendOverride) map[string]int32 {
	out := map[string]int32{}
	for _, o := range overrides {
		if o.Weight != nil {
			out[o.Name] = *o.Weight
		}
	}
	return out
}

func TestStages(t *testing.T) {
	assert.Len(t, Stages(canary()), 4, "stage 0, then each step")

	bg := &openchoreov1alpha1.EndpointRollout{
		Strategy: openchoreov1alpha1.EndpointRolloutBlueGreen, Stable: "stable",
		Candidates: []openchoreov1alpha1.EndpointRolloutCandidate{{Name: "green"}},
	}
	got := Stages(bg)
	require.Len(t, got, 2)
	assert.Equal(t, int32(100), got[1].percent("green"))

	ab := &openchoreov1alpha1.EndpointRollout{
		Strategy: openchoreov1alpha1.EndpointRolloutABTest, Stable: "stable",
		Candidates: []openchoreov1alpha1.EndpointRolloutCandidate{{Name: "b"}, {Name: "c"}},
	}
	assert.Len(t, Stages(ab), 1, "an abTest has no steps")
}

func TestOverrides_StableAndCandidateSplit100(t *testing.T) {
	r := canary()
	refs := []openchoreov1alpha1.EndpointBackendRef{backend("stable", 1), backend("v2", 0)}

	got, err := Overrides(r, refs, nil, Stages(r)[2])
	require.NoError(t, err)
	assert.Equal(t, map[string]int32{"stable": 75, "v2": 25}, weightsOf(got))
	assert.Equal(t, 2, Position(r, refs, got))
	assert.Equal(t, 0, Position(r, refs, nil), "no overrides is stage 0")
}

func TestOverrides_OtherBackendsKeepTheirShare(t *testing.T) {
	r := canary()
	refs := []openchoreov1alpha1.EndpointBackendRef{backend("stable", 60), backend("legacy", 40), backend("v2", 0)}

	got, err := Overrides(r, refs, nil, Stages(r)[1])
	require.NoError(t, err)
	assert.Equal(t, map[string]int32{"stable": 5700, "v2": 300, "legacy": 4000}, weightsOf(got),
		"v2 gets exactly 5% of stable's 60%, and legacy keeps its 40%")
	assert.Equal(t, 1, Position(r, refs, got))
}

func TestOverrides_KeepsTheEnvironmentsWeightForOtherBackends(t *testing.T) {
	r := canary()
	refs := []openchoreov1alpha1.EndpointBackendRef{backend("stable", 60), backend("legacy", 40), backend("v2", 0)}
	// The environment gives legacy 20 of 80, a quarter of the traffic.
	current := []openchoreov1alpha1.EndpointBackendOverride{{Name: "legacy", Weight: ptr(int32(20))}}

	for _, stage := range Stages(r)[1:] {
		got, err := Overrides(r, refs, current, stage)
		require.NoError(t, err)
		w := weightsOf(got)
		total := w["stable"] + w["v2"] + w["legacy"]
		share := float64(w["legacy"]) * 100 / float64(total)
		assert.Equal(t, float64(25), share, "legacy keeps its quarter at step %d", stage.Index)
		assert.Equal(t, stage.Index, Position(r, refs, got))
		current = got
	}
}

func TestOverrides_RepeatedStepsScaleOnce(t *testing.T) {
	r := canary()
	refs := []openchoreov1alpha1.EndpointBackendRef{backend("stable", 60), backend("legacy", 40), backend("v2", 0)}

	var current []openchoreov1alpha1.EndpointBackendOverride
	for _, i := range []int{1, 2, 1, 0, 3} {
		got, err := Overrides(r, refs, current, Stages(r)[i])
		require.NoError(t, err)
		w := weightsOf(got)
		assert.Equal(t, int32(4000), w["legacy"], "legacy is scaled once and then left alone")
		assert.Equal(t, int32(6000), w["stable"]+w["v2"])
		assert.Equal(t, i, Position(r, refs, got))
		current = got
	}
}

func TestOverrides_KeepsOtherOverrides(t *testing.T) {
	r := canary()
	shadow := backend("shadow", 0)
	shadow.Mirror = &openchoreov1alpha1.EndpointMirror{Percent: 100}
	refs := []openchoreov1alpha1.EndpointBackendRef{backend("stable", 1), backend("v2", 0), shadow}
	current := []openchoreov1alpha1.EndpointBackendOverride{
		{Name: "shadow", Mirror: &openchoreov1alpha1.EndpointMirror{Percent: 10}},
		{Name: "v2", Weight: ptr(int32(5))},
	}

	got, err := Overrides(r, refs, current, Stages(r)[3])
	require.NoError(t, err)
	require.Len(t, got, 3)
	assert.Equal(t, "shadow", got[0].Name)
	assert.Equal(t, int32(10), got[0].Mirror.Percent)
	assert.Equal(t, map[string]int32{"stable": 0, "v2": 100}, weightsOf(got))
}

func TestOverrides_Mirror(t *testing.T) {
	r := &openchoreov1alpha1.EndpointRollout{
		Strategy: openchoreov1alpha1.EndpointRolloutMirror, Stable: "stable",
		Candidates: []openchoreov1alpha1.EndpointRolloutCandidate{{Name: "shadow"}},
		Mirror:     steps("shadow", 10, 50),
	}
	shadow := backend("shadow", 0)
	shadow.Mirror = &openchoreov1alpha1.EndpointMirror{Percent: 0}
	refs := []openchoreov1alpha1.EndpointBackendRef{backend("stable", 1), shadow}

	got, err := Overrides(r, refs, nil, Stages(r)[2])
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Nil(t, got[0].Weight)
	assert.Equal(t, int32(50), got[0].Mirror.Percent)
	assert.Equal(t, 2, Position(r, refs, got))
}

func TestOverrides_RefusesAReleaseWithoutTheCandidate(t *testing.T) {
	r := canary()
	_, err := Overrides(r, []openchoreov1alpha1.EndpointBackendRef{backend("stable", 1)}, nil, Stages(r)[1])
	assert.ErrorContains(t, err, "v2")
}

func TestPosition_ByHand(t *testing.T) {
	r := canary()
	refs := []openchoreov1alpha1.EndpointBackendRef{backend("stable", 1), backend("v2", 0)}
	overrides := []openchoreov1alpha1.EndpointBackendOverride{
		{Name: "stable", Weight: ptr(int32(90))}, {Name: "v2", Weight: ptr(int32(10))},
	}
	assert.Equal(t, -1, Position(r, refs, overrides))
	assert.Equal(t, int32(10), Percents(r, refs, overrides)["v2"])
}

func TestFinish(t *testing.T) {
	r := canary()
	v2 := backend("v2", 0)
	v2.Match = &openchoreov1alpha1.EndpointMatch{
		Headers: []openchoreov1alpha1.EndpointHeaderMatch{{Name: "x-canary", Value: "true"}},
	}
	refs := []openchoreov1alpha1.EndpointBackendRef{backend("stable", 60), backend("legacy", 40), v2}

	out, leaving, err := Finish(r, refs, "v2")
	require.NoError(t, err)
	assert.Equal(t, []string{"stable"}, leaving)
	require.Len(t, out, 2)
	assert.Equal(t, "legacy", out[0].Name)
	assert.Equal(t, "v2", out[1].Name)
	assert.Equal(t, int32(60), *out[1].Weight, "the candidate takes stable's weight")
	assert.Nil(t, out[1].Match)

	out, leaving, err = Finish(r, refs, "stable")
	require.NoError(t, err)
	assert.Equal(t, []string{"v2"}, leaving)
	assert.Len(t, out, 2)

	_, _, err = Finish(r, refs, "legacy")
	assert.Error(t, err)
}

func TestFinish_MirrorKeepsOnlyStable(t *testing.T) {
	r := &openchoreov1alpha1.EndpointRollout{
		Strategy: openchoreov1alpha1.EndpointRolloutMirror, Stable: "stable",
		Candidates: []openchoreov1alpha1.EndpointRolloutCandidate{{Name: "shadow"}},
		Mirror:     steps("shadow", 100),
	}
	_, _, err := Finish(r, []openchoreov1alpha1.EndpointBackendRef{backend("stable", 1), backend("shadow", 0)}, "shadow")
	assert.ErrorContains(t, err, "only receives copies")
}

func TestValidate(t *testing.T) {
	refs := func() []openchoreov1alpha1.EndpointBackendRef {
		return []openchoreov1alpha1.EndpointBackendRef{backend("stable", 1), backend("v2", 0)}
	}
	require.NoError(t, Validate(canary(), refs()))

	type plan = *openchoreov1alpha1.EndpointRollout
	type backends = []openchoreov1alpha1.EndpointBackendRef
	cases := map[string]struct {
		edit func(plan, backends)
		want string
	}{
		"no candidate": {func(r plan, _ backends) {
			r.Strategy, r.Canary, r.Candidates = openchoreov1alpha1.EndpointRolloutBlueGreen, nil, nil
		}, "exactly one candidate"},
		"an unknown stable": {func(r plan, _ backends) {
			r.Stable = "missing"
		}, "rollout.stable must name a backend"},
		"a mirror as stable": {func(_ plan, refs backends) {
			refs[0].Mirror = &openchoreov1alpha1.EndpointMirror{Percent: 100}
		}, "can't be a mirror backend"},
		"a candidate with traffic": {func(_ plan, refs backends) {
			refs[1].Weight = nil
		}, "must start at weight 0"},
		"a repeated step": {func(r plan, _ backends) {
			r.Canary = steps("v2", 25, 25, 100)
		}, "step 2 (25%) must give v2 more than 25%"},
		"a candidate twice in a step": {func(r plan, _ backends) {
			r.Canary = steps("v2", 25, 100)
			r.Canary.Steps[0].Backends = append(r.Canary.Steps[0].Backends, r.Canary.Steps[0].Backends[0])
		}, `step 1 names "v2" twice`},
		"a first step at 0": {func(r plan, _ backends) {
			r.Canary = steps("v2", 0, 100)
		}, "step 1 (0%)"},
		"a canary that stops short": {func(r plan, _ backends) {
			r.Canary = steps("v2", 5, 50)
		}, "last step must be 100%"},
		"steps for another strategy": {func(r plan, _ backends) {
			r.Strategy = openchoreov1alpha1.EndpointRolloutBlueGreen
		}, "set canary for a canary rollout only"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			r, rs := canary(), refs()
			tc.edit(r, rs)
			assert.ErrorContains(t, Validate(r, rs), tc.want)
		})
	}

	shadow := backend("v2", 0)
	shadow.Mirror = &openchoreov1alpha1.EndpointMirror{Percent: 0}
	mirror := &openchoreov1alpha1.EndpointRollout{
		Strategy: openchoreov1alpha1.EndpointRolloutMirror, Stable: "stable",
		Candidates: []openchoreov1alpha1.EndpointRolloutCandidate{{Name: "v2"}},
		Mirror:     steps("v2", 10, 50),
	}
	assert.NoError(t, Validate(mirror, []openchoreov1alpha1.EndpointBackendRef{backend("stable", 1), shadow}),
		"a mirror needn't end at 100%")

	shadow.Mirror.Percent = 100
	assert.ErrorContains(t, Validate(mirror, []openchoreov1alpha1.EndpointBackendRef{backend("stable", 1), shadow}),
		"v2 must start at 0%, so it gets no copies before the first step")
}

func TestPosition_LargeOtherWeights(t *testing.T) {
	r := canary()
	refs := []openchoreov1alpha1.EndpointBackendRef{backend("stable", 3), backend("legacy", 20000), backend("v2", 0)}

	// legacy can't be scaled by 100, so every weight is scaled by 50.
	var current []openchoreov1alpha1.EndpointBackendOverride
	for _, stage := range Stages(r)[1:] {
		got, err := Overrides(r, refs, current, stage)
		require.NoError(t, err)
		assert.Equal(t, stage.percent("v2"), Percents(r, refs, got)["v2"], "v2 gets its percent at step %d", stage.Index)
		assert.Equal(t, stage.Index, Position(r, refs, got))
		w := weightsOf(got)
		assert.Equal(t, int32(1_000_000), w["legacy"])
		assert.Equal(t, int32(150), w["stable"]+w["v2"], "legacy keeps its share")
		current = got
	}
}

func TestOverrides_RefusesAStepTheWeightsCantSet(t *testing.T) {
	r := canary()
	refs := []openchoreov1alpha1.EndpointBackendRef{backend("stable", 1), backend("legacy", 999_999), backend("v2", 0)}

	_, err := Overrides(r, refs, nil, Stages(r)[1])
	require.ErrorIs(t, err, ErrStageTooFine)
	assert.ErrorContains(t, err, "v2 can't get 5% of 1")
	_, err = Overrides(r, refs, nil, Stages(r)[0])
	assert.NoError(t, err, "back to step 0 always works")
}

func TestOverrides_SwitchedOffBackendStaysOff(t *testing.T) {
	r := canary()
	refs := []openchoreov1alpha1.EndpointBackendRef{backend("stable", 60), backend("legacy", 40), backend("v2", 0)}
	current := []openchoreov1alpha1.EndpointBackendOverride{{Name: "legacy", Weight: ptr(int32(0))}}

	got, err := Overrides(r, refs, current, Stages(r)[2])
	require.NoError(t, err)
	assert.Equal(t, map[string]int32{"stable": 75, "v2": 25, "legacy": 0}, weightsOf(got))
}

func TestNext(t *testing.T) {
	r := canary()
	r.Canary = steps("v2", 25, 25, 100)
	refs := []openchoreov1alpha1.EndpointBackendRef{backend("stable", 1), backend("v2", 0)}

	assert.Equal(t, 1, Next(r, refs, nil))
	at25, err := Overrides(r, refs, nil, Stages(r)[1])
	require.NoError(t, err)
	assert.Equal(t, 3, Next(r, refs, at25), "a step that changes nothing is skipped")

	byHand := []openchoreov1alpha1.EndpointBackendOverride{
		{Name: "stable", Weight: ptr(int32(70))}, {Name: "v2", Weight: ptr(int32(30))},
	}
	assert.Equal(t, 3, Next(r, refs, byHand), "weights set by hand move to the first step past them")

	last, err := Overrides(r, refs, nil, Stages(r)[3])
	require.NoError(t, err)
	assert.Equal(t, -1, Next(r, refs, last))

	almost := []openchoreov1alpha1.EndpointBackendOverride{
		{Name: "stable", Weight: ptr(int32(1))}, {Name: "v2", Weight: ptr(int32(999))},
	}
	require.Equal(t, 3, Position(r, refs, almost), "v2's 99.9% rounds to the last step")
	assert.Equal(t, 3, Next(r, refs, almost), "stable still has weight, so the last step is still to take")
}

func TestMismatched(t *testing.T) {
	r := canary()
	spec := []openchoreov1alpha1.EndpointBackendRef{backend("stable", 100), backend("v2", 0), backend("legacy", 1)}
	release := []openchoreov1alpha1.EndpointBackendRef{backend("stable", 100), backend("v2", 0)}
	assert.Empty(t, Mismatched(r, spec, release))

	release[1].Component = &openchoreov1alpha1.EndpointComponentBackend{Name: "v2-old", Endpoint: "http"}
	assert.Equal(t, []string{"v2 routes to v2-old/http there, not v2/http"}, Mismatched(r, spec, release))

	release[1] = backend("v2", 0)
	release[1].Mirror = &openchoreov1alpha1.EndpointMirror{Percent: 10}
	assert.Equal(t, []string{"v2 is a mirror there, not a weighted backend"}, Mismatched(r, spec, release))

	release[1] = backend("v2", 0)
	release[1].Match = &openchoreov1alpha1.EndpointMatch{Path: &openchoreov1alpha1.EndpointPathMatch{Value: "/old"}}
	assert.Empty(t, Mismatched(r, spec, release), "a match alone doesn't change where a step sends traffic")
	assert.Empty(t, Mismatched(r, spec, release[:1]), "a missing backend is Missing's to report")
}

func TestOverrides_RefusesToInventTrafficForAPairTheEnvironmentTurnedOff(t *testing.T) {
	r := canary()
	refs := []openchoreov1alpha1.EndpointBackendRef{backend("stable", 60), backend("legacy", 40), backend("v2", 0)}
	current := []openchoreov1alpha1.EndpointBackendOverride{{Name: "stable", Weight: ptr(int32(0))}}

	_, err := Overrides(r, refs, current, Stages(r)[1])
	require.ErrorIs(t, err, ErrNoRolloutTraffic, "legacy's traffic is not handed to v2")

	got, err := Overrides(r, refs, current, Stages(r)[0])
	require.NoError(t, err)
	assert.Equal(t, map[string]int32{"stable": 0, "v2": 0}, weightsOf(got), "back to step 0 changes nothing")
}

func TestOverrides_KeepsRatiosAboveTheMaximumWeight(t *testing.T) {
	r := canary()
	refs := []openchoreov1alpha1.EndpointBackendRef{backend("stable", 60), backend("legacy", 40), backend("v2", 0)}
	current := []openchoreov1alpha1.EndpointBackendOverride{
		{Name: "stable", Weight: ptr(int32(800000))},
		{Name: "v2", Weight: ptr(int32(400000))},
		{Name: "legacy", Weight: ptr(int32(300000))},
	}

	var atZero []openchoreov1alpha1.EndpointBackendOverride
	for _, stage := range Stages(r) {
		got, err := Overrides(r, refs, current, stage)
		require.NoError(t, err)
		if stage.Index == 0 {
			atZero = got
		}
		w := weightsOf(got)
		for name, v := range w {
			assert.LessOrEqual(t, v, int32(maxWeight), "%s at step %d", name, stage.Index)
		}
		pair := float64(w["stable"] + w["v2"])
		assert.InDelta(t, 0.2, float64(w["legacy"])/(pair+float64(w["legacy"])), 0.001,
			"legacy keeps its fifth of the traffic at step %d", stage.Index)
		assert.Equal(t, stage.Index, Position(r, refs, got))
	}
	assert.Equal(t, 1, Next(r, refs, atZero), "from the scaled-down step 0, the next step is 1")
}
