// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package workflowrun

import (
	"context"
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log"

	openchoreodevv1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/labels"
)

// validateComponentWorkflowResult holds the outcome of component workflow validation.
type validateComponentWorkflowResult struct {
	// shouldReturn indicates the caller should return immediately with the given result.
	shouldReturn bool
	result       ctrl.Result
}

// validateComponentWorkflowRun validates that a component-scoped WorkflowRun references
// a workflow that is allowed by the component's ComponentType and matches the component's
// configured workflow. Returns a result indicating whether the caller should return early.
//
// Label rules:
//   - Both project + component labels present → validate workflow against component/componentType
//   - Neither label present → standalone workflow run, skip validation
//   - Only one label present → invalid, fail permanently
func (r *Reconciler) validateComponentWorkflowRun(
	ctx context.Context,
	workflowRun *openchoreodevv1alpha1.WorkflowRun,
) validateComponentWorkflowResult {
	projectLabel := workflowRun.Labels[labels.LabelKeyProjectName]
	componentLabel := workflowRun.Labels[labels.LabelKeyComponentName]

	hasProject := projectLabel != ""
	hasComponent := componentLabel != ""

	// Neither label → standalone workflow run, skip validation
	if !hasProject && !hasComponent {
		return validateComponentWorkflowResult{}
	}

	// Only one label present → invalid
	if hasProject != hasComponent {
		msg := "component workflow run must have both openchoreo.dev/project and openchoreo.dev/component labels"
		setComponentValidationFailedCondition(workflowRun, msg)
		return validateComponentWorkflowResult{shouldReturn: true, result: ctrl.Result{}}
	}

	// Both labels present → validate
	logger := log.FromContext(ctx)

	// Fetch the Component
	comp := &openchoreodevv1alpha1.Component{}
	compKey := types.NamespacedName{Name: componentLabel, Namespace: workflowRun.Namespace}
	if err := r.Get(ctx, compKey, comp); err != nil {
		if errors.IsNotFound(err) {
			msg := fmt.Sprintf("component %q not found", componentLabel)
			setComponentValidationFailedCondition(workflowRun, msg)
			return validateComponentWorkflowResult{shouldReturn: true, result: ctrl.Result{}}
		}
		logger.Error(err, "failed to fetch Component", "component", componentLabel)
		return validateComponentWorkflowResult{shouldReturn: true, result: ctrl.Result{Requeue: true}}
	}

	// Verify the project label matches the Component's owning project
	if comp.Spec.Owner.ProjectName != projectLabel {
		msg := fmt.Sprintf(
			"workflow run project label %q does not match component %q owner project %q",
			projectLabel, comp.Name, comp.Spec.Owner.ProjectName)
		setComponentValidationFailedCondition(workflowRun, msg)
		return validateComponentWorkflowResult{shouldReturn: true, result: ctrl.Result{}}
	}

	// Resolve the ComponentType
	ct, err := r.resolveComponentType(ctx, comp)
	if err != nil {
		logger.Error(err, "failed to resolve ComponentType", "component", comp.Name)
		return validateComponentWorkflowResult{shouldReturn: true, result: ctrl.Result{Requeue: true}}
	}
	if ct == nil {
		// ct is nil when the ComponentType was not found — permanent failure
		msg := fmt.Sprintf("component type %q not found for component %q", comp.Spec.ComponentType.Name, comp.Name)
		setComponentValidationFailedCondition(workflowRun, msg)
		return validateComponentWorkflowResult{shouldReturn: true, result: ctrl.Result{}}
	}

	// Validate workflow against ComponentType's allowedWorkflows
	wfKind := workflowRun.Spec.Workflow.Kind
	wfName := workflowRun.Spec.Workflow.Name

	if len(ct.Spec.AllowedWorkflows) > 0 {
		compositeKey := string(wfKind) + ":" + wfName
		allowed := false
		for _, ref := range ct.Spec.AllowedWorkflows {
			if string(ref.Kind)+":"+ref.Name == compositeKey {
				allowed = true
				break
			}
		}
		if !allowed {
			msg := fmt.Sprintf("workflow %s/%s is not allowed for component type %q; allowed: %v",
				wfKind, wfName, ct.Name, formatAllowedWorkflows(ct.Spec.AllowedWorkflows))
			setComponentValidationFailedCondition(workflowRun, msg)
			return validateComponentWorkflowResult{shouldReturn: true, result: ctrl.Result{}}
		}
	} else {
		msg := fmt.Sprintf("no workflows are allowed by component type %q, but workflow run references workflow %s/%s",
			ct.Name, wfKind, wfName)
		setComponentValidationFailedCondition(workflowRun, msg)
		return validateComponentWorkflowResult{shouldReturn: true, result: ctrl.Result{}}
	}

	// Validate workflow matches component's configured workflow
	if comp.Spec.Workflow != nil {
		compWfKind := comp.Spec.Workflow.Kind
		if wfKind != compWfKind || wfName != comp.Spec.Workflow.Name {
			msg := fmt.Sprintf(
				"workflow run references workflow %s/%s but component %q is configured with workflow %s/%s",
				wfKind, wfName, comp.Name, compWfKind, comp.Spec.Workflow.Name)
			setComponentValidationFailedCondition(workflowRun, msg)
			return validateComponentWorkflowResult{shouldReturn: true, result: ctrl.Result{}}
		}
	}

	return validateComponentWorkflowResult{}
}

