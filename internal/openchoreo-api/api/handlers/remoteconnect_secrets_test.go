// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	authzcore "github.com/openchoreo/openchoreo/internal/authz/core"
	svcpkg "github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	"github.com/openchoreo/openchoreo/internal/remoteconnect"
)

// actionPDP allows everything except the named actions. It exists because the
// interesting case here is a caller who may connect to a resource but may not read its
// credentials — a distinction recordingPDP (which decides on hierarchy alone) cannot
// express.
const docletDB = "doclet"

type actionPDP struct {
	deny map[string]bool
}

func (p *actionPDP) decision(action string) authzcore.Decision {
	return authzcore.Decision{Decision: !p.deny[action], Context: &authzcore.DecisionContext{}}
}

func (p *actionPDP) Evaluate(_ context.Context, req *authzcore.EvaluateRequest) (*authzcore.Decision, error) {
	d := p.decision(req.Action)
	return &d, nil
}

func (p *actionPDP) BatchEvaluate(_ context.Context, req *authzcore.BatchEvaluateRequest) (*authzcore.BatchEvaluateResponse, error) {
	resp := &authzcore.BatchEvaluateResponse{Decisions: make([]authzcore.Decision, 0, len(req.Requests))}
	for i := range req.Requests {
		resp.Decisions = append(resp.Decisions, p.decision(req.Requests[i].Action))
	}
	return resp, nil
}

func (p *actionPDP) GetSubjectProfile(context.Context, *authzcore.ProfileRequest) (*authzcore.UserCapabilitiesResponse, error) {
	return &authzcore.UserCapabilitiesResponse{}, nil
}

// secretResourceFixture builds a ready resource whose outputs cover all three kinds: a
// plain value, a Secret reference, and a ConfigMap reference. It declares no endpoint, so
// the tests below isolate value resolution from tunneling.
func secretResourceFixture() *openchoreov1alpha1.ResourceReleaseBinding {
	return &openchoreov1alpha1.ResourceReleaseBinding{
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
				{Name: "database", Value: docletDB},
				{Name: "password", SecretKeyRef: &openchoreov1alpha1.SecretKeyRef{Name: "pg-secret", Key: "password"}},
				{Name: "caCert", ConfigMapKeyRef: &openchoreov1alpha1.ConfigMapKeyRef{Name: "pg-tls", Key: "ca.crt"}},
			},
		},
	}
}

// resolveWithSecrets runs resolve against the fixture with a handler configured by the
// caller, and returns the response plus the verified capability claims.
func resolveWithSecrets(t *testing.T, pdp authzcore.PDP, secretsEnabled bool,
	dep remoteconnect.ResourceDep) (*remoteconnect.ResolveResponse, *remoteconnect.CapabilityClaims) {
	t.Helper()
	env, dp := testEnvironmentAndDataPlane()
	cl := fake.NewClientBuilder().WithScheme(remoteConnectScheme(t)).
		WithObjects(env, dp, secretResourceFixture()).Build()

	_, priv, _ := ed25519.GenerateKey(nil)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	h := &RemoteConnectHandler{
		k8sClient:      cl,
		authzChecker:   svcpkg.NewAuthzChecker(pdp, logger),
		signer:         &capabilitySigner{privKey: priv, keyID: "k1", issuer: "cp", ttl: 30 * time.Minute},
		secretsEnabled: secretsEnabled,
		logger:         logger,
	}

	resp, err := h.resolve(context.Background(), remoteconnect.ResolveRequest{
		Namespace: "default", Project: "doclet", Component: "doclet-document", Environment: "development",
		Resources: []remoteconnect.ResourceDep{dep},
	}, "user:alice")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	claims, verr := remoteconnect.VerifyCapability(resp.Capability, priv.Public().(ed25519.PublicKey))
	if verr != nil {
		t.Fatalf("verify capability: %v", verr)
	}
	return resp, claims
}

