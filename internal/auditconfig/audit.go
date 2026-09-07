// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

// Package auditconfig holds the koanf-decode-and-validate layer for a
// service's audit.policies configuration. It knows nothing about any one
// service's own operation table — a caller supplies its own Vocabulary (see
// NewVocabulary) so the same decode/validate logic works for openchoreo-api
// and observer alike.
package auditconfig

import (
	"fmt"

	"github.com/openchoreo/openchoreo/internal/config"
	"github.com/openchoreo/openchoreo/internal/server/middleware/audit"
)

// AuditConfig defines audit logging settings.
type AuditConfig struct {
	// Enabled enables audit event emission.
	Enabled bool `koanf:"enabled"`
	// Defaults are the policy settings applied when no Policies rule matches.
	Defaults PolicyDefaultsConfig `koanf:"defaults"`
	// Policies is an ordered, first-match-wins list of overrides. This block
	// is config-file-only: the env-var loader has no way to address list
	// elements or fields nested inside a list element.
	Policies []PolicyRuleConfig `koanf:"policies"`
}

// PolicyDefaultsConfig is the koanf-decoded shape of audit.defaults. Publish
// is the only field today.
//
// Publish: false is deliberate allowlist mode — only a policies rule with
// set.publish: true re-enables publishing, so this default is exempt from
// validatePolicy's guard against an accidental blanket-suppress rule. In
// allowlist mode, keep every match selector exact: a typo fails open under
// Publish: true but fails closed (silently drops events) here.
type PolicyDefaultsConfig struct {
	Publish bool `koanf:"publish"`
}

// PolicyRuleConfig is the koanf-decoded shape of one audit.policies entry. Set
// is deliberately untyped: a typed struct decoded by mapstructure silently
// drops unknown keys, and this needs to reject them with a specific message
// (see parsePartialSettings) — most importantly, a `category` key, which is
// stamped from the operation and never operator-configurable.
type PolicyRuleConfig struct {
	Match SelectorConfig `koanf:"match"`
	Set   map[string]any `koanf:"set"`
}

// SelectorConfig is the koanf-decoded shape of a policy's match block.
type SelectorConfig struct {
	Categories   []string `koanf:"categories"`
	Resources    []string `koanf:"resources"`
	Operations   []string `koanf:"operations"`
	Actions      []string `koanf:"actions"`
	Origins      []string `koanf:"origins"`
	ActorTypes   []string `koanf:"actor_types"`
	Actors       []string `koanf:"actors"`
	Entitlements []string `koanf:"entitlements"`
	Results      []string `koanf:"results"`
}

// AuditDefaults returns the default audit configuration.
func AuditDefaults() AuditConfig {
	return AuditConfig{
		Enabled: true,
		Defaults: PolicyDefaultsConfig{
			Publish: true,
		},
	}
}

var (
	validCategories = []string{
		string(audit.CategoryManagement), string(audit.CategoryAuthorization),
	}
	validOrigins = []string{string(audit.OriginAPI), string(audit.OriginMCP)}
	validResults = []string{
		string(audit.ResultSuccess), string(audit.ResultFailure),
		string(audit.ResultDenied), string(audit.ResultUnauthenticated),
	}
)

// Vocabulary is the set of selector values audit.policies[].match may name,
// derived from a service's own operation table — the same table
// BuildPatternMap cross-checks REST routes against — so a selector naming a
// resource/operation/action that no Operation actually produces fails at
// startup instead of quietly matching nothing.
type Vocabulary struct {
	Resources    []string
	OperationIDs []string
	Actions      []string
}

// NewVocabulary derives a Vocabulary from ops — the distinct ResourceType, ID
// and Action values across every audited Operation a service defines.
func NewVocabulary(ops []audit.Operation) Vocabulary {
	seenResources := make(map[string]bool)
	var resources []string
	operationIDs := make([]string, 0, len(ops))
	actions := make([]string, 0, len(ops))
	for _, op := range ops {
		if !seenResources[op.ResourceType] {
			seenResources[op.ResourceType] = true
			resources = append(resources, op.ResourceType)
		}
		operationIDs = append(operationIDs, op.ID)
		actions = append(actions, op.Action)
	}
	return Vocabulary{Resources: resources, OperationIDs: operationIDs, Actions: actions}
}

