// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package indexer

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

// ResourceKey identifies a resource by its kind and name.
type ResourceKey struct {
	Kind string
	Name string
}

// Location holds the file URI and position of a resource definition.
type Location struct {
	URI    string
	Line   int
	Column int
}

// Indexer maintains an in-memory index of all resources in the workspace.
type Indexer struct {
	mu                sync.RWMutex
	contents          map[string][]byte
	snapshotVersion   uint64
	snapshotDocuments []SnapshotDocument
	index             map[ResourceKey][]Location // kind/name → locations
	fileIndex         map[string][]ResourceKey   // file URI → resources in that file
	rootDirs          []string
	// version is a monotonic counter bumped on every mutation. Consumers can
	// use it as a fingerprint to invalidate derived state that depends on the
	// index (e.g. reference checks against FileNames).
	version uint64
}

// NewIndexer creates a new empty Indexer.
func NewIndexer() *Indexer {
	return &Indexer{
		index:     make(map[ResourceKey][]Location),
		fileIndex: make(map[string][]ResourceKey),
	}
}

// IndexWorkspace scans all .yaml files under a single root and builds the
// index. It is kept as a thin wrapper over IndexWorkspaces for callers that
// only ever have one root.
func (idx *Indexer) IndexWorkspace(rootDir string) error {
	return idx.IndexWorkspaces([]string{rootDir})
}

// IndexWorkspaces scans all .yaml files under the given roots and builds a
// single shared index. Roots are expected to be non-overlapping (callers
// should normalize first); scanning a root that is inside another root would
// double-index its files. The walk holds no lock: results are accumulated
// locally and swapped in under a short write lock, so concurrent readers keep
// working against the previous index while a large workspace is being scanned.
func (idx *Indexer) IndexWorkspaces(roots []string) error {
	return idx.IndexWorkspacesWithProgress(roots, nil)
}

// IndexWorkspaceWithProgress is like IndexWorkspace but invokes onProgress
// after each file is scanned (scanned/total, total from a pre-count pass so a
// percentage can be derived). onProgress may be nil.
func (idx *Indexer) IndexWorkspaceWithProgress(rootDir string, onProgress func(scanned, total int)) error {
	return idx.IndexWorkspacesWithProgress([]string{rootDir}, onProgress)
}

// IndexWorkspacesWithProgress is like IndexWorkspaces but invokes onProgress
// after each file is scanned, with the total taken from a pre-count pass over
// all roots. onProgress may be nil.
func (idx *Indexer) IndexWorkspacesWithProgress(roots []string, onProgress func(scanned, total int)) error {
	total, err := countYAMLFiles(roots)
	if err != nil {
		return err
	}

	newContents := make(map[string][]byte)
	newIndex := make(map[ResourceKey][]Location)
	newFileIndex := make(map[string][]ResourceKey)

	scanned := 0
	for _, rootDir := range roots {
		err := filepath.Walk(rootDir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return nil // skip unreadable files
			}
			if info.IsDir() || !isYAMLFile(path) {
				return nil
			}

			uri := pathToFileURI(path)
			data, readErr := os.ReadFile(path)
			if readErr != nil {
				return nil
			}
			newContents[uri] = data
			resources, err := scanYAMLBytes(data)
			if err != nil {
				return nil // skip unparseable files
			}

			for _, r := range resources {
				loc := Location{
					URI:    uri,
					Line:   r.Line,
					Column: r.Column,
				}
				key := ResourceKey{Kind: r.Kind, Name: r.Name}
				newIndex[key] = append(newIndex[key], loc)
				newFileIndex[uri] = append(newFileIndex[uri], key)
			}

			scanned++
			if onProgress != nil {
				onProgress(scanned, total)
			}

			return nil
		})
		if err != nil {
			return err
		}
	}

	idx.mu.Lock()
	idx.contents = newContents
	idx.rootDirs = roots
	idx.index = newIndex
	idx.fileIndex = newFileIndex
	idx.version++
	idx.mu.Unlock()

	return nil
}

// Version returns a monotonic counter that changes whenever the index is
// mutated (a file is scanned, updated, or removed). It lets callers cheaply
// detect "did the index change since I last looked".
func (idx *Indexer) Version() uint64 {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	return idx.version
}

