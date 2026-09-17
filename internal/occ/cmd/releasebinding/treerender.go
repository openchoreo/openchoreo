// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package releasebinding

import (
	"fmt"
	"io"
	"math"
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
	depth int
	kind  string
}

// row is one output line. Spanning rows (warnings, depth-cut markers) render
// their name across the whole width instead of participating in the columns.
type row struct {
	name      string
	health    string
	healthClr string
	age       string
	namespace string
	spanning  bool
	spanClr   string
}

func renderTree(w io.Writer, resp *gen.K8sResourceTreeResponse, opts renderOptions) {
	if len(resp.RenderedReleases) == 0 {
		fmt.Fprintln(w, "No rendered releases found")
		return
	}
	for i, rel := range resp.RenderedReleases {
		if i > 0 {
			fmt.Fprintln(w)
		}
		fmt.Fprintf(w, "Release: %s   (%s)\n\n", rel.Name, rel.TargetPlane)

		roots := buildForest(rel.Nodes)
		if renderForest(w, roots, opts) == 0 {
			fmt.Fprintln(w, "No resources found")
		}
	}
}

// renderForest walks the DAG and prints the table. It returns the number of
// node rows printed; zero means nothing was written at all. A node reached
// through a second parent renders as a reference line rather than being
// descended into again, unless the shorter path can show more of it than the
// earlier one did: an occurrence cut by --depth must not hide descendants that
// a later, shallower path reaches within the limit. Reaching a node that is on
// the current ancestor path always renders as a reference, which terminates the
// walk on ownership cycles.
func renderForest(w io.Writer, roots []*treeNode, opts renderOptions) int {
	var keep map[*treeNode]bool
	if opts.kind != "" {
		keep = kindMatchSet(roots, opts.kind)
	}
	multiNS := spansMultipleNamespaces(roots, keep)
	// expandedWith records the depth budget each node was last expanded with,
	// and onPath the nodes between the current node and its root.
	expandedWith := map[*treeNode]int{}
	onPath := map[*treeNode]bool{}
	var rows []row
	var walk func(tn *treeNode, depth int)
	walk = func(tn *treeNode, depth int) {
		if keep != nil && !keep[tn] {
			return
		}
		budget := remainingDepth(opts.depth, depth)
		prev, expanded := expandedWith[tn]
		if onPath[tn] || (expanded && prev >= budget) {
			r := nodeRow(tn, depth, multiNS)
			r.name += "  (shown above)"
			rows = append(rows, r)
			return
		}
		expandedWith[tn] = budget
		rows = append(rows, nodeRow(tn, depth, multiNS))
		rows = append(rows, warningRows(tn, depth)...)
		if opts.depth > 0 && depth+1 >= opts.depth {
			if n := countReachable(tn); n > 0 {
				rows = append(rows, row{
					name:     connectorPrefix(depth+1) + fmt.Sprintf("… (%d more %s, rerun without --depth)", n, resourceWord(n)),
					spanning: true,
				})
			}
			return
		}
		onPath[tn] = true
		for _, c := range tn.children {
			walk(c, depth+1)
		}
		delete(onPath, tn)
	}
	for _, r := range roots {
		walk(r, 0)
	}

	nodeCount := 0
	for _, r := range rows {
		if !r.spanning {
			nodeCount++
		}
	}
	if nodeCount == 0 {
		return 0
	}
	printRows(w, rows, multiNS, opts.color)
	return nodeCount
}

// remainingDepth is how many levels a node at this depth may still print,
// counting itself. An unlimited limit reports the largest budget there is, so
// every re-encounter of a node compares equal and collapses to a reference.
func remainingDepth(limit, depth int) int {
	if limit <= 0 {
		return math.MaxInt
	}
	return limit - depth
}

// resourceWord returns the noun that agrees with a resource count.
func resourceWord(n int) string {
	if n == 1 {
		return "resource"
	}
	return "resources"
}

// connectorPrefix indents a node line: roots have none, each deeper level adds
// three spaces before the branch connector.
func connectorPrefix(depth int) string {
	if depth == 0 {
		return ""
	}
	return strings.Repeat("   ", depth-1) + "└─ "
}

func nodeRow(tn *treeNode, depth int, multiNS bool) row {
	n := tn.node
	name := connectorPrefix(depth) + n.Kind + "/" + n.Name
	if n.MatchedBy != nil && *n.MatchedBy == "labelSelector" {
		name += "  [labels]"
	}
	r := row{name: name, health: "-", age: "-"}
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
// node. These are never dropped: their presence means children of that kind
// are incomplete, not absent. The kind is group-qualified, both because two
// groups can share a kind name and because an RBAC grant is per group; the
// spelling matches what --kind accepts. States other than forbidden display as
// error — the API leaves the state open and tells clients to treat unknown
// values as error.
func warningRows(tn *treeNode, depth int) []row {
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
			name:     strings.Repeat("   ", depth) + "   " + text,
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

// spansMultipleNamespaces reports whether the nodes live in more than one real
// namespace. Cluster-scoped nodes (no namespace) do not count toward the
// spread — a tree of one namespace plus cluster-scoped objects reads best
// without the extra column. Only nodes the --kind filter keeps are measured,
// so a filtered-out resource cannot add a column to a view that never shows
// it; a nil keep counts every node.
func spansMultipleNamespaces(roots []*treeNode, keep map[*treeNode]bool) bool {
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
	for _, r := range roots {
		walk(r)
	}
	return len(seen) > 1
}

func printRows(w io.Writer, rows []row, multiNS, color bool) {
	nameW := utf8.RuneCountInString("NAME")
	healthW := utf8.RuneCountInString("HEALTH")
	ageW := utf8.RuneCountInString("AGE")
	for _, r := range rows {
		if r.spanning {
			continue
		}
		nameW = max(nameW, utf8.RuneCountInString(r.name))
		healthW = max(healthW, utf8.RuneCountInString(r.health))
		ageW = max(ageW, utf8.RuneCountInString(r.age))
	}

	header := pad("NAME", nameW) + "  " + pad("HEALTH", healthW) + "  " + pad("AGE", ageW)
	if multiNS {
		header += "  NAMESPACE"
	}
	fmt.Fprintln(w, strings.TrimRight(header, " "))

	for _, r := range rows {
		if r.spanning {
			fmt.Fprintln(w, colorize(r.name, r.spanClr, color))
			continue
		}
		line := pad(r.name, nameW) + "  " + colorize(pad(r.health, healthW), r.healthClr, color) + "  " + pad(r.age, ageW)
		if multiNS {
			line += "  " + r.namespace
		}
		fmt.Fprintln(w, strings.TrimRight(line, " "))
	}
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

// hasForbidden reports whether any node's child discovery was blocked by
// missing permissions, which the follow-up hints turn into an RBAC pointer.
func hasForbidden(resp *gen.K8sResourceTreeResponse) bool {
	for _, rel := range resp.RenderedReleases {
		for _, n := range rel.Nodes {
			if n.ChildrenStatus == nil {
				continue
			}
			for _, cs := range *n.ChildrenStatus {
				if cs.State == "forbidden" {
					return true
				}
			}
		}
	}
	return false
}
