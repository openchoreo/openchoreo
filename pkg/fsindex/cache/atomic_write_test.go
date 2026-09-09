// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package cache

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestAtomicWriteFile_CreatesAndReplaces(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "index.json")

	first := []byte(`{"version":1}`)
	if err := atomicWriteFile(path, first, 0600); err != nil {
		t.Fatalf("atomicWriteFile() first write: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() after first write: %v", err)
	}
	if string(got) != string(first) {
		t.Fatalf("content = %q, want %q", got, first)
	}

	second := []byte(`{"version":2,"resources":[]}`)
	if err := atomicWriteFile(path, second, 0600); err != nil {
		t.Fatalf("atomicWriteFile() replace: %v", err)
	}

	got, err = os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() after replace: %v", err)
	}
	if string(got) != string(second) {
		t.Fatalf("content = %q, want %q", got, second)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir(): %v", err)
	}
	for _, entry := range entries {
		if entry.Name() != "index.json" {
			t.Fatalf("unexpected leftover file %q", entry.Name())
		}
	}
}

func TestLoadOrBuild_Concurrent(t *testing.T) {
	repoPath := setupTestRepo(t)

	const workers = 16
	var wg sync.WaitGroup
	errs := make(chan error, workers)

	wg.Add(workers)
	for range workers {
		go func() {
			defer wg.Done()
			pi, err := LoadOrBuild(repoPath)
			if err != nil {
				errs <- err
				return
			}
			if pi == nil || pi.Index == nil {
				errs <- errors.New("LoadOrBuild returned nil index")
			}
		}()
	}
	wg.Wait()
	close(errs)

	for err := range errs {
		t.Fatalf("concurrent LoadOrBuild() error: %v", err)
	}

	indexPath := filepath.Join(repoPath, DirName, IndexFile)
	metaPath := filepath.Join(repoPath, DirName, MetadataFile)

	indexData, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatalf("read index.json: %v", err)
	}
	if !json.Valid(indexData) {
		t.Fatalf("index.json is not valid JSON after concurrent LoadOrBuild: %s", indexData)
	}

	metaData, err := os.ReadFile(metaPath)
	if err != nil {
		t.Fatalf("read metadata.json: %v", err)
	}
	if !json.Valid(metaData) {
		t.Fatalf("metadata.json is not valid JSON after concurrent LoadOrBuild: %s", metaData)
	}

	pi, err := LoadOrBuild(repoPath)
	if err != nil {
		t.Fatalf("final LoadOrBuild() error: %v", err)
	}
	if pi.Index.Stats().TotalResources == 0 {
		t.Fatal("expected indexed resources after concurrent LoadOrBuild")
	}
}
