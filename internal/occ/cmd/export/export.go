// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

// Package export implements the local engine behind `occ export`.
package export

import (
	"context"
	"fmt"
	"path"
	"sort"
	"strings"

	"github.com/openchoreo/openchoreo/internal/occ/cmd/pagination"
	"github.com/openchoreo/openchoreo/internal/occ/resources/client"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
)

const componentAPIVersion = "openchoreo.dev/v1alpha1"

// Options configures one export run.
type Options struct {
	OutputDir string
}

// Report summarizes the files written by one export run.
type Report struct {
	Created     int
	Updated     int
	Unchanged   int
	Exported    int
	Withheld    []Withheld
	Unsupported []string
}

// Withheld describes a component omitted because it appears to contain
// plaintext credentials.
type Withheld struct {
	Namespace string
	Project   string
	Name      string
	Reasons   []string
}

// Run exports every component returned by the Control Plane API. This initial
// slice deliberately does not export its namespace, project, or workload
// dependencies; those categories are called out in the report.
func runEngine(ctx context.Context, c client.Interface, opts Options) (*Report, error) {
	if strings.TrimSpace(opts.OutputDir) == "" {
		return nil, fmt.Errorf("output directory is required (use -o <path>)")
	}

	namespaces, err := pagination.FetchAll(func(limit int, cursor string) ([]gen.Namespace, string, error) {
		params := &gen.ListNamespacesParams{Limit: &limit}
		if cursor != "" {
			params.Cursor = &cursor
		}
		result, err := c.ListNamespaces(ctx, params)
		if err != nil {
			return nil, "", err
		}
		return result.Items, nextCursor(result.Pagination), nil
	})
	if err != nil {
		return nil, fmt.Errorf("list namespaces (nothing was written): %w", err)
	}

	var resources []resource
	for _, namespace := range namespaces {
		namespaceName := namespace.Metadata.Name
		if err := validName("namespace", namespaceName); err != nil {
			return nil, err
		}

		components, err := pagination.FetchAll(func(limit int, cursor string) ([]gen.Component, string, error) {
			params := &gen.ListComponentsParams{Limit: &limit}
			if cursor != "" {
				params.Cursor = &cursor
			}
			result, err := c.ListComponents(ctx, namespaceName, "", params)
			if err != nil {
				return nil, "", err
			}
			return result.Items, nextCursor(result.Pagination), nil
		})
		if err != nil {
			return nil, fmt.Errorf("namespace %q: list components (nothing was written): %w", namespaceName, err)
		}

		for _, component := range components {
			componentName := component.Metadata.Name
			if err := validName("component", componentName); err != nil {
				return nil, err
			}
			if component.Spec == nil || component.Spec.Owner.ProjectName == "" {
				return nil, fmt.Errorf("component %q in namespace %q has no spec.owner.projectName", componentName, namespaceName)
			}
			projectName := component.Spec.Owner.ProjectName
			if err := validName("project", projectName); err != nil {
				return nil, err
			}

			object, err := componentObject(component, namespaceName)
			if err != nil {
				return nil, fmt.Errorf("component %q in namespace %q: %w", componentName, namespaceName, err)
			}
			resources = append(resources, resource{
				name:      componentName,
				namespace: namespaceName,
				project:   projectName,
				path: path.Join("namespaces", namespaceName, "projects", projectName,
					"components", componentName, "component.yaml"),
				object: object,
			})
		}
	}

	sort.Slice(resources, func(i, j int) bool { return resources[i].path < resources[j].path })
	report := &Report{
		Unsupported: []string{
			"namespace manifests",
			"project manifests",
			"workloads and component releases/bindings",
			"platform and cluster-scoped resources",
			"Kustomize/Flux configuration files",
		},
	}
	var safe []resource
	var rendered [][]byte
	for _, item := range resources {
		if reasons := findPlaintextSecrets(item.object); len(reasons) > 0 {
			report.Withheld = append(report.Withheld, Withheld{
				Namespace: item.namespace,
				Project:   item.project,
				Name:      item.name,
				Reasons:   reasons,
			})
			continue
		}
		data, err := serialize(item.object)
		if err != nil {
			return nil, fmt.Errorf("serialize component %q: %w", item.name, err)
		}
		safe = append(safe, item)
		rendered = append(rendered, data)
	}

	if err := prepareOutputDir(opts.OutputDir); err != nil {
		return nil, err
	}
	for i, item := range safe {
		outcome, err := writeFile(opts.OutputDir, item.path, rendered[i])
		if err != nil {
			return report, fmt.Errorf("write component %q: %w", item.name, err)
		}
		switch outcome {
		case outcomeCreated:
			report.Created++
		case outcomeUpdated:
			report.Updated++
		case outcomeUnchanged:
			report.Unchanged++
		}
		report.Exported++
	}
	return report, nil
}

type resource struct {
	name      string
	namespace string
	project   string
	path      string
	object    map[string]any
}

func nextCursor(p gen.Pagination) string {
	if p.NextCursor == nil {
		return ""
	}
	return *p.NextCursor
}

func validName(kind, name string) error {
	if name == "" || name == "." || name == ".." || strings.ContainsAny(name, `/\`) {
		return fmt.Errorf("%s name %q is not safe to use in an output path", kind, name)
	}
	return nil
}
