// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package render

import (
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	choreov1 "github.com/openchoreo/openchoreo/api/v1"
	dpkubernetes "github.com/openchoreo/openchoreo/internal/dataplane/kubernetes"
)

// CronJob creates a complete CronJob resource for scheduled task execution
func CronJob(rCtx Context) *choreov1.Resource {
	base := rCtx.ScheduledTaskClass.Spec.CronJobTemplate

	overlay := makeScheduledTaskCronJobSpec(rCtx)

	// The merge will override the schedule to empty if the overlay does not specify it due to not having omitempty
	if overlay.Schedule == "" {
		// If no schedule is provided, use the default from the class
		overlay.Schedule = rCtx.ScheduledTaskClass.Spec.CronJobTemplate.Schedule
	}

	mergedSpec, err := merge(&base, &overlay)
	if err != nil {
		rCtx.AddError(MergeError(err))
		return nil
	}

	cronJob := &batchv1.CronJob{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "batch/v1",
			Kind:       "CronJob",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      makeCronJobName(rCtx),
			Namespace: makeNamespaceName(rCtx),
			Labels:    makeScheduledTaskLabels(rCtx),
		},
		Spec: *mergedSpec,
	}

	rawExt := &runtime.RawExtension{}
	rawExt.Object = cronJob

	return &choreov1.Resource{
		ID:     makeCronJobResourceID(rCtx),
		Object: rawExt,
	}
}

func makeScheduledTaskCronJobSpec(rCtx Context) batchv1.CronJobSpec {
	cs := batchv1.CronJobSpec{}

	// Create the job template
	cs.JobTemplate = batchv1.JobTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{
			Labels: makeScheduledTaskLabels(rCtx),
		},
		Spec: batchv1.JobSpec{
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: makeScheduledTaskLabels(rCtx),
				},
				Spec: *makeScheduledTaskPodSpec(rCtx),
			},
		},
	}

	return cs
}

func makeCronJobName(rCtx Context) string {
	return dpkubernetes.GenerateK8sName(rCtx.ScheduledTaskBinding.Name)
}

func makeNamespaceName(rCtx Context) string {
	organizationName := rCtx.ScheduledTaskBinding.Namespace // Namespace is the organization name
	projectName := rCtx.ScheduledTaskBinding.Spec.Owner.ProjectName
	environmentName := rCtx.ScheduledTaskBinding.Spec.Environment
	// Limit the name to 63 characters to comply with the K8s name length limit for Namespaces
	return dpkubernetes.GenerateK8sNameWithLengthLimit(dpkubernetes.MaxNamespaceNameLength,
		"dp", organizationName, projectName, environmentName)
}

// TODO: Find a better way to generate resource IDs
func makeCronJobResourceID(rCtx Context) string {
	return rCtx.ScheduledTaskBinding.Name + "-cronjob"
}
