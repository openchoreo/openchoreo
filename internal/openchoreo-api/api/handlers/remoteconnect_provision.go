// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"log/slog"
	"math/big"
	"strconv"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openchoreo/openchoreo/internal/labels"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/config"
	"github.com/openchoreo/openchoreo/internal/remoteconnect"
)

const (
	// remoteAgentName is the fixed name of the remote-agent Deployment/Service/Secret within
	// a project+env data-plane namespace (the namespace already scopes it, so one
	// fixed name per namespace is unique).
	remoteAgentName = "openchoreo-remote-agent"
	// sniAnnotationKey holds each remote-agent Service's SNI host; the shared remote-connect
	// SNI router reads it to route occ's connection to this agent. Must match the
	// router's --sni-annotation (remoteagentrouter.DefaultSNIAnnotationKey).
	sniAnnotationKey = "openchoreo.dev/remote-connect-sni"
	// defaultSNISuffix is appended to the data-plane namespace to form an agent's SNI
	// host when remote_connect.sni_suffix is unset.
	defaultSNISuffix = "remote-connect"
	// remoteAgentFieldOwner is the server-side-apply field manager for provisioned resources.
	remoteAgentFieldOwner = "openchoreo-api-remote-connect"
	// lastUsedAnnotation records the last time a resolve refreshed this remote-agent; the
	// reaper deletes agents idle past the TTL.
	lastUsedAnnotation = "openchoreo.dev/remote-connect-last-used"
	// managedByLabelValue marks resources the remote-connect provisioner owns.
	managedByLabelValue = "openchoreo-api-remote-connect"
	// certValidity is the self-signed agent certificate lifetime.
	certValidity = 90 * 24 * time.Hour
	// certAnnotation carries the served cert's digest on the agent pod template.
	certAnnotation = "openchoreo.dev/remote-connect-cert"
	// certRenewBefore is how long before expiry a stored agent cert is reissued, so a
	// long-lived agent never serves an expired one.
	certRenewBefore = 30 * 24 * time.Hour
)

// agentEndpointInfo is what a provisioned remote-agent exposes to occ.
type agentEndpointInfo struct {
	endpoint   string // host:port occ dials
	caBundle   string // PEM cert occ pins
	serverName string // SAN occ verifies against
}

// remoteAgentProvisioner imperatively creates/updates a per-project+env remote-agent
// (Deployment + L4 Service + cert Secret) in the data-plane namespace and reads back
// its external address. Lifecycle is imperative (no CRD/controller): resolve
// provisions on demand and stamps a last-used annotation; the reaper GCs idle agents.
type remoteAgentProvisioner struct {
	cfg    config.RemoteConnectConfig
	now    func() time.Time
	logger *slog.Logger
}

func newRemoteAgentProvisioner(cfg config.RemoteConnectConfig, logger *slog.Logger) *remoteAgentProvisioner {
	return &remoteAgentProvisioner{cfg: cfg, now: time.Now, logger: logger.With("component", "remote-connect-provisioner")}
}

// agentSNI derives the agent's SNI host from its data-plane namespace (unique per
// project+env) plus the configured suffix. occ sends this as the TLS SNI; the shared
// router uses it to pick this agent; the agent's cert is signed for it.
func (p *remoteAgentProvisioner) agentSNI(dpNamespace string) string {
	suffix := p.cfg.SNISuffix
	if suffix == "" {
		suffix = defaultSNISuffix
	}
	return dpNamespace + "." + suffix
}

