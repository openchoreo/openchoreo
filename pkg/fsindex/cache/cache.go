// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package cache

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/openchoreo/openchoreo/pkg/fsindex/index"
	"github.com/openchoreo/openchoreo/pkg/fsindex/scanner"
)

const (
	DirName      = ".occ"
	IndexFile    = "index.json"
	MetadataFile = "metadata.json"
)

// FileState tracks the state of a single file for change detection
type FileState struct {
	ModTime time.Time `json:"modTime"`
	Size    int64     `json:"size"`
	Hash    string    `json:"hash"`
}

// CacheMetadata tracks cache state for invalidation
type CacheMetadata struct {
	// Directory-level hash for fast "anything changed?" check
	DirectoryHash string `json:"directoryHash"`

	// Per-file state for incremental updates
	FileStates map[string]FileState `json:"fileStates"`

	// Time-based metadata
	CreatedAt time.Time `json:"createdAt"`
	LastUsed  time.Time `json:"lastUsed"`

	// Index stats
	ResourceCount int `json:"resourceCount"`
}

// PersistentIndex wraps generic Index with filesystem persistence
type PersistentIndex struct {
	*index.Index
	repoPath string
	cacheDir string
	metadata *CacheMetadata
}

// LoadOrBuild loads existing index from cache or builds a new one
func LoadOrBuild(repoPath string) (*PersistentIndex, error) {
	pi := &PersistentIndex{
		Index:    index.New(repoPath),
		repoPath: repoPath,
		cacheDir: filepath.Join(repoPath, DirName),
	}

	// Try to load from cache
	if pi.isCacheValid() {
		if err := pi.loadFromDisk(); err == nil {
			// Check if incremental update is needed
			changedFiles, err := pi.getChangedFiles()
			if err == nil && len(changedFiles) > 0 {
				if err := pi.incrementalUpdate(changedFiles); err != nil {
					// If incremental update fails, do full rebuild
					return pi.fullRebuild()
				}
			}
			// Always save to update LastUsed timestamp
			if err := pi.saveToDisk(); err != nil {
				return nil, fmt.Errorf("failed to save updated index: %w", err)
			}
			return pi, nil
		}
	}

	// Full rebuild needed
	return pi.fullRebuild()
}

// fullRebuild performs a complete scan and builds the index from scratch
func (pi *PersistentIndex) fullRebuild() (*PersistentIndex, error) {
	// Use scanner to build index
	s := scanner.New(scanner.DefaultScanOptions())
	idx, err := s.Scan(pi.repoPath)
	if err != nil {
		return nil, fmt.Errorf("failed to scan repository: %w", err)
	}

	// Use the scanned generic index directly
	pi.Index = idx

	// Compute directory hash and file states
	dirHash, fileStates, err := pi.computeDirectoryState()
	if err != nil {
		return nil, fmt.Errorf("failed to compute directory state: %w", err)
	}

	pi.metadata = &CacheMetadata{
		DirectoryHash: dirHash,
		FileStates:    fileStates,
		CreatedAt:     time.Now(),
		LastUsed:      time.Now(),
		ResourceCount: pi.Index.Stats().TotalResources,
	}

	// Save to disk
	if err := pi.saveToDisk(); err != nil {
		return nil, fmt.Errorf("failed to save index: %w", err)
	}

	return pi, nil
}

// isCacheValid checks if the cached index can be used
func (pi *PersistentIndex) isCacheValid() bool {
	metaPath := filepath.Join(pi.cacheDir, MetadataFile)
	data, err := os.ReadFile(metaPath)
	if err != nil {
		return false
	}

	var meta CacheMetadata
	if err := json.Unmarshal(data, &meta); err != nil {
		return false
	}

	// Assign metadata to pi so computeCurrentDirectoryHash can use its FileStates
	pi.metadata = &meta

	// Compute current directory hash and compare with cached hash (data changes)
	currentDirHash, err := pi.computeCurrentDirectoryHash()
	if err != nil {
		// If there's an error computing the current hash, assume cache is invalid
		return false
	}
	if currentDirHash != meta.DirectoryHash {
		// Directory hash mismatch, something changed in the files
		return false
	}

	// If both version and directory hash match, the cache is considered valid
	return true
}

// loadFromDisk loads the index from the cache directory
func (pi *PersistentIndex) loadFromDisk() error {
	indexPath := filepath.Join(pi.cacheDir, IndexFile)
	data, err := os.ReadFile(indexPath)
	if err != nil {
		return fmt.Errorf("failed to read index file: %w", err)
	}

	var serializable index.SerializableIndex
	if err := json.Unmarshal(data, &serializable); err != nil {
		return fmt.Errorf("failed to unmarshal index: %w", err)
	}

	// Convert back to generic Index
	pi.Index = serializable.ToIndex(pi.repoPath)

	return nil
}