// resolveComponentType fetches the ComponentType or ClusterComponentType referenced by the component.
// Returns nil (no error) if the type was not found.
func (r *Reconciler) resolveComponentType(
	ctx context.Context,
	comp *openchoreodevv1alpha1.Component,
) (*openchoreodevv1alpha1.ComponentType, error) {
	ctRef := comp.Spec.ComponentType

	// Parse componentType name: {workloadType}/{componentTypeName}
	parts := strings.SplitN(ctRef.Name, "/", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid componentType name format %q, expected {workloadType}/{name}", ctRef.Name)
	}
	ctName := parts[1]

	switch ctRef.Kind {
	case openchoreodevv1alpha1.ComponentTypeRefKindClusterComponentType:
		cct := &openchoreodevv1alpha1.ClusterComponentType{}
		if err := r.Get(ctx, types.NamespacedName{Name: ctName}, cct); err != nil {
			if errors.IsNotFound(err) {
				return nil, nil
			}
			return nil, fmt.Errorf("failed to fetch ClusterComponentType %q: %w", ctName, err)
		}
		// Convert ClusterWorkflowRef to WorkflowRef for uniform handling
		allowedWorkflows := make([]openchoreodevv1alpha1.WorkflowRef, len(cct.Spec.AllowedWorkflows))
		for i, ref := range cct.Spec.AllowedWorkflows {
			allowedWorkflows[i] = openchoreodevv1alpha1.WorkflowRef{
				Kind: openchoreodevv1alpha1.WorkflowRefKind(ref.Kind),
				Name: ref.Name,
			}
		}
		return &openchoreodevv1alpha1.ComponentType{
			ObjectMeta: cct.ObjectMeta,
			Spec: openchoreodevv1alpha1.ComponentTypeSpec{
				WorkloadType:     cct.Spec.WorkloadType,
				AllowedWorkflows: allowedWorkflows,
			},
		}, nil

	default:
		ct := &openchoreodevv1alpha1.ComponentType{}
		if err := r.Get(ctx, types.NamespacedName{Name: ctName, Namespace: comp.Namespace}, ct); err != nil {
			if errors.IsNotFound(err) {
				return nil, nil
			}
			return nil, fmt.Errorf("failed to fetch ComponentType %q: %w", ctName, err)
		}
		return ct, nil
	}
}

// formatAllowedWorkflows returns a human-readable list of allowed workflow references.
func formatAllowedWorkflows(refs []openchoreodevv1alpha1.WorkflowRef) string {
	parts := make([]string, len(refs))
	for i, ref := range refs {
		parts[i] = fmt.Sprintf("%s/%s", ref.Kind, ref.Name)
	}
	return "[" + strings.Join(parts, ", ") + "]"
}
