// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package postrendervalidator

import (
	"fmt"
	"strings"

	"github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/tools/lint/parser"
)

// componentBundle is one Component together with the resources it references,
// and the source mapping needed to attribute its diagnostics.
type componentBundle struct {
	bundle   *Bundle
	sources  []postRenderSource
	fallback string
	err      error
	errPath  string
}

// bundlesFromSources groups documents into one bundle per Component. The CLI
// scans a whole project, so validating every document as a single bundle would
// pair a Component with an unrelated ComponentType and produce false findings.
func bundlesFromSources(sources []postRenderSource) []componentBundle {
	fileByDoc := make(map[*parser.DocumentNode]string, len(sources))
	projectDocs := make([]projectDocument, 0, len(sources))
	for _, source := range sources {
		fileByDoc[source.doc] = source.file
		projectDocs = append(projectDocs, projectDocument{file: source.file, doc: source.doc})
	}

	index, _ := buildProjectIndex(projectDocs)
	bundles := make([]componentBundle, 0, len(index.components))
	for _, component := range index.components {
		bundles = append(bundles, bundleForComponent(index, component, fileByDoc))
	}
	return bundles
}

func bundleForComponent(
	index *projectIndex,
	component componentEntry,
	fileByDoc map[*parser.DocumentNode]string,
) componentBundle {
	bundle := &Bundle{
		Component: &component.obj,
		Traits:    make([]v1alpha1.Trait, 0, len(component.obj.Spec.Traits)),
	}
	docs := []*parser.DocumentNode{component.doc}

	ct := index.matchComponentType(component)
	if ct == nil {
		return componentBundle{
			fallback: component.file,
			err: locateError(component.doc, component.file, "componentType", fmt.Errorf("Component %s references %s %q, but it was not found",
				component.obj.Name, componentTypeKind(component), component.obj.Spec.ComponentType.Name)),
			errPath: component.file,
		}
	}
	bundle.ComponentType = &ct.obj
	docs = append(docs, ct.doc)

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
		if trait := index.matchTrait(kind, ref.Name); trait != nil {
			bundle.Traits = append(bundle.Traits, trait.obj)
			docs = append(docs, trait.doc)
		}
	}

	workload := index.matchWorkload(component)
	if workload == nil {
		return componentBundle{
			fallback: component.file,
			err: locateError(component.doc, component.file, "", fmt.Errorf("no Workload for Component %s/%s was found",
				component.obj.Spec.Owner.ProjectName, component.obj.Name)),
			errPath: component.file,
		}
	}
	bundle.Workload = &workload.obj
	docs = append(docs, workload.doc)

	if rb := index.matchReleaseBinding(component); rb != nil {
		bundle.ReleaseBinding = &rb.obj
		docs = append(docs, rb.doc)
		env := index.matchEnvironment(rb.obj.Spec.Environment)
		if env == nil {
			return componentBundle{
				fallback: component.file,
				err: locateError(rb.doc, rb.file, "environment", fmt.Errorf("ReleaseBinding %s references Environment %q, but it was not found",
					rb.obj.Name, rb.obj.Spec.Environment)),
				errPath: rb.file,
			}
		}
		bundle.Environment = &env.obj
		docs = append(docs, env.doc)
		if ref := env.obj.Spec.DataPlaneRef; ref != nil {
			dp, cdp := index.matchDataPlane(env)
			if dp == nil && cdp == nil {
				return componentBundle{
					fallback: component.file,
					err: locateError(env.doc, env.file, "dataPlaneRef", fmt.Errorf("Environment %s references %s %q, but it was not found",
						env.obj.Name, dataPlaneKind(env), ref.Name)),
					errPath: env.file,
				}
			}
			if cdp != nil {
				bundle.DataPlane = clusterDataPlaneToDataPlane(cdp.obj)
				docs = append(docs, cdp.doc)
			} else {
				bundle.DataPlane = &dp.obj
				docs = append(docs, dp.doc)
			}
		}
	}

	return componentBundle{
		bundle:   bundle,
		sources:  bundleSources(docs, fileByDoc, component.file),
		fallback: component.file,
	}
}

