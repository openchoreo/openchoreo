// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package postrendervalidator

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/tools/lint/parser"
)

// activeKindPriority defines which document in a multi-document active file is
// treated as the one to validate. The first matching kind wins.
var activeKindPriority = []string{
	"Component",
	"Workload",
	"ReleaseBinding",
	"Trait",
	"ClusterTrait",
	"ComponentType",
	"ClusterComponentType",
	"Environment",
	"DataPlane",
	"ClusterDataPlane",
	"Project",
}

// projectDocument pairs a parsed document with its source file.
type projectDocument struct {
	file string
	doc  *parser.DocumentNode
}

type componentEntry struct {
	file string
	doc  *parser.DocumentNode
	obj  v1alpha1.Component
}

type componentTypeEntry struct {
	file         string
	doc          *parser.DocumentNode
	kind         string
	name         string
	workloadType string
	obj          v1alpha1.ComponentType
}

type traitEntry struct {
	file string
	doc  *parser.DocumentNode
	kind string
	name string
	obj  v1alpha1.Trait
}

type workloadEntry struct {
	file string
	doc  *parser.DocumentNode
	obj  v1alpha1.Workload
}

type releaseBindingEntry struct {
	file string
	doc  *parser.DocumentNode
	obj  v1alpha1.ReleaseBinding
}

type environmentEntry struct {
	file string
	doc  *parser.DocumentNode
	obj  v1alpha1.Environment
}

type dataPlaneEntry struct {
	file string
	doc  *parser.DocumentNode
	obj  v1alpha1.DataPlane
}

type clusterDataPlaneEntry struct {
	file string
	doc  *parser.DocumentNode
	obj  v1alpha1.ClusterDataPlane
}

// projectIndex groups every decodable resource in the active directory.
type projectIndex struct {
	components        []componentEntry
	componentTypes    []componentTypeEntry
	traits            []traitEntry
	workloads         []workloadEntry
	releaseBindings   []releaseBindingEntry
	environments      []environmentEntry
	dataPlanes        []dataPlaneEntry
	clusterDataPlanes []clusterDataPlaneEntry
}

// selectedProject is the single Component and its related resources that will
// be rendered together.
type selectedProject struct {
	component        componentEntry
	componentType    componentTypeEntry
	traits           []traitEntry
	workload         workloadEntry
	releaseBinding   *releaseBindingEntry
	environment      *environmentEntry
	dataPlane        *dataPlaneEntry
	clusterDataPlane *clusterDataPlaneEntry
}

// ValidateProject renders the resources related to the active document. It
// prefers the Component in the active file, resolves a Component from the
// active Workload/Trait/ReleaseBinding/etc. when the relationship is unique,
// and reports a clear error when the selection is ambiguous. Files are limited
// to the active file's directory, matching `occ linter vali -b`.
func ValidateProject(ctx context.Context, files []string, activeFile string) []Finding {
	activePath := absolutePath(activeFile)
	activeDoc, activeDocs, err := loadActiveDocument(activePath)
	if err != nil {
		return errorFinding(activePath, err)
	}

	docs := make([]projectDocument, 0, len(activeDocs))
	for _, doc := range activeDocs {
		docs = append(docs, projectDocument{file: activePath, doc: doc})
	}
	seenFiles := map[string]bool{activePath: true}
	for _, file := range files {
		path := absolutePath(file)
		if seenFiles[path] {
			continue
		}
		seenFiles[path] = true
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		parsed, err := parser.ParseYAML(data)
		if err != nil {
			continue
		}
		for _, doc := range parsed {
			docs = append(docs, projectDocument{file: path, doc: doc})
		}
	}

	index, decodeErrs := buildProjectIndex(docs)
	if err := decodeErrs[activeDoc]; err != nil {
		return errorFinding(activePath, err)
	}

	component, err := index.resolveActiveComponent(activeDoc)
	if err != nil {
		return errorFinding(activePath, err)
	}

	selected, err := index.selectRelated(component, activeDoc)
	if err != nil {
		return errorFinding(activePath, err)
	}

	selectedDocs := selected.documents()
	bundle, err := BundleFromDocuments(selectedDocs)
	if err != nil {
		return errorFinding(activePath, err)
	}
	if selected.clusterDataPlane != nil {
		bundle.DataPlane = clusterDataPlaneToDataPlane(selected.clusterDataPlane.obj)
	}

	diags, err := ValidateBundle(ctx, bundle)
	if err != nil {
		return errorFinding(activePath, err)
	}

	sources := selected.sources()
	out := make([]Finding, 0, len(diags))
	for _, d := range diags {
		file, pos := resolvePostRenderDiagnostic(d, sources, activePath)
		out = append(out, newFinding(file, pos, d))
	}
	return out
}

