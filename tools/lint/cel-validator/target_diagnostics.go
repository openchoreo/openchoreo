// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package celvalidator

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/openchoreo/openchoreo/tools/lint/strdist"
	"github.com/openchoreo/openchoreo/tools/lint/targetschema"
)

var undefinedTargetField = regexp.MustCompile(`undefined field '([^']+)'`)
var simpleResourcePath = regexp.MustCompile(`\bresource(?:\.[A-Za-z_][A-Za-z0-9_]*)+`)

// Enrich an existing CEL checker issue; this does not independently validate
// paths. Complex indexing/comprehension cases keep the native checker message.
func (c *checker) targetFieldMessage(expr, message string, offset int) string {
	bad := undefinedTargetField.FindStringSubmatch(message)
	if len(bad) != 2 || c.targetNode == nil || c.resolver == nil {
		return ""
	}
	group, version, kind := mappingValue(c.targetNode, "group"), mappingValue(c.targetNode, "version"), mappingValue(c.targetNode, "kind")
	if group == nil || version == nil || kind == nil {
		return ""
	}
	key, ok := targetschema.LiteralGVK(group.Value, version.Value, kind.Value)
	if !ok {
		return ""
	}
	root, err := c.resolver.Resolve(key)
	if err != nil || root == nil {
		return ""
	}
	root = targetschema.WithResourceEnvelope(root)
	for _, span := range simpleResourcePath.FindAllStringIndex(expr, -1) {
		if offset < span[0] || offset >= span[1] {
			continue
		}
		parts := strings.Split(expr[span[0]:span[1]], ".")
		parent := root
		for i, field := range parts[1:] {
			if parent.Type != "object" || parent.XPreserveUnknownFields || parent.AdditionalProperties != nil {
				break
			}
			child, exists := parent.Properties[field]
			if exists {
				parent = &child
				continue
			}
			if field != bad[1] {
				break
			}
			location := key.GroupVersion().String() + " " + key.Kind
			if i > 0 {
				location += "." + strings.Join(parts[1:i+1], ".")
			}
			result := fmt.Sprintf("Unknown field %q in %s.", field, location)
			if len(field) >= 4 {
				names := make([]string, 0, len(parent.Properties))
				for name := range parent.Properties {
					names = append(names, name)
				}
				sort.Strings(names)
				distance := 1
				if len(field) >= 6 {
					distance = 2
				}
				if candidate, _, confident := strdist.Closest(field, names, distance); confident {
					result += fmt.Sprintf(" Did you mean %q?", candidate)
				}
			}
			return result
		}
	}
	return ""
}
