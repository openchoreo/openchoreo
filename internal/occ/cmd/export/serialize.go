// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package export

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"sigs.k8s.io/yaml"
)

type resourceScope int

const (
	clusterScope resourceScope = iota
	namespaceScope
	platformScope
	projectScope
	componentScope
	resourceScopeLevel
)

type resourceLayout struct {
	scope     resourceScope
	directory string
	filename  string
}

// Each fetched kind has an explicit layout. Ownership comes from spec.owner;
// object names are never parsed or changed to construct the directory tree.
var resourceLayouts = map[string]resourceLayout{
	"ClusterProjectType":        {scope: clusterScope, directory: "project-types"},
	"ClusterComponentType":      {scope: clusterScope, directory: "component-types"},
	"ClusterResourceType":       {scope: clusterScope, directory: "resource-types"},
	"ClusterTrait":              {scope: clusterScope, directory: "traits"},
	"ClusterWorkflow":           {scope: clusterScope, directory: "workflows"},
	"ClusterDataPlane":          {scope: clusterScope, directory: "infra/data-planes"},
	"ClusterWorkflowPlane":      {scope: clusterScope, directory: "infra/workflow-planes"},
	"ClusterObservabilityPlane": {scope: clusterScope, directory: "infra/observability-planes"},
	"ClusterAuthzRole":          {scope: clusterScope, directory: "authz/roles"},
	"ClusterAuthzRoleBinding":   {scope: clusterScope, directory: "authz/role-bindings"},
	"Namespace":                 {scope: namespaceScope, filename: "namespace.yaml"},
	"DeploymentPipeline":        {scope: platformScope, directory: "infra/deployment-pipelines"},
	"Environment":               {scope: platformScope, directory: "infra/environments"},
	"DataPlane":                 {scope: platformScope, directory: "infra/data-planes"},
	"WorkflowPlane":             {scope: platformScope, directory: "infra/workflow-planes"},
	"ObservabilityPlane":        {scope: platformScope, directory: "infra/observability-planes"},
	"ProjectType":               {scope: platformScope, directory: "project-types"},
	"ComponentType":             {scope: platformScope, directory: "component-types"},
	"ResourceType":              {scope: platformScope, directory: "resource-types"},
	"Trait":                     {scope: platformScope, directory: "traits"},
	"Workflow":                  {scope: platformScope, directory: "workflows"},
	"AuthzRole":                 {scope: platformScope, directory: "authz/roles"},
	"AuthzRoleBinding":          {scope: platformScope, directory: "authz/role-bindings"},
	"SecretReference":           {scope: platformScope, directory: "secret-references"},
	"Project":                   {scope: projectScope, filename: "project.yaml"},
	"ProjectRelease":            {scope: projectScope, directory: "releases"},
	"ProjectReleaseBinding":     {scope: projectScope, directory: "release-bindings"},
	"Component":                 {scope: componentScope, filename: "component.yaml"},
	"Workload":                  {scope: componentScope, directory: "workloads"},
	"ComponentRelease":          {scope: componentScope, directory: "releases"},
	"ReleaseBinding":            {scope: componentScope, directory: "release-bindings"},
	"Resource":                  {scope: resourceScopeLevel, filename: "resource.yaml"},
	"ResourceRelease":           {scope: resourceScopeLevel, directory: "releases"},
	"ResourceReleaseBinding":    {scope: resourceScopeLevel, directory: "release-bindings"},
}

type resourceOwner struct {
	ProjectName   string `json:"projectName"`
	ComponentName string `json:"componentName"`
	ResourceName  string `json:"resourceName"`
}

// manifestIdentity reads only the fields needed for layout and validation.
// Encoding uses the complete JSON document, preserving every exposed API field.
type manifestIdentity struct {
	APIVersion string `json:"apiVersion"`
	Kind       string `json:"kind"`
	Metadata   struct {
		Name      string `json:"name"`
		Namespace string `json:"namespace"`
	} `json:"metadata"`
	Spec struct {
		Owner resourceOwner `json:"owner"`
	} `json:"spec"`
}

