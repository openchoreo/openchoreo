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
	kindDeployment  = "Deployment"
	kindStatefulSet = "StatefulSet"
)

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

// RenderForLint renders like Render but captures declared post-render validation
// failures as structured PostRenderFailure values instead of aborting, so a linter
// can report them and still inspect the fully post-processed resources.
//
// Non-post-render errors behave as in Render: Resources is nil and a non-nil error
// is returned (alongside any failures captured earlier).
func (p *Pipeline) RenderForLint(ctx context.Context, input *RenderInput) (*RenderOutput, []PostRenderFailure, error) {
	return p.renderForLint(ctx, input)
}

// renderForLint mirrors Render but calls evaluatePostRenderValidationsForLint so
// declared post-render failures are captured and the render continues past them.
func (p *Pipeline) renderForLint(ctx context.Context, input *RenderInput) (*RenderOutput, []PostRenderFailure, error) {
	ctx, cancel := template.WithRenderTimeout(ctx, p.renderTimeout)
	defer cancel()

	ctx = renderer.WithForEachBudget(ctx)

	// Validate input
	if err := p.validateInput(input); err != nil {
		return nil, nil, fmt.Errorf("invalid input: %w", err)
	}

	metadata := &RenderMetadata{
		Warnings: []string{},
	}

	lrc, err := p.buildLintRenderContext(ctx, input)
	if err != nil {
		return nil, nil, err
	}

	input.ApplyTargetPlaneDefaults()

	// Render base resources from ComponentType
	resourceRenderer := renderer.NewRenderer(p.templateEngine)
	renderedResources, err := resourceRenderer.RenderResources(
		ctx,
		input.ComponentType.Spec.Resources,
		lrc.componentContextMap,
	)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to render base resources: %w", err)
	}
	metadata.BaseResourceCount = len(renderedResources)

	run := newLintTraitRun(p.templateEngine, input, lrc, renderedResources)

	// Embedded traits run before component-level traits.
	if err := run.processEmbeddedTraits(ctx, input, metadata); err != nil {
		return nil, nil, err
	}
	if err := run.processComponentTraits(ctx, input, metadata); err != nil {
		return nil, nil, err
	}

	// The lint path captures declared post-render failures; the joined error is
	// discarded because the structured failures carry the same information.
	failures, _ := evaluatePostRenderValidationsForLint(ctx, p.templateEngine, run.rendered, run.pendingPostRenders)

	return p.finalizeLintRender(input, run.rendered, failures, metadata)
}

// lintRenderContext holds the contexts built once and shared across a lint render.
type lintRenderContext struct {
	traitBase           pipelinecontext.TraitContextBase
	componentContextMap map[string]any
}

// buildLintRenderContext builds the component context and the shared trait context
// base, and evaluates the ComponentType's pre-render validation rules.
func (p *Pipeline) buildLintRenderContext(ctx context.Context, input *RenderInput) (*lintRenderContext, error) {
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
	// references the workload.toEndpointResources() macro.
	var endpointResources pipelinecontext.EndpointResourceMap
	if usesEndpointResources(input) {
		endpointResources = pipelinecontext.ExtractEndpointResources(workload)
	}

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

	componentContextMap := componentContext.ToMap()

	if err := renderer.EvaluateValidationRules(
		ctx,
		p.templateEngine,
		input.ComponentType.Spec.EffectivePreRenderValidations(),
		componentContextMap,
	); err != nil {
		return nil, fmt.Errorf("component type validation failed: %w", err)
	}

	return &lintRenderContext{
		traitBase:           traitBase,
		componentContextMap: componentContextMap,
	}, nil
}

// lintTraitRun threads the mutable state through lint trait processing.
type lintTraitRun struct {
	rendered           []renderer.RenderedResource
	pendingPostRenders []pendingPostRender
	traitMap           map[string]*v1alpha1.Trait
	schemaCache        map[string]*pipelinecontext.SchemaBundle
	processor          *trait.Processor
	engine             *template.Engine
	lrc                *lintRenderContext
}

// newLintTraitRun seeds the trait run with the base resources and trait lookups.
func newLintTraitRun(
	engine *template.Engine,
	input *RenderInput,
	lrc *lintRenderContext,
	rendered []renderer.RenderedResource,
) *lintTraitRun {
	return &lintTraitRun{
		rendered:           rendered,
		pendingPostRenders: componentTypePendingPostRenders(input, lrc.componentContextMap),
		traitMap:           lintTraitMap(input.Traits),
		schemaCache:        make(map[string]*pipelinecontext.SchemaBundle),
		processor:          trait.NewProcessor(engine),
		engine:             engine,
		lrc:                lrc,
	}
}

