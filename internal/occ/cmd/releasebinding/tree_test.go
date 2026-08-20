// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package releasebinding

import (
	"bytes"
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/openchoreo/openchoreo/internal/occ/resources/client/mocks"
	"github.com/openchoreo/openchoreo/internal/occ/testutil"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
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

	depth, err := cmd.Flags().GetInt("depth")
	require.NoError(t, err)
	assert.Zero(t, depth)

	kind, err := cmd.Flags().GetString("kind")
	require.NoError(t, err)
	assert.Empty(t, kind)

	assert.NotNil(t, cmd.Flags().Lookup("namespace"))
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

func TestTree_RejectsNegativeDepth(t *testing.T) {
	err := New(nil).Tree(TreeParams{Namespace: "acme", ReleaseBindingName: "rb", Depth: -1})
	assert.ErrorContains(t, err, "--depth")
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
	err := New(mc).treeOnce(&buf, TreeParams{Namespace: "acme", ReleaseBindingName: "checkout-dev"}, renderOptions{}, false)
	require.NoError(t, err)
	out := buf.String()
	assert.Contains(t, out, "Release: checkout-dev-a1b2c3   (dataplane)")
	assert.Contains(t, out, "└─ Pod/checkout-abc")
	assert.NotContains(t, out, "Dig deeper:")
}

func TestTree_PropagatesFetchError(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetReleaseBindingResourceTree(mock.Anything, mock.Anything, mock.Anything).
		Return(nil, fmt.Errorf(`release binding "nope" not found`))

	var buf bytes.Buffer
	err := New(mc).treeOnce(&buf, TreeParams{Namespace: "acme", ReleaseBindingName: "nope"}, renderOptions{}, false)
	assert.EqualError(t, err, `release binding "nope" not found`)
}

func treeFixtureWithOwner() *gen.K8sResourceTreeResponse {
	resp := treeFixture()
	spec := &gen.RenderedReleaseSpec{EnvironmentName: "dev"}
	spec.Owner.ComponentName = "checkout"
	spec.Owner.ProjectName = "online-store"
	resp.RenderedReleases[0].RenderedRelease = &gen.RenderedRelease{Spec: spec}
	return resp
}

func TestPrintHints_WithOwnerAndForbidden(t *testing.T) {
	var buf bytes.Buffer
	printHints(&buf, TreeParams{Namespace: "acme", ReleaseBindingName: "checkout-dev"}, treeFixtureWithOwner())

	out := buf.String()
	assert.Contains(t, out, "Dig deeper:")
	assert.Contains(t, out, "occ component logs checkout --project online-store --env dev --namespace acme")
	assert.Contains(t, out, "occ releasebinding get checkout-dev --namespace acme")
	assert.Contains(t, out, "docs/resource-tree/child-discovery.md")
}

func TestPrintHints_NoOwnerOmitsLogsHint(t *testing.T) {
	resp := treeFixture()
	(*resp.RenderedReleases[0].Nodes[4].ChildrenStatus)[0].State = "error"

	var buf bytes.Buffer
	printHints(&buf, TreeParams{Namespace: "acme", ReleaseBindingName: "checkout-dev"}, resp)

	out := buf.String()
	assert.NotContains(t, out, "occ component logs")
	assert.Contains(t, out, "occ releasebinding get checkout-dev --namespace acme")
	assert.NotContains(t, out, "child-discovery.md", "no forbidden status, no RBAC hint")
}

func TestPrintHints_PartialOwnerOmitsLogsHint(t *testing.T) {
	resp := treeFixtureWithOwner()
	resp.RenderedReleases[0].RenderedRelease.Spec.Owner.ProjectName = ""

	var buf bytes.Buffer
	printHints(&buf, TreeParams{Namespace: "acme", ReleaseBindingName: "checkout-dev"}, resp)
	assert.NotContains(t, buf.String(), "occ component logs", "an empty project must not produce a broken command")
}

func TestTreeOnce_HintsEnabled(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetReleaseBindingResourceTree(mock.Anything, "acme", "checkout-dev").
		Return(treeFixtureWithOwner(), nil)

	var buf bytes.Buffer
	err := New(mc).treeOnce(&buf, TreeParams{Namespace: "acme", ReleaseBindingName: "checkout-dev"}, renderOptions{}, true)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "Dig deeper:")
}

func TestWatchLoop_FirstFetchErrorIsFatal(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetReleaseBindingResourceTree(mock.Anything, mock.Anything, mock.Anything).
		Return(nil, fmt.Errorf("gateway unavailable")).Once()

	var buf, errBuf bytes.Buffer
	err := New(mc).watchLoop(context.Background(), &buf, &errBuf,
		TreeParams{Namespace: "acme", ReleaseBindingName: "rb", Interval: time.Millisecond}, renderOptions{}, false)
	assert.EqualError(t, err, "gateway unavailable")
}

func TestWatchLoop_CancellationIsSuccess(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetReleaseBindingResourceTree(mock.Anything, mock.Anything, mock.Anything).
		Return(nil, context.Canceled).Maybe()

	var buf, errBuf bytes.Buffer
	err := New(mc).watchLoop(ctx, &buf, &errBuf,
		TreeParams{Namespace: "acme", ReleaseBindingName: "rb", Interval: time.Millisecond}, renderOptions{}, false)
	assert.NoError(t, err, "Ctrl-C during a fetch must exit 0, not report an error")
	assert.NotContains(t, errBuf.String(), "Stopped watching after", "Ctrl-C is not a timeout expiry")
}

