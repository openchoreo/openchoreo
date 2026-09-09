// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package remote

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/openchoreo/openchoreo/internal/remoteconnect"
)

// writeTree materializes name -> content under a fresh temp dir and returns the root.
func writeTree(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for name, content := range files {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

// providerComponent is the component providerWorkloadYAML declares.
const providerComponent = "comp2"

// noteFor returns the recorded reason for path, or "" when it was not recorded.
func noteFor(d discovery, path string) string {
	for _, s := range d.skipped {
		if s.path == path {
			return s.reason
		}
	}
	return ""
}

// squeeze collapses each line's column padding to single spaces, so a table assertion
// checks field order rather than tabwriter's widths.
func squeeze(s string) string {
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		lines[i] = strings.Join(strings.Fields(line), " ")
	}
	return strings.Join(lines, "\n")
}

const otherNamespaceWorkloadYAML = `apiVersion: openchoreo.dev/v1alpha1
kind: Workload
metadata:
  name: comp4
  namespace: acme
spec:
  owner:
    projectName: platform
    componentName: comp4
`

const projectYAML = `apiVersion: openchoreo.dev/v1alpha1
kind: Project
metadata:
  name: demo
`

const twoWorkloadsYAML = consumerWorkloadYAML + "---\n" + `apiVersion: openchoreo.dev/v1alpha1
kind: Workload
metadata:
  name: comp3
spec:
  owner:
    projectName: demo
    componentName: comp3
`

// TestDiscoverWorkloadsExpandsDirectoryRecursively covers both component layouts: a
// nested comp1/workload.yaml and a flat file holding Component + Workload.
func TestDiscoverWorkloadsExpandsDirectoryRecursively(t *testing.T) {
	root := writeTree(t, map[string]string{
		"components/comp1/workload.yaml": consumerWorkloadYAML,
		"components/comp2.yaml":          providerWorkloadYAML,
		"components/project.yaml":        projectYAML,
		"components/README.md":           "not yaml",
	})

	d, err := discoverWorkloads([]string{filepath.Join(root, "components")})
	if err != nil {
		t.Fatalf("discoverWorkloads: %v", err)
	}
	if !d.fromDir {
		t.Error("fromDir = false, want true for a directory argument")
	}
	found := d.workloads
	if len(found) != 2 {
		t.Fatalf("got %d workloads, want 2: %+v", len(found), found)
	}
	if got, want := found[0].wl.Spec.Owner.ComponentName, "comp1"; got != want {
		t.Errorf("found[0] component = %q, want %q", got, want)
	}
	if got, want := found[1].wl.Spec.Owner.ComponentName, providerComponent; got != want {
		t.Errorf("found[1] component = %q, want %q", got, want)
	}
}

// TestDiscoverWorkloadsSkipsNonWorkloadFilesInDirectory covers the asymmetry: a walk
// skips a non-workload file, naming one is an error.
func TestDiscoverWorkloadsSkipsNonWorkloadFilesInDirectory(t *testing.T) {
	root := writeTree(t, map[string]string{
		"components/project.yaml": projectYAML,
		"components/comp2.yaml":   providerWorkloadYAML,
		"components/garbage.yaml": "this: [is, not: valid",
		"components/empty.yml":    "",
		"components/binding.yaml": projectYAML,
	})

	d, err := discoverWorkloads([]string{filepath.Join(root, "components")})
	if err != nil {
		t.Fatalf("discoverWorkloads: %v", err)
	}
	if len(d.workloads) != 1 || d.workloads[0].wl.Spec.Owner.ComponentName != providerComponent {
		t.Fatalf("got %+v, want just comp2", d.workloads)
	}

	explicit := filepath.Join(root, "components", "project.yaml")
	if _, err := discoverWorkloads([]string{explicit}); !errors.Is(err, errNoWorkloadDoc) {
		t.Errorf("explicit non-workload file: err = %v, want errNoWorkloadDoc", err)
	}
}

func TestDiscoverWorkloadsTakesEveryWorkloadDoc(t *testing.T) {
	path := writeWorkloadFileContent(t, "all.yaml", twoWorkloadsYAML)

	d, err := discoverWorkloads([]string{path})
	if err != nil {
		t.Fatalf("discoverWorkloads: %v", err)
	}
	if d.fromDir {
		t.Error("fromDir = true, want false for a file argument")
	}
	found := d.workloads
	if len(found) != 2 {
		t.Fatalf("got %d workloads, want 2", len(found))
	}
	for i, want := range []string{"comp1", "comp3"} {
		if got := found[i].wl.Spec.Owner.ComponentName; got != want {
			t.Errorf("found[%d] component = %q, want %q", i, got, want)
		}
	}
}

// TestDiscoverWorkloadsDedupesOverlappingArguments names a directory and one of its own
// files, in both orders.
func TestDiscoverWorkloadsDedupesOverlappingArguments(t *testing.T) {
	root := writeTree(t, map[string]string{"components/comp2.yaml": providerWorkloadYAML})
	dir := filepath.Join(root, "components")
	file := filepath.Join(dir, "comp2.yaml")

	for _, paths := range [][]string{{dir, file}, {file, dir}} {
		d, err := discoverWorkloads(paths)
		if err != nil {
			t.Fatalf("discoverWorkloads(%v): %v", paths, err)
		}
		if len(d.workloads) != 1 {
			t.Fatalf("discoverWorkloads(%v) got %d workloads, want 1: %+v", paths, len(d.workloads), d.workloads)
		}
	}
}

// TestDiscoverWorkloadsSkipsExcludedDirectories checks the scanner's skip list applies
// to subdirectories but never to the named root.
func TestDiscoverWorkloadsSkipsExcludedDirectories(t *testing.T) {
	root := writeTree(t, map[string]string{
		"components/comp2.yaml":                 providerWorkloadYAML,
		"components/node_modules/vendored.yaml": consumerWorkloadYAML,
		"components/.hidden/comp1.yaml":         consumerWorkloadYAML,
		"components/vendor/comp1.yaml":          consumerWorkloadYAML,
	})

	d, err := discoverWorkloads([]string{filepath.Join(root, "components")})
	if err != nil {
		t.Fatalf("discoverWorkloads: %v", err)
	}
	if len(d.workloads) != 1 || d.workloads[0].wl.Spec.Owner.ComponentName != providerComponent {
		t.Fatalf("got %+v, want just comp2", d.workloads)
	}

	build := writeTree(t, map[string]string{"build/comp2.yaml": providerWorkloadYAML})
	d, err = discoverWorkloads([]string{filepath.Join(build, "build")})
	if err != nil {
		t.Fatalf("discoverWorkloads(build/): %v", err)
	}
	if len(d.workloads) != 1 {
		t.Fatalf("got %d workloads under an explicitly named build/, want 1", len(d.workloads))
	}
}

func TestDiscoverWorkloadsMissingPath(t *testing.T) {
	root := writeTree(t, map[string]string{"components/comp2.yaml": providerWorkloadYAML})

	_, err := discoverWorkloads([]string{filepath.Join(root, "nope")})
	if err == nil || !strings.Contains(err.Error(), "does not exist") {
		t.Fatalf("err = %v, want it to name a missing path", err)
	}
}

func TestDiscoverWorkloadsReportsEmptyDirectory(t *testing.T) {
	root := writeTree(t, map[string]string{
		"empty-dir/project.yaml": projectYAML,
		"components/comp2.yaml":  providerWorkloadYAML,
	})
	empty := filepath.Join(root, "empty-dir")

	d, err := discoverWorkloads([]string{empty, filepath.Join(root, "components")})
	if err != nil {
		t.Fatalf("discoverWorkloads: %v", err)
	}
	if len(d.workloads) != 1 {
		t.Fatalf("got %d workloads, want the one from components/", len(d.workloads))
	}
	if len(d.emptyDirs) != 1 || d.emptyDirs[0] != empty {
		t.Errorf("emptyDirs = %v, want [%s]", d.emptyDirs, empty)
	}
	if want := []string{filepath.Join(root, "components")}; !slices.Equal(d.sources, want) {
		t.Errorf("sources = %v, want %v", d.sources, want)
	}
}

func TestConnectDryRunListsExpansionWithoutResolving(t *testing.T) {
	root := writeTree(t, map[string]string{
		"components/comp1/workload.yaml": consumerWorkloadYAML,
		"components/comp2.yaml":          providerWorkloadYAML,
	})
	dir := filepath.Join(root, "components")

	d := New(erroringResolver{t: t})
	d.runShell = func(context.Context, []string) error {
		t.Fatal("runShell should not be called under --dry-run")
		return nil
	}

	var out bytes.Buffer
	if err := d.Connect(context.Background(), ConnectParams{
		WorkloadPaths: []string{dir},
		Namespace:     "default",
		DryRun:        true,
	}, &out); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	got := out.String()
	for _, want := range []string{
		"2 workloads from " + dir + " (namespace default):",
		"PATH PROJECT COMPONENT",
		filepath.Join(dir, "comp1", "workload.yaml") + " demo comp1",
		filepath.Join(dir, "comp2.yaml") + " demo comp2",
	} {
		if !strings.Contains(squeeze(got), want) {
			t.Errorf("output missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "NAMESPACE") {
		t.Errorf("shared namespace should not get its own column:\n%s", got)
	}
}

// TestConnectDryRunKeepsNamespaceColumnAcrossNamespaces covers a tree whose namespace
// the summary cannot state.
func TestConnectDryRunKeepsNamespaceColumnAcrossNamespaces(t *testing.T) {
	root := writeTree(t, map[string]string{
		"components/comp2.yaml": providerWorkloadYAML,
		"components/comp4.yaml": otherNamespaceWorkloadYAML,
	})
	dir := filepath.Join(root, "components")

	var out bytes.Buffer
	if err := New(erroringResolver{t: t}).Connect(context.Background(), ConnectParams{
		WorkloadPaths: []string{dir},
		Namespace:     "default",
		DryRun:        true,
	}, &out); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	got := out.String()
	if want := "2 workloads from " + dir + ":"; !strings.Contains(got, want) {
		t.Errorf("output missing %q:\n%s", want, got)
	}
	if strings.Contains(got, "(namespace") {
		t.Errorf("summary should not claim a namespace:\n%s", got)
	}
	for _, want := range []string{
		"PATH NAMESPACE PROJECT COMPONENT",
		filepath.Join(dir, "comp2.yaml") + " default demo comp2",
		filepath.Join(dir, "comp4.yaml") + " acme platform comp4",
	} {
		if !strings.Contains(squeeze(got), want) {
			t.Errorf("output missing %q:\n%s", want, got)
		}
	}
}

// TestConnectAnnouncesDirectoryExpansion covers the summary header, and its absence
// when the arguments are files.
func TestConnectAnnouncesDirectoryExpansion(t *testing.T) {
	root := writeTree(t, map[string]string{
		"components/comp1.yaml": consumerWorkloadYAML,
		"components/comp2.yaml": providerWorkloadYAML,
	})
	dir := filepath.Join(root, "components")

	run := func(t *testing.T, paths []string) string {
		t.Helper()
		d := New(erroringResolver{t: t})
		d.dialTunnel = func(context.Context, remoteconnect.AgentEndpoint, string) (tunnel, error) {
			t.Fatal("dialTunnel should not be called for a locally-linked dependency")
			return nil, nil
		}
		d.runShell = func(context.Context, []string) error { return nil }
		var out bytes.Buffer
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := d.Connect(ctx, ConnectParams{
			WorkloadPaths: paths,
			Namespace:     "default",
			Environment:   "development",
		}, &out); err != nil {
			t.Fatalf("Connect: %v", err)
		}
		return out.String()
	}

	if got, want := run(t, []string{dir}), "2 workloads from "+dir+", all treated as running locally"; !strings.Contains(got, want) {
		t.Errorf("directory output missing %q:\n%s", want, got)
	}
	named := run(t, []string{filepath.Join(dir, "comp1.yaml"), filepath.Join(dir, "comp2.yaml")})
	if strings.Contains(named, "all treated as running locally") {
		t.Errorf("named files should not print the expansion header:\n%s", named)
	}
}

func TestConnectDuplicateWorkloadNamesBothPaths(t *testing.T) {
	root := writeTree(t, map[string]string{
		"components/comp1.yaml": consumerWorkloadYAML,
		"components/copy.yaml":  consumerWorkloadYAML,
	})
	dir := filepath.Join(root, "components")

	err := New(erroringResolver{t: t}).Connect(context.Background(), ConnectParams{
		WorkloadPaths: []string{dir},
		Namespace:     "default",
		Environment:   "development",
	}, io.Discard)
	if err == nil {
		t.Fatal("Connect: want a duplicate-workload error")
	}
	for _, want := range []string{
		"duplicate workload for default/demo/comp1",
		filepath.Join(dir, "comp1.yaml"),
		filepath.Join(dir, "copy.yaml"),
	} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q missing %q", err, want)
		}
	}
}

// TestConnectWarnsAboutEmptyDirectoryAndContinues covers an unproductive directory
// argument alongside a productive one.
func TestConnectWarnsAboutEmptyDirectoryAndContinues(t *testing.T) {
	root := writeTree(t, map[string]string{
		"empty-dir/project.yaml": projectYAML,
		"components/comp2.yaml":  providerWorkloadYAML,
	})
	empty := filepath.Join(root, "empty-dir")

	var out bytes.Buffer
	if err := New(erroringResolver{t: t}).Connect(context.Background(), ConnectParams{
		WorkloadPaths: []string{empty, filepath.Join(root, "components")},
		Namespace:     "default",
		DryRun:        true,
	}, &out); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	got := out.String()
	if want := "! no Workload documents found under " + empty; !strings.Contains(got, want) {
		t.Errorf("output missing %q:\n%s", want, got)
	}
	if want := "demo " + providerComponent; !strings.Contains(squeeze(got), want) {
		t.Errorf("output missing the productive directory's workload %q:\n%s", want, got)
	}
	if want := "1 workload from " + filepath.Join(root, "components") + " (namespace default):"; !strings.Contains(got, want) {
		t.Errorf("summary should name only the productive path, want %q:\n%s", want, got)
	}
}

func TestConnectErrorsWhenEveryPathIsEmpty(t *testing.T) {
	root := writeTree(t, map[string]string{
		"empty-dir/project.yaml": projectYAML,
		"other-dir/binding.yaml": projectYAML,
	})
	first := filepath.Join(root, "empty-dir")
	second := filepath.Join(root, "other-dir")

	var out bytes.Buffer
	err := New(erroringResolver{t: t}).Connect(context.Background(), ConnectParams{
		WorkloadPaths: []string{first, second},
		Namespace:     "default",
		Environment:   "development",
	}, &out)
	if err == nil {
		t.Fatal("Connect: want an error when no path yields a workload")
	}
	if want := "no Workload documents found under " + first + ", " + second; err.Error() != want {
		t.Errorf("err = %q, want %q", err, want)
	}
	if out.Len() != 0 {
		t.Errorf("nothing should be printed before the error:\n%s", out.String())
	}
}

const foreignWorkloadYAML = `apiVersion: apps/v1
kind: Workload
metadata:
  name: theirs
spec:
  owner:
    projectName: theirs
    componentName: theirs
`

const ownerlessWorkloadYAML = `apiVersion: openchoreo.dev/v1alpha1
kind: Workload
metadata:
  name: stub
  namespace: default
spec: {}
`

// TestDiscoverWorkloadsIgnoresForeignAPIGroup covers a kind: Workload from another
// ecosystem, which several projects define.
func TestDiscoverWorkloadsIgnoresForeignAPIGroup(t *testing.T) {
	root := writeTree(t, map[string]string{
		"components/foreign.yaml": foreignWorkloadYAML,
		"components/comp2.yaml":   providerWorkloadYAML,
	})
	dir := filepath.Join(root, "components")

	d, err := discoverWorkloads([]string{dir})
	if err != nil {
		t.Fatalf("discoverWorkloads: %v", err)
	}
	if len(d.workloads) != 1 || d.workloads[0].wl.Spec.Owner.ComponentName != providerComponent {
		t.Fatalf("got %+v, want just comp2", d.workloads)
	}
	if len(d.skipped) != 0 {
		t.Errorf("a foreign document is not ours to report: %+v", d.skipped)
	}

	named := filepath.Join(dir, "foreign.yaml")
	if _, err := discoverWorkloads([]string{named}); !errors.Is(err, errNoWorkloadDoc) {
		t.Errorf("naming a foreign Workload: err = %v, want errNoWorkloadDoc", err)
	}
}

// TestDiscoverWorkloadsRejectsWorkloadWithoutOwner covers the kubebuilder-style stub,
// whose empty owner would otherwise reach the control plane as project="" component="".
func TestDiscoverWorkloadsRejectsWorkloadWithoutOwner(t *testing.T) {
	root := writeTree(t, map[string]string{
		"components/stub.yaml":  ownerlessWorkloadYAML,
		"components/comp2.yaml": providerWorkloadYAML,
	})
	dir := filepath.Join(root, "components")

	d, err := discoverWorkloads([]string{dir})
	if err != nil {
		t.Fatalf("discoverWorkloads: %v", err)
	}
	if len(d.workloads) != 1 {
		t.Fatalf("got %d workloads, want just the owned one", len(d.workloads))
	}
	stub := filepath.Join(dir, "stub.yaml")
	if !strings.Contains(noteFor(d, stub), "spec.owner") {
		t.Errorf("stub was not recorded with a reason: %+v", d.skipped)
	}

	_, err = discoverWorkloads([]string{stub})
	if err == nil || !strings.Contains(err.Error(), "spec.owner") {
		t.Errorf("naming the stub: err = %v, want it to name spec.owner", err)
	}
}

// TestConnectDryRunNeedsNoResolver pins that --dry-run reaches no control plane: a nil
// resolver is enough to complete it.
func TestConnectDryRunNeedsNoResolver(t *testing.T) {
	root := writeTree(t, map[string]string{"components/comp2.yaml": providerWorkloadYAML})

	var out bytes.Buffer
	if err := New(nil).Connect(t.Context(), ConnectParams{
		WorkloadPaths: []string{filepath.Join(root, "components")},
		Namespace:     "default",
		DryRun:        true,
	}, &out); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	if !strings.Contains(out.String(), providerComponent) {
		t.Errorf("output missing the discovered workload:\n%s", out.String())
	}
}

// TestConnectDryRunPrintsTableBeforeReportingDuplicate covers the preview still
// explaining an expansion that collides.
func TestConnectDryRunPrintsTableBeforeReportingDuplicate(t *testing.T) {
	root := writeTree(t, map[string]string{
		"components/comp1.yaml": consumerWorkloadYAML,
		"components/copy.yaml":  consumerWorkloadYAML,
	})
	dir := filepath.Join(root, "components")

	var out bytes.Buffer
	err := New(nil).Connect(t.Context(), ConnectParams{
		WorkloadPaths: []string{dir},
		Namespace:     "default",
		DryRun:        true,
	}, &out)
	if err == nil || !strings.Contains(err.Error(), "duplicate workload") {
		t.Fatalf("err = %v, want a duplicate-workload error", err)
	}
	if want := "2 workloads from " + dir; !strings.Contains(out.String(), want) {
		t.Errorf("the table should precede the error, want %q:\n%s", want, out.String())
	}
}

func TestConnectReportsDuplicateWithinOneFile(t *testing.T) {
	path := writeWorkloadFileContent(t, "both.yaml", consumerWorkloadYAML+"---\n"+consumerWorkloadYAML)

	err := New(nil).Connect(t.Context(), ConnectParams{
		WorkloadPaths: []string{path},
		Namespace:     "default",
		DryRun:        true,
	}, io.Discard)
	if want := "duplicate workload for default/demo/comp1: " + path + " declares it twice"; err == nil || err.Error() != want {
		t.Errorf("err = %v, want %q", err, want)
	}
}

// TestDiscoverWorkloadsOrdersByArgumentThenPath pins the order the flat last-writer-wins
// env merge depends on.
func TestDiscoverWorkloadsOrdersByArgumentThenPath(t *testing.T) {
	root := writeTree(t, map[string]string{
		"second/b.yaml":     providerWorkloadYAML,
		"first/z.yaml":      consumerWorkloadYAML,
		"first/a/comp.yaml": otherNamespaceWorkloadYAML,
	})
	first := filepath.Join(root, "first")
	second := filepath.Join(root, "second")

	d, err := discoverWorkloads([]string{first, second})
	if err != nil {
		t.Fatalf("discoverWorkloads: %v", err)
	}
	want := []string{
		filepath.Join(first, "a", "comp.yaml"),
		filepath.Join(first, "z.yaml"),
		filepath.Join(second, "b.yaml"),
	}
	got := make([]string, 0, len(d.workloads))
	for _, w := range d.workloads {
		got = append(got, w.path)
	}
	if !slices.Equal(got, want) {
		t.Errorf("order = %v, want %v", got, want)
	}
}

func TestDiscoverWorkloadsDedupesSources(t *testing.T) {
	root := writeTree(t, map[string]string{"components/comp2.yaml": providerWorkloadYAML})
	dir := filepath.Join(root, "components")

	d, err := discoverWorkloads([]string{dir, dir})
	if err != nil {
		t.Fatalf("discoverWorkloads: %v", err)
	}
	if want := []string{dir}; !slices.Equal(d.sources, want) {
		t.Errorf("sources = %v, want %v", d.sources, want)
	}
}

// perComponentResolver fails the components named in errs and returns an empty success
// for the rest, so a partial failure can be driven per workload.
type perComponentResolver struct {
	errs  map[string]error
	calls []string
}

func (r *perComponentResolver) Resolve(_ context.Context, req remoteconnect.ResolveRequest) (*remoteconnect.ResolveResponse, error) {
	r.calls = append(r.calls, req.Component)
	if err, bad := r.errs[req.Component]; bad {
		return nil, err
	}
	return &remoteconnect.ResolveResponse{
		Resources: []remoteconnect.ResourceBindings{{
			Ref:       "some-db",
			StaticEnv: map[string]string{envPrefix(req.Component) + "_READY": "1"},
		}},
	}, nil
}

// envPrefix renders a component name as an env var prefix.
func envPrefix(component string) string {
	return strings.ToUpper(strings.ReplaceAll(component, "-", "_"))
}

// resourceWorkloadYAML declares a resource dependency, which is what makes a workload
// reach the control plane at all.
func resourceWorkloadYAML(component string) string {
	return `apiVersion: openchoreo.dev/v1alpha1
kind: Workload
metadata:
  name: ` + component + `
  namespace: default
spec:
  owner:
    projectName: demo
    componentName: ` + component + `
  dependencies:
    resources:
      - ref: some-db
        envBindings:
          host: ` + envPrefix(component) + `_DB_HOST
`
}

// TestConnectContinuesPastAnUnresolvableWorkload covers one undeployed component no
// longer taking down the whole session.
func TestConnectContinuesPastAnUnresolvableWorkload(t *testing.T) {
	root := writeTree(t, map[string]string{
		"components/a.yaml": resourceWorkloadYAML("comp-a"),
		"components/b.yaml": resourceWorkloadYAML("comp-b"),
		"components/c.yaml": resourceWorkloadYAML("comp-c"),
	})
	resolver := &perComponentResolver{errs: map[string]error{
		"comp-b": errors.New(`component "comp-b" has no release in environment "development"`),
	}}

	d := New(resolver)
	var gotEnv map[string]string
	d.runShell = func(_ context.Context, env []string) error {
		gotEnv = envToMap(env)
		return nil
	}

	var out bytes.Buffer
	if err := d.Connect(t.Context(), ConnectParams{
		WorkloadPaths: []string{filepath.Join(root, "components")},
		Namespace:     "default",
		Environment:   "development",
	}, &out); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	if want := []string{"comp-a", "comp-b", "comp-c"}; !slices.Equal(resolver.calls, want) {
		t.Errorf("resolve calls = %v, want %v (the failure must not stop the loop)", resolver.calls, want)
	}
	for _, want := range []string{"COMP_A_READY", "COMP_C_READY"} {
		if _, ok := gotEnv[want]; !ok {
			t.Errorf("subshell env missing %s; the healthy workloads should still bind", want)
		}
	}
	if _, ok := gotEnv["COMP_B_READY"]; ok {
		t.Error("the failed workload should contribute nothing")
	}

	got := out.String()
	for _, want := range []string{
		"! could not connect demo/comp-b (" + filepath.Join(root, "components", "b.yaml") + "):",
		"has no release in environment",
		"! 1 of 3 workloads could not be connected",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("output missing %q:\n%s", want, got)
		}
	}
}

