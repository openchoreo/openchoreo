// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package releasebinding

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/openchoreo/openchoreo/internal/occ/resources/client"
	"github.com/openchoreo/openchoreo/internal/occ/resources/client/mocks"
	"github.com/openchoreo/openchoreo/internal/occ/testutil"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
)

var testClock = time.Date(2026, 9, 17, 12, 3, 4, 0, time.UTC)

func newTestWatcher(c client.Interface, w, errW io.Writer, params TreeParams, scr *termScreen) *treeWatcher {
	if params.Interval == 0 {
		params.Interval = time.Millisecond
	}
	return &treeWatcher{client: c, w: w, errW: errW, params: params, scr: scr, now: func() time.Time { return testClock }}
}

// fakeScreen is a terminal of the given height, wide enough that no fixture
// line is clipped, with stderr on the same terminal as on a real one.
func fakeScreen(w io.Writer, rows int) *termScreen {
	return &termScreen{w: w, size: func() (int, int, bool) { return 120, rows, rows > 0 }, sharedErr: true}
}

// countingWriter records how many Write calls reached the terminal. It does
// not embed bytes.Buffer, whose WriteString would bypass the count.
type countingWriter struct {
	buf    bytes.Buffer
	writes int
}

func (c *countingWriter) Write(p []byte) (int, error) {
	c.writes++
	return c.buf.Write(p)
}

// secondFixture is distinguishable from treeFixture so a test can tell which
// frame was kept.
func secondFixture() *gen.K8sResourceTreeResponse {
	resp := treeFixture()
	resp.RenderedReleases[0].Nodes[0].Name = "checkout-v2"
	return resp
}

func TestWatch_FirstFetchErrorIsFatal(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetReleaseBindingResourceTree(mock.Anything, mock.Anything, mock.Anything).
		Return(nil, fmt.Errorf("gateway unavailable")).Once()

	var buf bytes.Buffer
	tw := newTestWatcher(mc, &buf, &buf, TreeParams{Namespace: "acme", ReleaseBindingName: "rb"}, fakeScreen(&buf, 40))
	err := tw.run(context.Background())
	assert.EqualError(t, err, "gateway unavailable")
	assert.NotContains(t, buf.String(), "\033[?1049", "a watch that never drew must not touch the alternate screen")
}

func TestWatch_CancellationIsSuccess(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetReleaseBindingResourceTree(mock.Anything, mock.Anything, mock.Anything).
		Return(nil, context.Canceled).Maybe()

	var buf bytes.Buffer
	tw := newTestWatcher(mc, &buf, &buf, TreeParams{Namespace: "acme", ReleaseBindingName: "rb"}, fakeScreen(&buf, 40))
	err := tw.run(ctx)
	assert.NoError(t, err, "Ctrl-C during a fetch must exit 0, not report an error")
	assert.Empty(t, buf.String(), "Ctrl-C before the first frame prints nothing: no escapes, no reprint, no timeout notice")
}

func TestWatch_TimeoutIsSuccess(t *testing.T) {
	const timeout = 20 * time.Millisecond
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetReleaseBindingResourceTree(mock.Anything, "acme", "rb").
		Return(treeFixture(), nil)

	var buf, errBuf bytes.Buffer
	start := time.Now()
	tw := newTestWatcher(mc, &buf, &errBuf, TreeParams{Namespace: "acme", ReleaseBindingName: "rb", Timeout: timeout}, nil)
	err := tw.run(ctx)
	require.NoError(t, err, "an expired --timeout must exit 0, like Ctrl-C")
	assert.Less(t, time.Since(start), time.Second, "the loop must stop at the deadline, not run on")
	out := errBuf.String()
	assert.Contains(t, out, "\nStopped watching after", "the notice is separated from the last tree row by a blank line")
	assert.Contains(t, out, "--timeout")
	assert.NotContains(t, buf.String(), "Stopped watching after", "the stop notice must not contaminate redirected tree output")
	assert.NotContains(t, buf.String(), "\033[", "redirected output carries no escape sequences")
}

