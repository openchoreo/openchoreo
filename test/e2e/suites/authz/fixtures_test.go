// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package e2e

import (
	"fmt"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
)

const (
	projectName = "authz-proj"
	compName    = "authz-comp"
)

func cpNamespaceYAML(ns string) string {
	return fmt.Sprintf(`apiVersion: v1
kind: Namespace
metadata:
  name: %s
  labels:
    openchoreo.dev/control-plane: "true"
`, ns)
}

func platformResourcesYAML(ns string) string {
	return fmt.Sprintf(`apiVersion: openchoreo.dev/v1alpha1
kind: DeploymentPipeline
metadata:
  name: default
  namespace: %[1]s
spec:
  promotionPaths:
    - sourceEnvironmentRef:
        name: development
      targetEnvironmentRefs:
        - name: staging
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
---
apiVersion: openchoreo.dev/v1alpha1
kind: Environment
metadata:
  name: staging
  namespace: %[1]s
spec:
  dataPlaneRef:
    kind: ClusterDataPlane
    name: default
  isProduction: false
---
apiVersion: openchoreo.dev/v1alpha1
kind: Project
metadata:
  name: %[2]s
  namespace: %[1]s
  labels:
    openchoreo.dev/name: %[2]s
spec:
  deploymentPipelineRef:
    name: default
`, ns, projectName)
}

// clusterAuthzRoleBindingYAML creates a ClusterAuthzRoleBinding that maps the
// test subject (customer-portal-client) to the given ClusterAuthzRole.
func clusterAuthzRoleBindingYAML(name, roleName, effect string) string {
	labels := testLabel()
	return fmt.Sprintf(`apiVersion: openchoreo.dev/v1alpha1
kind: ClusterAuthzRoleBinding
metadata:
  name: %s
  labels:
    e2e-authz/run: %s
spec:
  roleMappings:
    - roleRef:
        name: %s
        kind: ClusterAuthzRole
  entitlement:
    claim: sub
    value: %s
  effect: %s
`, name, labels["e2e-authz/run"], roleName, subjectClientID, effect)
}

// scopedClusterAuthzRoleBindingYAML creates a ClusterAuthzRoleBinding scoped to a specific namespace.
func scopedClusterAuthzRoleBindingYAML(name, roleName, effect, scopeNs string) string {
	labels := testLabel()
	return fmt.Sprintf(`apiVersion: openchoreo.dev/v1alpha1
kind: ClusterAuthzRoleBinding
metadata:
  name: %s
  labels:
    e2e-authz/run: %s
spec:
  roleMappings:
    - roleRef:
        name: %s
        kind: ClusterAuthzRole
      scope:
        namespace: %s
  entitlement:
    claim: sub
    value: %s
  effect: %s
`, name, labels["e2e-authz/run"], roleName, scopeNs, subjectClientID, effect)
}

// clusterAuthzRoleYAML creates a ClusterAuthzRole with the given actions.
func clusterAuthzRoleYAML(name string, actions []string) string {
	labels := testLabel()
	actionsYAML := ""
	for _, a := range actions {
		actionsYAML += fmt.Sprintf("    - %q\n", a)
	}
	return fmt.Sprintf(`apiVersion: openchoreo.dev/v1alpha1
kind: ClusterAuthzRole
metadata:
  name: %s
  labels:
    e2e-authz/run: %s
spec:
  actions:
%s`, name, labels["e2e-authz/run"], actionsYAML)
}

// authzRoleYAML creates a namespace-scoped AuthzRole.
func authzRoleYAML(ns, name string, actions []string) string {
	actionsYAML := ""
	for _, a := range actions {
		actionsYAML += fmt.Sprintf("    - %q\n", a)
	}
	return fmt.Sprintf(`apiVersion: openchoreo.dev/v1alpha1
kind: AuthzRole
metadata:
  name: %s
  namespace: %s
spec:
  actions:
%s`, name, ns, actionsYAML)
}

// authzRoleBindingYAML creates a namespace-scoped AuthzRoleBinding.
func authzRoleBindingYAML(ns, name, roleName, effect string) string {
	return fmt.Sprintf(`apiVersion: openchoreo.dev/v1alpha1
kind: AuthzRoleBinding
metadata:
  name: %s
  namespace: %s
spec:
  roleMappings:
    - roleRef:
        name: %s
        kind: AuthzRole
  entitlement:
    claim: sub
    value: %s
  effect: %s
`, name, ns, roleName, subjectClientID, effect)
}

func newNamespace(name string) gen.Namespace {
	return gen.Namespace{
		Metadata: gen.ObjectMeta{
			Name: name,
		},
	}
}

func newComponent(name, project, clusterComponentTypeName string) gen.Component {
	return newComponentWithType(name, project, clusterComponentTypeName, gen.ComponentSpecComponentTypeKindClusterComponentType)
}

func newComponentWithType(name, project, typeName string, typeKind gen.ComponentSpecComponentTypeKind) gen.Component {
	autoDeploy := true
	return gen.Component{
		Metadata: gen.ObjectMeta{
			Name: name,
		},
		Spec: &gen.ComponentSpec{
			AutoDeploy: &autoDeploy,
			ComponentType: struct {
				Kind *gen.ComponentSpecComponentTypeKind `json:"kind,omitempty"`
				Name string                              `json:"name"`
			}{
				Kind: &typeKind,
				Name: typeName,
			},
			Owner: struct {
				ProjectName string `json:"projectName"`
			}{
				ProjectName: project,
			},
		},
	}
}

func componentTypeYAML(ns, name string) string {
	return fmt.Sprintf(`apiVersion: openchoreo.dev/v1alpha1
kind: ComponentType
metadata:
  name: %s
  namespace: %s
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
`, name, ns)
}

// clusterAuthzRoleBindingWithConditionsYAML creates a ClusterAuthzRoleBinding
// with a CEL condition on specific actions.
func clusterAuthzRoleBindingWithConditionsYAML(name, roleName, effect, condActions, condExpr string) string {
	labels := testLabel()
	return fmt.Sprintf(`apiVersion: openchoreo.dev/v1alpha1
kind: ClusterAuthzRoleBinding
metadata:
  name: %s
  labels:
    e2e-authz/run: %s
spec:
  roleMappings:
    - roleRef:
        name: %s
        kind: ClusterAuthzRole
      conditions:
        - actions:
            - %q
          expression: %q
  entitlement:
    claim: sub
    value: %s
  effect: %s
`, name, labels["e2e-authz/run"], roleName, condActions, condExpr, subjectClientID, effect)
}

func newEnvironment(name string) gen.Environment {
	dpKind := gen.EnvironmentSpecDataPlaneRefKindClusterDataPlane
	isProd := false
	return gen.Environment{
		Metadata: gen.ObjectMeta{
			Name: name,
		},
		Spec: &gen.EnvironmentSpec{
			DataPlaneRef: &struct {
				Kind gen.EnvironmentSpecDataPlaneRefKind `json:"kind"`
				Name string                              `json:"name"`
			}{
				Kind: dpKind,
				Name: "default",
			},
			IsProduction: &isProd,
		},
	}
}
