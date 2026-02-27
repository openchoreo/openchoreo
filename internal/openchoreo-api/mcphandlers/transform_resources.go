// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package mcphandlers

import (
	"encoding/json"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
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
	setIfNotEmpty(m, "deploymentPipelineRef", p.Spec.DeploymentPipelineRef)
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
	m["projectName"] = w.Spec.Owner.ProjectName
	m["componentName"] = w.Spec.Owner.ComponentName
	m["container"] = containerToMap(&w.Spec.Container)
	if len(w.Spec.Endpoints) > 0 {
		eps := make(map[string]any, len(w.Spec.Endpoints))
		for name, ep := range w.Spec.Endpoints {
			e := map[string]any{
				"type": string(ep.Type),
				"port": ep.Port,
			}
			if ep.TargetPort != 0 {
				e["targetPort"] = ep.TargetPort
			}
			if len(ep.Visibility) > 0 {
				vis := make([]string, 0, len(ep.Visibility))
				for _, v := range ep.Visibility {
					vis = append(vis, string(v))
				}
				e["visibility"] = vis
			}
			setIfNotEmpty(e, "displayName", ep.DisplayName)
			setIfNotEmpty(e, "basePath", ep.BasePath)
			eps[name] = e
		}
		m["endpoints"] = eps
	}
	if len(w.Spec.Connections) > 0 {
		conns := make(map[string]any, len(w.Spec.Connections))
		for name, conn := range w.Spec.Connections {
			c := map[string]any{
				"type": conn.Type,
			}
			if len(conn.Params) > 0 {
				c["params"] = conn.Params
			}
			conns[name] = c
		}
		m["connections"] = conns
	}
	return m
}

