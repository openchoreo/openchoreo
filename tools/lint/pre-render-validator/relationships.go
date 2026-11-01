// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package schemavalidator

import (
	"context"
	"fmt"
	"strings"

	"github.com/openchoreo/openchoreo/api/v1alpha1"
	"k8s.io/apimachinery/pkg/runtime"
	sigsyaml "sigs.k8s.io/yaml"
)

// relationship describes API-defined reference and value paths, not arbitrary
// fields beneath preserveUnknownFields sections.
type relationship struct {
	reference, values        []string
	defaultKind, clusterKind string
}

var parameterRelationships = map[string]relationship{
	"Resource":    {[]string{"type"}, []string{"parameters"}, "ResourceType", "ClusterResourceType"},
	"Project":     {[]string{"type"}, []string{"parameters"}, "ProjectType", "ClusterProjectType"},
	"WorkflowRun": {[]string{"workflow"}, []string{"workflow", "parameters"}, "ClusterWorkflow", "Workflow"},
}
var additionalDefinitions = map[string]bool{
	"ResourceType": true, "ClusterResourceType": true,
	"ProjectType": true, "ClusterProjectType": true,
	"Workflow": true, "ClusterWorkflow": true,
}

type relationshipValidator struct{}

func (relationshipValidator) Supports(kind string) bool {
	_, ok := parameterRelationships[kind]
	return ok || additionalDefinitions[kind]
}
func (relationshipValidator) Validate(ctx context.Context, r Resource, snapshot *ValidationContext) []Finding {
	if ctx.Err() != nil {
		return nil
	}
	d := r.Document
	if !strings.HasPrefix(d.APIVersion, "openchoreo.dev/") {
		return nil
	}
	if additionalDefinitions[d.Kind] {
		return validateDefinitionNode(r, d.Kind, d.Spec, d.Kind == "ProjectType" || d.Kind == "ClusterProjectType")
	}

	rel := parameterRelationships[d.Kind]
	ref := d.Spec
	for _, segment := range rel.reference {
		ref = findMappingValue(ref, segment)
	}
	if ref == nil {
		return nil
	}
	kind := rel.defaultKind
	if n := findMappingValue(ref, "kind"); n != nil {
		kind = n.Value
	}
	if kind != rel.defaultKind && kind != rel.clusterKind {
		return nil
	}
	name := findMappingValue(ref, "name")
	if name == nil {
		return nil
	}
	namespace := d.Metadata.Namespace
	if strings.HasPrefix(kind, "Cluster") {
		namespace = ""
	}
	candidates := snapshot.Resolve(ResourceKey{d.APIVersion, kind, namespace, name.Value})
	if len(candidates) != 1 {
		return nil
	} // ambiguity is reported by the shared engine
	var definition struct {
		Spec struct{ Parameters *v1alpha1.SchemaSection }
	}
	if err := decodeTyped(candidates[0].Document, &definition); err != nil {
		return []Finding{relationshipDecodeFinding(candidates[0], err)}
	}
	valueNode := d.Spec
	for _, segment := range rel.values {
		valueNode = findMappingValue(valueNode, segment)
	}
	var values *runtime.RawExtension
	if valueNode != nil {
		raw, err := yamlMarshalNode(valueNode)
		if err != nil {
			return []Finding{relationshipDecodeFinding(r, err)}
		}
		data, err := sigsyaml.YAMLToJSON(raw)
		if err != nil {
			return []Finding{relationshipDecodeFinding(r, err)}
		}
		values = &runtime.RawExtension{Raw: data}
	}

	return validateParamsAgainstSchema(definition.Spec.Parameters, values, fmt.Sprintf("%s %q", kind, name.Value), r.Path, d, "parameters")
}
func relationshipDecodeFinding(r Resource, err error) Finding {
	line, column := position(r.Document.Root)
	return Finding{Path: r.Path, Severity: SeverityError, Code: "pre-render-decode-error", Message: err.Error(), Line: line, Column: column}
}
