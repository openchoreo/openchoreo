// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package scanner

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/openchoreo/openchoreo/pkg/fsindex/index"
)

// ScanOptions configures the scanner behavior
type ScanOptions struct {
	Workers      int
	Filter       *FileFilter
	Verbose      bool
	ErrorHandler func(path string, err error)
}

// DefaultScanOptions returns default scanning options
func DefaultScanOptions() ScanOptions {
	return ScanOptions{
		Workers: 10,
		Filter:  DefaultFilter(),
		Verbose: false,
		ErrorHandler: func(path string, err error) {
			// Default: ignore errors silently
		},
	}
}

// Scanner handles repository scanning
type Scanner struct {
	opts ScanOptions
}

// New creates a new scanner with the given options
func New(opts ScanOptions) *Scanner {
	if opts.Filter == nil {
		opts.Filter = DefaultFilter()
	}
	if opts.Workers <= 0 {
		opts.Workers = 10
	}
	return &Scanner{opts: opts}
}

// Scan scans a repository and builds an index
func (s *Scanner) Scan(repoPath string) (*index.Index, error) {
	idx := index.New(repoPath)

	// Check if repo path exists
	if _, err := os.Stat(repoPath); err != nil {
		return nil, fmt.Errorf("repo path does not exist: %w", err)
	}

	// Channels for pipeline
	files := make(chan string, 100)
	results := make(chan *index.ResourceEntry, 100)
	errors := make(chan error, 10)

	// Start file discovery goroutine
	go func() {
		defer close(files)
		s.discoverFiles(repoPath, files, errors)
	}()

	// Start worker pool for parsing
	var wg sync.WaitGroup
	for i := 0; i < s.opts.Workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.processFiles(files, results, errors)
		}()
	}

	// Close results channel when all workers are done
	go func() {
		wg.Wait()
		close(results)
		close(errors)
	}()

	// Collect results and build index
	var errs []error
	done := make(chan struct{})

	// Error collector
	go func() {
		for err := range errors {
			errs = append(errs, err)
		}
	}()

	// Result collector
	go func() {
		for entry := range results {
			if err := idx.Add(entry); err != nil && s.opts.ErrorHandler != nil {
				s.opts.ErrorHandler(entry.FilePath, err)
			}
		}
		close(done)
	}()

	<-done

	// Return first error if any occurred
	if len(errs) > 0 && s.opts.Verbose {
		return idx, fmt.Errorf("scan completed with %d errors: first error: %w", len(errs), errs[0])
	}

	return idx, nil
}

// discoverFiles walks the directory tree and sends file paths to the channel
func (s *Scanner) discoverFiles(root string, files chan<- string, errors chan<- error) {
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			if s.opts.ErrorHandler != nil {
				s.opts.ErrorHandler(path, err)
			}
			return nil // Continue walking
		}

		// Skip directories we shouldn't descend into
		if info.IsDir() {
			if !s.opts.Filter.ShouldDescendIntoDir(info.Name()) && path != root {
				return filepath.SkipDir
			}
			return nil
		}

		// Check if file should be scanned
		if s.opts.Filter.ShouldScan(path) {
			files <- path
		}

		return nil
	})

	if err != nil {
		errors <- fmt.Errorf("error walking directory tree: %w", err)
	}
}

// processFiles parses files from the input channel and sends results
func (s *Scanner) processFiles(files <-chan string, results chan<- *index.ResourceEntry, _ chan<- error) {
	for path := range files {
		entries, err := ParseYAMLFile(path)
		if err != nil {
			if s.opts.ErrorHandler != nil {
				s.opts.ErrorHandler(path, err)
			}
			continue
		}

		for _, entry := range entries {
			// Validate resource before adding
			if err := ValidateResource(entry); err != nil {
				if s.opts.ErrorHandler != nil {
					s.opts.ErrorHandler(path, fmt.Errorf("invalid resource: %w", err))
				}
				continue
			}

			results <- entry
		}
	}
}

// ScanRepository is a convenience function to scan a repository with default options
func ScanRepository(repoPath string) (*index.Index, error) {
	scanner := New(DefaultScanOptions())
	return scanner.Scan(repoPath)
}

// ScanRepositoryWithOptions scans a repository with custom options
func ScanRepositoryWithOptions(repoPath string, opts ScanOptions) (*index.Index, error) {
	scanner := New(opts)
	return scanner.Scan(repoPath)
}
