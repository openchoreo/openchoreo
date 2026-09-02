// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"
	"crypto/ed25519"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	authzcore "github.com/openchoreo/openchoreo/internal/authz/core"
	"github.com/openchoreo/openchoreo/internal/controller/resourcereleasebinding"
	svcpkg "github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	"github.com/openchoreo/openchoreo/internal/remoteconnect"
)

// httpStr is the shared "http" literal used across remote-connect resolve tests both as an
// endpoint name and as a URL scheme.
const httpStr = "http"

const docletPostgres = "doclet-postgres"

func remoteConnectScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	if err := openchoreov1alpha1.AddToScheme(s); err != nil {
		t.Fatal(err)
	}
	return s
}

// testEnvironmentAndDataPlane builds an Environment (in ns "default", named
// "development") referencing a DataPlane with PlaneID "dp-1" — every remote-connect
// dependency resolves through this same plane (resolveRemoteConnectPlane runs once per
// request).
func testEnvironmentAndDataPlane() (*openchoreov1alpha1.Environment, *openchoreov1alpha1.DataPlane) {
	dp := &openchoreov1alpha1.DataPlane{
		ObjectMeta: metav1.ObjectMeta{Name: "default", Namespace: "default"},
		Spec:       openchoreov1alpha1.DataPlaneSpec{PlaneID: "dp-1"},
	}
	env := &openchoreov1alpha1.Environment{
		ObjectMeta: metav1.ObjectMeta{Name: "development", Namespace: "default"},
		Spec: openchoreov1alpha1.EnvironmentSpec{
			DataPlaneRef: &openchoreov1alpha1.DataPlaneRef{Kind: openchoreov1alpha1.DataPlaneRefKindDataPlane, Name: "default"},
		},
	}
	return env, dp
}