// loadActiveDocument parses the active file, returns every document so
// resources declared alongside the active one are indexed, and chooses the
// document to validate. A malformed active file is a hard error, unlike
// malformed neighbouring files which are skipped.
func loadActiveDocument(path string) (*parser.DocumentNode, []*parser.DocumentNode, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, err
	}
	if !parser.HasOpenChoreoAPIVersion(data) {
		return nil, nil, fmt.Errorf("active file %s does not declare an OpenChoreo apiVersion", path)
	}
	docs, err := parser.ParseYAML(data)
	if err != nil {
		return nil, nil, fmt.Errorf("parse active file %s: %w", path, err)
	}
	for _, kind := range activeKindPriority {
		for _, doc := range docs {
			if doc.Kind == kind {
				return doc, docs, nil
			}
		}
	}
	if len(docs) > 0 {
		return docs[0], docs, nil
	}
	return nil, nil, fmt.Errorf("active file %s contains no OpenChoreo documents", path)
}

func absolutePath(path string) string {
	if abs, err := filepath.Abs(path); err == nil {
		return abs
	}
	return filepath.Clean(path)
}

// buildProjectIndex decodes every known resource. Decode failures for
// non-active files are skipped; the active document's failure is surfaced by
// the caller through decodeErrs.
func buildProjectIndex(docs []projectDocument) (*projectIndex, map[*parser.DocumentNode]error) {
	index := &projectIndex{}
	errs := make(map[*parser.DocumentNode]error)

	for _, pd := range docs {
		if pd.doc == nil {
			continue
		}
		switch pd.doc.Kind {
		case "Component":
			obj, err := decodeTyped[v1alpha1.Component](pd.doc)
			if err != nil {
				errs[pd.doc] = fmt.Errorf("decode Component %s: %w", pd.doc.Metadata.Name, err)
				continue
			}
			index.components = append(index.components, componentEntry{file: pd.file, doc: pd.doc, obj: obj})
		case "ComponentType":
			obj, err := decodeTyped[v1alpha1.ComponentType](pd.doc)
			if err != nil {
				errs[pd.doc] = fmt.Errorf("decode ComponentType %s: %w", pd.doc.Metadata.Name, err)
				continue
			}
			index.componentTypes = append(index.componentTypes, componentTypeEntry{
				file:         pd.file,
				doc:          pd.doc,
				kind:         pd.doc.Kind,
				name:         obj.Name,
				workloadType: obj.Spec.WorkloadType,
				obj:          obj,
			})
		case "ClusterComponentType":
			obj, err := decodeTyped[v1alpha1.ClusterComponentType](pd.doc)
			if err != nil {
				errs[pd.doc] = fmt.Errorf("decode ClusterComponentType %s: %w", pd.doc.Metadata.Name, err)
				continue
			}
			converted := v1alpha1.ComponentType{
				TypeMeta:   obj.TypeMeta,
				ObjectMeta: obj.ObjectMeta,
				Spec:       obj.Spec.ToComponentTypeSpec(),
			}
			index.componentTypes = append(index.componentTypes, componentTypeEntry{
				file:         pd.file,
				doc:          pd.doc,
				kind:         pd.doc.Kind,
				name:         obj.Name,
				workloadType: obj.Spec.WorkloadType,
				obj:          converted,
			})
		case "Trait":
			obj, err := decodeTyped[v1alpha1.Trait](pd.doc)
			if err != nil {
				errs[pd.doc] = fmt.Errorf("decode Trait %s: %w", pd.doc.Metadata.Name, err)
				continue
			}
			index.traits = append(index.traits, traitEntry{file: pd.file, doc: pd.doc, kind: pd.doc.Kind, name: obj.Name, obj: obj})
		case "ClusterTrait":
			obj, err := decodeTyped[v1alpha1.ClusterTrait](pd.doc)
			if err != nil {
				// Mirror BundleFromDocuments, which falls back to Trait.
				if legacy, legacyErr := decodeTyped[v1alpha1.Trait](pd.doc); legacyErr != nil {
					errs[pd.doc] = fmt.Errorf("decode ClusterTrait %s: %w", pd.doc.Metadata.Name, err)
					continue
				} else {
					obj = v1alpha1.ClusterTrait{TypeMeta: legacy.TypeMeta, ObjectMeta: legacy.ObjectMeta, Spec: v1alpha1.ClusterTraitSpec(legacy.Spec)}
				}
			}
			converted := v1alpha1.Trait{
				TypeMeta:   obj.TypeMeta,
				ObjectMeta: obj.ObjectMeta,
				Spec:       v1alpha1.TraitSpec(obj.Spec),
			}
			index.traits = append(index.traits, traitEntry{file: pd.file, doc: pd.doc, kind: pd.doc.Kind, name: obj.Name, obj: converted})
		case "Workload":
			obj, err := decodeTyped[v1alpha1.Workload](pd.doc)
			if err != nil {
				errs[pd.doc] = fmt.Errorf("decode Workload %s: %w", pd.doc.Metadata.Name, err)
				continue
			}
			index.workloads = append(index.workloads, workloadEntry{file: pd.file, doc: pd.doc, obj: obj})
		case "ReleaseBinding":
			obj, err := decodeTyped[v1alpha1.ReleaseBinding](pd.doc)
			if err != nil {
				errs[pd.doc] = fmt.Errorf("decode ReleaseBinding %s: %w", pd.doc.Metadata.Name, err)
				continue
			}
			index.releaseBindings = append(index.releaseBindings, releaseBindingEntry{file: pd.file, doc: pd.doc, obj: obj})
		case "Environment":
			obj, err := decodeTyped[v1alpha1.Environment](pd.doc)
			if err != nil {
				errs[pd.doc] = fmt.Errorf("decode Environment %s: %w", pd.doc.Metadata.Name, err)
				continue
			}
			index.environments = append(index.environments, environmentEntry{file: pd.file, doc: pd.doc, obj: obj})
		case "DataPlane":
			obj, err := decodeTyped[v1alpha1.DataPlane](pd.doc)
			if err != nil {
				errs[pd.doc] = fmt.Errorf("decode DataPlane %s: %w", pd.doc.Metadata.Name, err)
				continue
			}
			index.dataPlanes = append(index.dataPlanes, dataPlaneEntry{file: pd.file, doc: pd.doc, obj: obj})
		case "ClusterDataPlane":
			obj, err := decodeTyped[v1alpha1.ClusterDataPlane](pd.doc)
			if err != nil {
				errs[pd.doc] = fmt.Errorf("decode ClusterDataPlane %s: %w", pd.doc.Metadata.Name, err)
				continue
			}
			index.clusterDataPlanes = append(index.clusterDataPlanes, clusterDataPlaneEntry{file: pd.file, doc: pd.doc, obj: obj})
		}
	}
	return index, errs
}

