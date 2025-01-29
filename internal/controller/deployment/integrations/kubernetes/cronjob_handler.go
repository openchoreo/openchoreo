/*
 * Copyright (c) 2025, WSO2 Inc. (http://www.wso2.org) All Rights Reserved.
 *
 * WSO2 Inc. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License.
 * You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

package kubernetes

import (
	"context"
	"errors"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	choreov1 "github.com/wso2-enterprise/choreo-cp-declarative-api/api/v1"
	"github.com/wso2-enterprise/choreo-cp-declarative-api/internal/dataplane"
	dpkubernetes "github.com/wso2-enterprise/choreo-cp-declarative-api/internal/dataplane/kubernetes"
	"github.com/wso2-enterprise/choreo-cp-declarative-api/internal/ptr"
)

type cronJobHandler struct {
	kubernetesClient client.Client
}

var _ dataplane.ResourceHandler = (*cronJobHandler)(nil)

func NewCronJobHandler(kubernetesClient client.Client) dataplane.ResourceHandler {
	return &cronJobHandler{
		kubernetesClient: kubernetesClient,
	}
}

func (h *cronJobHandler) Name() string {
	return "KubernetesCronJobHandler"
}

func (h *cronJobHandler) IsRequired(deployCtx *dataplane.DeploymentContext) bool {
	return deployCtx.Component.Spec.Type == choreov1.ComponentTypeScheduledTask
}

func (h *cronJobHandler) GetCurrentState(ctx context.Context, deployCtx *dataplane.DeploymentContext) (interface{}, error) {
	namespace := makeNamespaceName(deployCtx)
	name := makeCronJobName(deployCtx)
	out := &batchv1.CronJob{}
	err := h.kubernetesClient.Get(ctx, client.ObjectKey{Name: name, Namespace: namespace}, out)
	if apierrors.IsNotFound(err) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	return out, nil
}

func (h *cronJobHandler) Create(ctx context.Context, deployCtx *dataplane.DeploymentContext) error {
	cronJob := makeCronJob(deployCtx)
	return h.kubernetesClient.Create(ctx, cronJob)
}

func (h *cronJobHandler) Update(ctx context.Context, deployCtx *dataplane.DeploymentContext, currentState interface{}) error {
	currentCronJob, ok := currentState.(*batchv1.CronJob)
	if !ok {
		return errors.New("failed to cast current state to CronJob")
	}

	newCronJob := makeCronJob(deployCtx)

	if h.shouldUpdate(currentCronJob, newCronJob) {
		newCronJob.ResourceVersion = currentCronJob.ResourceVersion
		return h.kubernetesClient.Update(ctx, newCronJob)
	}

	return nil
}

func (h *cronJobHandler) Delete(ctx context.Context, deployCtx *dataplane.DeploymentContext) error {
	cronJob := makeCronJob(deployCtx)
	err := h.kubernetesClient.Delete(ctx, cronJob)
	if apierrors.IsNotFound(err) {
		return nil
	}
	return err
}

func (h *cronJobHandler) shouldUpdate(current, new *batchv1.CronJob) bool {
	// Compare the labels
	if !cmp.Equal(extractManagedLabels(current.Labels), extractManagedLabels(new.Labels)) {
		return true
	}

	if !cmp.Equal(current.Spec, new.Spec, cmpopts.EquateEmpty()) {
		return true
	}
	return false
}

func makeCronJobName(deployCtx *dataplane.DeploymentContext) string {
	componentName := deployCtx.Component.Name
	deploymentTrackName := deployCtx.DeploymentTrack.Name
	// Limit the name to 52 characters to comply with the K8s name length limit for CronJobs
	return dpkubernetes.GenerateK8sNameWithLengthLimit(52, componentName, deploymentTrackName)
}

// TODO: Unit test me
func makeCronJob(deployCtx *dataplane.DeploymentContext) *batchv1.CronJob {
	return &batchv1.CronJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      makeCronJobName(deployCtx),
			Namespace: makeNamespaceName(deployCtx),
			Labels:    makeWorkloadLabels(deployCtx),
		},
		Spec: makeCronJobSpec(deployCtx),
	}
}

func makeCronJobSpec(deployCtx *dataplane.DeploymentContext) batchv1.CronJobSpec {
	cronJobSpec := batchv1.CronJobSpec{
		ConcurrencyPolicy: batchv1.ForbidConcurrent,
		JobTemplate: batchv1.JobTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{
				Labels: makeWorkloadLabels(deployCtx),
			},
			Spec: batchv1.JobSpec{
				// TODO: These are free tire values from Choreo v2. Make these configurable that are coming from the deployment context
				ActiveDeadlineSeconds: ptr.Int64(300),
				BackoffLimit:          ptr.Int32(4),
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: makeWorkloadLabels(deployCtx),
					},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{
								Name:  "main",
								Env:   makeEnvironmentVariables(deployCtx),
								Image: "localhost:30003/" + deployCtx.ContainerImage,
							},
						},
						RestartPolicy: corev1.RestartPolicyNever,
					},
				},
				TTLSecondsAfterFinished: ptr.Int32(360),
			},
		},
		Suspend:  ptr.Bool(false),
		TimeZone: ptr.String("Etc/UTC"),
	}
	var taskSpec *choreov1.TaskConfig
	if deployCtx.DeployableArtifact.Spec.Configuration != nil &&
		deployCtx.DeployableArtifact.Spec.Configuration.Application != nil {
		taskSpec = deployCtx.DeployableArtifact.Spec.Configuration.Application.Task
	}
	if taskSpec == nil {
		return cronJobSpec
	}

	if taskSpec.Disabled {
		cronJobSpec.Suspend = ptr.Bool(true)
	}

	if taskSpec.Schedule != nil {
		cronJobSpec.Schedule = taskSpec.Schedule.Cron
		if taskSpec.Schedule.Timezone != "" {
			cronJobSpec.TimeZone = ptr.String(taskSpec.Schedule.Timezone)
		}
	}
	return cronJobSpec
}

// TODO: move this to a common place
func makeEnvironmentVariables(deployCtx *dataplane.DeploymentContext) []corev1.EnvVar {
	if deployCtx.DeployableArtifact.Spec.Configuration == nil ||
		deployCtx.DeployableArtifact.Spec.Configuration.Application == nil {
		return nil
	}

	envVars := deployCtx.DeployableArtifact.Spec.Configuration.Application.Env
	k8sEnvVars := make([]corev1.EnvVar, 0, len(envVars))
	for _, v := range envVars {
		if v.Key == "" {
			continue
		}
		k8sEnvVars = append(k8sEnvVars, corev1.EnvVar{
			Name:  v.Key,
			Value: v.Value,
		})
	}
	return k8sEnvVars
}
