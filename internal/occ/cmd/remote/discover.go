// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package remote

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/remoteconnect"
	"github.com/openchoreo/openchoreo/pkg/fsindex/scanner"
)

// errNoWorkloadDoc marks a YAML file with no Workload document: an error for a file the
// developer named, a skip for one a directory walk turned up.
var errNoWorkloadDoc = errors.New("no Workload document found")

// discovered is one Workload document and the file it came from.
type discovered struct {
	path string
	wl   *v1alpha1.Workload
	id   workloadIdentity // set by the caller, which resolves the effective namespace
}

// skippedPath is a path a directory walk could not use.
type skippedPath struct {
	path   string
	reason string
}

// discovery is what the command's path arguments expanded to.
type discovery struct {
	workloads  []discovered
	fromDir    bool     // whether any argument was a directory
	sources    []string // path arguments that contributed a Workload to the set
	sourceKeys map[string]bool
	emptyDirs  []string      // directory arguments holding no Workload document
	skipped    []skippedPath // paths a walk could not read or parse
}

func (d *discovery) note(path, reason string) {
	d.skipped = append(d.skipped, skippedPath{path: path, reason: reason})
}

func (d *discovery) addSource(path string) {
	key := fileKey(path)
	if d.sourceKeys == nil {
		d.sourceKeys = map[string]bool{}
	}
	if d.sourceKeys[key] {
		return
	}
	d.sourceKeys[key] = true
	d.sources = append(d.sources, path)
}

func (d *discovery) noteProblems(path string, problems []string) {
	for _, problem := range problems {
		d.note(path, problem)
	}
}

func (d *discovery) addEmptyDir(path string) {
	if !slices.Contains(d.emptyDirs, path) {
		d.emptyDirs = append(d.emptyDirs, path)
	}
}

// discoverWorkloads expands paths into the Workload documents they name: a file
// contributes every Workload document it holds, a directory every one beneath it. A
// file the developer named must yield a Workload; inside a directory an unusable file
// is recorded and the walk continues.
func discoverWorkloads(paths []string) (discovery, error) {
	var d discovery
	seen := make(map[string]bool, len(paths))
	for _, path := range paths {
		info, err := os.Stat(path)
		if err != nil {
			if os.IsNotExist(err) {
				return discovery{}, fmt.Errorf("path %s does not exist", path)
			}
			return discovery{}, fmt.Errorf("access path %s: %w", path, err)
		}

		if !info.IsDir() {
			lf, err := loadWorkloadsFromFile(path)
			if err != nil {
				return discovery{}, fmt.Errorf("%s: %w", path, err)
			}
			d.noteProblems(path, lf.problems)
			if d.appendUnseen(seen, path, lf.workloads) {
				d.addSource(path)
			}
			continue
		}

		withWorkloads := 0
		added := false
		for _, file := range walkYAMLFiles(path, &d) {
			lf, err := loadWorkloadsFromFile(file)
			if err != nil {
				if !errors.Is(err, errNoWorkloadDoc) {
					d.note(file, err.Error())
				}
				continue
			}
			d.noteProblems(file, lf.problems)
			withWorkloads++
			if d.appendUnseen(seen, file, lf.workloads) {
				added = true
			}
		}
		if withWorkloads == 0 {
			d.addEmptyDir(path)
			continue
		}
		if added {
			// Only a directory that chose part of the set makes it differ from what the
			// developer typed.
			d.fromDir = true
			d.addSource(path)
		}
	}
	return d, nil
}

// appendUnseen appends path's workloads unless an earlier argument already named the
// same file, and reports whether it appended any.
func (d *discovery) appendUnseen(seen map[string]bool, path string, wls []*v1alpha1.Workload) bool {
	key := fileKey(path)
	if seen[key] || len(wls) == 0 {
		return false
	}
	seen[key] = true
	for _, wl := range wls {
		d.workloads = append(d.workloads, discovered{path: path, wl: wl})
	}
	return true
}

// reasonOnly strips the path a filesystem error repeats, since every caller prints the
// path itself.
func reasonOnly(err error) error {
	var pe *os.PathError
	if errors.As(err, &pe) {
		return fmt.Errorf("cannot %s: %w", pe.Op, pe.Err)
	}
	return err
}

