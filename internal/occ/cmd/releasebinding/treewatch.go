// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package releasebinding

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
	"unicode"
	"unicode/utf8"

	"golang.org/x/term"
	"golang.org/x/text/width"

	"github.com/openchoreo/openchoreo/internal/occ/resources/client"
)

const (
	ansiEnterAltScreen = "\033[?1049h"
	ansiLeaveAltScreen = "\033[?1049l"
	ansiClearScreen    = "\033[H\033[J"
	clockLayout        = "15:04:05"
)

// treeWatch installs signal handling and runs the refresh loop. The signal
// context exists before the first request so Ctrl-C interrupts an in-flight
// fetch and still exits 0. A positive --timeout bounds the same context, so an
// unattended watch cannot run forever. Trees go to w and watch diagnostics to
// errW, so redirecting stdout captures only the tree. scr is nil when stdout
// is not a terminal, in which case frames are simply appended.
func (r *ReleaseBinding) treeWatch(w, errW io.Writer, params TreeParams, opts renderOptions, scr *termScreen) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if params.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, params.Timeout)
		defer cancel()
	}
	tw := &treeWatcher{client: r.client, w: w, errW: errW, params: params, opts: opts, scr: scr, now: time.Now}
	return tw.run(ctx)
}

// treeWatcher is the state of one --watch run: where frames go, the screen
// they are painted on, and the last successful frame, which survives
// transient fetch failures and is printed once more after the watch ends.
type treeWatcher struct {
	client client.Interface
	w      io.Writer
	errW   io.Writer
	params TreeParams
	opts   renderOptions
	scr    *termScreen
	now    func() time.Time

	last    *watchFrame
	failure string // latest fetch failure since last, empty once a fetch succeeds
}

type watchFrame struct {
	tree []byte
	at   time.Time
}

// run drives the loop and then puts the terminal back. The alternate screen
// takes the live frames with it, so on a terminal the last tree is printed
// again on the normal screen, without the watch header, before the stop
// notice. The deferred leave only matters when the loop panics; on every
// other path the explicit leave has already run.
func (tw *treeWatcher) run(ctx context.Context) error {
	defer tw.scr.leave()
	err := tw.loop(ctx)
	tw.scr.leave()
	if err != nil {
		return err
	}
	if tw.scr != nil && tw.last != nil {
		fmt.Fprintf(tw.w, "Watched %s/%s — last refreshed %s\n\n", tw.params.Namespace, tw.params.ReleaseBindingName, tw.last.at.Format(clockLayout))
		_, _ = tw.w.Write(tw.last.tree)
	}
	tw.finish(ctx)
	return nil
}

// loop refreshes the tree until ctx is canceled. The interval is measured
// from the end of each fetch, so a fetch slower than the interval is still
// followed by a full interval of quiet rather than an immediate retry. The
// first fetch failing is a hard error; later failures keep the loop alive so
// a transient gateway blip does not kill the watch. They are reported with a
// retry line on errW, except when errW is the terminal being painted: text
// appended there lands after a frame that fills the window and scrolls it,
// so the frame header carries the failure instead. Cancellation is success,
// even when it interrupts an in-flight request.
func (tw *treeWatcher) loop(ctx context.Context) error {
	if err := tw.draw(ctx); err != nil {
		if ctx.Err() != nil {
			return nil
		}
		return err
	}
	timer := time.NewTimer(tw.params.Interval)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-timer.C:
		}
		if err := tw.draw(ctx); err != nil {
			if ctx.Err() != nil {
				return nil
			}
			if !tw.scr.coversStderr() {
				fmt.Fprintf(tw.errW, "fetch failed: %v (retrying in %s)\n", err, tw.params.Interval)
			}
		}
		timer.Reset(tw.params.Interval)
	}
}

// finish explains on errW what the reprinted tree does not show. A watch
// that ended while fetches were failing says so, since on a terminal the
// failure was only ever shown in the frame header, which left with the
// alternate screen. An expired --timeout says so, since nobody is there to
// know why the output stopped; Ctrl-C needs no explanation.
func (tw *treeWatcher) finish(ctx context.Context) {
	var notes []string
	if tw.scr != nil && tw.failure != "" {
		notes = append(notes, tw.failure)
	}
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		notes = append(notes, fmt.Sprintf("Stopped watching after %s (--timeout)", tw.params.Timeout))
	}
	if len(notes) == 0 {
		return
	}
	fmt.Fprintln(tw.errW)
	for _, n := range notes {
		fmt.Fprintln(tw.errW, n)
	}
}

// draw fetches the tree and renders it. A failed fetch keeps the previous
// frame: on a terminal it is repainted with the failure in the header;
// without a terminal the retry line on stderr is the whole story. A fetch
// cut short by cancellation is not a failure and paints nothing.
func (tw *treeWatcher) draw(ctx context.Context) error {
	resp, err := tw.client.GetReleaseBindingResourceTree(ctx, tw.params.Namespace, tw.params.ReleaseBindingName)
	if err != nil {
		if ctx.Err() != nil {
			return err
		}
		tw.failure = fmt.Sprintf("fetch failed %s: %v", tw.now().Format(clockLayout), err)
		if tw.scr != nil && tw.last != nil {
			tw.render()
		}
		return err
	}
	tw.failure = ""
	var tree bytes.Buffer
	renderTree(&tree, tw.params.ReleaseBindingName, resp, tw.opts)
	tw.last = &watchFrame{tree: tree.Bytes(), at: tw.now()}
	tw.render()
	return nil
}

