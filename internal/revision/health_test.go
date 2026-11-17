// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package revision

import (
	"context"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	"sigs.k8s.io/controller-runtime/pkg/event"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/controller/renderedrelease"
)

// ─────────────────────────────────────────────────────────────
// Observation.Health
// ─────────────────────────────────────────────────────────────

const observedGen = 5

func cond(conditionType string, status metav1.ConditionStatus, reason string, generation int64) metav1.Condition {
	return metav1.Condition{Type: conditionType, Status: status, Reason: reason, ObservedGeneration: generation}
}

// renderedRelease returns a RenderedRelease at generation 3 whose ResourcesApplied
// condition was reported for appliedGen with the given status.
func renderedRelease(appliedGen int64, applied metav1.ConditionStatus) *openchoreov1alpha1.RenderedRelease {
	rr := &openchoreov1alpha1.RenderedRelease{ObjectMeta: metav1.ObjectMeta{Generation: 3}}
	if appliedGen > 0 {
		rr.Status.Conditions = []metav1.Condition{
			cond(renderedrelease.ConditionResourcesApplied, applied, "Applied", appliedGen),
		}
	}
	return rr
}

func TestObservationHealth(t *testing.T) {
	current := renderedRelease(3, metav1.ConditionTrue)
	ready := func(status metav1.ConditionStatus, reason string) []metav1.Condition {
		return []metav1.Condition{cond("ResourcesReady", status, reason, observedGen)}
	}

	tests := []struct {
		name string
		obs  Observation
		want openchoreov1alpha1.RevisionHealth
	}{
		// ResourcesReady mapping, with current RenderedRelease health.
		{name: "Ready is Healthy", obs: Observation{RenderedRelease: current, Conditions: ready(metav1.ConditionTrue, "Ready")},
			want: openchoreov1alpha1.RevisionHealthHealthy},
		{name: "ReadyWithSuspendedResources is Healthy", obs: Observation{RenderedRelease: current, Conditions: ready(metav1.ConditionTrue, "ReadyWithSuspendedResources")},
			want: openchoreov1alpha1.RevisionHealthHealthy},
		{name: "JobCompleted is Healthy", obs: Observation{RenderedRelease: current, Conditions: ready(metav1.ConditionTrue, "JobCompleted")},
			want: openchoreov1alpha1.RevisionHealthHealthy},
		{name: "ResourcesProgressing is Progressing", obs: Observation{RenderedRelease: current, Conditions: ready(metav1.ConditionFalse, "ResourcesProgressing")},
			want: openchoreov1alpha1.RevisionHealthProgressing},
		{name: "JobRunning is Progressing", obs: Observation{RenderedRelease: current, Conditions: ready(metav1.ConditionFalse, "JobRunning")},
			want: openchoreov1alpha1.RevisionHealthProgressing},
		{name: "ResourcesDegraded is Degraded", obs: Observation{RenderedRelease: current, Conditions: ready(metav1.ConditionFalse, "ResourcesDegraded")},
			want: openchoreov1alpha1.RevisionHealthDegraded},
		{name: "ResourcesNotReady is Failed", obs: Observation{RenderedRelease: current, Conditions: ready(metav1.ConditionFalse, "ResourcesNotReady")},
			want: openchoreov1alpha1.RevisionHealthFailed},
		{name: "ResourceApplyFailed is Failed", obs: Observation{RenderedRelease: current, Conditions: ready(metav1.ConditionFalse, "ResourceApplyFailed")},
			want: openchoreov1alpha1.RevisionHealthFailed},
		{name: "JobFailed is Failed", obs: Observation{RenderedRelease: current, Conditions: ready(metav1.ConditionFalse, "JobFailed")},
			want: openchoreov1alpha1.RevisionHealthFailed},
		{name: "ResourcesUnknown is Unknown", obs: Observation{RenderedRelease: current, Conditions: ready(metav1.ConditionFalse, "ResourcesUnknown")},
			want: openchoreov1alpha1.RevisionHealthUnknown},
		{name: "missing ResourcesReady is Unknown", obs: Observation{RenderedRelease: current},
			want: openchoreov1alpha1.RevisionHealthUnknown},

		// Freshness: health that describes an earlier spec or generation.
		{
			name: "ResourcesReady computed for an earlier binding generation is Progressing",
			obs: Observation{RenderedRelease: current, Conditions: []metav1.Condition{
				cond("ResourcesReady", metav1.ConditionTrue, "Ready", observedGen-1)}},
			want: openchoreov1alpha1.RevisionHealthProgressing,
		},
		{
			name: "old pods still serving: RenderedRelease health from an earlier generation is Progressing",
			obs:  Observation{RenderedRelease: renderedRelease(2, metav1.ConditionTrue), Conditions: ready(metav1.ConditionTrue, "Ready")},
			want: openchoreov1alpha1.RevisionHealthProgressing,
		},
		{
			name: "RenderedRelease not yet reported on is Progressing",
			obs:  Observation{RenderedRelease: renderedRelease(0, ""), Conditions: ready(metav1.ConditionTrue, "Ready")},
			want: openchoreov1alpha1.RevisionHealthProgressing,
		},
		{
			name: "RenderedRelease not created yet is Progressing",
			obs:  Observation{},
			want: openchoreov1alpha1.RevisionHealthProgressing,
		},
		{
			name: "apply failure for the current generation is Failed",
			obs:  Observation{RenderedRelease: renderedRelease(3, metav1.ConditionFalse), Conditions: ready(metav1.ConditionTrue, "Ready")},
			want: openchoreov1alpha1.RevisionHealthFailed,
		},

		// The spec could not be rendered or written.
		{
			name: "RenderingFailed is Failed",
			obs: Observation{RenderedRelease: current, SyncedCondition: "ReleaseSynced", Conditions: []metav1.Condition{
				cond("ReleaseSynced", metav1.ConditionFalse, "RenderingFailed", observedGen)}},
			want: openchoreov1alpha1.RevisionHealthFailed,
		},
		{
			name: "ReleaseOwnershipConflict is Failed, even before a RenderedRelease exists",
			obs: Observation{SyncedCondition: "Synced", Conditions: []metav1.Condition{
				cond("Synced", metav1.ConditionFalse, "ReleaseOwnershipConflict", observedGen)}},
			want: openchoreov1alpha1.RevisionHealthFailed,
		},
		{
			name: "a rendering failure from an earlier generation does not fail this spec",
			obs: Observation{RenderedRelease: current, SyncedCondition: "ReleaseSynced", Conditions: []metav1.Condition{
				cond("ReleaseSynced", metav1.ConditionFalse, "RenderingFailed", observedGen-1),
				cond("ResourcesReady", metav1.ConditionTrue, "Ready", observedGen)}},
			want: openchoreov1alpha1.RevisionHealthHealthy,
		},

		// Undeploy.
		{
			name: "Undeploy with resources still present is Progressing",
			obs:  Observation{Undeploy: true, RenderedRelease: current},
			want: openchoreov1alpha1.RevisionHealthProgressing,
		},
		{
			name: "Undeploy with resources gone is Healthy",
			obs:  Observation{Undeploy: true},
			want: openchoreov1alpha1.RevisionHealthHealthy,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.obs.Generation = observedGen
			assert.Equal(t, tt.want, tt.obs.Health())
		})
	}
}

