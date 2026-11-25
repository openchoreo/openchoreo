// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package export

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"syscall"

	yamlv3 "gopkg.in/yaml.v3"
	"sigs.k8s.io/yaml"
)

const (
	endpointKind        = "Endpoint"
	endpointReleaseKind = "EndpointRelease"
)

type resourceScope int

const (
	clusterScope resourceScope = iota
	namespaceScope
	platformScope
	projectScope
	componentScope
	resourceScopeLevel
	endpointScope
)

type resourceLayout struct {
	scope     resourceScope
	directory string
	filename  string
}

// resourceLayouts maps supported kinds to export scopes and paths.
var resourceLayouts = map[string]resourceLayout{
	"ClusterProjectType":                     {scope: clusterScope, directory: "project-types"},
	"ClusterComponentType":                   {scope: clusterScope, directory: "component-types"},
	"ClusterResourceType":                    {scope: clusterScope, directory: "resource-types"},
	"ClusterEndpointType":                    {scope: clusterScope, directory: "endpoint-types"},
	"ClusterTrait":                           {scope: clusterScope, directory: "traits"},
	"ClusterWorkflow":                        {scope: clusterScope, directory: "workflows"},
	"ClusterDataPlane":                       {scope: clusterScope, directory: "infra/data-planes"},
	"ClusterWorkflowPlane":                   {scope: clusterScope, directory: "infra/workflow-planes"},
	"ClusterObservabilityPlane":              {scope: clusterScope, directory: "infra/observability-planes"},
	"ClusterAuthzRole":                       {scope: clusterScope, directory: "authz/roles"},
	"ClusterAuthzRoleBinding":                {scope: clusterScope, directory: "authz/role-bindings"},
	"Namespace":                              {scope: namespaceScope, filename: "namespace.yaml"},
	"DeploymentPipeline":                     {scope: platformScope, directory: "infra/deployment-pipelines"},
	"Environment":                            {scope: platformScope, directory: "infra/environments"},
	"DataPlane":                              {scope: platformScope, directory: "infra/data-planes"},
	"WorkflowPlane":                          {scope: platformScope, directory: "infra/workflow-planes"},
	"ObservabilityPlane":                     {scope: platformScope, directory: "infra/observability-planes"},
	"ObservabilityAlertsNotificationChannel": {scope: platformScope, directory: "infra/observability-alerts-notification-channels"},
	"ProjectType":                            {scope: platformScope, directory: "project-types"},
	"ComponentType":                          {scope: platformScope, directory: "component-types"},
	"ResourceType":                           {scope: platformScope, directory: "resource-types"},
	"EndpointType":                           {scope: platformScope, directory: "endpoint-types"},
	"Trait":                                  {scope: platformScope, directory: "traits"},
	"Workflow":                               {scope: platformScope, directory: "workflows"},
	"AuthzRole":                              {scope: platformScope, directory: "authz/roles"},
	"AuthzRoleBinding":                       {scope: platformScope, directory: "authz/role-bindings"},
	"SecretReference":                        {scope: platformScope, directory: "secret-references"},
	"Project":                                {scope: projectScope, filename: "project.yaml"},
	"ProjectRelease":                         {scope: projectScope, directory: "releases"},
	"ProjectReleaseBinding":                  {scope: projectScope, directory: "release-bindings"},
	"Component":                              {scope: componentScope, filename: "component.yaml"},
	"ReleaseLine":                            {scope: componentScope, directory: "release-lines"},
	"Workload":                               {scope: componentScope, directory: "workloads"},
	"ComponentRelease":                       {scope: componentScope, directory: "releases"},
	"ReleaseBinding":                         {scope: componentScope, directory: "release-bindings"},
	"Resource":                               {scope: resourceScopeLevel, filename: "resource.yaml"},
	"ResourceRelease":                        {scope: resourceScopeLevel, directory: "releases"},
	"ResourceReleaseBinding":                 {scope: resourceScopeLevel, directory: "release-bindings"},
	endpointKind:                             {scope: endpointScope, filename: "endpoint.yaml"},
	endpointReleaseKind:                      {scope: endpointScope, directory: "releases"},
	"EndpointReleaseBinding":                 {scope: endpointScope, directory: "release-bindings"},
}

type resourceOwner struct {
	ProjectName   string `json:"projectName"`
	ComponentName string `json:"componentName"`
	ResourceName  string `json:"resourceName"`
	EndpointName  string `json:"endpointName"`
}

