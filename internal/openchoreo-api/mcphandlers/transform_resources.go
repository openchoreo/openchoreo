// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package mcphandlers

import (
	"encoding/json"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	k8sresourcessvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/k8sresources"
)

// ---------------------------------------------------------------------------
// Namespace
// ---------------------------------------------------------------------------

func namespaceSummary(ns corev1.Namespace) map[string]any {
	return extractCommonMeta(&ns)
}

// ---------------------------------------------------------------------------
// Project
// ---------------------------------------------------------------------------

func projectSummary(p openchoreov1alpha1.Project) map[string]any {
	m := extractCommonMeta(&p)
	setIfNotEmpty(m, "deploymentPipelineRef", p.Spec.DeploymentPipelineRef.Name)
	setIfNotEmpty(m, "status", readyStatus(p.Status.Conditions))
	return m
}

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

func componentSummary(c openchoreov1alpha1.Component) map[string]any {
	m := extractCommonMeta(&c)
	m["projectName"] = c.Spec.Owner.ProjectName
	m["componentType"] = c.Spec.ComponentType.Name
	m["autoDeploy"] = c.Spec.AutoDeploy
	if c.Spec.AutoBuild != nil {
		m["autoBuild"] = *c.Spec.AutoBuild
	}
	setIfNotEmpty(m, "status", readyStatus(c.Status.Conditions))
	if c.Status.LatestRelease != nil {
		m["latestRelease"] = c.Status.LatestRelease.Name
	}
	return m
}

func componentDetail(c *openchoreov1alpha1.Component) map[string]any {
	m := extractCommonMeta(c)
	m["projectName"] = c.Spec.Owner.ProjectName
	m["componentType"] = map[string]any{
		"kind": string(c.Spec.ComponentType.Kind),
		"name": c.Spec.ComponentType.Name,
	}
	m["autoDeploy"] = c.Spec.AutoDeploy
	if c.Spec.AutoBuild != nil {
		m["autoBuild"] = *c.Spec.AutoBuild
	}
	if c.Spec.Parameters != nil {
		m["parameters"] = rawExtensionToAny(c.Spec.Parameters)
	}
	if len(c.Spec.Traits) > 0 {
		traits := make([]map[string]any, 0, len(c.Spec.Traits))
		for i := range c.Spec.Traits {
			t := map[string]any{
				"name":         c.Spec.Traits[i].Name,
				"instanceName": c.Spec.Traits[i].InstanceName,
			}
			if c.Spec.Traits[i].Kind != "" {
				t["kind"] = string(c.Spec.Traits[i].Kind)
			}
			if c.Spec.Traits[i].Parameters != nil {
				t["parameters"] = rawExtensionToAny(c.Spec.Traits[i].Parameters)
			}
			traits = append(traits, t)
		}
		m["traits"] = traits
	}
	if c.Spec.Workflow != nil {
		wf := map[string]any{"name": c.Spec.Workflow.Name}
		if c.Spec.Workflow.Kind != "" {
			wf["kind"] = string(c.Spec.Workflow.Kind)
		}
		if c.Spec.Workflow.Parameters != nil {
			wf["parameters"] = rawExtensionToAny(c.Spec.Workflow.Parameters)
		}
		m["workflow"] = wf
	}
	if c.Status.LatestRelease != nil {
		m["latestRelease"] = c.Status.LatestRelease.Name
	}
	setIfNotEmpty(m, "status", readyStatus(c.Status.Conditions))
	if conds := conditionsSummary(c.Status.Conditions); conds != nil {
		m["conditions"] = conds
	}
	return m
}

// ---------------------------------------------------------------------------
// Workload
// ---------------------------------------------------------------------------

func workloadSummary(w openchoreov1alpha1.Workload) map[string]any {
	m := extractCommonMeta(&w)
	m["projectName"] = w.Spec.Owner.ProjectName
	m["componentName"] = w.Spec.Owner.ComponentName
	m["image"] = w.Spec.Container.Image
	if len(w.Spec.Endpoints) > 0 {
		names := make([]string, 0, len(w.Spec.Endpoints))
		for name := range w.Spec.Endpoints {
			names = append(names, name)
		}
		m["endpoints"] = names
	}
	return m
}

