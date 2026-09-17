// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package releasebinding

import (
	"bytes"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openchoreo/openchoreo/internal/occ/cmd/utils"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
)

func ptr[T any](v T) *T { return &v }

func treeFixture() *gen.K8sResourceTreeResponse {
	deployment := testNode("d1", "Deployment", "checkout")
	deployment.Health = &gen.HealthInfo{Status: "Healthy"}

	pod1 := testNode("p1", "Pod", "checkout-abc", "d1")
	pod1.Health = &gen.HealthInfo{Status: "Healthy"}
	pod2 := testNode("p2", "Pod", "checkout-def", "d1")
	pod2.Health = &gen.HealthInfo{Status: "Progressing"}

	secret := testNode("s1", "Secret", "checkout-tls")

	route := testNode("h1", "HTTPRoute", "checkout")
	route.MatchedBy = ptr("labelSelector")
	route.ChildrenStatus = &[]gen.ChildDiscoveryStatus{{
		Kind:    "Pod",
		Version: "v1",
		State:   "forbidden",
		Message: ptr("cluster agent is not permitted to list pods"),
	}}

	return &gen.K8sResourceTreeResponse{
		RenderedReleases: []gen.ReleaseResourceTree{{
			Name:        "checkout-dev-a1b2c3",
			TargetPlane: "dataplane",
			Nodes:       []gen.ResourceNode{deployment, pod1, pod2, secret, route},
		}},
	}
}

func render(t *testing.T, resp *gen.K8sResourceTreeResponse, opts renderOptions) string {
	t.Helper()
	var buf bytes.Buffer
	renderTree(&buf, resp, opts)
	return buf.String()
}

func TestRenderTree_BasicLayout(t *testing.T) {
	out := render(t, treeFixture(), renderOptions{})

	assert.Contains(t, out, "Release: checkout-dev-a1b2c3   (dataplane)")
	lines := strings.Split(out, "\n")
	require.Greater(t, len(lines), 3)
	assert.Regexp(t, `^NAME\s+HEALTH\s+AGE$`, lines[2])
	assert.Contains(t, out, "Deployment/checkout")
	assert.Contains(t, out, "└─ Pod/checkout-abc")
	assert.Contains(t, out, "└─ Pod/checkout-def")
	assert.Contains(t, out, "● Healthy")
	assert.Contains(t, out, "◌ Progressing")
	assert.Regexp(t, `Deployment/checkout\s+-\s+-`, out,
		"non-Pod health is suppressed even when the server reports it")
	assert.NotContains(t, out, "\033[", "no ANSI codes when color is off")
	assert.NotContains(t, out, "NAMESPACE", "single-namespace tree has no namespace column")
}

func TestRenderTree_GoldenNoColor(t *testing.T) {
	deployment := testNode("d1", "Deployment", "checkout")
	deployment.Health = &gen.HealthInfo{Status: "Healthy"}
	pod := testNode("p1", "Pod", "checkout-abc", "d1")
	pod.Health = &gen.HealthInfo{Status: "Progressing"}
	resp := &gen.K8sResourceTreeResponse{
		RenderedReleases: []gen.ReleaseResourceTree{{
			Name:        "golden",
			TargetPlane: "dataplane",
			Nodes:       []gen.ResourceNode{deployment, pod},
		}},
	}
	want := strings.Join([]string{
		"Release: golden   (dataplane)",
		"",
		"NAME                 HEALTH         AGE",
		"Deployment/checkout  -              -",
		"└─ Pod/checkout-abc  ◌ Progressing  -",
		"",
	}, "\n")
	assert.Equal(t, want, render(t, resp, renderOptions{}))
}

var ansiPattern = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func TestRenderTree_ColorStripsToSameText(t *testing.T) {
	plain := render(t, treeFixture(), renderOptions{})
	colored := render(t, treeFixture(), renderOptions{color: true})
	assert.Contains(t, colored, "\033[32m", "healthy is green")
	assert.Contains(t, colored, "\033[33m", "progressing and warnings are yellow")
	assert.Equal(t, plain, ansiPattern.ReplaceAllString(colored, ""),
		"color must change styling only, never visible text or alignment")
}

func TestRenderTree_LabelSelectorBadge(t *testing.T) {
	out := render(t, treeFixture(), renderOptions{})
	assert.Contains(t, out, "HTTPRoute/checkout  [labels]")
}

func TestRenderTree_ChildrenStatusWarning(t *testing.T) {
	out := render(t, treeFixture(), renderOptions{})
	assert.Contains(t, out, "⚠ Pod: forbidden — cluster agent is not permitted to list pods")
}

func TestRenderTree_WarningWithoutMessage(t *testing.T) {
	resp := treeFixture()
	(*resp.RenderedReleases[0].Nodes[4].ChildrenStatus)[0].Message = nil
	out := render(t, resp, renderOptions{})
	assert.Contains(t, out, "⚠ Pod: forbidden")
	assert.NotContains(t, out, "forbidden —")
}