func TestResolveEndpointAndResource(t *testing.T) {
	env, dp := testEnvironmentAndDataPlane()
	providerRB := &openchoreov1alpha1.ReleaseBinding{
		ObjectMeta: metav1.ObjectMeta{Name: "backend-api-development", Namespace: "default"},
		Spec: openchoreov1alpha1.ReleaseBindingSpec{
			Owner:       openchoreov1alpha1.ReleaseBindingOwner{ProjectName: "doclet", ComponentName: "backend-api"},
			Environment: "development",
		},
		Status: openchoreov1alpha1.ReleaseBindingStatus{
			Endpoints: []openchoreov1alpha1.EndpointURLStatus{{
				Name: httpStr,
				ServiceURL: &openchoreov1alpha1.EndpointURL{
					Scheme: httpStr, Host: "backend-api.dp-ns.svc.cluster.local", Port: 8080, Path: "/api",
				},
			}},
		},
	}
	providerRRB := &openchoreov1alpha1.ResourceReleaseBinding{
		ObjectMeta: metav1.ObjectMeta{Name: "doclet-postgres-development", Namespace: "default", Generation: 1},
		Spec: openchoreov1alpha1.ResourceReleaseBindingSpec{
			Owner:       openchoreov1alpha1.ResourceReleaseBindingOwner{ProjectName: "doclet", ResourceName: docletPostgres},
			Environment: "development",
		},
		Status: openchoreov1alpha1.ResourceReleaseBindingStatus{
			Conditions: []metav1.Condition{{
				Type: "Ready", Status: metav1.ConditionTrue, ObservedGeneration: 1,
				Reason: "Ready", LastTransitionTime: metav1.Now(),
			}},
			Outputs: []openchoreov1alpha1.ResolvedResourceOutput{
				{Name: "host", Value: "pg.dp-ns.svc.cluster.local"},
				{Name: "port", Value: "5432"},
				{Name: "database", Value: "doclet"},
				{Name: "password", SecretKeyRef: &openchoreov1alpha1.SecretKeyRef{Name: "pg-secret", Key: "password"}},
			},
			Endpoints: []openchoreov1alpha1.ResolvedResourceEndpoint{{
				Name: "client", Host: "pg.dp-ns.svc.cluster.local", Port: 5432,
				HostFrom: "host", PortFrom: "port",
			}},
		},
	}

	cl := fake.NewClientBuilder().WithScheme(remoteConnectScheme(t)).WithObjects(env, dp, providerRB, providerRRB).Build()

	_, priv, _ := ed25519.GenerateKey(nil)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	h := &RemoteConnectHandler{
		k8sClient:    cl,
		authzChecker: svcpkg.NewAuthzChecker(&recordingPDP{}, logger),
		signer:       &capabilitySigner{privKey: priv, keyID: "k1", issuer: "cp", ttl: 30 * time.Minute},
		// Matches the shipped default; the disabled path has its own test below.
		secretsEnabled: true,
		logger:         logger,
	}

	req := remoteconnect.ResolveRequest{
		Namespace:   "default",
		Project:     "doclet",
		Component:   "doclet-document",
		Environment: "development",
		Endpoints: []remoteconnect.EndpointDep{{
			Component: "backend-api", Name: httpStr, Visibility: "project",
			EnvBindings: remoteconnect.EndpointEnvBindings{Address: "BACKEND_API_URL"},
		}},
		Resources: []remoteconnect.ResourceDep{{
			Ref: docletPostgres,
			EnvBindings: map[string]string{
				"host": "DB_HOST", "port": "DB_PORT", "database": "DB_NAME", "password": "DB_PASSWORD",
			},
		}},
	}

	resp, err := h.resolve(context.Background(), req, "user:alice")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if len(resp.Unconnectable) != 0 {
		t.Fatalf("unexpected unconnectable: %+v", resp.Unconnectable)
	}
	if len(resp.Targets) != 2 {
		t.Fatalf("expected 2 targets, got %d: %+v", len(resp.Targets), resp.Targets)
	}

	// Endpoint render info.
	ep := findTarget(t, resp.Targets, "ep/doclet/backend-api/http")
	if ep.Endpoint == nil || ep.Endpoint.Scheme != httpStr || ep.Endpoint.BasePath != "/api" || ep.Endpoint.Bindings.Address != "BACKEND_API_URL" {
		t.Fatalf("bad endpoint render: %+v", ep.Endpoint)
	}
	// Resource render info: the endpoint's host/port bindings follow the tunnel.
	res := findTarget(t, resp.Targets, "res/doclet-postgres/client")
	if res.Resource == nil || res.Resource.HostEnv != "DB_HOST" || res.Resource.PortEnv != "DB_PORT" {
		t.Fatalf("bad resource render: %+v", res.Resource)
	}
	if res.Resource.Ref != docletPostgres || res.Resource.Endpoint != "client" {
		t.Fatalf("resource render not attributed to its endpoint: %+v", res.Resource)
	}
	// Tunnel-independent bindings: value-kind statics present, secret reported.
	if len(resp.Resources) != 1 || resp.Resources[0].Ref != docletPostgres {
		t.Fatalf("expected bindings for doclet-postgres, got %+v", resp.Resources)
	}
	rb := resp.Resources[0]
	if rb.StaticEnv["DB_NAME"] != "doclet" {
		t.Fatalf("expected DB_NAME=doclet, got %+v", rb.StaticEnv)
	}
	// host/port are the endpoint's own bindings and must not also be emitted as their
	// unreachable in-cluster values.
	if _, dup := rb.StaticEnv["DB_HOST"]; dup {
		t.Fatalf("DB_HOST leaked into static env: %+v", rb.StaticEnv)
	}
	if _, dup := rb.StaticEnv["DB_PORT"]; dup {
		t.Fatalf("DB_PORT leaked into static env: %+v", rb.StaticEnv)
	}
	// The secret-backed value is not in this response at all — it is a fetch key occ
	// resolves over the tunnel, which is what keeps the value off the control plane.
	if len(rb.OmittedSecretEnv) != 0 {
		t.Fatalf("expected nothing omitted, got %+v", rb.OmittedSecretEnv)
	}
	wantKey := remoteconnect.SecretGrantKey(docletPostgres, "password")
	if rb.FetchEnv["DB_PASSWORD"] != wantKey {
		t.Fatalf("expected DB_PASSWORD to fetch %q, got %+v", wantKey, rb.FetchEnv)
	}
	for _, v := range rb.StaticEnv {
		if v == "s3cr3t" {
			t.Fatalf("secret value leaked into static env: %+v", rb.StaticEnv)
		}
	}

	// Capability verifies and carries the concrete (signed) dial + plane-routing targets.
	assertResolveCapability(t, resp.Capability, priv)
}

