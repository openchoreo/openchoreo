// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"io"
	"log/slog"
	"slices"
	"strings"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/config"
	"github.com/openchoreo/openchoreo/internal/remoteconnect"
)

// parseCertPEM parses a PEM certificate, failing the test if malformed. The explicit
// return after t.Fatal keeps the nil path visible to static analysis.
func parseCertPEM(t *testing.T, certPEM string) *x509.Certificate {
	t.Helper()
	block, _ := pem.Decode([]byte(certPEM))
	if block == nil {
		t.Fatal("no PEM block in certificate")
		return nil
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatalf("parse certificate: %v", err)
	}
	return cert
}

func TestGenerateSelfSignedCert(t *testing.T) {
	certPEM, keyPEM, err := generateSelfSignedCert("myproj-dev.remote-connect", time.Now())
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if _, err := tls.X509KeyPair([]byte(certPEM), []byte(keyPEM)); err != nil {
		t.Fatalf("keypair does not load: %v", err)
	}
	cert := parseCertPEM(t, certPEM)
	if len(cert.DNSNames) != 1 || cert.DNSNames[0] != "myproj-dev.remote-connect" {
		t.Fatalf("unexpected SANs: %v", cert.DNSNames)
	}
	// occ pins the cert and verifies against the SNI it was told to use.
	pool := x509.NewCertPool()
	pool.AddCert(cert)
	if _, err := cert.Verify(x509.VerifyOptions{DNSName: "myproj-dev.remote-connect", Roots: pool}); err != nil {
		t.Fatalf("cert does not verify against its SAN: %v", err)
	}
}

func provScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	if err := corev1.AddToScheme(s); err != nil {
		t.Fatal(err)
	}
	if err := appsv1.AddToScheme(s); err != nil {
		t.Fatal(err)
	}
	if err := rbacv1.AddToScheme(s); err != nil {
		t.Fatal(err)
	}
	return s
}

func TestEnsureAgentProvisionsClusterIPWithSNI(t *testing.T) {
	cfg := config.RemoteConnectConfig{
		AgentImage:        "ghcr.io/openchoreo/remote-agent:test",
		AgentListenPort:   8443,
		EntrypointAddress: "remoteconnect.example.com:8443",
		SNISuffix:         "remote-connect",
		AuthorizeURL:      "https://api.example.com/api/v1/remote-connect:authorize",
	}
	p := newRemoteAgentProvisioner(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	cl := fake.NewClientBuilder().WithScheme(provScheme(t)).Build()

	const ns = "dp-default-default-development-abc123"
	info, err := p.ensureAgent(context.Background(), cl, ns, nil)
	if err != nil {
		t.Fatalf("ensureAgent: %v", err)
	}

	wantSNI := ns + ".remote-connect"
	if info.endpoint != "remoteconnect.example.com:8443" {
		t.Errorf("endpoint = %q, want the shared entrypoint", info.endpoint)
	}
	if info.serverName != wantSNI {
		t.Errorf("serverName = %q, want %q", info.serverName, wantSNI)
	}
	if info.caBundle == "" {
		t.Error("expected a CA bundle (agent cert)")
	}

	// Service is ClusterIP and annotated with the SNI the router routes on.
	svc := &corev1.Service{}
	if err := cl.Get(context.Background(), client.ObjectKey{Namespace: ns, Name: remoteAgentName}, svc); err != nil {
		t.Fatalf("get service: %v", err)
	}
	if svc.Spec.Type != corev1.ServiceTypeClusterIP {
		t.Errorf("service type = %q, want ClusterIP", svc.Spec.Type)
	}
	if svc.Annotations[sniAnnotationKey] != wantSNI {
		t.Errorf("service SNI annotation = %q, want %q", svc.Annotations[sniAnnotationKey], wantSNI)
	}

	// The cert's SAN matches the SNI.
	block, _ := pem.Decode([]byte(info.caBundle))
	cert, _ := x509.ParseCertificate(block.Bytes)
	if len(cert.DNSNames) != 1 || cert.DNSNames[0] != wantSNI {
		t.Errorf("cert SAN = %v, want [%s]", cert.DNSNames, wantSNI)
	}
}

// TestAgentCertIsNotACA: the serving key must not be able to sign certs, and pinning must
// still work without IsCA.
func TestAgentCertIsNotACA(t *testing.T) {
	const sni = "myproj-dev.remote-connect"
	certPEM, keyPEM, err := generateSelfSignedCert(sni, time.Now())
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	cert := parseCertPEM(t, certPEM)
	if cert.IsCA {
		t.Error("agent serving cert must not be a CA")
	}
	if cert.KeyUsage&x509.KeyUsageCertSign != 0 {
		t.Error("agent serving cert must not carry KeyUsageCertSign")
	}

	// A real handshake with occ's pinning config must still succeed.
	srvCert, err := tls.X509KeyPair([]byte(certPEM), []byte(keyPEM))
	if err != nil {
		t.Fatalf("keypair: %v", err)
	}
	ln, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{Certificates: []tls.Certificate{srvCert}, MinVersion: tls.VersionTLS12})
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		c, aerr := ln.Accept()
		if aerr != nil {
			return
		}
		defer c.Close()
		_ = c.(*tls.Conn).Handshake()
		time.Sleep(200 * time.Millisecond)
	}()

	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM([]byte(certPEM)) {
		t.Fatal("cert not accepted into pool")
	}
	conn, err := tls.Dial("tcp", ln.Addr().String(), &tls.Config{RootCAs: pool, ServerName: sni, MinVersion: tls.VersionTLS12})
	if err != nil {
		t.Fatalf("pinned handshake failed with a non-CA cert: %v", err)
	}
	_ = conn.Close()
}

