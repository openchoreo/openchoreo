// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package observabilityalertrule

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

var _ = Describe("ObservabilityAlertRule Controller", func() {
	Context("When reconciling a resource", func() {
		const resourceName = "test-resource"

		ctx := context.Background()
		var testServer *httptest.Server
		var originalObserverEndpoint string

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default",
		}
		observabilityalertrule := &openchoreov1alpha1.ObservabilityAlertRule{}

		BeforeEach(func() {
			// Save original OBSERVER_INTERNAL_ENDPOINT if set
			originalObserverEndpoint = os.Getenv("OBSERVER_INTERNAL_ENDPOINT")

			// Set up a test HTTP server to mock the observer internal API
			testServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// GET /api/v1alpha1/alerts/sources/{sourceType}/rules/{ruleName}
				if r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, alertsV1alpha1BasePath+"/") {
					// Simulate rule not found so POST path is exercised
					w.WriteHeader(http.StatusNotFound)
					return
				}
				// POST /api/v1alpha1/alerts/sources/{sourceType}/rules
				if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/rules") {
					response := alertRuleSyncResponse{
						Status:        "synced",
						Action:        "created",
						RuleLogicalID: resourceName,
						RuleBackendID: "test-backend-id-12345",
						LastSyncedAt:  time.Now().UTC().Format(time.RFC3339),
					}
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusCreated)
					if err := json.NewEncoder(w).Encode(response); err != nil {
						w.WriteHeader(http.StatusInternalServerError)
					}
					return
				}
				w.WriteHeader(http.StatusNotFound)
			}))

			// Point controller at test server
			os.Setenv("OBSERVER_INTERNAL_ENDPOINT", testServer.URL)

			By("creating the custom resource for the Kind ObservabilityAlertRule")
			err := k8sClient.Get(ctx, typeNamespacedName, observabilityalertrule)
			if err != nil && errors.IsNotFound(err) {
				resource := &openchoreov1alpha1.ObservabilityAlertRule{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: "default",
						Labels: map[string]string{
							"openchoreo.dev/component-uid":   "62b88e15-efc4-46da-86e3-cf19c6253118",
							"openchoreo.dev/project-uid":     "ba3de13e-ca40-44c6-9a30-02fc3db7c5a2",
							"openchoreo.dev/environment-uid": "b39a6cad-1b25-495a-a249-60d87275b60f",
						},
					},
					Spec: openchoreov1alpha1.ObservabilityAlertRuleSpec{
						Name:                resourceName,
						NotificationChannel: "test-channel",
						Source: openchoreov1alpha1.ObservabilityAlertSource{
							Type:  openchoreov1alpha1.ObservabilityAlertSourceTypeLog,
							Query: "error",
						},
						Condition: openchoreov1alpha1.ObservabilityAlertCondition{
							Window:    metav1.Duration{Duration: 5 * time.Minute},
							Interval:  metav1.Duration{Duration: 1 * time.Minute},
							Operator:  openchoreov1alpha1.ObservabilityAlertConditionOperatorGt,
							Threshold: 10,
						},
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			// Cleanup the resource instance
			resource := &openchoreov1alpha1.ObservabilityAlertRule{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			if err == nil {
				By("Cleanup the specific resource instance ObservabilityAlertRule")
				Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
			}

			// Restore original OBSERVER_INTERNAL_ENDPOINT
			if originalObserverEndpoint != "" {
				os.Setenv("OBSERVER_INTERNAL_ENDPOINT", originalObserverEndpoint)
			} else {
				os.Unsetenv("OBSERVER_INTERNAL_ENDPOINT")
			}

			// Close test server
			if testServer != nil {
				testServer.Close()
			}
		})

		It("should successfully reconcile the resource", func() {
			By("Reconciling the created resource")
			controllerReconciler := &Reconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
				httpClient: &http.Client{
					Timeout: 10 * time.Second,
				},
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
		})
	})
})