func workloadDetail(w *openchoreov1alpha1.Workload) map[string]any {
	m := extractCommonMeta(w)
	if spec := specToMap(w.Spec); len(spec) > 0 {
		m["spec"] = spec
	}
	return m
}

// ---------------------------------------------------------------------------
// Environment
// ---------------------------------------------------------------------------

func environmentSummary(e openchoreov1alpha1.Environment) map[string]any {
	m := extractCommonMeta(&e)
	m["isProduction"] = e.Spec.IsProduction
	if e.Spec.DataPlaneRef != nil {
		m["dataPlaneRef"] = map[string]any{
			"kind": string(e.Spec.DataPlaneRef.Kind),
			"name": e.Spec.DataPlaneRef.Name,
		}
	}
	setIfNotEmpty(m, "status", readyStatus(e.Status.Conditions))
	return m
}

// ---------------------------------------------------------------------------
// DataPlane
// ---------------------------------------------------------------------------

func dataplaneSummary(dp openchoreov1alpha1.DataPlane) map[string]any {
	m := extractCommonMeta(&dp)
	setIfNotEmpty(m, "planeID", dp.Spec.PlaneID)
	if dp.Status.AgentConnection != nil {
		m["agentConnected"] = dp.Status.AgentConnection.Connected
	}
	setIfNotEmpty(m, "status", readyStatus(dp.Status.Conditions))
	return m
}

func dataplaneDetail(dp *openchoreov1alpha1.DataPlane) map[string]any {
	m := extractCommonMeta(dp)
	if spec := specToMap(dp.Spec); len(spec) > 0 {
		m["spec"] = spec
	}
	if dp.Status.AgentConnection != nil {
		if ac := specToMap(dp.Status.AgentConnection); len(ac) > 0 {
			m["agentConnection"] = ac
		}
	}
	setIfNotEmpty(m, "status", readyStatus(dp.Status.Conditions))
	if conds := conditionsSummary(dp.Status.Conditions); conds != nil {
		m["conditions"] = conds
	}
	return m
}

// ---------------------------------------------------------------------------
// DeploymentPipeline
// ---------------------------------------------------------------------------

func deploymentPipelineSummary(dp openchoreov1alpha1.DeploymentPipeline) map[string]any {
	m := extractCommonMeta(&dp)
	setIfNotEmpty(m, "status", readyStatus(dp.Status.Conditions))
	return m
}

func deploymentPipelineDetail(dp *openchoreov1alpha1.DeploymentPipeline) map[string]any {
	m := extractCommonMeta(dp)
	if len(dp.Spec.PromotionPaths) > 0 {
		paths := make([]map[string]any, 0, len(dp.Spec.PromotionPaths))
		for i := range dp.Spec.PromotionPaths {
			pp := dp.Spec.PromotionPaths[i]
			p := map[string]any{
				"sourceEnvironmentRef": map[string]any{
					"kind": string(pp.SourceEnvironmentRef.Kind),
					"name": pp.SourceEnvironmentRef.Name,
				},
			}
			if len(pp.TargetEnvironmentRefs) > 0 {
				targets := make([]map[string]any, 0, len(pp.TargetEnvironmentRefs))
				for j := range pp.TargetEnvironmentRefs {
					t := map[string]any{
						"kind": string(pp.TargetEnvironmentRefs[j].Kind),
						"name": pp.TargetEnvironmentRefs[j].Name,
					}
					targets = append(targets, t)
				}
				p["targetEnvironmentRefs"] = targets
			}
			paths = append(paths, p)
		}
		m["promotionPaths"] = paths
	}
	setIfNotEmpty(m, "status", readyStatus(dp.Status.Conditions))
	if conds := conditionsSummary(dp.Status.Conditions); conds != nil {
		m["conditions"] = conds
	}
	return m
}

// ---------------------------------------------------------------------------
// ComponentRelease
// ---------------------------------------------------------------------------

