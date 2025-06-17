// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package render

import (
	dpkubernetes "github.com/openchoreo/openchoreo/internal/dataplane/kubernetes"
)

func makeWorkloadLabels(rCtx *Context) map[string]string {
	return map[string]string{
		dpkubernetes.LabelKeyOrganizationName: rCtx.WorkloadBinding.Namespace,
		dpkubernetes.LabelKeyProjectName:      rCtx.WorkloadBinding.Spec.WorkloadSpec.Owner.ProjectName,
		dpkubernetes.LabelKeyEnvironmentName:  rCtx.WorkloadBinding.Spec.EnvironmentName,
		dpkubernetes.LabelKeyComponentName:    rCtx.WorkloadBinding.Spec.WorkloadSpec.Owner.ComponentName,
		dpkubernetes.LabelKeyComponentType:    string(rCtx.WorkloadBinding.Spec.WorkloadSpec.Type),
		//dpkubernetes.LabelKeyManagedBy:        dpkubernetes.LabelValueManagedBy,
		//dpkubernetes.LabelKeyBelongTo:         dpkubernetes.LabelValueBelongTo,
	}
}