func TestConnectErrorsWhenNoWorkloadResolves(t *testing.T) {
	root := writeTree(t, map[string]string{
		"components/a.yaml": resourceWorkloadYAML("comp-a"),
		"components/b.yaml": resourceWorkloadYAML("comp-b"),
	})
	boom := errors.New("control plane unreachable")
	resolver := &perComponentResolver{errs: map[string]error{"comp-a": boom, "comp-b": boom}}

	d := New(resolver)
	d.runShell = func(context.Context, []string) error {
		t.Fatal("no subshell should start when nothing resolved")
		return nil
	}

	err := d.Connect(t.Context(), ConnectParams{
		WorkloadPaths: []string{filepath.Join(root, "components")},
		Namespace:     "default",
		Environment:   "development",
	}, io.Discard)
	if err == nil {
		t.Fatal("Connect: want an error when every workload failed")
	}
	for _, want := range []string{"none of the 2 workloads could be connected", "demo/comp-a", boom.Error()} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("err = %q, missing %q", err, want)
		}
	}
}

// TestConnectSingleWorkloadFailureIsUnchanged pins that the classic one-file
// invocation still fails with the resolver's own error and no progress noise.
func TestConnectSingleWorkloadFailureIsUnchanged(t *testing.T) {
	path := writeWorkloadFileContent(t, "a.yaml", resourceWorkloadYAML("comp-a"))
	boom := errors.New("404 Not Found")
	d := New(&perComponentResolver{errs: map[string]error{"comp-a": boom}})
	d.runShell = func(context.Context, []string) error {
		t.Fatal("no subshell should start")
		return nil
	}

	var out bytes.Buffer
	err := d.Connect(t.Context(), ConnectParams{
		WorkloadPaths: []string{path},
		Namespace:     "default",
		Environment:   "development",
	}, &out)
	if !errors.Is(err, boom) {
		t.Errorf("err = %v, want the resolver's own error", err)
	}
	if strings.Contains(out.String(), "could not connect") {
		t.Errorf("a single workload needs no per-workload failure line:\n%s", out.String())
	}
}