// storedPair reads back the cert/key the provisioner persisted for ns.
func storedPair(t *testing.T, cl client.Client, ns string) (cert, key []byte) {
	t.Helper()
	secret := &corev1.Secret{}
	if err := cl.Get(context.Background(), client.ObjectKey{Namespace: ns, Name: remoteAgentName}, secret); err != nil {
		t.Fatalf("get cert secret: %v", err)
	}
	return secret.Data[corev1.TLSCertKey], secret.Data[corev1.TLSPrivateKeyKey]
}

// storedCert reads back just the cert the provisioner persisted for ns.
func storedCert(t *testing.T, cl client.Client, ns string) string {
	t.Helper()
	cert, _ := storedPair(t, cl, ns)
	return string(cert)
}

// seedCertSecret stores a cert issued at issuedAt for sni (or rawCert verbatim), standing
// in for one left by an earlier run.
func seedCertSecret(t *testing.T, cl client.Client, ns, sni string, issuedAt time.Time, rawCert, rawKey string) {
	t.Helper()
	certPEM, keyPEM := rawCert, rawKey
	if rawCert == "" {
		var err error
		certPEM, keyPEM, err = generateSelfSignedCert(sni, issuedAt)
		if err != nil {
			t.Fatal(err)
		}
	}
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: remoteAgentName, Namespace: ns},
		Type:       corev1.SecretTypeTLS,
		Data: map[string][]byte{
			corev1.TLSCertKey:       []byte(certPEM),
			corev1.TLSPrivateKeyKey: []byte(keyPEM),
		},
	}
	if err := cl.Create(context.Background(), secret); err != nil {
		t.Fatalf("seed secret: %v", err)
	}
}

