// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package revision

import (
	"context"
	"fmt"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/labels"
)

const (
	// firstRelease and secondRelease are the ComponentReleases the test bindings move between.
	firstRelease  = "frontend-abc123"
	secondRelease = "frontend-def456"

	// secondResourceRelease is the ResourceRelease the test resource binding advances to.
	secondResourceRelease = "db-def456"
)

// test helpers

func newTestScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	require.NoError(t, openchoreov1alpha1.AddToScheme(s))
	return s
}

// newTestClient returns a fake client with status subresources for both revision
// kinds. Revision specs may only be written by Create: a main-resource Update of
// a revision fails the test, and so does a Patch that changes its spec. Patching
// revision metadata (making a revision active again) is allowed.
func newTestClient(t *testing.T, objs ...client.Object) client.Client {
	t.Helper()
	rejectRevisionWrite := func(obj client.Object, verb string) error {
		switch obj.(type) {
		case *openchoreov1alpha1.ReleaseBindingRevision, *openchoreov1alpha1.ResourceReleaseBindingRevision:
			t.Errorf("unexpected %s of revision %q outside the status subresource", verb, obj.GetName())
			return fmt.Errorf("revision %s is not allowed", verb)
		}
		return nil
	}
	return fake.NewClientBuilder().
		WithScheme(newTestScheme(t)).
		WithObjects(objs...).
		WithStatusSubresource(&openchoreov1alpha1.ReleaseBindingRevision{}, &openchoreov1alpha1.ResourceReleaseBindingRevision{}).
		WithInterceptorFuncs(interceptor.Funcs{
			Update: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.UpdateOption) error {
				if err := rejectRevisionWrite(obj, "update"); err != nil {
					return err
				}
				return c.Update(ctx, obj, opts...)
			},
			Patch: func(ctx context.Context, c client.WithWatch, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
				before, isRevision := revisionSpec(ctx, c, obj)
				if err := c.Patch(ctx, obj, patch, opts...); err != nil {
					return err
				}
				if after, _ := revisionSpec(ctx, c, obj); isRevision && !reflect.DeepEqual(before, after) {
					t.Errorf("unexpected change to the spec of revision %q", obj.GetName())
				}
				return nil
			},
		}).
		Build()
}

// revisionSpec returns the stored spec of obj when it is a revision.
func revisionSpec(ctx context.Context, c client.Reader, obj client.Object) (any, bool) {
	switch obj.(type) {
	case *openchoreov1alpha1.ReleaseBindingRevision:
		stored := &openchoreov1alpha1.ReleaseBindingRevision{}
		if err := c.Get(ctx, client.ObjectKeyFromObject(obj), stored); err != nil {
			return nil, true
		}
		return stored.Spec, true
	case *openchoreov1alpha1.ResourceReleaseBindingRevision:
		stored := &openchoreov1alpha1.ResourceReleaseBindingRevision{}
		if err := c.Get(ctx, client.ObjectKeyFromObject(obj), stored); err != nil {
			return nil, true
		}
		return stored.Spec, true
	}
	return nil, false
}

// newBinding returns a ReleaseBinding in development bound to firstRelease.
func newBinding(name string) *openchoreov1alpha1.ReleaseBinding {
	return &openchoreov1alpha1.ReleaseBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:       name,
			Namespace:  "default",
			UID:        types.UID("uid-" + name),
			Generation: 1,
		},
		Spec: openchoreov1alpha1.ReleaseBindingSpec{
			Owner:       openchoreov1alpha1.ReleaseBindingOwner{ProjectName: "doclet", ComponentName: "frontend"},
			Environment: "development",
			ReleaseName: firstRelease,
			State:       openchoreov1alpha1.ReleaseStateActive,
		},
	}
}

// bump applies a spec change to the in-memory binding and advances its
// generation, as the apiserver would.
func bump(b *openchoreov1alpha1.ReleaseBinding, mutate func(s *openchoreov1alpha1.ReleaseBindingSpec)) {
	mutate(&b.Spec)
	b.Generation++
}

// revisionKey matches "{binding}-r{N}": the tests' way of naming the revision
// numbered N of a binding, whatever its object name ({release}-r{N}).
var revisionKey = regexp.MustCompile(`^(.*)-r(\d+)$`)

// getRBR returns the revision with object name key, or else, for a key
// "{binding}-r{N}", the revision numbered N owned by the binding named binding.
func getRBR(t *testing.T, c client.Client, key string) *openchoreov1alpha1.ReleaseBindingRevision {
	t.Helper()
	rev := &openchoreov1alpha1.ReleaseBindingRevision{}
	if err := c.Get(context.Background(), client.ObjectKey{Namespace: "default", Name: key}, rev); err == nil {
		return rev
	}
	m := revisionKey.FindStringSubmatch(key)
	require.NotNil(t, m, "no revision %q", key)
	n, err := strconv.ParseInt(m[2], 10, 64)
	require.NoError(t, err)
	for _, r := range listRBRs(t, c) {
		if owner := metav1.GetControllerOf(&r); owner != nil && owner.Name == m[1] && r.Spec.Revision == n {
			return &r
		}
	}
	require.Failf(t, "revision not found", "no revision %d of binding %q", n, m[1])
	return nil
}

func listRBRs(t *testing.T, c client.Client) []openchoreov1alpha1.ReleaseBindingRevision {
	t.Helper()
	var list openchoreov1alpha1.ReleaseBindingRevisionList
	require.NoError(t, c.List(context.Background(), &list, client.InNamespace("default")))
	return list.Items
}

// ─────────────────────────────────────────────────────────────
// ReleaseBinding snapshotter
// ─────────────────────────────────────────────────────────────

func TestReleaseBindingSnapshotter_FirstRevision(t *testing.T) {
	ctx := context.Background()
	binding := newBinding("frontend-development")
	c := newTestClient(t)
	s := NewReleaseBindingSnapshotter(c, newTestScheme(t), Options{HistoryLimit: testHistoryLimit})

	res, err := s.Record(ctx, binding)
	require.NoError(t, err)
	assert.Equal(t, Result{Number: 1, Name: getRBR(t, c, "frontend-development-r1").Name, Created: true, Cause: openchoreov1alpha1.RevisionCauseDeploy, Highest: 1}, res)

	rev := getRBR(t, c, "frontend-development-r1")
	assert.Equal(t, "frontend-development-r1", rev.Name, "a revision object is named {binding}-r{N}")

	wantHash, err := CanonicalHash(releaseBindingAdapter{}.HashInput(binding))
	require.NoError(t, err)
	assert.Equal(t, int64(1), rev.Spec.Revision)
	assert.Equal(t, openchoreov1alpha1.RevisionCauseDeploy, rev.Spec.Cause)
	assert.Equal(t, firstRelease, rev.Spec.ComponentReleaseName)
	assert.Equal(t, wantHash, rev.Spec.SnapshotHash)
	assert.Equal(t, binding.Spec, rev.Spec.BindingSpec)

	assert.Equal(t, map[string]string{
		LabelKeyBindingUID:             "uid-frontend-development",
		LabelKeyReleaseBinding:         "frontend-development",
		labels.LabelKeyComponentName:   "frontend",
		labels.LabelKeyProjectName:     "doclet",
		labels.LabelKeyEnvironmentName: "development",
	}, rev.Labels)
	assert.Equal(t, "1", rev.Annotations[AnnotationKeyBindingGeneration])

	owner := metav1.GetControllerOf(rev)
	require.NotNil(t, owner)
	assert.Equal(t, "ReleaseBinding", owner.Kind)
	assert.Equal(t, binding.Name, owner.Name)
	assert.Equal(t, binding.UID, owner.UID)

	assert.True(t, rev.Status.Active)
	assert.Equal(t, openchoreov1alpha1.RevisionHealthProgressing, rev.Status.Health)
	assert.NotNil(t, rev.Status.ObservedAt)
}