// manifestIdentity reads only the fields needed for layout and validation.
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

// Serialize returns sorted YAML manifests, rejecting duplicate paths without modifying resources.
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
	document, ok := resource.Object.(resourceDocument)
	if !ok {
		var err error
		document, err = copyResourceDocument(resource)
		if err != nil {
			return SerializedFile{}, err
		}
	}
	identity, err := readManifestIdentity(document)
	if err != nil {
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
	encoded, err := encodeYAML(document)
	if err != nil {
		return SerializedFile{}, fmt.Errorf("encode YAML: %w", err)
	}
	return SerializedFile{Path: filePath, Data: encoded}, nil
}

func readManifestIdentity(document map[string]any) (manifestIdentity, error) {
	var identity manifestIdentity
	for _, field := range []struct {
		name   string
		target *string
	}{
		{"apiVersion", &identity.APIVersion},
		{"kind", &identity.Kind},
		{"metadata.name", &identity.Metadata.Name},
		{"metadata.namespace", &identity.Metadata.Namespace},
		{"spec.owner.projectName", &identity.Spec.Owner.ProjectName},
		{"spec.owner.componentName", &identity.Spec.Owner.ComponentName},
		{"spec.owner.resourceName", &identity.Spec.Owner.ResourceName},
		{"spec.owner.endpointName", &identity.Spec.Owner.EndpointName},
	} {
		var value any = document
		for _, key := range strings.Split(field.name, ".") {
			if value == nil {
				break
			}
			object, ok := value.(map[string]any)
			if !ok {
				return manifestIdentity{}, fmt.Errorf("%s must be within JSON objects", field.name)
			}
			value = object[key]
		}
		if value == nil {
			continue
		}
		text, ok := value.(string)
		if !ok {
			return manifestIdentity{}, fmt.Errorf("%s must be a string", field.name)
		}
		*field.target = text
	}
	return identity, nil
}

func encodeYAML(document map[string]any) ([]byte, error) {
	var output bytes.Buffer
	encoder := yamlv3.NewEncoder(&output)
	encoder.SetIndent(2)
	if err := encoder.Encode(prepareYAMLValues(document)); err != nil {
		return nil, err
	}
	if err := encoder.Close(); err != nil {
		return nil, err
	}
	return output.Bytes(), nil
}

// Preserve numeric text and literal map keys in the decoded JSON document.
func prepareYAMLValues(value any) any {
	switch value := value.(type) {
	case json.Number:
		// Emit the numeric literal without an explicit tag.
		return &yamlv3.Node{Kind: yamlv3.ScalarNode, Value: value.String()}
	case map[string]any:
		mapping := make(map[yamlMapKey]any, len(value))
		for key, item := range value {
			mapping[yamlMapKey(key)] = prepareYAMLValues(item)
		}
		return mapping
	case []any:
		sequence := make([]any, len(value))
		for i, item := range value {
			sequence[i] = prepareYAMLValues(item)
		}
		return sequence
	}
	return value
}

type yamlMapKey string

func (key yamlMapKey) MarshalYAML() (any, error) {
	if key == "<<" {
		// Quote the literal << key.
		return &yamlv3.Node{Kind: yamlv3.ScalarNode, Tag: "!!str", Value: string(key), Style: yamlv3.DoubleQuotedStyle}, nil
	}
	return string(key), nil
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
	parts := strings.Split(path.Join(base, layout.directory, filename), "/")
	for i, part := range parts {
		parts[i] = portablePathPart(part)
	}
	return strings.Join(parts, "/"), nil
}

