// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package e2e

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/yaml"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/test/e2e/framework"
)

const (
	clusterDataPlane   = "e2e-shared"
	openChoreoAPIVer   = "openchoreo.dev/v1alpha1"
	kubernetesAPIVerV1 = "v1"

	projectName = "build-proj"
	envDev      = "development"
	envStaging  = "staging"

	giteaNamespace          = framework.Tier3GiteaNamespace
	sampleWorkloadsRepo     = framework.Tier3SampleWorkloadsRepo
	noWorkloadRepo          = framework.Tier3NoWorkloadRepo
	paketoNodeRepo          = framework.Tier3PaketoNodeRepo
	upstreamSampleWorkloads = framework.Tier3UpstreamSampleWorkloads

	// Component / workflow names. Kept short so the rendered Argo Workflow's
	// generated names don't blow past Kubernetes' 63-char DNS label limit.
	componentDockerfile      = "df-svc"
	componentDockerfileReact = "df-spa"
	componentGCP             = "gcp-svc"
	componentPaketo          = "pkt-svc"
	componentBallerina       = "bal-svc"
	componentNoWorkload      = "auto-wl"
	componentExternalRefs    = "extref-svc"
	componentLogs            = "logs-svc"

	releaseBindingSuffix = "-" + envDev

	testerLabel     = "app=build-tester"
	testerContainer = "tester"
)

var buildRunID = fmt.Sprintf("%d", time.Now().UnixNano())

var cpNs = fmt.Sprintf("e2e-build-%s", buildRunID)

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
			Type:                  openchoreov1alpha1.ProjectTypeRef{Kind: openchoreov1alpha1.ProjectTypeRefKindClusterProjectType, Name: "default"},
		},
	}
	docs := []any{pipeline}
	docs = append(docs, envs...)
	docs = append(docs, proj)
	return mustYAMLDocs(docs...)
}

// buildComponentYAML returns a Component CR with the given builder workflow
// configured. The Component uses autoDeploy=true so the build pipeline's
// generated Workload triggers a ComponentRelease + ReleaseBinding automatically.
func buildComponentYAML(name, componentType, workflowName, gitURL, appPath, dockerfilePath string) string {
	params := map[string]any{
		"repository": map[string]any{
			"url":     gitURL,
			"appPath": appPath,
			"revision": map[string]any{
				"branch": "main",
			},
		},
	}
	if dockerfilePath != "" {
		params["docker"] = map[string]any{
			"context":  appPath,
			"filePath": dockerfilePath,
		}
	}
	// RawExtension.Raw is JSON, not YAML — using sigs.k8s.io/yaml.Marshal here
	// produces YAML bytes which RawExtension.MarshalJSON later rejects with
	// "cannot convert RawExtension with unrecognized content type to unstructured".
	raw, err := json.Marshal(params)
	if err != nil {
		panic(err)
	}
	comp := &openchoreov1alpha1.Component{
		TypeMeta: metav1.TypeMeta{APIVersion: openChoreoAPIVer, Kind: "Component"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: cpNs,
			Labels: map[string]string{
				"openchoreo.dev/name":      name,
				"openchoreo.dev/project":   projectName,
				"openchoreo.dev/component": name,
			},
		},
		Spec: openchoreov1alpha1.ComponentSpec{
			Owner: openchoreov1alpha1.ComponentOwner{ProjectName: projectName},
			ComponentType: openchoreov1alpha1.ComponentTypeRef{
				Kind: openchoreov1alpha1.ComponentTypeRefKindClusterComponentType,
				Name: componentType,
			},
			AutoDeploy: true,
			Workflow: &openchoreov1alpha1.ComponentWorkflowConfig{
				Kind:       openchoreov1alpha1.WorkflowRefKindClusterWorkflow,
				Name:       workflowName,
				Parameters: &runtime.RawExtension{Raw: raw},
			},
		},
	}
	return mustYAMLDocs(comp)
}

// workflowRunYAML pairs with buildComponentYAML to actually kick off the build.
// The Component carries the workflow config for `autoDeploy`/`autoBuild`
// bookkeeping; the WorkflowRun is the active trigger.
func workflowRunYAML(componentName, runName, workflowName, gitURL, appPath, dockerfilePath string) string {
	params := map[string]any{
		"repository": map[string]any{
			"url":     gitURL,
			"appPath": appPath,
			"revision": map[string]any{
				"branch": "main",
			},
		},
	}
	if dockerfilePath != "" {
		params["docker"] = map[string]any{
			"context":  appPath,
			"filePath": dockerfilePath,
		}
	}
	// RawExtension.Raw is JSON, not YAML — using sigs.k8s.io/yaml.Marshal here
	// produces YAML bytes which RawExtension.MarshalJSON later rejects with
	// "cannot convert RawExtension with unrecognized content type to unstructured".
	raw, err := json.Marshal(params)
	if err != nil {
		panic(err)
	}
	wfr := &openchoreov1alpha1.WorkflowRun{
		TypeMeta: metav1.TypeMeta{APIVersion: openChoreoAPIVer, Kind: "WorkflowRun"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      runName,
			Namespace: cpNs,
			Labels: map[string]string{
				"openchoreo.dev/project":   projectName,
				"openchoreo.dev/component": componentName,
			},
		},
		Spec: openchoreov1alpha1.WorkflowRunSpec{
			Workflow: openchoreov1alpha1.WorkflowRunConfig{
				Kind:       openchoreov1alpha1.WorkflowRefKindClusterWorkflow,
				Name:       workflowName,
				Parameters: &runtime.RawExtension{Raw: raw},
			},
		},
	}
	return mustYAMLDocs(wfr)
}

