// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"database/sql"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/openchoreo/openchoreo/internal/security-scanner/checkov"
	"github.com/openchoreo/openchoreo/internal/security-scanner/db/backend"
)

type mockQuerier struct {
	resources        map[string]*mockResource
	scannedResources map[string]*backend.PostureScannedResource
	labels           map[int64]map[string]string
	nextID           int64
}

type mockResource struct {
	ID              int64
	Type            string
	Namespace       string
	Name            string
	UID             string
	ResourceVersion string
}

func newMockQuerier() *mockQuerier {
	return &mockQuerier{
		resources:        make(map[string]*mockResource),
		scannedResources: make(map[string]*backend.PostureScannedResource),
		labels:           make(map[int64]map[string]string),
		nextID:           1,
	}
}

func (m *mockQuerier) UpsertResource(ctx context.Context, resourceType, resourceNamespace, resourceName, resourceUID, resourceVersion string) (int64, error) {
	key := resourceType + "/" + resourceNamespace + "/" + resourceName
	if r, exists := m.resources[key]; exists {
		r.UID = resourceUID
		r.ResourceVersion = resourceVersion
		return r.ID, nil
	}

	id := m.nextID
	m.nextID++
	m.resources[key] = &mockResource{
		ID:              id,
		Type:            resourceType,
		Namespace:       resourceNamespace,
		Name:            resourceName,
		UID:             resourceUID,
		ResourceVersion: resourceVersion,
	}
	return id, nil
}

func (m *mockQuerier) InsertResourceLabel(ctx context.Context, resourceID int64, labelKey, labelValue string) error {
	if m.labels[resourceID] == nil {
		m.labels[resourceID] = make(map[string]string)
	}
	m.labels[resourceID][labelKey] = labelValue
	return nil
}

func (m *mockQuerier) DeleteResourceLabels(ctx context.Context, resourceID int64) error {
	delete(m.labels, resourceID)
	return nil
}

func (m *mockQuerier) GetPostureScannedResource(ctx context.Context, resourceType, resourceNamespace, resourceName string) (backend.PostureScannedResource, error) {
	key := resourceType + "/" + resourceNamespace + "/" + resourceName
	if sr, exists := m.scannedResources[key]; exists {
		return *sr, nil
	}
	return backend.PostureScannedResource{}, sql.ErrNoRows
}

func (m *mockQuerier) UpsertPostureScannedResource(ctx context.Context, resourceID int64, resourceVersion string, scanDurationMs *int64) error {
	for _, r := range m.resources {
		if r.ID == resourceID {
			key := r.Type + "/" + r.Namespace + "/" + r.Name
			m.scannedResources[key] = &backend.PostureScannedResource{
				ResourceID:      resourceID,
				ResourceVersion: resourceVersion,
			}
			return nil
		}
	}
	return nil
}

func (m *mockQuerier) InsertPostureFinding(ctx context.Context, resourceID int64, checkID, checkName, severity string, category, description, remediation *string, resourceVersion string) error {
	return nil
}

func (m *mockQuerier) DeletePostureFindingsByResourceID(ctx context.Context, resourceID int64) error {
	return nil
}

func (m *mockQuerier) GetResource(ctx context.Context, resourceID int64) (backend.Resource, error) {
	for _, r := range m.resources {
		if r.ID == resourceID {
			return backend.Resource{
				ID:                r.ID,
				ResourceType:      r.Type,
				ResourceNamespace: r.Namespace,
				ResourceName:      r.Name,
				ResourceUID:       r.UID,
				ResourceVersion:   r.ResourceVersion,
			}, nil
		}
	}
	return backend.Resource{}, sql.ErrNoRows
}

func (m *mockQuerier) GetResourceLabels(ctx context.Context, resourceID int64) (map[string]string, error) {
	if labels, exists := m.labels[resourceID]; exists {
		return labels, nil
	}
	return make(map[string]string), nil
}