func (idx *projectIndex) resolveActiveComponent(active *parser.DocumentNode) (componentEntry, error) {
	switch active.Kind {
	case "Component":
		if c := idx.componentByDoc(active); c != nil {
			return *c, nil
		}
		return componentEntry{}, fmt.Errorf("active Component %q could not be decoded", active.Metadata.Name)
	case "Workload":
		w := idx.workloadByDoc(active)
		if w == nil {
			return componentEntry{}, fmt.Errorf("active Workload %q could not be decoded", active.Metadata.Name)
		}
		return oneComponent(idx.componentsByOwner(w.obj.Spec.Owner.ProjectName, w.obj.Spec.Owner.ComponentName),
			fmt.Sprintf("Workload %s/%s", w.obj.Spec.Owner.ProjectName, w.obj.Spec.Owner.ComponentName))
	case "ReleaseBinding":
		rb := idx.releaseBindingByDoc(active)
		if rb == nil {
			return componentEntry{}, fmt.Errorf("active ReleaseBinding %q could not be decoded", active.Metadata.Name)
		}
		return oneComponent(idx.componentsByOwner(rb.obj.Spec.Owner.ProjectName, rb.obj.Spec.Owner.ComponentName),
			fmt.Sprintf("ReleaseBinding %s", active.Metadata.Name))
	case "Trait", "ClusterTrait":
		if idx.traitByDoc(active) == nil {
			return componentEntry{}, fmt.Errorf("active %s %q could not be decoded", active.Kind, active.Metadata.Name)
		}
		return oneComponent(idx.componentsForTrait(active.Kind, active.Metadata.Name),
			fmt.Sprintf("%s %s", active.Kind, active.Metadata.Name))
	case "ComponentType", "ClusterComponentType":
		ct := idx.componentTypeByDoc(active)
		if ct == nil {
			return componentEntry{}, fmt.Errorf("active %s %q could not be decoded", active.Kind, active.Metadata.Name)
		}
		return oneComponent(idx.componentsForComponentType(ct.kind, ct.workloadType, ct.name),
			fmt.Sprintf("%s %s", active.Kind, active.Metadata.Name))
	case "Environment":
		env := idx.environmentByDoc(active)
		if env == nil {
			return componentEntry{}, fmt.Errorf("active Environment %q could not be decoded", active.Metadata.Name)
		}
		return oneComponent(idx.componentsForEnvironment(env.obj.Name),
			fmt.Sprintf("Environment %s", env.obj.Name))
	case "DataPlane", "ClusterDataPlane":
		return oneComponent(idx.componentsForDataPlane(active.Kind, active.Metadata.Name),
			fmt.Sprintf("%s %s", active.Kind, active.Metadata.Name))
	case "Project":
		return oneComponent(idx.componentsByProject(active.Metadata.Name),
			fmt.Sprintf("Project %s", active.Metadata.Name))
	default:
		return componentEntry{}, fmt.Errorf(
			"active document %s/%s cannot be resolved to a Component; open a Component, Workload, Trait, ReleaseBinding, ComponentType, Environment, DataPlane, or Project",
			active.Kind, active.Metadata.Name)
	}
}

