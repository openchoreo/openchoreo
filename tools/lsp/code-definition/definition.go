// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package codedefinition

import (
	"net/url"
	"path/filepath"
	"strings"

	"github.com/openchoreo/openchoreo/tools/lint/indexer"
	"github.com/openchoreo/openchoreo/tools/lint/parser"
	"github.com/openchoreo/openchoreo/tools/lsp/lsp-util"
	protocol "github.com/tliron/glsp/protocol_3_16"
	"gopkg.in/yaml.v3"
)

// KindIndexer is the subset of indexer operations needed to resolve
// go-to-definition requests. The lsp.Server satisfies this interface.
type KindIndexer interface {
	FindDefinitionByKind(kind string) []indexer.Location
	FindAllKinds() []string
	FindDefinition(kind, name string) []indexer.Location
	FindByName(name string) []indexer.Location
}

// Handle resolves the go-to-definition locations for the document text at the
// given 1-indexed cursor position. sourceURI is the document being navigated;
// candidates that belong to a different project (anything shallower than the
// deepest common path prefix with the source) are dropped, so go-to-definition
// only points at resources of the referencing project. It returns the target
// resource locations, or nil when nothing can be resolved.
func Handle(docText string, cursorLine, cursorCol int, sourceURI string, idx KindIndexer) []protocol.Location {
	if idx == nil {
		return nil
	}

	docs, err := parser.ParseYAML([]byte(docText))
	if err != nil || len(docs) == 0 {
		return nil
	}

	document := parser.DocumentForPosition(docs, cursorLine, cursorCol)
	if document == nil {
		return nil
	}

	// Find the scalar node at cursor position
	node := lsputil.FindNodeAtPosition(document.Root, cursorLine, cursorCol)
	if node == nil || node.Kind != yaml.ScalarNode {
		return nil
	}

	parentMapping, parentKey := lsputil.FindParentMappingAndKey(document.Root, node)
	if parentMapping == nil {
		return nil
	}

	// Embedded resource templates describe output resources, not OpenChoreo
	// workspace references. In particular containers[].name is a local name.
	path := lsputil.NormalizePath(lsputil.BuildPathToNode(document.Root, parentMapping))
	for _, segment := range lsputil.SplitPath(path) {
		if segment == "template" {
			return nil
		}
	}

	// Case 1: cursor is on a "kind" value → find all resources of that kind
	if parentKey == "kind" {
		kind := node.Value
		if kind == "" {
			return nil
		}
		locations := idx.FindDefinitionByKind(kind)
		return sameProjectLocations(sourceURI, toLSPLocations(locations))
	}

	// Case 2: cursor is on a "name" value → find the specific resource
	if parentKey == "name" {
		name := node.Value
		if name == "" {
			return nil
		}

		kind := ""

		// 2a. Try explicit sibling "kind" field in the parent mapping
		kindNode := lsputil.FindSiblingKey(parentMapping, "kind")
		if kindNode != nil && kindNode.Kind == yaml.ScalarNode {
			kind = kindNode.Value
		}

		// 2b. Try implicit kind inference from parent field name
		if kind == "" {
			yamlPath := lsputil.BuildPathToNode(document.Root, parentMapping)
			normalized := lsputil.NormalizePath(yamlPath)
			segments := lsputil.SplitPath(normalized)
			if len(segments) > 0 {
				fieldName := segments[len(segments)-1]
				registeredKinds := idx.FindAllKinds()
				kind = lsputil.FindKindByFieldName(fieldName, registeredKinds)
			}
		}

		// 2c. Walk up ancestors to find "kind" (for metadata.name in same file)
		if kind == "" {
			ancestor := parentMapping
			for ancestor != nil {
				grandparent, _ := lsputil.FindParentMappingAndKey(document.Root, ancestor)
				if grandparent == nil {
					break
				}
				kindNode = lsputil.FindSiblingKey(grandparent, "kind")
				if kindNode != nil && kindNode.Kind == yaml.ScalarNode {
					kind = kindNode.Value
					break
				}
				ancestor = grandparent
			}
		}

		if kind == "" {
			// Fallback: search by name across all kinds
			locations := idx.FindByName(name)
			return sameProjectLocations(sourceURI, toLSPLocations(locations))
		}

		if scoped, ok := idx.(interface {
			FindScopedDefinition(string, string, string, string) []indexer.Location
		}); ok {
			locations := scoped.FindScopedDefinition(kind, document.Metadata.Namespace, name, sourceURI)
			if scopedRoot, ok := idx.(interface{ HasWorkspace(string) bool }); ok && scopedRoot.HasWorkspace(sourceURI) {
				return toLSPLocations(locations)
			}
			return sameProjectLocations(sourceURI, toLSPLocations(locations))
		}
		locations := idx.FindDefinition(kind, name)
		return sameProjectLocations(sourceURI, toLSPLocations(locations))
	}

	return nil
}

