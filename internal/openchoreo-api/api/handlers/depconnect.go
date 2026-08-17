// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"
	"crypto/ed25519"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/golang-jwt/jwt/v5"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	authz "github.com/openchoreo/openchoreo/internal/authz/core"
	kubernetesClient "github.com/openchoreo/openchoreo/internal/clients/kubernetes"
	"github.com/openchoreo/openchoreo/internal/controller"
	dpkubernetes "github.com/openchoreo/openchoreo/internal/dataplane/kubernetes"
	"github.com/openchoreo/openchoreo/internal/depconnect"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/config"
	svcpkg "github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	authmw "github.com/openchoreo/openchoreo/internal/server/middleware/auth"
)

// DepConnectHandler serves POST /api/v1/dep-connect:resolve — it resolves a workload's
// declared dependencies against provider status, provisions (or refreshes) the
// per-project+env dep-agent (Deployment + dedicated L4 Service) in the data plane, and
// returns connection targets, a short-lived capability, and the dep-agent's L4 endpoint.
// occ dials that endpoint directly and presents the capability; the
// dep-agent authorizes each stream via DepConnectAuthorizeHandler (the byte path never
// traverses the control plane). The signing key stays on the control plane — the agent
// holds none.
type DepConnectHandler struct {
	k8sClient           client.Client
	planeClientProvider kubernetesClient.DataPlaneClientProvider
	authzChecker        *svcpkg.AuthzChecker
	signer              *capabilitySigner
	provisioner         *depAgentProvisioner
	logger              *slog.Logger
}

// NewDepConnectHandler loads the signing key and builds the handler. planeClientProvider
// is used to reach the data plane (through the cluster-gateway proxy) to provision the
// per-project+env dep-agent on resolve.
func NewDepConnectHandler(k8sClient client.Client, planeClientProvider kubernetesClient.DataPlaneClientProvider, authzChecker *svcpkg.AuthzChecker, cfg config.DepConnectConfig, logger *slog.Logger) (*DepConnectHandler, error) {
	priv, err := loadEd25519PrivateKeyPEM(cfg.SigningKeyPath)
	if err != nil {
		return nil, fmt.Errorf("dep-connect: load signing key: %w", err)
	}
	return &DepConnectHandler{
		k8sClient:           k8sClient,
		planeClientProvider: planeClientProvider,
		authzChecker:        authzChecker,
		signer: &capabilitySigner{
			privKey: priv,
			keyID:   cfg.KeyID,
			issuer:  cfg.Issuer,
			ttl:     time.Duration(cfg.TTLSeconds) * time.Second,
		},
		provisioner: newDepAgentProvisioner(cfg, logger),
		logger:      logger.With("component", "dep-connect-handler"),
	}, nil
}

// VerifyKey returns the Ed25519 public key that verifies capabilities this handler
// signs. Capabilities are minted and verified in the same process, so the verify key
// is simply the public half of the signing key.
func (h *DepConnectHandler) VerifyKey() ed25519.PublicKey {
	return h.signer.privKey.Public().(ed25519.PublicKey)
}

