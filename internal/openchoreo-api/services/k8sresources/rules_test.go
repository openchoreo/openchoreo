// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package k8sresources

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/config"
	"github.com/openchoreo/openchoreo/internal/resourcetree/protocol"
)

// rootRule returns the single compiled rule for a root group+kind, failing the
// test if the root is missing or ambiguous.
func rootRule(t *testing.T, rules *compiledRules, group, kind string) *compiledRule {
	t.Helper()
	got := rules.roots[groupKind{Group: group, Kind: kind}]
	require.Len(t, got, 1, "expected exactly one rule for root %s/%s", group, kind)
	return got[0]
}

// childByKind returns the single child edge for a kind among siblings.
func childByKind(t *testing.T, children []*compiledChild, kind string) *compiledChild {
	t.Helper()
	var found []*compiledChild
	for _, child := range children {
		if child.Kind.Kind == kind {
			found = append(found, child)
		}
	}
	require.Len(t, found, 1, "expected exactly one %s child", kind)
	return found[0]
}

// walkChildren visits every compiled edge depth-first.
func walkChildren(children []*compiledChild, visit func(*compiledChild)) {
	for _, child := range children {
		visit(child)
		walkChildren(child.Children, visit)
	}
}

// sortGroupKinds returns a copy sorted by group then kind, so map values can be
// compared without depending on insertion order.
func sortGroupKinds(in []groupKind) []groupKind {
	out := slices.Clone(in)
	slices.SortFunc(out, func(a, b groupKind) int {
		if a.Group != b.Group {
			return strings.Compare(a.Group, b.Group)
		}
		return strings.Compare(a.Kind, b.Kind)
	})
	return out
}

func sortedChildParents(in map[groupKind][]groupKind) map[groupKind][]groupKind {
	out := make(map[groupKind][]groupKind, len(in))
	for child, parents := range in {
		out[child] = sortGroupKinds(parents)
	}
	return out
}

func TestCompileRules_DefaultRoots(t *testing.T) {
	rules := compileRules(config.ResourceTreeDefaults())

	require.NotNil(t, rules.roots)
	gotRoots := make([]groupKind, 0, len(rules.roots))
	for root := range rules.roots {
		gotRoots = append(gotRoots, root)
	}

	wantRoots := []groupKind{
		{Group: "apps", Kind: "Deployment"},
		{Group: "batch", Kind: "CronJob"},
		{Group: "batch", Kind: "Job"},
		{Group: "external-secrets.io", Kind: "ExternalSecret"},
	}
	assert.Equal(t, sortGroupKinds(wantRoots), sortGroupKinds(gotRoots))

	// The rule keeps the full kind reference, including the resource the RBAC
	// generator and the agent both need.
	deployment := rootRule(t, rules, "apps", "Deployment")
	assert.Equal(t, config.KindRef{Group: "apps", Version: "v1", Kind: "Deployment", Resource: "deployments"}, deployment.Kind)
}

func TestCompileRules_NormalizesBuiltInMatchers(t *testing.T) {
	rules := compileRules(config.ResourceTreeDefaults())

	replicaSet := childByKind(t, rootRule(t, rules, "apps", "Deployment").Children, "ReplicaSet")
	// Built-in rules leave the matcher empty in config; compilation is where it
	// becomes explicit.
	assert.Equal(t, "ownerRef", replicaSet.Matcher)
	assert.True(t, replicaSet.Hide, "ReplicaSets are walked through but not shown")
	assert.Nil(t, replicaSet.Criteria, "ownerRef carries no criteria")

	pod := childByKind(t, replicaSet.Children, "Pod")
	assert.Equal(t, "ownerRef", pod.Matcher)
	assert.False(t, pod.Hide)

	// No built-in edge may be left with an empty matcher or stray criteria.
	for root := range rules.roots {
		for _, rule := range rules.roots[root] {
			walkChildren(rule.Children, func(child *compiledChild) {
				assert.Equal(t, "ownerRef", child.Matcher, "edge %s", child.EdgeID)
				assert.Nil(t, child.Criteria, "edge %s", child.EdgeID)
			})
		}
	}
}

// An unsupported matcher cannot reach compilation through validated config, but
// if one ever does it must not be folded into ownerRef: passed through, the
// agent rejects it before it reaches an API server.
func TestCompileRules_UnknownMatcherIsNotOwnerRef(t *testing.T) {
	cfg := config.ResourceTreeConfig{Rules: []config.ResourceTreeRule{{
		Root: config.KindRef{Group: "apps", Version: "v1", Kind: "Deployment", Resource: "deployments"},
		Children: []config.ChildRule{{
			Kind:    config.KindRef{Version: "v1", Kind: "Pod", Resource: "pods"},
			Matcher: "objectRef",
		}},
	}}}

	child := childByKind(t, rootRule(t, compileRules(cfg), "apps", "Deployment").Children, "Pod")
	assert.Equal(t, "objectRef", child.Matcher)
}

