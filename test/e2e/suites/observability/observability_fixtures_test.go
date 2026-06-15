// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package e2e

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2" //nolint:revive
	. "github.com/onsi/gomega"    //nolint:revive
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/yaml"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/test/e2e/framework"
)

const (
	clusterDataPlane   = "e2e-shared"
	openChoreoAPIVer   = "openchoreo.dev/v1alpha1"
	kubernetesAPIVerV1 = "v1"

	projectName = "obs-proj"
	envDev      = "development"
	envStaging  = "staging"

	componentGreeter     = "obs-greeter"
	releaseBindingSuffix = "-" + envDev

	servicePort = 9090

	// imageGreeter is the public OpenChoreo sample image with a
	// `/greeter/greet` endpoint and stdout logging.
	imageGreeter = "ghcr.io/openchoreo/samples/greeter-service@sha256:5c67732c99ac3505dbab14c7ec92c33be57904420d62812694c64b56c5f92d40"

	// curlImage is the in-cluster pod the framework execs queries through.
	// curl is available in the curlimages/curl image and the image is tiny
	// (~5MB) — a good fit for the suite's load-generation + observer-query
	// needs. Pinned to avoid a "latest" surprise.
	curlImage     = "curlimages/curl:8.10.1"
	curlPodLabel  = "app=obs-tester"
	curlContainer = "tester"
)

var obsRunID = fmt.Sprintf("%d", time.Now().UnixNano())

var cpNs = fmt.Sprintf("e2e-obs-%s", obsRunID)

func mustYAMLDocs(objects ...any) string {
	docs := make([]string, 0, len(objects))
	for _, obj := range objects {
		data, err := yaml.Marshal(obj)
		if err != nil {
			panic(fmt.Sprintf("failed to marshal yaml document: %v", err))
		}
		docs = append(docs, strings.TrimSpace(string(data)))
	}
	return strings.Join(docs, "\n---\n")
}

func cpNamespaceYAML() string {
	ns := &corev1.Namespace{
		TypeMeta: metav1.TypeMeta{APIVersion: kubernetesAPIVerV1, Kind: "Namespace"},
		ObjectMeta: metav1.ObjectMeta{
			Name: cpNs,
			Labels: map[string]string{
				"openchoreo.dev/control-plane": "true",
			},
		},
	}
	return mustYAMLDocs(ns)
}

func platformResourcesYAML() string {
	pipeline := &openchoreov1alpha1.DeploymentPipeline{
		TypeMeta: metav1.TypeMeta{APIVersion: openChoreoAPIVer, Kind: "DeploymentPipeline"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "default",
			Namespace: cpNs,
			Labels:    map[string]string{"openchoreo.dev/name": "default"},
		},
		Spec: openchoreov1alpha1.DeploymentPipelineSpec{
			PromotionPaths: []openchoreov1alpha1.PromotionPath{{
				SourceEnvironmentRef: openchoreov1alpha1.EnvironmentRef{Name: envDev},
				TargetEnvironmentRefs: []openchoreov1alpha1.TargetEnvironmentRef{
					{Name: envStaging},
				},
			}},
		},
	}
	envs := make([]any, 0, 2)
	for _, name := range []string{envDev, envStaging} {
		envs = append(envs, &openchoreov1alpha1.Environment{
			TypeMeta: metav1.TypeMeta{APIVersion: openChoreoAPIVer, Kind: "Environment"},
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: cpNs,
				Labels:    map[string]string{"openchoreo.dev/name": name},
			},
			Spec: openchoreov1alpha1.EnvironmentSpec{
				DataPlaneRef: &openchoreov1alpha1.DataPlaneRef{
					Kind: openchoreov1alpha1.DataPlaneRefKindClusterDataPlane,
					Name: clusterDataPlane,
				},
			},
		})
	}
	proj := &openchoreov1alpha1.Project{
		TypeMeta: metav1.TypeMeta{APIVersion: openChoreoAPIVer, Kind: "Project"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      projectName,
			Namespace: cpNs,
			Labels:    map[string]string{"openchoreo.dev/name": projectName},
		},
		Spec: openchoreov1alpha1.ProjectSpec{
			DeploymentPipelineRef: openchoreov1alpha1.DeploymentPipelineRef{Name: "default"},
		},
	}
	docs := []any{pipeline}
	docs = append(docs, envs...)
	docs = append(docs, proj)
	return mustYAMLDocs(docs...)
}

