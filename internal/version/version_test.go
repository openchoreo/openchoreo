// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package version

import (
	"runtime"
	"testing"
)

func TestGet_ReturnsInfo(t *testing.T) {
	info := Get()

	// Build-time defaults when not set by linker
	if info.Name == "" {
		t.Error("Get().Name should not be empty")
	}
	if info.Version == "" {
		t.Error("Get().Version should not be empty")
	}
	if info.GitRevision == "" {
		t.Error("Get().GitRevision should not be empty")
	}
	if info.BuildTime == "" {
		t.Error("Get().BuildTime should not be empty")
	}
	if info.GoOS != runtime.GOOS {
		t.Errorf("Get().GoOS = %q, want %q", info.GoOS, runtime.GOOS)
	}
	if info.GoArch != runtime.GOARCH {
		t.Errorf("Get().GoArch = %q, want %q", info.GoArch, runtime.GOARCH)
	}
	if info.GoVersion != runtime.Version() {
		t.Errorf("Get().GoVersion = %q, want %q", info.GoVersion, runtime.Version())
	}
}

func TestGetLogKeyValues_ReturnsKeyValuePairs(t *testing.T) {
	kvs := GetLogKeyValues()

	// Should return even number of key-value pairs
	if len(kvs)%2 != 0 {
		t.Errorf("GetLogKeyValues() returned odd-length slice: %d", len(kvs))
	}

	// Should have at least name, version, gitRevision, buildTime, goOS, goArch, goVersion
	if len(kvs) < 14 {
		t.Errorf("GetLogKeyValues() returned %d elements, want at least 14", len(kvs))
	}

	// Verify key names
	expectedKeys := []string{"name", "version", "gitRevision", "buildTime", "goOS", "goArch", "goVersion"}
	for i, key := range expectedKeys {
		idx := i * 2
		if kvs[idx] != key {
			t.Errorf("GetLogKeyValues()[%d] = %q, want %q", idx, kvs[idx], key)
		}
	}
}

func TestGetLogKeyValues_MatchesGet(t *testing.T) {
	info := Get()
	kvs := GetLogKeyValues()

	// Build a map from the key-value pairs
	m := make(map[string]string)
	for i := 0; i < len(kvs)-1; i += 2 {
		m[kvs[i].(string)] = kvs[i+1].(string)
	}

	if m["name"] != info.Name {
		t.Errorf("name mismatch: kv=%q info=%q", m["name"], info.Name)
	}
	if m["version"] != info.Version {
		t.Errorf("version mismatch: kv=%q info=%q", m["version"], info.Version)
	}
	if m["goOS"] != info.GoOS {
		t.Errorf("goOS mismatch: kv=%q info=%q", m["goOS"], info.GoOS)
	}
	if m["goArch"] != info.GoArch {
		t.Errorf("goArch mismatch: kv=%q info=%q", m["goArch"], info.GoArch)
	}
}
