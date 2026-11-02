// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

// Package component provides the main rendering pipeline for Component resources.
//
// The pipeline combines Component, ComponentType, Traits, Workload and ReleaseBinding
// to generate fully resolved Kubernetes resource manifests by:
//   - Building CEL evaluation contexts with parameters, overrides, and defaults
//   - Rendering base resources from ComponentType
//   - Processing traits (creates and patches)
//   - Post-processing (validation, labels, annotations)
package component

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/go-playground/validator/v10"

	"github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/labels"
	pipelinecontext "github.com/openchoreo/openchoreo/internal/pipeline/component/context"
	"github.com/openchoreo/openchoreo/internal/pipeline/component/renderer"
	"github.com/openchoreo/openchoreo/internal/pipeline/component/trait"
	"github.com/openchoreo/openchoreo/internal/template"
	"github.com/openchoreo/openchoreo/pkg/hash"
)

var validate = validator.New(validator.WithRequiredStructEnabled())

// Workload type constants for internal use
const (
	workloadTypeDeployment  = "deployment"
	workloadTypeStatefulSet = "statefulset"
)

// Kubernetes resource kind constants
const (
	kindDeployment     = "Deployment"
	kindStatefulSet    = "StatefulSet"
	kindConfigMap      = "ConfigMap"
	kindSecret         = "Secret"
	kindExternalSecret = "ExternalSecret"
)

// externalSecretsGroup is the API group of External Secrets Operator resources.
const externalSecretsGroup = "external-secrets.io"

// Option is a function that configures a Pipeline.
type Option func(*Pipeline)

// WithCostLimit sets the maximum accumulated cost for a single CEL expression
// evaluated by this pipeline's template engine. Zero selects the engine's
// built-in safe default; it never means unlimited.
func WithCostLimit(limit uint64) Option {
	return func(p *Pipeline) {
		p.celCostLimit = limit
	}
}

// WithRenderTimeout bounds the wall-clock duration of each call into this pipeline. The
// deadline is derived inside every entry point rather than by the caller, so it covers
// exactly one rendering step and never spans the work around it - an API read before, a
// status write after. Zero or negative means no deadline; the CEL cost limits remain the
// primary bound.
func WithRenderTimeout(timeout time.Duration) Option {
	return func(p *Pipeline) {
		p.renderTimeout = timeout
	}
}

// WithReferencedConfigRollout limits the dp-resource-hash annotation to the ConfigMaps,
// Secrets and ExternalSecrets the pod template references, and enables the restart-on-change
// annotation. Without it, the hash covers every dataplane resource except the workload, so a
// change to any of them restarts the pods.
func WithReferencedConfigRollout(enabled bool) Option {
	return func(p *Pipeline) {
		p.referencedConfigRollout = enabled
	}
}

// NewPipeline creates a new component rendering pipeline.
func NewPipeline(opts ...Option) *Pipeline {
	p := &Pipeline{}
	for _, opt := range opts {
		opt(p)
	}
	if p.templateEngine == nil {
		p.templateEngine = template.NewEngineWithOptions(
			template.WithCELExtensions(pipelinecontext.CELExtensions()...),
			template.WithCostLimit(p.celCostLimit),
		)
	}
	return p
}

