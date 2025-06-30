// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"crypto/tls"
	"flag"
	"os"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	// +kubebuilder:scaffold:imports
	egv1a1 "github.com/envoyproxy/gateway/api/v1alpha1"
	"github.com/google/go-github/v69/github"
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
	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	choreov1 "github.com/openchoreo/openchoreo/api/v1"
	corev1 "github.com/openchoreo/openchoreo/api/v1"
	"github.com/openchoreo/openchoreo/internal/controller/api"
	"github.com/openchoreo/openchoreo/internal/controller/apibinding"
	"github.com/openchoreo/openchoreo/internal/controller/apiclass"
	"github.com/openchoreo/openchoreo/internal/controller/apirelease"
	"github.com/openchoreo/openchoreo/internal/controller/build"
	"github.com/openchoreo/openchoreo/internal/controller/component"
	"github.com/openchoreo/openchoreo/internal/controller/componentv2"
	"github.com/openchoreo/openchoreo/internal/controller/dataplane"
	"github.com/openchoreo/openchoreo/internal/controller/deployableartifact"
	"github.com/openchoreo/openchoreo/internal/controller/deployment"
	"github.com/openchoreo/openchoreo/internal/controller/deploymentpipeline"
	"github.com/openchoreo/openchoreo/internal/controller/deploymenttrack"
	"github.com/openchoreo/openchoreo/internal/controller/endpoint"
	"github.com/openchoreo/openchoreo/internal/controller/endpointclass"
	"github.com/openchoreo/openchoreo/internal/controller/endpointrelease"
	"github.com/openchoreo/openchoreo/internal/controller/endpointv2"
	"github.com/openchoreo/openchoreo/internal/controller/environment"
	"github.com/openchoreo/openchoreo/internal/controller/gitcommitrequest"
	"github.com/openchoreo/openchoreo/internal/controller/organization"
	"github.com/openchoreo/openchoreo/internal/controller/project"
	"github.com/openchoreo/openchoreo/internal/controller/scheduledtask"
	"github.com/openchoreo/openchoreo/internal/controller/scheduledtaskbinding"
	"github.com/openchoreo/openchoreo/internal/controller/scheduledtaskclass"
	"github.com/openchoreo/openchoreo/internal/controller/scheduledtaskrelease"
	"github.com/openchoreo/openchoreo/internal/controller/service"
	"github.com/openchoreo/openchoreo/internal/controller/servicebinding"
	"github.com/openchoreo/openchoreo/internal/controller/serviceclass"
	"github.com/openchoreo/openchoreo/internal/controller/servicerelease"
	"github.com/openchoreo/openchoreo/internal/controller/webapplication"
	"github.com/openchoreo/openchoreo/internal/controller/webapplicationbinding"
	"github.com/openchoreo/openchoreo/internal/controller/webapplicationclass"
	"github.com/openchoreo/openchoreo/internal/controller/webapplicationrelease"
	"github.com/openchoreo/openchoreo/internal/controller/workload"
	"github.com/openchoreo/openchoreo/internal/controller/workloadbinding"
	"github.com/openchoreo/openchoreo/internal/controller/workloadclass"
	"github.com/openchoreo/openchoreo/internal/controller/workloadrelease"
	dpKubernetes "github.com/openchoreo/openchoreo/internal/dataplane/kubernetes"
	argo "github.com/openchoreo/openchoreo/internal/dataplane/kubernetes/types/argoproj.io/workflow/v1alpha1"
	ciliumv2 "github.com/openchoreo/openchoreo/internal/dataplane/kubernetes/types/cilium.io/v2"
	csisecretv1 "github.com/openchoreo/openchoreo/internal/dataplane/kubernetes/types/secretstorecsi/v1"
	"github.com/openchoreo/openchoreo/internal/version"
	webhookcorev1 "github.com/openchoreo/openchoreo/internal/webhook/v1"
	// +kubebuilder:scaffold:imports
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	utilruntime.Must(ciliumv2.AddToScheme(scheme))
	utilruntime.Must(choreov1.AddToScheme(scheme))
	utilruntime.Must(gwapiv1.Install(scheme))
	utilruntime.Must(egv1a1.AddToScheme(scheme))
	utilruntime.Must(argo.AddToScheme(scheme))
	utilruntime.Must(csisecretv1.Install(scheme))
	utilruntime.Must(corev1.AddToScheme(scheme))
	// +kubebuilder:scaffold:scheme
}

