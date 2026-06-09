// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package observabilityalertrule

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func newAlertRule(name string, labels map[string]string) *openchoreov1alpha1.ObservabilityAlertRule {
	return &openchoreov1alpha1.ObservabilityAlertRule{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "default",
			Labels:    labels,
		},
		Spec: openchoreov1alpha1.ObservabilityAlertRuleSpec{
			Name: name,
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
			Actions: openchoreov1alpha1.ObservabilityAlertActions{
				Notifications: openchoreov1alpha1.ObservabilityAlertNotifications{
					Channels: []openchoreov1alpha1.NotificationChannelName{"test-channel"},
				},
			},
		},
	}
}

func defaultLabels() map[string]string {
	return map[string]string{
		"openchoreo.dev/component-uid":   "62b88e15-efc4-46da-86e3-cf19c6253118",
		"openchoreo.dev/project-uid":     "ba3de13e-ca40-44c6-9a30-02fc3db7c5a2",
		"openchoreo.dev/environment-uid": "b39a6cad-1b25-495a-a249-60d87275b60f",
	}
}

func forceDeleteAlertRule(nn types.NamespacedName) {
	rule := &openchoreov1alpha1.ObservabilityAlertRule{}
	if err := k8sClient.Get(ctx, nn, rule); err != nil {
		return
	}
	if controllerutil.ContainsFinalizer(rule, AlertRuleCleanupFinalizer) {
		controllerutil.RemoveFinalizer(rule, AlertRuleCleanupFinalizer)
		_ = k8sClient.Update(ctx, rule)
	}
	_ = k8sClient.Delete(ctx, rule)
}

