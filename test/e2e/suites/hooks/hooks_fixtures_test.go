// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package e2e

import (
	"fmt"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/yaml"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

const (
	clusterDataPlane   = "e2e-shared"
	openChoreoAPIVer   = "openchoreo.dev/v1alpha1"
	kubernetesAPIVerV1 = "v1"

	projectName = "hk-proj"
	envDev      = "development"
	envStaging  = "staging"

	// A service is gated by the image-check binding; a worker by the approval
	// binding. Both are deployed from public images so the suite needs no build.
	componentGated    = "gated-svc"
	componentApproved = "approved-wrk"
	imageService      = "ghcr.io/openchoreo/samples/greeter-service@sha256:5c67732c99ac3505dbab14c7ec92c33be57904420d62812694c64b56c5f92d40"
	imageWorker       = "hashicorp/http-echo:1.0.0"
	servicePort       = 9090

	bindingImageCheck = "image-check"
	bindingApproval   = "approval"
)

var hkRunID = fmt.Sprintf("%d", time.Now().UnixNano())

var (
	cpNs = fmt.Sprintf("e2e-hk-%s", hkRunID)
	// Cluster-scoped fixtures carry the run id so parallel runs never collide.
	clusterWorkflowCheck    = "e2e-hk-check-" + hkRunID
	clusterWorkflowApproval = "e2e-hk-approval-" + hkRunID
	clusterHookCheck        = "e2e-hk-check-" + hkRunID
	clusterHookApproval     = "e2e-hk-approval-" + hkRunID
)

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
			Name:   cpNs,
			Labels: map[string]string{"openchoreo.dev/control-plane": "true"},
		},
	}
	return mustYAMLDocs(ns)
}

// hookWorkflowYAML renders a ClusterWorkflow whose single step runs `script`
// in an alpine container. The workflow takes one input, `image`, so the hook's
// parameter mapping is exercised end to end (the value is echoed by the step).
// When suspend is true an Argo suspend step precedes the script, which is how
// the approval scenario parks the hook in Running/Suspended until resumed.
func hookWorkflowYAML(name, script string, suspend bool) string {
	steps := ""
	if suspend {
		steps = `
        - - name: approve
            template: approve`
	}
	tmpl := `
apiVersion: argoproj.io/v1alpha1
kind: Workflow
metadata:
  name: ${metadata.workflowRunName}
  namespace: ${metadata.namespace}
spec:
  serviceAccountName: workflow-sa
  entrypoint: main
  arguments:
    parameters:
      - name: image
        value: ${parameters.image}
  templates:
    - name: main
      steps:` + steps + `
        - - name: check
            template: check
    - name: approve
      suspend: {}
    - name: check
      container:
        image: alpine:3.20
        command: [sh, -c]
        args:
          - |-
            echo ">> hook image: {{workflow.parameters.image}}"
            ` + script + `
`
	var runTemplate map[string]any
	if err := yaml.Unmarshal([]byte(tmpl), &runTemplate); err != nil {
		panic(fmt.Sprintf("bad run template: %v", err))
	}
	raw, err := yaml.Marshal(runTemplate)
	if err != nil {
		panic(err)
	}
	rawJSON, err := yaml.YAMLToJSON(raw)
	if err != nil {
		panic(err)
	}
	schema := []byte(`{"type":"object","required":["image"],"properties":{"image":{"type":"string"}}}`)
	wf := &openchoreov1alpha1.ClusterWorkflow{
		TypeMeta:   metav1.TypeMeta{APIVersion: openChoreoAPIVer, Kind: "ClusterWorkflow"},
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: openchoreov1alpha1.ClusterWorkflowSpec{
			WorkflowPlaneRef: &openchoreov1alpha1.ClusterWorkflowPlaneRef{
				Kind: openchoreov1alpha1.ClusterWorkflowPlaneRefKindClusterWorkflowPlane,
				Name: "default",
			},
			TTLAfterCompletion: "1h",
			Parameters: &openchoreov1alpha1.SchemaSection{
				OpenAPIV3Schema: &runtime.RawExtension{Raw: schema},
			},
			RunTemplate: &runtime.RawExtension{Raw: rawJSON},
		},
	}
	return mustYAMLDocs(wf)
}

func clusterHookYAML(name, workflow string) string {
	hook := &openchoreov1alpha1.ClusterHook{
		TypeMeta:   metav1.TypeMeta{APIVersion: openChoreoAPIVer, Kind: "ClusterHook"},
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: openchoreov1alpha1.HookSpec{
			Type: openchoreov1alpha1.HookTypeWorkflow,
			WorkflowRef: &openchoreov1alpha1.WorkflowRef{
				Kind: openchoreov1alpha1.WorkflowRefKindClusterWorkflow,
				Name: workflow,
			},
			Parameters: []openchoreov1alpha1.HookParameter{
				{Name: "image", From: "${deployment.workload.containers.main.image}"},
			},
		},
	}
	return mustYAMLDocs(hook)
}

