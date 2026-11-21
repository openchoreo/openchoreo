// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package releasebinding

import (
	"context"
	"errors"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/labels"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/bindingrevision"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/testutil"
	"github.com/openchoreo/openchoreo/internal/revision"
)

const (
	rbUID     = types.UID("uid-test-rb")
	releaseA  = "test-component-aaa111"
	releaseB  = "test-component-bbb222"
	releaseC  = "test-component-ccc333"
	secretRef = "db-credentials"
)

var t0 = time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)

// fixture is a binding at revision 3 of a three-revision history:
// r1 binds releaseA (Healthy, known-good), r2 binds releaseB, r3 binds releaseC
// with a replicas override and is active.
type fixture struct {
	binding *openchoreov1alpha1.ReleaseBinding
	revs    []*openchoreov1alpha1.ReleaseBindingRevision
	extra   []client.Object
}

// testReleaseLine is the release line of the tests' release-line bindings.
const testReleaseLine = "next"

func specFor(release string, replicas string) openchoreov1alpha1.ReleaseBindingSpec {
	spec := openchoreov1alpha1.ReleaseBindingSpec{
		Owner:       openchoreov1alpha1.ReleaseBindingOwner{ProjectName: testProjectName, ComponentName: testComponentName},
		Environment: testEnvironmentName,
		ReleaseName: release,
		State:       openchoreov1alpha1.ReleaseStateActive,
	}
	if replicas != "" {
		spec.ComponentTypeEnvironmentConfigs = &runtime.RawExtension{Raw: []byte(`{"replicas":` + replicas + `}`)}
	}
	return spec
}

func newRevision(t *testing.T, bindingName string, uid types.UID, n int64, spec openchoreov1alpha1.ReleaseBindingSpec,
	createdAt time.Time) *openchoreov1alpha1.ReleaseBindingRevision {
	t.Helper()
	hash, err := revision.ReleaseBindingSpecHash(&spec)
	require.NoError(t, err)
	return &openchoreov1alpha1.ReleaseBindingRevision{
		ObjectMeta: metav1.ObjectMeta{
			Name:              revision.Name(bindingName, n),
			Namespace:         testNamespace,
			CreationTimestamp: metav1.NewTime(createdAt),
			Labels: map[string]string{
				revision.LabelKeyBindingUID:    string(uid),
				labels.LabelKeyComponentName:   spec.Owner.ComponentName,
				labels.LabelKeyEnvironmentName: spec.Environment,
			},
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion: openchoreov1alpha1.GroupVersion.String(), Kind: "ReleaseBinding",
				Name: bindingName, UID: uid, Controller: ptr.To(true),
			}},
		},
		Spec: openchoreov1alpha1.ReleaseBindingRevisionSpec{
			Revision: n, Cause: openchoreov1alpha1.RevisionCauseDeploy,
			SnapshotHash: hash, BindingSpec: spec,
		},
	}
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	current := specFor(releaseC, "3")
	binding := testutil.NewReleaseBinding(testNamespace, testProjectName, testComponentName, testEnvironmentName, testRBName)
	binding.UID = rbUID
	binding.Spec = current

	r1 := newRevision(t, testRBName, rbUID, 1, specFor(releaseA, ""), t0)
	r1.Status = openchoreov1alpha1.BindingRevisionStatus{Health: openchoreov1alpha1.RevisionHealthHealthy, KnownGood: true}
	r2 := newRevision(t, testRBName, rbUID, 2, specFor(releaseB, ""), t0.Add(time.Hour))
	r3 := newRevision(t, testRBName, rbUID, 3, current, t0.Add(2*time.Hour))
	r3.Status = openchoreov1alpha1.BindingRevisionStatus{Health: openchoreov1alpha1.RevisionHealthHealthy, Active: true}

	return &fixture{binding: binding, revs: []*openchoreov1alpha1.ReleaseBindingRevision{r1, r2, r3}}
}

func (f *fixture) objects() []client.Object {
	objs := make([]client.Object, 0, 5+len(f.revs)+len(f.extra))
	objs = append(objs,
		f.binding,
		testutil.NewComponent(testNamespace, testProjectName, testComponentName),
		testutil.NewComponentRelease(testNamespace, testProjectName, testComponentName, releaseA),
		testutil.NewComponentRelease(testNamespace, testProjectName, testComponentName, releaseB),
		testutil.NewComponentRelease(testNamespace, testProjectName, testComponentName, releaseC),
	)
	for _, r := range f.revs {
		objs = append(objs, r)
	}
	return append(objs, f.extra...)
}

func (f *fixture) client() client.Client {
	return testutil.NewFakeClient(f.objects()...)
}

func getRB(t *testing.T, c client.Client) *openchoreov1alpha1.ReleaseBinding {
	t.Helper()
	rb := &openchoreov1alpha1.ReleaseBinding{}
	require.NoError(t, c.Get(context.Background(), client.ObjectKey{Namespace: testNamespace, Name: testRBName}, rb))
	return rb
}

// revisionVersions returns each revision's resourceVersion, to prove they were not written.
func revisionVersions(t *testing.T, c client.Client) map[string]string {
	t.Helper()
	var list openchoreov1alpha1.ReleaseBindingRevisionList
	require.NoError(t, c.List(context.Background(), &list, client.InNamespace(testNamespace)))
	out := map[string]string{}
	for _, r := range list.Items {
		out[r.Name] = r.ResourceVersion
	}
	return out
}

func warningCodes(ws []bindingrevision.Warning) []bindingrevision.WarningCode {
	out := make([]bindingrevision.WarningCode, 0, len(ws))
	for _, w := range ws {
		out = append(out, w.Code)
	}
	return out
}

// ─────────────────────────────────────────────────────────────
// RollbackReleaseBinding
// ─────────────────────────────────────────────────────────────

func TestRollbackReleaseBinding_Applies(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	c := f.client()
	svc := NewService(c, testutil.TestLogger())
	revsBefore := revisionVersions(t, c)

	result, err := svc.RollbackReleaseBinding(ctx, testNamespace, testRBName, bindingrevision.RollbackRequest{Revision: 1})
	require.NoError(t, err)

	assert.Equal(t, int64(1), result.Revision, "no revision is created: r1 becomes active again")
	assert.Equal(t, testRBName+"-r1", result.RevisionName)
	assert.Equal(t, int64(1), result.TargetRevision)
	assert.Equal(t, ptr.To(int64(3)), result.RevertedFrom)
	assert.False(t, result.DryRun)

	rb := getRB(t, c)
	assert.Equal(t, specFor(releaseA, ""), rb.Spec, "spec is r1's bindingSpec")
	assert.NotContains(t, rb.Annotations, openchoreov1alpha1.AnnotationKeyRevisionCause,
		"a rollback needs no cause marker: the controller matches the spec to r1")

	assert.Equal(t, revsBefore, revisionVersions(t, c), "the service never writes a revision")
}

