// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package remote

import (
	"bytes"
	"context"
	"strings"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/remoteconnect"
)

func lineWorkload(component, line string, deps ...v1alpha1.WorkloadConnection) *v1alpha1.Workload {
	return &v1alpha1.Workload{
		ObjectMeta: metav1.ObjectMeta{Name: component, Namespace: "default"},
		Spec: v1alpha1.WorkloadSpec{
			Owner: v1alpha1.WorkloadOwner{ProjectName: "default", ComponentName: component, ReleaseLine: line},
			WorkloadTemplateSpec: v1alpha1.WorkloadTemplateSpec{
				Endpoints:    map[string]v1alpha1.WorkloadEndpoint{"http": {Type: v1alpha1.EndpointTypeHTTP, Port: 9090}},
				Dependencies: &v1alpha1.WorkloadDependencies{Endpoints: deps},
			},
		},
	}
}

func helloDep(line string) v1alpha1.WorkloadConnection {
	return v1alpha1.WorkloadConnection{
		Component: "hello", ReleaseLine: line, Name: "http", Visibility: "project",
		EnvBindings: v1alpha1.ConnectionEnvBindings{Address: "HELLO_URL"},
	}
}

func TestBuildResolveRequestCarriesReleaseLine(t *testing.T) {
	wl := lineWorkload("caller", "", helloDep("next"))
	req := buildResolveRequest(wl, "default", "development", wl.Spec.Dependencies.Endpoints)
	if len(req.Endpoints) != 1 || req.Endpoints[0].ReleaseLine != "next" {
		t.Fatalf("release line not sent: %+v", req.Endpoints)
	}
}

// A dependency links to a local workload only when that workload is the named line.
func TestSplitDependenciesMatchesReleaseLine(t *testing.T) {
	found := []discovered{
		{path: "hello-next.yaml", wl: lineWorkload("hello", "next")},
		{path: "hello-main.yaml", wl: lineWorkload("hello", "main")},
	}
	if err := assignIdentities(found, ""); err != nil {
		t.Fatal(err)
	}
	byIdentity, err := indexByIdentity(found)
	if err != nil {
		t.Fatalf("two lines of one component must not clash: %v", err)
	}

	caller := lineWorkload("caller", "", helloDep("next"), helloDep("canary"))
	remote, links := splitDependencies(caller, "default", byIdentity)
	if len(links) != 1 || links[0].key != remoteconnect.EndpointTargetKey("default", "hello", "next", "http") {
		t.Fatalf("expected one local link to hello@next, got %+v", links)
	}
	if len(remote) != 1 || remote[0].ReleaseLine != "canary" {
		t.Fatalf("expected the canary dependency to resolve remotely, got %+v", remote)
	}
}

// When every declared dependency is unconnectable there is nothing to run against, so no
// shell is opened and the reasons stay on screen.
func TestConnectFailsWhenNoDependencyConnects(t *testing.T) {
	path := writeWorkloadFileContent(t, "workload.yaml", testEndpointWorkloadYAML)
	d := New(&fakeResolver{resp: &remoteconnect.ResolveResponse{
		Capability: "cap",
		Unconnectable: []remoteconnect.Unconnectable{{
			Ref: "ep/finance/ledger-svc/http", Reason: "component finance/ledger-svc uses release lines; set releaseLine on the dependency",
		}},
	}})
	shellRan := false
	d.runShell = func(context.Context, []string) error { shellRan = true; return nil }

	var out bytes.Buffer
	err := d.Connect(t.Context(), ConnectParams{WorkloadPaths: []string{path}, Namespace: "default", Environment: "development"}, &out)
	if err == nil || !strings.Contains(err.Error(), "none of the declared dependencies could be connected") {
		t.Fatalf("expected an error, got %v", err)
	}
	if shellRan {
		t.Fatal("a shell must not open when nothing connected")
	}
	if !strings.Contains(out.String(), "set releaseLine on the dependency") {
		t.Errorf("reason missing from output:\n%s", out.String())
	}
	if strings.Contains(out.String(), "Tunnels open") {
		t.Errorf("must not claim tunnels are open:\n%s", out.String())
	}
}
