// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package schemavalidator

import (
	"context"
	"errors"
	"fmt"
	openapierrors "k8s.io/kube-openapi/pkg/validation/errors"
	"os"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/schema"
	"github.com/openchoreo/openchoreo/tools/lint/parser"
	"k8s.io/apimachinery/pkg/runtime"
)

// ValidateFiles validates Component parameters and trait instance parameters
// against their referenced ComponentType/Trait schemas. It reads all files,
// builds an index, and reports schema validation errors with locations.
func ValidateFiles(ctx context.Context, files []string) []Finding {
	if len(files) == 0 {
		return nil
	}
	sources := loadSources(files)
	if len(sources) == 0 {
		return nil
	}
	return validateSources(ctx, sources)
}

func loadSources(files []string) []sourceDoc {
	var out []sourceDoc
	for _, path := range files {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		if !parser.HasOpenChoreoAPIVersion(data) {
			continue
		}
		parsed, err := parser.ParseYAML(data)
		if err != nil {
			continue
		}
		for _, doc := range parsed {
			out = append(out, sourceDoc{doc: doc, file: path})
		}
	}
	return out
}

type sourceDoc struct {
	doc  *parser.DocumentNode
	file string
}

func validateLegacySources(ctx context.Context, sources []sourceDoc) []Finding {
	index := buildIndex(sources)
	var findings []Finding

	findings = append(findings, validateSchemaDefinitions(index)...)

	for _, comp := range index.components {
		// Validate component parameters
		if ct, cct := index.resolveComponentType(comp); ct != nil || cct != nil {
			var schemaSection *v1alpha1.SchemaSection
			var owner string
			if ct != nil {
				schemaSection = ct.Spec.Parameters
				owner = fmt.Sprintf("ComponentType %q", ct.Name)
			} else if cct != nil {
				schemaSection = cct.Spec.Parameters
				owner = fmt.Sprintf("ClusterComponentType %q", cct.Name)
			}
			if schemaSection != nil {
				f := validateParamsAgainstSchema(schemaSection, comp.parameters, owner, comp.file, comp.doc, "parameters")
				findings = append(findings, f...)
			}
		}

		// Validate trait parameters
		for i, trait := range comp.traits {
			if t, ct2 := index.resolveTrait(comp.file, comp.doc, trait); t != nil || ct2 != nil {
				var schemaSection *v1alpha1.SchemaSection
				var owner string
				if t != nil {
					schemaSection = t.Spec.Parameters
					owner = fmt.Sprintf("Trait %q", t.Name)
				} else if ct2 != nil {
					schemaSection = ct2.Spec.Parameters
					owner = fmt.Sprintf("ClusterTrait %q", ct2.Name)
				}
				if schemaSection != nil {
					f := validateParamsAgainstSchema(schemaSection, trait.parameters, owner, comp.file, comp.doc, "traits", i)
					findings = append(findings, f...)
				}
			}
		}
	}
	return findings
}

