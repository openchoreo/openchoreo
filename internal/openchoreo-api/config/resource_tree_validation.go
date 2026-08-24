// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	"k8s.io/apimachinery/pkg/util/validation"

	coreconfig "github.com/openchoreo/openchoreo/internal/config"
	"github.com/openchoreo/openchoreo/internal/resourcetree/protocol"
)

const (
	maxRuleDepth = 8
	maxRuleEdges = 256
	// maxSelectorNamespaces caps label_selector.namespaces. The protocol value
	// is the agent's enforcement limit, so config can never validate a rule the
	// agent would reject.
	maxSelectorNamespaces = protocol.MaxSelectorNamespaces
)

// Validate validates the resource tree configuration. All defects are reported,
// not just the first one.
func (c ResourceTreeConfig) Validate(path *coreconfig.Path) coreconfig.ValidationErrors {
	var errs coreconfig.ValidationErrors

	rulesPath := path.Child("rules")
	seenRoots := make(map[string]bool, len(c.Rules))
	edges := 0

	for i, rule := range c.Rules {
		rulePath := rulesPath.Index(i)
		rootPath := rulePath.Child("root")

		errs = append(errs, validateKindRef(rule.Root, rootPath)...)

		// Rules are keyed by group+kind: a second rule for an already declared
		// root must replace the first explicitly, never shadow it silently.
		rootKey := rule.Root.Group + "/" + rule.Root.Kind
		if seenRoots[rootKey] {
			errs = append(errs, coreconfig.Invalid(rootPath,
				fmt.Sprintf("duplicate rule for root %s/%s; each root may appear at most once in the rule list",
					rule.Root.Group, rule.Root.Kind)))
		}
		seenRoots[rootKey] = true

		childrenPath := rulePath.Child("children")
		if len(rule.Children) == 0 {
			errs = append(errs, coreconfig.Invalid(childrenPath, "at least one child is required"))
		}
		errs = append(errs, validateChildren(rule.Children, childrenPath, 1, &edges)...)
	}

	if edges > maxRuleEdges {
		errs = append(errs, coreconfig.Invalid(rulesPath,
			fmt.Sprintf("child rules (%d) exceed the maximum of %d", edges, maxRuleEdges)))
	}

	return errs
}

// validateChildren validates one level of child edges and recurses. depth is the
// nesting level of children, starting at 1 for a root's direct children. edges
// accumulates the total edge count across the whole config.
func validateChildren(children []ChildRule, path *coreconfig.Path, depth int, edges *int) coreconfig.ValidationErrors {
	if len(children) == 0 {
		return nil
	}
	if depth > maxRuleDepth {
		return coreconfig.ValidationErrors{coreconfig.Invalid(path,
			fmt.Sprintf("child rule depth %d exceeds the maximum of %d", depth, maxRuleDepth))}
	}

	var errs coreconfig.ValidationErrors
	seenEdges := make(map[string]bool, len(children))

	for i, child := range children {
		childPath := path.Index(i)
		*edges++

		errs = append(errs, validateKindRef(child.Kind, childPath.Child("kind"))...)
		errs = append(errs, validateMatcher(child, childPath)...)

		// Siblings must be unambiguous: one edge per GVR and matcher.
		matcher := child.Matcher
		if matcher == "" {
			matcher = MatcherOwnerRef
		}
		edgeKey := strings.Join([]string{child.Kind.Group, child.Kind.Version, child.Kind.Resource, matcher}, "/")
		if seenEdges[edgeKey] {
			errs = append(errs, coreconfig.Invalid(childPath,
				fmt.Sprintf("duplicate child rule for resource %q with matcher %q", child.Kind.Resource, matcher)))
		}
		seenEdges[edgeKey] = true

		errs = append(errs, validateChildren(child.Children, childPath.Child("children"), depth+1, edges)...)
	}

	return errs
}

// validateKindRef checks a kind reference. Group may be empty: that is the core
// API group. Beyond requiredness, group/version/resource must be clean GVR syntax
// so a malformed resource cannot escape its namespaced REST path or slip past the
// agent's core-Secret strip; Kind is not part of the GVR and is only required.
func validateKindRef(ref KindRef, path *coreconfig.Path) coreconfig.ValidationErrors {
	var errs coreconfig.ValidationErrors

	if err := protocol.ValidateGroup(ref.Group); err != nil {
		errs = append(errs, coreconfig.Invalid(path.Child("group"), err.Error()))
	}

	if ref.Version == "" {
		errs = append(errs, coreconfig.Required(path.Child("version")))
	} else if err := protocol.ValidateVersion(ref.Version); err != nil {
		errs = append(errs, coreconfig.Invalid(path.Child("version"), err.Error()))
	}

	if ref.Kind == "" {
		errs = append(errs, coreconfig.Required(path.Child("kind")))
	}

	if ref.Resource == "" {
		errs = append(errs, coreconfig.Required(path.Child("resource")))
	} else if err := protocol.ValidateResource(ref.Resource); err != nil {
		errs = append(errs, coreconfig.Invalid(path.Child("resource"), err.Error()))
	}

	return errs
}

