// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	coreconfig "github.com/openchoreo/openchoreo/internal/config"
)

func TestResourceTreeDefaults_BuiltInRules(t *testing.T) {
	c := ResourceTreeDefaults()
	if len(c.Rules) != 4 {
		t.Fatalf("expected 4 built-in rules, got %d", len(c.Rules))
	}
	dep := c.Rules[0]
	if dep.Root.Kind != "Deployment" || dep.Root.Group != "apps" || dep.Root.Resource != "deployments" {
		t.Fatalf("unexpected first rule root: %+v", dep.Root)
	}
	rs := dep.Children[0]
	if !rs.Hide || rs.Kind.Kind != "ReplicaSet" {
		t.Fatalf("ReplicaSet child must be hidden: %+v", rs)
	}
	if rs.Children[0].Kind.Kind != "Pod" || rs.Children[0].Kind.Resource != "pods" {
		t.Fatalf("unexpected grandchild: %+v", rs.Children[0])
	}
	if c.Rules[1].Root.Kind != "CronJob" || c.Rules[1].Children[0].Kind.Kind != "Job" {
		t.Fatalf("expected CronJob -> Job rule, got %+v", c.Rules[1])
	}
	if c.Rules[2].Root.Kind != "Job" || c.Rules[2].Children[0].Kind.Kind != "Pod" {
		t.Fatalf("expected Job -> Pod rule, got %+v", c.Rules[2])
	}
	es := c.Rules[3]
	if es.Root.Kind != "ExternalSecret" || es.Root.Group != "external-secrets.io" || es.Root.Resource != "externalsecrets" {
		t.Fatalf("unexpected fourth rule root: %+v", es.Root)
	}
	if es.Children[0].Kind.Kind != "Secret" || es.Children[0].Kind.Group != "" || es.Children[0].Kind.Resource != "secrets" {
		t.Fatalf("expected ExternalSecret -> core Secret, got %+v", es.Children[0].Kind)
	}
	// Left nil so the core Secret default applies; spelling it out here would
	// pin the wrong thing if that default ever moves.
	if es.Children[0].MetadataOnly != nil {
		t.Errorf("Secret child must inherit the core Secret metadata_only default, got %v", *es.Children[0].MetadataOnly)
	}

	// Built-ins are ownerRef matched, spelled as the empty matcher that Task 7
	// normalizes to MatcherOwnerRef. The chart's files/resource-tree-builtin-rules.yaml
	// is a copy of these rules and must agree with this spelling.
	for _, rule := range c.Rules {
		forEachChild(rule.Children, func(child ChildRule) {
			if child.Matcher != "" {
				t.Errorf("built-in child %s must use the empty (ownerRef) matcher, got %q", child.Kind.Kind, child.Matcher)
			}
			if child.LabelSelector != nil {
				t.Errorf("built-in child %s must not carry a label_selector block", child.Kind.Kind)
			}
		})
	}
}

// forEachChild visits every child rule in the tree, at any depth.
func forEachChild(children []ChildRule, visit func(ChildRule)) {
	for _, child := range children {
		visit(child)
		forEachChild(child.Children, visit)
	}
}