func certTestProvisioner(now time.Time) *remoteAgentProvisioner {
	p := newRemoteAgentProvisioner(config.RemoteConnectConfig{
		AgentImage: "img", AgentListenPort: 8443, EntrypointAddress: "e:8443", SNISuffix: "remote-connect",
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	p.now = func() time.Time { return now }
	return p
}

// TestEnsureCertSecretCertLifecycle covers reuse vs reissue: without reissue a long-lived
// agent would eventually serve an expired cert and occ would fail closed.
func TestEnsureCertSecretCertLifecycle(t *testing.T) {
	const ns = "dp-default-proj-development"
	now := time.Date(2026, 8, 17, 12, 0, 0, 0, time.UTC)
	sni := ns + ".remote-connect"

	tests := []struct {
		name        string
		issuedAt    time.Time
		rawCert     string
		rawKey      string
		wantReissue bool
	}{
		{"fresh cert is reused", now.Add(-24 * time.Hour), "", "", false},
		{"cert outside the renew window is reused", now.Add(-(certValidity - certRenewBefore - 24*time.Hour)), "", "", false},
		{"cert inside the renew window is reissued", now.Add(-(certValidity - certRenewBefore + time.Hour)), "", "", true},
		{"expired cert is reissued", now.Add(-2 * certValidity), "", "", true},
		{"unparseable cert is reissued", time.Time{}, "not a pem block", "not a key", true},
		{"cert for a different SNI is reissued", time.Time{}, "", "", true},
		// A pair that does not load would CrashLoopBackOff the agent forever.
		{"key not matching the cert is reissued", time.Time{}, "", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cl := fake.NewClientBuilder().WithScheme(provScheme(t)).Build()
			raw, rawKey := tt.rawCert, tt.rawKey
			switch tt.name {
			case "cert for a different SNI is reissued":
				c, k, err := generateSelfSignedCert("someone-else.remote-connect", now)
				if err != nil {
					t.Fatal(err)
				}
				raw, rawKey = c, k
			case "key not matching the cert is reissued":
				c, _, err := generateSelfSignedCert(sni, now)
				if err != nil {
					t.Fatal(err)
				}
				_, otherKey, err := generateSelfSignedCert(sni, now)
				if err != nil {
					t.Fatal(err)
				}
				raw, rawKey = c, otherKey
			}
			issuedAt := tt.issuedAt
			if issuedAt.IsZero() {
				issuedAt = now
			}
			seedCertSecret(t, cl, ns, sni, issuedAt, raw, rawKey)
			before := storedCert(t, cl, ns)

			p := certTestProvisioner(now)
			got, err := p.ensureCertSecret(context.Background(), cl, ns, sni)
			if err != nil {
				t.Fatalf("ensureCertSecret: %v", err)
			}

			after := storedCert(t, cl, ns)
			reissued := got != before
			if reissued != tt.wantReissue {
				t.Fatalf("reissued = %v, want %v", reissued, tt.wantReissue)
			}
			if got != after {
				t.Error("returned cert does not match what was stored")
			}
			// Whatever comes back must always be a usable pair.
			cert, key := storedPair(t, cl, ns)
			if !p.certUsable(cert, key, sni) {
				t.Error("stored cert/key is not usable for the agent's SNI")
			}
		})
	}
}

// TestEnsureCertSecretAdoptsConcurrentWinner: when another resolve creates the Secret
// between our Get and our write, we must return the stored cert, not the one we
// generated — occ pins what it is handed, and the pod mounts what is stored.
func TestEnsureCertSecretAdoptsConcurrentWinner(t *testing.T) {
	const ns = "dp-default-race-development"
	now := time.Date(2026, 8, 17, 12, 0, 0, 0, time.UTC)
	sni := ns + ".remote-connect"

	cl := fake.NewClientBuilder().WithScheme(provScheme(t)).
		WithInterceptorFuncs(interceptor.Funcs{
			// Simulate the other resolve landing first: seed the Secret on the first
			// Get miss, so our subsequent Create hits AlreadyExists.
			Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
				err := c.Get(ctx, key, obj, opts...)
				if apierrors.IsNotFound(err) {
					winner, winnerKey, gerr := generateSelfSignedCert(sni, now)
					if gerr != nil {
						return gerr
					}
					seedCertSecretOn(ctx, c, ns, winner, winnerKey)
				}
				return err
			},
		}).Build()

	p := certTestProvisioner(now)
	got, err := p.ensureCertSecret(context.Background(), cl, ns, sni)
	if err != nil {
		t.Fatalf("ensureCertSecret: %v", err)
	}
	if want := storedCert(t, cl, ns); got != want {
		t.Error("returned a cert the agent will not serve; expected the stored one")
	}
	cert, key := storedPair(t, cl, ns)
	if !p.certUsable(cert, key, sni) {
		t.Error("adopted cert/key is not usable for the agent's SNI")
	}
}