// Render orchestrates the complete rendering workflow for a Component.
//
// Workflow:
//   - Validate input
//   - Build component context (parameters + overrides + defaults)
//   - Render base resources from ComponentType
//   - Process traits (creates and patches)
//   - Post-process (validate, add labels/annotations, sort)
//   - Return output
//
// Returns an error if any step fails.
func (p *Pipeline) Render(ctx context.Context, input *RenderInput) (*RenderOutput, error) {
	ctx, cancel := template.WithRenderTimeout(ctx, p.renderTimeout)
	defer cancel()

	ctx = renderer.WithForEachBudget(ctx)

	// Validate input
	if err := p.validateInput(input); err != nil {
		return nil, fmt.Errorf("invalid input: %w", err)
	}

	metadata := &RenderMetadata{
		Warnings: []string{},
	}

	// Apply workload overrides from ReleaseBinding if present
	workload := input.Workload
	if input.Workload != nil && input.ReleaseBinding != nil && input.ReleaseBinding.Spec.WorkloadOverrides != nil {
		workload = pipelinecontext.MergeWorkloadOverrides(input.Workload, input.ReleaseBinding.Spec.WorkloadOverrides)
	}

	// Pre-compute workload data and configurations once and share across all contexts
	workloadData := pipelinecontext.ExtractWorkloadData(workload)
	configurations := pipelinecontext.ExtractConfigurationsFromWorkload(input.SecretReferences, workload)
	dependenciesData := pipelinecontext.ConnectionsData{
		Items:     input.DependencyItems,
		Resources: input.ResourceDependencyItems,
	}

	// Endpoint API schema parsing is opt-in: only extract resources when a template
	// references the workload.toEndpointResources() macro. This keeps large schemas
	// out of the render context (and skips parsing) for the common case.
	var endpointResources pipelinecontext.EndpointResourceMap
	if usesEndpointResources(input) {
		endpointResources = pipelinecontext.ExtractEndpointResources(workload)
	}

	// Build the trait context base once and reuse it for every trait built in this render.
	// Both regular and embedded trait inputs embed TraitContextBase.
	traitBase := pipelinecontext.TraitContextBase{
		Metadata:                   input.Metadata,
		DataPlane:                  input.DataPlane,
		Environment:                input.Environment,
		WorkloadData:               workloadData,
		EndpointResources:          endpointResources,
		Configurations:             configurations,
		Dependencies:               dependenciesData,
		DefaultNotificationChannel: input.DefaultNotificationChannel,
	}

	// Build component context
	componentContext, err := pipelinecontext.BuildComponentContext(&pipelinecontext.ComponentContextInput{
		Component:                  input.Component,
		ComponentType:              input.ComponentType,
		ReleaseBinding:             input.ReleaseBinding,
		DataPlane:                  input.DataPlane,
		Environment:                input.Environment,
		WorkloadData:               workloadData,
		EndpointResources:          endpointResources,
		Configurations:             configurations,
		Dependencies:               dependenciesData,
		Metadata:                   input.Metadata,
		DefaultNotificationChannel: input.DefaultNotificationChannel,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to build component context: %w", err)
	}

	// Convert component context to map once and reuse for validation, base rendering,
	// and embedded trait binding resolution.
	componentContextMap := componentContext.ToMap()

	// Evaluate ComponentType pre-render validation rules
	if err := renderer.EvaluateValidationRules(
		ctx,
		p.templateEngine,
		input.ComponentType.Spec.EffectivePreRenderValidations(),
		componentContextMap,
	); err != nil {
		return nil, fmt.Errorf("component type validation failed: %w", err)
	}

	// Accumulate post-render validations across the ComponentType and every trait;
	// evaluated once after every trait's creates/patches/removes have been applied to
	// the final resource set.
	pendingPostRenders := componentTypePendingPostRenders(input, componentContextMap)

	input.ApplyTargetPlaneDefaults()

	// Render base resources from ComponentType
	resourceRenderer := renderer.NewRenderer(p.templateEngine)
	renderedResources, err := resourceRenderer.RenderResources(
		ctx,
		input.ComponentType.Spec.Resources,
		componentContextMap,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to render base resources: %w", err)
	}
	metadata.BaseResourceCount = len(renderedResources)

	// Process traits
	traitProcessor := trait.NewProcessor(p.templateEngine)

	// Build trait map keyed by "Kind:Name" to support same-name Trait and ClusterTrait coexisting.
	traitMap := make(map[string]*v1alpha1.Trait)
	for i := range input.Traits {
		t := &input.Traits[i]
		kind := t.Kind // TypeMeta.Kind — "Trait" or "ClusterTrait"
		if kind == "" {
			kind = string(v1alpha1.TraitRefKindTrait)
		}
		traitMap[kind+":"+t.Name] = t
	}

	// Create schema cache for trait reuse within this render
	schemaCache := make(map[string]*pipelinecontext.SchemaBundle)

	// Process embedded traits from ComponentType (before component-level traits)
	for _, embeddedTrait := range input.ComponentType.Spec.Traits {
		embeddedKind := string(embeddedTrait.Kind)
		if embeddedKind == "" {
			embeddedKind = string(v1alpha1.TraitRefKindTrait)
		}
		t, ok := traitMap[embeddedKind+":"+embeddedTrait.Name]
		if !ok {
			return nil, fmt.Errorf("embedded trait %s referenced but not found in traits list", embeddedTrait.Name)
		}

		// Resolve CEL bindings against component context
		resolvedParams, resolvedEnvironmentConfigs, err := pipelinecontext.ResolveEmbeddedTraitBindings(
			ctx,
			p.templateEngine,
			embeddedTrait,
			componentContextMap,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve embedded trait bindings for %s/%s: %w",
				embeddedTrait.Name, embeddedTrait.InstanceName, err)
		}

		// Build embedded trait context
		traitContext, err := pipelinecontext.BuildTraitContext(&pipelinecontext.TraitContextInput{
			TraitContextBase:           traitBase,
			Trait:                      t,
			InstanceName:               embeddedTrait.InstanceName,
			ResolvedParameters:         resolvedParams,
			ResolvedEnvironmentConfigs: resolvedEnvironmentConfigs,
			SchemaCache:                schemaCache,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to build embedded trait context for %s/%s: %w",
				embeddedTrait.Name, embeddedTrait.InstanceName, err)
		}

		traitContextMap := traitContext.ToMap()
		if err := renderer.EvaluateValidationRules(ctx, p.templateEngine, t.Spec.EffectivePreRenderValidations(), traitContextMap); err != nil {
			return nil, fmt.Errorf("trait %s/%s validation failed: %w",
				embeddedTrait.Name, embeddedTrait.InstanceName, err)
		}

		beforeCount := len(renderedResources)
		renderedResources, err = traitProcessor.ProcessTraits(ctx, renderedResources, t, traitContextMap)
		if err != nil {
			return nil, fmt.Errorf("failed to process embedded trait %s/%s: %w",
				embeddedTrait.Name, embeddedTrait.InstanceName, err)
		}

		if len(t.Spec.PostRenderValidations) > 0 {
			pendingPostRenders = append(pendingPostRenders, pendingPostRender{
				label:       fmt.Sprintf("%s %s/%s", embeddedKind, embeddedTrait.Name, embeddedTrait.InstanceName),
				context:     traitContextMap,
				validations: t.Spec.PostRenderValidations,
			})
		}

		metadata.TraitCount++
		metadata.TraitResourceCount += len(renderedResources) - beforeCount
	}

	// Process each component-level trait instance
	for _, traitInstance := range input.Component.Spec.Traits {
		instanceKind := string(traitInstance.Kind)
		if instanceKind == "" {
			instanceKind = string(v1alpha1.TraitRefKindTrait)
		}
		t, ok := traitMap[instanceKind+":"+traitInstance.Name]
		if !ok {
			return nil, fmt.Errorf("trait %s referenced but not found in traits list", traitInstance.Name)
		}

		// Resolve the component-level trait's instance bindings (just JSON deserialization)
		resolvedParams, resolvedEnvironmentConfigs, err := pipelinecontext.ExtractTraitInstanceBindings(traitInstance, input.ReleaseBinding)
		if err != nil {
			return nil, fmt.Errorf("failed to extract trait bindings for %s/%s: %w",
				traitInstance.Name, traitInstance.InstanceName, err)
		}

		// Build trait context (BuildTraitContext will handle schema caching)
		traitContext, err := pipelinecontext.BuildTraitContext(&pipelinecontext.TraitContextInput{
			TraitContextBase:           traitBase,
			Trait:                      t,
			InstanceName:               traitInstance.InstanceName,
			ResolvedParameters:         resolvedParams,
			ResolvedEnvironmentConfigs: resolvedEnvironmentConfigs,
			SchemaCache:                schemaCache,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to build trait context for %s/%s: %w",
				traitInstance.Name, traitInstance.InstanceName, err)
		}

		traitContextMap := traitContext.ToMap()
		if err := renderer.EvaluateValidationRules(ctx, p.templateEngine, t.Spec.EffectivePreRenderValidations(), traitContextMap); err != nil {
			return nil, fmt.Errorf("trait %s/%s validation failed: %w",
				traitInstance.Name, traitInstance.InstanceName, err)
		}

		beforeCount := len(renderedResources)
		renderedResources, err = traitProcessor.ProcessTraits(ctx, renderedResources, t, traitContextMap)
		if err != nil {
			return nil, fmt.Errorf("failed to process trait %s/%s: %w",
				traitInstance.Name, traitInstance.InstanceName, err)
		}

		if len(t.Spec.PostRenderValidations) > 0 {
			pendingPostRenders = append(pendingPostRenders, pendingPostRender{
				label:       fmt.Sprintf("%s %s/%s", instanceKind, traitInstance.Name, traitInstance.InstanceName),
				context:     traitContextMap,
				validations: t.Spec.PostRenderValidations,
			})
		}

		metadata.TraitCount++
		metadata.TraitResourceCount += len(renderedResources) - beforeCount
	}

	// Post-render validations run against the final resource set, after every trait's
	// creates/patches/removes have been applied and before OpenChoreo post-processing.
	if err := evaluatePostRenderValidations(ctx, p.templateEngine, renderedResources, pendingPostRenders); err != nil {
		return nil, fmt.Errorf("post-render validation failed: %w", err)
	}

	if err := p.postProcessResources(renderedResources, input); err != nil {
		return nil, fmt.Errorf("failed to post-process resources: %w", err)
	}

	if err := p.validateResources(renderedResources); err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	sortRenderedResources(renderedResources)

	metadata.ResourceCount = len(renderedResources)

	return &RenderOutput{
		Resources: renderedResources,
		Metadata:  metadata,
	}, nil
}

