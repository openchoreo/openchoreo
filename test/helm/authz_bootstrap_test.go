// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package helm

import (
	"os/exec"
	"regexp"
	"strings"
	"testing"
)

func TestHelmAuthzBootstrapCanBeDisabledIndependently(t *testing.T) {
	helm, err := exec.LookPath("helm")
	if err != nil {
		t.Skip("helm is not installed")
	}

	const setting = "openchoreoApi.config.security.authorization.bootstrap.enabled=false"

	args := []string{
		"template", "openchoreo", controlPlaneChart,
		"--namespace", "openchoreo-control-plane",
	}
	args = append(args, helmGuardOverrides...)
	args = append(args, "--set", setting)

	output, err := exec.CommandContext(t.Context(), helm, args...).CombinedOutput()
	if err != nil {
		t.Fatalf("bootstrap opt-out must render successfully: %v\n%s", err, output)
	}
	if strings.Contains(string(output), "name: openchoreo-authz-bootstrap") {
		t.Fatal("bootstrap resources still rendered when bootstrap.enabled=false")
	}

	config := renderAPIConfigYAML(t, "--set", setting)
	if !regexp.MustCompile(`(?m)^[ \t]*authorization:[ \t]*\n[ \t]+enabled:[ \t]*true[ \t]*$`).MatchString(config) {
		t.Fatal("disabling bootstrap must not disable authorization enforcement")
	}
}

func TestHelmAuthzBootstrapRenderingMatrix(t *testing.T) {
	helm, err := exec.LookPath("helm")
	if err != nil {
		t.Skip("helm is not installed")
	}

	tests := []struct {
		name          string
		settings      []string
		wantResources int
	}{
		{"default", nil, 5},
		{"explicitly_enabled", []string{
			"openchoreoApi.config.security.authorization.bootstrap.enabled=true",
		}, 5},
		{"bootstrap_disabled", []string{
			"openchoreoApi.config.security.authorization.bootstrap.enabled=false",
		}, 0},
		{"authorization_disabled", []string{
			"security.authz.enabled=false",
		}, 0},
		{"both_disabled", []string{
			"security.authz.enabled=false",
			"openchoreoApi.config.security.authorization.bootstrap.enabled=false",
		}, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := []string{
				"template", "openchoreo", controlPlaneChart,
				"--namespace", "openchoreo-control-plane",
			}
			args = append(args, helmGuardOverrides...)
			for _, setting := range tt.settings {
				args = append(args, "--set", setting)
			}

			output, err := exec.CommandContext(t.Context(), helm, args...).CombinedOutput()
			if err != nil {
				t.Fatalf("helm rendering failed: %v\n%s", err, output)
			}

			got := strings.Count(
				string(output),
				"# Source: openchoreo-control-plane/templates/authz/bootstrap-",
			)
			if got != tt.wantResources {
				t.Fatalf("bootstrap resources: got %d, want %d", got, tt.wantResources)
			}
		})
	}
}