func TestRenderTree_WarningQualifiesKindWithItsGroup(t *testing.T) {
	resp := treeFixture()
	statuses := resp.RenderedReleases[0].Nodes[4].ChildrenStatus
	*statuses = []gen.ChildDiscoveryStatus{
		{Kind: "Widget", Group: ptr("alpha.example"), Version: "v1", State: "forbidden"},
		{Kind: "Widget", Group: ptr("beta.example"), Version: "v1", State: "forbidden"},
		{Kind: "Pod", Group: ptr(""), Version: "v1", State: "forbidden"},
	}
	out := render(t, resp, renderOptions{})
	assert.Contains(t, out, "⚠ alpha.example/Widget: forbidden")
	assert.Contains(t, out, "⚠ beta.example/Widget: forbidden",
		"two groups with the same kind must not render identical lines")
	assert.Contains(t, out, "⚠ Pod: forbidden", "core kinds stay bare")
}

func TestRenderTree_UnknownStateNormalizesToError(t *testing.T) {
	resp := treeFixture()
	(*resp.RenderedReleases[0].Nodes[4].ChildrenStatus)[0].State = "mystery"
	out := render(t, resp, renderOptions{})
	assert.Contains(t, out, "⚠ Pod: error")
	assert.NotContains(t, out, "mystery")
}

func TestRenderTree_SharedNodeRendersReferenceOnSecondOccurrence(t *testing.T) {
	ownerA := testNode("a", "ConfigMap", "owner-a")
	ownerB := testNode("b", "ConfigMap", "owner-b")
	shared := testNode("c", "Pod", "shared-child", "a", "b")
	resp := &gen.K8sResourceTreeResponse{
		RenderedReleases: []gen.ReleaseResourceTree{{
			Name:        "shared",
			TargetPlane: "dataplane",
			Nodes:       []gen.ResourceNode{ownerA, ownerB, shared},
		}},
	}
	out := render(t, resp, renderOptions{})
	assert.Equal(t, 2, strings.Count(out, "Pod/shared-child"), "once in full, once as reference")
	assert.Contains(t, out, "Pod/shared-child  (shown above)")
}

func TestRenderTree_DepthCutOnOnePathStillExpandsOnAShorterOne(t *testing.T) {
	ownerA := testNode("a", "ConfigMap", "owner-a")
	mid := testNode("x", "ConfigMap", "mid", "a")
	shared := testNode("s", "Service", "shared", "x", "b")
	leaf := testNode("p", "Pod", "leaf", "s")
	ownerB := testNode("b", "ConfigMap", "owner-b")
	resp := &gen.K8sResourceTreeResponse{
		RenderedReleases: []gen.ReleaseResourceTree{{
			Name:        "depth-shared",
			TargetPlane: "dataplane",
			Nodes:       []gen.ResourceNode{ownerA, mid, shared, leaf, ownerB},
		}},
	}
	out := render(t, resp, renderOptions{depth: 3})
	assert.Contains(t, out, "Pod/leaf", "the leaf is within --depth 3 through owner-b")
	assert.Equal(t, 2, strings.Count(out, "Service/shared"),
		"the cut occurrence stays and the shorter path expands the node again")
	assert.NotContains(t, out, "Service/shared  (shown above)",
		"a node whose first occurrence was cut must not collapse to a reference")
	assert.Contains(t, out, "… (1 more resource, rerun without --depth)",
		"the deeper path keeps its cut marker")
}

func TestRenderTree_UnlimitedDepthExpandsASharedSubtreeOnce(t *testing.T) {
	ownerA := testNode("a", "ConfigMap", "owner-a")
	ownerB := testNode("b", "ConfigMap", "owner-b")
	shared := testNode("s", "Service", "shared", "a", "b")
	leaf := testNode("p", "Pod", "leaf", "s")
	resp := &gen.K8sResourceTreeResponse{
		RenderedReleases: []gen.ReleaseResourceTree{{
			Name:        "unlimited-shared",
			TargetPlane: "dataplane",
			Nodes:       []gen.ResourceNode{ownerA, ownerB, shared, leaf},
		}},
	}
	out := render(t, resp, renderOptions{})
	assert.Equal(t, 2, strings.Count(out, "Service/shared"), "once in full, once as reference")
	assert.Equal(t, 1, strings.Count(out, "Service/shared  (shown above)"))
	assert.Equal(t, 1, strings.Count(out, "Pod/leaf"), "the shared subtree expands only once")
}

