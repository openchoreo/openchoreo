// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package codecompletion

import (
	"encoding/json"
	"strings"

	"gopkg.in/yaml.v3"

	celvalidator "github.com/openchoreo/openchoreo/tools/lint/cel-validator"
	"github.com/openchoreo/openchoreo/tools/lint/indexer"
	"github.com/openchoreo/openchoreo/tools/lint/parser"
)

type completionRelationship struct{ reference, values, kind, other string }

var completionRelationships = map[string]completionRelationship{
	valueComponent: {"componentType", "parameters", valueComponentType, valueClusterComponentType},
	"Resource":     {"type", "parameters", "ResourceType", "ClusterResourceType"},
	"Project":      {"type", "parameters", "ProjectType", "ClusterProjectType"},
	"WorkflowRun":  {"workflow", "workflow.parameters", "ClusterWorkflow", "Workflow"},
}

type snapshotRelationship struct{ definition, values string }

var snapshotRelationships = map[string]snapshotRelationship{
	"ComponentRelease": {"componentType", "componentProfile.parameters"},
	"ResourceRelease":  {"resourceType", "parameters"},
	"ProjectRelease":   {"projectType", "parameters"},
}

type bindingRelationship struct{ pin, release, definition, values string }

var bindingRelationships = map[string]bindingRelationship{
	valueReleaseBinding:      {"releaseName", "ComponentRelease", "componentType", "componentTypeEnvironmentConfigs"},
	"ResourceReleaseBinding": {"resourceRelease", "ResourceRelease", "resourceType", "resourceTypeEnvironmentConfigs"},
	"ProjectReleaseBinding":  {"projectRelease", "ProjectRelease", "projectType", "environmentConfigs"},
}

func dynamicAt(n *yaml.Node, path string) *yaml.Node {
	for _, part := range strings.Split(path, ".") {
		n = dynamicNode(n, part)
	}
	return n
}
func dynamicScalar(n *yaml.Node) string {
	if n == nil {
		return ""
	}
	return n.Value
}
func uniqueDefinition(docs []indexer.SnapshotDocument, doc *parser.DocumentNode, kind, name string) *yaml.Node {
	if kind == "" || name == "" {
		return nil
	}
	normalized := indexer.ReferenceName(kind, name)
	var result *yaml.Node
	for _, entry := range docs {
		d := entry.Document
		if d.APIVersion != doc.APIVersion || d.Kind != kind || d.Metadata.Name != normalized {
			continue
		}
		if !strings.HasPrefix(kind, "Cluster") && indexer.NamespaceOrDefault(d.Metadata.Namespace) != indexer.NamespaceOrDefault(doc.Metadata.Namespace) {
			continue
		}
		if normalized != name && dynamicScalar(dynamicNode(d.Spec, "workloadType")) != strings.Split(name, "/")[0] {
			continue
		}
		if result != nil {
			return nil
		}
		result = d.Spec
	}
	return result
}
func selectedInstance(seq *yaml.Node, line int) *yaml.Node {
	if seq == nil {
		return nil
	}
	var result *yaml.Node
	for _, n := range seq.Content {
		if n.Line <= line {
			result = n
		}
	}
	return result
}
func frozenTrait(frozen, instance *yaml.Node) *yaml.Node {
	if instance == nil || dynamicScalar(dynamicNode(instance, "name")) == "" {
		return nil
	}
	seq := dynamicNode(frozen, "traits")
	if seq == nil {
		return nil
	}
	var result *yaml.Node
	for _, n := range seq.Content {
		if dynamicScalar(dynamicNode(n, valueKind)) == dynamicScalar(dynamicNode(instance, valueKind)) && dynamicScalar(dynamicNode(n, "name")) == dynamicScalar(dynamicNode(instance, "name")) {
			if result != nil {
				return nil
			}
			result = dynamicNode(n, "spec")
		}
	}
	return result
}