// seedCertSecretOn writes a cert Secret directly, standing in for a concurrent writer.
func seedCertSecretOn(ctx context.Context, c client.Client, ns, certPEM, keyPEM string) {
	_ = c.Create(ctx, &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: remoteAgentName, Namespace: ns},
		Type:       corev1.SecretTypeTLS,
		Data: map[string][]byte{
			corev1.TLSCertKey:       []byte(certPEM),
			corev1.TLSPrivateKeyKey: []byte(keyPEM),
		},
	})
}

// TestEnsureAgentDeploymentIsHardened: the agent runs in tenant namespaces, where a
// LimitRange or PodSecurity policy rejects a pod without these.
func TestEnsureAgentDeploymentIsHardened(t *testing.T) {
	p := certTestProvisioner(time.Now())
	cl := fake.NewClientBuilder().WithScheme(provScheme(t)).Build()
	const ns = "dp-default-proj-development"

	if _, err := p.ensureAgent(context.Background(), cl, ns, nil); err != nil {
		t.Fatalf("ensureAgent: %v", err)
	}

	dep := &appsv1.Deployment{}
	if err := cl.Get(context.Background(), client.ObjectKey{Namespace: ns, Name: remoteAgentName}, dep); err != nil {
		t.Fatalf("get deployment: %v", err)
	}
	pod := dep.Spec.Template.Spec
	c := pod.Containers[0]

	if c.Resources.Requests.Cpu().IsZero() || c.Resources.Requests.Memory().IsZero() {
		t.Error("container has no resource requests")
	}
	if c.SecurityContext == nil || c.SecurityContext.AllowPrivilegeEscalation == nil || *c.SecurityContext.AllowPrivilegeEscalation {
		t.Error("container must not allow privilege escalation")
	}
	if pod.SecurityContext == nil || pod.SecurityContext.RunAsNonRoot == nil || !*pod.SecurityContext.RunAsNonRoot {
		t.Error("pod must run as non-root")
	}
	if c.ReadinessProbe == nil || c.ReadinessProbe.TCPSocket == nil {
		t.Error("container has no readiness probe on the tunnel port")
	}
	if pod.SecurityContext.SeccompProfile == nil ||
		pod.SecurityContext.SeccompProfile.Type != corev1.SeccompProfileTypeRuntimeDefault {
		t.Error("pod does not use the runtime default seccomp profile")
	}
	// The agent does hold a token — it reads authorized Secrets/ConfigMaps through the
	// Kubernetes API. What keeps that safe is the Role, not the absence of a token, so
	// assert the token belongs to the agent's own dedicated ServiceAccount rather than
	// the namespace default (which may carry unrelated grants).
	if pod.ServiceAccountName != remoteAgentName {
		t.Errorf("pod must use its own service account, got %q", pod.ServiceAccountName)
	}
}