// validateMatcher checks that the matcher is supported by this binary and that
// the child carries exactly the config block that matcher needs.
func validateMatcher(child ChildRule, path *coreconfig.Path) coreconfig.ValidationErrors {
	var errs coreconfig.ValidationErrors

	switch child.Matcher {
	case "", MatcherOwnerRef:
		if child.LabelSelector != nil {
			errs = append(errs, coreconfig.Invalid(path, "label_selector is only valid with matcher labelSelector"))
		}
	case MatcherLabelSelector:
		if child.LabelSelector == nil {
			errs = append(errs, coreconfig.Invalid(path, "label_selector is required for matcher labelSelector"))
			break
		}
		errs = append(errs, child.LabelSelector.validate(path.Child("label_selector"))...)
	default:
		errs = append(errs, coreconfig.Invalid(path.Child("matcher"),
			fmt.Sprintf("matcher %q is not supported by this binary; supported: %s, %s",
				child.Matcher, MatcherOwnerRef, MatcherLabelSelector)))
	}

	return errs
}

// validate checks the labelSelector guardrails: the selector must be non-empty,
// must derive at least one value from the parent (so it cannot match unrelated
// resources cluster-wide), and may only name literal namespaces.
func (s *LabelSelectorSpec) validate(path *coreconfig.Path) coreconfig.ValidationErrors {
	var errs coreconfig.ValidationErrors

	labelsPath := path.Child("match_labels")
	if len(s.MatchLabels) == 0 {
		errs = append(errs, coreconfig.Required(labelsPath))
	} else {
		parentDerived := false
		for _, key := range slices.Sorted(maps.Keys(s.MatchLabels)) {
			if keyErrs := validation.IsQualifiedName(key); len(keyErrs) > 0 {
				errs = append(errs, coreconfig.Invalid(labelsPath.Child(key),
					fmt.Sprintf("%q is not a valid label key: %s", key, strings.Join(keyErrs, "; "))))
			}
			// Only values are substituted per parent. A token in a key would reach
			// the agent literally and match nothing, so reject it outright.
			if len(substitutionTokens(key)) > 0 {
				errs = append(errs, coreconfig.Invalid(labelsPath.Child(key),
					"substitution tokens are not supported in match_labels keys"))
			}
			value := s.MatchLabels[key]
			tokens := substitutionTokens(value)
			if len(tokens) == 0 {
				if valueErrs := validation.IsValidLabelValue(value); len(valueErrs) > 0 {
					errs = append(errs, coreconfig.Invalid(labelsPath.Child(key),
						fmt.Sprintf("%q is not a valid label value: %s", value, strings.Join(valueErrs, "; "))))
				}
			}
			for _, token := range tokens {
				switch token {
				case TokenParentName, TokenParentNamespace:
					parentDerived = true
				default:
					errs = append(errs, coreconfig.Invalid(labelsPath.Child(key),
						fmt.Sprintf("unknown substitution token %q", token)))
				}
			}
		}
		if !parentDerived {
			errs = append(errs, coreconfig.Invalid(labelsPath, "at least one value derived from the parent is required"))
		}
	}

	namespacesPath := path.Child("namespaces")
	if len(s.Namespaces) > maxSelectorNamespaces {
		errs = append(errs, coreconfig.Invalid(namespacesPath,
			fmt.Sprintf("at most %d namespaces are allowed", maxSelectorNamespaces)))
	}
	seenNamespaces := make(map[string]bool, len(s.Namespaces))
	for i, namespace := range s.Namespaces {
		entryPath := namespacesPath.Index(i)
		if err := coreconfig.MustNotBeEmpty(entryPath, namespace); err != nil {
			errs = append(errs, err)
			continue
		}
		if seenNamespaces[namespace] {
			errs = append(errs, coreconfig.Invalid(entryPath,
				fmt.Sprintf("duplicate namespace %q", namespace)))
			continue
		}
		seenNamespaces[namespace] = true
		switch {
		case strings.Contains(namespace, "*"):
			errs = append(errs, coreconfig.Invalid(entryPath, "wildcard namespaces are not allowed"))
		case len(substitutionTokens(namespace)) > 0:
			// Namespaces are literal names in v1. Nothing substitutes them, so a
			// token here — recognized or not — would list against a namespace that
			// cannot exist and silently yield no children.
			errs = append(errs, coreconfig.Invalid(entryPath, "substitution tokens are not supported in namespaces"))
		default:
			if namespaceErrs := validation.IsDNS1123Label(namespace); len(namespaceErrs) > 0 {
				errs = append(errs, coreconfig.Invalid(entryPath,
					fmt.Sprintf("%q is not a valid namespace name: %s", namespace, strings.Join(namespaceErrs, "; "))))
			}
		}
	}

	return errs
}

