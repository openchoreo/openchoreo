// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package projectreleasebinding

import (
	"fmt"

	"github.com/openchoreo/openchoreo/internal/controller"
	projectpipeline "github.com/openchoreo/openchoreo/internal/pipeline/project"
)

// validateRenderedResources checks that the rendered (Cluster)ProjectType
// output declares the project's data-plane namespace before it is emitted as
// a RenderedRelease: at least one v1/Namespace named namespaceName (the
// platform-computed namespace for this binding, exposed to templates as
// ${metadata.namespace}) must be present. The project type owns this
// namespace; without it the binding has nowhere to place its resources.
//
// The check runs on the rendered entries rather than the static spec, so an
// includeWhen that suppresses the namespace entry surfaces here as
// NamespaceMissing. Returns ("", "") on success; otherwise a (reason,
// message) pair to surface on the binding's Synced condition.
//
// General duplicate-resource detection (two entries colliding on
// apiVersion+kind+namespace+name) is intentionally out of scope here: it is a
// platform-wide gap shared by the Component and Resource release paths, where
// duplicates currently resolve to last-write-wins at server-side apply. It is
// tracked as a separate cross-cutting validation rather than bolted onto the
// project path alone.
func validateRenderedResources(
	entries []projectpipeline.RenderedEntry,
	namespaceName string,
) (controller.ConditionReason, string) {
	for i := range entries {
		obj := entries[i].Object
		apiVersion, _ := obj["apiVersion"].(string)
		kind, _ := obj["kind"].(string)
		metadata, _ := obj["metadata"].(map[string]any)
		name, _ := metadata["name"].(string)
		if apiVersion == "v1" && kind == "Namespace" && name == namespaceName {
			return "", ""
		}
	}
	return ReasonNamespaceMissing, fmt.Sprintf(
		"rendered resources must include a v1/Namespace named %q; declare it in "+
			"(Cluster)ProjectType.spec.resources with metadata.name set to ${metadata.namespace}",
		namespaceName)
}
