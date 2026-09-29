// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package releasebinding

import (
	"fmt"
	"io"
	"strings"
	"unicode/utf8"

	"github.com/openchoreo/openchoreo/internal/occ/cmd/utils"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
)

const (
	ansiGreen  = "\033[32m"
	ansiYellow = "\033[33m"
	ansiRed    = "\033[31m"
	ansiReset  = "\033[0m"
)

type renderOptions struct {
	color bool
	kind  string
}

// row is one output line. health, when set, follows the name in the NAME
// column rather than having a column of its own, since only Pods carry it.
// Spanning rows (warnings, empty-release markers) render their name across the
// whole width instead of participating in the columns.
type row struct {
	name      string
	health    string
	healthClr string
	age       string
	namespace string
	spanning  bool
	spanClr   string
}

// renderTree prints one table rooted at the release binding: each rendered
// release is a child of the binding, labeled with its target plane, and the
// resources it rendered hang under it. One table means the columns line up
// across releases.
func renderTree(w io.Writer, binding string, resp *gen.K8sResourceTreeResponse, opts renderOptions) {
	rows := []row{{name: "ReleaseBinding/" + binding, age: "-"}}
	if len(resp.RenderedReleases) == 0 {
		rows = append(rows, row{name: "   No rendered releases found", spanning: true})
		printRows(w, rows, false, opts.color)
		return
	}
	forests := make([][]*treeNode, len(resp.RenderedReleases))
	for i, rel := range resp.RenderedReleases {
		forests[i] = buildForest(rel.Nodes)
	}
	var keep map[*treeNode]bool
	if opts.kind != "" {
		keep = map[*treeNode]bool{}
		for _, roots := range forests {
			for n := range kindMatchSet(roots, opts.kind) {
				keep[n] = true
			}
		}
	}
	multiNS := spansMultipleNamespaces(forests, keep)
	for i, rel := range resp.RenderedReleases {
		last := i == len(resp.RenderedReleases)-1
		rows = append(rows, releaseRow(rel, branch(last), multiNS))
		under := guide(last)
		nodeRows := renderForest(forests[i], under, keep, multiNS)
		if len(nodeRows) == 0 {
			nodeRows = []row{{name: under + "   " + emptyRelease(len(rel.Nodes), opts), spanning: true}}
		}
		rows = append(rows, nodeRows...)
	}
	printRows(w, rows, multiNS, opts.color)
}

// branch is the connector in front of a node, guide the prefix its children
// inherit: a vertical bar while later siblings follow, blank space after the
// last one.
func branch(last bool) string {
	if last {
		return "└─ "
	}
	return "├─ "
}

func guide(last bool) string {
	if last {
		return "   "
	}
	return "│  "
}

func releaseRow(rel gen.ReleaseResourceTree, prefix string, multiNS bool) row {
	r := row{
		name: prefix + "RenderedRelease/" + rel.Name + "  (" + string(rel.TargetPlane) + ")",
		age:  "-",
	}
	if rel.RenderedRelease != nil && rel.RenderedRelease.Metadata.CreationTimestamp != nil {
		r.age = utils.FormatAge(*rel.RenderedRelease.Metadata.CreationTimestamp)
	}
	if multiNS {
		r.namespace = "-"
	}
	return r
}

// emptyRelease explains why a release has no rows under it. A release that
// rendered nothing says so; one whose resources were all hidden by --kind
// must not read the same way, so it names the filter and what it hid.
func emptyRelease(nodes int, opts renderOptions) string {
	if opts.kind == "" || nodes == 0 {
		return "No resources found"
	}
	return fmt.Sprintf("No %s resources (%d other %s, rerun without --kind)", opts.kind, nodes, resourceWord(nodes))
}

// renderForest walks one release's DAG and returns its rows, every name
// starting with prefix. keep is the --kind match set, nil when unfiltered.
// A node reached through a second parent renders as a reference line rather
// than being descended into again, and so does a node that is already on the
// current ancestor path, which terminates the walk on ownership cycles.
func renderForest(roots []*treeNode, prefix string, keep map[*treeNode]bool, multiNS bool) []row {
	expanded := map[*treeNode]bool{}
	onPath := map[*treeNode]bool{}
	var rows []row
	var walk func(tn *treeNode, prefix string, last bool)
	walk = func(tn *treeNode, prefix string, last bool) {
		if keep != nil && !keep[tn] {
			return
		}
		if onPath[tn] || expanded[tn] {
			r := nodeRow(tn, prefix+branch(last), multiNS)
			r.name += "  (shown above)"
			rows = append(rows, r)
			return
		}
		expanded[tn] = true
		rows = append(rows, nodeRow(tn, prefix+branch(last), multiNS))
		under := prefix + guide(last)
		rows = append(rows, warningRows(tn, under)...)
		onPath[tn] = true
		shown := visibleChildren(tn, keep)
		for i, c := range shown {
			walk(c, under, i == len(shown)-1)
		}
		delete(onPath, tn)
	}
	shown := visibleRoots(roots, keep)
	for i, r := range shown {
		walk(r, prefix, i == len(shown)-1)
	}
	return rows
}

// visibleChildren and visibleRoots drop the nodes --kind hides, so the last
// connector goes to the last node that is actually printed.
func visibleChildren(tn *treeNode, keep map[*treeNode]bool) []*treeNode {
	return visibleRoots(tn.children, keep)
}

func visibleRoots(nodes []*treeNode, keep map[*treeNode]bool) []*treeNode {
	if keep == nil {
		return nodes
	}
	var out []*treeNode
	for _, n := range nodes {
		if keep[n] {
			out = append(out, n)
		}
	}
	return out
}