// ensureAgent applies the remote-agent resources (Deployment + ClusterIP Service + cert
// Secret) into dpNamespace via dpClient (a proxy client to the data plane) and returns
// the endpoint occ should dial: the shared SNI router's address, plus this agent's SNI
// + cert. It is idempotent: repeated calls refresh the last-used annotation and reuse
// the existing cert Secret.
func (p *remoteAgentProvisioner) ensureAgent(ctx context.Context, dpClient client.Client, dpNamespace string) (*agentEndpointInfo, error) {
	if p.cfg.EntrypointAddress == "" {
		return nil, fmt.Errorf("remote_connect.entrypoint_address is not configured")
	}
	sni := p.agentSNI(dpNamespace)

	certPEM, err := p.ensureCertSecret(ctx, dpClient, dpNamespace, sni)
	if err != nil {
		return nil, fmt.Errorf("ensure remote-agent cert: %w", err)
	}
	if err := p.applyDeployment(ctx, dpClient, dpNamespace, certPEM); err != nil {
		return nil, fmt.Errorf("apply remote-agent deployment: %w", err)
	}
	if err := p.applyService(ctx, dpClient, dpNamespace, sni); err != nil {
		return nil, fmt.Errorf("apply remote-agent service: %w", err)
	}

	// occ dials the shared router endpoint with this agent's SNI; the router does
	// TLS-passthrough to this agent, which terminates TLS with the cert below.
	return &agentEndpointInfo{endpoint: p.cfg.EntrypointAddress, caBundle: certPEM, serverName: sni}, nil
}

// ensureCertSecret returns the agent's cert, generating a self-signed cert/key pair
// (SAN = sni) on first use and reusing the stored one until it nears expiry (so occ
// pins a stable cert). Reissuing is safe: every resolve returns the current cert.
func (p *remoteAgentProvisioner) ensureCertSecret(ctx context.Context, dpClient client.Client, ns, sni string) (string, error) {
	secret := &corev1.Secret{}
	getErr := dpClient.Get(ctx, client.ObjectKey{Namespace: ns, Name: remoteAgentName}, secret)
	if getErr == nil {
		c, k := secret.Data[corev1.TLSCertKey], secret.Data[corev1.TLSPrivateKeyKey]
		if p.certUsable(c, k, sni) {
			return string(c), nil
		}
	} else if !apierrors.IsNotFound(getErr) {
		return "", getErr
	}

	certPEM, keyPEM, err := generateSelfSignedCert(sni, p.now())
	if err != nil {
		return "", err
	}
	desired := &corev1.Secret{
		TypeMeta:   metav1.TypeMeta{APIVersion: "v1", Kind: "Secret"},
		ObjectMeta: p.objectMeta(ns),
		Type:       corev1.SecretTypeTLS,
		Data: map[string][]byte{
			corev1.TLSCertKey:       []byte(certPEM),
			corev1.TLSPrivateKeyKey: []byte(keyPEM),
		},
	}

	// Write with Create/Update rather than apply so concurrent resolves cannot each
	// install a different cert: whoever loses the race adopts the stored one, so the
	// cert occ pins is always the cert the agent pod mounts.
	if apierrors.IsNotFound(getErr) {
		if cerr := dpClient.Create(ctx, desired); cerr != nil {
			if !apierrors.IsAlreadyExists(cerr) {
				return "", cerr
			}
			return p.storedCert(ctx, dpClient, ns)
		}
		return certPEM, nil
	}

	desired.ResourceVersion = secret.ResourceVersion
	if uerr := dpClient.Update(ctx, desired); uerr != nil {
		if !apierrors.IsConflict(uerr) {
			return "", uerr
		}
		return p.storedCert(ctx, dpClient, ns)
	}
	return certPEM, nil
}

// storedCert re-reads the cert another writer installed after we lost a write race.
func (p *remoteAgentProvisioner) storedCert(ctx context.Context, dpClient client.Client, ns string) (string, error) {
	secret := &corev1.Secret{}
	if err := dpClient.Get(ctx, client.ObjectKey{Namespace: ns, Name: remoteAgentName}, secret); err != nil {
		return "", err
	}
	cert := secret.Data[corev1.TLSCertKey]
	if len(cert) == 0 {
		return "", fmt.Errorf("remote-agent cert secret in %s has no %s", ns, corev1.TLSCertKey)
	}
	return string(cert), nil
}

