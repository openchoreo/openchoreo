// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package helm

import (
	"bytes"
	"errors"
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"gopkg.in/yaml.v3"
)

// portalOffOverrides install the control plane without the bundled Backstage
// portal. Unlike helmGuardOverrides they deliberately set no backstage.secretName,
// baseUrl or http.hostnames: a portal-less install must not have to supply the
// portal's own required values.
var portalOffOverrides = []string{
	"--set", "backstage.enabled=false",
	"--set", "openchoreoApi.http.hostnames[0]=api.example.com",
	"--set", "gateway.tls.hostname=example.com",
	"--set", "security.oidc.issuer=https://idp.example.com",
}

// portalOnOverrides install the control plane with the portal, clearing the
// placeholder-domain and required-value guards the portal itself adds.
var portalOnOverrides = append(append([]string{}, portalOffOverrides[2:]...),
	"--set", "backstage.secretName=backstage-secrets",
	"--set", "backstage.baseUrl=https://console.example.com",
	"--set", "backstage.http.hostnames[0]=console.example.com",
)

// renderControlPlane renders the control-plane chart and returns the manifests
// it produces. The test is skipped when helm is not installed, so it adds
// coverage where the binary is available without making the package untestable
// where it is not.
func renderControlPlane(t *testing.T, args ...string) string {
	t.Helper()

	rendered, err := tryRenderControlPlane(t, args...)
	if err != nil {
		t.Fatalf("failed to render %s: %v", controlPlaneChart, err)
	}
	return rendered
}

// tryRenderControlPlane renders the chart and hands back the failure instead of
// ending the test, for the cases that assert a render succeeds or fails.
func tryRenderControlPlane(t *testing.T, args ...string) (string, error) {
	t.Helper()

	helm, err := exec.LookPath("helm")
	if err != nil {
		t.Skip("helm is not installed; skipping the rendered chart check")
	}

	full := append([]string{
		"template", "openchoreo", controlPlaneChart,
		"--namespace", "openchoreo-control-plane",
		// Pinned so the render does not depend on a reachable cluster.
		"--kube-version", "1.31.0",
	}, args...)

	var stderr bytes.Buffer
	cmd := exec.CommandContext(t.Context(), helm, full...)
	cmd.Stderr = &stderr
	rendered, err := cmd.Output()
	if err != nil {
		return "", errors.New(err.Error() + "\n" + stderr.String())
	}
	return string(rendered), nil
}

// authzBootstrapNames renders the authz bootstrap hook ConfigMap and returns the
// names of every object it seeds, keyed by name. The hook applies the ConfigMap's
// manifests.yaml as a multi-document stream, so that payload — not the ConfigMap
// itself — is what an install actually gets.
func authzBootstrapNames(t *testing.T, args ...string) map[string]string {
	t.Helper()

	full := append(append([]string{}, args...),
		"--show-only", "templates/authz/bootstrap-configmap.yaml")
	rendered := renderControlPlane(t, full...)

	var configMap struct {
		Data struct {
			Manifests string `yaml:"manifests.yaml"`
		} `yaml:"data"`
	}
	if err := yaml.Unmarshal([]byte(rendered), &configMap); err != nil {
		t.Fatalf("failed to parse the rendered bootstrap ConfigMap: %v\n%s", err, rendered)
	}
	if configMap.Data.Manifests == "" {
		t.Fatalf("the rendered bootstrap ConfigMap carries no manifests.yaml:\n%s", rendered)
	}

	kindsByName := map[string]string{}
	decoder := yaml.NewDecoder(strings.NewReader(configMap.Data.Manifests))
	for {
		var doc struct {
			Kind     string `yaml:"kind"`
			Metadata struct {
				Name string `yaml:"name"`
			} `yaml:"metadata"`
		}
		err := decoder.Decode(&doc)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("failed to parse a bootstrap document: %v\n%s", err, configMap.Data.Manifests)
		}
		if doc.Metadata.Name == "" {
			continue
		}
		if existing, ok := kindsByName[doc.Metadata.Name]; ok {
			t.Errorf("bootstrap seeds %s/%s twice; the hook applies the stream in order, so the "+
				"last copy silently wins", existing, doc.Metadata.Name)
		}
		kindsByName[doc.Metadata.Name] = doc.Kind
	}
	return kindsByName
}

