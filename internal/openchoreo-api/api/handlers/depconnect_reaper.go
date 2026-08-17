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
	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	kubernetesClient "github.com/openchoreo/openchoreo/internal/clients/kubernetes"
	"github.com/openchoreo/openchoreo/internal/controller"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/config"
)

// DepAgentReaper periodically deletes dep-agents idle past the configured TTL.
// Lifecycle is imperative (no CRD/controller): resolve and per-stream authorize
// refresh a last-used annotation; this reaper GCs any agent whose annotation is
// older than the TTL, across every data plane, so shared per-project+env agents are
// torn down once no session has touched them recently.
type DepAgentReaper struct {
	k8sClient           client.Client
	planeClientProvider kubernetesClient.DataPlaneClientProvider
	cfg                 config.DepConnectConfig
	now                 func() time.Time
	logger              *slog.Logger
}

// NewDepAgentReaper builds the reaper.
func NewDepAgentReaper(k8sClient client.Client, provider kubernetesClient.DataPlaneClientProvider, cfg config.DepConnectConfig, logger *slog.Logger) *DepAgentReaper {
	return &DepAgentReaper{
		k8sClient:           k8sClient,
		planeClientProvider: provider,
		cfg:                 cfg,
		now:                 time.Now,
		logger:              logger.With("component", "dep-connect-reaper"),
	}
}

// Start runs the reap loop until ctx is cancelled. Intended to be called in a goroutine.
func (r *DepAgentReaper) Start(ctx context.Context) {
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
func (r *DepAgentReaper) reapOnce(ctx context.Context) {
	clients, err := r.dataPlaneClients(ctx)
	if err != nil {
		r.logger.Warn("reaper: failed to enumerate data planes", "error", err)
		return
	}
	cutoff := r.now().Add(-r.cfg.ReaperTTL())
	for _, dpClient := range clients {
		r.reapPlane(ctx, dpClient, cutoff)
	}
}

func (r *DepAgentReaper) reapPlane(ctx context.Context, dpClient client.Client, cutoff time.Time) {
	var deps appsv1.DeploymentList
	if err := dpClient.List(ctx, &deps, client.MatchingLabels{"app.kubernetes.io/managed-by": managedByLabelValue}); err != nil {
		r.logger.Warn("reaper: list dep-agents failed", "error", err)
		return
	}
	for i := range deps.Items {
		dep := &deps.Items[i]
		ts := dep.Annotations[lastUsedAnnotation]
		lastUsed, perr := time.Parse(time.RFC3339, ts)
		if perr != nil {
			// No/invalid annotation: treat as stale to avoid leaking orphans.
			r.logger.Debug("reaper: dep-agent missing last-used annotation; reaping", "namespace", dep.Namespace)
		} else if lastUsed.After(cutoff) {
			continue // still fresh
		}
		r.deleteAgent(ctx, dpClient, dep.Namespace, cutoff)
	}
}

// deleteAgent removes the dep-agent Deployment, Service, and cert Secret in ns, after
// re-reading the Deployment so an agent claimed since the list is left alone.
func (r *DepAgentReaper) deleteAgent(ctx context.Context, dpClient client.Client, ns string, cutoff time.Time) {
	current := &appsv1.Deployment{}
	if err := dpClient.Get(ctx, client.ObjectKey{Namespace: ns, Name: depAgentName}, current); err != nil {
		if client.IgnoreNotFound(err) != nil {
			r.logger.Warn("reaper: re-read dep-agent failed", "namespace", ns, "error", err)
		}
		return
	}
	if lastUsed, perr := time.Parse(time.RFC3339, current.Annotations[lastUsedAnnotation]); perr == nil && lastUsed.After(cutoff) {
		r.logger.Debug("reaper: dep-agent became active; skipping", "namespace", ns)
		return
	}

	r.logger.Info("reaping idle dep-agent", "namespace", ns)
	objs := []client.Object{
		&appsv1.Deployment{},
		&corev1.Service{},
		&corev1.Secret{},
	}
	for _, obj := range objs {
		obj.SetName(depAgentName)
		obj.SetNamespace(ns)
		err := dpClient.Delete(ctx, obj)
		if client.IgnoreNotFound(err) != nil {
			r.logger.Warn("reaper: delete failed", "namespace", ns, "error", err)
		}
	}
}

// dataPlaneClients returns a proxy client for every DataPlane and ClusterDataPlane.
func (r *DepAgentReaper) dataPlaneClients(ctx context.Context) ([]client.Client, error) {
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