// resourceWord returns the noun that agrees with a resource count.
func resourceWord(n int) string {
	if n == 1 {
		return "resource"
	}
	return "resources"
}

func nodeRow(tn *treeNode, prefix string, multiNS bool) row {
	n := tn.node
	name := prefix + n.Kind + "/" + n.Name
	if n.MatchedBy != nil && *n.MatchedBy == "labelSelector" {
		name += "  [labels]"
	}
	r := row{name: name, age: "-"}
	// Health is rendered only for Pods: the server reports health for other
	// kinds too, but pod health is the only signal trusted for display so far.
	if n.Health != nil && isCorePod(n) {
		glyph, clr := healthGlyph(n.Health.Status)
		r.health = glyph + " " + n.Health.Status
		r.healthClr = clr
	}
	if n.CreatedAt != nil {
		r.age = utils.FormatAge(*n.CreatedAt)
	}
	if multiNS {
		r.namespace = "-"
		if n.Namespace != nil && *n.Namespace != "" {
			r.namespace = *n.Namespace
		}
	}
	return r
}

func isCorePod(n gen.ResourceNode) bool {
	return n.Kind == "Pod" && (n.Group == nil || *n.Group == "")
}

func healthGlyph(status string) (glyph, colorCode string) {
	switch status {
	case "Healthy":
		return "●", ansiGreen
	case "Progressing", "Suspended":
		return "◌", ansiYellow
	case "Degraded", "Unhealthy":
		return "✖", ansiRed
	default:
		return "○", ""
	}
}

// warningRows renders each childrenStatus entry as a full-width line under its
// node, indented past the guide its children share. These are never dropped:
// their presence means children of that kind are incomplete, not absent. The
// kind is group-qualified, both because two groups can share a kind name and
// because an RBAC grant is per group; the spelling matches what --kind
// accepts. States other than forbidden display as error — the API leaves the
// state open and tells clients to treat unknown values as error.
func warningRows(tn *treeNode, under string) []row {
	if tn.node.ChildrenStatus == nil {
		return nil
	}
	var out []row
	for _, cs := range *tn.node.ChildrenStatus {
		state := cs.State
		if state != "forbidden" {
			state = "error"
		}
		text := "⚠ " + qualifiedKind(cs.Group, cs.Kind) + ": " + state
		if cs.Message != nil && *cs.Message != "" {
			text += " — " + *cs.Message
		}
		out = append(out, row{
			name:     under + "   " + text,
			spanning: true,
			spanClr:  ansiYellow,
		})
	}
	return out
}

// qualifiedKind spells a kind as group/Kind, or bare for the core group.
func qualifiedKind(group *string, kind string) string {
	if group == nil || *group == "" {
		return kind
	}
	return *group + "/" + kind
}

// spansMultipleNamespaces reports whether the nodes across every release live
// in more than one real namespace. The table spans all releases, so two
// single-namespace releases in different namespaces count as a spread.
// Cluster-scoped nodes (no namespace) do not count toward the
// spread — a tree of one namespace plus cluster-scoped objects reads best
// without the extra column. Only nodes the --kind filter keeps are measured,
// so a filtered-out resource cannot add a column to a view that never shows
// it; a nil keep counts every node.
func spansMultipleNamespaces(forests [][]*treeNode, keep map[*treeNode]bool) bool {
	seen := map[string]bool{}
	visited := map[*treeNode]bool{}
	var walk func(tn *treeNode)
	walk = func(tn *treeNode) {
		if visited[tn] || (keep != nil && !keep[tn]) {
			return
		}
		visited[tn] = true
		if tn.node.Namespace != nil && *tn.node.Namespace != "" {
			seen[*tn.node.Namespace] = true
		}
		for _, c := range tn.children {
			walk(c)
		}
	}
	for _, roots := range forests {
		for _, r := range roots {
			walk(r)
		}
	}
	return len(seen) > 1
}

func printRows(w io.Writer, rows []row, multiNS, color bool) {
	nameW := utf8.RuneCountInString("NAME")
	ageW := utf8.RuneCountInString("AGE")
	for _, r := range rows {
		if r.spanning {
			continue
		}
		nameW = max(nameW, utf8.RuneCountInString(r.nameWithHealth(false)))
		ageW = max(ageW, utf8.RuneCountInString(r.age))
	}

	header := pad("NAME", nameW) + "  " + pad("AGE", ageW)
	if multiNS {
		header += "  NAMESPACE"
	}
	fmt.Fprintln(w, strings.TrimRight(header, " "))

	for _, r := range rows {
		if r.spanning {
			fmt.Fprintln(w, colorize(r.name, r.spanClr, color))
			continue
		}
		// Padding is measured on the uncolored text so escape codes never
		// shift the columns.
		gap := strings.Repeat(" ", nameW-utf8.RuneCountInString(r.nameWithHealth(false)))
		line := r.nameWithHealth(color) + gap + "  " + pad(r.age, ageW)
		if multiNS {
			line += "  " + r.namespace
		}
		fmt.Fprintln(w, strings.TrimRight(line, " "))
	}
}

// nameWithHealth is the NAME cell: the name, then the health when there is one.
func (r row) nameWithHealth(color bool) string {
	if r.health == "" {
		return r.name
	}
	return r.name + "  " + colorize(r.health, r.healthClr, color)
}

func pad(s string, width int) string {
	if n := width - utf8.RuneCountInString(s); n > 0 {
		return s + strings.Repeat(" ", n)
	}
	return s
}

func colorize(s, code string, enabled bool) string {
	if !enabled || code == "" {
		return s
	}
	return code + s + ansiReset
}
