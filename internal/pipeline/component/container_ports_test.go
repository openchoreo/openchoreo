// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package component

import (
	"fmt"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/validation"
	"sigs.k8s.io/yaml"

	"github.com/openchoreo/openchoreo/api/v1alpha1"
)

// TestRender_ContainerPortsMatchServicePorts renders a Deployment and Service the way
// the sample ComponentTypes do and checks the result is accepted by Kubernetes:
// container ports are unique by (containerPort, protocol) with valid names, and every
// Service targetPort stays numeric and resolves to a declared container port.
func TestRender_ContainerPortsMatchServicePorts(t *testing.T) {
	componentTypeYAML := `
spec:
  resources:
    - id: deployment
      template:
        apiVersion: apps/v1
        kind: Deployment
        metadata: {name: web}
        spec:
          template:
            spec:
              containers:
                - name: main
                  image: nginx
                  ports: ${workload.toContainerPorts()}
    - id: service
      template:
        apiVersion: v1
        kind: Service
        metadata: {name: web}
        spec:
          ports: ${workload.toServicePorts()}
`
	workloadYAML := `
spec:
  endpoints:
    api:     {type: HTTP, port: 8080}
    admin:   {type: HTTP, port: 8080}
    web:     {type: HTTP, port: 80, targetPort: 3000}
    dns-tcp: {type: TCP, port: 53}
    dns-udp: {type: UDP, port: 53}
    "8443":  {type: HTTP, port: 8443}
`

	var componentType v1alpha1.ComponentType
	if err := yaml.Unmarshal([]byte(componentTypeYAML), &componentType); err != nil {
		t.Fatalf("failed to parse componentType: %v", err)
	}
	var workload v1alpha1.Workload
	if err := yaml.Unmarshal([]byte(workloadYAML), &workload); err != nil {
		t.Fatalf("failed to parse workload: %v", err)
	}

	output, err := NewPipeline().Render(t.Context(), &RenderInput{
		ComponentType: &componentType,
		Component:     &v1alpha1.Component{},
		Workload:      &workload,
		Environment:   &v1alpha1.Environment{},
		DataPlane:     &v1alpha1.DataPlane{},
		Metadata:      postRenderTestMetadata(),
	})
	if err != nil {
		t.Fatalf("render failed: %v", err)
	}

	var deployment appsv1.Deployment
	var service corev1.Service
	for _, rr := range output.Resources {
		var target any
		switch rr.Resource["kind"] {
		case kindDeployment:
			target = &deployment
		case "Service":
			target = &service
		default:
			continue
		}
		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(rr.Resource, target); err != nil {
			t.Fatalf("failed to convert %v: %v", rr.Resource["kind"], err)
		}
	}

	containers := deployment.Spec.Template.Spec.Containers
	if len(containers) != 1 {
		t.Fatalf("expected 1 container, got %d", len(containers))
	}

	declared := make(map[string]bool)
	names := make(map[string]bool)
	for _, p := range containers[0].Ports {
		if errs := validation.IsValidPortName(p.Name); len(errs) != 0 {
			t.Errorf("container port name %q is invalid: %v", p.Name, errs)
		}
		if names[p.Name] {
			t.Errorf("duplicate container port name %q", p.Name)
		}
		names[p.Name] = true

		key := fmt.Sprintf("%d/%s", p.ContainerPort, p.Protocol)
		if declared[key] {
			t.Errorf("duplicate container port %s would be rejected by server-side apply", key)
		}
		declared[key] = true
	}
	if want := 5; len(containers[0].Ports) != want {
		t.Errorf("got %d container ports, want %d: %+v", len(containers[0].Ports), want, containers[0].Ports)
	}

	if len(service.Spec.Ports) == 0 {
		t.Fatal("expected service ports")
	}
	for _, sp := range service.Spec.Ports {
		if sp.TargetPort.Type != intstr.Int {
			t.Errorf("service port %q targetPort must stay numeric, got %q", sp.Name, sp.TargetPort.String())
			continue
		}
		key := fmt.Sprintf("%d/%s", sp.TargetPort.IntVal, sp.Protocol)
		if !declared[key] {
			t.Errorf("service port %q targets %s, which no container port declares", sp.Name, key)
		}
	}
}
