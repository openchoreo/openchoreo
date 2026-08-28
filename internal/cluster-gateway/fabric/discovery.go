// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package fabric

import (
	"context"
	"log/slog"
	"net"
	"sort"
	"strconv"
	"strings"
	"sync"

	discoveryv1 "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
)

// EndpointSliceDiscovery is the embedded PeerDiscovery: a watch (not a poll)
// on the EndpointSlices of the gateway's headless mesh Service. It needs
// nothing beside the platform itself — no external registry.
type EndpointSliceDiscovery struct {
	client      kubernetes.Interface
	namespace   string
	serviceName string
	meshPort    int
	selfPodName string
	logger      *slog.Logger

	// newListerWatcher builds the source the informer consumes. Overridable so
	// tests can drive peer sets through the real emit path: a fake clientset's
	// RESTClient is not backed by its object tracker, so the production
	// construction below cannot be exercised without an API server.
	newListerWatcher func() cache.ListerWatcher
}

func (d *EndpointSliceDiscovery) listerWatcher() cache.ListerWatcher {
	if d.newListerWatcher != nil {
		return d.newListerWatcher()
	}
	return cache.NewFilteredListWatchFromClient(
		d.client.DiscoveryV1().RESTClient(),
		"endpointslices",
		d.namespace,
		func(options *metav1.ListOptions) {
			options.LabelSelector = discoveryv1.LabelServiceName + "=" + d.serviceName
		},
	)
}

// NewEndpointSliceDiscovery watches the EndpointSlices of serviceName in
// namespace and reports ready endpoints (excluding selfPodName) as mesh peers
// on meshPort.
func NewEndpointSliceDiscovery(
	client kubernetes.Interface,
	namespace, serviceName string,
	meshPort int,
	selfPodName string,
	logger *slog.Logger,
) *EndpointSliceDiscovery {
	return &EndpointSliceDiscovery{
		client:      client,
		namespace:   namespace,
		serviceName: serviceName,
		meshPort:    meshPort,
		selfPodName: selfPodName,
		logger:      logger.With("component", "fabric-discovery"),
	}
}

// Watch starts an EndpointSlice informer and delivers the full peer set on
// every change. The informer handles list/watch retries and re-syncs
// internally.
func (d *EndpointSliceDiscovery) Watch(ctx context.Context) (<-chan []Peer, error) {
	informer := cache.NewSharedIndexInformer(d.listerWatcher(), &discoveryv1.EndpointSlice{}, 0, cache.Indexers{})

	ch := make(chan []Peer, 16)
	var mu sync.Mutex
	// emitted, rather than a zero-value fingerprint, is what marks "nothing
	// reported yet": an empty peer set fingerprints to "" too, so a pod with
	// no peers - the single-replica default, or the first pod up in a rollout
	// - would suppress its own first emission as a duplicate. The mesh would
	// then never see discovery report, and hold the pod out of the Service
	// for the whole convergence grace period waiting for peers it does not
	// have.
	var emitted bool
	var lastFingerprint string

	emit := func() {
		peers := d.peersFromStore(informer.GetStore())
		fp := peerFingerprint(peers)

		mu.Lock()
		defer mu.Unlock()
		if emitted && fp == lastFingerprint {
			return
		}
		emitted, lastFingerprint = true, fp

		select {
		case ch <- peers:
		default:
			// Consumer is behind: drop the stale pending set and queue the
			// latest one — only the most recent peer set matters.
			select {
			case <-ch:
			default:
			}
			select {
			case ch <- peers:
			default:
			}
		}
		d.logger.Info("peer set changed", "peers", len(peers))
	}

	_, err := informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    func(any) { emit() },
		UpdateFunc: func(any, any) { emit() },
		DeleteFunc: func(any) { emit() },
	})
	if err != nil {
		return nil, err
	}

	go func() {
		defer close(ch)
		informer.Run(ctx.Done())
	}()

	return ch, nil
}

// peersFromStore flattens all EndpointSlices in the informer store into a
// deduplicated, sorted peer list.
func (d *EndpointSliceDiscovery) peersFromStore(store cache.Store) []Peer {
	byID := make(map[string]Peer)

	for _, obj := range store.List() {
		slice, ok := obj.(*discoveryv1.EndpointSlice)
		if !ok {
			continue
		}
		for _, ep := range slice.Endpoints {
			if ep.Conditions.Ready != nil && !*ep.Conditions.Ready {
				continue
			}
			if len(ep.Addresses) == 0 {
				continue
			}

			id := ep.Addresses[0]
			if ep.TargetRef != nil && ep.TargetRef.Name != "" {
				id = ep.TargetRef.Name
			}
			if id == d.selfPodName {
				continue
			}

			byID[id] = Peer{
				ID:   id,
				Addr: net.JoinHostPort(ep.Addresses[0], strconv.Itoa(d.meshPort)),
			}
		}
	}

	peers := make([]Peer, 0, len(byID))
	for _, p := range byID {
		peers = append(peers, p)
	}
	sort.Slice(peers, func(i, j int) bool { return peers[i].ID < peers[j].ID })
	return peers
}

func peerFingerprint(peers []Peer) string {
	parts := make([]string, len(peers))
	for i, p := range peers {
		parts[i] = p.ID + "@" + p.Addr
	}
	return strings.Join(parts, ",")
}
