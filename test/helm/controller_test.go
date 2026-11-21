// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package helm

import (
	"bytes"
	"os/exec"
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// renderControllerManagerArgs renders the controller-manager Deployment and
// returns the args of its manager container. extraArgs are appended to the helm
// invocation. The test is skipped when helm is not installed.
func renderControllerManagerArgs(t *testing.T, extraArgs ...string) []string {
	t.Helper()

	helm, err := exec.LookPath("helm")
	if err != nil {
		t.Skip("helm is not installed; skipping the rendered controller-manager Deployment check")
	}

	args := append([]string{
		"template", "openchoreo", controlPlaneChart,
		"--namespace", "openchoreo-control-plane",
		"--show-only", "templates/controller-manager/deployment.yaml",
	}, backstageGuardOverrides...)
	args = append(args, extraArgs...)

	var stderr bytes.Buffer
	cmd := exec.CommandContext(t.Context(), helm, args...)
	cmd.Stderr = &stderr
	rendered, err := cmd.Output()
	if err != nil {
		t.Fatalf("failed to render %s: %v\n%s", controlPlaneChart, err, stderr.String())
	}

	var deployment struct {
		Spec struct {
			Template struct {
				Spec struct {
					Containers []struct {
						Name string   `yaml:"name"`
						Args []string `yaml:"args"`
					} `yaml:"containers"`
				} `yaml:"spec"`
			} `yaml:"template"`
		} `yaml:"spec"`
	}
	if err := yaml.Unmarshal(rendered, &deployment); err != nil {
		t.Fatalf("failed to parse the rendered Deployment: %v\n%s", err, rendered)
	}
	for _, c := range deployment.Spec.Template.Spec.Containers {
		if c.Name == "manager" {
			return c.Args
		}
	}
	t.Fatalf("the rendered Deployment has no manager container:\n%s", rendered)
	return nil
}

// TestControllerManager_ComponentRevisionHistoryLimit pins the flag the
// controllerManager.manager.componentRevisionHistoryLimit value is passed as,
// including an explicit 0, and that the old --revision-history-limit flag is
// no longer rendered.
func TestControllerManager_ComponentRevisionHistoryLimit(t *testing.T) {
	tests := []struct {
		name      string
		extraArgs []string
		want      string
	}{
		{name: "the chart default", want: "--component-revision-history-limit=10"},
		{
			name:      "an explicit 0",
			extraArgs: []string{"--set", "controllerManager.manager.componentRevisionHistoryLimit=0"},
			want:      "--component-revision-history-limit=0",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := renderControllerManagerArgs(t, tt.extraArgs...)
			if !slices.Contains(args, tt.want) {
				t.Errorf("manager args do not contain %q: %v", tt.want, args)
			}
			for _, arg := range args {
				if strings.HasPrefix(arg, "--revision-history-limit") {
					t.Errorf("manager args still pass the old flag: %q", arg)
				}
			}
		})
	}
}