// ServeHTTP handles the resolve request.
func (h *DepConnectHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req depconnect.ResolveRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.Namespace == "" || req.Project == "" || req.Component == "" || req.Environment == "" {
		http.Error(w, "namespace, project, component and environment are required", http.StatusBadRequest)
		return
	}

	// Authorization is per dependency, inside resolve — see authorizeProviders.
	subject := "unknown"
	if sc, ok := authmw.GetSubjectContext(r); ok && sc != nil {
		subject = sc.ID
	}

	resp, err := h.resolve(ctx, req, subject)
	if err != nil {
		h.logger.Error("dependency resolution failed", "error", err)
		http.Error(w, "dependency resolution failed", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		h.logger.Error("failed to encode response", "error", err)
	}
}

// unavailableReason is returned for every dependency that cannot be tunneled, whether
// because it does not exist, is not deployed, or the caller's role cannot reach it. The
// three are deliberately indistinguishable: a specific message would let a caller
// enumerate components in projects they have no access to.
const unavailableReason = "not available: it may not be deployed in this environment, or your role may not have access to it"

func providerKey(project, component string) string { return project + "/" + component }

// authorizeProviders reports which dependency components the caller may connect to,
// keyed by "project/component".
//
// Authorization is per dependency, never on the caller's own component: that identity
// comes from a local workload file and by design need not exist yet — `occ local` is
// used to test a component before its first deploy, and to try dependencies that have
// not been committed. So "may this role reach this dependency" is the only meaningful
// question, and a self-declared consumer adds nothing to it.
func (h *DepConnectHandler) authorizeProviders(ctx context.Context, req depconnect.ResolveRequest) (map[string]bool, error) {
	if h.authzChecker == nil {
		// Fail closed: a missing checker must not resolve every dependency.
		return nil, errors.New("no authorization checker configured")
	}

	type provider struct{ project, component string }
	var providers []provider
	seen := make(map[string]bool, len(req.Endpoints))
	for _, dep := range req.Endpoints {
		project := dep.Project
		if project == "" {
			project = req.Project
		}
		key := providerKey(project, dep.Component)
		if seen[key] {
			continue
		}
		seen[key] = true
		providers = append(providers, provider{project: project, component: dep.Component})
	}
	if len(providers) == 0 {
		return map[string]bool{}, nil
	}

	checks := make([]svcpkg.CheckRequest, 0, len(providers))
	for _, p := range providers {
		checks = append(checks, svcpkg.CheckRequest{
			Action:       authz.ActionConnectComponent,
			ResourceType: "component",
			ResourceID:   p.component,
			Hierarchy: authz.ResourceHierarchy{
				Namespace: req.Namespace,
				Project:   p.project,
				Component: p.component,
			},
			Context: authz.Context{
				Resource: authz.ResourceAttribute{
					Environment: svcpkg.FormatDualScopedResourceName(req.Namespace, req.Environment, false),
				},
			},
		})
	}

	decisions, err := h.authzChecker.BatchCheck(ctx, checks)
	if err != nil {
		return nil, err
	}
	if len(decisions) != len(providers) {
		return nil, fmt.Errorf("authorization returned %d decisions for %d dependencies", len(decisions), len(providers))
	}

	allowed := make(map[string]bool, len(providers))
	for i, p := range providers {
		allowed[providerKey(p.project, p.component)] = decisions[i]
	}
	return allowed, nil
}

// resolve turns declared dependencies into connection targets + a signed capability.
func (h *DepConnectHandler) resolve(ctx context.Context, req depconnect.ResolveRequest, subject string) (*depconnect.ResolveResponse, error) {
	// Every dependency for a given environment resolves through the same data
	// plane (an Environment has exactly one DataPlaneRef), so this runs once per
	// request rather than once per dependency.
	dpResult, err := h.resolveDepConnectPlane(ctx, req.Namespace, req.Environment)
	if err != nil {
		return nil, fmt.Errorf("resolve data plane for environment %q: %w", req.Environment, err)
	}

	var (
		capTargets    []depconnect.Target
		respTargets   []depconnect.ResolvedTarget
		unconnectable []depconnect.Unconnectable
	)

	// dpNamespaceFor names the data-plane namespace a target's dep-agent lives in — the
	// provider's project+env namespace, so the agent dials its dependency from within
	// the dependency's own namespace. It doubles as the agent ID occ routes on.
	dpNamespaceFor := func(project string) string {
		return dpkubernetes.GenerateK8sNameWithLengthLimit(
			dpkubernetes.MaxNamespaceNameLength, "dp", req.Namespace, project, req.Environment)
	}

	allowed, err := h.authorizeProviders(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("authorize dependencies: %w", err)
	}

	seenKeys := make(map[string]bool, len(req.Endpoints))
	for _, dep := range req.Endpoints {
		providerProject := dep.Project
		if providerProject == "" {
			providerProject = req.Project
		}
		key := depconnect.EndpointTargetKey(providerProject, dep.Component, dep.Name)
		// The agent resolves a stream by key alone, so two targets may never share one.
		if seenKeys[key] {
			h.logger.Warn("dep-connect: duplicate dependency declaration ignored", "ref", key)
			continue
		}
		seenKeys[key] = true
		if !allowed[providerKey(providerProject, dep.Component)] {
			h.logger.Info("dep-connect: dependency not authorized",
				"subject", subject, "project", providerProject, "component", dep.Component)
			unconnectable = append(unconnectable, depconnect.Unconnectable{Ref: key, Reason: unavailableReason})
			continue
		}
		url, err := h.resolveEndpoint(ctx, req.Namespace, providerProject, dep.Component, dep.Name, dep.Visibility, req.Environment)
		if err != nil {
			// Same reason as the unauthorized case: a distinct message would let a caller
			// probe which components exist in projects they cannot reach.
			h.logger.Debug("dep-connect: dependency unresolved", "ref", key, "error", err)
			unconnectable = append(unconnectable, depconnect.Unconnectable{Ref: key, Reason: unavailableReason})
			continue
		}
		agentNs := dpNamespaceFor(providerProject)
		capTargets = append(capTargets, depconnect.Target{
			Key: key, Proto: "tcp", Host: url.Host, Port: int(url.Port), AgentNamespace: agentNs,
		})
		respTargets = append(respTargets, depconnect.ResolvedTarget{
			Key:      key,
			Proto:    "tcp",
			Endpoint: &depconnect.EndpointRender{Scheme: url.Scheme, BasePath: url.Path, Bindings: dep.EnvBindings},
			AgentID:  agentNs,
		})
	}

	capability, err := h.signer.sign(subject, req.Namespace, depconnect.ComponentRef{Project: req.Project, Name: req.Component}, req.Environment, capTargets)
	if err != nil {
		return nil, fmt.Errorf("sign capability: %w", err)
	}

	resp := &depconnect.ResolveResponse{
		Capability:    capability,
		Targets:       respTargets,
		Unconnectable: unconnectable,
	}

	// Provision (or refresh) one dep-agent per distinct provider project+env namespace
	// the targets point at; occ dials each directly. Provisioning is idempotent, so an
	// agent already serving another session is simply reused (its last-used annotation
	// refreshes). A nil provisioner (resolution-only unit tests) skips provisioning.
	// Every dependency for the environment shares one data plane (Environment has one
	// DataPlaneRef), so a single dpClient reaches all the target namespaces.
	if len(capTargets) > 0 && h.provisioner != nil {
		dpClient, cerr := dpResult.GetK8sClient(h.planeClientProvider)
		if cerr != nil {
			return nil, fmt.Errorf("get data plane client: %w", cerr)
		}
		agents := make(map[string]depconnect.AgentEndpoint)
		for _, t := range capTargets {
			if _, done := agents[t.AgentNamespace]; done {
				continue
			}
			info, perr := h.provisioner.ensureAgent(ctx, dpClient, t.AgentNamespace)
			if perr != nil {
				return nil, fmt.Errorf("provision dep-agent in %s: %w", t.AgentNamespace, perr)
			}
			agents[t.AgentNamespace] = depconnect.AgentEndpoint{
				Endpoint: info.endpoint, CABundle: info.caBundle, ServerName: info.serverName,
			}
		}
		resp.Agents = agents
	}

	return resp, nil
}

// TouchAgent refreshes the last-used annotation of the dep-agent in dpNamespace so the
// reaper keeps it alive while a session is active. The control-plane namespace + env
// locate the data plane (they share one per env); dpNamespace names the specific agent
// (the one that served the authorized stream). Best-effort: errors are returned for the
// caller to log, not surfaced to the data plane.
func (h *DepConnectHandler) TouchAgent(ctx context.Context, namespace, env, dpNamespace string) error {
	dpResult, err := h.resolveDepConnectPlane(ctx, namespace, env)
	if err != nil {
		return err
	}
	dpClient, err := dpResult.GetK8sClient(h.planeClientProvider)
	if err != nil {
		return err
	}
	return h.provisioner.touchLastUsed(ctx, dpClient, dpNamespace)
}

// resolveDepConnectPlane resolves the data plane serving env, reusing the same
// DataPlane/ClusterDataPlane mapping ExecHandler uses for `occ exec`.
func (h *DepConnectHandler) resolveDepConnectPlane(ctx context.Context, ns, envName string) (*controller.DataPlaneResult, error) {
	env := &openchoreov1alpha1.Environment{}
	if err := h.k8sClient.Get(ctx, client.ObjectKey{Namespace: ns, Name: envName}, env); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, fmt.Errorf("environment %q not found in namespace %q", envName, ns)
		}
		return nil, fmt.Errorf("failed to look up environment %q: %w", envName, err)
	}
	if env.Spec.DataPlaneRef == nil {
		return nil, fmt.Errorf("environment %q has no data plane reference", envName)
	}

	dpResult, err := controller.GetDataPlaneFromRef(ctx, h.k8sClient, env.Namespace, env.Spec.DataPlaneRef)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve data plane: %w", err)
	}

	if resolveExecPlaneInfo(dpResult).planeID == "" {
		return nil, fmt.Errorf("failed to determine plane ID for environment %q", envName)
	}
	return dpResult, nil
}

