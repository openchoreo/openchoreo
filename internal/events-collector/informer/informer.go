// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package informer

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
)

const (
	// defaultResyncPeriod is how often the informer re-lists all events.
	defaultResyncPeriod = 30 * time.Minute
)

// EventHandler processes Kubernetes events.
type EventHandler interface {
	HandleEvent(ctx context.Context, ev *corev1.Event)
}

// EventInformer watches Kubernetes events using a shared informer
// and delegates processing to the event handler.
type EventInformer struct {
	clientset *kubernetes.Clientset
	handler   EventHandler
	logger    *slog.Logger
}

// New creates a new event informer.
func New(clientset *kubernetes.Clientset, handler EventHandler, logger *slog.Logger) *EventInformer {
	return &EventInformer{
		clientset: clientset,
		handler:   handler,
		logger:    logger,
	}
}

// Start begins watching Kubernetes events and blocks until the context is cancelled.
func (ei *EventInformer) Start(ctx context.Context) error {
	factory := informers.NewSharedInformerFactory(ei.clientset, defaultResyncPeriod)

	eventInformer := factory.Core().V1().Events().Informer()

	_, err := eventInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			ev, ok := obj.(*corev1.Event)
			if !ok {
				return
			}
			ei.handler.HandleEvent(ctx, ev)
		},
		UpdateFunc: func(_, newObj interface{}) {
			ev, ok := newObj.(*corev1.Event)
			if !ok {
				return
			}
			ei.handler.HandleEvent(ctx, ev)
		},
	})
	if err != nil {
		return fmt.Errorf("failed to add event handler to informer: %w", err)
	}

	ei.logger.Info("starting event informer")

	factory.Start(ctx.Done())

	// Wait for cache sync
	synced := factory.WaitForCacheSync(ctx.Done())
	for informerType, ok := range synced {
		if !ok {
			ei.logger.Warn("informer cache sync failed", "type", informerType)
		}
	}
	ei.logger.Info("event informer cache synced")

	// Block until context is cancelled
	<-ctx.Done()
	ei.logger.Info("event informer stopped")

	return nil
}
