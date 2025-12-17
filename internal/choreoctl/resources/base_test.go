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
	for i := range items {
		objs[i] = items[i]
	}
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
