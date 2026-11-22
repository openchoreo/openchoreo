// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package revision

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

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

		// A binding holding its rollout has not evaluated its resources.
		{
			name: "a held rollout is Progressing, whatever ResourcesReady still says",
			obs: Observation{RenderedRelease: current, HoldConditions: []string{"ConnectionsResolved", "ResourceDependenciesReady"},
				Conditions: []metav1.Condition{
					cond("ConnectionsResolved", metav1.ConditionTrue, "AllConnectionsResolved", observedGen),
					cond("ResourceDependenciesReady", metav1.ConditionFalse, "ResourceDependenciesPending", observedGen),
					cond("ResourcesReady", metav1.ConditionTrue, "Ready", observedGen)}},
			want: openchoreov1alpha1.RevisionHealthProgressing,
		},
		{
			name: "a rollout no longer held is judged by ResourcesReady",
			obs: Observation{RenderedRelease: current, HoldConditions: []string{"ConnectionsResolved"},
				Conditions: []metav1.Condition{
					cond("ConnectionsResolved", metav1.ConditionFalse, "ConnectionsPending", observedGen-1),
					cond("ResourcesReady", metav1.ConditionTrue, "Ready", observedGen)}},
			want: openchoreov1alpha1.RevisionHealthHealthy,
		},
		{
			name: "a held rollout still reports an apply failure",
			obs: Observation{RenderedRelease: renderedRelease(3, metav1.ConditionFalse), HoldConditions: []string{"ConnectionsResolved"},
				Conditions: []metav1.Condition{
					cond("ConnectionsResolved", metav1.ConditionFalse, "ConnectionsPending", observedGen)}},
			want: openchoreov1alpha1.RevisionHealthFailed,
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
	s := NewReleaseBindingSnapshotter(c, newTestScheme(t), Options{KnownGoodWindow: window, Now: clock.Now})
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

func TestObserveHealth_RevisionNotYetMarkedActiveIsSkipped(t *testing.T) {
	ctx := context.Background()
	s, binding, c, _ := observeHealthFixture(t, 0)

	// The revision records the binding's spec, but writing its active status
	// failed: its health window has not started.
	r1 := getRBR(t, c, "frontend-development-r1")
	r1.Status.Active = false
	require.NoError(t, c.Status().Update(ctx, r1))

	_, err := s.ObserveHealth(ctx, binding, openchoreov1alpha1.RevisionHealthHealthy)
	require.NoError(t, err)
	r1 = getRBR(t, c, "frontend-development-r1")
	assert.Equal(t, openchoreov1alpha1.RevisionHealthProgressing, r1.Status.Health)
	assert.False(t, r1.Status.KnownGood)
	assert.Nil(t, r1.Status.HealthySince)
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
	s := NewReleaseBindingSnapshotter(c, newTestScheme(t), Options{Now: clock.Now})
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
