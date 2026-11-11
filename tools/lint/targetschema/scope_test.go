// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0
package targetschema

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"k8s.io/apimachinery/pkg/runtime/schema"
)

const sampleCRD = `apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata: {name: widgets.example.com}
spec:
  group: example.com
  names: {kind: Widget, plural: widgets}
  scope: Namespaced
  versions:
    - name: v1
      served: true
      storage: true
      schema:
        openAPIV3Schema:
          type: object
          properties:
            spec:
              type: object
              properties:
                count: {type: integer}
    - name: v2
      served: true
      storage: false
      schema:
        openAPIV3Schema:
          type: object
          properties:
            spec:
              type: object
              properties:
                title: {type: string}
    - name: v3
      served: false
      storage: false
      schema:
        openAPIV3Schema: {type: object}
`

func TestBuiltinCatalogProvenanceAndTypes(t *testing.T) {
	reader, err := gzip.NewReader(bytes.NewReader(builtin))
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	raw, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(raw)
	var manifest struct {
		SHA256 string `json:"sha256"`
	}
	if err := json.Unmarshal(provenance, &manifest); err != nil {
		t.Fatal(err)
	}
	if hex.EncodeToString(sum[:]) != manifest.SHA256 {
		t.Fatal("catalog checksum differs from provenance")
	}

	for _, key := range []schema.GroupVersionKind{{Group: "apps", Version: "v1", Kind: "Deployment"}, {Version: "v1", Kind: "Pod"}, {Version: "v1", Kind: "Service"}, {Group: "batch", Version: "v1", Kind: "Job"}, {Group: "networking.k8s.io", Version: "v1", Kind: "Ingress"}} {
		found, err := Default().Resolve(key)
		if err != nil || found == nil || found.Type != "object" {
			t.Fatalf("%s: %v", key, err)
		}
	}
	for _, key := range []schema.GroupVersionKind{{Group: "apps", Version: "v999", Kind: "Deployment"}, {Group: "custom", Version: "v1", Kind: "Deployment"}, {Version: "v1", Kind: "Unknown"}} {
		found, err := Default().Resolve(key)
		if err != nil || found != nil {
			t.Fatalf("unknown GVK resolved: %s %v", key, err)
		}
	}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = Default().Resolve(schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"})
		}()
	}
	wg.Wait()
}

func TestWorkspaceVersionsScopesAndAmbiguity(t *testing.T) {
	root := t.TempDir()
	key := schema.GroupVersionKind{Group: "example.com", Version: "v1", Kind: "Widget"}
	for _, scope := range []string{"Namespaced", "Cluster"} {
		sources := map[string][]byte{filepath.Join(root, "platform", "widget.yaml"): []byte(strings.Replace(sampleCRD, "scope: Namespaced", "scope: "+scope, 1))}
		resolver := NewScope(root, sources)
		found, err := resolver.Resolve(key)
		if err != nil || found == nil {
			t.Fatal(scope, err)
		}
		if found.Properties["spec"].Properties["count"].Type != "integer" || found.Properties["metadata"].Properties["namespace"].Type != "string" {
			t.Fatalf("missing CRD/envelope type: %+v", found)
		}
		next := key
		next.Version = "v2"
		v2, err := resolver.Resolve(next)
		if err != nil || v2 == nil || v2.Properties["spec"].Properties["title"].Type != "string" {
			t.Fatal(err)
		}
		next.Version = "v3"
		v3, err := resolver.Resolve(next)
		if err != nil || v3 != nil {
			t.Fatal("unserved version selected")
		}
		sources[filepath.Join(root, "duplicate.yaml")] = sources[filepath.Join(root, "platform", "widget.yaml")]
		if _, err := NewScope(root, sources).Resolve(key); err != nil {
			t.Fatal("identical definitions treated as ambiguous", err)
		}
		sources[filepath.Join(root, "duplicate.yaml")] = []byte(strings.Replace(string(sources[filepath.Join(root, "duplicate.yaml")]), "count: {type: integer}", "count: {type: string}", 1))
		if found, err := NewScope(root, sources).Resolve(key); err == nil || found != nil {
			t.Fatal("conflicting definitions selected arbitrarily")
		}
	}
	outside := filepath.Join(t.TempDir(), "unrelated.yaml")
	if found, err := NewScope(root, map[string][]byte{outside: []byte(sampleCRD)}).Resolve(key); err != nil || found != nil {
		t.Fatal("schema escaped workspace boundary")
	}
}

