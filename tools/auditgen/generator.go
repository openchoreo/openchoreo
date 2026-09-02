// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"fmt"
	"go/format"
	"sort"
	"strings"
	"text/template"
	"unicode"

	"github.com/getkin/kin-openapi/openapi3"
)

// excludedOperationIDs are state-modifying routes deliberately not turned
// into an OperationDef here — see internal/openchoreo-api/audit/exemptions.go
// for the reason each is exempt rather than audited.
//
// GenerateRelease is NOT here: ComponentService.GenerateRelease calls
// s.k8sClient.Create on a new ComponentRelease
// (internal/openchoreo-api/services/component/service.go), so it needs a
// real definition — generateReleaseOverride below — rather than an
// exemption.
var excludedOperationIDs = map[string]bool{
	"Evaluates":       true,
	"HandleAutoBuild": true,
}

// generateReleaseOverride replaces deriveDefinition's generic verb+resourceType
// derivation for GenerateRelease: the path's own resource segment ("components")
// names the parent the release is generated *from*, not the resource actually
// created (a new ComponentRelease), so the generic rule would both misclassify
// ResourceType and wrongly treat {componentName} as the created resource's own
// identifying path parameter.
var generateReleaseOverride = operationDef{
	ID: "GenerateRelease", Action: "generate_release", ResourceType: "componentrelease",
	Category: "CategoryManagement",
}

// actionSuffixSegments are trailing path segments that name a verb on an
// otherwise ordinary resource path, not a further level of resource
// hierarchy — e.g. ".../releasebindings/{releaseBindingName}/trigger". They
// are skipped when locating both the resource's own path parameter and its
// resource-kind segment.
var actionSuffixSegments = map[string]bool{
	"trigger":          true,
	"generate-release": true,
}

// resourceCategories maps every resource kind (the plural REST path segment)
// that a state-modifying operation can target to its audit Category. This is
// exhaustive, not a default-plus-exceptions table: deriveDefinition fails
// generation for any kind segment missing here, so adding a new resource
// kind to the API forces a deliberate category choice instead of silently
// landing in CategoryManagement. Add the new kind's entry here as part of
// that change.
var resourceCategories = map[string]string{
	"authzroles":               "CategoryAuthorization",
	"authzrolebindings":        "CategoryAuthorization",
	"clusterauthzroles":        "CategoryAuthorization",
	"clusterauthzrolebindings": "CategoryAuthorization",

	"clustercomponenttypes":                   "CategoryManagement",
	"clusterdataplanes":                       "CategoryManagement",
	"clusterobservabilityplanes":              "CategoryManagement",
	"clusterprojecttypes":                     "CategoryManagement",
	"clusterresourcetypes":                    "CategoryManagement",
	"clustertraits":                           "CategoryManagement",
	"clusterworkflows":                        "CategoryManagement",
	"clusterworkflowplanes":                   "CategoryManagement",
	"components":                              "CategoryManagement",
	"componentreleases":                       "CategoryManagement",
	"componenttypes":                          "CategoryManagement",
	"dataplanes":                              "CategoryManagement",
	"deploymentpipelines":                     "CategoryManagement",
	"environments":                            "CategoryManagement",
	"gitsecrets":                              "CategoryManagement",
	"namespaces":                              "CategoryManagement",
	"observabilityalertsnotificationchannels": "CategoryManagement",
	"observabilityplanes":                     "CategoryManagement",
	"projects":                                "CategoryManagement",
	"projectreleases":                         "CategoryManagement",
	"projectreleasebindings":                  "CategoryManagement",
	"projecttypes":                            "CategoryManagement",
	"releasebindings":                         "CategoryManagement",
	"resources":                               "CategoryManagement",
	"resourcereleases":                        "CategoryManagement",
	"resourcereleasebindings":                 "CategoryManagement",
	"resourcetypes":                           "CategoryManagement",
	"secrets":                                 "CategoryManagement",
	"secretreferences":                        "CategoryManagement",
	"traits":                                  "CategoryManagement",
	"workflows":                               "CategoryManagement",
	"workflowplanes":                          "CategoryManagement",
	"workflowruns":                            "CategoryManagement",
	"workloads":                               "CategoryManagement",
}

// singularOverrides names resource-kind segments whose singular form isn't a
// bare trailing-"s" strip. Empty today — every kind currently in the spec
// singularizes mechanically — but kept as an explicit map rather than adding
// general inflector logic to singularize for cases that don't exist yet.
var singularOverrides = map[string]string{}

// operationDef mirrors audit.OperationDef's REST-relevant fields — auditgen
// doesn't need MCP fields, which are added by hand in mcp_bindings.go.
type operationDef struct {
	ID                string
	Action            string
	ResourceType      string
	Category          string
	RESTResourceParam string
}

