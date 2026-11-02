// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package schemavalidator

import (
	"context"
	"fmt"
	"github.com/openchoreo/openchoreo/tools/lint/indexer"
	"github.com/openchoreo/openchoreo/tools/lint/parser"
	"sort"
	"strings"
)

// Resource preserves the parsed source, including unsaved editor content.
type Resource struct {
	Path     string
	Document *parser.DocumentNode
}

// ResourceKey identifies a definition without crossing namespace boundaries.
type ResourceKey struct{ APIVersion, Kind, Namespace, Name string }

// ValidationContext is an immutable index for one validation snapshot.
type ValidationContext struct{ Resources map[ResourceKey][]Resource }

// Resolve returns every candidate: callers must not select an ambiguous match.
func (c *ValidationContext) Resolve(key ResourceKey) []Resource {
	if !strings.HasPrefix(key.Kind, "Cluster") {
		key.Namespace = indexer.NamespaceOrDefault(key.Namespace)
	}
	return c.Resources[key]
}

// Validator can serve multiple kinds; multiple validators may serve one kind.
type Validator interface {
	Supports(kind string) bool
	Validate(context.Context, Resource, *ValidationContext) []Finding
}

// ValidateDocuments validates already parsed input without rereading files.
// Optional validators supplement the existing schema checks.
func ValidateDocuments(ctx context.Context, resources []Resource, validators ...Validator) []Finding {
	validators = append([]Validator{relationshipValidator{}, snapshotValidator{}, environmentValidator{}, runtimeValidator{}}, validators...)
	snapshot := &ValidationContext{Resources: make(map[ResourceKey][]Resource)}
	sources := make([]sourceDoc, 0, len(resources))
	unique := make([]Resource, 0, len(resources))
	seen := make(map[string]bool)
	for _, r := range resources {
		d := r.Document
		if d == nil {
			continue
		}
		identity := fmt.Sprintf("%s:%d", r.Path, d.DocumentIndex)
		if seen[identity] {
			continue
		}
		seen[identity] = true
		unique = append(unique, r)
		namespace := indexer.NamespaceOrDefault(d.Metadata.Namespace)
		if strings.HasPrefix(d.Kind, "Cluster") {
			namespace = ""
		}
		key := ResourceKey{d.APIVersion, d.Kind, namespace, d.Metadata.Name}
		snapshot.Resources[key] = append(snapshot.Resources[key], r)
		sources = append(sources, sourceDoc{doc: d, file: r.Path})
	}
	resources = unique
	findings := validateLegacySources(ctx, sources)
	for _, candidates := range snapshot.Resources {
		if len(candidates) < 2 || !isSchemaDefinition(candidates[0].Document.Kind) || candidates[0].Document.Metadata.Name == "" || !strings.HasPrefix(candidates[0].Document.APIVersion, "openchoreo.dev/") {
			continue
		}
		for _, r := range candidates {
			line, column := position(r.Document.Root)
			findings = append(findings, Finding{Path: r.Path, Severity: SeverityError, Code: "pre-render-ambiguous-definition", Message: fmt.Sprintf("multiple local definitions for %s %q", r.Document.Kind, r.Document.Metadata.Name), Line: line, Column: column})
		}
	}
	for _, r := range resources {
		if ctx.Err() != nil {
			break
		}
		if r.Document == nil || !strings.HasPrefix(r.Document.APIVersion, "openchoreo.dev/") {
			continue
		}
		for _, v := range validators {
			if v.Supports(r.Document.Kind) {
				findings = append(findings, v.Validate(ctx, r, snapshot)...)
			}
		}
	}
	sort.SliceStable(findings, func(i, j int) bool {
		a, b := findings[i], findings[j]
		if a.Path != b.Path {
			return a.Path < b.Path
		}
		if a.Line != b.Line {
			return a.Line < b.Line
		}
		if a.Column != b.Column {
			return a.Column < b.Column
		}
		if a.Code != b.Code {
			return a.Code < b.Code
		}
		return a.Message < b.Message
	})
	return findings
}

func validateSources(ctx context.Context, sources []sourceDoc) []Finding {
	resources := make([]Resource, 0, len(sources))
	for _, s := range sources {
		resources = append(resources, Resource{Path: s.file, Document: s.doc})
	}
	return ValidateDocuments(ctx, resources)
}

func isSchemaDefinition(kind string) bool {
	switch kind {
	case "ComponentType", "ClusterComponentType", "Trait", "ClusterTrait":
		return true
	}
	return additionalDefinitions[kind]
}