func componentReleaseSummary(cr openchoreov1alpha1.ComponentRelease) map[string]any {
	m := extractCommonMeta(&cr)
	m["projectName"] = cr.Spec.Owner.ProjectName
	m["componentName"] = cr.Spec.Owner.ComponentName
	return m
}

func componentReleaseDetail(cr *openchoreov1alpha1.ComponentRelease) map[string]any {
	m := extractCommonMeta(cr)
	m["projectName"] = cr.Spec.Owner.ProjectName
	m["componentName"] = cr.Spec.Owner.ComponentName
	m["componentType"] = map[string]any{
		"kind": string(cr.Spec.ComponentType.Kind),
		"name": cr.Spec.ComponentType.Name,
	}
	m["workloadType"] = cr.Spec.ComponentType.Spec.WorkloadType
	m["image"] = cr.Spec.Workload.Container.Image
	if len(cr.Spec.Workload.Endpoints) > 0 {
		m["endpoints"] = cr.Spec.Workload.Endpoints
	}
	if cr.Spec.Workload.Dependencies != nil {
		m["dependencies"] = cr.Spec.Workload.Dependencies
	}
	if cr.Spec.ComponentProfile != nil && cr.Spec.ComponentProfile.Parameters != nil {
		m["parameters"] = rawExtensionToAny(cr.Spec.ComponentProfile.Parameters)
	}
	return m
}

// ---------------------------------------------------------------------------
// ReleaseBinding
// ---------------------------------------------------------------------------

func releaseBindingSummary(rb openchoreov1alpha1.ReleaseBinding) map[string]any {
	m := extractCommonMeta(&rb)
	m["projectName"] = rb.Spec.Owner.ProjectName
	m["componentName"] = rb.Spec.Owner.ComponentName
	m["environment"] = rb.Spec.Environment
	setIfNotEmpty(m, "releaseName", rb.Spec.ReleaseName)
	if rb.Spec.State != "" {
		m["state"] = string(rb.Spec.State)
	}
	if len(rb.Status.Endpoints) > 0 {
		m["endpoints"] = rb.Status.Endpoints
	}
	if len(rb.Status.PendingConnections) > 0 {
		m["pendingConnections"] = rb.Status.PendingConnections
	}
	setIfNotEmpty(m, "status", readyStatus(rb.Status.Conditions))
	return m
}

func releaseBindingDetail(rb *openchoreov1alpha1.ReleaseBinding) map[string]any {
	m := extractCommonMeta(rb)
	m["projectName"] = rb.Spec.Owner.ProjectName
	m["componentName"] = rb.Spec.Owner.ComponentName
	m["environment"] = rb.Spec.Environment
	setIfNotEmpty(m, "releaseName", rb.Spec.ReleaseName)
	if rb.Spec.State != "" {
		m["state"] = string(rb.Spec.State)
	}
	if rb.Spec.ComponentTypeEnvironmentConfigs != nil {
		m["componentTypeEnvironmentConfigs"] = rawExtensionToAny(rb.Spec.ComponentTypeEnvironmentConfigs)
	}
	if len(rb.Spec.TraitEnvironmentConfigs) > 0 {
		tec := make(map[string]any, len(rb.Spec.TraitEnvironmentConfigs))
		for k, v := range rb.Spec.TraitEnvironmentConfigs {
			tec[k] = rawExtensionToAny(&v)
		}
		m["traitEnvironmentConfigs"] = tec
	}
	if rb.Spec.WorkloadOverrides != nil {
		m["workloadOverrides"] = rb.Spec.WorkloadOverrides
	}
	if len(rb.Status.Endpoints) > 0 {
		m["endpoints"] = rb.Status.Endpoints
	}
	if len(rb.Status.ConnectionTargets) > 0 {
		m["connectionTargets"] = rb.Status.ConnectionTargets
	}
	if len(rb.Status.ResolvedConnections) > 0 {
		m["resolvedConnections"] = rb.Status.ResolvedConnections
	}
	if len(rb.Status.PendingConnections) > 0 {
		m["pendingConnections"] = rb.Status.PendingConnections
	}
	setIfNotEmpty(m, "status", readyStatus(rb.Status.Conditions))
	return m
}