// validateInput ensures the input has all required fields.
func (p *Pipeline) validateInput(input *RenderInput) error {
	if err := validate.Struct(input); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	// Additional validation that can't be expressed with struct tags
	if input.ComponentType.Spec.Resources == nil {
		return fmt.Errorf("component type has no resources")
	}

	return nil
}

// componentTypePendingPostRenders seeds the post-render accumulator with the
// ComponentType's own post-render validations (bound to the component context), so
// they are evaluated in the same stage as the traits', after every trait is applied.
func componentTypePendingPostRenders(input *RenderInput, componentContextMap map[string]any) []pendingPostRender {
	if len(input.ComponentType.Spec.PostRenderValidations) == 0 {
		return nil
	}
	return []pendingPostRender{{
		label:       fmt.Sprintf("ComponentType %s", input.ComponentType.Name),
		context:     componentContextMap,
		validations: input.ComponentType.Spec.PostRenderValidations,
	}}
}

// endpointResourcesMacroToken is the literal that appears in a template's CEL
// expressions when it opts into endpoint schema extraction via
// workload.toEndpointResources().
const endpointResourcesMacroToken = "toEndpointResources"

// usesEndpointResources reports whether any ComponentType or trait template
// uses the workload.toEndpointResources() macro.
func usesEndpointResources(input *RenderInput) bool {
	token := []byte(endpointResourcesMacroToken)
	ct := input.ComponentType.Spec
	//nolint:staticcheck // deprecated field still read for backward-compat alias fallback
	if jsonContainsToken(token, ct.Resources, ct.Validations, ct.PreRenderValidations, ct.PostRenderValidations, ct.Traits) {
		return true
	}
	for i := range input.Traits {
		t := input.Traits[i].Spec
		//nolint:staticcheck // deprecated field still read for backward-compat alias fallback
		if jsonContainsToken(token, t.Validations, t.PreRenderValidations, t.PostRenderValidations, t.Creates, t.Patches, t.Removes) {
			return true
		}
	}
	return false
}