// resolveEndpoint finds the provider ReleaseBinding and reads the named endpoint's
// in-cluster ServiceURL (for project/namespace visibility).
func (h *DepConnectHandler) resolveEndpoint(ctx context.Context, ns, project, component, epName, visibility, env string) (*openchoreov1alpha1.EndpointURL, error) {
	rb, err := h.findReleaseBinding(ctx, ns, project, component, env)
	if err != nil {
		return nil, err
	}
	for i := range rb.Status.Endpoints {
		ep := rb.Status.Endpoints[i]
		if ep.Name != epName {
			continue
		}
		url := urlForVisibility(ep, openchoreov1alpha1.EndpointVisibility(visibility))
		if url == nil {
			return nil, fmt.Errorf("endpoint %q not yet resolved for visibility %q", epName, visibility)
		}
		return url, nil
	}
	return nil, fmt.Errorf("endpoint %q not found on %s/%s in %s", epName, project, component, env)
}

// findReleaseBinding lists ReleaseBindings in the namespace and returns the single one
// matching (project, component, environment) that is not being undeployed. The API
// server's client is uncached with no field indexes, so we list + filter in memory.
func (h *DepConnectHandler) findReleaseBinding(ctx context.Context, ns, project, component, env string) (*openchoreov1alpha1.ReleaseBinding, error) {
	var list openchoreov1alpha1.ReleaseBindingList
	if err := h.k8sClient.List(ctx, &list, client.InNamespace(ns)); err != nil {
		return nil, fmt.Errorf("list release bindings: %w", err)
	}
	var match *openchoreov1alpha1.ReleaseBinding
	for i := range list.Items {
		rb := &list.Items[i]
		if rb.Spec.Owner.ProjectName != project || rb.Spec.Owner.ComponentName != component || rb.Spec.Environment != env {
			continue
		}
		if rb.Spec.State == openchoreov1alpha1.ReleaseStateUndeploy {
			continue
		}
		if match != nil {
			return nil, fmt.Errorf("multiple release bindings match %s/%s in %s", project, component, env)
		}
		match = rb
	}
	if match == nil {
		return nil, fmt.Errorf("no release binding for %s/%s in %s", project, component, env)
	}
	return match, nil
}

