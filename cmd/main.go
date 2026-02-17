// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"crypto/tls"
	"flag"
	"fmt"
	"os"

	// +kubebuilder:scaffold:imports
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/metrics/filters"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	gatewayClient "github.com/openchoreo/openchoreo/internal/clients/gateway"
	kubernetesClient "github.com/openchoreo/openchoreo/internal/clients/kubernetes"
	"github.com/openchoreo/openchoreo/internal/controller"
	"github.com/openchoreo/openchoreo/internal/controller/build"
	"github.com/openchoreo/openchoreo/internal/controller/buildplane"
	"github.com/openchoreo/openchoreo/internal/controller/clusterbuildplane"
	"github.com/openchoreo/openchoreo/internal/controller/clustercomponenttype"
	"github.com/openchoreo/openchoreo/internal/controller/clusterdataplane"
	"github.com/openchoreo/openchoreo/internal/controller/clusterobservabilityplane"
	"github.com/openchoreo/openchoreo/internal/controller/clustertrait"
	"github.com/openchoreo/openchoreo/internal/controller/component"
	"github.com/openchoreo/openchoreo/internal/controller/componentrelease"
	"github.com/openchoreo/openchoreo/internal/controller/componenttype"
	"github.com/openchoreo/openchoreo/internal/controller/componentworkflowrun"
	"github.com/openchoreo/openchoreo/internal/controller/dataplane"
	"github.com/openchoreo/openchoreo/internal/controller/deploymentpipeline"
	"github.com/openchoreo/openchoreo/internal/controller/deploymenttrack"
	"github.com/openchoreo/openchoreo/internal/controller/environment"
	"github.com/openchoreo/openchoreo/internal/controller/gitcommitrequest"
	"github.com/openchoreo/openchoreo/internal/controller/observabilityalertrule"
	"github.com/openchoreo/openchoreo/internal/controller/observabilityalertsnotificationchannel"
	"github.com/openchoreo/openchoreo/internal/controller/observabilityplane"
	"github.com/openchoreo/openchoreo/internal/controller/project"
	"github.com/openchoreo/openchoreo/internal/controller/release"
	"github.com/openchoreo/openchoreo/internal/controller/releasebinding"
	"github.com/openchoreo/openchoreo/internal/controller/secretreference"
	"github.com/openchoreo/openchoreo/internal/controller/trait"
	"github.com/openchoreo/openchoreo/internal/controller/workflow"
	"github.com/openchoreo/openchoreo/internal/controller/workflowrun"
	"github.com/openchoreo/openchoreo/internal/controller/workload"
	argo "github.com/openchoreo/openchoreo/internal/dataplane/kubernetes/types/argoproj.io/workflow/v1alpha1"
	ciliumv2 "github.com/openchoreo/openchoreo/internal/dataplane/kubernetes/types/cilium.io/v2"
	esv1 "github.com/openchoreo/openchoreo/internal/dataplane/kubernetes/types/externalsecrets/v1"
	csisecretv1 "github.com/openchoreo/openchoreo/internal/dataplane/kubernetes/types/secretstorecsi/v1"
	componentpipeline "github.com/openchoreo/openchoreo/internal/pipeline/component"
	componentworkflowpipeline "github.com/openchoreo/openchoreo/internal/pipeline/componentworkflow"
	workflowpipeline "github.com/openchoreo/openchoreo/internal/pipeline/workflow"
	"github.com/openchoreo/openchoreo/internal/version"
	clustercomponenttypewebhook "github.com/openchoreo/openchoreo/internal/webhook/clustercomponenttype"
	clustertraitwebhook "github.com/openchoreo/openchoreo/internal/webhook/clustertrait"
	componentwebhook "github.com/openchoreo/openchoreo/internal/webhook/component"
	componentreleasewebhook "github.com/openchoreo/openchoreo/internal/webhook/componentrelease"
	componenttypewebhook "github.com/openchoreo/openchoreo/internal/webhook/componenttype"
	projectwebhook "github.com/openchoreo/openchoreo/internal/webhook/project"
	releasebindingwebhook "github.com/openchoreo/openchoreo/internal/webhook/releasebinding"
	traitwebhook "github.com/openchoreo/openchoreo/internal/webhook/trait"
)