// lintTraitMap indexes the trait definitions by "Kind:Name" so a same-name Trait and
// ClusterTrait can coexist.
func lintTraitMap(traits []v1alpha1.Trait) map[string]*v1alpha1.Trait {
	traitMap := make(map[string]*v1alpha1.Trait, len(traits))
	for i := range traits {
		t := &traits[i]
		kind := t.Kind // TypeMeta.Kind — "Trait" or "ClusterTrait"
		if kind == "" {
			kind = string(v1alpha1.TraitRefKindTrait)
		}
		traitMap[kind+":"+t.Name] = t
	}
	return traitMap
}

// processEmbeddedTraits applies every ComponentType-embedded trait to the rendered
// resource set, accumulating trait post-render validations and metadata counts.
func (r *lintTraitRun) processEmbeddedTraits(ctx context.Context, input *RenderInput, metadata *RenderMetadata) error {
	for _, embeddedTrait := range input.ComponentType.Spec.Traits {
		embeddedKind := string(embeddedTrait.Kind)
		if embeddedKind == "" {
			embeddedKind = string(v1alpha1.TraitRefKindTrait)
		}
		t, ok := r.traitMap[embeddedKind+":"+embeddedTrait.Name]
		if !ok {
			return fmt.Errorf("embedded trait %s referenced but not found in traits list", embeddedTrait.Name)
		}

		// Resolve CEL bindings against component context
		resolvedParams, resolvedEnvironmentConfigs, err := pipelinecontext.ResolveEmbeddedTraitBindings(
			ctx,
			r.engine,
			embeddedTrait,
			r.lrc.componentContextMap,
		)
		if err != nil {
			return fmt.Errorf("failed to resolve embedded trait bindings for %s/%s: %w",
				embeddedTrait.Name, embeddedTrait.InstanceName, err)
		}

		traitContext, err := pipelinecontext.BuildTraitContext(&pipelinecontext.TraitContextInput{
			TraitContextBase:           r.lrc.traitBase,
			Trait:                      t,
			InstanceName:               embeddedTrait.InstanceName,
			ResolvedParameters:         resolvedParams,
			ResolvedEnvironmentConfigs: resolvedEnvironmentConfigs,
			SchemaCache:                r.schemaCache,
		})
		if err != nil {
			return fmt.Errorf("failed to build embedded trait context for %s/%s: %w",
				embeddedTrait.Name, embeddedTrait.InstanceName, err)
		}

		traitContextMap := traitContext.ToMap()
		if err := renderer.EvaluateValidationRules(ctx, r.engine, t.Spec.EffectivePreRenderValidations(), traitContextMap); err != nil {
			return fmt.Errorf("trait %s/%s validation failed: %w",
				embeddedTrait.Name, embeddedTrait.InstanceName, err)
		}

		beforeCount := len(r.rendered)
		r.rendered, err = r.processor.ProcessTraits(ctx, r.rendered, t, traitContextMap)
		if err != nil {
			return fmt.Errorf("failed to process embedded trait %s/%s: %w",
				embeddedTrait.Name, embeddedTrait.InstanceName, err)
		}

		r.addPendingPostRender(embeddedKind, embeddedTrait.Name, embeddedTrait.InstanceName, traitContextMap, t)
		metadata.TraitCount++
		metadata.TraitResourceCount += len(r.rendered) - beforeCount
	}
	return nil
}

// processComponentTraits applies each component-level trait instance to the rendered
// resource set, accumulating trait post-render validations and metadata counts.
func (r *lintTraitRun) processComponentTraits(ctx context.Context, input *RenderInput, metadata *RenderMetadata) error {
	for _, traitInstance := range input.Component.Spec.Traits {
		instanceKind := string(traitInstance.Kind)
		if instanceKind == "" {
			instanceKind = string(v1alpha1.TraitRefKindTrait)
		}
		t, ok := r.traitMap[instanceKind+":"+traitInstance.Name]
		if !ok {
			return fmt.Errorf("trait %s referenced but not found in traits list", traitInstance.Name)
		}

		// Resolve the component-level trait's instance bindings (just JSON deserialization)
		resolvedParams, resolvedEnvironmentConfigs, err := pipelinecontext.ExtractTraitInstanceBindings(traitInstance, input.ReleaseBinding)
		if err != nil {
			return fmt.Errorf("failed to extract trait bindings for %s/%s: %w",
				traitInstance.Name, traitInstance.InstanceName, err)
		}

		traitContext, err := pipelinecontext.BuildTraitContext(&pipelinecontext.TraitContextInput{
			TraitContextBase:           r.lrc.traitBase,
			Trait:                      t,
			InstanceName:               traitInstance.InstanceName,
			ResolvedParameters:         resolvedParams,
			ResolvedEnvironmentConfigs: resolvedEnvironmentConfigs,
			SchemaCache:                r.schemaCache,
		})
		if err != nil {
			return fmt.Errorf("failed to build trait context for %s/%s: %w",
				traitInstance.Name, traitInstance.InstanceName, err)
		}

		traitContextMap := traitContext.ToMap()
		if err := renderer.EvaluateValidationRules(ctx, r.engine, t.Spec.EffectivePreRenderValidations(), traitContextMap); err != nil {
			return fmt.Errorf("trait %s/%s validation failed: %w",
				traitInstance.Name, traitInstance.InstanceName, err)
		}

		beforeCount := len(r.rendered)
		r.rendered, err = r.processor.ProcessTraits(ctx, r.rendered, t, traitContextMap)
		if err != nil {
			return fmt.Errorf("failed to process trait %s/%s: %w",
				traitInstance.Name, traitInstance.InstanceName, err)
		}

		r.addPendingPostRender(instanceKind, traitInstance.Name, traitInstance.InstanceName, traitContextMap, t)
		metadata.TraitCount++
		metadata.TraitResourceCount += len(r.rendered) - beforeCount
	}
	return nil
}