func (idx *projectIndex) selectRelated(component componentEntry, active *parser.DocumentNode) (selectedProject, error) {
	selected := selectedProject{component: component}

	ct, err := idx.selectComponentType(component)
	if err != nil {
		return selectedProject{}, err
	}
	selected.componentType = ct

	seenTraits := make(map[string]bool)
	for _, ref := range component.obj.Spec.Traits {
		kind := string(ref.Kind)
		if kind == "" {
			kind = "Trait"
		}
		key := kind + "\x00" + ref.Name
		if seenTraits[key] {
			continue
		}
		seenTraits[key] = true
		trait, err := idx.selectTrait(kind, ref.Name)
		if err != nil {
			return selectedProject{}, locateError(component.doc, component.file, "traits", err)
		}
		selected.traits = append(selected.traits, trait)
	}

	workload, err := idx.selectWorkload(component)
	if err != nil {
		return selectedProject{}, err
	}
	selected.workload = workload

	rb, err := idx.selectReleaseBinding(component, active)
	if err != nil {
		return selectedProject{}, err
	}
	if rb != nil {
		selected.releaseBinding = rb
		env, err := idx.selectEnvironment(*rb)
		if err != nil {
			return selectedProject{}, err
		}
		if env != nil {
			selected.environment = env
			dp, cdp, err := idx.selectDataPlane(*env)
			if err != nil {
				return selectedProject{}, err
			}
			selected.dataPlane = dp
			selected.clusterDataPlane = cdp
		}
	}

	return selected, nil
}