// urlForVisibility mirrors the controller's resolveURLForVisibility: project/namespace
// visibility resolve to the in-cluster ServiceURL; external resolves to a gateway URL.
func urlForVisibility(ep openchoreov1alpha1.EndpointURLStatus, visibility openchoreov1alpha1.EndpointVisibility) *openchoreov1alpha1.EndpointURL {
	switch visibility {
	case openchoreov1alpha1.EndpointVisibilityProject, openchoreov1alpha1.EndpointVisibilityNamespace:
		return ep.ServiceURL
	case openchoreov1alpha1.EndpointVisibilityExternal:
		if ep.ExternalURLs != nil {
			if ep.ExternalURLs.HTTPS != nil {
				return ep.ExternalURLs.HTTPS
			}
			if ep.ExternalURLs.HTTP != nil {
				return ep.ExternalURLs.HTTP
			}
			if ep.ExternalURLs.TLS != nil {
				return ep.ExternalURLs.TLS
			}
		}
		return nil
	default:
		return nil
	}
}

// capabilitySigner mints capability JWTs.
type capabilitySigner struct {
	privKey ed25519.PrivateKey
	keyID   string
	issuer  string
	ttl     time.Duration
}

func (s *capabilitySigner) sign(subject, namespace string, comp depconnect.ComponentRef, env string, targets []depconnect.Target) (string, error) {
	now := time.Now()
	claims := &depconnect.CapabilityClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    s.issuer,
			Subject:   subject,
			Audience:  jwt.ClaimStrings{depconnect.CapabilityAudience},
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(s.ttl)),
		},
		Namespace: namespace,
		Component: comp,
		Env:       env,
		Targets:   targets,
	}
	return depconnect.SignCapability(claims, s.privKey, s.keyID)
}

func loadEd25519PrivateKeyPEM(path string) (ed25519.PrivateKey, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, errors.New("no PEM block in signing key")
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse PKCS8 private key: %w", err)
	}
	priv, ok := parsed.(ed25519.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("signing key is %T, want ed25519", parsed)
	}
	return priv, nil
}