func TestRollbackReleaseBinding_Message(t *testing.T) {
	ctx := context.Background()

	t.Run("applied: written with the restored spec, for the controller's event", func(t *testing.T) {
		c := newFixture(t).client()
		_, err := NewService(c, testutil.TestLogger()).RollbackReleaseBinding(ctx, testNamespace, testRBName,
			bindingrevision.RollbackRequest{Revision: 1, Message: "Payments failing since r3"})
		require.NoError(t, err)
		rb := getRB(t, c)
		assert.Equal(t, specFor(releaseA, ""), rb.Spec)
		assert.Equal(t, "Payments failing since r3", rb.Annotations[openchoreov1alpha1.AnnotationKeyChangeMessage])
	})

	t.Run("dry run writes nothing", func(t *testing.T) {
		c := newFixture(t).client()
		before := getRB(t, c)
		_, err := NewService(c, testutil.TestLogger()).RollbackReleaseBinding(ctx, testNamespace, testRBName,
			bindingrevision.RollbackRequest{Revision: 1, DryRun: true, Message: "Payments failing since r3"})
		require.NoError(t, err)
		after := getRB(t, c)
		assert.Equal(t, before.ResourceVersion, after.ResourceVersion)
		assert.NotContains(t, after.Annotations, openchoreov1alpha1.AnnotationKeyChangeMessage)
	})

	t.Run("no message: no annotation", func(t *testing.T) {
		c := newFixture(t).client()
		_, err := NewService(c, testutil.TestLogger()).RollbackReleaseBinding(ctx, testNamespace, testRBName,
			bindingrevision.RollbackRequest{Revision: 1})
		require.NoError(t, err)
		assert.NotContains(t, getRB(t, c).Annotations, openchoreov1alpha1.AnnotationKeyChangeMessage)
	})
}

func TestRollbackReleaseBinding_ActiveIsTheMostRecentlyActivated(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	// The binding was already rolled back to r1: r1 was made active at a later
	// binding generation than r3, the highest-numbered revision.
	f.binding.Spec = specFor(releaseA, "")
	f.revs[0].Annotations = map[string]string{revision.AnnotationKeyBindingGeneration: "9"}
	f.revs[2].Annotations = map[string]string{revision.AnnotationKeyBindingGeneration: "5"}

	result, err := NewService(f.client(), testutil.TestLogger()).RollbackReleaseBinding(ctx, testNamespace, testRBName,
		bindingrevision.RollbackRequest{Revision: 3, DryRun: true})
	require.NoError(t, err)
	assert.Equal(t, ptr.To(int64(1)), result.RevertedFrom, "r1 is active, though r3 is newer")
	assert.Equal(t, testRBName+"-r3", result.RevisionName)
	assert.NotContains(t, warningCodes(result.Warnings), bindingrevision.WarningUnrecordedChange,
		"the binding's spec is the one r1 records")
}

func TestRollbackReleaseBinding_DryRunChangesNothing(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	c := f.client()
	svc := NewService(c, testutil.TestLogger())
	before := getRB(t, c)
	revsBefore := revisionVersions(t, c)

	result, err := svc.RollbackReleaseBinding(ctx, testNamespace, testRBName,
		bindingrevision.RollbackRequest{Revision: 1, DryRun: true})
	require.NoError(t, err)

	assert.True(t, result.DryRun)
	assert.Equal(t, testRBName+"-r1", result.RevisionName)

	fields := map[string]bindingrevision.SpecChange{}
	for _, ch := range result.Changes {
		fields[ch.Field] = ch
	}
	assert.Len(t, result.Changes, 2, "releaseName and the override change: %+v", result.Changes)
	assert.Equal(t, releaseC, fields["releaseName"].Current)
	assert.Equal(t, releaseA, fields["releaseName"].Target)
	assert.Equal(t, map[string]any{"replicas": float64(3)}, fields["componentTypeEnvironmentConfigs"].Current)
	assert.Nil(t, fields["componentTypeEnvironmentConfigs"].Target, "the rollback unsets the override")

	after := getRB(t, c)
	assert.Equal(t, before.ResourceVersion, after.ResourceVersion, "dry run writes nothing")
	assert.Equal(t, before.Spec, after.Spec)
	assert.Equal(t, revsBefore, revisionVersions(t, c))
}

func TestRollbackReleaseBinding_Errors(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name    string
		setup   func(t *testing.T, f *fixture)
		binding string
		req     bindingrevision.RollbackRequest
		wantErr error
		wantVal bool
	}{
		{
			name:    "unknown revision is not found",
			req:     bindingrevision.RollbackRequest{Revision: 9},
			wantErr: bindingrevision.ErrRevisionNotFound,
		},
		{
			name: "a revision of another binding with the same name is not found",
			setup: func(t *testing.T, f *fixture) {
				f.revs[0] = newRevision(t, testRBName, "uid-deleted-binding", 1, specFor(releaseA, ""), t0)
			},
			req:     bindingrevision.RollbackRequest{Revision: 1},
			wantErr: bindingrevision.ErrRevisionNotFound,
		},
		{
			name:    "unknown binding is not found",
			binding: "no-such-binding",
			req:     bindingrevision.RollbackRequest{Revision: 1},
			wantErr: ErrReleaseBindingNotFound,
		},
		{
			name:    "a negative revision is rejected",
			req:     bindingrevision.RollbackRequest{Revision: -1},
			wantVal: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t)
			if tt.setup != nil {
				tt.setup(t, f)
			}
			c := f.client()
			before := getRB(t, c)
			name := testRBName
			if tt.binding != "" {
				name = tt.binding
			}

			_, err := NewService(c, testutil.TestLogger()).RollbackReleaseBinding(ctx, testNamespace, name, tt.req)
			require.Error(t, err)
			if tt.wantVal {
				var vErr *services.ValidationError
				assert.ErrorAs(t, err, &vErr)
			} else {
				assert.ErrorIs(t, err, tt.wantErr)
			}
			assert.Equal(t, before.ResourceVersion, getRB(t, c).ResourceVersion, "a failed rollback writes nothing")
		})
	}
}

