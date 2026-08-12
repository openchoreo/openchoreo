// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package k8sresources

import (
	"encoding/json"
	"fmt"
	"slices"

	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/config"
	"github.com/openchoreo/openchoreo/internal/resourcetree/protocol"
)

// groupKind identifies a kind by API group and kind, deliberately without a
// version: versions are representations of one object, so keying by full GVK
// would drop children whenever a release renders a different served version.
type groupKind struct {
	Group string
	Kind  string
}

// compiledRules is the in-memory form of the configured traversal rules.
type compiledRules struct {
	// roots maps a root group+kind to its rules. Validation rejects duplicate
	// roots, but compilation stays total rather than assuming it ran.
	roots map[groupKind][]*compiledRule
	// childParents maps a child group+kind to the root group+kinds it is
	// reachable from, at any depth. The value slices are shared, not copied per
	// lookup: callers must read them, never mutate or append to them.
	childParents map[groupKind][]groupKind
}

type compiledRule struct {
	Kind     config.KindRef
	Children []*compiledChild
}

type compiledChild struct {
	Kind config.KindRef
	// Matcher is normalized to MatcherOwnerRef or MatcherLabelSelector, never
	// empty. A matcher this binary does not support, which validation rejects
	// at startup, is passed through unchanged rather than reinterpreted.
	Matcher string
	// Criteria is the matcher-specific query payload, marshaled once here and
	// shared by every request that uses this edge, so it must be assigned as is
	// and never mutated. Nil for ownerRef.
	Criteria json.RawMessage
	// MetadataOnly is normalized: the core Secret default is already applied.
	MetadataOnly bool
	Hide         bool
	// EdgeID is the structural path of this edge from its root, used as the
	// match query ID. It is unique across the compiled rules as long as no two
	// rules share a root group+kind, which config validation enforces at
	// startup.
	EdgeID   string
	Children []*compiledChild
}

// gvr is the resource this edge lists its children through, on either path: the
// agent builds its dynamic client from it, and the fallback builds its list URL
// from it. Kind.Resource is a required config field — an empty one fails startup
// validation — so this is always the real plural, which is what makes it safe to
// key sanitizeObject's Secret strip off.
func (c *compiledChild) gvr() schema.GroupVersionResource {
	return schema.GroupVersionResource{Group: c.Kind.Group, Version: c.Kind.Version, Resource: c.Kind.Resource}
}

// compileRules turns validated configuration into the form the tree walk uses.
// It assumes the config already passed validation and therefore reports no
// errors of its own: anything validation would have rejected is normalized to a
// safe value rather than failing here.
func compileRules(cfg config.ResourceTreeConfig) *compiledRules {
	compiled := &compiledRules{
		roots:        make(map[groupKind][]*compiledRule, len(cfg.Rules)),
		childParents: make(map[groupKind][]groupKind),
	}

	for _, rule := range cfg.Rules {
		root := groupKind{Group: rule.Root.Group, Kind: rule.Root.Kind}
		compiled.roots[root] = append(compiled.roots[root], &compiledRule{
			Kind:     rule.Root,
			Children: compiled.compileChildren(rule.Children, edgeIDRoot(rule.Root), root),
		})
	}

	return compiled
}

// compileChildren compiles one level of child edges and recurses, recording
// each child kind against the root it is reachable from.
func (c *compiledRules) compileChildren(children []config.ChildRule, edgeIDPrefix string, root groupKind) []*compiledChild {
	if len(children) == 0 {
		return nil
	}

	compiled := make([]*compiledChild, 0, len(children))
	for i, child := range children {
		edgeID := fmt.Sprintf("%s/children[%d]", edgeIDPrefix, i)
		matcher := compileMatcher(child.Matcher)

		compiled = append(compiled, &compiledChild{
			Kind:         child.Kind,
			Matcher:      matcher,
			Criteria:     compileCriteria(matcher, child.LabelSelector),
			MetadataOnly: metadataOnly(child),
			Hide:         child.Hide,
			EdgeID:       edgeID,
			Children:     c.compileChildren(child.Children, edgeID, root),
		})

		c.recordParent(groupKind{Group: child.Kind.Group, Kind: child.Kind.Kind}, root)
	}

	return compiled
}

// recordParent notes that child is reachable from root. A child reachable by
// more than one path from the same root is recorded once.
func (c *compiledRules) recordParent(child, root groupKind) {
	if slices.Contains(c.childParents[child], root) {
		return
	}
	c.childParents[child] = append(c.childParents[child], root)
}

// compileMatcher normalizes the configured matcher to its wire value. An empty
// matcher means ownerRef. Validation already rejected anything this binary does
// not support; a value that slips through anyway is passed along unchanged so
// the agent rejects it with UnsupportedMatcher instead of it being reinterpreted.
func compileMatcher(matcher string) string {
	if matcher == "" {
		return protocol.MatcherOwnerRef
	}
	return matcher
}

// compileCriteria marshals the matcher-specific query payload once, so it is
// not rebuilt on every request. The substitution tokens inside stay verbatim:
// the agent substitutes them per parent, which is what lets one query serve
// many parents. ownerRef carries no criteria.
func compileCriteria(matcher string, selector *config.LabelSelectorSpec) json.RawMessage {
	if matcher != protocol.MatcherLabelSelector || selector == nil {
		return nil
	}

	// Marshaling maps and slices of strings cannot fail.
	criteria, _ := json.Marshal(protocol.LabelSelectorCriteria{
		MatchLabels: selector.MatchLabels,
		Namespaces:  selector.Namespaces,
	})

	return criteria
}

// metadataOnly resolves whether only object metadata should be fetched. Core
// Secrets default to metadata only so their data never crosses the tunnel; an
// explicit setting always wins, which is why config models it as a pointer.
func metadataOnly(child config.ChildRule) bool {
	if child.MetadataOnly != nil {
		return *child.MetadataOnly
	}
	return isCoreSecretResource(child.Kind.Group, child.Kind.Resource)
}

// isCoreSecretResource reports whether a GVR selects core v1 Secrets.
//
// The check is on the GVR, never on a kind string. A rule's kind is free text
// that validation only checks for emptiness, and it is what the list-and-filter
// fallback backfills onto items that arrive without one — so `kind: secret`
// with `resource: secrets` lists Secrets perfectly well while spelling every
// kind-keyed Secret defense away. The resource is what the list call is built
// from: misspell it and the list fails outright rather than succeeding
// unprotected. Requiring the core group also keeps a CRD named Secret in
// another group out of this branch.
func isCoreSecretResource(group, resource string) bool {
	return group == "" && resource == "secrets"
}

// edgeIDRoot returns the edge ID prefix for a root kind. The core group is
// elided, so a root reads as "apps/Deployment" or "Pod".
func edgeIDRoot(root config.KindRef) string {
	if root.Group == "" {
		return root.Kind
	}
	return root.Group + "/" + root.Kind
}