// TestChart_PortalDisabled_RendersNoPortalObjects pins that backstage.enabled=false
// leaves nothing of the portal behind. The portal's own templates are each gated,
// so this guards against a new one landing ungated.
func TestChart_PortalDisabled_RendersNoPortalObjects(t *testing.T) {
	rendered := renderControlPlane(t, portalOffOverrides...)

	if strings.Contains(rendered, "app.kubernetes.io/component: backstage") {
		t.Error("rendering with backstage.enabled=false still produced an object labelled " +
			"app.kubernetes.io/component: backstage")
	}
	// The event forwarder's default webhook endpoint names the portal Service, and
	// the portal's own objects are named after it, so a bare name match would be
	// noisy. Assert on the object names instead.
	for _, name := range []string{
		"name: backstage\n",
		"name: backstage-db\n",
		"name: backstage-ci-config\n",
		"name: backstage-csp-config\n",
	} {
		if strings.Contains(rendered, name) {
			t.Errorf("rendering with backstage.enabled=false still produced an object named %q",
				strings.TrimSuffix(strings.TrimPrefix(name, "name: "), "\n"))
		}
	}
}

// TestChart_PortalDisabled_NeedsNoPortalValues pins the validateHostnames
// short-circuit in _helpers.tpl: with the portal off, an install must not be
// asked for backstage.secretName, baseUrl or http.hostnames. Every e2e run
// depends on this, but only implicitly through helmGuardOverrides.
func TestChart_PortalDisabled_NeedsNoPortalValues(t *testing.T) {
	if _, err := tryRenderControlPlane(t, portalOffOverrides...); err != nil {
		t.Fatalf("a portal-less install should not require the portal's own values: %v", err)
	}
}

// TestAuthzBootstrap_CatalogReaderFollowsPortalFlag pins that the portal's
// catalog-reader role and binding are seeded only when the portal is installed.
// They grant read access to an OAuth client that a portal-less install never
// registers with the IdP.
func TestAuthzBootstrap_CatalogReaderFollowsPortalFlag(t *testing.T) {
	const (
		role    = "backstage-catalog-reader"
		binding = "backstage-catalog-reader-binding"
	)

	t.Run("portal enabled", func(t *testing.T) {
		seeded := authzBootstrapNames(t, portalOnOverrides...)
		if got := seeded[role]; got != "ClusterAuthzRole" {
			t.Errorf("want %s seeded as ClusterAuthzRole, got %q", role, got)
		}
		if got := seeded[binding]; got != "ClusterAuthzRoleBinding" {
			t.Errorf("want %s seeded as ClusterAuthzRoleBinding, got %q", binding, got)
		}
	})

	t.Run("portal disabled", func(t *testing.T) {
		seeded := authzBootstrapNames(t, portalOffOverrides...)
		for _, name := range []string{role, binding} {
			if kind, ok := seeded[name]; ok {
				t.Errorf("a portal-less install still seeds %s/%s", kind, name)
			}
		}
		// The rest of the bootstrap must survive, or this would pass for the
		// wrong reason.
		if _, ok := seeded["admin"]; !ok {
			t.Error("a portal-less install seeds no admin role; the bootstrap payload looks empty")
		}
	})
}

// TestAuthzBootstrap_CatalogReaderEntitlement pins which identity the catalog
// reader binding grants. The binding has to follow the client the catalog
// provider actually presents, and external IdPs identify that client through a
// different claim: Dex, for one, encodes the client id into sub.
func TestAuthzBootstrap_CatalogReaderEntitlement(t *testing.T) {
	entitlementOf := func(t *testing.T, args ...string) string {
		t.Helper()
		full := append(append([]string{}, args...),
			"--show-only", "templates/authz/bootstrap-configmap.yaml")
		return renderControlPlane(t, full...)
	}

	t.Run("defaults to the sign-in client", func(t *testing.T) {
		rendered := entitlementOf(t, portalOnOverrides...)
		if !strings.Contains(rendered, `value: "openchoreo-backstage-client"`) {
			t.Error("the catalog reader binding does not default to backstage.auth.clientId")
		}
	})

	t.Run("follows a dedicated service client", func(t *testing.T) {
		args := append(append([]string{}, portalOnOverrides...),
			"--set", "backstage.auth.serviceClientId=portal-catalog-provider")
		rendered := entitlementOf(t, args...)
		if !strings.Contains(rendered, `value: "portal-catalog-provider"`) {
			t.Error("the catalog reader binding ignores backstage.auth.serviceClientId, so a " +
				"deployment using a dedicated service client fails authorization closed")
		}
	})

	t.Run("honours an explicit entitlement", func(t *testing.T) {
		args := append(append([]string{}, portalOnOverrides...),
			"--set", "backstage.auth.authzEntitlement.claim=sub",
			"--set", "backstage.auth.authzEntitlement.value=encoded-client-id")
		rendered := entitlementOf(t, args...)
		if !strings.Contains(rendered, `claim: "sub"`) ||
			!strings.Contains(rendered, `value: "encoded-client-id"`) {
			t.Error("the catalog reader binding ignores backstage.auth.authzEntitlement")
		}
	})
}

