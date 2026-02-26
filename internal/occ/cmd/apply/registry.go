// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package apply

import (
	"context"
	"io"
	"sort"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
)

type resourceScope int

const (
	scopeCluster    resourceScope = iota
	scopeNamespaced resourceScope = iota
)

type applyCapability int

const (
	capCreateAndUpdate applyCapability = iota
	capCreateOnly      applyCapability = iota
)

const (
	contentTypeJSON = "application/json"
	apiGroup        = "openchoreo.dev"
)

// readOnlyKinds are valid K8s CRD kinds that have no Create/Update endpoints.
var readOnlyKinds = map[string]bool{
	"ComponentRelease": true,
	"Release":          true,
}

// getFn checks if a resource exists. Returns the HTTP status code.
type getFn func(ctx context.Context, c *gen.ClientWithResponses, ns, name string) (int, error)

// createFn creates a resource. Returns status code and response body.
type createFn func(ctx context.Context, c *gen.ClientWithResponses, ns string, body io.Reader) (int, []byte, error)

// updateFn updates a resource. Returns status code and response body.
type updateFn func(ctx context.Context, c *gen.ClientWithResponses, ns, name string, body io.Reader) (int, []byte, error)

type resourceEntry struct {
	scope      resourceScope
	capability applyCapability
	get        getFn
	create     createFn
	update     updateFn // nil for capCreateOnly
}

func getResourceRegistry() map[string]resourceEntry {
	reg := make(map[string]resourceEntry)
	addClusterScopedResources(reg)
	addNamespacedScopedResources(reg)
	return reg
}