// portablePathPart escapes Windows device names with ~ and doubles literal ~ characters.
func portablePathPart(value string) string {
	encoded := strings.ReplaceAll(value, "~", "~~")
	base, _, _ := strings.Cut(value, ".")
	base = strings.ToUpper(strings.TrimRight(base, " "))
	switch base {
	case "CON", "PRN", "AUX", "NUL", "CONIN$", "CONOUT$":
		return "~" + encoded
	}
	if strings.HasPrefix(base, "COM") || strings.HasPrefix(base, "LPT") {
		switch base[3:] {
		case "1", "2", "3", "4", "5", "6", "7", "8", "9", "¹", "²", "³":
			return "~" + encoded
		}
	}
	return encoded
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
	if scope == endpointScope {
		name := owner.EndpointName
		if resource.Kind == endpointKind {
			name = resource.Name
		}
		if err := validatePathPart(name, "spec.owner.endpointName"); err != nil {
			return "", err
		}
		return path.Join(base, "endpoints", name), nil
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

type writeOptions struct {
	preservedPaths map[string]struct{}
}

type fileSnapshot struct {
	data []byte
	mode fs.FileMode
}

type fileChange struct {
	path          string
	before        *fileSnapshot
	after         []byte
	remove        bool
	backedUp      bool
	backupMoved   bool
	installed     bool
	installedInfo fs.FileInfo
}

type directorySnapshot struct {
	path string
	mode fs.FileMode
}

// fileTransaction stages changes and retains originals for rollback.
type fileTransaction struct {
	root        *os.Root
	scratch     string
	changes     []fileChange
	createdDirs []string
	removedDirs []directorySnapshot
	keepBackups bool
	// link defaults to root.Link.
	link func(string, string) error
}

// writeFiles synchronizes managed manifests and rolls back failures.
// Rollback conflicts retain backups; abrupt termination is not recoverable.
func writeFiles(ctx context.Context, outputDir string, files []SerializedFile, options writeOptions) (counts FileCounts, resultErr error) {
	if err := validateExportFiles(ctx, files); err != nil {
		return FileCounts{}, err
	}
	created, err := createOutputDir(outputDir)
	if err != nil {
		return FileCounts{}, err
	}
	defer func() {
		if resultErr != nil {
			for i := len(created) - 1; i >= 0; i-- {
				// Remove newly created empty directories.
				if err := os.Remove(created[i]); err != nil {
					resultErr = errors.Join(resultErr, fmt.Errorf("remove new directory %q: %w", created[i], err))
				}
			}
		}
	}()
	root, err := os.OpenRoot(outputDir)
	if err != nil {
		return FileCounts{}, fmt.Errorf("open output directory: %w", err)
	}
	defer root.Close()
	changes, err := planFileChanges(ctx, root, files, options.preservedPaths)
	if err != nil {
		return FileCounts{}, err
	}
	transaction := &fileTransaction{root: root, changes: changes}
	defer func() {
		if transaction.scratch != "" && !transaction.keepBackups {
			_ = root.RemoveAll(transaction.scratch)
		}
	}()
	if err := transaction.stage(ctx); err != nil {
		return FileCounts{}, fmt.Errorf("stage export: %w", err)
	}
	if err := transaction.apply(ctx); err != nil {
		return FileCounts{}, transaction.abort(err, outputDir)
	}
	if err := ctx.Err(); err != nil {
		return FileCounts{}, transaction.abort(err, outputDir)
	}
	return summarizeFileChanges(len(files), changes), nil
}

// summarizeFileChanges counts committed creates, updates, deletions, and unchanged files.
func summarizeFileChanges(resources int, changes []fileChange) FileCounts {
	counts := FileCounts{Unchanged: resources}
	for _, change := range changes {
		switch {
		case change.remove:
			counts.Deleted++
		case change.before == nil:
			counts.Created++
			counts.Unchanged--
		default:
			counts.Updated++
			counts.Unchanged--
		}
	}
	return counts
}

func validateExportFiles(ctx context.Context, files []SerializedFile) error {
	seen := make(map[string]bool, len(files))
	for _, file := range files {
		if err := ctx.Err(); err != nil {
			return err
		}
		for _, part := range strings.Split(file.Path, "/") {
			if err := validatePathPart(part, "export path"); err != nil {
				return fmt.Errorf("invalid export path %q: %w", file.Path, err)
			}
		}
		if seen[file.Path] {
			return fmt.Errorf("duplicate export path %q", file.Path)
		}
		seen[file.Path] = true
	}
	return ctx.Err()
}

func createOutputDir(directory string) ([]string, error) {
	var missing, created []string
	for current := filepath.Clean(directory); ; current = filepath.Dir(current) {
		info, err := os.Stat(current)
		if err == nil {
			if !info.IsDir() {
				return nil, fmt.Errorf("output ancestor %q is not a directory", current)
			}
			break
		}
		if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		missing = append(missing, current)
	}
	for i := len(missing) - 1; i >= 0; i-- {
		if err := os.Mkdir(missing[i], 0o755); err != nil {
			for j := len(created) - 1; j >= 0; j-- {
				err = errors.Join(err, os.Remove(created[j]))
			}
			return nil, fmt.Errorf("create output directory: %w", err)
		}
		created = append(created, missing[i])
	}
	return created, nil
}

func planFileChanges(ctx context.Context, root *os.Root, files []SerializedFile, preservedPaths map[string]struct{}) ([]fileChange, error) {
	managed, err := findManagedFiles(ctx, root, preservedPaths)
	if err != nil {
		return nil, err
	}
	changes := make([]fileChange, 0, len(files)+len(managed))
	for _, file := range files {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if err := checkDestination(root, file.Path, managed); err != nil {
			return nil, err
		}
		previous := managed[file.Path]
		delete(managed, file.Path)
		if previous != nil && bytes.Equal(previous.data, file.Data) {
			continue
		}
		changes = append(changes, fileChange{path: file.Path, before: previous, after: file.Data})
	}
	for name, previous := range managed {
		changes = append(changes, fileChange{path: name, before: previous, remove: true})
	}
	sort.Slice(changes, func(i, j int) bool { return changes[i].path < changes[j].path })
	return changes, nil
}

func findManagedFiles(ctx context.Context, root *os.Root, preservedPaths map[string]struct{}) (map[string]*fileSnapshot, error) {
	managed := make(map[string]*fileSnapshot)
	for _, prefix := range []string{"platform-shared", "namespaces"} {
		err := fs.WalkDir(root.FS(), prefix, func(name string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				if name == prefix && errors.Is(walkErr, os.ErrNotExist) {
					return nil
				}
				return walkErr
			}
			if err := ctx.Err(); err != nil {
				return err
			}
			if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 || path.Ext(name) != ".yaml" {
				return nil
			}
			// Preserve existing releases excluded by the history window.
			if _, preserve := preservedPaths[name]; preserve {
				return nil
			}
			return collectManagedFile(root, name, entry, managed)
		})
		if err != nil {
			return nil, fmt.Errorf("inspect existing export: %w", err)
		}
	}
	return managed, nil
}

func collectManagedFile(root *os.Root, name string, entry fs.DirEntry, managed map[string]*fileSnapshot) error {
	info, err := entry.Info()
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return nil
	}
	data, err := root.ReadFile(filepath.FromSlash(name))
	if err != nil {
		return err
	}
	if isManagedManifest(name, data) {
		managed[name] = &fileSnapshot{data: data, mode: info.Mode().Perm()}
	}
	return nil
}