// jsonContainsToken reports whether the JSON encoding of any value contains token.
func jsonContainsToken(token []byte, values ...any) bool {
	for _, v := range values {
		if b, err := json.Marshal(v); err == nil && bytes.Contains(b, token) {
			return true
		}
	}
	return false
}

// postProcessResources adds labels, annotations, and performs cleanup.
func (p *Pipeline) postProcessResources(resources []renderer.RenderedResource, input *RenderInput) error {
	for _, rr := range resources {
		if err := addLabels(rr.Resource, input.Metadata.Labels); err != nil {
			return fmt.Errorf("failed to add labels: %w", err)
		}
	}

	if err := p.addDPResourceHashAnnotation(resources, input); err != nil {
		return fmt.Errorf("failed to add dp-resource-hash annotation: %w", err)
	}

	return nil
}

// addLabels adds labels to a resource's metadata.
func addLabels(resource map[string]any, labelsToAdd map[string]string) error {
	metadata, ok := resource["metadata"].(map[string]any)
	if !ok {
		return fmt.Errorf("resource missing metadata")
	}
	existingLabels, _ := metadata["labels"].(map[string]any)
	if existingLabels == nil {
		existingLabels = make(map[string]any)
	}
	for k, v := range labelsToAdd {
		existingLabels[k] = v
	}
	metadata["labels"] = existingLabels
	return nil
}

