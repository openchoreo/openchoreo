// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"flag"
	"os"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	clustergateway "github.com/openchoreo/openchoreo/internal/cluster-gateway"
	"github.com/openchoreo/openchoreo/internal/cmdutil"
)

const (
	defaultPort              = 8443
	defaultInternalPort      = 8444
	defaultReadTimeout       = 60 * time.Second
	defaultWriteTimeout      = 60 * time.Second
	defaultIdleTimeout       = 120 * time.Second
	defaultShutdownTimeout   = 30 * time.Second
	defaultHeartbeatInterval = 30 * time.Second
	defaultHeartbeatTimeout  = 90 * time.Second
)

var (
	scheme = runtime.NewScheme()
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(openchoreov1alpha1.AddToScheme(scheme))
}

func main() {
	var (
		port                  int
		internalPort          int
		serverCertPath        string
		serverKeyPath         string
		skipClientCertVerify  bool
		readTimeout           time.Duration
		writeTimeout          time.Duration
		idleTimeout           time.Duration
		shutdownTimeout       time.Duration
		heartbeatInterval     time.Duration
		heartbeatTimeout      time.Duration
		logLevel              string
		trustClientCertHeader bool
		clientCertHeaderName  string
	)

	flag.IntVar(&port, "port", cmdutil.GetEnvInt("AGENT_SERVER_PORT", defaultPort),
		"Public server port serving the agent WebSocket endpoint (/ws)")
	flag.IntVar(&internalPort, "internal-port", cmdutil.GetEnvInt("AGENT_INTERNAL_PORT", defaultInternalPort),
		"Internal server port serving the caller-facing /api/* endpoints "+
			"(in-cluster callers only; not exposed outside the cluster)")
	flag.StringVar(&serverCertPath, "server-cert",
		cmdutil.GetEnv("SERVER_CERT_PATH", "/certs/tls.crt"),
		"Path to server certificate")
	flag.StringVar(&serverKeyPath, "server-key",
		cmdutil.GetEnv("SERVER_KEY_PATH", "/certs/tls.key"),
		"Path to server private key")
	flag.BoolVar(&skipClientCertVerify, "skip-client-cert-verify",
		cmdutil.GetEnvBool("SKIP_CLIENT_CERT_VERIFY", false),
		"Skip client certificate verification (for single-cluster setups without mTLS)")
	flag.DurationVar(&readTimeout, "read-timeout", defaultReadTimeout, "HTTP read timeout")
	flag.DurationVar(&writeTimeout, "write-timeout", defaultWriteTimeout, "HTTP write timeout")
	flag.DurationVar(&idleTimeout, "idle-timeout", defaultIdleTimeout, "HTTP idle timeout")
	flag.DurationVar(&shutdownTimeout, "shutdown-timeout", defaultShutdownTimeout, "Graceful shutdown timeout")
	flag.DurationVar(&heartbeatInterval, "heartbeat-interval", defaultHeartbeatInterval, "Heartbeat ping interval")
	flag.DurationVar(&heartbeatTimeout, "heartbeat-timeout", defaultHeartbeatTimeout, "Heartbeat timeout duration")
	flag.StringVar(&logLevel, "log-level", cmdutil.GetEnv("LOG_LEVEL", "info"), "Log level (debug, info, warn, error)")
	flag.BoolVar(&trustClientCertHeader, "trust-client-cert-header",
		cmdutil.GetEnvBool("TRUST_CLIENT_CERT_HEADER", false),
		"Fall back to a header for the agent client certificate when the TLS handshake carries none "+
			"(for deployments behind a TLS-terminating L7 load balancer). Only enable this when the public "+
			"listener is unreachable except through that load balancer.")
	flag.StringVar(&clientCertHeaderName, "client-cert-header-name",
		cmdutil.GetEnv("CLIENT_CERT_HEADER_NAME", "X-Client-Cert"),
		"HTTP header carrying the base64-encoded PEM client certificate, used when -trust-client-cert-header is set")
	flag.Parse()

	logger := cmdutil.SetupLogger(logLevel)

	logger.Info("starting OpenChoreo Cluster Gateway",
		"port", port,
		"internalPort", internalPort,
		"serverCert", serverCertPath,
		"serverKey", serverKeyPath,
		"heartbeatInterval", heartbeatInterval,
		"heartbeatTimeout", heartbeatTimeout,
		"note", "Client CA certificates are loaded dynamically from DataPlane/WorkflowPlane/ObservabilityPlane CRs",
	)

	if trustClientCertHeader {
		logger.Warn("trust-client-cert-header is enabled: the public listener will accept a client "+
			"certificate from an HTTP header when the TLS handshake carries none. Ensure the public "+
			"listener is unreachable except through the trusted load balancer that sets this header.",
			"header", clientCertHeaderName,
		)
	}

	// Create Kubernetes client for querying DataPlane/WorkflowPlane/ObservabilityPlane CRs
	k8sConfig, err := ctrl.GetConfig()
	if err != nil {
		logger.Error("failed to get Kubernetes config", "error", err)
		os.Exit(1)
	}

	k8sClient, err := client.New(k8sConfig, client.Options{Scheme: scheme})
	if err != nil {
		logger.Error("failed to create Kubernetes client", "error", err)
		os.Exit(1)
	}

	logger.Info("Kubernetes client created successfully")

	config := &clustergateway.Config{
		Port:                  port,
		InternalPort:          internalPort,
		ServerCertPath:        serverCertPath,
		ServerKeyPath:         serverKeyPath,
		SkipClientCertVerify:  skipClientCertVerify,
		ReadTimeout:           readTimeout,
		WriteTimeout:          writeTimeout,
		IdleTimeout:           idleTimeout,
		ShutdownTimeout:       shutdownTimeout,
		HeartbeatInterval:     heartbeatInterval,
		HeartbeatTimeout:      heartbeatTimeout,
		TrustClientCertHeader: trustClientCertHeader,
		ClientCertHeaderName:  clientCertHeaderName,
	}

	srv := clustergateway.New(config, k8sClient, logger)
	if err := srv.Start(); err != nil {
		logger.Error("server failed", "error", err)
		os.Exit(1)
	}
}
