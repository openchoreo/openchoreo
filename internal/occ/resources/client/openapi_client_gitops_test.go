// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package client

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/utils/ptr"
	k8sclient "sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/handlers"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/handlerservices"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/releasebinding"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/resourcereleasebinding"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/testutil"
)

// TestRollback_GitOpsManagedOverHTTP runs the occ client against the real
// openchoreo-api handlers and services over HTTP: a rollback of a binding Flux
// applies is refused, and occ returns the server's message as the error.
func TestRollback_GitOpsManagedOverHTTP(t *testing.T) {
	flux := map[string]string{
		"kustomize.toolkit.fluxcd.io/name":      "apps",
		"kustomize.toolkit.fluxcd.io/namespace": "flux-system",
	}
	rb := testutil.NewReleaseBinding("default", "shop", "api", "development", "api-development")
	rb.Labels = flux
	rrb := testutil.NewResourceReleaseBinding("default", "shop", "db", "development", "db-development")
	rrb.Labels = flux
	k8s := testutil.NewFakeClient(rb, rrb)

	h := handlers.New(&handlerservices.Services{
		ReleaseBindingService:         releasebinding.NewService(k8s, testutil.TestLogger()),
		ResourceReleaseBindingService: resourcereleasebinding.NewService(k8s, testutil.TestLogger()),
	}, testutil.TestLogger(), nil)
	server := httptest.NewServer(gen.Handler(gen.NewStrictHandler(h, nil)))
	t.Cleanup(server.Close)
	genClient, err := gen.NewClientWithResponses(server.URL)
	require.NoError(t, err)
	c := &Client{client: genClient}

	const advice = `Rollback through OpenChoreo revision history is not supported for GitOps-managed bindings, ` +
		`because GitOps would restore the configuration in Git on its next sync. Revert the binding's ` +
		`configuration in Git and let GitOps reconcile the change`

	_, err = c.RollbackReleaseBinding(context.Background(), "default", "api-development",
		gen.RollbackRequest{Revision: ptr.To(int64(1))})
	require.Error(t, err)
	assert.Equal(t, `Rollback blocked: the binding is managed by GitOps: release binding "api-development" is applied `+
		`from Git by Flux Kustomization "flux-system/apps". `+advice, err.Error())

	_, err = c.RollbackResourceReleaseBinding(context.Background(), "default", "db-development",
		gen.RollbackRequest{DryRun: ptr.To(true)})
	require.Error(t, err)
	assert.Equal(t, `Rollback blocked: the binding is managed by GitOps: resource release binding "db-development" is `+
		`applied from Git by Flux Kustomization "flux-system/apps". `+advice, err.Error())

	stored := &openchoreov1alpha1.ReleaseBinding{}
	require.NoError(t, k8s.Get(context.Background(), k8sclient.ObjectKeyFromObject(rb), stored))
	assert.Equal(t, rb.Spec, stored.Spec, "the binding is not written")
}