// validateResources performs basic validation on rendered resources.
func (p *Pipeline) validateResources(resources []renderer.RenderedResource) error {
	for i, rr := range resources {
		resource := rr.Resource
		kind, _ := resource["kind"].(string)
		apiVersion, _ := resource["apiVersion"].(string)
		var resourceID string
		if kind != "" {
			resourceID = fmt.Sprintf("resource #%d (%s)", i, kind)
		} else {
			resourceID = fmt.Sprintf("resource #%d", i)
		}

		if apiVersion == "" {
			return fmt.Errorf("%s missing apiVersion", resourceID)
		}
		if kind == "" {
			return fmt.Errorf("%s missing kind", resourceID)
		}

		metadata, ok := resource["metadata"].(map[string]any)
		if !ok {
			return fmt.Errorf("%s missing metadata", resourceID)
		}

		name, _ := metadata["name"].(string)
		if name == "" {
			return fmt.Errorf("%s missing metadata.name", resourceID)
		}
	}
	return nil
}

func sortRenderedResources(renderedResources []renderer.RenderedResource) {
	sort.SliceStable(renderedResources, func(i, j int) bool {
		return compareResources(renderedResources[i].Resource, renderedResources[j].Resource)
	})
}

func compareResources(a, b map[string]any) bool {
	kind1, _ := a["kind"].(string)
	kind2, _ := b["kind"].(string)
	if kind1 != kind2 {
		return kind1 < kind2
	}

	apiVersion1, _ := a["apiVersion"].(string)
	apiVersion2, _ := b["apiVersion"].(string)
	if apiVersion1 != apiVersion2 {
		return apiVersion1 < apiVersion2
	}

	meta1, _ := a["metadata"].(map[string]any)
	meta2, _ := b["metadata"].(map[string]any)

	var ns1, ns2, name1, name2 string
	if meta1 != nil {
		ns1, _ = meta1["namespace"].(string)
		name1, _ = meta1["name"].(string)
	}
	if meta2 != nil {
		ns2, _ = meta2["namespace"].(string)
		name2, _ = meta2["name"].(string)
	}

	if ns1 != ns2 {
		return ns1 < ns2
	}
	return name1 < name2
}

// addDPResourceHashAnnotation adds an annotation to the pod template of Deployment/StatefulSet
// workloads containing a hash of other dataplane resources, so that a change to them triggers
// a pod rollout. By default, the hash covers every dataplane resource except the workload.
// With WithReferencedConfigRollout, it covers only the configuration the pods reference, see
// referencedConfigHash.
func (p *Pipeline) addDPResourceHashAnnotation(resources []renderer.RenderedResource, input *RenderInput) error {
	workloadType := input.ComponentType.Spec.WorkloadType

	// Only apply to deployment and statefulset workload types
	if workloadType != workloadTypeDeployment && workloadType != workloadTypeStatefulSet {
		return nil
	}

	allResources := make([]map[string]any, 0, len(resources))
	dataPlaneResources := make([]map[string]any, 0, len(resources))
	for _, rr := range resources {
		allResources = append(allResources, rr.Resource)
		if rr.TargetPlane == v1alpha1.TargetPlaneDataPlane {
			dataPlaneResources = append(dataPlaneResources, rr.Resource)
		}
	}

	workload := findMainWorkload(allResources, workloadType)
	if workload == nil {
		return nil
	}

	var resourceHash string
	if p.referencedConfigRollout {
		var err error
		if resourceHash, err = referencedConfigHash(workload, dataPlaneResources, input.PreviousDataPlaneResources); err != nil {
			return err
		}
	} else {
		nonWorkloadResources := make([]map[string]any, 0, len(dataPlaneResources))
		for _, resource := range dataPlaneResources {
			if kind, _ := resource["kind"].(string); !isMainWorkloadKind(kind, workloadType) {
				nonWorkloadResources = append(nonWorkloadResources, resource)
			}
		}
		resourceHash = hashResources(nonWorkloadResources)
	}

	// If no resource is hashed, there is nothing to annotate
	if resourceHash == "" {
		return nil
	}

	if err := addPodTemplateAnnotation(workload, labels.AnnotationKeyDPResourceHash, resourceHash); err != nil {
		kind, _ := workload["kind"].(string)
		return fmt.Errorf("failed to add annotation to %s: %w", kind, err)
	}
	return nil
}