// assertResolveCapability verifies the signed capability from TestResolveEndpointAndResource
// and asserts its claims plus the endpoint/resource dial and plane-routing targets.
func assertResolveCapability(t *testing.T, capabilityToken string, priv ed25519.PrivateKey) {
	t.Helper()
	pub := priv.Public().(ed25519.PublicKey)
	claims, err := remoteconnect.VerifyCapability(capabilityToken, pub)
	if err != nil {
		t.Fatalf("verify capability: %v", err)
	}
	if claims.Subject != "user:alice" || claims.Namespace != defaultPlaneName ||
		claims.Component.Name != "doclet-document" || claims.Env != "development" {
		t.Fatalf("unexpected capability claims: %+v", claims)
	}
	epT, ok := claims.TargetByKey("ep/doclet/backend-api/http")
	if !ok || epT.Host != "backend-api.dp-ns.svc.cluster.local" || epT.Port != 8080 {
		t.Fatalf("bad endpoint capability target: %+v ok=%v", epT, ok)
	}
	resT, ok := claims.TargetByKey("res/doclet-postgres/client")
	if !ok || resT.Host != "pg.dp-ns.svc.cluster.local" || resT.Port != 5432 {
		t.Fatalf("bad resource capability target: %+v ok=%v", resT, ok)
	}
}

func TestResolveUnreadyResourceIsUnconnectable(t *testing.T) {
	env, dp := testEnvironmentAndDataPlane()
	// ResourceReleaseBinding present but not Ready → reported unconnectable, not an error.
	notReady := &openchoreov1alpha1.ResourceReleaseBinding{
		ObjectMeta: metav1.ObjectMeta{Name: "doclet-postgres-development", Namespace: "default", Generation: 1},
		Spec: openchoreov1alpha1.ResourceReleaseBindingSpec{
			Owner:       openchoreov1alpha1.ResourceReleaseBindingOwner{ProjectName: "doclet", ResourceName: docletPostgres},
			Environment: "development",
		},
		Status: openchoreov1alpha1.ResourceReleaseBindingStatus{
			Conditions: []metav1.Condition{{
				Type: "Ready", Status: metav1.ConditionFalse, ObservedGeneration: 1,
				Reason: "Provisioning", LastTransitionTime: metav1.Now(),
			}},
		},
	}
	cl := fake.NewClientBuilder().WithScheme(remoteConnectScheme(t)).WithObjects(env, dp, notReady).Build()
	_, priv, _ := ed25519.GenerateKey(nil)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	h := &RemoteConnectHandler{
		k8sClient:    cl,
		authzChecker: svcpkg.NewAuthzChecker(&recordingPDP{}, logger),
		signer:       &capabilitySigner{privKey: priv, ttl: time.Minute},
		logger:       logger,
	}
	resp, err := h.resolve(context.Background(), remoteconnect.ResolveRequest{
		Namespace: "default", Project: "doclet", Component: "doclet-document", Environment: "development",
		Resources: []remoteconnect.ResourceDep{{Ref: docletPostgres, EnvBindings: map[string]string{"host": "DB_HOST", "port": "DB_PORT"}}},
	}, "user:alice")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if len(resp.Targets) != 0 || len(resp.Unconnectable) != 1 ||
		resp.Unconnectable[0].Ref != remoteconnect.ResourceRefKey(docletPostgres) {
		t.Fatalf("expected 1 unconnectable resource, got targets=%+v unconnectable=%+v", resp.Targets, resp.Unconnectable)
	}
	// The reason must not say WHY. "not ready", "does not exist" and "not authorized"
	// are one message, or a caller can probe another project's resources by name.
	if resp.Unconnectable[0].Reason != unavailableReason {
		t.Fatalf("reason leaks detail: %q", resp.Unconnectable[0].Reason)
	}
}