func onlyBindings(t *testing.T, resp *remoteconnect.ResolveResponse) remoteconnect.ResourceBindings {
	t.Helper()
	if len(resp.Resources) != 1 {
		t.Fatalf("expected bindings for one resource, got %+v", resp.Resources)
	}
	return resp.Resources[0]
}

// The happy path: an authorized caller gets a fetch key per ref-backed binding, and the
// capability carries the coordinates — never the value.
func TestResolveSignsSecretGrants(t *testing.T) {
	resp, claims := resolveWithSecrets(t, &actionPDP{}, true, remoteconnect.ResourceDep{
		Ref:          docletPostgres,
		EnvBindings:  map[string]string{"database": "DB_NAME", "password": "DB_PASSWORD"},
		FileBindings: map[string]string{"caCert": "/etc/tls/ca.crt"},
	})

	rb := onlyBindings(t, resp)
	if rb.StaticEnv["DB_NAME"] != docletDB {
		t.Errorf("plain value should still resolve directly: %+v", rb.StaticEnv)
	}
	if want := remoteconnect.SecretGrantKey(docletPostgres, "password"); rb.FetchEnv["DB_PASSWORD"] != want {
		t.Errorf("DB_PASSWORD should fetch %q, got %+v", want, rb.FetchEnv)
	}
	if want := remoteconnect.SecretGrantKey(docletPostgres, "caCert"); rb.FetchFile["/etc/tls/ca.crt"] != want {
		t.Errorf("file binding should fetch %q, got %+v", want, rb.FetchFile)
	}
	if len(rb.OmittedSecretEnv) != 0 {
		t.Errorf("nothing should be omitted: %+v", rb.OmittedSecretEnv)
	}

	if len(claims.Secrets) != 2 {
		t.Fatalf("expected two grants in the capability, got %+v", claims.Secrets)
	}
	for _, g := range claims.Secrets {
		if g.AgentNamespace == "" {
			t.Errorf("grant %q has no agent namespace, so the agent cannot enforce ownership", g.Key)
		}
		if g.SourceName == "" || g.SourceKey == "" {
			t.Errorf("grant %q is missing coordinates: %+v", g.Key, g)
		}
	}
}

// A caller who may connect to the resource but not read its secrets gets the ConfigMap
// value and a reasoned omission for the Secret one. Connecting and extracting the
// credential are separate grants, and this is the test that they are actually separable.
func TestResolveDeniesSecretButAllowsConfigMap(t *testing.T) {
	pdp := &actionPDP{deny: map[string]bool{authzcore.ActionReadResourceSecrets: true}}
	resp, claims := resolveWithSecrets(t, pdp, true, remoteconnect.ResourceDep{
		Ref:          docletPostgres,
		EnvBindings:  map[string]string{"password": "DB_PASSWORD", "caCert": "CA_PEM"},
		FileBindings: nil,
	})

	rb := onlyBindings(t, resp)
	if _, granted := rb.FetchEnv["DB_PASSWORD"]; granted {
		t.Error("DB_PASSWORD was granted despite resource:read-secrets being denied")
	}
	if want := remoteconnect.SecretGrantKey(docletPostgres, "caCert"); rb.FetchEnv["CA_PEM"] != want {
		t.Errorf("a ConfigMap value must not need resource:read-secrets, got %+v", rb.FetchEnv)
	}

	var omitted *remoteconnect.OmittedBinding
	for i := range rb.OmittedSecretEnv {
		if rb.OmittedSecretEnv[i].Target == "DB_PASSWORD" {
			omitted = &rb.OmittedSecretEnv[i]
		}
	}
	if omitted == nil {
		t.Fatalf("DB_PASSWORD was neither granted nor reported: %+v", rb)
	}
	if !strings.Contains(omitted.Reason, authzcore.ActionReadResourceSecrets) {
		t.Errorf("omission does not say which grant is missing: %q", omitted.Reason)
	}

	// The denied output must not appear in the capability at all: a grant signed but
	// unreferenced would still authorize the read.
	for _, g := range claims.Secrets {
		if g.SourceName == "pg-secret" {
			t.Errorf("a denied secret was signed into the capability: %+v", g)
		}
	}
}

