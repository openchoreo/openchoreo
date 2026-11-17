// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package releasebinding

import (
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/openchoreo/openchoreo/internal/occ/resources/client/mocks"
	"github.com/openchoreo/openchoreo/internal/occ/testutil"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
)

func TestRollback_MessageIsSentWithTheAppliedRollback(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().RollbackReleaseBinding(mock.Anything, rollbackNS, rollbackBinding,
		mock.MatchedBy(func(r gen.RollbackRequest) bool { return *r.DryRun && r.Message == nil })).
		Return(rollbackResp(true), nil).Once()
	mc.EXPECT().RollbackReleaseBinding(mock.Anything, rollbackNS, rollbackBinding,
		mock.MatchedBy(func(r gen.RollbackRequest) bool {
			return !*r.DryRun && r.Message != nil && *r.Message == "Payments failing since r3"
		})).Return(rollbackResp(false), nil).Once()

	r := New(mc)
	r.confirm = func(io.Writer, string) (bool, error) { return true, nil }
	testutil.CaptureStdout(t, func() {
		require.NoError(t, r.Rollback(RollbackParams{Namespace: rollbackNS, ReleaseBindingName: rollbackBinding,
			Revision: 1, Message: "Payments failing since r3"}))
	})
}

func TestRollback_MessageTooLongIsRefusedBeforeAnyCall(t *testing.T) {
	mc := mocks.NewMockInterface(t) // any call fails the test
	err := New(mc).Rollback(RollbackParams{Namespace: rollbackNS, ReleaseBindingName: rollbackBinding, Revision: 1,
		Message: strings.Repeat("x", 1025)})
	assert.EqualError(t, err, "--message must be at most 1024 characters (got 1025)")
}
