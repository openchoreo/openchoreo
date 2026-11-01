// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package lint_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestPreRenderSampleCLIModes(t *testing.T) {
	source := filepath.Join("..", "..", "..", "..", "tools", "lint", "testData", "sample-projects", "pre-render-schema-validation")
	dir := t.TempDir()
	for _, name := range []string{"componenttype.yaml", "component.yaml", "autoscaling-trait.yaml", "storage-trait.yaml", "workload.yaml", "environment.yaml", "dataplane.yaml"} {
		data, err := os.ReadFile(filepath.Join(source, name))
		if err != nil {
			t.Fatal(err)
		}
		if name == "componenttype.yaml" {
			text := strings.ReplaceAll(string(data), "\r\n", "\n")
			text = regexp.MustCompile(`(?m)(        replicas:\n          type: )[^\n]+`).ReplaceAllString(text, `${1}string`)
			text = regexp.MustCompile(`(?m)(          default: )"?2"?$`).ReplaceAllString(text, `${1}"2"`)
			data = []byte(text)
		}
		if err := os.WriteFile(filepath.Join(dir, name), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	for _, mode := range []string{"-detect", "-b"} {
		for _, jsonOutput := range []bool{false, true} {
			t.Run(mode+map[bool]string{false: "/text", true: "/json"}[jsonOutput], func(t *testing.T) {
				args := []string{"linter", "vali", mode}
				if jsonOutput {
					args = append(args, "-obj")
				}
				args = append(args, dir)
				code, stdout, stderr := runCLI(t, args...)
				if code != 1 || !strings.Contains(stdout, "schema-parameters-mismatch") || !strings.Contains(stdout, "replicas must be of type string") {
					t.Fatalf("exit=%d stdout=%s stderr=%s", code, stdout, stderr)
				}
			})
		}
	}
	// A matching schema must restore a successful folder-validation exit code.
	path := filepath.Join(dir, "componenttype.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	valid := strings.Replace(string(data), "replicas:\n          type: string\n          default: \"2\"", "replicas:\n          type: integer\n          default: 2", 1)
	if string(data) == valid {
		t.Fatal("sample schema changed; update test setup")
	}
	if err := os.WriteFile(path, []byte(valid), 0600); err != nil {
		t.Fatal(err)
	}
	code, stdout, stderr := runCLI(t, "linter", "vali", "-detect", dir)
	if code != 0 || strings.Contains(stdout, "schema-parameters-mismatch") {
		t.Fatalf("valid sample: exit=%d stdout=%s stderr=%s", code, stdout, stderr)
	}
}

func TestReleasePreRenderCLIEndToEnd(t *testing.T) {
	source := filepath.Join("..", "..", "..", "..", "tools", "lint", "pre-render-validator", "testdata", "releases.yaml")
	dir := t.TempDir()
	data, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(dir, "releases.yaml")
	if err := os.WriteFile(file, data, 0600); err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"single", "-detect", "-b", "json"} {
		args := []string{"linter", "vali"}
		target := dir
		switch mode {
		case "single":
			target = file
		case "json":
			args = append(args, "-detect", "-obj")
		default:
			args = append(args, mode)
		}
		args = append(args, target)
		code, stdout, stderr := runCLI(t, args...)
		if code != 1 {
			t.Fatalf("%s exit=%d stdout=%s stderr=%s", mode, code, stdout, stderr)
		}
		for _, want := range []string{"schema-parameters-mismatch", "schema-environment-configs-mismatch", "pre-render-validation-failed"} {
			if !strings.Contains(stdout, want) {
				t.Fatalf("%s missing %s: %s", mode, want, stdout)
			}
		}
	}
}
