// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package releaseline

import (
	"context"
	"fmt"
	"sort"

	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/controller"
	"github.com/openchoreo/openchoreo/internal/controller/component"
	rlconfig "github.com/openchoreo/openchoreo/internal/releaseline"
)

// Reconciler reconciles a ReleaseLine: it applies the line's overrides to its Component, releases the
// line's Workload and, with autoDeploy, binds the release in the first environment of the pipeline.
type Reconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder

	steps *component.Steps
}

// +kubebuilder:rbac:groups=openchoreo.dev,resources=releaselines,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=openchoreo.dev,resources=releaselines/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=openchoreo.dev,resources=releaselines/finalizers,verbs=update
// +kubebuilder:rbac:groups=openchoreo.dev,resources=components,verbs=get;list;watch
// +kubebuilder:rbac:groups=openchoreo.dev,resources=workloads,verbs=get;list;watch;delete
// +kubebuilder:rbac:groups=openchoreo.dev,resources=componentreleases,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=openchoreo.dev,resources=releasebindings,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=openchoreo.dev,resources=componenttypes;clustercomponenttypes;traits;clustertraits;workflows;clusterworkflows,verbs=get;list;watch
// +kubebuilder:rbac:groups=openchoreo.dev,resources=projects;deploymentpipelines,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch

// Reconcile reconciles one ReleaseLine.
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (result ctrl.Result, rErr error) {
	logger := log.FromContext(ctx)

	line := &openchoreov1alpha1.ReleaseLine{}
	if err := r.Get(ctx, req.NamespacedName, line); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !line.DeletionTimestamp.IsZero() {
		return r.finalize(ctx, line)
	}

	if controllerutil.AddFinalizer(line, ReleaseLineFinalizer) {
		return ctrl.Result{}, r.Update(ctx, line)
	}

	old := line.DeepCopy()
	defer func() {
		line.Status.ObservedGeneration = line.Generation
		if apiequality.Semantic.DeepEqual(old.Status, line.Status) {
			return
		}
		if err := r.Status().Update(ctx, line); err != nil {
			logger.Error(err, "Failed to update ReleaseLine status")
			rErr = kerrors.NewAggregate([]error{rErr, err})
		}
	}()

	return ctrl.Result{}, r.reconcileLine(ctx, line)
}

func (r *Reconciler) reconcileLine(ctx context.Context, line *openchoreov1alpha1.ReleaseLine) error {
	comp := &openchoreov1alpha1.Component{}
	if err := r.Get(ctx, types.NamespacedName{Namespace: line.Namespace, Name: line.Spec.Owner.ComponentName}, comp); err != nil {
		if apierrors.IsNotFound(err) {
			controller.MarkFalseCondition(line, ConditionReady, ReasonComponentNotFound,
				fmt.Sprintf("Component %q not found", line.Spec.Owner.ComponentName))
			return nil
		}
		return fmt.Errorf("failed to get Component: %w", err)
	}
	if comp.Spec.Owner.ProjectName != line.Spec.Owner.ProjectName {
		controller.MarkFalseCondition(line, ConditionReady, ReasonInvalidConfiguration,
			fmt.Sprintf("Component %q belongs to project %q, not %q", comp.Name, comp.Spec.Owner.ProjectName, line.Spec.Owner.ProjectName))
		return nil
	}
	if !comp.Spec.ReleaseLinesEnabled {
		controller.MarkFalseCondition(line, ConditionReady, ReasonReleaseLinesNotEnabled,
			fmt.Sprintf("Component %q does not have spec.releaseLinesEnabled; create a new component with it set to use release lines", comp.Name))
		return nil
	}
	if !comp.DeletionTimestamp.IsZero() {
		return nil
	}

	envs, err := r.environments(ctx, comp, line.Name)
	if err != nil {
		return err
	}
	line.Status.Environments = envs

	effective, err := rlconfig.EffectiveComponent(comp, line)
	if err != nil {
		controller.MarkFalseCondition(line, ConditionReady, ReasonInvalidConfiguration, err.Error())
		return nil
	}
	// The component steps record failures on the effective Component; lift them onto the line.
	liftFailure := func() {
		if c := meta.FindStatusCondition(effective.Status.Conditions, string(ConditionReady)); c != nil {
			controller.MarkFalseCondition(line, ConditionReady, controller.ConditionReason(c.Reason), c.Message)
		}
	}

	ct, err := r.steps.ComponentType(ctx, effective)
	if err != nil {
		return err
	}
	if ct == nil {
		liftFailure()
		return nil
	}
	if !ct.Spec.ReleaseLines.AllowsReleaseLines() {
		controller.MarkFalseCondition(line, ConditionReady, ReasonReleaseLinesNotAllowed,
			fmt.Sprintf("%s %q does not allow release lines; set spec.releaseLines to Allowed on the component type",
				comp.Spec.ComponentType.Kind, comp.Spec.ComponentType.Name))
		return nil
	}
	if ok, err := r.steps.ValidWorkflow(ctx, effective, ct); err != nil || !ok {
		liftFailure()
		return err
	}
	firstEnv, err := r.steps.FirstEnvironment(ctx, effective)
	if err != nil || firstEnv == "" {
		liftFailure()
		return err
	}

	workload, err := r.workload(ctx, comp, line)
	if err != nil || workload == nil {
		return err
	}

	if !r.steps.ValidTraits(ctx, effective, ct) {
		liftFailure()
		return nil
	}
	traits, clusterTraits, err := r.steps.Traits(ctx, ct, effective)
	if err != nil {
		if name, ok := component.TraitNotFound(err); ok {
			controller.MarkFalseCondition(line, ConditionReady, ReasonTraitNotFound, fmt.Sprintf("Trait %q not found", name))
			return nil
		}
		return err
	}

	autoDeploy := comp.Spec.AutoDeploy
	if line.Spec.AutoDeploy != nil {
		autoDeploy = *line.Spec.AutoDeploy
	}
	if !autoDeploy {
		controller.MarkTrueCondition(line, ConditionReady, ReasonReconciled, "ReleaseLine validated successfully")
		return nil
	}

	latest, err := r.steps.AutoDeploy(ctx, effective, ct, workload, traits, clusterTraits, firstEnv, line.Name, line.Status.LatestRelease)
	line.Status.LatestRelease = latest
	if err != nil {
		msg := fmt.Sprintf("Failed to handle autoDeploy: %v", err)
		controller.MarkFalseCondition(line, ConditionReady, ReasonAutoDeployFailed, msg)
		if apierrors.IsInvalid(err) || apierrors.IsBadRequest(err) {
			r.Recorder.Event(line, corev1.EventTypeWarning, string(ReasonAutoDeployFailed), msg)
			return nil
		}
		return err
	}

	controller.MarkTrueCondition(line, ConditionReady, ReasonComponentReleaseReady,
		fmt.Sprintf("ComponentRelease %q and ReleaseBinding %q successfully managed for environment %q",
			latest.Name, rlconfig.ScopedName(comp.Name, line.Name, firstEnv), firstEnv))
	return nil
}

