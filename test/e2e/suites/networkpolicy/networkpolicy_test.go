// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package e2e

import (
	"fmt"
	"os"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2" //nolint:revive
	. "github.com/onsi/gomega"    //nolint:revive

	"github.com/openchoreo/openchoreo/test/e2e/framework"
)

// Discovered data plane namespace names (populated in BeforeAll).
var (
	dpAcmeProj1Dev string
	dpAcmeProj1Stg string
	dpAcmeProj2Dev string
	dpBetaProj1Dev string
)

type connectivityScenario struct {
	name        string
	intent      string
	sourceNS    func() string
	sourceLabel string
	sourceCtr   string
	targetHost  func() string
	targetPort  int
	expectAllow bool
}

var _ = Describe("NetworkPolicy Enforcement", Ordered, func() {
	containsAny := func(text string, patterns ...string) bool {
		for _, pattern := range patterns {
			if strings.Contains(text, pattern) {
				return true
			}
		}
		return false
	}

	parseServiceFQDN := func(host string) (serviceName, namespace string, ok bool) {
		const suffix = ".svc.cluster.local"
		if !strings.HasSuffix(host, suffix) {
			return "", "", false
		}

		trimmed := strings.TrimSuffix(host, suffix)
		parts := strings.Split(trimmed, ".")
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			return "", "", false
		}
		return parts[0], parts[1], true
	}

	assertSourcePodReady := func(srcNs, srcLabel, srcContainer string) {
		Eventually(func() error {
			_, err := framework.KubectlExecByLabel(kubeContext, srcNs, srcLabel, srcContainer, "true")
			return err
		}, 30*time.Second, 2*time.Second).Should(Succeed(),
			"source pod should be exec-ready in %s (label=%s)", srcNs, srcLabel)
	}

	assertDNSResolution := func(srcNs, srcLabel, srcContainer, targetHost string) {
		Eventually(func() error {
			_, err := framework.KubectlExecByLabel(
				kubeContext,
				srcNs,
				srcLabel,
				srcContainer,
				"nslookup", targetHost,
			)
			return err
		}, 30*time.Second, 2*time.Second).Should(Succeed(),
			"source pod in %s (label=%s) should resolve host %s", srcNs, srcLabel, targetHost)
	}

	assertTargetServiceReady := func(targetHost string) {
		serviceName, namespace, isServiceFQDN := parseServiceFQDN(targetHost)
		if !isServiceFQDN {
			return
		}

		Eventually(func(g Gomega) {
			clusterIP, err := framework.KubectlGetJsonpath(
				kubeContext,
				namespace,
				"service",
				serviceName,
				"{.spec.clusterIP}",
			)
			g.Expect(err).NotTo(HaveOccurred(),
				"failed to read service %s in namespace %s", serviceName, namespace)
			g.Expect(clusterIP).NotTo(BeEmpty(),
				"service %s in namespace %s has empty cluster IP", serviceName, namespace)

			readyAddress, err := framework.KubectlGetJsonpath(
				kubeContext,
				namespace,
				"endpoints",
				serviceName,
				"{.subsets[0].addresses[0].ip}",
			)
			g.Expect(err).NotTo(HaveOccurred(),
				"failed to read endpoints for service %s in namespace %s", serviceName, namespace)
			g.Expect(readyAddress).NotTo(BeEmpty(),
				"service %s in namespace %s has no ready endpoints", serviceName, namespace)
		}, 2*time.Minute, 2*time.Second).Should(Succeed(),
			"service %s in namespace %s should be ready before connectivity assertions", serviceName, namespace)
	}

	// assertConnectivity verifies that traffic flows from a pod (identified by
	// label) to the target host:port. Uses Eventually to tolerate transient
	// failures during setup.
	assertConnectivity := func(srcNs, srcLabel, srcContainer, targetHost string, targetPort int) {
		target := fmt.Sprintf("http://%s:%d", targetHost, targetPort)
		assertSourcePodReady(srcNs, srcLabel, srcContainer)
		assertDNSResolution(srcNs, srcLabel, srcContainer, targetHost)
		assertTargetServiceReady(targetHost)

		Eventually(func() error {
			_, err := framework.KubectlExecByLabel(
				kubeContext,
				srcNs,
				srcLabel,
				srcContainer,
				"wget", "--spider", "--timeout=3", "-q", target,
			)
			return err
		}, 30*time.Second, 2*time.Second).Should(Succeed(),
			"expected connectivity from %s (label=%s) to %s", srcNs, srcLabel, target)
	}

	// assertNoConnectivity verifies that traffic stays blocked over time.
	// It also guards against false positives from DNS or source pod issues.
	assertNoConnectivityOnce := func(srcNs, srcLabel, srcContainer, targetHost string, targetPort int) error {
		target := fmt.Sprintf("http://%s:%d", targetHost, targetPort)
		output, err := framework.KubectlExecByLabel(
			kubeContext,
			srcNs,
			srcLabel,
			srcContainer,
			"wget", "--spider", "--timeout=3", target,
		)
		if err == nil {
			return fmt.Errorf("unexpected connectivity from %s (label=%s) to %s", srcNs, srcLabel, target)
		}

		details := strings.ToLower(strings.Join([]string{output, err.Error()}, "\n"))
		if containsAny(details,
			"bad address",
			"name or service not known",
			"temporary failure in name resolution",
			"no such host",
		) {
			return fmt.Errorf("DNS resolution failed while asserting blocked connectivity to %s: %s", target, details)
		}
		if containsAny(details,
			"no running pod found for selector",
			"failed to query running pod for selector",
		) {
			return fmt.Errorf("source pod not ready while asserting blocked connectivity from %s (label=%s): %s", srcNs, srcLabel, details)
		}

		return nil
	}

	assertNoConnectivity := func(srcNs, srcLabel, srcContainer, targetHost string, targetPort int) {
		target := fmt.Sprintf("http://%s:%d", targetHost, targetPort)
		assertSourcePodReady(srcNs, srcLabel, srcContainer)
		assertDNSResolution(srcNs, srcLabel, srcContainer, targetHost)
		assertTargetServiceReady(targetHost)

		Consistently(func() error {
			return assertNoConnectivityOnce(srcNs, srcLabel, srcContainer, targetHost, targetPort)
		}, 12*time.Second, 2*time.Second).Should(Succeed(),
			"expected connectivity from %s (label=%s) to stay blocked to %s", srcNs, srcLabel, target)
	}

	// fqdn returns the cluster-internal FQDN for a service.
	fqdn := func(svcName, namespace string) string {
		return fmt.Sprintf("%s.%s.svc.cluster.local", svcName, namespace)
	}

	assertNamespaceLabels := func(ns, cpNamespace, project, environment string) {
		Eventually(func(g Gomega) {
			labels, err := framework.Kubectl(
				kubeContext,
				"get", "namespace", ns,
				"-o", "jsonpath={.metadata.labels.openchoreo\\.dev/controlplane-namespace},{.metadata.labels.openchoreo\\.dev/project},{.metadata.labels.openchoreo\\.dev/environment}",
			)
			g.Expect(err).NotTo(HaveOccurred(), "failed to read labels for namespace %s", ns)

			parts := strings.SplitN(labels, ",", 3)
			g.Expect(parts).To(HaveLen(3), "expected 3 label values from namespace %s, got: %s", ns, labels)
			g.Expect(parts[0]).To(Equal(cpNamespace), "unexpected control plane label in namespace %s", ns)
			g.Expect(parts[1]).To(Equal(project), "unexpected project label in namespace %s", ns)
			g.Expect(parts[2]).To(Equal(environment), "unexpected environment label in namespace %s", ns)
		}, 2*time.Minute, 2*time.Second).Should(Succeed(),
			"expected namespace labels in %s to match cp=%s, project=%s, env=%s",
			ns, cpNamespace, project, environment,
		)
	}

	BeforeAll(func() {
		By("reading clientCA from existing DataPlane 'default'")
		clientCA, err := framework.KubectlGetJsonpath(
			kubeContext,
			"default",
			"dataplane",
			"default",
			"{.spec.clusterAgent.clientCA.value}",
		)
		Expect(err).NotTo(HaveOccurred(), "failed to read DataPlane default")
		Expect(clientCA).NotTo(BeEmpty(), "DataPlane default clientCA is empty")

		By("creating control plane namespaces")
		output, err := framework.KubectlApplyLiteral(kubeContext, cpNamespacesYAML())
		Expect(err).NotTo(HaveOccurred(), "failed to create CP namespaces: %s", output)

		By("creating DataPlanes for control plane namespaces")
		output, err = framework.KubectlApplyLiteral(kubeContext, dataPlaneYAML(cpNsAcme, clientCA))
		Expect(err).NotTo(HaveOccurred(), "failed to create DataPlane in %s: %s", cpNsAcme, output)
		output, err = framework.KubectlApplyLiteral(kubeContext, dataPlaneYAML(cpNsBeta, clientCA))
		Expect(err).NotTo(HaveOccurred(), "failed to create DataPlane in %s: %s", cpNsBeta, output)

		By("applying platform resources for acme")
		output, err = framework.KubectlApplyLiteral(kubeContext, platformResourcesYAML(cpNsAcme, []string{"development", "staging"}, []string{"proj1", "proj2"}))
		Expect(err).NotTo(HaveOccurred(), "failed to apply acme platform resources: %s", output)

		By("applying platform resources for beta")
		output, err = framework.KubectlApplyLiteral(kubeContext, platformResourcesYAML(cpNsBeta, []string{"development", "staging"}, []string{"proj1"}))
		Expect(err).NotTo(HaveOccurred(), "failed to apply beta platform resources: %s", output)

		By("applying ComponentTypes for acme")
		output, err = framework.KubectlApplyLiteral(kubeContext, componentTypesYAML(cpNsAcme))
		Expect(err).NotTo(HaveOccurred(), "failed to apply acme ComponentTypes: %s", output)

		By("applying ComponentTypes for beta")
		output, err = framework.KubectlApplyLiteral(kubeContext, componentTypesYAML(cpNsBeta))
		Expect(err).NotTo(HaveOccurred(), "failed to apply beta ComponentTypes: %s", output)

		By("creating Components and Workloads in acme")
		// comp-a: http-echo with project + namespace + external visibility.
		output, err = framework.KubectlApplyLiteral(kubeContext, componentYAML(
			cpNsAcme,
			"proj1",
			"comp-a",
			"deployment/e2e-service",
			"hashicorp/http-echo",
			[]string{"-text=comp-a", "-listen=:8080"},
			map[string]endpointDef{
				"http": {epType: "HTTP", port: 8080, visibility: []string{"project", "namespace", "external"}},
			},
		))
		Expect(err).NotTo(HaveOccurred(), "failed to create comp-a: %s", output)

		// comp-b: http-echo with project-only visibility.
		output, err = framework.KubectlApplyLiteral(kubeContext, componentYAML(
			cpNsAcme,
			"proj1",
			"comp-b",
			"deployment/e2e-service",
			"hashicorp/http-echo",
			[]string{"-text=comp-b", "-listen=:8080"},
			map[string]endpointDef{
				"http": {epType: "HTTP", port: 8080, visibility: []string{"project"}},
			},
		))
		Expect(err).NotTo(HaveOccurred(), "failed to create comp-b: %s", output)

		// client-a: busybox client in proj1.
		output, err = framework.KubectlApplyLiteral(kubeContext, componentYAML(
			cpNsAcme,
			"proj1",
			"client-a",
			"deployment/e2e-worker",
			"busybox:1.36",
			[]string{"sleep", "3600"},
			nil,
		))
		Expect(err).NotTo(HaveOccurred(), "failed to create client-a: %s", output)

		// client-b: busybox client in proj2.
		output, err = framework.KubectlApplyLiteral(kubeContext, componentYAML(
			cpNsAcme,
			"proj2",
			"client-b",
			"deployment/e2e-worker",
			"busybox:1.36",
			[]string{"sleep", "3600"},
			nil,
		))
		Expect(err).NotTo(HaveOccurred(), "failed to create client-b: %s", output)

		By("creating Components and Workloads in beta")
		// comp-d: http-echo with project-only visibility.
		output, err = framework.KubectlApplyLiteral(kubeContext, componentYAML(
			cpNsBeta,
			"proj1",
			"comp-d",
			"deployment/e2e-service",
			"hashicorp/http-echo",
			[]string{"-text=comp-d", "-listen=:8080"},
			map[string]endpointDef{
				"http": {epType: "HTTP", port: 8080, visibility: []string{"project"}},
			},
		))
		Expect(err).NotTo(HaveOccurred(), "failed to create comp-d: %s", output)

		// client-d: busybox client in beta/proj1.
		output, err = framework.KubectlApplyLiteral(kubeContext, componentYAML(
			cpNsBeta,
			"proj1",
			"client-d",
			"deployment/e2e-worker",
			"busybox:1.36",
			[]string{"sleep", "3600"},
			nil,
		))
		Expect(err).NotTo(HaveOccurred(), "failed to create client-d: %s", output)

		By("creating non-OC namespaces and pods")
		output, err = framework.KubectlApplyLiteral(kubeContext, nonOCNamespacesYAML())
		Expect(err).NotTo(HaveOccurred(), "failed to create non-OC namespaces: %s", output)
		output, err = framework.KubectlApplyLiteral(kubeContext, nonOCPodsYAML())
		Expect(err).NotTo(HaveOccurred(), "failed to create non-OC pods: %s", output)

		By("waiting for development data plane namespaces to appear")
		Eventually(func() error {
			var discoverErr error
			dpAcmeProj1Dev, discoverErr = framework.GetDPNamespace(kubeContext, cpNsAcme, "proj1", "development")
			return discoverErr
		}, 3*time.Minute, 5*time.Second).Should(Succeed(), "dp namespace for acme/proj1/development not found")
		fmt.Fprintf(GinkgoWriter, "discovered dp namespace: acme/proj1/dev = %s\n", dpAcmeProj1Dev)

		Eventually(func() error {
			var discoverErr error
			dpAcmeProj2Dev, discoverErr = framework.GetDPNamespace(kubeContext, cpNsAcme, "proj2", "development")
			return discoverErr
		}, 3*time.Minute, 5*time.Second).Should(Succeed(), "dp namespace for acme/proj2/development not found")
		fmt.Fprintf(GinkgoWriter, "discovered dp namespace: acme/proj2/dev = %s\n", dpAcmeProj2Dev)

		Eventually(func() error {
			var discoverErr error
			dpBetaProj1Dev, discoverErr = framework.GetDPNamespace(kubeContext, cpNsBeta, "proj1", "development")
			return discoverErr
		}, 3*time.Minute, 5*time.Second).Should(Succeed(), "dp namespace for beta/proj1/development not found")
		fmt.Fprintf(GinkgoWriter, "discovered dp namespace: beta/proj1/dev = %s\n", dpBetaProj1Dev)

		By("promoting comp-a to staging via ReleaseBinding")
		var compARelease string
		Eventually(func() error {
			var discoverErr error
			compARelease, discoverErr = framework.KubectlGetJsonpath(
				kubeContext,
				cpNsAcme,
				"component",
				"comp-a",
				"{.status.latestRelease.name}",
			)
			if discoverErr != nil {
				return discoverErr
			}
			if compARelease == "" {
				return fmt.Errorf("comp-a latestRelease.name not yet populated")
			}
			return nil
		}, 3*time.Minute, 5*time.Second).Should(Succeed(), "comp-a ComponentRelease not created")
		fmt.Fprintf(GinkgoWriter, "comp-a ComponentRelease: %s\n", compARelease)

		output, err = framework.KubectlApplyLiteral(kubeContext, releaseBindingYAML(
			cpNsAcme,
			"proj1",
			"comp-a",
			compARelease,
			"staging",
		))
		Expect(err).NotTo(HaveOccurred(), "failed to create staging ReleaseBinding: %s", output)

		By("waiting for staging data plane namespace to appear")
		Eventually(func() error {
			var discoverErr error
			dpAcmeProj1Stg, discoverErr = framework.GetDPNamespace(kubeContext, cpNsAcme, "proj1", "staging")
			return discoverErr
		}, 3*time.Minute, 5*time.Second).Should(Succeed(), "dp namespace for acme/proj1/staging not found")
		fmt.Fprintf(GinkgoWriter, "discovered dp namespace: acme/proj1/stg = %s\n", dpAcmeProj1Stg)

		By("waiting for expected labels on data plane namespaces")
		assertNamespaceLabels(dpAcmeProj1Dev, cpNsAcme, "proj1", "development")
		assertNamespaceLabels(dpAcmeProj1Stg, cpNsAcme, "proj1", "staging")
		assertNamespaceLabels(dpAcmeProj2Dev, cpNsAcme, "proj2", "development")
		assertNamespaceLabels(dpBetaProj1Dev, cpNsBeta, "proj1", "development")

		By("waiting for all pods to be Running in data plane namespaces")
		for _, ns := range []string{dpAcmeProj1Dev, dpAcmeProj1Stg, dpAcmeProj2Dev, dpBetaProj1Dev} {
			Eventually(func(g Gomega) {
				framework.AssertAllPodsRunning(g, kubeContext, ns)
			}, 3*time.Minute, 2*time.Second).Should(Succeed(),
				"pods in %s not running in time", ns)
		}

		By("waiting for all pods to be Running in non-OC namespaces")
		for _, ns := range []string{nsExtSvc, nsGateway} {
			Eventually(func(g Gomega) {
				framework.AssertAllPodsRunning(g, kubeContext, ns)
			}, 2*time.Minute, 2*time.Second).Should(Succeed(),
				"pods in %s not running in time", ns)
		}

		By("verifying baseline NetworkPolicies exist in data plane namespaces")
		for _, ns := range []string{dpAcmeProj1Dev, dpAcmeProj1Stg, dpAcmeProj2Dev, dpBetaProj1Dev} {
			Eventually(func(g Gomega) {
				framework.AssertResourceExists(g, kubeContext, ns, "networkpolicy", "openchoreo-deny-all-ingress")
				framework.AssertResourceExists(g, kubeContext, ns, "networkpolicy", "openchoreo-egress-isolation")
			}, 2*time.Minute, 2*time.Second).Should(Succeed(),
				"baseline NetworkPolicies not found in %s", ns)
		}

		By("verifying per-component NetworkPolicies exist")
		Eventually(func(g Gomega) {
			framework.AssertResourceExists(g, kubeContext, dpAcmeProj1Dev, "networkpolicy", "openchoreo-comp-a-ingress")
			framework.AssertResourceExists(g, kubeContext, dpAcmeProj1Dev, "networkpolicy", "openchoreo-comp-b-ingress")
		}, 2*time.Minute, 2*time.Second).Should(Succeed(),
			"per-component NetworkPolicies not found in acme/proj1/dev dp namespace")

		Eventually(func(g Gomega) {
			framework.AssertResourceExists(g, kubeContext, dpAcmeProj1Stg, "networkpolicy", "openchoreo-comp-a-ingress")
		}, 2*time.Minute, 2*time.Second).Should(Succeed(),
			"per-component NetworkPolicy not found in acme/proj1/staging dp namespace")

		Eventually(func(g Gomega) {
			framework.AssertResourceExists(g, kubeContext, dpBetaProj1Dev, "networkpolicy", "openchoreo-comp-d-ingress")
		}, 2*time.Minute, 2*time.Second).Should(Succeed(),
			"per-component NetworkPolicy not found in beta/proj1/dev dp namespace")

		By("waiting until policy enforcement is observed on a blocked path")
		assertSourcePodReady(dpAcmeProj2Dev, "openchoreo.dev/component=client-b", "main")
		assertDNSResolution(dpAcmeProj2Dev, "openchoreo.dev/component=client-b", "main", fqdn("comp-b", dpAcmeProj1Dev))
		assertTargetServiceReady(fqdn("comp-b", dpAcmeProj1Dev))
		Eventually(func() error {
			return assertNoConnectivityOnce(
				dpAcmeProj2Dev,
				"openchoreo.dev/component=client-b",
				"main",
				fqdn("comp-b", dpAcmeProj1Dev),
				8080,
			)
		}, 2*time.Minute, 2*time.Second).Should(Succeed(),
			"project-only endpoint policy not yet enforced for cross-project traffic")
	})

	AfterAll(func() {
		if os.Getenv("E2E_KEEP_RESOURCES") == "true" {
			By("skipping cleanup because E2E_KEEP_RESOURCES=true")
			return
		}

		By("cleaning up data plane namespaces")
		for _, ns := range []string{dpAcmeProj1Dev, dpAcmeProj1Stg, dpAcmeProj2Dev, dpBetaProj1Dev} {
			if ns != "" {
				_, _ = framework.Kubectl(kubeContext, "delete", "namespace", ns, "--ignore-not-found", "--wait=false")
			}
		}

		By("cleaning up non-OC namespaces")
		for _, ns := range []string{nsExtSvc, nsGateway} {
			_, _ = framework.Kubectl(kubeContext, "delete", "namespace", ns, "--ignore-not-found", "--wait=false")
		}

		By("cleaning up control plane namespaces")
		for _, ns := range []string{cpNsAcme, cpNsBeta} {
			_, _ = framework.Kubectl(kubeContext, "delete", "dataplane", dataPlane, "-n", ns, "--ignore-not-found")
			_, _ = framework.Kubectl(kubeContext, "delete", "namespace", ns, "--ignore-not-found", "--wait=false")
		}
	})

	scenarios := []connectivityScenario{
		{
			name:        "allows intra-namespace traffic to project-visible endpoint",
			intent:      "client-a and comp-b are in the same data plane namespace; project visibility should allow this path.",
			sourceNS:    func() string { return dpAcmeProj1Dev },
			sourceLabel: "openchoreo.dev/component=client-a",
			sourceCtr:   "main",
			targetHost:  func() string { return fqdn("comp-b", dpAcmeProj1Dev) },
			targetPort:  8080,
			expectAllow: true,
		},
		{
			name:        "allows cross-project traffic to namespace-visible endpoint",
			intent:      "comp-a is namespace-visible within the same control plane namespace, so client-b in proj2 should reach it.",
			sourceNS:    func() string { return dpAcmeProj2Dev },
			sourceLabel: "openchoreo.dev/component=client-b",
			sourceCtr:   "main",
			targetHost:  func() string { return fqdn("comp-a", dpAcmeProj1Dev) },
			targetPort:  8080,
			expectAllow: true,
		},
		{
			name:        "blocks cross-project traffic to project-only endpoint",
			intent:      "comp-b is project-visible only, so client-b from another project should be blocked.",
			sourceNS:    func() string { return dpAcmeProj2Dev },
			sourceLabel: "openchoreo.dev/component=client-b",
			sourceCtr:   "main",
			targetHost:  func() string { return fqdn("comp-b", dpAcmeProj1Dev) },
			targetPort:  8080,
			expectAllow: false,
		},
		{
			name:        "blocks cross-control-plane traffic from beta to acme",
			intent:      "traffic from beta/proj1 to acme/proj1 should be denied across control plane boundaries.",
			sourceNS:    func() string { return dpBetaProj1Dev },
			sourceLabel: "openchoreo.dev/component=client-d",
			sourceCtr:   "main",
			targetHost:  func() string { return fqdn("comp-a", dpAcmeProj1Dev) },
			targetPort:  8080,
			expectAllow: false,
		},
		{
			name:        "blocks cross-control-plane traffic from acme to beta",
			intent:      "traffic from acme/proj1 to beta/proj1 should be denied across control plane boundaries.",
			sourceNS:    func() string { return dpAcmeProj1Dev },
			sourceLabel: "openchoreo.dev/component=client-a",
			sourceCtr:   "main",
			targetHost:  func() string { return fqdn("comp-d", dpBetaProj1Dev) },
			targetPort:  8080,
			expectAllow: false,
		},
		{
			name:        "blocks cross-environment traffic within same control-plane namespace",
			intent:      "even with namespace visibility, development should not reach staging within the same control plane namespace.",
			sourceNS:    func() string { return dpAcmeProj1Dev },
			sourceLabel: "openchoreo.dev/component=client-a",
			sourceCtr:   "main",
			targetHost:  func() string { return fqdn("comp-a", dpAcmeProj1Stg) },
			targetPort:  8080,
			expectAllow: false,
		},
		{
			name:        "allows gateway traffic to external-visible endpoint",
			intent:      "gateway-proxy should reach comp-a because comp-a declares external visibility.",
			sourceNS:    func() string { return nsGateway },
			sourceLabel: "app=gateway-proxy",
			sourceCtr:   "",
			targetHost:  func() string { return fqdn("comp-a", dpAcmeProj1Dev) },
			targetPort:  8080,
			expectAllow: true,
		},
		{
			name:        "blocks gateway traffic to project-only endpoint",
			intent:      "gateway-proxy should not reach comp-b because comp-b is not externally visible.",
			sourceNS:    func() string { return nsGateway },
			sourceLabel: "app=gateway-proxy",
			sourceCtr:   "",
			targetHost:  func() string { return fqdn("comp-b", dpAcmeProj1Dev) },
			targetPort:  8080,
			expectAllow: false,
		},
		{
			name:        "allows egress to non-openchoreo namespace",
			intent:      "client-a should reach ext-service in a namespace outside OpenChoreo control plane labeling.",
			sourceNS:    func() string { return dpAcmeProj1Dev },
			sourceLabel: "openchoreo.dev/component=client-a",
			sourceCtr:   "main",
			targetHost:  func() string { return fqdn("ext-service", nsExtSvc) },
			targetPort:  8080,
			expectAllow: true,
		},
	}

	for _, scenario := range scenarios {
		It(scenario.name, func() {
			By(scenario.intent)
			if scenario.expectAllow {
				assertConnectivity(
					scenario.sourceNS(),
					scenario.sourceLabel,
					scenario.sourceCtr,
					scenario.targetHost(),
					scenario.targetPort,
				)
				return
			}

			assertNoConnectivity(
				scenario.sourceNS(),
				scenario.sourceLabel,
				scenario.sourceCtr,
				scenario.targetHost(),
				scenario.targetPort,
			)
		})
	}

	It("allows DNS resolution", func() {
		By("pods should resolve cluster DNS names while egress policies are in place")
		Eventually(func() error {
			_, err := framework.KubectlExecByLabel(
				kubeContext,
				dpAcmeProj1Dev,
				"openchoreo.dev/component=client-a",
				"main",
				"nslookup",
				"kubernetes.default.svc.cluster.local",
			)
			return err
		}, 30*time.Second, 2*time.Second).Should(Succeed(), "DNS resolution failed")
	})
})