// The agent's Role must name the objects it may read and grant only `get`.
// resourceNames is the whole containment: it does not constrain list or watch, so
// granting either would silently restore namespace-wide read of every Secret.
func TestEnsureReadRBACIsNameScopedGetOnly(t *testing.T) {
	const ns = "dp-default-proj-development"
	cl := fake.NewClientBuilder().WithScheme(provScheme(t)).Build()
	p := certTestProvisioner(time.Date(2026, 8, 17, 12, 0, 0, 0, time.UTC))

	reads := &agentReadSet{}
	reads.add(remoteconnect.SourceKindSecret, "pg-conn")
	reads.add(remoteconnect.SourceKindConfigMap, "pg-tls")

	if _, err := p.ensureAgent(context.Background(), cl, ns, reads); err != nil {
		t.Fatalf("ensureAgent: %v", err)
	}

	role := &rbacv1.Role{}
	if err := cl.Get(context.Background(), client.ObjectKey{Namespace: ns, Name: remoteAgentName}, role); err != nil {
		t.Fatalf("get role: %v", err)
	}
	if len(role.Rules) != 2 {
		t.Fatalf("expected a rule per resource kind, got %+v", role.Rules)
	}
	for _, rule := range role.Rules {
		if !slices.Equal(rule.Verbs, []string{"get"}) {
			t.Errorf("rule on %v grants %v; only get is safe with resourceNames", rule.Resources, rule.Verbs)
		}
		if len(rule.ResourceNames) == 0 {
			t.Errorf("rule on %v names no objects, so it covers the whole namespace", rule.Resources)
		}
	}

	binding := &rbacv1.RoleBinding{}
	if err := cl.Get(context.Background(), client.ObjectKey{Namespace: ns, Name: remoteAgentName}, binding); err != nil {
		t.Fatalf("get role binding: %v", err)
	}
	if len(binding.Subjects) != 1 || binding.Subjects[0].Name != remoteAgentName {
		t.Fatalf("binding does not target the agent's service account: %+v", binding.Subjects)
	}
}

// A session that only tunnels must not leave the agent holding read permissions it has
// no use for, so no Role is created when nothing is to be read.
// A tunnel-only session gets the ServiceAccount but no Role or RoleBinding.
func TestEnsureReadRBACSkippedWhenNothingToRead(t *testing.T) {
	const ns = "dp-default-proj-development"
	cl := fake.NewClientBuilder().WithScheme(provScheme(t)).Build()
	p := certTestProvisioner(time.Date(2026, 8, 17, 12, 0, 0, 0, time.UTC))

	if _, err := p.ensureAgent(context.Background(), cl, ns, nil); err != nil {
		t.Fatalf("ensureAgent: %v", err)
	}
	role := &rbacv1.Role{}
	err := cl.Get(context.Background(), client.ObjectKey{Namespace: ns, Name: remoteAgentName}, role)
	if !apierrors.IsNotFound(err) {
		t.Fatalf("expected no Role for a tunnel-only session, got err=%v role=%+v", err, role.Rules)
	}
	binding := &rbacv1.RoleBinding{}
	if err := cl.Get(context.Background(), client.ObjectKey{Namespace: ns, Name: remoteAgentName}, binding); !apierrors.IsNotFound(err) {
		t.Fatalf("expected no RoleBinding for a tunnel-only session, got err=%v", err)
	}
	sa := &corev1.ServiceAccount{}
	if err := cl.Get(context.Background(), client.ObjectKey{Namespace: ns, Name: remoteAgentName}, sa); err != nil {
		t.Fatalf("tunnel-only session must still get the ServiceAccount the Deployment names: %v", err)
	}
}

