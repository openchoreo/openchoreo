// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"github.com/openchoreo/openchoreo/internal/resourcetree/protocol"
)

// Matcher names and substitution tokens are the protocol's own: config
// validates exactly what the agent executes, so the two packages share one set
// and cannot drift.
const (
	MatcherOwnerRef      = protocol.MatcherOwnerRef
	MatcherLabelSelector = protocol.MatcherLabelSelector
	TokenParentName      = protocol.TokenParentName
	TokenParentNamespace = protocol.TokenParentNamespace
)

// ResourceTreeConfig declares how the resource tree walks from a root workload
// resource down to its children. It replaces a hardcoded parent/child kind
// traversal, so operators can describe kinds this binary has never heard of.
type ResourceTreeConfig struct {
	// Rules is the set of traversal rules, one per root kind.
	Rules []ResourceTreeRule `koanf:"rules" json:"rules"`
}

// ResourceTreeRule describes the child chains reachable from one root kind.
type ResourceTreeRule struct {
	// Root is the kind the rule applies to.
	Root KindRef `koanf:"root" json:"root"`
	// Children are the edges walked from the root.
	Children []ChildRule `koanf:"children" json:"children"`
}

// KindRef identifies an API kind and its REST plural. Resource is required
// because pluralization is not derivable for arbitrary CRDs and the offline
// RBAC generator needs the exact GVR.
type KindRef struct {
	Group    string `koanf:"group" json:"group,omitempty"`
	Version  string `koanf:"version" json:"version"`
	Kind     string `koanf:"kind" json:"kind"`
	Resource string `koanf:"resource" json:"resource"`
}

// ChildRule is a single parent-to-child edge in a traversal rule.
type ChildRule struct {
	Kind KindRef `koanf:"kind" json:"kind"`
	// Matcher selects the match strategy. Empty means ownerRef. labelSelector
	// requires the LabelSelector block below; future matchers (objectRef)
	// add their own config blocks here.
	Matcher       string             `koanf:"matcher" json:"matcher,omitempty"`
	LabelSelector *LabelSelectorSpec `koanf:"label_selector" json:"label_selector,omitempty"`
	MetadataOnly  *bool              `koanf:"metadata_only" json:"metadata_only,omitempty"`
	Hide          bool               `koanf:"hide" json:"hide,omitempty"`
	Children      []ChildRule        `koanf:"children" json:"children,omitempty"`
}

// LabelSelectorSpec configures matcher "labelSelector". MatchLabels values may
// contain TokenParentName/TokenParentNamespace, substituted per parent
// agent-side; keys may not, since nothing substitutes them. Namespaces empty
// means each parent's own namespace; entries are literal namespace names (no
// wildcard, no substitution tokens). v1 is match_labels only — matchExpressions
// is deferred.
type LabelSelectorSpec struct {
	MatchLabels map[string]string `koanf:"match_labels" json:"match_labels,omitempty"`
	Namespaces  []string          `koanf:"namespaces" json:"namespaces,omitempty"`
}

// ResourceTreeDefaults returns the built-in traversal rules: the parent/child
// chains the resource tree walked before rules were configurable, plus the
// ExternalSecret chain, which every install using secret management renders.
// Built-ins stay ownerRef matched.
func ResourceTreeDefaults() ResourceTreeConfig {
	pod := ChildRule{Kind: KindRef{Version: "v1", Kind: "Pod", Resource: "pods"}}
	job := KindRef{Group: "batch", Version: "v1", Kind: "Job", Resource: "jobs"}

	return ResourceTreeConfig{
		Rules: []ResourceTreeRule{
			{
				Root: KindRef{Group: "apps", Version: "v1", Kind: "Deployment", Resource: "deployments"},
				Children: []ChildRule{{
					Kind: KindRef{Group: "apps", Version: "v1", Kind: "ReplicaSet", Resource: "replicasets"},
					// ReplicaSets are an implementation detail of the Deployment:
					// walked through, but not shown in the tree.
					Hide:     true,
					Children: []ChildRule{pod},
				}},
			},
			{
				Root: KindRef{Group: "batch", Version: "v1", Kind: "CronJob", Resource: "cronjobs"},
				Children: []ChildRule{{
					Kind:     job,
					Children: []ChildRule{pod},
				}},
			},
			{
				Root:     job,
				Children: []ChildRule{pod},
			},
			{
				// External Secrets Operator stamps a controller ownerReference on
				// the Secret it syncs, so the default matcher links the two. The
				// Secret node is metadata only by the core Secret default, and its
				// contents are stripped regardless.
				Root: KindRef{Group: "external-secrets.io", Version: "v1", Kind: "ExternalSecret", Resource: "externalsecrets"},
				Children: []ChildRule{{
					Kind: KindRef{Version: "v1", Kind: "Secret", Resource: "secrets"},
				}},
			},
		},
	}
}
