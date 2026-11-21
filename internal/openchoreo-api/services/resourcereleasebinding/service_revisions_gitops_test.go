// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package resourcereleasebinding

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/bindingrevision"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/testutil"
	"github.com/openchoreo/openchoreo/internal/revision"
)

const wantRRBGitOpsMessage = `rollback blocked: the binding is managed by GitOps: resource release binding "` +
	testBindingName + `" is applied from Git by Flux Kustomization "flux-system/apps". Rollback through OpenChoreo ` +
	`revision history is not supported for GitOps-managed bindings, because GitOps would restore the configuration ` +
	`in Git on its next sync. Revert the binding's configuration in Git and let GitOps reconcile the change`

// rrbHistoryObjects is rrbObjects with activation history (r1, then r2), so a
// rollback without a revision targets r1, on a binding carrying the given metadata.
func rrbHistoryObjects(t *testing.T, labels, annotations map[string]string) []client.Object {
	t.Helper()
	objs := rrbObjects(t, true)
	binding := objs[0].(*openchoreov1alpha1.ResourceReleaseBinding)
	binding.Status.HighestRevision = 2
	binding.Labels, binding.Annotations = labels, annotations
	objs[1].SetAnnotations(map[string]string{revision.AnnotationKeyBindingGeneration: "1", revision.AnnotationKeyPreviousRevision: "0"})
	objs[2].SetAnnotations(map[string]string{revision.AnnotationKeyBindingGeneration: "2", revision.AnnotationKeyPreviousRevision: "1"})
	return objs
}

func rrbRevisionVersions(t *testing.T, c client.Client) map[string]string {
	t.Helper()
	var list openchoreov1alpha1.ResourceReleaseBindingRevisionList
	require.NoError(t, c.List(context.Background(), &list, client.InNamespace(testNamespace)))
	out := map[string]string{}
	for _, r := range list.Items {
		out[r.Name] = r.ResourceVersion
	}
	return out
}

func TestRollbackResourceReleaseBinding_GitOpsManagedIsRefused(t *testing.T) {
	ctx := context.Background()
	flux := map[string]string{
		"kustomize.toolkit.fluxcd.io/name":      "apps",
		"kustomize.toolkit.fluxcd.io/namespace": "flux-system",
	}
	requests := map[string]bindingrevision.RollbackRequest{
		"explicit revision":          {Revision: 1, Message: "back to the smaller instance"},
		"explicit revision, dry run": {Revision: 1, DryRun: true},
		"previous revision":          {},
		"previous revision, dry run": {DryRun: true},
	}
	for name, req := range requests {
		t.Run(name, func(t *testing.T) {
			c := testutil.NewFakeClient(rrbHistoryObjects(t, flux, nil)...)
			before, revsBefore := getRRB(t, c), rrbRevisionVersions(t, c)

			result, err := NewService(c, testutil.TestLogger()).RollbackResourceReleaseBinding(ctx, testNamespace,
				testBindingName, req)

			require.ErrorIs(t, err, bindingrevision.ErrGitOpsManaged)
			assert.Nil(t, result)
			assert.Equal(t, wantRRBGitOpsMessage, err.Error())
			after := getRRB(t, c)
			assert.Equal(t, before.ResourceVersion, after.ResourceVersion, "the binding is not written")
			assert.Equal(t, rrReleaseB, after.Spec.ResourceRelease)
			assert.Equal(t, before.Annotations, after.Annotations)
			assert.Equal(t, revsBefore, rrbRevisionVersions(t, c), "no revision is written")
		})
	}

	t.Run("through the authz wrapper", func(t *testing.T) {
		c := testutil.NewFakeClient(rrbHistoryObjects(t, flux, nil)...)
		_, err := NewServiceWithAuthz(c, testutil.AllowPDP(), testutil.TestLogger()).RollbackResourceReleaseBinding(
			testutil.AuthzContext(), testNamespace, testBindingName, bindingrevision.RollbackRequest{Revision: 1})
		require.ErrorIs(t, err, bindingrevision.ErrGitOpsManaged)
		assert.Equal(t, rrReleaseB, getRRB(t, c).Spec.ResourceRelease)
	})
}

func TestRollbackResourceReleaseBinding_GitOpsTrackingMetadata(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name        string
		labels      map[string]string
		annotations map[string]string
		refused     bool
	}{
		{name: "no tracking metadata (ClickOps): rolls back"},
		{name: "unrelated metadata: rolls back", labels: map[string]string{"team": "data"},
			annotations: map[string]string{"kustomize.toolkit.fluxcd.io/reconcile": "disabled"}},
		{name: "only the Flux namespace label: refused",
			labels: map[string]string{"kustomize.toolkit.fluxcd.io/namespace": "flux-system"}, refused: true},
		{name: "Flux HelmRelease label: refused",
			labels: map[string]string{"helm.toolkit.fluxcd.io/namespace": "flux-system"}, refused: true},
		{name: "Argo CD tracking annotation: refused",
			annotations: map[string]string{"argocd.argoproj.io/tracking-id": "data:openchoreo.dev/ResourceReleaseBinding:default/x"},
			refused:     true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := testutil.NewFakeClient(rrbHistoryObjects(t, tt.labels, tt.annotations)...)
			before := getRRB(t, c)

			for _, req := range []bindingrevision.RollbackRequest{{Revision: 1, DryRun: true}, {}} {
				result, err := NewService(c, testutil.TestLogger()).RollbackResourceReleaseBinding(ctx, testNamespace,
					testBindingName, req)
				if tt.refused {
					require.ErrorIs(t, err, bindingrevision.ErrGitOpsManaged)
					continue
				}
				require.NoError(t, err)
				assert.Equal(t, int64(1), result.TargetRevision)
			}

			after := getRRB(t, c)
			if tt.refused {
				assert.Equal(t, before.ResourceVersion, after.ResourceVersion, "the binding is not written")
				return
			}
			assert.Equal(t, rrReleaseA, after.Spec.ResourceRelease, "the rollback without a revision restored r1")
		})
	}
}
