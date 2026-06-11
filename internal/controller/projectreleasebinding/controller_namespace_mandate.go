// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package projectreleasebinding

import (
	"encoding/json"
	"fmt"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/controller"
)

// cellNamespacePlaceholder is the literal CEL expression that the cell
// namespace template must use for its metadata.name. The rendering pipeline
// substitutes this with the computed dp-{ns}-{project}-{env}-{hash} name
// before the manifest is applied. Validation matches this string literally;
// CEL is not evaluated at this layer.
const cellNamespacePlaceholder = "${metadata.cellNamespace}"

// templateHeader captures the minimal apiVersion + kind + metadata.name
// surface needed to identify a Namespace entry in
// (Cluster)ProjectType.spec.resources. Other fields on the template are
// ignored during validation.
type templateHeader struct {
	APIVersion string `json:"apiVersion"`
	Kind       string `json:"kind"`
	Metadata   struct {
		Name string `json:"name"`
	} `json:"metadata"`
}

// validateCellNamespaceMandate checks that the inlined ProjectType spec
// declares at least one v1/Namespace resource whose metadata.name equals the
// cell-namespace placeholder. Returns ("", "") on success; otherwise a
// (reason, message) pair to surface on the binding's Synced condition.
//
// Duplicate-namespace detection is intentionally out of scope: if two
// entries declare a Namespace with the same metadata.name, that is a
// duplicate-resource problem caught at render time when applying both
// manifests collides on the data plane.
func validateCellNamespaceMandate(spec openchoreov1alpha1.ProjectTypeSpec) (controller.ConditionReason, string) {
	for i := range spec.Resources {
		entry := &spec.Resources[i]
		if entry.Template == nil || len(entry.Template.Raw) == 0 {
			continue
		}
		var hdr templateHeader
		if err := json.Unmarshal(entry.Template.Raw, &hdr); err != nil {
			continue
		}
		if !(hdr.APIVersion == "v1" && hdr.Kind == "Namespace" && hdr.Metadata.Name == cellNamespacePlaceholder) {
			continue
		}
		// includeWhen / forEach on the cell namespace entry break the
		// "exactly one cell namespace per binding" guarantee — includeWhen
		// can suppress it entirely; forEach renders N namespaces that
		// collide on the same resolved metadata.name. Reject so the
		// misconfiguration is surfaced on Synced instead of materializing
		// as downstream apply failures.
		if entry.IncludeWhen != "" {
			return ReasonCellNamespaceMandateInvalid, fmt.Sprintf(
				"cell namespace entry %q must not set includeWhen; the cell namespace must be rendered unconditionally",
				entry.ID)
		}
		if entry.ForEach != "" {
			return ReasonCellNamespaceMandateInvalid, fmt.Sprintf(
				"cell namespace entry %q must not set forEach; exactly one cell namespace must be rendered per binding",
				entry.ID)
		}
		return "", ""
	}
	return ReasonCellNamespaceMissing,
		fmt.Sprintf("ProjectType.spec.resources must contain a v1/Namespace entry with metadata.name=%q", cellNamespacePlaceholder)
}
