// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package e2e

import (
	"fmt"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
)

// platformResourcesYAML returns YAML for a DeploymentPipeline and Environment
// that enable the release chain (ComponentRelease → ReleaseBinding) for a test namespace.
func platformResourcesYAML(namespace string) string {
	return fmt.Sprintf(`apiVersion: openchoreo.dev/v1alpha1
kind: DeploymentPipeline
metadata:
  name: default
  namespace: %[1]s
spec:
  promotionPaths:
    - sourceEnvironmentRef:
        name: development
      targetEnvironmentRefs: []
---
apiVersion: openchoreo.dev/v1alpha1
kind: Environment
metadata:
  name: development
  namespace: %[1]s
spec:
  dataPlaneRef:
    kind: ClusterDataPlane
    name: default
  isProduction: false
`, namespace)
}

// componentTypeYAML returns a minimal ComponentType YAML with a Deployment resource template.
func componentTypeYAML(namespace, name string) string {
	return fmt.Sprintf(`apiVersion: openchoreo.dev/v1alpha1
kind: ComponentType
metadata:
  name: %[2]s
  namespace: %[1]s
spec:
  workloadType: deployment
  resources:
    - id: deployment
      template:
        apiVersion: apps/v1
        kind: Deployment
        metadata:
          name: "${metadata.name}"
          namespace: "${metadata.namespace}"
          labels: "${metadata.labels}"
        spec:
          replicas: 1
          selector:
            matchLabels: "${metadata.podSelectors}"
          template:
            metadata:
              labels: "${metadata.podSelectors}"
            spec:
              containers:
                - name: main
                  image: "${workload.container.image}"
                  command: |
                    ${has(workload.container.command) ? workload.container.command : oc_omit()}
                  args: |
                    ${has(workload.container.args) ? workload.container.args : oc_omit()}
`, namespace, name)
}

// newNamespace returns a gen.Namespace for creation.
func newNamespace(name string) gen.Namespace {
	return gen.Namespace{
		Metadata: gen.ObjectMeta{
			Name: name,
		},
	}
}

// newProject returns a gen.Project for creation.
func newProject(name string) gen.Project {
	return gen.Project{
		Metadata: gen.ObjectMeta{
			Name: name,
		},
	}
}

// newComponent returns a gen.Component for creation, referencing a ClusterComponentType.
func newComponent(name, projectName, clusterComponentTypeName string) gen.Component {
	autoDeploy := true
	kind := gen.ComponentSpecComponentTypeKindClusterComponentType
	return gen.Component{
		Metadata: gen.ObjectMeta{
			Name: name,
		},
		Spec: &gen.ComponentSpec{
			AutoDeploy: &autoDeploy,
			ComponentType: struct {
				Kind *gen.ComponentSpecComponentTypeKind `json:"kind,omitempty"`
				Name string                             `json:"name"`
			}{
				Kind: &kind,
				Name: clusterComponentTypeName,
			},
			Owner: struct {
				ProjectName string `json:"projectName"`
			}{
				ProjectName: projectName,
			},
		},
	}
}

// newWorkload returns a gen.Workload with a minimal container and HTTP endpoint.
func newWorkload(name, componentName, projectName string) gen.Workload {
	port := 8080
	epType := gen.WorkloadEndpointTypeHTTP
	basePath := "/"
	args := []string{"-listen=:8080", "-text=api-e2e"}
	return gen.Workload{
		Metadata: gen.ObjectMeta{
			Name: name,
		},
		Spec: &gen.WorkloadSpec{
			Container: &gen.WorkloadContainer{
				Image: "hashicorp/http-echo:0.2.3",
				Args:  &args,
			},
			Owner: &struct {
				ComponentName string `json:"componentName"`
				ProjectName   string `json:"projectName"`
			}{
				ComponentName: componentName,
				ProjectName:   projectName,
			},
			Endpoints: &map[string]gen.WorkloadEndpoint{
				"http-ep": {
					Port:     port,
					Type:     epType,
					BasePath: &basePath,
				},
			},
		},
	}
}
