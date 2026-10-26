// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package endpointpipeline

import (
	"context"
	"errors"
	"fmt"
	"math"
	"slices"
	"strconv"

	"github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/pipeline/component/renderer"
	"github.com/openchoreo/openchoreo/internal/template"
)

// URLSchemes are the values an EndpointURLTemplate's scheme may resolve to.
var URLSchemes = []string{"http", "https", "ws", "wss", "grpc", "grpcs", "tcp", "tls"}

// SecuritySchemes are the values an EndpointSecurityTemplate's scheme may
// resolve to.
var SecuritySchemes = []string{
	string(v1alpha1.EndpointSecuritySchemeNone),
	string(v1alpha1.EndpointSecuritySchemeAPIKey),
	string(v1alpha1.EndpointSecuritySchemeOAuth2),
	string(v1alpha1.EndpointSecuritySchemeBasicAuth),
}

// APIKeyLocations are the values an EndpointAPIKeyTemplate's in may resolve to.
var APIKeyLocations = []string{
	string(v1alpha1.EndpointAPIKeyInHeader),
	string(v1alpha1.EndpointAPIKeyInQuery),
}

// ResolveURLs evaluates EndpointTypeSpec.URLs against the same CEL context
// Render uses. A visibility's URL is resolved when the type declares it, the
// Endpoint requests that visibility (project is implicit and always counts),
// and its IncludeWhen passes. URLs are computed from the render inputs alone;
// nothing is read back from the data plane.
//
// It returns nil when no URL resolved. Per-visibility errors are collected and
// returned as a joined error; the URLs that did resolve are still returned so
// the caller can report a partial result.
func (p *Pipeline) ResolveURLs(ctx context.Context, input *RenderInput) (*v1alpha1.EndpointReleaseBindingURLs, error) {
	ctx, cancel := template.WithRenderTimeout(ctx, p.renderTimeout)
	defer cancel()

	if err := validateInput(input); err != nil {
		return nil, err
	}
	declared := input.EndpointTypeSpec.URLs
	if declared == nil {
		return nil, nil
	}

	celContext, err := buildBaseContext(input)
	if err != nil {
		return nil, err
	}

	resolved := &v1alpha1.EndpointReleaseBindingURLs{}
	type entry struct {
		visibility v1alpha1.EndpointVisibility
		tmpl       *v1alpha1.EndpointURLTemplate
		out        **v1alpha1.ResolvedEndpointURL
	}
	entries := []entry{
		{v1alpha1.EndpointVisibilityProject, declared.Project, &resolved.Project},
		{v1alpha1.EndpointVisibilityNamespace, declared.Namespace, &resolved.Namespace},
		{v1alpha1.EndpointVisibilityInternal, declared.Internal, &resolved.Internal},
		{v1alpha1.EndpointVisibilityExternal, declared.External, &resolved.External},
	}

	var errs []error
	found := false
	for _, e := range entries {
		if e.tmpl == nil || !requested(e.visibility, input.Visibility) {
			continue
		}
		url, err := p.resolveURL(ctx, e.tmpl, celContext)
		if err != nil {
			errs = append(errs, fmt.Errorf("url %q: %w", e.visibility, err))
			if template.IsRenderAborted(err) {
				break
			}
			continue
		}
		if url != nil {
			*e.out = url
			found = true
		}
	}

	if !found {
		resolved = nil
	}
	return resolved, errors.Join(errs...)
}

// requested reports whether the Endpoint asks for visibility v. Project
// visibility is implicit for every Endpoint and cannot be requested, so it
// always counts.
func requested(v v1alpha1.EndpointVisibility, visibility []v1alpha1.EndpointVisibility) bool {
	return v == v1alpha1.EndpointVisibilityProject || slices.Contains(visibility, v)
}