// testerPodYAML returns a busybox pod used as the source of in-cluster
// reachability probes against rendered Services.
func testerPodYAML(dpNamespace string) string {
	pod := &corev1.Pod{
		TypeMeta: metav1.TypeMeta{APIVersion: kubernetesAPIVerV1, Kind: "Pod"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "build-tester",
			Namespace: dpNamespace,
			Labels: map[string]string{
				"app":                       "build-tester",
				"openchoreo.dev/project":    projectName,
				"openchoreo.dev/managed-by": "e2e-build",
			},
		},
		Spec: corev1.PodSpec{Containers: []corev1.Container{{
			Name:    testerContainer,
			Image:   "busybox:1.36",
			Command: []string{"sleep", "3600"},
		}}},
	}
	return mustYAMLDocs(pod)
}

// externalRefsFixtureYAML returns a SecretReference + Workflow + WorkflowRun
// triple used by the externalrefs-in-cel spec. The Workflow's runTemplate
// emits a trivial Argo Workflow whose top-level metadata carries a label
// populated from `${externalRefs['app-config'].spec.refreshInterval}`. We then
// kubectl-get that rendered Argo Workflow and assert the label matches the
// SecretReference's refreshInterval — proving the externalRef spec landed in
// the CEL context.
//
// We deliberately bind to a benign field (refreshInterval) rather than
// fabricating a custom value because SecretReference's schema is fixed.
func externalRefsFixtureYAML() string {
	secretRefName := componentExternalRefs + "-ref"
	wfName := componentExternalRefs + "-wf"
	runName := componentExternalRefs + "-run-01"

	secretRef := map[string]any{
		"apiVersion": openChoreoAPIVer,
		"kind":       "SecretReference",
		"metadata": map[string]any{
			"name":      secretRefName,
			"namespace": cpNs,
			"labels":    map[string]string{"openchoreo.dev/managed-by": "e2e-build"},
		},
		"spec": map[string]any{
			"template":        map[string]any{"type": "Opaque"},
			"refreshInterval": "7m42s",
			"data": []any{map[string]any{
				"secretKey": "marker",
				"remoteRef": map[string]any{"key": "secret/data/e2e/marker"},
			}},
		},
	}

	workflow := map[string]any{
		"apiVersion": openChoreoAPIVer,
		"kind":       "Workflow",
		"metadata": map[string]any{
			"name":      wfName,
			"namespace": cpNs,
		},
		"spec": map[string]any{
			"workflowPlaneRef":   map[string]any{"kind": "ClusterWorkflowPlane", "name": "default"},
			"ttlAfterCompletion": "1h",
			"externalRefs": []any{map[string]any{
				"id":         "app-config",
				"apiVersion": openChoreoAPIVer,
				"kind":       "SecretReference",
				"name":       secretRefName,
			}},
			"runTemplate": map[string]any{
				"apiVersion": "argoproj.io/v1alpha1",
				"kind":       "Workflow",
				"metadata": map[string]any{
					"name":      "${metadata.workflowRunName}",
					"namespace": "${metadata.namespace}",
					"labels": map[string]any{
						"openchoreo.dev/e2e-cel-probe": "${externalRefs['app-config'].spec.refreshInterval}",
					},
				},
				"spec": map[string]any{
					"entrypoint": "echo",
					"templates": []any{map[string]any{
						"name": "echo",
						"container": map[string]any{
							"image": "busybox:1.36",
							"command": []any{
								"sh", "-c", "echo cel-probe complete",
							},
						},
					}},
				},
			},
		},
	}

	wfRun := map[string]any{
		"apiVersion": openChoreoAPIVer,
		"kind":       "WorkflowRun",
		"metadata": map[string]any{
			"name":      runName,
			"namespace": cpNs,
			"labels": map[string]string{
				"openchoreo.dev/managed-by": "e2e-build",
			},
		},
		"spec": map[string]any{
			"workflow": map[string]any{
				"kind": "Workflow",
				"name": wfName,
			},
		},
	}

	return mustYAMLDocs(secretRef, workflow, wfRun)
}