func TestResourceTreeValidate(t *testing.T) {
	// Every wantErr matches message text, not just an error path, so a case
	// cannot pass on a different error that happens to name the same field.
	tests := []struct {
		name    string
		mutate  func(*ResourceTreeConfig)
		wantErr string // substring of the flattened error text; "" = valid
	}{
		{"defaults are valid", func(c *ResourceTreeConfig) {}, ""},
		{"missing resource", func(c *ResourceTreeConfig) { c.Rules[0].Children[0].Kind.Resource = "" }, "rules[0].children[0].kind.resource: is required"},
		{"missing version", func(c *ResourceTreeConfig) { c.Rules[0].Root.Version = "" }, "rules[0].root.version: is required"},
		{"dotted group and alpha version valid", func(c *ResourceTreeConfig) {
			c.Rules[0].Children[0].Kind = KindRef{Group: "gateway.networking.k8s.io", Version: "v1alpha1", Kind: "ExternalSecret", Resource: "externalsecrets"}
		}, ""},
		{"non-kube CRD version valid", func(c *ResourceTreeConfig) {
			c.Rules[0].Children[0].Kind = KindRef{Group: "example.com", Version: "v1gamma1", Kind: "Widget", Resource: "widgets"}
		}, ""},
		{"traversal resource", func(c *ResourceTreeConfig) { c.Rules[0].Children[0].Kind.Resource = "../../../../api/v1/secrets" },
			`rules[0].children[0].kind.resource: "../../../../api/v1/secrets" is not a valid resource name`},
		{"uppercase resource", func(c *ResourceTreeConfig) { c.Rules[0].Children[0].Kind.Resource = "Pods" },
			`rules[0].children[0].kind.resource: "Pods" is not a valid resource name`},
		{"malformed group", func(c *ResourceTreeConfig) { c.Rules[0].Root.Group = "apps/" },
			`rules[0].root.group: "apps/" is not a valid API group`},
		{"uppercase version", func(c *ResourceTreeConfig) { c.Rules[0].Root.Version = "V1" },
			`rules[0].root.version: "V1" is not a valid API version`},
		{"slashed version", func(c *ResourceTreeConfig) { c.Rules[0].Root.Version = "v1/" },
			`rules[0].root.version: "v1/" is not a valid API version`},
		{"unknown matcher", func(c *ResourceTreeConfig) { c.Rules[0].Children[0].Matcher = "objectRef" }, `matcher "objectRef" is not supported by this binary`},
		{"explicit ownerRef matcher", func(c *ResourceTreeConfig) { c.Rules[0].Children[0].Matcher = MatcherOwnerRef }, ""},
		{"root without children", func(c *ResourceTreeConfig) { c.Rules[0].Children = nil }, "at least one child is required"},
		{"depth cap", func(c *ResourceTreeConfig) {
			deep := ChildRule{Kind: KindRef{Version: "v1", Kind: "Pod", Resource: "pods"}}
			for i := 0; i < 10; i++ {
				deep = ChildRule{Kind: KindRef{Version: "v1", Kind: "Pod", Resource: "pods"}, Children: []ChildRule{deep}}
			}
			c.Rules[0].Children = []ChildRule{deep}
		}, fmt.Sprintf("child rule depth %d exceeds the maximum of %d", maxRuleDepth+1, maxRuleDepth)},
		{"edge cap", func(c *ResourceTreeConfig) {
			children := make([]ChildRule, 0, maxRuleEdges+1)
			for i := 0; i <= maxRuleEdges; i++ {
				children = append(children, ChildRule{
					Kind: KindRef{Version: "v1", Kind: "Pod", Resource: fmt.Sprintf("pods-%d", i)},
				})
			}
			// Drop the other rules so the reported total is exactly these edges.
			c.Rules = c.Rules[:1]
			c.Rules[0].Children = children
		}, fmt.Sprintf("child rules (%d) exceed the maximum of %d", maxRuleEdges+1, maxRuleEdges)},
		{"duplicate sibling edge", func(c *ResourceTreeConfig) {
			c.Rules[0].Children = append(c.Rules[0].Children, c.Rules[0].Children[0])
		}, `duplicate child rule for resource "replicasets" with matcher "ownerRef"`},
		// Two labelSelector siblings for the same GVR are rejected even though
		// their selectors differ: one edge per GVR keeps the edge unambiguous.
		{"duplicate labelSelector sibling with different selectors", func(c *ResourceTreeConfig) {
			owned := func(labelKey string) ChildRule {
				return ChildRule{
					Kind:    KindRef{Version: "v1", Kind: "Pod", Resource: "pods"},
					Matcher: MatcherLabelSelector,
					LabelSelector: &LabelSelectorSpec{
						MatchLabels: map[string]string{labelKey: TokenParentName},
					},
				}
			}
			c.Rules[0].Children = []ChildRule{owned("owner-a"), owned("owner-b")}
		}, `duplicate child rule for resource "pods" with matcher "labelSelector"`},
		{"duplicate root", func(c *ResourceTreeConfig) {
			c.Rules = append(c.Rules, c.Rules[0])
		}, "duplicate rule for root apps/Deployment"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := ResourceTreeDefaults()
			tt.mutate(&c)
			errs := c.Validate(coreconfig.NewPath("resource_tree"))
			assertValidationErr(t, errs, tt.wantErr)
		})
	}
}

