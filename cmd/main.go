// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"crypto/tls"
	"flag"
	"fmt"
	"os"
	"strconv"

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
	"github.com/openchoreo/openchoreo/internal/controller/clustercomponenttype"
	"github.com/openchoreo/openchoreo/internal/controller/clusterdataplane"
	"github.com/openchoreo/openchoreo/internal/controller/clusterobservabilityplane"
	"github.com/openchoreo/openchoreo/internal/controller/clusterresourcetype"
	"github.com/openchoreo/openchoreo/internal/controller/clustertrait"
	"github.com/openchoreo/openchoreo/internal/controller/clusterworkflow"
	"github.com/openchoreo/openchoreo/internal/controller/clusterworkflowplane"
	"github.com/openchoreo/openchoreo/internal/controller/component"
	"github.com/openchoreo/openchoreo/internal/controller/componentrelease"
	"github.com/openchoreo/openchoreo/internal/controller/componenttype"
	"github.com/openchoreo/openchoreo/internal/controller/dataplane"
	"github.com/openchoreo/openchoreo/internal/controller/deploymentpipeline"
	"github.com/openchoreo/openchoreo/internal/controller/environment"
	"github.com/openchoreo/openchoreo/internal/controller/observabilityalertrule"
	"github.com/openchoreo/openchoreo/internal/controller/observabilityalertsnotificationchannel"
	"github.com/openchoreo/openchoreo/internal/controller/observabilityplane"
	"github.com/openchoreo/openchoreo/internal/controller/project"
	"github.com/openchoreo/openchoreo/internal/controller/releasebinding"
	"github.com/openchoreo/openchoreo/internal/controller/renderedrelease"
	"github.com/openchoreo/openchoreo/internal/controller/resource"
	"github.com/openchoreo/openchoreo/internal/controller/resourcerelease"
	"github.com/openchoreo/openchoreo/internal/controller/resourcereleasebinding"
	"github.com/openchoreo/openchoreo/internal/controller/resourcetype"
	"github.com/openchoreo/openchoreo/internal/controller/secretreference"
	"github.com/openchoreo/openchoreo/internal/controller/trait"
	"github.com/openchoreo/openchoreo/internal/controller/workflow"
	"github.com/openchoreo/openchoreo/internal/controller/workflowplane"
	"github.com/openchoreo/openchoreo/internal/controller/workflowrun"
	"github.com/openchoreo/openchoreo/internal/controller/workload"
	argo "github.com/openchoreo/openchoreo/internal/dataplane/kubernetes/types/argoproj.io/workflow/v1alpha1"
	ciliumv2 "github.com/openchoreo/openchoreo/internal/dataplane/kubernetes/types/cilium.io/v2"
	esv1 "github.com/openchoreo/openchoreo/internal/dataplane/kubernetes/types/externalsecrets/v1"
	csisecretv1 "github.com/openchoreo/openchoreo/internal/dataplane/kubernetes/types/secretstorecsi/v1"
	componentpipeline "github.com/openchoreo/openchoreo/internal/pipeline/component"
	workflowpipeline "github.com/openchoreo/openchoreo/internal/pipeline/workflow"
	"github.com/openchoreo/openchoreo/internal/version"
	authzrolebindingwebhook "github.com/openchoreo/openchoreo/internal/webhook/authzrolebinding"
	clusterauthzrolebindingwebhook "github.com/openchoreo/openchoreo/internal/webhook/clusterauthzrolebinding"
	clustercomponenttypewebhook "github.com/openchoreo/openchoreo/internal/webhook/clustercomponenttype"
	clusterresourcetypewebhook "github.com/openchoreo/openchoreo/internal/webhook/clusterresourcetype"
	clustertraitwebhook "github.com/openchoreo/openchoreo/internal/webhook/clustertrait"
	clusterworkflowwebhook "github.com/openchoreo/openchoreo/internal/webhook/clusterworkflow"
	componentwebhook "github.com/openchoreo/openchoreo/internal/webhook/component"
	componentreleasewebhook "github.com/openchoreo/openchoreo/internal/webhook/componentrelease"
	componenttypewebhook "github.com/openchoreo/openchoreo/internal/webhook/componenttype"
	projectwebhook "github.com/openchoreo/openchoreo/internal/webhook/project"
	releasebindingwebhook "github.com/openchoreo/openchoreo/internal/webhook/releasebinding"
	resourcereleasewebhook "github.com/openchoreo/openchoreo/internal/webhook/resourcerelease"
	resourcetypewebhook "github.com/openchoreo/openchoreo/internal/webhook/resourcetype"
	traitwebhook "github.com/openchoreo/openchoreo/internal/webhook/trait"
	workflowwebhook "github.com/openchoreo/openchoreo/internal/webhook/workflow"
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

// controllerSetup is satisfied by any reconciler that wires itself to a manager.
// Lets setup* helpers register controllers from a slice with one error path.
type controllerSetup interface {
	SetupWithManager(mgr ctrl.Manager) error
}

