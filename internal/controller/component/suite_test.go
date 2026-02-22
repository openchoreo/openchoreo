// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package component

import (
	"context"
	"fmt"
	"path/filepath"
	"runtime"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/controller"
	ocLabels "github.com/openchoreo/openchoreo/internal/labels"
)

// These tests use Ginkgo (BDD-style Go testing framework). Refer to
// http://onsi.github.io/ginkgo/ to learn more about Ginkgo.

var cfg *rest.Config
var k8sClient client.Client
var testEnv *envtest.Environment
var ctx context.Context
var cancel context.CancelFunc

func TestControllers(t *testing.T) {
	RegisterFailHandler(Fail)

	RunSpecs(t, "Controller Suite")
}

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	ctx, cancel = context.WithCancel(context.TODO())

	By("bootstrapping test environment")
	testEnv = &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "..", "..", "config", "crd", "bases")},
		ErrorIfCRDPathMissing: true,

		// The BinaryAssetsDirectory is only required if you want to run the tests directly
		// without call the makefile target test. If not informed it will look for the
		// default path defined in controller-runtime which is /usr/local/kubebuilder/.
		// Note that you must have the required binaries setup under the bin directory to perform
		// the tests directly. When we run make test it will be setup and used automatically.
		BinaryAssetsDirectory: filepath.Join("..", "..", "..", "bin", "tools", "k8s",
			fmt.Sprintf("1.32.0-%s-%s", runtime.GOOS, runtime.GOARCH)),
	}

	var err error
	// cfg is defined in this file globally.
	cfg, err = testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())

	err = openchoreov1alpha1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	// +kubebuilder:scaffold:scheme

	// Create a manager with cache enabled only for types that need field index queries
	// (ComponentRelease, ReleaseBinding, Workload, WorkflowRun for owner lookups during finalization)
	// Other types bypass cache to avoid staleness issues in tests
	mgr, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme: scheme.Scheme,
		Metrics: metricsserver.Options{
			BindAddress: "0", // Disable metrics server in tests
		},
		Cache: cache.Options{
			// Only cache types required for field index queries
			ByObject: map[client.Object]cache.ByObject{
				&openchoreov1alpha1.ComponentRelease{}: {},
				&openchoreov1alpha1.ReleaseBinding{}:   {},
				&openchoreov1alpha1.Workload{}:         {},
				&openchoreov1alpha1.WorkflowRun{}:      {},
			},
		},
		Client: client.Options{
			Cache: &client.CacheOptions{
				// Disable cache reads for types not requiring field indices
				DisableFor: []client.Object{
					&openchoreov1alpha1.Component{},
				},
			},
		},
	})
	Expect(err).NotTo(HaveOccurred())

	// Register field indices used by the component controller for finalization
	err = mgr.GetFieldIndexer().IndexField(ctx, &openchoreov1alpha1.ComponentRelease{},
		"spec.owner.componentName", func(obj client.Object) []string {
			release := obj.(*openchoreov1alpha1.ComponentRelease)
			if release.Spec.Owner.ComponentName == "" {
				return nil
			}
			return []string{release.Spec.Owner.ComponentName}
		})
	Expect(err).NotTo(HaveOccurred())

	err = mgr.GetFieldIndexer().IndexField(ctx, &openchoreov1alpha1.ReleaseBinding{},
		controller.IndexKeyReleaseBindingOwnerComponentName, func(obj client.Object) []string {
			binding := obj.(*openchoreov1alpha1.ReleaseBinding)
			if binding.Spec.Owner.ComponentName == "" {
				return nil
			}
			return []string{binding.Spec.Owner.ComponentName}
		})
	Expect(err).NotTo(HaveOccurred())

	// Register field index for Workload by owner (projectName/componentName)
	err = mgr.GetFieldIndexer().IndexField(ctx, &openchoreov1alpha1.Workload{},
		workloadOwnerIndex, func(obj client.Object) []string {
			workload := obj.(*openchoreov1alpha1.Workload)
			ownerKey := fmt.Sprintf("%s/%s",
				workload.Spec.Owner.ProjectName,
				workload.Spec.Owner.ComponentName)
			return []string{ownerKey}
		})
	Expect(err).NotTo(HaveOccurred())

	// Register field index for WorkflowRun by owner component label.
	err = mgr.GetFieldIndexer().IndexField(ctx, &openchoreov1alpha1.WorkflowRun{},
		workflowRunOwnerIndex, func(obj client.Object) []string {
			workflowRun := obj.(*openchoreov1alpha1.WorkflowRun)
			componentName := workflowRun.Labels[ocLabels.LabelKeyComponentName]
			if componentName == "" {
				return nil
			}
			return []string{componentName}
		})
	Expect(err).NotTo(HaveOccurred())

	// Register field index for Project by deploymentPipelineRef
	err = mgr.GetFieldIndexer().IndexField(ctx, &openchoreov1alpha1.Project{},
		controller.IndexKeyProjectDeploymentPipelineRef, func(obj client.Object) []string {
			project := obj.(*openchoreov1alpha1.Project)
			if project.Spec.DeploymentPipelineRef == "" {
				return nil
			}
			return []string{project.Spec.DeploymentPipelineRef}
		})
	Expect(err).NotTo(HaveOccurred())

	// Register field index for Component by owner project name
	err = mgr.GetFieldIndexer().IndexField(ctx, &openchoreov1alpha1.Component{},
		controller.IndexKeyComponentOwnerProjectName, func(obj client.Object) []string {
			component := obj.(*openchoreov1alpha1.Component)
			if component.Spec.Owner.ProjectName == "" {
				return nil
			}
			return []string{component.Spec.Owner.ProjectName}
		})
	Expect(err).NotTo(HaveOccurred())

	// Start the manager in a goroutine
	go func() {
		defer GinkgoRecover()
		err := mgr.Start(ctx)
		Expect(err).NotTo(HaveOccurred())
	}()

	// Wait for cache to sync before running tests
	Expect(mgr.GetCache().WaitForCacheSync(ctx)).To(BeTrue())

	// Use the manager's client which has field index support
	k8sClient = mgr.GetClient()
	Expect(k8sClient).NotTo(BeNil())
})

var _ = AfterSuite(func() {
	By("tearing down the test environment")
	cancel()
	err := testEnv.Stop()
	Expect(err).NotTo(HaveOccurred())
})
