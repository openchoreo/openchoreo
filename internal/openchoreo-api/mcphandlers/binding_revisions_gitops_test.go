// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package mcphandlers

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/bindingrevision"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/handlerservices"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/releasebinding"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/resourcereleasebinding"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/testutil"
)

// The MCP rollback tools call the same services as the REST API, so an agent
// cannot roll back a GitOps-managed binding either, dry runs included.
func TestRollback_GitOpsManagedThroughMCP(t *testing.T) {
	ctx := context.Background()
	flux := map[string]string{
		"kustomize.toolkit.fluxcd.io/name":      "apps",
		"kustomize.toolkit.fluxcd.io/namespace": "flux-system",
	}
	rb := testutil.NewReleaseBinding(testNS, "shop", "api", "development", "api-development")
	rb.Labels = flux
	rrb := testutil.NewResourceReleaseBinding(testNS, "shop", "db", "development", "db-development")
	rrb.Labels = flux
	k8s := testutil.NewFakeClient(rb, rrb)
	h := NewMCPHandler(&handlerservices.Services{
		ReleaseBindingService:         releasebinding.NewService(k8s, testutil.TestLogger()),
		ResourceReleaseBindingService: resourcereleasebinding.NewService(k8s, testutil.TestLogger()),
	})

	for _, dryRun := range []bool{true, false} {
		_, err := h.RollbackReleaseBinding(ctx, testNS, "api-development", 1, dryRun, "")
		require.ErrorIs(t, err, bindingrevision.ErrGitOpsManaged, "dryRun=%t", dryRun)
		assert.ErrorContains(t, err, "Revert the binding's configuration in Git and let GitOps reconcile the change")

		_, err = h.RollbackResourceReleaseBinding(ctx, testNS, "db-development", 1, dryRun, "")
		require.ErrorIs(t, err, bindingrevision.ErrGitOpsManaged, "dryRun=%t", dryRun)
	}

	stored := &openchoreov1alpha1.ReleaseBinding{}
	require.NoError(t, k8s.Get(ctx, client.ObjectKeyFromObject(rb), stored))
	assert.Equal(t, rb.Spec, stored.Spec, "the binding is not written")
}