func TestReleaseBindingSnapshotter_SpecChanges(t *testing.T) {
	tests := []struct {
		name        string
		mutate      func(s *openchoreov1alpha1.ReleaseBindingSpec)
		wantNew     bool
		wantCause   openchoreov1alpha1.RevisionCause
		wantRelease string
	}{
		{
			name:        "new release is a Deploy",
			mutate:      func(s *openchoreov1alpha1.ReleaseBindingSpec) { s.ReleaseName = secondRelease },
			wantNew:     true,
			wantCause:   openchoreov1alpha1.RevisionCauseDeploy,
			wantRelease: secondRelease,
		},
		{
			name: "override change is an Override",
			mutate: func(s *openchoreov1alpha1.ReleaseBindingSpec) {
				s.ComponentTypeEnvironmentConfigs = &runtime.RawExtension{Raw: []byte(`{"replicas":3}`)}
			},
			wantNew:     true,
			wantCause:   openchoreov1alpha1.RevisionCauseOverride,
			wantRelease: firstRelease,
		},
		{
			name:        "switch to Undeploy is an Undeploy",
			mutate:      func(s *openchoreov1alpha1.ReleaseBindingSpec) { s.State = openchoreov1alpha1.ReleaseStateUndeploy },
			wantNew:     true,
			wantCause:   openchoreov1alpha1.RevisionCauseUndeploy,
			wantRelease: firstRelease,
		},
		{
			name: "raw key order alone records nothing",
			mutate: func(s *openchoreov1alpha1.ReleaseBindingSpec) {
				s.ComponentTypeEnvironmentConfigs = &runtime.RawExtension{Raw: []byte(`{"b":2,"a":1}`)}
			},
			wantNew: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			binding := newBinding("frontend-development")
			if tt.name == "raw key order alone records nothing" {
				binding.Spec.ComponentTypeEnvironmentConfigs = &runtime.RawExtension{Raw: []byte(`{"a":1,"b":2}`)}
			}
			c := newTestClient(t)
			s := NewReleaseBindingSnapshotter(c, newTestScheme(t), Options{HistoryLimit: testHistoryLimit})

			_, err := s.Record(ctx, binding)
			require.NoError(t, err)
			r1Before := getRBR(t, c, "frontend-development-r1")

			bump(binding, tt.mutate)
			res, err := s.Record(ctx, binding)
			require.NoError(t, err)

			r1 := getRBR(t, c, "frontend-development-r1")
			assert.Equal(t, r1Before.Spec, r1.Spec, "an existing revision's spec must never change")

			if !tt.wantNew {
				assert.Equal(t, Result{Number: 1, Name: getRBR(t, c, "frontend-development-r1").Name, Highest: 1}, res)
				assert.Len(t, listRBRs(t, c), 1)
				assert.True(t, r1.Status.Active)
				return
			}

			assert.Equal(t, Result{Number: 2, Name: getRBR(t, c, "frontend-development-r2").Name, Created: true, Cause: tt.wantCause, Highest: 2}, res)
			r2 := getRBR(t, c, "frontend-development-r2")
			assert.Equal(t, tt.wantCause, r2.Spec.Cause)
			assert.Equal(t, tt.wantRelease, r2.Spec.ComponentReleaseName)
			assert.Equal(t, "2", r2.Annotations[AnnotationKeyBindingGeneration])

			assert.True(t, r2.Status.Active)
			assert.Equal(t, openchoreov1alpha1.RevisionHealthProgressing, r2.Status.Health)
			assert.False(t, r1.Status.Active, "the previously active revision must be deactivated")
		})
	}
}

// specStep is a spec change applied to the binding between two Record calls.
type specStep func(s *openchoreov1alpha1.ReleaseBindingSpec)

func undeploy(s *openchoreov1alpha1.ReleaseBindingSpec) {
	s.State = openchoreov1alpha1.ReleaseStateUndeploy
}
func activate(s *openchoreov1alpha1.ReleaseBindingSpec) {
	s.State = openchoreov1alpha1.ReleaseStateActive
}
func newRelease(s *openchoreov1alpha1.ReleaseBindingSpec) { s.ReleaseName = secondRelease }
func newOverride(s *openchoreov1alpha1.ReleaseBindingSpec) {
	s.ComponentTypeEnvironmentConfigs = &runtime.RawExtension{Raw: []byte(`{"replicas":3}`)}
}

func TestReleaseBindingSnapshotter_Causes(t *testing.T) {
	promoted := map[string]string{openchoreov1alpha1.AnnotationKeyRevisionCause: string(openchoreov1alpha1.RevisionCausePromote)}

	tests := []struct {
		name string
		// initial is applied before the first Record; nil leaves the default spec.
		initial specStep
		// history is applied one step per Record before the change under test.
		history []specStep
		// change and annotations make up the write whose cause is checked. A nil
		// change means the first revision is the one under test.
		change      specStep
		annotations map[string]string
		wantCause   openchoreov1alpha1.RevisionCause
	}{
		// First revision.
		{name: "first revision is a Deploy", wantCause: openchoreov1alpha1.RevisionCauseDeploy},
		{name: "first revision of a promoted binding is a Promote", annotations: promoted, wantCause: openchoreov1alpha1.RevisionCausePromote},
		{name: "first revision created undeployed is an Undeploy", initial: undeploy, wantCause: openchoreov1alpha1.RevisionCauseUndeploy},

		// Spec-derived causes.
		{name: "releaseName changed is a Deploy", change: newRelease, wantCause: openchoreov1alpha1.RevisionCauseDeploy},
		{name: "only overrides changed is an Override", change: newOverride, wantCause: openchoreov1alpha1.RevisionCauseOverride},
		{name: "state to Undeploy is an Undeploy", change: undeploy, wantCause: openchoreov1alpha1.RevisionCauseUndeploy},
		// Back to Active with a spec no revision recorded; returning to an
		// already recorded spec reuses that revision instead.
		{name: "state back to Active is a Redeploy", history: []specStep{undeploy, newOverride}, change: activate, wantCause: openchoreov1alpha1.RevisionCauseRedeploy},
		{name: "override change while undeployed is an Override", history: []specStep{undeploy}, change: newOverride, wantCause: openchoreov1alpha1.RevisionCauseOverride},

		// Annotations.
		{
			name: "Promote annotation after the first revision is ignored", change: newRelease,
			annotations: promoted, wantCause: openchoreov1alpha1.RevisionCauseDeploy,
		},
		{
			name: "unknown cause annotation is ignored", change: newOverride,
			annotations: map[string]string{openchoreov1alpha1.AnnotationKeyRevisionCause: "Bogus"},
			wantCause:   openchoreov1alpha1.RevisionCauseOverride,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			binding := newBinding("frontend-development")
			if tt.initial != nil {
				tt.initial(&binding.Spec)
			}
			if tt.change == nil {
				binding.Annotations = tt.annotations
			}
			c := newTestClient(t, binding.DeepCopy())
			s := NewReleaseBindingSnapshotter(c, newTestScheme(t), Options{HistoryLimit: testHistoryLimit})

			res, err := s.Record(ctx, binding)
			require.NoError(t, err)
			for _, step := range tt.history {
				bump(binding, step)
				res, err = s.Record(ctx, binding)
				require.NoError(t, err)
			}
			if tt.change != nil {
				bump(binding, tt.change)
				binding.Annotations = tt.annotations
				res, err = s.Record(ctx, binding)
				require.NoError(t, err)
			}

			require.True(t, res.Created)
			assert.Equal(t, tt.wantCause, res.Cause)
			rev := getRBR(t, c, Name(binding.Name, res.Number))
			assert.Equal(t, tt.wantCause, rev.Spec.Cause)
		})
	}
}

