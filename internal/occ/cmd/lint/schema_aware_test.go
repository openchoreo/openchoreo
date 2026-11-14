// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package lint_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

type celReport struct {
	Runs []struct {
		Results []struct {
			RuleID  string `json:"ruleId"`
			Message struct {
				Text string `json:"text"`
			} `json:"message"`
			Locations []struct {
				Physical struct {
					Region struct {
						Line   int `json:"startLine"`
						Column int `json:"startColumn"`
					} `json:"region"`
				} `json:"physicalLocation"`
			} `json:"locations"`
		} `json:"results"`
	} `json:"runs"`
}

func reportCELCodes(t *testing.T, out string) map[string]int {
	t.Helper()
	var report celReport
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatal(err, out)
	}
	codes := map[string]int{}
	for _, run := range report.Runs {
		for _, r := range run.Results {
			if strings.HasPrefix(r.RuleID, "cel-") {
				codes[r.RuleID]++
			}
		}
	}
	return codes
}
func TestDemoSchemaAwareStaticCEL(t *testing.T) {
	source := filepath.Join("..", "..", "..", "..", "tools", "lint", "targetschema", "testdata", "demo", "componenttype.yaml")
	data, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	expression := regexp.MustCompile(`(?m)(rule:\s*)'\$\{(?:resource|resocurce)\.[^']+\}'`)
	if expression.Find(data) == nil {
		t.Fatal("demo post-render rule changed")
	}
	for _, tc := range []struct{ expr, code, token string }{{"resource.spec.replicas <= 2", "", ""}, {"resource.specc.replicas <= 2", "cel-type-error", "specc"}, {"resource.spec.repclicas <= 2", "cel-type-error", "repclicas"}, {"resocurce.spec.replicas <= 2", "cel-unknown-name", "resocurce"}} {
		t.Run(tc.expr, func(t *testing.T) {
			root := t.TempDir()
			file := filepath.Join(root, "componenttype.yaml")
			text := expression.ReplaceAllStringFunc(string(data), func(match string) string { return "rule: '${" + tc.expr + "}'" })
			if err := os.WriteFile(file, []byte(text), 0600); err != nil {
				t.Fatal(err)
			}
			for _, mode := range []string{"single", "-b", "-detect"} {
				args := []string{"linter", "vali", "-obj"}
				target := file
				if mode != "single" {
					args = append(args, mode)
					target = root
				}
				args = append(args, target)
				code, out, stderr := runCLI(t, args...)
				expected := 0
				if tc.code != "" {
					expected = 1
				}
				if code != expected {
					t.Fatalf("%s exit=%d want=%d: %s %s", mode, code, expected, out, stderr)
				}
				codes := reportCELCodes(t, out)
				if tc.code == "" {
					if len(codes) > 0 {
						t.Fatal(codes)
					}
					continue
				}
				if codes[tc.code] != 1 {
					t.Fatalf("missing/duplicate CEL findings: %v %s", codes, out)
				}
				var report celReport
				_ = json.Unmarshal([]byte(out), &report)
				expectedLine, expectedColumn := 0, 0
				for i, line := range strings.Split(text, "\n") {
					if strings.Contains(line, "rule:") && strings.Contains(line, tc.expr) {
						expectedLine = i + 1
						expectedColumn = strings.Index(line, tc.token) + 1
					}
				}
				for _, r := range report.Runs[0].Results {
					if r.RuleID == tc.code {
						if len(r.Locations) != 1 || r.Locations[0].Physical.Region.Line != expectedLine || r.Locations[0].Physical.Region.Column != expectedColumn {
							t.Fatalf("bad diagnostic location: %+v expected %d:%d", r, expectedLine, expectedColumn)
						}
					}
				}
			}
		})
	}
}

func TestCLICRDSchemaDiscoveryScope(t *testing.T) {
	base := filepath.Join("..", "..", "..", "..", "tools", "lint", "targetschema", "testdata", "workspace")
	typeData, err := os.ReadFile(filepath.Join(base, "project", "componenttype.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	crdData, err := os.ReadFile(filepath.Join(base, "platform", "widget-crd.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "platform"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "componenttype.yaml"), typeData, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "platform", "widget-crd.yaml"), crdData, 0600); err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"-b", "-detect"} {
		code, out, stderr := runCLI(t, "linter", "vali", mode, "-obj", root)
		codes := reportCELCodes(t, out)
		expected := 0
		if mode == "-detect" {
			expected = 1
		}
		if code != expected || codes["cel-type-error"] != expected {
			t.Fatalf("%s scope error: exit=%d codes=%v %s %s", mode, code, codes, out, stderr)
		}
	}
	// A real configured OpenAPI catalog is used by both batch and recursive mode.
	catalog := `{"openapi":"3.0.3","components":{"schemas":{"Widget":{"type":"object","x-kubernetes-group-version-kind":[{"group":"example.com","version":"v1","kind":"Widget"}],"properties":{"spec":{"type":"object","properties":{"coutn":{"type":"integer"}}}}}}}}`
	if err := os.WriteFile(filepath.Join(root, "catalog.json"), []byte(catalog), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".openchoreo-lint.yaml"), []byte("version: 1\nkubernetesSchemaVersion: '1.36.4'\nschemaCatalogs: [catalog.json]\n"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"-b", "-detect"} {
		code, out, stderr := runCLI(t, "linter", "vali", mode, "-obj", root)
		if code != 0 || len(reportCELCodes(t, out)) != 0 {
			t.Fatalf("configured schema did not override CRD: %s %s", out, stderr)
		}
	}
}
