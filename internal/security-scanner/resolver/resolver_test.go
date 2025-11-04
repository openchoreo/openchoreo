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
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestResolveParentController_NilPod(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()

	_, err := ResolveParentController(context.Background(), fakeClient, nil)
	if err == nil {
		t.Error("expected error for nil pod, got nil")
	}
}

func TestResolveParentController_OrphanedPod(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "orphan-pod",
			Namespace:       "default",
			UID:             "pod-uid",
			ResourceVersion: "123",
			Labels: map[string]string{
				"app": "test",
			},
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(pod).Build()
	result, err := ResolveParentController(context.Background(), fakeClient, pod)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Type != ControllerTypePod {
		t.Errorf("expected ControllerTypePod, got %s", result.Type)
	}
	if result.Name != "orphan-pod" {
		t.Errorf("expected name 'orphan-pod', got %s", result.Name)
	}
	if result.UID != "pod-uid" {
		t.Errorf("expected UID 'pod-uid', got %s", result.UID)
	}
}

func TestResolveParentController_PodToDeployment(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = appsv1.AddToScheme(scheme)

	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "test-deploy",
			Namespace:       "default",
			UID:             "deploy-uid",
			ResourceVersion: "100",
			Labels: map[string]string{
				"app": "test",
			},
		},
	}

	rs := &appsv1.ReplicaSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "test-deploy-abc123",
			Namespace:       "default",
			UID:             "rs-uid",
			ResourceVersion: "101",
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: "apps/v1",
					Kind:       "Deployment",
					Name:       "test-deploy",
					UID:        "deploy-uid",
					Controller: boolPtr(true),
				},
			},
		},
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "test-deploy-abc123-xyz",
			Namespace:       "default",
			UID:             "pod-uid",
			ResourceVersion: "102",
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: "apps/v1",
					Kind:       "ReplicaSet",
					Name:       "test-deploy-abc123",
					UID:        "rs-uid",
					Controller: boolPtr(true),
				},
			},
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(deploy, rs, pod).Build()
	result, err := ResolveParentController(context.Background(), fakeClient, pod)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Type != ControllerTypeDeployment {
		t.Errorf("expected ControllerTypeDeployment, got %s", result.Type)
	}
	if result.Name != "test-deploy" {
		t.Errorf("expected name 'test-deploy', got %s", result.Name)
	}
	if result.UID != "deploy-uid" {
		t.Errorf("expected UID 'deploy-uid', got %s", result.UID)
	}
	if result.ResourceVersion != "100" {
		t.Errorf("expected resourceVersion '100', got %s", result.ResourceVersion)
	}
}

func TestResolveParentController_PodToStatefulSet(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = appsv1.AddToScheme(scheme)

	sts := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "test-sts",
			Namespace:       "default",
			UID:             "sts-uid",
			ResourceVersion: "200",
			Labels: map[string]string{
				"app": "stateful",
			},
		},
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "test-sts-0",
			Namespace:       "default",
			UID:             "pod-uid",
			ResourceVersion: "201",
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: "apps/v1",
					Kind:       "StatefulSet",
					Name:       "test-sts",
					UID:        "sts-uid",
					Controller: boolPtr(true),
				},
			},
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(sts, pod).Build()
	result, err := ResolveParentController(context.Background(), fakeClient, pod)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Type != ControllerTypeStatefulSet {
		t.Errorf("expected ControllerTypeStatefulSet, got %s", result.Type)
	}
	if result.Name != "test-sts" {
		t.Errorf("expected name 'test-sts', got %s", result.Name)
	}
}

func TestResolveParentController_PodToDaemonSet(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = appsv1.AddToScheme(scheme)

	ds := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "test-ds",
			Namespace:       "kube-system",
			UID:             "ds-uid",
			ResourceVersion: "300",
		},
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "test-ds-abc",
			Namespace:       "kube-system",
			UID:             "pod-uid",
			ResourceVersion: "301",
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: "apps/v1",
					Kind:       "DaemonSet",
					Name:       "test-ds",
					UID:        "ds-uid",
					Controller: boolPtr(true),
				},
			},
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ds, pod).Build()
	result, err := ResolveParentController(context.Background(), fakeClient, pod)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Type != ControllerTypeDaemonSet {
		t.Errorf("expected ControllerTypeDaemonSet, got %s", result.Type)
	}
	if result.Name != "test-ds" {
		t.Errorf("expected name 'test-ds', got %s", result.Name)
	}
}