// workload returns the line's single Workload, or nil with a condition set on the line.
func (r *Reconciler) workload(
	ctx context.Context,
	comp *openchoreov1alpha1.Component,
	line *openchoreov1alpha1.ReleaseLine,
) (*openchoreov1alpha1.Workload, error) {
	all, err := r.steps.Workloads(ctx, comp)
	if err != nil {
		return nil, err
	}
	var items []openchoreov1alpha1.Workload
	for i := range all {
		if all[i].Spec.Owner.ReleaseLine == line.Name {
			items = append(items, all[i])
		}
	}
	switch len(items) {
	case 0:
		controller.MarkFalseCondition(line, ConditionReady, ReasonWorkloadNotFound,
			fmt.Sprintf("Workload for release line %q not found, waiting for workload to be created", line.Name))
		return nil, nil
	case 1:
		return &items[0], nil
	default:
		controller.MarkFalseCondition(line, ConditionReady, ReasonInvalidConfiguration,
			fmt.Sprintf("Multiple Workloads found for release line %q (expected exactly 1)", line.Name))
		return nil, nil
	}
}

// environments summarizes the line's ReleaseBindings, one entry per environment.
func (r *Reconciler) environments(ctx context.Context, comp *openchoreov1alpha1.Component, line string) ([]openchoreov1alpha1.ReleaseLineEnvironment, error) {
	var bindings openchoreov1alpha1.ReleaseBindingList
	if err := r.List(ctx, &bindings, client.InNamespace(comp.Namespace),
		client.MatchingFields{controller.IndexKeyReleaseBindingOwnerComponentName: comp.Name}); err != nil {
		return nil, fmt.Errorf("failed to list ReleaseBindings: %w", err)
	}
	return lineEnvironments(bindings.Items, comp.Spec.Owner.ProjectName, line), nil
}

// lineEnvironments summarizes the line's ReleaseBindings, one entry per environment, sorted by environment.
func lineEnvironments(bindings []openchoreov1alpha1.ReleaseBinding, projectName, line string) []openchoreov1alpha1.ReleaseLineEnvironment {
	var envs []openchoreov1alpha1.ReleaseLineEnvironment
	seen := map[string]bool{}
	for i := range bindings {
		rb := &bindings[i]
		if rb.Spec.Owner.ProjectName != projectName || rb.Spec.Owner.ReleaseLine != line || seen[rb.Spec.Environment] {
			continue
		}
		seen[rb.Spec.Environment] = true
		envs = append(envs, openchoreov1alpha1.ReleaseLineEnvironment{
			Environment:    rb.Spec.Environment,
			ReleaseBinding: rb.Name,
			Release:        rb.Spec.ReleaseName,
			Ready:          meta.IsStatusConditionTrue(rb.Status.Conditions, string(ConditionReady)),
		})
	}
	sort.Slice(envs, func(i, j int) bool { return envs[i].Environment < envs[j].Environment })
	return envs
}