// bundleSources deduplicates the documents in a bundle and attaches each to
// its file. The component file is used only as a safe fallback for documents
// whose source was not recorded.
func bundleSources(
	docs []*parser.DocumentNode,
	fileByDoc map[*parser.DocumentNode]string,
	fallback string,
) []postRenderSource {
	seen := make(map[*parser.DocumentNode]bool, len(docs))
	out := make([]postRenderSource, 0, len(docs))
	for _, doc := range docs {
		if doc == nil || seen[doc] {
			continue
		}
		seen[doc] = true
		file := fileByDoc[doc]
		if file == "" {
			file = fallback
		}
		out = append(out, postRenderSource{doc: doc, file: file})
	}
	return out
}

func componentTypeKind(component componentEntry) string {
	kind := string(component.obj.Spec.ComponentType.Kind)
	if kind == "" {
		return "ComponentType"
	}
	return kind
}

func dataPlaneKind(env *environmentEntry) string {
	if env == nil || env.obj.Spec.DataPlaneRef == nil {
		return "DataPlane"
	}
	kind := string(env.obj.Spec.DataPlaneRef.Kind)
	if kind == "" {
		return "DataPlane"
	}
	return kind
}

func (idx *projectIndex) matchComponentType(component componentEntry) *componentTypeEntry {
	refKind := componentTypeKind(component)
	workloadType, name, hasWorkloadType := strings.Cut(component.obj.Spec.ComponentType.Name, "/")
	if !hasWorkloadType {
		name = workloadType
		workloadType = ""
	}
	for i := range idx.componentTypes {
		ct := &idx.componentTypes[i]
		if ct.kind != refKind || ct.name != name {
			continue
		}
		if workloadType != "" && ct.workloadType != workloadType {
			continue
		}
		return ct
	}
	return nil
}

func (idx *projectIndex) matchTrait(kind, name string) *traitEntry {
	for i := range idx.traits {
		if idx.traits[i].kind == kind && idx.traits[i].name == name {
			return &idx.traits[i]
		}
	}
	return nil
}

func (idx *projectIndex) matchWorkload(component componentEntry) *workloadEntry {
	project := component.obj.Spec.Owner.ProjectName
	name := component.obj.Name
	for i := range idx.workloads {
		workload := &idx.workloads[i]
		if workload.obj.Spec.Owner.ComponentName != name {
			continue
		}
		if project != "" && workload.obj.Spec.Owner.ProjectName != project {
			continue
		}
		return workload
	}
	return nil
}

func (idx *projectIndex) matchReleaseBinding(component componentEntry) *releaseBindingEntry {
	bindings := idx.releaseBindingsForComponent(component)
	if len(bindings) == 0 {
		return nil
	}
	return &bindings[0]
}

func (idx *projectIndex) matchEnvironment(name string) *environmentEntry {
	for i := range idx.environments {
		if idx.environments[i].obj.Name == name {
			return &idx.environments[i]
		}
	}
	return nil
}

func (idx *projectIndex) matchDataPlane(env *environmentEntry) (*dataPlaneEntry, *clusterDataPlaneEntry) {
	if env == nil || env.obj.Spec.DataPlaneRef == nil {
		return nil, nil
	}
	ref := env.obj.Spec.DataPlaneRef
	switch dataPlaneKind(env) {
	case "ClusterDataPlane":
		for i := range idx.clusterDataPlanes {
			if idx.clusterDataPlanes[i].obj.Name == ref.Name {
				return nil, &idx.clusterDataPlanes[i]
			}
		}
	case "DataPlane":
		for i := range idx.dataPlanes {
			if idx.dataPlanes[i].obj.Name == ref.Name {
				return &idx.dataPlanes[i], nil
			}
		}
	}
	return nil, nil
}