func (idx *projectIndex) selectComponentType(component componentEntry) (componentTypeEntry, error) {
	refKind := string(component.obj.Spec.ComponentType.Kind)
	if refKind == "" {
		refKind = "ComponentType"
	}
	refName := component.obj.Spec.ComponentType.Name
	workloadType, name, hasWorkloadType := strings.Cut(refName, "/")
	if !hasWorkloadType {
		name = refName
		workloadType = ""
	}

	var matches []componentTypeEntry
	for _, ct := range idx.componentTypes {
		if ct.kind != refKind || ct.name != name {
			continue
		}
		if workloadType != "" && ct.workloadType != workloadType {
			continue
		}
		matches = append(matches, ct)
	}
	if len(matches) == 0 {
		return componentTypeEntry{}, locateError(component.doc, component.file, "componentType", fmt.Errorf(
			"Component %s references %s %q, but it was not found in the active directory",
			component.obj.Name, refKind, refName))
	}
	if len(matches) > 1 {
		return componentTypeEntry{}, locateError(component.doc, component.file, "componentType", fmt.Errorf(
			"multiple %s resources named %q match Component %s; keep only one in the active directory",
			refKind, name, component.obj.Name))
	}
	return matches[0], nil
}

func (idx *projectIndex) selectTrait(kind, name string) (traitEntry, error) {
	var matches []traitEntry
	for _, trait := range idx.traits {
		if trait.kind == kind && trait.name == name {
			matches = append(matches, trait)
		}
	}
	if len(matches) == 0 {
		return traitEntry{}, fmt.Errorf("%s %q referenced by the Component was not found in the active directory", kind, name)
	}
	if len(matches) > 1 {
		return traitEntry{}, fmt.Errorf("multiple %s resources named %q match the Component; keep only one in the active directory", kind, name)
	}
	return matches[0], nil
}

func (idx *projectIndex) selectWorkload(component componentEntry) (workloadEntry, error) {
	project := component.obj.Spec.Owner.ProjectName
	name := component.obj.Name
	var matches []workloadEntry
	for _, workload := range idx.workloads {
		if workload.obj.Spec.Owner.ComponentName != name {
			continue
		}
		if project != "" && workload.obj.Spec.Owner.ProjectName != project {
			continue
		}
		matches = append(matches, workload)
	}
	if len(matches) == 0 {
		return workloadEntry{}, locateError(component.doc, component.file, "", fmt.Errorf("no Workload for Component %s/%s was found in the active directory", project, name))
	}
	if len(matches) > 1 {
		return workloadEntry{}, locateError(component.doc, component.file, "", fmt.Errorf("multiple Workloads match Component %s/%s; keep only one in the active directory", project, name))
	}
	return matches[0], nil
}

func (idx *projectIndex) selectReleaseBinding(component componentEntry, active *parser.DocumentNode) (*releaseBindingEntry, error) {
	candidates := idx.releaseBindingsForComponent(component)
	if active.Kind == "ReleaseBinding" {
		for i := range candidates {
			if candidates[i].doc == active {
				return &candidates[i], nil
			}
		}
		return nil, fmt.Errorf("active ReleaseBinding %s does not match Component %s/%s",
			active.Metadata.Name, component.obj.Spec.Owner.ProjectName, component.obj.Name)
	}
	if len(candidates) == 0 {
		return nil, nil
	}
	if len(candidates) == 1 {
		return &candidates[0], nil
	}
	if active.Kind == "Environment" {
		var matching []releaseBindingEntry
		for _, rb := range candidates {
			if rb.obj.Spec.Environment == active.Metadata.Name {
				matching = append(matching, rb)
			}
		}
		if len(matching) == 1 {
			return &matching[0], nil
		}
	}
	return nil, locateError(component.doc, component.file, "", fmt.Errorf(
		"multiple ReleaseBindings match Component %s/%s (%s); open the intended ReleaseBinding and run again",
		component.obj.Spec.Owner.ProjectName, component.obj.Name, describeReleaseBindings(candidates)))
}

