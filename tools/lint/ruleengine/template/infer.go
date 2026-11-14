// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package template

import (
	"gopkg.in/yaml.v3"

	"github.com/openchoreo/openchoreo/tools/lint/parser"
	"github.com/openchoreo/openchoreo/tools/lint/strdist"
)

// inferMaxDistance is the largest edit distance a spec field name may have from
// a schema property and still count as the same field.
const inferMaxDistance = 2

// InferKind guesses the registered kind closest to a document whose kind is
// not recognized, by matching the document's spec keys against each kind's
// spec schema properties. Matches are fuzzy (edit distance <= 2) so typo'd
// field names still count, letting an unknown kind still be validated deeply
// instead of only producing an unknown-kind warning.
//
// It returns "" when the document has no usable spec mapping or no kind
// plausibly matches.
func InferKind(doc *parser.DocumentNode) string {
	if doc == nil || doc.Spec == nil || doc.Spec.Kind != yaml.MappingNode {
		return ""
	}
	if load() != nil {
		return ""
	}

	var docKeys []string
	for i := 0; i+1 < len(doc.Spec.Content); i += 2 {
		k := doc.Spec.Content[i]
		if k != nil && k.Kind == yaml.ScalarNode && k.Value != "" {
			docKeys = append(docKeys, k.Value)
		}
	}
	if len(docKeys) == 0 {
		return ""
	}

	schemas := schemasByVersion[currentVersion()]
	bestKind := ""
	bestScore := 0
	for _, ks := range kinds {
		s := schemas[ks.kind]
		if s == nil {
			continue
		}
		specSchema, ok := s.Properties["spec"]
		if !ok || specSchema.Properties == nil {
			continue
		}
		score := 0
		for _, k := range docKeys {
			for prop := range specSchema.Properties {
				if strdist.Distance(k, prop) <= inferMaxDistance {
					score++
					break
				}
			}
		}
		if score > bestScore {
			bestScore = score
			bestKind = ks.kind
		}
	}
	return bestKind
}
