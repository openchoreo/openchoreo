// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package endpoint

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"k8s.io/utils/ptr"

	"github.com/openchoreo/openchoreo/internal/occ/resources/client/mocks"
	"github.com/openchoreo/openchoreo/internal/occ/testutil"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
)

func tripsRefs() []gen.EndpointBackendRef {
	return []gen.EndpointBackendRef{
		{Name: "trips-python", Weight: ptr.To[int32](50)},
		{Name: "trips-stale", Weight: ptr.To[int32](50)},
		{Name: "trips-go", Weight: ptr.To[int32](0), Match: &gen.EndpointMatch{
			Headers: &[]gen.EndpointHeaderMatch{{Name: "x-preview", Value: "true"}},
		}},
		{Name: "trips-shadow", Weight: ptr.To[int32](0), Mirror: &gen.EndpointMirror{Percent: 10}},
	}
}

func tripsEndpoint() *gen.Endpoint {
	return &gen.Endpoint{Metadata: gen.ObjectMeta{Name: "trips"}, Spec: &gen.EndpointSpec{BackendRefs: tripsRefs()}}
}

func tripsBinding(env string, overrides ...gen.EndpointBackendOverride) gen.EndpointReleaseBinding {
	b := bindingForEnv("trips-"+env, env)
	b.Spec.EndpointRelease = ptr.To("trips-7")
	if len(overrides) > 0 {
		b.Spec.BackendOverrides = &overrides
	}
	return b
}

func expectBindings(mc *mocks.MockInterface, bindings ...gen.EndpointReleaseBinding) {
	mc.EXPECT().ListEndpointReleaseBindings(mock.Anything, "acme", mock.Anything).
		Return(&gen.EndpointReleaseBindingList{Items: bindings}, nil)
}

func expectRelease(mc *mocks.MockInterface) {
	mc.EXPECT().GetEndpointRelease(mock.Anything, "acme", "trips-7").
		Return(&gen.EndpointRelease{Spec: &gen.EndpointReleaseSpec{BackendRefs: tripsRefs()}}, nil)
}

func TestTrafficGet(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetEndpoint(mock.Anything, "acme", "trips").Return(tripsEndpoint(), nil)
	expectBindings(mc,
		tripsBinding("development",
			gen.EndpointBackendOverride{Name: "trips-python", Weight: ptr.To[int32](45)},
			gen.EndpointBackendOverride{Name: "trips-go", Weight: ptr.To[int32](5)},
			gen.EndpointBackendOverride{Name: "trips-java", Weight: ptr.To[int32](10)}),
		tripsBinding("staging", gen.EndpointBackendOverride{Name: "trips-java", Weight: ptr.To[int32](10)}),
		tripsBinding("production", gen.EndpointBackendOverride{Name: "trips-go", Match: &gen.EndpointMatch{
			Path: &gen.EndpointPathMatch{Value: "/beta"},
		}}),
	)
	mc.EXPECT().GetEndpointRelease(mock.Anything, "acme", "trips-7").
		Return(&gen.EndpointRelease{Spec: &gen.EndpointReleaseSpec{BackendRefs: tripsRefs()}}, nil).Times(3)

	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, New(mc).TrafficGet(TrafficGetParams{Namespace: "acme", EndpointName: "trips"}))
	})
	assert.Contains(t, out, "trips-python 50% · trips-stale 50% · trips-go 0% · copy 10% → trips-shadow")
	assert.Contains(t, out, "trips-python 90% · trips-stale 0% · trips-go 10%", "a backend the own split leaves out gets no share")
	assert.Regexp(t, `development\s+.*own split`, out)
	assert.Regexp(t, `staging\s+.*default split`, out, "an override for a backend the release lacks doesn't count")
	assert.Contains(t, out, "Default conditions:\n  trips-go gets requests with header x-preview: true")
	assert.Regexp(t, `production\s+.*own split`, out, "its own condition makes it differ")
	assert.Contains(t, out, "production's conditions:\n  trips-go gets requests with path /beta")
}