func (idx *projectIndex) selectEnvironment(rb releaseBindingEntry) (*environmentEntry, error) {
	var matches []environmentEntry
	for _, env := range idx.environments {
		if env.obj.Name == rb.obj.Spec.Environment {
			matches = append(matches, env)
		}
	}
	if len(matches) == 0 {
		return nil, locateError(rb.doc, rb.file, "environment", fmt.Errorf("ReleaseBinding %s references Environment %q, but it was not found in the active directory",
			rb.obj.Name, rb.obj.Spec.Environment))
	}
	if len(matches) > 1 {
		return nil, locateError(rb.doc, rb.file, "environment", fmt.Errorf("multiple Environments named %q match ReleaseBinding %s; keep only one in the active directory",
			rb.obj.Spec.Environment, rb.obj.Name))
	}
	return &matches[0], nil
}

func (idx *projectIndex) selectDataPlane(env environmentEntry) (*dataPlaneEntry, *clusterDataPlaneEntry, error) {
	ref := env.obj.Spec.DataPlaneRef
	if ref == nil {
		return nil, nil, nil
	}
	kind := string(ref.Kind)
	if kind == "" {
		kind = "DataPlane"
	}
	switch kind {
	case "ClusterDataPlane":
		var matches []clusterDataPlaneEntry
		for _, cdp := range idx.clusterDataPlanes {
			if cdp.obj.Name == ref.Name {
				matches = append(matches, cdp)
			}
		}
		if len(matches) == 0 {
			return nil, nil, locateError(env.doc, env.file, "dataPlaneRef", fmt.Errorf("Environment %s references ClusterDataPlane %q, but it was not found in the active directory", env.obj.Name, ref.Name))
		}
		if len(matches) > 1 {
			return nil, nil, locateError(env.doc, env.file, "dataPlaneRef", fmt.Errorf("multiple ClusterDataPlanes named %q match Environment %s; keep only one in the active directory", ref.Name, env.obj.Name))
		}
		return nil, &matches[0], nil
	case "DataPlane":
		var matches []dataPlaneEntry
		for _, dp := range idx.dataPlanes {
			if dp.obj.Name == ref.Name {
				matches = append(matches, dp)
			}
		}
		if len(matches) == 0 {
			return nil, nil, locateError(env.doc, env.file, "dataPlaneRef", fmt.Errorf("Environment %s references DataPlane %q, but it was not found in the active directory", env.obj.Name, ref.Name))
		}
		if len(matches) > 1 {
			return nil, nil, locateError(env.doc, env.file, "dataPlaneRef", fmt.Errorf("multiple DataPlanes named %q match Environment %s; keep only one in the active directory", ref.Name, env.obj.Name))
		}
		return &matches[0], nil, nil
	default:
		return nil, nil, locateError(env.doc, env.file, "dataPlaneRef", fmt.Errorf("Environment %s references unsupported data plane kind %q", env.obj.Name, kind))
	}
}

func (idx *projectIndex) componentsByOwner(project, name string) []componentEntry {
	var out []componentEntry
	for _, component := range idx.components {
		if component.obj.Name != name {
			continue
		}
		if project != "" && component.obj.Spec.Owner.ProjectName != project {
			continue
		}
		out = append(out, component)
	}
	return dedupeComponents(out)
}

func (idx *projectIndex) componentsByProject(project string) []componentEntry {
	var out []componentEntry
	for _, component := range idx.components {
		if component.obj.Spec.Owner.ProjectName == project {
			out = append(out, component)
		}
	}
	return dedupeComponents(out)
}

func (idx *projectIndex) componentsForTrait(kind, name string) []componentEntry {
	var out []componentEntry
	for _, component := range idx.components {
		for _, ref := range component.obj.Spec.Traits {
			refKind := string(ref.Kind)
			if refKind == "" {
				refKind = "Trait"
			}
			if refKind == kind && ref.Name == name {
				out = append(out, component)
				break
			}
		}
	}
	return dedupeComponents(out)
}