const (
	deploymentPlaneControlPlane       = "controlplane"
	deploymentPlaneObservabilityPlane = "observabilityplane"
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	utilruntime.Must(ciliumv2.AddToScheme(scheme))
	utilruntime.Must(openchoreov1alpha1.AddToScheme(scheme))
	utilruntime.Must(argo.AddToScheme(scheme))
	utilruntime.Must(csisecretv1.Install(scheme))
	utilruntime.Must(esv1.AddToScheme(scheme))
	// +kubebuilder:scaffold:scheme
}

// setupControlPlaneControllers sets up all control plane controllers with the manager
func setupControlPlaneControllers(
	mgr ctrl.Manager,
	k8sClientMgr *kubernetesClient.KubeMultiClientManager,
	clusterGatewayURL string,
	enableLegacyCRDs bool,
) error {
	// Create gateway client for plane lifecycle notifications
	var gwClient *gatewayClient.Client
	if clusterGatewayURL != "" {
		gwClient = gatewayClient.NewClient(clusterGatewayURL)
		setupLog.Info("gateway client initialized", "url", clusterGatewayURL)
	}

	// Setup shared field indexes before controllers are initialized.
	if err := controller.SetupSharedIndexes(context.Background(), mgr); err != nil {
		return fmt.Errorf("failed to setup shared indexes: %w", err)
	}

	if enableLegacyCRDs {
		if err := (&environment.Reconciler{
			Client:       mgr.GetClient(),
			K8sClientMgr: k8sClientMgr,
			Scheme:       mgr.GetScheme(),
			GatewayURL:   clusterGatewayURL,
		}).SetupWithManager(mgr); err != nil {
			return err
		}
		if err := (&deploymentpipeline.Reconciler{
			Client: mgr.GetClient(),
			Scheme: mgr.GetScheme(),
		}).SetupWithManager(mgr); err != nil {
			return err
		}
		if err := (&deploymenttrack.Reconciler{
			Client: mgr.GetClient(),
			Scheme: mgr.GetScheme(),
		}).SetupWithManager(mgr); err != nil {
			return err
		}
		if err := (&workload.Reconciler{
			Client: mgr.GetClient(),
			Scheme: mgr.GetScheme(),
		}).SetupWithManager(mgr); err != nil {
			return err
		}
	}

	if err := (&dataplane.Reconciler{
		Client:        mgr.GetClient(),
		Scheme:        mgr.GetScheme(),
		ClientMgr:     k8sClientMgr,
		GatewayClient: gwClient,
		CacheVersion:  "v2",
	}).SetupWithManager(mgr); err != nil {
		return err
	}

	if err := (&clusterdataplane.Reconciler{
		Client:        mgr.GetClient(),
		Scheme:        mgr.GetScheme(),
		ClientMgr:     k8sClientMgr,
		GatewayClient: gwClient,
		CacheVersion:  "v2",
	}).SetupWithManager(mgr); err != nil {
		return err
	}

	if err := (&clusterbuildplane.Reconciler{
		Client:        mgr.GetClient(),
		Scheme:        mgr.GetScheme(),
		ClientMgr:     k8sClientMgr,
		GatewayClient: gwClient,
		CacheVersion:  "v2",
	}).SetupWithManager(mgr); err != nil {
		return err
	}

	if err := (&project.Reconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		return err
	}

	if err := (&component.Reconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		return err
	}

	if err := (&componenttype.Reconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		return err
	}

	if err := (&clustercomponenttype.Reconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		return err
	}

	if err := (&trait.Reconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		return err
	}

	if err := (&clustertrait.Reconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		return err
	}

	if err := (&componentrelease.Reconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		return err
	}

	if err := (&releasebinding.Reconciler{
		Client:   mgr.GetClient(),
		Scheme:   mgr.GetScheme(),
		Pipeline: componentpipeline.NewPipeline(),
	}).SetupWithManager(mgr); err != nil {
		return err
	}

	if err := (&gitcommitrequest.Reconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		return err
	}

	if err := (&release.Reconciler{
		Client:       mgr.GetClient(),
		K8sClientMgr: k8sClientMgr,
		Scheme:       mgr.GetScheme(),
		GatewayURL:   clusterGatewayURL,
	}).SetupWithManager(mgr); err != nil {
		return err
	}

	if err := (&workflow.Reconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		return err
	}

	if err := (&workflowrun.Reconciler{
		Client:       mgr.GetClient(),
		K8sClientMgr: k8sClientMgr,
		Scheme:       mgr.GetScheme(),
		GatewayURL:   clusterGatewayURL,
		Pipeline:     workflowpipeline.NewPipeline(),
	}).SetupWithManager(mgr); err != nil {
		return err
	}

	if err := (&build.Reconciler{
		Client:       mgr.GetClient(),
		K8sClientMgr: k8sClientMgr,
		Scheme:       mgr.GetScheme(),
		GatewayURL:   clusterGatewayURL,
	}).SetupWithManager(mgr); err != nil {
		return err
	}

	if err := (&buildplane.Reconciler{
		Client:        mgr.GetClient(),
		Scheme:        mgr.GetScheme(),
		ClientMgr:     k8sClientMgr,
		GatewayClient: gwClient,
		CacheVersion:  "v2",
	}).SetupWithManager(mgr); err != nil {
		return err
	}

	if err := (&secretreference.Reconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		return err
	}

	if err := (&componentworkflowrun.Reconciler{
		Client:       mgr.GetClient(),
		K8sClientMgr: k8sClientMgr,
		Scheme:       mgr.GetScheme(),
		Pipeline:     componentworkflowpipeline.NewPipeline(),
		GatewayURL:   clusterGatewayURL,
	}).SetupWithManager(mgr); err != nil {
		return err
	}

	if err := (&observabilityplane.Reconciler{
		Client:        mgr.GetClient(),
		Scheme:        mgr.GetScheme(),
		ClientMgr:     k8sClientMgr,
		GatewayClient: gwClient,
		CacheVersion:  "v2",
	}).SetupWithManager(mgr); err != nil {
		return err
	}

	if err := (&clusterobservabilityplane.Reconciler{
		Client:        mgr.GetClient(),
		Scheme:        mgr.GetScheme(),
		ClientMgr:     k8sClientMgr,
		GatewayClient: gwClient,
		CacheVersion:  "v2",
	}).SetupWithManager(mgr); err != nil {
		return err
	}

	if err := (&observabilityalertsnotificationchannel.Reconciler{
		Client:       mgr.GetClient(),
		K8sClientMgr: k8sClientMgr,
		Scheme:       mgr.GetScheme(),
		GatewayURL:   clusterGatewayURL,
	}).SetupWithManager(mgr); err != nil {
		return err
	}

	return nil
}