// ─────────────────────────────────────────────────────────────
// ObserveHealth
// ─────────────────────────────────────────────────────────────

// fakeClock is a settable clock for Options.Now.
type fakeClock struct{ now time.Time }

func (c *fakeClock) Now() time.Time          { return c.now }
func (c *fakeClock) Advance(d time.Duration) { c.now = c.now.Add(d) }

func newClock() *fakeClock {
	return &fakeClock{now: time.Date(2026, 10, 12, 9, 0, 0, 0, time.UTC)}
}

// observeHealthFixture records r1 for a binding and returns what the tests need.
func observeHealthFixture(t *testing.T, window time.Duration) (*Snapshotter[*openchoreov1alpha1.ReleaseBinding, *openchoreov1alpha1.ReleaseBindingRevision], *openchoreov1alpha1.ReleaseBinding, client.Client, *fakeClock) {
	t.Helper()
	clock := newClock()
	binding := newBinding("frontend-development")
	c := newTestClient(t, binding.DeepCopy())
	s := NewReleaseBindingSnapshotter(c, newTestScheme(t), Options{KnownGoodWindow: window, Now: clock.Now, HistoryLimit: testHistoryLimit})
	_, err := s.Record(context.Background(), binding)
	require.NoError(t, err)
	return s, binding, c, clock
}