func (idx *projectIndex) componentsForComponentType(kind, workloadType, name string) []componentEntry {
	var out []componentEntry
	for _, component := range idx.components {
		refKind := string(component.obj.Spec.ComponentType.Kind)
		if refKind == "" {
			refKind = "ComponentType"
		}
		if refKind != kind {
			continue
		}
		refWorkloadType, refName, ok := strings.Cut(component.obj.Spec.ComponentType.Name, "/")
		if !ok {
			refName = component.obj.Spec.ComponentType.Name
			refWorkloadType = ""
		}
		if refName != name {
			continue
		}
		if refWorkloadType != "" && workloadType != "" && refWorkloadType != workloadType {
			continue
		}
		out = append(out, component)
	}
	return dedupeComponents(out)
}

func (idx *projectIndex) componentsForEnvironment(name string) []componentEntry {
	var out []componentEntry
	for _, rb := range idx.releaseBindings {
		if rb.obj.Spec.Environment != name {
			continue
		}
		out = append(out, idx.componentsByOwner(rb.obj.Spec.Owner.ProjectName, rb.obj.Spec.Owner.ComponentName)...)
	}
	return dedupeComponents(out)
}

func (idx *projectIndex) componentsForDataPlane(kind, name string) []componentEntry {
	var out []componentEntry
	for _, env := range idx.environments {
		ref := env.obj.Spec.DataPlaneRef
		if ref == nil {
			continue
		}
		refKind := string(ref.Kind)
		if refKind == "" {
			refKind = "DataPlane"
		}
		if refKind != kind || ref.Name != name {
			continue
		}
		for _, rb := range idx.releaseBindings {
			if rb.obj.Spec.Environment != env.obj.Name {
				continue
			}
			out = append(out, idx.componentsByOwner(rb.obj.Spec.Owner.ProjectName, rb.obj.Spec.Owner.ComponentName)...)
		}
	}
	return dedupeComponents(out)
}

func (idx *projectIndex) releaseBindingsForComponent(component componentEntry) []releaseBindingEntry {
	project := component.obj.Spec.Owner.ProjectName
	name := component.obj.Name
	var out []releaseBindingEntry
	for _, rb := range idx.releaseBindings {
		if rb.obj.Spec.Owner.ProjectName != project || rb.obj.Spec.Owner.ComponentName != name {
			continue
		}
		out = append(out, rb)
	}
	return out
}

func (idx *projectIndex) componentByDoc(doc *parser.DocumentNode) *componentEntry {
	for i := range idx.components {
		if idx.components[i].doc == doc {
			return &idx.components[i]
		}
	}
	return nil
}

func (idx *projectIndex) componentTypeByDoc(doc *parser.DocumentNode) *componentTypeEntry {
	for i := range idx.componentTypes {
		if idx.componentTypes[i].doc == doc {
			return &idx.componentTypes[i]
		}
	}
	return nil
}

func (idx *projectIndex) traitByDoc(doc *parser.DocumentNode) *traitEntry {
	for i := range idx.traits {
		if idx.traits[i].doc == doc {
			return &idx.traits[i]
		}
	}
	return nil
}

func (idx *projectIndex) workloadByDoc(doc *parser.DocumentNode) *workloadEntry {
	for i := range idx.workloads {
		if idx.workloads[i].doc == doc {
			return &idx.workloads[i]
		}
	}
	return nil
}

func (idx *projectIndex) releaseBindingByDoc(doc *parser.DocumentNode) *releaseBindingEntry {
	for i := range idx.releaseBindings {
		if idx.releaseBindings[i].doc == doc {
			return &idx.releaseBindings[i]
		}
	}
	return nil
}

func (idx *projectIndex) environmentByDoc(doc *parser.DocumentNode) *environmentEntry {
	for i := range idx.environments {
		if idx.environments[i].doc == doc {
			return &idx.environments[i]
		}
	}
	return nil
}

func dedupeComponents(components []componentEntry) []componentEntry {
	seen := make(map[string]bool)
	out := make([]componentEntry, 0, len(components))
	for _, component := range components {
		key := component.file + "\x00" + component.obj.Spec.Owner.ProjectName + "\x00" + component.obj.Name
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, component)
	}
	return out
}

