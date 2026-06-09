// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package framework

import (
	"fmt"
	"strings"
	"time"
)

// FluxNamespace is the namespace Flux's controllers live in after InstallFlux.
const FluxNamespace = "flux-system"

// fluxInstallManifest is the pinned URL of the upstream Flux installer YAML.
// Pinned to a release so e2e behaviour is reproducible week-over-week — bump
// alongside the rest of the e2e dependency versions in make/e2e.mk when the
// minimum-supported Flux version moves.
const fluxInstallManifest = "https://github.com/fluxcd/flux2/releases/download/v2.4.0/install.yaml"

// InstallFlux applies the upstream Flux v2 install bundle (CRDs + source +
// kustomize controllers) and waits for the core deployments to reach
// Available. We do not need the helm/notification controllers for the gitops
// suite, but applying the bundle is the path of least resistance.
func InstallFlux(kubeContext string) error {
	if _, err := Kubectl(kubeContext, "apply", "--server-side", "-f", fluxInstallManifest); err != nil {
		return fmt.Errorf("failed to apply flux install manifest: %w", err)
	}
	// The install manifest creates the namespace; wait for the two controllers
	// we exercise. `kubectl wait` retries internally — short timeout is fine.
	for _, dep := range []string{"source-controller", "kustomize-controller"} {
		if _, err := Kubectl(kubeContext,
			"-n", FluxNamespace,
			"wait", "--for=condition=available",
			"deployment/"+dep,
			"--timeout=5m",
		); err != nil {
			return fmt.Errorf("flux %s did not become available: %w", dep, err)
		}
	}
	return nil
}

// ApplyGitRepository applies a Flux GitRepository pointing at the given URL.
// Branch defaults to "main" when empty. The poll interval is intentionally
// short (15s) so e2e specs don't burn time waiting for Flux to notice an edit.
func ApplyGitRepository(kubeContext, namespace, name, repoURL, branch string) error {
	if branch == "" {
		branch = "main"
	}
	manifest := fmt.Sprintf(`apiVersion: source.toolkit.fluxcd.io/v1
kind: GitRepository
metadata:
  name: %[1]s
  namespace: %[2]s
spec:
  interval: 15s
  url: %[3]s
  ref:
    branch: %[4]s
  # In-cluster Gitea is HTTP-only and serves no signed commits.
  ignore: |
    /.git
`, name, namespace, repoURL, branch)
	if _, err := KubectlApplyLiteral(kubeContext, manifest); err != nil {
		return fmt.Errorf("failed to apply GitRepository %q: %w", name, err)
	}
	return nil
}

// ApplyKustomization applies a Flux Kustomization that reconciles `path`
// (relative to the GitRepository root) into `targetNamespace`. Setting
// targetNamespace empty leaves each object's own metadata.namespace untouched
// — required when the git tree contains both CP-namespaced and other resources.
func ApplyKustomization(kubeContext, namespace, name, sourceName, path, targetNamespace string) error {
	tnLine := ""
	if targetNamespace != "" {
		tnLine = fmt.Sprintf("  targetNamespace: %s\n", targetNamespace)
	}
	manifest := fmt.Sprintf(`apiVersion: kustomize.toolkit.fluxcd.io/v1
kind: Kustomization
metadata:
  name: %[1]s
  namespace: %[2]s
spec:
  interval: 15s
  retryInterval: 15s
  sourceRef:
    kind: GitRepository
    name: %[3]s
  path: %[4]s
  prune: true
%[5]s`, name, namespace, sourceName, ensureLeadingSlash(path), tnLine)
	if _, err := KubectlApplyLiteral(kubeContext, manifest); err != nil {
		return fmt.Errorf("failed to apply Kustomization %q: %w", name, err)
	}
	return nil
}

// WaitForKustomizationReady polls until the Flux Kustomization reports the
// Ready=True condition and returns an error if it does not become ready before
// the timeout.
func WaitForKustomizationReady(kubeContext, namespace, name string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		out, err := KubectlGetJsonpath(kubeContext, namespace, "kustomization", name,
			`{.status.conditions[?(@.type=="Ready")].status}`)
		if err == nil && strings.TrimSpace(out) == "True" {
			return nil
		}
		time.Sleep(3 * time.Second)
	}
	return fmt.Errorf("kustomization %s/%s did not become Ready within %s", namespace, name, timeout)
}

func ensureLeadingSlash(p string) string {
	if p == "" {
		return "./"
	}
	if !strings.HasPrefix(p, "/") && !strings.HasPrefix(p, "./") {
		return "./" + p
	}
	return p
}
