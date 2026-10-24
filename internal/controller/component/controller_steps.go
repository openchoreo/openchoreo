// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package component

import (
	"context"
	"errors"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

// Steps exposes the component reconcile steps that the ReleaseLine controller runs for each line,
// against the line's effective Component. Validation failures are recorded as Ready conditions on
// the Component passed in.
type Steps struct {
	r *Reconciler
}

// NewSteps returns Steps backed by the given client.
func NewSteps(c client.Client, scheme *runtime.Scheme, recorder record.EventRecorder) *Steps {
	return &Steps{r: &Reconciler{Client: c, Scheme: scheme, Recorder: recorder}}
}

// ComponentType returns the component's ComponentType, or nil when it is invalid or missing.
func (s *Steps) ComponentType(ctx context.Context, comp *openchoreov1alpha1.Component) (*openchoreov1alpha1.ComponentType, error) {
	return s.r.validateAndFetchComponentType(ctx, comp)
}

// ValidWorkflow reports whether the component's workflow, if any, is valid for the ComponentType.
func (s *Steps) ValidWorkflow(ctx context.Context, comp *openchoreov1alpha1.Component, ct *openchoreov1alpha1.ComponentType) (bool, error) {
	wf, err := s.r.validateWorkflow(ctx, comp, ct)
	if err != nil {
		return false, err
	}
	return wf != nil || comp.Spec.Workflow == nil, nil
}

// FirstEnvironment returns the root environment of the component's deployment pipeline, or "" when it cannot be resolved.
func (s *Steps) FirstEnvironment(ctx context.Context, comp *openchoreov1alpha1.Component) (string, error) {
	return s.r.validateAndFetchDeploymentPipeline(ctx, comp)
}

// ValidTraits reports whether the component's traits are allowed and valid for the ComponentType.
func (s *Steps) ValidTraits(ctx context.Context, comp *openchoreov1alpha1.Component, ct *openchoreov1alpha1.ComponentType) bool {
	return s.r.areValidTraits(ctx, comp, ct)
}

// Traits fetches the Trait and ClusterTrait specs the component and its ComponentType use.
func (s *Steps) Traits(ctx context.Context, ct *openchoreov1alpha1.ComponentType, comp *openchoreov1alpha1.Component) (map[string]openchoreov1alpha1.TraitSpec, map[string]openchoreov1alpha1.ClusterTraitSpec, error) {
	return s.r.fetchAllTraits(ctx, ct, comp)
}

// Workloads returns every Workload of the component, on any release line.
func (s *Steps) Workloads(ctx context.Context, comp *openchoreov1alpha1.Component) ([]openchoreov1alpha1.Workload, error) {
	var workloads openchoreov1alpha1.WorkloadList
	if err := s.r.List(ctx, &workloads, client.InNamespace(comp.Namespace),
		client.MatchingFields{workloadOwnerIndex: fmt.Sprintf("%s/%s", comp.Spec.Owner.ProjectName, comp.Name)}); err != nil {
		return nil, fmt.Errorf("failed to list Workloads: %w", err)
	}
	return workloads.Items, nil
}

// AutoDeploy creates the ComponentRelease for the workload when its content changed and binds it in
// firstEnv. line scopes the release and binding names and owners. It returns the latest release.
func (s *Steps) AutoDeploy(
	ctx context.Context,
	comp *openchoreov1alpha1.Component,
	ct *openchoreov1alpha1.ComponentType,
	workload *openchoreov1alpha1.Workload,
	traits map[string]openchoreov1alpha1.TraitSpec,
	clusterTraits map[string]openchoreov1alpha1.ClusterTraitSpec,
	firstEnv, line string,
	latest *openchoreov1alpha1.LatestRelease,
) (*openchoreov1alpha1.LatestRelease, error) {
	return s.r.handleAutoDeploy(ctx, comp, ct, workload, traits, clusterTraits, firstEnv, line, latest)
}

// TraitNotFound returns the name of the missing trait when err from Traits is a not-found error.
func TraitNotFound(err error) (string, bool) {
	var traitErr *traitFetchError
	if apierrors.IsNotFound(err) && errors.As(err, &traitErr) {
		return traitErr.traitName, true
	}
	return "", false
}

// Watch pairs a watched object with a mapper to the Components its changes affect.
type Watch struct {
	Object client.Object
	Map    handler.MapFunc
}

// ComponentWatches lists the shared objects whose changes alter what a Component releases.
// The mappers rely on the indexes the component controller registers.
func (s *Steps) ComponentWatches() []Watch {
	return []Watch{
		{&openchoreov1alpha1.ComponentType{}, s.r.listComponentsForComponentType},
		{&openchoreov1alpha1.ClusterComponentType{}, s.r.listComponentsForClusterComponentType},
		{&openchoreov1alpha1.Trait{}, s.r.listComponentsUsingTrait},
		{&openchoreov1alpha1.ClusterTrait{}, s.r.listComponentsUsingClusterTrait},
		{&openchoreov1alpha1.Workflow{}, s.r.listComponentsForWorkflow},
		{&openchoreov1alpha1.ClusterWorkflow{}, s.r.listComponentsForClusterWorkflow},
		{&openchoreov1alpha1.Project{}, s.r.listComponentsForProject},
		{&openchoreov1alpha1.DeploymentPipeline{}, s.r.listComponentsForDeploymentPipeline},
	}
}

// SetupIndexes registers the field indexes Steps relies on. The component controller registers them
// in SetupWithManager, so call this only on a manager without the component controller.
func SetupIndexes(ctx context.Context, mgr ctrl.Manager) error {
	r := &Reconciler{}
	for _, setup := range []func(context.Context, ctrl.Manager) error{
		r.setupComponentTypeRefIndex,
		r.setupTraitsRefIndex,
		r.setupWorkflowRefIndex,
		r.setupWorkloadOwnerIndex,
		r.setupComponentReleaseOwnerIndex,
	} {
		if err := setup(ctx, mgr); err != nil {
			return err
		}
	}
	return nil
}
