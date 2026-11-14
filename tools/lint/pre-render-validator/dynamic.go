// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package schemavalidator

import (
	"context"
	"encoding/json"
	"fmt"

	"gopkg.in/yaml.v3"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	sigsyaml "sigs.k8s.io/yaml"

	"github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/validation/schemautil"
	celvalidator "github.com/openchoreo/openchoreo/tools/lint/cel-validator"
)

// Views retain original YAML nodes and locations; snapshots never resolve to live definitions.
func specView(r Resource, kind string, spec *yaml.Node) Resource {
	d := *r.Document
	d.Kind = kind
	d.Spec = spec
	d.Root = &yaml.Node{Kind: yaml.MappingNode, Content: []*yaml.Node{{Kind: yaml.ScalarNode, Value: "spec"}, spec}}
	return Resource{Path: r.Path, Document: &d}
}
func nodeAt(node *yaml.Node, path ...string) *yaml.Node {
	for _, key := range path {
		node = findMappingValue(node, key)
	}
	return node
}
func rawNode(node *yaml.Node) (*runtime.RawExtension, error) {
	if node == nil {
		return nil, nil
	}
	raw, err := yaml.Marshal(node)
	if err != nil {
		return nil, err
	}
	raw, err = sigsyaml.YAMLToJSON(raw)
	if err != nil {
		return nil, err
	}
	return &runtime.RawExtension{Raw: raw}, nil
}
func sectionNode(node *yaml.Node) (*v1alpha1.SchemaSection, error) {
	raw, err := rawNode(node)
	if err != nil || raw == nil {
		return nil, err
	}
	var section v1alpha1.SchemaSection
	err = json.Unmarshal(raw.Raw, &section)
	return &section, err
}
func validateValueNode(r Resource, section *v1alpha1.SchemaSection, values *yaml.Node, owner string) []Finding {
	raw, err := rawNode(values)
	if err != nil {
		return []Finding{relationshipDecodeFinding(r, err)}
	}
	// Route every value relationship through the established validator and codes.
	line, column := position(r.Document.Spec)
	spec := &yaml.Node{Kind: yaml.MappingNode, Line: line, Column: column}
	if values != nil {
		spec.Content = []*yaml.Node{{Kind: yaml.ScalarNode, Value: valueParameters, Line: values.Line, Column: values.Column}, values}
	}
	view := specView(r, r.Document.Kind, spec)
	return validateParamsAgainstSchema(section, raw, owner, r.Path, view.Document, valueParameters)
}
func validateDefinitionNode(r Resource, kind string, spec *yaml.Node, checkCEL bool) []Finding {
	if spec == nil {
		return nil
	}
	view := specView(r, kind, spec)
	var def struct {
		Spec struct{ Parameters, EnvironmentConfigs *v1alpha1.SchemaSection }
	}
	if err := decodeTyped(view.Document, &def); err != nil {
		return []Finding{relationshipDecodeFinding(view, err)}
	}
	_, _, errs := schemautil.ExtractAndValidateSchemas(def.Spec.Parameters, def.Spec.EnvironmentConfigs, field.NewPath("spec"))
	findings := mapSchemaDefErrors(errs, r.Path, view.Document, kind, r.Document.Metadata.Name)
	if checkCEL && len(errs) == 0 {
		pair, err := celvalidator.ExtractSchemas(spec)
		if err == nil {
			found, err := celvalidator.ValidateDocument(view.Document, pair)
			if err == nil {
				for _, f := range found {
					findings = append(findings, Finding{Path: r.Path, Code: f.Code, Message: f.Message, Severity: SeverityError, Line: f.Range.Start.Line, Column: f.Range.Start.Column})
				}
			}
		}
	}
	return findings
}

type snapshotValidator struct{}

