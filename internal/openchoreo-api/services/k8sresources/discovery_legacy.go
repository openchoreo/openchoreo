// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package k8sresources

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"k8s.io/apimachinery/pkg/labels"

	"github.com/openchoreo/openchoreo/internal/resourcetree/protocol"
)

// This file holds the control-plane fallback traversal, the path taken only when
// the cluster agent predates resource-tree matching. It is kept apart from the
// agent-backed walk in discovery.go so it can be deleted in one piece once every
// agent in the field answers match queries.

// resolveGroupsLegacy runs the same level control-plane side, one parent per
// edge, for an agent that predates resource-tree matching.
func (w *treeWalk) resolveGroupsLegacy(ctx context.Context, groups []*edgeGroup) [][]childMatch {
	matches := make([][]childMatch, len(groups))
	for i, group := range groups {
		for _, parent := range group.parents {
			objects, err := w.svc.fetchEdgeChildren(ctx, w.pi, group.edge, parent)
			if err != nil {
				w.acc.addStatus(parent.anchorRef.UID, statusFor(group.edge, legacyFailureState(err), err.Error()))
				continue
			}
			for _, obj := range objects {
				matches[i] = append(matches[i], childMatch{obj: obj, parentUIDs: []string{parent.uid}})
			}
		}
	}
	return matches
}

// legacyFailureState distinguishes a missing RBAC grant, which an operator can
// fix, from every other listing failure.
func legacyFailureState(err error) string {
	var statusErr *liveResourceStatusError
	if errors.As(err, &statusErr) && statusErr.statusCode == http.StatusForbidden {
		return discoveryStateForbidden
	}
	return discoveryStateError
}

// fetchEdgeChildren resolves ONE parent's children for one edge the way the
// control plane did before the agent could match: list the child kind and filter
// here. Both matchers are covered, so a rules-driven tree still renders against
// an agent that predates resource-tree support.
func (s *k8sResourcesService) fetchEdgeChildren(ctx context.Context, pi planeInfo,
	edge *compiledChild, parent *walkParent) ([]map[string]any, error) {
	switch edge.Matcher {
	case protocol.MatcherOwnerRef:
		return s.fetchOwnedResources(ctx, pi, edge.Kind.Group, edge.Kind.Version, edge.Kind.Kind,
			edge.Kind.Resource, parent.namespace, parent.uid)
	case protocol.MatcherLabelSelector:
		return s.fetchSelectedChildren(ctx, pi, edge, parent)
	default:
		// Startup validation rejects an unknown matcher, and the agent answers
		// one with UnsupportedMatcher. The fallback fails closed the same way
		// rather than guessing a strategy.
		return nil, fmt.Errorf("matcher %q is not supported by the control-plane fallback", edge.Matcher)
	}
}

// fetchSelectedChildren reproduces the agent's labelSelector semantics on the
// control-plane side: substitute the parent tokens, validate the resulting
// selector, and list each target namespace with it pushed server side. A parent
// whose referenced field is empty is skipped before any of that, the same way
// the agent skips it.
func (s *k8sResourcesService) fetchSelectedChildren(ctx context.Context, pi planeInfo,
	edge *compiledChild, parent *walkParent) ([]map[string]any, error) {
	var criteria protocol.LabelSelectorCriteria
	if err := json.Unmarshal(edge.Criteria, &criteria); err != nil {
		return nil, fmt.Errorf("failed to decode labelSelector criteria: %w", err)
	}

	if usesName, usesNamespace := criteria.ParentFieldUse(); (usesName && parent.name == "") ||
		(usesNamespace && parent.namespace == "") {
		// Same skip the agent applies: with the referenced field empty the
		// substituted selector keeps nothing of this parent, so it would
		// mis-attribute children.
		s.logger.Debug("Skipping parent with an empty field the selector derives from",
			"parent", parent.name, "namespace", parent.namespace)
		return nil, nil
	}

	set := make(labels.Set, len(criteria.MatchLabels))
	for key, value := range criteria.MatchLabels {
		substituted, err := protocol.SubstituteParentTokens(value, parent.name, parent.namespace)
		if err != nil {
			// Unreachable while config validation runs: the tokens in a rule are
			// checked at startup. Kept so a substitution failure can never fall
			// through as a literal selector value.
			return nil, fmt.Errorf("criteria.matchLabels[%q]: %w", key, err)
		}
		set[key] = substituted
	}

	selector, err := labels.ValidatedSelectorFromSet(set)
	if err != nil {
		// The agent's semantics: no object can carry an illegal label value, so
		// "no matches" is the true answer for this parent, not a failure.
		s.logger.Debug("Skipping parent whose substituted selector is not a valid label selector",
			"parent", parent.name, "namespace", parent.namespace, "error", err)
		return nil, nil
	}

	namespaces := criteria.Namespaces
	if len(namespaces) == 0 {
		namespaces = []string{parent.namespace}
	}

	var objects []map[string]any
	for _, namespace := range namespaces {
		items, err := s.fetchLabelSelectedResources(ctx, pi, edge.Kind.Group, edge.Kind.Version,
			edge.Kind.Kind, edge.Kind.Resource, namespace, selector.String())
		if err != nil {
			return nil, err
		}
		objects = append(objects, items...)
	}
	return objects, nil
}

// warnLegacyFallback logs the version-skew warning once per tree walk. The flag
// it reads lives on the accumulator, which outlives this file: the walk's shared
// state stays where the primary path declares it so that deleting the fallback
// means deleting this file rather than editing that struct.
func (w *treeWalk) warnLegacyFallback() {
	if w.acc.warnedLegacyFallback {
		return
	}
	w.acc.warnedLegacyFallback = true
	w.svc.logger.Warn("cluster agent lacks resource-tree support; falling back to control-plane filtering")
}
