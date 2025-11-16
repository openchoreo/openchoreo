// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package releasebinding

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/controller"
)

// setResourcesReadyStatus evaluates the ResourcesReady condition from the Release status
// and mirrors it to the ReleaseBinding status.
//
// nolint:unparam // ctx and error return kept for consistency with other status methods
func (r *Reconciler) setResourcesReadyStatus(ctx context.Context, releaseBinding *openchoreov1alpha1.ReleaseBinding,
	release *openchoreov1alpha1.Release) error {
	// Find ResourcesReady condition in Release status
	var resourcesReadyCondition *metav1.Condition
	for i := range release.Status.Conditions {
		if release.Status.Conditions[i].Type == "ResourcesReady" {
			resourcesReadyCondition = &release.Status.Conditions[i]
			break
		}
	}

	// If Release doesn't have ResourcesReady condition yet, mark as Progressing
	if resourcesReadyCondition == nil {
		msg := fmt.Sprintf("Release %q is being processed", release.Name)
		controller.MarkFalseCondition(releaseBinding, ConditionResourcesReady,
			ReasonResourcesProgressing, msg)
		return nil
	}

	// Mirror the condition from Release to ReleaseBinding
	switch resourcesReadyCondition.Status {
	case metav1.ConditionTrue:
		controller.MarkTrueCondition(releaseBinding, ConditionResourcesReady,
			ReasonResourcesReady, resourcesReadyCondition.Message)

	case metav1.ConditionFalse:
		controller.MarkFalseCondition(releaseBinding, ConditionResourcesReady,
			ReasonResourcesNotReady, resourcesReadyCondition.Message)

	case metav1.ConditionUnknown:
		controller.MarkFalseCondition(releaseBinding, ConditionResourcesReady,
			ReasonResourcesProgressing, resourcesReadyCondition.Message)
	}

	return nil
}