func (m *mockQuerier) GetPostureFindingsByResourceID(ctx context.Context, resourceID int64) ([]backend.PostureFinding, error) {
	return []backend.PostureFinding{}, nil
}

func (m *mockQuerier) ListPostureFindings(ctx context.Context, limit, offset int64) ([]backend.PostureFindingWithResource, error) {
	return []backend.PostureFindingWithResource{}, nil
}

func (m *mockQuerier) ListResourcesWithPostureFindings(ctx context.Context, limit, offset int64) ([]backend.Resource, error) {
	return []backend.Resource{}, nil
}

func (m *mockQuerier) CountResourcesWithPostureFindings(ctx context.Context) (int64, error) {
	return 0, nil
}

func TestPodReconciler_StandalonePod(t *testing.T) {
	checkov.SetMockScanner(&checkov.MockScanner{})
	defer checkov.ResetScanner()

	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "standalone-pod",
			Namespace:       "default",
			UID:             "pod-uid-123",
			ResourceVersion: "100",
			Labels: map[string]string{
				"app":  "test",
				"tier": "frontend",
			},
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(pod).Build()
	mockQ := newMockQuerier()

	reconciler := &PodReconciler{
		Client:  fakeClient,
		Scheme:  scheme,
		Queries: mockQ,
	}

	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Namespace: "default",
			Name:      "standalone-pod",
		},
	}

	result, err := reconciler.Reconcile(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Requeue {
		t.Error("expected no requeue")
	}

	key := "Pod/default/standalone-pod"
	if _, exists := mockQ.resources[key]; !exists {
		t.Error("expected resource to be upserted")
	}

	if _, exists := mockQ.scannedResources[key]; !exists {
		t.Error("expected resource to be marked as scanned")
	}

	resourceID := mockQ.resources[key].ID
	if labels, exists := mockQ.labels[resourceID]; exists {
		if labels["app"] != "test" {
			t.Errorf("expected label app=test, got %s", labels["app"])
		}
		if labels["tier"] != "frontend" {
			t.Errorf("expected label tier=frontend, got %s", labels["tier"])
		}
	} else {
		t.Error("expected labels to be stored")
	}
}

func TestPodReconciler_PodWithDeployment(t *testing.T) {
	checkov.SetMockScanner(&checkov.MockScanner{})
	defer checkov.ResetScanner()

	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = appsv1.AddToScheme(scheme)

	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "test-deploy",
			Namespace:       "default",
			UID:             "deploy-uid",
			ResourceVersion: "200",
			Labels: map[string]string{
				"app": "myapp",
			},
		},
	}

	rs := &appsv1.ReplicaSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "test-deploy-abc",
			Namespace:       "default",
			UID:             "rs-uid",
			ResourceVersion: "201",
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
			Name:            "test-deploy-abc-xyz",
			Namespace:       "default",
			UID:             "pod-uid",
			ResourceVersion: "202",
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: "apps/v1",
					Kind:       "ReplicaSet",
					Name:       "test-deploy-abc",
					UID:        "rs-uid",
					Controller: boolPtr(true),
				},
			},
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(deploy, rs, pod).Build()
	mockQ := newMockQuerier()

	reconciler := &PodReconciler{
		Client:  fakeClient,
		Scheme:  scheme,
		Queries: mockQ,
	}

	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Namespace: "default",
			Name:      "test-deploy-abc-xyz",
		},
	}

	result, err := reconciler.Reconcile(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Requeue {
		t.Error("expected no requeue")
	}

	key := "Deployment/default/test-deploy"
	resource, exists := mockQ.resources[key]
	if !exists {
		t.Fatal("expected deployment to be upserted")
	}

	if resource.Type != "Deployment" {
		t.Errorf("expected type Deployment, got %s", resource.Type)
	}

	if resource.Name != "test-deploy" {
		t.Errorf("expected name test-deploy, got %s", resource.Name)
	}

	if _, exists := mockQ.scannedResources[key]; !exists {
		t.Error("expected deployment to be marked as scanned")
	}
}