// referencedConfigHash returns the dp-resource-hash value for the workload when only the
// configuration the pods reference counts, or "" when the workload gets no annotation. It hashes
// the dataplane ConfigMaps, Secrets and ExternalSecrets that the pod template references, so
// changes to Services, HTTPRoutes, HorizontalPodAutoscalers and to configuration the pods do not
// reference do not restart the pods. ComponentType and Trait authors can override this per
// resource with the restart-on-change annotation.
//
// When the previous render is given, the resources this render counts have the same content in
// it, and the resources the previous render counted are all still rendered, the previous value is
// kept. This way, changing what the hash counts, by turning the option on or with the
// restart-on-change annotation, does not restart the pods by itself.
func referencedConfigHash(workload map[string]any, resources, previousResources []map[string]any) (string, error) {
	if err := validateRestartOnChange(resources, workload); err != nil {
		return "", err
	}

	configResources := configResourcesToHash(workload, resources)
	resourceHash := hashResources(configResources)

	// Keep the previous value while the counted resources are unchanged and none was removed
	if previousWorkload := findResource(previousResources, resourceKey(workload)); previousWorkload != nil {
		unchanged, err := sameContent(configResources, previousResources)
		if err != nil {
			return "", err
		}
		if unchanged && allRendered(configResourcesToHash(previousWorkload, previousResources), resources) {
			previousHash, _ := podTemplateAnnotation(previousWorkload, labels.AnnotationKeyDPResourceHash)
			return previousHash, nil
		}
	}
	return resourceHash, nil
}

// findMainWorkload returns the first resource whose kind is the main workload kind for the
// given workloadType, or nil if there is none.
func findMainWorkload(resources []map[string]any, workloadType string) map[string]any {
	for _, resource := range resources {
		kind, _ := resource["kind"].(string)
		if isMainWorkloadKind(kind, workloadType) {
			return resource
		}
	}
	return nil
}

// resourceKey identifies a resource by its apiVersion, kind, namespace and name.
func resourceKey(resource map[string]any) string {
	apiVersion, _ := resource["apiVersion"].(string)
	kind, _ := resource["kind"].(string)
	return strings.Join([]string{
		apiVersion, kind, nestedString(resource, "metadata", "namespace"), nestedString(resource, "metadata", "name"),
	}, "/")
}

// findResource returns the resource with the given key, or nil if there is none.
func findResource(resources []map[string]any, key string) map[string]any {
	for _, resource := range resources {
		if resourceKey(resource) == key {
			return resource
		}
	}
	return nil
}

// configResourcesToHash returns the ConfigMaps, Secrets and ExternalSecrets in resources that the
// workload's pod template references. A resource with the restart-on-change annotation is
// included when it is "true" and left out when it is "false". The workload itself is never included.
func configResourcesToHash(workload map[string]any, resources []map[string]any) []map[string]any {
	refs := podTemplateConfigRefs(workload)
	workloadKey := resourceKey(workload)

	selected := make([]map[string]any, 0, len(resources))
	for _, resource := range resources {
		if resourceKey(resource) == workloadKey {
			continue
		}

		switch restartOnChange(resource) {
		case restartOnChangeTrue:
			selected = append(selected, resource)
		case restartOnChangeFalse:
			continue
		default:
			if refs.references(resource) {
				selected = append(selected, resource)
			}
		}
	}
	return selected
}

// hashResources returns a hash of the content of the resources, excluding metadata, or "" if
// there are none.
func hashResources(resources []map[string]any) string {
	if len(resources) == 0 {
		return ""
	}

	// Sort resources to ensure deterministic hash regardless of resource ordering
	sorted := append([]map[string]any(nil), resources...)
	sort.SliceStable(sorted, func(i, j int) bool {
		return compareResources(sorted[i], sorted[j])
	})

	// Extract content excluding metadata for hashing
	hashContent := make([]map[string]any, 0, len(sorted))
	for _, resource := range sorted {
		hashContent = append(hashContent, extractContentExcludingMetadata(resource))
	}

	return hash.ComputeHash(hashContent, nil)
}