func TestTrafficSet_EnvironmentKeepsOwnConditions(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	expectBindings(mc, tripsBinding("development", gen.EndpointBackendOverride{Name: "trips-go", Match: &gen.EndpointMatch{
		Path: &gen.EndpointPathMatch{Value: "/beta"},
	}}))
	expectRelease(mc)
	mc.EXPECT().UpdateEndpointReleaseBinding(mock.Anything, "acme", "trips-development", mock.MatchedBy(func(b gen.EndpointReleaseBinding) bool {
		o := *b.Spec.BackendOverrides
		return len(o) == 3 && o[2].Name == "trips-go" && *o[2].Weight == 5 && o[2].Match.Path.Value == "/beta"
	})).Return(&gen.EndpointReleaseBinding{}, nil)

	testutil.CaptureStdout(t, func() {
		require.NoError(t, New(mc).TrafficSet(TrafficSetParams{
			Namespace: "acme", EndpointName: "trips", Environment: "development",
			Split: map[string]int32{"trips-python": 45, "trips-stale": 50, "trips-go": 5},
		}))
	})
}

func TestTrafficSet_Environment(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	expectBindings(mc, tripsBinding("development"))
	expectRelease(mc)
	mc.EXPECT().UpdateEndpointReleaseBinding(mock.Anything, "acme", "trips-development", mock.MatchedBy(func(b gen.EndpointReleaseBinding) bool {
		o := *b.Spec.BackendOverrides
		return len(o) == 3 && o[0].Name == "trips-python" && *o[0].Weight == 45 && *o[2].Weight == 5
	})).Return(&gen.EndpointReleaseBinding{}, nil)

	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, New(mc).TrafficSet(TrafficSetParams{
			Namespace: "acme", EndpointName: "trips", Environment: "development",
			Split: map[string]int32{"trips-python": 45, "trips-stale": 50, "trips-go": 5},
		}))
	})
	assert.Contains(t, out, "development: own split saved")
}

func TestTrafficSet_EnvironmentCopyKeepsShares(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	expectBindings(mc, tripsBinding("staging",
		gen.EndpointBackendOverride{Name: "trips-python", Weight: ptr.To[int32](40)},
		gen.EndpointBackendOverride{Name: "trips-stale", Weight: ptr.To[int32](50)},
		gen.EndpointBackendOverride{Name: "trips-go", Weight: ptr.To[int32](10)}))
	expectRelease(mc)
	mc.EXPECT().UpdateEndpointReleaseBinding(mock.Anything, "acme", "trips-staging", mock.MatchedBy(func(b gen.EndpointReleaseBinding) bool {
		o := *b.Spec.BackendOverrides
		return len(o) == 4 && *o[0].Weight == 40 && o[3].Name == "trips-shadow" && o[3].Mirror.Percent == 25
	})).Return(&gen.EndpointReleaseBinding{}, nil)

	testutil.CaptureStdout(t, func() {
		require.NoError(t, New(mc).TrafficSet(TrafficSetParams{
			Namespace: "acme", EndpointName: "trips", Environment: "staging",
			Split: map[string]int32{"trips-shadow": 25},
		}))
	})
}

func TestTrafficSet_EnvironmentSameAsDefaultStoresNothing(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	expectBindings(mc, tripsBinding("development", gen.EndpointBackendOverride{Name: "trips-go", Weight: ptr.To[int32](5)}))
	expectRelease(mc)
	mc.EXPECT().UpdateEndpointReleaseBinding(mock.Anything, "acme", "trips-development", mock.MatchedBy(func(b gen.EndpointReleaseBinding) bool {
		return b.Spec.BackendOverrides == nil
	})).Return(&gen.EndpointReleaseBinding{}, nil)

	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, New(mc).TrafficSet(TrafficSetParams{
			Namespace: "acme", EndpointName: "trips", Environment: "development",
			Split: map[string]int32{"trips-python": 50, "trips-stale": 50, "trips-go": 0},
		}))
	})
	assert.Contains(t, out, "same as the default split")
}

