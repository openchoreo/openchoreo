// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	clustergateway "github.com/openchoreo/openchoreo/internal/cluster-gateway"
	"github.com/openchoreo/openchoreo/internal/cluster-gateway/fabric"
	"github.com/openchoreo/openchoreo/internal/cmdutil"
)

const (
	defaultPort              = 8443
	defaultInternalPort      = 8444
	defaultMeshPort          = 8445
	defaultReadTimeout       = 60 * time.Second
	defaultWriteTimeout      = 60 * time.Second
	defaultIdleTimeout       = 120 * time.Second
	defaultShutdownTimeout   = 30 * time.Second
	defaultHeartbeatInterval = 30 * time.Second
	defaultHeartbeatTimeout  = 90 * time.Second
	defaultDrainWindow       = 10 * time.Second
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
		port                     int
		internalPort             int
		serverCertPath           string
		serverKeyPath            string
		skipClientCertVerify     bool
		internalMTLS             bool
		internalClientCAPath     string
		agentAuthMode            string
		agentAuthForwardedHeader string
		readTimeout              time.Duration
		writeTimeout             time.Duration
		idleTimeout              time.Duration
		shutdownTimeout          time.Duration
		heartbeatInterval        time.Duration
		heartbeatTimeout         time.Duration
		logLevel                 string
		meshEnabled              bool
		meshPort                 int
		meshCACertPath           string
		meshServiceName          string
		drainWindow              time.Duration
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
		"Deprecated: has no effect. Agent certificates are always verified per plane CR; "+
			"use --internal-mtls to control internal API verification")
	flag.BoolVar(&internalMTLS, "internal-mtls",
		cmdutil.GetEnvBool("INTERNAL_MTLS_ENABLED", true),
		"Require and verify client certificates on the internal API listener (/api/*)")
	flag.StringVar(&internalClientCAPath, "internal-client-ca-cert",
		cmdutil.GetEnv("INTERNAL_CLIENT_CA_PATH", ""),
		"Path to the CA bundle used to verify internal API clients (required when --internal-mtls is enabled)")
	flag.StringVar(&agentAuthMode, "agent-auth-mode",
		cmdutil.GetEnv("AGENT_AUTH_MODE", string(clustergateway.AgentAuthModeMTLS)),
		"How the public listener obtains the agent client certificate: "+
			"'mtls' (from the TLS handshake) or 'forwarded-header' (from a header set by a trusted TLS-terminating proxy)")
	flag.StringVar(&agentAuthForwardedHeader, "agent-auth-forwarded-header",
		cmdutil.GetEnv("AGENT_AUTH_FORWARDED_HEADER", clustergateway.DefaultForwardedHeaderName),
		"Request header carrying the URL-encoded client certificate chain when --agent-auth-mode=forwarded-header "+
			"(e.g. X-Amzn-Mtls-Clientcert for AWS ALB in mTLS passthrough mode — "+
			"ALB verify mode sends certificate-attribute headers instead, "+
			"X-Forwarded-Client-Cert for Envoy/Istio)")
	flag.DurationVar(&readTimeout, "read-timeout", defaultReadTimeout, "HTTP read timeout")
	flag.DurationVar(&writeTimeout, "write-timeout", defaultWriteTimeout, "HTTP write timeout")
	flag.DurationVar(&idleTimeout, "idle-timeout", defaultIdleTimeout, "HTTP idle timeout")
	flag.DurationVar(&shutdownTimeout, "shutdown-timeout", defaultShutdownTimeout, "Graceful shutdown timeout")
	flag.DurationVar(&heartbeatInterval, "heartbeat-interval", defaultHeartbeatInterval, "Heartbeat ping interval")
	flag.DurationVar(&heartbeatTimeout, "heartbeat-timeout", defaultHeartbeatTimeout, "Heartbeat timeout duration")
	flag.BoolVar(&meshEnabled, "mesh",
		cmdutil.GetEnvBool("MESH_ENABLED", false),
		"Enable the gateway mesh fabric: replicate the connection registry across gateway "+
			"replicas and forward requests between them (required for clusterGateway.replicas > 1)")
	flag.IntVar(&meshPort, "mesh-port", cmdutil.GetEnvInt("MESH_PORT", defaultMeshPort),
		"Gateway-to-gateway mesh listener port")
	flag.StringVar(&meshCACertPath, "mesh-ca-cert",
		cmdutil.GetEnv("MESH_CA_PATH", ""),
		"Path to the CA bundle used to verify mesh peer certificates in both directions "+
			"(the gateway serving certificate doubles as the mesh identity)")
	flag.StringVar(&meshServiceName, "mesh-service",
		cmdutil.GetEnv("MESH_SERVICE_NAME", ""),
		"Name of the headless Service whose EndpointSlices are watched for mesh peer discovery")
	flag.DurationVar(&drainWindow, "drain-window",
		cmdutil.GetEnvDuration("DRAIN_WINDOW", defaultDrainWindow),
		"Period over which GOAWAY frames are spread across agent connections during shutdown")
	flag.StringVar(&logLevel, "log-level", cmdutil.GetEnv("LOG_LEVEL", "info"), "Log level (debug, info, warn, error)")
	flag.Parse()

	logger := cmdutil.SetupLogger(logLevel)

	logger.Info("starting OpenChoreo Cluster Gateway",
		"port", port,
		"internalPort", internalPort,
		"serverCert", serverCertPath,
		"serverKey", serverKeyPath,
		"internalMTLS", internalMTLS,
		"internalClientCA", internalClientCAPath,
		"agentAuthMode", agentAuthMode,
		"heartbeatInterval", heartbeatInterval,
		"heartbeatTimeout", heartbeatTimeout,
		"note", "Client CA certificates are loaded dynamically from DataPlane/WorkflowPlane/ObservabilityPlane CRs",
	)

	if skipClientCertVerify {
		logger.Warn("--skip-client-cert-verify is deprecated and has no effect",
			"note", "agent certificates are always verified per plane CR; "+
				"use --internal-mtls=false to disable internal API verification",
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
		Port:                     port,
		InternalPort:             internalPort,
		ServerCertPath:           serverCertPath,
		ServerKeyPath:            serverKeyPath,
		SkipClientCertVerify:     skipClientCertVerify,
		InternalMTLSEnabled:      internalMTLS,
		InternalClientCAPath:     internalClientCAPath,
		AgentAuthMode:            clustergateway.AgentAuthMode(agentAuthMode),
		AgentAuthForwardedHeader: agentAuthForwardedHeader,
		ReadTimeout:              readTimeout,
		WriteTimeout:             writeTimeout,
		IdleTimeout:              idleTimeout,
		ShutdownTimeout:          shutdownTimeout,
		HeartbeatInterval:        heartbeatInterval,
		HeartbeatTimeout:         heartbeatTimeout,
		DrainWindow:              drainWindow,
	}

	srv := clustergateway.New(config, k8sClient, logger)

	// Gateway mesh fabric: replicate the connection registry across replicas
	// and forward requests between them, so the gateway can scale horizontally
	// behind an ordinary Service.
	if meshEnabled {
		podName := os.Getenv("POD_NAME")
		podIP := os.Getenv("POD_IP")
		podNamespace := os.Getenv("POD_NAMESPACE")

		switch {
		case podName == "" || podIP == "" || podNamespace == "":
			logger.Warn("mesh disabled: pod identity not available",
				"note", "set POD_NAME, POD_IP and POD_NAMESPACE (downward API) to enable the gateway mesh",
			)
		case meshServiceName == "":
			logger.Warn("mesh disabled: no mesh service configured",
				"note", "set --mesh-service to the headless mesh Service name to enable peer discovery",
			)
		default:
			clientset, err := kubernetes.NewForConfig(k8sConfig)
			if err != nil {
				logger.Error("failed to create Kubernetes clientset for mesh discovery", "error", err)
				os.Exit(1)
			}

			registry := fabric.NewRegistry(logger)
			discovery := fabric.NewEndpointSliceDiscovery(
				clientset, podNamespace, meshServiceName, meshPort, podName, logger)
			mesh := fabric.NewMesh(fabric.MeshConfig{
				Self:       fabric.Peer{ID: podName, Addr: fmt.Sprintf("%s:%d", podIP, meshPort)},
				ListenPort: meshPort,
				CertFile:   serverCertPath,
				KeyFile:    serverKeyPath,
				CAFile:     meshCACertPath,
				ServerName: fmt.Sprintf("%s.%s.svc", meshServiceName, podNamespace),
			}, registry, discovery, srv, logger)

			srv.SetFabric(mesh, registry)
			logger.Info("gateway mesh fabric enabled",
				"pod", podName,
				"meshPort", meshPort,
				"meshService", meshServiceName,
				"meshCA", meshCACertPath,
			)
		}
	}

	if err := srv.Start(); err != nil {
		logger.Error("server failed", "error", err)
		os.Exit(1)
	}
}
