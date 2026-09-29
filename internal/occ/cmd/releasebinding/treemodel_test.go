// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package releasebinding

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
)

func testNode(uid, kind, name string, parentUIDs ...string) gen.ResourceNode {
	n := gen.ResourceNode{Uid: uid, Kind: kind, Name: name, Version: "v1"}
	if len(parentUIDs) > 0 {
		refs := make([]gen.ResourceRef, 0, len(parentUIDs))
		for _, p := range parentUIDs {
			refs = append(refs, gen.ResourceRef{Uid: p, Kind: "Parent", Name: "parent", Version: "v1"})
		}
		n.ParentRefs = &refs
	}
	return n
}

func TestBuildForest_LinksChildrenByUID(t *testing.T) {
	roots := buildForest([]gen.ResourceNode{
		testNode("d1", "Deployment", "checkout"),
		testNode("p1", "Pod", "checkout-abc", "d1"),
		testNode("p2", "Pod", "checkout-def", "d1"),
		testNode("s1", "Service", "checkout"),
	})
	require.Len(t, roots, 2)
	assert.Equal(t, "Deployment", roots[0].node.Kind)
	require.Len(t, roots[0].children, 2)
	assert.Equal(t, "checkout-abc", roots[0].children[0].node.Name)
	assert.Equal(t, "checkout-def", roots[0].children[1].node.Name)
	assert.Empty(t, roots[1].children)
}

func TestBuildForest_OrphanParentBecomesRoot(t *testing.T) {
	roots := buildForest([]gen.ResourceNode{
		testNode("p1", "Pod", "orphan", "missing-uid"),
	})
	require.Len(t, roots, 1)
	assert.Equal(t, "orphan", roots[0].node.Name)
}

func TestBuildForest_MultiParentAttachesUnderEveryKnownParent(t *testing.T) {
	roots := buildForest([]gen.ResourceNode{
		testNode("a", "ConfigMap", "owner-a"),
		testNode("b", "ConfigMap", "owner-b"),
		testNode("c", "Pod", "shared-child", "a", "b"),
	})
	require.Len(t, roots, 2)
	require.Len(t, roots[0].children, 1)
	require.Len(t, roots[1].children, 1)
	assert.Same(t, roots[0].children[0], roots[1].children[0], "one node object shared, not a copy")
	assert.Len(t, roots[0].children[0].parents, 2)
}

func TestBuildForest_SelfReferenceBecomesRoot(t *testing.T) {
	roots := buildForest([]gen.ResourceNode{
		testNode("x", "Widget", "self-ref", "x"),
	})
	require.Len(t, roots, 1)
	assert.Empty(t, roots[0].children)
}

func TestBuildForest_TwoNodeCyclePromotesDeterministicRoot(t *testing.T) {
	roots := buildForest([]gen.ResourceNode{
		testNode("a", "Widget", "first", "b"),
		testNode("b", "Widget", "second", "a"),
	})
	require.Len(t, roots, 1, "cycle members must not vanish")
	assert.Equal(t, "first", roots[0].node.Name, "promotion follows input order")
	require.Len(t, roots[0].children, 1)
	assert.Equal(t, "second", roots[0].children[0].node.Name)
}

func TestBuildForest_CycleReachableFromRootNotPromoted(t *testing.T) {
	roots := buildForest([]gen.ResourceNode{
		testNode("r", "Deployment", "root"),
		testNode("a", "Widget", "in-cycle-a", "r", "b"),
		testNode("b", "Widget", "in-cycle-b", "a"),
	})
	require.Len(t, roots, 1)
	assert.Equal(t, "root", roots[0].node.Name)
}

func TestKindMatchSet_KeepsAncestorsOfMatch(t *testing.T) {
	roots := buildForest([]gen.ResourceNode{
		testNode("d1", "Deployment", "checkout"),
		testNode("p1", "Pod", "checkout-abc", "d1"),
		testNode("s1", "Service", "checkout"),
	})
	keep := kindMatchSet(roots, "pod")
	assert.True(t, keep[roots[0]], "ancestor of a match stays")
	assert.True(t, keep[roots[0].children[0]])
	assert.False(t, keep[roots[1]])
}