//nolint:funlen // cluster-scoped resources registry table
func addClusterScopedResources(reg map[string]resourceEntry) {
	reg["Namespace"] = resourceEntry{
		scope: scopeCluster,
		get: func(ctx context.Context, c *gen.ClientWithResponses, _, name string) (int, error) {
			r, err := c.GetNamespaceWithResponse(ctx, name)
			if err != nil {
				return 0, err
			}
			return r.StatusCode(), nil
		},
		create: func(ctx context.Context, c *gen.ClientWithResponses, _ string, body io.Reader) (int, []byte, error) {
			r, err := c.CreateNamespaceWithBodyWithResponse(ctx, contentTypeJSON, body)
			if err != nil {
				return 0, nil, err
			}
			return r.StatusCode(), r.Body, nil
		},
		update: func(ctx context.Context, c *gen.ClientWithResponses, _, name string, body io.Reader) (int, []byte, error) {
			r, err := c.UpdateNamespaceWithBodyWithResponse(ctx, name, contentTypeJSON, body)
			if err != nil {
				return 0, nil, err
			}
			return r.StatusCode(), r.Body, nil
		},
	}

	reg["ClusterComponentType"] = resourceEntry{
		scope: scopeCluster,
		get: func(ctx context.Context, c *gen.ClientWithResponses, _, name string) (int, error) {
			r, err := c.GetClusterComponentTypeWithResponse(ctx, name)
			if err != nil {
				return 0, err
			}
			return r.StatusCode(), nil
		},
		create: func(ctx context.Context, c *gen.ClientWithResponses, _ string, body io.Reader) (int, []byte, error) {
			r, err := c.CreateClusterComponentTypeWithBodyWithResponse(ctx, contentTypeJSON, body)
			if err != nil {
				return 0, nil, err
			}
			return r.StatusCode(), r.Body, nil
		},
		update: func(ctx context.Context, c *gen.ClientWithResponses, _, name string, body io.Reader) (int, []byte, error) {
			r, err := c.UpdateClusterComponentTypeWithBodyWithResponse(ctx, name, contentTypeJSON, body)
			if err != nil {
				return 0, nil, err
			}
			return r.StatusCode(), r.Body, nil
		},
	}

	reg["ClusterTrait"] = resourceEntry{
		scope: scopeCluster,
		get: func(ctx context.Context, c *gen.ClientWithResponses, _, name string) (int, error) {
			r, err := c.GetClusterTraitWithResponse(ctx, name)
			if err != nil {
				return 0, err
			}
			return r.StatusCode(), nil
		},
		create: func(ctx context.Context, c *gen.ClientWithResponses, _ string, body io.Reader) (int, []byte, error) {
			r, err := c.CreateClusterTraitWithBodyWithResponse(ctx, contentTypeJSON, body)
			if err != nil {
				return 0, nil, err
			}
			return r.StatusCode(), r.Body, nil
		},
		update: func(ctx context.Context, c *gen.ClientWithResponses, _, name string, body io.Reader) (int, []byte, error) {
			r, err := c.UpdateClusterTraitWithBodyWithResponse(ctx, name, contentTypeJSON, body)
			if err != nil {
				return 0, nil, err
			}
			return r.StatusCode(), r.Body, nil
		},
	}

	reg["ClusterBuildPlane"] = resourceEntry{
		scope: scopeCluster,
		get: func(ctx context.Context, c *gen.ClientWithResponses, _, name string) (int, error) {
			r, err := c.GetClusterBuildPlaneWithResponse(ctx, name)
			if err != nil {
				return 0, err
			}
			return r.StatusCode(), nil
		},
		create: func(ctx context.Context, c *gen.ClientWithResponses, _ string, body io.Reader) (int, []byte, error) {
			r, err := c.CreateClusterBuildPlaneWithBodyWithResponse(ctx, contentTypeJSON, body)
			if err != nil {
				return 0, nil, err
			}
			return r.StatusCode(), r.Body, nil
		},
		update: func(ctx context.Context, c *gen.ClientWithResponses, _, name string, body io.Reader) (int, []byte, error) {
			r, err := c.UpdateClusterBuildPlaneWithBodyWithResponse(ctx, name, contentTypeJSON, body)
			if err != nil {
				return 0, nil, err
			}
			return r.StatusCode(), r.Body, nil
		},
	}

	reg["ClusterDataPlane"] = resourceEntry{
		scope: scopeCluster,
		get: func(ctx context.Context, c *gen.ClientWithResponses, _, name string) (int, error) {
			r, err := c.GetClusterDataPlaneWithResponse(ctx, name)
			if err != nil {
				return 0, err
			}
			return r.StatusCode(), nil
		},
		create: func(ctx context.Context, c *gen.ClientWithResponses, _ string, body io.Reader) (int, []byte, error) {
			r, err := c.CreateClusterDataPlaneWithBodyWithResponse(ctx, contentTypeJSON, body)
			if err != nil {
				return 0, nil, err
			}
			return r.StatusCode(), r.Body, nil
		},
		update: func(ctx context.Context, c *gen.ClientWithResponses, _, name string, body io.Reader) (int, []byte, error) {
			r, err := c.UpdateClusterDataPlaneWithBodyWithResponse(ctx, name, contentTypeJSON, body)
			if err != nil {
				return 0, nil, err
			}
			return r.StatusCode(), r.Body, nil
		},
	}

	reg["ClusterObservabilityPlane"] = resourceEntry{
		scope: scopeCluster,
		get: func(ctx context.Context, c *gen.ClientWithResponses, _, name string) (int, error) {
			r, err := c.GetClusterObservabilityPlaneWithResponse(ctx, name)
			if err != nil {
				return 0, err
			}
			return r.StatusCode(), nil
		},
		create: func(ctx context.Context, c *gen.ClientWithResponses, _ string, body io.Reader) (int, []byte, error) {
			r, err := c.CreateClusterObservabilityPlaneWithBodyWithResponse(ctx, contentTypeJSON, body)
			if err != nil {
				return 0, nil, err
			}
			return r.StatusCode(), r.Body, nil
		},
		update: func(ctx context.Context, c *gen.ClientWithResponses, _, name string, body io.Reader) (int, []byte, error) {
			r, err := c.UpdateClusterObservabilityPlaneWithBodyWithResponse(ctx, name, contentTypeJSON, body)
			if err != nil {
				return 0, nil, err
			}
			return r.StatusCode(), r.Body, nil
		},
	}

	reg["AuthzClusterRole"] = resourceEntry{
		scope: scopeCluster,
		get: func(ctx context.Context, c *gen.ClientWithResponses, _, name string) (int, error) {
			r, err := c.GetClusterRoleWithResponse(ctx, name)
			if err != nil {
				return 0, err
			}
			return r.StatusCode(), nil
		},
		create: func(ctx context.Context, c *gen.ClientWithResponses, _ string, body io.Reader) (int, []byte, error) {
			r, err := c.CreateClusterRoleWithBodyWithResponse(ctx, contentTypeJSON, body)
			if err != nil {
				return 0, nil, err
			}
			return r.StatusCode(), r.Body, nil
		},
		update: func(ctx context.Context, c *gen.ClientWithResponses, _, name string, body io.Reader) (int, []byte, error) {
			r, err := c.UpdateClusterRoleWithBodyWithResponse(ctx, name, contentTypeJSON, body)
			if err != nil {
				return 0, nil, err
			}
			return r.StatusCode(), r.Body, nil
		},
	}

	reg["AuthzClusterRoleBinding"] = resourceEntry{
		scope: scopeCluster,
		get: func(ctx context.Context, c *gen.ClientWithResponses, _, name string) (int, error) {
			r, err := c.GetClusterRoleBindingWithResponse(ctx, name)
			if err != nil {
				return 0, err
			}
			return r.StatusCode(), nil
		},
		create: func(ctx context.Context, c *gen.ClientWithResponses, _ string, body io.Reader) (int, []byte, error) {
			r, err := c.CreateClusterRoleBindingWithBodyWithResponse(ctx, contentTypeJSON, body)
			if err != nil {
				return 0, nil, err
			}
			return r.StatusCode(), r.Body, nil
		},
		update: func(ctx context.Context, c *gen.ClientWithResponses, _, name string, body io.Reader) (int, []byte, error) {
			r, err := c.UpdateClusterRoleBindingWithBodyWithResponse(ctx, name, contentTypeJSON, body)
			if err != nil {
				return 0, nil, err
			}
			return r.StatusCode(), r.Body, nil
		},
	}
}