func TestObserveHealth_KnownGoodAfterWindow(t *testing.T) {
	ctx := context.Background()
	s, binding, c, clock := observeHealthFixture(t, 60*time.Second)

	requeue, err := s.ObserveHealth(ctx, binding, openchoreov1alpha1.RevisionHealthHealthy)
	require.NoError(t, err)
	assert.Equal(t, 60*time.Second, requeue, "the full window is left on the first Healthy observation")
	r1 := getRBR(t, c, "frontend-development-r1")
	assert.Equal(t, openchoreov1alpha1.RevisionHealthHealthy, r1.Status.Health)
	require.NotNil(t, r1.Status.HealthySince)
	assert.False(t, r1.Status.KnownGood)

	clock.Advance(45 * time.Second)
	requeue, err = s.ObserveHealth(ctx, binding, openchoreov1alpha1.RevisionHealthHealthy)
	require.NoError(t, err)
	assert.Equal(t, 15*time.Second, requeue, "requeue for the rest of the window, never sleep")
	assert.False(t, getRBR(t, c, "frontend-development-r1").Status.KnownGood)

	clock.Advance(15 * time.Second)
	requeue, err = s.ObserveHealth(ctx, binding, openchoreov1alpha1.RevisionHealthHealthy)
	require.NoError(t, err)
	assert.Zero(t, requeue)
	assert.True(t, getRBR(t, c, "frontend-development-r1").Status.KnownGood)
}

func TestObserveHealth_LeavingHealthyRestartsTheWindow(t *testing.T) {
	ctx := context.Background()
	s, binding, c, clock := observeHealthFixture(t, 60*time.Second)

	_, err := s.ObserveHealth(ctx, binding, openchoreov1alpha1.RevisionHealthHealthy)
	require.NoError(t, err)
	clock.Advance(50 * time.Second)

	_, err = s.ObserveHealth(ctx, binding, openchoreov1alpha1.RevisionHealthDegraded)
	require.NoError(t, err)
	r1 := getRBR(t, c, "frontend-development-r1")
	assert.Equal(t, openchoreov1alpha1.RevisionHealthDegraded, r1.Status.Health)
	assert.Nil(t, r1.Status.HealthySince, "leaving Healthy clears healthySince")

	clock.Advance(20 * time.Second)
	requeue, err := s.ObserveHealth(ctx, binding, openchoreov1alpha1.RevisionHealthHealthy)
	require.NoError(t, err)
	assert.Equal(t, 60*time.Second, requeue, "the window starts again from the new Healthy observation")
	assert.False(t, getRBR(t, c, "frontend-development-r1").Status.KnownGood)
}

func TestObserveHealth_KnownGoodIsSticky(t *testing.T) {
	ctx := context.Background()
	s, binding, c, _ := observeHealthFixture(t, 0)

	_, err := s.ObserveHealth(ctx, binding, openchoreov1alpha1.RevisionHealthHealthy)
	require.NoError(t, err)
	require.True(t, getRBR(t, c, "frontend-development-r1").Status.KnownGood, "a zero window marks the first Healthy observation")

	for _, h := range []openchoreov1alpha1.RevisionHealth{
		openchoreov1alpha1.RevisionHealthDegraded, openchoreov1alpha1.RevisionHealthFailed, openchoreov1alpha1.RevisionHealthUnknown,
	} {
		_, err := s.ObserveHealth(ctx, binding, h)
		require.NoError(t, err)
		r1 := getRBR(t, c, "frontend-development-r1")
		assert.Equal(t, h, r1.Status.Health)
		assert.True(t, r1.Status.KnownGood, "known-good must stay true after %s", h)
	}
}

func TestObserveHealth_ObservedAtChangesOnlyWithHealth(t *testing.T) {
	ctx := context.Background()
	s, binding, c, clock := observeHealthFixture(t, time.Hour)

	_, err := s.ObserveHealth(ctx, binding, openchoreov1alpha1.RevisionHealthDegraded)
	require.NoError(t, err)
	first := getRBR(t, c, "frontend-development-r1")

	clock.Advance(time.Minute)
	_, err = s.ObserveHealth(ctx, binding, openchoreov1alpha1.RevisionHealthDegraded)
	require.NoError(t, err)
	again := getRBR(t, c, "frontend-development-r1")
	assert.Equal(t, first.ResourceVersion, again.ResourceVersion, "the same health writes nothing")
	assert.Equal(t, first.Status.ObservedAt, again.Status.ObservedAt)
}