func TestRollbackReleaseBinding_ReleaseDeletedIsConflict(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	objs := []client.Object{}
	for _, o := range f.objects() {
		if o.GetName() != releaseA { // r1's ComponentRelease has been deleted
			objs = append(objs, o)
		}
	}
	c := testutil.NewFakeClient(objs...)
	before := getRB(t, c)

	_, err := NewService(c, testutil.TestLogger()).RollbackReleaseBinding(ctx, testNamespace, testRBName,
		bindingrevision.RollbackRequest{Revision: 1})
	require.ErrorIs(t, err, bindingrevision.ErrReleaseDeleted)
	assert.Contains(t, err.Error(), releaseA)
	assert.Equal(t, before.ResourceVersion, getRB(t, c).ResourceVersion)

	// A dry run is blocked the same way.
	_, err = NewService(c, testutil.TestLogger()).RollbackReleaseBinding(ctx, testNamespace, testRBName,
		bindingrevision.RollbackRequest{Revision: 1, DryRun: true})
	require.ErrorIs(t, err, bindingrevision.ErrReleaseDeleted)
}

func TestRollbackReleaseBinding_ConcurrentEditIsConflict(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)

	// Someone edits the binding between the rollback reading it and patching it.
	c := fake.NewClientBuilder().
		WithScheme(testutil.NewScheme()).
		WithObjects(f.objects()...).
		WithInterceptorFuncs(interceptor.Funcs{
			Patch: func(ctx context.Context, cl client.WithWatch, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
				if _, ok := obj.(*openchoreov1alpha1.ReleaseBinding); ok {
					other := &openchoreov1alpha1.ReleaseBinding{}
					if err := cl.Get(ctx, client.ObjectKeyFromObject(obj), other); err != nil {
						return err
					}
					other.Spec.ReleaseName = releaseB
					if err := cl.Update(ctx, other); err != nil {
						return err
					}
				}
				return cl.Patch(ctx, obj, patch, opts...)
			},
		}).
		Build()

	_, err := NewService(c, testutil.TestLogger()).RollbackReleaseBinding(ctx, testNamespace, testRBName,
		bindingrevision.RollbackRequest{Revision: 1})
	require.ErrorIs(t, err, bindingrevision.ErrConcurrentModification)

	rb := getRB(t, c)
	assert.Equal(t, releaseB, rb.Spec.ReleaseName, "the concurrent edit stands; the rollback did not overwrite it")
	assert.NotContains(t, rb.Annotations, openchoreov1alpha1.AnnotationKeyRevisionCause)
}

func TestRollbackReleaseBinding_Warnings(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name    string
		setup   func(t *testing.T, f *fixture)
		target  int64
		want    []bindingrevision.WarningCode
		wantNot []bindingrevision.WarningCode
	}{
		{
			name:    "rolling back to a known-good, healthy revision warns about nothing",
			target:  1,
			wantNot: []bindingrevision.WarningCode{bindingrevision.WarningTargetNotKnownGood, bindingrevision.WarningTargetUnhealthy},
		},
		{
			name:   "target never became known-good",
			target: 2,
			want:   []bindingrevision.WarningCode{bindingrevision.WarningTargetNotKnownGood},
		},
		{
			name: "target was Degraded when it stopped being active",
			setup: func(_ *testing.T, f *fixture) {
				f.revs[1].Status = openchoreov1alpha1.BindingRevisionStatus{Health: openchoreov1alpha1.RevisionHealthDegraded}
			},
			target: 2,
			want:   []bindingrevision.WarningCode{bindingrevision.WarningTargetNotKnownGood, bindingrevision.WarningTargetUnhealthy},
		},
		{
			name: "target undeploys",
			setup: func(t *testing.T, f *fixture) {
				spec := specFor(releaseB, "")
				spec.State = openchoreov1alpha1.ReleaseStateUndeploy
				f.revs[1] = newRevision(t, testRBName, rbUID, 2, spec, t0.Add(time.Hour))
			},
			target: 2,
			want:   []bindingrevision.WarningCode{bindingrevision.WarningTargetUndeploys},
		},
		{
			name:   "target records the current spec: nothing changes",
			target: 3,
			want:   []bindingrevision.WarningCode{bindingrevision.WarningNoSpecChange},
		},
		{
			name: "the binding has a change the controller has not recorded",
			setup: func(_ *testing.T, f *fixture) {
				f.binding.Spec.ComponentTypeEnvironmentConfigs = &runtime.RawExtension{Raw: []byte(`{"replicas":5}`)}
			},
			target: 1,
			want:   []bindingrevision.WarningCode{bindingrevision.WarningUnrecordedChange},
		},
		{
			name: "the project's cell moved to a ProjectRelease created after the target",
			setup: func(_ *testing.T, f *fixture) {
				pr := testutil.NewProjectRelease(testNamespace, testProjectName, "test-project-cell-2")
				pr.CreationTimestamp = metav1.NewTime(t0.Add(30 * time.Minute)) // after r1, before r2
				prb := testutil.NewProjectReleaseBinding(testNamespace, testProjectName, testEnvironmentName, "test-project-dev")
				prb.Spec.ProjectRelease = pr.Name
				f.extra = append(f.extra, pr, prb)
			},
			target: 1,
			want:   []bindingrevision.WarningCode{bindingrevision.WarningProjectCellDrift},
		},
		{
			name: "a ProjectRelease older than the target is no drift",
			setup: func(_ *testing.T, f *fixture) {
				pr := testutil.NewProjectRelease(testNamespace, testProjectName, "test-project-cell-1")
				pr.CreationTimestamp = metav1.NewTime(t0.Add(-time.Hour))
				prb := testutil.NewProjectReleaseBinding(testNamespace, testProjectName, testEnvironmentName, "test-project-dev")
				prb.Spec.ProjectRelease = pr.Name
				f.extra = append(f.extra, pr, prb)
			},
			target:  1,
			wantNot: []bindingrevision.WarningCode{bindingrevision.WarningProjectCellDrift},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t)
			if tt.setup != nil {
				tt.setup(t, f)
			}
			result, err := NewService(f.client(), testutil.TestLogger()).RollbackReleaseBinding(ctx, testNamespace,
				testRBName, bindingrevision.RollbackRequest{Revision: tt.target, DryRun: true})
			require.NoError(t, err)

			codes := warningCodes(result.Warnings)
			for _, w := range tt.want {
				assert.Contains(t, codes, w)
			}
			for _, w := range tt.wantNot {
				assert.NotContains(t, codes, w)
			}
			// A rollback always makes its target active again; it never creates one.
			assert.Equal(t, tt.target, result.Revision)
			assert.Equal(t, revision.Name(testRBName, tt.target), result.RevisionName)
		})
	}
}

