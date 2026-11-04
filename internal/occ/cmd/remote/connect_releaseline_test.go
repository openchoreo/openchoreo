// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package remote

import (
	"bytes"
	"context"
	"slices"
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
	wl := lineWorkload("caller", "", helloDep("next"), helloDep("main"))
	req := buildResolveRequest(wl, "default", "development", wl.Spec.Dependencies.Endpoints)
	if len(req.Endpoints) != 2 || req.Endpoints[0].ReleaseLine != "next" {
		t.Fatalf("release line not sent: %+v", req.Endpoints)
	}
	if req.Endpoints[1].ReleaseLine != "" {
		t.Fatalf("the main line keyword must be sent as the main line, got %q", req.Endpoints[1].ReleaseLine)
	}
}

// A dependency links to a local workload only when that workload is the named line. The main
// keyword names the main line, in a workload's owner and in a dependency alike.
func TestSplitDependenciesMatchesReleaseLine(t *testing.T) {
	tests := []struct {
		name     string
		mainLine string
		mainDep  string
	}{
		{name: "main line without a line", mainLine: "", mainDep: ""},
		{name: "main line keyword in the workload", mainLine: "main", mainDep: ""},
		{name: "main line keyword in the dependency", mainLine: "", mainDep: "main"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			found := []discovered{
				{path: "hello-next.yaml", wl: lineWorkload("hello", "next")},
				{path: "hello.yaml", wl: lineWorkload("hello", tt.mainLine)},
			}
			if err := assignIdentities(found, ""); err != nil {
				t.Fatal(err)
			}
			byIdentity, err := indexByIdentity(found)
			if err != nil {
				t.Fatalf("two lines of one component must not clash: %v", err)
			}

			caller := lineWorkload("caller", "", helloDep("next"), helloDep(tt.mainDep), helloDep("canary"))
			remote, links := splitDependencies(caller, "default", byIdentity)
			got := make([]string, 0, len(links))
			for _, l := range links {
				got = append(got, l.provider+" "+l.key)
			}
			want := []string{
				"hello@next " + remoteconnect.EndpointTargetKey("default", "hello", "next", "http"),
				"hello " + remoteconnect.EndpointTargetKey("default", "hello", "", "http"),
			}
			if !slices.Equal(got, want) {
				t.Fatalf("expected local links to hello@next and hello, got %v", got)
			}
			if len(remote) != 1 || remote[0].ReleaseLine != "canary" {
				t.Fatalf("expected the canary dependency to resolve remotely, got %+v", remote)
			}
		})
	}
}

// Two lines of one component run on their own ports, so --local addresses each line.
func TestLocalOverridesPerReleaseLine(t *testing.T) {
	overrides, err := parseLocalOverrides([]string{"hello=127.0.0.1:9091", "hello@next=127.0.0.1:9092", "web@main=127.0.0.1:9093"})
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		provider string
		port     int
	}{{"hello", 9091}, {"hello@next", 9092}, {"web", 9093}, {"hello@canary", 9090}} {
		_, port := localLink{provider: tt.provider, defaultPort: 9090}.target(overrides)
		if port != tt.port {
			t.Errorf("%s: got port %d, want %d", tt.provider, port, tt.port)
		}
	}

	for _, bad := range []string{"@next=127.0.0.1:9092", "hello@=127.0.0.1:9092"} {
		if _, err := parseLocalOverrides([]string{bad}); err == nil {
			t.Errorf("%s: expected an error", bad)
		}
	}
}

// When every declared dependency is unconnectable there is nothing to run against, so no
// shell is opened and the reasons stay on screen.
func TestConnectFailsWhenNoDependencyConnects(t *testing.T) {
	path := writeWorkloadFileContent(t, "workload.yaml", testEndpointWorkloadYAML)
	d := New(&fakeResolver{resp: &remoteconnect.ResolveResponse{
		Capability: "cap",
		Unconnectable: []remoteconnect.Unconnectable{{
			Ref: "ep/finance/ledger-svc/http", Reason: "the main line of component finance/ledger-svc is not deployed in environment development; " +
				"set releaseLine to depend on one of its release lines",
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
	if !strings.Contains(out.String(), "set releaseLine to depend on one of its release lines") {
		t.Errorf("reason missing from output:\n%s", out.String())
	}
	if strings.Contains(out.String(), "Tunnels open") {
		t.Errorf("must not claim tunnels are open:\n%s", out.String())
	}
}