// setupControlPlaneControllers sets up all control plane controllers with the manager
func setupControlPlaneControllers(
	mgr ctrl.Manager,
	k8sClientMgr *kubernetesClient.KubeMultiClientManager,
	clusterGatewayURL string,
	gwTLS gatewayClient.TLSConfig,
) error {
	// Create gateway client for plane lifecycle notifications
	var gwClient *gatewayClient.Client
	if clusterGatewayURL != "" {
		var err error
		gwClient, err = gatewayClient.NewClientWithConfig(&gatewayClient.Config{
			BaseURL: clusterGatewayURL,
			TLS:     gwTLS,
		})
		if err != nil {
			return fmt.Errorf("failed to create cluster gateway client: %w", err)
		}
		setupLog.Info("gateway client initialized",
			"url", clusterGatewayURL,
			"caCert", gwTLS.CAFile != "",
			"clientCert", gwTLS.ClientCertFile != "",
			"insecure", gwTLS.InsecureSkipVerify,
		)
	}

	// Create plane client provider for controllers that need to talk to remote planes.
	// This wraps KubeMultiClientManager + gatewayURL behind an interface, keeping
	// infrastructure concerns out of controller code.
	planeClientProvider := kubernetesClient.NewPlaneClientProvider(k8sClientMgr, clusterGatewayURL)

	// Setup shared field indexes before controllers are initialized.
	if err := controller.SetupSharedIndexes(context.Background(), mgr); err != nil {
		return fmt.Errorf("failed to setup shared indexes: %w", err)
	}

	c, s := mgr.GetClient(), mgr.GetScheme()

	reconcilers := []controllerSetup{
		&deploymentpipeline.Reconciler{Client: c, Scheme: s},
		&workload.Reconciler{Client: c, Scheme: s},
		&environment.Reconciler{Client: c, PlaneClientProvider: planeClientProvider, Scheme: s},
		&dataplane.Reconciler{
			Client:        c,
			Scheme:        s,
			ClientMgr:     k8sClientMgr,
			GatewayClient: gwClient,
			CacheVersion:  "v2",
		},
		&clusterdataplane.Reconciler{
			Client:        c,
			Scheme:        s,
			ClientMgr:     k8sClientMgr,
			GatewayClient: gwClient,
			CacheVersion:  "v2",
		},
		&clusterworkflowplane.Reconciler{
			Client:        c,
			Scheme:        s,
			ClientMgr:     k8sClientMgr,
			GatewayClient: gwClient,
			CacheVersion:  "v2",
		},
		&project.Reconciler{Client: c, Scheme: s},
		&component.Reconciler{Client: c, Scheme: s},
		&componenttype.Reconciler{Client: c, Scheme: s},
		&clustercomponenttype.Reconciler{Client: c, Scheme: s},
		&trait.Reconciler{Client: c, Scheme: s},
		&clustertrait.Reconciler{Client: c, Scheme: s},
		&componentrelease.Reconciler{Client: c, Scheme: s},
		// Resource family — templates (cluster-scoped before namespaced),
		// then consumer, immutable release snapshot, per-env binding.
		&clusterresourcetype.Reconciler{Client: c, Scheme: s},
		&resourcetype.Reconciler{Client: c, Scheme: s},
		&resource.Reconciler{Client: c, Scheme: s},
		&resourcerelease.Reconciler{Client: c, Scheme: s},
		&resourcereleasebinding.Reconciler{Client: c, Scheme: s},
		&releasebinding.Reconciler{Client: c, Scheme: s, Pipeline: componentpipeline.NewPipeline()},
		&renderedrelease.Reconciler{Client: c, PlaneClientProvider: planeClientProvider, Scheme: s},
		&workflow.Reconciler{Client: c, Scheme: s},
		&clusterworkflow.Reconciler{Client: c, Scheme: s},
		&workflowrun.Reconciler{
			Client:              c,
			Scheme:              s,
			PlaneClientProvider: planeClientProvider,
			Pipeline:            workflowpipeline.NewPipeline(),
		},
		&workflowplane.Reconciler{
			Client:        c,
			Scheme:        s,
			ClientMgr:     k8sClientMgr,
			GatewayClient: gwClient,
			CacheVersion:  "v2",
		},
		&secretreference.Reconciler{Client: c, Scheme: s},
		&observabilityplane.Reconciler{
			Client:        c,
			Scheme:        s,
			ClientMgr:     k8sClientMgr,
			GatewayClient: gwClient,
			CacheVersion:  "v2",
		},
		&clusterobservabilityplane.Reconciler{
			Client:        c,
			Scheme:        s,
			ClientMgr:     k8sClientMgr,
			GatewayClient: gwClient,
			CacheVersion:  "v2",
		},
		&observabilityalertsnotificationchannel.Reconciler{Client: c, PlaneClientProvider: planeClientProvider, Scheme: s},
	}

	for _, r := range reconcilers {
		if err := r.SetupWithManager(mgr); err != nil {
			return err
		}
	}

	return nil
}

