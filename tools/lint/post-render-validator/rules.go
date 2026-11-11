// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package postrendervalidator

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/openchoreo/openchoreo/internal/pipeline/component/renderer"
	"github.com/openchoreo/openchoreo/tools/lint/ruleengine"
)

// Rule is a post-render validation rule that operates on the rendered
// Kubernetes resources produced by the rendering pipeline.
type Rule interface {
	// Name is the rule's identifier, used as the diagnostic code.
	Name() string
	// Evaluate returns the diagnostics the rule found in the given resources.
	Evaluate(resources []renderer.RenderedResource) ruleengine.Diagnostics
}

// makeDiag creates a diagnostic for a rendered resource. The diagnostic is
// anchored to the resource identity rather than to a source range: rendered
// resources are plain maps with no source positions.
func makeDiag(sev ruleengine.Severity, code, message string, r renderer.RenderedResource) ruleengine.Diagnostic {
	return ruleengine.Diagnostic{
		Severity: sev,
		Code:     code,
		Message:  fmt.Sprintf("%s: %s", resourceIdentity(r), message),
	}
}

// resourceIdentity names a rendered resource as "Kind/name" so a finding can
// be traced back to the object it was found in.
func resourceIdentity(r renderer.RenderedResource) string {
	kind, _ := r.Resource["kind"].(string)
	name := ""
	if meta, ok := r.Resource["metadata"].(map[string]any); ok {
		name, _ = meta["name"].(string)
	}
	switch {
	case kind == "" && name == "":
		return "unknown resource"
	case kind == "":
		return name
	case name == "":
		return kind
	}
	return kind + "/" + name
}

// defaultRules returns the built-in post-render rules, in the order they are
// evaluated.
func defaultRules() []Rule {
	return []Rule{
		unresolvedExprRule{},
		missingFieldsRule{},
		imageTagRule{},
		resourceLimitsRule{},
		resourceRequestsRule{},
		replicasRule{},
		hostNamespaceRule{},
		privilegedRule{},
	}
}

// unresolvedExprRule reports expressions that survived rendering, which means
// the template referenced something the context did not provide.
type unresolvedExprRule struct{}

func (unresolvedExprRule) Name() string { return "post-render-unresolved-expression" }

func (u unresolvedExprRule) Evaluate(resources []renderer.RenderedResource) ruleengine.Diagnostics {
	var diags ruleengine.Diagnostics
	for _, r := range resources {
		if expr, ok := findUnresolved(r.Resource); ok {
			diags = append(diags, makeDiag(
				ruleengine.SeverityError,
				u.Name(),
				fmt.Sprintf("unresolved expression %q", expr),
				r,
			))
		}
	}
	return diags
}

// findUnresolved walks a rendered value and reports the first string that
// still contains a template expression.
func findUnresolved(v any) (string, bool) {
	switch x := v.(type) {
	case string:
		if i := strings.Index(x, "${"); i >= 0 {
			return x[i:], true
		}
	case map[string]any:
		for _, val := range x {
			if s, ok := findUnresolved(val); ok {
				return s, true
			}
		}
	case []any:
		for _, val := range x {
			if s, ok := findUnresolved(val); ok {
				return s, true
			}
		}
	}
	return "", false
}

// missingFieldsRule reports resources that are not usable Kubernetes objects.
type missingFieldsRule struct{}

func (missingFieldsRule) Name() string { return "post-render-missing-field" }

func (m missingFieldsRule) Evaluate(resources []renderer.RenderedResource) ruleengine.Diagnostics {
	var diags ruleengine.Diagnostics
	for _, r := range resources {
		apiVersion, _ := r.Resource["apiVersion"].(string)
		kind, _ := r.Resource["kind"].(string)
		switch {
		case apiVersion == "":
			diags = append(diags, makeDiag(ruleengine.SeverityError, m.Name(), "resource has no apiVersion", r))
			continue
		case kind == "":
			diags = append(diags, makeDiag(ruleengine.SeverityError, m.Name(), "resource has no kind", r))
			continue
		}
		meta, ok := r.Resource["metadata"].(map[string]any)
		if !ok {
			diags = append(diags, makeDiag(ruleengine.SeverityError, m.Name(), "resource has no metadata", r))
			continue
		}
		if name, _ := meta["name"].(string); name == "" {
			diags = append(diags, makeDiag(ruleengine.SeverityError, m.Name(), "resource has no metadata.name", r))
		}
	}
	return diags
}

// imageTagRule requires an explicit tag (including latest) or digest.
type imageTagRule struct{}

func (imageTagRule) Name() string { return "image-latest" }

func (i imageTagRule) Evaluate(resources []renderer.RenderedResource) ruleengine.Diagnostics {
	var diags ruleengine.Diagnostics
	for _, r := range resources {
		for _, c := range containersOf(r) {
			if c.image == "" {
				continue
			}
			if reason := unpinnedImageReason(c.image); reason != "" {
				diags = append(diags, makeDiag(
					ruleengine.SeverityError,
					i.Name(),
					fmt.Sprintf("container %q uses image %q: %s", c.name, c.image, reason),
					r,
				))
			}
		}
	}
	return diags
}