// buildDefinitions walks every state-modifying operation in swagger and
// derives one operationDef per non-excluded operation.
func buildDefinitions(swagger *openapi3.T) ([]operationDef, error) {
	var defs []operationDef
	usedKinds := make(map[string]bool)

	for _, path := range swagger.Paths.InMatchingOrder() {
		item := swagger.Paths.Find(path)
		for method, op := range item.Operations() {
			if method == "GET" {
				continue
			}
			if op.OperationID == "" {
				return nil, fmt.Errorf("operation %s %s has no operationId", method, path)
			}
			if excludedOperationIDs[op.OperationID] {
				continue
			}

			kindSegment, _, _ := pathTail(path)
			usedKinds[kindSegment] = true

			var def operationDef
			if op.OperationID == generateReleaseOverride.ID {
				def = generateReleaseOverride
			} else {
				var err error
				def, err = deriveDefinition(method, path, op.OperationID)
				if err != nil {
					return nil, fmt.Errorf("operation %s (%s %s): %w", op.OperationID, method, path, err)
				}
			}
			defs = append(defs, def)
		}
	}

	sort.Slice(defs, func(i, j int) bool { return defs[i].ID < defs[j].ID })

	seen := make(map[string]bool, len(defs))
	for _, d := range defs {
		if seen[d.ID] {
			return nil, fmt.Errorf("duplicate operationId %q", d.ID)
		}
		seen[d.ID] = true
	}

	if err := checkNoOrphanCategories(usedKinds); err != nil {
		return nil, err
	}

	return defs, nil
}

// checkNoOrphanCategories fails generation if resourceCategories has an entry
// for a resource-kind segment no state-modifying operation in the live spec
// actually uses — e.g. a resource kind renamed or removed from the API
// without removing its now-stale entry here. resourceCategories is exhaustive
// in both directions: deriveDefinition already fails when a real kind has no
// entry (see resourceCategories' doc comment); this is the mirror check for
// an entry with no real kind behind it, catching drift a bare map-literal
// diff wouldn't — an orphaned entry compiles and generates fine, and looks
// identical to a legitimate one without cross-referencing the live spec.
func checkNoOrphanCategories(usedKinds map[string]bool) error {
	var orphans []string
	for kind := range resourceCategories {
		if !usedKinds[kind] {
			orphans = append(orphans, kind)
		}
	}
	if len(orphans) == 0 {
		return nil
	}
	sort.Strings(orphans)
	return fmt.Errorf(
		"resourceCategories has orphan entries naming a resource kind no operation in the live spec uses: %v — remove them",
		orphans)
}

// deriveDefinition computes one operationDef from a route's method, path and
// operationId. See pathTail's doc comment for the path-parsing rule.
func deriveDefinition(method, path, operationID string) (operationDef, error) {
	kindSegment, restResourceParam, lastRawSegment := pathTail(path)

	resourceType, err := singularize(kindSegment)
	if err != nil {
		return operationDef{}, err
	}

	verb := "create"
	switch {
	case lastRawSegment == "trigger":
		verb = "trigger"
	case method == "PUT" || method == "PATCH":
		verb = "update"
	case method == "DELETE":
		verb = "delete"
	}

	category, ok := resourceCategories[kindSegment]
	if !ok {
		return operationDef{}, fmt.Errorf(
			"resource-kind segment %q has no entry in resourceCategories; add one "+
				"(CategoryManagement or CategoryAuthorization) rather than defaulting silently", kindSegment)
	}

	action := actionFromOperationID(operationID)
	if !strings.HasPrefix(action, verb+"_") {
		// The method/path-derived verb and the operationId's own leading word
		// are computed independently and expected to always agree (verified
		// against every operationId in the live spec). A mismatch means one
		// of the two derivations is wrong for this route, not a case to paper
		// over — same reasoning as resourceCategories' exhaustiveness checks.
		return operationDef{}, fmt.Errorf(
			"operation %s: derived action %q does not start with verb %q (from %s %s) — "+
				"operationId's leading word must match the method/path-derived verb",
			operationID, action, verb, method, path)
	}

	return operationDef{
		ID:                operationID,
		Action:            action,
		ResourceType:      resourceType,
		Category:          category,
		RESTResourceParam: restResourceParam,
	}, nil
}

