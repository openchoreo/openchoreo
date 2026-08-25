// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package audit

import (
	"slices"

	coreconfig "github.com/openchoreo/openchoreo/internal/config"
)

// Selector constrains which operations/actors/results a Policy applies to.
// Every populated field is OR'd internally (matches if the observed value is
// in the list); all populated fields are AND'd together (every non-empty axis
// must match). A selector with every field empty matches everything.
type Selector struct {
	Categories   []Category
	Resources    []string
	Operations   []string
	Actions      []string
	Origins      []Origin
	ActorTypes   []string
	Actors       []string
	Entitlements []string
	Results      []Result
}

// PartialSettings is what one Policy rule overrides. A nil field means
// "inherit from defaults / untouched by this rule." Category is deliberately
// not a field here: it is stamped from the Operation, never operator-set —
// see the config layer's rejection of a `set: {category: ...}` key.
//
// Publish is the only field today. pre_action (P10a) and delivery/retries
// (P10b) are reserved for later phases — they are deliberately not modeled
// here until the phase that acts on them lands, rather than carried as inert
// config that does nothing.
type PartialSettings struct {
	Publish *bool
}

// Policy is one ordered rule: if Match applies, Set overrides the defaults.
type Policy struct {
	Match Selector
	Set   PartialSettings
}

// Settings is a fully resolved policy outcome — no pointers, defaults applied.
type Settings struct {
	Publish bool
}

// ResolveContext carries what PolicySet.Resolve needs to evaluate selectors.
// Operation may be nil (an event that never resolved to one); Resolve treats
// that as failing any Categories/Resources/Operations/Actions selector rather
// than panicking.
type ResolveContext struct {
	Operation *Operation
	Actor     Actor
	Origin    Origin
	Result    Result
}

// PolicySet is an ordered, first-match-wins set of policies, immutable after
// construction — required because concurrent MCP tool calls on one session
// resolve settings from the same PolicySet in parallel goroutines.
type PolicySet struct {
	defaults Settings
	policies []Policy
}

// NewPolicySet validates and builds an immutable PolicySet. It is the single
// enforcement point for selector admissibility — both AuditConfig.Validate
// and the real build path call this same function, so there is no second code
// path that could construct an invalid PolicySet.
func NewPolicySet(path *coreconfig.Path, defaults Settings, policies []Policy) (*PolicySet, coreconfig.ValidationErrors) {
	var errs coreconfig.ValidationErrors

	policiesPath := path.Child("policies")
	copied := make([]Policy, len(policies))
	for i, p := range policies {
		cp := clonePolicy(p)
		copied[i] = cp
		errs = append(errs, validatePolicy(policiesPath.Index(i), cp)...)
	}
	if len(errs) > 0 {
		return nil, errs
	}
	return &PolicySet{defaults: defaults, policies: copied}, nil
}

// validatePolicy enforces the selector-admissibility gate: a selector is only
// legal if it passes both a timing gate (is the value known when this rule's
// settings take effect?) and a surface-parity gate (would the rule behave
// identically on REST and MCP?).
func validatePolicy(path *coreconfig.Path, p Policy) coreconfig.ValidationErrors {
	var errs coreconfig.ValidationErrors

	setsPublish := p.Set.Publish != nil
	setsSuppress := setsPublish && !*p.Set.Publish
	setsNothing := p.Set.Publish == nil
	matchIsEmpty := p.Match.isEmpty()

	if (len(p.Match.Actors) > 0 || len(p.Match.Entitlements) > 0) && setsSuppress {
		errs = append(errs, coreconfig.Invalid(path.Child("set").Child("publish"),
			"actors/entitlements may not be used to suppress publishing (publish: false); "+
				"permitted for escalation (publish: true)"))
	}
	if setsNothing {
		errs = append(errs, coreconfig.Invalid(path.Child("set"),
			"policy sets nothing; a rule must override publish"))
	}
	if matchIsEmpty && setsSuppress {
		errs = append(errs, coreconfig.Invalid(path.Child("match"),
			"a policy with no match constraints and publish: false would silence every event; "+
				"scope this rule with at least one selector"))
	}
	return errs
}

// Resolve evaluates policies in order and returns the first match's settings
// applied over the defaults, or the bare defaults if nothing matches.
func (ps *PolicySet) Resolve(rc ResolveContext) Settings {
	for _, p := range ps.policies {
		if p.Match.matches(rc) {
			return ps.defaults.applyOverride(p.Set)
		}
	}
	return ps.defaults
}

func (s Selector) isEmpty() bool {
	return len(s.Categories) == 0 && len(s.Resources) == 0 && len(s.Operations) == 0 &&
		len(s.Actions) == 0 && len(s.Origins) == 0 && len(s.ActorTypes) == 0 &&
		len(s.Actors) == 0 && len(s.Entitlements) == 0 && len(s.Results) == 0
}

func (s Selector) matches(rc ResolveContext) bool {
	if len(s.Categories) > 0 && (rc.Operation == nil || !slices.Contains(s.Categories, rc.Operation.Category)) {
		return false
	}
	if len(s.Resources) > 0 && (rc.Operation == nil || !slices.Contains(s.Resources, rc.Operation.ResourceType)) {
		return false
	}
	if len(s.Operations) > 0 && (rc.Operation == nil || !slices.Contains(s.Operations, rc.Operation.ID)) {
		return false
	}
	if len(s.Actions) > 0 && (rc.Operation == nil || !slices.Contains(s.Actions, rc.Operation.Action)) {
		return false
	}
	if len(s.Origins) > 0 && !slices.Contains(s.Origins, rc.Origin) {
		return false
	}
	if len(s.ActorTypes) > 0 && !slices.Contains(s.ActorTypes, rc.Actor.Type) {
		return false
	}
	if len(s.Actors) > 0 && !slices.Contains(s.Actors, rc.Actor.ID) {
		return false
	}
	if len(s.Entitlements) > 0 && !entitlementsMatch(rc.Actor.Entitlements, s.Entitlements) {
		return false
	}
	if len(s.Results) > 0 && !slices.Contains(s.Results, rc.Result) {
		return false
	}
	return true
}

func entitlementsMatch(entitlements map[string][]string, want []string) bool {
	for _, values := range entitlements {
		for _, val := range values {
			if slices.Contains(want, val) {
				return true
			}
		}
	}
	return false
}

func (s Settings) applyOverride(set PartialSettings) Settings {
	result := s
	if set.Publish != nil {
		result.Publish = *set.Publish
	}
	return result
}

// clonePolicy deep-copies a Policy, including every slice/map in its Selector
// and every pointee in its PartialSettings, so PolicySet holds no aliases into
// caller-owned memory that could be mutated after construction.
func clonePolicy(p Policy) Policy {
	return Policy{
		Match: Selector{
			Categories:   slices.Clone(p.Match.Categories),
			Resources:    slices.Clone(p.Match.Resources),
			Operations:   slices.Clone(p.Match.Operations),
			Actions:      slices.Clone(p.Match.Actions),
			Origins:      slices.Clone(p.Match.Origins),
			ActorTypes:   slices.Clone(p.Match.ActorTypes),
			Actors:       slices.Clone(p.Match.Actors),
			Entitlements: slices.Clone(p.Match.Entitlements),
			Results:      slices.Clone(p.Match.Results),
		},
		Set: clonePartialSettings(p.Set),
	}
}

func clonePartialSettings(s PartialSettings) PartialSettings {
	var out PartialSettings
	if s.Publish != nil {
		v := *s.Publish
		out.Publish = &v
	}
	return out
}