// sameProjectLocations drops definition locations that live in a different
// project than sourceURI. The project of a file is its git working-tree root;
// candidates inside another repository (clones, upstream checkouts) are
// excluded. When git cannot identify the project, it falls back to keeping the
// location with the deepest common path prefix. Retained locations keep their
// original order.
func sameProjectLocations(sourceURI string, locations []protocol.Location) []protocol.Location {
	if len(locations) < 2 || sourceURI == "" {
		return locations
	}

	sourceRoot := projectRoots.rootFor(sourceURI)
	if sourceRoot == "" {
		return closestByPathDepth(sourceURI, locations)
	}

	result := make([]protocol.Location, 0, len(locations))
	for _, loc := range locations {
		if projectRoots.rootFor(string(loc.URI)) == sourceRoot {
			result = append(result, loc)
		}
	}
	if len(result) == 0 {
		return closestByPathDepth(sourceURI, locations)
	}
	return result
}

// closestByPathDepth keeps only the definition locations sharing the deepest
// common path prefix with the source URI, preserving their original order.
func closestByPathDepth(sourceURI string, locations []protocol.Location) []protocol.Location {
	if len(locations) < 2 || sourceURI == "" {
		return locations
	}

	maxScore := 0
	scores := make([]int, len(locations))
	for i, loc := range locations {
		scores[i] = pathSharedDepth(sourceURI, string(loc.URI))
		if scores[i] > maxScore {
			maxScore = scores[i]
		}
	}

	if maxScore == 0 {
		return locations
	}

	result := make([]protocol.Location, 0, len(locations))
	for i, loc := range locations {
		if scores[i] == maxScore {
			result = append(result, loc)
		}
	}
	return result
}

// pathSharedDepth returns the number of leading path segments shared by the
// two file URIs. Both are percent-decoded first, so the raw URI received from
// the client (e.g. OpenChoreoLSP%26MCPToolkit) still compares equal to the
// decoded path the indexer stores (OpenChoreoLSP&MCPToolkit).
func pathSharedDepth(a, b string) int {
	aPath := filepath.ToSlash(fileURIToPath(a))
	bPath := filepath.ToSlash(fileURIToPath(b))
	aSegs := strings.Split(aPath, "/")
	bSegs := strings.Split(bPath, "/")
	n := len(aSegs)
	if len(bSegs) < n {
		n = len(bSegs)
	}
	shared := 0
	for i := 0; i < n; i++ {
		if aSegs[i] != bSegs[i] {
			break
		}
		shared++
	}
	return shared
}

// fileURIToPath converts a file:// URI to a local filesystem path, decoding
// percent-escapes so the path always matches what is on disk. Non-file URIs
// are returned unchanged.
func fileURIToPath(uri string) string {
	if !strings.HasPrefix(uri, "file://") {
		return uri
	}
	path := strings.TrimPrefix(uri, "file://")
	decoded, err := url.PathUnescape(path)
	if err != nil {
		return path
	}
	return decoded
}

// toLSPLocations converts indexer locations to LSP locations.
func toLSPLocations(locations []indexer.Location) []protocol.Location {
	var result []protocol.Location
	for _, loc := range locations {
		result = append(result, protocol.Location{
			URI: protocol.DocumentUri(loc.URI),
			Range: protocol.Range{
				Start: protocol.Position{
					Line:      uint32(loc.Line - 1),
					Character: uint32(loc.Column - 1),
				},
				End: protocol.Position{
					Line:      uint32(loc.Line - 1),
					Character: uint32(loc.Column - 1 + 4), // cover "kind" keyword
				},
			},
		})
	}
	return result
}
