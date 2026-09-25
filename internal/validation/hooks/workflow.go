// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package hooks

import (
	"fmt"
	"sort"

	"k8s.io/apimachinery/pkg/util/validation/field"

	"github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/schema"
)

// ValidateParametersAgainstWorkflow checks that every hook parameter names an input
// the workflow declares and that every input the workflow requires is mapped.
// A mismatch would only surface when the first WorkflowRun is rejected, long after
// the platform engineer has moved on, so it is caught here.
func ValidateParametersAgainstWorkflow(params []v1alpha1.HookParameter, workflowName string, wf *v1alpha1.WorkflowSpec, path *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}
	if wf == nil {
		return allErrs
	}

	structural, err := schema.ResolveSectionToStructural(wf.Parameters)
	if err != nil {
		// The workflow webhook already rejects an unparsable schema; nothing to add here.
		return allErrs
	}

	properties := map[string]bool{}
	var required []string
	if structural != nil {
		for name := range structural.Properties {
			properties[name] = true
		}
		if structural.ValueValidation != nil {
			required = structural.ValueValidation.Required
		}
	}

	mapped := make(map[string]bool, len(params))
	for i, p := range params {
		mapped[p.Name] = true
		if !properties[p.Name] {
			allErrs = append(allErrs, field.Invalid(path.Index(i).Child("name"), p.Name,
				fmt.Sprintf("workflow %q does not declare an input named %q", workflowName, p.Name)))
		}
	}

	sort.Strings(required)
	for _, name := range required {
		if !mapped[name] {
			allErrs = append(allErrs, field.Required(path,
				fmt.Sprintf("workflow input %q is required but not mapped by any parameter", name)))
		}
	}
	return allErrs
}