// platformResourcesYAML: development is the root, staging the gated
// environment. Both bindings live on the staging Environment and are Sync
// pre-deploy with Block so a failure is visible as PreDeployHooksPassed=False
// and no RenderedRelease.
func platformResourcesYAML() string {
	stagingHooks := &openchoreov1alpha1.HookSet{
		PreDeploy: []openchoreov1alpha1.HookBinding{
			{
				Name:      bindingImageCheck,
				HookRef:   openchoreov1alpha1.HookRef{Kind: openchoreov1alpha1.HookRefKindClusterHook, Name: clusterHookCheck},
				Mode:      openchoreov1alpha1.HookModeSync,
				OnFailure: openchoreov1alpha1.HookFailurePolicyBlock,
				Timeout:   "10m",
				AppliesTo: []openchoreov1alpha1.HookSubjectSelector{
					{Kind: openchoreov1alpha1.HookSubjectSelectorKindClusterComponentType, Name: "service"},
				},
			},
			{
				Name:      bindingApproval,
				HookRef:   openchoreov1alpha1.HookRef{Kind: openchoreov1alpha1.HookRefKindClusterHook, Name: clusterHookApproval},
				Mode:      openchoreov1alpha1.HookModeSync,
				OnFailure: openchoreov1alpha1.HookFailurePolicyBlock,
				Timeout:   "10m",
				AppliesTo: []openchoreov1alpha1.HookSubjectSelector{
					{Kind: openchoreov1alpha1.HookSubjectSelectorKindClusterComponentType, Name: "worker"},
				},
			},
		},
	}
	pipeline := &openchoreov1alpha1.DeploymentPipeline{
		TypeMeta: metav1.TypeMeta{APIVersion: openChoreoAPIVer, Kind: "DeploymentPipeline"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "default",
			Namespace: cpNs,
			Labels:    map[string]string{"openchoreo.dev/name": "default"},
		},
		Spec: openchoreov1alpha1.DeploymentPipelineSpec{
			PromotionPaths: []openchoreov1alpha1.PromotionPath{{
				SourceEnvironmentRef:  openchoreov1alpha1.EnvironmentRef{Name: envDev},
				TargetEnvironmentRefs: []openchoreov1alpha1.TargetEnvironmentRef{{Name: envStaging}},
			}},
		},
	}
	docs := []any{pipeline}
	for _, name := range []string{envDev, envStaging} {
		env := &openchoreov1alpha1.Environment{
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
		}
		if name == envStaging {
			env.Spec.Hooks = stagingHooks
		}
		docs = append(docs, env)
	}
	docs = append(docs, &openchoreov1alpha1.Project{
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
	})
	// One ProjectReleaseBinding per environment so both cells exist before the
	// staging ReleaseBinding is created.
	for _, env := range []string{envDev, envStaging} {
		docs = append(docs, &openchoreov1alpha1.ProjectReleaseBinding{
			TypeMeta: metav1.TypeMeta{APIVersion: openChoreoAPIVer, Kind: "ProjectReleaseBinding"},
			ObjectMeta: metav1.ObjectMeta{
				Name:      projectName + "-" + env,
				Namespace: cpNs,
				Labels: map[string]string{
					"openchoreo.dev/project":     projectName,
					"openchoreo.dev/environment": env,
				},
			},
			Spec: openchoreov1alpha1.ProjectReleaseBindingSpec{
				Owner:       openchoreov1alpha1.ProjectReleaseBindingOwner{ProjectName: projectName},
				Environment: env,
			},
		})
	}
	return mustYAMLDocs(docs...)
}

// componentWithImageYAML returns a Component + Workload for a deployment-style
// ClusterComponentType. AutoDeploy creates the development ReleaseBinding and
// the ComponentRelease the staging binding is then pinned to.
func componentWithImageYAML(name, componentType, image string, port int) string {
	comp := &openchoreov1alpha1.Component{
		TypeMeta: metav1.TypeMeta{APIVersion: openChoreoAPIVer, Kind: "Component"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: cpNs,
			Labels:    map[string]string{"openchoreo.dev/name": name},
		},
		Spec: openchoreov1alpha1.ComponentSpec{
			Owner: openchoreov1alpha1.ComponentOwner{ProjectName: projectName},
			ComponentType: openchoreov1alpha1.ComponentTypeRef{
				Kind: openchoreov1alpha1.ComponentTypeRefKindClusterComponentType,
				Name: componentType,
			},
			AutoDeploy: true,
		},
	}
	endpoints := map[string]openchoreov1alpha1.WorkloadEndpoint{}
	if port > 0 {
		endpoints["http"] = openchoreov1alpha1.WorkloadEndpoint{
			Type:       openchoreov1alpha1.EndpointType("HTTP"),
			Port:       int32(port),
			Visibility: []openchoreov1alpha1.EndpointVisibility{"project"},
		}
	}
	workload := &openchoreov1alpha1.Workload{
		TypeMeta: metav1.TypeMeta{APIVersion: openChoreoAPIVer, Kind: "Workload"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: cpNs,
			Labels:    map[string]string{"openchoreo.dev/name": name},
		},
		Spec: openchoreov1alpha1.WorkloadSpec{
			Owner: openchoreov1alpha1.WorkloadOwner{ProjectName: projectName, ComponentName: name},
			WorkloadTemplateSpec: openchoreov1alpha1.WorkloadTemplateSpec{
				Endpoints: endpoints,
				Container: openchoreov1alpha1.Container{Image: image},
			},
		},
	}
	return mustYAMLDocs(comp, workload)
}

// stagingReleaseBindingYAML pins the given ComponentRelease into staging — the
// promotion the gate protects.
func stagingReleaseBindingYAML(component, releaseName string) string {
	rb := &openchoreov1alpha1.ReleaseBinding{
		TypeMeta: metav1.TypeMeta{APIVersion: openChoreoAPIVer, Kind: "ReleaseBinding"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      component + "-" + envStaging,
			Namespace: cpNs,
			Labels: map[string]string{
				"openchoreo.dev/project":     projectName,
				"openchoreo.dev/component":   component,
				"openchoreo.dev/environment": envStaging,
			},
		},
		Spec: openchoreov1alpha1.ReleaseBindingSpec{
			Owner:       openchoreov1alpha1.ReleaseBindingOwner{ProjectName: projectName, ComponentName: component},
			Environment: envStaging,
			ReleaseName: releaseName,
		},
	}
	return mustYAMLDocs(rb)
}