func TestObserveHealth_InactiveRevisionIsFrozen(t *testing.T) {
	ctx := context.Background()
	s, binding, c, clock := observeHealthFixture(t, 0)

	_, err := s.ObserveHealth(ctx, binding, openchoreov1alpha1.RevisionHealthHealthy)
	require.NoError(t, err)

	// A new deploy makes r2 active; r1 keeps the health it had.
	bump(binding, newRelease)
	_, err = s.Record(ctx, binding)
	require.NoError(t, err)
	r1Before := getRBR(t, c, "frontend-development-r1")
	r2 := getRBR(t, c, "frontend-development-r2")
	assert.Equal(t, openchoreov1alpha1.RevisionHealthProgressing, r2.Status.Health, "a new revision starts at Progressing")

	clock.Advance(time.Minute)
	_, err = s.ObserveHealth(ctx, binding, openchoreov1alpha1.RevisionHealthDegraded)
	require.NoError(t, err)

	r1 := getRBR(t, c, "frontend-development-r1")
	assert.False(t, r1.Status.Active)
	assert.Equal(t, openchoreov1alpha1.RevisionHealthHealthy, r1.Status.Health, "r1 is frozen at its last observed health")
	assert.True(t, r1.Status.KnownGood)
	assert.Equal(t, r1Before.Status, r1.Status)
	assert.Equal(t, openchoreov1alpha1.RevisionHealthDegraded, getRBR(t, c, "frontend-development-r2").Status.Health)
}

func TestObserveHealth_UndeployRevisionNeverKnownGood(t *testing.T) {
	ctx := context.Background()
	s, binding, c, _ := observeHealthFixture(t, 0)

	bump(binding, undeploy)
	_, err := s.Record(ctx, binding)
	require.NoError(t, err)

	requeue, err := s.ObserveHealth(ctx, binding, openchoreov1alpha1.RevisionHealthHealthy)
	require.NoError(t, err)
	assert.Zero(t, requeue, "nothing to wait for")
	r2 := getRBR(t, c, "frontend-development-r2")
	assert.Equal(t, openchoreov1alpha1.RevisionHealthHealthy, r2.Status.Health)
	assert.False(t, r2.Status.KnownGood)
}

func TestObserveHealth_UnrecordedSpecIsIgnored(t *testing.T) {
	ctx := context.Background()
	s, binding, c, _ := observeHealthFixture(t, 0)

	// The binding has moved on, but its revision is not written yet.
	changed := binding.DeepCopy()
	bump(changed, newRelease)

	_, err := s.ObserveHealth(ctx, changed, openchoreov1alpha1.RevisionHealthFailed)
	require.NoError(t, err)
	assert.Equal(t, openchoreov1alpha1.RevisionHealthProgressing, getRBR(t, c, "frontend-development-r1").Status.Health,
		"health of an unrecorded spec must not land on the previous revision")
}

func TestObserveHealth_RetriesOnConflict(t *testing.T) {
	ctx := context.Background()
	clock := newClock()
	binding := newBinding("frontend-development")

	conflicts := 0
	c := fake.NewClientBuilder().
		WithScheme(newTestScheme(t)).
		WithObjects(binding.DeepCopy()).
		WithStatusSubresource(&openchoreov1alpha1.ReleaseBindingRevision{}).
		WithInterceptorFuncs(interceptor.Funcs{
			SubResourceUpdate: func(ctx context.Context, c client.Client, sub string, obj client.Object, opts ...client.SubResourceUpdateOption) error {
				rev, ok := obj.(*openchoreov1alpha1.ReleaseBindingRevision)
				if ok && rev.Status.Health == openchoreov1alpha1.RevisionHealthHealthy && conflicts == 0 {
					conflicts++
					// Someone else changes the revision first.
					other := &openchoreov1alpha1.ReleaseBindingRevision{}
					if err := c.Get(ctx, client.ObjectKeyFromObject(rev), other); err != nil {
						return err
					}
					other.Annotations["example.com/concurrent"] = "yes"
					if err := c.Update(ctx, other); err != nil {
						return err
					}
					return apierrors.NewConflict(schema.GroupResource{Group: "openchoreo.dev", Resource: "releasebindingrevisions"},
						rev.Name, fmt.Errorf("the object has been modified"))
				}
				return c.SubResource(sub).Update(ctx, obj, opts...)
			},
		}).
		Build()
	s := NewReleaseBindingSnapshotter(c, newTestScheme(t), Options{Now: clock.Now, HistoryLimit: testHistoryLimit})
	_, err := s.Record(ctx, binding)
	require.NoError(t, err)

	_, err = s.ObserveHealth(ctx, binding, openchoreov1alpha1.RevisionHealthHealthy)
	require.NoError(t, err)
	assert.Equal(t, 1, conflicts)

	r1 := getRBR(t, c, "frontend-development-r1")
	assert.Equal(t, openchoreov1alpha1.RevisionHealthHealthy, r1.Status.Health, "the write is retried on the fresh object")
	assert.True(t, r1.Status.KnownGood)
	assert.Equal(t, "yes", r1.Annotations["example.com/concurrent"], "the concurrent change survives")
}