// TestConnectKeepsLocalLinksWhenRemoteHalfFails covers a cross-link surviving its
// consumer's failed resolve, since a link needs no control plane.
func TestConnectKeepsLocalLinksWhenRemoteHalfFails(t *testing.T) {
	root := writeTree(t, map[string]string{
		"components/comp1.yaml": strings.Replace(consumerWorkloadYAML,
			"  dependencies:\n    endpoints:",
			"  dependencies:\n    resources:\n      - ref: some-db\n        envBindings:\n          host: DB_HOST\n    endpoints:", 1),
		"components/comp2.yaml": providerWorkloadYAML,
	})
	resolver := &perComponentResolver{errs: map[string]error{"comp1": errors.New("resolve exploded")}}

	d := New(resolver)
	var gotEnv map[string]string
	d.runShell = func(_ context.Context, env []string) error {
		gotEnv = envToMap(env)
		return nil
	}
	d.dialTunnel = func(context.Context, remoteconnect.AgentEndpoint, string) (tunnel, error) {
		t.Fatal("dialTunnel should not be called for a locally-linked dependency")
		return nil, nil
	}

	if err := d.Connect(t.Context(), ConnectParams{
		WorkloadPaths: []string{filepath.Join(root, "components")},
		Namespace:     "default",
		Environment:   "development",
	}, io.Discard); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	if got, want := gotEnv["COMP2_URL"], "http://127.0.0.1:9091"; got != want {
		t.Errorf("COMP2_URL = %q, want %q even though comp1's resolve failed", got, want)
	}
}

