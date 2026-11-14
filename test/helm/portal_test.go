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

const (
	catalogReaderRole    = "backstage-catalog-reader"
	catalogReaderBinding = "backstage-catalog-reader-binding"
)

// The portal-off and portal-on fixtures are the ones this package already has:
// helmGuardOverrides (values_test.go) clears the chart's placeholder-domain
// guards with backstage.enabled=false and nothing portal-specific, which is
// exactly a portal-less install; backstageGuardOverrides (backstage_test.go)
// supplies the portal's own required values.

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

// authzBootstrapDoc is one document from the authz bootstrap payload: the hook
// applies manifests.yaml as a multi-document stream, so that payload — not the
// ConfigMap wrapping it — is what an install actually gets.
type authzBootstrapDoc struct {
	Kind     string `yaml:"kind"`
	Metadata struct {
		Name string `yaml:"name"`
	} `yaml:"metadata"`
	Spec struct {
		Actions     []string `yaml:"actions"`
		Entitlement struct {
			Claim string `yaml:"claim"`
			Value string `yaml:"value"`
		} `yaml:"entitlement"`
	} `yaml:"spec"`
}

// authzBootstrapDocs renders the authz bootstrap hook ConfigMap and returns the
// objects it seeds.
func authzBootstrapDocs(t *testing.T, args ...string) []authzBootstrapDoc {
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
	return decodeAuthzBootstrap(t, configMap.Data.Manifests)
}

// authzBootstrapRemovals renders the hook ConfigMap and returns the objects its
// removals.yaml tells the hook to delete, or nothing when the key is absent.
func authzBootstrapRemovals(t *testing.T, args ...string) []authzBootstrapDoc {
	t.Helper()

	full := append(append([]string{}, args...),
		"--show-only", "templates/authz/bootstrap-configmap.yaml")
	rendered := renderControlPlane(t, full...)

	var configMap struct {
		Data struct {
			Removals string `yaml:"removals.yaml"`
		} `yaml:"data"`
	}
	if err := yaml.Unmarshal([]byte(rendered), &configMap); err != nil {
		t.Fatalf("failed to parse the rendered bootstrap ConfigMap: %v\n%s", err, rendered)
	}
	if configMap.Data.Removals == "" {
		return nil
	}
	return decodeAuthzBootstrap(t, configMap.Data.Removals)
}

// decodeAuthzBootstrap parses one of the hook's multi-document payloads.
func decodeAuthzBootstrap(t *testing.T, payload string) []authzBootstrapDoc {
	t.Helper()

	var docs []authzBootstrapDoc
	decoder := yaml.NewDecoder(strings.NewReader(payload))
	for {
		var doc authzBootstrapDoc
		err := decoder.Decode(&doc)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("failed to parse a bootstrap document: %v\n%s", err, payload)
		}
		if doc.Metadata.Name != "" {
			docs = append(docs, doc)
		}
	}
	return docs
}