// The Deployment's serviceAccountName names an existing account for every read set.
func TestEnsureAgentServiceAccountAlwaysExistsForDeployment(t *testing.T) {
	const ns = "dp-default-proj-development"
	for _, tc := range []struct {
		name  string
		reads *agentReadSet
	}{
		{"tunnel only", nil},
		{"empty read set", &agentReadSet{}},
		{"with reads", &agentReadSet{secrets: []string{"db-creds"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cl := fake.NewClientBuilder().WithScheme(provScheme(t)).Build()
			p := certTestProvisioner(time.Date(2026, 8, 17, 12, 0, 0, 0, time.UTC))

			if _, err := p.ensureAgent(context.Background(), cl, ns, tc.reads); err != nil {
				t.Fatalf("ensureAgent: %v", err)
			}

			deploy := &appsv1.Deployment{}
			if err := cl.Get(context.Background(), client.ObjectKey{Namespace: ns, Name: remoteAgentName}, deploy); err != nil {
				t.Fatalf("get deployment: %v", err)
			}
			saName := deploy.Spec.Template.Spec.ServiceAccountName
			if saName == "" {
				t.Fatal("deployment does not set serviceAccountName")
			}
			sa := &corev1.ServiceAccount{}
			if err := cl.Get(context.Background(), client.ObjectKey{Namespace: ns, Name: saName}, sa); err != nil {
				t.Fatalf("deployment names ServiceAccount %q, which does not exist: %v", saName, err)
			}
		})
	}
}

// A second resolve in the same namespace must not revoke the first's grant: replacing
// resourceNames would break a concurrent session's fetch, which the affected developer
// would see as a secret that mysteriously stopped resolving.
func TestEnsureReadRBACIsAdditiveAcrossResolves(t *testing.T) {
	const ns = "dp-default-proj-development"
	cl := fake.NewClientBuilder().WithScheme(provScheme(t)).Build()
	p := certTestProvisioner(time.Date(2026, 8, 17, 12, 0, 0, 0, time.UTC))

	first := &agentReadSet{}
	first.add(remoteconnect.SourceKindSecret, "session-a")
	if _, err := p.ensureAgent(context.Background(), cl, ns, first); err != nil {
		t.Fatalf("first ensureAgent: %v", err)
	}

	second := &agentReadSet{}
	second.add(remoteconnect.SourceKindSecret, "session-b")
	if _, err := p.ensureAgent(context.Background(), cl, ns, second); err != nil {
		t.Fatalf("second ensureAgent: %v", err)
	}

	role := &rbacv1.Role{}
	if err := cl.Get(context.Background(), client.ObjectKey{Namespace: ns, Name: remoteAgentName}, role); err != nil {
		t.Fatalf("get role: %v", err)
	}
	names := make([]string, 0, len(role.Rules))
	for _, rule := range role.Rules {
		names = append(names, rule.ResourceNames...)
	}
	for _, want := range []string{"session-a", "session-b"} {
		if !slices.Contains(names, want) {
			t.Errorf("role dropped %q; names=%v", want, names)
		}
	}
}

// TestEnsureAgentRollsPodWhenCertRotates: the agent loads its keypair at start-up, so a
// reissued cert must change the pod template.
func TestEnsureAgentRollsPodWhenCertRotates(t *testing.T) {
	const ns = "dp-default-proj-development"
	now := time.Date(2026, 8, 17, 12, 0, 0, 0, time.UTC)
	cl := fake.NewClientBuilder().WithScheme(provScheme(t)).Build()

	p := certTestProvisioner(now)
	if _, err := p.ensureAgent(context.Background(), cl, ns, nil); err != nil {
		t.Fatalf("first ensureAgent: %v", err)
	}
	dep := &appsv1.Deployment{}
	if err := cl.Get(context.Background(), client.ObjectKey{Namespace: ns, Name: remoteAgentName}, dep); err != nil {
		t.Fatal(err)
	}
	first := dep.Spec.Template.Annotations[certAnnotation]
	if first == "" {
		t.Fatal("pod template carries no cert annotation")
	}

	// Same cert on a second call: the pod must not roll.
	if _, err := p.ensureAgent(context.Background(), cl, ns, nil); err != nil {
		t.Fatalf("second ensureAgent: %v", err)
	}
	if err := cl.Get(context.Background(), client.ObjectKey{Namespace: ns, Name: remoteAgentName}, dep); err != nil {
		t.Fatal(err)
	}
	if got := dep.Spec.Template.Annotations[certAnnotation]; got != first {
		t.Errorf("annotation changed without a cert change: %q -> %q", first, got)
	}

	// Advance past the renewal window: the cert is reissued and the pod must follow.
	rotated := certTestProvisioner(now.Add(certValidity - certRenewBefore + time.Hour))
	if _, err := rotated.ensureAgent(context.Background(), cl, ns, nil); err != nil {
		t.Fatalf("rotating ensureAgent: %v", err)
	}
	if err := cl.Get(context.Background(), client.ObjectKey{Namespace: ns, Name: remoteAgentName}, dep); err != nil {
		t.Fatal(err)
	}
	if got := dep.Spec.Template.Annotations[certAnnotation]; got == first {
		t.Error("cert was reissued but the pod template did not change, so the agent keeps serving the old cert")
	}
}

// TestApplyDeploymentHeartbeatURLDerivation: the heartbeat URL is derived by swapping the
// authorize path. A non-standard authorizeUrl silently drops the flag, and without
// heartbeats a paused session gets reaped mid-use — so the behavior is pinned here.
func TestApplyDeploymentHeartbeatURLDerivation(t *testing.T) {
	tests := []struct {
		name          string
		authorizeURL  string
		wantHeartbeat string
	}{
		{
			name:          "canonical path yields a heartbeat flag",
			authorizeURL:  "https://api.example.com" + remoteconnect.AuthorizePath,
			wantHeartbeat: "--heartbeat-url=https://api.example.com" + remoteconnect.HeartbeatPath,
		},
		{
			name:         "non-standard path drops the flag rather than guessing",
			authorizeURL: "https://api.example.com/custom/authorize",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := newRemoteAgentProvisioner(config.RemoteConnectConfig{
				AgentImage: "img", AgentListenPort: 8443, EntrypointAddress: "e:8443",
				SNISuffix: "remote-connect", AuthorizeURL: tt.authorizeURL,
			}, slog.New(slog.NewTextHandler(io.Discard, nil)))
			cl := fake.NewClientBuilder().WithScheme(provScheme(t)).Build()
			const ns = "dp-default-proj-development"

			if err := p.applyDeployment(context.Background(), cl, ns, "cert"); err != nil {
				t.Fatalf("applyDeployment: %v", err)
			}
			dep := &appsv1.Deployment{}
			if err := cl.Get(context.Background(), client.ObjectKey{Namespace: ns, Name: remoteAgentName}, dep); err != nil {
				t.Fatal(err)
			}
			args := dep.Spec.Template.Spec.Containers[0].Args

			var heartbeat string
			for _, a := range args {
				if strings.HasPrefix(a, "--heartbeat-url=") {
					heartbeat = a
				}
			}
			if heartbeat != tt.wantHeartbeat {
				t.Errorf("heartbeat arg = %q, want %q (args: %v)", heartbeat, tt.wantHeartbeat, args)
			}
		})
	}
}

