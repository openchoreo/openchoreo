// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package secret

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
)

// parseTargetPlane parses a "Kind/Name" string into a TargetPlaneRef.
func parseTargetPlane(raw string) (*gen.TargetPlaneRef, error) {
	parts := strings.SplitN(raw, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return nil, fmt.Errorf("invalid --target-plane %q: expected Kind/Name (e.g. DataPlane/dp-prod)", raw)
	}
	kind := parts[0]
	switch gen.TargetPlaneRefKind(kind) {
	case gen.TargetPlaneRefKindDataPlane,
		gen.TargetPlaneRefKindClusterDataPlane,
		gen.TargetPlaneRefKindWorkflowPlane,
		gen.TargetPlaneRefKindClusterWorkflowPlane:
	default:
		return nil, fmt.Errorf("invalid --target-plane kind %q: must be one of DataPlane, ClusterDataPlane, WorkflowPlane, ClusterWorkflowPlane", kind)
	}
	return &gen.TargetPlaneRef{
		Kind: gen.TargetPlaneRefKind(kind),
		Name: parts[1],
	}, nil
}

// collectData merges --from-literal, --from-file, and --from-env-file inputs
// into a single map. Later sources override earlier ones, matching kubectl behavior.
func collectData(literals, files, envFiles []string) (map[string]string, error) {
	data := map[string]string{}

	for _, lit := range literals {
		k, v, err := parseLiteral(lit)
		if err != nil {
			return nil, err
		}
		data[k] = v
	}

	for _, f := range files {
		k, v, err := readFile(f)
		if err != nil {
			return nil, err
		}
		data[k] = v
	}

	for _, ef := range envFiles {
		entries, err := readEnvFile(ef)
		if err != nil {
			return nil, err
		}
		for k, v := range entries {
			data[k] = v
		}
	}

	return data, nil
}

func parseLiteral(s string) (string, string, error) {
	idx := strings.Index(s, "=")
	if idx <= 0 {
		return "", "", fmt.Errorf("invalid --from-literal %q: expected key=value", s)
	}
	return s[:idx], s[idx+1:], nil
}

// readFile parses a --from-file value: either "key=path" or "path" (key inferred
// from the filename).
func readFile(s string) (string, string, error) {
	var key, path string
	if idx := strings.Index(s, "="); idx > 0 {
		key = s[:idx]
		path = s[idx+1:]
	} else {
		path = s
		key = filepath.Base(s)
	}
	if path == "" {
		return "", "", fmt.Errorf("invalid --from-file %q: missing path", s)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return "", "", fmt.Errorf("read --from-file %s: %w", path, err)
	}
	return key, string(b), nil
}

// readEnvFile parses a KEY=VALUE file. Blank lines and lines starting with '#'
// are ignored.
func readEnvFile(path string) (map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("read --from-env-file %s: %w", path, err)
	}
	defer f.Close()

	out := map[string]string{}
	scanner := bufio.NewScanner(f)
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		idx := strings.Index(line, "=")
		if idx <= 0 {
			return nil, fmt.Errorf("--from-env-file %s: line %d: expected KEY=VALUE", path, lineNo)
		}
		out[line[:idx]] = line[idx+1:]
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read --from-env-file %s: %w", path, err)
	}
	return out, nil
}