func TestReleaseBindingSnapshotter_CauseAnnotationsRemovedAfterWrite(t *testing.T) {
	ctx := context.Background()
	binding := newBinding("frontend-development")
	binding.Annotations = map[string]string{
		openchoreov1alpha1.AnnotationKeyRevisionCause: string(openchoreov1alpha1.RevisionCausePromote),
		"example.com/keep":                            "yes",
	}
	c := newTestClient(t, binding.DeepCopy())
	require.NoError(t, c.Get(ctx, client.ObjectKeyFromObject(binding), binding))

	// Status the controller has changed in memory but not written yet.
	binding.Status.ObservedGeneration = 7

	res, err := NewReleaseBindingSnapshotter(c, newTestScheme(t), Options{HistoryLimit: testHistoryLimit}).Record(ctx, binding)
	require.NoError(t, err)
	assert.Equal(t, openchoreov1alpha1.RevisionCausePromote, res.Cause)

	stored := &openchoreov1alpha1.ReleaseBinding{}
	require.NoError(t, c.Get(ctx, client.ObjectKeyFromObject(binding), stored))
	assert.Equal(t, map[string]string{"example.com/keep": "yes"}, stored.Annotations)

	assert.Equal(t, stored.Annotations, binding.Annotations, "in-memory annotations follow the patch")
	assert.Equal(t, stored.ResourceVersion, binding.ResourceVersion, "a later status update must not conflict")
	assert.Equal(t, int64(7), binding.Status.ObservedGeneration, "in-memory status must be kept")
}

func TestReleaseBindingSnapshotter_CauseAnnotationsRetriedAfterPatchFailure(t *testing.T) {
	ctx := context.Background()
	binding := newBinding("frontend-development")
	binding.Annotations = map[string]string{
		openchoreov1alpha1.AnnotationKeyRevisionCause: string(openchoreov1alpha1.RevisionCausePromote),
	}

	failPatch := true
	c := fake.NewClientBuilder().
		WithScheme(newTestScheme(t)).
		WithObjects(binding.DeepCopy()).
		WithStatusSubresource(&openchoreov1alpha1.ReleaseBindingRevision{}).
		WithInterceptorFuncs(interceptor.Funcs{
			Patch: func(ctx context.Context, c client.WithWatch, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
				if _, ok := obj.(*openchoreov1alpha1.ReleaseBinding); ok && failPatch {
					return fmt.Errorf("injected patch failure")
				}
				return c.Patch(ctx, obj, patch, opts...)
			},
		}).
		Build()
	require.NoError(t, c.Get(ctx, client.ObjectKeyFromObject(binding), binding))
	s := NewReleaseBindingSnapshotter(c, newTestScheme(t), Options{HistoryLimit: testHistoryLimit})

	res, err := s.Record(ctx, binding)
	require.ErrorContains(t, err, "injected patch failure")
	assert.Equal(t, Result{Number: 1, Name: getRBR(t, c, "frontend-development-r1").Name, Created: true, Cause: openchoreov1alpha1.RevisionCausePromote, Highest: 1}, res,
		"the revision is written even when removing the annotation fails")

	// The retry finds the spec recorded: it creates nothing and only removes the annotation.
	failPatch = false
	res, err = s.Record(ctx, binding)
	require.NoError(t, err)
	assert.Equal(t, Result{Number: 1, Name: getRBR(t, c, "frontend-development-r1").Name, Highest: 1}, res)
	assert.Len(t, listRBRs(t, c), 1)

	stored := &openchoreov1alpha1.ReleaseBinding{}
	require.NoError(t, c.Get(ctx, client.ObjectKeyFromObject(binding), stored))
	assert.NotContains(t, stored.Annotations, openchoreov1alpha1.AnnotationKeyRevisionCause)
}

func TestReleaseBindingSnapshotter_StaleBindingKeepsCauseAnnotations(t *testing.T) {
	ctx := context.Background()
	binding := newBinding("frontend-development")
	c := newTestClient(t, binding.DeepCopy())
	require.NoError(t, c.Get(ctx, client.ObjectKeyFromObject(binding), binding))
	s := NewReleaseBindingSnapshotter(c, newTestScheme(t), Options{HistoryLimit: testHistoryLimit})

	// Generation 2 is recorded first; a lagging cache then hands back generation 1
	// carrying a cause marker meant for a change that is not recorded yet.
	stale := binding.DeepCopy()
	stale.Spec.ReleaseName = "frontend-older"
	stale.Annotations = map[string]string{openchoreov1alpha1.AnnotationKeyRevisionCause: string(openchoreov1alpha1.RevisionCausePromote)}
	bump(binding, newRelease)
	_, err := s.Record(ctx, binding)
	require.NoError(t, err)

	_, err = s.Record(ctx, stale)
	require.NoError(t, err)
	assert.Contains(t, stale.Annotations, openchoreov1alpha1.AnnotationKeyRevisionCause,
		"annotations for an unrecorded spec must be left for the reconcile that records it")
	assert.Len(t, listRBRs(t, c), 1)
}

func TestReleaseBindingSnapshotter_StaleBindingRecordsNothing(t *testing.T) {
	ctx := context.Background()
	binding := newBinding("frontend-development")
	c := newTestClient(t)
	s := NewReleaseBindingSnapshotter(c, newTestScheme(t), Options{HistoryLimit: testHistoryLimit})

	stale := binding.DeepCopy()
	bump(binding, func(s *openchoreov1alpha1.ReleaseBindingSpec) { s.ReleaseName = secondRelease })
	_, err := s.Record(ctx, binding) // records generation 2 as r1
	require.NoError(t, err)

	// A reconcile working from the generation-1 object seen through a lagging cache.
	res, err := s.Record(ctx, stale)
	require.NoError(t, err)
	assert.Equal(t, Result{Number: 1, Name: getRBR(t, c, "frontend-development-r1").Name, Highest: 1}, res)
	assert.Len(t, listRBRs(t, c), 1)
}

func TestReleaseBindingSnapshotter_AlreadyExistsIsSuccess(t *testing.T) {
	ctx := context.Background()
	binding := newBinding("frontend-development")

	// r1 was created by an earlier attempt whose response was lost, and the cache
	// has not seen it yet: it is invisible to List but occupies the name.
	existing := earlierRevision(binding, binding.UID)
	c := newTestClient(t, existing)
	s := NewReleaseBindingSnapshotter(c, newTestScheme(t), Options{HistoryLimit: testHistoryLimit})

	res, err := s.Record(ctx, binding)
	require.NoError(t, err)
	assert.Equal(t, Result{Number: 1, Name: getRBR(t, c, "frontend-development-r1").Name, Created: false, Cause: openchoreov1alpha1.RevisionCauseDeploy, Highest: 1}, res)

	rev := getRBR(t, c, "frontend-development-r1")
	assert.Equal(t, "from-earlier-attempt", rev.Spec.SnapshotHash, "the stored revision must not be overwritten")
	assert.True(t, rev.Status.Active)
	assert.Equal(t, openchoreov1alpha1.RevisionHealthProgressing, rev.Status.Health)
}

