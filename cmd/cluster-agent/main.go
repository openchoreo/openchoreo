// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	agentclient "github.com/openchoreo/openchoreo/internal/cluster-agent"
	"github.com/openchoreo/openchoreo/internal/cmdutil"
)

const (
	defaultReconnectDelay    = 5 * time.Second
	defaultHeartbeatInterval = 30 * time.Second
	defaultRequestTimeout    = 30 * time.Second
)

func main() {
	var (
		serverURL         string
		planeType         string
		planeName         string
		clientCertPath    string
		clientKeyPath     string
		serverCAPath      string
		reconnectDelay    time.Duration
		heartbeatInterval time.Duration
		requestTimeout    time.Duration
		logLevel          string
	)

	var kubeconfig string

	flag.StringVar(&serverURL, "server-url",
		cmdutil.GetEnv("SERVER_URL", "wss://cluster-gateway:8443/ws"),
		"Cluster gateway WebSocket URL")
	flag.StringVar(&planeType, "plane-type", cmdutil.GetEnv("PLANE_TYPE", "dataplane"),
		"Plane type: dataplane or buildplane (required)")
	flag.StringVar(&planeName, "plane-name", cmdutil.GetEnv("PLANE_NAME", ""),
		"Plane name (required)")
	flag.StringVar(&clientCertPath, "client-cert",
		cmdutil.GetEnv("CLIENT_CERT_PATH", "/certs/tls.crt"),
		"Path to client certificate")
	flag.StringVar(&clientKeyPath, "client-key",
		cmdutil.GetEnv("CLIENT_KEY_PATH", "/certs/tls.key"),
		"Path to client private key")
	flag.StringVar(&serverCAPath, "server-ca",
		cmdutil.GetEnv("SERVER_CA_PATH", "/ca-certs/server-ca.crt"),
		"Path to server CA certificate for verification")
	flag.StringVar(&kubeconfig, "kubeconfig", cmdutil.GetEnv("KUBECONFIG", ""),
		"Path to kubeconfig file (for local development, defaults to in-cluster config)")
	flag.DurationVar(&reconnectDelay, "reconnect-delay", defaultReconnectDelay, "Delay between reconnection attempts")
	flag.DurationVar(&heartbeatInterval, "heartbeat-interval", defaultHeartbeatInterval, "Heartbeat message interval")
	flag.DurationVar(&requestTimeout, "request-timeout", defaultRequestTimeout, "Request timeout duration")
	flag.StringVar(&logLevel, "log-level", cmdutil.GetEnv("LOG_LEVEL", "info"), "Log level (debug, info, warn, error)")
	flag.Parse()

	if planeName == "" {
		fmt.Println("Error: plane-name is required")
		flag.Usage()
		os.Exit(1)
	}

	if planeType == "" {
		planeType = "dataplane"
	}

	// Validate planeType
	if planeType != "dataplane" && planeType != "buildplane" {
		fmt.Printf("Error: plane-type must be 'dataplane' or 'buildplane', got: %s\n", planeType)
		flag.Usage()
		os.Exit(1)
	}

	logger := cmdutil.SetupLogger(logLevel)

	logger.Info("starting OpenChoreo Cluster Agent",
		"serverURL", serverURL,
		"planeType", planeType,
		"planeName", planeName,
		"clientCert", clientCertPath,
		"clientKey", clientKeyPath,
		"serverCA", serverCAPath,
		"kubeconfig", kubeconfig,
	)

	// Create Kubernetes client (in-cluster or from kubeconfig)
	k8sClient, err := createKubernetesClient(kubeconfig)
	if err != nil {
		logger.Error("failed to create Kubernetes client", "error", err)
		os.Exit(1)
	}

	if kubeconfig != "" {
		logger.Info("Kubernetes client created successfully", "mode", "out-of-cluster", "kubeconfig", kubeconfig)
	} else {
		logger.Info("Kubernetes client created successfully", "mode", "in-cluster")
	}

	config := &agentclient.Config{
		ServerURL:         serverURL,
		PlaneType:         planeType,
		PlaneName:         planeName,
		ClientCertPath:    clientCertPath,
		ClientKeyPath:     clientKeyPath,
		ServerCAPath:      serverCAPath,
		ReconnectDelay:    reconnectDelay,
		HeartbeatInterval: heartbeatInterval,
		RequestTimeout:    requestTimeout,
	}

	agent, err := agentclient.New(config, k8sClient, logger)
	if err != nil {
		logger.Error("failed to create agent", "error", err)
		os.Exit(1)
	}

	// Setup context with signal handling
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	logger.Info("agent starting")
	if err := agent.Start(ctx); err != nil && !errors.Is(err, context.Canceled) {
		logger.Error("agent failed", "error", err)
		os.Exit(1)
	}

	logger.Info("agent shutdown completed")
}
