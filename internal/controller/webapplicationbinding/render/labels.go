// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package render

import (
	dpkubernetes "github.com/openchoreo/openchoreo/internal/dataplane/kubernetes"
)

func makeWebApplicationLabels(rCtx Context) map[string]string {
	return map[string]string{
		dpkubernetes.LabelKeyOrganizationName: rCtx.WebApplicationBinding.Namespace,
		dpkubernetes.LabelKeyProjectName:      rCtx.WebApplicationBinding.Spec.Owner.ProjectName,
		dpkubernetes.LabelKeyEnvironmentName:  rCtx.WebApplicationBinding.Spec.Environment,
		dpkubernetes.LabelKeyComponentName:    rCtx.WebApplicationBinding.Spec.Owner.ComponentName,
		// dpkubernetes.LabelKeyManagedBy:        dpkubernetes.LabelValueManagedBy,
		// dpkubernetes.LabelKeyBelongTo:         dpkubernetes.LabelValueBelongTo,
	}
}