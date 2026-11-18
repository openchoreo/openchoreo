// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0
package lsp

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/openchoreo/openchoreo/tools/lint/indexer"
	"github.com/openchoreo/openchoreo/tools/lint/parser"
	"github.com/tliron/glsp"
	"gopkg.in/yaml.v3"
)

type clusterSchema struct {
	Parameters         map[string]any `json:"parameters"`
	EnvironmentConfigs map[string]any `json:"environmentConfigs"`
}
type clusterFeed struct {
	mu      sync.RWMutex
	names   map[string][]string
	schemas map[string]map[string]clusterSchema
}
type clusterHandler struct {
	glsp.Handler
	feed   *clusterFeed
	report func(error)
}

func (h *clusterHandler) Handle(c *glsp.Context) (any, bool, bool, error) {
	if c.Method != "openchoreo/updateResources" && c.Method != "openchoreo/updateResourceSchemas" {
		return h.Handler.Handle(c)
	}
	if err := h.feed.update(c.Method, c.Params); err != nil {
		h.report(err)
	}
	return nil, true, true, nil
}
func (f *clusterFeed) update(method string, data []byte) error {
	if len(strings.TrimSpace(string(data))) == 0 || strings.TrimSpace(string(data))[0] != '{' {
		return fmt.Errorf("invalid cluster feed object")
	}
	if method == "openchoreo/updateResources" {
		var names map[string][]string
		if err := json.Unmarshal(data, &names); err != nil {
			return err
		}
		for k, v := range names {
			if k == "" || v == nil {
				return fmt.Errorf("invalid resource names")
			}
			for _, n := range v {
				if n == "" {
					return fmt.Errorf("empty resource name")
				}
			}
		}
		f.mu.Lock()
		f.names = names
		f.mu.Unlock()
	} else {
		var schemas map[string]map[string]clusterSchema
		if err := json.Unmarshal(data, &schemas); err != nil {
			return err
		}
		for k, byName := range schemas {
			if k == "" || byName == nil {
				return fmt.Errorf("invalid schema group")
			}
			for n, s := range byName {
				if n == "" {
					return fmt.Errorf("empty schema name")
				}
				for _, v := range []map[string]any{s.Parameters, s.EnvironmentConfigs} {
					if v != nil {
						if err := validateFeedSchema(v); err != nil {
							return err
						}
					}
				}
			}
		}
		f.mu.Lock()
		f.schemas = schemas
		f.mu.Unlock()
	}
	return nil
}

// completionIndex is a request-scoped overlay. It never enters the filesystem index.
type completionIndex struct {
	names  map[string][]string
	docs   []indexer.SnapshotDocument
	remote map[string][]string
}

func (i *completionIndex) ClusterNames(kind string) []string          { return i.remote[kind] }
func (i *completionIndex) NamesByKind() map[string][]string           { return i.names }
func (i *completionIndex) Snapshot(string) []indexer.SnapshotDocument { return i.docs }
func (s *Server) clusterCompletionIndex(uri, text string) *completionIndex {
	out := &completionIndex{names: map[string][]string{}, docs: s.indexer.Snapshot(uri)}
	for k, v := range s.indexer.NamesByKind() {
		out.names[k] = append([]string(nil), v...)
	}
	api, ns := "openchoreo.dev/v1alpha1", "default"
	if docs, err := parser.ParseYAML([]byte(text)); err == nil && len(docs) > 0 {
		api = docs[0].APIVersion
		ns = indexer.NamespaceOrDefault(docs[0].Metadata.Namespace)
	}
	s.cluster.mu.RLock()
	defer s.cluster.mu.RUnlock()
	out.remote = s.cluster.names
	for k, names := range s.cluster.names {
		if strings.Contains(k, ":") {
			continue
		}
		seen := map[string]bool{}
		for _, n := range out.names[k] {
			seen[n] = true
		}
		for _, n := range names {
			if !seen[n] {
				out.names[k] = append(out.names[k], n)
				seen[n] = true
			}
		}
		sort.Strings(out.names[k])
	}
	for kind, byName := range s.cluster.schemas {
		for name, entry := range byName {
			exists := false
			for _, d := range out.docs {
				if d.Document.Kind == kind && d.Document.Metadata.Name == indexer.ReferenceName(kind, name) && d.Document.APIVersion == api && (strings.HasPrefix(kind, "Cluster") || indexer.NamespaceOrDefault(d.Document.Metadata.Namespace) == ns) {
					exists = true
					break
				}
			}
			if exists {
				continue
			}
			spec := map[string]any{}
			if entry.Parameters != nil {
				spec["parameters"] = map[string]any{"openAPIV3Schema": entry.Parameters}
			}
			if entry.EnvironmentConfigs != nil {
				spec["environmentConfigs"] = map[string]any{"openAPIV3Schema": entry.EnvironmentConfigs}
			}
			resourceName := indexer.ReferenceName(kind, name)
			if resourceName != name {
				spec["workloadType"] = strings.Split(name, "/")[0]
			}
			var node yaml.Node
			_ = node.Encode(spec)
			out.docs = append(out.docs, indexer.SnapshotDocument{Document: &parser.DocumentNode{APIVersion: api, Kind: kind, Metadata: parser.MetadataNode{Name: resourceName, Namespace: ns}, Spec: &node}})
		}
	}
	return out
}

// Validate schema containers before publishing a new immutable snapshot.
func validateFeedSchema(s map[string]any) error {
	if typ, ok := s["type"]; ok {
		if _, ok := typ.(string); !ok {
			return fmt.Errorf("invalid schema type")
		}
	}
	if props, ok := s["properties"]; ok {
		m, ok := props.(map[string]any)
		if !ok {
			return fmt.Errorf("invalid schema properties")
		}
		for _, value := range m {
			child, ok := value.(map[string]any)
			if !ok {
				return fmt.Errorf("invalid property schema")
			}
			if err := validateFeedSchema(child); err != nil {
				return err
			}
		}
	}
	if items, ok := s["items"]; ok {
		child, ok := items.(map[string]any)
		if !ok {
			return fmt.Errorf("invalid array item schema")
		}
		if err := validateFeedSchema(child); err != nil {
			return err
		}
	}
	if required, ok := s["required"]; ok {
		entries, ok := required.([]any)
		if !ok {
			return fmt.Errorf("invalid required list")
		}
		for _, e := range entries {
			if _, ok := e.(string); !ok {
				return fmt.Errorf("invalid required field")
			}
		}
	}
	return nil
}
