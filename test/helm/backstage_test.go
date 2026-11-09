// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package helm

import (
	"bytes"
	"os/exec"
	"testing"

	"github.com/google/go-cmp/cmp"
	"gopkg.in/yaml.v3"
)

// backstageGuardOverrides clear the chart's placeholder-domain and
// required-value guards with Backstage enabled, so its Deployment renders.
var backstageGuardOverrides = []string{
	"--set", "openchoreoApi.http.hostnames[0]=api.example.com",
	"--set", "gateway.tls.hostname=example.com",
	"--set", "security.oidc.issuer=https://idp.example.com",
	"--set", "backstage.http.hostnames[0]=portal.example.com",
	"--set", "backstage.baseUrl=https://portal.example.com",
	"--set", "backstage.secretName=backstage-secrets",
}

// renderedProbe is the part of a rendered container probe these tests check.
type renderedProbe struct {
	HTTPGet struct {
		Path string `yaml:"path"`
		Port string `yaml:"port"`
	} `yaml:"httpGet"`
	InitialDelaySeconds int `yaml:"initialDelaySeconds"`
	PeriodSeconds       int `yaml:"periodSeconds"`
	TimeoutSeconds      int `yaml:"timeoutSeconds"`
	FailureThreshold    int `yaml:"failureThreshold"`
}

// renderedContainer is the part of a rendered container these tests check.
type renderedContainer struct {
	Name           string         `yaml:"name"`
	StartupProbe   *renderedProbe `yaml:"startupProbe"`
	LivenessProbe  *renderedProbe `yaml:"livenessProbe"`
	ReadinessProbe *renderedProbe `yaml:"readinessProbe"`
}

// renderBackstageContainer renders the Backstage Deployment and returns its
// backstage container. extraArgs are appended to the helm invocation. The test
// is skipped when helm is not installed.
func renderBackstageContainer(t *testing.T, extraArgs ...string) renderedContainer {
	t.Helper()

	helm, err := exec.LookPath("helm")
	if err != nil {
		t.Skip("helm is not installed; skipping the rendered Backstage Deployment check")
	}

	args := append([]string{
		"template", "openchoreo", controlPlaneChart,
		"--namespace", "openchoreo-control-plane",
		"--show-only", "templates/backstage/deployment.yaml",
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
					Containers []renderedContainer `yaml:"containers"`
				} `yaml:"spec"`
			} `yaml:"template"`
		} `yaml:"spec"`
	}
	if err := yaml.Unmarshal(rendered, &deployment); err != nil {
		t.Fatalf("failed to parse the rendered Deployment: %v\n%s", err, rendered)
	}
	for _, c := range deployment.Spec.Template.Spec.Containers {
		if c.Name == "backstage" {
			return c
		}
	}
	t.Fatalf("the rendered Deployment has no backstage container:\n%s", rendered)
	return renderedContainer{}
}

// TestBackstageDeployment_StartupProbeByDefault pins the startup probe that
// keeps the liveness probe from restarting Backstage while it is still
// loading its plugins on a slow machine (#5126).
func TestBackstageDeployment_StartupProbeByDefault(t *testing.T) {
	c := renderBackstageContainer(t)

	if c.StartupProbe == nil {
		t.Fatal("the Backstage container must have a startup probe by default")
	}
	want := renderedProbe{PeriodSeconds: 10, TimeoutSeconds: 10, FailureThreshold: 30}
	want.HTTPGet.Path = "/healthcheck"
	want.HTTPGet.Port = "http"
	if diff := cmp.Diff(want, *c.StartupProbe); diff != "" {
		t.Errorf("unexpected default startup probe (-want +got):\n%s", diff)
	}
	if c.LivenessProbe == nil || c.ReadinessProbe == nil {
		t.Error("the liveness and readiness probes must still be rendered")
	}
}

// TestBackstageDeployment_StartupProbeOverrides checks that every startup
// probe value reaches the Deployment.
func TestBackstageDeployment_StartupProbeOverrides(t *testing.T) {
	c := renderBackstageContainer(t,
		"--set", "backstage.startupProbe.periodSeconds=5",
		"--set", "backstage.startupProbe.timeoutSeconds=3",
		"--set", "backstage.startupProbe.failureThreshold=60",
	)

	if c.StartupProbe == nil {
		t.Fatal("the Backstage container must have a startup probe")
	}
	got := []int{c.StartupProbe.PeriodSeconds, c.StartupProbe.TimeoutSeconds, c.StartupProbe.FailureThreshold}
	if diff := cmp.Diff([]int{5, 3, 60}, got); diff != "" {
		t.Errorf("startup probe period, timeout and failure threshold (-want +got):\n%s", diff)
	}
}

// TestBackstageDeployment_StartupProbeDisabled checks that the startup probe
// can be turned off, leaving the liveness and readiness probes in place.
func TestBackstageDeployment_StartupProbeDisabled(t *testing.T) {
	c := renderBackstageContainer(t, "--set", "backstage.startupProbe.enabled=false")

	if c.StartupProbe != nil {
		t.Errorf("backstage.startupProbe.enabled=false must not render a startup probe, got %+v", *c.StartupProbe)
	}
	if c.LivenessProbe == nil || c.ReadinessProbe == nil {
		t.Error("the liveness and readiness probes must still be rendered")
	}
}

// TestBackstageDeployment_StartupProbeNull checks that clearing the whole
// startupProbe object with null renders the Deployment without the probe
// instead of failing on a nil value.
func TestBackstageDeployment_StartupProbeNull(t *testing.T) {
	c := renderBackstageContainer(t, "--set", "backstage.startupProbe=null")

	if c.StartupProbe != nil {
		t.Errorf("backstage.startupProbe=null must not render a startup probe, got %+v", *c.StartupProbe)
	}
	if c.LivenessProbe == nil || c.ReadinessProbe == nil {
		t.Error("the liveness and readiness probes must still be rendered")
	}
}

// TestBackstageSchema_StartupProbeFitsInt32 checks that the schema rejects
// startup probe values Kubernetes cannot store (probe fields are int32), so
// they fail at render time instead of when the Deployment is applied.
func TestBackstageSchema_StartupProbeFitsInt32(t *testing.T) {
	helm, err := exec.LookPath("helm")
	if err != nil {
		t.Skip("helm is not installed; skipping the startup probe schema check")
	}

	for _, field := range []string{"periodSeconds", "timeoutSeconds", "failureThreshold"} {
		t.Run(field, func(t *testing.T) {
			args := append([]string{
				"template", "openchoreo", controlPlaneChart,
				"--namespace", "openchoreo-control-plane",
				"--show-only", "templates/backstage/deployment.yaml",
			}, backstageGuardOverrides...)

			ok := exec.CommandContext(t.Context(), helm,
				append(args, "--set", "backstage.startupProbe."+field+"=2147483647")...)
			if out, err := ok.CombinedOutput(); err != nil {
				t.Fatalf("%s=2147483647 must be accepted: %v\n%s", field, err, out)
			}

			tooBig := exec.CommandContext(t.Context(), helm,
				append(args, "--set", "backstage.startupProbe."+field+"=2147483648")...)
			out, err := tooBig.CombinedOutput()
			if err == nil {
				t.Fatalf("%s=2147483648 must be rejected by the schema, got:\n%s", field, out)
			}
			if !bytes.Contains(out, []byte("maximum")) {
				t.Errorf("%s=2147483648 must fail on the schema maximum, got:\n%s", field, out)
			}
		})
	}
}