func validateParamsAgainstSchema(
	schemaSection *v1alpha1.SchemaSection,
	rawParams *runtime.RawExtension,
	schemaOwner string,
	file string,
	doc *parser.DocumentNode,
	key string,
	traitIndex ...int,
) []Finding {
	if schemaSection == nil {
		return nil
	}
	paramsRaw := schemaSection.GetRaw()
	if paramsRaw == nil || len(paramsRaw.Raw) == 0 {
		return nil
	}

	structural, jsonSchema, err := schema.ResolveSectionToBundle(schemaSection)
	if err != nil {
		line, column := position(doc.Spec)
		if key == "parameters" && len(traitIndex) == 0 {
			if n := findKeyNode(doc.Root, "parameters"); n != nil {
				line, column = position(n)
			}
		} else if key == "traits" && len(traitIndex) == 1 {
			if n := findTraitParametersKey(doc, traitIndex[0]); n != nil {
				line, column = position(n)
			}
		}
		return []Finding{{
			Path:     file,
			Severity: SeverityError,
			Code:     "schema-invalid-definition",
			Message:  fmt.Sprintf("%s has invalid schema definition: %v", schemaOwner, err),
			Line:     line,
			Column:   column,
		}}
	}

	var params map[string]any
	if rawParams != nil && len(rawParams.Raw) > 0 {
		if err := yamlUnmarshal(rawParams.Raw, &params); err != nil {
			line, column := position(doc.Spec)
			if key == "parameters" && len(traitIndex) == 0 {
				if n := findKeyNode(doc.Root, "parameters"); n != nil {
					line, column = position(n)
				}
			} else if key == "traits" && len(traitIndex) == 1 {
				if n := findTraitParametersKey(doc, traitIndex[0]); n != nil {
					line, column = position(n)
				}
			}
			return []Finding{{
				Path:     file,
				Severity: SeverityError,
				Code:     "schema-invalid-params",
				Message:  fmt.Sprintf("failed to parse parameters: %v", err),
				Line:     line,
				Column:   column,
			}}
		}
	} else {
		params = map[string]any{}
	}

	params = schema.ApplyDefaults(params, structural)
	if err := schema.ValidateWithJSONSchema(params, jsonSchema); err != nil {
		line, column := position(doc.Spec)
		if key == "parameters" && len(traitIndex) == 0 {
			if n := findKeyNode(doc.Root, "parameters"); n != nil {
				line, column = position(n)
			}
		} else if key == "traits" && len(traitIndex) == 1 {
			if n := findTraitParametersKey(doc, traitIndex[0]); n != nil {
				line, column = position(n)
			}
		}
		base := findMappingValue(doc.Root, "spec")
		if key == "traits" && len(traitIndex) == 1 {
			base = findMappingValue(base, "traits")
			if base != nil && base.Kind == yaml.SequenceNode && traitIndex[0] >= 0 && traitIndex[0] < len(base.Content) {
				base = base.Content[traitIndex[0]]
			} else {
				base = nil
			}
		}
		base = findMappingValue(base, "parameters")
		var structured *schema.ValueValidationError
		if errors.As(err, &structured) {
			var findings []Finding
			for _, detail := range valueErrors(structured.Errors) {
				n := valuePathNode(base, detail.Name)
				l, c := line, column
				if n != nil {
					l, c = position(n)
				}
				findings = append(findings, Finding{Path: file, Severity: SeverityError, Code: "schema-parameters-mismatch", Message: fmt.Sprintf("parameters do not match %s schema: %s", schemaOwner, strings.ReplaceAll(detail.Error(), " in body", "")), Line: l, Column: c})
			}
			if len(findings) > 0 {
				return findings
			}
		}

		return []Finding{{
			Path:     file,
			Severity: SeverityError,
			Code:     "schema-parameters-mismatch",
			Message:  fmt.Sprintf("parameters do not match %s schema: %v", schemaOwner, err),
			Line:     line,
			Column:   column,
		}}
	}
	return nil
}

func yamlUnmarshal(data []byte, v any) error {
	return yaml.Unmarshal(data, v)
}

// valuePathNode maps Kubernetes dotted/indexed value paths to YAML nodes.
// Missing properties retain their nearest existing container location.
func valuePathNode(node *yaml.Node, path string) *yaml.Node {
	path = strings.ReplaceAll(strings.ReplaceAll(path, "[", "."), "]", "")
	for _, part := range strings.Split(path, ".") {
		if part == "" {
			continue
		}
		if node == nil {
			return nil
		}
		var next *yaml.Node
		if node.Kind == yaml.SequenceNode {
			i, err := strconv.Atoi(part)
			if err == nil && i >= 0 && i < len(node.Content) {
				next = node.Content[i]
			}
		} else {
			next = findMappingValue(node, part)
		}
		if next == nil {
			return node
		}
		node = next
	}
	return node
}

func valueErrors(errs []error) []*openapierrors.Validation {
	var result []*openapierrors.Validation
	for _, err := range errs {
		switch e := err.(type) {
		case *openapierrors.Validation:
			result = append(result, e)
		case *openapierrors.CompositeError:
			result = append(result, valueErrors(e.Errors)...)
		}
	}
	return result
}
