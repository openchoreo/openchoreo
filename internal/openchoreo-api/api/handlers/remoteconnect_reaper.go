// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	kubernetesClient "github.com/openchoreo/openchoreo/internal/clients/kubernetes"
	"github.com/openchoreo/openchoreo/internal/controller"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/config"
)

// RemoteAgentReaper periodically deletes remote-agents idle past the configured TTL.
// Lifecycle is imperative (no CRD/controller): resolve and per-stream authorize
// refresh a last-used annotation; this reaper GCs any agent whose annotation is
// older than the TTL, across every data plane, so shared per-project+env agents are
// torn down once no session has touched them recently.
type RemoteAgentReaper struct {
	k8sClient           client.Client
	planeClientProvider kubernetesClient.DataPlaneClientProvider
	cfg                 config.RemoteConnectConfig
	now                 func() time.Time
	logger              *slog.Logger
}

// NewRemoteAgentReaper builds the reaper.
func NewRemoteAgentReaper(k8sClient client.Client, provider kubernetesClient.DataPlaneClientProvider, cfg config.RemoteConnectConfig, logger *slog.Logger) *RemoteAgentReaper {
	return &RemoteAgentReaper{
		k8sClient:           k8sClient,
		planeClientProvider: provider,
		cfg:                 cfg,
		now:                 time.Now,
		logger:              logger.With("component", "remote-connect-reaper"),
	}
}

// Start runs the reap loop until ctx is cancelled. Intended to be called in a goroutine.
func (r *RemoteAgentReaper) Start(ctx context.Context) {
	ticker := time.NewTicker(r.cfg.ReaperInterval())
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.reapOnce(ctx)
		}
	}
}

// reapOnce sweeps every data plane once. It is best-effort: per-plane errors are
// logged and do not abort the sweep.
func (r *RemoteAgentReaper) reapOnce(ctx context.Context) {
	clients, err := r.dataPlaneClients(ctx)
	if err != nil {
		r.logger.Warn("reaper: failed to enumerate data planes", "error", err)
		return
	}
	now := r.now()
	cutoff := now.Add(-r.cfg.ReaperTTL())
	grantCutoff := now.Add(-r.cfg.GrantTTL())
	for _, dpClient := range clients {
		r.reapPlane(ctx, dpClient, cutoff, grantCutoff)
	}
}

func (r *RemoteAgentReaper) reapPlane(ctx context.Context, dpClient client.Client, cutoff, grantCutoff time.Time) {
	var deps appsv1.DeploymentList
	if err := dpClient.List(ctx, &deps, client.MatchingLabels{"app.kubernetes.io/managed-by": managedByLabelValue}); err != nil {
		r.logger.Warn("reaper: list remote-agents failed", "error", err)
		return
	}
	withDeployment := make(map[string]bool, len(deps.Items))
	for i := range deps.Items {
		dep := &deps.Items[i]
		withDeployment[dep.Namespace] = true
		ts := dep.Annotations[lastUsedAnnotation]
		lastUsed, perr := time.Parse(time.RFC3339, ts)
		if perr != nil {
			// No/invalid annotation: treat as stale to avoid leaking orphans.
			r.logger.Debug("reaper: remote-agent missing last-used annotation; reaping", "namespace", dep.Namespace)
		} else if lastUsed.After(cutoff) {
			continue // still fresh
		}
		r.deleteAgent(ctx, dpClient, dep.Namespace, cutoff)
	}
	r.reapOrphans(ctx, dpClient, cutoff, withDeployment)
	r.reapReadGrants(ctx, dpClient, grantCutoff, withDeployment)
}

