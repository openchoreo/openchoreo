// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package releasebinding

import (
	"errors"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/openchoreo/openchoreo/internal/occ/resources/client/mocks"
	"github.com/openchoreo/openchoreo/internal/occ/testutil"
)

// errGitOpsRefusal is the error the occ client returns for the server's 409
// GITOPS_MANAGED response (see TestRollback_GitOpsManagedOverHTTP).
var errGitOpsRefusal = errors.New(`Rollback blocked: the binding is managed by GitOps: release binding "api-dev" is ` +
	`applied from Git by Flux Kustomization "flux-system/apps". Rollback through OpenChoreo revision history is not ` +
	`supported for GitOps-managed bindings, because GitOps would restore the configuration in Git on its next sync. ` +
	`Revert the binding's configuration in Git and let GitOps reconcile the change`)

func TestRollback_GitOpsManaged(t *testing.T) {
	tests := []struct {
		name   string
		params RollbackParams
	}{
		{name: "preview first: refused before any prompt", params: RollbackParams{Revision: 1}},
		{name: "--yes: refused", params: RollbackParams{Revision: 1, Yes: true}},
		{name: "--dry-run: refused", params: RollbackParams{Revision: 1, DryRun: true}},
		{name: "previous revision: refused", params: RollbackParams{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mc := mocks.NewMockInterface(t)
			// One request only: a refused preview is never followed by an apply.
			mc.EXPECT().RollbackReleaseBinding(mock.Anything, rollbackNS, rollbackBinding, mock.Anything).
				Return(nil, errGitOpsRefusal).Once()
			r := New(mc)
			r.confirm = func(io.Writer, string) (bool, error) { t.Fatal("must not prompt"); return false, nil }

			params := tt.params
			params.Namespace, params.ReleaseBindingName = rollbackNS, rollbackBinding
			var err error
			out := testutil.CaptureStdout(t, func() { err = r.Rollback(params) })

			require.Error(t, err, "the command fails, so occ exits non-zero")
			assert.Equal(t, errGitOpsRefusal.Error(), err.Error(), "the server's message is shown unchanged")
			assert.NotContains(t, out, "Rolled back")
			assert.NotContains(t, out, "is active again")
		})
	}
}