// ─────────────────────────────────────────────────────────────
// History limit through Record
// ─────────────────────────────────────────────────────────────

func TestRecord_EvictsBeyondHistoryLimit(t *testing.T) {
	ctx := context.Background()
	binding := newBinding("frontend-development")
	c := newTestClient(t, binding.DeepCopy())
	recorder := record.NewFakeRecorder(10)
	// Two old revisions are kept besides the active one.
	s := NewReleaseBindingSnapshotter(c, newTestScheme(t), Options{HistoryLimit: 2, Recorder: recorder})

	// r1 becomes known-good, then four more revisions follow.
	_, err := s.Record(ctx, binding)
	require.NoError(t, err)
	_, err = s.ObserveHealth(ctx, binding, openchoreov1alpha1.RevisionHealthHealthy)
	require.NoError(t, err)
	for i := 2; i <= 5; i++ {
		replicas := fmt.Sprintf(`{"replicas":%d}`, i)
		bump(binding, func(spec *openchoreov1alpha1.ReleaseBindingSpec) {
			spec.ComponentTypeEnvironmentConfigs = raw(replicas)
		})
		_, err = s.Record(ctx, binding)
		require.NoError(t, err)
	}

	stored := listRBRs(t, c)
	numbers := make([]int64, 0, len(stored))
	for _, rev := range stored {
		numbers = append(numbers, rev.Spec.Revision)
	}
	assert.ElementsMatch(t, []int64{
		1, // the newest (and only) known-good revision
		4,
		5, // active
	}, numbers)
	assert.Empty(t, recorder.Events, "the limit was met, so no event")
}

func TestRecord_ProtectedRevisionsAboveLimitEmitEvent(t *testing.T) {
	ctx := context.Background()
	binding := newBinding("frontend-development")
	c := newTestClient(t, binding.DeepCopy())
	recorder := record.NewFakeRecorder(10)
	s := NewReleaseBindingSnapshotter(c, newTestScheme(t), Options{HistoryLimit: 1, Recorder: recorder})

	_, err := s.Record(ctx, binding)
	require.NoError(t, err)
	_, err = s.ObserveHealth(ctx, binding, openchoreov1alpha1.RevisionHealthHealthy)
	require.NoError(t, err)

	recordRevisions(t, s, binding, 1)
	assert.Empty(t, recorder.Events, "one old revision is within the limit")

	recordRevisions(t, s, binding, 1)
	assert.Equal(t, []int64{1, 2, 3}, revisionNumbers(t, c),
		"r1 (newest known-good) and r2 (the undo target) are both spared, r3 is active")
	require.Len(t, recorder.Events, 1)
	assert.Contains(t, <-recorder.Events, EventReasonRevisionHistoryLimitExceeded)

	// Reconciling again without a new revision does not repeat the event.
	_, err = s.Record(ctx, binding)
	require.NoError(t, err)
	assert.Empty(t, recorder.Events)
}