func TestWatch_TerminalLeavesOneTreeBehind(t *testing.T) {
	const timeout = 20 * time.Millisecond
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetReleaseBindingResourceTree(mock.Anything, "acme", "rb").
		Return(treeFixture(), nil)

	// One buffer for stdout and stderr, as on a real terminal, so ordering is observable.
	var buf bytes.Buffer
	tw := newTestWatcher(mc, &buf, &buf, TreeParams{Namespace: "acme", ReleaseBindingName: "rb", Timeout: timeout}, fakeScreen(&buf, 40))
	require.NoError(t, tw.run(ctx))
	out := buf.String()

	enter := strings.Index(out, ansiEnterAltScreen)
	leave := strings.Index(out, ansiLeaveAltScreen)
	require.NotEqual(t, -1, enter)
	require.NotEqual(t, -1, leave)
	assert.Equal(t, 1, strings.Count(out, ansiEnterAltScreen), "the alternate screen is entered once")
	assert.Equal(t, 1, strings.Count(out, ansiLeaveAltScreen), "and left once")
	assert.Less(t, enter, strings.Index(out, "Watching acme/rb"), "entered before the first frame")
	assert.Greater(t, strings.Count(out[enter:leave], ansiClearScreen), 1, "each refresh clears and repaints the alternate screen")

	after := out[leave+len(ansiLeaveAltScreen):]
	assert.Equal(t, 1, strings.Count(after, "Deployment/checkout"), "exactly one tree is left on the normal screen")
	assert.NotContains(t, after, "Ctrl-C to stop", "the live header does not belong on the reprint")
	assert.Contains(t, after, "Watched acme/rb — last refreshed 12:03:04")
	assert.Less(t, strings.Index(after, "Deployment/checkout"), strings.Index(after, "Stopped watching after"),
		"the reprinted tree precedes the stop notice")
	assert.NotContains(t, after, "\033[?1049", "the alternate screen is not re-entered")
	assert.NotContains(t, after, ansiClearScreen, "nothing after leaving the alternate screen clears the terminal")
}

func TestWatch_ColorReplayKeepsColorsOnly(t *testing.T) {
	const timeout = 20 * time.Millisecond
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetReleaseBindingResourceTree(mock.Anything, "acme", "rb").
		Return(treeFixture(), nil)

	var buf bytes.Buffer
	tw := newTestWatcher(mc, &buf, &buf, TreeParams{Namespace: "acme", ReleaseBindingName: "rb", Timeout: timeout}, fakeScreen(&buf, 40))
	tw.opts.color = true
	require.NoError(t, tw.run(ctx))
	out := buf.String()
	after := out[strings.Index(out, ansiLeaveAltScreen)+len(ansiLeaveAltScreen):]
	assert.Contains(t, after, ansiGreen+"● Healthy", "the reprinted tree keeps its colors")
	assert.NotContains(t, after, "\033[?1049")
	assert.NotContains(t, after, ansiClearScreen)
}

func TestWatch_TransientErrorKeepsWatching(t *testing.T) {
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
	tw := newTestWatcher(mc, &buf, &errBuf, TreeParams{Namespace: "acme", ReleaseBindingName: "rb"}, nil)
	require.NoError(t, tw.run(ctx))
	assert.Contains(t, errBuf.String(), "fetch failed: blip")
	assert.NotContains(t, buf.String(), "fetch failed", "the retry line must not contaminate redirected tree output")
	assert.GreaterOrEqual(t, calls, 3, "the loop must survive a transient error")
}

func TestWatch_TerminalKeepsLastGoodFrameAcrossFailure(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	mc := mocks.NewMockInterface(t)
	calls := 0
	mc.EXPECT().GetReleaseBindingResourceTree(mock.Anything, "acme", "rb").
		RunAndReturn(func(context.Context, string, string) (*gen.K8sResourceTreeResponse, error) {
			calls++
			switch calls {
			case 1:
				return treeFixture(), nil
			case 2:
				return secondFixture(), nil
			case 3:
				return nil, fmt.Errorf("blip")
			default:
				cancel()
				return nil, context.Canceled
			}
		})

	var buf bytes.Buffer
	tw := newTestWatcher(mc, &buf, &buf, TreeParams{Namespace: "acme", ReleaseBindingName: "rb"}, fakeScreen(&buf, 40))
	require.NoError(t, tw.run(ctx))
	out := buf.String()

	leave := strings.Index(out, ansiLeaveAltScreen)
	require.NotEqual(t, -1, leave)
	live, after := out[:leave], out[leave+len(ansiLeaveAltScreen):]

	frames := strings.Split(live, ansiClearScreen)
	require.Len(t, frames, 4, "two good frames and one repaint after the failure; the canceled fetch paints nothing")
	assert.Contains(t, frames[3], "fetch failed 12:03:04: blip — showing 12:03:04, retrying in 1ms",
		"a failed fetch repaints the last good frame with the failure in the header")
	assert.Contains(t, frames[3], "Deployment/checkout-v2")
	assert.NotContains(t, live, "fetch failed: blip (retrying",
		"the retry line is not written to a stderr that shares the terminal: it would land after the frame and scroll it")
	assert.False(t, strings.HasSuffix(live, "\n"), "the live frame ends without a newline right up to the leave")
	assert.Contains(t, after, "Deployment/checkout-v2", "the reprint is the last successful tree")
	assert.Contains(t, after, "\nfetch failed 12:03:04: blip\n",
		"a watch that ended mid-outage says so after the reprint: the retry lines left with the alternate screen")
	assert.Less(t, strings.Index(after, "Deployment/checkout-v2"), strings.Index(after, "fetch failed"))
	assert.NotContains(t, after, "Stopped watching", "Ctrl-C is not a timeout expiry")
}