// reapReadGrants removes the read Role and RoleBinding of an agent whose last read is
// older than grantCutoff, leaving the agent running. A session reads its values once at
// startup, so a live session keeps working. The ServiceAccount is kept: the Deployment
// names it, and removing it would stop the agent being admitted.
func (r *RemoteAgentReaper) reapReadGrants(ctx context.Context, dpClient client.Client, grantCutoff time.Time, withDeployment map[string]bool) {
	if r.cfg.GrantTTL() <= 0 {
		// Without a lifetime every grant looks stale, revoking reads cluster-wide.
		r.logger.Warn("reaper: grant TTL is not positive; leaving read grants in place")
		return
	}
	var roles rbacv1.RoleList
	if err := dpClient.List(ctx, &roles, client.MatchingLabels{"app.kubernetes.io/managed-by": managedByLabelValue}); err != nil {
		r.logger.Warn("reaper: list remote-agent roles failed", "error", err)
		return
	}
	for i := range roles.Items {
		role := &roles.Items[i]
		// A namespace with no agent is swept above, Role included.
		if !withDeployment[role.Namespace] || !orphanIdle(role, grantCutoff) {
			continue
		}
		r.logger.Info("reaping unused remote-agent read grants", "namespace", role.Namespace)
		// The listing is a snapshot, and a resolve may have refreshed this Role since.
		// The resourceVersion precondition makes the delete fail instead of revoking a
		// grant a live session just took; the RoleBinding goes only once it holds.
		if err := dpClient.Delete(ctx, role, client.Preconditions{ResourceVersion: &role.ResourceVersion}); err != nil {
			if apierrors.IsConflict(err) {
				r.logger.Debug("reaper: read grant refreshed since listing; keeping it",
					"namespace", role.Namespace)
			} else if !apierrors.IsNotFound(err) {
				r.logger.Warn("reaper: delete read role failed", "namespace", role.Namespace, "error", err)
			}
			continue
		}
		binding := &rbacv1.RoleBinding{}
		binding.Name = remoteAgentName
		binding.Namespace = role.Namespace
		if err := client.IgnoreNotFound(dpClient.Delete(ctx, binding)); err != nil {
			r.logger.Warn("reaper: delete read role binding failed", "namespace", role.Namespace, "error", err)
		}
	}
	r.reapRoleBindingsWithoutARole(ctx, dpClient, withDeployment)
}

// reapRoleBindingsWithoutARole removes a RoleBinding left behind by a Role delete whose
// own delete failed. ensureReadRBAC always writes the Role first, so a binding with no
// Role has only that cause, and the binding grants nothing while it lasts.
//
// Absence of the Role is the signal, not the binding's own last-used stamp: a read
// refreshes the Role, while the binding's stamp moves only when a resolve re-applies it,
// so a long-lived session has a fresh Role and a stale binding.
func (r *RemoteAgentReaper) reapRoleBindingsWithoutARole(ctx context.Context, dpClient client.Client, withDeployment map[string]bool) {
	var bindings rbacv1.RoleBindingList
	if err := dpClient.List(ctx, &bindings, client.MatchingLabels{"app.kubernetes.io/managed-by": managedByLabelValue}); err != nil {
		r.logger.Warn("reaper: list remote-agent role bindings failed", "error", err)
		return
	}
	for i := range bindings.Items {
		binding := &bindings.Items[i]
		// A namespace with no agent is swept whole, binding included.
		if !withDeployment[binding.Namespace] {
			continue
		}
		err := dpClient.Get(ctx, client.ObjectKey{Namespace: binding.Namespace, Name: remoteAgentName}, &rbacv1.Role{})
		if err == nil || client.IgnoreNotFound(err) != nil {
			continue
		}
		r.logger.Info("reaping remote-agent role binding left without a role", "namespace", binding.Namespace)
		delErr := dpClient.Delete(ctx, binding, client.Preconditions{ResourceVersion: &binding.ResourceVersion})
		if delErr != nil && !apierrors.IsNotFound(delErr) && !apierrors.IsConflict(delErr) {
			r.logger.Warn("reaper: delete orphaned read role binding failed",
				"namespace", binding.Namespace, "error", delErr)
		}
	}
}

// reapOrphans removes agent objects in namespaces with no agent Deployment. The cert
// Secret is applied first, so it is present for every partial provision.
func (r *RemoteAgentReaper) reapOrphans(ctx context.Context, dpClient client.Client, cutoff time.Time, withDeployment map[string]bool) {
	var secrets corev1.SecretList
	if err := dpClient.List(ctx, &secrets, client.MatchingLabels{"app.kubernetes.io/managed-by": managedByLabelValue}); err != nil {
		r.logger.Warn("reaper: list remote-agent certs failed", "error", err)
		return
	}
	for i := range secrets.Items {
		sec := &secrets.Items[i]
		if withDeployment[sec.Namespace] || !orphanIdle(sec, cutoff) {
			continue
		}
		// The listing is a snapshot and a reused cert leaves the Secret's stamp untouched,
		// so neither shows a namespace provisioned since. Re-read before deleting.
		if r.agentProvisioned(ctx, dpClient, sec.Namespace) {
			r.logger.Debug("reaper: remote-agent provisioned since listing; skipping orphan sweep",
				"namespace", sec.Namespace)
			continue
		}
		r.logger.Info("reaping orphaned remote-agent objects", "namespace", sec.Namespace)
		r.deleteAgentObjects(ctx, dpClient, sec.Namespace)
	}
}

