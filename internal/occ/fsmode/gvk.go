// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package fsmode

import "k8s.io/apimachinery/pkg/runtime/schema"

// OpenChoreo resource GroupVersionKinds
var (
	ComponentGVK        = schema.GroupVersionKind{Group: "openchoreo.dev", Version: "v1alpha1", Kind: "Component"}
	ComponentTypeGVK    = schema.GroupVersionKind{Group: "openchoreo.dev", Version: "v1alpha1", Kind: "ComponentType"}
	WorkloadGVK         = schema.GroupVersionKind{Group: "openchoreo.dev", Version: "v1alpha1", Kind: "Workload"}
	TraitGVK            = schema.GroupVersionKind{Group: "openchoreo.dev", Version: "v1alpha1", Kind: "Trait"}
	ComponentReleaseGVK = schema.GroupVersionKind{Group: "openchoreo.dev", Version: "v1alpha1", Kind: "ComponentRelease"}
	ProjectGVK          = schema.GroupVersionKind{Group: "openchoreo.dev", Version: "v1alpha1", Kind: "Project"}
	EnvironmentGVK      = schema.GroupVersionKind{Group: "openchoreo.dev", Version: "v1alpha1", Kind: "Environment"}
	DataPlaneGVK        = schema.GroupVersionKind{Group: "openchoreo.dev", Version: "v1alpha1", Kind: "DataPlane"}
)