func newReconciler() *Reconciler {
	return &Reconciler{
		Client: k8sClient,
		Scheme: k8sClient.Scheme(),
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

func reconcileRequest(name string) reconcile.Request {
	return reconcile.Request{
		NamespacedName: types.NamespacedName{Name: name, Namespace: "default"},
	}
}

// ---------------------------------------------------------------------------
// integration tests
// ---------------------------------------------------------------------------

var _ = Describe("ObservabilityAlertRule Controller", func() {

	// -----------------------------------------------------------------------
	// Non-existent resource
	// -----------------------------------------------------------------------
	Context("When reconciling a non-existent resource", func() {
		It("should return no error", func() {
			r := newReconciler()
			result, err := r.Reconcile(ctx, reconcileRequest("does-not-exist"))
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Requeue).To(BeFalse())
		})
	})

	// -----------------------------------------------------------------------
	// First reconcile: adds finalizer
	// -----------------------------------------------------------------------
	Context("When reconciling a new resource for the first time", func() {
		const name = "test-first-reconcile"
		nn := types.NamespacedName{Name: name, Namespace: "default"}

		BeforeEach(func() {
			rule := newAlertRule(name, defaultLabels())
			Expect(k8sClient.Create(ctx, rule)).To(Succeed())
		})

		AfterEach(func() {
			forceDeleteAlertRule(nn)
		})

		It("should add the cleanup finalizer and requeue", func() {
			r := newReconciler()
			result, err := r.Reconcile(ctx, reconcileRequest(name))
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Requeue).To(BeTrue())

			rule := &openchoreov1alpha1.ObservabilityAlertRule{}
			Expect(k8sClient.Get(ctx, nn, rule)).To(Succeed())
			Expect(controllerutil.ContainsFinalizer(rule, AlertRuleCleanupFinalizer)).To(BeTrue())
		})
	})

	// -----------------------------------------------------------------------
	// Missing labels
	// -----------------------------------------------------------------------
	Context("When reconciling a resource with missing UID labels", func() {
		AfterEach(func() {
			forceDeleteAlertRule(types.NamespacedName{Name: "test-missing-comp", Namespace: "default"})
			forceDeleteAlertRule(types.NamespacedName{Name: "test-missing-proj", Namespace: "default"})
			forceDeleteAlertRule(types.NamespacedName{Name: "test-missing-env", Namespace: "default"})
		})

		It("should set Error status when component-uid label is missing", func() {
			labels := defaultLabels()
			delete(labels, "openchoreo.dev/component-uid")
			rule := newAlertRule("test-missing-comp", labels)
			controllerutil.AddFinalizer(rule, AlertRuleCleanupFinalizer)
			Expect(k8sClient.Create(ctx, rule)).To(Succeed())

			r := &Reconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			_, err := r.Reconcile(ctx, reconcileRequest("test-missing-comp"))
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("component UID is required"))

			fetched := &openchoreov1alpha1.ObservabilityAlertRule{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "test-missing-comp", Namespace: "default"}, fetched)).To(Succeed())
			Expect(fetched.Status.Phase).To(Equal(openchoreov1alpha1.ObservabilityAlertRulePhaseError))
			cond := apimeta.FindStatusCondition(fetched.Status.Conditions, conditionTypeSynced)
			Expect(cond).NotTo(BeNil())
			Expect(cond.Status).To(Equal(metav1.ConditionFalse))
			Expect(cond.Reason).To(Equal("SyncFailed"))
		})

		It("should set Error status when project-uid label is missing", func() {
			labels := defaultLabels()
			delete(labels, "openchoreo.dev/project-uid")
			rule := newAlertRule("test-missing-proj", labels)
			controllerutil.AddFinalizer(rule, AlertRuleCleanupFinalizer)
			Expect(k8sClient.Create(ctx, rule)).To(Succeed())

			r := &Reconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			_, err := r.Reconcile(ctx, reconcileRequest("test-missing-proj"))
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("project UID is required"))

			fetched := &openchoreov1alpha1.ObservabilityAlertRule{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "test-missing-proj", Namespace: "default"}, fetched)).To(Succeed())
			Expect(fetched.Status.Phase).To(Equal(openchoreov1alpha1.ObservabilityAlertRulePhaseError))
		})

		It("should set Error status when environment-uid label is missing", func() {
			labels := defaultLabels()
			delete(labels, "openchoreo.dev/environment-uid")
			rule := newAlertRule("test-missing-env", labels)
			controllerutil.AddFinalizer(rule, AlertRuleCleanupFinalizer)
			Expect(k8sClient.Create(ctx, rule)).To(Succeed())

			r := &Reconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			_, err := r.Reconcile(ctx, reconcileRequest("test-missing-env"))
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("environment UID is required"))

			fetched := &openchoreov1alpha1.ObservabilityAlertRule{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "test-missing-env", Namespace: "default"}, fetched)).To(Succeed())
			Expect(fetched.Status.Phase).To(Equal(openchoreov1alpha1.ObservabilityAlertRulePhaseError))
		})
	})

	// -----------------------------------------------------------------------
	// Successful sync: POST (new rule)
	// -----------------------------------------------------------------------
	Context("When syncing a new alert rule via POST", func() {
		const name = "test-post-rule"
		nn := types.NamespacedName{Name: name, Namespace: "default"}
		var testServer *httptest.Server
		var origEndpoint string

		BeforeEach(func() {
			origEndpoint = os.Getenv("OBSERVER_INTERNAL_ENDPOINT")

			testServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				defer GinkgoRecover()

				// GET → 404 (rule does not exist yet)
				if r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/rules/"+name) {
					w.WriteHeader(http.StatusNotFound)
					return
				}
				// POST → 201
				if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/rules") {
					body, err := io.ReadAll(r.Body)
					Expect(err).NotTo(HaveOccurred())
					var payload alertRuleRequest
					Expect(json.Unmarshal(body, &payload)).To(Succeed())
					Expect(payload.Metadata.Name).To(Equal(name))
					Expect(payload.Metadata.Namespace).To(Equal("default"))
					Expect(payload.Source.Type).To(Equal("log"))
					Expect(payload.Condition.Enabled).To(BeTrue())
					Expect(payload.Condition.Window).To(Equal("5m"))
					Expect(payload.Condition.Interval).To(Equal("1m"))
					Expect(payload.Condition.Operator).To(Equal("gt"))
					Expect(payload.Condition.Threshold).To(Equal(10.0))

					// Verify actions are not in the payload contract
					var raw map[string]any
					Expect(json.Unmarshal(body, &raw)).To(Succeed())
					_, hasActions := raw["actions"]
					Expect(hasActions).To(BeFalse(), "controller request payload must not include actions")

					resp := alertRuleSyncResponse{
						Status:        "synced",
						Action:        "created",
						RuleLogicalID: name,
						RuleBackendID: "backend-id-001",
						LastSyncedAt:  time.Now().UTC().Format(time.RFC3339),
					}
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusCreated)
					_ = json.NewEncoder(w).Encode(resp)
					return
				}
				w.WriteHeader(http.StatusNotFound)
			}))
			os.Setenv("OBSERVER_INTERNAL_ENDPOINT", testServer.URL)

			rule := newAlertRule(name, defaultLabels())
			controllerutil.AddFinalizer(rule, AlertRuleCleanupFinalizer)
			Expect(k8sClient.Create(ctx, rule)).To(Succeed())
		})

		AfterEach(func() {
			if origEndpoint != "" {
				os.Setenv("OBSERVER_INTERNAL_ENDPOINT", origEndpoint)
			} else {
				os.Unsetenv("OBSERVER_INTERNAL_ENDPOINT")
			}
			if testServer != nil {
				testServer.Close()
			}
			forceDeleteAlertRule(nn)
		})

		It("should call POST and set status to Ready with Synced=True", func() {
			r := newReconciler()
			result, err := r.Reconcile(ctx, reconcileRequest(name))
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Requeue).To(BeFalse())

			fetched := &openchoreov1alpha1.ObservabilityAlertRule{}
			Expect(k8sClient.Get(ctx, nn, fetched)).To(Succeed())
			Expect(fetched.Status.Phase).To(Equal(openchoreov1alpha1.ObservabilityAlertRulePhaseReady))
			Expect(fetched.Status.BackendMonitorID).To(Equal("backend-id-001"))
			Expect(fetched.Status.LastReconcileTime).NotTo(BeNil())
			Expect(fetched.Status.LastSyncTime).NotTo(BeNil())

			cond := apimeta.FindStatusCondition(fetched.Status.Conditions, conditionTypeSynced)
			Expect(cond).NotTo(BeNil())
			Expect(cond.Status).To(Equal(metav1.ConditionTrue))
			Expect(cond.Reason).To(Equal("SyncSucceeded"))
		})
	})

	// -----------------------------------------------------------------------
	// Successful sync: PUT (existing rule)
	// -----------------------------------------------------------------------
	Context("When syncing an existing alert rule via PUT", func() {
		const name = "test-put-rule"
		nn := types.NamespacedName{Name: name, Namespace: "default"}
		var testServer *httptest.Server
		var origEndpoint string

		BeforeEach(func() {
			origEndpoint = os.Getenv("OBSERVER_INTERNAL_ENDPOINT")

			testServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				defer GinkgoRecover()

				// GET → 200 (rule already exists)
				if r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/rules/"+name) {
					getResp := alertRuleGetResponse{
						RuleLogicalID: name,
						RuleBackendID: "backend-id-existing",
					}
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusOK)
					_ = json.NewEncoder(w).Encode(getResp)
					return
				}
				// PUT → 200
				if r.Method == http.MethodPut && strings.Contains(r.URL.Path, "/rules/"+name) {
					resp := alertRuleSyncResponse{
						Status:        "synced",
						Action:        "updated",
						RuleLogicalID: name,
						RuleBackendID: "backend-id-existing",
						LastSyncedAt:  time.Now().UTC().Format(time.RFC3339),
					}
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusOK)
					_ = json.NewEncoder(w).Encode(resp)
					return
				}
				w.WriteHeader(http.StatusNotFound)
			}))
			os.Setenv("OBSERVER_INTERNAL_ENDPOINT", testServer.URL)

			rule := newAlertRule(name, defaultLabels())
			controllerutil.AddFinalizer(rule, AlertRuleCleanupFinalizer)
			Expect(k8sClient.Create(ctx, rule)).To(Succeed())
		})

		AfterEach(func() {
			if origEndpoint != "" {
				os.Setenv("OBSERVER_INTERNAL_ENDPOINT", origEndpoint)
			} else {
				os.Unsetenv("OBSERVER_INTERNAL_ENDPOINT")
			}
			if testServer != nil {
				testServer.Close()
			}
			forceDeleteAlertRule(nn)
		})

		It("should call PUT and set status to Ready", func() {
			r := newReconciler()
			result, err := r.Reconcile(ctx, reconcileRequest(name))
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Requeue).To(BeFalse())

			fetched := &openchoreov1alpha1.ObservabilityAlertRule{}
			Expect(k8sClient.Get(ctx, nn, fetched)).To(Succeed())
			Expect(fetched.Status.Phase).To(Equal(openchoreov1alpha1.ObservabilityAlertRulePhaseReady))
			Expect(fetched.Status.BackendMonitorID).To(Equal("backend-id-existing"))

			cond := apimeta.FindStatusCondition(fetched.Status.Conditions, conditionTypeSynced)
			Expect(cond).NotTo(BeNil())
			Expect(cond.Status).To(Equal(metav1.ConditionTrue))
		})
	})

	// -----------------------------------------------------------------------
	// Sync error: observer returns unexpected status
	// -----------------------------------------------------------------------
	Context("When the observer returns an unexpected GET status", func() {
		const name = "test-get-error"
		nn := types.NamespacedName{Name: name, Namespace: "default"}
		var testServer *httptest.Server
		var origEndpoint string

		BeforeEach(func() {
			origEndpoint = os.Getenv("OBSERVER_INTERNAL_ENDPOINT")

			testServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Return 500 for any GET so upsertAlertRule returns an error
				w.WriteHeader(http.StatusInternalServerError)
			}))
			os.Setenv("OBSERVER_INTERNAL_ENDPOINT", testServer.URL)

			rule := newAlertRule(name, defaultLabels())
			controllerutil.AddFinalizer(rule, AlertRuleCleanupFinalizer)
			Expect(k8sClient.Create(ctx, rule)).To(Succeed())
		})

		AfterEach(func() {
			if origEndpoint != "" {
				os.Setenv("OBSERVER_INTERNAL_ENDPOINT", origEndpoint)
			} else {
				os.Unsetenv("OBSERVER_INTERNAL_ENDPOINT")
			}
			if testServer != nil {
				testServer.Close()
			}
			forceDeleteAlertRule(nn)
		})

		It("should set Error status and return error", func() {
			r := newReconciler()
			_, err := r.Reconcile(ctx, reconcileRequest(name))
			Expect(err).To(HaveOccurred())

			fetched := &openchoreov1alpha1.ObservabilityAlertRule{}
			Expect(k8sClient.Get(ctx, nn, fetched)).To(Succeed())
			Expect(fetched.Status.Phase).To(Equal(openchoreov1alpha1.ObservabilityAlertRulePhaseError))

			cond := apimeta.FindStatusCondition(fetched.Status.Conditions, conditionTypeSynced)
			Expect(cond).NotTo(BeNil())
			Expect(cond.Status).To(Equal(metav1.ConditionFalse))
			Expect(cond.Reason).To(Equal("SyncFailed"))
		})
	})

	// -----------------------------------------------------------------------
	// Sync: observer returns non-synced status (POST path)
	// -----------------------------------------------------------------------
	Context("When the observer reports a non-synced status after POST", func() {
		const name = "test-pending-post"
		nn := types.NamespacedName{Name: name, Namespace: "default"}
		var testServer *httptest.Server
		var origEndpoint string

		BeforeEach(func() {
			origEndpoint = os.Getenv("OBSERVER_INTERNAL_ENDPOINT")

			testServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				defer GinkgoRecover()
				if r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/rules/"+name) {
					w.WriteHeader(http.StatusNotFound)
					return
				}
				if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/rules") {
					resp := alertRuleSyncResponse{
						Status:        "pending",
						Action:        "created",
						RuleLogicalID: name,
						RuleBackendID: "backend-id-pending",
					}
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusCreated)
					_ = json.NewEncoder(w).Encode(resp)
					return
				}
				w.WriteHeader(http.StatusNotFound)
			}))
			os.Setenv("OBSERVER_INTERNAL_ENDPOINT", testServer.URL)

			rule := newAlertRule(name, defaultLabels())
			controllerutil.AddFinalizer(rule, AlertRuleCleanupFinalizer)
			Expect(k8sClient.Create(ctx, rule)).To(Succeed())
		})

		AfterEach(func() {
			if origEndpoint != "" {
				os.Setenv("OBSERVER_INTERNAL_ENDPOINT", origEndpoint)
			} else {
				os.Unsetenv("OBSERVER_INTERNAL_ENDPOINT")
			}
			testServer.Close()
			forceDeleteAlertRule(nn)
		})

		It("should set Phase=Error and condition reason SyncNotSynced", func() {
			r := newReconciler()
			result, err := r.Reconcile(ctx, reconcileRequest(name))
			// The non-synced path does NOT call updateStatusWithError; it writes status inline and returns nil error.
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Requeue).To(BeFalse())

			fetched := &openchoreov1alpha1.ObservabilityAlertRule{}
			Expect(k8sClient.Get(ctx, nn, fetched)).To(Succeed())
			Expect(fetched.Status.Phase).To(Equal(openchoreov1alpha1.ObservabilityAlertRulePhaseError))

			cond := apimeta.FindStatusCondition(fetched.Status.Conditions, conditionTypeSynced)
			Expect(cond).NotTo(BeNil())
			Expect(cond.Status).To(Equal(metav1.ConditionFalse))
			Expect(cond.Reason).To(Equal("SyncNotSynced"))
			Expect(cond.Message).To(ContainSubstring(`"pending"`))
		})
	})

	// -----------------------------------------------------------------------
	// Sync: observer returns non-synced status (PUT path)
	// -----------------------------------------------------------------------
	Context("When the observer reports a non-synced status after PUT", func() {
		const name = "test-pending-put"
		nn := types.NamespacedName{Name: name, Namespace: "default"}
		var testServer *httptest.Server
		var origEndpoint string

		BeforeEach(func() {
			origEndpoint = os.Getenv("OBSERVER_INTERNAL_ENDPOINT")

			testServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				defer GinkgoRecover()
				if r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/rules/"+name) {
					getResp := alertRuleGetResponse{RuleLogicalID: name, RuleBackendID: "existing-id"}
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusOK)
					_ = json.NewEncoder(w).Encode(getResp)
					return
				}
				if r.Method == http.MethodPut && strings.Contains(r.URL.Path, "/rules/"+name) {
					resp := alertRuleSyncResponse{
						Status:        "pending",
						Action:        "updated",
						RuleLogicalID: name,
						RuleBackendID: "existing-id",
					}
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusOK)
					_ = json.NewEncoder(w).Encode(resp)
					return
				}
				w.WriteHeader(http.StatusNotFound)
			}))
			os.Setenv("OBSERVER_INTERNAL_ENDPOINT", testServer.URL)

			rule := newAlertRule(name, defaultLabels())
			controllerutil.AddFinalizer(rule, AlertRuleCleanupFinalizer)
			Expect(k8sClient.Create(ctx, rule)).To(Succeed())
		})

		AfterEach(func() {
			if origEndpoint != "" {
				os.Setenv("OBSERVER_INTERNAL_ENDPOINT", origEndpoint)
			} else {
				os.Unsetenv("OBSERVER_INTERNAL_ENDPOINT")
			}
			testServer.Close()
			forceDeleteAlertRule(nn)
		})

		It("should set Phase=Error and condition reason SyncNotSynced", func() {
			r := newReconciler()
			_, err := r.Reconcile(ctx, reconcileRequest(name))
			Expect(err).NotTo(HaveOccurred())

			fetched := &openchoreov1alpha1.ObservabilityAlertRule{}
			Expect(k8sClient.Get(ctx, nn, fetched)).To(Succeed())
			Expect(fetched.Status.Phase).To(Equal(openchoreov1alpha1.ObservabilityAlertRulePhaseError))

			cond := apimeta.FindStatusCondition(fetched.Status.Conditions, conditionTypeSynced)
			Expect(cond).NotTo(BeNil())
			Expect(cond.Status).To(Equal(metav1.ConditionFalse))
			Expect(cond.Reason).To(Equal("SyncNotSynced"))
			Expect(cond.Message).To(ContainSubstring(`"pending"`))
		})
	})

	// -----------------------------------------------------------------------
	// Sync error: GET returns 200 with invalid JSON (decode error)
	// -----------------------------------------------------------------------
	Context("When the observer GET returns 200 with invalid JSON", func() {
		const name = "test-get-invalid-json"
		nn := types.NamespacedName{Name: name, Namespace: "default"}
		var testServer *httptest.Server
		var origEndpoint string

		BeforeEach(func() {
			origEndpoint = os.Getenv("OBSERVER_INTERNAL_ENDPOINT")

			testServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				defer GinkgoRecover()
				if r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/rules/"+name) {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusOK)
					// Deliberately malformed JSON to trigger the decode-error branch in getAlertRule.
					_, _ = w.Write([]byte(`not-valid-json`))
					return
				}
				w.WriteHeader(http.StatusNotFound)
			}))
			os.Setenv("OBSERVER_INTERNAL_ENDPOINT", testServer.URL)

			rule := newAlertRule(name, defaultLabels())
			controllerutil.AddFinalizer(rule, AlertRuleCleanupFinalizer)
			Expect(k8sClient.Create(ctx, rule)).To(Succeed())
		})

		AfterEach(func() {
			if origEndpoint != "" {
				os.Setenv("OBSERVER_INTERNAL_ENDPOINT", origEndpoint)
			} else {
				os.Unsetenv("OBSERVER_INTERNAL_ENDPOINT")
			}
			testServer.Close()
			forceDeleteAlertRule(nn)
		})

		It("should set Error status and return an error about the decode failure", func() {
			r := newReconciler()
			_, err := r.Reconcile(ctx, reconcileRequest(name))
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to GET alert rule"))
			Expect(err.Error()).To(ContainSubstring("failed to decode GET response"))

			fetched := &openchoreov1alpha1.ObservabilityAlertRule{}
			Expect(k8sClient.Get(ctx, nn, fetched)).To(Succeed())
			Expect(fetched.Status.Phase).To(Equal(openchoreov1alpha1.ObservabilityAlertRulePhaseError))

			cond := apimeta.FindStatusCondition(fetched.Status.Conditions, conditionTypeSynced)
			Expect(cond).NotTo(BeNil())
			Expect(cond.Status).To(Equal(metav1.ConditionFalse))
			Expect(cond.Reason).To(Equal("SyncFailed"))
			Expect(cond.Message).To(ContainSubstring("failed to decode GET response"))
		})
	})

	// -----------------------------------------------------------------------
	// Sync error: POST returns 409 Conflict
	// -----------------------------------------------------------------------
	Context("When the observer returns 409 Conflict on POST", func() {
		const name = "test-post-409"
		nn := types.NamespacedName{Name: name, Namespace: "default"}
		var testServer *httptest.Server
		var origEndpoint string

		BeforeEach(func() {
			origEndpoint = os.Getenv("OBSERVER_INTERNAL_ENDPOINT")

			testServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				defer GinkgoRecover()
				if r.Method == http.MethodGet {
					w.WriteHeader(http.StatusNotFound)
					return
				}
				if r.Method == http.MethodPost {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusConflict)
					_, _ = w.Write([]byte(`{"message":"rule already exists"}`))
					return
				}
				w.WriteHeader(http.StatusNotFound)
			}))
			os.Setenv("OBSERVER_INTERNAL_ENDPOINT", testServer.URL)

			rule := newAlertRule(name, defaultLabels())
			controllerutil.AddFinalizer(rule, AlertRuleCleanupFinalizer)
			Expect(k8sClient.Create(ctx, rule)).To(Succeed())
		})

		AfterEach(func() {
			if origEndpoint != "" {
				os.Setenv("OBSERVER_INTERNAL_ENDPOINT", origEndpoint)
			} else {
				os.Unsetenv("OBSERVER_INTERNAL_ENDPOINT")
			}
			testServer.Close()
			forceDeleteAlertRule(nn)
		})

		It("should set Error status and return an error mentioning unexpected status 409", func() {
			r := newReconciler()
			_, err := r.Reconcile(ctx, reconcileRequest(name))
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("409"))

			fetched := &openchoreov1alpha1.ObservabilityAlertRule{}
			Expect(k8sClient.Get(ctx, nn, fetched)).To(Succeed())
			Expect(fetched.Status.Phase).To(Equal(openchoreov1alpha1.ObservabilityAlertRulePhaseError))

			cond := apimeta.FindStatusCondition(fetched.Status.Conditions, conditionTypeSynced)
			Expect(cond).NotTo(BeNil())
			Expect(cond.Status).To(Equal(metav1.ConditionFalse))
			Expect(cond.Reason).To(Equal("SyncFailed"))
		})
	})

	// -----------------------------------------------------------------------
	// Sync error: POST returns 500
	// -----------------------------------------------------------------------
	Context("When the observer returns 500 on POST", func() {
		const name = "test-post-500"
		nn := types.NamespacedName{Name: name, Namespace: "default"}
		var testServer *httptest.Server
		var origEndpoint string

		BeforeEach(func() {
			origEndpoint = os.Getenv("OBSERVER_INTERNAL_ENDPOINT")

			testServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				defer GinkgoRecover()
				if r.Method == http.MethodGet {
					w.WriteHeader(http.StatusNotFound)
					return
				}
				if r.Method == http.MethodPost {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusInternalServerError)
					_, _ = w.Write([]byte(`{"message":"internal server error"}`))
					return
				}
				w.WriteHeader(http.StatusNotFound)
			}))
			os.Setenv("OBSERVER_INTERNAL_ENDPOINT", testServer.URL)

			rule := newAlertRule(name, defaultLabels())
			controllerutil.AddFinalizer(rule, AlertRuleCleanupFinalizer)
			Expect(k8sClient.Create(ctx, rule)).To(Succeed())
		})

		AfterEach(func() {
			if origEndpoint != "" {
				os.Setenv("OBSERVER_INTERNAL_ENDPOINT", origEndpoint)
			} else {
				os.Unsetenv("OBSERVER_INTERNAL_ENDPOINT")
			}
			testServer.Close()
			forceDeleteAlertRule(nn)
		})

		It("should set Error status and return an error containing status 500", func() {
			r := newReconciler()
			_, err := r.Reconcile(ctx, reconcileRequest(name))
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("500"))

			fetched := &openchoreov1alpha1.ObservabilityAlertRule{}
			Expect(k8sClient.Get(ctx, nn, fetched)).To(Succeed())
			Expect(fetched.Status.Phase).To(Equal(openchoreov1alpha1.ObservabilityAlertRulePhaseError))

			cond := apimeta.FindStatusCondition(fetched.Status.Conditions, conditionTypeSynced)
			Expect(cond).NotTo(BeNil())
			Expect(cond.Status).To(Equal(metav1.ConditionFalse))
			Expect(cond.Reason).To(Equal("SyncFailed"))
			Expect(cond.Message).To(ContainSubstring("500"))
		})
	})

	// -----------------------------------------------------------------------
	// Sync error: PUT returns 409 Conflict
	// -----------------------------------------------------------------------
	Context("When the observer returns 409 Conflict on PUT", func() {
		const name = "test-put-409"
		nn := types.NamespacedName{Name: name, Namespace: "default"}
		var testServer *httptest.Server
		var origEndpoint string

		BeforeEach(func() {
			origEndpoint = os.Getenv("OBSERVER_INTERNAL_ENDPOINT")

			testServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				defer GinkgoRecover()
				if r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/rules/"+name) {
					getResp := alertRuleGetResponse{RuleLogicalID: name, RuleBackendID: "existing-id"}
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusOK)
					_ = json.NewEncoder(w).Encode(getResp)
					return
				}
				if r.Method == http.MethodPut {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusConflict)
					_, _ = w.Write([]byte(`{"message":"optimistic lock conflict"}`))
					return
				}
				w.WriteHeader(http.StatusNotFound)
			}))
			os.Setenv("OBSERVER_INTERNAL_ENDPOINT", testServer.URL)

			rule := newAlertRule(name, defaultLabels())
			controllerutil.AddFinalizer(rule, AlertRuleCleanupFinalizer)
			Expect(k8sClient.Create(ctx, rule)).To(Succeed())
		})

		AfterEach(func() {
			if origEndpoint != "" {
				os.Setenv("OBSERVER_INTERNAL_ENDPOINT", origEndpoint)
			} else {
				os.Unsetenv("OBSERVER_INTERNAL_ENDPOINT")
			}
			testServer.Close()
			forceDeleteAlertRule(nn)
		})

		It("should set Error status and return an error mentioning unexpected status 409", func() {
			r := newReconciler()
			_, err := r.Reconcile(ctx, reconcileRequest(name))
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("409"))

			fetched := &openchoreov1alpha1.ObservabilityAlertRule{}
			Expect(k8sClient.Get(ctx, nn, fetched)).To(Succeed())
			Expect(fetched.Status.Phase).To(Equal(openchoreov1alpha1.ObservabilityAlertRulePhaseError))

			cond := apimeta.FindStatusCondition(fetched.Status.Conditions, conditionTypeSynced)
			Expect(cond).NotTo(BeNil())
			Expect(cond.Status).To(Equal(metav1.ConditionFalse))
			Expect(cond.Reason).To(Equal("SyncFailed"))
		})
	})

	// -----------------------------------------------------------------------
	// Sync error: PUT returns 500
	// -----------------------------------------------------------------------
	Context("When the observer returns 500 on PUT", func() {
		const name = "test-put-500"
		nn := types.NamespacedName{Name: name, Namespace: "default"}
		var testServer *httptest.Server
		var origEndpoint string

		BeforeEach(func() {
			origEndpoint = os.Getenv("OBSERVER_INTERNAL_ENDPOINT")

			testServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				defer GinkgoRecover()
				if r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/rules/"+name) {
					getResp := alertRuleGetResponse{RuleLogicalID: name, RuleBackendID: "existing-id"}
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusOK)
					_ = json.NewEncoder(w).Encode(getResp)
					return
				}
				if r.Method == http.MethodPut {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusInternalServerError)
					_, _ = w.Write([]byte(`{"message":"backend failure"}`))
					return
				}
				w.WriteHeader(http.StatusNotFound)
			}))
			os.Setenv("OBSERVER_INTERNAL_ENDPOINT", testServer.URL)

			rule := newAlertRule(name, defaultLabels())
			controllerutil.AddFinalizer(rule, AlertRuleCleanupFinalizer)
			Expect(k8sClient.Create(ctx, rule)).To(Succeed())
		})

		AfterEach(func() {
			if origEndpoint != "" {
				os.Setenv("OBSERVER_INTERNAL_ENDPOINT", origEndpoint)
			} else {
				os.Unsetenv("OBSERVER_INTERNAL_ENDPOINT")
			}
			testServer.Close()
			forceDeleteAlertRule(nn)
		})

		It("should set Error status and return an error containing status 500", func() {
			r := newReconciler()
			_, err := r.Reconcile(ctx, reconcileRequest(name))
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("500"))

			fetched := &openchoreov1alpha1.ObservabilityAlertRule{}
			Expect(k8sClient.Get(ctx, nn, fetched)).To(Succeed())
			Expect(fetched.Status.Phase).To(Equal(openchoreov1alpha1.ObservabilityAlertRulePhaseError))

			cond := apimeta.FindStatusCondition(fetched.Status.Conditions, conditionTypeSynced)
			Expect(cond).NotTo(BeNil())
			Expect(cond.Status).To(Equal(metav1.ConditionFalse))
			Expect(cond.Reason).To(Equal("SyncFailed"))
		})
	})

	// -----------------------------------------------------------------------
	// Finalization: resource has no AlertRuleCleanupFinalizer (early return)
	// -----------------------------------------------------------------------
	Context("When finalize() is called but the resource has no alertrule-cleanup finalizer", func() {
		const name = "test-finalize-no-finalizer"
		nn := types.NamespacedName{Name: name, Namespace: "default"}
		var requestCount int
		var testServer *httptest.Server
		var origEndpoint string

		BeforeEach(func() {
			origEndpoint = os.Getenv("OBSERVER_INTERNAL_ENDPOINT")
			requestCount = 0

			// Count any HTTP calls to confirm none are made.
			testServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				defer GinkgoRecover()
				requestCount++
				w.WriteHeader(http.StatusNoContent)
			}))
			os.Setenv("OBSERVER_INTERNAL_ENDPOINT", testServer.URL)

			rule := newAlertRule(name, defaultLabels())
			// Use a sentinel finalizer (not AlertRuleCleanupFinalizer) so that Delete sets
			// DeletionTimestamp while keeping the object alive — allowing Reconcile to reach finalize().
			rule.Finalizers = []string{"test.dev/sentinel"}
			Expect(k8sClient.Create(ctx, rule)).To(Succeed())
		})

		AfterEach(func() {
			if origEndpoint != "" {
				os.Setenv("OBSERVER_INTERNAL_ENDPOINT", origEndpoint)
			} else {
				os.Unsetenv("OBSERVER_INTERNAL_ENDPOINT")
			}
			testServer.Close()
			// Best-effort cleanup: clear all finalizers (including the sentinel) before deleting.
			rule := &openchoreov1alpha1.ObservabilityAlertRule{}
			if err := k8sClient.Get(ctx, nn, rule); err == nil {
				rule.Finalizers = nil
				_ = k8sClient.Update(ctx, rule)
				_ = k8sClient.Delete(ctx, rule)
			}
		})

		It("should return immediately without making any HTTP calls to the observer", func() {
			By("Deleting the resource — sentinel finalizer keeps it alive with a DeletionTimestamp")
			rule := &openchoreov1alpha1.ObservabilityAlertRule{}
			Expect(k8sClient.Get(ctx, nn, rule)).To(Succeed())
			Expect(k8sClient.Delete(ctx, rule)).To(Succeed())

			By("Verifying DeletionTimestamp is set")
			updated := &openchoreov1alpha1.ObservabilityAlertRule{}
			Expect(k8sClient.Get(ctx, nn, updated)).To(Succeed())
			Expect(updated.DeletionTimestamp).NotTo(BeNil())

			By("Reconciling — finalize() should return early since AlertRuleCleanupFinalizer is absent")
			r := newReconciler()
			result, err := r.Reconcile(ctx, reconcileRequest(name))
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Requeue).To(BeFalse())

			By("Verifying no HTTP requests were made to the observer backend")
			Expect(requestCount).To(Equal(0))
		})
	})

	// -----------------------------------------------------------------------
	// Finalization: DELETE succeeds (204)
	// -----------------------------------------------------------------------
	Context("When finalizing a deleted resource (backend returns 204)", func() {
		const name = "test-finalize-204"
		nn := types.NamespacedName{Name: name, Namespace: "default"}
		var testServer *httptest.Server
		var origEndpoint string

		BeforeEach(func() {
			origEndpoint = os.Getenv("OBSERVER_INTERNAL_ENDPOINT")

			testServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodDelete {
					w.WriteHeader(http.StatusNoContent)
					return
				}
				w.WriteHeader(http.StatusNotFound)
			}))
			os.Setenv("OBSERVER_INTERNAL_ENDPOINT", testServer.URL)

			rule := newAlertRule(name, defaultLabels())
			controllerutil.AddFinalizer(rule, AlertRuleCleanupFinalizer)
			Expect(k8sClient.Create(ctx, rule)).To(Succeed())
		})

		AfterEach(func() {
			if origEndpoint != "" {
				os.Setenv("OBSERVER_INTERNAL_ENDPOINT", origEndpoint)
			} else {
				os.Unsetenv("OBSERVER_INTERNAL_ENDPOINT")
			}
			if testServer != nil {
				testServer.Close()
			}
			forceDeleteAlertRule(nn)
		})

		It("should remove finalizer and allow resource deletion", func() {
			By("Deleting the resource to trigger finalization")
			rule := &openchoreov1alpha1.ObservabilityAlertRule{}
			Expect(k8sClient.Get(ctx, nn, rule)).To(Succeed())
			Expect(k8sClient.Delete(ctx, rule)).To(Succeed())

			By("Reconciling to trigger finalize()")
			r := newReconciler()
			result, err := r.Reconcile(ctx, reconcileRequest(name))
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Requeue).To(BeFalse())

			By("Verifying the resource is eventually gone")
			Eventually(func() bool {
				err := k8sClient.Get(ctx, nn, &openchoreov1alpha1.ObservabilityAlertRule{})
				return err != nil
			}, "5s", "250ms").Should(BeTrue())
		})
	})

	// -----------------------------------------------------------------------
	// Finalization: DELETE succeeds (200 OK)
	// -----------------------------------------------------------------------
	Context("When finalizing a deleted resource (backend returns 200 OK)", func() {
		const name = "test-finalize-200"
		nn := types.NamespacedName{Name: name, Namespace: "default"}
		var testServer *httptest.Server
		var origEndpoint string

		BeforeEach(func() {
			origEndpoint = os.Getenv("OBSERVER_INTERNAL_ENDPOINT")

			testServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				defer GinkgoRecover()
				if r.Method == http.MethodDelete {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusOK)
					_, _ = w.Write([]byte(`{"message":"deleted"}`))
					return
				}
				w.WriteHeader(http.StatusNotFound)
			}))
			os.Setenv("OBSERVER_INTERNAL_ENDPOINT", testServer.URL)

			rule := newAlertRule(name, defaultLabels())
			controllerutil.AddFinalizer(rule, AlertRuleCleanupFinalizer)
			Expect(k8sClient.Create(ctx, rule)).To(Succeed())
		})

		AfterEach(func() {
			if origEndpoint != "" {
				os.Setenv("OBSERVER_INTERNAL_ENDPOINT", origEndpoint)
			} else {
				os.Unsetenv("OBSERVER_INTERNAL_ENDPOINT")
			}
			testServer.Close()
			forceDeleteAlertRule(nn)
		})

		It("should treat 200 as success, remove finalizer, and allow resource deletion", func() {
			By("Deleting the resource to trigger finalization")
			rule := &openchoreov1alpha1.ObservabilityAlertRule{}
			Expect(k8sClient.Get(ctx, nn, rule)).To(Succeed())
			Expect(k8sClient.Delete(ctx, rule)).To(Succeed())

			By("Reconciling to trigger finalize()")
			r := newReconciler()
			result, err := r.Reconcile(ctx, reconcileRequest(name))
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Requeue).To(BeFalse())

			By("Verifying the resource is eventually gone")
			Eventually(func() bool {
				err := k8sClient.Get(ctx, nn, &openchoreov1alpha1.ObservabilityAlertRule{})
				return err != nil
			}, "5s", "250ms").Should(BeTrue())
		})
	})

	// -----------------------------------------------------------------------
	// Finalization: backend returns 404 (already deleted)
	// -----------------------------------------------------------------------
	Context("When finalizing a deleted resource (backend returns 404)", func() {
		const name = "test-finalize-404"
		nn := types.NamespacedName{Name: name, Namespace: "default"}
		var testServer *httptest.Server
		var origEndpoint string

		BeforeEach(func() {
			origEndpoint = os.Getenv("OBSERVER_INTERNAL_ENDPOINT")

			testServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// 404 means already deleted — controller should still succeed
				w.WriteHeader(http.StatusNotFound)
			}))
			os.Setenv("OBSERVER_INTERNAL_ENDPOINT", testServer.URL)

			rule := newAlertRule(name, defaultLabels())
			controllerutil.AddFinalizer(rule, AlertRuleCleanupFinalizer)
			Expect(k8sClient.Create(ctx, rule)).To(Succeed())
		})

		AfterEach(func() {
			if origEndpoint != "" {
				os.Setenv("OBSERVER_INTERNAL_ENDPOINT", origEndpoint)
			} else {
				os.Unsetenv("OBSERVER_INTERNAL_ENDPOINT")
			}
			if testServer != nil {
				testServer.Close()
			}
			forceDeleteAlertRule(nn)
		})

		It("should succeed and remove finalizer when backend already deleted rule", func() {
			By("Deleting the resource")
			rule := &openchoreov1alpha1.ObservabilityAlertRule{}
			Expect(k8sClient.Get(ctx, nn, rule)).To(Succeed())
			Expect(k8sClient.Delete(ctx, rule)).To(Succeed())

			By("Reconciling")
			r := newReconciler()
			_, err := r.Reconcile(ctx, reconcileRequest(name))
			Expect(err).NotTo(HaveOccurred())

			By("Resource should be gone")
			Eventually(func() bool {
				err := k8sClient.Get(ctx, nn, &openchoreov1alpha1.ObservabilityAlertRule{})
				return err != nil
			}, "5s", "250ms").Should(BeTrue())
		})
	})

	// -----------------------------------------------------------------------
	// Finalization: backend returns 500 (error)
	// -----------------------------------------------------------------------
	Context("When finalizing a deleted resource (backend returns 500)", func() {
		const name = "test-finalize-500"
		nn := types.NamespacedName{Name: name, Namespace: "default"}
		var testServer *httptest.Server
		var origEndpoint string

		BeforeEach(func() {
			origEndpoint = os.Getenv("OBSERVER_INTERNAL_ENDPOINT")

			testServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte(`{"message":"internal error"}`))
			}))
			os.Setenv("OBSERVER_INTERNAL_ENDPOINT", testServer.URL)

			rule := newAlertRule(name, defaultLabels())
			controllerutil.AddFinalizer(rule, AlertRuleCleanupFinalizer)
			Expect(k8sClient.Create(ctx, rule)).To(Succeed())
		})

		AfterEach(func() {
			if origEndpoint != "" {
				os.Setenv("OBSERVER_INTERNAL_ENDPOINT", origEndpoint)
			} else {
				os.Unsetenv("OBSERVER_INTERNAL_ENDPOINT")
			}
			if testServer != nil {
				testServer.Close()
			}
			forceDeleteAlertRule(nn)
		})

		It("should return an error and keep the finalizer", func() {
			By("Deleting the resource")
			rule := &openchoreov1alpha1.ObservabilityAlertRule{}
			Expect(k8sClient.Get(ctx, nn, rule)).To(Succeed())
			Expect(k8sClient.Delete(ctx, rule)).To(Succeed())

			By("Reconciling")
			r := newReconciler()
			_, err := r.Reconcile(ctx, reconcileRequest(name))
			Expect(err).To(HaveOccurred())

			By("Verifying the finalizer is still present")
			fetched := &openchoreov1alpha1.ObservabilityAlertRule{}
			Expect(k8sClient.Get(ctx, nn, fetched)).To(Succeed())
			Expect(controllerutil.ContainsFinalizer(fetched, AlertRuleCleanupFinalizer)).To(BeTrue())
		})
	})

	// -----------------------------------------------------------------------
	// Network error: upsert fails when observer is unreachable (GET)
	// -----------------------------------------------------------------------
	Context("When the observer backend is unreachable during upsert (GET fails)", func() {
		const name = "test-network-error-get"
		nn := types.NamespacedName{Name: name, Namespace: "default"}
		var origEndpoint string

		BeforeEach(func() {
			origEndpoint = os.Getenv("OBSERVER_INTERNAL_ENDPOINT")

			// Start a server and immediately close it to simulate connection refused.
			closedServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
			closedServer.Close()
			os.Setenv("OBSERVER_INTERNAL_ENDPOINT", closedServer.URL)

			rule := newAlertRule(name, defaultLabels())
			controllerutil.AddFinalizer(rule, AlertRuleCleanupFinalizer)
			Expect(k8sClient.Create(ctx, rule)).To(Succeed())
		})

		AfterEach(func() {
			if origEndpoint != "" {
				os.Setenv("OBSERVER_INTERNAL_ENDPOINT", origEndpoint)
			} else {
				os.Unsetenv("OBSERVER_INTERNAL_ENDPOINT")
			}
			forceDeleteAlertRule(nn)
		})

		It("should set Error status and return an error when observer is unreachable", func() {
			r := newReconciler()
			_, err := r.Reconcile(ctx, reconcileRequest(name))
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("GET request failed"))

			fetched := &openchoreov1alpha1.ObservabilityAlertRule{}
			Expect(k8sClient.Get(ctx, nn, fetched)).To(Succeed())
			Expect(fetched.Status.Phase).To(Equal(openchoreov1alpha1.ObservabilityAlertRulePhaseError))
			cond := apimeta.FindStatusCondition(fetched.Status.Conditions, conditionTypeSynced)
			Expect(cond).NotTo(BeNil())
			Expect(cond.Status).To(Equal(metav1.ConditionFalse))
			Expect(cond.Reason).To(Equal("SyncFailed"))
			Expect(cond.Message).To(ContainSubstring("GET request failed"))
		})
	})

	// -----------------------------------------------------------------------
	// Network error: upsert fails when POST connection is dropped
	// -----------------------------------------------------------------------
	Context("When the observer backend closes the connection during POST", func() {
		const name = "test-network-error-post"
		nn := types.NamespacedName{Name: name, Namespace: "default"}
		var testServer *httptest.Server
		var origEndpoint string

		BeforeEach(func() {
			origEndpoint = os.Getenv("OBSERVER_INTERNAL_ENDPOINT")

			testServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				defer GinkgoRecover()
				if r.Method == http.MethodGet {
					// Rule not found — controller will proceed to POST.
					w.WriteHeader(http.StatusNotFound)
					return
				}
				// Abruptly close the connection on POST to simulate a network error.
				hj, ok := w.(http.Hijacker)
				Expect(ok).To(BeTrue())
				conn, _, err := hj.Hijack()
				Expect(err).NotTo(HaveOccurred())
				conn.Close()
			}))
			os.Setenv("OBSERVER_INTERNAL_ENDPOINT", testServer.URL)

			rule := newAlertRule(name, defaultLabels())
			controllerutil.AddFinalizer(rule, AlertRuleCleanupFinalizer)
			Expect(k8sClient.Create(ctx, rule)).To(Succeed())
		})

		AfterEach(func() {
			if origEndpoint != "" {
				os.Setenv("OBSERVER_INTERNAL_ENDPOINT", origEndpoint)
			} else {
				os.Unsetenv("OBSERVER_INTERNAL_ENDPOINT")
			}
			if testServer != nil {
				testServer.Close()
			}
			forceDeleteAlertRule(nn)
		})

		It("should set Error status and return an error when POST connection is dropped", func() {
			r := newReconciler()
			_, err := r.Reconcile(ctx, reconcileRequest(name))
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("POST request failed"))

			fetched := &openchoreov1alpha1.ObservabilityAlertRule{}
			Expect(k8sClient.Get(ctx, nn, fetched)).To(Succeed())
			Expect(fetched.Status.Phase).To(Equal(openchoreov1alpha1.ObservabilityAlertRulePhaseError))
			cond := apimeta.FindStatusCondition(fetched.Status.Conditions, conditionTypeSynced)
			Expect(cond).NotTo(BeNil())
			Expect(cond.Status).To(Equal(metav1.ConditionFalse))
			Expect(cond.Reason).To(Equal("SyncFailed"))
			Expect(cond.Message).To(ContainSubstring("POST request failed"))
		})
	})

	// -----------------------------------------------------------------------
	// Network error: upsert fails when PUT connection is dropped
	// -----------------------------------------------------------------------
	Context("When the observer backend closes the connection during PUT", func() {
		const name = "test-network-error-put"
		nn := types.NamespacedName{Name: name, Namespace: "default"}
		var testServer *httptest.Server
		var origEndpoint string

		BeforeEach(func() {
			origEndpoint = os.Getenv("OBSERVER_INTERNAL_ENDPOINT")

			testServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				defer GinkgoRecover()
				if r.Method == http.MethodGet {
					// Rule exists — controller will proceed to PUT.
					resp := alertRuleGetResponse{
						RuleLogicalID: name,
						RuleBackendID: "existing-backend-id",
					}
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusOK)
					_ = json.NewEncoder(w).Encode(resp)
					return
				}
				// Abruptly close the connection on PUT to simulate a network error.
				hj, ok := w.(http.Hijacker)
				Expect(ok).To(BeTrue())
				conn, _, err := hj.Hijack()
				Expect(err).NotTo(HaveOccurred())
				conn.Close()
			}))
			os.Setenv("OBSERVER_INTERNAL_ENDPOINT", testServer.URL)

			rule := newAlertRule(name, defaultLabels())
			controllerutil.AddFinalizer(rule, AlertRuleCleanupFinalizer)
			Expect(k8sClient.Create(ctx, rule)).To(Succeed())
		})

		AfterEach(func() {
			if origEndpoint != "" {
				os.Setenv("OBSERVER_INTERNAL_ENDPOINT", origEndpoint)
			} else {
				os.Unsetenv("OBSERVER_INTERNAL_ENDPOINT")
			}
			if testServer != nil {
				testServer.Close()
			}
			forceDeleteAlertRule(nn)
		})

		It("should set Error status and return an error when PUT connection is dropped", func() {
			r := newReconciler()
			_, err := r.Reconcile(ctx, reconcileRequest(name))
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("PUT request failed"))

			fetched := &openchoreov1alpha1.ObservabilityAlertRule{}
			Expect(k8sClient.Get(ctx, nn, fetched)).To(Succeed())
			Expect(fetched.Status.Phase).To(Equal(openchoreov1alpha1.ObservabilityAlertRulePhaseError))
			cond := apimeta.FindStatusCondition(fetched.Status.Conditions, conditionTypeSynced)
			Expect(cond).NotTo(BeNil())
			Expect(cond.Status).To(Equal(metav1.ConditionFalse))
			Expect(cond.Reason).To(Equal("SyncFailed"))
			Expect(cond.Message).To(ContainSubstring("PUT request failed"))
		})
	})

	// -----------------------------------------------------------------------
	// Network error: finalization fails when observer is unreachable (DELETE)
	// -----------------------------------------------------------------------
	Context("When the observer backend is unreachable during finalization (DELETE fails)", func() {
		const name = "test-network-error-delete"
		nn := types.NamespacedName{Name: name, Namespace: "default"}
		var origEndpoint string

		BeforeEach(func() {
			origEndpoint = os.Getenv("OBSERVER_INTERNAL_ENDPOINT")

			// Start a server and immediately close it to simulate connection refused.
			closedServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
			closedServer.Close()
			os.Setenv("OBSERVER_INTERNAL_ENDPOINT", closedServer.URL)

			rule := newAlertRule(name, defaultLabels())
			controllerutil.AddFinalizer(rule, AlertRuleCleanupFinalizer)
			Expect(k8sClient.Create(ctx, rule)).To(Succeed())
		})

		AfterEach(func() {
			if origEndpoint != "" {
				os.Setenv("OBSERVER_INTERNAL_ENDPOINT", origEndpoint)
			} else {
				os.Unsetenv("OBSERVER_INTERNAL_ENDPOINT")
			}
			forceDeleteAlertRule(nn)
		})

		It("should return an error and keep the finalizer when the DELETE call fails", func() {
			By("Triggering deletion")
			rule := &openchoreov1alpha1.ObservabilityAlertRule{}
			Expect(k8sClient.Get(ctx, nn, rule)).To(Succeed())
			Expect(k8sClient.Delete(ctx, rule)).To(Succeed())

			By("Reconciling — finalize() should fail on the DELETE HTTP call")
			r := newReconciler()
			_, err := r.Reconcile(ctx, reconcileRequest(name))
			Expect(err).To(HaveOccurred())

			By("Verifying the finalizer is still present")
			fetched := &openchoreov1alpha1.ObservabilityAlertRule{}
			Expect(k8sClient.Get(ctx, nn, fetched)).To(Succeed())
			Expect(controllerutil.ContainsFinalizer(fetched, AlertRuleCleanupFinalizer)).To(BeTrue())
		})
	})

	// -----------------------------------------------------------------------
	// Status subresource persistence
	// -----------------------------------------------------------------------
	Context("Status subresource persistence", func() {
		const name = "test-status-persist"
		nn := types.NamespacedName{Name: name, Namespace: "default"}

		BeforeEach(func() {
			rule := newAlertRule(name, defaultLabels())
			Expect(k8sClient.Create(ctx, rule)).To(Succeed())
		})

		AfterEach(func() {
			forceDeleteAlertRule(nn)
		})

		It("should persist status updates via the status subresource", func() {
			rule := &openchoreov1alpha1.ObservabilityAlertRule{}
			Expect(k8sClient.Get(ctx, nn, rule)).To(Succeed())

			now := metav1.NewTime(time.Now())
			rule.Status.Phase = openchoreov1alpha1.ObservabilityAlertRulePhaseReady
			rule.Status.BackendMonitorID = "persisted-backend-id"
			rule.Status.LastReconcileTime = &now
			setStatusCondition(rule, metav1.ConditionTrue, "SyncSucceeded", "test persistence")
			Expect(k8sClient.Status().Update(ctx, rule)).To(Succeed())

			fetched := &openchoreov1alpha1.ObservabilityAlertRule{}
			Expect(k8sClient.Get(ctx, nn, fetched)).To(Succeed())
			Expect(fetched.Status.Phase).To(Equal(openchoreov1alpha1.ObservabilityAlertRulePhaseReady))
			Expect(fetched.Status.BackendMonitorID).To(Equal("persisted-backend-id"))
			Expect(fetched.Status.LastReconcileTime).NotTo(BeNil())

			cond := apimeta.FindStatusCondition(fetched.Status.Conditions, conditionTypeSynced)
			Expect(cond).NotTo(BeNil())
			Expect(cond.Status).To(Equal(metav1.ConditionTrue))
			Expect(cond.Reason).To(Equal("SyncSucceeded"))
		})
	})

	// -----------------------------------------------------------------------
	// Successful reconcile: observedGeneration is updated
	// -----------------------------------------------------------------------
	Context("When reconcile succeeds", func() {
		const name = "test-observed-gen"
		nn := types.NamespacedName{Name: name, Namespace: "default"}
		var testServer *httptest.Server
		var origEndpoint string

		BeforeEach(func() {
			origEndpoint = os.Getenv("OBSERVER_INTERNAL_ENDPOINT")

			testServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				defer GinkgoRecover()
				if r.Method == http.MethodGet {
					w.WriteHeader(http.StatusNotFound)
					return
				}
				if r.Method == http.MethodPost {
					resp := alertRuleSyncResponse{
						Status:        "synced",
						Action:        "created",
						RuleLogicalID: name,
						RuleBackendID: "gen-test-id",
						LastSyncedAt:  time.Now().UTC().Format(time.RFC3339),
					}
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusCreated)
					_ = json.NewEncoder(w).Encode(resp)
					return
				}
				w.WriteHeader(http.StatusNotFound)
			}))
			os.Setenv("OBSERVER_INTERNAL_ENDPOINT", testServer.URL)

			rule := newAlertRule(name, defaultLabels())
			controllerutil.AddFinalizer(rule, AlertRuleCleanupFinalizer)
			Expect(k8sClient.Create(ctx, rule)).To(Succeed())
		})

		AfterEach(func() {
			if origEndpoint != "" {
				os.Setenv("OBSERVER_INTERNAL_ENDPOINT", origEndpoint)
			} else {
				os.Unsetenv("OBSERVER_INTERNAL_ENDPOINT")
			}
			if testServer != nil {
				testServer.Close()
			}
			forceDeleteAlertRule(nn)
		})

		It("should update observedGeneration in status", func() {
			r := newReconciler()
			Expect(r.Reconcile(ctx, reconcileRequest(name))).Error().NotTo(HaveOccurred())

			fetched := &openchoreov1alpha1.ObservabilityAlertRule{}
			Expect(k8sClient.Get(ctx, nn, fetched)).To(Succeed())
			Expect(fetched.Status.ObservedGeneration).To(Equal(fetched.Generation))
		})
	})

	// -----------------------------------------------------------------------
	// CEL XValidation: AI cost analysis
	// -----------------------------------------------------------------------
	Context("CEL validation for AI cost analysis", func() {
		AfterEach(func() {
			forceDeleteAlertRule(types.NamespacedName{Name: "test-cel-cost-analysis-non-budget", Namespace: "default"})
			forceDeleteAlertRule(types.NamespacedName{Name: "test-cel-cost-analysis-incident-disabled", Namespace: "default"})
			forceDeleteAlertRule(types.NamespacedName{Name: "test-cel-cost-analysis-valid", Namespace: "default"})
		})

		It("should reject when TriggerAiCostAnalysis is true but Source.Type is not budget", func() {
			enabled := true
			triggerCostAnalysis := true
			rule := newAlertRule("test-cel-cost-analysis-non-budget", defaultLabels())
			rule.Spec.Source.Type = openchoreov1alpha1.ObservabilityAlertSourceTypeLog
			rule.Spec.Actions.Incident = &openchoreov1alpha1.ObservabilityAlertIncident{
				Enabled:               &enabled,
				TriggerAiCostAnalysis: &triggerCostAnalysis,
			}

			err := k8sClient.Create(ctx, rule)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("AI cost analysis can only be triggered for budget alerts"))
		})

		It("should reject when TriggerAiCostAnalysis is true but incident is not enabled", func() {
			disabled := false
			triggerCostAnalysis := true
			rule := newAlertRule("test-cel-cost-analysis-incident-disabled", defaultLabels())
			rule.Spec.Source.Type = openchoreov1alpha1.ObservabilityAlertSourceTypeBudget
			rule.Spec.Actions.Incident = &openchoreov1alpha1.ObservabilityAlertIncident{
				Enabled:               &disabled,
				TriggerAiCostAnalysis: &triggerCostAnalysis,
			}

			err := k8sClient.Create(ctx, rule)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("Incident must be enabled to trigger AI cost analysis"))
		})

		It("should accept when TriggerAiCostAnalysis is true with Source.Type=budget and incident enabled", func() {
			enabled := true
			triggerCostAnalysis := true
			rule := newAlertRule("test-cel-cost-analysis-valid", defaultLabels())
			rule.Spec.Source.Type = openchoreov1alpha1.ObservabilityAlertSourceTypeBudget
			rule.Spec.Actions.Incident = &openchoreov1alpha1.ObservabilityAlertIncident{
				Enabled:               &enabled,
				TriggerAiCostAnalysis: &triggerCostAnalysis,
			}

			err := k8sClient.Create(ctx, rule)
			Expect(err).NotTo(HaveOccurred())
		})
	})
})