// countYAMLFiles counts the YAML files under the given roots in a separate
// pass so IndexWorkspacesWithProgress can report an accurate total. Unreadable
// subtrees are skipped.
func countYAMLFiles(roots []string) (int, error) {
	count := 0
	for _, rootDir := range roots {
		err := filepath.Walk(rootDir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return nil
			}
			if !info.IsDir() && isYAMLFile(path) {
				count++
			}
			return nil
		})
		if err != nil {
			return count, err
		}
	}
	return count, nil
}

// UpdateFile re-indexes a single file, replacing any previous entries.
func (idx *Indexer) UpdateFile(uri string, content []byte) error {
	idx.mu.Lock()
	defer idx.mu.Unlock()
	if idx.contents == nil {
		idx.contents = map[string][]byte{}
	}
	idx.contents[uri] = append([]byte(nil), content...)

	// Remove old entries for this file
	if oldKeys, ok := idx.fileIndex[uri]; ok {
		for _, key := range oldKeys {
			locations := idx.index[key]
			for i := len(locations) - 1; i >= 0; i-- {
				if locations[i].URI == uri {
					idx.index[key] = append(locations[:i], locations[i+1:]...)
				}
			}
			if len(idx.index[key]) == 0 {
				delete(idx.index, key)
			}
		}
		delete(idx.fileIndex, uri)
	}

	idx.version++

	// Parse and index new content
	resources, err := scanYAMLBytes(content)
	if err != nil {
		return nil // skip unparseable content
	}

	for _, r := range resources {
		loc := Location{
			URI:    uri,
			Line:   r.Line,
			Column: r.Column,
		}
		key := ResourceKey{Kind: r.Kind, Name: r.Name}
		idx.index[key] = append(idx.index[key], loc)
		idx.fileIndex[uri] = append(idx.fileIndex[uri], key)
	}

	return nil
}

// RemoveFile removes all index entries for a file.
func (idx *Indexer) RemoveFile(uri string) {
	idx.mu.Lock()
	defer idx.mu.Unlock()
	delete(idx.contents, uri)

	if oldKeys, ok := idx.fileIndex[uri]; ok {
		for _, key := range oldKeys {
			locations := idx.index[key]
			for i := len(locations) - 1; i >= 0; i-- {
				if locations[i].URI == uri {
					idx.index[key] = append(locations[:i], locations[i+1:]...)
				}
			}
			if len(idx.index[key]) == 0 {
				delete(idx.index, key)
			}
		}
		delete(idx.fileIndex, uri)
	}

	idx.version++
}

// FindDefinition returns all locations for a resource with the given kind and name.
func (idx *Indexer) FindDefinition(kind, name string) []Location {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	key := ResourceKey{Kind: kind, Name: name}
	locs := idx.index[key]
	return locs
}

// FindAllKeys returns all resource keys in the index (for debugging).
func (idx *Indexer) FindAllKeys() []ResourceKey {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	keys := make([]ResourceKey, 0, len(idx.index))
	for k := range idx.index {
		keys = append(keys, k)
	}
	return keys
}

// FindDefinitionByKind returns all locations for resources of a given kind.
func (idx *Indexer) FindDefinitionByKind(kind string) []Location {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	var result []Location
	for key, locs := range idx.index {
		if key.Kind == kind {
			result = append(result, locs...)
		}
	}
	return result
}

// FindByName returns all locations for resources with the given name, regardless of kind.
func (idx *Indexer) FindByName(name string) []Location {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	var result []Location
	for key, locs := range idx.index {
		if key.Name == name {
			result = append(result, locs...)
		}
	}
	return result
}

// FindAllKinds returns all unique kind names in the index.
func (idx *Indexer) FindAllKinds() []string {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	seen := make(map[string]bool)
	var kinds []string
	for key := range idx.index {
		if !seen[key.Kind] {
			seen[key.Kind] = true
			kinds = append(kinds, key.Kind)
		}
	}
	return kinds
}

// URIs returns the file:// URIs of every indexed file. It lets watchers walk
// the index and stat each entry to detect deletions whose names no longer
// appear in a directory traversal.
func (idx *Indexer) URIs() []string {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	uris := make([]string, 0, len(idx.fileIndex))
	for uri := range idx.fileIndex {
		uris = append(uris, uri)
	}
	return uris
}

// ResourceNames returns the metadata.name of every indexed resource. A
// `name:` reference resolves against these; file basenames never satisfy a
// reference, only resource names do.
func (idx *Indexer) ResourceNames() []string {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	return idx.resourceNamesLocked()
}