func TestRollbackReleaseBinding_SecretReferenceDrift(t *testing.T) {
	ctx := context.Background()

	secretRefFixture := func(createdAt time.Time, keys ...string) *openchoreov1alpha1.SecretReference {
		ref := &openchoreov1alpha1.SecretReference{
			ObjectMeta: metav1.ObjectMeta{Name: secretRef, Namespace: testNamespace, CreationTimestamp: metav1.NewTime(createdAt)},
		}
		for _, k := range keys {
			ref.Spec.Data = append(ref.Spec.Data, openchoreov1alpha1.SecretDataSource{SecretKey: k})
		}
		return ref
	}

	tests := []struct {
		name      string
		secretRef *openchoreov1alpha1.SecretReference
		wantMsgs  []string
	}{
		{name: "present, older than the target, with the key: no warning", secretRef: secretRefFixture(t0.Add(-time.Hour), "password")},
		{name: "missing", secretRef: nil, wantMsgs: []string{`SecretReference "db-credentials" used by revision 1 no longer exists`}},
		{name: "key removed", secretRef: secretRefFixture(t0.Add(-time.Hour), "username"),
			wantMsgs: []string{`SecretReference "db-credentials" no longer has key "password" used by revision 1`}},
		{name: "recreated after the target", secretRef: secretRefFixture(t0.Add(time.Minute), "password"),
			wantMsgs: []string{`SecretReference "db-credentials" was recreated after revision 1`}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t)
			// r1's spec overrides an env var from the SecretReference.
			spec := specFor(releaseA, "")
			spec.WorkloadOverrides = &openchoreov1alpha1.WorkloadOverrideTemplateSpec{
				Container: &openchoreov1alpha1.ContainerOverride{Env: []openchoreov1alpha1.EnvVar{{
					Key:       "DB_PASSWORD",
					ValueFrom: &openchoreov1alpha1.EnvVarValueFrom{SecretKeyRef: &openchoreov1alpha1.SecretKeyRef{Name: secretRef, Key: "password"}},
				}}},
			}
			f.revs[0] = newRevision(t, testRBName, rbUID, 1, spec, t0)
			if tt.secretRef != nil {
				f.extra = append(f.extra, tt.secretRef)
			}

			result, err := NewService(f.client(), testutil.TestLogger()).RollbackReleaseBinding(ctx, testNamespace,
				testRBName, bindingrevision.RollbackRequest{Revision: 1, DryRun: true})
			require.NoError(t, err)

			var msgs []string
			for _, w := range result.Warnings {
				if w.Code == bindingrevision.WarningSecretReferenceDrift {
					msgs = append(msgs, w.Message)
				}
			}
			assert.Equal(t, tt.wantMsgs, msgs)
		})
	}
}

// ─────────────────────────────────────────────────────────────
// List and Get
// ─────────────────────────────────────────────────────────────

func TestListReleaseBindingRevisions(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)

	// A staging binding's history, and a revision of another component.
	staging := newRevision(t, "test-rb-staging", "uid-staging", 1, func() openchoreov1alpha1.ReleaseBindingSpec {
		s := specFor(releaseA, "")
		s.Environment = "staging"
		return s
	}(), t0)
	other := newRevision(t, "other-dev", "uid-other", 1, func() openchoreov1alpha1.ReleaseBindingSpec {
		s := specFor(releaseA, "")
		s.Owner.ComponentName = "other"
		return s
	}(), t0)
	f.extra = append(f.extra, staging, other)
	svc := NewService(f.client(), testutil.TestLogger())

	names := func(items []openchoreov1alpha1.ReleaseBindingRevision) []string {
		out := make([]string, 0, len(items))
		for _, i := range items {
			out = append(out, i.Name)
		}
		return out
	}

	t.Run("all environments, by environment then newest first", func(t *testing.T) {
		result, err := svc.ListReleaseBindingRevisions(ctx, testNamespace, testComponentName, "", "", services.ListOptions{})
		require.NoError(t, err)
		assert.Equal(t, []string{"test-rb-r3", "test-rb-r2", "test-rb-r1", "test-rb-staging-r1"}, names(result.Items))
		assert.Equal(t, releaseBindingRevisionTypeMeta, result.Items[0].TypeMeta)
	})

	t.Run("one environment", func(t *testing.T) {
		result, err := svc.ListReleaseBindingRevisions(ctx, testNamespace, testComponentName, "staging", "", services.ListOptions{})
		require.NoError(t, err)
		assert.Equal(t, []string{"test-rb-staging-r1"}, names(result.Items))
	})

	t.Run("pages keep the order", func(t *testing.T) {
		page1, err := svc.ListReleaseBindingRevisions(ctx, testNamespace, testComponentName, "", "", services.ListOptions{Limit: 3})
		require.NoError(t, err)
		assert.Equal(t, []string{"test-rb-r3", "test-rb-r2", "test-rb-r1"}, names(page1.Items))
		require.NotEmpty(t, page1.NextCursor)
		assert.Equal(t, ptr.To(int64(1)), page1.RemainingCount)

		page2, err := svc.ListReleaseBindingRevisions(ctx, testNamespace, testComponentName, "", "",
			services.ListOptions{Limit: 3, Cursor: page1.NextCursor})
		require.NoError(t, err)
		assert.Equal(t, []string{"test-rb-staging-r1"}, names(page2.Items))
		assert.Empty(t, page2.NextCursor)
	})

	t.Run("invalid cursor is a validation error", func(t *testing.T) {
		_, err := svc.ListReleaseBindingRevisions(ctx, testNamespace, testComponentName, "", "", services.ListOptions{Cursor: "%%%"})
		var vErr *services.ValidationError
		assert.True(t, errors.As(err, &vErr))
	})

	t.Run("unknown component", func(t *testing.T) {
		_, err := svc.ListReleaseBindingRevisions(ctx, testNamespace, "no-such-component", "", "", services.ListOptions{})
		assert.ErrorIs(t, err, ErrComponentNotFound)
	})
}

