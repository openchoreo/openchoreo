// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/handlerservices"
	releasebindingsvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/releasebinding"
	resourcereleasebindingsvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/resourcereleasebinding"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/testutil"
	"github.com/openchoreo/openchoreo/internal/revision"
)

const (
	gitOpsNS        = "default"
	gitOpsProject   = "shop"
	gitOpsComponent = "api"
	gitOpsResource  = "db"
	gitOpsEnv       = "development"
	gitOpsRB        = "api-development"
	gitOpsRRB       = "db-development"
)

var gitOpsFluxLabels = map[string]string{
	"kustomize.toolkit.fluxcd.io/name":      "apps",
	"kustomize.toolkit.fluxcd.io/namespace": "flux-system",
}

func ownedBy(kind, name string, uid types.UID, n int64, l map[string]string) metav1.ObjectMeta {
	l[revision.LabelKeyBindingUID] = string(uid)
	return metav1.ObjectMeta{
		Name: revision.Name(name, n), Namespace: gitOpsNS, Labels: l,
		OwnerReferences: []metav1.OwnerReference{{
			APIVersion: openchoreov1alpha1.GroupVersion.String(), Kind: kind, Name: name, UID: uid, Controller: ptr.To(true),
		}},
	}
}

// gitOpsObjects is a ReleaseBinding and a ResourceReleaseBinding, each at r2 with
// r1 before it, both carrying bindingLabels.
func gitOpsObjects(t *testing.T, bindingLabels map[string]string) []client.Object {
	t.Helper()
	rbSpec := func(release string) openchoreov1alpha1.ReleaseBindingSpec {
		return openchoreov1alpha1.ReleaseBindingSpec{
			Owner:       openchoreov1alpha1.ReleaseBindingOwner{ProjectName: gitOpsProject, ComponentName: gitOpsComponent},
			Environment: gitOpsEnv, ReleaseName: release, State: openchoreov1alpha1.ReleaseStateActive,
		}
	}
	rb := testutil.NewReleaseBinding(gitOpsNS, gitOpsProject, gitOpsComponent, gitOpsEnv, gitOpsRB)
	rb.UID, rb.Labels, rb.Spec = "uid-rb", bindingLabels, rbSpec("api-b")
	rbRevs := make([]client.Object, 0, 2)
	for n, release := range map[int64]string{1: "api-a", 2: "api-b"} {
		spec := rbSpec(release)
		hash, err := revision.ReleaseBindingSpecHash(&spec)
		require.NoError(t, err)
		rbRevs = append(rbRevs, &openchoreov1alpha1.ReleaseBindingRevision{
			ObjectMeta: ownedBy("ReleaseBinding", gitOpsRB, rb.UID, n, map[string]string{}),
			Spec: openchoreov1alpha1.ReleaseBindingRevisionSpec{Revision: n, Cause: openchoreov1alpha1.RevisionCauseDeploy,
				ComponentReleaseName: release, SnapshotHash: hash, BindingSpec: spec},
		})
	}

	rrbSpec := func(release string) openchoreov1alpha1.ResourceReleaseBindingSpec {
		return openchoreov1alpha1.ResourceReleaseBindingSpec{
			Owner:       openchoreov1alpha1.ResourceReleaseBindingOwner{ProjectName: gitOpsProject, ResourceName: gitOpsResource},
			Environment: gitOpsEnv, ResourceRelease: release,
		}
	}
	rrb := testutil.NewResourceReleaseBinding(gitOpsNS, gitOpsProject, gitOpsResource, gitOpsEnv, gitOpsRRB)
	rrb.UID, rrb.Labels, rrb.Spec = "uid-rrb", bindingLabels, rrbSpec("db-b")
	rrbRevs := make([]client.Object, 0, 2)
	for n, release := range map[int64]string{1: "db-a", 2: "db-b"} {
		spec := rrbSpec(release)
		hash, err := revision.ResourceReleaseBindingSpecHash(&spec)
		require.NoError(t, err)
		rrbRevs = append(rrbRevs, &openchoreov1alpha1.ResourceReleaseBindingRevision{
			ObjectMeta: ownedBy("ResourceReleaseBinding", gitOpsRRB, rrb.UID, n, map[string]string{}),
			Spec: openchoreov1alpha1.ResourceReleaseBindingRevisionSpec{Revision: n, Cause: openchoreov1alpha1.RevisionCauseDeploy,
				ResourceReleaseName: release, SnapshotHash: hash, BindingSpec: spec},
		})
	}

	objs := make([]client.Object, 0, 8+len(rbRevs)+len(rrbRevs))
	objs = append(objs,
		rb, rrb,
		testutil.NewComponent(gitOpsNS, gitOpsProject, gitOpsComponent),
		testutil.NewComponentRelease(gitOpsNS, gitOpsProject, gitOpsComponent, "api-a"),
		testutil.NewComponentRelease(gitOpsNS, gitOpsProject, gitOpsComponent, "api-b"),
		testutil.NewResource(gitOpsNS, gitOpsProject, gitOpsResource),
		testutil.NewResourceRelease(gitOpsNS, gitOpsProject, gitOpsResource, "db-a"),
		testutil.NewResourceRelease(gitOpsNS, gitOpsProject, gitOpsResource, "db-b"),
	)
	objs = append(objs, rbRevs...)
	return append(objs, rrbRevs...)
}