// agentProvisioned reports whether an agent Deployment exists in ns. A read failure
// counts as provisioned; the sweep runs again on the next tick.
func (r *RemoteAgentReaper) agentProvisioned(ctx context.Context, dpClient client.Client, ns string) bool {
	dep := &appsv1.Deployment{}
	err := dpClient.Get(ctx, client.ObjectKey{Namespace: ns, Name: remoteAgentName}, dep)
	if err == nil {
		return true
	}
	if client.IgnoreNotFound(err) != nil {
		r.logger.Warn("reaper: re-read remote-agent failed", "namespace", ns, "error", err)
		return true
	}
	return false
}

// orphanIdle reports whether an orphan is old enough to remove rather than a provision
// still in flight.
func orphanIdle(obj client.Object, cutoff time.Time) bool {
	if ts, err := time.Parse(time.RFC3339, obj.GetAnnotations()[lastUsedAnnotation]); err == nil {
		return ts.Before(cutoff)
	}
	return obj.GetCreationTimestamp().Time.Before(cutoff)
}

// deleteAgent removes the remote-agent Deployment, Service, and cert Secret in ns, after
// re-reading the Deployment so an agent claimed since the list is left alone.
func (r *RemoteAgentReaper) deleteAgent(ctx context.Context, dpClient client.Client, ns string, cutoff time.Time) {
	current := &appsv1.Deployment{}
	if err := dpClient.Get(ctx, client.ObjectKey{Namespace: ns, Name: remoteAgentName}, current); err != nil {
		if client.IgnoreNotFound(err) != nil {
			r.logger.Warn("reaper: re-read remote-agent failed", "namespace", ns, "error", err)
		}
		return
	}
	if lastUsed, perr := time.Parse(time.RFC3339, current.Annotations[lastUsedAnnotation]); perr == nil && lastUsed.After(cutoff) {
		r.logger.Debug("reaper: remote-agent became active; skipping", "namespace", ns)
		return
	}

	r.logger.Info("reaping idle remote-agent", "namespace", ns)
	r.deleteAgentObjects(ctx, dpClient, ns)
}

// deleteAgentObjects removes every object the provisioner applies for the agent in ns.
func (r *RemoteAgentReaper) deleteAgentObjects(ctx context.Context, dpClient client.Client, ns string) {
	// RoleBinding before Role before ServiceAccount, so a partial failure never leaves a
	// binding pointing at a deleted Role (which reads as a misconfiguration rather than
	// a half-finished delete).
	objs := []client.Object{
		&appsv1.Deployment{},
		&corev1.Service{},
		&corev1.Secret{},
		&rbacv1.RoleBinding{},
		&rbacv1.Role{},
		&corev1.ServiceAccount{},
	}
	for _, obj := range objs {
		obj.SetName(remoteAgentName)
		obj.SetNamespace(ns)
		err := dpClient.Delete(ctx, obj)
		if client.IgnoreNotFound(err) != nil {
			r.logger.Warn("reaper: delete failed", "namespace", ns, "error", err)
		}
	}
}

// dataPlaneClients returns a proxy client for every DataPlane and ClusterDataPlane.
func (r *RemoteAgentReaper) dataPlaneClients(ctx context.Context) ([]client.Client, error) {
	var out []client.Client

	var dpList openchoreov1alpha1.DataPlaneList
	if err := r.k8sClient.List(ctx, &dpList); err != nil {
		return nil, fmt.Errorf("list DataPlanes: %w", err)
	}
	for i := range dpList.Items {
		res := &controller.DataPlaneResult{DataPlane: &dpList.Items[i]}
		c, err := res.GetK8sClient(r.planeClientProvider)
		if err != nil {
			r.logger.Warn("reaper: data plane client failed", "dataplane", dpList.Items[i].Name, "error", err)
			continue
		}
		out = append(out, c)
	}

	var cdpList openchoreov1alpha1.ClusterDataPlaneList
	if err := r.k8sClient.List(ctx, &cdpList); err != nil {
		return nil, fmt.Errorf("list ClusterDataPlanes: %w", err)
	}
	for i := range cdpList.Items {
		res := &controller.DataPlaneResult{ClusterDataPlane: &cdpList.Items[i]}
		c, err := res.GetK8sClient(r.planeClientProvider)
		if err != nil {
			r.logger.Warn("reaper: cluster data plane client failed", "clusterdataplane", cdpList.Items[i].Name, "error", err)
			continue
		}
		out = append(out, c)
	}

	return out, nil
}