// resolveURL evaluates one URL template. It returns nil without error when
// IncludeWhen excludes the URL.
func (p *Pipeline) resolveURL(
	ctx context.Context,
	tmpl *v1alpha1.EndpointURLTemplate,
	celContext map[string]any,
) (*v1alpha1.ResolvedEndpointURL, error) {
	include, err := renderer.ShouldInclude(ctx, p.templateEngine, tmpl.IncludeWhen, celContext)
	if err != nil {
		return nil, fmt.Errorf("evaluate includeWhen: %w", err)
	}
	if !include {
		return nil, nil
	}

	url := &v1alpha1.ResolvedEndpointURL{}
	if url.Scheme, err = p.renderEnum(ctx, "scheme", tmpl.Scheme, URLSchemes, celContext); err != nil {
		return nil, err
	}
	if url.Host, err = p.renderString(ctx, "host", tmpl.Host, celContext); err != nil {
		return nil, err
	}
	if url.Host == "" {
		return nil, fmt.Errorf("host resolved to an empty string")
	}
	if tmpl.Port != "" {
		if url.Port, err = p.renderPort(ctx, tmpl.Port, celContext); err != nil {
			return nil, err
		}
	}
	if tmpl.Path != "" {
		if url.Path, err = p.renderString(ctx, "path", tmpl.Path, celContext); err != nil {
			return nil, err
		}
	}
	if tmpl.Security != nil {
		if url.Security, err = p.renderSecurity(ctx, tmpl.Security, celContext); err != nil {
			return nil, err
		}
	}
	return url, nil
}

func (p *Pipeline) renderSecurity(
	ctx context.Context,
	tmpl *v1alpha1.EndpointSecurityTemplate,
	celContext map[string]any,
) (*v1alpha1.EndpointSecurity, error) {
	scheme, err := p.renderEnum(ctx, "security.scheme", tmpl.Scheme, SecuritySchemes, celContext)
	if err != nil {
		return nil, err
	}
	security := &v1alpha1.EndpointSecurity{Scheme: v1alpha1.EndpointSecurityScheme(scheme)}

	// apiKey is read only when the scheme resolves to apiKey. A templated scheme
	// can switch API-key auth off (e.g. ${parameters.requireApiKey ? "apiKey" :
	// "none"}), and a template cannot remove the apiKey block with it, so the
	// block is ignored rather than rejected then.
	isAPIKey := security.Scheme == v1alpha1.EndpointSecuritySchemeAPIKey
	switch {
	case isAPIKey && tmpl.APIKey == nil:
		return nil, fmt.Errorf("security.apiKey is required when security.scheme is %q", scheme)
	case isAPIKey:
		in, err := p.renderEnum(ctx, "security.apiKey.in", tmpl.APIKey.In, APIKeyLocations, celContext)
		if err != nil {
			return nil, err
		}
		name, err := p.renderString(ctx, "security.apiKey.name", tmpl.APIKey.Name, celContext)
		if err != nil {
			return nil, err
		}
		if name == "" {
			return nil, fmt.Errorf("security.apiKey.name resolved to an empty string")
		}
		security.APIKey = &v1alpha1.EndpointAPIKey{In: v1alpha1.EndpointAPIKeyLocation(in), Name: name}
	}
	return security, nil
}

// renderString evaluates a literal or ${...} template that must resolve to a
// string.
func (p *Pipeline) renderString(ctx context.Context, field, expr string, celContext map[string]any) (string, error) {
	result, err := p.templateEngine.Render(ctx, expr, celContext)
	if err != nil {
		return "", fmt.Errorf("%s: %w", field, err)
	}
	s, ok := result.(string)
	if !ok {
		return "", fmt.Errorf("%s must resolve to a string, got %T", field, result)
	}
	return s, nil
}

// renderEnum evaluates a string template and checks the result is one of
// allowed.
func (p *Pipeline) renderEnum(ctx context.Context, field, expr string, allowed []string, celContext map[string]any) (string, error) {
	s, err := p.renderString(ctx, field, expr, celContext)
	if err != nil {
		return "", err
	}
	if !slices.Contains(allowed, s) {
		return "", fmt.Errorf("%s resolved to %q, must be one of %v", field, s, allowed)
	}
	return s, nil
}

// renderPort evaluates a port template. A standalone expression yields the
// engine's native number type; an interpolated one yields a string.
func (p *Pipeline) renderPort(ctx context.Context, expr string, celContext map[string]any) (int32, error) {
	result, err := p.templateEngine.Render(ctx, expr, celContext)
	if err != nil {
		return 0, fmt.Errorf("port: %w", err)
	}
	var port float64
	switch v := result.(type) {
	case int64:
		port = float64(v)
	case float64:
		port = v
	case string:
		n, err := strconv.Atoi(v)
		if err != nil {
			return 0, fmt.Errorf("port resolved to %q, which is not an integer", v)
		}
		port = float64(n)
	default:
		return 0, fmt.Errorf("port must resolve to an integer, got %T", result)
	}
	if port != math.Trunc(port) || port < 1 || port > 65535 {
		return 0, fmt.Errorf("port resolved to %v, must be an integer between 1 and 65535", port)
	}
	return int32(port), nil
}