// Denying resource:connect keeps the whole dependency out, so the secret question is
// never even asked — a caller who cannot reach the resource must not learn what its
// outputs are called.
func TestResolveDeniedResourceGrantsNoSecrets(t *testing.T) {
	pdp := &actionPDP{deny: map[string]bool{authzcore.ActionConnectResource: true}}
	resp, claims := resolveWithSecrets(t, pdp, true, remoteconnect.ResourceDep{
		Ref:         docletPostgres,
		EnvBindings: map[string]string{"password": "DB_PASSWORD"},
	})

	if len(resp.Resources) != 0 {
		t.Errorf("an unauthorized resource contributed bindings: %+v", resp.Resources)
	}
	if len(claims.Secrets) != 0 {
		t.Errorf("an unauthorized resource produced grants: %+v", claims.Secrets)
	}
	if len(resp.Unconnectable) != 1 || resp.Unconnectable[0].Ref != remoteconnect.ResourceRefKey(docletPostgres) {
		t.Errorf("expected the resource reported unconnectable, got %+v", resp.Unconnectable)
	}
}

// The operator kill switch overrides policy: with it off, no grant is signed however the
// roles are configured.
func TestResolveSecretsDisabledSignsNoGrants(t *testing.T) {
	resp, claims := resolveWithSecrets(t, &actionPDP{}, false, remoteconnect.ResourceDep{
		Ref:         docletPostgres,
		EnvBindings: map[string]string{"database": "DB_NAME", "password": "DB_PASSWORD"},
	})

	if len(claims.Secrets) != 0 {
		t.Errorf("grants signed while secrets are disabled: %+v", claims.Secrets)
	}
	rb := onlyBindings(t, resp)
	if len(rb.FetchEnv) != 0 {
		t.Errorf("fetch keys emitted while secrets are disabled: %+v", rb.FetchEnv)
	}
	// The plain value is unaffected — the switch is about references, not resolution.
	if rb.StaticEnv["DB_NAME"] != docletDB {
		t.Errorf("plain value should still resolve: %+v", rb.StaticEnv)
	}
	if len(rb.OmittedSecretEnv) != 1 || !strings.Contains(rb.OmittedSecretEnv[0].Reason, "secrets_enabled") {
		t.Errorf("omission should name the switch, got %+v", rb.OmittedSecretEnv)
	}
}

// One output bound to both an env var and a file yields one grant, referenced twice. Two
// grants for the same coordinates would double the agent's Role entries and the audit
// trail for a single authorization.
func TestResolveDeduplicatesGrantPerOutput(t *testing.T) {
	resp, claims := resolveWithSecrets(t, &actionPDP{}, true, remoteconnect.ResourceDep{
		Ref:          docletPostgres,
		EnvBindings:  map[string]string{"caCert": "CA_PEM"},
		FileBindings: map[string]string{"caCert": "/etc/tls/ca.crt"},
	})

	if len(claims.Secrets) != 1 {
		t.Fatalf("expected one grant for one output, got %+v", claims.Secrets)
	}
	rb := onlyBindings(t, resp)
	key := remoteconnect.SecretGrantKey(docletPostgres, "caCert")
	if rb.FetchEnv["CA_PEM"] != key || rb.FetchFile["/etc/tls/ca.crt"] != key {
		t.Errorf("both bindings should reference the same key: env=%+v file=%+v", rb.FetchEnv, rb.FetchFile)
	}
}

