// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package cache

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
)

func TestAtomicWriteFile_CreatesAndReplaces(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "index.json")

	first := []byte(`{"version":1}`)
	if err := atomicWriteFile(path, first); err != nil {
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
	if err := atomicWriteFile(path, second); err != nil {
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

	assertValidCache(t, repoPath)
}

func TestLoadOrBuild_ConcurrentProcesses(t *testing.T) {
	if os.Getenv("FSINDEX_CACHE_HELPER") == "1" {
		runCacheHelperProcess(t)
		return
	}

	repoPath := setupTestRepo(t)
	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable(): %v", err)
	}

	const workers = 8
	cmds := make([]*exec.Cmd, 0, workers)
	for i := range workers {
		mode := "load"
		if i%2 == 0 {
			mode = "force"
		}
		cmd := exec.Command(exe, "-test.run=^TestLoadOrBuild_ConcurrentProcesses$", "-test.v")
		cmd.Env = append(os.Environ(),
			"FSINDEX_CACHE_HELPER=1",
			"FSINDEX_CACHE_REPO="+repoPath,
			"FSINDEX_CACHE_MODE="+mode,
		)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Start(); err != nil {
			t.Fatalf("start helper %d: %v", i, err)
		}
		cmds = append(cmds, cmd)
	}

	for i, cmd := range cmds {
		if err := cmd.Wait(); err != nil {
			t.Fatalf("helper %d failed: %v", i, err)
		}
	}

	assertValidCache(t, repoPath)
}

func TestClearCache_ConcurrentWithLoadOrBuild(t *testing.T) {
	repoPath := setupTestRepo(t)
	if _, err := LoadOrBuild(repoPath); err != nil {
		t.Fatalf("initial LoadOrBuild(): %v", err)
	}

	const workers = 12
	var wg sync.WaitGroup
	errs := make(chan error, workers)

	wg.Add(workers)
	for i := range workers {
		go func(i int) {
			defer wg.Done()
			if i%3 == 0 {
				if err := ClearCache(repoPath); err != nil {
					errs <- err
				}
				return
			}
			pi, err := LoadOrBuild(repoPath)
			if err != nil {
				errs <- err
				return
			}
			if pi == nil || pi.Index == nil {
				errs <- errors.New("LoadOrBuild returned nil index")
			}
		}(i)
	}
	wg.Wait()
	close(errs)

	for err := range errs {
		t.Fatalf("concurrent clear/load error: %v", err)
	}

	// After racing clear and load, a fresh load should still succeed.
	pi, err := LoadOrBuild(repoPath)
	if err != nil {
		t.Fatalf("final LoadOrBuild(): %v", err)
	}
	if pi.Index.Stats().TotalResources == 0 {
		t.Fatal("expected indexed resources after concurrent clear/load")
	}
}

func runCacheHelperProcess(t *testing.T) {
	t.Helper()

	repoPath := os.Getenv("FSINDEX_CACHE_REPO")
	if repoPath == "" {
		t.Fatal("FSINDEX_CACHE_REPO is required")
	}

	var err error
	switch os.Getenv("FSINDEX_CACHE_MODE") {
	case "force":
		_, err = ForceRebuild(repoPath)
	default:
		_, err = LoadOrBuild(repoPath)
	}
	if err != nil {
		t.Fatalf("helper cache op: %v", err)
	}
}

func assertValidCache(t *testing.T, repoPath string) {
	t.Helper()

	indexPath := filepath.Join(repoPath, DirName, IndexFile)
	metaPath := filepath.Join(repoPath, DirName, MetadataFile)

	indexData, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatalf("read index.json: %v", err)
	}
	if !json.Valid(indexData) {
		t.Fatalf("index.json is not valid JSON: %s", indexData)
	}

	metaData, err := os.ReadFile(metaPath)
	if err != nil {
		t.Fatalf("read metadata.json: %v", err)
	}
	if !json.Valid(metaData) {
		t.Fatalf("metadata.json is not valid JSON: %s", metaData)
	}

	pi, err := LoadOrBuild(repoPath)
	if err != nil {
		t.Fatalf("final LoadOrBuild() error: %v", err)
	}
	if pi.Index.Stats().TotalResources == 0 {
		t.Fatal("expected indexed resources")
	}
}