func TestRenderTree_CycleTerminatesWithAReference(t *testing.T) {
	root := testNode("r", "ConfigMap", "root")
	inCycleA := testNode("a", "Widget", "in-cycle-a", "r", "b")
	inCycleB := testNode("b", "Widget", "in-cycle-b", "a")
	resp := &gen.K8sResourceTreeResponse{
		RenderedReleases: []gen.ReleaseResourceTree{{
			Name:        "cycle",
			TargetPlane: "dataplane",
			Nodes:       []gen.ResourceNode{root, inCycleA, inCycleB},
		}},
	}
	out := render(t, resp, renderOptions{})
	assert.Equal(t, 2, strings.Count(out, "Widget/in-cycle-a"), "once in full, once as the cycle reference")
	assert.Equal(t, 1, strings.Count(out, "Widget/in-cycle-b"))
	assert.Contains(t, out, "Widget/in-cycle-a  (shown above)")
}

func TestRenderTree_DepthPrunedMarker(t *testing.T) {
	out := render(t, treeFixture(), renderOptions{depth: 1})
	assert.Contains(t, out, "… (2 more resources, rerun without --depth)")
	assert.NotContains(t, out, "Pod/checkout-abc")
}

func TestRenderTree_DepthPrunedMarkerSingular(t *testing.T) {
	deployment := testNode("d1", "Deployment", "checkout")
	pod := testNode("p1", "Pod", "checkout-abc", "d1")
	resp := &gen.K8sResourceTreeResponse{
		RenderedReleases: []gen.ReleaseResourceTree{{
			Name:        "singular",
			TargetPlane: "dataplane",
			Nodes:       []gen.ResourceNode{deployment, pod},
		}},
	}
	out := render(t, resp, renderOptions{depth: 1})
	assert.Contains(t, out, "… (1 more resource, rerun without --depth)")
	assert.NotContains(t, out, "1 more resources")
	assert.NotContains(t, out, "Pod/checkout-abc")
}

func TestRenderTree_KindFilter(t *testing.T) {
	out := render(t, treeFixture(), renderOptions{kind: "Pod"})
	assert.Contains(t, out, "Deployment/checkout", "ancestor of a match stays visible")
	assert.Contains(t, out, "Pod/checkout-abc")
	assert.Contains(t, out, "HTTPRoute/checkout", "node with undiscoverable Pod children stays visible")
	assert.Contains(t, out, "⚠ Pod: forbidden", "its warning stays with it")
	assert.NotContains(t, out, "Secret/checkout-tls")
}

func TestRenderTree_KindFilterNoMatches(t *testing.T) {
	out := render(t, treeFixture(), renderOptions{kind: "DaemonSet"})
	assert.Contains(t, out, "No resources found")
}

func TestRenderTree_NamespaceColumnNeedsTwoRealNamespaces(t *testing.T) {
	resp := treeFixture()
	nodes := resp.RenderedReleases[0].Nodes
	nodes[0].Namespace = ptr("dp-default")
	out := render(t, resp, renderOptions{})
	assert.NotContains(t, out, "NAMESPACE",
		"one real namespace plus cluster-scoped nodes must not trigger the column")

	nodes[3].Namespace = ptr("envoy-gateway-system")
	out = render(t, resp, renderOptions{})
	assert.Contains(t, out, "NAMESPACE")
	assert.Contains(t, out, "envoy-gateway-system")
}

func TestRenderTree_NamespaceColumnIgnoresKindFilteredNodes(t *testing.T) {
	resp := treeFixture()
	nodes := resp.RenderedReleases[0].Nodes
	nodes[0].Namespace = ptr("dp-default")
	nodes[3].Namespace = ptr("envoy-gateway-system")
	out := render(t, resp, renderOptions{kind: "Pod"})
	assert.NotContains(t, out, "NAMESPACE",
		"a second namespace seen only on a filtered-out node must not trigger the column")

	nodes[2].Namespace = ptr("dp-other")
	out = render(t, resp, renderOptions{kind: "Pod"})
	assert.Contains(t, out, "NAMESPACE", "two namespaces among kept nodes still trigger it")
	assert.Contains(t, out, "dp-other")
}

func TestRenderTree_AgeFromCreatedAt(t *testing.T) {
	created := time.Now().Add(-48 * time.Hour)
	resp := treeFixture()
	resp.RenderedReleases[0].Nodes[0].CreatedAt = ptr(created)
	out := render(t, resp, renderOptions{})
	assert.Contains(t, out, utils.FormatAge(created))
}

func TestRenderTree_NoRenderedReleases(t *testing.T) {
	out := render(t, &gen.K8sResourceTreeResponse{}, renderOptions{})
	assert.Equal(t, "No rendered releases found\n", out)
}

func TestRenderTree_MultipleReleases(t *testing.T) {
	resp := treeFixture()
	resp.RenderedReleases = append(resp.RenderedReleases, gen.ReleaseResourceTree{
		Name:        "checkout-dev-obs",
		TargetPlane: "observabilityplane",
	})
	out := render(t, resp, renderOptions{})
	assert.Contains(t, out, "Release: checkout-dev-a1b2c3   (dataplane)")
	assert.Contains(t, out, "Release: checkout-dev-obs   (observabilityplane)")
	assert.Contains(t, out, "No resources found")
}