func TestPodReconciler_ResourceVersionDeduplication(t *testing.T) {
	checkov.SetMockScanner(&checkov.MockScanner{})
	defer checkov.ResetScanner()

	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "test-pod",
			Namespace:       "default",
			UID:             "pod-uid",
			ResourceVersion: "100",
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(pod).Build()
	mockQ := newMockQuerier()

	reconciler := &PodReconciler{
		Client:  fakeClient,
		Scheme:  scheme,
		Queries: mockQ,
	}

	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Namespace: "default",
			Name:      "test-pod",
		},
	}

	_, err := reconciler.Reconcile(context.Background(), req)
	if err != nil {
		t.Fatalf("first reconcile failed: %v", err)
	}

	key := "Pod/default/test-pod"
	if _, exists := mockQ.scannedResources[key]; !exists {
		t.Fatal("expected resource to be marked as scanned after first reconcile")
	}

	labelCountBefore := len(mockQ.labels)

	_, err = reconciler.Reconcile(context.Background(), req)
	if err != nil {
		t.Fatalf("second reconcile failed: %v", err)
	}

	labelCountAfter := len(mockQ.labels)
	if labelCountAfter != labelCountBefore {
		t.Error("expected labels not to be re-inserted on second reconcile (deduplication should skip)")
	}
}

func TestPodReconciler_ResourceVersionChanged(t *testing.T) {
	checkov.SetMockScanner(&checkov.MockScanner{})
	defer checkov.ResetScanner()

	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	pod1 := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "test-pod",
			Namespace:       "default",
			UID:             "pod-uid",
			ResourceVersion: "100",
		},
	}

	fakeClient1 := fake.NewClientBuilder().WithScheme(scheme).WithObjects(pod1).Build()
	mockQ := newMockQuerier()

	reconciler := &PodReconciler{
		Client:  fakeClient1,
		Scheme:  scheme,
		Queries: mockQ,
	}

	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Namespace: "default",
			Name:      "test-pod",
		},
	}

	_, err := reconciler.Reconcile(context.Background(), req)
	if err != nil {
		t.Fatalf("first reconcile failed: %v", err)
	}

	key := "Pod/default/test-pod"
	scannedV1 := mockQ.scannedResources[key]
	if scannedV1.ResourceVersion != "100" {
		t.Errorf("expected scanned version 100, got %s", scannedV1.ResourceVersion)
	}

	pod2 := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "test-pod",
			Namespace:       "default",
			UID:             "pod-uid",
			ResourceVersion: "101",
		},
	}

	fakeClient2 := fake.NewClientBuilder().WithScheme(scheme).WithObjects(pod2).Build()
	reconciler.Client = fakeClient2

	_, err = reconciler.Reconcile(context.Background(), req)
	if err != nil {
		t.Fatalf("second reconcile failed: %v", err)
	}

	scannedV2 := mockQ.scannedResources[key]
	if scannedV2.ResourceVersion != "101" {
		t.Errorf("expected scanned version 101, got %s", scannedV2.ResourceVersion)
	}
}

func TestPodReconciler_PodNotFound(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	mockQ := newMockQuerier()

	reconciler := &PodReconciler{
		Client:  fakeClient,
		Scheme:  scheme,
		Queries: mockQ,
	}

	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Namespace: "default",
			Name:      "nonexistent-pod",
		},
	}

	result, err := reconciler.Reconcile(context.Background(), req)
	if err != nil {
		t.Errorf("expected no error for missing pod, got %v", err)
	}

	if result.Requeue {
		t.Error("expected no requeue for missing pod")
	}

	if len(mockQ.resources) > 0 {
		t.Error("expected no resources to be created for missing pod")
	}
}

func boolPtr(b bool) *bool {
	return &b
}
