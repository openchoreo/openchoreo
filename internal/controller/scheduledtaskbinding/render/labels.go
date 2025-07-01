// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package render

import (
	dpkubernetes "github.com/openchoreo/openchoreo/internal/dataplane/kubernetes"
)

func makeScheduledTaskLabels(rCtx Context) map[string]string {
	return map[string]string{
		dpkubernetes.LabelKeyOrganizationName: rCtx.ScheduledTaskBinding.Namespace,
		dpkubernetes.LabelKeyProjectName:      rCtx.ScheduledTaskBinding.Spec.Owner.ProjectName,
		dpkubernetes.LabelKeyEnvironmentName:  rCtx.ScheduledTaskBinding.Spec.Environment,
		dpkubernetes.LabelKeyComponentName:    rCtx.ScheduledTaskBinding.Spec.Owner.ComponentName,
		// dpkubernetes.LabelKeyManagedBy:        dpkubernetes.LabelValueManagedBy,
		// dpkubernetes.LabelKeyBelongTo:         dpkubernetes.LabelValueBelongTo,
	}
}