// nolint:gocyclo
func main() {
	var metricsAddr string
	var enableLeaderElection bool
	var probeAddr string
	var secureMetrics bool
	var enableHTTP2 bool
	var enableLegacyCRDs bool
	var tlsOpts []func(*tls.Config)
	flag.StringVar(&metricsAddr, "metrics-bind-address", "0", "The address the metrics endpoint binds to. "+
		"Use :8443 for HTTPS or :8080 for HTTP, or leave as 0 to disable the metrics service.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	flag.BoolVar(&secureMetrics, "metrics-secure", true,
		"If set, the metrics endpoint is served securely via HTTPS. Use --metrics-secure=false to use HTTP instead.")
	flag.BoolVar(&enableHTTP2, "enable-http2", false,
		"If set, HTTP/2 will be enabled for the metrics and webhook servers")
	flag.BoolVar(&enableLegacyCRDs, "enable-legacy-crds", false, // TODO <-- remove me
		"If set, legacy CRDs will be enabled. This is only for the POC and will be removed in the future.")
	opts := zap.Options{
		Development: true,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	setupLog.Info("starting controller manager", version.GetLogKeyValues()...)

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
		LeaderElectionID:       "43500532.choreo.dev",
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

	// Initialize dataPlane client manager
	dpClientMgr := dpKubernetes.NewManager()

	// -----------------------------------------------------------------------------
	// Setup controllers with the controller manager
	// -----------------------------------------------------------------------------

	if enableLegacyCRDs {
		if err = (&organization.Reconciler{
			Client: mgr.GetClient(),
			Scheme: mgr.GetScheme(),
		}).SetupWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create controller", "controller", "Organization")
			os.Exit(1)
		}
		if err = (&project.Reconciler{
			Client: mgr.GetClient(),
			Scheme: mgr.GetScheme(),
		}).SetupWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create controller", "controller", "Project")
			os.Exit(1)
		}
		if err = (&build.Reconciler{
			Client:       mgr.GetClient(),
			DpClientMgr:  dpClientMgr,
			Scheme:       mgr.GetScheme(),
			GithubClient: github.NewClient(nil),
		}).SetupWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create controller", "controller", "Build")
			os.Exit(1)
		}
		if err = (&environment.Reconciler{
			Client:      mgr.GetClient(),
			DpClientMgr: dpClientMgr,
			Scheme:      mgr.GetScheme(),
		}).SetupWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create controller", "controller", "Environment")
			os.Exit(1)
		}
		if err = (&dataplane.Reconciler{
			Client: mgr.GetClient(),
			Scheme: mgr.GetScheme(),
		}).SetupWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create controller", "controller", "DataPlane")
			os.Exit(1)
		}
		if err = (&deploymentpipeline.Reconciler{
			Client: mgr.GetClient(),
			Scheme: mgr.GetScheme(),
		}).SetupWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create controller", "controller", "DeploymentPipeline")
			os.Exit(1)
		}
		if err = (&component.Reconciler{
			Client: mgr.GetClient(),
			Scheme: mgr.GetScheme(),
		}).SetupWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create controller", "controller", "Component")
			os.Exit(1)
		}
		if err = (&deploymenttrack.Reconciler{
			Client: mgr.GetClient(),
			Scheme: mgr.GetScheme(),
		}).SetupWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create controller", "controller", "DeploymentTrack")
			os.Exit(1)
		}
		if err = (&deployableartifact.Reconciler{
			Client: mgr.GetClient(),
			Scheme: mgr.GetScheme(),
		}).SetupWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create controller", "controller", "DeployableArtifact")
			os.Exit(1)
		}
		if err = (&deployment.Reconciler{
			Client:      mgr.GetClient(),
			DpClientMgr: dpClientMgr,
			Scheme:      mgr.GetScheme(),
		}).SetupWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create controller", "controller", "Deployment")
			os.Exit(1)
		}
		if err = (&endpoint.Reconciler{
			Client:      mgr.GetClient(),
			DpClientMgr: dpClientMgr,
			Scheme:      mgr.GetScheme(),
		}).SetupWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create controller", "controller", "Endpoint")
			os.Exit(1)
		}
		// PoC controllers that will be removed in the future.
		if err = (&workloadclass.Reconciler{
			Client: mgr.GetClient(),
			Scheme: mgr.GetScheme(),
		}).SetupWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create controller", "controller", "WorkloadClass")
			os.Exit(1)
		}
		if err = (&workload.Reconciler{
			Client: mgr.GetClient(),
			Scheme: mgr.GetScheme(),
		}).SetupWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create controller", "controller", "Workload")
			os.Exit(1)
		}
		if err = (&workloadbinding.Reconciler{
			Client: mgr.GetClient(),
			Scheme: mgr.GetScheme(),
		}).SetupWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create controller", "controller", "WorkloadBinding")
			os.Exit(1)
		}
		if err = (&workloadrelease.Reconciler{
			Client:      mgr.GetClient(),
			DpClientMgr: dpClientMgr,
			Scheme:      mgr.GetScheme(),
		}).SetupWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create controller", "controller", "WorkloadRelease")
			os.Exit(1)
		}
		if err = (&endpointv2.Reconciler{
			Client: mgr.GetClient(),
			Scheme: mgr.GetScheme(),
		}).SetupWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create controller", "controller", "EndpointV2")
			os.Exit(1)
		}
		if err = (&endpointclass.Reconciler{
			Client: mgr.GetClient(),
			Scheme: mgr.GetScheme(),
		}).SetupWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create controller", "controller", "EndpointClass")
			os.Exit(1)
		}
		if err = (&endpointrelease.Reconciler{
			Client: mgr.GetClient(),
			Scheme: mgr.GetScheme(),
		}).SetupWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create controller", "controller", "EndpointRelease")
			os.Exit(1)
		}
	}

	if err = (&componentv2.Reconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "ComponentV2")
		os.Exit(1)
	}

	if err = (&gitcommitrequest.Reconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "GitCommitRequest")
		os.Exit(1)
	}

	// API controllers
	if err = (&api.Reconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "API")
		os.Exit(1)
	}
	if err = (&apiclass.Reconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "APIClass")
		os.Exit(1)
	}
	if err = (&apirelease.Reconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "APIRelease")
		os.Exit(1)
	}
	if err = (&apibinding.Reconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "APIBinding")
		os.Exit(1)
	}

	// Service controllers
	if err := (&service.Reconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Service")
		os.Exit(1)
	}
	if err := (&serviceclass.Reconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "ServiceClass")
		os.Exit(1)
	}
	if err := (&servicebinding.Reconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "ServiceBinding")
		os.Exit(1)
	}
	if err := (&servicerelease.Reconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "ServiceRelease")
		os.Exit(1)
	}

	// WebApplication controllers
	if err := (&webapplication.Reconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "WebApplication")
		os.Exit(1)
	}
	if err := (&webapplicationclass.Reconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "WebApplicationClass")
		os.Exit(1)
	}
	if err := (&webapplicationbinding.Reconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "WebApplicationBinding")
		os.Exit(1)
	}
	if err := (&webapplicationrelease.Reconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "WebApplicationRelease")
		os.Exit(1)
	}

	// ScheduledTask controllers
	if err := (&scheduledtask.Reconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "ScheduledTask")
		os.Exit(1)
	}
	if err := (&scheduledtaskclass.Reconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "ScheduledTaskClass")
		os.Exit(1)
	}
	if err := (&scheduledtaskbinding.Reconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "ScheduledTaskBinding")
		os.Exit(1)
	}
	if err := (&scheduledtaskrelease.Reconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "ScheduledTaskRelease")
		os.Exit(1)
	}

	// +kubebuilder:scaffold:builder

	// -----------------------------------------------------------------------------
	// Setup webhooks with the controller manager
	// -----------------------------------------------------------------------------

	// nolint:goconst
	if os.Getenv("ENABLE_WEBHOOKS") != "false" {
		if err = webhookcorev1.SetupProjectWebhookWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create webhook", "webhook", "Project")
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