func TestCompileRules_MetadataOnlyNormalization(t *testing.T) {
	coreSecret := config.KindRef{Version: "v1", Kind: "Secret", Resource: "secrets"}
	corePod := config.KindRef{Version: "v1", Kind: "Pod", Resource: "pods"}
	// A Secret kind in another group must not inherit the core Secret default.
	otherSecret := config.KindRef{Group: "example.com", Version: "v1", Kind: "Secret", Resource: "secrets"}
	// The kind is free text that validation only checks for emptiness, and it is
	// what the fallback backfills onto list items. The default keys off the
	// resource so a spelling like this cannot silently turn it off.
	lowercaseSecret := config.KindRef{Version: "v1", Kind: "secret", Resource: "secrets"}
	// The flip side: the resource is what decides, so a kind spelled "Secret"
	// over some other resource is not a Secret edge.
	secretKindOverConfigMaps := config.KindRef{Version: "v1", Kind: "Secret", Resource: "configmaps"}

	tests := []struct {
		name         string
		kind         config.KindRef
		metadataOnly *bool
		want         bool
	}{
		{"core Secret defaults to metadata only", coreSecret, nil, true},
		{"explicit false wins over the Secret default", coreSecret, ptr(false), false},
		{"explicit true is preserved", coreSecret, ptr(true), true},
		{"other kinds default to full objects", corePod, nil, false},
		{"other kinds may opt in", corePod, ptr(true), true},
		{"Secret in another group has no default", otherSecret, nil, false},
		{"a non-canonical kind spelling keeps the default", lowercaseSecret, nil, true},
		{"the resource decides, not the kind spelling", secretKindOverConfigMaps, nil, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := config.ResourceTreeConfig{Rules: []config.ResourceTreeRule{{
				Root:     config.KindRef{Group: "apps", Version: "v1", Kind: "Deployment", Resource: "deployments"},
				Children: []config.ChildRule{{Kind: tt.kind, MetadataOnly: tt.metadataOnly}},
			}}}

			rules := compileRules(cfg)
			child := childByKind(t, rootRule(t, rules, "apps", "Deployment").Children, tt.kind.Kind)
			assert.Equal(t, tt.want, child.MetadataOnly)
		})
	}
}

func TestCompileRules_LabelSelectorCriteria(t *testing.T) {
	selector := &config.LabelSelectorSpec{
		MatchLabels: map[string]string{
			"gateway.networking.k8s.io/gateway-name":      config.TokenParentName,
			"gateway.networking.k8s.io/gateway-namespace": config.TokenParentNamespace,
			"app.kubernetes.io/managed-by":                "kgateway",
		},
		Namespaces: []string{"kgateway-system"},
	}
	cfg := config.ResourceTreeConfig{Rules: []config.ResourceTreeRule{{
		Root: config.KindRef{Group: "gateway.networking.k8s.io", Version: "v1", Kind: "Gateway", Resource: "gateways"},
		Children: []config.ChildRule{{
			Kind:          config.KindRef{Group: "apps", Version: "v1", Kind: "Deployment", Resource: "deployments"},
			Matcher:       config.MatcherLabelSelector,
			LabelSelector: selector,
		}},
	}}}

	rules := compileRules(cfg)
	child := childByKind(t, rootRule(t, rules, "gateway.networking.k8s.io", "Gateway").Children, "Deployment")

	assert.Equal(t, "labelSelector", child.Matcher)
	require.NotNil(t, child.Criteria)

	var criteria protocol.LabelSelectorCriteria
	require.NoError(t, json.Unmarshal(child.Criteria, &criteria))
	assert.Equal(t, protocol.LabelSelectorCriteria{
		MatchLabels: selector.MatchLabels,
		Namespaces:  selector.Namespaces,
	}, criteria)

	// Tokens travel to the agent unsubstituted: one query serves N parents.
	assert.Equal(t, "${parent.metadata.name}", criteria.MatchLabels["gateway.networking.k8s.io/gateway-name"])
	assert.Equal(t, "${parent.metadata.namespace}", criteria.MatchLabels["gateway.networking.k8s.io/gateway-namespace"])

	// The criteria is wire JSON, not config YAML: match_labels became matchLabels.
	var wire map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(child.Criteria, &wire))
	assert.Contains(t, wire, "matchLabels")
	assert.NotContains(t, wire, "match_labels")
}

