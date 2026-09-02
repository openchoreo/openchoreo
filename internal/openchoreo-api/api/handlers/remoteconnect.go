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
	"maps"
	"net"
	"net/http"
	"os"
	"slices"
	"strconv"
	"time"

	"github.com/golang-jwt/jwt/v5"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	authz "github.com/openchoreo/openchoreo/internal/authz/core"
	kubernetesClient "github.com/openchoreo/openchoreo/internal/clients/kubernetes"
	"github.com/openchoreo/openchoreo/internal/controller"
	"github.com/openchoreo/openchoreo/internal/controller/resourcereleasebinding"
	dpkubernetes "github.com/openchoreo/openchoreo/internal/dataplane/kubernetes"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/config"
	svcpkg "github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	"github.com/openchoreo/openchoreo/internal/remoteconnect"
	authmw "github.com/openchoreo/openchoreo/internal/server/middleware/auth"
)

// RemoteConnectHandler serves POST /api/v1/remote-connect:resolve — it resolves a workload's
// declared dependencies against provider status, provisions (or refreshes) the
// per-project+env remote-agent (Deployment + dedicated L4 Service) in the data plane, and
// returns connection targets, a short-lived capability, and the remote-agent's L4 endpoint.
// occ dials that endpoint directly and presents the capability; the
// remote-agent authorizes each stream via RemoteConnectAuthorizeHandler (the byte path never
// traverses the control plane). The signing key stays on the control plane — the agent
// holds none.
//
// Values behind a Secret/ConfigMap reference are emitted as fetch grants, not values:
// the control plane signs the coordinates and the remote-agent reads them in the data
// plane, returning each value to occ over the tunnel. Secret material therefore never
// enters a control-plane response.
type RemoteConnectHandler struct {
	k8sClient           client.Client
	planeClientProvider kubernetesClient.DataPlaneClientProvider
	authzChecker        *svcpkg.AuthzChecker
	signer              *capabilitySigner
	provisioner         *remoteAgentProvisioner
	// secretsEnabled is the operator kill switch for value resolution, independent of
	// policy: off means no capability authorizes a read, whatever roles grant.
	secretsEnabled bool
	logger         *slog.Logger
}

// NewRemoteConnectHandler loads the signing key and builds the handler. planeClientProvider
// is used to reach the data plane (through the cluster-gateway proxy) to provision the
// per-project+env remote-agent on resolve.
func NewRemoteConnectHandler(k8sClient client.Client, planeClientProvider kubernetesClient.DataPlaneClientProvider, authzChecker *svcpkg.AuthzChecker, cfg config.RemoteConnectConfig, logger *slog.Logger) (*RemoteConnectHandler, error) {
	priv, err := loadEd25519PrivateKeyPEM(cfg.SigningKeyPath)
	if err != nil {
		return nil, fmt.Errorf("remote-connect: load signing key: %w", err)
	}
	return &RemoteConnectHandler{
		k8sClient:           k8sClient,
		planeClientProvider: planeClientProvider,
		authzChecker:        authzChecker,
		signer: &capabilitySigner{
			privKey:   priv,
			keyID:     cfg.KeyID,
			issuer:    cfg.Issuer,
			ttl:       time.Duration(cfg.TTLSeconds) * time.Second,
			secretTTL: time.Duration(cfg.SecretTTLSeconds) * time.Second,
		},
		provisioner:    newRemoteAgentProvisioner(cfg, logger),
		secretsEnabled: cfg.SecretsEnabled,
		logger:         logger.With("component", "remote-connect-handler"),
	}, nil
}

// VerifyKey returns the Ed25519 public key that verifies capabilities this handler
// signs. Capabilities are minted and verified in the same process, so the verify key
// is simply the public half of the signing key.
func (h *RemoteConnectHandler) VerifyKey() ed25519.PublicKey {
	return h.signer.privKey.Public().(ed25519.PublicKey)
}

