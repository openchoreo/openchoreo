// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package gate

import (
	"encoding/json"
	"strconv"
	"strings"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

// ContextInput carries what the deployment context is built from.
type ContextInput struct {
	Release     *openchoreov1alpha1.ComponentRelease
	Component   *openchoreov1alpha1.Component
	Project     *openchoreov1alpha1.Project
	Environment *openchoreov1alpha1.Environment
	Trigger     openchoreov1alpha1.DeploymentTrigger
	// Endpoints are the resolved endpoint URLs; only available post-deploy.
	Endpoints []openchoreov1alpha1.EndpointURLStatus
}

// mainContainerName is the key the single workload container is exposed under.
const mainContainerName = "main"

// BuildContext returns the CEL inputs a hook's From expressions see. Everything
// hangs off one root, `deployment`:
//
//	deployment.release                       ComponentRelease name
//	deployment.component.{name,labels,annotations,parameters}
//	deployment.componentType.{kind,name}
//	deployment.projectType.{kind,name}
//	deployment.project.name
//	deployment.environment.{name,isProduction}
//	deployment.workload.image
//	deployment.workload.containers.main.image
//	deployment.trigger
//	deployment.endpoints.<name>.{invokeURL,serviceURL}   (post-deploy)
func BuildContext(in ContextInput) map[string]any {
	d := map[string]any{}

	if in.Release != nil {
		d["release"] = in.Release.Name
		d["componentType"] = map[string]any{
			"kind": string(in.Release.Spec.ComponentType.Kind),
			"name": in.Release.Spec.ComponentType.Name,
		}
		image := in.Release.Spec.Workload.Container.Image
		d["workload"] = map[string]any{
			"image": image,
			"containers": map[string]any{
				mainContainerName: map[string]any{"image": image},
			},
		}
		comp := map[string]any{
			"name":       in.Release.Spec.Owner.ComponentName,
			"parameters": rawToAny(in.Release.Spec.ComponentProfile),
		}
		if in.Component != nil {
			comp["labels"] = stringMap(in.Component.Labels)
			comp["annotations"] = stringMap(in.Component.Annotations)
		} else {
			comp["labels"] = map[string]any{}
			comp["annotations"] = map[string]any{}
		}
		d["component"] = comp
		d["project"] = map[string]any{"name": in.Release.Spec.Owner.ProjectName}
	}
	if in.Project != nil {
		d["project"] = map[string]any{"name": in.Project.Name}
		d["projectType"] = map[string]any{
			"kind": string(in.Project.Spec.Type.Kind),
			"name": in.Project.Spec.Type.Name,
		}
	}
	if in.Environment != nil {
		d["environment"] = map[string]any{
			"name":         in.Environment.Name,
			"isProduction": in.Environment.Spec.IsProduction,
		}
	}
	d["trigger"] = string(in.Trigger)

	endpoints := map[string]any{}
	for _, ep := range in.Endpoints {
		e := map[string]any{"invokeURL": ep.InvokeURL, "serviceURL": ""}
		if ep.ServiceURL != nil {
			e["serviceURL"] = formatURL(*ep.ServiceURL)
		}
		endpoints[ep.Name] = e
	}
	d["endpoints"] = endpoints

	return map[string]any{"deployment": d}
}

// formatURL renders an EndpointURL as scheme://host:port/path, omitting the
// scheme prefix for schemes without a URL form (grpc, tcp, udp).
func formatURL(u openchoreov1alpha1.EndpointURL) string {
	var sb strings.Builder
	switch u.Scheme {
	case "http", "https", "ws", "wss", "tls":
		sb.WriteString(u.Scheme)
		sb.WriteString("://")
	}
	sb.WriteString(u.Host)
	if u.Port != 0 {
		sb.WriteString(":")
		sb.WriteString(strconv.Itoa(int(u.Port)))
	}
	if u.Path != "" {
		if !strings.HasPrefix(u.Path, "/") {
			sb.WriteString("/")
		}
		sb.WriteString(u.Path)
	}
	return sb.String()
}

func rawToAny(profile *openchoreov1alpha1.ComponentProfile) any {
	if profile == nil || profile.Parameters == nil || len(profile.Parameters.Raw) == 0 {
		return map[string]any{}
	}
	var out any
	if err := json.Unmarshal(profile.Parameters.Raw, &out); err != nil {
		return map[string]any{}
	}
	return out
}

func stringMap(m map[string]string) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}
