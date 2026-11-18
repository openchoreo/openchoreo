// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package observabilityalertrule

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

// fakeObserver records the alert rule calls it receives. GET answers getStatus, with a rule built
// from getNamespace's rule.
type fakeObserver struct {
	mu           sync.Mutex
	getStatus    int
	getNamespace string
	calls        map[string]int
	namespaces   []string
}

func newFakeObserver() (*fakeObserver, *httptest.Server) {
	f := &fakeObserver{getStatus: http.StatusNotFound, calls: map[string]int{}}
	return f, httptest.NewServer(http.HandlerFunc(f.serve))
}

func (f *fakeObserver) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls[r.Method]++
	if r.Method != http.MethodPost {
		f.namespaces = append(f.namespaces, r.URL.Query().Get("namespace"))
	}
	w.Header().Set("Content-Type", "application/json")

	switch r.Method {
	case http.MethodGet:
		w.WriteHeader(f.getStatus)
		if f.getStatus == http.StatusOK {
			resp := alertRuleGetResponse{}
			resp.Metadata.Namespace = f.getNamespace
			_ = json.NewEncoder(w).Encode(resp)
		}
	case http.MethodPost, http.MethodPut:
		status := http.StatusOK
		if r.Method == http.MethodPost {
			status = http.StatusCreated
		}
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(alertRuleSyncResponse{
			Status: "synced", Action: "created", RuleBackendID: "backend-id",
			LastSyncedAt: time.Now().UTC().Format(time.RFC3339),
		})
	case http.MethodDelete:
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(alertRuleSyncResponse{Status: "synced", Action: "deleted"})
	}
}

func (f *fakeObserver) set(getStatus int, getNamespace string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.getStatus, f.getNamespace = getStatus, getNamespace
}

func (f *fakeObserver) count(method string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls[method]
}

func (f *fakeObserver) seenNamespaces() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.namespaces...)
}

