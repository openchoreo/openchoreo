// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package resources

import (
	"fmt"
	"os"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	fakeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/openchoreo/openchoreo/pkg/cli/common/constants"
)

func TestPrint_FilterByName_UsesLabelLookup(t *testing.T) {
	target := "target"
	podA := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "a", Namespace: "default", Labels: map[string]string{constants.LabelName: target}}}
	podB := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "b", Namespace: "default"}}

	fc := fakeclient.NewClientBuilder().WithObjects(podA, podB).Build()
	b := &BaseResource[*corev1.Pod, *corev1.PodList]{client: fc, namespace: "default", labels: map[string]string{}}

	// Capture stdout
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := b.Print(OutputFormatTable, &ResourceFilter{Name: target})

	// Restore stdout and read output
	w.Close()
	os.Stdout = old
	var buf strings.Builder
	var tmp = make([]byte, 1024)
	for {
		n, _ := r.Read(tmp)
		if n == 0 {
			break
		}
		buf.Write(tmp[:n])
	}

	if err != nil {
		t.Fatalf("Print failed: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, target) {
		t.Fatalf("expected output to contain %q, got: %s", target, out)
	}
}

func TestPrint_FilterByName_FallbackPagedSearch(t *testing.T) {
	target := "target"
	// no items with logical name label, but one with k8s name equal to target
	items := []client.Object{}
	for i := 0; i < 10; i++ {
		name := fmt.Sprintf("pod-%d", i)
		items = append(items, &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"}})
	}
	// put target later in list so paged search needs to iterate

	items = append(items, &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: target, Namespace: "default"}})
	objs := make([]client.Object, len(items))
	copy(objs, items)
	fc := fakeclient.NewClientBuilder().WithObjects(objs...).Build()
	b := &BaseResource[*corev1.Pod, *corev1.PodList]{client: fc, namespace: "default", labels: map[string]string{}}

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := b.Print(OutputFormatTable, &ResourceFilter{Name: target})

	w.Close()
	os.Stdout = old
	var buf strings.Builder
	var tmp = make([]byte, 1024)
	for {
		n, _ := r.Read(tmp)
		if n == 0 {
			break
		}
		buf.Write(tmp[:n])
	}

	if err != nil {
		t.Fatalf("Print failed: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, target) {
		t.Fatalf("expected output to contain %q, got: %s", target, out)
	}

	// Note: underlying fake client may ignore Limit/Continue; we only assert
	// that the fallback printed the expected resource.
}

// TestListPageWithToken_ReflectionWithPointerSlices tests reflection with pointer slice types
func TestListPageWithToken_ReflectionWithPointerSlices(t *testing.T) {
	// Create test pods (pointer types)
	pods := []*corev1.Pod{
		{ObjectMeta: metav1.ObjectMeta{Name: "pod-1", Namespace: "default"}},
		{ObjectMeta: metav1.ObjectMeta{Name: "pod-2", Namespace: "default"}},
		{ObjectMeta: metav1.ObjectMeta{Name: "pod-3", Namespace: "default"}},
	}

	// Create fake client with pointer slice
	fc := fakeclient.NewClientBuilder().
		WithObjects(pods[0], pods[1], pods[2]).
		Build()

	b := &BaseResource[*corev1.Pod, *corev1.PodList]{client: fc, namespace: "default", labels: map[string]string{}}

	// Test listing with token (even though fake client may ignore it)
	results, nextToken, err := b.listPageWithToken([]client.ListOption{}, 2, "")
	if err != nil {
		t.Fatalf("listPageWithToken failed: %v", err)
	}

	if len(results) == 0 {
		t.Error("Expected some results, got none")
	}

	// Verify results are properly typed
	for _, wrapper := range results {
		if wrapper.KubernetesName == "" {
			t.Error("Expected KubernetesName to be set")
		}
		if wrapper.Resource == nil {
			t.Error("Expected Resource to be set")
		}
	}

	// nextToken may be empty since fake client doesn't implement real pagination
	t.Logf("Next token: %s", nextToken)
}

// TestListPageWithToken_EmptyList tests reflection with empty lists
func TestListPageWithToken_EmptyList(t *testing.T) {
	fc := fakeclient.NewClientBuilder().Build()
	b := &BaseResource[*corev1.Pod, *corev1.PodList]{client: fc, namespace: "default", labels: map[string]string{}}

	results, nextToken, err := b.listPageWithToken([]client.ListOption{}, 10, "")
	if err != nil {
		t.Fatalf("listPageWithToken failed: %v", err)
	}

	if len(results) != 0 {
		t.Errorf("Expected empty results, got %d items", len(results))
	}

	if nextToken != "" {
		t.Errorf("Expected empty next token, got %s", nextToken)
	}
}

