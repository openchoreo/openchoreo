// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package e2e

import (
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/yaml"
)

func mustYAMLDocs(objects ...any) string {
	docs := make([]string, 0, len(objects))
	for _, obj := range objects {
		data, err := yaml.Marshal(obj)
		if err != nil {
			panic(fmt.Sprintf("failed to marshal yaml document: %v", err))
		}
		docs = append(docs, strings.TrimSpace(string(data)))
	}
	return strings.Join(docs, "\n---\n")
}

// testerPodYAML returns a busybox pod that sleeps forever, used as the source
// of in-cluster wget probes against the demo's rendered Services.
func testerPodYAML(dpNamespace string) string {
	pod := &corev1.Pod{
		TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "Pod"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "msd-tester",
			Namespace: dpNamespace,
			Labels: map[string]string{
				"app":                       "msd-tester",
				"openchoreo.dev/managed-by": "e2e-microservicesdemo",
			},
		},
		Spec: corev1.PodSpec{Containers: []corev1.Container{{
			Name:    "tester",
			Image:   "busybox:1.36",
			Command: []string{"sleep", "3600"},
		}}},
	}
	return mustYAMLDocs(pod)
}