func TestCompileRules_ChildParents(t *testing.T) {
	rules := compileRules(config.ResourceTreeDefaults())

	deployment := groupKind{Group: "apps", Kind: "Deployment"}
	cronJob := groupKind{Group: "batch", Kind: "CronJob"}
	job := groupKind{Group: "batch", Kind: "Job"}
	externalSecret := groupKind{Group: "external-secrets.io", Kind: "ExternalSecret"}

	want := map[groupKind][]groupKind{
		{Kind: "Pod"}:                       sortGroupKinds([]groupKind{deployment, job, cronJob}),
		{Group: "apps", Kind: "ReplicaSet"}: {deployment},
		job:                                 {cronJob},
		{Kind: "Secret"}:                    {externalSecret},
	}
	assert.Equal(t, want, sortedChildParents(rules.childParents))
}

func TestCompileRules_ChildParentsDeduplicatesRoots(t *testing.T) {
	pod := config.KindRef{Version: "v1", Kind: "Pod", Resource: "pods"}
	// Pods are reachable from the Deployment twice: directly and via ReplicaSet.
	cfg := config.ResourceTreeConfig{Rules: []config.ResourceTreeRule{{
		Root: config.KindRef{Group: "apps", Version: "v1", Kind: "Deployment", Resource: "deployments"},
		Children: []config.ChildRule{
			{
				Kind:     config.KindRef{Group: "apps", Version: "v1", Kind: "ReplicaSet", Resource: "replicasets"},
				Children: []config.ChildRule{{Kind: pod}},
			},
			{Kind: pod},
		},
	}}}

	rules := compileRules(cfg)
	assert.Equal(t, []groupKind{{Group: "apps", Kind: "Deployment"}}, rules.childParents[groupKind{Kind: "Pod"}])
}

func TestCompileRules_EdgeIDs(t *testing.T) {
	rules := compileRules(config.ResourceTreeDefaults())

	deployment := rootRule(t, rules, "apps", "Deployment")
	replicaSet := childByKind(t, deployment.Children, "ReplicaSet")
	assert.Equal(t, "apps/Deployment/children[0]", replicaSet.EdgeID)
	assert.Equal(t, "apps/Deployment/children[0]/children[0]", childByKind(t, replicaSet.Children, "Pod").EdgeID)

	cronJob := rootRule(t, rules, "batch", "CronJob")
	jobEdge := childByKind(t, cronJob.Children, "Job")
	assert.Equal(t, "batch/CronJob/children[0]", jobEdge.EdgeID)
	assert.Equal(t, "batch/CronJob/children[0]/children[0]", childByKind(t, jobEdge.Children, "Pod").EdgeID)

	assert.Equal(t, "batch/Job/children[0]", childByKind(t, rootRule(t, rules, "batch", "Job").Children, "Pod").EdgeID)
}

func TestCompileRules_EdgeIDsAreUniqueAcrossBranches(t *testing.T) {
	pod := config.ChildRule{Kind: config.KindRef{Version: "v1", Kind: "Pod", Resource: "pods"}}
	replicaSet := config.KindRef{Group: "apps", Version: "v1", Kind: "ReplicaSet", Resource: "replicasets"}
	// The same parent and child kinds at the same depth under two sibling
	// branches, which validation allows because the matchers differ. Level plus
	// parent kind plus child kind collides here; the structural path does not.
	cfg := config.ResourceTreeConfig{Rules: []config.ResourceTreeRule{
		{
			Root: config.KindRef{Group: "apps", Version: "v1", Kind: "Deployment", Resource: "deployments"},
			Children: []config.ChildRule{
				{Kind: replicaSet, Children: []config.ChildRule{pod}},
				{
					Kind:    replicaSet,
					Matcher: config.MatcherLabelSelector,
					LabelSelector: &config.LabelSelectorSpec{
						MatchLabels: map[string]string{"app": config.TokenParentName},
					},
					Children: []config.ChildRule{pod},
				},
			},
		},
		{
			Root:     config.KindRef{Group: "batch", Version: "v1", Kind: "Job", Resource: "jobs"},
			Children: []config.ChildRule{pod},
		},
	}}

	rules := compileRules(cfg)

	seen := make(map[string]bool)
	edges := 0
	for root := range rules.roots {
		for _, rule := range rules.roots[root] {
			walkChildren(rule.Children, func(child *compiledChild) {
				edges++
				assert.NotEmpty(t, child.EdgeID)
				assert.False(t, seen[child.EdgeID], "duplicate edge ID %q", child.EdgeID)
				seen[child.EdgeID] = true
			})
		}
	}
	assert.Equal(t, 5, edges, "two ReplicaSet branches with a Pod leaf each, plus the Job rule")
	assert.Len(t, seen, edges)
}

func ptr[T any](v T) *T {
	return &v
}