func TestReleaseBindingSnapshotter_NameHeldByDeletedBindingsRevision(t *testing.T) {
	ctx := context.Background()
	binding := newBinding("frontend-development")

	// The binding was deleted and recreated under the same name; its old r1 has
	// not been garbage collected yet and still occupies the name.
	leftover := earlierRevision(binding, "uid-of-deleted-binding")
	c := newTestClient(t, leftover)
	s := NewReleaseBindingSnapshotter(c, newTestScheme(t), Options{HistoryLimit: testHistoryLimit})

	_, err := s.Record(ctx, binding)
	require.ErrorContains(t, err, "held by a revision of another binding")

	rev := getRBR(t, c, "frontend-development-r1")
	assert.False(t, rev.Status.Active, "another binding's revision must not be adopted or activated")

	// Once garbage collection removes it, the next pass records r1 normally.
	require.NoError(t, c.Delete(ctx, rev))
	res, err := s.Record(ctx, binding)
	require.NoError(t, err)
	assert.Equal(t, Result{Number: 1, Name: getRBR(t, c, "frontend-development-r1").Name, Created: true, Cause: openchoreov1alpha1.RevisionCauseDeploy, Highest: 1}, res)
}

// earlierRevision returns an r1 for binding that an earlier write created, owned
// by the binding with ownerUID and labeled so that List does not find it -- as
// when the cache has not caught up yet.
func earlierRevision(binding *openchoreov1alpha1.ReleaseBinding, ownerUID types.UID) *openchoreov1alpha1.ReleaseBindingRevision {
	return &openchoreov1alpha1.ReleaseBindingRevision{
		ObjectMeta: metav1.ObjectMeta{
			Name:      Name(binding.Name, 1),
			Namespace: "default",
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion: openchoreov1alpha1.GroupVersion.String(),
				Kind:       "ReleaseBinding",
				Name:       binding.Name,
				UID:        ownerUID,
				Controller: ptrTo(true),
			}},
		},
		Spec: openchoreov1alpha1.ReleaseBindingRevisionSpec{
			Revision: 1, Cause: openchoreov1alpha1.RevisionCauseDeploy,
			ComponentReleaseName: firstRelease, SnapshotHash: "from-earlier-attempt",
			BindingSpec: binding.Spec,
		},
	}
}

func TestReleaseBindingSnapshotter_ProjectionKeepsHealthAndIsIdempotent(t *testing.T) {
	ctx := context.Background()
	binding := newBinding("frontend-development")
	c := newTestClient(t)
	s := NewReleaseBindingSnapshotter(c, newTestScheme(t), Options{HistoryLimit: testHistoryLimit})

	_, err := s.Record(ctx, binding)
	require.NoError(t, err)

	// The health writer later marks r1 Healthy.
	r1 := getRBR(t, c, "frontend-development-r1")
	base := r1.DeepCopy()
	r1.Status.Health = openchoreov1alpha1.RevisionHealthHealthy
	require.NoError(t, c.Status().Patch(ctx, r1, client.MergeFrom(base)))
	rv := getRBR(t, c, "frontend-development-r1").ResourceVersion

	// Further reconciles with no spec change write nothing.
	for range 3 {
		res, err := s.Record(ctx, binding)
		require.NoError(t, err)
		assert.Equal(t, Result{Number: 1, Name: getRBR(t, c, "frontend-development-r1").Name, Highest: 1}, res)
	}
	after := getRBR(t, c, "frontend-development-r1")
	assert.Equal(t, rv, after.ResourceVersion, "a no-op reconcile must not write")
	assert.Equal(t, openchoreov1alpha1.RevisionHealthHealthy, after.Status.Health)
}

func TestReleaseBindingSnapshotter_ProjectionRepairsActive(t *testing.T) {
	ctx := context.Background()
	binding := newBinding("frontend-development")
	c := newTestClient(t)
	s := NewReleaseBindingSnapshotter(c, newTestScheme(t), Options{HistoryLimit: testHistoryLimit})

	_, err := s.Record(ctx, binding)
	require.NoError(t, err)
	bump(binding, func(s *openchoreov1alpha1.ReleaseBindingSpec) { s.ReleaseName = secondRelease })
	_, err = s.Record(ctx, binding)
	require.NoError(t, err)

	// Simulate a partial failure that left r1 active and r2 inactive.
	for name, active := range map[string]bool{"frontend-development-r1": true, "frontend-development-r2": false} {
		rev := getRBR(t, c, name)
		base := rev.DeepCopy()
		rev.Status.Active = active
		require.NoError(t, c.Status().Patch(ctx, rev, client.MergeFrom(base)))
	}

	_, err = s.Record(ctx, binding)
	require.NoError(t, err)
	assert.False(t, getRBR(t, c, "frontend-development-r1").Status.Active)
	assert.True(t, getRBR(t, c, "frontend-development-r2").Status.Active)
}

func TestReleaseBindingSnapshotter_IgnoresOtherBindingsRevisions(t *testing.T) {
	ctx := context.Background()
	binding := newBinding("frontend-development")
	c := newTestClient(t)
	s := NewReleaseBindingSnapshotter(c, newTestScheme(t), Options{HistoryLimit: testHistoryLimit})

	// A same-named binding that was deleted and recreated leaves revisions with
	// the old UID behind until garbage collection removes them.
	old := binding.DeepCopy()
	old.UID = "uid-old"
	old.Spec.ReleaseName = "frontend-old"
	_, err := s.Record(ctx, old)
	require.NoError(t, err)
	oldRev := getRBR(t, c, "frontend-development-r1")
	require.NoError(t, c.Delete(ctx, oldRev)) // free the name, keep the scenario about UIDs

	// It carries this binding's UID label (as a mislabeled object could), so only
	// the controller-reference check keeps it out.
	foreign := oldRev.DeepCopy()
	foreign.ResourceVersion = ""
	foreign.Name = "leftover"
	foreign.Labels[LabelKeyBindingUID] = string(binding.UID)
	require.NoError(t, c.Create(ctx, foreign))

	res, err := s.Record(ctx, binding)
	require.NoError(t, err)
	assert.Equal(t, Result{Number: 1, Name: "frontend-development-r1", Created: true, Cause: openchoreov1alpha1.RevisionCauseDeploy, Highest: 1}, res)
	assert.True(t, getRBR(t, c, "leftover").Status.Active, "another binding's revision must not be touched")
}

// stagingEnv is the second environment the naming test binds to.
const stagingEnv = "staging"

