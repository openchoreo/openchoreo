// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package projectreleasebinding

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/controller"
	"github.com/openchoreo/openchoreo/internal/controller/renderedrelease"
)

// reconcileObserve maps the binding's environment to an existing namespace.
// While the environment allows that namespace, the binding keeps an
// Observe-mode RenderedRelease that reads the Namespace and the claimed
// objects, and reports the namespace in status. Otherwise it deletes that
// release and clears status.namespace, so nothing reads the namespace.
func (r *Reconciler) reconcileObserve(ctx context.Context, binding *openchoreov1alpha1.ProjectReleaseBinding) error {
	namespace := binding.Spec.Observe.Namespace

	environment := &openchoreov1alpha1.Environment{}
	if err := r.Get(ctx, types.NamespacedName{Name: binding.Spec.Environment, Namespace: binding.Namespace}, environment); err != nil {
		if apierrors.IsNotFound(err) {
			return r.stopObserving(ctx, binding, ReasonEnvironmentNotFound,
				fmt.Sprintf("Environment %q not found", binding.Spec.Environment))
		}
		return fmt.Errorf("get Environment %q: %w", binding.Spec.Environment, err)
	}
	if !environment.IsNamespaceObservable(namespace) {
		return r.stopObserving(ctx, binding, ReasonNamespaceNotAllowed, fmt.Sprintf(
			"Namespace %q is not in Environment %q spec.observe.allowedNamespaces", namespace, environment.Name))
	}
	binding.Status.Namespace = namespace

	if env, _, _, err := r.fetchSupportingResources(ctx, binding); err != nil || env == nil {
		return err
	}

	observed := append([]openchoreov1alpha1.ObservedResource{{
		ID:         controller.ObservedResourceID("v1", "Namespace", "", namespace),
		APIVersion: "v1",
		Kind:       "Namespace",
		Name:       namespace,
	}}, controller.ObservedResources(namespace, binding.Spec.Observe.Resources)...)

	rr, err := r.emitRenderedRelease(ctx, binding, openchoreov1alpha1.RenderedReleaseSpec{
		Owner: openchoreov1alpha1.RenderedReleaseOwner{
			ProjectName: binding.Spec.Owner.ProjectName,
		},
		EnvironmentName:   binding.Spec.Environment,
		TargetPlane:       openchoreov1alpha1.TargetPlaneDataPlane,
		Mode:              openchoreov1alpha1.RenderedReleaseModeObserve,
		ObservedResources: observed,
	}, fmt.Sprintf("%d observed resource(s)", len(observed)))
	if err != nil || rr == nil {
		return err
	}

	if !renderedrelease.ObservedResourcesCurrent(rr) {
		msg := fmt.Sprintf("RenderedRelease %q has not read the current observed resources yet", rr.Name)
		controller.MarkFalseCondition(binding, ConditionNamespaceReady, ReasonNamespaceProgressing, msg)
		controller.MarkFalseCondition(binding, ConditionResourcesReady, ReasonResourcesProgressing, msg)
		return nil
	}
	evaluateObservedNamespaceReady(binding, rr.Status.ObservedResources)
	evaluateResourcesReady(binding, rr.Status.ObservedResources)
	return nil
}

// evaluateObservedNamespaceReady sets NamespaceReady from the observed
// Namespace. The release reports a Namespace it could not read as Unknown.
func evaluateObservedNamespaceReady(
	binding *openchoreov1alpha1.ProjectReleaseBinding,
	statuses []openchoreov1alpha1.RenderedManifestStatus,
) {
	entry := findNamespaceEntry(statuses, binding.Status.Namespace)
	if entry != nil && entry.HealthStatus == openchoreov1alpha1.HealthStatusUnknown {
		controller.MarkFalseCondition(binding, ConditionNamespaceReady, ReasonNamespaceNotFound,
			fmt.Sprintf("Namespace %q was not found on the data plane", binding.Status.Namespace))
		return
	}
	evaluateNamespaceReady(binding, statuses)
}

// stopObserving clears status.namespace, so observed ReleaseBindings stop too,
// deletes the binding's RenderedRelease and says why on Synced.
func (r *Reconciler) stopObserving(ctx context.Context, binding *openchoreov1alpha1.ProjectReleaseBinding,
	reason controller.ConditionReason, msg string) error {
	binding.Status.Namespace = ""
	if err := r.deleteRenderedRelease(ctx, binding); err != nil {
		return err
	}
	markSyncedFalse(binding, reason, msg)
	return nil
}

// deleteRenderedRelease deletes the binding's RenderedRelease if the binding
// controls it and it is in Observe mode. The delete is conditioned on the
// release's UID and resourceVersion, so a release changed since the read is
// never deleted.
func (r *Reconciler) deleteRenderedRelease(ctx context.Context, binding *openchoreov1alpha1.ProjectReleaseBinding) error {
	rr := &openchoreov1alpha1.RenderedRelease{}
	key := types.NamespacedName{Name: makeRenderedReleaseName(binding), Namespace: binding.Namespace}
	if err := r.Get(ctx, key, rr); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("get RenderedRelease %q: %w", key.Name, err)
	}
	if !metav1.IsControlledBy(rr, binding) || !rr.DeletionTimestamp.IsZero() ||
		rr.Spec.Mode != openchoreov1alpha1.RenderedReleaseModeObserve {
		return nil
	}
	err := r.Delete(ctx, rr, client.Preconditions{UID: &rr.UID, ResourceVersion: &rr.ResourceVersion})
	if err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("delete RenderedRelease %q: %w", key.Name, err)
	}
	return nil
}
