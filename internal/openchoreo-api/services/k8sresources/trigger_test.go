// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package k8sresources

import (
	"strings"
	"testing"
)

const kindCronJob = "CronJob"

func sampleCronJob() map[string]any {
	return map[string]any{
		"apiVersion": "batch/v1",
		"kind":       kindCronJob,
		"metadata": map[string]any{
			"name":      "my-task",
			"namespace": "dp-ns",
			"uid":       "cronjob-uid-123",
		},
		"spec": map[string]any{
			"schedule": "*/5 * * * *",
			"jobTemplate": map[string]any{
				"metadata": map[string]any{
					"labels":      map[string]any{"app": "my-task"},
					"annotations": map[string]any{"team": "platform"},
				},
				"spec": map[string]any{
					"template": map[string]any{
						"spec": map[string]any{
							"restartPolicy": "Never",
							"containers": []any{
								map[string]any{"name": "task", "image": "busybox"},
							},
						},
					},
				},
			},
		},
	}
}

func TestBuildJobFromCronJob(t *testing.T) {
	job, err := buildJobFromCronJob(sampleCronJob())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if job["apiVersion"] != "batch/v1" || job["kind"] != "Job" {
		t.Fatalf("unexpected job type: %v/%v", job["apiVersion"], job["kind"])
	}

	meta := job["metadata"].(map[string]any)

	// Name is prefixed with the cronjob name and carries a unique suffix.
	name := meta["name"].(string)
	if !strings.HasPrefix(name, "my-task-") {
		t.Fatalf("job name %q does not start with cronjob name", name)
	}
	if meta["namespace"] != "dp-ns" {
		t.Fatalf("unexpected namespace: %v", meta["namespace"])
	}

	// Owner reference points back at the CronJob.
	owners := meta["ownerReferences"].([]any)
	if len(owners) != 1 {
		t.Fatalf("expected 1 owner reference, got %d", len(owners))
	}
	owner := owners[0].(map[string]any)
	if owner["kind"] != kindCronJob || owner["name"] != "my-task" || owner["uid"] != "cronjob-uid-123" {
		t.Fatalf("unexpected owner reference: %v", owner)
	}
	if owner["controller"] != true || owner["blockOwnerDeletion"] != true {
		t.Fatalf("owner reference should set controller/blockOwnerDeletion: %v", owner)
	}

	// Manual instantiate annotation is present, plus carried-over jobTemplate annotations.
	anns := meta["annotations"].(map[string]any)
	if anns[instantiateAnnotationKey] != instantiateAnnotationValue {
		t.Fatalf("missing instantiate annotation: %v", anns)
	}
	if anns["team"] != "platform" {
		t.Fatalf("jobTemplate annotation not carried over: %v", anns)
	}

	// Labels carried over from jobTemplate metadata.
	labels := meta["labels"].(map[string]any)
	if labels["app"] != "my-task" {
		t.Fatalf("jobTemplate labels not carried over: %v", labels)
	}

	// Job spec matches the cronjob's jobTemplate.spec.
	spec := job["spec"].(map[string]any)
	if _, ok := spec["template"]; !ok {
		t.Fatalf("job spec should contain template from jobTemplate.spec: %v", spec)
	}
}

func TestBuildJobFromCronJobMissingFields(t *testing.T) {
	cases := map[string]map[string]any{
		"no spec": {
			"metadata": map[string]any{"name": "x", "uid": "u"},
		},
		"no jobTemplate": {
			"metadata": map[string]any{"name": "x", "uid": "u"},
			"spec":     map[string]any{},
		},
		"no name/uid": {
			"spec": map[string]any{"jobTemplate": map[string]any{"spec": map[string]any{}}},
		},
	}
	for name, cj := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := buildJobFromCronJob(cj); err == nil {
				t.Fatalf("expected error for %q", name)
			}
		})
	}
}

func TestMakeJobNameTruncates(t *testing.T) {
	long := strings.Repeat("a", 80)
	name := makeJobName(long)
	if len(name) > maxJobNameLength {
		t.Fatalf("job name exceeds %d chars: %d", maxJobNameLength, len(name))
	}
	if !strings.Contains(name, "-") {
		t.Fatalf("job name should contain a timestamp suffix: %q", name)
	}
}

// TestMakeJobNameUnique guards against the previous behavior where a second trigger within the
// same second produced an identical name. The random suffix must make repeated names distinct.
func TestMakeJobNameUnique(t *testing.T) {
	const n = 100
	seen := make(map[string]struct{}, n)
	for i := 0; i < n; i++ {
		name := makeJobName("my-task")
		if _, dup := seen[name]; dup {
			t.Fatalf("duplicate job name generated within same run: %q", name)
		}
		seen[name] = struct{}{}
	}
}