func TestWatch_RedirectedStderrKeepsRetryLine(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	mc := mocks.NewMockInterface(t)
	calls := 0
	mc.EXPECT().GetReleaseBindingResourceTree(mock.Anything, "acme", "rb").
		RunAndReturn(func(context.Context, string, string) (*gen.K8sResourceTreeResponse, error) {
			calls++
			switch calls {
			case 1:
				return treeFixture(), nil
			case 2:
				return nil, fmt.Errorf("blip")
			default:
				cancel()
				return nil, context.Canceled
			}
		})

	var buf, errBuf bytes.Buffer
	scr := fakeScreen(&buf, 40)
	scr.sharedErr = false
	tw := newTestWatcher(mc, &buf, &errBuf, TreeParams{Namespace: "acme", ReleaseBindingName: "rb"}, scr)
	require.NoError(t, tw.run(ctx))
	assert.Contains(t, errBuf.String(), "fetch failed: blip (retrying in 1ms)", "a stderr file gets the retry line as before")
	assert.NotContains(t, buf.String(), "fetch failed: blip (retrying")
}

func TestWatch_FrameIsOneWrite(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetReleaseBindingResourceTree(mock.Anything, "acme", "rb").
		Return(treeFixture(), nil)

	var cw countingWriter
	tw := newTestWatcher(mc, &cw, io.Discard, TreeParams{Namespace: "acme", ReleaseBindingName: "rb"}, fakeScreen(&cw, 40))
	require.NoError(t, tw.draw(context.Background()))
	assert.Equal(t, 1, cw.writes, "entering the alternate screen and the whole first frame are one write")
	assert.True(t, strings.HasPrefix(cw.buf.String(), ansiEnterAltScreen+ansiClearScreen))

	cw.writes = 0
	require.NoError(t, tw.draw(context.Background()))
	assert.Equal(t, 1, cw.writes, "a refresh is a single write")
}

func TestWatch_WaitsIntervalAfterSlowFailure(t *testing.T) {
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
	tw := newTestWatcher(mc, &buf, &errBuf, TreeParams{Namespace: "acme", ReleaseBindingName: "rb", Interval: interval}, nil)
	require.NoError(t, tw.run(ctx))
	require.GreaterOrEqual(t, calls, 3, "the loop must retry after the slow failure")
	assert.GreaterOrEqual(t, gap, interval,
		"the retry must wait a full interval measured from the end of the failed fetch, not from a free-running ticker")
}

func TestWatchDraw_AppendsWhenNotTTY(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetReleaseBindingResourceTree(mock.Anything, "acme", "rb").
		Return(treeFixture(), nil)

	var buf bytes.Buffer
	tw := newTestWatcher(mc, &buf, io.Discard, TreeParams{Namespace: "acme", ReleaseBindingName: "rb", Interval: 10 * time.Second}, nil)
	require.NoError(t, tw.draw(context.Background()))
	require.NoError(t, tw.draw(context.Background()))
	out := buf.String()
	assert.NotContains(t, out, "\033[")
	assert.Equal(t, 2, strings.Count(out, "Deployment/checkout"), "frames are appended, never replaced")
	assert.Contains(t, out, "Last refreshed 12:03:04")
}

