// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

// Package auditgen holds the OpenAPI-spec-walking logic tools/auditgen uses
// for every service it generates (see that command's -service flag). Each
// service supplies its own Config — resource categories, exclusions,
// overrides — and calls BuildDefinitions/RenderDefinitions with it; no
// service-specific data lives here.
package auditgen

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

// OperationDef mirrors audit.OperationDef's REST-relevant fields — auditgen
// doesn't need MCP fields, which are added by hand per service (e.g.
// mcp_bindings.go).
type OperationDef struct {
	ID                string
	Action            string
	ResourceType      string
	Category          string
	RESTResourceParam string
}

// Config parameterizes BuildDefinitions for one service's spec and
// conventions.
type Config struct {
	// ResourceCategories maps every resource kind (the plural REST path
	// segment) a state-modifying operation can target to its audit Category.
	// Exhaustive, not a default-plus-exceptions table: deriveDefinition fails
	// generation for any kind segment missing here, and checkNoOrphanCategories
	// fails generation for any entry naming a kind no live operation uses.
	ResourceCategories map[string]string
	// ExcludedOperationIDs are state-modifying routes deliberately not turned
	// into a definition — see each service's own exemptions table for why.
	ExcludedOperationIDs map[string]bool
	// SingularOverrides names resource-kind segments whose singular form
	// isn't a bare trailing-"s" strip.
	SingularOverrides map[string]string
	// ActionSuffixSegments are trailing path segments that name a verb on an
	// otherwise ordinary resource path, not a further level of resource
	// hierarchy (e.g. ".../releasebindings/{releaseBindingName}/trigger").
	ActionSuffixSegments map[string]bool
	// Overrides replaces deriveDefinition's generic verb+resourceType
	// derivation for specific operationIds, keyed by operationId.
	Overrides map[string]OperationDef
}

// BuildDefinitions walks every state-modifying operation in swagger and
// derives one OperationDef per non-excluded operation, per cfg.
func BuildDefinitions(swagger *openapi3.T, cfg Config) ([]OperationDef, error) {
	var defs []OperationDef
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
			if cfg.ExcludedOperationIDs[op.OperationID] {
				continue
			}

			kindSegment, _, _ := pathTail(path, cfg.ActionSuffixSegments)
			usedKinds[kindSegment] = true

			var def OperationDef
			if override, ok := cfg.Overrides[op.OperationID]; ok {
				def = override
			} else {
				var err error
				def, err = deriveDefinition(method, path, op.OperationID, cfg)
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

	if err := checkNoOrphanCategories(usedKinds, cfg.ResourceCategories); err != nil {
		return nil, err
	}

	return defs, nil
}

// checkNoOrphanCategories fails generation if cfg.ResourceCategories has an
// entry for a resource-kind segment no state-modifying operation in the live
// spec actually uses — e.g. a resource kind renamed or removed from the API
// without removing its now-stale entry. resourceCategories is exhaustive in
// both directions: deriveDefinition already fails when a real kind has no
// entry; this is the mirror check for an entry with no real kind behind it.
func checkNoOrphanCategories(usedKinds map[string]bool, resourceCategories map[string]string) error {
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

// deriveDefinition computes one OperationDef from a route's method, path and
// operationId. See pathTail's doc comment for the path-parsing rule.
func deriveDefinition(method, path, operationID string, cfg Config) (OperationDef, error) {
	kindSegment, restResourceParam, lastRawSegment := pathTail(path, cfg.ActionSuffixSegments)

	resourceType, err := singularize(kindSegment, cfg.SingularOverrides)
	if err != nil {
		return OperationDef{}, err
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

	category, ok := cfg.ResourceCategories[kindSegment]
	if !ok {
		return OperationDef{}, fmt.Errorf(
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
		return OperationDef{}, fmt.Errorf(
			"operation %s: derived action %q does not start with verb %q (from %s %s) — "+
				"operationId's leading word must match the method/path-derived verb",
			operationID, action, verb, method, path)
	}

	return OperationDef{
		ID:                operationID,
		Action:            action,
		ResourceType:      resourceType,
		Category:          category,
		RESTResourceParam: restResourceParam,
	}, nil
}

// actionFromOperationID derives the operator-facing action string by
// word-splitting the operationId's PascalCase, e.g. "CreateClusterComponentType"
// becomes "create_cluster_component_type" — matching pkg/mcp/tools' MCP tool
// naming for the same operations.
//
// ResourceType is untouched and keeps its plain concatenated form (e.g.
// "clustercomponenttype"), since SetResource override tables, resource:verb
// authz actions, and published events already key on that spelling.
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
func pathTail(
	path string, actionSuffixSegments map[string]bool,
) (kindSegment, restResourceParam, lastRawSegment string) {
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
func singularize(kindSegment string, overrides map[string]string) (string, error) {
	if s, ok := overrides[kindSegment]; ok {
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
// excludedOperationIDs.{{ with .DocSuffix }} {{ . }}{{ end }}
//
// Regenerate with: make code.gen (runs tools/auditgen).
func generatedOperationDefs() []audit.OperationDef {
	return []audit.OperationDef{
{{- range .Defs }}
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

// RenderOptions carries the per-service parts of the generated file, so the
// template asserts nothing false about a service's own layout — e.g. that
// every service enriches entries from an mcp_bindings.go, which is true only
// of openchoreo-api.
type RenderOptions struct {
	// DocSuffix is appended to generatedOperationDefs' first doc paragraph,
	// on the same line. It may span lines if each continuation begins with
	// "// ", since it is emitted verbatim into a comment block. Empty for a
	// service with nothing extra to say.
	DocSuffix string
}

// RenderDefinitions renders defs into the generated Go source, gofmt'd.
func RenderDefinitions(defs []OperationDef, opts RenderOptions) ([]byte, error) {
	tmpl, err := template.New("definitions").Parse(fileTemplate)
	if err != nil {
		return nil, err
	}
	data := struct {
		Defs      []OperationDef
		DocSuffix string
	}{Defs: defs, DocSuffix: opts.DocSuffix}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return nil, err
	}
	return format.Source(buf.Bytes())
}
