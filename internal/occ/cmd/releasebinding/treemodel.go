// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package releasebinding

import (
	"strings"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
)

// treeNode is one resource in the assembled structure. The server deduplicates
// nodes release-wide and merges parents, so this is a DAG, not a tree: a node
// shared by several parents appears in each parent's children and lists all of
// them in parents. The renderer decides how repeat occurrences are shown.
type treeNode struct {
	node     gen.ResourceNode
	children []*treeNode
	parents  []*treeNode
}

// buildForest links flat resource nodes into a DAG by UID and returns its
// roots. A node whose parent references are all absent from the set becomes a
// root, so an orphaned reference surfaces rather than dropping the node. Nodes
// trapped in a parent cycle that no root reaches are promoted to roots in
// input order, so a malformed ownership loop cannot silently swallow part of
// the tree. Input order is preserved for roots and siblings.
func buildForest(nodes []gen.ResourceNode) []*treeNode {
	byUID := make(map[string]*treeNode, len(nodes))
	order := make([]*treeNode, 0, len(nodes))
	for i := range nodes {
		tn := &treeNode{node: nodes[i]}
		byUID[nodes[i].Uid] = tn
		order = append(order, tn)
	}
	for _, tn := range order {
		if tn.node.ParentRefs == nil {
			continue
		}
		for _, ref := range *tn.node.ParentRefs {
			if p, ok := byUID[ref.Uid]; ok && p != tn {
				p.children = append(p.children, tn)
				tn.parents = append(tn.parents, p)
			}
		}
	}
	var roots []*treeNode
	for _, tn := range order {
		if len(tn.parents) == 0 {
			roots = append(roots, tn)
		}
	}
	reached := make(map[*treeNode]bool, len(order))
	var mark func(tn *treeNode)
	mark = func(tn *treeNode) {
		if reached[tn] {
			return
		}
		reached[tn] = true
		for _, c := range tn.children {
			mark(c)
		}
	}
	for _, r := range roots {
		mark(r)
	}
	for _, tn := range order {
		if !reached[tn] {
			roots = append(roots, tn)
			mark(tn)
		}
	}
	return roots
}

// kindMatchSet returns the nodes a --kind filter keeps: nodes of the kind,
// nodes whose childrenStatus reports that kind (children of the kind exist but
// could not be discovered — they must not disappear from a filtered view),
// every ancestor of either, and the full subtree under any matching node. The
// ancestor walk and the subtree expansion are separate passes over their own
// bookkeeping: a node kept only because a descendant matched must not stop the
// expansion of a matched subtree it is also shared into, which would drop its
// other children depending on the order the roots are visited in. The walk is
// cycle-safe; a match seen only through a cycle back-edge may be missed on the
// ancestor side of the cycle, which is acceptable for a pathological ownership
// loop.
func kindMatchSet(roots []*treeNode, kindArg string) map[*treeNode]bool {
	group, kind := splitKindArg(kindArg)
	keep := map[*treeNode]bool{}
	var matches []*treeNode
	const (
		visiting = 1
		done     = 2
	)
	state := map[*treeNode]int{}
	var visit func(tn *treeNode) bool
	visit = func(tn *treeNode) bool {
		switch state[tn] {
		case visiting:
			return false
		case done:
			return keep[tn]
		}
		state[tn] = visiting
		match := matchesKind(tn.node, group, kind) || statusReportsKind(tn.node, group, kind)
		if match {
			matches = append(matches, tn)
		}
		for _, c := range tn.children {
			if visit(c) {
				match = true
			}
		}
		state[tn] = done
		if match {
			keep[tn] = true
		}
		return match
	}
	for _, r := range roots {
		visit(r)
	}
	underMatch := map[*treeNode]bool{}
	for _, m := range matches {
		markSubtree(m, underMatch)
	}
	for tn := range underMatch {
		keep[tn] = true
	}
	return keep
}

// markSubtree marks tn and everything below it. It stops on already-marked
// nodes, so a cycle inside a matched subtree terminates.
func markSubtree(tn *treeNode, marked map[*treeNode]bool) {
	if marked[tn] {
		return
	}
	marked[tn] = true
	for _, c := range tn.children {
		markSubtree(c, marked)
	}
}

// splitKindArg splits a --kind argument into its optional group and its kind.
func splitKindArg(arg string) (group, kind string) {
	if i := strings.LastIndex(arg, "/"); i >= 0 {
		return arg[:i], arg[i+1:]
	}
	return "", arg
}

func matchesKind(n gen.ResourceNode, group, kind string) bool {
	if !strings.EqualFold(n.Kind, kind) {
		return false
	}
	if group == "" {
		return true
	}
	nodeGroup := ""
	if n.Group != nil {
		nodeGroup = *n.Group
	}
	return strings.EqualFold(nodeGroup, group)
}

// statusReportsKind reports whether the node carries a child-discovery failure
// for the filtered kind. Such a node must survive the filter: the failure means
// children of that kind are incomplete, not absent.
func statusReportsKind(n gen.ResourceNode, group, kind string) bool {
	if n.ChildrenStatus == nil {
		return false
	}
	for _, cs := range *n.ChildrenStatus {
		if !strings.EqualFold(cs.Kind, kind) {
			continue
		}
		if group == "" {
			return true
		}
		csGroup := ""
		if cs.Group != nil {
			csGroup = *cs.Group
		}
		if strings.EqualFold(csGroup, group) {
			return true
		}
	}
	return false
}