// TestListReleaseBindingRevisions_OneBindingPerReleaseLine covers a component
// with a release line: its main-line and line bindings share the environment
// and both count from r1, so only the binding filter keeps their histories apart.
func TestListReleaseBindingRevisions_OneBindingPerReleaseLine(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	lineSpec := specFor(releaseA, "")
	lineSpec.Owner.ReleaseLine = testReleaseLine
	f.extra = append(f.extra, newRevision(t, "test-rb-next", types.UID("next-uid"), 1, lineSpec, t0))
	svc := NewService(f.client(), testutil.TestLogger())

	names := func(items []openchoreov1alpha1.ReleaseBindingRevision) []string {
		out := make([]string, 0, len(items))
		for _, i := range items {
			out = append(out, i.Name)
		}
		return out
	}

	t.Run("without a binding, the environment mixes both lines", func(t *testing.T) {
		result, err := svc.ListReleaseBindingRevisions(ctx, testNamespace, testComponentName, testEnvironmentName, "",
			services.ListOptions{})
		require.NoError(t, err)
		assert.ElementsMatch(t, []string{"test-rb-r3", "test-rb-r2", "test-rb-r1", "test-rb-next-r1"}, names(result.Items))
	})

	t.Run("the main-line binding", func(t *testing.T) {
		result, err := svc.ListReleaseBindingRevisions(ctx, testNamespace, testComponentName, testEnvironmentName, testRBName,
			services.ListOptions{})
		require.NoError(t, err)
		assert.Equal(t, []string{"test-rb-r3", "test-rb-r2", "test-rb-r1"}, names(result.Items))
	})

	t.Run("the release line's binding", func(t *testing.T) {
		result, err := svc.ListReleaseBindingRevisions(ctx, testNamespace, testComponentName, "", "test-rb-next",
			services.ListOptions{})
		require.NoError(t, err)
		assert.Equal(t, []string{"test-rb-next-r1"}, names(result.Items))
	})

	t.Run("an unknown binding lists nothing", func(t *testing.T) {
		result, err := svc.ListReleaseBindingRevisions(ctx, testNamespace, testComponentName, "", "no-such-binding",
			services.ListOptions{})
		require.NoError(t, err)
		assert.Empty(t, result.Items)
	})
}

func TestGetReleaseBindingRevision(t *testing.T) {
	ctx := context.Background()
	svc := NewService(newFixture(t).client(), testutil.TestLogger())

	rev, err := svc.GetReleaseBindingRevision(ctx, testNamespace, "test-rb-r2")
	require.NoError(t, err)
	assert.Equal(t, int64(2), rev.Spec.Revision)
	assert.Equal(t, releaseBindingRevisionTypeMeta, rev.TypeMeta)

	_, err = svc.GetReleaseBindingRevision(ctx, testNamespace, "test-rb-r9")
	assert.ErrorIs(t, err, bindingrevision.ErrRevisionNotFound)
}

// ─────────────────────────────────────────────────────────────
// DeleteReleaseBindingRevision
// ─────────────────────────────────────────────────────────────

// deleteFixture is newFixture with the binding's status as the controller
// leaves it: r3 current and 3 the highest revision recorded.
func deleteFixture(t *testing.T) *fixture {
	t.Helper()
	f := newFixture(t)
	f.binding.Status.CurrentRevision = f.revs[2].Name
	f.binding.Status.HighestRevision = 3
	return f
}

func TestDeleteReleaseBindingRevision_DeletesAnInactiveRevision(t *testing.T) {
	ctx := context.Background()
	f := deleteFixture(t)
	c := f.client()
	before := revisionVersions(t, c)
	bindingVersion := getRB(t, c).ResourceVersion

	got, err := NewService(c, testutil.TestLogger()).DeleteReleaseBindingRevision(ctx, testNamespace,
		f.revs[1].Name, bindingrevision.DeleteRequest{Binding: testRBName})
	require.NoError(t, err)
	assert.Equal(t, &bindingrevision.DeleteResult{
		RevisionName: f.revs[1].Name, RevisionDisplayName: testComponentName + "-r2", Revision: 2,
		Binding: testRBName, Environment: testEnvironmentName,
	}, got)

	after := revisionVersions(t, c)
	assert.NotContains(t, after, f.revs[1].Name)
	assert.Equal(t, before[f.revs[0].Name], after[f.revs[0].Name], "r1 is untouched and keeps its number")
	assert.Equal(t, before[f.revs[2].Name], after[f.revs[2].Name], "r3 is untouched and keeps its number")
	rb := getRB(t, c)
	assert.Equal(t, bindingVersion, rb.ResourceVersion, "the binding is not written")
	assert.Equal(t, f.revs[2].Name, rb.Status.CurrentRevision)
}

func TestDeleteReleaseBindingRevision_KeepAnnotationDoesNotProtect(t *testing.T) {
	ctx := context.Background()
	f := deleteFixture(t)
	f.revs[1].Annotations = map[string]string{"openchoreo.dev/keep": "true"}
	c := f.client()

	_, err := NewService(c, testutil.TestLogger()).DeleteReleaseBindingRevision(ctx, testNamespace,
		f.revs[1].Name, bindingrevision.DeleteRequest{Binding: testRBName})
	require.NoError(t, err, "openchoreo.dev/keep is not honored")
	assert.NotContains(t, revisionVersions(t, c), f.revs[1].Name)
}

func TestDeleteReleaseBindingRevision_DryRunDeletesNothing(t *testing.T) {
	f := deleteFixture(t)
	c := f.client()
	before := revisionVersions(t, c)

	got, err := NewService(c, testutil.TestLogger()).DeleteReleaseBindingRevision(context.Background(), testNamespace,
		f.revs[1].Name, bindingrevision.DeleteRequest{DryRun: true})
	require.NoError(t, err)
	assert.True(t, got.DryRun)
	assert.Equal(t, before, revisionVersions(t, c))
}