func TestTrafficSet_RefusesABadSplit(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	expectBindings(mc, tripsBinding("development"))
	expectRelease(mc)

	err := New(mc).TrafficSet(TrafficSetParams{
		Namespace: "acme", EndpointName: "trips", Environment: "development",
		Split: map[string]int32{"trips-python": 60, "trips-stale": 50, "trips-go": 0},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "add up to 110, not 100")
}

func TestTrafficSet_Default(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetEndpoint(mock.Anything, "acme", "trips").Return(tripsEndpoint(), nil)
	mc.EXPECT().UpdateEndpoint(mock.Anything, "acme", "trips", mock.MatchedBy(func(e gen.Endpoint) bool {
		r := e.Spec.BackendRefs
		return *r[0].Weight == 40 && *r[2].Weight == 10 && r[2].Match != nil && r[3].Mirror.Percent == 20
	})).Return(&gen.Endpoint{}, nil)

	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, New(mc).TrafficSet(TrafficSetParams{
			Namespace: "acme", EndpointName: "trips",
			Split: map[string]int32{"trips-python": 40, "trips-stale": 50, "trips-go": 10, "trips-shadow": 20},
		}))
	})
	assert.Contains(t, out, "Default split saved")
}

func TestTrafficSet_DryRunWritesNothing(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetEndpoint(mock.Anything, "acme", "trips").Return(tripsEndpoint(), nil)

	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, New(mc).TrafficSet(TrafficSetParams{
			Namespace: "acme", EndpointName: "trips", DryRun: true,
			Split: map[string]int32{"trips-python": 40, "trips-stale": 50, "trips-go": 10},
		}))
	})
	assert.Contains(t, out, "kind: Endpoint")
	assert.Contains(t, out, "weight: 40")
}

func TestTrafficReset(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	expectBindings(mc, tripsBinding("development", gen.EndpointBackendOverride{Name: "trips-go", Weight: ptr.To[int32](5)}))
	mc.EXPECT().UpdateEndpointReleaseBinding(mock.Anything, "acme", "trips-development", mock.MatchedBy(func(b gen.EndpointReleaseBinding) bool {
		return b.Spec.BackendOverrides == nil
	})).Return(&gen.EndpointReleaseBinding{}, nil)

	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, New(mc).TrafficReset(TrafficResetParams{Namespace: "acme", EndpointName: "trips", Environment: "development"}))
	})
	assert.Contains(t, out, "follows the default split")
}

func TestTrafficValidation(t *testing.T) {
	r := New(mocks.NewMockInterface(t))
	assert.Error(t, r.TrafficGet(TrafficGetParams{Namespace: "acme"}))
	assert.Error(t, r.TrafficSet(TrafficSetParams{Namespace: "acme", EndpointName: "trips"}))
	assert.Error(t, r.TrafficReset(TrafficResetParams{Namespace: "acme", EndpointName: "trips"}))
}

func TestParseSplit(t *testing.T) {
	got, err := parseSplit([]string{"trips-python=45", "trips-go=5%"})
	require.NoError(t, err)
	assert.Equal(t, map[string]int32{"trips-python": 45, "trips-go": 5}, got)

	for _, bad := range []string{"trips-go", "=5", "trips-go=101", "trips-go=-1", "trips-go=half"} {
		_, err := parseSplit([]string{bad})
		assert.Error(t, err, bad)
	}
}

func TestTrafficSet_EnvironmentKeepsMatchesTheReleaseIgnores(t *testing.T) {
	// trips-python has no match in trips-7; its override applies again once a
	// release gives it one.
	mc := mocks.NewMockInterface(t)
	expectBindings(mc, tripsBinding("development", gen.EndpointBackendOverride{Name: "trips-python", Match: &gen.EndpointMatch{
		Path: &gen.EndpointPathMatch{Value: "/beta"},
	}}))
	expectRelease(mc)
	mc.EXPECT().UpdateEndpointReleaseBinding(mock.Anything, "acme", "trips-development", mock.MatchedBy(func(b gen.EndpointReleaseBinding) bool {
		o := *b.Spec.BackendOverrides
		return o[0].Name == "trips-python" && o[0].Match != nil && o[0].Match.Path.Value == "/beta"
	})).Return(&gen.EndpointReleaseBinding{}, nil)

	testutil.CaptureStdout(t, func() {
		require.NoError(t, New(mc).TrafficSet(TrafficSetParams{
			Namespace: "acme", EndpointName: "trips", Environment: "development",
			Split: map[string]int32{"trips-python": 45, "trips-stale": 50, "trips-go": 5},
		}))
	})
}