func TestResolveParentController_PodToCronJob(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = batchv1.AddToScheme(scheme)

	cronjob := &batchv1.CronJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "test-cronjob",
			Namespace:       "default",
			UID:             "cronjob-uid",
			ResourceVersion: "400",
		},
	}

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "test-cronjob-12345",
			Namespace:       "default",
			UID:             "job-uid",
			ResourceVersion: "401",
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: "batch/v1",
					Kind:       "CronJob",
					Name:       "test-cronjob",
					UID:        "cronjob-uid",
					Controller: boolPtr(true),
				},
			},
		},
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "test-cronjob-12345-xyz",
			Namespace:       "default",
			UID:             "pod-uid",
			ResourceVersion: "402",
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: "batch/v1",
					Kind:       "Job",
					Name:       "test-cronjob-12345",
					UID:        "job-uid",
					Controller: boolPtr(true),
				},
			},
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cronjob, job, pod).Build()
	result, err := ResolveParentController(context.Background(), fakeClient, pod)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Type != ControllerTypeCronJob {
		t.Errorf("expected ControllerTypeCronJob, got %s", result.Type)
	}
	if result.Name != "test-cronjob" {
		t.Errorf("expected name 'test-cronjob', got %s", result.Name)
	}
}

func TestResolveParentController_PodToJob(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = batchv1.AddToScheme(scheme)

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "standalone-job",
			Namespace:       "default",
			UID:             "job-uid",
			ResourceVersion: "500",
		},
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "standalone-job-xyz",
			Namespace:       "default",
			UID:             "pod-uid",
			ResourceVersion: "501",
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: "batch/v1",
					Kind:       "Job",
					Name:       "standalone-job",
					UID:        "job-uid",
					Controller: boolPtr(true),
				},
			},
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(job, pod).Build()
	result, err := ResolveParentController(context.Background(), fakeClient, pod)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Type != ControllerTypeJob {
		t.Errorf("expected ControllerTypeJob, got %s", result.Type)
	}
	if result.Name != "standalone-job" {
		t.Errorf("expected name 'standalone-job', got %s", result.Name)
	}
}

func TestResolveParentController_PodToReplicaSet(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = appsv1.AddToScheme(scheme)

	rs := &appsv1.ReplicaSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "standalone-rs",
			Namespace:       "default",
			UID:             "rs-uid",
			ResourceVersion: "600",
		},
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "standalone-rs-xyz",
			Namespace:       "default",
			UID:             "pod-uid",
			ResourceVersion: "601",
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: "apps/v1",
					Kind:       "ReplicaSet",
					Name:       "standalone-rs",
					UID:        "rs-uid",
					Controller: boolPtr(true),
				},
			},
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(rs, pod).Build()
	result, err := ResolveParentController(context.Background(), fakeClient, pod)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Type != ControllerTypeReplicaSet {
		t.Errorf("expected ControllerTypeReplicaSet, got %s", result.Type)
	}
	if result.Name != "standalone-rs" {
		t.Errorf("expected name 'standalone-rs', got %s", result.Name)
	}
}

func TestResolveParentController_MissingOwner(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = appsv1.AddToScheme(scheme)

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "test-pod",
			Namespace:       "default",
			UID:             "pod-uid",
			ResourceVersion: "700",
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: "apps/v1",
					Kind:       "ReplicaSet",
					Name:       "missing-rs",
					UID:        "rs-uid",
					Controller: boolPtr(true),
				},
			},
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(pod).Build()
	_, err := ResolveParentController(context.Background(), fakeClient, pod)
	if err == nil {
		t.Error("expected error for missing replicaset, got nil")
	}
}

func boolPtr(b bool) *bool {
	return &b
}