// ---------------------------------------------------------------------------
// WorkflowRun
// ---------------------------------------------------------------------------

func workflowRunSummary(wr openchoreov1alpha1.WorkflowRun) map[string]any {
	m := extractCommonMeta(&wr)
	m["workflowName"] = wr.Spec.Workflow.Name
	setIfNotEmpty(m, "status", readyStatus(wr.Status.Conditions))
	if wr.Status.StartedAt != nil {
		m["startedAt"] = wr.Status.StartedAt.UTC().Format("2006-01-02T15:04:05Z")
	}
	if wr.Status.CompletedAt != nil {
		m["completedAt"] = wr.Status.CompletedAt.UTC().Format("2006-01-02T15:04:05Z")
	}
	return m
}

func workflowRunDetail(wr *openchoreov1alpha1.WorkflowRun) map[string]any {
	m := extractCommonMeta(wr)
	m["workflowName"] = wr.Spec.Workflow.Name
	if wr.Spec.Workflow.Parameters != nil {
		m["parameters"] = rawExtensionToAny(wr.Spec.Workflow.Parameters)
	}
	setIfNotEmpty(m, "status", readyStatus(wr.Status.Conditions))
	if wr.Status.StartedAt != nil {
		m["startedAt"] = wr.Status.StartedAt.UTC().Format("2006-01-02T15:04:05Z")
	}
	if wr.Status.CompletedAt != nil {
		m["completedAt"] = wr.Status.CompletedAt.UTC().Format("2006-01-02T15:04:05Z")
	}
	if len(wr.Status.Tasks) > 0 {
		tasks := make([]map[string]any, 0, len(wr.Status.Tasks))
		for i := range wr.Status.Tasks {
			t := map[string]any{"name": wr.Status.Tasks[i].Name}
			setIfNotEmpty(t, "phase", wr.Status.Tasks[i].Phase)
			setIfNotEmpty(t, "message", wr.Status.Tasks[i].Message)
			if wr.Status.Tasks[i].StartedAt != nil {
				t["startedAt"] = wr.Status.Tasks[i].StartedAt.UTC().Format("2006-01-02T15:04:05Z")
			}
			if wr.Status.Tasks[i].CompletedAt != nil {
				t["completedAt"] = wr.Status.Tasks[i].CompletedAt.UTC().Format("2006-01-02T15:04:05Z")
			}
			tasks = append(tasks, t)
		}
		m["tasks"] = tasks
	}
	if conds := conditionsSummary(wr.Status.Conditions); conds != nil {
		m["conditions"] = conds
	}
	return m
}

// ---------------------------------------------------------------------------
// Workflow
// ---------------------------------------------------------------------------

func workflowSummary(wf openchoreov1alpha1.Workflow) map[string]any {
	m := extractCommonMeta(&wf)
	setIfNotEmpty(m, "ttlAfterCompletion", wf.Spec.TTLAfterCompletion)
	setIfNotEmpty(m, "status", readyStatus(wf.Status.Conditions))
	return m
}

func workflowDetail(wf *openchoreov1alpha1.Workflow) map[string]any {
	m := extractCommonMeta(wf)
	if spec := specToMap(wf.Spec); len(spec) > 0 {
		m["spec"] = spec
	}
	setIfNotEmpty(m, "status", readyStatus(wf.Status.Conditions))
	return m
}

// ---------------------------------------------------------------------------
// ComponentType
// ---------------------------------------------------------------------------

func componentTypeSummary(ct openchoreov1alpha1.ComponentType) map[string]any {
	m := extractCommonMeta(&ct)
	m["workloadType"] = ct.Spec.WorkloadType
	if len(ct.Spec.AllowedWorkflows) > 0 {
		wfs := make([]map[string]string, len(ct.Spec.AllowedWorkflows))
		for i, ref := range ct.Spec.AllowedWorkflows {
			wfs[i] = map[string]string{"kind": string(ref.Kind), "name": ref.Name}
		}
		m["allowedWorkflows"] = wfs
	}
	return m
}