// TestResourceTreeValidate_ReportsAllErrors pins the "validation is total" contract:
// every defect is reported, not just the first one found.
func TestResourceTreeValidate_ReportsAllErrors(t *testing.T) {
	c := ResourceTreeDefaults()
	c.Rules[0].Root.Version = ""
	c.Rules[1].Children[0].Kind.Resource = ""

	errs := c.Validate(coreconfig.NewPath("resource_tree"))
	got := errs.Error()
	for _, want := range []string{"rules[0].root.version", "rules[1].children[0].kind.resource"} {
		if !strings.Contains(got, want) {
			t.Errorf("expected an error for %s, got:\n%s", want, got)
		}
	}
}

// TestValidateDuplicateRootMessageIsGeneric pins the wording. The validator has
// no provenance for where a rule came from, so an error naming a Helm values key
// is wrong for the operator/operator case and couples a server message to a
// chart the server knows nothing about.
func TestValidateDuplicateRootMessageIsGeneric(t *testing.T) {
	deployment := KindRef{Group: "apps", Version: "v1", Kind: "Deployment", Resource: "deployments"}
	pod := ChildRule{Kind: KindRef{Version: "v1", Kind: "Pod", Resource: "pods"}}

	cfg := ResourceTreeConfig{Rules: []ResourceTreeRule{
		{Root: deployment, Children: []ChildRule{pod}},
		{Root: deployment, Children: []ChildRule{pod}},
	}}

	errs := cfg.Validate(coreconfig.NewPath("resource_tree"))
	if len(errs) == 0 {
		t.Fatal("a duplicate root must be rejected")
	}

	message := errs.Error()
	if !strings.Contains(message, "duplicate rule for root apps/Deployment") {
		t.Errorf("the error must name the duplicated root, got: %s", message)
	}
	for _, forbidden := range []string{"builtInRules", "Helm", "helm"} {
		if strings.Contains(message, forbidden) {
			t.Errorf("the error must not name %q; the validator has no provenance for where a rule came from, got: %s",
				forbidden, message)
		}
	}
}