// A resource with no declared endpoints is not an error. It contributes its
// resolvable bindings and produces no dial target -- a bucket name and a region need
// no tunnel.
func TestResolveResourceWithoutEndpointsStillBinds(t *testing.T) {
	env, dp := testEnvironmentAndDataPlane()
	bucket := &openchoreov1alpha1.ResourceReleaseBinding{
		ObjectMeta: metav1.ObjectMeta{Name: "tc-bucket-development", Namespace: "default", Generation: 1},
		Spec: openchoreov1alpha1.ResourceReleaseBindingSpec{
			Owner:       openchoreov1alpha1.ResourceReleaseBindingOwner{ProjectName: "doclet", ResourceName: "tc-bucket"},
			Environment: "development",
		},
		Status: openchoreov1alpha1.ResourceReleaseBindingStatus{
			Conditions: []metav1.Condition{{
				Type: "Ready", Status: metav1.ConditionTrue, ObservedGeneration: 1,
				Reason: "Ready", LastTransitionTime: metav1.Now(),
			}},
			Outputs: []openchoreov1alpha1.ResolvedResourceOutput{
				{Name: "bucket", Value: "assets"},
				{Name: "region", Value: "us-east-1"},
			},
			// No Endpoints: nothing dialable.
		},
	}
	cl := fake.NewClientBuilder().WithScheme(remoteConnectScheme(t)).WithObjects(env, dp, bucket).Build()
	_, priv, _ := ed25519.GenerateKey(nil)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	h := &RemoteConnectHandler{
		k8sClient:    cl,
		authzChecker: svcpkg.NewAuthzChecker(&recordingPDP{}, logger),
		signer:       &capabilitySigner{privKey: priv, ttl: time.Minute},
		logger:       logger,
	}

	resp, err := h.resolve(context.Background(), remoteconnect.ResolveRequest{
		Namespace: "default", Project: "doclet", Component: "doclet-document", Environment: "development",
		Resources: []remoteconnect.ResourceDep{{
			Ref:         "tc-bucket",
			EnvBindings: map[string]string{"bucket": "ASSETS_BUCKET", "region": "ASSETS_REGION"},
		}},
	}, "user:alice")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if len(resp.Unconnectable) != 0 {
		t.Fatalf("a resource with no endpoint must not be unconnectable: %+v", resp.Unconnectable)
	}
	if len(resp.Targets) != 0 {
		t.Fatalf("expected no dial targets, got %+v", resp.Targets)
	}
	if len(resp.Resources) != 1 {
		t.Fatalf("expected bindings for tc-bucket, got %+v", resp.Resources)
	}
	got := resp.Resources[0].StaticEnv
	if got["ASSETS_BUCKET"] != "assets" || got["ASSETS_REGION"] != "us-east-1" {
		t.Fatalf("bindings dropped for an endpointless resource: %+v", got)
	}
}

// Two endpoints on one resource produce two distinct targets, each rewriting
// only its own port binding while both name the same host binding.
func TestResolveResourceWithTwoEndpoints(t *testing.T) {
	env, dp := testEnvironmentAndDataPlane()
	broker := &openchoreov1alpha1.ResourceReleaseBinding{
		ObjectMeta: metav1.ObjectMeta{Name: "tc-broker-development", Namespace: "default", Generation: 1},
		Spec: openchoreov1alpha1.ResourceReleaseBindingSpec{
			Owner:       openchoreov1alpha1.ResourceReleaseBindingOwner{ProjectName: "doclet", ResourceName: "tc-broker"},
			Environment: "development",
		},
		Status: openchoreov1alpha1.ResourceReleaseBindingStatus{
			Conditions: []metav1.Condition{{
				Type: "Ready", Status: metav1.ConditionTrue, ObservedGeneration: 1,
				Reason: "Ready", LastTransitionTime: metav1.Now(),
			}},
			Outputs: []openchoreov1alpha1.ResolvedResourceOutput{
				{Name: "host", Value: "nats.dp-ns.svc.cluster.local"},
				{Name: "port", Value: "4222"},
				{Name: "monitorPort", Value: "8222"},
			},
			Endpoints: []openchoreov1alpha1.ResolvedResourceEndpoint{
				{Name: "client", Host: "nats.dp-ns.svc.cluster.local", Port: 4222, HostFrom: "host", PortFrom: "port"},
				{Name: "monitor", Host: "nats.dp-ns.svc.cluster.local", Port: 8222, HostFrom: "host", PortFrom: "monitorPort"},
			},
		},
	}
	cl := fake.NewClientBuilder().WithScheme(remoteConnectScheme(t)).WithObjects(env, dp, broker).Build()
	_, priv, _ := ed25519.GenerateKey(nil)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	h := &RemoteConnectHandler{
		k8sClient:    cl,
		authzChecker: svcpkg.NewAuthzChecker(&recordingPDP{}, logger),
		signer:       &capabilitySigner{privKey: priv, ttl: time.Minute},
		logger:       logger,
	}

	resp, err := h.resolve(context.Background(), remoteconnect.ResolveRequest{
		Namespace: "default", Project: "doclet", Component: "doclet-document", Environment: "development",
		Resources: []remoteconnect.ResourceDep{{
			Ref: "tc-broker",
			EnvBindings: map[string]string{
				"host": "BROKER_HOST", "port": "BROKER_PORT", "monitorPort": "BROKER_MONITOR_PORT",
			},
		}},
	}, "user:alice")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if len(resp.Targets) != 2 {
		t.Fatalf("expected 2 targets, got %+v", resp.Targets)
	}
	client := findTarget(t, resp.Targets, "res/tc-broker/client")
	monitor := findTarget(t, resp.Targets, "res/tc-broker/monitor")
	if client.Resource.PortEnv != "BROKER_PORT" || monitor.Resource.PortEnv != "BROKER_MONITOR_PORT" {
		t.Fatalf("port bindings not split per endpoint: client=%+v monitor=%+v", client.Resource, monitor.Resource)
	}
	if client.Resource.HostEnv != "BROKER_HOST" || monitor.Resource.HostEnv != "BROKER_HOST" {
		t.Fatalf("both endpoints should name the same host binding: client=%+v monitor=%+v", client.Resource, monitor.Resource)
	}
	// The monitoring port must not also leak through as its unreachable remote value.
	if len(resp.Resources) != 1 {
		t.Fatalf("expected one bindings entry, got %+v", resp.Resources)
	}
	if _, leaked := resp.Resources[0].StaticEnv["BROKER_MONITOR_PORT"]; leaked {
		t.Fatalf("BROKER_MONITOR_PORT leaked as a static value: %+v", resp.Resources[0].StaticEnv)
	}

	// Both endpoints are signed into the capability as separate dial targets.
	pub := priv.Public().(ed25519.PublicKey)
	claims, verr := remoteconnect.VerifyCapability(resp.Capability, pub)
	if verr != nil {
		t.Fatalf("verify capability: %v", verr)
	}
	c, okC := claims.TargetByKey("res/tc-broker/client")
	m, okM := claims.TargetByKey("res/tc-broker/monitor")
	if !okC || !okM || c.Port != 4222 || m.Port != 8222 {
		t.Fatalf("capability targets wrong: client=%+v (%v) monitor=%+v (%v)", c, okC, m, okM)
	}
}

