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
		Name      any  `json:"name"`      // Can be string or CEL expression
		Namespace *any `json:"namespace"` // Optional; if present, must be ${metadata.namespace}
	} `json:"metadata"`
}

// allowedNamespaceValue is the only permitted value for metadata.namespace in resource templates.
// The rendering pipeline sets the target namespace, so templates must not hardcode it.
const allowedNamespaceValue = "${metadata.namespace}"

// ValidateWorkloadResources validates resource templates and ensures workloadType matches a resource kind
func ValidateWorkloadResources(workloadType string, resources []v1alpha1.ResourceTemplate, basePath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	// Proxy workload type doesn't require workload resource validation
	if workloadType == "proxy" {
		// Still validate template structure and reject workload resources
		for i, resource := range resources {
			resourcePath := basePath.Index(i)
			templatePath := resourcePath.Child("template")
			if resource.Template == nil {
				allErrs = append(allErrs, field.Required(templatePath, "template is required"))
				continue
			}
			obj, errs := ValidateResourceTemplateStructure(*resource.Template, templatePath)
			allErrs = append(allErrs, errs...)

			if obj != nil && IsWorkloadResourceKind(obj.Kind) {
				allErrs = append(allErrs, field.Forbidden(
					templatePath.Child("kind"),
					fmt.Sprintf("proxy ComponentType must not contain workload resources (kind %q)", obj.Kind),
				))
			}
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

		if obj == nil {
			continue
		}

		// Check if this resource's kind matches the workloadType
		if strings.EqualFold(obj.Kind, workloadType) {
			workloadTypeMatchCount++
			workloadTypeIndices = append(workloadTypeIndices, i)
		} else if IsWorkloadResourceKind(obj.Kind) {
			// Reject workload resource kinds that don't match the declared workloadType
			allErrs = append(allErrs, field.Forbidden(
				templatePath.Child("kind"),
				fmt.Sprintf("resource kind %q is a workload type that does not match the declared workloadType %q; only one workload resource is allowed", obj.Kind, workloadType),
			))
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

	// Validate kind exists and is a literal value (CEL expressions are not allowed)
	if header.Kind == "" {
		allErrs = append(allErrs, field.Required(
			fieldPath.Child("kind"),
			"kind is required in resource template"))
	} else if strings.Contains(header.Kind, "${") {
		allErrs = append(allErrs, field.Forbidden(
			fieldPath.Child("kind"),
			"kind must be a literal value, not a template expression"))
	}

	// Validate metadata.name exists (can be any value including CEL expression)
	if header.Metadata.Name == nil || header.Metadata.Name == "" {
		allErrs = append(allErrs, field.Required(
			fieldPath.Child("metadata", "name"),
			"metadata.name is required in resource template"))
	}

	// Validate metadata.namespace if present: only ${metadata.namespace} is allowed.
	// We trim outer whitespace and normalize inner CEL whitespace so that
	// "  ${metadata.namespace}  " and "${ metadata.namespace }" are accepted.
	if header.Metadata.Namespace != nil {
		nsStr, ok := (*header.Metadata.Namespace).(string)
		if !ok || !isAllowedNamespaceValue(nsStr) {
			allErrs = append(allErrs, field.Invalid(
				fieldPath.Child("metadata", "namespace"),
				*header.Metadata.Namespace,
				fmt.Sprintf("if metadata.namespace is specified, it must be %q", allowedNamespaceValue)))
		}
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

// workloadResourceKinds contains the Kubernetes resource kinds that represent primary workloads.
// Each component can have only one primary workload defined by the ComponentType.
// Traits must not create these, and ComponentTypes must not include additional workload kinds
// beyond the declared workloadType.
var workloadResourceKinds = map[string]bool{
	"deployment":  true,
	"statefulset": true,
	"daemonset":   true,
	"cronjob":     true,
	"job":         true,
}

// IsWorkloadResourceKind returns true if the given kind (case-insensitive) is a workload resource kind.
func IsWorkloadResourceKind(kind string) bool {
	return workloadResourceKinds[strings.ToLower(kind)]
}

// ValidateTraitCreateTemplates validates that trait create templates have required
// K8s resource fields (apiVersion, kind, metadata.name) and do not create workload resources.
func ValidateTraitCreateTemplates(creates []v1alpha1.TraitCreate, basePath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}
	for i, create := range creates {
		createPath := basePath.Index(i)
		templatePath := createPath.Child("template")

		if create.Template == nil {
			allErrs = append(allErrs, field.Required(templatePath, "template is required"))
			continue
		}

		obj, errs := ValidateResourceTemplateStructure(*create.Template, templatePath)
		allErrs = append(allErrs, errs...)

		if obj != nil && IsWorkloadResourceKind(obj.Kind) {
			allErrs = append(allErrs, field.Forbidden(
				templatePath.Child("kind"),
				fmt.Sprintf("traits must not create workload resources (kind %q); the primary workload is defined by the ComponentType", obj.Kind),
			))
		}
	}
	return allErrs
}

// isAllowedNamespaceValue checks whether the given string is an acceptable
// metadata.namespace value. It trims outer whitespace and normalizes
// whitespace inside the ${...} delimiters so that variations like
// "  ${metadata.namespace}  " and "${ metadata.namespace }" are accepted.
func isAllowedNamespaceValue(val string) bool {
	trimmed := strings.TrimSpace(val)
	if !strings.HasPrefix(trimmed, "${") || !strings.HasSuffix(trimmed, "}") {
		return false
	}
	inner := strings.TrimSpace(trimmed[2 : len(trimmed)-1])
	return inner == "metadata.namespace"
}
