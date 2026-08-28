// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package k8sresources

import (
	"errors"
	"strings"
	"testing"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/models"
)

const kindCronJob = "CronJob"

// templateArg is the args value baked into the sample CronJob's jobTemplate.
const templateArg = "--from-template"

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
	job, err := buildJobFromCronJob(sampleCronJob(), nil)
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
			if _, err := buildJobFromCronJob(cj, nil); err == nil {
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

// containerFromJob returns the single container of a built Job manifest.
func containerFromJob(t *testing.T, job map[string]any) map[string]any {
	t.Helper()
	spec := job["spec"].(map[string]any)
	podSpec := spec["template"].(map[string]any)["spec"].(map[string]any)
	containers := podSpec["containers"].([]any)
	if len(containers) != 1 {
		t.Fatalf("expected 1 container, got %d", len(containers))
	}
	return containers[0].(map[string]any)
}

// TestBuildJobFromCronJobArgsOverride covers the args override applied to the created Job.
func TestBuildJobFromCronJobArgsOverride(t *testing.T) {
	t.Run("nil overrides keeps template args", func(t *testing.T) {
		cj := sampleCronJob()
		container := cronJobContainer(t, cj)
		container["args"] = []any{templateArg}

		job, err := buildJobFromCronJob(cj, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		args := containerFromJob(t, job)["args"].([]any)
		if len(args) != 1 || args[0] != templateArg {
			t.Fatalf("template args should be preserved, got %v", args)
		}
	})

	t.Run("nil Args field keeps template args", func(t *testing.T) {
		cj := sampleCronJob()
		cronJobContainer(t, cj)["args"] = []any{templateArg}

		job, err := buildJobFromCronJob(cj, &models.CronJobTriggerRequest{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		args := containerFromJob(t, job)["args"].([]any)
		if len(args) != 1 || args[0] != templateArg {
			t.Fatalf("template args should be preserved, got %v", args)
		}
	})

	t.Run("args replace template args", func(t *testing.T) {
		cj := sampleCronJob()
		cronJobContainer(t, cj)["args"] = []any{templateArg}

		job, err := buildJobFromCronJob(cj, &models.CronJobTriggerRequest{
			Args: []string{"--mode", "backfill"},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		args := containerFromJob(t, job)["args"].([]any)
		if len(args) != 2 || args[0] != "--mode" || args[1] != "backfill" {
			t.Fatalf("args not replaced, got %v", args)
		}
	})

	t.Run("empty args clears template args", func(t *testing.T) {
		cj := sampleCronJob()
		cronJobContainer(t, cj)["args"] = []any{templateArg}

		job, err := buildJobFromCronJob(cj, &models.CronJobTriggerRequest{Args: []string{}})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if _, ok := containerFromJob(t, job)["args"]; ok {
			t.Fatal("args should be cleared so the image entrypoint applies")
		}
	})

	// The override must not write back into the CronJob we fetched, or a later read of that
	// object would report args the CronJob does not actually have.
	t.Run("override does not mutate the source cronjob", func(t *testing.T) {
		cj := sampleCronJob()
		cronJobContainer(t, cj)["args"] = []any{templateArg}

		if _, err := buildJobFromCronJob(cj, &models.CronJobTriggerRequest{
			Args: []string{"--mode", "backfill"},
		}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		args := cronJobContainer(t, cj)["args"].([]any)
		if len(args) != 1 || args[0] != templateArg {
			t.Fatalf("source cronjob was mutated: %v", args)
		}
	})
}

// TestApplyArgsOverrideContainerSelection covers which container the args land on.
func TestApplyArgsOverrideContainerSelection(t *testing.T) {
	t.Run("targets the container named main", func(t *testing.T) {
		cj := sampleCronJob()
		podSpec := cronJobPodSpec(t, cj)
		podSpec["containers"] = []any{
			map[string]any{"name": "sidecar", "image": "proxy"},
			map[string]any{"name": mainContainerName, "image": "busybox"},
		}

		job, err := buildJobFromCronJob(cj, &models.CronJobTriggerRequest{Args: []string{"--go"}})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		spec := job["spec"].(map[string]any)
		containers := spec["template"].(map[string]any)["spec"].(map[string]any)["containers"].([]any)
		sidecar := containers[0].(map[string]any)
		main := containers[1].(map[string]any)
		if _, ok := sidecar["args"]; ok {
			t.Fatal("sidecar must not receive the args override")
		}
		if got := main["args"].([]any); len(got) != 1 || got[0] != "--go" {
			t.Fatalf("main container did not receive args: %v", main["args"])
		}
	})

	// Guessing here would silently run the wrong container, so this is an error instead.
	t.Run("multiple containers without main is an error", func(t *testing.T) {
		cj := sampleCronJob()
		cronJobPodSpec(t, cj)["containers"] = []any{
			map[string]any{"name": "one", "image": "busybox"},
			map[string]any{"name": "two", "image": "busybox"},
		}

		_, err := buildJobFromCronJob(cj, &models.CronJobTriggerRequest{Args: []string{"--go"}})
		if !errors.Is(err, ErrTriggerContainerAmbiguous) {
			t.Fatalf("expected ErrTriggerContainerAmbiguous, got %v", err)
		}
	})

	t.Run("no containers is an error", func(t *testing.T) {
		cj := sampleCronJob()
		cronJobPodSpec(t, cj)["containers"] = []any{}

		_, err := buildJobFromCronJob(cj, &models.CronJobTriggerRequest{Args: []string{"--go"}})
		if !errors.Is(err, ErrTriggerContainerAmbiguous) {
			t.Fatalf("expected ErrTriggerContainerAmbiguous, got %v", err)
		}
	})
}

// cronJobPodSpec returns the pod spec inside a sample CronJob's jobTemplate.
func cronJobPodSpec(t *testing.T, cj map[string]any) map[string]any {
	t.Helper()
	podSpec, ok := getNestedMap(cj, "spec", "jobTemplate", "spec", "template", "spec")
	if !ok {
		t.Fatal("sample cronjob has no pod spec")
	}
	return podSpec
}

// cronJobContainer returns the first container from a sample CronJob's jobTemplate.
func cronJobContainer(t *testing.T, cj map[string]any) map[string]any {
	t.Helper()
	containers := cronJobPodSpec(t, cj)["containers"].([]any)
	return containers[0].(map[string]any)
}
