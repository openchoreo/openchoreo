// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package resourcepipeline

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/template"
)

// ErrNotYetResolved marks an endpoint whose address depends on state the pipeline has
// not observed yet. It clears once that state arrives, so a caller retries rather than
// reporting a defect in the ResourceType.
var ErrNotYetResolved = errors.New("not yet resolved")

// ResolvedEndpoint is a dialable endpoint resolved from a ResourceType endpoint
// declaration. HostFrom/PortFrom are carried through so a consumer can map the
// endpoint back to the outputs a workload binds to env vars.
//
// Host and Port are zero when the address is published only through a Secret, which
// is not read here.
type ResolvedEndpoint struct {
	Name     string
	Host     string
	Port     int32
	HostFrom string
	PortFrom string
}

// Resolved reports whether this endpoint has an address to dial.
func (e ResolvedEndpoint) Resolved() bool { return e.Host != "" && e.Port != 0 }

// ResolveEndpoints resolves every endpoint declared on the ResourceType. Outputs must
// already be resolved. An endpoint's address comes from its HostFrom/PortFrom outputs,
// or from the inline Host/Port expressions.
//
// An address held only in a Secret is not an error: the endpoint is returned with no
// address, since the reference is a valid declaration whose value is not read here.
// Callers check Resolved before dialing.
//
// Per-endpoint failures are joined and the successfully resolved subset is still
// returned, matching ResolveOutputs' partial-failure contract.
func (p *Pipeline) ResolveEndpoints(
	ctx context.Context,
	input *RenderInput,
	observed map[string]map[string]any,
	outputs []ResolvedOutput,
) ([]ResolvedEndpoint, error) {
	ctx, cancel := template.WithRenderTimeout(ctx, p.renderTimeout)
	defer cancel()

	if err := validateInput(input); err != nil {
		return nil, err
	}

	spec := resourceTypeSpec(input)
	if len(spec.Endpoints) == 0 {
		return nil, nil
	}

	byName := make(map[string]*ResolvedOutput, len(outputs))
	for i := range outputs {
		byName[outputs[i].Name] = &outputs[i]
	}

	var celContext map[string]any
	if needsInlineContext(spec.Endpoints) {
		base, err := buildBaseContext(input)
		if err != nil {
			return nil, err
		}
		celContext = withApplied(base, observed)
	}

	// One env var cannot carry a redirected address for two endpoints, so a shared
	// port output is rejected rather than resolved arbitrarily.
	portOwner := make(map[string]string, len(spec.Endpoints))

	resolved := make([]ResolvedEndpoint, 0, len(spec.Endpoints))
	var errs []error
	for i := range spec.Endpoints {
		ep := &spec.Endpoints[i]
		// Only a named port output can clash. An inline port has no output name, so
		// keying on the empty string would collapse every inline-port endpoint onto one
		// key and reject the second — the shape ResourceType validation accepts.
		if ep.PortFrom != "" {
			if owner, taken := portOwner[ep.PortFrom]; taken {
				errs = append(errs, fmt.Errorf("endpoint %q: port output %q is already used by endpoint %q",
					ep.Name, ep.PortFrom, owner))
				continue
			}
		}
		re, err := p.resolveEndpoint(ctx, ep, byName, celContext)
		if err != nil {
			errs = append(errs, fmt.Errorf("endpoint %q: %w", ep.Name, err))
			continue
		}
		if ep.PortFrom != "" {
			portOwner[ep.PortFrom] = ep.Name
		}
		resolved = append(resolved, re)
	}

	return resolved, errors.Join(errs...)
}

// needsInlineContext reports whether any endpoint sets Host or Port inline, so the
// CEL context is only built when something actually needs it.
func needsInlineContext(endpoints []v1alpha1.ResourceTypeEndpoint) bool {
	for i := range endpoints {
		if endpoints[i].Host != "" || endpoints[i].Port != "" {
			return true
		}
	}
	return false
}