func componentTypeDetail(ct *openchoreov1alpha1.ComponentType) map[string]any {
	m := extractCommonMeta(ct)
	if spec := specToMap(ct.Spec); len(spec) > 0 {
		m["spec"] = spec
	}
	return m
}

// ---------------------------------------------------------------------------
// Trait
// ---------------------------------------------------------------------------

func traitSummary(t openchoreov1alpha1.Trait) map[string]any {
	return extractCommonMeta(&t)
}

func traitDetail(t *openchoreov1alpha1.Trait) map[string]any {
	m := extractCommonMeta(t)
	if spec := specToMap(t.Spec); len(spec) > 0 {
		m["spec"] = spec
	}
	return m
}

// ---------------------------------------------------------------------------
// WorkflowPlane
// ---------------------------------------------------------------------------

func workflowPlaneSummary(wp openchoreov1alpha1.WorkflowPlane) map[string]any {
	m := extractCommonMeta(&wp)
	setIfNotEmpty(m, "planeID", wp.Spec.PlaneID)
	if wp.Status.AgentConnection != nil {
		m["agentConnected"] = wp.Status.AgentConnection.Connected
	}
	setIfNotEmpty(m, "status", readyStatus(wp.Status.Conditions))
	return m
}

func workflowPlaneDetail(wp *openchoreov1alpha1.WorkflowPlane) map[string]any {
	m := extractCommonMeta(wp)
	if spec := specToMap(wp.Spec); len(spec) > 0 {
		m["spec"] = spec
	}
	if wp.Status.AgentConnection != nil {
		if ac := specToMap(wp.Status.AgentConnection); len(ac) > 0 {
			m["agentConnection"] = ac
		}
	}
	setIfNotEmpty(m, "status", readyStatus(wp.Status.Conditions))
	if conds := conditionsSummary(wp.Status.Conditions); conds != nil {
		m["conditions"] = conds
	}
	return m
}

// ---------------------------------------------------------------------------
// ObservabilityPlane
// ---------------------------------------------------------------------------

func observabilityPlaneSummary(op openchoreov1alpha1.ObservabilityPlane) map[string]any {
	m := extractCommonMeta(&op)
	setIfNotEmpty(m, "planeID", op.Spec.PlaneID)
	setIfNotEmpty(m, "observerURL", op.Spec.ObserverURL)
	if op.Status.AgentConnection != nil {
		m["agentConnected"] = op.Status.AgentConnection.Connected
	}
	setIfNotEmpty(m, "status", readyStatus(op.Status.Conditions))
	return m
}

func observabilityPlaneDetail(op *openchoreov1alpha1.ObservabilityPlane) map[string]any {
	m := extractCommonMeta(op)
	if spec := specToMap(op.Spec); len(spec) > 0 {
		m["spec"] = spec
	}
	if op.Status.AgentConnection != nil {
		if ac := specToMap(op.Status.AgentConnection); len(ac) > 0 {
			m["agentConnection"] = ac
		}
	}
	setIfNotEmpty(m, "status", readyStatus(op.Status.Conditions))
	if conds := conditionsSummary(op.Status.Conditions); conds != nil {
		m["conditions"] = conds
	}
	return m
}

// ---------------------------------------------------------------------------
// SecretReference
// ---------------------------------------------------------------------------

// TODO: surface SecretReference status fields (Conditions, LastRefreshTime, SecretStores) once the
// control-plane controller populates them. Until then, sync status surfaces on the rendered
// ExternalSecret in the data plane and is queryable via get_resource_events.

func secretReferenceSummary(sr openchoreov1alpha1.SecretReference) map[string]any {
	return extractCommonMeta(&sr)
}

func secretReferenceDetail(sr *openchoreov1alpha1.SecretReference) map[string]any {
	m := extractCommonMeta(sr)
	if spec := specToMap(sr.Spec); len(spec) > 0 {
		m["spec"] = spec
	}
	return m
}

// ---------------------------------------------------------------------------
// ClusterDataPlane
// ---------------------------------------------------------------------------