func gitOpsHTTPHandler(t *testing.T, c client.Client) http.Handler {
	t.Helper()
	return newTestHTTPHandler(t, &handlerservices.Services{
		ReleaseBindingService:         releasebindingsvc.NewService(c, testutil.TestLogger()),
		ResourceReleaseBindingService: resourcereleasebindingsvc.NewService(c, testutil.TestLogger()),
	})
}

// TestRollbackHTTP_GitOpsManaged sends rollbacks through the real router and
// middleware to the real services: GitOps-managed bindings get a 409
// GITOPS_MANAGED and are not written; other bindings roll back as before.
func TestRollbackHTTP_GitOpsManaged(t *testing.T) {
	kinds := []struct {
		kind, path, name string
		get              func(c client.Client) (string, string) // resourceVersion, release
		wantMessage      string
	}{
		{
			kind: "ReleaseBinding", name: gitOpsRB,
			path: "/api/v1/namespaces/" + gitOpsNS + "/releasebindings/" + gitOpsRB + "/rollback",
			get: func(c client.Client) (string, string) {
				b := &openchoreov1alpha1.ReleaseBinding{}
				require.NoError(t, c.Get(context.Background(), client.ObjectKey{Namespace: gitOpsNS, Name: gitOpsRB}, b))
				return b.ResourceVersion, b.Spec.ReleaseName
			},
			wantMessage: `Rollback blocked: the binding is managed by GitOps: release binding "api-development" is ` +
				`applied from Git by Flux Kustomization "flux-system/apps". Rollback through OpenChoreo revision ` +
				`history is not supported for GitOps-managed bindings, because GitOps would restore the configuration ` +
				`in Git on its next sync. Revert the binding's configuration in Git and let GitOps reconcile the change`,
		},
		{
			kind: "ResourceReleaseBinding", name: gitOpsRRB,
			path: "/api/v1/namespaces/" + gitOpsNS + "/resourcereleasebindings/" + gitOpsRRB + "/rollback",
			get: func(c client.Client) (string, string) {
				b := &openchoreov1alpha1.ResourceReleaseBinding{}
				require.NoError(t, c.Get(context.Background(), client.ObjectKey{Namespace: gitOpsNS, Name: gitOpsRRB}, b))
				return b.ResourceVersion, b.Spec.ResourceRelease
			},
			wantMessage: `Rollback blocked: the binding is managed by GitOps: resource release binding "db-development" ` +
				`is applied from Git by Flux Kustomization "flux-system/apps". Rollback through OpenChoreo revision ` +
				`history is not supported for GitOps-managed bindings, because GitOps would restore the configuration ` +
				`in Git on its next sync. Revert the binding's configuration in Git and let GitOps reconcile the change`,
		},
	}
	bodies := map[string]string{
		"explicit":         `{"revision":1}`,
		"explicit dry run": `{"revision":1,"dryRun":true}`,
		"previous":         `{}`,
	}

	for _, k := range kinds {
		t.Run(k.kind+" managed by Flux: 409 GITOPS_MANAGED, nothing written", func(t *testing.T) {
			requireRouteInSpec(t, http.MethodPost, k.path)
			c := testutil.NewFakeClient(gitOpsObjects(t, gitOpsFluxLabels)...)
			h := gitOpsHTTPHandler(t, c)
			rvBefore, releaseBefore := k.get(c)

			for name, body := range bodies {
				req, rec := doRequest(t, h, http.MethodPost, k.path, []byte(body))
				require.Equal(t, http.StatusConflict, rec.Code, "%s: %s", name, rec.Body.String())
				assertConformsToSpec(t, req, rec.Code, rec.Header(), rec.Body.Bytes())

				var resp gen.ErrorResponse
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
				assert.Equal(t, gen.GITOPSMANAGED, resp.Code, name)
				assert.Equal(t, k.wantMessage, resp.Error, name)
			}

			rvAfter, releaseAfter := k.get(c)
			assert.Equal(t, rvBefore, rvAfter, "the binding is not written")
			assert.Equal(t, releaseBefore, releaseAfter)
		})

		t.Run(k.kind+" not managed by GitOps: rolls back as before", func(t *testing.T) {
			c := testutil.NewFakeClient(gitOpsObjects(t, map[string]string{"team": "payments"})...)
			h := gitOpsHTTPHandler(t, c)

			req, rec := doRequest(t, h, http.MethodPost, k.path, []byte(`{"revision":1}`))
			require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
			assertConformsToSpec(t, req, rec.Code, rec.Header(), rec.Body.Bytes())

			var resp gen.RollbackResponse
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
			assert.Equal(t, int64(1), resp.TargetRevision)
			assert.Equal(t, revision.Name(k.name, 1), resp.RevisionName)
			_, release := k.get(c)
			assert.Contains(t, []string{"api-a", "db-a"}, release, "r1's release is restored")
		})
	}
}