// setupObservabilityPlaneControllers sets up all observability plane controllers with the manager
func setupObservabilityPlaneControllers(mgr ctrl.Manager) error {
	c, s := mgr.GetClient(), mgr.GetScheme()

	reconcilers := []controllerSetup{
		&observabilityalertrule.Reconciler{Client: c, Scheme: s},
	}

	for _, r := range reconcilers {
		if err := r.SetupWithManager(mgr); err != nil {
			return err
		}
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
	var clusterGatewayURL string
	var clusterGatewayCACert string
	var clusterGatewayClientCert string
	var clusterGatewayClientKey string
	var clusterGatewayInsecure bool
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
	flag.BoolVar(&clusterGatewayInsecure, "cluster-gateway-insecure",
		getEnvBool("CLUSTER_GATEWAY_INSECURE", false),
		"Skip TLS verification when calling the cluster gateway. "+
			"For local development only. Do not enable in production.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	flag.BoolVar(&secureMetrics, "metrics-secure", true,
		"If set, the metrics endpoint is served securely via HTTPS. Use --metrics-secure=false to use HTTP instead.")
	flag.BoolVar(&enableHTTP2, "enable-http2", false,
		"If set, HTTP/2 will be enabled for the metrics and webhook servers")
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
	// The k8sClientMgr manages cached Kubernetes clients for accessing data planes and workflow planes.
	// It supports both direct access mode and agent mode (via HTTP proxy through cluster gateway).
	k8sClientMgr := kubernetesClient.NewManagerWithProxyTLS(&kubernetesClient.ProxyTLSConfig{
		CACertPath:     clusterGatewayCACert,
		ClientCertPath: clusterGatewayClientCert,
		ClientKeyPath:  clusterGatewayClientKey,
		Insecure:       clusterGatewayInsecure,
	})
	setupLog.Info("Kubernetes client manager created with proxy TLS configuration",
		"caCert", clusterGatewayCACert != "",
		"clientCert", clusterGatewayClientCert != "",
		"clientKey", clusterGatewayClientKey != "",
		"insecure", clusterGatewayInsecure)
	if clusterGatewayURL != "" && clusterGatewayInsecure {
		setupLog.Info("WARNING: Cluster gateway TLS verification is disabled (--cluster-gateway-insecure). " +
			"Do not use this setting in production.")
	}

	// -----------------------------------------------------------------------------
	// Setup controllers with the controller manager
	// -----------------------------------------------------------------------------

	switch deploymentPlane {
	// Control plane controllers
	case deploymentPlaneControlPlane:
		err = setupControlPlaneControllers(mgr, k8sClientMgr, clusterGatewayURL, gatewayClient.TLSConfig{
			CAFile:             clusterGatewayCACert,
			ClientCertFile:     clusterGatewayClientCert,
			ClientKeyFile:      clusterGatewayClientKey,
			InsecureSkipVerify: clusterGatewayInsecure,
		})
		if err != nil {
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

	if os.Getenv("ENABLE_WEBHOOKS") != "false" {
		webhookSetups := []struct {
			name  string
			setup func(ctrl.Manager) error
		}{
			{"Project", projectwebhook.SetupProjectWebhookWithManager},
			{"ComponentType", componenttypewebhook.SetupComponentTypeWebhookWithManager},
			{"ClusterComponentType", clustercomponenttypewebhook.SetupClusterComponentTypeWebhookWithManager},
			{"Component", componentwebhook.SetupComponentWebhookWithManager},
			{"Trait", traitwebhook.SetupTraitWebhookWithManager},
			{"ClusterTrait", clustertraitwebhook.SetupClusterTraitWebhookWithManager},
			{"ComponentRelease", componentreleasewebhook.SetupComponentReleaseWebhookWithManager},
			{"ReleaseBinding", releasebindingwebhook.SetupReleaseBindingWebhookWithManager},
			{"ResourceType", resourcetypewebhook.SetupResourceTypeWebhookWithManager},
			{"ClusterResourceType", clusterresourcetypewebhook.SetupClusterResourceTypeWebhookWithManager},
			{"ResourceRelease", resourcereleasewebhook.SetupResourceReleaseWebhookWithManager},
			{"Workflow", workflowwebhook.SetupWorkflowWebhookWithManager},
			{"ClusterWorkflow", clusterworkflowwebhook.SetupClusterWorkflowWebhookWithManager},
			{"AuthzRoleBinding", authzrolebindingwebhook.SetupAuthzRoleBindingWebhookWithManager},
			{"ClusterAuthzRoleBinding", clusterauthzrolebindingwebhook.SetupClusterAuthzRoleBindingWebhookWithManager},
		}
		for _, w := range webhookSetups {
			if err := w.setup(mgr); err != nil {
				setupLog.Error(err, "unable to create webhook", "webhook", w.name)
				os.Exit(1)
			}
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

// getEnvBool retrieves a boolean environment variable, returning a default if
// unset or unparseable.
func getEnvBool(key string, defaultValue bool) bool {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return defaultValue
	}
	return parsed
}