// greeterComponentYAML returns a service-flavour Component + Workload that
// exposes the greeter sample on `servicePort`. Logs land on stdout, which
// the cluster's logs-adapter ships into OpenSearch under the rendered DP
// namespace + the component's pod labels — which is exactly what the
// observer's componentSearchScope query needs to find them.
func greeterComponentYAML() string {
	comp := &openchoreov1alpha1.Component{
		TypeMeta: metav1.TypeMeta{APIVersion: openChoreoAPIVer, Kind: "Component"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      componentGreeter,
			Namespace: cpNs,
			Labels:    map[string]string{"openchoreo.dev/name": componentGreeter},
		},
		Spec: openchoreov1alpha1.ComponentSpec{
			Owner: openchoreov1alpha1.ComponentOwner{ProjectName: projectName},
			ComponentType: openchoreov1alpha1.ComponentTypeRef{
				Kind: openchoreov1alpha1.ComponentTypeRefKindClusterComponentType,
				Name: "deployment/service",
			},
			AutoDeploy: true,
		},
	}
	workload := &openchoreov1alpha1.Workload{
		TypeMeta: metav1.TypeMeta{APIVersion: openChoreoAPIVer, Kind: "Workload"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      componentGreeter,
			Namespace: cpNs,
			Labels:    map[string]string{"openchoreo.dev/name": componentGreeter},
		},
		Spec: openchoreov1alpha1.WorkloadSpec{
			Owner: openchoreov1alpha1.WorkloadOwner{
				ProjectName:   projectName,
				ComponentName: componentGreeter,
			},
			WorkloadTemplateSpec: openchoreov1alpha1.WorkloadTemplateSpec{
				Endpoints: map[string]openchoreov1alpha1.WorkloadEndpoint{
					"http": {
						Type:       openchoreov1alpha1.EndpointType("HTTP"),
						Port:       int32(servicePort),
						Visibility: []openchoreov1alpha1.EndpointVisibility{"project"},
					},
				},
				Container: openchoreov1alpha1.Container{
					Image: imageGreeter,
					Args:  []string{"--port", strconv.Itoa(servicePort)},
				},
			},
		},
	}
	return mustYAMLDocs(comp, workload)
}

// curlPodYAML returns a curl-enabled tester pod the framework execs through
// to (a) generate HTTP traffic against the greeter and (b) call the observer
// query API. Same pattern as wt-tester from the workloadtypes suite, but
// using curlimages/curl so we get a TLS-capable curl (the observer is plain
// HTTP, but using one image keeps the suites consistent).
func curlPodYAML(dpNamespace string) string {
	pod := &corev1.Pod{
		TypeMeta: metav1.TypeMeta{APIVersion: kubernetesAPIVerV1, Kind: "Pod"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "obs-tester",
			Namespace: dpNamespace,
			Labels: map[string]string{
				"app":                       "obs-tester",
				"openchoreo.dev/project":    projectName,
				"openchoreo.dev/managed-by": "e2e-observability",
			},
		},
		Spec: corev1.PodSpec{Containers: []corev1.Container{{
			Name:    curlContainer,
			Image:   curlImage,
			Command: []string{"sleep", "3600"},
		}}},
	}
	return mustYAMLDocs(pod)
}