var _ = Describe("ObservabilityAlertRule namespace identity", func() {
	const name = "test-namespace-identity"
	nn := types.NamespacedName{Name: name, Namespace: "default"}
	var observer *fakeObserver
	var server *httptest.Server
	var origEndpoint string

	BeforeEach(func() {
		origEndpoint = os.Getenv("OBSERVER_INTERNAL_ENDPOINT")
		observer, server = newFakeObserver()
		os.Setenv("OBSERVER_INTERNAL_ENDPOINT", server.URL)

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
		server.Close()
		forceDeleteAlertRule(nn)
	})

	It("should send the rule's namespace and schedule a resync", func() {
		result, err := newReconciler().Reconcile(ctx, reconcileRequest(name))
		Expect(err).NotTo(HaveOccurred())
		Expect(result.RequeueAfter).To(BeNumerically(">=", resyncInterval))
		Expect(result.RequeueAfter).To(BeNumerically("<", resyncInterval+resyncJitter))

		Expect(observer.count(http.MethodPost)).To(Equal(1))
		Expect(observer.seenNamespaces()).To(ConsistOf("default"))
	})

	It("should not update a backend rule built from another namespace's rule", func() {
		observer.set(http.StatusOK, "other-ns")

		_, err := newReconciler().Reconcile(ctx, reconcileRequest(name))
		Expect(err).To(MatchError(errRuleOwnedByOtherNamespace))
		Expect(observer.count(http.MethodPut)).To(BeZero())

		fetched := &openchoreov1alpha1.ObservabilityAlertRule{}
		Expect(k8sClient.Get(ctx, nn, fetched)).To(Succeed())
		Expect(fetched.Status.Phase).To(Equal(openchoreov1alpha1.ObservabilityAlertRulePhaseError))
		cond := apimeta.FindStatusCondition(fetched.Status.Conditions, conditionTypeSynced)
		Expect(cond).NotTo(BeNil())
		Expect(cond.Status).To(Equal(metav1.ConditionFalse))
		Expect(cond.Message).To(ContainSubstring("other-ns"))
	})

	It("should only check the backend rule on a resync, and recreate it when missing", func() {
		r := newReconciler()
		_, err := r.Reconcile(ctx, reconcileRequest(name))
		Expect(err).NotTo(HaveOccurred())
		Expect(observer.count(http.MethodPost)).To(Equal(1))

		By("resyncing while the backend rule exists")
		observer.set(http.StatusOK, "default")
		result, err := r.Reconcile(ctx, reconcileRequest(name))
		Expect(err).NotTo(HaveOccurred())
		Expect(result.RequeueAfter).To(BeNumerically(">=", resyncInterval))
		Expect(observer.count(http.MethodPut)).To(BeZero())
		Expect(observer.count(http.MethodPost)).To(Equal(1))

		By("resyncing after the backend rule disappeared")
		observer.set(http.StatusNotFound, "")
		_, err = r.Reconcile(ctx, reconcileRequest(name))
		Expect(err).NotTo(HaveOccurred())
		Expect(observer.count(http.MethodPost)).To(Equal(2))
	})

	It("should retry a failed resync check without rewriting the backend rule", func() {
		r := newReconciler()
		_, err := r.Reconcile(ctx, reconcileRequest(name))
		Expect(err).NotTo(HaveOccurred())

		By("failing the resync check")
		observer.set(http.StatusInternalServerError, "")
		_, err = r.Reconcile(ctx, reconcileRequest(name))
		Expect(err).To(HaveOccurred())

		By("retrying the check after the backend recovers")
		observer.set(http.StatusOK, "default")
		result, err := r.Reconcile(ctx, reconcileRequest(name))
		Expect(err).NotTo(HaveOccurred())
		Expect(result.RequeueAfter).To(BeNumerically(">=", resyncInterval))
		Expect(observer.count(http.MethodPut)).To(BeZero())
		Expect(observer.count(http.MethodPost)).To(Equal(1))

		fetched := &openchoreov1alpha1.ObservabilityAlertRule{}
		Expect(k8sClient.Get(ctx, nn, fetched)).To(Succeed())
		Expect(fetched.Status.Phase).To(Equal(openchoreov1alpha1.ObservabilityAlertRulePhaseReady))
	})

	It("should update the backend rule on its first sync after a restart", func() {
		_, err := newReconciler().Reconcile(ctx, reconcileRequest(name))
		Expect(err).NotTo(HaveOccurred())

		observer.set(http.StatusOK, "default")
		_, err = newReconciler().Reconcile(ctx, reconcileRequest(name))
		Expect(err).NotTo(HaveOccurred())
		Expect(observer.count(http.MethodPut)).To(Equal(1))
	})

	It("should not delete a backend rule built from another namespace's rule", func() {
		observer.set(http.StatusOK, "other-ns")
		rule := &openchoreov1alpha1.ObservabilityAlertRule{}
		Expect(k8sClient.Get(ctx, nn, rule)).To(Succeed())
		Expect(k8sClient.Delete(ctx, rule)).To(Succeed())

		_, err := newReconciler().Reconcile(ctx, reconcileRequest(name))
		Expect(err).NotTo(HaveOccurred())
		Expect(observer.count(http.MethodDelete)).To(BeZero())
		Eventually(func() bool {
			return k8sClient.Get(ctx, nn, &openchoreov1alpha1.ObservabilityAlertRule{}) != nil
		}, "5s", "250ms").Should(BeTrue())
	})

	It("should not delete the backend rule when its ownership cannot be checked", func() {
		observer.set(http.StatusInternalServerError, "")
		rule := &openchoreov1alpha1.ObservabilityAlertRule{}
		Expect(k8sClient.Get(ctx, nn, rule)).To(Succeed())
		Expect(k8sClient.Delete(ctx, rule)).To(Succeed())

		_, err := newReconciler().Reconcile(ctx, reconcileRequest(name))
		Expect(err).To(HaveOccurred())
		Expect(observer.count(http.MethodDelete)).To(BeZero())

		fetched := &openchoreov1alpha1.ObservabilityAlertRule{}
		Expect(k8sClient.Get(ctx, nn, fetched)).To(Succeed())
		Expect(controllerutil.ContainsFinalizer(fetched, AlertRuleCleanupFinalizer)).To(BeTrue())
	})

	It("should delete its own backend rule with the rule's namespace", func() {
		observer.set(http.StatusOK, "default")
		rule := &openchoreov1alpha1.ObservabilityAlertRule{}
		Expect(k8sClient.Get(ctx, nn, rule)).To(Succeed())
		Expect(k8sClient.Delete(ctx, rule)).To(Succeed())

		_, err := newReconciler().Reconcile(ctx, reconcileRequest(name))
		Expect(err).NotTo(HaveOccurred())
		Expect(observer.count(http.MethodDelete)).To(Equal(1))
		Expect(observer.seenNamespaces()).To(HaveEach("default"))
	})
})