// TestConnectDryRunPreviewsLocalLinks covers the block that names the dependencies a
// directory expansion pulls away from the environment.
func TestConnectDryRunPreviewsLocalLinks(t *testing.T) {
	root := writeTree(t, map[string]string{
		"components/comp1.yaml": consumerWorkloadYAML,
		"components/comp2.yaml": providerWorkloadYAML,
	})
	dir := filepath.Join(root, "components")

	run := func(t *testing.T, overrides map[string]LocalTarget) string {
		t.Helper()
		var out bytes.Buffer
		if err := New(nil).Connect(t.Context(), ConnectParams{
			WorkloadPaths:  []string{dir},
			Namespace:      "default",
			LocalOverrides: overrides,
			DryRun:         true,
		}, &out); err != nil {
			t.Fatalf("Connect: %v", err)
		}
		return squeeze(out.String())
	}

	got := run(t, nil)
	for _, want := range []string{
		"1 dependency wired to your machine instead of the environment:",
		"FROM TO LOCAL ADDRESS BINDS",
		"comp1 demo/comp2/http 127.0.0.1:9091 COMP2_URL",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("output missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "would be tunneled") {
		t.Errorf("nothing here needs a tunnel:\n%s", got)
	}

	// --local moves the preview's address, so it matches what a real run would wire.
	if want := "comp1 demo/comp2/http 127.0.0.1:3000 COMP2_URL"; !strings.Contains(
		run(t, map[string]LocalTarget{"comp2": {Host: "127.0.0.1", Port: 3000}}), want) {
		t.Errorf("output missing %q", want)
	}
}

func TestConnectDryRunCountsTunneledDependencies(t *testing.T) {
	root := writeTree(t, map[string]string{"components/a.yaml": resourceWorkloadYAML("comp-a")})

	var out bytes.Buffer
	if err := New(nil).Connect(t.Context(), ConnectParams{
		WorkloadPaths: []string{filepath.Join(root, "components")},
		Namespace:     "default",
		DryRun:        true,
	}, &out); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	got := out.String()
	if want := "1 dependency would be tunneled from the environment."; !strings.Contains(got, want) {
		t.Errorf("output missing %q:\n%s", want, got)
	}
	if strings.Contains(got, "wired to your machine") {
		t.Errorf("there is no cross-link here:\n%s", got)
	}
}

const mixedDocsYAML = `apiVersion: openchoreo.dev/v1alpha1
kind: Workload
metadata:
  name: good
  namespace: default
spec:
  owner:
    projectName: demo
    componentName: good
---
apiVersion: openchoreo.dev/v1alpha1
kind: Workload
metadata:
  name: stub
  namespace: default
spec: {}
`

// TestDiscoverWorkloadsKeepsGoodSiblingDocuments covers one unusable document not
// taking its own file's usable documents down with it.
func TestDiscoverWorkloadsKeepsGoodSiblingDocuments(t *testing.T) {
	root := writeTree(t, map[string]string{"components/mixed.yaml": mixedDocsYAML})
	dir := filepath.Join(root, "components")
	mixed := filepath.Join(dir, "mixed.yaml")

	d, err := discoverWorkloads([]string{dir})
	if err != nil {
		t.Fatalf("discoverWorkloads: %v", err)
	}
	if len(d.workloads) != 1 || d.workloads[0].wl.Spec.Owner.ComponentName != "good" {
		t.Fatalf("got %+v, want the usable sibling", d.workloads)
	}
	if got := noteFor(d, mixed); !strings.Contains(got, `"stub"`) || !strings.Contains(got, "spec.owner") {
		t.Errorf("note = %q, want it to name the unusable document", got)
	}
}

func TestDiscoverWorkloadsReportsUnusableDocuments(t *testing.T) {
	const malformed = "kind: Workload\nmetadata:\n   name: bad\n  namespace: default\n"
	const templated = "apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: {{ .Values.name }}\n"
	const noAPIVersion = "kind: Workload\nmetadata:\n  name: noapi\nspec:\n  owner: {projectName: p, componentName: c}\n"
	const badVersion = "apiVersion: openchoreo.dev/v99\nkind: Workload\nmetadata:\n  name: future\nspec:\n  owner: {projectName: p, componentName: c}\n"

	tests := []struct {
		name, content, wantNote string
	}{
		{"malformed but means to be a Workload", malformed, "does not parse"},
		{"a template that is not a Workload", templated, ""},
		{"no apiVersion", noAPIVersion, "has no apiVersion"},
		{"unsupported apiVersion", badVersion, "unsupported apiVersion"},
		{"another ecosystem's Workload", foreignWorkloadYAML, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := writeTree(t, map[string]string{
				"components/comp2.yaml":  providerWorkloadYAML,
				"components/subject.yml": tt.content,
			})
			dir := filepath.Join(root, "components")

			d, err := discoverWorkloads([]string{dir})
			if err != nil {
				t.Fatalf("discoverWorkloads: %v", err)
			}
			if len(d.workloads) != 1 {
				t.Fatalf("got %d workloads, want only comp2", len(d.workloads))
			}
			got := noteFor(d, filepath.Join(dir, "subject.yml"))
			if tt.wantNote == "" {
				if got != "" {
					t.Errorf("note = %q, want silence for a document that is not ours", got)
				}
				return
			}
			if !strings.Contains(got, tt.wantNote) {
				t.Errorf("note = %q, want it to contain %q", got, tt.wantNote)
			}
		})
	}
}

