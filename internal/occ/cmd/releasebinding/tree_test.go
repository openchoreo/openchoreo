// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package releasebinding

import (
	"bytes"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/openchoreo/openchoreo/internal/occ/resources/client/mocks"
	"github.com/openchoreo/openchoreo/internal/occ/testutil"
)

func TestTreeCmd_FactoryError(t *testing.T) {
	cmd := newTreeCmd(errFactory("factory failed"))
	err := cmd.RunE(cmd, []string{"my-binding"})
	assert.EqualError(t, err, "factory failed")
}

func TestTreeCmd_ArgValidation(t *testing.T) {
	cmd := newTreeCmd(errFactory("unused"))
	assert.Error(t, cmd.Args(cmd, []string{}))
	assert.Error(t, cmd.Args(cmd, []string{"a", "b"}))
	assert.NoError(t, cmd.Args(cmd, []string{"a"}))
}

func TestTreeCmd_FlagDefaults(t *testing.T) {
	cmd := newTreeCmd(errFactory("unused"))

	watch, err := cmd.Flags().GetBool("watch")
	require.NoError(t, err)
	assert.False(t, watch)

	interval, err := cmd.Flags().GetDuration("interval")
	require.NoError(t, err)
	assert.Equal(t, 10*time.Second, interval)

	timeout, err := cmd.Flags().GetDuration("timeout")
	require.NoError(t, err)
	assert.Equal(t, 10*time.Minute, timeout)

	kind, err := cmd.Flags().GetString("kind")
	require.NoError(t, err)
	assert.Empty(t, kind)

	assert.NotNil(t, cmd.Flags().Lookup("namespace"))
	assert.Nil(t, cmd.Flags().Lookup("depth"), "--depth was removed; --kind is the way to narrow the tree")
}

func TestTreeCmd_FlagsReachRendering(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetReleaseBindingResourceTree(mock.Anything, "acme", "checkout-dev").
		Return(treeFixture(), nil)

	cmd := newTreeCmd(mockFactory(mc))
	require.NoError(t, cmd.Flags().Set("namespace", "acme"))
	require.NoError(t, cmd.Flags().Set("kind", "Pod"))

	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, cmd.RunE(cmd, []string{"checkout-dev"}))
	})
	assert.Contains(t, out, "Pod/checkout-abc")
	assert.NotContains(t, out, "Secret/checkout-tls", "--kind must actually filter the output")
}

func TestTree_RequiresNamespace(t *testing.T) {
	err := New(nil).Tree(TreeParams{ReleaseBindingName: "rb"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "namespace")
}

func TestTree_RejectsNonPositiveIntervalWithWatch(t *testing.T) {
	err := New(nil).Tree(TreeParams{Namespace: "acme", ReleaseBindingName: "rb", Watch: true, Interval: 0})
	assert.ErrorContains(t, err, "--interval")
}

func TestTree_RejectsNegativeTimeout(t *testing.T) {
	err := New(nil).Tree(TreeParams{Namespace: "acme", ReleaseBindingName: "rb", Timeout: -time.Second})
	assert.ErrorContains(t, err, "--timeout")
}

func TestTree_RendersFetchedTree(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetReleaseBindingResourceTree(mock.Anything, "acme", "checkout-dev").
		Return(treeFixture(), nil)

	var buf bytes.Buffer
	err := New(mc).treeOnce(&buf, TreeParams{Namespace: "acme", ReleaseBindingName: "checkout-dev"}, renderOptions{})
	require.NoError(t, err)
	out := buf.String()
	assert.Contains(t, out, "ReleaseBinding/checkout-dev")
	assert.Contains(t, out, "RenderedRelease/checkout-dev-a1b2c3  (dataplane)")
	assert.Contains(t, out, "├─ Pod/checkout-abc")
}

func TestTree_PropagatesFetchError(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetReleaseBindingResourceTree(mock.Anything, mock.Anything, mock.Anything).
		Return(nil, fmt.Errorf(`release binding "nope" not found`))

	var buf bytes.Buffer
	err := New(mc).treeOnce(&buf, TreeParams{Namespace: "acme", ReleaseBindingName: "nope"}, renderOptions{})
	assert.EqualError(t, err, `release binding "nope" not found`)
}