// obsFixturesReady guards ensureObservabilityFixtures so the shared greeter +
// tester pod plumbing is provisioned at most once even though two top-level
// Ordered Describes (Observability Signals and Observer MCP) both call it from
// their BeforeAll, and Ginkgo v2 randomizes top-level container ordering. The
// underlying applies are idempotent kubectl applies and the waits are
// Eventually-based, so this is an optimization; correctness comes from
// idempotence. It is set only AFTER successful provisioning: a transient failure
// panics out via a failed assertion and leaves it false, so the other Describe's
// BeforeAll retries the setup. (sync.Once would latch even on panic, dooming the
// second Describe.)
var obsFixturesReady bool

// ensureObservabilityFixtures provisions the shared observability fixtures
// (CP namespace, platform resources, greeter component, DP tester pod) and
// resolves the package-level dpNs / observerQ / greeterHost / greeterPort used
// by both the REST and MCP observability Describes. Safe to call from multiple
// BeforeAll blocks.
func ensureObservabilityFixtures() {
	if obsFixturesReady {
		return
	}

	By("creating control plane namespace")
	out, err := framework.KubectlApplyLiteral(kubeContext, cpNamespaceYAML())
	Expect(err).NotTo(HaveOccurred(), "create cp namespace: %s", out)

	By("applying platform resources (pipeline, environments, project)")
	out, err = framework.KubectlApplyLiteral(kubeContext, platformResourcesYAML())
	Expect(err).NotTo(HaveOccurred(), "apply platform resources: %s", out)

	By("deploying greeter component")
	out, err = framework.KubectlApplyLiteral(kubeContext, greeterComponentYAML())
	Expect(err).NotTo(HaveOccurred(), "create greeter: %s", out)

	By("discovering data plane namespace")
	Eventually(func() error {
		var derr error
		dpNs, derr = framework.GetDPNamespace(dpCtx(), cpNs, projectName, envDev)
		return derr
	}, 3*time.Minute, 5*time.Second).Should(Succeed())
	fmt.Fprintf(GinkgoWriter, "discovered dp namespace: %s\n", dpNs)

	By("deploying tester pod")
	out, err = framework.KubectlApplyLiteral(dpCtx(), curlPodYAML(dpNs))
	Expect(err).NotTo(HaveOccurred(), "create tester pod: %s", out)

	By("waiting for tester pod to be Running")
	Eventually(func(g Gomega) {
		framework.AssertPodsRunning(g, dpCtx(), dpNs, curlPodLabel)
	}, 4*time.Minute, 3*time.Second).Should(Succeed())

	By("waiting for greeter ReleaseBinding Ready")
	Eventually(func(g Gomega) {
		framework.AssertReleaseBindingReady(g, kubeContext, cpNs,
			componentGreeter+releaseBindingSuffix)
	}, 5*time.Minute, 5*time.Second).Should(Succeed())

	By("waiting for greeter pod Running")
	Eventually(func(g Gomega) {
		framework.AssertPodsRunning(g, dpCtx(), dpNs,
			"openchoreo.dev/component="+componentGreeter)
	}, 3*time.Minute, 3*time.Second).Should(Succeed())

	By("resolving greeter Service host:port")
	Eventually(func(g Gomega) {
		h, p := serviceURLHostPort(g, componentGreeter+releaseBindingSuffix)
		greeterHost, greeterPort = h, p
	}, 3*time.Minute, 3*time.Second).Should(Succeed())
	fmt.Fprintf(GinkgoWriter, "greeter resolved at %s:%s\n", greeterHost, greeterPort)

	// In multi-cluster mode the tester pod sits in the DP cluster and needs
	// to reach Thunder (CP) and the observer (OP) via external hostnames
	// resolved through the DP cluster's CoreDNS rewrites. In single-cluster
	// mode the empty fields fall back to the in-cluster defaults.
	observerQ = framework.ObserverQueryFrom{
		KubeContext:     dpCtx(),
		Namespace:       dpNs,
		PodLabel:        curlPodLabel,
		Container:       curlContainer,
		ThunderTokenURL: mcThunderURL(),
		ObserverURL:     mcObserverURL(),
	}

	obsFixturesReady = true
}