// TestDiscoverWorkloadsNamesEachPathOnce covers a filesystem error not repeating the
// path the caller already prints.
func TestDiscoverWorkloadsNamesEachPathOnce(t *testing.T) {
	root := writeTree(t, map[string]string{"components/subject.yaml": "kind: Workload\nmetadata:\n   a: 1\n  b: 2\n"})
	subject := filepath.Join(root, "components", "subject.yaml")

	_, err := discoverWorkloads([]string{subject})
	if err == nil {
		t.Fatal("want an error for a malformed named file")
	}
	if n := strings.Count(err.Error(), subject); n != 1 {
		t.Errorf("path appears %d times in %q, want once", n, err)
	}
}

// TestConnectFailedWorkloadContributesNothing covers a workload that fails partway
// through its targets: its bindings must not reach the subshell, since the summary
// tells the developer they are missing.
func TestConnectFailedWorkloadContributesNothing(t *testing.T) {
	root := writeTree(t, map[string]string{
		"components/a.yaml": resourceWorkloadYAML("comp-a"),
		"components/b.yaml": resourceWorkloadYAML("comp-b"),
	})

	// comp-b's second target names an agent the response never defines, so the workload
	// fails only after its first target merged env and opened a listener.
	d := New(&brokenSecondTargetResolver{failFor: "comp-b"})
	d.dialTunnel = func(context.Context, remoteconnect.AgentEndpoint, string) (tunnel, error) {
		return &fakeTunnel{addr: "127.0.0.1:1"}, nil
	}
	var gotEnv map[string]string
	d.runShell = func(_ context.Context, env []string) error {
		gotEnv = envToMap(env)
		return nil
	}

	var out bytes.Buffer
	if err := d.Connect(t.Context(), ConnectParams{
		WorkloadPaths: []string{filepath.Join(root, "components")},
		Namespace:     "default",
		Environment:   "development",
	}, &out); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	if _, ok := gotEnv["COMP_A_FIRST"]; !ok {
		t.Error("the healthy workload should still bind")
	}
	for _, name := range []string{"COMP_B_FIRST", "COMP_B_READY"} {
		if got, ok := gotEnv[name]; ok {
			t.Errorf("%s = %q leaked from the failed workload", name, got)
		}
	}
	if !strings.Contains(out.String(), "! 1 of 2 workloads could not be connected") {
		t.Errorf("output missing the failure summary:\n%s", out.String())
	}
}

