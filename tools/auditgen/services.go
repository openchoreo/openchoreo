// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"sort"

	"github.com/getkin/kin-openapi/openapi3"

	observergen "github.com/openchoreo/openchoreo/internal/observer/api/gen"
	observerinternalgen "github.com/openchoreo/openchoreo/internal/observer/api/internalgen"
	apigen "github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
	"github.com/openchoreo/openchoreo/tools/internal/auditgen"
)

// spec is one OpenAPI spec to walk, with the auditgen.Config for it.
//
// getSwagger is the generated accessor, never the raw openapi/*.yaml: the raw
// specs use lowerCamelCase operationIds, and oapi-codegen's PascalCase form is
// what audit.BuildPatternMap cross-references at request time.
type spec struct {
	name       string
	getSwagger func() (*openapi3.T, error)
	config     auditgen.Config
}

// service is one API server's generation inputs. Adding a third service is an
// entry in the registry below plus its own config file — not another main
// package.
type service struct {
	defaultOut string
	// specs are walked in order and their definitions merged. Per-spec configs
	// because BuildDefinitions runs checkNoOrphanCategories against the single
	// spec it is given, so a kind real in one spec looks orphaned in the other.
	specs []spec
	// render carries anything service-specific about the generated file's
	// layout, so the shared template claims nothing that is untrue of a
	// given service.
	render auditgen.RenderOptions
}

// services is the registry the -service flag selects from.
var services = map[string]service{
	"openchoreo-api": {
		defaultOut: "internal/openchoreo-api/audit/definitions.gen.go",
		specs: []spec{
			{name: "openchoreo-api.yaml", getSwagger: apigen.GetSwagger, config: apiConfig()},
		},
		render: auditgen.RenderOptions{
			DocSuffix: "MCP fields are zero-valued here — mcp_bindings.go\n" +
				"// enriches specific entries by ID, since the MCP tool/scope/resource-arg\n" +
				"// mapping isn't derivable from the spec.",
		},
	},
	"observer": {
		defaultOut: "internal/observer/audit/definitions.gen.go",
		// Both specs: the coverage gate is about observer's whole served
		// surface, so walking only the public one would let a new unaudited
		// alert-rule write pass CI.
		specs: []spec{
			{
				name:       "observer-api.yaml",
				getSwagger: observergen.GetSwagger,
				config:     observerPublicConfig(),
			},
			{
				name:       "observer-internal-api.yaml",
				getSwagger: observerinternalgen.GetSwagger,
				config:     observerInternalConfig(),
			},
		},
		// No DocSuffix: observer has no mcp_bindings.go and, having no
		// mutating MCP tools, never will.
	},
}

// serviceNames returns the registry's keys in sorted order, for flag help and
// error messages.
func serviceNames() []string {
	names := make([]string, 0, len(services))
	for name := range services {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