// TestListPageWithToken_LargeList tests reflection with pagination across multiple pages
func TestListPageWithToken_LargeList(t *testing.T) {
	// Create 25 pods to test pagination
	var objects []client.Object
	for i := 0; i < 25; i++ {
		objects = append(objects, &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("pod-%d", i),
				Namespace: "default",
			},
		})
	}

	fc := fakeclient.NewClientBuilder().WithObjects(objects...).Build()
	b := &BaseResource[*corev1.Pod, *corev1.PodList]{client: fc, namespace: "default", labels: map[string]string{}}

	// First page
	results1, nextToken1, err := b.listPageWithToken([]client.ListOption{}, 10, "")
	if err != nil {
		t.Fatalf("First page failed: %v", err)
	}

	if len(results1) == 0 {
		t.Error("Expected results on first page, got none")
	}

	// Second page (if token is returned)
	if nextToken1 != "" {
		results2, nextToken2, err := b.listPageWithToken([]client.ListOption{}, 10, nextToken1)
		if err != nil {
			t.Fatalf("Second page failed: %v", err)
		}

		if len(results2) == 0 {
			t.Error("Expected results on second page, got none")
		}

		t.Logf("Second page next token: %s", nextToken2)
	}
}

// TestListPageWithToken_NamespacedResources tests reflection with namespaced resources
func TestListPageWithToken_NamespacedResources(t *testing.T) {
	// Create pods in different namespaces
	pods := []*corev1.Pod{
		{ObjectMeta: metav1.ObjectMeta{Name: "pod-1", Namespace: "namespace-a"}},
		{ObjectMeta: metav1.ObjectMeta{Name: "pod-2", Namespace: "namespace-b"}},
	}

	fc := fakeclient.NewClientBuilder().
		WithObjects(pods[0], pods[1]).
		Build()

	// Test listing from specific namespace
	b := &BaseResource[*corev1.Pod, *corev1.PodList]{client: fc, namespace: "namespace-a", labels: map[string]string{}}

	results, _, err := b.listPageWithToken([]client.ListOption{client.InNamespace("namespace-a")}, 10, "")
	if err != nil {
		t.Fatalf("listPageWithToken failed: %v", err)
	}

	// Should only get pod from namespace-a
	foundPod1 := false
	for _, wrapper := range results {
		if wrapper.KubernetesName == "pod-1" {
			foundPod1 = true
		}
		if wrapper.KubernetesName == "pod-2" {
			t.Error("Should not find pod from different namespace")
		}
	}

	if !foundPod1 {
		t.Error("Expected to find pod-1 from namespace-a")
	}
}

// TestListPageWithToken_WithLabels tests reflection with label selectors in ListOptions
func TestListPageWithToken_WithLabels(t *testing.T) {
	pods := []*corev1.Pod{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "pod-with-label",
				Namespace: "default",
				Labels:    map[string]string{"app": "test"},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "pod-without-label",
				Namespace: "default",
			},
		},
	}

	fc := fakeclient.NewClientBuilder().
		WithObjects(pods[0], pods[1]).
		Build()

	b := &BaseResource[*corev1.Pod, *corev1.PodList]{
		client:    fc,
		namespace: "default",
		labels:    map[string]string{},
	}

	// Test that listPageWithToken works with label selectors in ListOptions
	// Note: fake client may not filter by labels, but we test that the reflection
	// logic handles the ListOptions correctly
	results, _, err := b.listPageWithToken([]client.ListOption{
		client.MatchingLabels(map[string]string{"app": "test"}),
	}, 10, "")
	if err != nil {
		t.Fatalf("listPageWithToken failed: %v", err)
	}

	// We should get some results (fake client may return all, but reflection should work)
	if len(results) == 0 {
		t.Error("Expected some results with label selector")
	}

	// Verify the reflection worked correctly - all results should be proper wrappers
	for _, wrapper := range results {
		if wrapper.Resource == nil {
			t.Error("Expected Resource to be set in wrapper")
		}
		if wrapper.KubernetesName == "" {
			t.Error("Expected KubernetesName to be set in wrapper")
		}
	}
}