func TestTrafficGet_AMatchEqualToTheReleaseIsNotOwn(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetEndpoint(mock.Anything, "acme", "trips").Return(tripsEndpoint(), nil)
	exact := gen.EndpointHeaderMatchTypeExact
	expectBindings(mc, tripsBinding("staging", gen.EndpointBackendOverride{Name: "trips-go", Match: &gen.EndpointMatch{
		Headers: &[]gen.EndpointHeaderMatch{{Name: "X-Preview", Type: &exact, Value: "true"}},
	}}))
	expectRelease(mc)

	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, New(mc).TrafficGet(TrafficGetParams{Namespace: "acme", EndpointName: "trips"}))
	})
	assert.Regexp(t, `staging\s+.*default split`, out)
	assert.NotContains(t, out, "staging's conditions")
}

func TestTrafficGet_ListsTheConditionsOfAnOlderRelease(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetEndpoint(mock.Anything, "acme", "trips").Return(tripsEndpoint(), nil)
	staging := tripsBinding("staging")
	staging.Spec.EndpointRelease = ptr.To("trips-6")
	expectBindings(mc, staging)
	older := tripsRefs()
	older[2].Match = &gen.EndpointMatch{Path: &gen.EndpointPathMatch{Value: "/old"}}
	mc.EXPECT().GetEndpointRelease(mock.Anything, "acme", "trips-6").
		Return(&gen.EndpointRelease{Spec: &gen.EndpointReleaseSpec{BackendRefs: older}}, nil)

	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, New(mc).TrafficGet(TrafficGetParams{Namespace: "acme", EndpointName: "trips"}))
	})
	assert.Contains(t, out, "Default conditions:\n  trips-go gets requests with header x-preview: true")
	assert.Contains(t, out, "staging's conditions:\n  trips-go gets requests with path /old")
}

func TestTrafficGetShowsAConditionThatDiffersOnlyByType(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetEndpoint(mock.Anything, "acme", "trips").Return(tripsEndpoint(), nil)
	regex := gen.EndpointHeaderMatchTypeRegularExpression
	expectBindings(mc,
		tripsBinding("staging", gen.EndpointBackendOverride{Name: "trips-go", Match: &gen.EndpointMatch{
			Headers: &[]gen.EndpointHeaderMatch{{Name: "x-preview", Type: &regex, Value: "true"}},
		}}),
		tripsBinding("production", gen.EndpointBackendOverride{Name: "trips-go", Match: &gen.EndpointMatch{
			Headers: &[]gen.EndpointHeaderMatch{{Name: "X-Preview", Value: "true"}},
		}}),
	)
	mc.EXPECT().GetEndpointRelease(mock.Anything, "acme", "trips-7").
		Return(&gen.EndpointRelease{Spec: &gen.EndpointReleaseSpec{BackendRefs: tripsRefs()}}, nil).Times(2)

	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, New(mc).TrafficGet(TrafficGetParams{Namespace: "acme", EndpointName: "trips"}))
	})
	assert.Contains(t, out, "staging's conditions:\n  trips-go gets requests with header x-preview matching true")
	assert.NotContains(t, out, "production's conditions", "the same header in another case selects the same requests")
}

func TestTrafficGetSaysWhenAnOwnSplitIsNotApplied(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetEndpoint(mock.Anything, "acme", "trips").Return(tripsEndpoint(), nil)
	expectBindings(mc, tripsBinding("staging",
		gen.EndpointBackendOverride{Name: "trips-python", Weight: ptr.To[int32](0)},
		gen.EndpointBackendOverride{Name: "trips-stale", Weight: ptr.To[int32](0)}))
	expectRelease(mc)

	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, New(mc).TrafficGet(TrafficGetParams{Namespace: "acme", EndpointName: "trips"}))
	})
	assert.Regexp(t, `staging\s+-\s+own split not applied: no backend has a share, so the previous route stays`, out)
	assert.NotContains(t, out, "trips-python 0% · trips-stale 0%")
}