// buildPolicySet converts the koanf-decoded config into the core audit types
// and runs them through audit.NewPolicySet, which owns the selector
// admissibility gate. Validate calls this and discards the *PolicySet,
// keeping only the errors; BuildPolicySet calls it and keeps the *PolicySet.
// This is the single conversion — there is no second, independently
// maintained path from config to core types. knownActorTypes validates
// match.actor_types — see SecurityConfig.KnownActorTypes.
func (c *AuditConfig) buildPolicySet(
	path *config.Path, vocab Vocabulary, knownActorTypes []string,
) (*audit.PolicySet, config.ValidationErrors) {
	var errs config.ValidationErrors

	defaults := audit.Settings{
		Publish: c.Defaults.Publish,
	}

	policiesPath := path.Child("policies")
	policies := make([]audit.Policy, 0, len(c.Policies))
	for i, rule := range c.Policies {
		rulePath := policiesPath.Index(i)

		selector, selErrs := rule.Match.toSelector(rulePath.Child("match"), vocab, knownActorTypes)
		errs = append(errs, selErrs...)

		set, setErrs := parsePartialSettings(rulePath.Child("set"), rule.Set)
		errs = append(errs, setErrs...)

		policies = append(policies, audit.Policy{Match: selector, Set: set})
	}

	if len(errs) > 0 {
		return nil, errs
	}

	ps, psErrs := audit.NewPolicySet(path, defaults, policies)
	if len(psErrs) > 0 {
		return nil, psErrs
	}
	return ps, nil
}

// Validate validates the audit configuration, including every policy's
// selector admissibility. vocab validates match.resources/operations/actions;
// knownActorTypes validates match.actor_types — see SecurityConfig.KnownActorTypes.
func (c *AuditConfig) Validate(path *config.Path, vocab Vocabulary, knownActorTypes []string) config.ValidationErrors {
	_, errs := c.buildPolicySet(path, vocab, knownActorTypes)
	return errs
}

// BuildPolicySet is the production entry point used at wiring time. Startup
// should already have failed via Validate before this runs; a non-nil error
// here is a defensive second check, not the primary gate.
func (c *AuditConfig) BuildPolicySet(vocab Vocabulary, knownActorTypes []string) (*audit.PolicySet, error) {
	ps, errs := c.buildPolicySet(config.NewPath("audit"), vocab, knownActorTypes)
	return ps, errs.OrNil()
}

func (sc SelectorConfig) toSelector(
	path *config.Path, vocab Vocabulary, knownActorTypes []string,
) (audit.Selector, config.ValidationErrors) {
	var errs config.ValidationErrors

	categories := make([]audit.Category, 0, len(sc.Categories))
	for i, c := range sc.Categories {
		if fe := config.MustBeOneOf(path.Child("categories").Index(i), c, validCategories); fe != nil {
			errs = append(errs, fe)
			continue
		}
		categories = append(categories, audit.Category(c))
	}

	origins := make([]audit.Origin, 0, len(sc.Origins))
	for i, o := range sc.Origins {
		if fe := config.MustBeOneOf(path.Child("origins").Index(i), o, validOrigins); fe != nil {
			errs = append(errs, fe)
			continue
		}
		origins = append(origins, audit.Origin(o))
	}

	results := make([]audit.Result, 0, len(sc.Results))
	for i, r := range sc.Results {
		if fe := config.MustBeOneOf(path.Child("results").Index(i), r, validResults); fe != nil {
			errs = append(errs, fe)
			continue
		}
		results = append(results, audit.Result(r))
	}

	for i, r := range sc.Resources {
		if fe := config.MustBeOneOf(path.Child("resources").Index(i), r, vocab.Resources); fe != nil {
			errs = append(errs, fe)
		}
	}

	for i, op := range sc.Operations {
		if fe := config.MustBeOneOf(path.Child("operations").Index(i), op, vocab.OperationIDs); fe != nil {
			errs = append(errs, fe)
		}
	}

	for i, a := range sc.Actions {
		if fe := config.MustBeOneOf(path.Child("actions").Index(i), a, vocab.Actions); fe != nil {
			errs = append(errs, fe)
		}
	}

	for i, at := range sc.ActorTypes {
		if fe := config.MustBeOneOf(path.Child("actor_types").Index(i), at, knownActorTypes); fe != nil {
			errs = append(errs, fe)
		}
	}

	if len(errs) > 0 {
		return audit.Selector{}, errs
	}

	return audit.Selector{
		Categories:   categories,
		Resources:    sc.Resources,
		Operations:   sc.Operations,
		Actions:      sc.Actions,
		Origins:      origins,
		ActorTypes:   sc.ActorTypes,
		Actors:       sc.Actors,
		Entitlements: sc.Entitlements,
		Results:      results,
	}, nil
}

// parsePartialSettings decodes a policy's untyped `set:` map into
// audit.PartialSettings, rejecting unrecognized keys with a specific message
// — most importantly "category", which is stamped from the operation and
// never operator-configurable.
func parsePartialSettings(path *config.Path, set map[string]any) (audit.PartialSettings, config.ValidationErrors) {
	var errs config.ValidationErrors
	var out audit.PartialSettings

	for key, value := range set {
		switch key {
		case "publish":
			b, ok := value.(bool)
			if !ok {
				errs = append(errs, config.Invalid(path.Child(key), "must be a boolean"))
				continue
			}
			out.Publish = &b
		case "category", "categories":
			errs = append(errs, config.Invalid(path.Child(key),
				"category is stamped from the operation, not operator-configurable; remove this key"))
		default:
			errs = append(errs, config.Invalid(path.Child(key), fmt.Sprintf("unknown setting %q", key)))
		}
	}

	return out, errs
}