// sameContent reports whether every resource in resources is in previousResources with the same
// content. The previous render is decoded from JSON, so the resources are compared through a JSON
// copy. Otherwise, a rendered int64 would not match the float64 it decodes to.
func sameContent(resources, previousResources []map[string]any) (bool, error) {
	previousByKey := make(map[string]map[string]any, len(previousResources))
	for _, resource := range previousResources {
		previousByKey[resourceKey(resource)] = resource
	}

	previous := make([]map[string]any, 0, len(resources))
	for _, resource := range resources {
		match, found := previousByKey[resourceKey(resource)]
		if !found {
			return false, nil
		}
		previous = append(previous, match)
	}

	current, err := jsonCopy(resources)
	if err != nil {
		return false, fmt.Errorf("failed to copy the configuration resources: %w", err)
	}
	return hashResources(current) == hashResources(previous), nil
}

// allRendered reports whether every resource in previous is still in resources, matched by key.
func allRendered(previous, resources []map[string]any) bool {
	rendered := make(map[string]bool, len(resources))
	for _, resource := range resources {
		rendered[resourceKey(resource)] = true
	}
	for _, resource := range previous {
		if !rendered[resourceKey(resource)] {
			return false
		}
	}
	return true
}

// jsonCopy returns a copy of v decoded from its JSON encoding.
func jsonCopy[T any](v T) (T, error) {
	var out T
	data, err := json.Marshal(v)
	if err != nil {
		return out, err
	}
	err = json.Unmarshal(data, &out)
	return out, err
}

// Values of the restart-on-change annotation
const (
	restartOnChangeTrue  = "true"
	restartOnChangeFalse = "false"
)

// restartOnChange returns the value of the restart-on-change annotation on a resource, or "" if
// it is not set.
func restartOnChange(resource map[string]any) string {
	value, _ := nestedMap(resource, "metadata", "annotations")[labels.AnnotationKeyRestartOnChange].(string)
	return value
}

// validateRestartOnChange returns an error for the first resource whose restart-on-change
// annotation is neither "true" nor "false". Annotation values must be strings, so an unquoted
// true in a template is reported too. The workload is skipped, since the annotation does not
// apply to it.
func validateRestartOnChange(resources []map[string]any, workload map[string]any) error {
	workloadKey := resourceKey(workload)
	for _, resource := range resources {
		if resourceKey(resource) == workloadKey {
			continue
		}
		value, found := nestedMap(resource, "metadata", "annotations")[labels.AnnotationKeyRestartOnChange]
		if !found || value == restartOnChangeTrue || value == restartOnChangeFalse {
			continue
		}
		kind, _ := resource["kind"].(string)
		name := nestedString(resource, "metadata", "name")
		if _, isString := value.(string); !isString {
			return fmt.Errorf("%s %q: annotation %s must be the string %q or %q, got %v (%T)", kind, name,
				labels.AnnotationKeyRestartOnChange, restartOnChangeTrue, restartOnChangeFalse, value, value)
		}
		return fmt.Errorf("%s %q: annotation %s must be %q or %q, got %q", kind, name,
			labels.AnnotationKeyRestartOnChange, restartOnChangeTrue, restartOnChangeFalse, value)
	}
	return nil
}

// configRefs holds the names of the ConfigMaps and Secrets that a pod template references.
type configRefs struct {
	configMaps map[string]bool
	secrets    map[string]bool
}

// podTemplateConfigRefs collects the ConfigMaps and Secrets that the workload's pod template
// references through env, envFrom and volumes. Init containers are included, since native
// sidecars are declared as init containers.
func podTemplateConfigRefs(workload map[string]any) configRefs {
	refs := configRefs{configMaps: map[string]bool{}, secrets: map[string]bool{}}
	podSpec := nestedMap(workload, "spec", "template", "spec")

	for _, field := range []string{"containers", "initContainers"} {
		for _, container := range nestedMaps(podSpec, field) {
			for _, env := range nestedMaps(container, "env") {
				refs.addConfigMap(nestedString(env, "valueFrom", "configMapKeyRef", "name"))
				refs.addSecret(nestedString(env, "valueFrom", "secretKeyRef", "name"))
			}
			for _, envFrom := range nestedMaps(container, "envFrom") {
				refs.addConfigMap(nestedString(envFrom, "configMapRef", "name"))
				refs.addSecret(nestedString(envFrom, "secretRef", "name"))
			}
		}
	}

	for _, volume := range nestedMaps(podSpec, "volumes") {
		refs.addConfigMap(nestedString(volume, "configMap", "name"))
		refs.addSecret(nestedString(volume, "secret", "secretName"))
		for _, source := range nestedMaps(volume, "projected", "sources") {
			refs.addConfigMap(nestedString(source, "configMap", "name"))
			refs.addSecret(nestedString(source, "secret", "name"))
		}
	}

	return refs
}