// render writes the last frame, header included, as one write, so the
// terminal is handed a whole frame rather than a header and then rows.
func (tw *treeWatcher) render() {
	var frame bytes.Buffer
	cadence := fmt.Sprintf("refreshes every %s", tw.params.Interval)
	if tw.params.Timeout > 0 {
		cadence = fmt.Sprintf("%s, stops after %s", cadence, tw.params.Timeout)
	}
	fmt.Fprintf(&frame, "Watching %s/%s — %s (Ctrl-C to stop)\n", tw.params.Namespace, tw.params.ReleaseBindingName, cadence)
	refreshed := tw.last.at.Format(clockLayout)
	if tw.failure != "" {
		fmt.Fprintf(&frame, "%s\n", colorize(fmt.Sprintf("%s — showing %s, retrying in %s", tw.failure, refreshed, tw.params.Interval), ansiYellow, tw.opts.color))
	} else {
		fmt.Fprintf(&frame, "Last refreshed %s\n", refreshed)
	}
	frame.WriteString("\n")
	frame.Write(tw.last.tree)
	if tw.scr == nil {
		_, _ = tw.w.Write(frame.Bytes())
		return
	}
	tw.scr.paint(frame.Bytes())
}

// termScreen paints frames on the terminal's alternate screen buffer. That
// buffer has no scrollback, so refreshes never pile up above the live frame,
// and leaving it restores whatever was on screen before the watch started.
// Because there is no scrollback, a frame that does not fit the window is cut
// to it rather than allowed to scroll the header away.
type termScreen struct {
	w    io.Writer
	size func() (cols, rows int, ok bool)
	// sharedErr records that stderr is a terminal too, so anything written
	// there while the screen is active would land on the frame.
	sharedErr bool
	active    bool
}

func newTermScreen(f *os.File, sharedErr bool) *termScreen {
	return &termScreen{
		w: f,
		size: func() (int, int, bool) {
			cols, rows, err := term.GetSize(int(f.Fd()))
			return cols, rows, err == nil
		},
		sharedErr: sharedErr,
	}
}

// coversStderr reports whether text written to stderr right now would land
// on the live frame. A nil or inactive screen covers nothing.
func (s *termScreen) coversStderr() bool {
	return s != nil && s.active && s.sharedErr
}

// paint replaces whatever is on the alternate screen with frame, in one
// write. The first paint also enters the alternate screen, so a watch whose
// first fetch fails never touches it. Without a known window size the frame
// is still stripped of its trailing newline, which could otherwise scroll
// the screen when the last row sits on the bottom line.
func (s *termScreen) paint(frame []byte) {
	cols, rows, ok := s.size()
	if !ok {
		cols, rows = 0, 0
	}
	frame = fitFrame(frame, cols, rows)
	out := make([]byte, 0, len(ansiEnterAltScreen)+len(ansiClearScreen)+len(frame))
	if !s.active {
		s.active = true
		out = append(out, ansiEnterAltScreen...)
	}
	out = append(out, ansiClearScreen...)
	out = append(out, frame...)
	_, _ = s.w.Write(out)
}

// leave returns to the normal screen. It is safe on a nil screen and after
// an earlier leave, so it can be both deferred and called explicitly.
func (s *termScreen) leave() {
	if s == nil || !s.active {
		return
	}
	s.active = false
	_, _ = io.WriteString(s.w, ansiLeaveAltScreen)
}

// fitFrame cuts a frame to a cols x rows window; a zero dimension means
// unknown and is not enforced. At most rows lines are kept, the last of them
// a marker for what was cut, and every line including the marker is clipped
// to the width so none wraps onto a second row. The result has no trailing
// newline: a newline on the bottom row would scroll the screen and lose the
// header.
func fitFrame(frame []byte, cols, rows int) []byte {
	lines := strings.Split(strings.TrimSuffix(string(frame), "\n"), "\n")
	if rows > 0 && len(lines) > rows {
		cut := len(lines) - (rows - 1)
		lines = append(lines[:rows-1], fmt.Sprintf("… %d more lines (narrow with --kind)", cut))
	}
	if cols > 0 {
		for i, l := range lines {
			lines[i] = clipLine(l, cols)
		}
	}
	return []byte(strings.Join(lines, "\n"))
}

// clipLine truncates s to width terminal cells. SGR color sequences are
// copied through without counting, and a color left open by the cut is
// reset so it cannot bleed into the next line. A wide rune that would
// straddle the edge is dropped rather than wrapped.
func clipLine(s string, width int) string {
	var b strings.Builder
	used := 0
	colored := false
	for i := 0; i < len(s); {
		if strings.HasPrefix(s[i:], "\033[") {
			end := strings.IndexByte(s[i:], 'm')
			if end < 0 {
				break
			}
			seq := s[i : i+end+1]
			b.WriteString(seq)
			colored = seq != ansiReset
			i += end + 1
			continue
		}
		r, n := utf8.DecodeRuneInString(s[i:])
		if used+cellWidth(r) > width {
			if colored {
				b.WriteString(ansiReset)
			}
			return b.String()
		}
		b.WriteRune(r)
		used += cellWidth(r)
		i += n
	}
	return b.String()
}

// cellWidth is the number of terminal cells r occupies: none for combining
// marks and format characters, two for East Asian wide and fullwidth runes,
// one for everything else. Ambiguous-width runes such as the tree glyphs
// count as one, which is how non-CJK locales render them.
func cellWidth(r rune) int {
	if unicode.In(r, unicode.Mn, unicode.Me, unicode.Cf) {
		return 0
	}
	switch width.LookupRune(r).Kind() {
	case width.EastAsianWide, width.EastAsianFullwidth:
		return 2
	}
	return 1
}