func (p *remoteAgentProvisioner) applyDeployment(ctx context.Context, dpClient client.Client, ns string, certPEM string) error {
	replicas := int32(1)
	port := int32(p.cfg.AgentListenPort) //nolint:gosec // config-bounded port
	labelSet := p.labelSet()

	args := []string{
		"--listen=:" + strconv.Itoa(p.cfg.AgentListenPort),
		"--authorize-url=" + p.cfg.AuthorizeURL,
		"--tls-cert=/certs/tls.crt",
		"--tls-key=/certs/tls.key",
	}
	if p.cfg.AuthorizeInsecure {
		args = append(args, "--authorize-insecure")
	}
	// The heartbeat endpoint shares the authorize URL's host; derive it by swapping the
	// path. If the authorize URL isn't the standard path, heartbeats stay off (the agent
	// warns) rather than pointing at the wrong endpoint.
	if hb := strings.Replace(p.cfg.AuthorizeURL, remoteconnect.AuthorizePath, remoteconnect.HeartbeatPath, 1); hb != p.cfg.AuthorizeURL {
		args = append(args, "--heartbeat-url="+hb)
	}

	dep := &appsv1.Deployment{
		TypeMeta:   metav1.TypeMeta{APIVersion: "apps/v1", Kind: "Deployment"},
		ObjectMeta: p.objectMeta(ns),
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": remoteAgentName}},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labelSet,
					// The agent loads its keypair at start-up, so the pod must roll when the
					// cert is reissued.
					Annotations: map[string]string{certAnnotation: certFingerprint(certPEM)},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name:            "remote-agent",
						Image:           p.cfg.AgentImage,
						ImagePullPolicy: corev1.PullIfNotPresent,
						Args:            args,
						// The agent sends its own namespace in heartbeats so the control
						// plane refreshes the right agent.
						Env: []corev1.EnvVar{{
							Name: "POD_NAMESPACE",
							ValueFrom: &corev1.EnvVarSource{
								FieldRef: &corev1.ObjectFieldSelector{FieldPath: "metadata.namespace"},
							},
						}},
						Ports: []corev1.ContainerPort{{ContainerPort: port}},
						VolumeMounts: []corev1.VolumeMount{{
							Name:      "certs",
							MountPath: "/certs",
							ReadOnly:  true,
						}},
						// The agent is a bare TCP listener with no HTTP surface, so
						// readiness is "is the port accepting".
						ReadinessProbe: &corev1.Probe{
							ProbeHandler: corev1.ProbeHandler{
								TCPSocket: &corev1.TCPSocketAction{Port: intstr.FromInt32(port)},
							},
							InitialDelaySeconds: 2,
							PeriodSeconds:       10,
						},
						// Requests keep the agent schedulable under a namespace quota;
						// tenant namespaces often carry a LimitRange that rejects pods
						// without them.
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("50m"),
								corev1.ResourceMemory: resource.MustParse("64Mi"),
							},
							Limits: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("200m"),
								corev1.ResourceMemory: resource.MustParse("128Mi"),
							},
						},
						SecurityContext: &corev1.SecurityContext{
							AllowPrivilegeEscalation: ptr.To(false),
							ReadOnlyRootFilesystem:   ptr.To(true),
							Capabilities:             &corev1.Capabilities{Drop: []corev1.Capability{"ALL"}},
						},
					}},
					SecurityContext: &corev1.PodSecurityContext{
						RunAsNonRoot:   ptr.To(true),
						RunAsUser:      ptr.To(int64(1000)),
						FSGroup:        ptr.To(int64(1000)),
						SeccompProfile: &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeRuntimeDefault},
					},
					// The agent never calls the Kubernetes API.
					AutomountServiceAccountToken: ptr.To(false),
					Volumes: []corev1.Volume{{
						Name:         "certs",
						VolumeSource: corev1.VolumeSource{Secret: &corev1.SecretVolumeSource{SecretName: remoteAgentName}},
					}},
				},
			},
		},
	}
	return dpClient.Patch(ctx, dep, client.Apply, client.ForceOwnership, client.FieldOwner(remoteAgentFieldOwner))
}