// unpinnedImageReason returns why an image lacks an explicit tag or digest.
func unpinnedImageReason(image string) string {
	// A digest reference is immutable by definition.
	if i := strings.Index(image, "@"); i >= 0 && image[i+1:] != "" {
		return ""
	}
	// Only the tag after the last colon of the last path segment counts.
	// A colon before the last slash is a registry port, not a tag.
	lastSlash := strings.LastIndex(image, "/")
	segment := image[lastSlash+1:]
	colon := strings.LastIndex(segment, ":")
	if colon < 0 {
		return "specify an image tag, such as a version or latest, or an image digest"
	}
	tag := segment[colon+1:]
	if tag == "" {
		return "specify an image tag, such as a version or latest, or an image digest"
	}
	return ""
}

// resourceLimitsRule reports containers that do not declare CPU and memory
// limits.
type resourceLimitsRule struct{}

func (resourceLimitsRule) Name() string { return "limits-required" }

func (l resourceLimitsRule) Evaluate(resources []renderer.RenderedResource) ruleengine.Diagnostics {
	return checkResourceQuantities(resources, l.Name(), "limits")
}

// resourceRequestsRule reports containers that do not declare CPU and memory
// requests.
type resourceRequestsRule struct{}

func (resourceRequestsRule) Name() string { return "requests-required" }

func (q resourceRequestsRule) Evaluate(resources []renderer.RenderedResource) ruleengine.Diagnostics {
	return checkResourceQuantities(resources, q.Name(), "requests")
}

// checkResourceQuantities reports containers whose resources.<section> map is
// missing the cpu or the memory entry.
func checkResourceQuantities(
	resources []renderer.RenderedResource,
	code, section string,
) ruleengine.Diagnostics {
	var diags ruleengine.Diagnostics
	for _, r := range resources {
		for _, c := range containersOf(r) {
			values, ok := c.resources[section].(map[string]any)
			if !ok {
				diags = append(diags, makeDiag(
					ruleengine.SeverityError,
					code,
					fmt.Sprintf("container %q declares no resource %s", c.name, section),
					r,
				))
				continue
			}
			for _, key := range []string{"cpu", "memory"} {
				if _, ok := values[key]; !ok {
					diags = append(diags, makeDiag(
						ruleengine.SeverityError,
						code,
						fmt.Sprintf("container %q has no %s %s", c.name, section, key),
						r,
					))
				}
			}
		}
	}
	return diags
}

// replicasRule reports replicated workloads scaled to zero. A rendered
// controller with no replicas starts no pods, which is almost always a
// mistake in the ComponentType template or the parameters feeding it.
type replicasRule struct{}

func (replicasRule) Name() string { return "replicas-positive" }

// replicatedKinds are the workload kinds that honour spec.replicas.
var replicatedKinds = map[string]bool{
	"Deployment":            true,
	"StatefulSet":           true,
	"ReplicaSet":            true,
	"ReplicationController": true,
}

func (r replicasRule) Evaluate(resources []renderer.RenderedResource) ruleengine.Diagnostics {
	var diags ruleengine.Diagnostics
	for _, res := range resources {
		kind, _ := res.Resource["kind"].(string)
		if !replicatedKinds[kind] {
			continue
		}
		spec, ok := res.Resource["spec"].(map[string]any)
		if !ok {
			continue
		}
		value, ok := spec["replicas"]
		if !ok {
			continue
		}
		count, ok := numericValue(value)
		if !ok || count >= 1 {
			continue
		}
		diags = append(diags, makeDiag(
			ruleengine.SeverityError,
			r.Name(),
			fmt.Sprintf("spec.replicas is %d: at least one replica is required", count),
			res,
		))
	}
	return diags
}

// numericValue converts a decoded YAML/JSON number to an int. Rendered
// manifests are decoded with encoding/json, so integers usually arrive as
// float64, but callers may also produce int or json.Number.
func numericValue(value any) (int, bool) {
	switch n := value.(type) {
	case int:
		return n, true
	case int32:
		return int(n), true
	case int64:
		return int(n), true
	case float32:
		return int(n), true
	case float64:
		return int(n), true
	case json.Number:
		i, err := n.Int64()
		if err != nil {
			return 0, false
		}
		return int(i), true
	}
	return 0, false
}

// hostNamespaceRule reports pods that share a namespace with the host.
type hostNamespaceRule struct{}

func (hostNamespaceRule) Name() string { return "no-host-namespaces" }

func (h hostNamespaceRule) Evaluate(resources []renderer.RenderedResource) ruleengine.Diagnostics {
	var diags ruleengine.Diagnostics
	for _, r := range resources {
		for _, spec := range podSpecsOf(r) {
			for _, field := range []string{"hostNetwork", "hostPID", "hostIPC"} {
				if enabled, ok := spec[field].(bool); ok && enabled {
					diags = append(diags, makeDiag(
						ruleengine.SeverityError,
						h.Name(),
						fmt.Sprintf("%s is enabled", field),
						r,
					))
				}
			}
		}
	}
	return diags
}

// privilegedRule reports containers that run as privileged.
type privilegedRule struct{}

func (privilegedRule) Name() string { return "no-privileged" }

func (p privilegedRule) Evaluate(resources []renderer.RenderedResource) ruleengine.Diagnostics {
	var diags ruleengine.Diagnostics
	for _, r := range resources {
		for _, c := range containersOf(r) {
			if c.privileged {
				diags = append(diags, makeDiag(
					ruleengine.SeverityError,
					p.Name(),
					fmt.Sprintf("container %q runs as privileged", c.name),
					r,
				))
			}
		}
	}
	return diags
}