// actionFromOperationID derives the operator-facing action string by
// word-splitting the operationId's PascalCase — e.g. "CreateClusterComponentType"
// becomes "create_cluster_component_type" — rather than concatenating the
// method-derived verb with the singularized, unseparated path segment (which
// produced unreadable actions like "create_clustercomponenttype" for every
// multi-word resource kind). This is also the convention pkg/mcp/tools' MCP
// tool names already use for the same operations (e.g. "create_component_type").
//
// This deliberately does not change ResourceType, which keeps its plain
// concatenated form (e.g. "clustercomponenttype") — SetResource override
// tables, resource:verb authz actions and existing published events already
// key on that spelling. Action and ResourceType are different fields for
// different purposes and are no longer substrings of one another by
// construction (Action: "create_cluster_component_type", ResourceType:
// "clustercomponenttype" for the same operation).
func actionFromOperationID(operationID string) string {
	words := splitPascalCase(operationID)
	for i, w := range words {
		words[i] = strings.ToLower(w)
	}
	return strings.Join(words, "_")
}

// splitPascalCase splits a PascalCase identifier into its constituent words,
// e.g. "ClusterComponentType" -> ["Cluster", "Component", "Type"]. Every
// operationId in the live spec is plain PascalCase with no acronym run (no
// all-caps sequence like "URL" or "ID"), so splitting at each uppercase
// letter is exact for this input. Not a general-purpose PascalCase splitter:
// an acronym run would mis-segment into single-letter words.
func splitPascalCase(s string) []string {
	var words []string
	current := make([]rune, 0, len(s))
	for _, r := range s {
		if unicode.IsUpper(r) && len(current) > 0 {
			words = append(words, string(current))
			current = nil
		}
		current = append(current, r)
	}
	if len(current) > 0 {
		words = append(words, string(current))
	}
	return words
}

// pathTail parses a route path from the end to find:
//   - kindSegment: the resource-kind path segment (e.g. "dataplanes"),
//   - restResourceParam: the path parameter naming this operation's own
//     resource (e.g. "dpName"), empty for a route with no resource id
//     in the path (a create route),
//   - lastRawSegment: the path's final segment before any adjustment, used
//     to detect a trigger-style action suffix.
//
// A trailing action-suffix segment (see actionSuffixSegments) is skipped
// first, since it names a verb rather than a further resource level. What
// remains is then either the resource's own "{id}" path parameter — in which
// case the segment before that is the resource kind — or, for a create
// route, the resource kind directly with no id segment at all.
func pathTail(path string) (kindSegment, restResourceParam, lastRawSegment string) {
	segs := strings.Split(strings.Trim(path, "/"), "/")
	i := len(segs) - 1
	lastRawSegment = segs[i]

	if actionSuffixSegments[segs[i]] {
		i--
	}
	if isPathParam(segs[i]) {
		restResourceParam = strings.Trim(segs[i], "{}")
		i--
	}
	kindSegment = segs[i]
	return kindSegment, restResourceParam, lastRawSegment
}

func isPathParam(seg string) bool {
	return strings.HasPrefix(seg, "{") && strings.HasSuffix(seg, "}")
}

// singularize converts a plural REST path segment to the singular form
// handlers use in SetResource and Operation.ResourceType. See
// singularOverrides' doc comment for why this isn't a general inflector.
func singularize(kindSegment string) (string, error) {
	if s, ok := singularOverrides[kindSegment]; ok {
		return s, nil
	}
	if !strings.HasSuffix(kindSegment, "s") {
		return "", fmt.Errorf(
			"resource-kind segment %q doesn't end in \"s\" and has no entry in singularOverrides; "+
				"add one rather than guessing", kindSegment)
	}
	return strings.TrimSuffix(kindSegment, "s"), nil
}

const fileTemplate = `// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

// Code generated by tools/auditgen. DO NOT EDIT.

package audit

import (
	"github.com/openchoreo/openchoreo/internal/server/middleware/audit"
)

// generatedOperationDefs returns one audit.OperationDef per state-modifying
// REST operation in the OpenAPI spec, minus the operations in
// excludedOperationIDs. MCP fields are zero-valued here — mcp_bindings.go
// enriches specific entries by ID, since the MCP tool/scope/resource-arg
// mapping isn't derivable from the spec.
//
// Regenerate with: make code.gen (runs tools/auditgen).
func generatedOperationDefs() []audit.OperationDef {
	return []audit.OperationDef{
{{- range . }}
		{
			ID: {{ printf "%q" .ID }}, Action: {{ printf "%q" .Action }}, ResourceType: {{ printf "%q" .ResourceType }},
			Category: audit.{{ .Category }},
{{- if .RESTResourceParam }} RESTResourceParam: {{ printf "%q" .RESTResourceParam }},
{{- end }}
		},
{{- end }}
	}
}
`

// renderDefinitions renders defs into the generated Go source, gofmt'd.
func renderDefinitions(defs []operationDef) ([]byte, error) {
	tmpl, err := template.New("definitions").Parse(fileTemplate)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, defs); err != nil {
		return nil, err
	}
	return format.Source(buf.Bytes())
}
