// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0
package targetschema

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"

	schemas "github.com/openchoreo/openchoreo/internal/schema"
	structural "k8s.io/apiextensions-apiserver/pkg/apiserver/schema"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

type Resolver interface {
	Resolve(schema.GroupVersionKind) (*structural.Structural, error)
}

//go:embed kubernetes-openapi.json.gz
var builtin []byte

//go:embed catalog.json
var provenance []byte
var once sync.Once
var defaults *Catalog
var defaultError error

func Default() Resolver {
	once.Do(func() {
		reader, e := gzip.NewReader(bytes.NewReader(builtin))
		if e != nil {
			defaultError = e
			return
		}
		defer reader.Close()
		raw, e := io.ReadAll(reader)
		if e != nil {
			defaultError = e
			return
		}
		var pin struct {
			SHA256 string `json:"sha256"`
		}
		if e = json.Unmarshal(provenance, &pin); e != nil {
			defaultError = e
			return
		}
		sum := sha256.Sum256(raw)
		if hex.EncodeToString(sum[:]) != pin.SHA256 {
			defaultError = fmt.Errorf("bundled Kubernetes catalog checksum mismatch")
			return
		}
		defaults, defaultError = NewCatalog(raw)
	})
	if defaults == nil {
		return failedResolver{defaultError}
	}
	return defaults
}

type failedResolver struct{ err error }

func (f failedResolver) Resolve(schema.GroupVersionKind) (*structural.Structural, error) {
	return nil, f.err
}

type result struct {
	s   *structural.Structural
	err error
}
type Catalog struct {
	mu          sync.Mutex
	definitions map[string]any
	targets     map[schema.GroupVersionKind][]string
	namedCache  map[string]result
}

func NewCatalog(raw []byte) (*Catalog, error) {
	// Adapt standard Swagger local references to the existing schema resolver's
	// $defs syntax. Conversion and reference expansion remain shared.
	raw = bytes.ReplaceAll(raw, []byte("#/definitions/"), []byte("#/$defs/"))
	raw = bytes.ReplaceAll(raw, []byte("#/components/schemas/"), []byte("#/$defs/"))
	var doc struct {
		Definitions map[string]any `json:"definitions"`
		Swagger     string         `json:"swagger"`
		OpenAPI     string         `json:"openapi"`
		Components  struct {
			Schemas map[string]any `json:"schemas"`
		} `json:"components"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, err
	}
	if doc.OpenAPI != "" {
		if !strings.HasPrefix(doc.OpenAPI, "3.0.") {
			return nil, fmt.Errorf("unsupported OpenAPI version %s", doc.OpenAPI)
		}
		doc.Definitions = doc.Components.Schemas
	}
	if len(doc.Definitions) == 0 {
		return nil, fmt.Errorf("catalog has no schema definitions")
	}
	if doc.OpenAPI == "" && doc.Swagger != "2.0" {
		return nil, fmt.Errorf("catalog must be OpenAPI 2.0 or 3.0")
	}
	// Swagger represents IntOrString as a string format. Preserve its real
	// Kubernetes union semantics when passing through structural conversion.
	var normalize func(any)
	normalize = func(v any) {
		switch n := v.(type) {
		case map[string]any:
			if n["format"] == "int-or-string" {
				n["x-kubernetes-int-or-string"] = true
			}
			for _, child := range n {
				normalize(child)
			}
		case []any:
			for _, child := range n {
				normalize(child)
			}
		}
	}
	normalize(doc.Definitions)
	c := &Catalog{definitions: doc.Definitions, targets: map[schema.GroupVersionKind][]string{}}
	for name, value := range doc.Definitions {
		node, ok := value.(map[string]any)
		if !ok {
			continue
		}
		entries, _ := node["x-kubernetes-group-version-kind"].([]any)
		for _, entry := range entries {
			m, ok := entry.(map[string]any)
			if !ok {
				continue
			}
			g, _ := m["group"].(string)
			v, _ := m["version"].(string)
			k, _ := m["kind"].(string)
			if v != "" && k != "" {
				key := schema.GroupVersionKind{Group: g, Version: v, Kind: k}
				c.targets[key] = append(c.targets[key], name)
			}
		}
	}
	if len(c.targets) == 0 {
		return nil, fmt.Errorf("catalog has no x-kubernetes-group-version-kind declarations")
	}
	return c, nil
}
func (c *Catalog) Resolve(key schema.GroupVersionKind) (*structural.Structural, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	names := c.targets[key]
	if len(names) == 0 {
		return nil, nil
	}
	if len(names) != 1 {
		return nil, fmt.Errorf("ambiguous schema for %s", key)
	}
	return c.resolveNamed(names[0])
}

func (c *Catalog) resolveNamed(name string) (*structural.Structural, error) {
	if c.namedCache == nil {
		c.namedCache = map[string]result{}
	}
	if cached, ok := c.namedCache[name]; ok {
		return cached.s, cached.err
	}
	source, ok := c.definitions[name].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("unknown schema definition %s", name)
	}
	root := map[string]any{}
	for k, v := range source {
		root[k] = v
	}
	root["$defs"] = c.definitions
	resolved, err := schemas.OpenAPIV3ToStructural(root)
	c.namedCache[name] = result{resolved, err}
	return resolved, err
}
func LiteralGVK(group, version, kind string) (schema.GroupVersionKind, bool) {
	key := schema.GroupVersionKind{Group: group, Version: version, Kind: kind}
	return key, version != "" && kind != "" && !strings.Contains(group+version+kind, "$") && !strings.Contains(group+version+kind, "*")
}

// ObjectMeta resolves the standard resource envelope from upstream schema data.
func (c *Catalog) ObjectMeta() (*structural.Structural, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.resolveNamed("io.k8s.apimachinery.pkg.apis.meta.v1.ObjectMeta")
}

// Version is the bundled Kubernetes release, independent of OpenChoreo templates.
func Version() string {
	var pin struct {
		Version string `json:"version"`
	}
	_ = json.Unmarshal(provenance, &pin)
	return strings.TrimPrefix(pin.Version, "v")
}