func (snapshotValidator) Supports(kind string) bool {
	return kind == valueComponentRelease || kind == valueResourceRelease || kind == valueProjectRelease
}
func (snapshotValidator) Validate(ctx context.Context, r Resource, _ *ValidationContext) []Finding {
	if ctx.Err() != nil {
		return nil
	}
	spec := r.Document.Spec
	key, kind, values := "componentType", valueComponentType, nodeAt(spec, "componentProfile", valueParameters)
	if r.Document.Kind == valueResourceRelease {
		key, kind, values = "resourceType", "ResourceType", nodeAt(spec, valueParameters)
	}
	if r.Document.Kind == valueProjectRelease {
		key, kind, values = "projectType", valueProjectType, nodeAt(spec, valueParameters)
	}
	definition := nodeAt(spec, key, "spec")
	findings := validateDefinitionNode(r, kind, definition, true)
	section, err := sectionNode(nodeAt(definition, valueParameters))
	if err == nil {
		findings = append(findings, validateValueNode(r, section, values, r.Document.Kind+" snapshot")...)
	}
	if r.Document.Kind != valueComponentRelease {
		return findings
	}
	traits := nodeAt(spec, valueTraits)
	definitions := map[string][]*yaml.Node{}
	if traits != nil && traits.Kind == yaml.SequenceNode {
		for _, trait := range traits.Content {
			k := scalar(nodeAt(trait, "kind"))
			name := scalar(nodeAt(trait, "name"))
			if k != valueTrait && k != valueClusterTrait {
				continue
			}
			defs := nodeAt(trait, "spec")
			definitions[k+":"+name] = append(definitions[k+":"+name], defs)
			findings = append(findings, validateDefinitionNode(r, k, defs, true)...)
		}
	}
	findings = append(findings, validateEmbeddedBindings(ctx, r, definition, values, nil, false, func(k, n string) []*yaml.Node { return definitions[k+":"+n] })...)
	instances := nodeAt(spec, "componentProfile", valueTraits)
	if instances != nil && instances.Kind == yaml.SequenceNode {
		for _, instance := range instances.Content {
			key := scalar(nodeAt(instance, "kind")) + ":" + scalar(nodeAt(instance, "name"))
			candidates := definitions[key]
			if len(candidates) > 1 {
				findings = append(findings, nodeFinding(r, instance, "pre-render-ambiguous-definition", "ambiguous embedded trait "+key))
				continue
			}
			if len(candidates) != 1 {
				findings = append(findings, nodeFinding(r, instance, "pre-render-missing-definition", "embedded trait snapshot not found: "+key))
				continue
			}
			section, err := sectionNode(nodeAt(candidates[0], valueParameters))
			if err != nil {
				continue
			}
			findings = append(findings, validateValueNode(r, section, nodeAt(instance, valueParameters), "embedded trait "+key)...)
			findings = append(findings, evaluateRulesAt(ctx, r, candidates[0], nodeAt(instance, valueParameters), nil, false, scalar(nodeAt(instance, "kind")))...)
		}
	}
	return findings
}
func scalar(n *yaml.Node) string {
	if n == nil {
		return ""
	}
	return n.Value
}
func nodeFinding(r Resource, n *yaml.Node, code, message string) Finding {
	if n == nil {
		n = r.Document.Spec
	}
	line, column := position(n)
	return Finding{Path: r.Path, Severity: SeverityError, Code: code, Message: message, Line: line, Column: column}
}

// Bindings pin releases, so their schemas come from the frozen snapshots.
type environmentValidator struct{}