// recordRevisions records n new revisions of binding, each with a spec no
// earlier revision recorded.
func recordRevisions(t *testing.T, s *Snapshotter[*openchoreov1alpha1.ReleaseBinding, *openchoreov1alpha1.ReleaseBindingRevision],
	binding *openchoreov1alpha1.ReleaseBinding, n int) {
	t.Helper()
	for range n {
		replicas := fmt.Sprintf(`{"replicas":%d}`, binding.Generation+1)
		bump(binding, func(spec *openchoreov1alpha1.ReleaseBindingSpec) {
			spec.ComponentTypeEnvironmentConfigs = raw(replicas)
		})
		_, err := s.Record(context.Background(), binding)
		require.NoError(t, err)
	}
}

// revisionNumbers returns the numbers of the stored revisions, ascending.
func revisionNumbers(t *testing.T, c client.Client) []int64 {
	t.Helper()
	stored := listRBRs(t, c)
	numbers := make([]int64, 0, len(stored))
	for _, rev := range stored {
		numbers = append(numbers, rev.Spec.Revision)
	}
	slices.Sort(numbers)
	return numbers
}

// numberRange returns from, from+1, ..., to.
func numberRange(from, to int64) []int64 {
	numbers := make([]int64, 0, to-from+1)
	for n := from; n <= to; n++ {
		numbers = append(numbers, n)
	}
	return numbers
}

// testHistoryLimit is the history limit of tests that are not about the limit:
// high enough that nothing they record is trimmed.
const testHistoryLimit = 100

// The default limit keeps ten old revisions besides the active one.
func TestRecord_DefaultLimitKeepsTenOldRevisions(t *testing.T) {
	require.Equal(t, 10, DefaultHistoryLimit)
	binding := newBinding("frontend-development")
	c := newTestClient(t, binding.DeepCopy())
	s := NewReleaseBindingSnapshotter(c, newTestScheme(t), Options{HistoryLimit: DefaultHistoryLimit})

	recordRevisions(t, s, binding, 11)
	assert.Equal(t, numberRange(1, 11), revisionNumbers(t, c), "ten old revisions and the active one")

	recordRevisions(t, s, binding, 2)
	assert.Equal(t, numberRange(3, 13), revisionNumbers(t, c))
}

// A limit of 0 keeps no old revision: only the active one.
func TestRecord_ZeroLimitKeepsOnlyTheActiveRevision(t *testing.T) {
	binding := newBinding("frontend-development")
	c := newTestClient(t, binding.DeepCopy())
	s := NewReleaseBindingSnapshotter(c, newTestScheme(t), Options{HistoryLimit: 0})

	recordRevisions(t, s, binding, 5)

	assert.Equal(t, []int64{5}, revisionNumbers(t, c))
}

// At a limit of 0 even the newest known-good revision and the undo target go.
func TestRecord_ZeroLimitDeletesKnownGoodAndUndoTarget(t *testing.T) {
	ctx := context.Background()
	binding := newBinding("frontend-development")
	c := newTestClient(t, binding.DeepCopy())
	recorder := record.NewFakeRecorder(10)
	s := NewReleaseBindingSnapshotter(c, newTestScheme(t), Options{HistoryLimit: 0, Recorder: recorder})

	_, err := s.Record(ctx, binding)
	require.NoError(t, err)
	_, err = s.ObserveHealth(ctx, binding, openchoreov1alpha1.RevisionHealthHealthy)
	require.NoError(t, err)

	bump(binding, newRelease)
	_, err = s.Record(ctx, binding)
	require.NoError(t, err)

	assert.Equal(t, []int64{2}, revisionNumbers(t, c), "r1 was known-good and the undo target")
	assert.Empty(t, recorder.Events)
}

// The limit counts old revisions: the active one is kept on top of it.
func TestRecord_BindingLimitDeletesOldestFirst(t *testing.T) {
	binding := newBinding("frontend-development")
	binding.Spec.RevisionHistoryLimit = ptr.To(int32(20))
	c := newTestClient(t, binding.DeepCopy())
	s := NewReleaseBindingSnapshotter(c, newTestScheme(t), Options{HistoryLimit: DefaultHistoryLimit})

	recordRevisions(t, s, binding, 21)
	assert.Equal(t, numberRange(1, 21), revisionNumbers(t, c), "20 old revisions and the active one: at the limit")

	recordRevisions(t, s, binding, 1)
	assert.Equal(t, numberRange(2, 22), revisionNumbers(t, c), "r1 is deleted first")

	recordRevisions(t, s, binding, 4)
	assert.Equal(t, numberRange(6, 26), revisionNumbers(t, c), "then r2, r3, r4 and r5")
}

