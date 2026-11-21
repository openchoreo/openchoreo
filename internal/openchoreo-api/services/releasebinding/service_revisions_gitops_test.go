// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package releasebinding

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/bindingrevision"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/testutil"
)

// fluxLabels are the labels Flux's kustomize-controller puts on every object a
// Kustomization applies.
var fluxLabels = map[string]string{
	"kustomize.toolkit.fluxcd.io/name":      "apps",
	"kustomize.toolkit.fluxcd.io/namespace": "flux-system",
}

const wantGitOpsMessage = `rollback blocked: the binding is managed by GitOps: release binding "` + testRBName +
	`" is applied from Git by Flux Kustomization "flux-system/apps". Rollback through OpenChoreo revision history ` +
	`is not supported for GitOps-managed bindings, because GitOps would restore the configuration in Git on its ` +
	`next sync. Revert the binding's configuration in Git and let GitOps reconcile the change`

// gitOpsFixture is historyFixture -- r1, r2, r3 active, with activation history so
// a rollback without a revision has a target -- on a binding carrying labels.
func gitOpsFixture(t *testing.T, bindingLabels map[string]string) *fixture {
	t.Helper()
	f := historyFixture(t)
	f.binding.Labels = bindingLabels
	return f
}

// requireUntouched asserts that the binding and its revisions are exactly as they
// were: same resourceVersion, spec and annotations, and no revision written.
func requireUntouched(t *testing.T, c client.Client, before *openchoreov1alpha1.ReleaseBinding, revsBefore map[string]string) {
	t.Helper()
	after := getRB(t, c)
	assert.Equal(t, before.ResourceVersion, after.ResourceVersion, "the binding is not written")
	assert.Equal(t, before.Spec, after.Spec, "the binding spec is unchanged")
	assert.Equal(t, before.Annotations, after.Annotations, "no change message or cause is written")
	assert.Equal(t, revsBefore, revisionVersions(t, c), "no revision is written")
}

func TestRollbackReleaseBinding_GitOpsManagedIsRefused(t *testing.T) {
	ctx := context.Background()
	requests := map[string]bindingrevision.RollbackRequest{
		"explicit revision":             {Revision: 1, Message: "back to A"},
		"explicit revision, dry run":    {Revision: 1, DryRun: true},
		"previous revision":             {},
		"previous revision, dry run":    {DryRun: true},
		"the revision already restored": {Revision: 3},
	}
	for name, req := range requests {
		t.Run(name, func(t *testing.T) {
			c := gitOpsFixture(t, fluxLabels).client()
			before, revsBefore := getRB(t, c), revisionVersions(t, c)

			result, err := NewService(c, testutil.TestLogger()).RollbackReleaseBinding(ctx, testNamespace, testRBName, req)

			require.ErrorIs(t, err, bindingrevision.ErrGitOpsManaged)
			assert.Nil(t, result)
			assert.Equal(t, wantGitOpsMessage, err.Error())
			requireUntouched(t, c, before, revsBefore)
		})
	}
}

func TestRollbackReleaseBinding_GitOpsTrackingMetadata(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name        string
		labels      map[string]string
		annotations map[string]string
		refused     bool
	}{
		{name: "no tracking metadata (ClickOps): rolls back"},
		{
			name:        "unrelated labels and annotations: rolls back",
			labels:      map[string]string{"app.kubernetes.io/managed-by": "Helm", "team": "payments"},
			annotations: map[string]string{"kustomize.toolkit.fluxcd.io/reconcile": "disabled", "note": "x"},
		},
		{name: "Flux Kustomization labels: refused", labels: fluxLabels, refused: true},
		{name: "only the Flux name label: refused", labels: map[string]string{"kustomize.toolkit.fluxcd.io/name": "apps"}, refused: true},
		{name: "empty Flux namespace label: refused", labels: map[string]string{"kustomize.toolkit.fluxcd.io/namespace": ""}, refused: true},
		{name: "Flux HelmRelease labels: refused", labels: map[string]string{"helm.toolkit.fluxcd.io/name": "platform"}, refused: true},
		{
			name:        "Argo CD tracking annotation: refused",
			annotations: map[string]string{"argocd.argoproj.io/tracking-id": "apps:openchoreo.dev/ReleaseBinding:default/x"},
			refused:     true,
		},
		{
			name:        "malformed Argo CD tracking annotation: refused",
			annotations: map[string]string{"argocd.argoproj.io/tracking-id": "???"},
			refused:     true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := gitOpsFixture(t, tt.labels)
			f.binding.Annotations = tt.annotations
			c := f.client()
			before, revsBefore := getRB(t, c), revisionVersions(t, c)

			result, err := NewService(c, testutil.TestLogger()).RollbackReleaseBinding(ctx, testNamespace, testRBName,
				bindingrevision.RollbackRequest{Revision: 1})

			if tt.refused {
				require.ErrorIs(t, err, bindingrevision.ErrGitOpsManaged)
				requireUntouched(t, c, before, revsBefore)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, int64(1), result.TargetRevision)
			assert.Equal(t, specFor(releaseA, ""), getRB(t, c).Spec, "r1's spec is restored")
			assert.Equal(t, revsBefore, revisionVersions(t, c), "the service never writes a revision")
		})
	}
}

