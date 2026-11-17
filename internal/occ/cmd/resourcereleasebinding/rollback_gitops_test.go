// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package resourcereleasebinding

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

func TestRollback_GitOpsManaged(t *testing.T) {
	const ns, binding = "ns", "db-dev"
	refusal := errors.New(`Rollback blocked: the binding is managed by GitOps: resource release binding "db-dev" is ` +
		`applied from Git by Flux Kustomization "flux-system/apps". Rollback through OpenChoreo revision history is ` +
		`not supported for GitOps-managed bindings, because GitOps would restore the configuration in Git on its next ` +
		`sync. Revert the binding's configuration in Git and let GitOps reconcile the change`)

	for name, params := range map[string]RollbackParams{
		"preview first":     {Revision: 1},
		"--yes":             {Revision: 1, Yes: true},
		"previous revision": {},
	} {
		t.Run(name, func(t *testing.T) {
			mc := mocks.NewMockInterface(t)
			mc.EXPECT().RollbackResourceReleaseBinding(mock.Anything, ns, binding, mock.Anything).Return(nil, refusal).Once()
			r := New(mc)
			r.confirm = func(io.Writer, string) (bool, error) { t.Fatal("must not prompt"); return false, nil }

			params.Namespace, params.ResourceReleaseBindingName = ns, binding
			var err error
			out := testutil.CaptureStdout(t, func() { err = r.Rollback(params) })

			require.Error(t, err)
			assert.Equal(t, refusal.Error(), err.Error())
			assert.NotContains(t, out, "Rolled back")
		})
	}
}