func TestRecord_BindingLimitOverridesOptionsLimit(t *testing.T) {
	tests := []struct {
		name  string
		limit *int32
		want  []int64
	}{
		{name: "unset uses the Options limit", limit: nil, want: numberRange(4, 8)},
		{name: "zero keeps no old revision", limit: ptr.To(int32(0)), want: numberRange(8, 8)},
		{name: "a higher limit keeps more", limit: ptr.To(int32(6)), want: numberRange(2, 8)},
		{name: "a lower limit keeps fewer", limit: ptr.To(int32(2)), want: numberRange(6, 8)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			binding := newBinding("frontend-development")
			binding.Spec.RevisionHistoryLimit = tt.limit
			c := newTestClient(t, binding.DeepCopy())
			s := NewReleaseBindingSnapshotter(c, newTestScheme(t), Options{HistoryLimit: 4})

			recordRevisions(t, s, binding, 8)

			assert.Equal(t, tt.want, revisionNumbers(t, c))
		})
	}
}

// Changing the limit is not a change to what is deployed: it creates no
// revision, and lowering it trims the history on the next Record.
func TestRecord_ChangingTheLimitCreatesNoRevisionAndTrims(t *testing.T) {
	ctx := context.Background()
	binding := newBinding("frontend-development")
	binding.Spec.RevisionHistoryLimit = ptr.To(int32(10))
	c := newTestClient(t, binding.DeepCopy())
	s := NewReleaseBindingSnapshotter(c, newTestScheme(t), Options{})

	recordRevisions(t, s, binding, 6)
	require.Equal(t, numberRange(1, 6), revisionNumbers(t, c))

	bump(binding, func(spec *openchoreov1alpha1.ReleaseBindingSpec) {
		spec.RevisionHistoryLimit = ptr.To(int32(2))
	})
	res, err := s.Record(ctx, binding)
	require.NoError(t, err)
	assert.False(t, res.Created, "the limit is not recorded, so no revision is created")
	assert.False(t, res.Reactivated)
	assert.Equal(t, int64(6), res.Number)
	assert.Equal(t, numberRange(4, 6), revisionNumbers(t, c), "two old revisions and the active one remain")

	bump(binding, func(spec *openchoreov1alpha1.ReleaseBindingSpec) {
		spec.RevisionHistoryLimit = nil
	})
	res, err = s.Record(ctx, binding)
	require.NoError(t, err)
	assert.False(t, res.Created)
	assert.Equal(t, []int64{6}, revisionNumbers(t, c), "unset falls back to the Options limit of 0")
}

// A revision stores and hashes the binding spec without the limit.
func TestRecord_RevisionDoesNotRecordTheLimit(t *testing.T) {
	ctx := context.Background()
	binding := newBinding("frontend-development")
	withoutLimit, err := ReleaseBindingSpecHash(&binding.Spec)
	require.NoError(t, err)

	binding.Spec.RevisionHistoryLimit = ptr.To(int32(5))
	withLimit, err := ReleaseBindingSpecHash(&binding.Spec)
	require.NoError(t, err)
	assert.Equal(t, withoutLimit, withLimit, "the limit is not part of the hash")

	c := newTestClient(t, binding.DeepCopy())
	_, err = NewReleaseBindingSnapshotter(c, newTestScheme(t), Options{}).Record(ctx, binding)
	require.NoError(t, err)

	stored := listRBRs(t, c)
	require.Len(t, stored, 1)
	assert.Nil(t, stored[0].Spec.BindingSpec.RevisionHistoryLimit)
	assert.Equal(t, withoutLimit, stored[0].Spec.SnapshotHash)
	assert.Equal(t, ptr.To(int32(5)), binding.Spec.RevisionHistoryLimit, "the binding itself is not modified")
}

func TestResourceReleaseBindingSpecHash_IgnoresTheLimit(t *testing.T) {
	spec := openchoreov1alpha1.ResourceReleaseBindingSpec{Environment: "development", ResourceRelease: "db-abc"}
	spec.Owner.ProjectName, spec.Owner.ResourceName = "shop", "db"
	withoutLimit, err := ResourceReleaseBindingSpecHash(&spec)
	require.NoError(t, err)

	spec.RevisionHistoryLimit = ptr.To(int32(3))
	withLimit, err := ResourceReleaseBindingSpecHash(&spec)
	require.NoError(t, err)
	assert.Equal(t, withoutLimit, withLimit)
	assert.Nil(t, ResourceReleaseBindingRecordedSpec(&spec).RevisionHistoryLimit)
	assert.NotNil(t, spec.RevisionHistoryLimit, "the spec itself is not modified")
}

