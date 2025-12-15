// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package component

import (
	"encoding/json"
	"fmt"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"

	"github.com/openchoreo/openchoreo/api/v1alpha1"
)

// resourceTemplateHeader represents the minimal required fields in a resource template
// CEL expressions are represented as strings, so we accept any value type
type resourceTemplateHeader struct {
	APIVersion string `json:"apiVersion"`
	Kind       string `json:"kind"`
	Metadata   struct {
		Name any `json:"name"` // Can be string or CEL expression
	} `json:"metadata"`
}

// ValidateWorkloadResources validates resource templates and ensures workloadType matches a resource kind
func ValidateWorkloadResources(workloadType string, resources []v1alpha1.ResourceTemplate, basePath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	// Proxy workload type doesn't require workload resource validation
	if workloadType == "proxy" {
		// Still validate template structure for all resources
		for i, resource := range resources {
			resourcePath := basePath.Index(i)
			templatePath := resourcePath.Child("template")
			if resource.Template == nil {
				allErrs = append(allErrs, field.Required(templatePath, "template is required"))
				continue
			}
			_, errs := ValidateResourceTemplateStructure(*resource.Template, templatePath)
			allErrs = append(allErrs, errs...)
		}
		return allErrs
	}

	workloadTypeMatchCount := 0
	var workloadTypeIndices []int

	// Validate resource templates have required fields and check for workload type match
	for i, resource := range resources {
		resourcePath := basePath.Index(i)
		templatePath := resourcePath.Child("template")
		if resource.Template == nil {
			allErrs = append(allErrs, field.Required(templatePath, "template is required"))
			continue
		}
		obj, errs := ValidateResourceTemplateStructure(*resource.Template, templatePath)
		allErrs = append(allErrs, errs...)

		// Check if this resource's kind matches the workloadType
		if obj != nil && strings.EqualFold(obj.Kind, workloadType) {
			workloadTypeMatchCount++
			workloadTypeIndices = append(workloadTypeIndices, i)
		}
	}

	// Ensure exactly one workloadType resource exists
	if workloadTypeMatchCount == 0 {
		allErrs = append(allErrs, field.Required(
			basePath,
			fmt.Sprintf("must have exactly one resource with kind matching workloadType %q", workloadType)))
	} else if workloadTypeMatchCount > 1 {
		// Report error on all matching resources
		for _, idx := range workloadTypeIndices {
			allErrs = append(allErrs, field.Duplicate(
				basePath.Index(idx).Child("template", "kind"),
				workloadType))
		}
		allErrs = append(allErrs, field.Invalid(
			basePath,
			fmt.Sprintf("%d resources", workloadTypeMatchCount),
			fmt.Sprintf("must have exactly one resource with kind matching workloadType %q, found %d", workloadType, workloadTypeMatchCount)))
	}

	return allErrs
}

// ValidateResourceTemplateStructure validates that a resource template has required Kubernetes fields
// and returns the parsed metadata for reuse by callers
func ValidateResourceTemplateStructure(template runtime.RawExtension, fieldPath *field.Path) (*metav1.PartialObjectMetadata, field.ErrorList) {
	allErrs := field.ErrorList{}

	if len(template.Raw) == 0 {
		allErrs = append(allErrs, field.Required(fieldPath, "template is required"))
		return nil, allErrs
	}

	// Parse template header (accepts CEL expressions as strings)
	// Note: runtime.RawExtension.Raw contains JSON bytes, not YAML
	var header resourceTemplateHeader
	if err := json.Unmarshal(template.Raw, &header); err != nil {
		allErrs = append(allErrs, field.Invalid(
			fieldPath,
			"<invalid>",
			fmt.Sprintf("failed to parse template: %v", err)))
		return nil, allErrs
	}

	// Validate apiVersion exists
	if header.APIVersion == "" {
		allErrs = append(allErrs, field.Required(
			fieldPath.Child("apiVersion"),
			"apiVersion is required in resource template"))
	}

	// Validate kind exists
	if header.Kind == "" {
		allErrs = append(allErrs, field.Required(
			fieldPath.Child("kind"),
			"kind is required in resource template"))
	}

	// Validate metadata.name exists (can be any value including CEL expression)
	if header.Metadata.Name == nil || header.Metadata.Name == "" {
		allErrs = append(allErrs, field.Required(
			fieldPath.Child("metadata", "name"),
			"metadata.name is required in resource template"))
	}

	// Build partial object metadata for return value (for kind matching)
	obj := &metav1.PartialObjectMetadata{}
	obj.APIVersion = header.APIVersion
	obj.Kind = header.Kind
	if nameStr, ok := header.Metadata.Name.(string); ok {
		obj.Name = nameStr
	}

	return obj, allErrs
}
