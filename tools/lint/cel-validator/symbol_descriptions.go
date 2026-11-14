// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package celvalidator

import (
	_ "embed"
	"encoding/json"
	"strings"

	structural "k8s.io/apiextensions-apiserver/pkg/apiserver/schema"

	"github.com/openchoreo/openchoreo/tools/lint/targetschema"
)

//go:embed context_descriptions.json
var contextDescriptionData []byte
var contextDescriptions = func() map[string]map[string]string {
	var out map[string]map[string]string
	_ = json.Unmarshal(contextDescriptionData, &out)
	return out
}()

func (c *checker) symbolDescription(parts []string, parent string) string {
	var root *structural.Structural
	switch parts[0] {
	case "parameters":
		root = c.schemas.Parameters
	case "environmentConfigs":
		root = c.schemas.EnvironmentConfigs
	case "resource":
		if c.targetNode != nil && c.resolver != nil {
			g, v, k := mappingValue(c.targetNode, "group"), mappingValue(c.targetNode, "version"), mappingValue(c.targetNode, "kind")
			if g != nil && v != nil && k != nil {
				if key, ok := targetschema.LiteralGVK(g.Value, v.Value, k.Value); ok {
					root, _ = c.resolver.Resolve(key)
					root = targetschema.WithResourceEnvelope(root)
				}
			}
		}
	}
	if root != nil {
		node := root
		for _, field := range parts[1:] {
			child, exists := node.Properties[strings.TrimSuffix(field, "[]")]
			if !exists {
				return ""
			}
			node = &child
			if strings.HasSuffix(field, "[]") {
				if node.Items == nil {
					return ""
				}
				node = node.Items
			}
		}
		return node.Description
	}
	family := "component"
	switch c.kind {
	case KindResourceType, KindClusterResourceType:
		family = "resource"
	case KindProjectType, KindClusterProjectType:
		family = "project"
	case KindWorkflow, KindClusterWorkflow:
		family = "workflow"
	}
	docs := contextDescriptions[family]
	if len(parts) == 1 {
		if typ := variableType(c.currentEnv, parts[0]); typ != nil {
			return docs[typ.TypeName()]
		}
		return ""
	}
	// Only reflected context types have Go source documentation. Renamed/local
	// types and dynamic fields do not borrow another type's field descriptions.
	return strings.TrimSpace(docs[parent+"."+parts[len(parts)-1]])
}