func (p *remoteAgentProvisioner) applyService(ctx context.Context, dpClient client.Client, ns, sni string) error {
	port := int32(p.cfg.AgentListenPort) //nolint:gosec // config-bounded port
	meta := p.objectMeta(ns)
	// The shared SNI router discovers this agent by this annotation.
	meta.Annotations[sniAnnotationKey] = sni
	svc := &corev1.Service{
		TypeMeta:   metav1.TypeMeta{APIVersion: "v1", Kind: "Service"},
		ObjectMeta: meta,
		Spec: corev1.ServiceSpec{
			// ClusterIP only: external reachability is the shared router's job, not a
			// per-agent LoadBalancer/NodePort.
			Type:     corev1.ServiceTypeClusterIP,
			Selector: map[string]string{"app": remoteAgentName},
			Ports: []corev1.ServicePort{{
				Name:       "tunnel",
				Port:       port,
				TargetPort: intstr.FromInt32(port),
				Protocol:   corev1.ProtocolTCP,
			}},
		},
	}
	return dpClient.Patch(ctx, svc, client.Apply, client.ForceOwnership, client.FieldOwner(remoteAgentFieldOwner))
}

// touchLastUsed refreshes the remote-agent's last-used annotation so the reaper keeps it
// alive while sessions are active. It is a merge patch (not server-side apply) so it
// only updates the annotation and leaves the rest of the object untouched.
func (p *remoteAgentProvisioner) touchLastUsed(ctx context.Context, dpClient client.Client, ns string) error {
	patch := fmt.Sprintf(`{"metadata":{"annotations":{%q:%q}}}`,
		lastUsedAnnotation, p.now().UTC().Format(time.RFC3339))
	dep := &appsv1.Deployment{
		TypeMeta:   metav1.TypeMeta{APIVersion: "apps/v1", Kind: "Deployment"},
		ObjectMeta: metav1.ObjectMeta{Name: remoteAgentName, Namespace: ns},
	}
	return dpClient.Patch(ctx, dep, client.RawPatch(types.MergePatchType, []byte(patch)))
}

func (p *remoteAgentProvisioner) objectMeta(ns string) metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Name:        remoteAgentName,
		Namespace:   ns,
		Labels:      p.labelSet(),
		Annotations: map[string]string{lastUsedAnnotation: p.now().UTC().Format(time.RFC3339)},
	}
}

// labelSet is applied to all provisioned resources and pods. The system-component
// label admits the agent through per-component NetworkPolicies to reach dependency
// services in other namespaces.
func (p *remoteAgentProvisioner) labelSet() map[string]string {
	return map[string]string{
		"app":                          remoteAgentName,
		"app.kubernetes.io/managed-by": managedByLabelValue,
		labels.LabelKeySystemComponent: remoteAgentName,
	}
}

// generateSelfSignedCert produces a PEM cert/key pair with serverName as its SAN,
// used by the remote-agent as its TLS server cert and pinned by occ.
// certUsable reports whether a stored cert/key pair can still be served: the pair must
// load together, carry sni as a SAN, and not be within certRenewBefore of expiry.
// Anything else is reissued rather than surfaced as an error — a mismatched pair would
// otherwise CrashLoopBackOff the agent on tls.LoadX509KeyPair with no way to recover.
func (p *remoteAgentProvisioner) certUsable(certPEM, keyPEM []byte, sni string) bool {
	if len(certPEM) == 0 || len(keyPEM) == 0 {
		return false
	}
	if _, err := tls.X509KeyPair(certPEM, keyPEM); err != nil {
		return false
	}
	block, _ := pem.Decode(certPEM)
	if block == nil {
		return false
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return false
	}
	if cert.VerifyHostname(sni) != nil {
		return false
	}
	return p.now().Add(certRenewBefore).Before(cert.NotAfter)
}

func generateSelfSignedCert(serverName string, now time.Time) (certPEM, keyPEM string, err error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return "", "", err
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return "", "", err
	}
	tmpl := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: serverName},
		DNSNames:              []string{serverName},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.Add(certValidity),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		// Not a CA: occ pins this exact cert as its root, which Go accepts without IsCA.
		IsCA: false,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return "", "", err
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return "", "", err
	}
	certPEM = string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
	keyPEM = string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER}))
	return certPEM, keyPEM, nil
}

// certFingerprint is a short digest of the agent's cert, used as a pod annotation so a
// reissued cert rolls the Deployment.
func certFingerprint(certPEM string) string {
	sum := sha256.Sum256([]byte(certPEM))
	return hex.EncodeToString(sum[:8])
}
