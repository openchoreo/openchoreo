// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package render

import (
	dpkubernetes "github.com/openchoreo/openchoreo/internal/dataplane/kubernetes"
)

func makeWorkloadLabels(rCtx *Context) map[string]string {
	return map[string]string{
		dpkubernetes.LabelKeyOrganizationName: rCtx.Workload.Namespace,
		dpkubernetes.LabelKeyProjectName:      rCtx.Workload.Spec.Owner.ProjectName,
		dpkubernetes.LabelKeyEnvironmentName:  rCtx.Workload.Spec.EnvironmentName,
		dpkubernetes.LabelKeyComponentName:    rCtx.Workload.Spec.Owner.ComponentName,
		dpkubernetes.LabelKeyComponentType:    string(rCtx.Workload.Spec.Type),
		//dpkubernetes.LabelKeyManagedBy:        dpkubernetes.LabelValueManagedBy,
		//dpkubernetes.LabelKeyBelongTo:         dpkubernetes.LabelValueBelongTo,
	}
}