// A conflicting write is retried against a fresh read, keeping every session's names.
func TestEnsureReadRBACRetriesOnConflict(t *testing.T) {
	const ns = "dp-default-proj-development"
	first := &agentReadSet{secrets: []string{"first-secret"}}
	second := &agentReadSet{secrets: []string{"second-secret"}}

	var updates int
	cl := fake.NewClientBuilder().WithScheme(provScheme(t)).
		WithInterceptorFuncs(interceptor.Funcs{
			Update: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.UpdateOption) error {
				role, isRole := obj.(*rbacv1.Role)
				if !isRole {
					return c.Update(ctx, obj, opts...)
				}
				updates++
				// A racing resolve commits between this caller's read and write.
				if updates == 1 {
					live := &rbacv1.Role{}
					if err := c.Get(ctx, client.ObjectKeyFromObject(role), live); err != nil {
						return err
					}
					// The racer merges additively, as ensureReadRBAC does.
					names := []string{"racer-secret"}
					for _, rule := range live.Rules {
						names = append(names, rule.ResourceNames...)
					}
					slices.Sort(names)
					live.Rules = []rbacv1.PolicyRule{{
						APIGroups: []string{""}, Resources: []string{"secrets"},
						Verbs: []string{"get"}, ResourceNames: names,
					}}
					if err := c.Update(ctx, live); err != nil {
						return err
					}
					return apierrors.NewConflict(
						schema.GroupResource{Group: rbacv1.GroupName, Resource: "roles"}, role.Name,
						errors.New("simulated conflict"))
				}
				return c.Update(ctx, obj, opts...)
			},
		}).Build()

	p := certTestProvisioner(time.Date(2026, 8, 17, 12, 0, 0, 0, time.UTC))
	if _, err := p.ensureAgent(context.Background(), cl, ns, first); err != nil {
		t.Fatalf("first ensureAgent: %v", err)
	}
	if _, err := p.ensureAgent(context.Background(), cl, ns, second); err != nil {
		t.Fatalf("second ensureAgent: %v", err)
	}

	role := &rbacv1.Role{}
	if err := cl.Get(context.Background(), client.ObjectKey{Namespace: ns, Name: remoteAgentName}, role); err != nil {
		t.Fatalf("get role: %v", err)
	}
	got := make([]string, 0, len(role.Rules))
	for _, rule := range role.Rules {
		got = append(got, rule.ResourceNames...)
	}
	// Both sessions' names and the racer's survive.
	for _, want := range []string{"first-secret", "second-secret", "racer-secret"} {
		if !slices.Contains(got, want) {
			t.Fatalf("resourceNames %v lost %q after the conflict retry", got, want)
		}
	}
}

