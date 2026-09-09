// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

//go:build unix

package remote

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

func TestDiscoverWorkloadsFollowsSymlinkedDirectoryArgument(t *testing.T) {
	root := writeTree(t, map[string]string{"real/comp2.yaml": providerWorkloadYAML})
	link := filepath.Join(root, "link")
	if err := os.Symlink(filepath.Join(root, "real"), link); err != nil {
		t.Fatal(err)
	}

	d, err := discoverWorkloads([]string{link})
	if err != nil {
		t.Fatalf("discoverWorkloads: %v", err)
	}
	if len(d.workloads) != 1 {
		t.Fatalf("got %d workloads through a symlinked directory, want 1 (emptyDirs=%v)", len(d.workloads), d.emptyDirs)
	}
	// Paths are reported under the name the developer gave, not the link target.
	if got := d.workloads[0].path; !strings.HasPrefix(got, link) {
		t.Errorf("path = %q, want it under %q", got, link)
	}
}

func TestDiscoverWorkloadsFollowsSymlinkedSubdirectoryInsideTheRoot(t *testing.T) {
	root := writeTree(t, map[string]string{
		"components/real/comp1.yaml": consumerWorkloadYAML,
		"components/comp2.yaml":      providerWorkloadYAML,
	})
	dir := filepath.Join(root, "components")
	if err := os.Symlink(filepath.Join(dir, "real"), filepath.Join(dir, "alias")); err != nil {
		t.Fatal(err)
	}

	d, err := discoverWorkloads([]string{dir})
	if err != nil {
		t.Fatalf("discoverWorkloads: %v", err)
	}
	if len(d.workloads) != 2 {
		t.Fatalf("got %d workloads, want 2 with the in-root symlink followed once", len(d.workloads))
	}
}

// TestDiscoverWorkloadsRefusesSymlinkOutOfTheRoot covers a link that would pull in
// workloads the developer never named.
func TestDiscoverWorkloadsRefusesSymlinkOutOfTheRoot(t *testing.T) {
	root := writeTree(t, map[string]string{
		"components/comp2.yaml": providerWorkloadYAML,
		"sibling.yaml":          consumerWorkloadYAML,
		"elsewhere/comp3.yaml":  otherNamespaceWorkloadYAML,
	})
	dir := filepath.Join(root, "components")
	up := filepath.Join(dir, "up")
	if err := os.Symlink("..", up); err != nil {
		t.Fatal(err)
	}

	d, err := discoverWorkloads([]string{dir})
	if err != nil {
		t.Fatalf("discoverWorkloads: %v", err)
	}
	if len(d.workloads) != 1 || d.workloads[0].wl.Spec.Owner.ComponentName != providerComponent {
		t.Fatalf("got %+v, want only the workload inside the named root", d.workloads)
	}
	if got := noteFor(d, up); !strings.Contains(got, "outside") {
		t.Errorf("note for the escaping link = %q, want it to say the link leads outside", got)
	}
}

