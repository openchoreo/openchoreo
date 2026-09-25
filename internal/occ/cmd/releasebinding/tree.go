// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package releasebinding

import (
	"context"
	"fmt"
	"io"
	"os"

	"golang.org/x/term"

	"github.com/openchoreo/openchoreo/internal/occ/cmdutil"
)

// Tree shows the Kubernetes resource tree for a release binding's rendered
// releases, including child resources discovered in the target planes.
func (r *ReleaseBinding) Tree(params TreeParams) error {
	if err := cmdutil.RequireFields("tree", "releasebinding", map[string]string{"namespace": params.Namespace}); err != nil {
		return err
	}
	if params.Timeout < 0 {
		return fmt.Errorf("--timeout must be zero or positive")
	}
	if params.Watch && params.Interval <= 0 {
		return fmt.Errorf("--interval must be positive")
	}
	// Escape sequences need a terminal that understands them, which a dumb
	// one does not.
	tty := isTerminal(os.Stdout) && os.Getenv("TERM") != "dumb"
	opts := renderOptions{
		color: tty && os.Getenv("NO_COLOR") == "",
		kind:  params.Kind,
	}
	if params.Watch {
		var scr *termScreen
		if tty {
			scr = newTermScreen(os.Stdout, isTerminal(os.Stderr))
		}
		return r.treeWatch(os.Stdout, os.Stderr, params, opts, scr)
	}
	return r.treeOnce(os.Stdout, params, opts)
}

func (r *ReleaseBinding) treeOnce(w io.Writer, params TreeParams, opts renderOptions) error {
	resp, err := r.client.GetReleaseBindingResourceTree(context.Background(), params.Namespace, params.ReleaseBindingName)
	if err != nil {
		return err
	}
	renderTree(w, params.ReleaseBindingName, resp, opts)
	return nil
}

func isTerminal(f *os.File) bool {
	return term.IsTerminal(int(f.Fd()))
}
