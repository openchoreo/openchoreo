// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package revision

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

// These tests use Ginkgo (BDD-style Go testing framework). Refer to
// http://onsi.github.io/ginkgo/ to learn more about Ginkgo.
//
// The suite runs the revision CRDs against a real kube-apiserver so the
// x-kubernetes-validations rules and the status subresource are exercised as
// the cluster enforces them. No controller or webhook is started: the contract
// under test is enforced by the CRD alone.

var (
	cfg       *rest.Config
	k8sClient client.Client
	testEnv   *envtest.Environment
	ctx       context.Context
	cancel    context.CancelFunc
)

func TestRevisionCRDs(t *testing.T) {
	RegisterFailHandler(Fail)

	RunSpecs(t, "Revision CRD Suite")
}

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	ctx, cancel = context.WithCancel(context.TODO())

	binaryAssetsDir := filepath.Join("..", "..", "bin", "tools", "k8s",
		fmt.Sprintf("1.36.0-%s-%s", runtime.GOOS, runtime.GOARCH))

	// Missing envtest binaries fail the run rather than skip it: a skipped suite
	// passes silently while the CRD contract goes untested. `make test` sets
	// KUBEBUILDER_ASSETS; for a plain `go test`, install them with
	// `make envtest && bin/tools/setup-envtest use 1.36.0 --bin-dir bin/tools`.
	if _, err := os.Stat(binaryAssetsDir); err != nil && os.Getenv("KUBEBUILDER_ASSETS") == "" {
		Fail(fmt.Sprintf("envtest binaries not found at %s and KUBEBUILDER_ASSETS is unset; "+
			"run `make test`, or install them with `make envtest` and "+
			"`bin/tools/setup-envtest use 1.36.0 --bin-dir bin/tools`", binaryAssetsDir))
	}

	By("bootstrapping test environment")
	testEnv = &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "..", "config", "crd", "bases")},
		ErrorIfCRDPathMissing: true,
		BinaryAssetsDirectory: binaryAssetsDir,
	}

	var err error
	cfg, err = testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())

	scheme := k8sruntime.NewScheme()
	Expect(openchoreov1alpha1.AddToScheme(scheme)).To(Succeed())

	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(k8sClient).NotTo(BeNil())
})

var _ = AfterSuite(func() {
	if cancel != nil {
		cancel()
	}
	if testEnv == nil {
		return
	}
	By("tearing down the test environment")
	Expect(testEnv.Stop()).To(Succeed())
})
