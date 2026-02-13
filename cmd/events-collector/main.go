// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/openchoreo/openchoreo/internal/cmdutil"
	"github.com/openchoreo/openchoreo/internal/events-collector/checkpoint"
	"github.com/openchoreo/openchoreo/internal/events-collector/handler"
	"github.com/openchoreo/openchoreo/internal/events-collector/informer"
	"github.com/openchoreo/openchoreo/internal/events-collector/labelcache"
	"github.com/openchoreo/openchoreo/internal/events-collector/labelresolver"
)

const (
	defaultCheckpointCleanupInterval = 10 * time.Minute
	defaultCheckpointTTL             = 1 * time.Hour
	defaultLabelCacheTTL             = 5 * time.Minute
	defaultLabelCacheEvictInterval   = 1 * time.Minute
)

func main() {
	var (
		kubeconfig                string
		logLevel                  string
		checkpointDir             string
		checkpointCleanupInterval time.Duration
		checkpointTTL             time.Duration
		labelCacheTTL             time.Duration
	)

	flag.StringVar(&kubeconfig, "kubeconfig", cmdutil.GetEnv("KUBECONFIG", ""),
		"Path to kubeconfig file (for local development, defaults to in-cluster config)")
	flag.StringVar(&logLevel, "log-level", cmdutil.GetEnv("LOG_LEVEL", "info"),
		"Log level (debug, info, warn, error)")
	flag.StringVar(&checkpointDir, "checkpoint-dir", cmdutil.GetEnv("CHECKPOINT_DIR", "/data"),
		"Directory for the checkpoint SQLite database file")
	flag.DurationVar(&checkpointCleanupInterval, "checkpoint-cleanup-interval", defaultCheckpointCleanupInterval,
		"How often to run checkpoint cleanup")
	flag.DurationVar(&checkpointTTL, "checkpoint-ttl", defaultCheckpointTTL,
		"How long to keep checkpoint records (events older than this are cleaned up)")
	flag.DurationVar(&labelCacheTTL, "label-cache-ttl", defaultLabelCacheTTL,
		"TTL for cached labels of involved objects")
	flag.Parse()

	logger := cmdutil.SetupLogger(logLevel)

	logger.Info("starting events-collector",
		"checkpoint_dir", checkpointDir,
		"checkpoint_cleanup_interval", checkpointCleanupInterval.String(),
		"checkpoint_ttl", checkpointTTL.String(),
		"label_cache_ttl", labelCacheTTL.String(),
	)

	// Create Kubernetes clients
	k8sConfig, err := createKubeConfig(kubeconfig)
	if err != nil {
		logger.Error("failed to create Kubernetes config", "error", err)
		os.Exit(1)
	}

	clientset, err := kubernetes.NewForConfig(k8sConfig)
	if err != nil {
		logger.Error("failed to create Kubernetes clientset", "error", err)
		os.Exit(1)
	}

	dynamicClient, err := dynamic.NewForConfig(k8sConfig)
	if err != nil {
		logger.Error("failed to create dynamic Kubernetes client", "error", err)
		os.Exit(1)
	}

	if kubeconfig != "" {
		logger.Info("Kubernetes clients created successfully", "mode", "out-of-cluster", "kubeconfig", kubeconfig)
	} else {
		logger.Info("Kubernetes clients created successfully", "mode", "in-cluster")
	}

	// Initialize checkpoint store
	dbPath := filepath.Join(checkpointDir, "checkpoint.db")
	store, err := checkpoint.New(dbPath)
	if err != nil {
		logger.Error("failed to create checkpoint store", "error", err, "path", dbPath)
		os.Exit(1)
	}
	defer store.Close()
	logger.Info("checkpoint store initialized", "path", dbPath)

	// Initialize labels cache
	cache := labelcache.New(labelCacheTTL, logger)

	// Initialize label resolver
	resolver := labelresolver.New(dynamicClient, cache, logger)

	// Initialize event handler
	eventHandler := handler.New(store, resolver, logger)

	// Initialize event informer
	eventInformer := informer.New(clientset, eventHandler, logger)

	// Setup context with signal handling
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Start checkpoint cleanup manager in background
	checkpointManager := checkpoint.NewManager(store, checkpointCleanupInterval, checkpointTTL, logger)
	go checkpointManager.Start(ctx)

	// Start label cache eviction in background
	go cache.StartEviction(ctx, defaultLabelCacheEvictInterval)

	// Start event informer (blocks until context is cancelled)
	logger.Info("events-collector starting")
	if err := eventInformer.Start(ctx); err != nil && !errors.Is(err, context.Canceled) {
		fmt.Fprintf(os.Stderr, "event informer failed: %v\n", err)
		os.Exit(1)
	}

	logger.Info("events-collector shutdown completed")
}

// createKubeConfig creates a Kubernetes REST config.
// If kubeconfigPath is empty, it uses in-cluster config.
func createKubeConfig(kubeconfigPath string) (*rest.Config, error) {
	if kubeconfigPath == "" {
		config, err := rest.InClusterConfig()
		if err != nil {
			return nil, fmt.Errorf("failed to get in-cluster config: %w (use --kubeconfig for local development)", err)
		}
		return config, nil
	}

	loadingRules := &clientcmd.ClientConfigLoadingRules{ExplicitPath: kubeconfigPath}
	configOverrides := &clientcmd.ConfigOverrides{}
	kubeConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, configOverrides)

	config, err := kubeConfig.ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to load kubeconfig: %w", err)
	}
	return config, nil
}