// setupObservabilityPlaneControllers sets up all observability plane controllers with the manager
func setupObservabilityPlaneControllers(mgr ctrl.Manager) error {
	if err := (&observabilityalertrule.Reconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		return err
	}

	return nil
}

// nolint:gocyclo
func main() {
	var metricsAddr string
	var enableLeaderElection bool
	var probeAddr string
	var secureMetrics bool
	var enableHTTP2 bool
	var enableLegacyCRDs bool
	var clusterGatewayURL string
	var clusterGatewayCACert string
	var clusterGatewayClientCert string
	var clusterGatewayClientKey string
	var deploymentPlane string
	var tlsOpts []func(*tls.Config)
	flag.StringVar(&metricsAddr, "metrics-bind-address", "0", "The address the metrics endpoint binds to. "+
		"Use :8443 for HTTPS or :8080 for HTTP, or leave as 0 to disable the metrics service.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.StringVar(&clusterGatewayURL, "cluster-gateway-url",
		getEnv("CLUSTER_GATEWAY_URL", "https://cluster-gateway.openchoreo-control-plane.svc.cluster.local:8443"),
		"The URL of the cluster gateway for HTTP proxy communication with data planes. "+
			"Required for agent mode. Example: https://localhost:8443")
	flag.StringVar(&clusterGatewayCACert, "cluster-gateway-ca-cert", getEnv("CLUSTER_GATEWAY_CA_CERT", ""),
		"Path to CA certificate for verifying the cluster gateway's TLS certificate. "+
			"If not specified, InsecureSkipVerify will be used (not recommended for production).")
	flag.StringVar(&clusterGatewayClientCert, "cluster-gateway-client-cert", getEnv("CLUSTER_GATEWAY_CLIENT_CERT", ""),
		"Path to client certificate for mTLS authentication with the cluster gateway.")
	flag.StringVar(&clusterGatewayClientKey, "cluster-gateway-client-key", getEnv("CLUSTER_GATEWAY_CLIENT_KEY", ""),
		"Path to client private key for mTLS authentication with the cluster gateway.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	flag.BoolVar(&secureMetrics, "metrics-secure", true,
		"If set, the metrics endpoint is served securely via HTTPS. Use --metrics-secure=false to use HTTP instead.")
	flag.BoolVar(&enableHTTP2, "enable-http2", false,
		"If set, HTTP/2 will be enabled for the metrics and webhook servers")
	flag.BoolVar(&enableLegacyCRDs, "enable-legacy-crds", false, // TODO <-- remove me
		"If set, legacy CRDs will be enabled. This is only for the POC and will be removed in the future.")
	flag.StringVar(&deploymentPlane, "deployment-plane", deploymentPlaneControlPlane,
		"The deployment plane this manager should serve. Supported values: controlplane, observabilityplane")
	opts := zap.Options{
		Development: true,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	if deploymentPlane != deploymentPlaneControlPlane && deploymentPlane != deploymentPlaneObservabilityPlane {
		setupLog.Error(nil, "invalid deployment plane", "deploymentPlane", deploymentPlane)
		os.Exit(1)
	}

	setupLog.Info("starting controller manager", append(version.GetLogKeyValues(), "deploymentPlane", deploymentPlane)...)

	// if the enable-http2 flag is false (the default), http/2 should be disabled
	// due to its vulnerabilities. More specifically, disabling http/2 will
	// prevent from being vulnerable to the HTTP/2 Stream Cancellation and
	// Rapid Reset CVEs. For more information see:
	// - https://github.com/advisories/GHSA-qppj-fm5r-hxr3
	// - https://github.com/advisories/GHSA-4374-p667-p6c8
	disableHTTP2 := func(c *tls.Config) {
		setupLog.Info("disabling http/2")
		c.NextProtos = []string{"http/1.1"}
	}

	if !enableHTTP2 {
		tlsOpts = append(tlsOpts, disableHTTP2)
	}

	webhookServer := webhook.NewServer(webhook.Options{
		TLSOpts: tlsOpts,
	})

	// Metrics endpoint is enabled in 'config/default/kustomization.yaml'. The Metrics options configure the server.
	// More info:
	// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.19.1/pkg/metrics/server
	// - https://book.kubebuilder.io/reference/metrics.html
	metricsServerOptions := metricsserver.Options{
		BindAddress:   metricsAddr,
		SecureServing: secureMetrics,
		TLSOpts:       tlsOpts,
	}

	if secureMetrics {
		// FilterProvider is used to protect the metrics endpoint with authn/authz.
		// These configurations ensure that only authorized users and service accounts
		// can access the metrics endpoint. The RBAC are configured in 'config/rbac/kustomization.yaml'. More info:
		// https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.19.1/pkg/metrics/filters#WithAuthenticationAndAuthorization
		metricsServerOptions.FilterProvider = filters.WithAuthenticationAndAuthorization

		// TODO(user): If CertDir, CertName, and KeyName are not specified, controller-runtime will automatically
		// generate self-signed certificates for the metrics server. While convenient for development and testing,
		// this setup is not recommended for production.
	}

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                 scheme,
		Metrics:                metricsServerOptions,
		WebhookServer:          webhookServer,
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "43500532.openchoreo.dev",
		// LeaderElectionReleaseOnCancel defines if the leader should step down voluntarily
		// when the Manager ends. This requires the binary to immediately end when the
		// Manager is stopped, otherwise, this setting is unsafe. Setting this significantly
		// speeds up voluntary leader transitions as the new leader don't have to wait
		// LeaseDuration time first.
		//
		// In the default scaffold provided, the program ends immediately after
		// the manager stops, so would be fine to enable this option. However,
		// if you are doing or is intended to do any operation such as perform cleanups
		// after the manager stops then its usage might be unsafe.
		// LeaderElectionReleaseOnCancel: true,
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	// -----------------------------------------------------------------------------
	// Setup Kubernetes multi-client manager
	// -----------------------------------------------------------------------------
	// The k8sClientMgr manages cached Kubernetes clients for accessing data planes and build planes.
	// It supports both direct access mode and agent mode (via HTTP proxy through cluster gateway).
	var k8sClientMgr *kubernetesClient.KubeMultiClientManager
	if clusterGatewayCACert != "" || clusterGatewayClientCert != "" || clusterGatewayClientKey != "" {
		// Create client manager with TLS configuration for HTTP proxy
		k8sClientMgr = kubernetesClient.NewManagerWithProxyTLS(&kubernetesClient.ProxyTLSConfig{
			CACertPath:     clusterGatewayCACert,
			ClientCertPath: clusterGatewayClientCert,
			ClientKeyPath:  clusterGatewayClientKey,
		})
		setupLog.Info("Kubernetes client manager created with proxy TLS configuration",
			"caCert", clusterGatewayCACert != "",
			"clientCert", clusterGatewayClientCert != "",
			"clientKey", clusterGatewayClientKey != "")
	} else {
		// Create client manager without TLS configuration (insecure mode)
		k8sClientMgr = kubernetesClient.NewManager()
		if clusterGatewayURL != "" {
			setupLog.Info("WARNING: Using insecure mode for cluster gateway connection. " +
				"Please provide TLS certificates for production use.")
		}
	}

	// -----------------------------------------------------------------------------
	// Setup controllers with the controller manager
	// -----------------------------------------------------------------------------

	switch deploymentPlane {
	// Control plane controllers
	case deploymentPlaneControlPlane:
		if err = setupControlPlaneControllers(mgr, k8sClientMgr, clusterGatewayURL, enableLegacyCRDs); err != nil {
			setupLog.Error(err, "unable to setup control plane controllers")
			os.Exit(1)
		}

	// Observability plane controllers
	case deploymentPlaneObservabilityPlane:
		if err = setupObservabilityPlaneControllers(mgr); err != nil {
			setupLog.Error(err, "unable to setup observability plane controllers")
			os.Exit(1)
		}
	default:
		setupLog.Error(nil, "invalid deployment plane", "deploymentPlane", deploymentPlane)
		os.Exit(1)
	}

	// +kubebuilder:scaffold:builder

	// -----------------------------------------------------------------------------
	// Setup webhooks with the controller manager
	// -----------------------------------------------------------------------------

	// nolint:goconst
	if os.Getenv("ENABLE_WEBHOOKS") != "false" {
		if err = projectwebhook.SetupProjectWebhookWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create webhook", "webhook", "Project")
			os.Exit(1)
		}
	}

	// nolint:goconst
	if os.Getenv("ENABLE_WEBHOOKS") != "false" {
		if err := componenttypewebhook.SetupComponentTypeWebhookWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create webhook", "webhook", "ComponentType")
			os.Exit(1)
		}
	}
	// nolint:goconst
	if os.Getenv("ENABLE_WEBHOOKS") != "false" {
		if err := clustercomponenttypewebhook.SetupClusterComponentTypeWebhookWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create webhook", "webhook", "ClusterComponentType")
			os.Exit(1)
		}
	}
	// nolint:goconst
	if os.Getenv("ENABLE_WEBHOOKS") != "false" {
		if err := componentwebhook.SetupComponentWebhookWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create webhook", "webhook", "Component")
			os.Exit(1)
		}
	}
	// nolint:goconst
	if os.Getenv("ENABLE_WEBHOOKS") != "false" {
		if err := traitwebhook.SetupTraitWebhookWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create webhook", "webhook", "Trait")
			os.Exit(1)
		}
	}
	// nolint:goconst
	if os.Getenv("ENABLE_WEBHOOKS") != "false" {
		if err := clustertraitwebhook.SetupClusterTraitWebhookWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create webhook", "webhook", "ClusterTrait")
			os.Exit(1)
		}
	}
	// nolint:goconst
	if os.Getenv("ENABLE_WEBHOOKS") != "false" {
		if err := componentreleasewebhook.SetupComponentReleaseWebhookWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create webhook", "webhook", "ComponentRelease")
			os.Exit(1)
		}
	}
	// nolint:goconst
	if os.Getenv("ENABLE_WEBHOOKS") != "false" {
		if err := releasebindingwebhook.SetupReleaseBindingWebhookWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create webhook", "webhook", "ReleaseBinding")
			os.Exit(1)
		}
	}

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}

// getEnv retrieves an environment variable value, returning a default if not set
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
