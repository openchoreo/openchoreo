// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"

	yamlv3 "gopkg.in/yaml.v3"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/config"
)

// chartFilePath is the checked-in artifact this command generates.
const chartFilePath = "../../install/helm/openchoreo-control-plane/files/resource-tree-builtin-rules.yaml"

func mustRender(t *testing.T) []byte {
	t.Helper()

	rendered, err := renderRules()
	if err != nil {
		t.Fatalf("renderRules: %v", err)
	}

	return rendered
}

// TestGeneratedFileIsUpToDate is the drift check that runs under a bare
// go test, with no make involved: it fires whenever someone edits
// ResourceTreeDefaults() or the banner without regenerating, or hand-edits the
// chart file. The whole file is compared, banner included, so header drift is
// caught as well as rule drift.
func TestGeneratedFileIsUpToDate(t *testing.T) {
	want, err := os.ReadFile(chartFilePath)
	if err != nil {
		t.Fatalf("read %s: %v", chartFilePath, err)
	}

	if got := mustRender(t); !bytes.Equal(got, want) {
		t.Errorf("%s is out of date; regenerate it with `make helm-generate.openchoreo-control-plane`\ngot:\n%s\nwant:\n%s",
			chartFilePath, got, want)
	}
}

// TestRenderRulesIsStableAcrossCalls catches map iteration order leaking into
// the output, which a single comparison against the checked-in file can miss.
func TestRenderRulesIsStableAcrossCalls(t *testing.T) {
	first := mustRender(t)

	for i := range 20 {
		if got := mustRender(t); !bytes.Equal(got, first) {
			t.Fatalf("call %d rendered differently:\ngot:\n%s\nwant:\n%s", i, got, first)
		}
	}
}

// TestWriteFileAtomicLeavesNoTempFiles pins the atomic write: the destination
// ends up with the rendered bytes and the directory holds nothing else, so a
// generation run never litters the chart's files directory.
func TestWriteFileAtomicLeavesNoTempFiles(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/rules.yaml"
	rendered := mustRender(t)

	if err := writeFileAtomic(path, rendered); err != nil {
		t.Fatalf("writeFileAtomic: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if !bytes.Equal(got, rendered) {
		t.Errorf("written file differs from the rendered bytes\ngot:\n%s\nwant:\n%s", got, rendered)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir %s: %v", dir, err)
	}
	if len(entries) != 1 {
		names := make([]string, 0, len(entries))
		for _, entry := range entries {
			names = append(names, entry.Name())
		}
		t.Errorf("expected only the destination file, got %v", names)
	}
}

// TestKeyOrderCoversEveryRuleField pins keyOrder to the rule structs: a field
// added to ResourceTreeRule and friends without a place in keyOrder would
// silently land at the end of its mapping, alphabetically, rather than
// wherever it belongs. Failing here is the prompt to place it.
func TestKeyOrderCoversEveryRuleField(t *testing.T) {
	for _, name := range jsonFieldNames(reflect.TypeOf(config.ResourceTreeRule{}), map[reflect.Type]bool{}) {
		if !slices.Contains(keyOrder, name) {
			t.Errorf("json field %q is not in keyOrder; add it where it belongs in the emitted mapping", name)
		}
	}
}

// jsonFieldNames collects the json tag names of a struct and of every struct
// reachable from it, following pointers, slices and map values. seen guards
// the recursion against the self reference in ChildRule.Children.
func jsonFieldNames(typ reflect.Type, seen map[reflect.Type]bool) []string {
	for typ.Kind() == reflect.Pointer || typ.Kind() == reflect.Slice || typ.Kind() == reflect.Map {
		typ = typ.Elem()
	}
	if typ.Kind() != reflect.Struct || seen[typ] {
		return nil
	}
	seen[typ] = true

	var names []string
	for i := range typ.NumField() {
		field := typ.Field(i)

		name, _, _ := strings.Cut(field.Tag.Get("json"), ",")
		if name == "" || name == "-" {
			t := field.Name
			name = strings.ToLower(t[:1]) + t[1:]
		}

		names = append(names, name)
		names = append(names, jsonFieldNames(field.Type, seen)...)
	}

	return names
}

// TestOrderKeysLeavesLabelKeysAlone covers the shape the built-in rules do not
// have: a labelSelector child. Schema fields are reordered around it, while
// the keys under match_labels are label names chosen by an operator and stay
// as they were, even when one is spelled like a schema field.
func TestOrderKeysLeavesLabelKeysAlone(t *testing.T) {
	const in = `
children:
  - matcher: labelSelector
    label_selector:
      namespaces:
        - envoy-gateway-system
      match_labels:
        kind: zebra
        group: apple
    kind:
      resource: deployments
      kind: Deployment
      version: v1
      group: apps
root:
  resource: gateways
  kind: Gateway
  version: v1
  group: gateway.networking.k8s.io
`

	const want = `root:
  group: gateway.networking.k8s.io
  version: v1
  kind: Gateway
  resource: gateways
children:
  - kind:
      group: apps
      version: v1
      kind: Deployment
      resource: deployments
    matcher: labelSelector
    label_selector:
      match_labels:
        kind: zebra
        group: apple
      namespaces:
        - envoy-gateway-system
`

	var doc yamlv3.Node
	if err := yamlv3.Unmarshal([]byte(in), &doc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	orderKeys(doc.Content[0])

	var buf bytes.Buffer
	enc := yamlv3.NewEncoder(&buf)
	enc.SetIndent(yamlIndent)

	if err := enc.Encode(doc.Content[0]); err != nil {
		t.Fatalf("encode: %v", err)
	}
	if err := enc.Close(); err != nil {
		t.Fatalf("close encoder: %v", err)
	}

	if got := buf.String(); got != want {
		t.Errorf("orderKeys produced:\n%s\nwant:\n%s", got, want)
	}
}
