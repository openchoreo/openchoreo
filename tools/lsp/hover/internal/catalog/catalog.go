// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package catalog

import (
	"encoding/json"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Generate retains descriptions by exact API version, kind and schema path.
func Generate(dir string) ([]byte, error) {
	out := map[string]map[string]string{}
	files, err := filepath.Glob(filepath.Join(dir, "*.yaml"))
	if err != nil {
		return nil, err
	}
	for _, file := range files {
		raw, err := os.ReadFile(file)
		if err != nil {
			return nil, err
		}
		var crd struct {
			Spec struct {
				Group string `yaml:"group"`
				Names struct {
					Kind string `yaml:"kind"`
				} `yaml:"names"`
				Versions []struct {
					Name   string `yaml:"name"`
					Schema struct {
						Open map[string]any `yaml:"openAPIV3Schema"`
					} `yaml:"schema"`
				} `yaml:"versions"`
			} `yaml:"spec"`
		}
		if err := yaml.Unmarshal(raw, &crd); err != nil {
			return nil, err
		}
		for _, version := range crd.Spec.Versions {
			fields := map[string]string{}
			var walk func(map[string]any, string)
			walk = func(node map[string]any, path string) {
				if desc, ok := node["description"].(string); ok && path != "" {
					fields[path] = desc
				}
				if props, ok := node["properties"].(map[string]any); ok {
					for key, value := range props {
						if child, ok := value.(map[string]any); ok {
							p := key
							if path != "" {
								p = path + "." + key
							}
							walk(child, p)
						}
					}
				}
				if child, ok := node["items"].(map[string]any); ok {
					walk(child, path+"[]")
				}
				if child, ok := node["additionalProperties"].(map[string]any); ok {
					walk(child, path+".*")
				}
			}
			walk(version.Schema.Open, "")
			out[crd.Spec.Group+"/"+version.Name+"/"+crd.Spec.Names.Kind] = fields
		}
	}
	return json.MarshalIndent(out, "", "  ")
}
