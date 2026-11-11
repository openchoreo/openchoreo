// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package schemavalidator

import (
	"encoding/json"
	"github.com/openchoreo/openchoreo/tools/lint/indexer"
	"strings"

	"github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/occ/fsmode/typed"
	"github.com/openchoreo/openchoreo/tools/lint/parser"
	"gopkg.in/yaml.v3"
	"k8s.io/apimachinery/pkg/runtime"
	sigsyaml "sigs.k8s.io/yaml"
)

type index struct {
	componentTypes        []componentTypeEntry
	clusterComponentTypes []clusterComponentTypeEntry
	traits                []traitEntry
	clusterTraits         []clusterTraitEntry
	components            []componentEntry
}

type componentTypeEntry struct {
	doc  v1alpha1.ComponentType
	file string
	pdoc *parser.DocumentNode
}
type clusterComponentTypeEntry struct {
	doc  v1alpha1.ClusterComponentType
	file string
	pdoc *parser.DocumentNode
}
type traitEntry struct {
	doc  v1alpha1.Trait
	file string
	pdoc *parser.DocumentNode
}
type clusterTraitEntry struct {
	doc  v1alpha1.ClusterTrait
	file string
	pdoc *parser.DocumentNode
}

type componentEntry struct {
	doc        *parser.DocumentNode
	file       string
	obj        v1alpha1.Component
	parameters *runtime.RawExtension
	traits     []componentTraitEntry
}

type componentTraitEntry struct {
	kind       string
	name       string
	parameters *runtime.RawExtension
}

func buildIndex(sources []sourceDoc) *index {
	idx := &index{}
	for _, s := range sources {
		doc := s.doc
		if doc == nil || !strings.HasPrefix(doc.APIVersion, "openchoreo.dev/") {
			continue
		}
		switch doc.Kind {
		case "ComponentType":
			var obj v1alpha1.ComponentType
			if err := decodeTyped(doc, &obj); err == nil {
				if obj.Name == "" && doc.Metadata.Name != "" {
					obj.Name = doc.Metadata.Name
				}
				if obj.Namespace == "" && doc.Metadata.Namespace != "" {
					obj.Namespace = doc.Metadata.Namespace
				}
				idx.componentTypes = append(idx.componentTypes, componentTypeEntry{doc: obj, file: s.file, pdoc: doc})
			} else {
			}
		case "ClusterComponentType":
			var obj v1alpha1.ClusterComponentType
			if err := decodeTyped(doc, &obj); err == nil {
				idx.clusterComponentTypes = append(idx.clusterComponentTypes, clusterComponentTypeEntry{doc: obj, file: s.file, pdoc: doc})
			}
		case "Trait":
			var obj v1alpha1.Trait
			if err := decodeTyped(doc, &obj); err == nil {
				idx.traits = append(idx.traits, traitEntry{doc: obj, file: s.file, pdoc: doc})
			}
		case "ClusterTrait":
			var obj v1alpha1.ClusterTrait
			if err := decodeTyped(doc, &obj); err == nil {
				idx.clusterTraits = append(idx.clusterTraits, clusterTraitEntry{doc: obj, file: s.file, pdoc: doc})
			}
		case "Component":
			// Decode only the fields this validator consumes. Unrelated invalid
			// fields (autoDeploy, workflow, etc.) must not suppress value checks.
			var obj v1alpha1.Component
			obj.Name, obj.Namespace = doc.Metadata.Name, doc.Metadata.Namespace
			reference := projectMapping(doc.Spec, "componentType")
			view := *doc
			view.Root = &yaml.Node{Kind: yaml.MappingNode, Content: []*yaml.Node{
				{Kind: yaml.ScalarNode, Value: "spec"}, reference,
			}}
			var projected v1alpha1.Component
			if err := decodeTyped(&view, &projected); err == nil {
				obj.Spec.ComponentType = projected.Spec.ComponentType
			}
			params, err := rawNode(findMappingValue(doc.Spec, "parameters"))
			if err != nil {
				continue
			}
			entry := componentEntry{doc: doc, file: s.file, obj: obj, parameters: params}
			if traits := findMappingValue(doc.Spec, "traits"); traits != nil && traits.Kind == yaml.SequenceNode {
				for _, node := range traits.Content {
					trait := componentTraitEntry{}
					view.Root = projectMapping(node, "kind", "name", "parameters")
					var decoded v1alpha1.ComponentTrait
					if err := decodeTyped(&view, &decoded); err == nil {
						trait.kind, trait.name, trait.parameters = string(decoded.Kind), decoded.Name, decoded.Parameters
						if trait.kind == "" {
							trait.kind = "Trait"
						}
					}
					// Preserve sequence positions even when another reference is invalid.
					entry.traits = append(entry.traits, trait)
				}
			}
			idx.components = append(idx.components, entry)
		}
	}
	return idx
}