func isManagedManifest(name string, data []byte) bool {
	// Accept only valid, single-document YAML.
	decoder := yamlv3.NewDecoder(bytes.NewReader(data))
	var node yamlv3.Node
	if err := decoder.Decode(&node); err != nil {
		return false
	}
	if err := decoder.Decode(&node); !errors.Is(err, io.EOF) {
		return false
	}
	data, err := yaml.YAMLToJSONStrict(data)
	if err != nil {
		return false
	}
	var identity manifestIdentity
	if err := json.Unmarshal(data, &identity); err != nil {
		return false
	}
	version := "openchoreo.dev/v1alpha1"
	if identity.Kind == "Namespace" {
		version = "v1"
	}
	if identity.APIVersion != version {
		return false
	}
	resource := Resource{Kind: identity.Kind, Name: identity.Metadata.Name, Namespace: identity.Metadata.Namespace}
	expected, err := resourcePath(resource, identity.Spec.Owner)
	return err == nil && expected == name
}

func checkDestination(root *os.Root, name string, managed map[string]*fileSnapshot) error {
	parts := strings.Split(name, "/")
	for i := range parts {
		part := filepath.Join(parts[:i+1]...)
		info, err := root.Lstat(part)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("check destination %q: %w", name, err)
		}
		if info.Mode()&os.ModeSymlink != 0 || (i < len(parts)-1 && !info.IsDir()) {
			return fmt.Errorf("export destination %q already exists with a symlink or non-directory ancestor", name)
		}
		if i == len(parts)-1 && (!info.Mode().IsRegular() || managed[name] == nil) {
			return fmt.Errorf("export destination %q already exists and is not a managed manifest", name)
		}
	}
	return nil
}

