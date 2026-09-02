// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

// Command auditgen generates definitions.gen.go: one audit.OperationDef per
// state-modifying REST operation, stamping its action, resource type and
// category.
//
// Operations in excludedOperationIDs are exempt rather than audited — see
// internal/openchoreo-api/audit/exemptions.go.
//
// Reads the spec via gen.GetSwagger(), not the raw openapi/openchoreo-api.yaml
// file: oapi-codegen normalizes each operationId to PascalCase (the raw spec
// has lowerCamelCase, e.g. "handleAutoBuild") when it builds the embedded
// runtime spec, and that PascalCase form is what BuildPatternMap cross-
// references at request time (internal/openchoreo-api/api/handlers/handler.go
// passes gen.GetSwagger — not the file — into audit.NewMiddleware). Loading
// the raw file directly would generate IDs that never match a real route.
package main

import (
	"flag"
	"log"
	"os"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
)

func main() {
	outPath := flag.String(
		"out", "internal/openchoreo-api/audit/definitions.gen.go", "Path to write the generated Go file",
	)
	flag.Parse()

	swagger, err := gen.GetSwagger()
	if err != nil {
		log.Fatalf("auditgen: failed to load OpenAPI spec: %v", err)
	}

	defs, err := buildDefinitions(swagger)
	if err != nil {
		log.Fatalf("auditgen: %v", err)
	}

	src, err := renderDefinitions(defs)
	if err != nil {
		log.Fatalf("auditgen: failed to render generated source: %v", err)
	}

	if err := os.WriteFile(*outPath, src, 0o600); err != nil {
		log.Fatalf("auditgen: failed to write %q: %v", *outPath, err)
	}
}