func TestResolveMissingDataPlaneRefFails(t *testing.T) {
	// Environment with no DataPlaneRef: resolution fails outright rather than
	// silently marking every dependency unconnectable.
	env := &openchoreov1alpha1.Environment{
		ObjectMeta: metav1.ObjectMeta{Name: "development", Namespace: "default"},
	}
	cl := fake.NewClientBuilder().WithScheme(remoteConnectScheme(t)).WithObjects(env).Build()
	_, priv, _ := ed25519.GenerateKey(nil)
	h := &RemoteConnectHandler{
		k8sClient: cl,
		signer:    &capabilitySigner{privKey: priv, ttl: time.Minute},
		logger:    slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	_, err := h.resolve(context.Background(), remoteconnect.ResolveRequest{
		Namespace: "default", Project: "doclet", Component: "doclet-document", Environment: "development",
	}, "user:alice")
	if err == nil {
		t.Fatal("expected an error when the environment has no data plane reference")
	}
}

func findTarget(t *testing.T, targets []remoteconnect.ResolvedTarget, key string) remoteconnect.ResolvedTarget {
	t.Helper()
	for _, tg := range targets {
		if tg.Key == key {
			return tg
		}
	}
	t.Fatalf("target %q not found in %+v", key, targets)
	return remoteconnect.ResolvedTarget{}
}

// readyPostgresRRB builds a Ready ResourceReleaseBinding for project/resource with one
// fully resolved endpoint, so a caller that is authorized gets a real dial target.
func readyPostgresRRB(project, resource string) *openchoreov1alpha1.ResourceReleaseBinding {
	return &openchoreov1alpha1.ResourceReleaseBinding{
		ObjectMeta: metav1.ObjectMeta{Name: project + "-" + resource + "-development", Namespace: "default", Generation: 1},
		Spec: openchoreov1alpha1.ResourceReleaseBindingSpec{
			Owner:       openchoreov1alpha1.ResourceReleaseBindingOwner{ProjectName: project, ResourceName: resource},
			Environment: "development",
		},
		Status: openchoreov1alpha1.ResourceReleaseBindingStatus{
			Conditions: []metav1.Condition{{
				Type: "Ready", Status: metav1.ConditionTrue, ObservedGeneration: 1,
				Reason: "Ready", LastTransitionTime: metav1.Now(),
			}},
			Outputs: []openchoreov1alpha1.ResolvedResourceOutput{
				{Name: "host", Value: "pg." + project + ".svc.cluster.local"},
				{Name: "port", Value: "5432"},
				{Name: "adminPassword", Value: "plain-in-output"},
			},
			Endpoints: []openchoreov1alpha1.ResolvedResourceEndpoint{{
				Name: "client", Host: "pg." + project + ".svc.cluster.local", Port: 5432,
				HostFrom: "host", PortFrom: "port",
			}},
		},
	}
}

func resourceHandler(t *testing.T, pdp *recordingPDP, objs ...client.Object) *RemoteConnectHandler {
	t.Helper()
	env, dp := testEnvironmentAndDataPlane()
	all := append([]client.Object{env, dp}, objs...)
	cl := fake.NewClientBuilder().WithScheme(remoteConnectScheme(t)).WithObjects(all...).Build()
	_, priv, _ := ed25519.GenerateKey(nil)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return &RemoteConnectHandler{
		k8sClient:    cl,
		authzChecker: svcpkg.NewAuthzChecker(pdp, logger),
		signer:       &capabilitySigner{privKey: priv, ttl: time.Minute},
		logger:       logger,
	}
}

// A resource dependency must be authorized. req.Project comes from a local workload file
// and is never itself authorized, so without a per-resource check a caller could name any
// project and receive both its plain outputs and a signed tunnel into its data plane.
func TestResolveResourceRequiresAuthorization(t *testing.T) {
	// Granted only within the caller's own project, not "victim".
	pdp := &recordingPDP{grants: []string{"ns/default/project/attacker"}}
	h := resourceHandler(t, pdp, readyPostgresRRB("victim", "victim-db"))

	resp, err := h.resolve(context.Background(), remoteconnect.ResolveRequest{
		Namespace: "default", Project: "victim", Component: "anything", Environment: "development",
		Resources: []remoteconnect.ResourceDep{{
			Ref:         "victim-db",
			EnvBindings: map[string]string{"host": "DB_HOST", "port": "DB_PORT", "adminPassword": "PW"},
		}},
	}, "user:attacker")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if len(resp.Targets) != 0 {
		t.Fatalf("unauthorized resource yielded dial targets: %+v", resp.Targets)
	}
	// No bindings either: a denied resource must not leak the plain values of its outputs.
	if len(resp.Resources) != 0 {
		t.Fatalf("unauthorized resource leaked bindings: %+v", resp.Resources)
	}
	if len(resp.Unconnectable) != 1 || resp.Unconnectable[0].Reason != unavailableReason {
		t.Fatalf("expected one indistinguishable unconnectable, got %+v", resp.Unconnectable)
	}
	// The check must actually have been made, at resource scope.
	if len(pdp.requests) != 1 {
		t.Fatalf("expected 1 authorization request, got %d", len(pdp.requests))
	}
	if got := pdp.requests[0].Action; got != authzcore.ActionConnectResource {
		t.Fatalf("action = %q, want %q", got, authzcore.ActionConnectResource)
	}
	if h := pdp.requests[0].Resource.Hierarchy; h.Project != "victim" || h.Resource != "victim-db" {
		t.Fatalf("hierarchy = %+v, want project=victim resource=victim-db", h)
	}
}

// The same request from a caller the policy does grant resolves fully, so the check
// above is gating on authorization rather than on something incidental.
func TestResolveResourceAuthorizedResolves(t *testing.T) {
	pdp := &recordingPDP{grants: []string{"ns/default/project/doclet"}}
	h := resourceHandler(t, pdp, readyPostgresRRB("doclet", docletPostgres))

	resp, err := h.resolve(context.Background(), remoteconnect.ResolveRequest{
		Namespace: "default", Project: "doclet", Component: "doclet-document", Environment: "development",
		Resources: []remoteconnect.ResourceDep{{
			Ref:         docletPostgres,
			EnvBindings: map[string]string{"host": "DB_HOST", "port": "DB_PORT"},
		}},
	}, "user:alice")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if len(resp.Targets) != 1 || len(resp.Unconnectable) != 0 {
		t.Fatalf("expected one target and no unconnectable, got %+v / %+v", resp.Targets, resp.Unconnectable)
	}
}

// An endpoint naming its host output but not its port output cannot be tunneled: the
// host binding would follow the tunnel while the port binding kept the in-cluster port,
// pointing the app at 127.0.0.1:<remote port>. ResourceType validation rejects the shape,
// so this only arrives from a ResourceRelease cut before that rule existed.
func TestResolveResourceHalfRedirectableEndpointIsRefused(t *testing.T) {
	rrb := readyPostgresRRB("doclet", docletPostgres)
	rrb.Status.Endpoints[0].PortFrom = "" // host output named, port inline
	pdp := &recordingPDP{}
	h := resourceHandler(t, pdp, rrb)

	resp, err := h.resolve(context.Background(), remoteconnect.ResolveRequest{
		Namespace: "default", Project: "doclet", Component: "doclet-document", Environment: "development",
		Resources: []remoteconnect.ResourceDep{{
			Ref:         docletPostgres,
			EnvBindings: map[string]string{"host": "DB_HOST", "port": "DB_PORT"},
		}},
	}, "user:alice")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if len(resp.Targets) != 0 {
		t.Fatalf("half-redirectable endpoint was tunneled: %+v", resp.Targets)
	}
	if len(resp.Unconnectable) != 1 {
		t.Fatalf("expected the endpoint reported, got %+v", resp.Unconnectable)
	}
	// Nothing may be silently redirected: DB_HOST must not become a local address while
	// DB_PORT keeps the remote port.
	if len(resp.Resources) != 1 {
		t.Fatalf("expected one binding set, got %+v", resp.Resources)
	}
	static := resp.Resources[0].StaticEnv
	if static["DB_HOST"] != "pg.doclet.svc.cluster.local" || static["DB_PORT"] != "5432" {
		t.Fatalf("bindings should stay as published in-cluster, got %+v", static)
	}
}

// One ref declared twice contributes one set of bindings. Two would make occ report a
// bogus clash between a workload and itself.
func TestResolveResourceDuplicateRefBindsOnce(t *testing.T) {
	pdp := &recordingPDP{}
	h := resourceHandler(t, pdp, readyPostgresRRB("doclet", docletPostgres))

	dep := remoteconnect.ResourceDep{
		Ref:         docletPostgres,
		EnvBindings: map[string]string{"host": "DB_HOST", "port": "DB_PORT"},
	}
	resp, err := h.resolve(context.Background(), remoteconnect.ResolveRequest{
		Namespace: "default", Project: "doclet", Component: "doclet-document", Environment: "development",
		Resources: []remoteconnect.ResourceDep{dep, dep},
	}, "user:alice")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if len(resp.Resources) != 1 {
		t.Fatalf("expected 1 binding set for a duplicated ref, got %d: %+v", len(resp.Resources), resp.Resources)
	}
	if len(resp.Targets) != 1 {
		t.Fatalf("expected 1 target for a duplicated ref, got %+v", resp.Targets)
	}
}

// One half of a redirectable address bound and the other not: neither is redirected,
// and the endpoint is reported.
func TestResolveResourceHalfBoundAddressIsNotRedirected(t *testing.T) {
	for _, tc := range []struct {
		name     string
		bindings map[string]string
		want     map[string]string
	}{
		{
			name:     "host bound, port not",
			bindings: map[string]string{"host": "DB_HOST"},
			want:     map[string]string{"DB_HOST": "pg.doclet.svc.cluster.local"},
		},
		{
			name:     "port bound, host not",
			bindings: map[string]string{"port": "DB_PORT"},
			want:     map[string]string{"DB_PORT": "5432"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := resourceHandler(t, &recordingPDP{}, readyPostgresRRB("doclet", docletPostgres))

			resp, err := h.resolve(context.Background(), remoteconnect.ResolveRequest{
				Namespace: "default", Project: "doclet", Component: "doclet-document", Environment: "development",
				Resources: []remoteconnect.ResourceDep{{Ref: docletPostgres, EnvBindings: tc.bindings}},
			}, "user:alice")
			if err != nil {
				t.Fatalf("resolve: %v", err)
			}

			target := findTarget(t, resp.Targets, "res/"+docletPostgres+"/client")
			if target.Resource.HostEnv != "" || target.Resource.PortEnv != "" {
				t.Fatalf("half-bound address was redirected: HostEnv=%q PortEnv=%q",
					target.Resource.HostEnv, target.Resource.PortEnv)
			}
			if len(resp.Unconnectable) != 1 {
				t.Fatalf("expected the endpoint reported once, got %+v", resp.Unconnectable)
			}
			if len(resp.Resources) != 1 {
				t.Fatalf("expected one binding set, got %+v", resp.Resources)
			}
			// The bound half is still emitted, as its published value.
			static := resp.Resources[0].StaticEnv
			for k, v := range tc.want {
				if static[k] != v {
					t.Fatalf("%s = %q, want the published %q; got %+v", k, static[k], v, static)
				}
			}
		})
	}
}

// Both halves bound is the ordinary case and still redirects.
func TestResolveResourceFullyBoundAddressIsRedirected(t *testing.T) {
	h := resourceHandler(t, &recordingPDP{}, readyPostgresRRB("doclet", docletPostgres))

	resp, err := h.resolve(context.Background(), remoteconnect.ResolveRequest{
		Namespace: "default", Project: "doclet", Component: "doclet-document", Environment: "development",
		Resources: []remoteconnect.ResourceDep{{
			Ref:         docletPostgres,
			EnvBindings: map[string]string{"host": "DB_HOST", "port": "DB_PORT"},
		}},
	}, "user:alice")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	target := findTarget(t, resp.Targets, "res/"+docletPostgres+"/client")
	if target.Resource.HostEnv != "DB_HOST" || target.Resource.PortEnv != "DB_PORT" {
		t.Fatalf("bound address should follow the tunnel: %+v", target.Resource)
	}
	if len(resp.Unconnectable) != 0 {
		t.Fatalf("unexpected unconnectable: %+v", resp.Unconnectable)
	}
	if static := resp.Resources[0].StaticEnv; static["DB_HOST"] != "" || static["DB_PORT"] != "" {
		t.Fatalf("redirected bindings must not also be published in-cluster: %+v", static)
	}
}

// The condition's message reaches occ, worded to separate a transient wait from a defect.
func TestResolveResourceReportsUnresolvedEndpointReason(t *testing.T) {
	for _, tc := range []struct {
		name       string
		reason     string
		message    string
		wantSubstr string
	}{
		{
			name:       "pending",
			reason:     string(resourcereleasebinding.ReasonEndpointsPending),
			message:    `endpoint "client": output "port" rendered an empty port: not yet resolved`,
			wantSubstr: "not resolved yet",
		},
		{
			name:       "failed",
			reason:     string(resourcereleasebinding.ReasonEndpointResolutionFailed),
			message:    `endpoint "client": port: port "quite-wrong" is not numeric`,
			wantSubstr: "could not be resolved by the control plane",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rrb := readyPostgresRRB("doclet", docletPostgres)
			// The endpoint failed to resolve, so it never reached status.endpoints.
			rrb.Status.Endpoints = nil
			rrb.Status.Conditions = append(rrb.Status.Conditions, metav1.Condition{
				Type: string(resourcereleasebinding.ConditionEndpointsResolved), Status: metav1.ConditionFalse,
				Reason: tc.reason, Message: tc.message, LastTransitionTime: metav1.Now(), ObservedGeneration: 1,
			})
			h := resourceHandler(t, &recordingPDP{}, rrb)

			resp, err := h.resolve(context.Background(), remoteconnect.ResolveRequest{
				Namespace: "default", Project: "doclet", Component: "doclet-document", Environment: "development",
				Resources: []remoteconnect.ResourceDep{{
					Ref:         docletPostgres,
					EnvBindings: map[string]string{"host": "DB_HOST", "port": "DB_PORT"},
				}},
			}, "user:alice")
			if err != nil {
				t.Fatalf("resolve: %v", err)
			}

			if len(resp.Unconnectable) != 1 {
				t.Fatalf("expected the resource reported once, got %+v", resp.Unconnectable)
			}
			got := resp.Unconnectable[0]
			if got.Ref != remoteconnect.ResourceRefKey(docletPostgres) {
				t.Fatalf("report not attributed to the resource: %+v", got)
			}
			if !strings.Contains(got.Reason, tc.wantSubstr) {
				t.Fatalf("reason %q does not say %q", got.Reason, tc.wantSubstr)
			}
			// The controller's own message must survive, so the developer sees the cause.
			if !strings.Contains(got.Reason, tc.message) {
				t.Fatalf("reason %q drops the controller's message %q", got.Reason, tc.message)
			}
		})
	}
}

// A resource whose endpoints resolved reports nothing extra.
func TestResolveResourceReportsNothingWhenEndpointsResolved(t *testing.T) {
	h := resourceHandler(t, &recordingPDP{}, readyPostgresRRB("doclet", docletPostgres))

	resp, err := h.resolve(context.Background(), remoteconnect.ResolveRequest{
		Namespace: "default", Project: "doclet", Component: "doclet-document", Environment: "development",
		Resources: []remoteconnect.ResourceDep{{
			Ref:         docletPostgres,
			EnvBindings: map[string]string{"host": "DB_HOST", "port": "DB_PORT"},
		}},
	}, "user:alice")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if len(resp.Unconnectable) != 0 {
		t.Fatalf("unexpected unconnectable: %+v", resp.Unconnectable)
	}
}