// substitutionTokens returns every ${...} occurrence in value, including
// unterminated ones so they are reported rather than silently accepted.
func substitutionTokens(value string) []string {
	var tokens []string

	rest := value
	for {
		start := strings.Index(rest, "${")
		if start < 0 {
			return tokens
		}
		rest = rest[start:]
		end := strings.Index(rest, "}")
		if end < 0 {
			return append(tokens, rest)
		}
		tokens = append(tokens, rest[:end+1])
		rest = rest[end+1:]
	}
}

// Known keys per config level, used to reject typos that would otherwise be
// silently dropped when the section is unmarshaled.
var (
	resourceTreeKeys  = []string{"rules"}
	ruleKeys          = []string{"root", "children"}
	kindRefKeys       = []string{"group", "version", "kind", "resource"}
	childRuleKeys     = []string{"kind", "matcher", "label_selector", "metadata_only", "hide", "children"}
	labelSelectorKeys = []string{"match_labels", "namespaces"}
)

// validateRawKeys reports keys in the raw resource_tree section that this binary
// does not know, and rejects a null section or null rules. Unmarshaling ignores
// unknown keys, so a typo such as "metadata_onlyy" would otherwise take effect
// as its default; and a null section or `rules:` with no value overwrites the
// built-in defaults with nothing, which would validate as zero rules and
// silently disable all child discovery. An absent section keeps the built-ins;
// disabling them is spelled `rules: []`.
func (ResourceTreeConfig) validateRawKeys(raw any) coreconfig.ValidationErrors {
	path := coreconfig.NewPath("resource_tree")

	// An absent section leaves the merged defaults in place, so a nil here is an
	// explicit `resource_tree:` with no value — never a missing section.
	if raw == nil {
		return coreconfig.ValidationErrors{coreconfig.Invalid(path,
			"is null; omit it to keep the built-in rules, or set rules: [] to disable them")}
	}

	section, ok := raw.(map[string]any)
	if !ok {
		return coreconfig.ValidationErrors{coreconfig.Invalid(path,
			fmt.Sprintf("must be a mapping, got %T", raw))}
	}

	if rulesVal, present := section["rules"]; present && rulesVal == nil {
		return coreconfig.ValidationErrors{coreconfig.Invalid(path.Child("rules"),
			"is null; omit resource_tree to keep the built-in rules, or set rules: [] to disable them")}
	}

	errs := unknownKeyErrors(section, resourceTreeKeys, path)

	rulesPath := path.Child("rules")
	for i, item := range rawList(section["rules"]) {
		rule, ok := item.(map[string]any)
		if !ok {
			continue
		}
		rulePath := rulesPath.Index(i)
		errs = append(errs, unknownKeyErrors(rule, ruleKeys, rulePath)...)
		if root, ok := rule["root"].(map[string]any); ok {
			errs = append(errs, unknownKeyErrors(root, kindRefKeys, rulePath.Child("root"))...)
		}
		errs = append(errs, childRuleKeyErrors(rule["children"], rulePath.Child("children"))...)
	}

	return errs
}

// childRuleKeyErrors checks one raw level of child rules and recurses.
func childRuleKeyErrors(raw any, path *coreconfig.Path) coreconfig.ValidationErrors {
	var errs coreconfig.ValidationErrors

	for i, item := range rawList(raw) {
		child, ok := item.(map[string]any)
		if !ok {
			continue
		}
		childPath := path.Index(i)
		errs = append(errs, unknownKeyErrors(child, childRuleKeys, childPath)...)
		if kind, ok := child["kind"].(map[string]any); ok {
			errs = append(errs, unknownKeyErrors(kind, kindRefKeys, childPath.Child("kind"))...)
		}
		if selector, ok := child["label_selector"].(map[string]any); ok {
			errs = append(errs, unknownKeyErrors(selector, labelSelectorKeys, childPath.Child("label_selector"))...)
		}
		errs = append(errs, childRuleKeyErrors(child["children"], childPath.Child("children"))...)
	}

	return errs
}

// unknownKeyErrors reports every key of m that is not in known, in a stable order.
func unknownKeyErrors(m map[string]any, known []string, path *coreconfig.Path) coreconfig.ValidationErrors {
	var unknown []string
	for key := range m {
		if !slices.Contains(known, key) {
			unknown = append(unknown, key)
		}
	}
	slices.Sort(unknown)

	var errs coreconfig.ValidationErrors
	for _, key := range unknown {
		errs = append(errs, coreconfig.Invalid(path,
			fmt.Sprintf("unknown key %q; valid keys: %s", key, strings.Join(known, ", "))))
	}

	return errs
}

// rawList returns raw as a list, or nil if it is not one. Entries keep their
// index so error paths point at the offending element.
func rawList(raw any) []any {
	items, _ := raw.([]any)
	return items
}
