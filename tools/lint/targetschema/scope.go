// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0
package targetschema

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	schemas "github.com/openchoreo/openchoreo/internal/schema"
	"github.com/openchoreo/openchoreo/tools/lint/parser"
	"gopkg.in/yaml.v3"
	structural "k8s.io/apiextensions-apiserver/pkg/apiserver/schema"
	"k8s.io/apiextensions-apiserver/pkg/apiserver/schema/cel/model"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

const ConfigName = ".openchoreo-lint.yaml"

type contextKey struct{}

func WithResolver(ctx context.Context, r Resolver) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, contextKey{}, r)
}
func FromContext(ctx context.Context) Resolver {
	if ctx != nil {
		if r, ok := ctx.Value(contextKey{}).(Resolver); ok && r != nil {
			return r
		}
	}
	return Default()
}

type Warning struct{ Path, Message string }

func Warnings(r Resolver) []Warning {
	if s, ok := r.(*Scope); ok && s != nil {
		return append([]Warning{}, s.warnings...)
	}
	return nil
}

// Scope owns immutable source bytes and priority tiers for one validation run.
// A new snapshot gets new resolution caches; no workspace resolver is global.
type Scope struct {
	dependencies []string
	sources      map[string][]byte
	explicit     []Resolver
	workspace    *workspaceCatalog
	warnings     []Warning
}

func absolute(path string) string {
	p, e := filepath.Abs(path)
	if e != nil {
		return filepath.Clean(path)
	}
	return filepath.Clean(p)
}
func (s *Scope) Content(path string) ([]byte, bool) {
	data, ok := s.sources[absolute(path)]
	return data, ok
}
func (s *Scope) Resolve(gvk schema.GroupVersionKind) (*structural.Structural, error) {
	if s == nil {
		return Default().Resolve(gvk)
	}
	if len(s.explicit) > 0 {
		if found, err := resolveTier(s.explicit, gvk); found != nil || err != nil {
			return found, err
		}
	}
	if s.workspace != nil {
		if found, err := s.workspace.Resolve(gvk); found != nil || err != nil {
			return found, err
		}
	}
	return Default().Resolve(gvk)
}
func resolveTier(resolvers []Resolver, key schema.GroupVersionKind) (*structural.Structural, error) {
	var selected *structural.Structural
	var fingerprint string
	for _, r := range resolvers {
		found, err := r.Resolve(key)
		if err != nil {
			return nil, err
		}
		if found == nil {
			continue
		}
		data := schemaFingerprint(found)
		if selected != nil && fingerprint != data {
			return nil, fmt.Errorf("conflicting schemas for %s", key)
		}
		selected = found
		fingerprint = data
	}
	return selected, nil
}

// ReadSources discovers schema dependencies without changing the CLI's existing
// OpenChoreo-file discovery, counts, flags or resource-name index.
func ReadSources(root string, recursive bool) (map[string][]byte, error) {
	root = absolute(root)
	out := map[string][]byte{}
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path != root && !recursive {
				return filepath.SkipDir
			}
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		if ext != ".yaml" && ext != ".yml" && ext != ".json" {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		out[absolute(path)] = data
		return nil
	})
	return out, err
}

