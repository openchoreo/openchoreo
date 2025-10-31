// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

// Copyright 2025 Choreo Project
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package resolver

import (
	"context"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type ControllerType string

const (
	ControllerTypeDeployment  ControllerType = "Deployment"
	ControllerTypeStatefulSet ControllerType = "StatefulSet"
	ControllerTypeDaemonSet   ControllerType = "DaemonSet"
	ControllerTypeJob         ControllerType = "Job"
	ControllerTypeCronJob     ControllerType = "CronJob"
	ControllerTypeReplicaSet  ControllerType = "ReplicaSet"
	ControllerTypePod         ControllerType = "Pod"
)

type ResolvedController struct {
	Type            ControllerType
	Object          runtime.Object
	Namespace       string
	Name            string
	UID             string
	ResourceVersion string
	Labels          map[string]string
}

func ResolveParentController(ctx context.Context, k8sClient client.Client, pod *corev1.Pod) (*ResolvedController, error) {
	if pod == nil {
		return nil, fmt.Errorf("pod is nil")
	}

	ownerRef := metav1.GetControllerOf(pod)
	if ownerRef == nil {
		return &ResolvedController{
			Type:            ControllerTypePod,
			Object:          pod,
			Namespace:       pod.Namespace,
			Name:            pod.Name,
			UID:             string(pod.UID),
			ResourceVersion: pod.ResourceVersion,
			Labels:          pod.Labels,
		}, nil
	}

	switch ownerRef.Kind {
	case "ReplicaSet":
		return resolveReplicaSetOwner(ctx, k8sClient, pod.Namespace, ownerRef)
	case "StatefulSet":
		return resolveStatefulSet(ctx, k8sClient, pod.Namespace, ownerRef)
	case "DaemonSet":
		return resolveDaemonSet(ctx, k8sClient, pod.Namespace, ownerRef)
	case "Job":
		return resolveJob(ctx, k8sClient, pod.Namespace, ownerRef)
	case "CronJob":
		return resolveCronJob(ctx, k8sClient, pod.Namespace, ownerRef)
	default:
		return &ResolvedController{
			Type:            ControllerTypePod,
			Object:          pod,
			Namespace:       pod.Namespace,
			Name:            pod.Name,
			UID:             string(pod.UID),
			ResourceVersion: pod.ResourceVersion,
			Labels:          pod.Labels,
		}, nil
	}
}

func resolveReplicaSetOwner(ctx context.Context, k8sClient client.Client, namespace string, ownerRef *metav1.OwnerReference) (*ResolvedController, error) {
	rs := &appsv1.ReplicaSet{}
	if err := k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: ownerRef.Name}, rs); err != nil {
		if errors.IsNotFound(err) {
			return nil, fmt.Errorf("replicaset %s/%s not found", namespace, ownerRef.Name)
		}
		return nil, fmt.Errorf("failed to get replicaset: %w", err)
	}

	rsOwnerRef := metav1.GetControllerOf(rs)
	if rsOwnerRef != nil && rsOwnerRef.Kind == "Deployment" {
		return resolveDeployment(ctx, k8sClient, namespace, rsOwnerRef)
	}

	return &ResolvedController{
		Type:            ControllerTypeReplicaSet,
		Object:          rs,
		Namespace:       rs.Namespace,
		Name:            rs.Name,
		UID:             string(rs.UID),
		ResourceVersion: rs.ResourceVersion,
		Labels:          rs.Labels,
	}, nil
}

func resolveDeployment(ctx context.Context, k8sClient client.Client, namespace string, ownerRef *metav1.OwnerReference) (*ResolvedController, error) {
	deploy := &appsv1.Deployment{}
	if err := k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: ownerRef.Name}, deploy); err != nil {
		if errors.IsNotFound(err) {
			return nil, fmt.Errorf("deployment %s/%s not found", namespace, ownerRef.Name)
		}
		return nil, fmt.Errorf("failed to get deployment: %w", err)
	}

	return &ResolvedController{
		Type:            ControllerTypeDeployment,
		Object:          deploy,
		Namespace:       deploy.Namespace,
		Name:            deploy.Name,
		UID:             string(deploy.UID),
		ResourceVersion: deploy.ResourceVersion,
		Labels:          deploy.Labels,
	}, nil
}