func TestReleaseBindingSnapshotter_EachEnvironmentCountsFromOne(t *testing.T) {
	ctx := context.Background()
	dev := newBinding("frontend-development")
	staging := newBinding("frontend-staging")
	staging.Spec.Environment = stagingEnv
	c := newTestClient(t, dev.DeepCopy(), staging.DeepCopy())
	s := NewReleaseBindingSnapshotter(c, newTestScheme(t), Options{HistoryLimit: testHistoryLimit})

	res, err := s.Record(ctx, dev)
	require.NoError(t, err)
	assert.Equal(t, "frontend-development-r1", res.Name)

	// Promoting the same release to staging starts staging's own history at r1;
	// the object name is per binding, so it does not collide with development's.
	res, err = s.Record(ctx, staging)
	require.NoError(t, err)
	assert.Equal(t, Result{Number: 1, Name: "frontend-staging-r1", Created: true, Cause: openchoreov1alpha1.RevisionCauseDeploy, Highest: 1}, res)

	bump(dev, newRelease)
	res, err = s.Record(ctx, dev)
	require.NoError(t, err)
	assert.Equal(t, Result{Number: 2, Name: "frontend-development-r2", Created: true, Cause: openchoreov1alpha1.RevisionCauseDeploy, Highest: 2}, res)
}

func TestDisplayName(t *testing.T) {
	assert.Equal(t, "frontend-r1", DisplayName("frontend", 1))
}

func TestReleaseBindingSnapshotter_LongNamesStayValid(t *testing.T) {
	ctx := context.Background()
	long := strings.Repeat("a", 240)
	binding := newBinding(long)
	binding.UID = "6f1c2a9e-3b4d-4e5f-8a7b-9c0d1e2f3a4b" // a real 36-character UID
	binding.Spec.Owner.ComponentName = strings.Repeat("c", 100)
	c := newTestClient(t)
	s := NewReleaseBindingSnapshotter(c, newTestScheme(t), Options{HistoryLimit: testHistoryLimit})

	res, err := s.Record(ctx, binding)
	require.NoError(t, err)

	rev := getRBR(t, c, Name(long, res.Number))
	assert.Empty(t, validation.IsDNS1123Subdomain(rev.Name))
	for k, v := range rev.Labels {
		assert.Empty(t, validation.IsValidLabelValue(v), "label %s=%q", k, v)
	}
}

// ─────────────────────────────────────────────────────────────
// ResourceReleaseBinding snapshotter
// ─────────────────────────────────────────────────────────────

func newResourceBinding(name, env, release string) *openchoreov1alpha1.ResourceReleaseBinding {
	return &openchoreov1alpha1.ResourceReleaseBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:       name,
			Namespace:  "default",
			UID:        types.UID("uid-" + name),
			Generation: 1,
		},
		Spec: openchoreov1alpha1.ResourceReleaseBindingSpec{
			Owner:           openchoreov1alpha1.ResourceReleaseBindingOwner{ProjectName: "doclet", ResourceName: "db"},
			Environment:     env,
			ResourceRelease: release,
		},
	}
}

// reactivationFixture records r1 (firstRelease) and r2 (secondRelease), with
// r1 Healthy and known-good, and returns the snapshotter, binding and client.
func reactivationFixture(t *testing.T) (*Snapshotter[*openchoreov1alpha1.ReleaseBinding,
	*openchoreov1alpha1.ReleaseBindingRevision], *openchoreov1alpha1.ReleaseBinding, client.Client, *record.FakeRecorder) {
	t.Helper()
	ctx := context.Background()
	binding := newBinding("frontend-development")
	c := newTestClient(t, binding.DeepCopy())
	require.NoError(t, c.Get(ctx, client.ObjectKeyFromObject(binding), binding))
	recorder := record.NewFakeRecorder(10)
	s := NewReleaseBindingSnapshotter(c, newTestScheme(t), Options{Recorder: recorder, HistoryLimit: testHistoryLimit})

	_, err := s.Record(ctx, binding)
	require.NoError(t, err)
	_, err = s.ObserveHealth(ctx, binding, openchoreov1alpha1.RevisionHealthHealthy)
	require.NoError(t, err)
	require.True(t, getRBR(t, c, "frontend-development-r1").Status.KnownGood)

	bump(binding, newRelease)
	_, err = s.Record(ctx, binding)
	require.NoError(t, err)
	_, err = s.ObserveHealth(ctx, binding, openchoreov1alpha1.RevisionHealthDegraded)
	require.NoError(t, err)
	return s, binding, c, recorder
}

func TestReleaseBindingSnapshotter_RollbackReactivatesTheTarget(t *testing.T) {
	ctx := context.Background()
	s, binding, c, recorder := reactivationFixture(t)

	// The rollback restores r1's spec.
	bump(binding, func(spec *openchoreov1alpha1.ReleaseBindingSpec) {
		spec.ReleaseName = firstRelease
	})
	res, err := s.Record(ctx, binding)
	require.NoError(t, err)
	assert.Equal(t, Result{Number: 1, Name: getRBR(t, c, "frontend-development-r1").Name, Reactivated: true, Highest: 2}, res)

	assert.Len(t, listRBRs(t, c), 2, "no revision is created")
	r1 := getRBR(t, c, "frontend-development-r1")
	r2 := getRBR(t, c, "frontend-development-r2")
	assert.True(t, r1.Status.Active)
	assert.False(t, r2.Status.Active)
	assert.Equal(t, openchoreov1alpha1.RevisionHealthProgressing, r1.Status.Health,
		"a revision made active again is applied anew and starts at Progressing")
	assert.Nil(t, r1.Status.HealthySince)
	assert.True(t, r1.Status.KnownGood, "known-good is never lost")
	assert.Equal(t, openchoreov1alpha1.RevisionHealthDegraded, r2.Status.Health, "r2 keeps the health it last had")
	assert.Equal(t, "3", r1.Annotations[AnnotationKeyBindingGeneration], "r1 records the generation it became active at")
	assert.Equal(t, "Normal RevisionReactivated Revision 1 (frontend-development-r1) is active again: "+
		"the binding spec matches the one it recorded", <-recorder.Events)

	// Reconciling the same binding again changes nothing.
	res, err = s.Record(ctx, binding)
	require.NoError(t, err)
	assert.Equal(t, Result{Number: 1, Name: getRBR(t, c, "frontend-development-r1").Name, Highest: 2}, res)

	// Health is observed on r1, the active revision, not on r2, the newest.
	_, err = s.ObserveHealth(ctx, binding, openchoreov1alpha1.RevisionHealthHealthy)
	require.NoError(t, err)
	assert.Equal(t, openchoreov1alpha1.RevisionHealthHealthy, getRBR(t, c, "frontend-development-r1").Status.Health)
	assert.Equal(t, openchoreov1alpha1.RevisionHealthDegraded, getRBR(t, c, "frontend-development-r2").Status.Health)
}

func TestReleaseBindingSnapshotter_ChangeAfterReactivationIsNumberedAboveTheHighest(t *testing.T) {
	ctx := context.Background()
	s, binding, c, _ := reactivationFixture(t)

	bump(binding, func(spec *openchoreov1alpha1.ReleaseBindingSpec) { spec.ReleaseName = firstRelease })
	_, err := s.Record(ctx, binding)
	require.NoError(t, err)

	// An override on top of r1's spec: classified against r1, the revision it replaces.
	bump(binding, newOverride)
	res, err := s.Record(ctx, binding)
	require.NoError(t, err)
	assert.Equal(t, Result{Number: 3, Name: getRBR(t, c, "frontend-development-r3").Name, Created: true,
		Cause: openchoreov1alpha1.RevisionCauseOverride, Highest: 3}, res)
	assert.True(t, getRBR(t, c, "frontend-development-r3").Status.Active)
	assert.False(t, getRBR(t, c, "frontend-development-r1").Status.Active)

	// Returning to r2's spec makes r2 active again.
	bump(binding, func(spec *openchoreov1alpha1.ReleaseBindingSpec) {
		spec.ReleaseName = secondRelease
		spec.ComponentTypeEnvironmentConfigs = nil
	})
	res, err = s.Record(ctx, binding)
	require.NoError(t, err)
	assert.Equal(t, Result{Number: 2, Name: getRBR(t, c, "frontend-development-r2").Name, Reactivated: true, Highest: 3}, res)
	assert.Len(t, listRBRs(t, c), 3)
}

