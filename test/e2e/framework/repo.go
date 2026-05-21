// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package framework

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// RepoRoot returns the absolute path of the OpenChoreo checkout by walking up
// from this file's location until it finds a `go.mod`. Suites use this to
// apply files under `samples/` regardless of where `go test` was invoked.
func RepoRoot() (string, error) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		return "", fmt.Errorf("runtime.Caller failed to resolve framework path")
	}
	dir := filepath.Dir(thisFile)
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("go.mod not found walking up from %s", filepath.Dir(thisFile))
		}
		dir = parent
	}
}