//nolint:funlen,gocyclo // namespaced resources registry table — one entry per resource kind
func addNamespacedScopedResources(reg map[string]resourceEntry) {
	reg["Project"] = resourceEntry{
		scope: scopeNamespaced,
		get: func(ctx context.Context, c *gen.ClientWithResponses, ns, name string) (int, error) {
			r, err := c.GetProjectWithResponse(ctx, ns, name)
			if err != nil {
				return 0, err
			}
			return r.StatusCode(), nil
		},
		create: func(ctx context.Context, c *gen.ClientWithResponses, ns string, body io.Reader) (int, []byte, error) {
			r, err := c.CreateProjectWithBodyWithResponse(ctx, ns, contentTypeJSON, body)
			if err != nil {
				return 0, nil, err
			}
			return r.StatusCode(), r.Body, nil
		},
		update: func(ctx context.Context, c *gen.ClientWithResponses, ns, name string, body io.Reader) (int, []byte, error) {
			r, err := c.UpdateProjectWithBodyWithResponse(ctx, ns, name, contentTypeJSON, body)
			if err != nil {
				return 0, nil, err
			}
			return r.StatusCode(), r.Body, nil
		},
	}

	reg["Component"] = resourceEntry{
		scope: scopeNamespaced,
		get: func(ctx context.Context, c *gen.ClientWithResponses, ns, name string) (int, error) {
			r, err := c.GetComponentWithResponse(ctx, ns, name)
			if err != nil {
				return 0, err
			}
			return r.StatusCode(), nil
		},
		create: func(ctx context.Context, c *gen.ClientWithResponses, ns string, body io.Reader) (int, []byte, error) {
			r, err := c.CreateComponentWithBodyWithResponse(ctx, ns, contentTypeJSON, body)
			if err != nil {
				return 0, nil, err
			}
			return r.StatusCode(), r.Body, nil
		},
		update: func(ctx context.Context, c *gen.ClientWithResponses, ns, name string, body io.Reader) (int, []byte, error) {
			r, err := c.UpdateComponentWithBodyWithResponse(ctx, ns, name, contentTypeJSON, body)
			if err != nil {
				return 0, nil, err
			}
			return r.StatusCode(), r.Body, nil
		},
	}

	reg["ComponentType"] = resourceEntry{
		scope: scopeNamespaced,
		get: func(ctx context.Context, c *gen.ClientWithResponses, ns, name string) (int, error) {
			r, err := c.GetComponentTypeWithResponse(ctx, ns, name)
			if err != nil {
				return 0, err
			}
			return r.StatusCode(), nil
		},
		create: func(ctx context.Context, c *gen.ClientWithResponses, ns string, body io.Reader) (int, []byte, error) {
			r, err := c.CreateComponentTypeWithBodyWithResponse(ctx, ns, contentTypeJSON, body)
			if err != nil {
				return 0, nil, err
			}
			return r.StatusCode(), r.Body, nil
		},
		update: func(ctx context.Context, c *gen.ClientWithResponses, ns, name string, body io.Reader) (int, []byte, error) {
			r, err := c.UpdateComponentTypeWithBodyWithResponse(ctx, ns, name, contentTypeJSON, body)
			if err != nil {
				return 0, nil, err
			}
			return r.StatusCode(), r.Body, nil
		},
	}

	reg["Environment"] = resourceEntry{
		scope: scopeNamespaced,
		get: func(ctx context.Context, c *gen.ClientWithResponses, ns, name string) (int, error) {
			r, err := c.GetEnvironmentWithResponse(ctx, ns, name)
			if err != nil {
				return 0, err
			}
			return r.StatusCode(), nil
		},
		create: func(ctx context.Context, c *gen.ClientWithResponses, ns string, body io.Reader) (int, []byte, error) {
			r, err := c.CreateEnvironmentWithBodyWithResponse(ctx, ns, contentTypeJSON, body)
			if err != nil {
				return 0, nil, err
			}
			return r.StatusCode(), r.Body, nil
		},
		update: func(ctx context.Context, c *gen.ClientWithResponses, ns, name string, body io.Reader) (int, []byte, error) {
			r, err := c.UpdateEnvironmentWithBodyWithResponse(ctx, ns, name, contentTypeJSON, body)
			if err != nil {
				return 0, nil, err
			}
			return r.StatusCode(), r.Body, nil
		},
	}

	reg["DataPlane"] = resourceEntry{
		scope: scopeNamespaced,
		get: func(ctx context.Context, c *gen.ClientWithResponses, ns, name string) (int, error) {
			r, err := c.GetDataPlaneWithResponse(ctx, ns, name)
			if err != nil {
				return 0, err
			}
			return r.StatusCode(), nil
		},
		create: func(ctx context.Context, c *gen.ClientWithResponses, ns string, body io.Reader) (int, []byte, error) {
			r, err := c.CreateDataPlaneWithBodyWithResponse(ctx, ns, contentTypeJSON, body)
			if err != nil {
				return 0, nil, err
			}
			return r.StatusCode(), r.Body, nil
		},
		update: func(ctx context.Context, c *gen.ClientWithResponses, ns, name string, body io.Reader) (int, []byte, error) {
			r, err := c.UpdateDataPlaneWithBodyWithResponse(ctx, ns, name, contentTypeJSON, body)
			if err != nil {
				return 0, nil, err
			}
			return r.StatusCode(), r.Body, nil
		},
	}

	reg["BuildPlane"] = resourceEntry{
		scope: scopeNamespaced,
		get: func(ctx context.Context, c *gen.ClientWithResponses, ns, name string) (int, error) {
			r, err := c.GetBuildPlaneWithResponse(ctx, ns, name)
			if err != nil {
				return 0, err
			}
			return r.StatusCode(), nil
		},
		create: func(ctx context.Context, c *gen.ClientWithResponses, ns string, body io.Reader) (int, []byte, error) {
			r, err := c.CreateBuildPlaneWithBodyWithResponse(ctx, ns, contentTypeJSON, body)
			if err != nil {
				return 0, nil, err
			}
			return r.StatusCode(), r.Body, nil
		},
		update: func(ctx context.Context, c *gen.ClientWithResponses, ns, name string, body io.Reader) (int, []byte, error) {
			r, err := c.UpdateBuildPlaneWithBodyWithResponse(ctx, ns, name, contentTypeJSON, body)
			if err != nil {
				return 0, nil, err
			}
			return r.StatusCode(), r.Body, nil
		},
	}

	reg["ObservabilityPlane"] = resourceEntry{
		scope: scopeNamespaced,
		get: func(ctx context.Context, c *gen.ClientWithResponses, ns, name string) (int, error) {
			r, err := c.GetObservabilityPlaneWithResponse(ctx, ns, name)
			if err != nil {
				return 0, err
			}
			return r.StatusCode(), nil
		},
		create: func(ctx context.Context, c *gen.ClientWithResponses, ns string, body io.Reader) (int, []byte, error) {
			r, err := c.CreateObservabilityPlaneWithBodyWithResponse(ctx, ns, contentTypeJSON, body)
			if err != nil {
				return 0, nil, err
			}
			return r.StatusCode(), r.Body, nil
		},
		update: func(ctx context.Context, c *gen.ClientWithResponses, ns, name string, body io.Reader) (int, []byte, error) {
			r, err := c.UpdateObservabilityPlaneWithBodyWithResponse(ctx, ns, name, contentTypeJSON, body)
			if err != nil {
				return 0, nil, err
			}
			return r.StatusCode(), r.Body, nil
		},
	}

	reg["DeploymentPipeline"] = resourceEntry{
		scope: scopeNamespaced,
		get: func(ctx context.Context, c *gen.ClientWithResponses, ns, name string) (int, error) {
			r, err := c.GetDeploymentPipelineWithResponse(ctx, ns, name)
			if err != nil {
				return 0, err
			}
			return r.StatusCode(), nil
		},
		create: func(ctx context.Context, c *gen.ClientWithResponses, ns string, body io.Reader) (int, []byte, error) {
			r, err := c.CreateDeploymentPipelineWithBodyWithResponse(ctx, ns, contentTypeJSON, body)
			if err != nil {
				return 0, nil, err
			}
			return r.StatusCode(), r.Body, nil
		},
		update: func(ctx context.Context, c *gen.ClientWithResponses, ns, name string, body io.Reader) (int, []byte, error) {
			r, err := c.UpdateDeploymentPipelineWithBodyWithResponse(ctx, ns, name, contentTypeJSON, body)
			if err != nil {
				return 0, nil, err
			}
			return r.StatusCode(), r.Body, nil
		},
	}

	reg["Trait"] = resourceEntry{
		scope: scopeNamespaced,
		get: func(ctx context.Context, c *gen.ClientWithResponses, ns, name string) (int, error) {
			r, err := c.GetTraitWithResponse(ctx, ns, name)
			if err != nil {
				return 0, err
			}
			return r.StatusCode(), nil
		},
		create: func(ctx context.Context, c *gen.ClientWithResponses, ns string, body io.Reader) (int, []byte, error) {
			r, err := c.CreateTraitWithBodyWithResponse(ctx, ns, contentTypeJSON, body)
			if err != nil {
				return 0, nil, err
			}
			return r.StatusCode(), r.Body, nil
		},
		update: func(ctx context.Context, c *gen.ClientWithResponses, ns, name string, body io.Reader) (int, []byte, error) {
			r, err := c.UpdateTraitWithBodyWithResponse(ctx, ns, name, contentTypeJSON, body)
			if err != nil {
				return 0, nil, err
			}
			return r.StatusCode(), r.Body, nil
		},
	}

	reg["SecretReference"] = resourceEntry{
		scope: scopeNamespaced,
		get: func(ctx context.Context, c *gen.ClientWithResponses, ns, name string) (int, error) {
			r, err := c.GetSecretReferenceWithResponse(ctx, ns, name)
			if err != nil {
				return 0, err
			}
			return r.StatusCode(), nil
		},
		create: func(ctx context.Context, c *gen.ClientWithResponses, ns string, body io.Reader) (int, []byte, error) {
			r, err := c.CreateSecretReferenceWithBodyWithResponse(ctx, ns, contentTypeJSON, body)
			if err != nil {
				return 0, nil, err
			}
			return r.StatusCode(), r.Body, nil
		},
		update: func(ctx context.Context, c *gen.ClientWithResponses, ns, name string, body io.Reader) (int, []byte, error) {
			r, err := c.UpdateSecretReferenceWithBodyWithResponse(ctx, ns, name, contentTypeJSON, body)
			if err != nil {
				return 0, nil, err
			}
			return r.StatusCode(), r.Body, nil
		},
	}

	reg["Workflow"] = resourceEntry{
		scope: scopeNamespaced,
		get: func(ctx context.Context, c *gen.ClientWithResponses, ns, name string) (int, error) {
			r, err := c.GetWorkflowWithResponse(ctx, ns, name)
			if err != nil {
				return 0, err
			}
			return r.StatusCode(), nil
		},
		create: func(ctx context.Context, c *gen.ClientWithResponses, ns string, body io.Reader) (int, []byte, error) {
			r, err := c.CreateWorkflowWithBodyWithResponse(ctx, ns, contentTypeJSON, body)
			if err != nil {
				return 0, nil, err
			}
			return r.StatusCode(), r.Body, nil
		},
		update: func(ctx context.Context, c *gen.ClientWithResponses, ns, name string, body io.Reader) (int, []byte, error) {
			r, err := c.UpdateWorkflowWithBodyWithResponse(ctx, ns, name, contentTypeJSON, body)
			if err != nil {
				return 0, nil, err
			}
			return r.StatusCode(), r.Body, nil
		},
	}

	reg["Workload"] = resourceEntry{
		scope: scopeNamespaced,
		get: func(ctx context.Context, c *gen.ClientWithResponses, ns, name string) (int, error) {
			r, err := c.GetWorkloadWithResponse(ctx, ns, name)
			if err != nil {
				return 0, err
			}
			return r.StatusCode(), nil
		},
		create: func(ctx context.Context, c *gen.ClientWithResponses, ns string, body io.Reader) (int, []byte, error) {
			r, err := c.CreateWorkloadWithBodyWithResponse(ctx, ns, contentTypeJSON, body)
			if err != nil {
				return 0, nil, err
			}
			return r.StatusCode(), r.Body, nil
		},
		update: func(ctx context.Context, c *gen.ClientWithResponses, ns, name string, body io.Reader) (int, []byte, error) {
			r, err := c.UpdateWorkloadWithBodyWithResponse(ctx, ns, name, contentTypeJSON, body)
			if err != nil {
				return 0, nil, err
			}
			return r.StatusCode(), r.Body, nil
		},
	}

	reg["ReleaseBinding"] = resourceEntry{
		scope: scopeNamespaced,
		get: func(ctx context.Context, c *gen.ClientWithResponses, ns, name string) (int, error) {
			r, err := c.GetReleaseBindingWithResponse(ctx, ns, name)
			if err != nil {
				return 0, err
			}
			return r.StatusCode(), nil
		},
		create: func(ctx context.Context, c *gen.ClientWithResponses, ns string, body io.Reader) (int, []byte, error) {
			r, err := c.CreateReleaseBindingWithBodyWithResponse(ctx, ns, contentTypeJSON, body)
			if err != nil {
				return 0, nil, err
			}
			return r.StatusCode(), r.Body, nil
		},
		update: func(ctx context.Context, c *gen.ClientWithResponses, ns, name string, body io.Reader) (int, []byte, error) {
			r, err := c.UpdateReleaseBindingWithBodyWithResponse(ctx, ns, name, contentTypeJSON, body)
			if err != nil {
				return 0, nil, err
			}
			return r.StatusCode(), r.Body, nil
		},
	}

	reg["ObservabilityAlertsNotificationChannel"] = resourceEntry{
		scope: scopeNamespaced,
		get: func(ctx context.Context, c *gen.ClientWithResponses, ns, name string) (int, error) {
			r, err := c.GetObservabilityAlertsNotificationChannelWithResponse(ctx, ns, name)
			if err != nil {
				return 0, err
			}
			return r.StatusCode(), nil
		},
		create: func(ctx context.Context, c *gen.ClientWithResponses, ns string, body io.Reader) (int, []byte, error) {
			r, err := c.CreateObservabilityAlertsNotificationChannelWithBodyWithResponse(ctx, ns, contentTypeJSON, body)
			if err != nil {
				return 0, nil, err
			}
			return r.StatusCode(), r.Body, nil
		},
		update: func(ctx context.Context, c *gen.ClientWithResponses, ns, name string, body io.Reader) (int, []byte, error) {
			r, err := c.UpdateObservabilityAlertsNotificationChannelWithBodyWithResponse(ctx, ns, name, contentTypeJSON, body)
			if err != nil {
				return 0, nil, err
			}
			return r.StatusCode(), r.Body, nil
		},
	}

	reg["AuthzRole"] = resourceEntry{
		scope: scopeNamespaced,
		get: func(ctx context.Context, c *gen.ClientWithResponses, ns, name string) (int, error) {
			r, err := c.GetNamespaceRoleWithResponse(ctx, ns, name)
			if err != nil {
				return 0, err
			}
			return r.StatusCode(), nil
		},
		create: func(ctx context.Context, c *gen.ClientWithResponses, ns string, body io.Reader) (int, []byte, error) {
			r, err := c.CreateNamespaceRoleWithBodyWithResponse(ctx, ns, contentTypeJSON, body)
			if err != nil {
				return 0, nil, err
			}
			return r.StatusCode(), r.Body, nil
		},
		update: func(ctx context.Context, c *gen.ClientWithResponses, ns, name string, body io.Reader) (int, []byte, error) {
			r, err := c.UpdateNamespaceRoleWithBodyWithResponse(ctx, ns, name, contentTypeJSON, body)
			if err != nil {
				return 0, nil, err
			}
			return r.StatusCode(), r.Body, nil
		},
	}

	reg["AuthzRoleBinding"] = resourceEntry{
		scope: scopeNamespaced,
		get: func(ctx context.Context, c *gen.ClientWithResponses, ns, name string) (int, error) {
			r, err := c.GetNamespaceRoleBindingWithResponse(ctx, ns, name)
			if err != nil {
				return 0, err
			}
			return r.StatusCode(), nil
		},
		create: func(ctx context.Context, c *gen.ClientWithResponses, ns string, body io.Reader) (int, []byte, error) {
			r, err := c.CreateNamespaceRoleBindingWithBodyWithResponse(ctx, ns, contentTypeJSON, body)
			if err != nil {
				return 0, nil, err
			}
			return r.StatusCode(), r.Body, nil
		},
		update: func(ctx context.Context, c *gen.ClientWithResponses, ns, name string, body io.Reader) (int, []byte, error) {
			r, err := c.UpdateNamespaceRoleBindingWithBodyWithResponse(ctx, ns, name, contentTypeJSON, body)
			if err != nil {
				return 0, nil, err
			}
			return r.StatusCode(), r.Body, nil
		},
	}

	// Create-only resources

	reg["WorkflowRun"] = resourceEntry{
		scope:      scopeNamespaced,
		capability: capCreateOnly,
		get: func(ctx context.Context, c *gen.ClientWithResponses, ns, name string) (int, error) {
			r, err := c.GetWorkflowRunWithResponse(ctx, ns, name)
			if err != nil {
				return 0, err
			}
			return r.StatusCode(), nil
		},
		create: func(ctx context.Context, c *gen.ClientWithResponses, ns string, body io.Reader) (int, []byte, error) {
			r, err := c.CreateWorkflowRunWithBodyWithResponse(ctx, ns, contentTypeJSON, body)
			if err != nil {
				return 0, nil, err
			}
			return r.StatusCode(), r.Body, nil
		},
	}
}

// supportedKinds returns a sorted list of supported kind names.
func supportedKinds() []string {
	reg := getResourceRegistry()
	kinds := make([]string, 0, len(reg))
	for k := range reg {
		kinds = append(kinds, k)
	}
	sort.Strings(kinds)
	return kinds
}