// saveToDisk persists the index to the filesystem
func (pi *PersistentIndex) saveToDisk() error {
	// Create cache directory
	if err := os.MkdirAll(pi.cacheDir, 0755); err != nil {
		return fmt.Errorf("failed to create cache directory: %w", err)
	}

	// Save index
	indexPath := filepath.Join(pi.cacheDir, IndexFile)
	serializable := pi.Index.ToSerializable()
	indexData, err := json.MarshalIndent(serializable, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal index: %w", err)
	}
	if err := os.WriteFile(indexPath, indexData, 0600); err != nil {
		return fmt.Errorf("failed to write index file: %w", err)
	}

	// Update and save metadata
	pi.metadata.LastUsed = time.Now()
	pi.metadata.ResourceCount = pi.Index.Stats().TotalResources

	metaPath := filepath.Join(pi.cacheDir, MetadataFile)
	metaData, err := json.MarshalIndent(pi.metadata, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}
	if err := os.WriteFile(metaPath, metaData, 0600); err != nil {
		return fmt.Errorf("failed to write metadata file: %w", err)
	}

	return nil
}

// isYAMLFile checks if a file has a YAML extension
func isYAMLFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return ext == ".yaml" || ext == ".yml"
}

// hashFileContent computes SHA256 hash of file contents
func hashFileContent(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:]), nil
}

// computeDirectoryState computes the directory hash and per-file states
// This is the core of the cache invalidation mechanism
func (pi *PersistentIndex) computeDirectoryState() (string, map[string]FileState, error) {
	dirHasher := sha256.New()
	fileStates := make(map[string]FileState)

	// Collect all YAML files
	var paths []string
	err := filepath.WalkDir(pi.repoPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // Skip errors
		}

		// Skip cache directory
		if d.IsDir() && d.Name() == DirName {
			return filepath.SkipDir
		}

		// Skip hidden directories
		if d.IsDir() && strings.HasPrefix(d.Name(), ".") {
			return filepath.SkipDir
		}

		if !d.IsDir() && isYAMLFile(path) {
			paths = append(paths, path)
		}
		return nil
	})
	if err != nil {
		return "", nil, fmt.Errorf("failed to walk directory: %w", err)
	}

	// Sort for deterministic hash
	sort.Strings(paths)

	// Process each file
	for _, path := range paths {
		relPath, err := filepath.Rel(pi.repoPath, path)
		if err != nil {
			continue
		}

		info, err := os.Stat(path)
		if err != nil {
			continue
		}

		// Compute content hash
		contentHash, err := hashFileContent(path)
		if err != nil {
			continue
		}

		// Store file state
		fileStates[relPath] = FileState{
			ModTime: info.ModTime(),
			Size:    info.Size(),
			Hash:    contentHash,
		}

		// Add to directory hash: "relPath|size|hash\n"
		entry := fmt.Sprintf("%s|%d|%s\n", relPath, info.Size(), contentHash)
		dirHasher.Write([]byte(entry))
	}

	return hex.EncodeToString(dirHasher.Sum(nil)), fileStates, nil
}

// computeCurrentDirectoryHash quickly computes the current directory hash
// using only metadata (size + mtime) for speed, falling back to content hash
// only when metadata suggests a change
func (pi *PersistentIndex) computeCurrentDirectoryHash() (string, error) {
	dirHasher := sha256.New()

	var paths []string
	err := filepath.WalkDir(pi.repoPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() && (d.Name() == DirName || strings.HasPrefix(d.Name(), ".")) {
			return filepath.SkipDir
		}
		if !d.IsDir() && isYAMLFile(path) {
			paths = append(paths, path)
		}
		return nil
	})
	if err != nil {
		return "", err
	}

	sort.Strings(paths)

	for _, path := range paths {
		relPath, _ := filepath.Rel(pi.repoPath, path)
		info, err := os.Stat(path)
		if err != nil {
			continue
		}

		// Use cached hash if file hasn't changed (size + mtime match)
		var contentHash string
		if cached, ok := pi.metadata.FileStates[relPath]; ok {
			if cached.Size == info.Size() && cached.ModTime.Equal(info.ModTime()) {
				// File unchanged - use cached hash (fast path)
				contentHash = cached.Hash
			} else {
				// File changed - recompute hash
				var err error
				contentHash, err = hashFileContent(path)
				if err != nil {
					return "", fmt.Errorf("failed to hash changed file %s: %w", relPath, err)
				}
			}
		} else {
			// New file - compute hash
			var err error
			contentHash, err = hashFileContent(path)
			if err != nil {
				return "", fmt.Errorf("failed to hash new file %s: %w", relPath, err)
			}
		}

		entry := fmt.Sprintf("%s|%d|%s\n", relPath, info.Size(), contentHash)
		dirHasher.Write([]byte(entry))
	}

	return hex.EncodeToString(dirHasher.Sum(nil)), nil
}