func TestEnvironmentHistoryLimit(t *testing.T) {
	env := &openchoreov1alpha1.Environment{}
	assert.Equal(t, 7, EnvironmentHistoryLimit(nil, 7), "no Environment: the controller's limit")
	assert.Equal(t, 7, EnvironmentHistoryLimit(env, 7), "unset on the Environment: the controller's limit")

	env.Spec.ComponentRevisionHistoryLimit = ptr.To(int32(3))
	assert.Equal(t, 3, EnvironmentHistoryLimit(env, 7))

	env.Spec.ComponentRevisionHistoryLimit = ptr.To(int32(0))
	assert.Equal(t, 0, EnvironmentHistoryLimit(env, 7), "an explicit 0 is a limit, not unset")
}

func TestEnvironmentHistoryLimitChanged(t *testing.T) {
	env := func(limit *int32) *openchoreov1alpha1.Environment {
		e := &openchoreov1alpha1.Environment{}
		e.Spec.ComponentRevisionHistoryLimit = limit
		return e
	}
	p := EnvironmentHistoryLimitChanged()
	tests := []struct {
		name     string
		old, new *int32
		want     bool
	}{
		{name: "unset to unset", want: false},
		{name: "unset to set", new: ptr.To(int32(3)), want: true},
		{name: "set to unset", old: ptr.To(int32(3)), want: true},
		{name: "unset to zero", new: ptr.To(int32(0)), want: true},
		{name: "same value", old: ptr.To(int32(3)), new: ptr.To(int32(3)), want: false},
		{name: "different value", old: ptr.To(int32(3)), new: ptr.To(int32(4)), want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, p.Update(event.UpdateEvent{ObjectOld: env(tt.old), ObjectNew: env(tt.new)}))
		})
	}
	assert.False(t, p.Create(event.CreateEvent{Object: env(ptr.To(int32(3)))}))
	assert.False(t, p.Delete(event.DeleteEvent{Object: env(ptr.To(int32(3)))}))
	assert.False(t, p.Update(event.UpdateEvent{ObjectOld: &openchoreov1alpha1.ReleaseBinding{}, ObjectNew: env(nil)}))
}

func TestRecord_KeepAnnotationDoesNotProtect(t *testing.T) {
	ctx := context.Background()
	binding := newBinding("frontend-development")
	// openchoreo.dev/keep is no longer honored: a revision annotated with it is
	// trimmed like any other. A user annotates the revision, a main-resource
	// write newTestClient forbids to the Snapshotter; this test needs a client
	// that allows it.
	c := fake.NewClientBuilder().
		WithScheme(newTestScheme(t)).
		WithObjects(binding.DeepCopy()).
		WithStatusSubresource(&openchoreov1alpha1.ReleaseBindingRevision{}).
		Build()
	s := NewReleaseBindingSnapshotter(c, newTestScheme(t), Options{HistoryLimit: 2})

	_, err := s.Record(ctx, binding)
	require.NoError(t, err)
	r1 := getRBR(t, c, "frontend-development-r1")
	r1.Annotations["openchoreo.dev/keep"] = "true"
	require.NoError(t, c.Update(ctx, r1))

	for i := 2; i <= 4; i++ {
		replicas := fmt.Sprintf(`{"replicas":%d}`, i)
		bump(binding, func(spec *openchoreov1alpha1.ReleaseBindingSpec) {
			spec.ComponentTypeEnvironmentConfigs = raw(replicas)
		})
		_, err = s.Record(ctx, binding)
		require.NoError(t, err)
	}

	stored := listRBRs(t, c)
	numbers := make([]int64, 0, len(stored))
	for _, rev := range stored {
		numbers = append(numbers, rev.Spec.Revision)
	}
	// r4 is active and r3, the revision active before r4, is the target of an
	// undo; r2 is kept to fill the limit of 2. r1 goes despite its annotation.
	assert.ElementsMatch(t, []int64{2, 3, 4}, numbers)
}