// fileKey identifies a file by its resolved absolute path, so two names for one file
// are the same file.
func fileKey(path string) string {
	resolved := path
	if real, err := filepath.EvalSymlinks(path); err == nil {
		resolved = real
	}
	if abs, err := filepath.Abs(resolved); err == nil {
		return abs
	}
	return resolved
}

// walkYAMLFiles returns every regular YAML file under root in lexical order, skipping
// the directories the repo's GitOps scanner excludes. A symlinked root is followed
// because the developer named it, but a symlinked directory below it may not lead
// outside that root. A directory reached twice is walked once, so a cycle terminates.
// Paths it cannot read are recorded in d instead of failing the walk.
func walkYAMLFiles(root string, d *discovery) []string {
	filter := scanner.DefaultFilter()
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		d.note(root, reasonOnly(err).Error())
		return nil
	}
	visited := map[string]bool{}
	var files []string

	var walk func(dir, realDir string)
	walk = func(dir, realDir string) {
		if visited[realDir] {
			return
		}
		visited[realDir] = true

		entries, err := os.ReadDir(dir)
		if err != nil {
			d.note(dir, reasonOnly(err).Error())
			return
		}
		for _, entry := range entries {
			path := filepath.Join(dir, entry.Name())
			mode := entry.Type()
			linked := mode&fs.ModeSymlink != 0
			if linked {
				info, serr := os.Stat(path)
				if serr != nil {
					if scanner.IsYAMLFile(path) {
						d.note(path, reasonOnly(serr).Error())
					}
					continue
				}
				mode = info.Mode().Type()
			}
			switch {
			case mode.IsDir():
				if !filter.ShouldDescendIntoDir(entry.Name()) {
					continue
				}
				realChild := filepath.Join(realDir, entry.Name())
				if linked {
					if realChild, err = filepath.EvalSymlinks(path); err != nil {
						d.note(path, err.Error())
						continue
					}
					if !within(realRoot, realChild) {
						d.note(path, "directory symlink leads outside "+root)
						continue
					}
				}
				walk(path, realChild)
			case mode.IsRegular():
				if scanner.IsYAMLFile(path) {
					files = append(files, path)
				}
			default:
				// A FIFO or device named *.yaml would block or never end on read.
				if scanner.IsYAMLFile(path) {
					d.note(path, "not a regular file")
				}
			}
		}
	}
	walk(root, realRoot)
	slices.Sort(files)
	return files
}

// within reports whether path is root or sits beneath it.
func within(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}

// reportDiscovery prints what the expansion could not use, and reports whether it
// found anything at all.
func reportDiscovery(out io.Writer, d discovery) error {
	for _, s := range d.skipped {
		fmt.Fprintf(out, "! skipped %s: %s\n", s.path, s.reason)
	}
	if len(d.workloads) == 0 {
		return fmt.Errorf("no Workload documents found under %s", strings.Join(d.emptyDirs, ", "))
	}
	for _, dir := range d.emptyDirs {
		fmt.Fprintf(out, "! no Workload documents found under %s\n", dir)
	}
	return nil
}

// printDiscovered reports what sources expanded to, for --dry-run. A namespace every
// workload shares is stated once in the summary instead of repeated per row.
func printDiscovered(out io.Writer, sources []string, found []discovered) {
	namespace, shared := commonNamespace(found)
	summary := fmt.Sprintf("%s from %s", countWorkloads(len(found)), strings.Join(sources, ", "))
	if shared {
		summary += fmt.Sprintf(" (namespace %s)", namespace)
	}
	fmt.Fprintf(out, "%s:\n", summary)

	heads := []string{"PATH", "PROJECT", "COMPONENT"}
	cells := func(f discovered) []string { return []string{f.path, f.id.project, f.id.component} }
	if !shared {
		heads = []string{"PATH", "NAMESPACE", "PROJECT", "COMPONENT"}
		cells = func(f discovered) []string {
			return []string{f.path, f.id.namespace, f.id.project, f.id.component}
		}
	}

	w := tabwriter.NewWriter(out, 0, 0, 3, ' ', 0)
	fmt.Fprintf(w, "  %s\n", strings.Join(heads, "\t"))
	for _, f := range found {
		fmt.Fprintf(w, "  %s\n", strings.Join(cells(f), "\t"))
	}
	_ = w.Flush()
}