func yamlMarshalNode(n *yaml.Node) ([]byte, error) {
	return yaml.Marshal(n)
}

func decodeTyped(doc *parser.DocumentNode, out interface{}) error {
	raw, err := yamlMarshalNode(doc.Root)
	if err != nil {
		return err
	}
	data, err := sigsyaml.YAMLToJSON(raw)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, out)
}

func (idx *index) resolveComponentType(comp componentEntry) (*v1alpha1.ComponentType, *v1alpha1.ClusterComponentType) {
	ref := comp.obj.Spec.ComponentType
	kind := string(ref.Kind)
	if kind == "" {
		kind = "ComponentType"
	}
	name := (&typed.Component{Component: &comp.obj}).ComponentTypeName()
	if kind == "ClusterComponentType" {
		var match *v1alpha1.ClusterComponentType
		for i := range idx.clusterComponentTypes {
			candidate := &idx.clusterComponentTypes[i].doc
			if prefix, _, qualified := strings.Cut(ref.Name, "/"); qualified && string(candidate.Spec.WorkloadType) != "" && string(candidate.Spec.WorkloadType) != prefix {
				continue
			}
			if candidate.Name == name {
				if match != nil {
					return nil, nil
				}
				match = candidate
			}
		}
		return nil, match
	}
	if kind != "ComponentType" {
		return nil, nil
	}
	ns := indexer.NamespaceOrDefault(comp.obj.Namespace)
	var match *v1alpha1.ComponentType
	for i := range idx.componentTypes {
		candidate := &idx.componentTypes[i].doc
		if prefix, _, qualified := strings.Cut(ref.Name, "/"); qualified && string(candidate.Spec.WorkloadType) != "" && string(candidate.Spec.WorkloadType) != prefix {
			continue
		}
		if candidate.Name == name && indexer.NamespaceOrDefault(candidate.Namespace) == ns {
			if match != nil {
				return nil, nil
			}
			match = candidate
		}
	}
	return match, nil
}

func (idx *index) resolveTrait(file string, doc *parser.DocumentNode, trait componentTraitEntry) (*v1alpha1.Trait, *v1alpha1.ClusterTrait) {
	kind := trait.kind
	if kind == "" {
		kind = "Trait"
	}
	name := trait.name
	if kind == "ClusterTrait" {
		var match *v1alpha1.ClusterTrait
		for i := range idx.clusterTraits {
			candidate := &idx.clusterTraits[i].doc
			if candidate.Name == name {
				if match != nil {
					return nil, nil
				}
				match = candidate
			}
		}
		return nil, match
	}
	if kind != "Trait" {
		return nil, nil
	}
	ns := ""
	if doc != nil {
		ns = doc.Metadata.Namespace
	}
	ns = indexer.NamespaceOrDefault(ns)
	var match *v1alpha1.Trait
	for i := range idx.traits {
		candidate := &idx.traits[i].doc
		if candidate.Name == name && indexer.NamespaceOrDefault(candidate.Namespace) == ns {
			if match != nil {
				return nil, nil
			}
			match = candidate
		}
	}
	return match, nil
}

var _ runtime.RawExtension

// projectMapping builds a read-only view without changing the original AST.
func projectMapping(node *yaml.Node, fields ...string) *yaml.Node {
	out := &yaml.Node{Kind: yaml.MappingNode}
	for _, field := range fields {
		if value := findMappingValue(node, field); value != nil {
			out.Content = append(out.Content, &yaml.Node{Kind: yaml.ScalarNode, Value: field}, value)
		}
	}
	return out
}
