// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package resourcereleasebinding

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	apiequality "k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/controller"
	dpkubernetes "github.com/openchoreo/openchoreo/internal/dataplane/kubernetes"
	"github.com/openchoreo/openchoreo/internal/labels"
	resourcepipeline "github.com/openchoreo/openchoreo/internal/pipeline/resource"
)

const ownershipConflictMarker = "RenderedRelease exists but is not owned by this binding"

// Reconciler reconciles a ResourceReleaseBinding object.
type Reconciler struct {
	client.Client
	Scheme *runtime.Scheme

	// Pipeline renders ResourceType templates and resolves outputs. The
	// instance holds CEL env and program caches; reuse it across reconciles
	// to keep them warm.
	Pipeline *resourcepipeline.Pipeline
}

// +kubebuilder:rbac:groups=openchoreo.dev,resources=resourcereleasebindings,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=openchoreo.dev,resources=resourcereleasebindings/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=openchoreo.dev,resources=resourcereleasebindings/finalizers,verbs=update
// +kubebuilder:rbac:groups=openchoreo.dev,resources=resourcereleases,verbs=get;list;watch
// +kubebuilder:rbac:groups=openchoreo.dev,resources=environments,verbs=get;list;watch
// +kubebuilder:rbac:groups=openchoreo.dev,resources=dataplanes,verbs=get;list;watch
// +kubebuilder:rbac:groups=openchoreo.dev,resources=clusterdataplanes,verbs=get;list;watch
// +kubebuilder:rbac:groups=openchoreo.dev,resources=renderedreleases,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop.
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	binding := &openchoreov1alpha1.ResourceReleaseBinding{}
	if err := r.Get(ctx, req.NamespacedName, binding); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Failed to get ResourceReleaseBinding")
		return ctrl.Result{}, err
	}

	old := binding.DeepCopy()

	if !binding.DeletionTimestamp.IsZero() {
		return r.finalize(ctx, old, binding)
	}

	if added, err := r.ensureFinalizer(ctx, binding); err != nil || added {
		return ctrl.Result{}, err
	}

	return r.reconcile(ctx, old, binding)
}