func (p *Pipeline) resolveEndpoint(
	ctx context.Context,
	ep *v1alpha1.ResourceTypeEndpoint,
	outputs map[string]*ResolvedOutput,
	celContext map[string]any,
) (ResolvedEndpoint, error) {
	re := ResolvedEndpoint{Name: ep.Name, HostFrom: ep.HostFrom, PortFrom: ep.PortFrom}

	// A named output must exist: it is what maps this endpoint to a consumer's
	// bindings, whether or not the dial address comes from it.
	if ep.HostFrom != "" {
		if _, ok := outputs[ep.HostFrom]; !ok {
			return re, fmt.Errorf("hostFrom references undeclared output %q", ep.HostFrom)
		}
	}
	if ep.PortFrom != "" {
		if _, ok := outputs[ep.PortFrom]; !ok {
			return re, fmt.Errorf("portFrom references undeclared output %q", ep.PortFrom)
		}
	}

	// An address published only through a Secret or ConfigMap reference is not readable
	// here, so the endpoint is recorded without one rather than failing the whole
	// resource. Its mapping to a consumer's bindings still holds.
	if addressOnlyInRef(ep, outputs) {
		return re, nil
	}

	host, err := p.resolveEndpointHost(ctx, ep, outputs, celContext)
	if err != nil {
		return re, err
	}
	port, err := p.resolveEndpointPort(ctx, ep, outputs, celContext)
	if err != nil {
		return re, err
	}
	re.Host, re.Port = host, port
	return re, nil
}

// addressOnlyInRef reports whether either half of this endpoint's address comes from a
// reference-backed output, with no inline expression supplying it instead. A ConfigMap
// reference counts alongside a Secret one: neither value is read here, so both leave the
// endpoint without an address to dial.
func addressOnlyInRef(ep *v1alpha1.ResourceTypeEndpoint, outputs map[string]*ResolvedOutput) bool {
	hostInRef := ep.Host == "" && ep.HostFrom != "" && isRefBacked(outputs[ep.HostFrom])
	portInRef := ep.Port == "" && ep.PortFrom != "" && isRefBacked(outputs[ep.PortFrom])
	return hostInRef || portInRef
}

// isRefBacked reports whether an output's value lives on the data plane behind a
// reference, and so is not available to the control plane.
func isRefBacked(o *ResolvedOutput) bool {
	return o.SecretKeyRef != nil || o.ConfigMapKeyRef != nil
}

// resolveEndpointHost prefers the inline Host expression and falls back to the
// HostFrom output, which must then be a plain value.
func (p *Pipeline) resolveEndpointHost(
	ctx context.Context,
	ep *v1alpha1.ResourceTypeEndpoint,
	outputs map[string]*ResolvedOutput,
	celContext map[string]any,
) (string, error) {
	if ep.Host != "" {
		host, err := p.renderStringValue(ctx, ep.Host, celContext)
		if err != nil {
			return "", fmt.Errorf("host: %w", err)
		}
		if host == "" {
			return "", fmt.Errorf("host rendered empty: %w", ErrNotYetResolved)
		}
		return host, nil
	}
	if ep.HostFrom == "" {
		return "", fmt.Errorf("no host: set hostFrom or host")
	}
	o := outputs[ep.HostFrom]
	if o.Value == "" {
		return "", fmt.Errorf("output %q rendered an empty host: %w", ep.HostFrom, ErrNotYetResolved)
	}
	return o.Value, nil
}

// resolveEndpointPort prefers the inline Port expression and falls back to the
// PortFrom output, which must then be a plain value.
func (p *Pipeline) resolveEndpointPort(
	ctx context.Context,
	ep *v1alpha1.ResourceTypeEndpoint,
	outputs map[string]*ResolvedOutput,
	celContext map[string]any,
) (int32, error) {
	if ep.Port != "" {
		rendered, err := p.renderStringValue(ctx, ep.Port, celContext)
		if err != nil {
			return 0, fmt.Errorf("port: %w", err)
		}
		if rendered == "" {
			return 0, fmt.Errorf("port rendered empty: %w", ErrNotYetResolved)
		}
		n, err := parsePort(rendered)
		if err != nil {
			return 0, fmt.Errorf("port: %w", err)
		}
		return n, nil
	}
	if ep.PortFrom == "" {
		return 0, fmt.Errorf("no port: set portFrom or port")
	}
	o := outputs[ep.PortFrom]
	if o.Value == "" {
		return 0, fmt.Errorf("output %q rendered an empty port: %w", ep.PortFrom, ErrNotYetResolved)
	}
	n, err := parsePort(o.Value)
	if err != nil {
		return 0, fmt.Errorf("output %q: %w", ep.PortFrom, err)
	}
	return n, nil
}

func parsePort(s string) (int32, error) {
	// ParseInt with an explicit bit size keeps the conversion provably in range.
	n, err := strconv.ParseInt(s, 10, 32)
	if err != nil {
		return 0, fmt.Errorf("port %q is not numeric", s)
	}
	if n < 1 || n > 65535 {
		return 0, fmt.Errorf("port %d is out of range", n)
	}
	return int32(n), nil
}