func clusterDataPlaneSummary(cdp openchoreov1alpha1.ClusterDataPlane) map[string]any {
	m := extractCommonMeta(&cdp)
	setIfNotEmpty(m, "planeID", cdp.Spec.PlaneID)
	if cdp.Status.AgentConnection != nil {
		m["agentConnected"] = cdp.Status.AgentConnection.Connected
	}
	setIfNotEmpty(m, "status", readyStatus(cdp.Status.Conditions))
	return m
}

func clusterDataPlaneDetail(cdp *openchoreov1alpha1.ClusterDataPlane) map[string]any {
	m := extractCommonMeta(cdp)
	if spec := specToMap(cdp.Spec); len(spec) > 0 {
		m["spec"] = spec
	}
	if cdp.Status.AgentConnection != nil {
		if ac := specToMap(cdp.Status.AgentConnection); len(ac) > 0 {
			m["agentConnection"] = ac
		}
	}
	setIfNotEmpty(m, "status", readyStatus(cdp.Status.Conditions))
	if conds := conditionsSummary(cdp.Status.Conditions); conds != nil {
		m["conditions"] = conds
	}
	return m
}

// ---------------------------------------------------------------------------
// ClusterWorkflowPlane
// ---------------------------------------------------------------------------

func clusterWorkflowPlaneSummary(cwp openchoreov1alpha1.ClusterWorkflowPlane) map[string]any {
	m := extractCommonMeta(&cwp)
	setIfNotEmpty(m, "planeID", cwp.Spec.PlaneID)
	if cwp.Status.AgentConnection != nil {
		m["agentConnected"] = cwp.Status.AgentConnection.Connected
	}
	return m
}

func clusterWorkflowPlaneDetail(cbp *openchoreov1alpha1.ClusterWorkflowPlane) map[string]any {
	m := extractCommonMeta(cbp)
	if spec := specToMap(cbp.Spec); len(spec) > 0 {
		m["spec"] = spec
	}
	if cbp.Status.AgentConnection != nil {
		if ac := specToMap(cbp.Status.AgentConnection); len(ac) > 0 {
			m["agentConnection"] = ac
		}
	}
	setIfNotEmpty(m, "status", readyStatus(cbp.Status.Conditions))
	if conds := conditionsSummary(cbp.Status.Conditions); conds != nil {
		m["conditions"] = conds
	}
	return m
}

// ---------------------------------------------------------------------------
// ClusterObservabilityPlane
// ---------------------------------------------------------------------------

func clusterObservabilityPlaneSummary(cop openchoreov1alpha1.ClusterObservabilityPlane) map[string]any {
	m := extractCommonMeta(&cop)
	setIfNotEmpty(m, "planeID", cop.Spec.PlaneID)
	setIfNotEmpty(m, "observerURL", cop.Spec.ObserverURL)
	if cop.Status.AgentConnection != nil {
		m["agentConnected"] = cop.Status.AgentConnection.Connected
	}
	return m
}

func clusterObservabilityPlaneDetail(cop *openchoreov1alpha1.ClusterObservabilityPlane) map[string]any {
	m := extractCommonMeta(cop)
	if spec := specToMap(cop.Spec); len(spec) > 0 {
		m["spec"] = spec
	}
	if cop.Status.AgentConnection != nil {
		if ac := specToMap(cop.Status.AgentConnection); len(ac) > 0 {
			m["agentConnection"] = ac
		}
	}
	setIfNotEmpty(m, "status", readyStatus(cop.Status.Conditions))
	if conds := conditionsSummary(cop.Status.Conditions); conds != nil {
		m["conditions"] = conds
	}
	return m
}

// ---------------------------------------------------------------------------
// ClusterComponentType
// ---------------------------------------------------------------------------

func clusterComponentTypeSummary(cct openchoreov1alpha1.ClusterComponentType) map[string]any {
	m := extractCommonMeta(&cct)
	m["workloadType"] = cct.Spec.WorkloadType
	if len(cct.Spec.AllowedWorkflows) > 0 {
		wfs := make([]map[string]string, len(cct.Spec.AllowedWorkflows))
		for i, ref := range cct.Spec.AllowedWorkflows {
			wfs[i] = map[string]string{"kind": string(ref.Kind), "name": ref.Name}
		}
		m["allowedWorkflows"] = wfs
	}
	return m
}