// Serialize prepares one YAML manifest per resource without writing files or
// modifying the input objects. Paths are sorted and duplicates are rejected.
// Filtering, redaction, and canonicalization remain separate pipeline stages.
func Serialize(ctx context.Context, resources []Resource) ([]SerializedFile, error) {
	files := make([]SerializedFile, 0, len(resources))
	seen := make(map[string]struct{}, len(resources))
	for _, resource := range resources {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		file, err := serializeResource(resource)
		if err != nil {
			return nil, fmt.Errorf("serialize %s %q in namespace %q: %w", resource.Kind, resource.Name, resource.Namespace, err)
		}
		if _, exists := seen[file.Path]; exists {
			return nil, fmt.Errorf("duplicate export path %q", file.Path)
		}
		seen[file.Path] = struct{}{}
		files = append(files, file)
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return files, nil
}

func serializeResource(resource Resource) (SerializedFile, error) {
	data, err := json.Marshal(resource.Object)
	if err != nil {
		return SerializedFile{}, fmt.Errorf("encode API object: %w", err)
	}
	if resource.Kind == "Namespace" {
		data, err = namespaceManifest(data)
		if err != nil {
			return SerializedFile{}, err
		}
	}
	var identity manifestIdentity
	if err := json.Unmarshal(data, &identity); err != nil {
		return SerializedFile{}, fmt.Errorf("read manifest identity: %w", err)
	}
	if identity.Metadata.Name != resource.Name || identity.Metadata.Namespace != resource.Namespace {
		return SerializedFile{}, fmt.Errorf("object metadata does not match resource identity")
	}
	if identity.Kind != resource.Kind || identity.APIVersion == "" {
		return SerializedFile{}, fmt.Errorf("object must have matching kind and a nonempty apiVersion")
	}
	filePath, err := resourcePath(resource, identity.Spec.Owner)
	if err != nil {
		return SerializedFile{}, err
	}
	// JSONToYAML uses the same JSON-tag-aware encoding as yaml.Marshal, without
	// marshaling the typed object again after reading its identity.
	encoded, err := yaml.JSONToYAML(data)
	if err != nil {
		return SerializedFile{}, fmt.Errorf("encode YAML: %w", err)
	}
	return SerializedFile{Path: filePath, Data: encoded}, nil
}

// Namespace is the one generated API model without type metadata. Raw messages
// add its Kubernetes envelope while preserving the returned metadata and status.
func namespaceManifest(data []byte) ([]byte, error) {
	var document map[string]json.RawMessage
	if err := json.Unmarshal(data, &document); err != nil {
		return nil, fmt.Errorf("read Namespace object: %w", err)
	}
	if document == nil {
		return nil, fmt.Errorf("namespace object is required")
	}
	document["apiVersion"] = json.RawMessage(`"v1"`)
	document["kind"] = json.RawMessage(`"Namespace"`)
	return json.Marshal(document)
}

func resourcePath(resource Resource, owner resourceOwner) (string, error) {
	layout, ok := resourceLayouts[resource.Kind]
	if !ok {
		return "", fmt.Errorf("unsupported resource kind %q", resource.Kind)
	}
	if err := validatePathPart(resource.Name, "metadata.name"); err != nil {
		return "", err
	}
	base, err := layoutBase(resource, layout.scope, owner)
	if err != nil {
		return "", err
	}
	filename := layout.filename
	if filename == "" {
		filename = resource.Name + ".yaml"
	}
	return path.Join(base, layout.directory, filename), nil
}

func layoutBase(resource Resource, scope resourceScope, owner resourceOwner) (string, error) {
	if scope == clusterScope || scope == namespaceScope {
		if resource.Namespace != "" {
			return "", fmt.Errorf("cluster-scoped object must not have a namespace")
		}
		if scope == namespaceScope {
			return path.Join("namespaces", resource.Name), nil
		}
		return "platform-shared", nil
	}
	if err := validatePathPart(resource.Namespace, "metadata.namespace"); err != nil {
		return "", err
	}
	base := path.Join("namespaces", resource.Namespace)
	if scope == platformScope {
		return path.Join(base, "platform"), nil
	}
	return applicationBase(base, resource, scope, owner)
}

func applicationBase(base string, resource Resource, scope resourceScope, owner resourceOwner) (string, error) {
	projectName := owner.ProjectName
	if resource.Kind == "Project" {
		projectName = resource.Name
	}
	if err := validatePathPart(projectName, "spec.owner.projectName"); err != nil {
		return "", err
	}
	base = path.Join(base, "projects", projectName)
	if scope == projectScope {
		return base, nil
	}
	if scope == componentScope {
		name := owner.ComponentName
		if resource.Kind == "Component" {
			name = resource.Name
		}
		if err := validatePathPart(name, "spec.owner.componentName"); err != nil {
			return "", err
		}
		return path.Join(base, "components", name), nil
	}
	name := owner.ResourceName
	if resource.Kind == "Resource" {
		name = resource.Name
	}
	if err := validatePathPart(name, "spec.owner.resourceName"); err != nil {
		return "", err
	}
	return path.Join(base, "resources", name), nil
}

func validatePathPart(value, field string) error {
	if value == "" || value == "." || value == ".." || strings.ContainsAny(value, "/\\:\x00") || strings.TrimSpace(value) != value {
		return fmt.Errorf("%s must be a nonempty, safe path component", field)
	}
	return nil
}

// writeFiles checks every destination before creating any manifest. Root keeps
// file operations inside the chosen output directory, including through symlinks.
// Exclusive file creation also prevents overwrites if a file appears after the
// destination check. An I/O failure may leave files already written by this run.
func writeFiles(ctx context.Context, outputDir string, files []SerializedFile) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	for _, file := range files {
		for _, part := range strings.Split(file.Path, "/") {
			if err := validatePathPart(part, "export path"); err != nil {
				return fmt.Errorf("invalid export path %q: %w", file.Path, err)
			}
		}
	}
	if err := os.MkdirAll(outputDir, 0o700); err != nil {
		return fmt.Errorf("create output directory %q: %w", outputDir, err)
	}
	root, err := os.OpenRoot(outputDir)
	if err != nil {
		return fmt.Errorf("open output directory %q: %w", outputDir, err)
	}
	defer root.Close()
	if err := checkDestinations(ctx, root, files); err != nil {
		return err
	}
	for _, file := range files {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := root.MkdirAll(filepath.Dir(filepath.FromSlash(file.Path)), 0o700); err != nil {
			return fmt.Errorf("create parent directory for %q: %w", file.Path, err)
		}
		if err := writeManifest(root, file); err != nil {
			return err
		}
	}
	return ctx.Err()
}

func checkDestinations(ctx context.Context, root *os.Root, files []SerializedFile) error {
	for _, file := range files {
		if err := ctx.Err(); err != nil {
			return err
		}
		_, err := root.Lstat(filepath.FromSlash(file.Path))
		if err == nil {
			return fmt.Errorf("export destination %q already exists; existing files are not overwritten", file.Path)
		}
		if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("check export destination %q: %w", file.Path, err)
		}
	}
	return nil
}

func writeManifest(root *os.Root, file SerializedFile) error {
	name := filepath.FromSlash(file.Path)
	f, err := root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("create manifest %q: %w", file.Path, err)
	}
	_, writeErr := f.Write(file.Data)
	if err := errors.Join(writeErr, f.Close()); err != nil {
		_ = root.Remove(name)
		return fmt.Errorf("write manifest %q: %w", file.Path, err)
	}
	return nil
}
