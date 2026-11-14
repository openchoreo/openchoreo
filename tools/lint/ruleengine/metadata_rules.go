// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package ruleengine

import (
	"gopkg.in/yaml.v3"

	"github.com/openchoreo/openchoreo/tools/lint/parser"
)

var knownMetadataFields = map[string]bool{
	"annotations":                true,
	"creationTimestamp":          true,
	"deletionGracePeriodSeconds": true,
	"deletionTimestamp":          true,
	"finalizers":                 true,
	"generateName":               true,
	"generation":                 true,
	"labels":                     true,
	"managedFields":              true,
	"name":                       true,
	"namespace":                  true,
	"ownerReferences":            true,
	"resourceVersion":            true,
	"uid":                        true,
}

type MetadataRules struct{}

func (r *MetadataRules) Name() string { return "metadata-fields" }

func (r *MetadataRules) Evaluate(doc *parser.DocumentNode) Diagnostics {
	metaField := doc.GetField("metadata")
	if metaField == nil {
		return nil
	}
	node := metaField.ValueNode
	if node == nil || node.Kind != yaml.MappingNode {
		return nil
	}
	var diags Diagnostics
	for i := 0; i+1 < len(node.Content); i += 2 {
		keyNode := node.Content[i]
		if keyNode.Kind != yaml.ScalarNode {
			continue
		}
		if !knownMetadataFields[keyNode.Value] {
			diags = append(diags, Diagnostic{
				Range:    nodeRange(keyNode),
				Severity: SeverityError,
				Code:     "unknown-metadata-field",
				Message:  "unknown metadata field: " + keyNode.Value,
			})
		}
	}
	return diags
}