// dynamicRelationship follows API-defined relationships. Flexible fields alone
// never imply a schema; pinned bindings always use frozen release definitions.
func dynamicRelationship(doc *parser.DocumentNode, path string, line int, docs []indexer.SnapshotDocument) (string, *yaml.Node, bool) {
	// Definition-level Trait bindings have the same linked schema contract as
	// Component Trait instances, including their environment configuration.
	if (doc.Kind == valueComponentType || doc.Kind == valueClusterComponentType) && strings.HasPrefix(path, "spec.traits") {
		return definitionTraitRelationship(doc, path, line, docs)
	}
	if rel, ok := completionRelationships[doc.Kind]; ok {
		if doc.Kind == valueComponent && strings.HasPrefix(path, "spec.workflow") {
			rel = completionRelationship{"workflow", "workflow.parameters", "ClusterWorkflow", "Workflow"}
		}
		ref := dynamicAt(doc.Spec, rel.reference)
		root := "spec." + rel.values
		if doc.Kind == valueComponent && strings.HasPrefix(path, "spec.traits") {
			ref = selectedInstance(dynamicNode(doc.Spec, "traits"), line)
			root = "spec.traits.parameters"
			rel.kind, rel.other = valueTrait, "ClusterTrait"
		}
		kind := dynamicScalar(dynamicNode(ref, valueKind))
		if kind == "" {
			kind = rel.kind
		}
		if kind != rel.kind && kind != rel.other {
			return "", nil, false
		}
		return root, uniqueDefinition(docs, doc, kind, dynamicScalar(dynamicNode(ref, "name"))), false
	}
	if rel, ok := snapshotRelationships[doc.Kind]; ok {
		if doc.Kind == "ComponentRelease" && strings.HasPrefix(path, "spec.componentType.spec.traits") {
			instance := selectedInstance(dynamicAt(doc.Spec, "componentType.spec.traits"), line)
			section := "parameters"
			environment := strings.Contains(path, ".environmentConfigs")
			if environment {
				section = "environmentConfigs"
			}
			return "spec.componentType.spec.traits." + section, frozenTrait(doc.Spec, instance), environment
		}

		if doc.Kind == "ComponentRelease" && strings.HasPrefix(path, "spec.componentProfile.traits") {
			instance := selectedInstance(dynamicAt(doc.Spec, "componentProfile.traits"), line)
			return "spec.componentProfile.traits.parameters", frozenTrait(doc.Spec, instance), false
		}
		return "spec." + rel.values, dynamicAt(doc.Spec, rel.definition+".spec"), false
	}
	if rel, ok := bindingRelationships[doc.Kind]; ok {
		frozen := uniqueDefinition(docs, doc, rel.release, dynamicScalar(dynamicNode(doc.Spec, rel.pin)))
		if frozen == nil {
			return "", nil, false
		}
		if doc.Kind == valueReleaseBinding && path == "spec.traitEnvironmentConfigs" {
			return "spec.traitEnvironmentConfigs", traitConfigMapSchema(frozen), true
		}
		if doc.Kind == valueReleaseBinding && strings.HasPrefix(path, "spec.traitEnvironmentConfigs.") {
			parts := strings.Split(path, ".")
			name := parts[2]
			instances := dynamicAt(frozen, "componentProfile.traits")
			if instances != nil {
				var instance *yaml.Node
				for _, n := range instances.Content {
					if dynamicScalar(dynamicNode(n, "instanceName")) == name {
						if instance != nil {
							return "", nil, false
						}
						instance = n
					}
				}
				return "spec.traitEnvironmentConfigs." + name, frozenTrait(frozen, instance), true
			}
		}
		return "spec." + rel.values, dynamicAt(frozen, rel.definition+".spec"), true
	}
	return "", nil, false
}

// The map keys here are actual instance names from the pinned release, not
// guessed children of a preserved object. Resolve each section before nesting it
// so local schema references retain their original meaning.
func traitConfigMapSchema(frozen *yaml.Node) *yaml.Node {
	instances := dynamicAt(frozen, "componentProfile.traits")
	if instances == nil {
		return nil
	}
	props := map[string]any{}
	for _, instance := range instances.Content {
		name := dynamicScalar(dynamicNode(instance, "instanceName"))
		if name == "" {
			continue
		}
		if _, exists := props[name]; exists {
			return nil
		}
		spec := frozenTrait(frozen, instance)
		if spec == nil {
			continue
		}
		pair, err := celvalidator.ExtractSchemas(spec)
		if err != nil || pair.EnvironmentConfigs == nil {
			continue
		}
		props[name] = pair.EnvironmentConfigs.ToKubeOpenAPI()
	}
	if len(props) == 0 {
		return nil
	}
	data, err := json.Marshal(map[string]any{"environmentConfigs": map[string]any{"openAPIV3Schema": map[string]any{"type": valueObject, "properties": props}}})
	if err != nil {
		return nil
	}
	var root yaml.Node
	if yaml.Unmarshal(data, &root) != nil || len(root.Content) != 1 {
		return nil
	}
	return root.Content[0]
}

func definitionTraitRelationship(doc *parser.DocumentNode, path string, line int, docs []indexer.SnapshotDocument) (string, *yaml.Node, bool) {
	instance := selectedInstance(dynamicNode(doc.Spec, "traits"), line)
	kind := dynamicScalar(dynamicNode(instance, valueKind))
	if kind == "" {
		kind = valueTrait
	}
	if kind != valueTrait && kind != "ClusterTrait" {
		return "", nil, false
	}
	section := "parameters"
	environment := strings.Contains(path, ".environmentConfigs")
	if environment {
		section = "environmentConfigs"
	}
	return "spec.traits." + section, uniqueDefinition(docs, doc, kind, dynamicScalar(dynamicNode(instance, "name"))), environment
}