func (t *fileTransaction) stage(ctx context.Context) error {
	if len(t.changes) == 0 {
		return nil
	}
	t.scratch = ".occ-export-" + rand.Text()
	if err := t.root.Mkdir(t.scratch, 0o700); err != nil {
		// Clear the scratch path after creation fails.
		t.scratch = ""
		return err
	}
	for i, change := range t.changes {
		if err := ctx.Err(); err != nil {
			return err
		}
		if change.remove {
			continue
		}
		if err := t.stageFile(i); err != nil {
			return fmt.Errorf("prepare %q: %w", change.path, err)
		}
	}
	return nil
}

func (t *fileTransaction) stageFile(index int) error {
	change := t.changes[index]
	mode := fs.FileMode(0o644)
	if change.before != nil {
		mode = change.before.mode
	}
	file, err := t.root.OpenFile(t.stagedName("new", index), os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	_, writeErr := file.Write(change.after)
	var chmodErr error
	if change.before != nil {
		// Restore the existing file mode.
		chmodErr = file.Chmod(mode)
	}
	return errors.Join(writeErr, chmodErr, file.Sync(), file.Close())
}

func (t *fileTransaction) stagedName(prefix string, index int) string {
	return filepath.Join(t.scratch, fmt.Sprintf("%s-%d", prefix, index))
}

func (t *fileTransaction) apply(ctx context.Context) error {
	for i := range t.changes {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := t.applyChange(i); err != nil {
			return fmt.Errorf("synchronize %q: %w", t.changes[i].path, err)
		}
	}
	return t.removeEmptyDirs(ctx)
}

func (t *fileTransaction) applyChange(index int) error {
	change := &t.changes[index]
	if err := t.checkUnchanged(change); err != nil {
		return err
	}
	if change.before != nil {
		var err error
		if change.remove {
			err = t.root.Rename(filepath.FromSlash(change.path), t.stagedName("old", index))
		} else {
			// Link the original into the backup directory.
			link := t.link
			if link == nil {
				link = t.root.Link
			}
			err = link(filepath.FromSlash(change.path), t.stagedName("old", index))
			// Fall back to renaming when hard links are unavailable.
			if errors.Is(err, errors.ErrUnsupported) || errors.Is(err, syscall.EPERM) {
				err = t.root.Rename(filepath.FromSlash(change.path), t.stagedName("old", index))
				change.backupMoved = err == nil
			}
		}
		if err != nil {
			return err
		}
		change.backedUp = true
	}
	if change.remove {
		return nil
	}
	if err := t.makeParents(change.path); err != nil {
		return err
	}
	installedInfo, err := t.root.Lstat(t.stagedName("new", index))
	if err != nil {
		return err
	}
	if err := t.root.Rename(t.stagedName("new", index), filepath.FromSlash(change.path)); err != nil {
		return err
	}
	change.installed = true
	change.installedInfo = installedInfo
	return nil
}

func (t *fileTransaction) checkUnchanged(change *fileChange) error {
	info, err := t.root.Lstat(filepath.FromSlash(change.path))
	if change.before == nil && errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if change.before == nil || !info.Mode().IsRegular() {
		return fmt.Errorf("destination changed while export was being prepared")
	}
	data, err := t.root.ReadFile(filepath.FromSlash(change.path))
	if err != nil {
		return err
	}
	if !bytes.Equal(data, change.before.data) || info.Mode().Perm() != change.before.mode {
		return fmt.Errorf("destination changed while export was being prepared")
	}
	return nil
}

func (t *fileTransaction) makeParents(name string) error {
	parts := strings.Split(name, "/")
	for i := 1; i < len(parts); i++ {
		directory := filepath.Join(parts[:i]...)
		info, err := t.root.Lstat(directory)
		if err == nil {
			if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
				return fmt.Errorf("parent %q is not a regular directory", directory)
			}
			continue
		}
		if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if err := t.root.Mkdir(directory, 0o755); err != nil {
			return err
		}
		t.createdDirs = append(t.createdDirs, directory)
	}
	return nil
}

func (t *fileTransaction) removeEmptyDirs(ctx context.Context) error {
	candidates := make(map[string]bool)
	for _, change := range t.changes {
		if change.remove {
			for directory := path.Dir(change.path); directory != "."; directory = path.Dir(directory) {
				candidates[directory] = true
			}
		}
	}
	directories := make([]string, 0, len(candidates))
	for directory := range candidates {
		directories = append(directories, directory)
	}
	sort.Slice(directories, func(i, j int) bool { return directories[i] > directories[j] })
	for _, directory := range directories {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := t.removeEmptyDir(directory); err != nil {
			return err
		}
	}
	return nil
}

