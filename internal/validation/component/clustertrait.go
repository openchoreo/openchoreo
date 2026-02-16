// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package component

import (
	apiextschema "k8s.io/apiextensions-apiserver/pkg/apiserver/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"

	"github.com/openchoreo/openchoreo/api/v1alpha1"
)

// ValidateClusterTraitCreatesAndPatchesWithSchema validates all creates and patches in a ClusterTrait with schema-aware type checking.
func ValidateClusterTraitCreatesAndPatchesWithSchema(
	ct *v1alpha1.ClusterTrait,
	parametersSchema *apiextschema.Structural,
	envOverridesSchema *apiextschema.Structural,
) field.ErrorList {
	return validateTraitCreatesAndPatchesWithSchema(ct.Spec.Creates, ct.Spec.Patches, parametersSchema, envOverridesSchema)
}