func openAPICatalog(v3 bool, fieldType string) []byte {
	node := map[string]any{"type": "object", "properties": map[string]any{"spec": map[string]any{"$ref": "#/definitions/Spec"}}, "x-kubernetes-group-version-kind": []any{map[string]any{"group": "example.com", "version": "v1", "kind": "Widget"}}}
	definitions := map[string]any{"Widget": node, "Spec": map[string]any{"type": "object", "properties": map[string]any{"count": map[string]any{"type": fieldType}}}}
	doc := map[string]any{"swagger": "2.0", "definitions": definitions}
	if v3 {
		node["properties"].(map[string]any)["spec"] = map[string]any{"$ref": "#/components/schemas/Spec"}
		doc = map[string]any{"openapi": "3.0.3", "components": map[string]any{"schemas": definitions}}
	}
	raw, _ := json.Marshal(doc)
	return raw
}
func TestConfiguredCatalogsAndFallback(t *testing.T) {
	root := t.TempDir()
	key := schema.GroupVersionKind{Group: "example.com", Version: "v1", Kind: "Widget"}
	for _, v3 := range []bool{false, true} {
		sources := map[string][]byte{filepath.Join(root, ConfigName): []byte("version: 1\nkubernetesSchemaVersion: '1.36.3'\nschemaCatalogs: [schemas/one.json]\n"), filepath.Join(root, "schemas", "one.json"): openAPICatalog(v3, "string"), filepath.Join(root, "widget.yaml"): []byte(sampleCRD)}
		resolver := NewScope(root, sources)
		found, err := resolver.Resolve(key)
		if err != nil || found == nil || found.Properties["spec"].Properties["count"].Type != "string" {
			t.Fatal("configured catalog must take precedence", err, resolver.warnings)
		}
		if len(resolver.warnings) != 0 {
			t.Fatal(resolver.warnings)
		}
		sources[filepath.Join(root, ConfigName)] = []byte("version: 1\nschemaCatalogs: [schemas/one.json, schemas/two.json]\n")
		sources[filepath.Join(root, "schemas", "two.json")] = openAPICatalog(v3, "integer")
		if found, err := NewScope(root, sources).Resolve(key); err == nil || found != nil {
			t.Fatal("conflicting configured schemas must not fall through to the CRD")
		}
	}
	for _, config := range []string{"version: [", "version: 7", "version: 1\nkubernetesSchemaVersion: '0.0.0'", "version: 1\nschemaCatalogs: [https://example.invalid/schema.json]", "version: 1\nschemaCatalogs: [missing.json]"} {
		s := NewScope(root, map[string][]byte{filepath.Join(root, ConfigName): []byte(config)})
		if len(s.warnings) == 0 {
			t.Fatal("invalid explicit config must produce a warning", config)
		}
		if found, err := s.Resolve(key); err == nil || found != nil {
			t.Fatal("invalid configuration must fall back to dynamic checking")
		}
	}
}

// Audit every declared built-in GVK, rather than extrapolating from Deployment.
func TestBuiltinCatalogConversionCoverage(t *testing.T) {
	catalog, ok := Default().(*Catalog)
	if !ok {
		t.Fatal("built-in catalog unavailable")
	}
	converted := 0
	for gvk := range catalog.targets {
		resolved, err := catalog.Resolve(gvk)
		if gvk.Group == "apiextensions.k8s.io" && (gvk.Kind == "CustomResourceDefinition" || gvk.Kind == "CustomResourceDefinitionList") {
			if err == nil || !strings.Contains(err.Error(), "circular $ref") {
				t.Errorf("expected documented recursive-schema fallback for %s, got %v", gvk, err)
			}
			continue
		}
		if err != nil || resolved == nil {
			t.Errorf("%s: unexpected schema conversion failure: %v", gvk, err)
			continue
		}
		converted++
	}
	t.Logf("converted %d of %d declared built-in GVKs", converted, len(catalog.targets))
}
