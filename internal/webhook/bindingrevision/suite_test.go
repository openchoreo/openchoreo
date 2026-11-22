// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package bindingrevision

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	openchoreodevv1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

var (
	ctx       context.Context
	cancel    context.CancelFunc
	k8sClient client.Client
	cfg       *rest.Config
	testEnv   *envtest.Environment
)

func TestAPIs(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Webhook Suite")
}

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	ctx, cancel = context.WithCancel(context.TODO())

	// Skip envtest setup when binaries are not available and no envtest asset
	// environment variables are set; unit tests calling webhook functions directly
	// will still run.
	binaryAssetsDir := getFirstFoundEnvTestBinaryDir()
	if binaryAssetsDir == "" &&
		os.Getenv("KUBEBUILDER_ASSETS") == "" &&
		os.Getenv("TEST_ASSET_KUBE_APISERVER") == "" &&
		os.Getenv("TEST_ASSET_ETCD") == "" &&
		os.Getenv("TEST_ASSET_KUBECTL") == "" {
		return
	}

	var err error
	err = openchoreodevv1alpha1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	By("bootstrapping test environment")
	testEnv = &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "..", "..", "config", "crd", "bases")},
		ErrorIfCRDPathMissing: false,
		WebhookInstallOptions: envtest.WebhookInstallOptions{
			Paths: []string{filepath.Join("..", "..", "..", "config", "webhook")},
		},
	}

	if binaryAssetsDir != "" {
		testEnv.BinaryAssetsDirectory = binaryAssetsDir
	}

	cfg, err = testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())

	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(k8sClient).NotTo(BeNil())

	webhookInstallOptions := &testEnv.WebhookInstallOptions
	mgr, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme: scheme.Scheme,
		WebhookServer: webhook.NewServer(webhook.Options{
			Host:    webhookInstallOptions.LocalServingHost,
			Port:    webhookInstallOptions.LocalServingPort,
			CertDir: webhookInstallOptions.LocalServingCertDir,
		}),
		LeaderElection: false,
		Metrics:        metricsserver.Options{BindAddress: "0"},
	})
	Expect(err).NotTo(HaveOccurred())

	err = SetupReleaseBindingRevisionWebhookWithManager(mgr)
	Expect(err).NotTo(HaveOccurred())
	err = SetupResourceReleaseBindingRevisionWebhookWithManager(mgr)
	Expect(err).NotTo(HaveOccurred())

	go func() {
		defer GinkgoRecover()
		err = mgr.Start(ctx)
		Expect(err).NotTo(HaveOccurred())
	}()

	dialer := &net.Dialer{Timeout: time.Second}
	addrPort := fmt.Sprintf("%s:%d", webhookInstallOptions.LocalServingHost, webhookInstallOptions.LocalServingPort)
	Eventually(func() error {
		//nolint:gosec // G402: Using self-signed cert in test environment
		conn, err := tls.DialWithDialer(dialer, "tcp", addrPort, &tls.Config{InsecureSkipVerify: true})
		if err != nil {
			return err
		}
		return conn.Close()
	}).Should(Succeed())

	// The listener being up does not mean the apiserver calls the webhook yet:
	// it applies a webhook configuration some time after the object exists.
	waitForWebhook()
})

// waitForWebhook returns once the apiserver sends ReleaseBindingRevision
// updates through the validating webhook: a spec change only the webhook
// catches, a config key replaced by another, which the CRD's CEL rule lets
// through, is denied by it.
func waitForWebhook() {
	GinkgoHelper()
	rev := releaseBindingRevision()
	Expect(k8sClient.Create(ctx, rev)).To(Succeed())
	attempt := 0
	Eventually(func(g Gomega) {
		// Each attempt renames the key anew: an attempt that got through before
		// the webhook was active changed the stored spec.
		attempt++
		g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(rev), rev)).To(Succeed())
		rev.Spec.BindingSpec.ComponentTypeEnvironmentConfigs.Raw = fmt.Appendf(nil, `{"probe%d":2,"cpu":"100m"}`, attempt)
		g.Expect(k8sClient.Update(ctx, rev)).To(MatchError(ContainSubstring(
			fmt.Sprintf("admission webhook %q denied the request", releaseBindingRevisionWebhook))))
	}).WithTimeout(30 * time.Second).Should(Succeed())
	Expect(k8sClient.Delete(ctx, rev)).To(Succeed())
}

var _ = AfterSuite(func() {
	cancel()
	if testEnv == nil {
		return
	}
	By("tearing down the test environment")
	err := testEnv.Stop()
	Expect(err).NotTo(HaveOccurred())
})

func getFirstFoundEnvTestBinaryDir() string {
	basePath := filepath.Join("..", "..", "..", "bin", "k8s")
	entries, err := os.ReadDir(basePath)
	if err != nil {
		logf.Log.Error(err, "Failed to read directory", "path", basePath)
		return ""
	}
	for _, entry := range entries {
		if entry.IsDir() {
			return filepath.Join(basePath, entry.Name())
		}
	}
	return ""
}