func TestWatchDraw_HeaderShowsTimeout(t *testing.T) {
	draw := func(t *testing.T, timeout time.Duration) string {
		t.Helper()
		mc := mocks.NewMockInterface(t)
		mc.EXPECT().GetReleaseBindingResourceTree(mock.Anything, "acme", "rb").
			Return(treeFixture(), nil)

		var buf bytes.Buffer
		tw := newTestWatcher(mc, &buf, io.Discard,
			TreeParams{Namespace: "acme", ReleaseBindingName: "rb", Interval: 10 * time.Second, Timeout: timeout}, nil)
		require.NoError(t, tw.draw(context.Background()))
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

func TestWatchDraw_CutsFrameToWindow(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetReleaseBindingResourceTree(mock.Anything, "acme", "rb").
		Return(treeFixture(), nil)

	var full bytes.Buffer
	tw := newTestWatcher(mc, &full, io.Discard, TreeParams{Namespace: "acme", ReleaseBindingName: "rb"}, nil)
	require.NoError(t, tw.draw(context.Background()))
	total := strings.Count(full.String(), "\n")
	require.Greater(t, total, 6, "the fixture must be taller than the fake window for this test to mean anything")

	var buf bytes.Buffer
	tw = newTestWatcher(mc, &buf, io.Discard, TreeParams{Namespace: "acme", ReleaseBindingName: "rb"}, fakeScreen(&buf, 6))
	require.NoError(t, tw.draw(context.Background()))
	frame := strings.TrimPrefix(buf.String(), ansiEnterAltScreen+ansiClearScreen)
	lines := strings.Split(frame, "\n")
	assert.Len(t, lines, 6, "the frame fills the window exactly: 5 lines and the marker")
	assert.Equal(t, fmt.Sprintf("… %d more lines (narrow with --kind)", total-5), lines[5])
	assert.False(t, strings.HasSuffix(frame, "\n"), "a trailing newline on the bottom row would scroll the header away")
}

func TestFitFrame(t *testing.T) {
	frame := []byte("one\ntwo\nthree\nfour\n")

	t.Run("fits", func(t *testing.T) {
		assert.Equal(t, "one\ntwo\nthree\nfour", string(fitFrame(frame, 80, 4)), "exactly rows lines fit without a trailing newline")
		assert.Equal(t, "one\ntwo\nthree\nfour", string(fitFrame(frame, 80, 40)))
	})

	t.Run("cuts height", func(t *testing.T) {
		assert.Equal(t, "one\ntwo\n… 2 more lines (narrow with --kind)", string(fitFrame(frame, 80, 3)))
	})

	t.Run("unknown size leaves the frame alone", func(t *testing.T) {
		assert.Equal(t, "one\ntwo\nthree\nfour", string(fitFrame(frame, 0, 0)))
	})

	t.Run("clips width", func(t *testing.T) {
		assert.Equal(t, "on\ntw\nth\nfo", string(fitFrame(frame, 2, 40)))
	})

	t.Run("one row shows only the marker", func(t *testing.T) {
		assert.Equal(t, "… 4 more lines (narrow with --kind)", string(fitFrame(frame, 80, 1)))
		assert.Equal(t, "one", string(fitFrame([]byte("one\n"), 80, 1)))
	})

	t.Run("empty frame", func(t *testing.T) {
		assert.Equal(t, "", string(fitFrame(nil, 80, 3)))
	})

	t.Run("clips the marker too", func(t *testing.T) {
		assert.Equal(t, "one\ntwo\n… 2 more lines (narr", string(fitFrame(frame, 20, 3)),
			"a marker wider than the window would wrap and scroll the header away")
	})
}

func TestClipLine(t *testing.T) {
	assert.Equal(t, "abc", clipLine("abc", 5), "short lines are untouched")
	assert.Equal(t, "abc", clipLine("abcdef", 3))
	assert.Equal(t, "└─ Po", clipLine("└─ Pod/x", 5), "runes, not bytes, are counted")
	assert.Equal(t, "界", clipLine("界界", 3), "wide runes take two cells and never straddle the edge")
	assert.Equal(t, "e\u0301e\u0301", clipLine("e\u0301e\u0301e\u0301", 2), "combining marks take no cell")
	assert.Equal(t, "", clipLine("abc", 0))
	colored := "name  " + ansiGreen + "● Healthy" + ansiReset + "  2d"
	assert.Equal(t, colored, clipLine(colored, 30), "escape sequences do not count toward the width")
	assert.Equal(t, "name  "+ansiGreen+"● He"+ansiReset, clipLine(colored, 10), "a cut inside a color closes it")
}

func TestTreeCmd_WatchOnPipeAppendsFrames(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetReleaseBindingResourceTree(mock.Anything, "acme", "checkout-dev").
		Return(treeFixture(), nil)

	cmd := newTreeCmd(mockFactory(mc))
	require.NoError(t, cmd.Flags().Set("namespace", "acme"))
	require.NoError(t, cmd.Flags().Set("watch", "true"))
	require.NoError(t, cmd.Flags().Set("interval", "1ms"))
	require.NoError(t, cmd.Flags().Set("timeout", "20ms"))

	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, cmd.RunE(cmd, []string{"checkout-dev"}))
	})
	assert.NotContains(t, out, "\033[", "captured stdout is a pipe, so no terminal escapes")
	assert.Greater(t, strings.Count(out, "Watching acme/checkout-dev"), 1, "frames are appended while the watch runs")
	assert.NotContains(t, out, "Watched acme/checkout-dev", "no reprint without a terminal")
}
