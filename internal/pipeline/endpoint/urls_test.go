// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package endpointpipeline

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openchoreo/openchoreo/api/v1alpha1"
)

func TestResolveURLs_SampleResolvesExternalURL(t *testing.T) {
	urls, err := NewPipeline().ResolveURLs(context.Background(), sampleInput(t))
	require.NoError(t, err)
	assert.Equal(t, &v1alpha1.EndpointReleaseBindingURLs{
		External: &v1alpha1.ResolvedEndpointURL{
			EndpointURL: v1alpha1.EndpointURL{
				Scheme: "http", Host: "dev-greeter-api.openchoreoapis.localhost", Port: 80, Path: "/greeter",
			},
			Security: &v1alpha1.EndpointSecurity{Scheme: v1alpha1.EndpointSecuritySchemeNone},
		},
	}, urls)
}

// urlsInput returns the sample input with the type's URLs replaced.
func urlsInput(t *testing.T, urls *v1alpha1.EndpointTypeURLs, visibility ...v1alpha1.EndpointVisibility) *RenderInput {
	input := sampleInput(t)
	input.EndpointTypeSpec = &v1alpha1.EndpointTypeSpec{URLs: urls}
	input.Visibility = visibility
	return input
}

func literalURL(host string) *v1alpha1.EndpointURLTemplate {
	return &v1alpha1.EndpointURLTemplate{Scheme: "https", Host: host}
}

func TestResolveURLs_VisibilityGate(t *testing.T) {
	declared := &v1alpha1.EndpointTypeURLs{
		Project:   literalURL("project.example.com"),
		Namespace: literalURL("namespace.example.com"),
		Internal:  literalURL("internal.example.com"),
		External:  literalURL("external.example.com"),
	}

	urls, err := NewPipeline().ResolveURLs(context.Background(),
		urlsInput(t, declared, v1alpha1.EndpointVisibilityInternal))
	require.NoError(t, err)
	require.NotNil(t, urls)
	assert.NotNil(t, urls.Project, "project visibility is implicit, so it is always resolved")
	assert.Nil(t, urls.Namespace, "namespace was not requested")
	assert.Equal(t, "internal.example.com", urls.Internal.Host)
	assert.Nil(t, urls.External, "external was not requested")
}

func TestResolveURLs_OnlyDeclaredVisibilities(t *testing.T) {
	declared := &v1alpha1.EndpointTypeURLs{Internal: literalURL("internal.example.com")}

	urls, err := NewPipeline().ResolveURLs(context.Background(),
		urlsInput(t, declared, v1alpha1.EndpointVisibilityInternal, v1alpha1.EndpointVisibilityExternal))
	require.NoError(t, err)
	assert.Equal(t, &v1alpha1.EndpointReleaseBindingURLs{
		Internal: &v1alpha1.ResolvedEndpointURL{EndpointURL: v1alpha1.EndpointURL{Scheme: "https", Host: "internal.example.com"}},
	}, urls, "external is requested but the type declares no URL for it")
}

func TestResolveURLs_IncludeWhenIsANDedWithVisibility(t *testing.T) {
	external := literalURL("external.example.com")
	external.IncludeWhen = "${gateway.?ingress.?external.?https.hasValue()}" // fixture has http only

	urls, err := NewPipeline().ResolveURLs(context.Background(), urlsInput(t,
		&v1alpha1.EndpointTypeURLs{External: external}, v1alpha1.EndpointVisibilityExternal))
	require.NoError(t, err)
	assert.Nil(t, urls, "nothing resolved, so status.urls is omitted")
}

func TestResolveURLs_NoURLsDeclared(t *testing.T) {
	urls, err := NewPipeline().ResolveURLs(context.Background(), urlsInput(t, nil, v1alpha1.EndpointVisibilityExternal))
	require.NoError(t, err)
	assert.Nil(t, urls)
}

func TestResolveURLs_TemplatesEveryField(t *testing.T) {
	input := urlsInput(t, &v1alpha1.EndpointTypeURLs{External: &v1alpha1.EndpointURLTemplate{
		Scheme: `${"external" in visibility ? "https" : "http"}`,
		Host:   "${metadata.endpointName}.${gateway.ingress.external.http.host}",
		Port:   "${int(gateway.ingress.external.http.port) + 8363}",
		Path:   "${parameters.pathPrefix}/v1",
		Security: &v1alpha1.EndpointSecurityTemplate{
			Scheme: `${parameters.pathPrefix == "/greeter" ? "apiKey" : "none"}`,
			APIKey: &v1alpha1.EndpointAPIKeyTemplate{In: "header", Name: "x-${metadata.endpointName}-key"},
		},
	}}, v1alpha1.EndpointVisibilityExternal)

	urls, err := NewPipeline().ResolveURLs(context.Background(), input)
	require.NoError(t, err)
	assert.Equal(t, &v1alpha1.ResolvedEndpointURL{
		EndpointURL: v1alpha1.EndpointURL{
			Scheme: "https", Host: "greeter-api.openchoreoapis.localhost", Port: 8443, Path: "/greeter/v1",
		},
		Security: &v1alpha1.EndpointSecurity{
			Scheme: v1alpha1.EndpointSecuritySchemeAPIKey,
			APIKey: &v1alpha1.EndpointAPIKey{In: v1alpha1.EndpointAPIKeyInHeader, Name: "x-greeter-api-key"},
		},
	}, urls.External)
}

