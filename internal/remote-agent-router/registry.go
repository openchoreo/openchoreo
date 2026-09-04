// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package remoteagentrouter

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"sync"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// registry maps a remote-agent's SNI host to its in-cluster backend address, discovered
// by listing the per-project+env remote-agent Services the control plane provisions
// (labeled + annotated with their SNI). It refreshes periodically and on a lookup
// miss, so a freshly-provisioned agent becomes routable within a refresh cycle.
type registry struct {
	client     kubernetes.Interface
	labelSel   string
	sniAnnoKey string
	log        *slog.Logger

	mu     sync.RWMutex
	byHost map[string]string

	// missMu serializes miss-triggered refreshes and guards lastMissRefresh.
	missMu          sync.Mutex
	lastMissRefresh time.Time
}

// missRefreshCooldown is the minimum spacing between miss-triggered refreshes. Unknown
// SNIs arrive before any authentication, so they must not drive one List per connection.
const missRefreshCooldown = time.Second

func newRegistry(client kubernetes.Interface, labelSelector, sniAnnotationKey string, log *slog.Logger) *registry {
	return &registry{
		client:     client,
		labelSel:   labelSelector,
		sniAnnoKey: sniAnnotationKey,
		log:        log,
		byHost:     map[string]string{},
	}
}

// Start refreshes the backend map every interval until ctx is done.
func (r *registry) Start(ctx context.Context, interval time.Duration) {
	r.refresh(ctx)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.refresh(ctx)
		}
	}
}

// lookup returns the backend "host:port" for an SNI host, refreshing once on a miss.
func (r *registry) lookup(ctx context.Context, host string) (string, bool) {
	r.mu.RLock()
	addr, ok := r.byHost[host]
	r.mu.RUnlock()
	if ok {
		return addr, true
	}
	// Miss: the agent may have just been provisioned — refresh and retry once.
	r.refreshOnMiss(ctx)
	r.mu.RLock()
	addr, ok = r.byHost[host]
	r.mu.RUnlock()
	return addr, ok
}

// refreshOnMiss refreshes at most once per missRefreshCooldown. Concurrent callers
// block on the same mutex, so a burst of misses costs one List, and they observe the
// map it produced.
func (r *registry) refreshOnMiss(ctx context.Context) {
	r.missMu.Lock()
	defer r.missMu.Unlock()
	if time.Since(r.lastMissRefresh) < missRefreshCooldown {
		return
	}
	r.refresh(ctx)
	r.lastMissRefresh = time.Now()
}

// refresh rebuilds the SNI->backend map from the current remote-agent Services.
func (r *registry) refresh(ctx context.Context) {
	list, err := r.client.CoreV1().Services("").List(ctx, metav1.ListOptions{LabelSelector: r.labelSel})
	if err != nil {
		r.log.Warn("registry refresh failed", "error", err)
		return
	}
	next := make(map[string]string, len(list.Items))
	for i := range list.Items {
		svc := &list.Items[i]
		sni := svc.Annotations[r.sniAnnoKey]
		if sni == "" || len(svc.Spec.Ports) == 0 {
			continue
		}
		port := svc.Spec.Ports[0].Port
		backend := fmt.Sprintf("%s.%s.svc.cluster.local:%s", svc.Name, svc.Namespace, strconv.Itoa(int(port)))
		// One SNI resolves to one backend: keep the first and report the clash.
		if existing, dup := next[sni]; dup {
			if existing != backend {
				r.log.Warn("duplicate remote-connect SNI; ignoring", "sni", sni, "kept", existing, "ignored", backend)
			}
			continue
		}
		next[sni] = backend
	}
	r.mu.Lock()
	r.byHost = next
	r.mu.Unlock()
	r.log.Debug("registry refreshed", "agents", len(next))
}
