// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package k8sresources

import (
	"encoding/json"
	"reflect"
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
								map[string]any{
									"name":    "task",
									"image":   "busybox",
									"command": []any{"/entrypoint.sh"},
									"args":    []any{"--schedule", "nightly"},
									"env": []any{
										map[string]any{"name": "ENV_VAR", "value": "test"},
									},
								},
							},
						},
					},
				},
			},
		},
	}
}

func sampleMultiContainerCronJob() map[string]any {
	cj := sampleCronJob()
	spec := cj["spec"].(map[string]any)
	jt := spec["jobTemplate"].(map[string]any)
	jtSpec := jt["spec"].(map[string]any)
	tmpl := jtSpec["template"].(map[string]any)
	podSpec := tmpl["spec"].(map[string]any)
	podSpec["containers"] = []any{
		map[string]any{
			"name":    "task",
			"image":   "busybox",
			"command": []any{"/entrypoint.sh"},
			"args":    []any{"--schedule", "nightly"},
		},
		map[string]any{
			"name":    "sidecar",
			"image":   "sidecar:latest",
			"command": []any{"/sidecar"},
			"args":    []any{"--sidecar-arg", "true"},
		},
	}
	return cj
}

func deepCopyMap(t *testing.T, src map[string]any) map[string]any {
	t.Helper()
	b, err := json.Marshal(src)
	if err != nil {
		t.Fatalf("failed to marshal map: %v", err)
	}
	var dst map[string]any
	if err := json.Unmarshal(b, &dst); err != nil {
		t.Fatalf("failed to unmarshal map: %v", err)
	}
	return dst
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

func TestBuildJobFromCronJob_Arguments(t *testing.T) {
	tests := []struct {
		name          string
		cronJob       func() map[string]any
		args          *[]string
		wantPrimary   []any
		wantSidecar   []any
		checkSidecars bool
	}{
		{
			name:        "nil override preserves inherited cronjob args",
			cronJob:     sampleCronJob,
			args:        nil,
			wantPrimary: []any{"--schedule", "nightly"},
		},
		{
			name:        "non-empty override replaces inherited args in exact order",
			cronJob:     sampleCronJob,
			args:        &[]string{"--date", "2026-08-25", "--mode", "rebuild"},
			wantPrimary: []any{"--date", "2026-08-25", "--mode", "rebuild"},
		},
		{
			name:        "explicit empty override clears inherited args",
			cronJob:     sampleCronJob,
			args:        &[]string{},
			wantPrimary: []any{},
		},
		{
			name:          "multi-container template updates only primary container",
			cronJob:       sampleMultiContainerCronJob,
			args:          &[]string{"--task-override", "value1"},
			wantPrimary:   []any{"--task-override", "value1"},
			wantSidecar:   []any{"--sidecar-arg", "true"},
			checkSidecars: true,
		},
		{
			name:    "override with spaces quotes shell metacharacters and unicode preserves exact string elements",
			cronJob: sampleCronJob,
			args: &[]string{
				"--query", "SELECT * FROM users WHERE name = 'O''Reilly';",
				"--shell", "$VAR && rm -rf / | `whoami` > /dev/null",
				"--unicode", "🚀 openchoreo 日本語 🔥",
			},
			wantPrimary: []any{
				"--query", "SELECT * FROM users WHERE name = 'O''Reilly';",
				"--shell", "$VAR && rm -rf / | `whoami` > /dev/null",
				"--unicode", "🚀 openchoreo 日本語 🔥",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			job, err := buildJobFromCronJob(tt.cronJob(), tt.args)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			spec := job["spec"].(map[string]any)
			tmpl := spec["template"].(map[string]any)
			podSpec := tmpl["spec"].(map[string]any)
			containers := podSpec["containers"].([]any)

			primary := containers[0].(map[string]any)
			primaryArgs := primary["args"]
			if !reflect.DeepEqual(primaryArgs, tt.wantPrimary) {
				t.Errorf("primary container args mismatch: got %#v, want %#v", primaryArgs, tt.wantPrimary)
			}

			// Ensure command is preserved and not altered
			if !reflect.DeepEqual(primary["command"], []any{"/entrypoint.sh"}) {
				t.Errorf("primary container command altered: got %#v", primary["command"])
			}

			if tt.checkSidecars {
				if len(containers) < 2 {
					t.Fatalf("expected at least 2 containers, got %d", len(containers))
				}
				sidecar := containers[1].(map[string]any)
				sidecarArgs := sidecar["args"]
				if !reflect.DeepEqual(sidecarArgs, tt.wantSidecar) {
					t.Errorf("sidecar container args modified: got %#v, want %#v", sidecarArgs, tt.wantSidecar)
				}
				if !reflect.DeepEqual(sidecar["command"], []any{"/sidecar"}) {
					t.Errorf("sidecar container command modified: got %#v", sidecar["command"])
				}
			}
		})
	}
}

func TestBuildJobFromCronJob_InvalidContainers(t *testing.T) {
	args := &[]string{"--test"}

	tests := []struct {
		name    string
		cronJob map[string]any
	}{
		{
			name: "missing template",
			cronJob: map[string]any{
				"metadata": map[string]any{"name": "task", "uid": "u-1"},
				"spec": map[string]any{
					"jobTemplate": map[string]any{
						"spec": map[string]any{},
					},
				},
			},
		},
		{
			name: "template is not a map",
			cronJob: map[string]any{
				"metadata": map[string]any{"name": "task", "uid": "u-1"},
				"spec": map[string]any{
					"jobTemplate": map[string]any{
						"spec": map[string]any{
							"template": "not-a-map",
						},
					},
				},
			},
		},
		{
			name: "missing template.spec",
			cronJob: map[string]any{
				"metadata": map[string]any{"name": "task", "uid": "u-1"},
				"spec": map[string]any{
					"jobTemplate": map[string]any{
						"spec": map[string]any{
							"template": map[string]any{},
						},
					},
				},
			},
		},
		{
			name: "template.spec is not a map",
			cronJob: map[string]any{
				"metadata": map[string]any{"name": "task", "uid": "u-1"},
				"spec": map[string]any{
					"jobTemplate": map[string]any{
						"spec": map[string]any{
							"template": map[string]any{
								"spec": "not-a-map",
							},
						},
					},
				},
			},
		},
		{
			name: "missing containers in template.spec",
			cronJob: map[string]any{
				"metadata": map[string]any{"name": "task", "uid": "u-1"},
				"spec": map[string]any{
					"jobTemplate": map[string]any{
						"spec": map[string]any{
							"template": map[string]any{
								"spec": map[string]any{},
							},
						},
					},
				},
			},
		},
		{
			name: "empty containers slice",
			cronJob: map[string]any{
				"metadata": map[string]any{"name": "task", "uid": "u-1"},
				"spec": map[string]any{
					"jobTemplate": map[string]any{
						"spec": map[string]any{
							"template": map[string]any{
								"spec": map[string]any{
									"containers": []any{},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "invalid primary container entry",
			cronJob: map[string]any{
				"metadata": map[string]any{"name": "task", "uid": "u-1"},
				"spec": map[string]any{
					"jobTemplate": map[string]any{
						"spec": map[string]any{
							"template": map[string]any{
								"spec": map[string]any{
									"containers": []any{"not-a-container-map"},
								},
							},
						},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := buildJobFromCronJob(tt.cronJob, args)
			if err == nil {
				t.Fatalf("expected error for invalid container structure %q, got nil", tt.name)
			}
		})
	}
}

func TestBuildJobFromCronJob_InputImmutability(t *testing.T) {
	inputCJ := sampleCronJob()
	originalCopy := deepCopyMap(t, inputCJ)

	overrideArgs := &[]string{"--new-arg-1", "--new-arg-2"}
	_, err := buildJobFromCronJob(inputCJ, overrideArgs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify that the input CronJob map was not modified at all
	if !reflect.DeepEqual(inputCJ, originalCopy) {
		t.Fatalf("input CronJob map was mutated during buildJobFromCronJob: got %#v, want %#v", inputCJ, originalCopy)
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