func TestDeleteReleaseBindingRevision_Refused(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(f *fixture)
		target  func(f *fixture) string
		req     bindingrevision.DeleteRequest
		wantErr error
		wantMsg string
	}{
		{name: "the active revision", target: func(f *fixture) string { return f.revs[2].Name },
			wantErr: bindingrevision.ErrRevisionProtected,
			wantMsg: "cannot delete revision 3 because it is currently active"},
		{name: "the revision status.currentRevision names",
			mutate:  func(f *fixture) { f.binding.Status.CurrentRevision = f.revs[1].Name },
			target:  func(f *fixture) string { return f.revs[1].Name },
			wantErr: bindingrevision.ErrRevisionProtected, wantMsg: "currently active"},
		{name: "the newest known-good revision", target: func(f *fixture) string { return f.revs[0].Name },
			wantErr: bindingrevision.ErrRevisionProtected, wantMsg: "newest known-good revision"},
		{name: "a revision a rollback is restoring",
			mutate:  func(f *fixture) { f.binding.Spec = f.revs[1].Spec.BindingSpec },
			target:  func(f *fixture) string { return f.revs[1].Name },
			wantErr: bindingrevision.ErrRevisionProtected, wantMsg: "a rollback to it is being applied"},
		{name: "a revision the controller has not finished recording",
			mutate:  func(f *fixture) { f.binding.Status.HighestRevision = 1 },
			target:  func(f *fixture) string { return f.revs[1].Name },
			wantErr: bindingrevision.ErrRevisionProtected, wantMsg: "has not finished recording"},
		{name: "an unknown revision", target: func(*fixture) string { return testRBName + "-r9" },
			wantErr: bindingrevision.ErrRevisionNotFound},
		{name: "a revision of another binding", target: func(f *fixture) string { return f.revs[1].Name },
			req: bindingrevision.DeleteRequest{Binding: "other-binding"}, wantErr: bindingrevision.ErrRevisionNotFound,
			wantMsg: `is not a revision of release binding "other-binding"`},
		{name: "a revision whose binding is gone",
			mutate:  func(f *fixture) { f.binding.UID = "uid-recreated" },
			target:  func(f *fixture) string { return f.revs[1].Name },
			wantErr: ErrReleaseBindingNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := deleteFixture(t)
			if tt.mutate != nil {
				tt.mutate(f)
			}
			c := f.client()
			before := revisionVersions(t, c)

			_, err := NewService(c, testutil.TestLogger()).DeleteReleaseBindingRevision(context.Background(),
				testNamespace, tt.target(f), tt.req)
			require.ErrorIs(t, err, tt.wantErr)
			if tt.wantMsg != "" {
				assert.ErrorContains(t, err, tt.wantMsg)
			}
			assert.Equal(t, before, revisionVersions(t, c), "nothing is deleted")
		})
	}
}

func TestDeleteReleaseBindingRevision_ActivatedMeanwhileIsConflict(t *testing.T) {
	ctx := context.Background()
	f := deleteFixture(t)

	// The controller makes r2 active again between the checks and the delete:
	// it patches r2's metadata, changing its resourceVersion.
	c := fake.NewClientBuilder().
		WithScheme(testutil.NewScheme()).
		WithObjects(f.objects()...).
		WithInterceptorFuncs(interceptor.Funcs{
			Delete: func(ctx context.Context, cl client.WithWatch, obj client.Object, opts ...client.DeleteOption) error {
				rev := &openchoreov1alpha1.ReleaseBindingRevision{}
				if err := cl.Get(ctx, client.ObjectKeyFromObject(obj), rev); err != nil {
					return err
				}
				rev.Annotations = map[string]string{revision.AnnotationKeyBindingGeneration: "9"}
				if err := cl.Update(ctx, rev); err != nil {
					return err
				}
				return cl.Delete(ctx, obj, opts...)
			},
		}).
		Build()

	_, err := NewService(c, testutil.TestLogger()).DeleteReleaseBindingRevision(ctx, testNamespace,
		f.revs[1].Name, bindingrevision.DeleteRequest{})
	require.ErrorIs(t, err, bindingrevision.ErrConcurrentModification)
	assert.Contains(t, revisionVersions(t, c), f.revs[1].Name, "the revision made active is not deleted")
}

func TestRollbackReleaseBinding_ToADeletedRevisionIsNotFound(t *testing.T) {
	ctx := context.Background()
	f := deleteFixture(t)
	c := f.client()
	svc := NewService(c, testutil.TestLogger())

	_, err := svc.DeleteReleaseBindingRevision(ctx, testNamespace, f.revs[1].Name, bindingrevision.DeleteRequest{})
	require.NoError(t, err)

	_, err = svc.RollbackReleaseBinding(ctx, testNamespace, testRBName, bindingrevision.RollbackRequest{Revision: 2})
	require.ErrorIs(t, err, bindingrevision.ErrRevisionNotFound)
	assert.ErrorContains(t, err, "revision 2 of release binding")
	assert.ErrorContains(t, err, "has been deleted or never existed")
	assert.Equal(t, releaseC, getRB(t, c).Spec.ReleaseName, "the binding is not changed")
	assert.NotContains(t, revisionVersions(t, c), f.revs[1].Name, "the deleted revision is not recreated")
}

// ─────────────────────────────────────────────────────────────
// RollbackReleaseBinding without a revision
// ─────────────────────────────────────────────────────────────

// activated records on rev the binding generation it last became active at
// and the revision it replaced then (-1 for none recorded, as before activation
// tracking).
func activated(rev *openchoreov1alpha1.ReleaseBindingRevision, generation, previous int64) {
	rev.Annotations = map[string]string{revision.AnnotationKeyBindingGeneration: strconv.FormatInt(generation, 10)}
	if previous >= 0 {
		rev.Annotations[revision.AnnotationKeyPreviousRevision] = strconv.FormatInt(previous, 10)
	}
}

// historyFixture is newFixture with its activation history: r1, then r2, then r3.
func historyFixture(t *testing.T) *fixture {
	t.Helper()
	f := newFixture(t)
	activated(f.revs[0], 1, 0)
	activated(f.revs[1], 2, 1)
	activated(f.revs[2], 3, 2)
	f.binding.Status.HighestRevision = 3
	return f
}

