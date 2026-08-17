// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"
	"crypto/ed25519"
	"io"
	"log/slog"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/depconnect"
	svcpkg "github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
)

// httpStr is the shared "http" literal used across dep-connect resolve tests both as an
// endpoint name and as a URL scheme.
const httpStr = "http"

func depConnectScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	if err := openchoreov1alpha1.AddToScheme(s); err != nil {
		t.Fatal(err)
	}
	return s
}

// testEnvironmentAndDataPlane builds an Environment (in ns "default", named
// "development") referencing a DataPlane with PlaneID "dp-1" — every dep-connect
// dependency resolves through this same plane (resolveDepConnectPlane runs once per
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

func TestResolveEndpoint(t *testing.T) {
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

	cl := fake.NewClientBuilder().WithScheme(depConnectScheme(t)).WithObjects(env, dp, providerRB).Build()

	_, priv, _ := ed25519.GenerateKey(nil)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	h := &DepConnectHandler{
		k8sClient:    cl,
		authzChecker: svcpkg.NewAuthzChecker(&recordingPDP{}, logger),
		signer:       &capabilitySigner{privKey: priv, keyID: "k1", issuer: "cp", ttl: 30 * time.Minute},
		logger:       logger,
	}

	req := depconnect.ResolveRequest{
		Namespace:   "default",
		Project:     "doclet",
		Component:   "doclet-document",
		Environment: "development",
		Endpoints: []depconnect.EndpointDep{{
			Component: "backend-api", Name: httpStr, Visibility: "project",
			EnvBindings: depconnect.EndpointEnvBindings{Address: "BACKEND_API_URL"},
		}},
	}

	resp, err := h.resolve(context.Background(), req, "user:alice")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if len(resp.Unconnectable) != 0 {
		t.Fatalf("unexpected unconnectable: %+v", resp.Unconnectable)
	}
	if len(resp.Targets) != 1 {
		t.Fatalf("expected 1 target, got %d: %+v", len(resp.Targets), resp.Targets)
	}

	// Endpoint render info.
	ep := findTarget(t, resp.Targets, "ep/doclet/backend-api/http")
	if ep.Endpoint == nil || ep.Endpoint.Scheme != httpStr || ep.Endpoint.BasePath != "/api" || ep.Endpoint.Bindings.Address != "BACKEND_API_URL" {
		t.Fatalf("bad endpoint render: %+v", ep.Endpoint)
	}

	// Capability verifies and carries the concrete (signed) dial target.
	assertResolveCapability(t, resp.Capability, priv)
}

// assertResolveCapability verifies the signed capability from TestResolveEndpoint and
// asserts its claims plus the endpoint dial target.
func assertResolveCapability(t *testing.T, capabilityToken string, priv ed25519.PrivateKey) {
	t.Helper()
	pub := priv.Public().(ed25519.PublicKey)
	claims, err := depconnect.VerifyCapability(capabilityToken, pub)
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
}

func TestResolveMissingDataPlaneRefFails(t *testing.T) {
	// Environment with no DataPlaneRef: resolution fails outright rather than
	// silently marking every dependency unconnectable.
	env := &openchoreov1alpha1.Environment{
		ObjectMeta: metav1.ObjectMeta{Name: "development", Namespace: "default"},
	}
	cl := fake.NewClientBuilder().WithScheme(depConnectScheme(t)).WithObjects(env).Build()
	_, priv, _ := ed25519.GenerateKey(nil)
	h := &DepConnectHandler{
		k8sClient: cl,
		signer:    &capabilitySigner{privKey: priv, ttl: time.Minute},
		logger:    slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	_, err := h.resolve(context.Background(), depconnect.ResolveRequest{
		Namespace: "default", Project: "doclet", Component: "doclet-document", Environment: "development",
	}, "user:alice")
	if err == nil {
		t.Fatal("expected an error when the environment has no data plane reference")
	}
}

func findTarget(t *testing.T, targets []depconnect.ResolvedTarget, key string) depconnect.ResolvedTarget {
	t.Helper()
	for _, tg := range targets {
		if tg.Key == key {
			return tg
		}
	}
	t.Fatalf("target %q not found in %+v", key, targets)
	return depconnect.ResolvedTarget{}
}