// Two resolves racing the first create: the loser must re-read and merge, not fail.
func TestEnsureReadRBACRetriesOnCreateRace(t *testing.T) {
	const ns = "dp-default-proj-development"
	mine := &agentReadSet{secrets: []string{"mine-secret"}}

	var roleCreates int
	cl := fake.NewClientBuilder().WithScheme(provScheme(t)).
		WithInterceptorFuncs(interceptor.Funcs{
			Create: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
				role, isRole := obj.(*rbacv1.Role)
				if !isRole {
					return c.Create(ctx, obj, opts...)
				}
				roleCreates++
				if roleCreates > 1 {
					return c.Create(ctx, obj, opts...)
				}
				// A racing resolve creates the Role first, so this create loses.
				racer := &rbacv1.Role{
					ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: remoteAgentName},
					Rules: []rbacv1.PolicyRule{{
						APIGroups: []string{""}, Resources: []string{"secrets"},
						Verbs: []string{"get"}, ResourceNames: []string{"racer-secret"},
					}},
				}
				if err := c.Create(ctx, racer); err != nil {
					return err
				}
				return apierrors.NewAlreadyExists(
					schema.GroupResource{Group: rbacv1.GroupName, Resource: "roles"}, role.Name)
			},
		}).Build()

	p := certTestProvisioner(time.Date(2026, 8, 17, 12, 0, 0, 0, time.UTC))
	if _, err := p.ensureAgent(context.Background(), cl, ns, mine); err != nil {
		t.Fatalf("ensureAgent lost the create race: %v", err)
	}

	role := &rbacv1.Role{}
	if err := cl.Get(context.Background(), client.ObjectKey{Namespace: ns, Name: remoteAgentName}, role); err != nil {
		t.Fatalf("get role: %v", err)
	}
	got := make([]string, 0, len(role.Rules))
	for _, rule := range role.Rules {
		got = append(got, rule.ResourceNames...)
	}
	for _, want := range []string{"mine-secret", "racer-secret"} {
		if !slices.Contains(got, want) {
			t.Fatalf("resourceNames %v lost %q after the create race", got, want)
		}
	}
}