func TestWatchLoop_TimeoutIsSuccess(t *testing.T) {
	const timeout = 20 * time.Millisecond
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetReleaseBindingResourceTree(mock.Anything, "acme", "rb").
		Return(treeFixture(), nil)

	var buf, errBuf bytes.Buffer
	start := time.Now()
	err := New(mc).watchLoop(ctx, &buf, &errBuf,
		TreeParams{Namespace: "acme", ReleaseBindingName: "rb", Interval: time.Millisecond, Timeout: timeout},
		renderOptions{}, false)
	require.NoError(t, err, "an expired --timeout must exit 0, like Ctrl-C")
	assert.Less(t, time.Since(start), time.Second, "the loop must stop at the deadline, not run on")
	out := errBuf.String()
	assert.Contains(t, out, "Stopped watching after")
	assert.Contains(t, out, "--timeout")
	assert.NotContains(t, buf.String(), "Stopped watching after", "the stop notice must not contaminate redirected tree output")
}

func TestWatchLoop_TransientErrorKeepsWatching(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	mc := mocks.NewMockInterface(t)
	calls := 0
	mc.EXPECT().GetReleaseBindingResourceTree(mock.Anything, "acme", "rb").
		RunAndReturn(func(context.Context, string, string) (*gen.K8sResourceTreeResponse, error) {
			calls++
			switch {
			case calls == 2:
				return nil, fmt.Errorf("blip")
			case calls >= 3:
				cancel()
			}
			return treeFixture(), nil
		})

	var buf, errBuf bytes.Buffer
	err := New(mc).watchLoop(ctx, &buf, &errBuf,
		TreeParams{Namespace: "acme", ReleaseBindingName: "rb", Interval: time.Millisecond}, renderOptions{}, false)
	require.NoError(t, err)
	assert.Contains(t, errBuf.String(), "fetch failed: blip")
	assert.NotContains(t, buf.String(), "fetch failed", "the retry line must not contaminate redirected tree output")
	assert.GreaterOrEqual(t, calls, 3, "the loop must survive a transient error")
}

func TestWatchLoop_WaitsIntervalAfterSlowFailure(t *testing.T) {
	const interval = 30 * time.Millisecond
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	mc := mocks.NewMockInterface(t)
	calls := 0
	var failedAt time.Time
	var gap time.Duration
	mc.EXPECT().GetReleaseBindingResourceTree(mock.Anything, "acme", "rb").
		RunAndReturn(func(context.Context, string, string) (*gen.K8sResourceTreeResponse, error) {
			calls++
			switch calls {
			case 1:
				return treeFixture(), nil
			case 2:
				time.Sleep(4 * interval)
				failedAt = time.Now()
				return nil, fmt.Errorf("slow blip")
			default:
				gap = time.Since(failedAt)
				cancel()
				return treeFixture(), nil
			}
		})

	var buf, errBuf bytes.Buffer
	err := New(mc).watchLoop(ctx, &buf, &errBuf,
		TreeParams{Namespace: "acme", ReleaseBindingName: "rb", Interval: interval}, renderOptions{}, false)
	require.NoError(t, err)
	require.GreaterOrEqual(t, calls, 3, "the loop must retry after the slow failure")
	assert.GreaterOrEqual(t, gap, interval,
		"the retry must wait a full interval measured from the end of the failed fetch, not from a free-running ticker")
}

func TestWatchDraw_RendersWithoutClearWhenNotTTY(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetReleaseBindingResourceTree(mock.Anything, "acme", "rb").
		Return(treeFixture(), nil)

	var buf bytes.Buffer
	err := New(mc).watchDraw(context.Background(), &buf,
		TreeParams{Namespace: "acme", ReleaseBindingName: "rb", Interval: 10 * time.Second}, renderOptions{}, false)
	require.NoError(t, err)
	out := buf.String()
	assert.NotContains(t, out, "\033[2J")
	assert.Contains(t, out, "Deployment/checkout")
	assert.NotContains(t, out, "Dig deeper:")
}

func TestWatchDraw_HeaderShowsTimeout(t *testing.T) {
	draw := func(t *testing.T, timeout time.Duration) string {
		t.Helper()
		mc := mocks.NewMockInterface(t)
		mc.EXPECT().GetReleaseBindingResourceTree(mock.Anything, "acme", "rb").
			Return(treeFixture(), nil)

		var buf bytes.Buffer
		err := New(mc).watchDraw(context.Background(), &buf,
			TreeParams{Namespace: "acme", ReleaseBindingName: "rb", Interval: 10 * time.Second, Timeout: timeout},
			renderOptions{}, false)
		require.NoError(t, err)
		return buf.String()
	}

	t.Run("with timeout", func(t *testing.T) {
		out := draw(t, 10*time.Minute)
		assert.Contains(t, out, "refreshes every 10s")
		assert.Contains(t, out, "stops after 10m0s")
		assert.Contains(t, out, "(Ctrl-C to stop)")
	})

	t.Run("without timeout", func(t *testing.T) {
		out := draw(t, 0)
		assert.Contains(t, out, "refreshes every 10s")
		assert.NotContains(t, out, "stops after")
		assert.Contains(t, out, "(Ctrl-C to stop)")
	})
}

func TestWatchDraw_ClearsScreenOnTTY(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetReleaseBindingResourceTree(mock.Anything, "acme", "rb").
		Return(treeFixture(), nil)

	var buf bytes.Buffer
	err := New(mc).watchDraw(context.Background(), &buf,
		TreeParams{Namespace: "acme", ReleaseBindingName: "rb", Interval: 10 * time.Second}, renderOptions{}, true)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "\033[2J")
}