func TestRollbackReleaseBinding_ClickOpsUnchanged(t *testing.T) {
	ctx := context.Background()

	t.Run("explicit: r1's release and overrides are restored", func(t *testing.T) {
		c := gitOpsFixture(t, map[string]string{"team": "payments"}).client()
		result, err := NewService(c, testutil.TestLogger()).RollbackReleaseBinding(ctx, testNamespace, testRBName,
			bindingrevision.RollbackRequest{Revision: 1})
		require.NoError(t, err)
		assert.Equal(t, testRBName+"-r1", result.RevisionName)
		assert.Equal(t, int64(3), *result.RevertedFrom)
		rb := getRB(t, c)
		assert.Equal(t, specFor(releaseA, ""), rb.Spec)
		assert.Equal(t, map[string]string{"team": "payments"}, rb.Labels, "labels are not touched")
	})

	t.Run("previous revision: dry run then apply", func(t *testing.T) {
		c := gitOpsFixture(t, nil).client()
		svc := NewService(c, testutil.TestLogger())
		preview, err := svc.RollbackReleaseBinding(ctx, testNamespace, testRBName, bindingrevision.RollbackRequest{DryRun: true})
		require.NoError(t, err)
		assert.Equal(t, int64(2), preview.TargetRevision)
		assert.Equal(t, releaseC, getRB(t, c).Spec.ReleaseName)

		applied, err := svc.RollbackReleaseBinding(ctx, testNamespace, testRBName, bindingrevision.RollbackRequest{})
		require.NoError(t, err)
		assert.Equal(t, int64(2), applied.TargetRevision)
		assert.Equal(t, specFor(releaseB, ""), getRB(t, c).Spec)
	})
}

// The GitOps check runs before the auto-deploy check: a GitOps-managed binding
// is refused for being managed from Git, which is what the user has to act on.
func TestRollbackReleaseBinding_GitOpsBeforeAutoDeploy(t *testing.T) {
	f := newFixture(t)
	f.binding.Labels = fluxLabels
	c := testutil.NewFakeClient(autoDeployObjects(f, true)...)

	_, err := NewService(c, testutil.TestLogger()).RollbackReleaseBinding(context.Background(), testNamespace, testRBName,
		bindingrevision.RollbackRequest{Revision: 1})

	require.ErrorIs(t, err, bindingrevision.ErrGitOpsManaged)
	assert.NotErrorIs(t, err, bindingrevision.ErrAutoDeployEnabled)
}

// When the binding cannot be read, its tracking metadata is unknown: the
// rollback fails and nothing is written.
func TestRollbackReleaseBinding_UnreadableBindingWritesNothing(t *testing.T) {
	patched := false
	c := fake.NewClientBuilder().WithScheme(testutil.NewScheme()).WithObjects(gitOpsFixture(t, nil).objects()...).
		WithInterceptorFuncs(interceptor.Funcs{
			Get: func(ctx context.Context, cl client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
				if _, ok := obj.(*openchoreov1alpha1.ReleaseBinding); ok {
					return errors.New("etcd is unavailable")
				}
				return cl.Get(ctx, key, obj, opts...)
			},
			Patch: func(ctx context.Context, cl client.WithWatch, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
				patched = true
				return cl.Patch(ctx, obj, patch, opts...)
			},
		}).Build()

	_, err := NewService(c, testutil.TestLogger()).RollbackReleaseBinding(context.Background(), testNamespace, testRBName,
		bindingrevision.RollbackRequest{Revision: 1})

	require.ErrorContains(t, err, "etcd is unavailable")
	assert.False(t, patched, "nothing is written")
}

// The authz wrapper, which the API server uses, cannot bypass the check.
func TestRollbackReleaseBinding_GitOpsManagedThroughAuthz(t *testing.T) {
	c := gitOpsFixture(t, fluxLabels).client()
	before, revsBefore := getRB(t, c), revisionVersions(t, c)
	svc := NewServiceWithAuthz(c, testutil.AllowPDP(), testutil.TestLogger())

	_, err := svc.RollbackReleaseBinding(testutil.AuthzContext(), testNamespace, testRBName,
		bindingrevision.RollbackRequest{Revision: 1})

	require.ErrorIs(t, err, bindingrevision.ErrGitOpsManaged)
	requireUntouched(t, c, before, revsBefore)

	t.Run("a caller without permission is still denied first", func(t *testing.T) {
		_, err := NewServiceWithAuthz(c, testutil.DenyPDP(), testutil.TestLogger()).RollbackReleaseBinding(
			testutil.AuthzContext(), testNamespace, testRBName, bindingrevision.RollbackRequest{Revision: 1})
		require.ErrorIs(t, err, services.ErrForbidden)
	})
}