// brokenSecondTargetResolver answers with two targets, the second naming an undefined
// agent for the component in failFor.
type brokenSecondTargetResolver struct{ failFor string }

func (r *brokenSecondTargetResolver) Resolve(_ context.Context, req remoteconnect.ResolveRequest) (*remoteconnect.ResolveResponse, error) {
	const agent = "dp-agent"
	resp := &remoteconnect.ResolveResponse{
		Capability: "cap",
		Targets: []remoteconnect.ResolvedTarget{{
			Key:      "res/some-db/first",
			Proto:    "tcp",
			Resource: &remoteconnect.ResourceRender{Ref: "some-db", Address: "first", HostEnv: envPrefix(req.Component) + "_FIRST"},
			AgentID:  agent,
		}},
		Resources: []remoteconnect.ResourceBindings{{
			Ref:       "some-db",
			StaticEnv: map[string]string{envPrefix(req.Component) + "_READY": "1"},
		}},
		Agents: map[string]remoteconnect.AgentEndpoint{agent: {Endpoint: "router:8443"}},
	}
	if req.Component == r.failFor {
		resp.Targets = append(resp.Targets, remoteconnect.ResolvedTarget{
			Key:      "res/some-db/second",
			Proto:    "tcp",
			Resource: &remoteconnect.ResourceRender{Ref: "some-db", Address: "second", HostEnv: "UNREACHED"},
			AgentID:  "undefined-agent",
		})
	}
	return resp, nil
}

