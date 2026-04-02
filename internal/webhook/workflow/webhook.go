// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package workflow

import (
	"context"
	"encoding/json"
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	openchoreodevv1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/validation/component"
	"github.com/openchoreo/openchoreo/internal/validation/schemautil"
)

// nolint:unused
// log is for logging in this package.
var workflowlog = logf.Log.WithName("workflow-resource")

// SetupWorkflowWebhookWithManager registers the webhook for Workflow in the manager.
func SetupWorkflowWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).For(&openchoreodevv1alpha1.Workflow{}).
		WithValidator(&Validator{}).
		WithDefaulter(&Defaulter{}).
		Complete()
}

// +kubebuilder:webhook:path=/validate-openchoreo-dev-v1alpha1-workflow,mutating=false,failurePolicy=fail,sideEffects=None,groups=openchoreo.dev,resources=workflows,verbs=create;update,versions=v1alpha1,name=vworkflow-v1alpha1.kb.io,admissionReviewVersions=v1

// Validator validates Workflow resources
// +kubebuilder:object:generate=false
type Validator struct{}

var _ webhook.CustomValidator = &Validator{}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type Workflow.
func (v *Validator) ValidateCreate(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	wf, ok := obj.(*openchoreodevv1alpha1.Workflow)
	if !ok {
		return nil, fmt.Errorf("expected a Workflow object but got %T", obj)
	}
	workflowlog.Info("Validation for Workflow upon creation", "name", wf.GetName())

	allErrs := ValidateWorkflowSpec(wf.Spec.RunTemplate, wf.Spec.Resources, wf.Spec.ExternalRefs, wf.Spec.Parameters)

	if len(allErrs) > 0 {
		return nil, allErrs.ToAggregate()
	}

	return nil, nil
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type Workflow.
func (v *Validator) ValidateUpdate(_ context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	newWf, ok := newObj.(*openchoreodevv1alpha1.Workflow)
	if !ok {
		return nil, fmt.Errorf("expected a Workflow object for the newObj but got %T", newObj)
	}
	workflowlog.Info("Validation for Workflow upon update", "name", newWf.GetName())

	allErrs := ValidateWorkflowSpec(newWf.Spec.RunTemplate, newWf.Spec.Resources, newWf.Spec.ExternalRefs, newWf.Spec.Parameters)

	if len(allErrs) > 0 {
		return nil, allErrs.ToAggregate()
	}

	return nil, nil
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type Workflow.
func (v *Validator) ValidateDelete(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	wf, ok := obj.(*openchoreodevv1alpha1.Workflow)
	if !ok {
		return nil, fmt.Errorf("expected a Workflow object but got %T", obj)
	}
	workflowlog.Info("Validation for Workflow upon deletion", "name", wf.GetName())

	return nil, nil
}

// +kubebuilder:webhook:path=/mutate-openchoreo-dev-v1alpha1-workflow,mutating=true,failurePolicy=fail,sideEffects=None,groups=openchoreo.dev,resources=workflows,verbs=create;update,versions=v1alpha1,name=mworkflow-v1alpha1.kb.io,admissionReviewVersions=v1

// Defaulter sets defaults on Workflow resources
// +kubebuilder:object:generate=false
type Defaulter struct{}

var _ webhook.CustomDefaulter = &Defaulter{}

// Default implements webhook.CustomDefaulter so a webhook will be registered for the type Workflow.
func (d *Defaulter) Default(_ context.Context, obj runtime.Object) error {
	wf, ok := obj.(*openchoreodevv1alpha1.Workflow)
	if !ok {
		return fmt.Errorf("expected a Workflow object but got %T", obj)
	}
	workflowlog.Info("Defaulting for Workflow", "name", wf.GetName())

	return InjectServiceAccountName(wf.Spec.RunTemplate)
}

// ValidateWorkflowSpec performs all validation for a Workflow or ClusterWorkflow spec.
// Exported for reuse by the ClusterWorkflow webhook.
func ValidateWorkflowSpec(
	runTemplate *runtime.RawExtension,
	resources []openchoreodevv1alpha1.WorkflowResource,
	externalRefs []openchoreodevv1alpha1.ExternalRef,
	parameters *openchoreodevv1alpha1.SchemaSection,
) field.ErrorList {
	allErrs := field.ErrorList{}

	// Validate runTemplate structure and namespace
	runTemplatePath := field.NewPath("spec", "runTemplate")
	if runTemplate != nil {
		_, errs := component.ValidateResourceTemplateStructure(*runTemplate, runTemplatePath)
		allErrs = append(allErrs, errs...)

		allErrs = append(allErrs, validateTemplateNamespace(*runTemplate, runTemplatePath)...)
	}

	// Validate resources
	resourcesPath := field.NewPath("spec", "resources")
	resourceIDs := make(map[string]int)
	for i, res := range resources {
		resPath := resourcesPath.Index(i)
		templatePath := resPath.Child("template")

		// Validate resource ID uniqueness
		if prevIdx, exists := resourceIDs[res.ID]; exists {
			allErrs = append(allErrs, field.Duplicate(
				resPath.Child("id"),
				fmt.Sprintf("resource id %q is already used at index %d", res.ID, prevIdx),
			))
		}
		resourceIDs[res.ID] = i

		// Validate resource template structure and namespace
		if res.Template != nil {
			_, errs := component.ValidateResourceTemplateStructure(*res.Template, templatePath)
			allErrs = append(allErrs, errs...)

			allErrs = append(allErrs, validateTemplateNamespace(*res.Template, templatePath)...)
		}
	}

	// Validate externalRef ID uniqueness
	externalRefIDs := make(map[string]int)
	externalRefsPath := field.NewPath("spec", "externalRefs")
	for i, ref := range externalRefs {
		if prevIdx, exists := externalRefIDs[ref.ID]; exists {
			allErrs = append(allErrs, field.Duplicate(
				externalRefsPath.Index(i).Child("id"),
				fmt.Sprintf("externalRef id %q is already used at index %d", ref.ID, prevIdx),
			))
		}
		externalRefIDs[ref.ID] = i
	}

	// Validate parameters schema
	if parameters != nil {
		_, _, schemaErrs := schemautil.ExtractAndValidateSchemas(parameters, nil, field.NewPath("spec"))
		allErrs = append(allErrs, schemaErrs...)
	}

	return allErrs
}

// templateMetadata is used to extract metadata.namespace from a raw template
type templateMetadata struct {
	Metadata struct {
		Namespace string `json:"namespace"`
	} `json:"metadata"`
}

// validateTemplateNamespace validates that the template's metadata.namespace is "${metadata.namespace}".
func validateTemplateNamespace(template runtime.RawExtension, fieldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	if len(template.Raw) == 0 {
		return allErrs
	}

	var meta templateMetadata
	if err := json.Unmarshal(template.Raw, &meta); err != nil {
		// Structure validation already reports parse errors
		return allErrs
	}

	namespacePath := fieldPath.Child("metadata", "namespace")
	if meta.Metadata.Namespace == "" {
		allErrs = append(allErrs, field.Required(
			namespacePath,
			"metadata.namespace is required and must be set to \"${metadata.namespace}\"",
		))
	} else if meta.Metadata.Namespace != "${metadata.namespace}" {
		allErrs = append(allErrs, field.Invalid(
			namespacePath,
			meta.Metadata.Namespace,
			"metadata.namespace must be set to \"${metadata.namespace}\"",
		))
	}

	return allErrs
}

// InjectServiceAccountName sets spec.serviceAccountName to "workflow-sa" in the runTemplate.
// Exported for reuse by the ClusterWorkflow webhook.
func InjectServiceAccountName(runTemplate *runtime.RawExtension) error {
	if runTemplate == nil || len(runTemplate.Raw) == 0 {
		return nil
	}

	var raw map[string]any
	if err := json.Unmarshal(runTemplate.Raw, &raw); err != nil {
		return fmt.Errorf("failed to unmarshal runTemplate: %w", err)
	}

	var spec map[string]any
	if v, exists := raw["spec"]; exists {
		var ok bool
		spec, ok = v.(map[string]any)
		if !ok {
			return fmt.Errorf("failed to default runTemplate: spec is %T, expected object", v)
		}
	} else {
		spec = make(map[string]any)
		raw["spec"] = spec
	}

	spec["serviceAccountName"] = "workflow-sa"

	data, err := json.Marshal(raw)
	if err != nil {
		return fmt.Errorf("failed to marshal runTemplate: %w", err)
	}

	runTemplate.Raw = data
	return nil
}