func (t *fileTransaction) removeEmptyDir(directory string) error {
	file, err := t.root.Open(filepath.FromSlash(directory))
	if err != nil {
		return err
	}
	info, statErr := file.Stat()
	_, readErr := file.ReadDir(1)
	if err := errors.Join(statErr, file.Close()); err != nil {
		return err
	}
	if readErr == nil {
		return nil // Not empty: preserve unrelated content and other manifests.
	}
	if !errors.Is(readErr, io.EOF) {
		return readErr
	}
	if err := t.root.Remove(filepath.FromSlash(directory)); err != nil {
		return err
	}
	t.removedDirs = append(t.removedDirs, directorySnapshot{path: directory, mode: info.Mode().Perm()})
	return nil
}

func (t *fileTransaction) abort(cause error, outputDir string) error {
	if err := t.rollback(); err != nil {
		t.keepBackups = true
		return errors.Join(cause, fmt.Errorf("export rollback failed; recover backups from %q: %w",
			filepath.Join(outputDir, t.scratch), err))
	}
	return fmt.Errorf("export failed; previous files restored: %w", cause)
}

func (t *fileTransaction) rollback() error {
	var rollbackErr error
	for i := len(t.removedDirs) - 1; i >= 0; i-- {
		directory := t.removedDirs[i]
		if err := t.root.Mkdir(filepath.FromSlash(directory.path), directory.mode); err != nil {
			rollbackErr = errors.Join(rollbackErr, err)
			continue
		}
		rollbackErr = errors.Join(rollbackErr, t.root.Chmod(filepath.FromSlash(directory.path), directory.mode))
	}
	for i := len(t.changes) - 1; i >= 0; i-- {
		change := t.changes[i]
		if !change.installed && !change.backedUp {
			continue
		}
		if err := t.checkRollbackChange(i); err != nil {
			rollbackErr = errors.Join(rollbackErr, fmt.Errorf("rollback conflict at %q: %w", change.path, err))
			continue
		}
		if change.installed && !change.backedUp {
			rollbackErr = errors.Join(rollbackErr, t.root.Remove(filepath.FromSlash(change.path)))
		}
		if change.backedUp {
			rollbackErr = errors.Join(rollbackErr, t.root.Rename(t.stagedName("old", i), filepath.FromSlash(change.path)))
		}
	}
	if rollbackErr != nil {
		// Preserve recovery paths when rollback encounters conflicts.
		return rollbackErr
	}
	for i := len(t.createdDirs) - 1; i >= 0; i-- {
		rollbackErr = errors.Join(rollbackErr, t.root.Remove(t.createdDirs[i]))
	}
	return rollbackErr
}

// checkRollbackChange verifies that the destination still matches the transaction's state.
func (t *fileTransaction) checkRollbackChange(index int) error {
	change := t.changes[index]
	for directory := path.Dir(change.path); directory != "."; directory = path.Dir(directory) {
		info, err := t.root.Lstat(filepath.FromSlash(directory))
		if err != nil {
			return err
		}
		if !info.IsDir() {
			return fmt.Errorf("parent %q is no longer a regular directory", directory)
		}
	}
	expected := fileChange{path: change.path, before: change.before}
	var expectedInfo fs.FileInfo
	switch {
	case change.installed:
		expectedInfo = change.installedInfo
		expected.before = &fileSnapshot{data: change.after, mode: expectedInfo.Mode().Perm()}
	case change.remove || change.backupMoved:
		// Expect the deleted or moved destination to be absent.
		expected.before = nil
	default:
		// Check the original left at the destination after a failed installation.
		var err error
		expectedInfo, err = t.root.Lstat(t.stagedName("old", index))
		if err != nil {
			return err
		}
	}
	if err := t.checkUnchanged(&expected); err != nil {
		return err
	}
	if expectedInfo != nil {
		info, err := t.root.Lstat(filepath.FromSlash(change.path))
		if err != nil {
			return err
		}
		if !os.SameFile(expectedInfo, info) || !expectedInfo.ModTime().Equal(info.ModTime()) {
			return fmt.Errorf("destination changed since export")
		}
	}
	return nil
}