func TestResourceTreeValidate_LabelSelector(t *testing.T) {
	// base: a valid labelSelector rule (the Envoy Gateway shape from the design doc)
	valid := ResourceTreeRule{
		Root: KindRef{Group: "gateway.networking.k8s.io", Version: "v1", Kind: "Gateway", Resource: "gateways"},
		Children: []ChildRule{{
			Kind:    KindRef{Group: "apps", Version: "v1", Kind: "Deployment", Resource: "deployments"},
			Matcher: MatcherLabelSelector,
			LabelSelector: &LabelSelectorSpec{
				MatchLabels: map[string]string{
					"gateway.envoyproxy.io/owning-gateway-name":      "${parent.metadata.name}",
					"gateway.envoyproxy.io/owning-gateway-namespace": "${parent.metadata.namespace}",
				},
				Namespaces: []string{"envoy-gateway-system"},
			},
		}},
	}
	tests := []struct {
		name    string
		mutate  func(*ChildRule)
		wantErr string // "" = valid
	}{
		{"valid rule passes", func(c *ChildRule) {}, ""},
		{"empty namespaces defaults to parent ns, still valid", func(c *ChildRule) { c.LabelSelector.Namespaces = nil }, ""},
		{"labelSelector without block", func(c *ChildRule) { c.LabelSelector = nil }, "label_selector is required for matcher labelSelector"},
		{"ownerRef with block", func(c *ChildRule) { c.Matcher = "" }, "label_selector is only valid with matcher labelSelector"},
		{"empty match_labels", func(c *ChildRule) { c.LabelSelector.MatchLabels = map[string]string{} }, "match_labels: is required"},
		{"no parent-derived value", func(c *ChildRule) {
			c.LabelSelector.MatchLabels = map[string]string{"app": "static"}
		}, "at least one value derived from the parent is required"},
		{"qualified label key", func(c *ChildRule) {
			c.LabelSelector.MatchLabels = map[string]string{"example.com/owner": TokenParentName}
		}, ""},
		{"invalid label key", func(c *ChildRule) {
			c.LabelSelector.MatchLabels = map[string]string{"Bad Key!": TokenParentName}
		}, "is not a valid label key"},
		{"invalid token-free label value", func(c *ChildRule) {
			c.LabelSelector.MatchLabels["static"] = "Bad Value!"
		}, "is not a valid label value"},
		{"unknown token", func(c *ChildRule) {
			c.LabelSelector.MatchLabels["x"] = "${parent.spec.foo}"
		}, `unknown substitution token "${parent.spec.foo}"`},
		{"recognized token in match_labels key", func(c *ChildRule) {
			c.LabelSelector.MatchLabels[TokenParentName] = "static"
		}, "substitution tokens are not supported in match_labels keys"},
		{"unknown token in match_labels key", func(c *ChildRule) {
			c.LabelSelector.MatchLabels["owner-${parent.spec.foo}"] = "static"
		}, "substitution tokens are not supported in match_labels keys"},
		{"wildcard namespace", func(c *ChildRule) { c.LabelSelector.Namespaces = []string{"*"} }, "wildcard namespaces are not allowed"},
		{"embedded wildcard namespace", func(c *ChildRule) { c.LabelSelector.Namespaces = []string{"team-*"} }, "wildcard namespaces are not allowed"},
		{"invalid DNS namespace", func(c *ChildRule) { c.LabelSelector.Namespaces = []string{"Team_A"} }, "is not a valid namespace name"},
		{"recognized token in namespaces", func(c *ChildRule) {
			c.LabelSelector.Namespaces = []string{TokenParentNamespace}
		}, "substitution tokens are not supported in namespaces"},
		{"unknown token in namespaces", func(c *ChildRule) {
			c.LabelSelector.Namespaces = []string{"ns-${parent.spec.foo}"}
		}, "substitution tokens are not supported in namespaces"},
		{"empty namespace entry", func(c *ChildRule) { c.LabelSelector.Namespaces = []string{""} }, "namespaces[0]: must not be empty"},
		{"duplicate namespace", func(c *ChildRule) {
			c.LabelSelector.Namespaces = []string{"team-a", "team-a"}
		}, `duplicate namespace "team-a"`},
		{"too many namespaces", func(c *ChildRule) {
			for i := 0; i < 9; i++ {
				c.LabelSelector.Namespaces = append(c.LabelSelector.Namespaces, fmt.Sprintf("ns-%d", i))
			}
		}, fmt.Sprintf("at most %d namespaces are allowed", maxSelectorNamespaces)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := ResourceTreeConfig{Rules: []ResourceTreeRule{deepCopyRule(valid)}} // helper: copy so mutations don't leak
			tt.mutate(&c.Rules[0].Children[0])
			errs := c.Validate(coreconfig.NewPath("resource_tree"))
			assertValidationErr(t, errs, tt.wantErr)
		})
	}
}

// TestMaxSelectorNamespacesIsEight pins the namespace cap to its literal value.
// The "too many namespaces" case above covers the behavior but builds its
// expectation from the constant, so raising the cap by one would leave it
// passing; this pins the value itself. maxSelectorNamespaces now aliases
// protocol.MaxSelectorNamespaces, which carries the mirror-image pin, so this
// check is currently redundant — it earns its place by failing if the alias is
// ever replaced by a config-local literal that drifts from the cap the agent
// enforces.
func TestMaxSelectorNamespacesIsEight(t *testing.T) {
	if maxSelectorNamespaces != 8 {
		t.Errorf("maxSelectorNamespaces = %d, want 8; protocol.MaxSelectorNamespaces must be changed to match",
			maxSelectorNamespaces)
	}
}

