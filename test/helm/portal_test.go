// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package helm

import (
	"bytes"
	"errors"
	"os/exec"
	"strings"
	"testing"
)

// The portal-off and portal-on fixtures are the ones this package already has:
// helmGuardOverrides (values_test.go) clears the chart's placeholder-domain
// guards with backstage.enabled=false and nothing portal-specific, which is
// exactly a portal-less install; backstageGuardOverrides (backstage_test.go)
// supplies the portal's own required values.

// renderControlPlane renders the control-plane chart and returns the manifests
// it produces. The test is skipped when helm is not installed, so it adds
// coverage where the binary is available without making the package untestable
// where it is not.
func renderControlPlane(t *testing.T, args ...string) string {
	t.Helper()

	rendered, err := tryRenderControlPlane(t, args...)
	if err != nil {
		t.Fatalf("failed to render %s: %v", controlPlaneChart, err)
	}
	return rendered
}

// tryRenderControlPlane renders the chart and hands back the failure instead of
// ending the test, for the cases that assert a render succeeds or fails.
func tryRenderControlPlane(t *testing.T, args ...string) (string, error) {
	t.Helper()

	helm, err := exec.LookPath("helm")
	if err != nil {
		t.Skip("helm is not installed; skipping the rendered chart check")
	}

	full := append([]string{
		"template", "openchoreo", controlPlaneChart,
		"--namespace", "openchoreo-control-plane",
		// Pinned so the render does not depend on a reachable cluster.
		"--kube-version", "1.31.0",
	}, args...)

	var stderr bytes.Buffer
	cmd := exec.CommandContext(t.Context(), helm, full...)
	cmd.Stderr = &stderr
	rendered, err := cmd.Output()
	if err != nil {
		return "", errors.New(err.Error() + "\n" + stderr.String())
	}
	return string(rendered), nil
}

// leaves nothing of the portal behind. The portal's own templates are each gated,
// so this guards against a new one landing ungated.
func TestChart_PortalDisabled_RendersNoPortalObjects(t *testing.T) {
	rendered := renderControlPlane(t, helmGuardOverrides...)

	if strings.Contains(rendered, "app.kubernetes.io/component: backstage") {
		t.Error("rendering with backstage.enabled=false still produced an object labelled " +
			"app.kubernetes.io/component: backstage")
	}
	// The event forwarder's default webhook endpoint names the portal Service, and
	// the portal's own objects are named after it, so a bare name match would be
	// noisy. Assert on the object names instead.
	for _, name := range []string{
		"name: backstage\n",
		"name: backstage-db\n",
		"name: backstage-ci-config\n",
		"name: backstage-csp-config\n",
	} {
		if strings.Contains(rendered, name) {
			t.Errorf("rendering with backstage.enabled=false still produced an object named %q",
				strings.TrimSuffix(strings.TrimPrefix(name, "name: "), "\n"))
		}
	}
}

// TestChart_PortalDisabled_NeedsNoPortalValues pins the validateHostnames
// short-circuit in _helpers.tpl: with the portal off, an install must not be
// asked for backstage.secretName, baseUrl or http.hostnames. Every e2e run
// depends on this, but only implicitly through helmGuardOverrides.
func TestChart_PortalDisabled_NeedsNoPortalValues(t *testing.T) {
	if _, err := tryRenderControlPlane(t, helmGuardOverrides...); err != nil {
		t.Fatalf("a portal-less install should not require the portal's own values: %v", err)
	}
}

// TestAuthzBootstrap_CatalogReaderFollowsPortalFlag pins which half of the

// TestAPINetworkPolicy_PortalSelectorFollowsPortalFlag pins that the API
// server's NetworkPolicy stops allowing the portal in once the portal is gone.
func TestAPINetworkPolicy_PortalSelectorFollowsPortalFlag(t *testing.T) {
	render := func(t *testing.T, overrides []string) string {
		t.Helper()
		args := append(append([]string{}, overrides...),
			"--set", "openchoreoApi.networkPolicy.enabled=true",
			"--show-only", "templates/openchoreo-api/networkpolicy.yaml")
		return renderControlPlane(t, args...)
	}

	if got := render(t, backstageGuardOverrides); !strings.Contains(got, "component: backstage") {
		t.Error("with the portal installed, the API NetworkPolicy should still allow it in")
	}
	if got := render(t, helmGuardOverrides); strings.Contains(got, "component: backstage") {
		t.Error("with the portal uninstalled, the API NetworkPolicy still carries its podSelector")
	}
}