// ServeHTTP handles the resolve request.
func (h *RemoteConnectHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req remoteconnect.ResolveRequest
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
// comes from a local workload file and by design need not exist yet — `occ remote` is
// used to test a component before its first deploy, and to try dependencies that have
// not been committed. So "may this role reach this dependency" is the only meaningful
// question, and a self-declared consumer adds nothing to it.
//
// Resource dependencies are authorized separately by authorizeResources: they are keyed
// by resource name rather than component, and carry their own action.
func (h *RemoteConnectHandler) authorizeProviders(ctx context.Context, req remoteconnect.ResolveRequest) (map[string]bool, error) {
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

// authorizeResources reports which resource dependencies the caller may connect to,
// keyed by resource name.
//
// A resource dependency is always consumed from the caller's own project (cross-project
// consumption is not supported), so unlike an endpoint dependency there is no separate
// provider project to check. That does NOT make the check redundant: req.Project comes
// from a local workload file and is never itself authorized, so without this a caller
// could name any project and tunnel into its resources. Connecting is its own action
// rather than resource:view because a tunnel is raw TCP to the backing service, which
// is strictly more than reading the resource's definition.
func (h *RemoteConnectHandler) authorizeResources(ctx context.Context, req remoteconnect.ResolveRequest) (map[string]bool, error) {
	if h.authzChecker == nil {
		// Fail closed, as authorizeProviders does.
		return nil, errors.New("no authorization checker configured")
	}

	var refs []string
	seen := make(map[string]bool, len(req.Resources))
	for _, dep := range req.Resources {
		if dep.Ref == "" || seen[dep.Ref] {
			continue
		}
		seen[dep.Ref] = true
		refs = append(refs, dep.Ref)
	}
	if len(refs) == 0 {
		return map[string]bool{}, nil
	}

	checks := make([]svcpkg.CheckRequest, 0, len(refs))
	for _, ref := range refs {
		checks = append(checks, svcpkg.CheckRequest{
			Action:       authz.ActionConnectResource,
			ResourceType: "resource",
			ResourceID:   ref,
			Hierarchy: authz.ResourceHierarchy{
				Namespace: req.Namespace,
				Project:   req.Project,
				Resource:  ref,
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
	if len(decisions) != len(refs) {
		return nil, fmt.Errorf("authorization returned %d decisions for %d resource dependencies", len(decisions), len(refs))
	}

	allowed := make(map[string]bool, len(refs))
	for i, ref := range refs {
		allowed[ref] = decisions[i]
	}
	return allowed, nil
}

// authorizeResourceSecrets reports which resource dependencies the caller may read the
// secret-backed output VALUES of, keyed by resource name.
//
// This is a second, stricter pass over the same refs authorizeResources checked, not a
// refinement of it: connecting to a resource and extracting the credential that opens
// it are different grants, and an installation must be able to give a role the tunnel
// without the password. Only refs that already passed authorizeResources are asked
// about — a caller who may not reach the resource at all is never asked whether they
// may read its secrets.
func (h *RemoteConnectHandler) authorizeResourceSecrets(ctx context.Context, req remoteconnect.ResolveRequest, connectable map[string]bool) (map[string]bool, error) {
	if h.authzChecker == nil {
		// Fail closed, as the other authorization passes do.
		return nil, errors.New("no authorization checker configured")
	}

	var refs []string
	seen := make(map[string]bool, len(req.Resources))
	for _, dep := range req.Resources {
		if dep.Ref == "" || seen[dep.Ref] || !connectable[dep.Ref] {
			continue
		}
		// Nothing to ask about for a dependency that binds no values at all.
		if len(dep.EnvBindings) == 0 && len(dep.FileBindings) == 0 {
			continue
		}
		seen[dep.Ref] = true
		refs = append(refs, dep.Ref)
	}
	if len(refs) == 0 {
		return map[string]bool{}, nil
	}

	checks := make([]svcpkg.CheckRequest, 0, len(refs))
	for _, ref := range refs {
		checks = append(checks, svcpkg.CheckRequest{
			Action:       authz.ActionReadResourceSecrets,
			ResourceType: "resource",
			ResourceID:   ref,
			Hierarchy: authz.ResourceHierarchy{
				Namespace: req.Namespace,
				Project:   req.Project,
				Resource:  ref,
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
	if len(decisions) != len(refs) {
		return nil, fmt.Errorf("authorization returned %d decisions for %d resource secret reads", len(decisions), len(refs))
	}

	allowed := make(map[string]bool, len(refs))
	for i, ref := range refs {
		allowed[ref] = decisions[i]
	}
	return allowed, nil
}

// resolve turns declared dependencies into connection targets + a signed capability.
func (h *RemoteConnectHandler) resolve(ctx context.Context, req remoteconnect.ResolveRequest, subject string) (*remoteconnect.ResolveResponse, error) {
	// Every dependency for a given environment resolves through the same data
	// plane (an Environment has exactly one DataPlaneRef), so this runs once per
	// request rather than once per dependency.
	dpResult, err := h.resolveRemoteConnectPlane(ctx, req.Namespace, req.Environment)
	if err != nil {
		return nil, fmt.Errorf("resolve data plane for environment %q: %w", req.Environment, err)
	}

	var (
		capTargets    []remoteconnect.Target
		capGrants     []remoteconnect.SecretGrant
		respTargets   []remoteconnect.ResolvedTarget
		unconnectable []remoteconnect.Unconnectable
	)

	// dpNamespaceFor names the data-plane namespace a target's remote-agent lives in — the
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
	allowedResources, err := h.authorizeResources(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("authorize resource dependencies: %w", err)
	}
	allowedSecrets, err := h.authorizeResourceSecrets(ctx, req, allowedResources)
	if err != nil {
		return nil, fmt.Errorf("authorize resource secret reads: %w", err)
	}

	seenKeys := make(map[string]bool, len(req.Endpoints))
	for _, dep := range req.Endpoints {
		providerProject := dep.Project
		if providerProject == "" {
			providerProject = req.Project
		}
		key := remoteconnect.EndpointTargetKey(providerProject, dep.Component, dep.Name)
		// The agent resolves a stream by key alone, so two targets may never share one.
		if seenKeys[key] {
			h.logger.Warn("remote-connect: duplicate dependency declaration ignored", "ref", key)
			continue
		}
		seenKeys[key] = true
		if !allowed[providerKey(providerProject, dep.Component)] {
			h.logger.Info("remote-connect: dependency not authorized",
				"subject", subject, "project", providerProject, "component", dep.Component)
			unconnectable = append(unconnectable, remoteconnect.Unconnectable{Ref: key, Reason: unavailableReason})
			continue
		}
		url, err := h.resolveEndpoint(ctx, req.Namespace, providerProject, dep.Component, dep.Name, dep.Visibility, req.Environment)
		if err != nil {
			// Same reason as the unauthorized case: a distinct message would let a caller
			// probe which components exist in projects they cannot reach.
			h.logger.Debug("remote-connect: dependency unresolved", "ref", key, "error", err)
			unconnectable = append(unconnectable, remoteconnect.Unconnectable{Ref: key, Reason: unavailableReason})
			continue
		}
		agentNs := dpNamespaceFor(providerProject)
		capTargets = append(capTargets, remoteconnect.Target{
			Key: key, Proto: "tcp", Host: url.Host, Port: int(url.Port), AgentNamespace: agentNs,
		})
		respTargets = append(respTargets, remoteconnect.ResolvedTarget{
			Key:      key,
			Proto:    "tcp",
			Endpoint: &remoteconnect.EndpointRender{Scheme: url.Scheme, BasePath: url.Path, Bindings: dep.EnvBindings},
			AgentID:  agentNs,
		})
	}

	var resourceBindings []remoteconnect.ResourceBindings
	seenRefs := make(map[string]bool, len(req.Resources))
	for _, dep := range req.Resources {
		// One ref contributes one set of bindings however many times it is declared;
		// deduping here (not just per target key) keeps occ from seeing the same
		// resource twice and reporting a bogus "set by multiple workloads" clash.
		if seenRefs[dep.Ref] {
			h.logger.Warn("remote-connect: duplicate resource dependency ignored", "ref", dep.Ref)
			continue
		}
		seenRefs[dep.Ref] = true

		refKey := remoteconnect.ResourceRefKey(dep.Ref)
		if !allowedResources[dep.Ref] {
			h.logger.Info("remote-connect: resource dependency not authorized",
				"subject", subject, "project", req.Project, "resource", dep.Ref)
			unconnectable = append(unconnectable, remoteconnect.Unconnectable{Ref: refKey, Reason: unavailableReason})
			continue
		}
		fetch := fetchPolicy{
			// A ConfigMap-backed value is not secret; reading it rides on the same
			// resource:connect grant that admitted the dependency (checked above).
			configMaps: true,
			secrets:    allowedSecrets[dep.Ref],
		}
		if !h.secretsEnabled {
			fetch.disabledReason = "value resolution is disabled on this control plane " +
				"(remote_connect.secrets_enabled)"
		}
		res, err := h.resolveResource(ctx, req.Namespace, req.Project, dep, req.Environment, fetch)
		if err != nil {
			// Collapsed to the shared reason for the same purpose as the endpoint path:
			// "does not exist", "not ready" and "not authorized" must be
			// indistinguishable, or the caller can probe another project's resources.
			h.logger.Debug("remote-connect: resource dependency unresolved", "ref", dep.Ref, "error", err)
			unconnectable = append(unconnectable, remoteconnect.Unconnectable{Ref: refKey, Reason: unavailableReason})
			continue
		}
		// A resource is owned by the consuming project, so its agent lives in the
		// consumer's project+env namespace (which has egress to the resource backend).
		// The Secrets and ConfigMaps its outputs reference are namespace-local to the
		// consuming workload, so they live in that same namespace — which is what makes
		// a namespace-scoped, read-only Role on that one agent sufficient.
		agentNs := dpNamespaceFor(req.Project)
		res.bindings.FetchAgentID = fetchAgentFor(res.bindings, agentNs)
		resourceBindings = append(resourceBindings, res.bindings)
		unconnectable = append(unconnectable, res.unconnectable...)
		for _, g := range res.grants {
			g.AgentNamespace = agentNs
			capGrants = append(capGrants, g)
			// Logged at Info, one line per authorized read, with the subject attached:
			// this is the record a reviewer wants, because it captures the authorization
			// decision rather than the agent's later read of it. It is a structured log
			// and not an entry in internal/openchoreo-api/audit, which keys off OpenAPI
			// operationIds for state-modifying REST/MCP routes — resolve is neither.
			// Never log the value; only where it lives and who was allowed it.
			h.logger.Info("remote-connect: authorized resource value read",
				"subject", subject, "namespace", req.Namespace, "project", req.Project,
				"environment", req.Environment, "resource", dep.Ref,
				"sourceKind", g.SourceKind, "sourceName", g.SourceName, "key", g.Key)
		}
		for i, target := range res.targets {
			if seenKeys[target.Key] {
				h.logger.Warn("remote-connect: duplicate dependency declaration ignored", "ref", target.Key)
				continue
			}
			seenKeys[target.Key] = true
			target.AgentNamespace = agentNs
			capTargets = append(capTargets, target)
			respTargets = append(respTargets, remoteconnect.ResolvedTarget{
				Key: target.Key, Proto: "tcp", Resource: res.renders[i], AgentID: agentNs,
			})
		}
	}

	capability, err := h.signer.sign(subject, req.Namespace,
		remoteconnect.ComponentRef{Project: req.Project, Name: req.Component}, req.Environment, capTargets, capGrants)
	if err != nil {
		return nil, fmt.Errorf("sign capability: %w", err)
	}

	resp := &remoteconnect.ResolveResponse{
		Capability:    capability,
		Targets:       respTargets,
		Unconnectable: unconnectable,
		Resources:     resourceBindings,
	}

	// Provision (or refresh) one remote-agent per distinct provider project+env namespace
	// the targets point at; occ dials each directly. Provisioning is idempotent, so an
	// agent already serving another session is simply reused (its last-used annotation
	// refreshes). A nil provisioner (resolution-only unit tests) skips provisioning.
	// Every dependency for the environment shares one data plane (Environment has one
	// DataPlaneRef), so a single dpClient reaches all the target namespaces.
	// A grant needs an agent just as a target does — a resource with no endpoint but a
	// secret-backed output still opens a tunnel, purely to fetch.
	if (len(capTargets) > 0 || len(capGrants) > 0) && h.provisioner != nil {
		dpClient, cerr := dpResult.GetK8sClient(h.planeClientProvider)
		if cerr != nil {
			return nil, fmt.Errorf("get data plane client: %w", cerr)
		}
		// Group the objects each agent must be able to read, so its Role can name them
		// explicitly instead of granting the whole namespace.
		reads := make(map[string]*agentReadSet)
		for _, g := range capGrants {
			set, ok := reads[g.AgentNamespace]
			if !ok {
				set = &agentReadSet{}
				reads[g.AgentNamespace] = set
			}
			set.add(g.SourceKind, g.SourceName)
		}

		namespaces := make([]string, 0, len(reads)+len(capTargets))
		for _, t := range capTargets {
			namespaces = append(namespaces, t.AgentNamespace)
		}
		for ns := range reads {
			namespaces = append(namespaces, ns)
		}

		agents := make(map[string]remoteconnect.AgentEndpoint)
		for _, ns := range namespaces {
			if _, done := agents[ns]; done {
				continue
			}
			info, perr := h.provisioner.ensureAgent(ctx, dpClient, ns, reads[ns])
			if perr != nil {
				return nil, fmt.Errorf("provision remote-agent in %s: %w", ns, perr)
			}
			agents[ns] = remoteconnect.AgentEndpoint{
				Endpoint: info.endpoint, CABundle: info.caBundle, ServerName: info.serverName,
			}
		}
		resp.Agents = agents
	}

	return resp, nil
}

// TouchAgent refreshes the last-used annotation of the remote-agent in dpNamespace so the
// reaper keeps it alive while a session is active; readsSecret also refreshes its read
// Role. The control-plane namespace + env locate the data plane (they share one per env);
// dpNamespace names the specific agent (the one that served the authorized stream).
// Best-effort: errors are returned for the caller to log, not surfaced to the data plane.
func (h *RemoteConnectHandler) TouchAgent(ctx context.Context, namespace, env, dpNamespace string, readsSecret bool) error {
	dpResult, err := h.resolveRemoteConnectPlane(ctx, namespace, env)
	if err != nil {
		return err
	}
	dpClient, err := dpResult.GetK8sClient(h.planeClientProvider)
	if err != nil {
		return err
	}
	return h.provisioner.touchLastUsed(ctx, dpClient, dpNamespace, readsSecret)
}

// resolveRemoteConnectPlane resolves the data plane serving env, reusing the same
// DataPlane/ClusterDataPlane mapping ExecHandler uses for `occ exec`.
func (h *RemoteConnectHandler) resolveRemoteConnectPlane(ctx context.Context, ns, envName string) (*controller.DataPlaneResult, error) {
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
func (h *RemoteConnectHandler) resolveEndpoint(ctx context.Context, ns, project, component, epName, visibility, env string) (*openchoreov1alpha1.EndpointURL, error) {
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

// resolvedResource is the outcome of resolving one resource dependency: a dial target
// per endpoint the ResourceType declares, and the tunnel-independent env bindings. A
// resource that declares no endpoints yields no targets and is not an error — a bucket
// name and a region are perfectly resolvable without anything to dial.
type resolvedResource struct {
	targets  []remoteconnect.Target
	renders  []*remoteconnect.ResourceRender
	bindings remoteconnect.ResourceBindings
	// grants are the ref-backed outputs occ may fetch over the tunnel. AgentNamespace
	// is left empty here and stamped by the caller, which owns the namespace mapping.
	grants []remoteconnect.SecretGrant
	// unconnectable reports endpoints that are declared but have no address to dial.
	unconnectable []remoteconnect.Unconnectable
}

// fetchAgentFor names the agent holding a resource's fetch values, or "" when it has none.
func fetchAgentFor(b remoteconnect.ResourceBindings, agentNs string) string {
	if len(b.FetchEnv) > 0 || len(b.FetchFile) > 0 {
		return agentNs
	}
	return ""
}

// resolveResource finds the provider ResourceReleaseBinding, verifies it is Ready, and
// turns each endpoint the binding resolved into a dial target. Which env vars follow a
// tunnel is decided by the endpoint's hostFrom/portFrom outputs, not by output naming:
// an endpoint whose address outputs are secret-backed still rewrites its bindings
// correctly, because the local address never comes from those outputs.
func (h *RemoteConnectHandler) resolveResource(ctx context.Context, ns, project string, dep remoteconnect.ResourceDep, env string, fetch fetchPolicy) (*resolvedResource, error) {
	rrb, err := h.findResourceReleaseBinding(ctx, ns, project, dep.Ref, env)
	if err != nil {
		return nil, err
	}
	if !isResourceReleaseBindingReady(rrb) {
		return nil, fmt.Errorf("resource %q is not ready in %s", dep.Ref, env)
	}

	outputs := make(map[string]openchoreov1alpha1.ResolvedResourceOutput, len(rrb.Status.Outputs))
	for _, o := range rrb.Status.Outputs {
		outputs[o.Name] = o
	}

	out := &resolvedResource{
		bindings: remoteconnect.ResourceBindings{Ref: dep.Ref, StaticEnv: map[string]string{}},
	}

	// An endpoint that did not resolve is absent from status.endpoints, so the loop below
	// cannot report it; carry the controller's reason instead.
	if reason := endpointsUnresolvedReason(rrb); reason != "" {
		out.unconnectable = append(out.unconnectable, remoteconnect.Unconnectable{
			Ref: remoteconnect.ResourceRefKey(dep.Ref), Reason: reason,
		})
	}

	// tunneled collects the output names whose bindings an endpoint will rewrite, so
	// they are not also emitted as their (unreachable) in-cluster value.
	tunneled := make(map[string]bool, 2*len(rrb.Status.Endpoints))
	for i := range rrb.Status.Endpoints {
		ep := &rrb.Status.Endpoints[i]
		key := remoteconnect.ResourceTargetKey(dep.Ref, ep.Name)

		// The address is published only through a Secret, which the control plane does
		// not read, so there is nothing to dial. Its outputs are deliberately left out
		// of tunneled: they fall through to the binding loop below and are reported
		// there, rather than being dropped as if a tunnel had claimed them.
		if ep.Host == "" || ep.Port == 0 {
			out.unconnectable = append(out.unconnectable, remoteconnect.Unconnectable{
				Ref: key,
				Reason: "endpoint address is published only through a Secret or ConfigMap " +
					"reference and was not resolved",
			})
			continue
		}

		// An endpoint that publishes its host as an output but not its port cannot be
		// tunneled: the host binding would be redirected to the local listener while
		// any port binding kept the in-cluster port, pointing the app at
		// 127.0.0.1:<remote port> — some unrelated local process, or nothing. Refusing
		// is the only safe answer; ResourceType validation rejects this shape, so
		// reaching it means an older ResourceRelease is still pinned.
		if ep.HostFrom != "" && ep.PortFrom == "" {
			out.unconnectable = append(out.unconnectable, remoteconnect.Unconnectable{
				Ref: key,
				Reason: "endpoint declares hostFrom without portFrom, so its port binding " +
					"cannot follow the tunnel; the resource type must publish the port as " +
					"an output and reference it from portFrom",
			})
			continue
		}

		// An endpoint whose address no output carries (a type publishing only a
		// composed URL) has no discrete bindings to point at the tunnel; the address
		// is substituted inside the composed value instead.
		var hostEnv, portEnv string
		if ep.HostFrom != "" {
			hostEnv = dep.EnvBindings[ep.HostFrom]
		}
		if ep.PortFrom != "" {
			portEnv = dep.EnvBindings[ep.PortFrom]
		}
		// Both halves must follow the tunnel together: redirecting one alone points the
		// app at 127.0.0.1:<in-cluster port> or <in-cluster host>:<local port>.
		if (hostEnv == "") != (portEnv == "") {
			bound, unbound := ep.HostFrom, ep.PortFrom
			if hostEnv == "" {
				bound, unbound = ep.PortFrom, ep.HostFrom
			}
			// A half declared inline has no output name to report as unbound.
			reason := fmt.Sprintf("workload binds the %q output but not %q, so the endpoint's "+
				"address cannot follow the tunnel; bind both or neither", bound, unbound)
			if unbound == "" {
				reason = fmt.Sprintf("workload binds the %q output, but this endpoint's other "+
					"half is declared inline and has no output to bind, so its address cannot "+
					"follow the tunnel; unbind %q, or publish both halves as outputs", bound, bound)
			}
			out.unconnectable = append(out.unconnectable, remoteconnect.Unconnectable{
				Ref:    key,
				Reason: reason,
			})
			hostEnv, portEnv = "", ""
		}

		// Only a redirected pair is tunneled; an output left as published is still emitted.
		if hostEnv != "" && portEnv != "" {
			tunneled[ep.HostFrom] = true
			tunneled[ep.PortFrom] = true
		}

		out.targets = append(out.targets, remoteconnect.Target{
			Key: key, Proto: "tcp", Host: ep.Host, Port: int(ep.Port),
		})
		out.renders = append(out.renders, &remoteconnect.ResourceRender{
			Ref:        dep.Ref,
			Endpoint:   ep.Name,
			RemoteAddr: net.JoinHostPort(ep.Host, strconv.Itoa(int(ep.Port))),
			HostEnv:    hostEnv,
			PortEnv:    portEnv,
		})
	}

	// Iterating a map yields env vars in random order, which would make an identical
	// request produce differently-ordered grants and omissions on every call. Sort so
	// the response — and the resourceNames list derived from it — is stable.
	for _, outputName := range slices.Sorted(maps.Keys(dep.EnvBindings)) {
		envVar := dep.EnvBindings[outputName]
		if tunneled[outputName] {
			continue // set from the local listener by the endpoint's render
		}
		o, ok := outputs[outputName]
		if !ok {
			// A typo in envBindings, or a binding written against a different release.
			// The deployed path fails the whole dependency on this; report it here so the
			// env var is not simply missing.
			out.bindings.OmittedSecretEnv = append(out.bindings.OmittedSecretEnv,
				remoteconnect.OmittedBinding{
					Target: envVar,
					Reason: fmt.Sprintf("resource publishes no output named %q", outputName),
				})
			continue
		}
		if !isRefBacked(o) {
			out.bindings.StaticEnv[envVar] = o.Value
			continue
		}
		grant, reason := out.grant(dep.Ref, outputName, o, fetch)
		if reason != "" {
			out.bindings.OmittedSecretEnv = append(out.bindings.OmittedSecretEnv,
				remoteconnect.OmittedBinding{Target: envVar, Reason: reason})
			continue
		}
		if out.bindings.FetchEnv == nil {
			out.bindings.FetchEnv = map[string]string{}
		}
		out.bindings.FetchEnv[envVar] = grant.Key
	}

	// File bindings are always ref-backed (the API rejects mounting a plain value), so
	// every one is a fetch. A path is the identity here, not an env var name.
	for _, outputName := range slices.Sorted(maps.Keys(dep.FileBindings)) {
		mountPath := dep.FileBindings[outputName]
		o, ok := outputs[outputName]
		if !ok {
			out.bindings.OmittedSecretEnv = append(out.bindings.OmittedSecretEnv,
				remoteconnect.OmittedBinding{
					Target: mountPath, File: true,
					Reason: fmt.Sprintf("resource publishes no output named %q", outputName),
				})
			continue
		}
		if !isRefBacked(o) {
			// A plain value has no data-plane object to mount, so the cluster would not
			// have produced a file either. Report rather than inventing one.
			out.bindings.OmittedSecretEnv = append(out.bindings.OmittedSecretEnv,
				remoteconnect.OmittedBinding{
					Target: mountPath, File: true,
					Reason: "output is a plain value, which has no file in the cluster either",
				})
			continue
		}
		grant, reason := out.grant(dep.Ref, outputName, o, fetch)
		if reason != "" {
			out.bindings.OmittedSecretEnv = append(out.bindings.OmittedSecretEnv,
				remoteconnect.OmittedBinding{Target: mountPath, Reason: reason, File: true})
			continue
		}
		if out.bindings.FetchFile == nil {
			out.bindings.FetchFile = map[string]string{}
		}
		out.bindings.FetchFile[mountPath] = grant.Key
	}

	return out, nil
}

// fetchPolicy says which ref-backed outputs of one resource may be fetched. It is
// resolved once per resource, from the operator kill switch and the caller's
// per-resource authorization, so the per-binding decision below is a lookup.
type fetchPolicy struct {
	// configMaps allows ConfigMap-backed reads. A ConfigMap value is not secret, so
	// this rides on the same resource:connect grant that authorized the dependency.
	configMaps bool
	// secrets allows Secret-backed reads, requiring ActionReadResourceSecrets.
	secrets bool
	// disabledReason, when set, is why fetching is off entirely (the operator switch).
	disabledReason string
}

// grant resolves one ref-backed output into a fetch grant, or returns the reason it
// cannot be fetched. A grant is recorded on the resolvedResource so the caller can sign
// it into the capability; the reason is phrased for the developer reading occ's output.
//
// Unlike the tunnel path, these reasons are specific rather than collapsed into
// unavailableReason: reaching here means the caller is already authorized to connect to
// this resource, so it discloses nothing they could not already learn.
func (r *resolvedResource) grant(ref, outputName string, o openchoreov1alpha1.ResolvedResourceOutput, fetch fetchPolicy) (remoteconnect.SecretGrant, string) {
	if fetch.disabledReason != "" {
		return remoteconnect.SecretGrant{}, fetch.disabledReason
	}
	grant, ok := grantFor(ref, outputName, o)
	if !ok {
		return remoteconnect.SecretGrant{}, "output reference names no object or key"
	}
	if grant.SourceKind == remoteconnect.SourceKindSecret && !fetch.secrets {
		return remoteconnect.SecretGrant{},
			"secret-backed, and your role does not grant " + authz.ActionReadResourceSecrets
	}
	if grant.SourceKind == remoteconnect.SourceKindConfigMap && !fetch.configMaps {
		return remoteconnect.SecretGrant{}, "not authorized to read this resource's configuration"
	}
	// One output may be bound to both an env var and a file; the grant is per output,
	// so record it once and let both bindings reference the same key.
	for _, existing := range r.grants {
		if existing.Key == grant.Key {
			return existing, ""
		}
	}
	r.grants = append(r.grants, grant)
	return grant, ""
}

// findReleaseBinding lists ReleaseBindings in the namespace and returns the single one
// matching (project, component, environment) that is not being undeployed. The API
// server's client is uncached with no field indexes, so we list + filter in memory.
func (h *RemoteConnectHandler) findReleaseBinding(ctx context.Context, ns, project, component, env string) (*openchoreov1alpha1.ReleaseBinding, error) {
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

func (h *RemoteConnectHandler) findResourceReleaseBinding(ctx context.Context, ns, project, resource, env string) (*openchoreov1alpha1.ResourceReleaseBinding, error) {
	var list openchoreov1alpha1.ResourceReleaseBindingList
	if err := h.k8sClient.List(ctx, &list, client.InNamespace(ns)); err != nil {
		return nil, fmt.Errorf("list resource release bindings: %w", err)
	}
	var match *openchoreov1alpha1.ResourceReleaseBinding
	for i := range list.Items {
		rrb := &list.Items[i]
		if rrb.Spec.Owner.ProjectName != project || rrb.Spec.Owner.ResourceName != resource || rrb.Spec.Environment != env {
			continue
		}
		if match != nil {
			return nil, fmt.Errorf("multiple resource release bindings match %s/%s in %s", project, resource, env)
		}
		match = rrb
	}
	if match == nil {
		return nil, fmt.Errorf("no resource release binding for %s/%s in %s", project, resource, env)
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

// isRefBacked reports whether an output's value lives behind a data-plane object
// reference rather than in the status itself. Such a value is never visible to the
// control plane and must be fetched by the remote-agent.
func isRefBacked(o openchoreov1alpha1.ResolvedResourceOutput) bool {
	return o.SecretKeyRef != nil || o.ConfigMapKeyRef != nil
}

// grantFor turns a ref-backed output into the coordinates of the value, or reports that
// the output carries no reference at all. A Secret and a ConfigMap are distinguished
// here and stay distinguished all the way to the agent, so a ConfigMap read can never
// be satisfied from a Secret: only Secret reads are gated on
// ActionReadResourceSecrets, and conflating the two would let the weaker grant reach
// the stronger object.
func grantFor(ref, outputName string, o openchoreov1alpha1.ResolvedResourceOutput) (remoteconnect.SecretGrant, bool) {
	g := remoteconnect.SecretGrant{Key: remoteconnect.SecretGrantKey(ref, outputName)}
	switch {
	case o.SecretKeyRef != nil:
		g.SourceKind = remoteconnect.SourceKindSecret
		g.SourceName = o.SecretKeyRef.Name
		g.SourceKey = o.SecretKeyRef.Key
	case o.ConfigMapKeyRef != nil:
		g.SourceKind = remoteconnect.SourceKindConfigMap
		g.SourceName = o.ConfigMapKeyRef.Name
		g.SourceKey = o.ConfigMapKeyRef.Key
	default:
		return remoteconnect.SecretGrant{}, false
	}
	if g.SourceName == "" || g.SourceKey == "" {
		return remoteconnect.SecretGrant{}, false
	}
	return g, true
}

// isResourceReleaseBindingReady mirrors the consumer controller's readiness gate: the
// aggregate Ready condition is True and observed the current generation.
// endpointsUnresolvedReason returns why a resource's endpoints did not resolve, or "" when
// they did or the type declares none.
func endpointsUnresolvedReason(rrb *openchoreov1alpha1.ResourceReleaseBinding) string {
	cond := meta.FindStatusCondition(rrb.Status.Conditions, string(resourcereleasebinding.ConditionEndpointsResolved))
	if cond == nil || cond.Status == metav1.ConditionTrue {
		return ""
	}
	if cond.Reason == string(resourcereleasebinding.ReasonEndpointsPending) {
		return "endpoint address is not resolved yet; retry once the resource reports it (" + cond.Message + ")"
	}
	return "endpoint could not be resolved by the control plane (" + cond.Message + ")"
}

func isResourceReleaseBindingReady(rrb *openchoreov1alpha1.ResourceReleaseBinding) bool {
	cond := meta.FindStatusCondition(rrb.Status.Conditions, string(resourcereleasebinding.ConditionReady))
	return cond != nil && cond.Status == metav1.ConditionTrue && cond.ObservedGeneration == rrb.Generation
}

// capabilitySigner mints capability JWTs.
type capabilitySigner struct {
	privKey ed25519.PrivateKey
	keyID   string
	issuer  string
	ttl     time.Duration
	// secretTTL is the (shorter) lifetime for a capability carrying secret grants. The
	// per-stream authorize callback re-checks no policy, so a capability's expiry is the
	// entire revocation window for the reads it authorizes. Zero uses ttl.
	secretTTL time.Duration
}

func (s *capabilitySigner) sign(subject, namespace string, comp remoteconnect.ComponentRef, env string,
	targets []remoteconnect.Target, grants []remoteconnect.SecretGrant) (string, error) {
	now := time.Now()
	ttl := s.ttl
	if len(grants) > 0 && s.secretTTL > 0 {
		ttl = s.secretTTL
	}
	claims := &remoteconnect.CapabilityClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    s.issuer,
			Subject:   subject,
			Audience:  jwt.ClaimStrings{remoteconnect.CapabilityAudience},
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
		},
		Namespace: namespace,
		Component: comp,
		Env:       env,
		Targets:   targets,
		Secrets:   grants,
	}
	return remoteconnect.SignCapability(claims, s.privKey, s.keyID)
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