func TestReleaseBindingSnapshotter_StaleReadDoesNotReactivate(t *testing.T) {
	ctx := context.Background()
	s, binding, c, _ := reactivationFixture(t)
	stale := binding.DeepCopy() // generation 2, r2's spec

	bump(binding, func(spec *openchoreov1alpha1.ReleaseBindingSpec) { spec.ReleaseName = firstRelease })
	_, err := s.Record(ctx, binding) // generation 3 makes r1 active
	require.NoError(t, err)

	// A lagging cache hands back generation 2, whose spec r2 records.
	res, err := s.Record(ctx, stale)
	require.NoError(t, err)
	assert.Equal(t, Result{Number: 1, Name: getRBR(t, c, "frontend-development-r1").Name, Highest: 2}, res)
	assert.True(t, getRBR(t, c, "frontend-development-r1").Status.Active)
	assert.False(t, getRBR(t, c, "frontend-development-r2").Status.Active)
}

func TestReleaseBindingSnapshotter_DeletedRevisionIsNotRecreatedOrRenumbered(t *testing.T) {
	ctx := context.Background()
	s, binding, c, _ := reactivationFixture(t)

	// A rollback to r1 leaves r2, the highest revision, inactive. The controller
	// persists Result.Highest as status.highestRevision.
	bump(binding, func(spec *openchoreov1alpha1.ReleaseBindingSpec) {
		spec.ReleaseName = firstRelease
	})
	res, err := s.Record(ctx, binding)
	require.NoError(t, err)
	binding.Status.HighestRevision = res.Highest
	require.Equal(t, int64(2), binding.Status.HighestRevision)

	require.NoError(t, c.Delete(ctx, getRBR(t, c, "frontend-development-r2")))

	// Reconciling the unchanged binding recreates nothing; r1 stays active.
	res, err = s.Record(ctx, binding)
	require.NoError(t, err)
	assert.Equal(t, Result{Number: 1, Name: getRBR(t, c, "frontend-development-r1").Name, Highest: 2}, res)
	require.Len(t, listRBRs(t, c), 1)
	assert.True(t, getRBR(t, c, "frontend-development-r1").Status.Active)

	// Deploying the deleted revision's spec again records a new revision,
	// numbered above the deleted one: number 2 is not reused.
	bump(binding, func(spec *openchoreov1alpha1.ReleaseBindingSpec) { spec.ReleaseName = secondRelease })
	res, err = s.Record(ctx, binding)
	require.NoError(t, err)
	assert.Equal(t, int64(3), res.Number)
	assert.True(t, res.Created)
	assert.Equal(t, int64(3), res.Highest)
	revs := listRBRs(t, c)
	numbers := make([]int64, 0, len(revs))
	for _, rev := range revs {
		numbers = append(numbers, rev.Spec.Revision)
	}
	assert.ElementsMatch(t, []int64{1, 3}, numbers, "the gap left by r2 stays")
	assert.False(t, getRBR(t, c, "frontend-development-r1").Status.Active)
	assert.True(t, getRBR(t, c, "frontend-development-r3").Status.Active)
}

func TestReleaseBindingSnapshotter_NumbersAboveStatusHighestRevision(t *testing.T) {
	ctx := context.Background()
	binding := newBinding("frontend-development")
	binding.Status.HighestRevision = 7 // r1..r7 were recorded and later deleted
	c := newTestClient(t, binding.DeepCopy())
	s := NewReleaseBindingSnapshotter(c, newTestScheme(t), Options{HistoryLimit: testHistoryLimit})

	res, err := s.Record(ctx, binding)
	require.NoError(t, err)
	assert.Equal(t, Result{Number: 8, Name: "frontend-development-r8", Created: true,
		Cause: openchoreov1alpha1.RevisionCauseDeploy, Highest: 8}, res)
}

func TestResourceReleaseBindingSnapshotter_NumbersAboveStatusHighestRevision(t *testing.T) {
	ctx := context.Background()
	binding := newResourceBinding("db-development", "development", "db-abc123")
	binding.Status.HighestRevision = 4
	c := newTestClient(t, binding.DeepCopy())
	s := NewResourceReleaseBindingSnapshotter(c, newTestScheme(t), Options{HistoryLimit: testHistoryLimit})

	res, err := s.Record(ctx, binding)
	require.NoError(t, err)
	assert.Equal(t, int64(5), res.Number)
	assert.Equal(t, int64(5), res.Highest)
	assert.Equal(t, "db-development-r5", res.Name)
}

func TestResourceReleaseBindingSnapshotter(t *testing.T) {
	ctx := context.Background()
	dev := newResourceBinding("db-development", "development", "db-abc123")
	c := newTestClient(t, dev)
	s := NewResourceReleaseBindingSnapshotter(c, newTestScheme(t), Options{HistoryLimit: testHistoryLimit})

	getRRBR := func(name string) *openchoreov1alpha1.ResourceReleaseBindingRevision {
		rev := &openchoreov1alpha1.ResourceReleaseBindingRevision{}
		require.NoError(t, c.Get(ctx, client.ObjectKey{Namespace: "default", Name: name}, rev))
		return rev
	}

	res, err := s.Record(ctx, dev)
	require.NoError(t, err)
	assert.Equal(t, Result{Number: 1, Name: "db-development-r1", Created: true, Cause: openchoreov1alpha1.RevisionCauseDeploy, Highest: 1}, res)

	r1 := getRRBR("db-development-r1")
	assert.Equal(t, "db-abc123", r1.Spec.ResourceReleaseName)
	assert.Equal(t, dev.Spec, r1.Spec.BindingSpec)
	assert.Equal(t, map[string]string{
		LabelKeyBindingUID:             "uid-db-development",
		LabelKeyResourceReleaseBinding: "db-development",
		labels.LabelKeyResourceName:    "db",
		labels.LabelKeyProjectName:     "doclet",
		labels.LabelKeyEnvironmentName: "development",
	}, r1.Labels)
	assert.Equal(t, "ResourceReleaseBinding", metav1.GetControllerOf(r1).Kind)
	assert.True(t, r1.Status.Active)

	// A binding created by promotion records Promote, and loses the marker. It
	// counts its own revisions from r1.
	staging := newResourceBinding("db-staging", "staging", "db-abc123")
	staging.Annotations = map[string]string{openchoreov1alpha1.AnnotationKeyRevisionCause: string(openchoreov1alpha1.RevisionCausePromote)}
	require.NoError(t, c.Create(ctx, staging))
	res, err = s.Record(ctx, staging)
	require.NoError(t, err)
	assert.Equal(t, Result{Number: 1, Name: "db-staging-r1", Created: true, Cause: openchoreov1alpha1.RevisionCausePromote, Highest: 1}, res)
	assert.NotContains(t, staging.Annotations, openchoreov1alpha1.AnnotationKeyRevisionCause)

	// Advancing the pinned release is a Deploy.
	dev.Spec.ResourceRelease = secondResourceRelease
	dev.Generation++
	res, err = s.Record(ctx, dev)
	require.NoError(t, err)
	assert.Equal(t, Result{Number: 2, Name: "db-development-r2", Created: true, Cause: openchoreov1alpha1.RevisionCauseDeploy, Highest: 2}, res)

	// A retain policy change keeps the release: an Override.
	dev.Spec.RetainPolicy = openchoreov1alpha1.ResourceRetainPolicyRetain
	dev.Generation++
	res, err = s.Record(ctx, dev)
	require.NoError(t, err)
	assert.Equal(t, Result{Number: 3, Name: "db-development-r3", Created: true, Cause: openchoreov1alpha1.RevisionCauseOverride, Highest: 3}, res)
	assert.False(t, getRRBR("db-development-r2").Status.Active)
	assert.True(t, getRRBR("db-development-r3").Status.Active)

	// The pin alone is not a revision.
	dev.Generation++
	res, err = s.Record(ctx, dev)
	require.NoError(t, err)
	assert.Equal(t, Result{Number: 3, Name: "db-development-r3", Highest: 3}, res)

	// Rolling back to r2's spec makes r2 active again; nothing is created.
	dev.Spec.RetainPolicy = r1.Spec.BindingSpec.RetainPolicy
	dev.Generation++
	res, err = s.Record(ctx, dev)
	require.NoError(t, err)
	assert.Equal(t, Result{Number: 2, Name: "db-development-r2", Reactivated: true, Highest: 3}, res)
	assert.True(t, getRRBR("db-development-r2").Status.Active)
	assert.False(t, getRRBR("db-development-r3").Status.Active)
}