// resourceNamesLocked returns the unique resource metadata.names while the
// index lock is held.
func (idx *Indexer) resourceNamesLocked() []string {
	seen := make(map[string]bool)
	var names []string
	for key := range idx.index {
		if key.Name != "" {
			seen[key.Name] = true
			names = append(names, key.Name)
		}
	}
	return names
}

// NamesByKind returns the unique metadata.names of every indexed resource,
// grouped by resource kind. It powers reference autocompletion: a `name:`
// value under an object that also carries a `kind:` sibling can be filtered to
// the names of that kind. Kinds with no resources are omitted.
func (idx *Indexer) NamesByKind() map[string][]string {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	names := make(map[string][]string)
	seen := make(map[ResourceKey]bool)
	for key := range idx.index {
		if key.Name == "" || seen[key] {
			continue
		}
		seen[key] = true
		names[key.Kind] = append(names[key.Kind], key.Name)
	}
	return names
}

// FileNames returns the unique basenames (without .yaml/.yml extension) of all
// indexed YAML files, plus the resource names (metadata.name) declared inside
// them. References are matched against both, so a resource whose file name
// differs from its metadata.name is still a valid fix target.
func (idx *Indexer) FileNames() []string {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	names := idx.resourceNamesLocked()
	seen := make(map[string]bool, len(names)+len(idx.fileIndex))
	for _, n := range names {
		seen[n] = true
	}
	for uri := range idx.fileIndex {
		base := filepath.Base(uri)
		name := strings.TrimSuffix(base, filepath.Ext(base))
		if name != "" && !seen[name] {
			seen[name] = true
			names = append(names, name)
		}
	}
	return names
}

// FileCount returns the number of files tracked by the indexer.
func (idx *Indexer) FileCount() int {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	return len(idx.fileIndex)
}

// ResourceCount returns the number of unique resources indexed.
func (idx *Indexer) ResourceCount() int {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	return len(idx.index)
}

// ResourceEntry holds parsed resource info from a single YAML file.
type ResourceEntry struct {
	Kind   string
	Name   string
	Line   int
	Column int
}

// scanYAMLFile reads and parses a YAML file, returning all OpenChoreo resources found.
func scanYAMLFile(path string) ([]ResourceEntry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return scanYAMLBytes(data)
}

// scanYAMLBytes parses raw YAML bytes and extracts resource kind+name pairs.
func scanYAMLBytes(data []byte) ([]ResourceEntry, error) {
	decoder := yaml.NewDecoder(bytes.NewReader(data))

	var resources []ResourceEntry
	for {
		var node yaml.Node
		err := decoder.Decode(&node)
		if err != nil {
			if err == io.EOF {
				break
			}
			break // skip parse errors
		}
		if node.Kind == 0 {
			break
		}

		// Unwrap document node
		root := &node
		if root.Kind == yaml.DocumentNode {
			if len(root.Content) == 0 {
				continue
			}
			root = root.Content[0]
		}

		if root.Kind != yaml.MappingNode {
			continue
		}

		entry := extractResource(root)
		if entry != nil {
			resources = append(resources, *entry)
		}
	}

	return resources, nil
}

// extractResource extracts kind and metadata.name from a YAML mapping node.
func extractResource(node *yaml.Node) *ResourceEntry {
	if node == nil || node.Kind != yaml.MappingNode {
		return nil
	}

	var kind string
	var kindLine, kindCol int
	var name string

	for i := 0; i+1 < len(node.Content); i += 2 {
		key := node.Content[i]
		val := node.Content[i+1]

		switch key.Value {
		case "kind":
			if val.Kind == yaml.ScalarNode {
				kind = val.Value
				kindLine = val.Line
				kindCol = val.Column
			}
		case "metadata":
			if val.Kind == yaml.MappingNode {
				for j := 0; j+1 < len(val.Content); j += 2 {
					metaKey := val.Content[j]
					metaVal := val.Content[j+1]
					if metaKey.Value == "name" && metaVal.Kind == yaml.ScalarNode {
						name = metaVal.Value
					}
				}
			}
		}
	}

	if kind == "" || name == "" {
		return nil
	}

	return &ResourceEntry{
		Kind:   kind,
		Name:   name,
		Line:   kindLine,
		Column: kindCol,
	}
}

// isYAMLFile checks if a file has a YAML extension.
func isYAMLFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return ext == ".yaml" || ext == ".yml"
}

// pathToFileURI converts an OS path to a file:// URI.
func pathToFileURI(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		return path
	}
	// On macOS/Linux, just prefix with file://
	if !strings.HasPrefix(abs, "file://") {
		return "file://" + abs
	}
	return abs
}