func clusterComponentTypeDetail(cct *openchoreov1alpha1.ClusterComponentType) map[string]any {
	m := extractCommonMeta(cct)
	if spec := specToMap(cct.Spec); len(spec) > 0 {
		m["spec"] = spec
	}
	return m
}

// ---------------------------------------------------------------------------
// ClusterTrait
// ---------------------------------------------------------------------------

func clusterTraitSummary(ct openchoreov1alpha1.ClusterTrait) map[string]any {
	return extractCommonMeta(&ct)
}

func clusterTraitDetail(ct *openchoreov1alpha1.ClusterTrait) map[string]any {
	m := extractCommonMeta(ct)
	if spec := specToMap(ct.Spec); len(spec) > 0 {
		m["spec"] = spec
	}
	return m
}

// ---------------------------------------------------------------------------
// ClusterWorkflow
// ---------------------------------------------------------------------------

func clusterWorkflowSummary(cwf openchoreov1alpha1.ClusterWorkflow) map[string]any {
	return extractCommonMeta(&cwf)
}

func clusterWorkflowDetail(cwf *openchoreov1alpha1.ClusterWorkflow) map[string]any {
	m := extractCommonMeta(cwf)
	if spec := specToMap(cwf.Spec); len(spec) > 0 {
		m["spec"] = spec
	}
	return m
}

// ---------------------------------------------------------------------------
// Resource tree (diagnostics)
// ---------------------------------------------------------------------------

// resourceTreeDetail drops the per-node Object blob and embedded RenderedRelease CR
// — both bloat MCP context with no diagnostic value.
func resourceTreeDetail(result *k8sresourcessvc.K8sResourceTreeResult) map[string]any {
	releases := make([]map[string]any, 0, len(result.RenderedReleases))
	for _, r := range result.RenderedReleases {
		entry := map[string]any{
			"name":         r.Name,
			"target_plane": r.TargetPlane,
		}
		nodes := make([]map[string]any, 0, len(r.Nodes))
		for i := range r.Nodes {
			n := &r.Nodes[i]
			node := map[string]any{
				"version": n.Version,
				"kind":    n.Kind,
				"name":    n.Name,
			}
			if n.Group != "" {
				node["group"] = n.Group
			}
			if n.Namespace != "" {
				node["namespace"] = n.Namespace
			}
			if n.CreatedAt != nil {
				node["created_at"] = n.CreatedAt.UTC().Format("2006-01-02T15:04:05Z")
			}
			if len(n.ParentRefs) > 0 {
				parents := make([]map[string]any, 0, len(n.ParentRefs))
				for j := range n.ParentRefs {
					p := &n.ParentRefs[j]
					ref := map[string]any{
						"kind": p.Kind,
						"name": p.Name,
						"uid":  p.UID,
					}
					if p.Namespace != "" {
						ref["namespace"] = p.Namespace
					}
					parents = append(parents, ref)
				}
				node["parent_refs"] = parents
			}
			if n.Health != nil {
				h := map[string]any{"status": n.Health.Status}
				if n.Health.Message != "" {
					h["message"] = n.Health.Message
				}
				node["health"] = h
			}
			nodes = append(nodes, node)
		}
		entry["nodes"] = nodes
		releases = append(releases, entry)
	}
	return map[string]any{"rendered_releases": releases}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// rawExtensionToAny converts a RawExtension to a native Go value so that
// JSON marshaling produces the original structure instead of a base64 blob.
func rawExtensionToAny(raw *runtime.RawExtension) any {
	if raw == nil || len(raw.Raw) == 0 {
		return nil
	}
	var v any
	if err := json.Unmarshal(raw.Raw, &v); err != nil {
		return string(raw.Raw)
	}
	return v
}

// specToMap marshals a spec struct to a map[string]any via JSON round-trip,
// preserving all fields including RawExtension values.
func specToMap(spec any) map[string]any {
	data, err := json.Marshal(spec)
	if err != nil {
		return nil
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		return nil
	}
	return m
}