// addPendingPostRender records a trait's declared post-render validations (if any) so
// they are evaluated against the final resource set.
func (r *lintTraitRun) addPendingPostRender(kind, name, instance string, ctxMap map[string]any, t *v1alpha1.Trait) {
	if len(t.Spec.PostRenderValidations) == 0 {
		return
	}
	r.pendingPostRenders = append(r.pendingPostRenders, pendingPostRender{
		label:       fmt.Sprintf("%s %s/%s", kind, name, instance),
		context:     ctxMap,
		validations: t.Spec.PostRenderValidations,
	})
}

// finalizeLintRender post-processes, validates and sorts the rendered resources,
// returning the output alongside the captured post-render failures.
func (p *Pipeline) finalizeLintRender(
	input *RenderInput,
	resources []renderer.RenderedResource,
	failures []PostRenderFailure,
	metadata *RenderMetadata,
) (*RenderOutput, []PostRenderFailure, error) {
	if err := p.postProcessResources(resources, input); err != nil {
		return nil, failures, fmt.Errorf("failed to post-process resources: %w", err)
	}

	if err := p.validateResources(resources); err != nil {
		return nil, failures, fmt.Errorf("validation failed: %w", err)
	}

	sortRenderedResources(resources)

	metadata.ResourceCount = len(resources)

	return &RenderOutput{
		Resources: resources,
		Metadata:  metadata,
	}, failures, nil
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
// workloads containing a hash of all non-workload dataplane resources. This triggers pod
// rollout when ConfigMaps, Secrets, or other dependent resources change.
func (p *Pipeline) addDPResourceHashAnnotation(resources []renderer.RenderedResource, input *RenderInput) error {
	workloadType := input.ComponentType.Spec.WorkloadType

	// Only apply to deployment and statefulset workload types
	if workloadType != workloadTypeDeployment && workloadType != workloadTypeStatefulSet {
		return nil
	}

	// Collect all non-workload dataplane resources
	resourcesToHash := make([]map[string]any, 0, len(resources))
	for _, rr := range resources {
		// Skip non-dataplane resources
		if rr.TargetPlane != v1alpha1.TargetPlaneDataPlane {
			continue
		}

		// Skip the main workload resource
		kind, _ := rr.Resource["kind"].(string)
		if isMainWorkloadKind(kind, workloadType) {
			continue
		}

		resourcesToHash = append(resourcesToHash, rr.Resource)
	}

	// If no non-workload dataplane resources, nothing to hash
	if len(resourcesToHash) == 0 {
		return nil
	}

	// Sort resources to ensure deterministic hash regardless of resource ordering
	sort.SliceStable(resourcesToHash, func(i, j int) bool {
		return compareResources(resourcesToHash[i], resourcesToHash[j])
	})

	// Extract content excluding metadata for hashing
	hashContent := make([]map[string]any, 0, len(resourcesToHash))
	for _, resource := range resourcesToHash {
		hashContent = append(hashContent, extractContentExcludingMetadata(resource))
	}

	// Compute hash
	resourceHash := hash.ComputeHash(hashContent, nil)

	// Find the main workload resource and add annotation to pod template
	for _, rr := range resources {
		kind, _ := rr.Resource["kind"].(string)
		if !isMainWorkloadKind(kind, workloadType) {
			continue
		}

		if err := addPodTemplateAnnotation(rr.Resource, labels.AnnotationKeyDPResourceHash, resourceHash); err != nil {
			return fmt.Errorf("failed to add annotation to %s: %w", kind, err)
		}
		break // Only one main workload
	}

	return nil
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
