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

const (
	openchoreoAPIVersion = "openchoreo.dev/v1alpha1"
	namespaceAPIVersion  = "v1"
)

// Options configures one export run.
type Options struct {
	OutputDir     string
	KeepPlaintext func(Withheld) bool
}

// Report summarizes the files written by one export run.
type Report struct {
	Created         int
	Updated         int
	Unchanged       int
	Exported        int
	ExportedByKind  map[string]int
	SecretDecisions []SecretDecision
	Unsupported     []string
}

// Withheld describes the locations of credential-like values without storing
// or exposing the values themselves.
type Withheld struct {
	Kind      string
	Namespace string
	Project   string
	Name      string
	Reasons   []string
}

// SecretDecision records how a resource containing credential-like values was handled.
type SecretDecision struct {
	Withheld
	Action string
}

// runEngine exports namespaces, projects, components, and workloads returned
// by the Control Plane API.
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

		namespaceObject, err := normalizeObject(namespace, "Namespace", "")
		if err != nil {
			return nil, fmt.Errorf("namespace %q: %w", namespaceName, err)
		}
		resources = append(resources, resource{
			kind: "Namespace", name: namespaceName,
			path: path.Join("namespaces", namespaceName, "namespace.yaml"), object: namespaceObject,
		})

		projects, err := pagination.FetchAll(func(limit int, cursor string) ([]gen.Project, string, error) {
			params := &gen.ListProjectsParams{Limit: &limit}
			if cursor != "" {
				params.Cursor = &cursor
			}
			result, err := c.ListProjects(ctx, namespaceName, params)
			if err != nil {
				return nil, "", err
			}
			return result.Items, nextCursor(result.Pagination), nil
		})
		if err != nil {
			return nil, fmt.Errorf("namespace %q: list projects (nothing was written): %w", namespaceName, err)
		}

		for _, project := range projects {
			projectName := project.Metadata.Name
			if err := validName("project", projectName); err != nil {
				return nil, err
			}
			projectObject, err := normalizeObject(project, "Project", namespaceName)
			if err != nil {
				return nil, fmt.Errorf("project %q in namespace %q: %w", projectName, namespaceName, err)
			}
			projectDir := path.Join("namespaces", namespaceName, "projects", projectName)
			resources = append(resources, resource{
				kind: "Project", name: projectName, namespace: namespaceName, project: projectName,
				path: path.Join(projectDir, "project.yaml"), object: projectObject,
			})

			components, err := pagination.FetchAll(func(limit int, cursor string) ([]gen.Component, string, error) {
				params := &gen.ListComponentsParams{Limit: &limit, Project: &projectName}
				if cursor != "" {
					params.Cursor = &cursor
				}
				result, err := c.ListComponents(ctx, namespaceName, projectName, params)
				if err != nil {
					return nil, "", err
				}
				return result.Items, nextCursor(result.Pagination), nil
			})
			if err != nil {
				return nil, fmt.Errorf("project %q in namespace %q: list components (nothing was written): %w", projectName, namespaceName, err)
			}

			for _, component := range components {
				componentName := component.Metadata.Name
				if err := validName("component", componentName); err != nil {
					return nil, err
				}
				if component.Spec == nil || component.Spec.Owner.ProjectName != projectName {
					return nil, fmt.Errorf("component %q in namespace %q has an invalid project owner; expected %q", componentName, namespaceName, projectName)
				}
				componentObject, err := normalizeObject(component, "Component", namespaceName)
				if err != nil {
					return nil, fmt.Errorf("component %q in namespace %q: %w", componentName, namespaceName, err)
				}
				componentDir := path.Join(projectDir, "components", componentName)
				resources = append(resources, resource{
					kind: "Component", name: componentName, namespace: namespaceName, project: projectName,
					path: path.Join(componentDir, "component.yaml"), object: componentObject,
				})

				workloads, err := pagination.FetchAll(func(limit int, cursor string) ([]gen.Workload, string, error) {
					params := &gen.ListWorkloadsParams{Limit: &limit, Component: &componentName}
					if cursor != "" {
						params.Cursor = &cursor
					}
					result, err := c.ListWorkloads(ctx, namespaceName, params)
					if err != nil {
						return nil, "", err
					}
					return result.Items, nextCursor(result.Pagination), nil
				})
				if err != nil {
					return nil, fmt.Errorf("component %q in namespace %q: list workloads (nothing was written): %w", componentName, namespaceName, err)
				}
				if len(workloads) > 1 {
					return nil, fmt.Errorf("component %q in namespace %q has %d workloads; expected at most one", componentName, namespaceName, len(workloads))
				}
				for _, workload := range workloads {
					workloadName := workload.Metadata.Name
					if err := validName("workload", workloadName); err != nil {
						return nil, err
					}
					if workload.Spec == nil || workload.Spec.Owner == nil || workload.Spec.Owner.ComponentName != componentName || workload.Spec.Owner.ProjectName != projectName {
						return nil, fmt.Errorf("workload %q in namespace %q has an invalid owner; expected component %q in project %q", workloadName, namespaceName, componentName, projectName)
					}
					workloadObject, err := normalizeObject(workload, "Workload", namespaceName)
					if err != nil {
						return nil, fmt.Errorf("workload %q in namespace %q: %w", workloadName, namespaceName, err)
					}
					resources = append(resources, resource{
						kind: "Workload", name: workloadName, namespace: namespaceName, project: projectName,
						path: path.Join(componentDir, "workload.yaml"), object: workloadObject,
					})
				}
			}
		}
	}

	sort.Slice(resources, func(i, j int) bool { return resources[i].path < resources[j].path })
	report := &Report{
		Unsupported: []string{
			"component types, traits, workflows, SecretReferences, and other namespace platform configuration",
			"component releases and release bindings",
			"cluster-scoped resources",
		},
		ExportedByKind: map[string]int{},
	}
	var safe []resource
	var rendered [][]byte
	for _, item := range resources {
		if reasons := findPlaintextSecrets(item.object); len(reasons) > 0 {
			finding := Withheld{
				Kind:      item.kind,
				Namespace: item.namespace,
				Project:   item.project,
				Name:      item.name,
				Reasons:   reasons,
			}
			if opts.KeepPlaintext != nil && opts.KeepPlaintext(finding) {
				report.SecretDecisions = append(report.SecretDecisions, SecretDecision{Withheld: finding, Action: "kept"})
			} else {
				stripped := stripPlaintextSecrets(item.object)
				report.SecretDecisions = append(report.SecretDecisions, SecretDecision{Withheld: finding, Action: fmt.Sprintf("stripped %d value(s)", stripped)})
			}
		}
		data, err := serialize(item.object)
		if err != nil {
			return nil, fmt.Errorf("serialize %s %q: %w", item.kind, item.name, err)
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
			return report, fmt.Errorf("write %s %q: %w", item.kind, item.name, err)
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
		report.ExportedByKind[item.kind]++
	}
	return report, nil
}

type resource struct {
	kind      string
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
