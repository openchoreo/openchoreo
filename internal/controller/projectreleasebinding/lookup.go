// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package projectreleasebinding

import (
	"context"
	"errors"
	"fmt"

	"k8s.io/apimachinery/pkg/api/meta"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/controller"
	dpkubernetes "github.com/openchoreo/openchoreo/internal/dataplane/kubernetes"
)

// Reasons the other bindings report when their project's ProjectReleaseBinding
// gives them no namespace to observe.
const (
	// ReasonProjectReleaseBindingNotFound indicates there is no
	// ProjectReleaseBinding for the binding's project and environment, which
	// Observe mode needs.
	ReasonProjectReleaseBindingNotFound controller.ConditionReason = "ProjectReleaseBindingNotFound"

	// ReasonProjectReleaseBindingConflict indicates several ProjectReleaseBindings
	// target the binding's project and environment.
	ReasonProjectReleaseBindingConflict controller.ConditionReason = "ProjectReleaseBindingConflict"

	// ReasonProjectNotObserved indicates the project's ProjectReleaseBinding is in
	// Manage mode. Observe mode under it is not supported yet.
	ReasonProjectNotObserved controller.ConditionReason = "ProjectNotObserved"

	// ReasonNamespaceNotObserved indicates the project's ProjectReleaseBinding is in
	// Observe mode but not observing a namespace, for example because its
	// Environment does not allow the namespace.
	ReasonNamespaceNotObserved controller.ConditionReason = "NamespaceNotObserved"
)

// Reasons a binding in Manage mode reports when its project's
// ProjectReleaseBinding stops it rendering.
const (
	// ReasonProjectObserved indicates the project's ProjectReleaseBinding is in
	// Observe mode. Manage mode under it is not supported yet, so the binding
	// renders and applies nothing.
	ReasonProjectObserved controller.ConditionReason = "ProjectObserved"

	// ReasonProjectNamespacePending indicates the project's ProjectReleaseBinding
	// is in Manage mode but hasn't reported its namespace yet.
	ReasonProjectNamespacePending controller.ConditionReason = "ProjectNamespacePending"
)

// ErrAmbiguous is returned when several ProjectReleaseBindings target one
// project and environment.
var ErrAmbiguous = errors.New("several ProjectReleaseBindings target this project and environment")

// Find returns the ProjectReleaseBinding of project and environment in
// namespace, nil when there is none, and ErrAmbiguous when there are several.
// One being deleted is skipped, so its replacement takes over while it
// finalizes.
func Find(
	ctx context.Context,
	c client.Reader,
	namespace, project, environment string,
) (*openchoreov1alpha1.ProjectReleaseBinding, error) {
	list := &openchoreov1alpha1.ProjectReleaseBindingList{}
	if err := c.List(ctx, list, client.InNamespace(namespace)); err != nil {
		return nil, fmt.Errorf("failed to list ProjectReleaseBindings: %w", err)
	}
	var found *openchoreov1alpha1.ProjectReleaseBinding
	for i := range list.Items {
		prb := &list.Items[i]
		if prb.Spec.Owner.ProjectName != project ||
			prb.Spec.Environment != environment ||
			!prb.DeletionTimestamp.IsZero() {
			continue
		}
		if found != nil {
			return nil, ErrAmbiguous
		}
		found = prb
	}
	return found, nil
}

// ObservedNamespace returns the namespace that the ProjectReleaseBinding of
// project and environment observes, for a binding in Observe mode. When there
// is no such namespace, it returns "" with the reason and a message for the
// binding's conditions.
func ObservedNamespace(
	ctx context.Context,
	c client.Reader,
	namespace, project, environment string,
) (string, controller.ConditionReason, string, error) {
	prb, err := Find(ctx, c, namespace, project, environment)
	switch {
	case errors.Is(err, ErrAmbiguous):
		return "", ReasonProjectReleaseBindingConflict, conflictMessage(project, environment), nil
	case err != nil:
		return "", "", "", err
	case prb == nil:
		return "", ReasonProjectReleaseBindingNotFound,
			fmt.Sprintf("No ProjectReleaseBinding for project %q in environment %q; Observe mode needs one in Observe mode",
				project, environment), nil
	case !prb.Spec.IsObserve():
		return "", ReasonProjectNotObserved,
			fmt.Sprintf("ProjectReleaseBinding %q is in Manage mode; Observe mode under it is not supported yet",
				prb.Name), nil
	case prb.Status.Namespace == "":
		return "", ReasonNamespaceNotObserved,
			withReadyMessage(prb, fmt.Sprintf("ProjectReleaseBinding %q is not observing a namespace", prb.Name)), nil
	}
	return prb.Status.Namespace, "", "", nil
}

// ManagedNamespace returns the data-plane namespace a binding in Manage mode
// renders into for project and environment: the ProjectReleaseBinding's
// status.namespace, or the generated dp-* name when the project has no
// ProjectReleaseBinding there, as before ProjectReleaseBindings existed. When
// the binding must render nothing, it returns "" with the reason and a message
// for its conditions: several ProjectReleaseBindings, an observed one, or a
// managed one that hasn't reported its namespace yet.
func ManagedNamespace(
	ctx context.Context,
	c client.Reader,
	namespace, project, environment string,
) (string, controller.ConditionReason, string, error) {
	prb, err := Find(ctx, c, namespace, project, environment)
	switch {
	case errors.Is(err, ErrAmbiguous):
		return "", ReasonProjectReleaseBindingConflict, conflictMessage(project, environment), nil
	case err != nil:
		return "", "", "", err
	case prb == nil:
		return dpkubernetes.DataPlaneNamespace(namespace, project, environment), "", "", nil
	case prb.Spec.IsObserve():
		return "", ReasonProjectObserved,
			fmt.Sprintf("ProjectReleaseBinding %q is in Observe mode; Manage mode under it is not supported yet",
				prb.Name), nil
	case prb.Status.Namespace == "":
		return "", ReasonProjectNamespacePending,
			withReadyMessage(prb, fmt.Sprintf("ProjectReleaseBinding %q has not reported its namespace yet", prb.Name)), nil
	}
	return prb.Status.Namespace, "", "", nil
}

// conflictMessage says that several ProjectReleaseBindings target project and
// environment.
func conflictMessage(project, environment string) string {
	return fmt.Sprintf("Several ProjectReleaseBindings target project %q in environment %q", project, environment)
}

// withReadyMessage adds the ProjectReleaseBinding's Ready message to msg, so
// the binding says why its ProjectReleaseBinding has no namespace.
func withReadyMessage(prb *openchoreov1alpha1.ProjectReleaseBinding, msg string) string {
	if ready := meta.FindStatusCondition(prb.Status.Conditions, string(ConditionReady)); ready != nil && ready.Message != "" {
		msg += ": " + ready.Message
	}
	return msg
}

// ObserveChangedPredicate passes creates, deletes, and updates that change
// what the other bindings read from a ProjectReleaseBinding: the mode and the
// namespace in status.
func ObserveChangedPredicate() predicate.Predicate {
	return predicate.Funcs{
		UpdateFunc: func(e event.UpdateEvent) bool {
			oldPRB, okOld := e.ObjectOld.(*openchoreov1alpha1.ProjectReleaseBinding)
			newPRB, okNew := e.ObjectNew.(*openchoreov1alpha1.ProjectReleaseBinding)
			if !okOld || !okNew {
				return false
			}
			return oldPRB.Spec.Mode != newPRB.Spec.Mode || oldPRB.Status.Namespace != newPRB.Status.Namespace
		},
	}
}
