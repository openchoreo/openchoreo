// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package postrendervalidator

import (
	"github.com/openchoreo/openchoreo/internal/pipeline/component/renderer"
)

// containerInfo is the parts of a container the policy rules look at.
type containerInfo struct {
	name string
	// image is the container's image reference.
	image string
	// resources is the container's resources block (limits/requests).
	resources map[string]any
	// privileged reports securityContext.privileged.
	privileged bool
}

// podSpec is the parts of a pod spec the policy rules look at.
type podSpec map[string]any

// containersOf returns the containers of a rendered resource. A workload
// carries its pod spec under spec.template.spec; a bare Pod carries it under
// spec. A resource that is neither has no containers.
func containersOf(r renderer.RenderedResource) []containerInfo {
	var out []containerInfo
	for _, spec := range podSpecsOf(r) {
		list, ok := spec["containers"].([]any)
		if !ok {
			continue
		}
		for _, item := range list {
			c, ok := item.(map[string]any)
			if !ok {
				continue
			}
			info := containerInfo{}
			info.name, _ = c["name"].(string)
			info.image, _ = c["image"].(string)
			if res, ok := c["resources"].(map[string]any); ok {
				info.resources = res
			}
			if sc, ok := c["securityContext"].(map[string]any); ok {
				info.privileged, _ = sc["privileged"].(bool)
			}
			out = append(out, info)
		}
	}
	return out
}

// podSpecsOf returns the pod specs of a rendered resource: the workload's
// pod template for a Deployment-like object, or the pod spec itself for a Pod.
func podSpecsOf(r renderer.RenderedResource) []podSpec {
	spec, ok := r.Resource["spec"].(map[string]any)
	if !ok {
		return nil
	}
	kind, _ := r.Resource["kind"].(string)
	if kind == "Pod" {
		return []podSpec{podSpec(spec)}
	}
	template, ok := spec["template"].(map[string]any)
	if !ok {
		return nil
	}
	pod, ok := template["spec"].(map[string]any)
	if !ok {
		return nil
	}
	return []podSpec{podSpec(pod)}
}
