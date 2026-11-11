// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0
package targetschema

import (
	"crypto/sha256"
	"encoding/json"
	"gopkg.in/yaml.v3"
	structural "k8s.io/apiextensions-apiserver/pkg/apiserver/schema"
	"sync"
)

var catalogCache = struct {
	sync.Mutex
	entries map[[32]byte]*Catalog
}{entries: map[[32]byte]*Catalog{}}

func cachedCatalog(raw []byte) (*Catalog, error) {
	key := sha256.Sum256(raw)
	catalogCache.Lock()
	cached := catalogCache.entries[key]
	catalogCache.Unlock()
	if cached != nil {
		return cached, nil
	}
	jsonData := raw
	if !json.Valid(raw) {
		var value map[string]any
		if err := yaml.Unmarshal(raw, &value); err != nil {
			return nil, err
		}
		var err error
		jsonData, err = json.Marshal(value)
		if err != nil {
			return nil, err
		}
	}
	parsed, err := NewCatalog(jsonData)
	if err != nil {
		return nil, err
	}
	catalogCache.Lock()
	defer catalogCache.Unlock()
	if existing := catalogCache.entries[key]; existing != nil {
		return existing, nil
	}
	// Old snapshots retain their own immutable catalogs after eviction.
	if len(catalogCache.entries) >= 8 {
		catalogCache.entries = map[[32]byte]*Catalog{}
	}
	catalogCache.entries[key] = parsed
	return parsed, nil
}
func schemaFingerprint(s *structural.Structural) string {
	copySchema := s.DeepCopy()
	var clean func(*structural.Structural)
	clean = func(n *structural.Structural) {
		n.Description = ""
		n.Title = ""
		n.ExternalDocs = nil
		n.Example = structural.JSON{}
		if n.Items != nil {
			clean(n.Items)
		}
		for name, value := range n.Properties {
			clean(&value)
			n.Properties[name] = value
		}
		if n.AdditionalProperties != nil && n.AdditionalProperties.Structural != nil {
			clean(n.AdditionalProperties.Structural)
		}
	}
	clean(copySchema)
	encoded, _ := json.Marshal(copySchema)
	return string(encoded)
}