// TestAPINetworkPolicy_PortalSelectorFollowsPortalFlag pins that the API
// server's NetworkPolicy stops allowing the portal in once the portal is gone.
func TestAPINetworkPolicy_PortalSelectorFollowsPortalFlag(t *testing.T) {
	render := func(t *testing.T, overrides []string) string {
		t.Helper()
		args := append(append([]string{}, overrides...),
			"--set", "openchoreoApi.networkPolicy.enabled=true",
			"--show-only", "templates/openchoreo-api/networkpolicy.yaml")
		return renderControlPlane(t, args...)
	}

	if got := render(t, portalOnOverrides); !strings.Contains(got, "component: backstage") {
		t.Error("with the portal installed, the API NetworkPolicy should still allow it in")
	}
	if got := render(t, portalOffOverrides); strings.Contains(got, "component: backstage") {
		t.Error("with the portal uninstalled, the API NetworkPolicy still carries its podSelector")
	}
}

// catalogReaderActions renders the authz bootstrap and returns the action list of
// the portal's catalog-reader role.
func catalogReaderActions(t *testing.T, args ...string) []string {
	t.Helper()

	full := append(append([]string{}, args...),
		"--show-only", "templates/authz/bootstrap-configmap.yaml")
	rendered := renderControlPlane(t, full...)

	var configMap struct {
		Data struct {
			Manifests string `yaml:"manifests.yaml"`
		} `yaml:"data"`
	}
	if err := yaml.Unmarshal([]byte(rendered), &configMap); err != nil {
		t.Fatalf("failed to parse the rendered bootstrap ConfigMap: %v", err)
	}

	decoder := yaml.NewDecoder(strings.NewReader(configMap.Data.Manifests))
	for {
		var doc struct {
			Kind     string `yaml:"kind"`
			Metadata struct {
				Name string `yaml:"name"`
			} `yaml:"metadata"`
			Spec struct {
				Actions []string `yaml:"actions"`
			} `yaml:"spec"`
		}
		err := decoder.Decode(&doc)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("failed to parse a bootstrap document: %v", err)
		}
		if doc.Kind == "ClusterAuthzRole" && doc.Metadata.Name == "backstage-catalog-reader" {
			return doc.Spec.Actions
		}
	}
	t.Fatal("the rendered bootstrap carries no backstage-catalog-reader ClusterAuthzRole")
	return nil
}

// valuesCatalogReaderActions reads the same role's action list straight out of
// values.yaml, which is where the chart defines it.
func valuesCatalogReaderActions(t *testing.T) []string {
	t.Helper()

	raw, err := os.ReadFile(helmValuesPath)
	if err != nil {
		t.Fatalf("failed to read %s: %v", helmValuesPath, err)
	}

	var values struct {
		OpenChoreoAPI struct {
			Config struct {
				Security struct {
					Authorization struct {
						Bootstrap struct {
							Roles []struct {
								Name    string   `yaml:"name"`
								Actions []string `yaml:"actions"`
							} `yaml:"roles"`
						} `yaml:"bootstrap"`
					} `yaml:"authorization"`
				} `yaml:"security"`
			} `yaml:"config"`
		} `yaml:"openchoreoApi"`
	}
	if err := yaml.Unmarshal(raw, &values); err != nil {
		t.Fatalf("failed to parse %s: %v", helmValuesPath, err)
	}

	for _, r := range values.OpenChoreoAPI.Config.Security.Authorization.Bootstrap.Roles {
		if r.Name == "backstage-catalog-reader" {
			return r.Actions
		}
	}
	t.Fatalf("%s defines no backstage-catalog-reader role", helmValuesPath)
	return nil
}

// TestAuthzBootstrap_CatalogReaderActionsMatchValues pins the role's action list
// to values.yaml.
//
// This guards a specific, already-observed failure: the role grants the portal's
// catalog provider read access to every resource kind, upstream extends it
// whenever a kind is added (releaseline, endpoint, releasebinding have all landed
// this way), and a copy kept anywhere else silently lags. A stale copy does not
// fail to render — catalog sync just starts failing closed on the kinds that went
// missing. Keeping the role in values.yaml and asserting the render matches makes
// that impossible rather than merely detectable.
func TestAuthzBootstrap_CatalogReaderActionsMatchValues(t *testing.T) {
	want := valuesCatalogReaderActions(t)
	if len(want) == 0 {
		t.Fatal("values.yaml lists no actions for backstage-catalog-reader")
	}

	got := catalogReaderActions(t, portalOnOverrides...)
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("rendered catalog-reader actions differ from %s (-values +rendered):\n%s",
			helmValuesPath, diff)
	}
}
