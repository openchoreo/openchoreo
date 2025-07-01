// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package render

import (
	dpkubernetes "github.com/openchoreo/openchoreo/internal/dataplane/kubernetes"
)

func makeServiceLabels(rCtx Context) map[string]string {
	return map[string]string{
		dpkubernetes.LabelKeyOrganizationName: rCtx.ServiceBinding.Namespace,
		dpkubernetes.LabelKeyProjectName:      rCtx.ServiceBinding.Spec.Owner.ProjectName,
		dpkubernetes.LabelKeyEnvironmentName:  rCtx.ServiceBinding.Spec.Environment,
		dpkubernetes.LabelKeyComponentName:    rCtx.ServiceBinding.Spec.Owner.ComponentName,
		// dpkubernetes.LabelKeyManagedBy:        dpkubernetes.LabelValueManagedBy,
		// dpkubernetes.LabelKeyBelongTo:         dpkubernetes.LabelValueBelongTo,
	}
}
