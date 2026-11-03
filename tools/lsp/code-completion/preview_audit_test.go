// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0
package codecompletion

import (
	"encoding/json"
	"github.com/openchoreo/openchoreo/tools/lint/ruleengine/schema"
	"github.com/openchoreo/openchoreo/tools/templates"
	"gopkg.in/yaml.v3"
	"io/fs"
	"regexp"
	"strings"
	"testing"

	"github.com/openchoreo/openchoreo/tools/lint/parser"
	"github.com/openchoreo/openchoreo/tools/lint/ruleengine"
	"github.com/openchoreo/openchoreo/tools/lint/ruleengine/template"
)

func acceptAuditPreview(text, preview string, line int) string {
	concrete := regexp.MustCompile(`\$\{[0-9]+:([^}]*)\}`).ReplaceAllString(preview, "$1")
	concrete = strings.NewReplacer(`\\`, `\`, `\$`, `$`, `\}`, `}`).Replace(concrete)
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	lines[line] += strings.ReplaceAll(concrete, "\r\n", "\n")
	return strings.Join(lines, "\n")
}

func TestAuditEveryKindHeader(t *testing.T) {
	original := template.CurrentVersion()
	t.Cleanup(func() { _ = template.SetVersion(original) })
	for _, version := range template.AvailableVersions() {
		if err := template.SetVersion(version); err != nil {
			t.Fatal(err)
		}
		for _, kind := range template.AllKinds() {
			t.Run(version+"/"+kind, func(t *testing.T) {
				text := "apiVersion: openchoreo.dev/v1alpha1\nkind: " + kind
				preview := RequiredPreview(text, 1, len("kind: "+kind), "  ")
				if preview == "" {
					t.Fatal("header preview unexpectedly absent")
				}
				accepted := acceptAuditPreview(text, preview, 1)
				docs, err := parser.ParseYAML([]byte(accepted))
				if err != nil {
					t.Fatal(err)
				}
				engine := ruleengine.NewEngine()
				engine.AddKindRules(kind, template.RulesForKind(kind))
				for _, finding := range engine.Evaluate(docs[0]) {
					if finding.Severity == ruleengine.SeverityError {
						t.Errorf("%s: %s", finding.Code, finding.Message)
					}
				}
				if t.Failed() {
					t.Logf("accepted preview:\n%s", accepted)
				}
				if strings.Contains(accepted, "\nstatus:") {
					t.Error("status must not be generated")
				}
			})
		}
	}
}

// Legacy template directories may remain embedded without being selectable in
// the manifest. Audit their schema data too, without changing version support.
func TestAuditEveryEmbeddedTemplate(t *testing.T) {
	files, err := fs.Glob(templates.FS, "template-v*/*.json")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatal("no templates discovered")
	}
	for _, file := range files {
		t.Run(file, func(t *testing.T) {
			data, err := templates.FS.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			root, err := schema.Parse(data)
			if err != nil {
				t.Fatal(err)
			}
			if root.Properties["kind"] == nil {
				t.Fatal("template has no kind")
			}
			kind := root.Properties["kind"].Default
			if kind == "" {
				t.Fatal("template has no kind default")
			}
			text := "apiVersion: openchoreo.dev/v1alpha1\nkind: " + kind
			lookup := func(k string) *schema.FieldSchema {
				if k == kind {
					return root
				}
				return nil
			}
			before, err := json.Marshal(root)
			if err != nil {
				t.Fatal(err)
			}
			preview := requiredPreviewWithSchema(text, 1, len("kind: "+kind), "  ", lookup)
			if preview == "" {
				t.Fatal("header preview unexpectedly absent")
			}
			accepted := acceptAuditPreview(text, preview, 1)
			docs, err := parser.ParseYAML([]byte(accepted))
			if err != nil {
				t.Fatal(err)
			}
			diags := schema.Validate(docs[0].Root, root, kind)
			diags = append(diags, ruleengine.NewEngine().Evaluate(docs[0])...)
			for _, finding := range diags {
				if finding.Severity == ruleengine.SeverityError {
					t.Errorf("%s: %s", finding.Code, finding.Message)
				}
			}
			if t.Failed() {
				t.Logf("accepted:\n%s", accepted)
			}
			after, err := json.Marshal(root)
			if err != nil {
				t.Fatal(err)
			}
			if string(before) != string(after) {
				t.Fatal("preview mutated template schema")
			}
			complete := accepted + "\n"
			completeLine := len(strings.Split(complete, "\n")) - 1
			if again := requiredPreviewWithSchema(complete, completeLine, 0, "  ", lookup); again != "" {
				t.Fatalf("completed root suggested again: %q", again)
			}

		})
	}
	t.Logf("audited %d embedded resource templates", len(files))
}

func TestAuditEveryObjectContext(t *testing.T) {
	files, err := fs.Glob(templates.FS, "template-v*/*.json")
	if err != nil {
		t.Fatal(err)
	}
	checked, emitted, abstained := 0, 0, 0
	for _, file := range files {
		t.Run(file, func(t *testing.T) {
			data, err := templates.FS.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			root, err := schema.Parse(data)
			if err != nil {
				t.Fatal(err)
			}
			kind := root.Properties["kind"].Default
			lookup := func(k string) *schema.FieldSchema {
				if k == kind {
					return root
				}
				return nil
			}
			var visit func(*schema.FieldSchema, []string)
			visit = func(node *schema.FieldSchema, path []string) {
				if node.Type != "object" {
					return
				}
				if len(path) > 0 {
					for _, indent := range []string{"  ", "    "} {
						for _, eol := range []string{"\n", "\r\n"} {
							text := "apiVersion: openchoreo.dev/v1alpha1\nkind: " + kind + "\n"
							for depth, key := range path {
								text += strings.Repeat(indent, depth) + key + ":\n"
							}
							prefix := strings.Repeat(indent, len(path))
							text += prefix
							text = strings.ReplaceAll(text, "\n", eol)
							line := len(strings.Split(text, "\n")) - 1
							preview := requiredPreviewWithSchema(text, line, len(prefix), indent, lookup)
							checked++
							if preview == "" {
								abstained++
								if !node.Preserve && hasPreviewRequirements(node) {
									t.Errorf("missing preview for %s (indent %d CRLF %v)", strings.Join(path, "."), len(indent), eol == "\r\n")
								}
								continue
							}
							emitted++
							accepted := acceptAuditPreview(text, preview, line)
							docs, err := parser.ParseYAML([]byte(accepted))
							if err != nil {
								t.Error(err)
								continue
							}
							target := docs[0].Root
							if target.Kind == yaml.DocumentNode {
								target = target.Content[0]
							}
							for _, key := range path {
								target = mappingValue(target, key)
							}
							for _, finding := range schema.Validate(target, node, strings.Join(path, ".")) {
								if finding.Severity == ruleengine.SeverityError {
									t.Errorf("%s: %s", strings.Join(path, "."), finding.Message)
								}
							}
							// Accepted content must not be suggested a second time in this mapping.
							next := accepted + "\n" + prefix
							nextLine := len(strings.Split(next, "\n")) - 1
							if again := requiredPreviewWithSchema(next, nextLine, len(prefix), indent, lookup); again != "" {
								t.Errorf("repeated suggestion in %s: %q", strings.Join(path, "."), again)
							}
						}
					}
				}
				for _, name := range sortedFieldNames(node) {
					if len(path) == 0 && (name == "status" || name == "apiVersion" || name == "kind") {
						continue
					}
					child := node.Properties[name]
					visit(child, append(append([]string{}, path...), name))
				}
			}
			visit(root, nil)
		})
	}
	t.Logf("object contexts: checked=%d emitted=%d safe abstentions=%d", checked, emitted, abstained)
}

func hasPreviewRequirements(node *schema.FieldSchema) bool {
	if node == nil || node.Preserve {
		return false
	}
	for name, child := range node.Properties {
		if child.Required || referenceKindHint(node, name, child) {
			return true
		}
		if child.Type == "object" && hasPreviewRequirements(child) {
			return true
		}
	}
	return false
}
