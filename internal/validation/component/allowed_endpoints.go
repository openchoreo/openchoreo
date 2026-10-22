// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package component

import (
	"fmt"

	"k8s.io/apimachinery/pkg/util/validation/field"

	"github.com/openchoreo/openchoreo/api/v1alpha1"
)

// ValidateAllowedEndpoints validates the allowedEndpoints list shared by
// ComponentType and ClusterComponentType: project visibility is rejected (it is
// implicit and never exposed through an Endpoint), aliases are unique, and at most one
// default entry matches any (protocol, visibility) pair, so the automatic
// EndpointType choice for a workload endpoint is never ambiguous.
func ValidateAllowedEndpoints(entries []v1alpha1.AllowedEndpointType, path *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	aliases := make(map[string]int)
	defaults := make(map[string]int)
	for i, entry := range entries {
		entryPath := path.Index(i)

		if entry.Name != "" {
			if prev, ok := aliases[entry.Name]; ok {
				allErrs = append(allErrs, field.Invalid(entryPath.Child("name"), entry.Name,
					fmt.Sprintf("alias is already used by entry %d", prev)))
			} else {
				aliases[entry.Name] = i
			}
		}

		for j, visibility := range entry.Visibilities {
			if visibility == v1alpha1.EndpointVisibilityProject {
				allErrs = append(allErrs, field.NotSupported(entryPath.Child("visibilities").Index(j), visibility,
					[]string{string(v1alpha1.EndpointVisibilityNamespace), string(v1alpha1.EndpointVisibilityInternal),
						string(v1alpha1.EndpointVisibilityExternal)}))
			}
		}

		if !entry.Default {
			continue
		}
		for _, protocol := range entry.Protocols {
			for _, visibility := range entry.Visibilities {
				key := string(protocol) + "/" + string(visibility)
				if prev, ok := defaults[key]; ok {
					allErrs = append(allErrs, field.Invalid(entryPath.Child("default"), entry.Default,
						fmt.Sprintf("entry %d is already the default for protocol %s with visibility %s",
							prev, protocol, visibility)))
					continue
				}
				defaults[key] = i
			}
		}
	}

	return allErrs
}