func oneComponent(components []componentEntry, label string) (componentEntry, error) {
	if len(components) == 0 {
		return componentEntry{}, fmt.Errorf("no Component matches %s in the active directory", label)
	}
	if len(components) > 1 {
		return componentEntry{}, fmt.Errorf(
			"multiple Components match %s (%s); open the intended Component file",
			label, describeComponents(components))
	}
	return components[0], nil
}

func describeComponents(components []componentEntry) string {
	parts := make([]string, 0, len(components))
	for _, component := range components {
		parts = append(parts, fmt.Sprintf("%s/%s in %s",
			component.obj.Spec.Owner.ProjectName, component.obj.Name, filepath.Base(component.file)))
	}
	return strings.Join(parts, ", ")
}

func describeReleaseBindings(bindings []releaseBindingEntry) string {
	parts := make([]string, 0, len(bindings))
	for _, rb := range bindings {
		parts = append(parts, fmt.Sprintf("%s (environment %s)", rb.obj.Name, rb.obj.Spec.Environment))
	}
	return strings.Join(parts, ", ")
}

func (s selectedProject) documents() []*parser.DocumentNode {
	return s.sourceDocs()
}

func (s selectedProject) sources() []postRenderSource {
	docs := s.sourceDocs()
	sources := make([]postRenderSource, 0, len(docs))
	for _, doc := range docs {
		sources = append(sources, postRenderSource{doc: doc, file: s.fileForDoc(doc)})
	}
	return sources
}

func (s selectedProject) sourceDocs() []*parser.DocumentNode {
	docs := []*parser.DocumentNode{s.component.doc, s.componentType.doc}
	for _, trait := range s.traits {
		docs = append(docs, trait.doc)
	}
	docs = append(docs, s.workload.doc)
	if s.releaseBinding != nil {
		docs = append(docs, s.releaseBinding.doc)
	}
	if s.environment != nil {
		docs = append(docs, s.environment.doc)
	}
	if s.dataPlane != nil {
		docs = append(docs, s.dataPlane.doc)
	}
	if s.clusterDataPlane != nil {
		docs = append(docs, s.clusterDataPlane.doc)
	}
	seen := make(map[*parser.DocumentNode]bool, len(docs))
	out := make([]*parser.DocumentNode, 0, len(docs))
	for _, doc := range docs {
		if seen[doc] {
			continue
		}
		seen[doc] = true
		out = append(out, doc)
	}
	return out
}

func (s selectedProject) fileForDoc(doc *parser.DocumentNode) string {
	if doc == s.component.doc {
		return s.component.file
	}
	if doc == s.componentType.doc {
		return s.componentType.file
	}
	for _, trait := range s.traits {
		if doc == trait.doc {
			return trait.file
		}
	}
	if doc == s.workload.doc {
		return s.workload.file
	}
	if s.releaseBinding != nil && doc == s.releaseBinding.doc {
		return s.releaseBinding.file
	}
	if s.environment != nil && doc == s.environment.doc {
		return s.environment.file
	}
	if s.dataPlane != nil && doc == s.dataPlane.doc {
		return s.dataPlane.file
	}
	if s.clusterDataPlane != nil && doc == s.clusterDataPlane.doc {
		return s.clusterDataPlane.file
	}
	return ""
}

// clusterDataPlaneToDataPlane adapts a cluster-scoped data plane to the
// namespaced type the rendering pipeline expects. The LSP uses this locally so
// the CLI's shared BundleFromDocuments behaviour stays unchanged.
func clusterDataPlaneToDataPlane(cdp v1alpha1.ClusterDataPlane) *v1alpha1.DataPlane {
	typeMeta := cdp.TypeMeta
	typeMeta.Kind = "DataPlane"
	return &v1alpha1.DataPlane{
		TypeMeta:   typeMeta,
		ObjectMeta: cdp.ObjectMeta,
		Spec: v1alpha1.DataPlaneSpec{
			PlaneID:        cdp.Spec.PlaneID,
			ClusterAgent:   cdp.Spec.ClusterAgent,
			Gateway:        cdp.Spec.Gateway,
			SecretStoreRef: cdp.Spec.SecretStoreRef,
		},
	}
}