func (environmentValidator) Supports(k string) bool {
	return k == "ReleaseBinding" || k == "ResourceReleaseBinding" || k == valueProjectReleaseBinding
}
func (environmentValidator) Validate(ctx context.Context, r Resource, c *ValidationContext) []Finding {
	pin, release, definition, values := "releaseName", valueComponentRelease, "componentType", "componentTypeEnvironmentConfigs"
	if r.Document.Kind == "ResourceReleaseBinding" {
		pin, release, definition, values = "resourceRelease", valueResourceRelease, "resourceType", "resourceTypeEnvironmentConfigs"
	}
	if r.Document.Kind == valueProjectReleaseBinding {
		pin, release, definition, values = "projectRelease", valueProjectRelease, "projectType", valueEnvironmentConfigs
	}
	name := scalar(nodeAt(r.Document.Spec, pin))
	if name == "" {
		return nil
	}
	candidates := c.Resolve(ResourceKey{r.Document.APIVersion, release, r.Document.Metadata.Namespace, name})
	if len(candidates) != 1 {
		if len(candidates) > 1 {
			return []Finding{nodeFinding(r, nodeAt(r.Document.Spec, pin), "pre-render-ambiguous-definition", "ambiguous release "+name)}
		}
		return nil
	}
	frozen := candidates[0].Document.Spec
	section, err := sectionNode(nodeAt(frozen, definition, "spec", valueEnvironmentConfigs))
	if err != nil {
		return nil
	}
	findings := validateEnvironmentNode(r, section, nodeAt(r.Document.Spec, values), release+" "+name)
	if release == valueComponentRelease {
		findings = append(findings, evaluateRulesAt(ctx, r, nodeAt(frozen, definition, "spec"), nodeAt(frozen, "componentProfile", valueParameters), nodeAt(r.Document.Spec, values), true, valueComponentType)...)
	}
	if release == valueProjectRelease {
		findings = append(findings, evaluateRulesAt(ctx, r, nodeAt(frozen, definition, "spec"), nodeAt(frozen, valueParameters), nodeAt(r.Document.Spec, values), true, valueProjectType)...)
	}
	if release != valueComponentRelease {
		return findings
	}
	findings = append(findings, validateEmbeddedBindings(ctx, r, nodeAt(frozen, "componentType", "spec"), nodeAt(frozen, "componentProfile", valueParameters), nodeAt(r.Document.Spec, values), true, func(k, n string) []*yaml.Node {
		var matches []*yaml.Node
		traits := nodeAt(frozen, valueTraits)
		if traits != nil && traits.Kind == yaml.SequenceNode {
			for _, t := range traits.Content {
				if scalar(nodeAt(t, "kind")) == k && scalar(nodeAt(t, "name")) == n {
					matches = append(matches, nodeAt(t, "spec"))
				}
			}
		}
		return matches
	})...)
	// Component-level trait configuration is keyed by instanceName; embedded
	// trait bindings are evaluated through the production context builder later.
	instances := nodeAt(frozen, "componentProfile", valueTraits)
	traits := nodeAt(frozen, valueTraits)
	if instances == nil || traits == nil || instances.Kind != yaml.SequenceNode || traits.Kind != yaml.SequenceNode {
		return findings
	}
	for _, instance := range instances.Content {
		var matches []*yaml.Node
		for _, trait := range traits.Content {
			if scalar(nodeAt(trait, "kind")) == scalar(nodeAt(instance, "kind")) && scalar(nodeAt(trait, "name")) == scalar(nodeAt(instance, "name")) {
				matches = append(matches, trait)
			}
		}
		if len(matches) != 1 {
			continue
		}
		section, err := sectionNode(nodeAt(matches[0], "spec", valueEnvironmentConfigs))
		if err != nil {
			continue
		}
		instanceName := scalar(nodeAt(instance, "instanceName"))
		findings = append(findings, validateEnvironmentNode(r, section, nodeAt(r.Document.Spec, "traitEnvironmentConfigs", instanceName), "trait "+instanceName)...)
		findings = append(findings, evaluateRulesAt(ctx, r, nodeAt(matches[0], "spec"), nodeAt(instance, valueParameters), nodeAt(r.Document.Spec, "traitEnvironmentConfigs", instanceName), true, valueTrait)...)
	}
	return findings
}
func validateEnvironmentNode(r Resource, section *v1alpha1.SchemaSection, values *yaml.Node, owner string) []Finding {
	findings := validateValueNode(r, section, values, owner)
	for i := range findings {
		if findings[i].Code == "schema-parameters-mismatch" {
			findings[i].Code = "schema-environment-configs-mismatch"
			findings[i].Message = fmt.Sprintf("environment configurations do not match %s schema: %s", owner, findings[i].Message)
		}
	}
	return findings
}