func TestResolveURLs_PortForms(t *testing.T) {
	for _, tc := range []struct {
		port    string
		want    int32
		wantErr string
	}{
		{port: "8443", want: 8443},
		{port: "${gateway.ingress.external.http.port}", want: 80},
		{port: "${string(gateway.ingress.external.http.port)}", want: 80},
		{port: "0", wantErr: "between 1 and 65535"},
		{port: "70000", wantErr: "between 1 and 65535"},
		{port: "http", wantErr: "not an integer"},
		{port: "${visibility}", wantErr: "must resolve to an integer"},
	} {
		t.Run(tc.port, func(t *testing.T) {
			tmpl := literalURL("example.com")
			tmpl.Port = tc.port
			urls, err := NewPipeline().ResolveURLs(context.Background(), urlsInput(t,
				&v1alpha1.EndpointTypeURLs{External: tmpl}, v1alpha1.EndpointVisibilityExternal))
			if tc.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, urls.External.Port)
		})
	}
}

func TestResolveURLs_RejectsInvalidValues(t *testing.T) {
	for name, tc := range map[string]struct {
		tmpl    *v1alpha1.EndpointURLTemplate
		wantErr string
	}{
		"unknown scheme": {
			tmpl:    &v1alpha1.EndpointURLTemplate{Scheme: "ftp", Host: "example.com"},
			wantErr: `scheme resolved to "ftp"`,
		},
		"empty host": {
			tmpl:    &v1alpha1.EndpointURLTemplate{Scheme: "https", Host: `${""}`},
			wantErr: "host resolved to an empty string",
		},
		"non-string host": {
			tmpl:    &v1alpha1.EndpointURLTemplate{Scheme: "https", Host: "${backends.size()}"},
			wantErr: "host must resolve to a string",
		},
		"unknown security scheme": {
			tmpl: &v1alpha1.EndpointURLTemplate{Scheme: "https", Host: "example.com",
				Security: &v1alpha1.EndpointSecurityTemplate{Scheme: "jwt"}},
			wantErr: `security.scheme resolved to "jwt"`,
		},
		"apiKey without apiKey block": {
			tmpl: &v1alpha1.EndpointURLTemplate{Scheme: "https", Host: "example.com",
				Security: &v1alpha1.EndpointSecurityTemplate{Scheme: "apiKey"}},
			wantErr: "security.apiKey is required",
		},
		"unknown apiKey location": {
			tmpl: &v1alpha1.EndpointURLTemplate{Scheme: "https", Host: "example.com",
				Security: &v1alpha1.EndpointSecurityTemplate{Scheme: "apiKey",
					APIKey: &v1alpha1.EndpointAPIKeyTemplate{In: "cookie", Name: "k"}}},
			wantErr: `security.apiKey.in resolved to "cookie"`,
		},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := NewPipeline().ResolveURLs(context.Background(), urlsInput(t,
				&v1alpha1.EndpointTypeURLs{External: tc.tmpl}, v1alpha1.EndpointVisibilityExternal))
			require.Error(t, err)
			assert.Contains(t, err.Error(), `url "external"`)
			assert.Contains(t, err.Error(), tc.wantErr)
		})
	}
}

func TestResolveURLs_TemplatedSchemeCanSwitchAPIKeyOff(t *testing.T) {
	tmpl := func(require bool) *v1alpha1.EndpointTypeURLs {
		scheme := `${"external" in visibility ? "apiKey" : "none"}`
		if !require {
			scheme = `${"internal" in visibility ? "apiKey" : "none"}`
		}
		return &v1alpha1.EndpointTypeURLs{External: &v1alpha1.EndpointURLTemplate{
			Scheme: "https", Host: "example.com",
			Security: &v1alpha1.EndpointSecurityTemplate{
				Scheme: scheme,
				APIKey: &v1alpha1.EndpointAPIKeyTemplate{In: "header", Name: "api-key"},
			},
		}}
	}

	on, err := NewPipeline().ResolveURLs(context.Background(), urlsInput(t, tmpl(true), v1alpha1.EndpointVisibilityExternal))
	require.NoError(t, err)
	assert.Equal(t, &v1alpha1.EndpointSecurity{
		Scheme: v1alpha1.EndpointSecuritySchemeAPIKey,
		APIKey: &v1alpha1.EndpointAPIKey{In: v1alpha1.EndpointAPIKeyInHeader, Name: "api-key"},
	}, on.External.Security)

	off, err := NewPipeline().ResolveURLs(context.Background(), urlsInput(t, tmpl(false), v1alpha1.EndpointVisibilityExternal))
	require.NoError(t, err, "the apiKey block is ignored, not rejected, when the scheme resolves to none")
	assert.Equal(t, &v1alpha1.EndpointSecurity{Scheme: v1alpha1.EndpointSecuritySchemeNone}, off.External.Security)
}

func TestResolveURLs_ReturnsResolvedURLsAlongsideErrors(t *testing.T) {
	urls, err := NewPipeline().ResolveURLs(context.Background(), urlsInput(t, &v1alpha1.EndpointTypeURLs{
		Internal: literalURL("internal.example.com"),
		External: &v1alpha1.EndpointURLTemplate{Scheme: "https", Host: "${parameters.missing}"},
	}, v1alpha1.EndpointVisibilityInternal, v1alpha1.EndpointVisibilityExternal))

	require.Error(t, err)
	assert.Contains(t, err.Error(), `url "external"`)
	require.NotNil(t, urls)
	assert.Equal(t, "internal.example.com", urls.Internal.Host)
	assert.Nil(t, urls.External)
}

func TestResolveURLs_InputValidation(t *testing.T) {
	_, err := NewPipeline().ResolveURLs(context.Background(), nil)
	require.Error(t, err)
}