func TestDiscoverWorkloadsTerminatesOnSymlinkCycle(t *testing.T) {
	root := writeTree(t, map[string]string{"components/comp2.yaml": providerWorkloadYAML})
	dir := filepath.Join(root, "components")
	if err := os.Symlink(dir, filepath.Join(dir, "self")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(root, filepath.Join(dir, "up")); err != nil {
		t.Fatal(err)
	}

	d, err := discoverWorkloads([]string{dir})
	if err != nil {
		t.Fatalf("discoverWorkloads: %v", err)
	}
	if len(d.workloads) != 1 {
		t.Fatalf("got %d workloads, want 1", len(d.workloads))
	}
}

// TestDiscoverWorkloadsDedupesSymlinkedAlias covers a second name for one file, which
// must not read as two workloads claiming the same component.
func TestDiscoverWorkloadsDedupesSymlinkedAlias(t *testing.T) {
	root := writeTree(t, map[string]string{"components/comp2.yaml": providerWorkloadYAML})
	dir := filepath.Join(root, "components")
	if err := os.Symlink(filepath.Join(dir, "comp2.yaml"), filepath.Join(dir, "latest.yaml")); err != nil {
		t.Fatal(err)
	}

	d, err := discoverWorkloads([]string{dir})
	if err != nil {
		t.Fatalf("discoverWorkloads: %v", err)
	}
	if len(d.workloads) != 1 {
		t.Fatalf("got %d workloads for one file under two names, want 1", len(d.workloads))
	}
	if err := New(erroringResolver{t: t}).Connect(t.Context(), ConnectParams{
		WorkloadPaths: []string{dir},
		Namespace:     "default",
		DryRun:        true,
	}, io.Discard); err != nil {
		t.Errorf("Connect: %v, want no duplicate-workload error", err)
	}
}

// TestDiscoverWorkloadsSkipsIrregularFile covers a FIFO named *.yaml, which blocks
// forever on read. A regression here hangs rather than fails.
func TestDiscoverWorkloadsSkipsIrregularFile(t *testing.T) {
	root := writeTree(t, map[string]string{"components/comp2.yaml": providerWorkloadYAML})
	dir := filepath.Join(root, "components")
	fifo := filepath.Join(dir, "pipe.yaml")
	if err := syscall.Mkfifo(fifo, 0o600); err != nil {
		t.Fatal(err)
	}

	d, err := discoverWorkloads([]string{dir})
	if err != nil {
		t.Fatalf("discoverWorkloads: %v", err)
	}
	if len(d.workloads) != 1 {
		t.Fatalf("got %d workloads, want the one regular file", len(d.workloads))
	}
	if got := noteFor(d, fifo); got != "not a regular file" {
		t.Errorf("note for the FIFO = %q, want %q", got, "not a regular file")
	}
}

func TestDiscoverWorkloadsSkipsDanglingSymlink(t *testing.T) {
	root := writeTree(t, map[string]string{"components/comp2.yaml": providerWorkloadYAML})
	dir := filepath.Join(root, "components")
	dangling := filepath.Join(dir, "gone.yaml")
	if err := os.Symlink(filepath.Join(root, "nowhere.yaml"), dangling); err != nil {
		t.Fatal(err)
	}

	d, err := discoverWorkloads([]string{dir})
	if err != nil {
		t.Fatalf("discoverWorkloads: %v", err)
	}
	if len(d.workloads) != 1 {
		t.Fatalf("got %d workloads, want 1 alongside the dangling link", len(d.workloads))
	}
	if noteFor(d, dangling) == "" {
		t.Errorf("dangling symlink was not recorded: %+v", d.skipped)
	}
}

func TestDiscoverWorkloadsSkipsUnreadablePaths(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root bypasses file permissions")
	}
	root := writeTree(t, map[string]string{
		"components/comp2.yaml":       providerWorkloadYAML,
		"components/locked.yaml":      providerWorkloadYAML,
		"components/lockeddir/x.yaml": providerWorkloadYAML,
	})
	dir := filepath.Join(root, "components")
	lockedFile := filepath.Join(dir, "locked.yaml")
	lockedDir := filepath.Join(dir, "lockeddir")
	if err := os.Chmod(lockedFile, 0o000); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(lockedDir, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.Chmod(lockedFile, 0o600)
		_ = os.Chmod(lockedDir, 0o700)
	})

	d, err := discoverWorkloads([]string{dir})
	if err != nil {
		t.Fatalf("discoverWorkloads: %v", err)
	}
	if len(d.workloads) != 1 {
		t.Fatalf("got %d workloads, want the one readable file", len(d.workloads))
	}
	for _, path := range []string{lockedFile, lockedDir} {
		if noteFor(d, path) == "" {
			t.Errorf("%s was not recorded: %+v", path, d.skipped)
		}
	}
}