func TestRollbackReleaseBinding_WithoutRevision(t *testing.T) {
	ctx := context.Background()

	t.Run("previews and applies the previously active revision", func(t *testing.T) {
		f := historyFixture(t)
		c := f.client()
		svc := NewService(c, testutil.TestLogger())

		preview, err := svc.RollbackReleaseBinding(ctx, testNamespace, testRBName, bindingrevision.RollbackRequest{DryRun: true})
		require.NoError(t, err)
		assert.Equal(t, int64(2), preview.TargetRevision)
		assert.Equal(t, f.revs[1].Name, preview.RevisionName)
		assert.Equal(t, releaseC, getRB(t, c).Spec.ReleaseName, "a dry run writes nothing")

		applied, err := svc.RollbackReleaseBinding(ctx, testNamespace, testRBName, bindingrevision.RollbackRequest{})
		require.NoError(t, err)
		assert.Equal(t, preview.TargetRevision, applied.TargetRevision, "preview and rollback pick the same target")
		rb := getRB(t, c)
		assert.Equal(t, releaseB, rb.Spec.ReleaseName)
	})

	t.Run("after a rollback to r1 the previous revision is r3, numbered above it", func(t *testing.T) {
		f := historyFixture(t)
		activated(f.revs[0], 4, 3)
		f.revs[0].Status.Active, f.revs[2].Status.Active = true, false
		f.binding.Spec = f.revs[0].Spec.BindingSpec

		got, err := NewService(f.client(), testutil.TestLogger()).RollbackReleaseBinding(ctx, testNamespace, testRBName,
			bindingrevision.RollbackRequest{DryRun: true})
		require.NoError(t, err)
		assert.Equal(t, int64(3), got.TargetRevision)
		assert.Equal(t, int64(1), *got.RevertedFrom)
	})

	t.Run("an explicit revision is unaffected by the history", func(t *testing.T) {
		got, err := NewService(historyFixture(t).client(), testutil.TestLogger()).RollbackReleaseBinding(ctx, testNamespace,
			testRBName, bindingrevision.RollbackRequest{Revision: 1, DryRun: true})
		require.NoError(t, err)
		assert.Equal(t, int64(1), got.TargetRevision)
	})

	refusals := []struct {
		name    string
		setup   func(f *fixture)
		objects func(f *fixture) []client.Object
		wantErr error
		wantMsg string
	}{
		{name: "the previously active revision was deleted",
			setup:   func(f *fixture) { f.revs = []*openchoreov1alpha1.ReleaseBindingRevision{f.revs[0], f.revs[2]} },
			wantErr: bindingrevision.ErrNoPreviousRevision, wantMsg: "the previously active revision has been deleted: revision 2"},
		{name: "the current revision was the first activation",
			setup:   func(f *fixture) { activated(f.revs[2], 3, 0) },
			wantErr: bindingrevision.ErrNoPreviousRevision, wantMsg: "was the binding's first"},
		{name: "the previous revision's release was deleted",
			objects: func(f *fixture) []client.Object {
				var objs []client.Object
				for _, o := range f.objects() {
					if cr, ok := o.(*openchoreov1alpha1.ComponentRelease); ok && cr.Name == releaseB {
						continue
					}
					objs = append(objs, o)
				}
				return objs
			},
			wantErr: bindingrevision.ErrReleaseDeleted},
		{name: "the binding has a change the controller has not recorded",
			setup:   func(f *fixture) { f.binding.Spec = specFor(releaseA, "9") },
			wantErr: bindingrevision.ErrConcurrentModification, wantMsg: "has not recorded yet"},
	}
	for _, tt := range refusals {
		t.Run("refused: "+tt.name, func(t *testing.T) {
			f := historyFixture(t)
			if tt.setup != nil {
				tt.setup(f)
			}
			objs := f.objects()
			if tt.objects != nil {
				objs = tt.objects(f)
			}
			c := testutil.NewFakeClient(objs...)
			before := getRB(t, c).ResourceVersion

			_, err := NewService(c, testutil.TestLogger()).RollbackReleaseBinding(ctx, testNamespace, testRBName,
				bindingrevision.RollbackRequest{})
			require.ErrorIs(t, err, tt.wantErr)
			if tt.wantMsg != "" {
				assert.ErrorContains(t, err, tt.wantMsg)
			}
			assert.Equal(t, before, getRB(t, c).ResourceVersion, "the binding is unchanged")
		})
	}

	t.Run("legacy history with a gap is inferred from activation order, with a warning", func(t *testing.T) {
		f := historyFixture(t)
		activated(f.revs[0], 1, -1)
		activated(f.revs[2], 3, -1)
		f.revs = []*openchoreov1alpha1.ReleaseBindingRevision{f.revs[0], f.revs[2]}
		got, err := NewService(f.client(), testutil.TestLogger()).RollbackReleaseBinding(ctx, testNamespace, testRBName,
			bindingrevision.RollbackRequest{DryRun: true})
		require.NoError(t, err)
		assert.Equal(t, int64(1), got.TargetRevision)
		require.NotEmpty(t, got.Warnings)
		assert.Equal(t, bindingrevision.WarningPreviousRevisionInferred, got.Warnings[0].Code)
		assert.Contains(t, got.Warnings[0].Message, "a deleted revision may have been active in between")
	})

	t.Run("legacy history without gaps follows activation order", func(t *testing.T) {
		f := historyFixture(t)
		for i, r := range f.revs {
			activated(r, int64(i+1), -1)
		}
		got, err := NewService(f.client(), testutil.TestLogger()).RollbackReleaseBinding(ctx, testNamespace, testRBName,
			bindingrevision.RollbackRequest{DryRun: true})
		require.NoError(t, err)
		assert.Equal(t, int64(2), got.TargetRevision)
		assert.NotContains(t, warningCodes(got.Warnings), bindingrevision.WarningPreviousRevisionInferred,
			"a history without gaps is certain")
	})
}

// ─────────────────────────────────────────────────────────────
// RollbackReleaseBinding and the component's auto-deploy
// ─────────────────────────────────────────────────────────────

// autoDeployObjects is the fixture with a project and pipeline (dev -> prod, so
// dev, the binding's environment, is the root) and a component whose autoDeploy
// is as given.
func autoDeployObjects(f *fixture, autoDeploy bool) []client.Object {
	objs := make([]client.Object, 0, len(f.objects())+2)
	for _, o := range f.objects() {
		if _, ok := o.(*openchoreov1alpha1.Component); ok {
			continue
		}
		objs = append(objs, o)
	}
	comp := testutil.NewComponent(testNamespace, testProjectName, testComponentName)
	comp.Spec.AutoDeploy = autoDeploy
	return append(objs, comp,
		testutil.NewProject(testNamespace, testProjectName),
		testutil.NewDeploymentPipeline(testNamespace, "default"))
}

func componentAutoDeploy(t *testing.T, c client.Client) bool {
	t.Helper()
	comp := &openchoreov1alpha1.Component{}
	require.NoError(t, c.Get(context.Background(), client.ObjectKey{Namespace: testNamespace, Name: testComponentName}, comp))
	return comp.Spec.AutoDeploy
}

