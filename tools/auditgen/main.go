// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

// Command auditgen generates a service's definitions.gen.go: one
// audit.OperationDef per state-modifying REST operation, stamping its action,
// resource type and category.
//
// It serves every API server in the repo, selected with -service (see
// services.go). One binary rather than one per service: only the data differs,
// while the generation logic is shared through tools/internal/auditgen.
//
// Operations in a service's exclusion list are exempt rather than audited —
// see each service's own audit/exemptions.go for why.
//
//	go run ./tools/auditgen -service openchoreo-api
//	go run ./tools/auditgen -service observer
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"sort"
	"strings"

	"github.com/openchoreo/openchoreo/tools/internal/auditgen"
)

func main() {
	svcName := flag.String("service", "",
		"API server to generate for, one of: "+strings.Join(serviceNames(), ", "))
	outPath := flag.String("out", "",
		"Path to write the generated Go file (default: the service's own definitions.gen.go)")
	flag.Parse()

	svc, ok := services[*svcName]
	if !ok {
		log.Fatalf("auditgen: -service must be one of: %s (got %q)",
			strings.Join(serviceNames(), ", "), *svcName)
	}

	out := *outPath
	if out == "" {
		out = svc.defaultOut
	}

	defs, err := buildService(svc)
	if err != nil {
		log.Fatalf("auditgen: %s: %v", *svcName, err)
	}

	src, err := auditgen.RenderDefinitions(defs, svc.render)
	if err != nil {
		log.Fatalf("auditgen: failed to render generated source: %v", err)
	}

	if err := os.WriteFile(out, src, 0o600); err != nil {
		log.Fatalf("auditgen: failed to write %q: %v", out, err)
	}
}

// buildService walks every spec the service declares and merges the results
// into the single table its definitions.gen.go holds.
func buildService(svc service) ([]auditgen.OperationDef, error) {
	var defs []auditgen.OperationDef
	for _, sp := range svc.specs {
		swagger, err := sp.getSwagger()
		if err != nil {
			return nil, fmt.Errorf("failed to load %s: %w", sp.name, err)
		}
		specDefs, err := auditgen.BuildDefinitions(swagger, sp.config)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", sp.name, err)
		}
		defs = append(defs, specDefs...)
	}
	return mergeDefinitions(defs)
}

// mergeDefinitions sorts and duplicate-checks the concatenated per-spec
// tables.
//
// Sorts because BuildDefinitions sorts per call and RenderDefinitions not at
// all, so concatenating two sorted slices leaves an unsorted one — the file
// would reorder wholesale the first time an operation moved between specs.
// A no-op for a single-spec service.
//
// Rejects duplicates because BuildDefinitions' own check cannot see across
// specs, and a shared operationId would fail BuildPatternMap at startup.
func mergeDefinitions(defs []auditgen.OperationDef) ([]auditgen.OperationDef, error) {
	sort.Slice(defs, func(i, j int) bool { return defs[i].ID < defs[j].ID })

	seen := make(map[string]bool, len(defs))
	for _, d := range defs {
		if seen[d.ID] {
			return nil, fmt.Errorf("operationId %q is declared by more than one spec", d.ID)
		}
		seen[d.ID] = true
	}
	return defs, nil
}