func TestResourceTreeValidateRawKeys(t *testing.T) {
	raw := map[string]any{"rules": []any{map[string]any{
		"root":     map[string]any{"group": "apps", "version": "v1", "kind": "Deployment", "resource": "deployments"},
		"children": []any{map[string]any{"kind": map[string]any{"version": "v1", "kind": "Pod", "resource": "pods"}, "metadata_onlyy": true}},
	}}}
	errs := ResourceTreeConfig{}.validateRawKeys(raw)
	if len(errs) == 0 {
		t.Fatal("typo'd key metadata_onlyy must be rejected")
	}
	if got := errs.Error(); !strings.Contains(got, "metadata_onlyy") {
		t.Errorf("expected the error to name the unknown key, got:\n%s", got)
	}
}

func TestResourceTreeValidateRawKeys_KnownKeysAccepted(t *testing.T) {
	raw := map[string]any{"rules": []any{map[string]any{
		"root": map[string]any{"group": "gateway.networking.k8s.io", "version": "v1", "kind": "Gateway", "resource": "gateways"},
		"children": []any{map[string]any{
			"kind":    map[string]any{"group": "apps", "version": "v1", "kind": "Deployment", "resource": "deployments"},
			"matcher": "labelSelector",
			"label_selector": map[string]any{
				"match_labels": map[string]any{"owner": "${parent.metadata.name}"},
				"namespaces":   []any{"envoy-gateway-system"},
			},
			"metadata_only": false,
			"hide":          true,
			"children": []any{map[string]any{
				"kind": map[string]any{"version": "v1", "kind": "Pod", "resource": "pods"},
			}},
		}},
	}}}
	if errs := (ResourceTreeConfig{}).validateRawKeys(raw); len(errs) != 0 {
		t.Fatalf("expected no unknown-key errors, got:\n%s", errs.Error())
	}
}

// TestResourceTreeValidateRawKeys_NullSection covers the M2 cases. Because the
// loader merges the built-in defaults before the config file, an ABSENT section
// leaves a populated map in the raw config; a nil here is therefore an explicit
// `resource_tree:` with no value, which must be rejected rather than silently
// disabling every built-in rule. The `rules:`-null case is the same trap one
// level down, while an explicit empty list is the supported opt-out.
func TestResourceTreeValidateRawKeys_NullSection(t *testing.T) {
	tests := []struct {
		name    string
		raw     any
		wantErr string // "" = valid
	}{
		{"null section", nil, "resource_tree: is null"},
		{"null rules", map[string]any{"rules": nil}, "resource_tree.rules: is null"},
		{"explicit empty rules is the opt-out", map[string]any{"rules": []any{}}, ""},
		{"non-mapping section", "nonsense", "must be a mapping"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertValidationErr(t, ResourceTreeConfig{}.validateRawKeys(tt.raw), tt.wantErr)
		})
	}
}

// TestResourceTreeNullSectionRejectedEndToEnd proves the same through the real
// loader: an absent section keeps the built-in rules, a null section or null
// rules is rejected, and an explicit empty list validates as the opt-out.
func TestResourceTreeNullSectionRejectedEndToEnd(t *testing.T) {
	tests := []struct {
		name      string
		body      string
		wantErr   string
		wantRules int // only checked when wantErr == ""
	}{
		{"absent keeps built-ins", "server:\n  port: 8080\n", "", len(ResourceTreeDefaults().Rules)},
		{"null section rejected", "resource_tree:\n", "resource_tree: is null", 0},
		{"null rules rejected", "resource_tree:\n  rules:\n", "resource_tree.rules: is null", 0},
		{"explicit empty is the opt-out", "resource_tree:\n  rules: []\n", "", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			p := filepath.Join(dir, "config.yaml")
			if err := os.WriteFile(p, []byte(tt.body), 0o600); err != nil {
				t.Fatal(err)
			}
			loader, err := NewLoader(p, nil)
			if err != nil {
				t.Fatalf("loader: %v", err)
			}
			var cfg Config
			if err := loader.Unmarshal("", &cfg); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			err = cfg.ValidateWithRaw(loader)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("expected valid config, got: %v", err)
				}
				if got := len(cfg.ResourceTree.Rules); got != tt.wantRules {
					t.Errorf("rule count = %d, want %d", got, tt.wantRules)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("expected error containing %q, got: %v", tt.wantErr, err)
			}
		})
	}
}

