// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package mcphandlers

import (
	"strconv"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

// ---------------------------------------------------------------------------
// EndpointType / ClusterEndpointType
// ---------------------------------------------------------------------------

func endpointTypeSummary(et openchoreov1alpha1.EndpointType) map[string]any {
	m := extractCommonMeta(&et)
	if ids := endpointTypeResourceIDs(et.Spec.Resources); len(ids) > 0 {
		m["resources"] = ids
	}
	return m
}

func endpointTypeDetail(et *openchoreov1alpha1.EndpointType) map[string]any {
	m := extractCommonMeta(et)
	if spec := specToMap(et.Spec); len(spec) > 0 {
		m["spec"] = spec
	}
	return m
}

func clusterEndpointTypeSummary(cet openchoreov1alpha1.ClusterEndpointType) map[string]any {
	m := extractCommonMeta(&cet)
	if ids := endpointTypeResourceIDs(cet.Spec.Resources); len(ids) > 0 {
		m["resources"] = ids
	}
	return m
}

func clusterEndpointTypeDetail(cet *openchoreov1alpha1.ClusterEndpointType) map[string]any {
	m := extractCommonMeta(cet)
	if spec := specToMap(cet.Spec); len(spec) > 0 {
		m["spec"] = spec
	}
	return m
}

// endpointTypeResourceIDs lists the ids of the resources a type renders, which
// is the quickest way to tell types apart in a listing.
func endpointTypeResourceIDs(resources []openchoreov1alpha1.ResourceTemplate) []string {
	if len(resources) == 0 {
		return nil
	}
	ids := make([]string, 0, len(resources))
	for i := range resources {
		ids = append(ids, resources[i].ID)
	}
	return ids
}

// ---------------------------------------------------------------------------
// Endpoint
// ---------------------------------------------------------------------------

func endpointSummary(ep openchoreov1alpha1.Endpoint) map[string]any {
	m := extractCommonMeta(&ep)
	m["projectName"] = ep.Spec.Owner.ProjectName
	m["type"] = endpointTypeRef(ep.Spec.Type)
	m["visibility"] = endpointVisibilities(ep.Spec.Visibility)
	m["backends"] = endpointBackends(ep.Spec.BackendRefs)
	setIfNotEmpty(m, "status", readyStatus(ep.Status.Conditions))
	if ep.Status.LatestRelease != nil {
		m["latestRelease"] = ep.Status.LatestRelease.Name
	}
	return m
}

func endpointDetail(ep *openchoreov1alpha1.Endpoint) map[string]any {
	m := extractCommonMeta(ep)
	m["projectName"] = ep.Spec.Owner.ProjectName
	m["type"] = endpointTypeRef(ep.Spec.Type)
	m["visibility"] = endpointVisibilities(ep.Spec.Visibility)
	m["backends"] = endpointBackends(ep.Spec.BackendRefs)
	if ep.Spec.Schema != nil {
		m["schema"] = map[string]any{"type": ep.Spec.Schema.Type, "content": ep.Spec.Schema.Content}
	}
	if ep.Spec.Parameters != nil {
		m["parameters"] = rawExtensionToAny(ep.Spec.Parameters)
	}
	if ep.Spec.AutoDeploy {
		m["autoDeploy"] = true
	}
	if ep.Status.LatestRelease != nil {
		m["latestRelease"] = map[string]any{
			"name": ep.Status.LatestRelease.Name,
			"hash": ep.Status.LatestRelease.Hash,
		}
	}
	setIfNotEmpty(m, "status", readyStatus(ep.Status.Conditions))
	if conds := conditionsSummary(ep.Status.Conditions); conds != nil {
		m["conditions"] = conds
	}
	return m
}

func endpointTypeRef(ref openchoreov1alpha1.EndpointTypeRef) map[string]any {
	return map[string]any{"kind": string(ref.Kind), "name": ref.Name}
}

func endpointVisibilities(vs []openchoreov1alpha1.EndpointVisibility) []string {
	out := make([]string, 0, len(vs))
	for _, v := range vs {
		out = append(out, string(v))
	}
	return out
}

// endpointBackends flattens backendRefs to the fields an agent reasons about:
// which component endpoint, and what share of traffic.
func endpointBackends(refs []openchoreov1alpha1.EndpointBackendRef) []map[string]any {
	out := make([]map[string]any, 0, len(refs))
	for _, ref := range refs {
		b := map[string]any{"name": ref.Name, "kind": string(ref.Kind)}
		if ref.Component != nil {
			b["component"] = ref.Component.Name
			b["endpoint"] = ref.Component.Endpoint
		}
		if ref.Weight != nil {
			b["weight"] = *ref.Weight
		}
		out = append(out, b)
	}
	return out
}

// ---------------------------------------------------------------------------
// EndpointRelease
// ---------------------------------------------------------------------------

func endpointReleaseSummary(er openchoreov1alpha1.EndpointRelease) map[string]any {
	m := extractCommonMeta(&er)
	m["projectName"] = er.Spec.Owner.ProjectName
	m["endpointName"] = er.Spec.Owner.EndpointName
	m["endpointType"] = map[string]any{
		"kind": string(er.Spec.EndpointType.Kind),
		"name": er.Spec.EndpointType.Name,
	}
	return m
}

func endpointReleaseDetail(er *openchoreov1alpha1.EndpointRelease) map[string]any {
	m := extractCommonMeta(er)
	m["projectName"] = er.Spec.Owner.ProjectName
	m["endpointName"] = er.Spec.Owner.EndpointName
	m["endpointType"] = map[string]any{
		"kind": string(er.Spec.EndpointType.Kind),
		"name": er.Spec.EndpointType.Name,
	}
	if spec := specToMap(er.Spec.EndpointType.Spec); len(spec) > 0 {
		m["endpointTypeSpec"] = spec
	}
	m["visibility"] = endpointVisibilities(er.Spec.Visibility)
	m["backends"] = endpointBackends(er.Spec.BackendRefs)
	if er.Spec.Parameters != nil {
		m["parameters"] = rawExtensionToAny(er.Spec.Parameters)
	}
	return m
}

// ---------------------------------------------------------------------------
// EndpointReleaseBinding
// ---------------------------------------------------------------------------

func endpointReleaseBindingSummary(b openchoreov1alpha1.EndpointReleaseBinding) map[string]any {
	m := extractCommonMeta(&b)
	m["projectName"] = b.Spec.Owner.ProjectName
	m["endpointName"] = b.Spec.Owner.EndpointName
	m["environment"] = b.Spec.Environment
	setIfNotEmpty(m, "endpointRelease", b.Spec.EndpointRelease)
	setIfNotEmpty(m, "status", readyStatus(b.Status.Conditions))
	return m
}

func endpointReleaseBindingDetail(b *openchoreov1alpha1.EndpointReleaseBinding) map[string]any {
	m := extractCommonMeta(b)
	m["projectName"] = b.Spec.Owner.ProjectName
	m["endpointName"] = b.Spec.Owner.EndpointName
	m["environment"] = b.Spec.Environment
	setIfNotEmpty(m, "endpointRelease", b.Spec.EndpointRelease)
	if b.Spec.EndpointTypeEnvironmentConfigs != nil {
		m["endpointTypeEnvironmentConfigs"] = rawExtensionToAny(b.Spec.EndpointTypeEnvironmentConfigs)
	}
	m["backends"] = resolvedEndpointBackends(b.Status.Backends)
	if urls := resolvedEndpointURLs(b.Status.URLs); urls != nil {
		m["urls"] = urls
	}
	setIfNotEmpty(m, "status", readyStatus(b.Status.Conditions))
	if conds := conditionsSummary(b.Status.Conditions); conds != nil {
		m["conditions"] = conds
	}
	return m
}

// resolvedEndpointBackends reports, per backend, whether it resolved in the
// binding's environment and, if not, why.
func resolvedEndpointBackends(backends []openchoreov1alpha1.ResolvedEndpointBackend) []map[string]any {
	out := make([]map[string]any, 0, len(backends))
	for _, rb := range backends {
		b := map[string]any{
			"name":     rb.Name,
			"kind":     string(rb.Kind),
			"weight":   rb.Weight,
			"resolved": rb.Resolved,
		}
		if rb.ServiceURL != nil {
			b["serviceURL"] = map[string]any{
				"scheme": rb.ServiceURL.Scheme,
				"host":   rb.ServiceURL.Host,
				"port":   rb.ServiceURL.Port,
				"path":   rb.ServiceURL.Path,
			}
		}
		setIfNotEmpty(b, "reason", rb.Reason)
		setIfNotEmpty(b, "message", rb.Message)
		out = append(out, b)
	}
	return out
}

// resolvedEndpointURLs reports the URL resolved for each visibility, keyed by
// visibility, with the security clients must satisfy to call it. Returns nil
// when there are none.
func resolvedEndpointURLs(urls *openchoreov1alpha1.EndpointReleaseBindingURLs) map[string]any {
	if urls == nil {
		return nil
	}
	out := map[string]any{}
	for visibility, u := range map[string]*openchoreov1alpha1.ResolvedEndpointURL{
		"project":   urls.Project,
		"namespace": urls.Namespace,
		"internal":  urls.Internal,
		"external":  urls.External,
	} {
		if u == nil {
			continue
		}
		entry := map[string]any{"url": formatEndpointURL(u.EndpointURL)}
		if u.Security != nil {
			security := map[string]any{"scheme": string(u.Security.Scheme)}
			if u.Security.APIKey != nil {
				security["apiKey"] = map[string]any{"in": string(u.Security.APIKey.In), "name": u.Security.APIKey.Name}
			}
			entry["security"] = security
		}
		out[visibility] = entry
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// formatEndpointURL renders a structured URL as scheme://host[:port][path].
func formatEndpointURL(u openchoreov1alpha1.EndpointURL) string {
	s := u.Host
	if u.Scheme != "" {
		s = u.Scheme + "://" + s
	}
	if u.Port != 0 {
		s += ":" + strconv.Itoa(int(u.Port))
	}
	return s + u.Path
}