func TestRollbackReleaseBinding_AutoDeploy(t *testing.T) {
	ctx := context.Background()
	rollback := func(c client.Client, dryRun bool) (*bindingrevision.RollbackResult, error) {
		return NewService(c, testutil.TestLogger()).RollbackReleaseBinding(ctx, testNamespace, testRBName,
			bindingrevision.RollbackRequest{Revision: 1, DryRun: dryRun})
	}

	t.Run("first environment, auto-deploy on: refused, dry run included, nothing changes", func(t *testing.T) {
		c := testutil.NewFakeClient(autoDeployObjects(newFixture(t), true)...)
		before := getRB(t, c).ResourceVersion
		for _, dryRun := range []bool{true, false} {
			_, err := rollback(c, dryRun)
			require.ErrorIs(t, err, bindingrevision.ErrAutoDeployEnabled, "dryRun=%t", dryRun)
			assert.ErrorContains(t, err, `auto-deploy is enabled for component "`+testComponentName+`", and "`+
				testEnvironmentName+`" is the first environment of its deployment pipeline`)
			assert.ErrorContains(t, err, "Turn off auto-deploy for the component, then roll back again")
		}
		assert.Equal(t, before, getRB(t, c).ResourceVersion, "the binding is unchanged")
		assert.True(t, componentAutoDeploy(t, c), "auto-deploy is not touched")
	})

	t.Run("first environment, auto-deploy off: rolls back normally", func(t *testing.T) {
		c := testutil.NewFakeClient(autoDeployObjects(newFixture(t), false)...)
		got, err := rollback(c, false)
		require.NoError(t, err)
		assert.Equal(t, int64(1), got.TargetRevision)
		assert.Equal(t, releaseA, getRB(t, c).Spec.ReleaseName)
		assert.False(t, componentAutoDeploy(t, c))
	})

	t.Run("another environment, auto-deploy on: rolls back normally", func(t *testing.T) {
		objs := autoDeployObjects(newFixture(t), true)
		for _, o := range objs {
			if p, ok := o.(*openchoreov1alpha1.DeploymentPipeline); ok {
				// The root is qa; the binding's environment is its target.
				p.Spec.PromotionPaths = []openchoreov1alpha1.PromotionPath{{
					SourceEnvironmentRef:  openchoreov1alpha1.EnvironmentRef{Name: "qa"},
					TargetEnvironmentRefs: []openchoreov1alpha1.TargetEnvironmentRef{{Name: testEnvironmentName}},
				}}
			}
		}
		c := testutil.NewFakeClient(objs...)
		_, err := rollback(c, false)
		require.NoError(t, err)
		assert.Equal(t, releaseA, getRB(t, c).Spec.ReleaseName)
		assert.True(t, componentAutoDeploy(t, c), "auto-deploy is not touched")
	})

	t.Run("the first environment comes from the pipeline, not the environment's name", func(t *testing.T) {
		objs := autoDeployObjects(newFixture(t), true)
		for _, o := range objs {
			if p, ok := o.(*openchoreov1alpha1.DeploymentPipeline); ok {
				// dev is not first here: staging is, and promotes to dev.
				p.Spec.PromotionPaths = []openchoreov1alpha1.PromotionPath{{
					SourceEnvironmentRef:  openchoreov1alpha1.EnvironmentRef{Name: "staging"},
					TargetEnvironmentRefs: []openchoreov1alpha1.TargetEnvironmentRef{{Name: testEnvironmentName}},
				}}
			}
		}
		_, err := rollback(testutil.NewFakeClient(objs...), true)
		require.NoError(t, err)
	})
}

// releaseLineObjects is autoDeployObjects with the binding on release line
// "next" and, when withLine is true, that line's ReleaseLine with the given
// spec.autoDeploy (nil inherits the component's).
func releaseLineObjects(f *fixture, componentAutoDeploy bool, withLine bool, lineAutoDeploy *bool) []client.Object {
	f.binding.Spec.Owner.ReleaseLine = testReleaseLine
	objs := autoDeployObjects(f, componentAutoDeploy)
	if withLine {
		objs = append(objs, &openchoreov1alpha1.ReleaseLine{
			ObjectMeta: metav1.ObjectMeta{Name: testReleaseLine, Namespace: testNamespace},
			Spec: openchoreov1alpha1.ReleaseLineSpec{
				Owner:      openchoreov1alpha1.ReleaseLineOwner{ProjectName: testProjectName, ComponentName: testComponentName},
				AutoDeploy: lineAutoDeploy,
			},
		})
	}
	return objs
}

func TestRollbackReleaseBinding_ReleaseLineAutoDeploy(t *testing.T) {
	ctx := context.Background()
	dryRun := func(c client.Client) error {
		_, err := NewService(c, testutil.TestLogger()).RollbackReleaseBinding(ctx, testNamespace, testRBName,
			bindingrevision.RollbackRequest{Revision: 1, DryRun: true})
		return err
	}
	on, off := true, false

	t.Run("the line's auto-deploy on, component's off: refused, naming the line", func(t *testing.T) {
		err := dryRun(testutil.NewFakeClient(releaseLineObjects(newFixture(t), false, true, &on)...))
		require.ErrorIs(t, err, bindingrevision.ErrAutoDeployEnabled)
		assert.ErrorContains(t, err, `auto-deploy is enabled for release line "next" of component "`+testComponentName+`"`)
		assert.ErrorContains(t, err, "Turn off auto-deploy for the release line, then roll back again")
	})

	t.Run("the line inherits the component's auto-deploy: refused", func(t *testing.T) {
		err := dryRun(testutil.NewFakeClient(releaseLineObjects(newFixture(t), true, true, nil)...))
		require.ErrorIs(t, err, bindingrevision.ErrAutoDeployEnabled)
	})

	t.Run("the line turns off the component's auto-deploy: allowed", func(t *testing.T) {
		require.NoError(t, dryRun(testutil.NewFakeClient(releaseLineObjects(newFixture(t), true, true, &off)...)))
	})

	t.Run("the line does not exist, so nothing auto-deploys it: allowed", func(t *testing.T) {
		require.NoError(t, dryRun(testutil.NewFakeClient(releaseLineObjects(newFixture(t), true, false, nil)...)))
	})

	t.Run("a ReleaseLine of the same name owned by another component does not count: allowed", func(t *testing.T) {
		objs := releaseLineObjects(newFixture(t), false, true, &on)
		for _, o := range objs {
			if rl, ok := o.(*openchoreov1alpha1.ReleaseLine); ok {
				rl.Spec.Owner.ComponentName = "other"
			}
		}
		require.NoError(t, dryRun(testutil.NewFakeClient(objs...)))
	})

	t.Run("a main-line binding ignores release lines: the component's auto-deploy decides", func(t *testing.T) {
		f := newFixture(t)
		objs := releaseLineObjects(f, false, true, &on)
		f.binding.Spec.Owner.ReleaseLine = ""
		require.NoError(t, dryRun(testutil.NewFakeClient(objs...)))
	})
}
