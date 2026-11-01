// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0
package postrendervalidator

import (
	"context"
	"github.com/openchoreo/openchoreo/tools/lint/parser"
	"strings"
)

// SourceDocument is a parsed file snapshot, including unsaved editor content.
type SourceDocument struct {
	Path     string
	Document *parser.DocumentNode
}

// ValidateWorkspace renders every Component/binding independently without reading disk.
func ValidateWorkspace(ctx context.Context, sources []SourceDocument) []Finding {
	var all []projectDocument
	for _, s := range sources {
		if s.Document != nil && parser.IsOpenChoreoAPIVersion(s.Document.APIVersion) {
			all = append(all, projectDocument{file: s.Path, doc: s.Document})
		}
	}
	var out []Finding
	for _, source := range sources {
		if ctx.Err() != nil {
			break
		}
		if source.Document == nil || source.Document.Kind != "Component" || !parser.IsOpenChoreoAPIVersion(source.Document.APIVersion) {
			continue
		}
		// Namespace isolation is essential when an entire workspace is indexed.
		var scoped []projectDocument
		for _, d := range all {
			if d.doc.Metadata.Namespace == source.Document.Metadata.Namespace || strings.HasPrefix(d.doc.Kind, "Cluster") {
				scoped = append(scoped, d)
			}
		}
		index, decodeErrors := buildProjectIndex(scoped)
		if err := decodeErrors[source.Document]; err != nil {
			out = append(out, errorFinding(source.Path, err)...)
			continue
		}
		var component *componentEntry
		for i := range index.components {
			if index.components[i].doc == source.Document {
				component = &index.components[i]
				break
			}
		}
		if component == nil {
			continue
		}
		bindings := index.releaseBindingsForComponent(*component)
		targets := []*parser.DocumentNode{source.Document}
		if len(bindings) > 0 {
			targets = nil
			for _, b := range bindings {
				targets = append(targets, b.doc)
			}
		}
		for _, target := range targets {
			selected, err := index.selectRelated(*component, target)
			if err != nil {
				out = append(out, errorFinding(source.Path, err)...)
				continue
			}
			for _, embedded := range selected.componentType.obj.Spec.Traits {
				kind := string(embedded.Kind)
				if kind == "" {
					kind = "Trait"
				}
				found := false
				for _, t := range selected.traits {
					if t.kind == kind && t.name == embedded.Name {
						found = true
					}
				}
				if !found {
					t, e := index.selectTrait(kind, embedded.Name)
					if e != nil {
						err = e
						break
					}
					selected.traits = append(selected.traits, t)
				}
			}
			if err != nil {
				out = append(out, errorFinding(source.Path, err)...)
				continue
			}
			bundle, err := BundleFromDocuments(selected.documents())
			if err != nil {
				out = append(out, errorFinding(source.Path, err)...)
				continue
			}
			if selected.clusterDataPlane != nil {
				bundle.DataPlane = clusterDataPlaneToDataPlane(selected.clusterDataPlane.obj)
			}
			diags, err := ValidateBundle(ctx, bundle)
			if err != nil {
				out = append(out, errorFinding(source.Path, err)...)
				continue
			}
			for _, d := range diags {
				file, pos := resolvePostRenderDiagnostic(d, selected.sources(), source.Path)
				out = append(out, newFinding(file, pos, d))
			}
		}
	}
	return dedupeFindings(out)
}