func (r configRefs) addConfigMap(name string) {
	if name != "" {
		r.configMaps[name] = true
	}
}

func (r configRefs) addSecret(name string) {
	if name != "" {
		r.secrets[name] = true
	}
}

// references reports whether the pod template references the given resource. An ExternalSecret
// counts when the pod template references the Secret it creates, which is named by
// spec.target.name, or after the ExternalSecret itself when that is not set.
func (r configRefs) references(resource map[string]any) bool {
	apiVersion, _ := resource["apiVersion"].(string)
	kind, _ := resource["kind"].(string)
	name := nestedString(resource, "metadata", "name")

	switch {
	case apiVersion == "v1" && kind == kindConfigMap:
		return r.configMaps[name]
	case apiVersion == "v1" && kind == kindSecret:
		return r.secrets[name]
	case strings.HasPrefix(apiVersion, externalSecretsGroup+"/") && kind == kindExternalSecret:
		if target := nestedString(resource, "spec", "target", "name"); target != "" {
			name = target
		}
		return r.secrets[name]
	default:
		return false
	}
}

// podTemplateAnnotation returns the value of an annotation on the pod template of a workload resource.
func podTemplateAnnotation(resource map[string]any, key string) (string, bool) {
	annotations := nestedMap(resource, "spec", "template", "metadata", "annotations")
	value, found := annotations[key].(string)
	return value, found
}

// nestedMap returns the map at the given path, or nil if any element is missing or not a map.
func nestedMap(obj map[string]any, fields ...string) map[string]any {
	current := obj
	for _, field := range fields {
		next, ok := current[field].(map[string]any)
		if !ok {
			return nil
		}
		current = next
	}
	return current
}

// nestedMaps returns the map items of the list at the given path, skipping items that are not maps.
func nestedMaps(obj map[string]any, fields ...string) []map[string]any {
	parent := nestedMap(obj, fields[:len(fields)-1]...)
	switch items := parent[fields[len(fields)-1]].(type) {
	case []map[string]any:
		return items
	case []any:
		result := make([]map[string]any, 0, len(items))
		for _, item := range items {
			if m, ok := item.(map[string]any); ok {
				result = append(result, m)
			}
		}
		return result
	default:
		return nil
	}
}

// nestedString returns the string at the given path, or "" if it is missing or not a string.
func nestedString(obj map[string]any, fields ...string) string {
	parent := nestedMap(obj, fields[:len(fields)-1]...)
	value, _ := parent[fields[len(fields)-1]].(string)
	return value
}

// isMainWorkloadKind returns true if the kind matches the expected main workload for the given workloadType.
func isMainWorkloadKind(kind, workloadType string) bool {
	switch workloadType {
	case workloadTypeDeployment:
		return kind == kindDeployment
	case workloadTypeStatefulSet:
		return kind == kindStatefulSet
	default:
		return false
	}
}

// extractContentExcludingMetadata returns a copy of the resource with metadata removed.
func extractContentExcludingMetadata(resource map[string]any) map[string]any {
	result := make(map[string]any)
	for k, v := range resource {
		if k == "metadata" {
			continue
		}
		result[k] = v
	}
	return result
}

// addPodTemplateAnnotation adds an annotation to the pod template of a workload resource.
func addPodTemplateAnnotation(resource map[string]any, key, value string) error {
	// For Deployment/StatefulSet, pod template is at spec.template
	spec, ok := resource["spec"].(map[string]any)
	if !ok {
		return fmt.Errorf("resource missing spec")
	}

	template, ok := spec["template"].(map[string]any)
	if !ok {
		// Create template if it doesn't exist
		template = make(map[string]any)
		spec["template"] = template
	}

	templateMeta, ok := template["metadata"].(map[string]any)
	if !ok {
		templateMeta = make(map[string]any)
		template["metadata"] = templateMeta
	}

	annotations, ok := templateMeta["annotations"].(map[string]any)
	if !ok {
		annotations = make(map[string]any)
		templateMeta["annotations"] = annotations
	}

	annotations[key] = value
	return nil
}
