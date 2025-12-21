// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package index

import (
	"fmt"
	"sync"

	"k8s.io/apimachinery/pkg/runtime/schema"
)

// Index is a generic resource index that provides fast lookups by GVK and file path
type Index struct {
	mu sync.RWMutex

	// Generic indexes
	byGVK      map[schema.GroupVersionKind]map[string]*ResourceEntry // GVK -> "ns/name" -> entry
	byFilePath map[string][]*ResourceEntry                           // path -> entries

	// Metadata
	repoPath  string
	commitSHA string
}

// New creates a new empty index
func New(repoPath string) *Index {
	return &Index{
		byGVK:      make(map[schema.GroupVersionKind]map[string]*ResourceEntry),
		byFilePath: make(map[string][]*ResourceEntry),
		repoPath:   repoPath,
	}
}

// Add adds a resource entry to the index
func (idx *Index) Add(entry *ResourceEntry) error {
	if entry == nil || entry.Resource == nil {
		return fmt.Errorf("cannot add nil entry or resource")
	}

	idx.mu.Lock()
	defer idx.mu.Unlock()

	gvk := entry.Resource.GroupVersionKind()
	nsName := entry.NamespacedName()

	// Add to GVK index
	if idx.byGVK[gvk] == nil {
		idx.byGVK[gvk] = make(map[string]*ResourceEntry)
	}
	idx.byGVK[gvk][nsName] = entry

	// Add to file path index
	idx.byFilePath[entry.FilePath] = append(idx.byFilePath[entry.FilePath], entry)

	return nil
}

// Get retrieves a resource by GVK and namespaced name
func (idx *Index) Get(gvk schema.GroupVersionKind, namespace, name string) (*ResourceEntry, bool) {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	nsName := namespace + "/" + name
	if namespace == "" {
		nsName = name
	}

	gvkMap, ok := idx.byGVK[gvk]
	if !ok {
		return nil, false
	}

	entry, ok := gvkMap[nsName]
	return entry, ok
}

// List returns all resources of a specific GVK
func (idx *Index) List(gvk schema.GroupVersionKind) []*ResourceEntry {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	gvkMap, ok := idx.byGVK[gvk]
	if !ok {
		return nil
	}

	entries := make([]*ResourceEntry, 0, len(gvkMap))
	for _, entry := range gvkMap {
		entries = append(entries, entry)
	}

	return entries
}

// ListAll returns all resources across all GVKs
func (idx *Index) ListAll() []*ResourceEntry {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	var entries []*ResourceEntry
	for _, gvkMap := range idx.byGVK {
		for _, entry := range gvkMap {
			entries = append(entries, entry)
		}
	}

	return entries
}

// GetByFile returns all resources from a specific file
func (idx *Index) GetByFile(filePath string) []*ResourceEntry {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	return idx.byFilePath[filePath]
}

// RemoveEntriesForFile removes all entries associated with a file
func (idx *Index) RemoveEntriesForFile(filePath string) {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	entries := idx.byFilePath[filePath]
	if entries == nil {
		return
	}

	// Remove from GVK index
	for _, entry := range entries {
		gvk := entry.Resource.GroupVersionKind()
		nsName := entry.NamespacedName()

		if gvkMap, ok := idx.byGVK[gvk]; ok {
			delete(gvkMap, nsName)
		}
	}

	// Remove from file path index
	delete(idx.byFilePath, filePath)
}

// Stats returns generic statistics about the index
func (idx *Index) Stats() IndexStats {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	totalResources := 0
	gvkCounts := make(map[string]int)

	for gvk, gvkMap := range idx.byGVK {
		count := len(gvkMap)
		totalResources += count
		gvkCounts[gvk.Kind] = count
	}

	return IndexStats{
		TotalResources: totalResources,
		TotalFiles:     len(idx.byFilePath),
		GVKCounts:      gvkCounts,
	}
}

// GetRepoPath returns the repository path
func (idx *Index) GetRepoPath() string {
	return idx.repoPath
}

// GetCommitSHA returns the commit SHA
func (idx *Index) GetCommitSHA() string {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	return idx.commitSHA
}

// SetCommitSHA sets the commit SHA
func (idx *Index) SetCommitSHA(sha string) {
	idx.mu.Lock()
	defer idx.mu.Unlock()
	idx.commitSHA = sha
}

// IndexStats holds generic statistics about the index
type IndexStats struct {
	TotalResources int
	TotalFiles     int
	GVKCounts      map[string]int // Kind -> count
}
