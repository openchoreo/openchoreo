// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package generator

import (
	"fmt"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/componentrelease"
	"github.com/openchoreo/openchoreo/internal/occ/fsmode"
	componentvalidation "github.com/openchoreo/openchoreo/internal/validation/component"
)

const (
	componentReleaseAPIVersion = "openchoreo.dev/v1alpha1"
	componentReleaseKind       = "ComponentRelease"
)

// ReleaseGenerator generates ComponentRelease resources
type ReleaseGenerator struct {
	index *fsmode.Index
}

// NewReleaseGenerator creates a new release generator
func NewReleaseGenerator(index *fsmode.Index) *ReleaseGenerator {
	return &ReleaseGenerator{index: index}
}

// ReleaseOptions configures release generation
type ReleaseOptions struct {
	ComponentName string
	ProjectName   string
	Namespace     string
	ReleaseName   string    // Optional: custom release name (if empty, auto-generated from component, date, version)
	Version       string    // Optional: auto-generated if empty
	Date          time.Time // Optional: uses current date if zero
}

// GenerateRelease generates a ComponentRelease for the specified component.
// It resolves the component, its ComponentType (or ClusterComponentType), the referenced
// traits, and the workload, then delegates spec assembly to componentrelease.BuildSpec so
// that filesystem mode produces the same ComponentReleaseSpec as the controller and API server.
func (g *ReleaseGenerator) GenerateRelease(opts ReleaseOptions) (*unstructured.Unstructured, error) {
	comp, err := g.index.GetTypedComponent(opts.Namespace, opts.ComponentName)
	if err != nil {
		return nil, err
	}

	// Validate project name matches if specified
	if opts.ProjectName != "" && comp.ProjectName() != opts.ProjectName {
		return nil, fmt.Errorf("component %q belongs to project %q, not %q",
			opts.ComponentName, comp.ProjectName(), opts.ProjectName)
	}

	// Resolve the ComponentType (or ClusterComponentType) spec snapshot.
	ctKind := string(comp.Spec.ComponentType.Kind)
	if ctKind == "" {
		ctKind = string(v1alpha1.ComponentTypeRefKindComponentType)
	}
	typeName := comp.ComponentTypeName()

	var ctSpec v1alpha1.ComponentTypeSpec
	switch ctKind {
	case string(v1alpha1.ComponentTypeRefKindComponentType):
		ct, err := g.index.GetTypedComponentType(opts.Namespace, typeName)
		if err != nil {
			return nil, fmt.Errorf("component type %q not found (referenced by component %q): %w",
				typeName, opts.ComponentName, err)
		}
		ctSpec = ct.Spec
	case string(v1alpha1.ComponentTypeRefKindClusterComponentType):
		cct, err := g.index.GetTypedClusterComponentType(typeName)
		if err != nil {
			return nil, fmt.Errorf("cluster component type %q not found (referenced by component %q): %w",
				typeName, opts.ComponentName, err)
		}
		ctSpec = cct.Spec.ToComponentTypeSpec()
	default:
		return nil, fmt.Errorf("unsupported component type kind %q for component %q", ctKind, opts.ComponentName)
	}

	// Trait kind defaults (Trait / ClusterTrait) are applied by the API server at admission time.
	// fsmode loads raw YAML, so an omitted kind reaches us as "". Default them here — before
	// validation, gatherTraits, and BuildSpec — so lookups route correctly, allowedTraits
	// validation compares like-for-like, and the frozen spec matches control-plane-generated
	// releases byte-for-byte.
	defaultTraitKinds(&ctSpec, comp.Component, ctKind)

	// Enforce the same pre-build validation the controller and API server run: component-level
	// traits must be permitted by allowedTraits, and trait instance names must be unique across
	// embedded and component-level traits.
	if err := componentvalidation.ValidateAllowedTraits(comp.Spec.Traits, ctSpec.AllowedTraits); err != nil {
		return nil, fmt.Errorf("component %q trait validation failed: %w", opts.ComponentName, err)
	}
	if err := componentvalidation.ValidateTraitInstanceNameUniqueness(comp.Spec.Traits, ctSpec.Traits); err != nil {
		return nil, fmt.Errorf("component %q trait validation failed: %w", opts.ComponentName, err)
	}

	// Gather every Trait/ClusterTrait referenced by both the embedded ComponentType traits
	// and the component-level traits so BuildSpec can freeze their specs.
	traits, clusterTraits, err := g.gatherTraits(&ctSpec, comp.Component, opts.Namespace)
	if err != nil {
		return nil, err
	}

	// Fetch the workload that carries the built container image.
	wl, err := g.index.GetTypedWorkloadForComponent(opts.Namespace, comp.ProjectName(), comp.Name)
	if err != nil {
		return nil, err
	}

	// Determine release name.
	releaseName := opts.ReleaseName
	if releaseName == "" {
		releaseName, err = GenerateReleaseName(comp.Name, opts.Date, opts.Version, opts.Namespace, comp.ProjectName(), g.index)
		if err != nil {
			return nil, fmt.Errorf("failed to generate release name: %w", err)
		}
	}

	crSpec, err := componentrelease.BuildSpec(componentrelease.BuildInput{
		Component: comp.Component,
		ComponentType: v1alpha1.ComponentReleaseComponentType{
			Kind: v1alpha1.ComponentTypeRefKind(ctKind),
			Name: comp.Spec.ComponentType.Name,
			Spec: ctSpec,
		},
		Traits:        traits,
		ClusterTraits: clusterTraits,
		Workload:      &wl.Spec.WorkloadTemplateSpec,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to build component release spec: %w", err)
	}

	return toUnstructuredRelease(releaseName, opts.Namespace, crSpec)
}

// defaultTraitKinds fills in the kind of any trait reference that omitted it in the source file,
// mirroring the kubebuilder defaults the API server applies at admission. fsmode loads raw YAML,
// so an omitted kind reaches us as "".
//
//   - Embedded ComponentType traits (ctSpec.Traits) and allowedTraits (ctSpec.AllowedTraits)
//     default to Trait for a ComponentType source and ClusterTrait for a ClusterComponentType
//     source, matching the TraitRef / ClusterTraitRef admission defaults for that scope.
//   - Component-level traits (comp.Spec.Traits) always default to Trait — a component references
//     namespace-scoped Traits by default regardless of its ComponentType's scope (ComponentTrait
//     kubebuilder default).
//
// Defaulting allowedTraits and the component-level trait kinds together keeps ValidateAllowedTraits
// symmetric (it compares on kind:name, so a raw "" on one side and a defaulted "Trait" on the other
// would falsely reject an allowed trait), and freezes allowedTraits into the release spec with the
// same concrete kinds the controller and API server emit. ctSpec is a fresh per-call conversion and
// comp is a fresh FromUnstructured decode, so mutating them in place does not touch the cached
// index entries.
func defaultTraitKinds(ctSpec *v1alpha1.ComponentTypeSpec, comp *v1alpha1.Component, ctKind string) {
	ctDefaultKind := v1alpha1.TraitRefKindTrait
	if ctKind == string(v1alpha1.ComponentTypeRefKindClusterComponentType) {
		ctDefaultKind = v1alpha1.TraitRefKindClusterTrait
	}
	for i := range ctSpec.Traits {
		if ctSpec.Traits[i].Kind == "" {
			ctSpec.Traits[i].Kind = ctDefaultKind
		}
	}
	for i := range ctSpec.AllowedTraits {
		if ctSpec.AllowedTraits[i].Kind == "" {
			ctSpec.AllowedTraits[i].Kind = ctDefaultKind
		}
	}
	for i := range comp.Spec.Traits {
		if comp.Spec.Traits[i].Kind == "" {
			comp.Spec.Traits[i].Kind = v1alpha1.TraitRefKindTrait
		}
	}
}

// gatherTraits resolves all unique Trait/ClusterTrait specs referenced by the embedded
// ComponentType traits and the component-level traits, deduplicating by name within each kind.
// It mirrors the controller's fetchAllTraits so BuildSpec sees the same inputs across producers.
func (g *ReleaseGenerator) gatherTraits(
	ctSpec *v1alpha1.ComponentTypeSpec,
	comp *v1alpha1.Component,
	namespace string,
) (map[string]v1alpha1.TraitSpec, map[string]v1alpha1.ClusterTraitSpec, error) {
	traits := make(map[string]v1alpha1.TraitSpec)
	clusterTraits := make(map[string]v1alpha1.ClusterTraitSpec)

	addTrait := func(kind v1alpha1.TraitRefKind, name string) error {
		switch kind {
		case v1alpha1.TraitRefKindClusterTrait:
			if _, exists := clusterTraits[name]; exists {
				return nil
			}
			ct, err := g.index.GetTypedClusterTrait(name)
			if err != nil {
				return err
			}
			clusterTraits[name] = ct.Spec
		case v1alpha1.TraitRefKindTrait, "":
			if _, exists := traits[name]; exists {
				return nil
			}
			t, err := g.index.GetTypedTrait(namespace, name)
			if err != nil {
				return err
			}
			traits[name] = t.Spec
		default:
			return fmt.Errorf("unsupported trait kind %q for trait %q", kind, name)
		}
		return nil
	}

	for _, et := range ctSpec.Traits {
		if err := addTrait(et.Kind, et.Name); err != nil {
			return nil, nil, err
		}
	}
	for _, ref := range comp.Spec.Traits {
		if err := addTrait(ref.Kind, ref.Name); err != nil {
			return nil, nil, err
		}
	}

	return traits, clusterTraits, nil
}

// toUnstructuredRelease wraps a ComponentReleaseSpec in a typed ComponentRelease and converts it
// to unstructured, pruning the zero-valued status and creationTimestamp that typed conversion emits.
func toUnstructuredRelease(name, namespace string, crSpec *v1alpha1.ComponentReleaseSpec) (*unstructured.Unstructured, error) {
	release := &v1alpha1.ComponentRelease{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: *crSpec,
	}

	obj, err := runtime.DefaultUnstructuredConverter.ToUnstructured(release)
	if err != nil {
		return nil, fmt.Errorf("failed to convert component release to unstructured: %w", err)
	}

	obj["apiVersion"] = componentReleaseAPIVersion
	obj["kind"] = componentReleaseKind

	delete(obj, "status")
	if metadata, ok := obj["metadata"].(map[string]interface{}); ok {
		delete(metadata, "creationTimestamp")
	}

	return &unstructured.Unstructured{Object: obj}, nil
}