// assertValidationErr asserts that the flattened error text contains wantErr,
// or that validation produced no errors at all when wantErr is empty.
func assertValidationErr(t *testing.T, errs coreconfig.ValidationErrors, wantErr string) {
	t.Helper()
	if wantErr == "" {
		if len(errs) != 0 {
			t.Fatalf("expected no validation errors, got:\n%s", errs.Error())
		}
		return
	}
	if len(errs) == 0 {
		t.Fatalf("expected a validation error containing %q, got none", wantErr)
	}
	if got := errs.Error(); !strings.Contains(got, wantErr) {
		t.Fatalf("expected a validation error containing %q, got:\n%s", wantErr, got)
	}
}

// deepCopyRule copies a rule so that a table case mutating one copy cannot leak
// into another.
func deepCopyRule(r ResourceTreeRule) ResourceTreeRule {
	out := r
	out.Children = deepCopyChildren(r.Children)
	return out
}

func deepCopyChildren(children []ChildRule) []ChildRule {
	if len(children) == 0 {
		return nil
	}
	out := make([]ChildRule, len(children))
	for i, child := range children {
		copied := child
		if child.LabelSelector != nil {
			selector := *child.LabelSelector
			selector.MatchLabels = maps.Clone(child.LabelSelector.MatchLabels)
			selector.Namespaces = slices.Clone(child.LabelSelector.Namespaces)
			copied.LabelSelector = &selector
		}
		if child.MetadataOnly != nil {
			metadataOnly := *child.MetadataOnly
			copied.MetadataOnly = &metadataOnly
		}
		copied.Children = deepCopyChildren(child.Children)
		out[i] = copied
	}
	return out
}

// TestResourceTreeTagsAgree pins the two tag sets on these structs to the same
// key. The server loads them through koanf; hack/resource-tree-builtin-rules
// speaks json instead, through sigs.k8s.io/yaml, which honors json tags only.
//
// That tool marshals these structs into the chart file the koanf loader later
// reads, so a spelling divergence would write a key the server ignores. On a
// field the built-ins leave zero-valued it is invisible to both nets around it
// — the golden test regenerates the file to match the wrong spelling, and the
// koanf round-trip mirror test has nothing emitted to check — which makes this
// test the only thing pinning it.
func TestResourceTreeTagsAgree(t *testing.T) {
	seen := make(map[reflect.Type]bool)

	var check func(typ reflect.Type)
	check = func(typ reflect.Type) {
		for typ.Kind() == reflect.Pointer || typ.Kind() == reflect.Slice || typ.Kind() == reflect.Map {
			typ = typ.Elem()
		}
		if typ.Kind() != reflect.Struct || seen[typ] {
			return
		}
		seen[typ] = true

		for i := range typ.NumField() {
			field := typ.Field(i)
			koanfTag := field.Tag.Get("koanf")
			jsonTag, _, _ := strings.Cut(field.Tag.Get("json"), ",")

			switch {
			case koanfTag == "":
				t.Errorf("%s.%s has no koanf tag", typ.Name(), field.Name)
			case jsonTag == "":
				t.Errorf("%s.%s has no json tag; the RBAC generator would not see it", typ.Name(), field.Name)
			case koanfTag != jsonTag:
				t.Errorf("%s.%s: koanf tag %q and json tag %q disagree", typ.Name(), field.Name, koanfTag, jsonTag)
			}

			check(field.Type)
		}
	}

	check(reflect.TypeFor[ResourceTreeConfig]())
}