func containerToMap(c *openchoreov1alpha1.Container) map[string]any {
	m := map[string]any{"image": c.Image}
	if len(c.Command) > 0 {
		m["command"] = c.Command
	}
	if len(c.Args) > 0 {
		m["args"] = c.Args
	}
	if len(c.Env) > 0 {
		envs := make([]map[string]any, 0, len(c.Env))
		for i := range c.Env {
			e := map[string]any{"key": c.Env[i].Key}
			if c.Env[i].Value != "" {
				e["value"] = c.Env[i].Value
			}
			envs = append(envs, e)
		}
		m["env"] = envs
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

func environmentDetail(e *openchoreov1alpha1.Environment) map[string]any {
	m := extractCommonMeta(e)
	m["isProduction"] = e.Spec.IsProduction
	if e.Spec.DataPlaneRef != nil {
		m["dataPlaneRef"] = map[string]any{
			"kind": string(e.Spec.DataPlaneRef.Kind),
			"name": e.Spec.DataPlaneRef.Name,
		}
	}
	setIfNotEmpty(m, "status", readyStatus(e.Status.Conditions))
	if conds := conditionsSummary(e.Status.Conditions); conds != nil {
		m["conditions"] = conds
	}
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
	setIfNotEmpty(m, "planeID", dp.Spec.PlaneID)
	if dp.Spec.ObservabilityPlaneRef != nil {
		m["observabilityPlaneRef"] = map[string]any{
			"kind": string(dp.Spec.ObservabilityPlaneRef.Kind),
			"name": dp.Spec.ObservabilityPlaneRef.Name,
		}
	}
	if len(dp.Spec.ImagePullSecretRefs) > 0 {
		m["imagePullSecretRefs"] = dp.Spec.ImagePullSecretRefs
	}
	if dp.Spec.SecretStoreRef != nil {
		m["secretStoreRef"] = dp.Spec.SecretStoreRef.Name
	}
	if dp.Status.AgentConnection != nil {
		m["agentConnection"] = agentConnectionToMap(dp.Status.AgentConnection)
	}
	setIfNotEmpty(m, "status", readyStatus(dp.Status.Conditions))
	if conds := conditionsSummary(dp.Status.Conditions); conds != nil {
		m["conditions"] = conds
	}
	return m
}

func agentConnectionToMap(ac *openchoreov1alpha1.AgentConnectionStatus) map[string]any {
	m := map[string]any{
		"connected":       ac.Connected,
		"connectedAgents": ac.ConnectedAgents,
	}
	if ac.LastConnectedTime != nil {
		m["lastConnectedTime"] = ac.LastConnectedTime.UTC().Format("2006-01-02T15:04:05Z")
	}
	if ac.LastDisconnectedTime != nil {
		m["lastDisconnectedTime"] = ac.LastDisconnectedTime.UTC().Format("2006-01-02T15:04:05Z")
	}
	if ac.LastHeartbeatTime != nil {
		m["lastHeartbeatTime"] = ac.LastHeartbeatTime.UTC().Format("2006-01-02T15:04:05Z")
	}
	setIfNotEmpty(m, "message", ac.Message)
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
				"sourceEnvironmentRef": pp.SourceEnvironmentRef,
			}
			if len(pp.TargetEnvironmentRefs) > 0 {
				targets := make([]map[string]any, 0, len(pp.TargetEnvironmentRefs))
				for j := range pp.TargetEnvironmentRefs {
					t := map[string]any{
						"name": pp.TargetEnvironmentRefs[j].Name,
					}
					if pp.TargetEnvironmentRefs[j].RequiresApproval {
						t["requiresApproval"] = true
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
	m["workloadType"] = cr.Spec.ComponentType.WorkloadType
	m["image"] = cr.Spec.Workload.Container.Image
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
	if rb.Spec.ComponentTypeEnvOverrides != nil {
		m["overrides"] = rawExtensionToAny(rb.Spec.ComponentTypeEnvOverrides)
	}
	if len(rb.Status.Endpoints) > 0 {
		urls := make([]map[string]any, 0, len(rb.Status.Endpoints))
		for i := range rb.Status.Endpoints {
			ep := rb.Status.Endpoints[i]
			e := map[string]any{"name": ep.Name}
			setIfNotEmpty(e, "invokeURL", ep.InvokeURL)
			if ep.Type != "" {
				e["type"] = string(ep.Type)
			}
			urls = append(urls, e)
		}
		m["endpoints"] = urls
	}
	setIfNotEmpty(m, "status", readyStatus(rb.Status.Conditions))
	if conds := conditionsSummary(rb.Status.Conditions); conds != nil {
		m["conditions"] = conds
	}
	return m
}

// ---------------------------------------------------------------------------
// Release
// ---------------------------------------------------------------------------

func releaseDetail(r *openchoreov1alpha1.Release) map[string]any {
	m := extractCommonMeta(r)
	m["projectName"] = r.Spec.Owner.ProjectName
	m["componentName"] = r.Spec.Owner.ComponentName
	m["environmentName"] = r.Spec.EnvironmentName
	setIfNotEmpty(m, "status", readyStatus(r.Status.Conditions))
	if len(r.Status.Resources) > 0 {
		m["resourceHealth"] = resourceHealthSummary(r.Status.Resources)
	}
	if conds := conditionsSummary(r.Status.Conditions); conds != nil {
		m["conditions"] = conds
	}
	return m
}

func resourceHealthSummary(resources []openchoreov1alpha1.ResourceStatus) map[string]int {
	counts := make(map[string]int)
	for i := range resources {
		status := string(resources[i].HealthStatus)
		if status == "" {
			status = "Unknown"
		}
		counts[status]++
	}
	return counts
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

// ---------------------------------------------------------------------------
// ComponentType
// ---------------------------------------------------------------------------

func componentTypeSummary(ct openchoreov1alpha1.ComponentType) map[string]any {
	m := extractCommonMeta(&ct)
	m["workloadType"] = ct.Spec.WorkloadType
	if len(ct.Spec.AllowedWorkflows) > 0 {
		m["allowedWorkflows"] = ct.Spec.AllowedWorkflows
	}
	return m
}

// ---------------------------------------------------------------------------
// Trait
// ---------------------------------------------------------------------------

func traitSummary(t openchoreov1alpha1.Trait) map[string]any {
	return extractCommonMeta(&t)
}

// ---------------------------------------------------------------------------
// BuildPlane
// ---------------------------------------------------------------------------

func buildPlaneSummary(bp openchoreov1alpha1.BuildPlane) map[string]any {
	m := extractCommonMeta(&bp)
	setIfNotEmpty(m, "planeID", bp.Spec.PlaneID)
	if bp.Status.AgentConnection != nil {
		m["agentConnected"] = bp.Status.AgentConnection.Connected
	}
	setIfNotEmpty(m, "status", readyStatus(bp.Status.Conditions))
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

// ---------------------------------------------------------------------------
// SecretReference
// ---------------------------------------------------------------------------

func secretReferenceSummary(sr openchoreov1alpha1.SecretReference) map[string]any {
	m := extractCommonMeta(&sr)
	setIfNotEmpty(m, "status", readyStatus(sr.Status.Conditions))
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
	setIfNotEmpty(m, "planeID", cdp.Spec.PlaneID)
	if cdp.Spec.ObservabilityPlaneRef != nil {
		m["observabilityPlaneRef"] = map[string]any{
			"kind": string(cdp.Spec.ObservabilityPlaneRef.Kind),
			"name": cdp.Spec.ObservabilityPlaneRef.Name,
		}
	}
	if len(cdp.Spec.ImagePullSecretRefs) > 0 {
		m["imagePullSecretRefs"] = cdp.Spec.ImagePullSecretRefs
	}
	if cdp.Spec.SecretStoreRef != nil {
		m["secretStoreRef"] = cdp.Spec.SecretStoreRef.Name
	}
	if cdp.Status.AgentConnection != nil {
		m["agentConnection"] = agentConnectionToMap(cdp.Status.AgentConnection)
	}
	setIfNotEmpty(m, "status", readyStatus(cdp.Status.Conditions))
	if conds := conditionsSummary(cdp.Status.Conditions); conds != nil {
		m["conditions"] = conds
	}
	return m
}

// ---------------------------------------------------------------------------
// ClusterBuildPlane
// ---------------------------------------------------------------------------

func clusterBuildPlaneSummary(cbp openchoreov1alpha1.ClusterBuildPlane) map[string]any {
	m := extractCommonMeta(&cbp)
	setIfNotEmpty(m, "planeID", cbp.Spec.PlaneID)
	if cbp.Status.AgentConnection != nil {
		m["agentConnected"] = cbp.Status.AgentConnection.Connected
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

// ---------------------------------------------------------------------------
// ClusterComponentType
// ---------------------------------------------------------------------------

func clusterComponentTypeSummary(cct openchoreov1alpha1.ClusterComponentType) map[string]any {
	m := extractCommonMeta(&cct)
	m["workloadType"] = cct.Spec.WorkloadType
	if len(cct.Spec.AllowedWorkflows) > 0 {
		m["allowedWorkflows"] = cct.Spec.AllowedWorkflows
	}
	return m
}

func clusterComponentTypeDetail(cct *openchoreov1alpha1.ClusterComponentType) map[string]any {
	m := extractCommonMeta(cct)
	m["workloadType"] = cct.Spec.WorkloadType
	if len(cct.Spec.AllowedWorkflows) > 0 {
		m["allowedWorkflows"] = cct.Spec.AllowedWorkflows
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
	return extractCommonMeta(ct)
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