// reconcile is the live-object path: validate, render, emit RenderedRelease,
// resolve outputs and readiness.
func (r *Reconciler) reconcile(ctx context.Context, old, binding *openchoreov1alpha1.ResourceReleaseBinding) (result ctrl.Result, rErr error) {
	logger := log.FromContext(ctx)

	// Deferred status write: aggregate Ready (so every exit path produces it),
	// skip the API call when nothing changed, aggregate errors with any
	// returned by the body. observedGeneration is set per-condition by
	// MarkTrueCondition / MarkFalseCondition (project convention); the status
	// type has no top-level ObservedGeneration field.
	defer func() {
		r.setReadyCondition(binding)
		if apiequality.Semantic.DeepEqual(old.Status, binding.Status) {
			return
		}
		if err := r.Status().Update(ctx, binding); err != nil {
			logger.Error(err, "Failed to update ResourceReleaseBinding status")
			rErr = kerrors.NewAggregate([]error{rErr, err})
		}
	}()

	if binding.Spec.ResourceRelease == "" {
		markSyncedFalse(binding, ReasonResourceReleaseNotSet,
			"spec.resourceRelease is unset; pin a ResourceRelease to deploy this binding")
		return ctrl.Result{}, nil
	}

	release := &openchoreov1alpha1.ResourceRelease{}
	if err := r.Get(ctx, types.NamespacedName{
		Name:      binding.Spec.ResourceRelease,
		Namespace: binding.Namespace,
	}, release); err != nil {
		if apierrors.IsNotFound(err) {
			markSyncedFalse(binding, ReasonResourceReleaseNotFound,
				fmt.Sprintf("ResourceRelease %q not found", binding.Spec.ResourceRelease))
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Failed to get ResourceRelease", "resourceRelease", binding.Spec.ResourceRelease)
		return ctrl.Result{}, err
	}

	if err := validateReleaseOwner(release, binding); err != nil {
		markSyncedFalse(binding, ReasonInvalidReleaseConfiguration, err.Error())
		return ctrl.Result{}, nil
	}

	environment := &openchoreov1alpha1.Environment{}
	if err := r.Get(ctx, types.NamespacedName{
		Name:      binding.Spec.Environment,
		Namespace: binding.Namespace,
	}, environment); err != nil {
		if apierrors.IsNotFound(err) {
			markSyncedFalse(binding, ReasonEnvironmentNotFound,
				fmt.Sprintf("Environment %q not found", binding.Spec.Environment))
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Failed to get Environment", "environment", binding.Spec.Environment)
		return ctrl.Result{}, err
	}

	dataPlaneResult, err := controller.GetDataPlaneFromRef(ctx, r.Client, environment.Namespace, environment.Spec.DataPlaneRef)
	if err != nil {
		if apierrors.IsNotFound(err) {
			markSyncedFalse(binding, ReasonDataPlaneNotFound,
				fmt.Sprintf("DataPlane not found for environment %q", environment.Name))
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Failed to resolve DataPlane", "environment", environment.Name)
		return ctrl.Result{}, err
	}
	dataPlane := dataPlaneResult.ToDataPlane()

	// Fetch owning Resource and Project for their UIDs (used in label
	// metadata exposed to PE templates and applied to DP-side objects).
	// Mirrors the releasebinding precedent at controller.go:178-209.
	resource := &openchoreov1alpha1.Resource{}
	if err := r.Get(ctx, types.NamespacedName{
		Name:      binding.Spec.Owner.ResourceName,
		Namespace: binding.Namespace,
	}, resource); err != nil {
		if apierrors.IsNotFound(err) {
			markSyncedFalse(binding, ReasonResourceNotFound,
				fmt.Sprintf("Resource %q not found", binding.Spec.Owner.ResourceName))
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Failed to get Resource", "resource", binding.Spec.Owner.ResourceName)
		return ctrl.Result{}, err
	}

	project := &openchoreov1alpha1.Project{}
	if err := r.Get(ctx, types.NamespacedName{
		Name:      binding.Spec.Owner.ProjectName,
		Namespace: binding.Namespace,
	}, project); err != nil {
		if apierrors.IsNotFound(err) {
			markSyncedFalse(binding, ReasonProjectNotFound,
				fmt.Sprintf("Project %q not found", binding.Spec.Owner.ProjectName))
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Failed to get Project", "project", binding.Spec.Owner.ProjectName)
		return ctrl.Result{}, err
	}

	rr, err := r.renderAndEmit(ctx, binding, release, environment, dataPlane, resource, project)
	if err != nil {
		return ctrl.Result{}, err
	}
	if rr == nil {
		// Synced is False; the deferred aggregator handles Ready.
		return ctrl.Result{}, nil
	}

	r.evaluateReadiness(ctx, binding, release, environment, dataPlane, resource, project, rr)
	return ctrl.Result{}, nil
}

// renderAndEmit drives the pipeline against the snapshot and writes the
// resulting RenderedRelease. Returns the live RenderedRelease on success
// (Created, Updated, or already in sync) or nil when no further evaluation
// should run (render error, ownership conflict). Transient API errors are
// returned to the caller for requeue.
func (r *Reconciler) renderAndEmit(
	ctx context.Context,
	binding *openchoreov1alpha1.ResourceReleaseBinding,
	release *openchoreov1alpha1.ResourceRelease,
	environment *openchoreov1alpha1.Environment,
	dataPlane *openchoreov1alpha1.DataPlane,
	resource *openchoreov1alpha1.Resource,
	project *openchoreov1alpha1.Project,
) (*openchoreov1alpha1.RenderedRelease, error) {
	logger := log.FromContext(ctx)

	resourceType := buildResourceTypeFromRelease(release)
	snapshotResource := buildResourceFromRelease(release)
	metadataCtx := buildMetadataContext(binding, environment, dataPlane, resource, project)
	dpCtx := buildDataPlaneContext(dataPlane)

	input := &resourcepipeline.RenderInput{
		ResourceType:           resourceType,
		Resource:               snapshotResource,
		ResourceReleaseBinding: binding,
		Metadata:               metadataCtx,
		DataPlane:              dpCtx,
	}

	output, err := r.Pipeline.RenderManifests(input)
	if err != nil {
		markSyncedFalse(binding, ReasonRenderingFailed,
			fmt.Sprintf("Failed to render manifests: %v", err))
		logger.Info("Pipeline render failed", "error", err)
		return nil, nil
	}

	manifests, err := convertEntriesToManifests(output.Entries)
	if err != nil {
		markSyncedFalse(binding, ReasonRenderingFailed,
			fmt.Sprintf("Failed to encode rendered manifests: %v", err))
		return nil, err
	}

	rrName := makeRenderedReleaseName(binding)
	rr := &openchoreov1alpha1.RenderedRelease{
		ObjectMeta: metav1.ObjectMeta{
			Name:      rrName,
			Namespace: binding.Namespace,
		},
	}

	op, err := controllerutil.CreateOrUpdate(ctx, r.Client, rr, func() error {
		if rr.UID != "" {
			hasOwner, ownerErr := controllerutil.HasOwnerReference(rr.GetOwnerReferences(), binding, r.Scheme)
			if ownerErr != nil {
				return fmt.Errorf("check owner reference: %w", ownerErr)
			}
			if !hasOwner {
				return fmt.Errorf("%s: %q", ownershipConflictMarker, rrName)
			}
		}

		rr.Labels = map[string]string{
			labels.LabelKeyNamespaceName:   binding.Namespace,
			labels.LabelKeyProjectName:     binding.Spec.Owner.ProjectName,
			labels.LabelKeyResourceName:    binding.Spec.Owner.ResourceName,
			labels.LabelKeyEnvironmentName: binding.Spec.Environment,
		}
		rr.Spec = openchoreov1alpha1.RenderedReleaseSpec{
			Owner: openchoreov1alpha1.RenderedReleaseOwner{
				ProjectName:  binding.Spec.Owner.ProjectName,
				ResourceName: binding.Spec.Owner.ResourceName,
			},
			EnvironmentName: binding.Spec.Environment,
			TargetPlane:     openchoreov1alpha1.TargetPlaneDataPlane,
			Resources:       manifests,
		}
		return controllerutil.SetControllerReference(binding, rr, r.Scheme)
	})

	if err != nil {
		if strings.Contains(err.Error(), ownershipConflictMarker) {
			markSyncedFalse(binding, ReasonReleaseOwnershipConflict, err.Error())
			return nil, nil
		}
		markSyncedFalse(binding, ReasonReleaseUpdateFailed,
			fmt.Sprintf("Failed to reconcile RenderedRelease %q: %v", rrName, err))
		return nil, err
	}

	switch op {
	case controllerutil.OperationResultCreated, controllerutil.OperationResultUpdated:
		controller.MarkTrueCondition(binding, ConditionSynced, ReasonReleaseCreated,
			fmt.Sprintf("RenderedRelease %q %s with %d resource(s)", rrName, op, len(manifests)))
	case controllerutil.OperationResultNone:
		controller.MarkTrueCondition(binding, ConditionSynced, ReasonReleaseSynced,
			fmt.Sprintf("RenderedRelease %q is up to date", rrName))
	}

	return rr, nil
}

// makeRenderedReleaseName returns the RenderedRelease name for a binding.
// Mirrors buildMetadataContext's "r_"-discriminator + hash pattern so the
// RenderedRelease object name is collision-safe against (a) component-side
// RenderedReleases and (b) other Resource bindings whose hyphenated
// resource/env names would otherwise produce the same flat join.
func makeRenderedReleaseName(binding *openchoreov1alpha1.ResourceReleaseBinding) string {
	return dpkubernetes.GenerateK8sName("r_"+binding.Spec.Owner.ResourceName, binding.Spec.Environment)
}

// convertEntriesToManifests marshals each pipeline-rendered map into a
// RenderedManifest with its ID preserved verbatim. The pipeline already
// supplies IDs from ResourceType.spec.resources[].id; this controller does
// not regenerate them from kind/name.
func convertEntriesToManifests(entries []resourcepipeline.RenderedEntry) ([]openchoreov1alpha1.RenderedManifest, error) {
	manifests := make([]openchoreov1alpha1.RenderedManifest, 0, len(entries))
	for i := range entries {
		raw, err := json.Marshal(entries[i].Object)
		if err != nil {
			return nil, fmt.Errorf("marshal rendered manifest %q: %w", entries[i].ID, err)
		}
		manifests = append(manifests, openchoreov1alpha1.RenderedManifest{
			ID:     entries[i].ID,
			Object: &runtime.RawExtension{Raw: raw},
		})
	}
	return manifests, nil
}

// buildResourceTypeFromRelease rehydrates a ResourceType view from the
// snapshot. Name is a placeholder; the pipeline does not consume it.
// Mirrors releasebinding.buildComponentTypeFromRelease.
func buildResourceTypeFromRelease(release *openchoreov1alpha1.ResourceRelease) *openchoreov1alpha1.ResourceType {
	return &openchoreov1alpha1.ResourceType{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "from-release",
			Namespace: release.Namespace,
		},
		Spec: release.Spec.ResourceType.Spec,
	}
}

// buildResourceFromRelease rehydrates a Resource view from the snapshot.
// Carries the owner project, the original type ref, and the snapshotted
// parameters so the pipeline sees the same surface a live Resource would
// expose.
func buildResourceFromRelease(release *openchoreov1alpha1.ResourceRelease) *openchoreov1alpha1.Resource {
	return &openchoreov1alpha1.Resource{
		ObjectMeta: metav1.ObjectMeta{
			Name:      release.Spec.Owner.ResourceName,
			Namespace: release.Namespace,
		},
		Spec: openchoreov1alpha1.ResourceSpec{
			Owner: openchoreov1alpha1.ResourceOwner{
				ProjectName: release.Spec.Owner.ProjectName,
			},
			Type: openchoreov1alpha1.ResourceTypeRef{
				Kind: release.Spec.ResourceType.Kind,
				Name: release.Spec.ResourceType.Name,
			},
			Parameters: release.Spec.Parameters,
		},
	}
}

// buildMetadataContext computes the platform-injected metadata surface
// exposed to CEL templates. Name and Namespace follow the platform naming
// scheme; the remaining fields are populated from the binding, environment,
// dataplane, owning Resource, and owning Project.
//
// The base name uses an "r_" discriminator on the first hash input. The
// underscore is non-alphanumeric, so dpkubernetes.sanitizeName replaces it
// with "-" in the visible name (giving an "r-" prefix) but the original
// string with the underscore drives the SHA hash. K8s name validation
// forbids "_" in user-supplied names, so a Component happening to be named
// "r-{resource}" hashes a different input string and produces a different
// final name despite the matching visible base. Operators distinguish
// owners further via labels.
func buildMetadataContext(
	binding *openchoreov1alpha1.ResourceReleaseBinding,
	environment *openchoreov1alpha1.Environment,
	dataPlane *openchoreov1alpha1.DataPlane,
	resource *openchoreov1alpha1.Resource,
	project *openchoreov1alpha1.Project,
) resourcepipeline.MetadataContext {
	resourceName := binding.Spec.Owner.ResourceName
	projectName := binding.Spec.Owner.ProjectName
	envName := binding.Spec.Environment
	bindingNamespace := binding.Namespace

	baseName := dpkubernetes.GenerateK8sName("r_"+resourceName, envName)
	dpNamespace := dpkubernetes.GenerateK8sNameWithLengthLimit(
		dpkubernetes.MaxNamespaceNameLength,
		"dp", bindingNamespace, projectName, envName,
	)

	standardLabels := map[string]string{
		labels.LabelKeyNamespaceName:   bindingNamespace,
		labels.LabelKeyProjectName:     projectName,
		labels.LabelKeyProjectUID:      string(project.UID),
		labels.LabelKeyResourceName:    resourceName,
		labels.LabelKeyResourceUID:     string(resource.UID),
		labels.LabelKeyEnvironmentName: envName,
		labels.LabelKeyEnvironmentUID:  string(environment.UID),
	}

	return resourcepipeline.MetadataContext{
		Name:              baseName,
		Namespace:         dpNamespace,
		ResourceNamespace: bindingNamespace,
		ResourceName:      resourceName,
		ResourceUID:       string(resource.UID),
		ProjectName:       projectName,
		ProjectUID:        string(project.UID),
		EnvironmentName:   envName,
		EnvironmentUID:    string(environment.UID),
		DataPlaneName:     dataPlane.Name,
		DataPlaneUID:      string(dataPlane.UID),
		Labels:            standardLabels,
		Annotations:       binding.Annotations,
	}
}

// buildDataPlaneContext exposes the dataplane fields the resource pipeline
// surfaces to CEL templates: secretStore and observabilityPlaneRef.
func buildDataPlaneContext(dataPlane *openchoreov1alpha1.DataPlane) resourcepipeline.DataPlaneContext {
	dpCtx := resourcepipeline.DataPlaneContext{}
	if dataPlane.Spec.SecretStoreRef != nil {
		dpCtx.SecretStore = dataPlane.Spec.SecretStoreRef.Name
	}
	if dataPlane.Spec.ObservabilityPlaneRef != nil {
		dpCtx.ObservabilityPlaneRef = &resourcepipeline.ObservabilityPlaneRefContext{
			Kind: string(dataPlane.Spec.ObservabilityPlaneRef.Kind),
			Name: dataPlane.Spec.ObservabilityPlaneRef.Name,
		}
	}
	return dpCtx
}

// validateReleaseOwner enforces that the binding and the pinned
// ResourceRelease agree on which Resource they belong to. A mismatch usually
// means the binding's spec.resourceRelease points at a release cut for a
// different Resource (typo, copy-paste mistake, or stale reference).
func validateReleaseOwner(release *openchoreov1alpha1.ResourceRelease, binding *openchoreov1alpha1.ResourceReleaseBinding) error {
	if release.Spec.Owner.ProjectName != binding.Spec.Owner.ProjectName ||
		release.Spec.Owner.ResourceName != binding.Spec.Owner.ResourceName {
		return fmt.Errorf("binding owner (project: %q, resource: %q) does not match ResourceRelease owner (project: %q, resource: %q)",
			binding.Spec.Owner.ProjectName, binding.Spec.Owner.ResourceName,
			release.Spec.Owner.ProjectName, release.Spec.Owner.ResourceName)
	}
	return nil
}

// SetupWithManager sets up the controller with the Manager. The watch
// surface:
//   - For: ResourceReleaseBinding (primary).
//   - Owns: RenderedRelease — re-enqueues on DP-side status updates so
//     readiness and outputs flow back into the binding.
//   - Watches: ResourceRelease — ResourceReleases are immutable, so only
//     Create and Delete events fire. Create wakes a binding that was
//     authored before its pinned release existed (the GitOps "apply
//     everything at once" race). Delete surfaces failure quickly when a
//     live binding's pinned release is removed; in the normal teardown
//     path bindings are already deleting under their own finalizer by
//     the time the Resource finalizer cascades the snapshot.
//
// (Cluster)ResourceType is intentionally not watched. Rendering reads from
// the immutable ResourceRelease.spec.resourceType.spec snapshot, so live
// template edits do not affect already-pinned bindings — only a new
// ResourceRelease cut by the Resource controller plus a manual pin
// advance moves a binding forward.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	if r.Pipeline == nil {
		r.Pipeline = resourcepipeline.NewPipeline()
	}

	if err := r.setupResourceReleaseRefIndex(context.Background(), mgr); err != nil {
		return fmt.Errorf("setup ResourceRelease ref index: %w", err)
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&openchoreov1alpha1.ResourceReleaseBinding{}).
		Owns(&openchoreov1alpha1.RenderedRelease{}).
		Watches(&openchoreov1alpha1.ResourceRelease{},
			handler.EnqueueRequestsFromMapFunc(r.listResourceReleaseBindingsForResourceRelease)).
		Named("resourcereleasebinding").
		Complete(r)
}