func resolveStatefulSet(ctx context.Context, k8sClient client.Client, namespace string, ownerRef *metav1.OwnerReference) (*ResolvedController, error) {
	sts := &appsv1.StatefulSet{}
	if err := k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: ownerRef.Name}, sts); err != nil {
		if errors.IsNotFound(err) {
			return nil, fmt.Errorf("statefulset %s/%s not found", namespace, ownerRef.Name)
		}
		return nil, fmt.Errorf("failed to get statefulset: %w", err)
	}

	return &ResolvedController{
		Type:            ControllerTypeStatefulSet,
		Object:          sts,
		Namespace:       sts.Namespace,
		Name:            sts.Name,
		UID:             string(sts.UID),
		ResourceVersion: sts.ResourceVersion,
		Labels:          sts.Labels,
	}, nil
}

func resolveDaemonSet(ctx context.Context, k8sClient client.Client, namespace string, ownerRef *metav1.OwnerReference) (*ResolvedController, error) {
	ds := &appsv1.DaemonSet{}
	if err := k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: ownerRef.Name}, ds); err != nil {
		if errors.IsNotFound(err) {
			return nil, fmt.Errorf("daemonset %s/%s not found", namespace, ownerRef.Name)
		}
		return nil, fmt.Errorf("failed to get daemonset: %w", err)
	}

	return &ResolvedController{
		Type:            ControllerTypeDaemonSet,
		Object:          ds,
		Namespace:       ds.Namespace,
		Name:            ds.Name,
		UID:             string(ds.UID),
		ResourceVersion: ds.ResourceVersion,
		Labels:          ds.Labels,
	}, nil
}

func resolveJob(ctx context.Context, k8sClient client.Client, namespace string, ownerRef *metav1.OwnerReference) (*ResolvedController, error) {
	job := &batchv1.Job{}
	if err := k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: ownerRef.Name}, job); err != nil {
		if errors.IsNotFound(err) {
			return nil, fmt.Errorf("job %s/%s not found", namespace, ownerRef.Name)
		}
		return nil, fmt.Errorf("failed to get job: %w", err)
	}

	jobOwnerRef := metav1.GetControllerOf(job)
	if jobOwnerRef != nil && jobOwnerRef.Kind == "CronJob" {
		return resolveCronJob(ctx, k8sClient, namespace, jobOwnerRef)
	}

	return &ResolvedController{
		Type:            ControllerTypeJob,
		Object:          job,
		Namespace:       job.Namespace,
		Name:            job.Name,
		UID:             string(job.UID),
		ResourceVersion: job.ResourceVersion,
		Labels:          job.Labels,
	}, nil
}

func resolveCronJob(ctx context.Context, k8sClient client.Client, namespace string, ownerRef *metav1.OwnerReference) (*ResolvedController, error) {
	cronjob := &batchv1.CronJob{}
	if err := k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: ownerRef.Name}, cronjob); err != nil {
		if errors.IsNotFound(err) {
			return nil, fmt.Errorf("cronjob %s/%s not found", namespace, ownerRef.Name)
		}
		return nil, fmt.Errorf("failed to get cronjob: %w", err)
	}

	return &ResolvedController{
		Type:            ControllerTypeCronJob,
		Object:          cronjob,
		Namespace:       cronjob.Namespace,
		Name:            cronjob.Name,
		UID:             string(cronjob.UID),
		ResourceVersion: cronjob.ResourceVersion,
		Labels:          cronjob.Labels,
	}, nil
}