// ─────────────────────────────────────────────────────────────
// LabelValue
// ─────────────────────────────────────────────────────────────

func TestLabelValue(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "short value unchanged", input: "frontend-development", want: "frontend-development"},
		{name: "63 characters unchanged", input: strings.Repeat("a", 63), want: strings.Repeat("a", 63)},
		{name: "64 characters shortened", input: strings.Repeat("a", 64)},
		{name: "dotted subdomain shortened", input: strings.Repeat("seg.", 30)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := LabelValue(tt.input)
			assert.Empty(t, validation.IsValidLabelValue(got))
			if tt.want != "" {
				assert.Equal(t, tt.want, got)
			} else {
				assert.LessOrEqual(t, len(got), maxLabelValueLength)
				assert.NotEqual(t, LabelValue(tt.input+"x"), got)
			}
		})
	}
}

func ptrTo[T any](v T) *T { return &v }

// activationsOf lists the binding's revisions as PreviousActive takes them.
func activationsOf(t *testing.T, c client.Client) []Activation {
	t.Helper()
	revs := listRBRs(t, c)
	out := make([]Activation, len(revs))
	for i := range revs {
		out[i] = Activation{Object: &revs[i], Number: revs[i].Spec.Revision}
	}
	return out
}

func TestReleaseBindingSnapshotter_RecordsThePreviouslyActiveRevision(t *testing.T) {
	ctx := context.Background()
	binding := newBinding("frontend-development")
	c := newTestClient(t, binding.DeepCopy())
	require.NoError(t, c.Get(ctx, client.ObjectKeyFromObject(binding), binding))
	s := NewReleaseBindingSnapshotter(c, newTestScheme(t), Options{HistoryLimit: testHistoryLimit})
	record := func() Result {
		t.Helper()
		res, err := s.Record(ctx, binding)
		require.NoError(t, err)
		binding.Status.HighestRevision = res.Highest
		return res
	}
	previous := func() (int64, int64, error) {
		t.Helper()
		cur, prev, inferred, err := PreviousActive(activationsOf(t, c), binding.Status.HighestRevision)
		assert.Empty(t, inferred, "recorded history is never inferred")
		return cur.Number, prev.Number, err
	}

	record() // r1
	_, _, err := previous()
	require.ErrorIs(t, err, ErrNoPreviousRevision, "r1 was the first activation")
	assert.Equal(t, "0", getRBR(t, c, "frontend-development-r1").Annotations[AnnotationKeyPreviousRevision])

	bump(binding, newRelease)
	record() // r2
	bump(binding, newOverride)
	record() // r3
	cur, prev, err := previous()
	require.NoError(t, err)
	assert.Equal(t, [2]int64{3, 2}, [2]int64{cur, prev})

	// Roll back to r1: r1 is active again and records r3 as its predecessor.
	bump(binding, func(spec *openchoreov1alpha1.ReleaseBindingSpec) {
		spec.ReleaseName = firstRelease
		spec.ComponentTypeEnvironmentConfigs = nil
	})
	assert.True(t, record().Reactivated)
	cur, prev, err = previous()
	require.NoError(t, err)
	assert.Equal(t, [2]int64{1, 3}, [2]int64{cur, prev}, "the previous revision is r3, not r2")

	// Roll back again, to r2: its predecessor is r1, the revision active just before.
	bump(binding, newRelease)
	assert.True(t, record().Reactivated)

	// A controller restart: a new Snapshotter reads the same history from the objects.
	s = NewReleaseBindingSnapshotter(c, newTestScheme(t), Options{HistoryLimit: testHistoryLimit})
	cur, prev, err = previous()
	require.NoError(t, err)
	assert.Equal(t, [2]int64{2, 1}, [2]int64{cur, prev})
	res, err := s.Record(ctx, binding)
	require.NoError(t, err)
	assert.False(t, res.Created || res.Reactivated, "reconciling again changes nothing")
	cur, prev, err = previous()
	require.NoError(t, err)
	assert.Equal(t, [2]int64{2, 1}, [2]int64{cur, prev})

	// The previous revision is deleted: refused, not replaced by r3.
	require.NoError(t, c.Delete(ctx, getRBR(t, c, "frontend-development-r1")))
	_, _, err = previous()
	require.ErrorIs(t, err, ErrPreviousRevisionDeleted)
}

func TestResourceReleaseBindingSnapshotter_RecordsThePreviouslyActiveRevision(t *testing.T) {
	ctx := context.Background()
	binding := newResourceBinding("db-development", "development", "db-def456")
	c := newTestClient(t, binding.DeepCopy())
	require.NoError(t, c.Get(ctx, client.ObjectKeyFromObject(binding), binding))
	s := NewResourceReleaseBindingSnapshotter(c, newTestScheme(t), Options{HistoryLimit: testHistoryLimit})

	for _, release := range []string{"db-abc123", "db-def456", "db-abc123"} {
		binding.Spec.ResourceRelease = release
		binding.Generation++
		_, err := s.Record(ctx, binding)
		require.NoError(t, err)
	}

	var list openchoreov1alpha1.ResourceReleaseBindingRevisionList
	require.NoError(t, c.List(ctx, &list, client.InNamespace("default")))
	revs := make([]Activation, len(list.Items))
	for i := range list.Items {
		revs[i] = Activation{Object: &list.Items[i], Number: list.Items[i].Spec.Revision}
	}
	cur, prev, _, err := PreviousActive(revs, 2)
	require.NoError(t, err)
	assert.Equal(t, [2]int64{1, 2}, [2]int64{cur.Number, prev.Number}, "r1 is active again after r2")
}

