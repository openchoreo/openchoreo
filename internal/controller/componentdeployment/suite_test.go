// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package componentdeployment

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
	"sigs.k8s.io/controller-runtime/pkg/manager"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	componentpipeline "github.com/openchoreo/openchoreo/internal/crd-renderer/component-pipeline"
	// +kubebuilder:scaffold:imports
)

// These tests use Ginkgo (BDD-style Go testing framework). Refer to
// http://onsi.github.io/ginkgo/ to learn more about Ginkgo.

var cfg *rest.Config
var k8sClient client.Client
var testEnv *envtest.Environment
var ctx context.Context
var cancel context.CancelFunc
var k8sManager manager.Manager

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

	// Create a manager to properly set up field indexes
	k8sManager, err = ctrl.NewManager(cfg, ctrl.Options{
		Scheme: scheme.Scheme,
		Cache: cache.Options{
			DefaultNamespaces: map[string]cache.Config{},
		},
	})
	Expect(err).NotTo(HaveOccurred())

	// Set up field indexes without registering the controller
	// This allows tests to call Reconcile manually without conflicts
	reconciler := &Reconciler{
		Client:   k8sManager.GetClient(),
		Scheme:   k8sManager.GetScheme(),
		Pipeline: componentpipeline.NewPipeline(),
	}

	// Register field indexes manually (without the controller)
	err = reconciler.setupComponentIndex(ctx, k8sManager)
	Expect(err).NotTo(HaveOccurred())

	err = reconciler.setupEnvironmentIndex(ctx, k8sManager)
	Expect(err).NotTo(HaveOccurred())

	err = reconciler.setupComponentDeploymentCompositeIndex(ctx, k8sManager)
	Expect(err).NotTo(HaveOccurred())

	err = reconciler.setupSnapshotOwnerIndex(ctx, k8sManager)
	Expect(err).NotTo(HaveOccurred())

	// Start the manager in the background
	go func() {
		defer GinkgoRecover()
		err = k8sManager.Start(ctx)
		Expect(err).ToNot(HaveOccurred(), "failed to run manager")
	}()

	// Wait for the cache to sync before running tests
	By("Waiting for cache to sync")
	Expect(k8sManager.GetCache().WaitForCacheSync(ctx)).To(BeTrue())

	// Use the manager's client for tests
	k8sClient = k8sManager.GetClient()
	Expect(k8sClient).NotTo(BeNil())

})

var _ = AfterSuite(func() {
	By("tearing down the test environment")
	cancel()
	err := testEnv.Stop()
	Expect(err).NotTo(HaveOccurred())
})