// printPlannedLinks reports, for --dry-run, the dependencies the discovered set wires
// to this machine and how many would be tunneled instead.
func printPlannedLinks(out io.Writer, found []discovered,
	byIdentity map[workloadIdentity]*v1alpha1.Workload, overrides map[string]LocalTarget) {
	rows := make([][]string, 0, len(found))
	tunneled := 0
	for _, f := range found {
		remoteEndpoints, links := splitDependencies(f.wl, f.id.namespace, byIdentity)
		tunneled += len(remoteEndpoints)
		if f.wl.Spec.Dependencies != nil {
			tunneled += len(f.wl.Spec.Dependencies.Resources)
		}
		for _, link := range links {
			host, port := link.target(overrides)
			rows = append(rows, []string{
				f.id.component,
				strings.TrimPrefix(link.key, "ep/"),
				net.JoinHostPort(host, strconv.Itoa(port)),
				strings.Join(bindingNames(link.envBindings), ", "),
			})
		}
	}

	if len(rows) > 0 {
		fmt.Fprintf(out, "\n%s wired to your machine instead of the environment:\n", countDependencies(len(rows)))
		w := tabwriter.NewWriter(out, 0, 0, 3, ' ', 0)
		fmt.Fprintln(w, "  FROM\tTO\tLOCAL ADDRESS\tBINDS")
		for _, r := range rows {
			fmt.Fprintf(w, "  %s\n", strings.Join(r, "\t"))
		}
		_ = w.Flush()
	}
	if tunneled > 0 {
		fmt.Fprintf(out, "\n%s would be tunneled from the environment.\n", countDependencies(tunneled))
	}
}

// printBindingCollisions warns, for --dry-run, about env vars more than one workload
// binds. The bindings merge into one flat map where the last writer wins, so a
// collision silently decides which upstream an app reaches.
func printBindingCollisions(out io.Writer, found []discovered) {
	owners := map[string][]string{}
	for _, f := range found {
		for _, name := range declaredEnvNames(f.wl) {
			if !slices.Contains(owners[name], f.id.component) {
				owners[name] = append(owners[name], f.id.component)
			}
		}
	}
	for _, name := range sortedKeys(owners) {
		if len(owners[name]) > 1 {
			fmt.Fprintf(out, "! %s is bound by %d workloads (%s); the last one wins\n",
				name, len(owners[name]), strings.Join(owners[name], ", "))
		}
	}
}

// declaredEnvNames lists every env var a workload's dependencies bind.
func declaredEnvNames(wl *v1alpha1.Workload) []string {
	if wl.Spec.Dependencies == nil {
		return nil
	}
	var names []string
	for _, ep := range wl.Spec.Dependencies.Endpoints {
		for _, name := range []string{ep.EnvBindings.Address, ep.EnvBindings.Host,
			ep.EnvBindings.Port, ep.EnvBindings.BasePath} {
			if name != "" {
				names = append(names, name)
			}
		}
	}
	for _, res := range wl.Spec.Dependencies.Resources {
		for _, name := range res.EnvBindings {
			if name != "" {
				names = append(names, name)
			}
		}
	}
	return names
}

// bindingNames lists the env vars an endpoint's bindings name, in a stable order.
func bindingNames(b remoteconnect.EndpointEnvBindings) []string {
	var names []string
	for _, name := range []string{b.Address, b.Host, b.Port, b.BasePath} {
		if name != "" {
			names = append(names, name)
		}
	}
	return names
}

func countDependencies(n int) string {
	if n == 1 {
		return "1 dependency"
	}
	return fmt.Sprintf("%d dependencies", n)
}

// commonNamespace returns the namespace every discovered workload shares, if they all
// share one.
func commonNamespace(found []discovered) (string, bool) {
	if len(found) == 0 {
		return "", false
	}
	namespace := found[0].id.namespace
	for _, f := range found[1:] {
		if f.id.namespace != namespace {
			return "", false
		}
	}
	return namespace, true
}

func countWorkloads(n int) string {
	if n == 1 {
		return "1 workload"
	}
	return fmt.Sprintf("%d workloads", n)
}
