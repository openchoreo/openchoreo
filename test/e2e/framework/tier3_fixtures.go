// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package framework

import (
	"fmt"
	"path/filepath"
	"time"
)

const (
	Tier3GiteaNamespace          = "e2e-gitea"
	Tier3UpstreamSampleWorkloads = "https://github.com/openchoreo/sample-workloads.git"
	Tier3SampleWorkloadsRepo     = "sample-workloads"
	Tier3NoWorkloadRepo          = "no-workload-sample"
	Tier3PaketoNodeRepo          = "paketo-node-sample"

	tier3SampleWorkloadsMarker = "tier3-sample-workloads-ready"
	tier3BuildSourcesMarker    = "tier3-build-sources-ready"
)

// EnsureTier3SampleWorkloads ensures the shared Tier 3 Gitea install and
// sample-workloads mirror exist in the current e2e cluster. The fixture is
// intentionally scoped to one k3d cluster/job; it never crosses CI tiers.
func EnsureTier3SampleWorkloads(kubeContext string) error {
	if tier3FixtureMarkerExists(kubeContext, tier3SampleWorkloadsMarker) {
		return nil
	}
	if err := InstallGitea(kubeContext, Tier3GiteaNamespace); err != nil {
		return err
	}
	if err := MigrateRepo(kubeContext, Tier3GiteaNamespace,
		Tier3SampleWorkloadsRepo, Tier3UpstreamSampleWorkloads); err != nil {
		return err
	}
	return applyTier3FixtureMarker(kubeContext, tier3SampleWorkloadsMarker)
}

// EnsureTier3BuildSources extends EnsureTier3SampleWorkloads with the local
// build fixture repositories used by the Tier 3 build matrix.
func EnsureTier3BuildSources(kubeContext string) error {
	if tier3FixtureMarkerExists(kubeContext, tier3BuildSourcesMarker) {
		return nil
	}
	if err := EnsureTier3SampleWorkloads(kubeContext); err != nil {
		return err
	}
	repoRoot, err := RepoRoot()
	if err != nil {
		return err
	}
	if err := EnsureGiteaRepo(kubeContext, Tier3GiteaNamespace, Tier3NoWorkloadRepo); err != nil {
		return err
	}
	if err := PushTree(kubeContext, Tier3GiteaNamespace, Tier3NoWorkloadRepo, "main",
		filepath.Join(repoRoot, "test/e2e/fixtures/build/no-workload")); err != nil {
		return err
	}
	if err := EnsureGiteaRepo(kubeContext, Tier3GiteaNamespace, Tier3PaketoNodeRepo); err != nil {
		return err
	}
	if err := PushTree(kubeContext, Tier3GiteaNamespace, Tier3PaketoNodeRepo, "main",
		filepath.Join(repoRoot, "test/e2e/fixtures/build/paketo-node")); err != nil {
		return err
	}
	return applyTier3FixtureMarker(kubeContext, tier3BuildSourcesMarker)
}

func tier3FixtureMarkerExists(kubeContext, name string) bool {
	_, err := Kubectl(kubeContext, "get", "configmap", name, "-n", Tier3GiteaNamespace)
	return err == nil
}

func applyTier3FixtureMarker(kubeContext, name string) error {
	_, err := KubectlApplyLiteral(kubeContext, fmt.Sprintf(`apiVersion: v1
kind: ConfigMap
metadata:
  name: %s
  namespace: %s
  labels:
    openchoreo.dev/e2e-managed: "true"
data:
  readyAt: "%s"
`, name, Tier3GiteaNamespace, time.Now().UTC().Format(time.RFC3339)))
	return err
}