// authzBootstrapNames maps each seeded object's name to its kind, and fails if a
// name is seeded twice: the hook applies the stream in order, so a duplicate
// means the last copy silently wins.
func authzBootstrapNames(t *testing.T, args ...string) map[string]string {
	t.Helper()

	kindsByName := map[string]string{}
	for _, doc := range authzBootstrapDocs(t, args...) {
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
	rendered := renderControlPlane(t, helmGuardOverrides...)

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
	if _, err := tryRenderControlPlane(t, helmGuardOverrides...); err != nil {
		t.Fatalf("a portal-less install should not require the portal's own values: %v", err)
	}
}

// TestAuthzBootstrap_CatalogReaderFollowsPortalFlag pins which half of the
// catalog-reader grant follows the portal flag: the binding, which points the
// bundled portal's OAuth client at the role, and not the role itself. The role
// grants nothing unbound and is what a portal you run yourself binds to, so
// dropping it would make bringing your own portal harder, not cleaner.
func TestAuthzBootstrap_CatalogReaderFollowsPortalFlag(t *testing.T) {
	t.Run("portal enabled", func(t *testing.T) {
		seeded := authzBootstrapNames(t, backstageGuardOverrides...)
		if got := seeded[catalogReaderRole]; got != "ClusterAuthzRole" {
			t.Errorf("want %s seeded as ClusterAuthzRole, got %q", catalogReaderRole, got)
		}
		if got := seeded[catalogReaderBinding]; got != "ClusterAuthzRoleBinding" {
			t.Errorf("want %s seeded as ClusterAuthzRoleBinding, got %q", catalogReaderBinding, got)
		}
	})

	t.Run("portal disabled", func(t *testing.T) {
		seeded := authzBootstrapNames(t, helmGuardOverrides...)
		if got := seeded[catalogReaderRole]; got != "ClusterAuthzRole" {
			t.Errorf("a portal-less install does not seed %s (%q); a portal of your own would "+
				"have to redeclare its whole action list", catalogReaderRole, got)
		}
		if kind, ok := seeded[catalogReaderBinding]; ok {
			t.Errorf("a portal-less install still seeds %s/%s, granting read access to an OAuth "+
				"client it never registers", kind, catalogReaderBinding)
		}
		// The rest of the bootstrap must survive, or this would pass for the
		// wrong reason.
		if _, ok := seeded["admin"]; !ok {
			t.Error("a portal-less install seeds no admin role; the bootstrap payload looks empty")
		}
	})
}

// TestAuthzBootstrap_PortalBindingIsPruned pins that disabling the portal does
// not just stop seeding its binding but removes one an earlier revision seeded.
// The hook applies its payload with plain kubectl apply, which cannot delete,
// so without the removal stream the binding survives in the cluster and keeps
// granting cluster-wide read to a client whose secret is a published default.
func TestAuthzBootstrap_PortalBindingIsPruned(t *testing.T) {
	// The hook only deletes what it is given, and it must be given the binding
	// alone: the role is deliberately kept.
	t.Run("portal disabled", func(t *testing.T) {
		removals := authzBootstrapRemovals(t, helmGuardOverrides...)
		var names []string
		for _, doc := range removals {
			names = append(names, doc.Kind+"/"+doc.Metadata.Name)
		}
		want := []string{"ClusterAuthzRoleBinding/" + catalogReaderBinding}
		if diff := cmp.Diff(want, names); diff != "" {
			t.Errorf("unexpected removal stream (-want +got):\n%s", diff)
		}

		// The delete needs a container to run in and the permission to do it.
		job := renderControlPlane(t, append(append([]string{}, helmGuardOverrides...),
			"--show-only", "templates/authz/bootstrap-job.yaml")...)
		if !strings.Contains(job, "/bootstrap/removals.yaml") {
			t.Error("the bootstrap Job never reads removals.yaml, so nothing applies the removals")
		}
		rbac := renderControlPlane(t, append(append([]string{}, helmGuardOverrides...),
			"--show-only", "templates/authz/bootstrap-rbac.yaml")...)
		if !strings.Contains(rbac, "- delete") {
			t.Error("the bootstrap ServiceAccount cannot delete, so the prune fails the whole hook")
		}
	})

	// Nothing to remove, and no permission to remove anything.
	t.Run("portal enabled", func(t *testing.T) {
		if removals := authzBootstrapRemovals(t, backstageGuardOverrides...); removals != nil {
			t.Errorf("an install with the portal still carries a removal stream: %+v", removals)
		}
		rbac := renderControlPlane(t, append(append([]string{}, backstageGuardOverrides...),
			"--show-only", "templates/authz/bootstrap-rbac.yaml")...)
		if strings.Contains(rbac, "- delete") {
			t.Error("the bootstrap ServiceAccount may delete authz objects with nothing to prune")
		}
	})

	// A deployment that declares the binding itself owns it - it is how an
	// external portal rebinds the role - so the chart must not delete it.
	t.Run("declared by the deployment", func(t *testing.T) {
		args := append(append([]string{}, helmGuardOverrides...),
			"--set", "openchoreoApi.config.security.authorization.bootstrap.mappings[0].name="+catalogReaderBinding,
			"--set", "openchoreoApi.config.security.authorization.bootstrap.mappings[0].kind=ClusterAuthzRoleBinding",
			"--set", "openchoreoApi.config.security.authorization.bootstrap.mappings[0].roleMappings[0].roleRef.name="+catalogReaderRole,
			"--set", "openchoreoApi.config.security.authorization.bootstrap.mappings[0].roleMappings[0].roleRef.kind=ClusterAuthzRole",
			"--set", "openchoreoApi.config.security.authorization.bootstrap.mappings[0].entitlement.claim=sub",
			"--set", "openchoreoApi.config.security.authorization.bootstrap.mappings[0].entitlement.value=my-own-portal",
			"--set", "openchoreoApi.config.security.authorization.bootstrap.mappings[0].effect=allow",
		)
		if removals := authzBootstrapRemovals(t, args...); removals != nil {
			t.Errorf("the chart would delete a binding the deployment declares itself: %+v", removals)
		}
		if got := authzBootstrapNames(t, args...)[catalogReaderBinding]; got != "ClusterAuthzRoleBinding" {
			t.Errorf("the deployment's own binding is not seeded (%q)", got)
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
		rendered := entitlementOf(t, backstageGuardOverrides...)
		if !strings.Contains(rendered, `value: "openchoreo-backstage-client"`) {
			t.Error("the catalog reader binding does not default to backstage.auth.clientId")
		}
	})

	t.Run("follows a dedicated service client", func(t *testing.T) {
		args := append(append([]string{}, backstageGuardOverrides...),
			"--set", "backstage.auth.serviceClientId=portal-catalog-provider")
		rendered := entitlementOf(t, args...)
		if !strings.Contains(rendered, `value: "portal-catalog-provider"`) {
			t.Error("the catalog reader binding ignores backstage.auth.serviceClientId, so a " +
				"deployment using a dedicated service client fails authorization closed")
		}
	})

	t.Run("honours an explicit entitlement", func(t *testing.T) {
		args := append(append([]string{}, backstageGuardOverrides...),
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

	if got := render(t, backstageGuardOverrides); !strings.Contains(got, "component: backstage") {
		t.Error("with the portal installed, the API NetworkPolicy should still allow it in")
	}
	if got := render(t, helmGuardOverrides); strings.Contains(got, "component: backstage") {
		t.Error("with the portal uninstalled, the API NetworkPolicy still carries its podSelector")
	}
}

// catalogReaderActions returns the action list of the portal's catalog-reader
// role as rendered.
func catalogReaderActions(t *testing.T, args ...string) []string {
	t.Helper()

	for _, doc := range authzBootstrapDocs(t, args...) {
		if doc.Kind == "ClusterAuthzRole" && doc.Metadata.Name == catalogReaderRole {
			return doc.Spec.Actions
		}
	}
	t.Fatalf("the rendered bootstrap carries no %s ClusterAuthzRole", catalogReaderRole)
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
		if r.Name == catalogReaderRole {
			return r.Actions
		}
	}
	t.Fatalf("%s defines no %s role", helmValuesPath, catalogReaderRole)
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

	got := catalogReaderActions(t, backstageGuardOverrides...)
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("rendered catalog-reader actions differ from %s (-values +rendered):\n%s",
			helmValuesPath, diff)
	}
}
