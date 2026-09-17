// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package releasebinding

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"

	"golang.org/x/term"

	"github.com/openchoreo/openchoreo/internal/occ/cmdutil"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
)

// Tree shows the Kubernetes resource tree for a release binding's rendered
// releases, including child resources discovered in the target planes.
func (r *ReleaseBinding) Tree(params TreeParams) error {
	if err := cmdutil.RequireFields("tree", "releasebinding", map[string]string{"namespace": params.Namespace}); err != nil {
		return err
	}
	if params.Depth < 0 {
		return fmt.Errorf("--depth must be zero or positive")
	}
	if params.Timeout < 0 {
		return fmt.Errorf("--timeout must be zero or positive")
	}
	if params.Watch && params.Interval <= 0 {
		return fmt.Errorf("--interval must be positive")
	}
	tty := isTerminal(os.Stdout)
	opts := renderOptions{
		color: tty && os.Getenv("NO_COLOR") == "",
		depth: params.Depth,
		kind:  params.Kind,
	}
	if params.Watch {
		return r.treeWatch(os.Stdout, os.Stderr, params, opts, tty)
	}
	return r.treeOnce(os.Stdout, params, opts, tty)
}

func (r *ReleaseBinding) treeOnce(w io.Writer, params TreeParams, opts renderOptions, hints bool) error {
	resp, err := r.client.GetReleaseBindingResourceTree(context.Background(), params.Namespace, params.ReleaseBindingName)
	if err != nil {
		return err
	}
	renderTree(w, resp, opts)
	if hints {
		printHints(w, params, resp)
	}
	return nil
}

func isTerminal(f *os.File) bool {
	return term.IsTerminal(int(f.Fd()))
}

// printHints suggests copy-pasteable follow-up commands, derived only from
// data already in the response — it must never fail the command or trigger
// another request.
func printHints(w io.Writer, params TreeParams, resp *gen.K8sResourceTreeResponse) {
	var lines []string
	if component, project, env, ok := releaseOwner(resp); ok {
		lines = append(lines, fmt.Sprintf("occ component logs %s --project %s --env %s --namespace %s",
			component, project, env, params.Namespace))
	}
	lines = append(lines, fmt.Sprintf("occ releasebinding get %s --namespace %s",
		params.ReleaseBindingName, params.Namespace))
	if hasForbidden(resp) {
		lines = append(lines, "grant the cluster agent access to the forbidden kinds: docs/resource-tree/child-discovery.md")
	}

	fmt.Fprintln(w)
	fmt.Fprintln(w, "Dig deeper:")
	for _, l := range lines {
		fmt.Fprintf(w, "  %s\n", l)
	}
}

// releaseOwner pulls component, project and environment from the first
// rendered release carrying its RenderedRelease CR. All three must be present
// for a hint command to be constructable.
func releaseOwner(resp *gen.K8sResourceTreeResponse) (component, project, env string, ok bool) {
	for _, rel := range resp.RenderedReleases {
		if rel.RenderedRelease == nil || rel.RenderedRelease.Spec == nil {
			continue
		}
		spec := rel.RenderedRelease.Spec
		if spec.Owner.ComponentName == "" || spec.Owner.ProjectName == "" || spec.EnvironmentName == "" {
			continue
		}
		return spec.Owner.ComponentName, spec.Owner.ProjectName, spec.EnvironmentName, true
	}
	return "", "", "", false
}

// treeWatch installs signal handling and runs the refresh loop. The signal
// context exists before the first request so Ctrl-C interrupts an in-flight
// fetch and still exits 0. A positive --timeout bounds the same context, so an
// unattended watch cannot run forever. Trees go to w and watch diagnostics to
// errW, so redirecting stdout captures only the tree.
func (r *ReleaseBinding) treeWatch(w, errW io.Writer, params TreeParams, opts renderOptions, tty bool) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if params.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, params.Timeout)
		defer cancel()
	}
	return r.watchLoop(ctx, w, errW, params, opts, tty)
}

// watchLoop refreshes the tree until ctx is canceled. The interval is measured
// from the end of each fetch, so a fetch slower than the interval is still
// followed by a full interval of quiet rather than an immediate retry. The
// first fetch failing is a hard error; later failures print a retry line to
// errW and keep the loop alive so a transient gateway blip does not kill the
// watch. Cancellation is success, even when it interrupts an in-flight request.
func (r *ReleaseBinding) watchLoop(ctx context.Context, w, errW io.Writer, params TreeParams, opts renderOptions, tty bool) error {
	if err := r.watchDraw(ctx, w, params, opts, tty); err != nil {
		if ctx.Err() != nil {
			finishWatch(ctx, errW, params.Timeout)
			return nil
		}
		return err
	}
	timer := time.NewTimer(params.Interval)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			finishWatch(ctx, errW, params.Timeout)
			return nil
		case <-timer.C:
		}
		if err := r.watchDraw(ctx, w, params, opts, tty); err != nil {
			if ctx.Err() != nil {
				finishWatch(ctx, errW, params.Timeout)
				return nil
			}
			fmt.Fprintf(errW, "fetch failed: %v (retrying in %s)\n", err, params.Interval)
		}
		timer.Reset(params.Interval)
	}
}

// finishWatch explains on errW why a watch that observed cancellation stopped.
// An expired --timeout says so, since nobody is there to know why the output
// stopped; Ctrl-C needs no explanation.
func finishWatch(ctx context.Context, errW io.Writer, timeout time.Duration) {
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		fmt.Fprintf(errW, "Stopped watching after %s (--timeout)\n", timeout)
	}
}

func (r *ReleaseBinding) watchDraw(ctx context.Context, w io.Writer, params TreeParams, opts renderOptions, tty bool) error {
	resp, err := r.client.GetReleaseBindingResourceTree(ctx, params.Namespace, params.ReleaseBindingName)
	if err != nil {
		return err
	}
	if tty {
		fmt.Fprint(w, "\033[2J\033[H")
	}
	cadence := fmt.Sprintf("refreshes every %s", params.Interval)
	if params.Timeout > 0 {
		cadence = fmt.Sprintf("%s, stops after %s", cadence, params.Timeout)
	}
	fmt.Fprintf(w, "Watching %s/%s — %s (Ctrl-C to stop)\n\n",
		params.Namespace, params.ReleaseBindingName, cadence)
	renderTree(w, resp, opts)
	return nil
}