// rollbackTo makes the binding's spec the one revision n records, as the
// rollback API does, pinning the binding.
func rollbackTo(t *testing.T, c client.Client, binding *openchoreov1alpha1.ReleaseBinding, n int64) {
	t.Helper()
	rev := getRBR(t, c, "frontend-development-r"+strconv.FormatInt(n, 10))
	bump(binding, func(spec *openchoreov1alpha1.ReleaseBindingSpec) {
		*spec = *rev.Spec.BindingSpec.DeepCopy()
	})
}

// TestReleaseBindingSnapshotter_UndoFollowsActivationHistory is the r1..r5
// scenario: after an explicit rollback from r5 to r3, an undo returns to r5,
// and the next undo to r3 again.
func TestReleaseBindingSnapshotter_UndoFollowsActivationHistory(t *testing.T) {
	ctx := context.Background()
	binding := newBinding("frontend-development")
	c := newTestClient(t, binding.DeepCopy())
	require.NoError(t, c.Get(ctx, client.ObjectKeyFromObject(binding), binding))
	s := NewReleaseBindingSnapshotter(c, newTestScheme(t), Options{HistoryLimit: testHistoryLimit})
	record := func() {
		t.Helper()
		res, err := s.Record(ctx, binding)
		require.NoError(t, err)
		binding.Status.HighestRevision = res.Highest
	}
	undoTarget := func() int64 {
		t.Helper()
		_, prev, inferred, err := PreviousActive(activationsOf(t, c), binding.Status.HighestRevision)
		require.NoError(t, err)
		require.Empty(t, inferred)
		return prev.Number
	}
	previousAnnotation := func(n int64) string {
		return getRBR(t, c, "frontend-development-r"+strconv.FormatInt(n, 10)).Annotations[AnnotationKeyPreviousRevision]
	}

	record() // r1
	for i := 2; i <= 5; i++ {
		bump(binding, func(spec *openchoreov1alpha1.ReleaseBindingSpec) {
			spec.ComponentTypeEnvironmentConfigs = &runtime.RawExtension{Raw: []byte(`{"replicas":` + strconv.Itoa(i) + `}`)}
		})
		record()
	}
	require.Equal(t, "4", previousAnnotation(5))
	require.Equal(t, int64(4), undoTarget())

	// occ rb rollback frontend-development --revision 3
	rollbackTo(t, c, binding, 3)
	record()
	assert.Equal(t, "5", previousAnnotation(3), "r3 records r5, the revision it replaced")
	assert.Equal(t, int64(5), undoTarget(), "undo returns to r5, not r2 or r4")

	// Reconciling again records no new activation.
	before := getRBR(t, c, "frontend-development-r3").ResourceVersion
	record()
	record()
	assert.Equal(t, before, getRBR(t, c, "frontend-development-r3").ResourceVersion, "no rewrite without an activation")

	// occ rb rollback frontend-development (undo) -> r5
	rollbackTo(t, c, binding, undoTarget())
	record()
	assert.Equal(t, "3", previousAnnotation(5))
	assert.Equal(t, int64(3), undoTarget(), "the next undo returns to r3")

	// A controller restart rebuilds nothing; the objects carry the history.
	s = NewReleaseBindingSnapshotter(c, newTestScheme(t), Options{HistoryLimit: testHistoryLimit})
	record()
	assert.Equal(t, int64(3), undoTarget())

	// A stale binding read (the cache still holds the rollback to r3) changes nothing.
	stale := binding.DeepCopy()
	stale.Generation--
	stale.Spec = *getRBR(t, c, "frontend-development-r3").Spec.BindingSpec.DeepCopy()
	_, err := s.Record(ctx, stale)
	require.NoError(t, err)
	assert.True(t, getRBR(t, c, "frontend-development-r5").Status.Active)
	assert.Equal(t, int64(3), undoTarget())

	// Undo again -> r3; numbering never moved.
	rollbackTo(t, c, binding, undoTarget())
	record()
	assert.Equal(t, int64(5), undoTarget())
	assert.Len(t, listRBRs(t, c), 5, "rollbacks create no revisions")
}

// TestReleaseBindingSnapshotter_HistoryLimitKeepsTheUndoTarget: a rollback to
// the oldest revision followed by a new deploy must not evict the revision an
// undo returns to.
func TestReleaseBindingSnapshotter_HistoryLimitKeepsTheUndoTarget(t *testing.T) {
	ctx := context.Background()
	binding := newBinding("frontend-development")
	c := newTestClient(t, binding.DeepCopy())
	require.NoError(t, c.Get(ctx, client.ObjectKeyFromObject(binding), binding))
	// Two old revisions are kept besides the active one.
	s := NewReleaseBindingSnapshotter(c, newTestScheme(t), Options{HistoryLimit: 2})
	record := func() {
		t.Helper()
		res, err := s.Record(ctx, binding)
		require.NoError(t, err)
		binding.Status.HighestRevision = res.Highest
	}
	record() // r1
	for i := 2; i <= 3; i++ {
		bump(binding, func(spec *openchoreov1alpha1.ReleaseBindingSpec) {
			spec.ComponentTypeEnvironmentConfigs = &runtime.RawExtension{Raw: []byte(`{"replicas":` + strconv.Itoa(i) + `}`)}
		})
		record()
	}
	rollbackTo(t, c, binding, 1)
	record()
	bump(binding, func(spec *openchoreov1alpha1.ReleaseBindingSpec) { spec.ReleaseName = secondRelease })
	record() // r4 replaces r1; the limit of 2 old revisions evicts one revision

	_, prev, _, err := PreviousActive(activationsOf(t, c), binding.Status.HighestRevision)
	require.NoError(t, err, "the undo target was not evicted")
	assert.Equal(t, int64(1), prev.Number)
	stored := listRBRs(t, c)
	numbers := make([]int64, 0, len(stored))
	for _, r := range stored {
		numbers = append(numbers, r.Spec.Revision)
	}
	assert.ElementsMatch(t, []int64{1, 3, 4}, numbers, "r2, the oldest revision that is not protected, went")
}

func TestResourceReleaseBindingSnapshotter_UndoFollowsActivationHistory(t *testing.T) {
	ctx := context.Background()
	binding := newResourceBinding("db-development", "development", "db-r1")
	c := newTestClient(t, binding.DeepCopy())
	require.NoError(t, c.Get(ctx, client.ObjectKeyFromObject(binding), binding))
	s := NewResourceReleaseBindingSnapshotter(c, newTestScheme(t), Options{HistoryLimit: testHistoryLimit})
	var highest int64
	deploy := func(release string) {
		t.Helper()
		binding.Spec.ResourceRelease = release
		binding.Generation++
		res, err := s.Record(ctx, binding)
		require.NoError(t, err)
		highest = res.Highest
	}
	undoTarget := func() int64 {
		t.Helper()
		var list openchoreov1alpha1.ResourceReleaseBindingRevisionList
		require.NoError(t, c.List(ctx, &list, client.InNamespace("default")))
		acts := make([]Activation, len(list.Items))
		for i := range list.Items {
			acts[i] = Activation{Object: &list.Items[i], Number: list.Items[i].Spec.Revision}
		}
		_, prev, _, err := PreviousActive(acts, highest)
		require.NoError(t, err)
		return prev.Number
	}

	for i := 1; i <= 5; i++ {
		deploy("db-r" + strconv.Itoa(i))
	}
	deploy("db-r3") // rollback to r3
	assert.Equal(t, int64(5), undoTarget())
	deploy("db-r5") // undo
	assert.Equal(t, int64(3), undoTarget())
}