// getChangedFiles detects which files have changed using hash comparison
func (pi *PersistentIndex) getChangedFiles() ([]string, error) {
	if pi.metadata == nil || pi.metadata.FileStates == nil {
		return nil, nil
	}

	// Quick check: compute current directory hash
	currentDirHash, err := pi.computeCurrentDirectoryHash()
	if err != nil {
		return nil, err
	}

	// If directory hash matches, nothing changed
	if currentDirHash == pi.metadata.DirectoryHash {
		return nil, nil
	}

	// Something changed - find what
	var changedFiles []string
	currentFiles := make(map[string]bool)

	// Walk current files
	err = filepath.WalkDir(pi.repoPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() && (d.Name() == DirName || strings.HasPrefix(d.Name(), ".")) {
			return filepath.SkipDir
		}
		if d.IsDir() || !isYAMLFile(path) {
			return nil
		}

		relPath, _ := filepath.Rel(pi.repoPath, path)
		currentFiles[relPath] = true

		info, err := os.Stat(path)
		if err != nil {
			return nil
		}

		cached, exists := pi.metadata.FileStates[relPath]
		if !exists {
			// New file
			changedFiles = append(changedFiles, relPath)
			return nil
		}

		// Fast check: size and mtime
		if cached.Size != info.Size() || !cached.ModTime.Equal(info.ModTime()) {
			// Potentially changed - verify with hash
			currentHash, err := hashFileContent(path)
			if err != nil {
				changedFiles = append(changedFiles, relPath)
				return nil
			}
			if currentHash != cached.Hash {
				changedFiles = append(changedFiles, relPath)
			}
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	// Check for deleted files
	for cachedPath := range pi.metadata.FileStates {
		if !currentFiles[cachedPath] {
			changedFiles = append(changedFiles, cachedPath)
		}
	}

	return changedFiles, nil
}

// incrementalUpdate only re-indexes changed files
func (pi *PersistentIndex) incrementalUpdate(changedFiles []string) error {
	for _, file := range changedFiles {
		fullPath := filepath.Join(pi.repoPath, file)

		// Check if file was deleted
		if _, err := os.Stat(fullPath); os.IsNotExist(err) {
			pi.Index.RemoveEntriesForFile(fullPath)
			delete(pi.metadata.FileStates, file)
			continue
		}

		// Re-parse the file
		entries, err := scanner.ParseYAMLFile(fullPath)
		if err != nil {
			// Skip invalid files but still update state
			info, statErr := os.Stat(fullPath)
			if statErr == nil {
				hash, hashErr := hashFileContent(fullPath)
				if hashErr != nil {
					// If we can't hash the file, skip updating its state
					continue
				}
				pi.metadata.FileStates[file] = FileState{
					ModTime: info.ModTime(),
					Size:    info.Size(),
					Hash:    hash,
				}
			}
			continue
		}

		// Remove old entries and add new ones
		pi.Index.RemoveEntriesForFile(fullPath)
		for _, entry := range entries {
			if err := pi.Index.Add(entry); err != nil {
				return fmt.Errorf("failed to add entry from %s: %w", file, err)
			}
		}

		// Update file state
		info, err := os.Stat(fullPath)
		if err == nil {
			hash, hashErr := hashFileContent(fullPath)
			if hashErr != nil {
				return fmt.Errorf("failed to hash file %s: %w", file, hashErr)
			}
			pi.metadata.FileStates[file] = FileState{
				ModTime: info.ModTime(),
				Size:    info.Size(),
				Hash:    hash,
			}
		}
	}

	// Recompute directory hash
	dirHash, _, err := pi.computeDirectoryState()
	if err != nil {
		return fmt.Errorf("failed to recompute directory hash: %w", err)
	}
	pi.metadata.DirectoryHash = dirHash
	pi.metadata.LastUsed = time.Now()

	return nil
}

// ForceRebuild forces a full rebuild of the index, ignoring cache
func ForceRebuild(repoPath string) (*PersistentIndex, error) {
	pi := &PersistentIndex{
		Index:    index.New(repoPath),
		repoPath: repoPath,
		cacheDir: filepath.Join(repoPath, DirName),
	}
	return pi.fullRebuild()
}

// ClearCache removes the cache directory
func ClearCache(repoPath string) error {
	cacheDir := filepath.Join(repoPath, DirName)
	if err := os.RemoveAll(cacheDir); err != nil {
		return fmt.Errorf("failed to clear cache: %w", err)
	}
	return nil
}