// NewScope builds schema dependencies from the caller's saved/unsaved snapshot.
func NewScope(root string, sources map[string][]byte) *Scope {
	root = absolute(root)
	s := &Scope{sources: map[string][]byte{}, workspace: &workspaceCatalog{definitions: map[schema.GroupVersionKind][]map[string]any{}, cache: map[schema.GroupVersionKind]result{}}}
	paths := make([]string, 0, len(sources))
	for path, data := range sources {
		path = absolute(path)
		s.sources[path] = append([]byte(nil), data...)
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, path := range paths {
		rel, e := filepath.Rel(root, path)
		if e != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			continue
		}
		docs, err := parser.ParseYAML(s.sources[path])
		if err != nil {
			continue
		}
		for _, doc := range docs {
			if doc.APIVersion != "apiextensions.k8s.io/v1" || doc.Kind != "CustomResourceDefinition" {
				continue
			}
			s.dependencies = append(s.dependencies, path)
			var raw map[string]any
			if doc.Root.Decode(&raw) != nil {
				continue
			}
			spec, _ := raw["spec"].(map[string]any)
			names, _ := spec["names"].(map[string]any)
			group, _ := spec["group"].(string)
			kind, _ := names["kind"].(string)
			scope, _ := spec["scope"].(string)
			if group == "" || kind == "" || (scope != "Namespaced" && scope != "Cluster") {
				continue
			}
			versions, _ := spec["versions"].([]any)
			for _, version := range versions {
				v, _ := version.(map[string]any)
				served, _ := v["served"].(bool)
				name, _ := v["name"].(string)
				if !served || name == "" {
					continue
				}
				section, _ := v["schema"].(map[string]any)
				definition, _ := section["openAPIV3Schema"].(map[string]any)
				key := schema.GroupVersionKind{Group: group, Version: name, Kind: kind}
				// Retain missing schemas as unresolved candidates, preventing a stale or
				// lower-priority schema from silently replacing an invalid edited CRD.
				s.workspace.definitions[key] = append(s.workspace.definitions[key], definition)
			}
		}
	}
	root = absolute(root)
	for dir := root; ; dir = filepath.Dir(dir) {
		path := filepath.Join(dir, ConfigName)
		data, ok := s.Content(path)
		if !ok {
			var err error
			data, err = os.ReadFile(path)
			if err != nil {
				if !os.IsNotExist(err) {
					s.catalogWarning(path, err)
				}
				if filepath.Dir(dir) == dir {
					break
				}
				continue
			}
		}
		s.dependencies = append(s.dependencies, path)
		s.loadConfig(path, data)
		break
	}
	return s
}
func (s *Scope) catalogWarning(path string, err error) {
	s.warnings = append(s.warnings, Warning{Path: path, Message: err.Error()})
	s.explicit = append(s.explicit, failedResolver{fmt.Errorf("%s: %w", path, err)})
}
func (s *Scope) loadConfig(path string, data []byte) {
	var cfg struct {
		Version                 int      `yaml:"version"`
		KubernetesSchemaVersion string   `yaml:"kubernetesSchemaVersion"`
		SchemaCatalogs          []string `yaml:"schemaCatalogs"`
	}
	decoder := yaml.NewDecoder(strings.NewReader(string(data)))
	decoder.KnownFields(true)
	if err := decoder.Decode(&cfg); err != nil {
		s.catalogWarning(path, err)
		return
	}
	if cfg.Version != 1 {
		s.catalogWarning(path, fmt.Errorf("configuration version must be 1"))
		return
	}
	if cfg.KubernetesSchemaVersion != "" && cfg.KubernetesSchemaVersion != Version() {
		s.catalogWarning(path, fmt.Errorf("unsupported Kubernetes schema version %q; bundled version is %s", cfg.KubernetesSchemaVersion, Version()))
		return
	}
	for _, catalog := range cfg.SchemaCatalogs {
		if strings.Contains(catalog, "://") {
			s.catalogWarning(path, fmt.Errorf("catalog must be a local file: %s", catalog))
			continue
		}
		if !filepath.IsAbs(catalog) {
			catalog = filepath.Join(filepath.Dir(path), catalog)
		}
		s.dependencies = append(s.dependencies, absolute(catalog))
		raw, ok := s.Content(catalog)
		if !ok {
			var err error
			raw, err = os.ReadFile(catalog)
			if err != nil {
				s.catalogWarning(catalog, err)
				continue
			}
		}

		resolver, err := cachedCatalog(raw)
		if err != nil {
			s.catalogWarning(catalog, err)
			continue
		}
		s.explicit = append(s.explicit, resolver)
	}
}

type workspaceCatalog struct {
	mu          sync.Mutex
	definitions map[schema.GroupVersionKind][]map[string]any
	cache       map[schema.GroupVersionKind]result
}

func (c *workspaceCatalog) Resolve(key schema.GroupVersionKind) (*structural.Structural, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if cached, ok := c.cache[key]; ok {
		return cached.s, cached.err
	}
	definitions := c.definitions[key]
	if len(definitions) == 0 {
		return nil, nil
	}
	var fingerprint string
	var converted *structural.Structural
	var err error
	for _, definition := range definitions {
		if definition == nil {
			return nil, fmt.Errorf("CRD %s has no usable schema", key)
		}
		candidate, e := schemas.OpenAPIV3ToStructural(definition)
		if e != nil {
			c.cache[key] = result{nil, e}
			return nil, e
		}
		candidate = WithResourceEnvelope(candidate)
		next := schemaFingerprint(candidate)
		if converted != nil && fingerprint != next {
			return nil, fmt.Errorf("conflicting CRDs for %s", key)
		}
		converted = candidate
		fingerprint = next
	}

	c.cache[key] = result{converted, err}
	return converted, err
}

func (s *Scope) Dependencies() []string { return append([]string{}, s.dependencies...) }

// WithResourceEnvelope uses upstream metadata declarations and Kubernetes' CEL
// resource-envelope helper. It never changes the supplied schema.
func WithResourceEnvelope(s *structural.Structural) *structural.Structural {
	if s == nil || s.Type != "object" {
		return s
	}
	if _, ok := s.Properties["metadata"]; ok {
		if _, ok := s.Properties["kind"]; ok {
			if _, ok := s.Properties["apiVersion"]; ok {
				return s
			}
		}
	}
	copySchema := s.DeepCopy()
	envelope := model.WithTypeAndObjectMeta(&structural.Structural{Generic: structural.Generic{Type: "object"}})
	metadata := envelope.Properties["metadata"]
	metadata.XPreserveUnknownFields = true
	if builtin, ok := Default().(*Catalog); ok {
		if resolved, e := builtin.ObjectMeta(); e == nil {
			metadata = *resolved.DeepCopy()
		}
	}
	envelope.Properties["metadata"] = metadata
	if copySchema.Properties == nil {
		copySchema.Properties = map[string]structural.Structural{}
	}
	for name, field := range envelope.Properties {
		if _, exists := copySchema.Properties[name]; !exists {
			copySchema.Properties[name] = field
		}
	}
	return copySchema
}

// ReleaseSources drops raw input after an automatic-validation snapshot has
// built its schema dependencies. Resolver state remains immutable and usable.
func (s *Scope) ReleaseSources() { s.sources = nil }