// A capability carrying grants expires sooner than a tunnel-only one, because the
// per-stream callback re-checks no policy and the expiry is the whole revocation window.
func TestSignerTightensTTLForSecretGrants(t *testing.T) {
	_, priv, _ := ed25519.GenerateKey(nil)
	s := &capabilitySigner{privKey: priv, keyID: "k1", issuer: "cp", ttl: time.Hour, secretTTL: 10 * time.Minute}
	comp := remoteconnect.ComponentRef{Project: "doclet", Name: "doc"}
	pub := priv.Public().(ed25519.PublicKey)

	tunnelOnly, err := s.sign("user:alice", "default", comp, "development",
		[]remoteconnect.Target{{Key: "ep/a/b", Host: "h", Port: 1}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	withGrants, err := s.sign("user:alice", "default", comp, "development", nil,
		[]remoteconnect.SecretGrant{{Key: "sec/r/o", SourceKind: remoteconnect.SourceKindSecret, SourceName: "s", SourceKey: "k"}})
	if err != nil {
		t.Fatal(err)
	}

	a, err := remoteconnect.VerifyCapability(tunnelOnly, pub)
	if err != nil {
		t.Fatal(err)
	}
	b, err := remoteconnect.VerifyCapability(withGrants, pub)
	if err != nil {
		t.Fatal(err)
	}
	if !b.ExpiresAt.Before(a.ExpiresAt.Time) {
		t.Errorf("secret-bearing capability (%v) should expire before a tunnel-only one (%v)",
			b.ExpiresAt, a.ExpiresAt)
	}
}

// The authorize callback must resolve a key in exactly one space, chosen by its prefix.
// A fetch key that happens to name a signed dial target must not resolve: searching both
// tables is what would let a caller convert one kind of grant into the other.
func TestAuthorizeKeySpacesDoNotCross(t *testing.T) {
	_, priv, _ := ed25519.GenerateKey(nil)
	pub := priv.Public().(ed25519.PublicKey)
	signer := &capabilitySigner{privKey: priv, keyID: "k1", issuer: "cp", ttl: time.Minute}
	comp := remoteconnect.ComponentRef{Project: "doclet", Name: "doc"}

	fetchKey := remoteconnect.SecretGrantKey(docletPostgres, "password")
	// The same key is signed as a DIAL target, not as a grant.
	token, err := signer.sign("user:alice", "default", comp, "development",
		[]remoteconnect.Target{{Key: fetchKey, Proto: "tcp", Host: "pg", Port: 5432, AgentNamespace: "dp-ns"}}, nil)
	if err != nil {
		t.Fatal(err)
	}

	h := NewRemoteConnectAuthorizeHandler(pub, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	rec := postAuthorize(t, h, remoteconnect.AuthorizeRequest{Capability: token, Key: fetchKey})
	if rec.Code != http.StatusForbidden {
		t.Fatalf("a fetch key must not resolve from the dial table; status=%d body=%s",
			rec.Code, rec.Body.String())
	}
}

// The mirror: a dial key must not resolve from the grant table.
func TestAuthorizeDialKeyDoesNotResolveFromGrants(t *testing.T) {
	_, priv, _ := ed25519.GenerateKey(nil)
	pub := priv.Public().(ed25519.PublicKey)
	signer := &capabilitySigner{privKey: priv, keyID: "k1", issuer: "cp", ttl: time.Minute}
	comp := remoteconnect.ComponentRef{Project: "doclet", Name: "doc"}

	token, err := signer.sign("user:alice", "default", comp, "development", nil,
		[]remoteconnect.SecretGrant{{
			Key: "ep/doclet/backend-api/http", AgentNamespace: "dp-ns",
			SourceKind: remoteconnect.SourceKindSecret, SourceName: "pg-secret", SourceKey: "password",
		}})
	if err != nil {
		t.Fatal(err)
	}

	h := NewRemoteConnectAuthorizeHandler(pub, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	rec := postAuthorize(t, h, remoteconnect.AuthorizeRequest{Capability: token, Key: "ep/doclet/backend-api/http"})
	if rec.Code != http.StatusForbidden {
		t.Fatalf("a dial key must not resolve from the grant table; status=%d body=%s",
			rec.Code, rec.Body.String())
	}
}

// A granted fetch key returns the coordinates and never a value: the endpoint holds no
// data-plane client, so there is nothing for it to leak, and the response shape must keep
// it that way.
func TestAuthorizeReturnsGrantCoordinatesOnly(t *testing.T) {
	_, priv, _ := ed25519.GenerateKey(nil)
	pub := priv.Public().(ed25519.PublicKey)
	signer := &capabilitySigner{privKey: priv, keyID: "k1", issuer: "cp", ttl: time.Minute}
	comp := remoteconnect.ComponentRef{Project: "doclet", Name: "doc"}

	key := remoteconnect.SecretGrantKey(docletPostgres, "password")
	token, err := signer.sign("user:alice", "default", comp, "development", nil,
		[]remoteconnect.SecretGrant{{
			Key: key, AgentNamespace: "dp-ns",
			SourceKind: remoteconnect.SourceKindSecret, SourceName: "pg-secret", SourceKey: "password",
		}})
	if err != nil {
		t.Fatal(err)
	}

	h := NewRemoteConnectAuthorizeHandler(pub, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	rec := postAuthorize(t, h, remoteconnect.AuthorizeRequest{Capability: token, Key: key})
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var resp remoteconnect.AuthorizeResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Kind != remoteconnect.AuthorizeKindSecret {
		t.Errorf("kind = %q, want %q", resp.Kind, remoteconnect.AuthorizeKindSecret)
	}
	if resp.Secret == nil || resp.Secret.SourceName != "pg-secret" || resp.Secret.SourceKey != "password" {
		t.Fatalf("coordinates missing: %+v", resp.Secret)
	}
	if resp.Host != "" || resp.Port != 0 {
		t.Errorf("a grant answer must carry no dial target: %+v", resp)
	}
}

// A binding naming an output the resource does not publish -- a typo, or a workload
// written against a different release. The deployed path fails the whole dependency on
// this, so the report must at least say why the env var and the file are missing.
func TestResolveReportsBindingToUnknownOutput(t *testing.T) {
	resp, _ := resolveWithSecrets(t, &actionPDP{}, true, remoteconnect.ResourceDep{
		Ref:          docletPostgres,
		EnvBindings:  map[string]string{"database": "DB_NAME", "databse": "DB_TYPO"},
		FileBindings: map[string]string{"nosuch": "/etc/tls/missing.crt"},
	})

	rb := onlyBindings(t, resp)
	if _, set := rb.StaticEnv["DB_TYPO"]; set {
		t.Errorf("an unpublished output must not produce an env var: %+v", rb.StaticEnv)
	}
	if rb.StaticEnv["DB_NAME"] != docletDB {
		t.Errorf("the correctly named binding still resolves: %+v", rb.StaticEnv)
	}

	byTarget := make(map[string]remoteconnect.OmittedBinding, len(rb.OmittedSecretEnv))
	for _, o := range rb.OmittedSecretEnv {
		byTarget[o.Target] = o
	}
	env, ok := byTarget["DB_TYPO"]
	if !ok {
		t.Fatalf("DB_TYPO should be reported, got %+v", rb.OmittedSecretEnv)
	}
	if !strings.Contains(env.Reason, `"databse"`) {
		t.Errorf("reason should name the output that does not exist, got %q", env.Reason)
	}
	if env.File {
		t.Error("an env binding must not be reported as a file")
	}
	file, ok := byTarget["/etc/tls/missing.crt"]
	if !ok {
		t.Fatalf("the file binding should be reported, got %+v", rb.OmittedSecretEnv)
	}
	if !file.File || !strings.Contains(file.Reason, `"nosuch"`) {
		t.Errorf("file omission should be marked and named, got %+v", file)
	}
}