// TestConnectDryRunWarnsAboutBindingCollisions covers the preview naming env vars two
// workloads both claim, which the flat merge silently resolves by order.
func TestConnectDryRunWarnsAboutBindingCollisions(t *testing.T) {
	collide := func(component string) string {
		return `apiVersion: openchoreo.dev/v1alpha1
kind: Workload
metadata:
  name: ` + component + `
  namespace: default
spec:
  owner:
    projectName: demo
    componentName: ` + component + `
  dependencies:
    resources:
      - ref: db-` + component + `
        envBindings:
          host: SHARED_HOST
`
	}
	root := writeTree(t, map[string]string{
		"components/a.yaml": collide("comp-a"),
		"components/b.yaml": collide("comp-b"),
	})

	var out bytes.Buffer
	if err := New(nil).Connect(t.Context(), ConnectParams{
		WorkloadPaths: []string{filepath.Join(root, "components")},
		Namespace:     "default",
		DryRun:        true,
	}, &out); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	if want := "! SHARED_HOST is bound by 2 workloads (comp-a, comp-b); the last one wins"; !strings.Contains(out.String(), want) {
		t.Errorf("output missing %q:\n%s", want, out.String())
	}
}

// TestConnectNoDirectoryHeaderWhenDirectoryContributedNothing covers the header only
// appearing when a directory actually chose part of the set.
func TestConnectNoDirectoryHeaderWhenDirectoryContributedNothing(t *testing.T) {
	root := writeTree(t, map[string]string{
		"empty-dir/project.yaml": projectYAML,
		"comp2.yaml":             providerWorkloadYAML,
	})
	file := filepath.Join(root, "comp2.yaml")

	d := New(erroringResolver{t: t})
	d.runShell = func(context.Context, []string) error { return nil }
	var out bytes.Buffer
	if err := d.Connect(t.Context(), ConnectParams{
		WorkloadPaths: []string{filepath.Join(root, "empty-dir"), file},
		Namespace:     "default",
		Environment:   "development",
	}, &out); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	if strings.Contains(out.String(), "all treated as running locally") {
		t.Errorf("no directory chose the set, so no header belongs here:\n%s", out.String())
	}
}