func TestKindMatchSet_MatchKeepsItsSubtree(t *testing.T) {
	roots := buildForest([]gen.ResourceNode{
		testNode("d1", "Deployment", "checkout"),
		testNode("p1", "Pod", "checkout-abc", "d1"),
	})
	keep := kindMatchSet(roots, "Deployment")
	assert.True(t, keep[roots[0]])
	assert.True(t, keep[roots[0].children[0]], "children of a matched node stay visible")
}

func TestKindMatchSet_ChildrenStatusCountsAsMatch(t *testing.T) {
	blocked := testNode("h1", "HTTPRoute", "checkout")
	blocked.ChildrenStatus = &[]gen.ChildDiscoveryStatus{{Kind: "Pod", Version: "v1", State: "forbidden"}}
	roots := buildForest([]gen.ResourceNode{blocked, testNode("s1", "Service", "svc")})

	keep := kindMatchSet(roots, "Pod")
	assert.True(t, keep[roots[0]], "a node whose Pod children could not be discovered must survive --kind Pod")
	assert.False(t, keep[roots[1]])
}

func TestKindMatchSet_GroupQualified(t *testing.T) {
	withGroup := testNode("g1", "Gateway", "gw")
	group := "gateway.networking.k8s.io"
	withGroup.Group = &group
	roots := buildForest([]gen.ResourceNode{withGroup, testNode("s1", "Service", "svc")})

	keep := kindMatchSet(roots, "gateway.networking.k8s.io/Gateway")
	assert.True(t, keep[roots[0]])
	assert.False(t, keep[roots[1]])

	keepWrong := kindMatchSet(roots, "wrong.group/Gateway")
	assert.False(t, keepWrong[roots[0]])
}

// findNode locates a node by name anywhere in the forest, so a test can assert
// on a shared node without depending on which root it is reached through.
func findNode(t *testing.T, roots []*treeNode, name string) *treeNode {
	t.Helper()
	seen := map[*treeNode]bool{}
	var walk func(n *treeNode) *treeNode
	walk = func(n *treeNode) *treeNode {
		if seen[n] {
			return nil
		}
		seen[n] = true
		if n.node.Name == name {
			return n
		}
		for _, c := range n.children {
			if found := walk(c); found != nil {
				return found
			}
		}
		return nil
	}
	for _, r := range roots {
		if found := walk(r); found != nil {
			return found
		}
	}
	require.FailNowf(t, "node not found", "no node named %q in the forest", name)
	return nil
}

func TestKindMatchSet_SharedNodeSubtreeKeptInEitherRootOrder(t *testing.T) {
	plainRoot := testNode("r", "ConfigMap", "plain-root")
	matchingRoot := testNode("b", "Deployment", "matching-root")
	shared := testNode("x", "ReplicaSet", "shared", "r", "b")
	deployLeaf := testNode("d", "Deployment", "deploy-leaf", "x")
	serviceLeaf := testNode("s", "Service", "service-leaf", "x")
	unrelatedLeaf := testNode("u", "ConfigMap", "unrelated-leaf", "r")

	cases := []struct {
		name  string
		nodes []gen.ResourceNode
	}{
		{"plain root first", []gen.ResourceNode{plainRoot, matchingRoot, shared, deployLeaf, serviceLeaf, unrelatedLeaf}},
		{"matching root first", []gen.ResourceNode{matchingRoot, plainRoot, shared, deployLeaf, serviceLeaf, unrelatedLeaf}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			roots := buildForest(tc.nodes)
			require.Len(t, roots, 2)
			keep := kindMatchSet(roots, "Deployment")

			assert.True(t, keep[findNode(t, roots, "matching-root")], "the matching node stays")
			assert.True(t, keep[findNode(t, roots, "shared")], "a shared node under a match stays")
			assert.True(t, keep[findNode(t, roots, "deploy-leaf")], "the matching leaf stays")
			assert.True(t, keep[findNode(t, roots, "service-leaf")],
				"a non-matching node under a match stays whichever root reaches its parent first")
			assert.True(t, keep[findNode(t, roots, "plain-root")], "an ancestor of a match stays")
			assert.False(t, keep[findNode(t, roots, "unrelated-leaf")],
				"a leaf that is neither under a match nor an ancestor of one is filtered out")
		})
	}
}
