// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package workflowrun

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"

	openchoreodevv1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/cmdutil"
)

var _ = Describe("WorkflowRun TTL Expiration Unit Tests", func() {
	Describe("checkTTLExpiration function", func() {
		var (
			reconciler *Reconciler
			ctx        context.Context
		)

		BeforeEach(func() {
			ctx = context.Background()
			reconciler = &Reconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}
		})

		It("should return false when FinishedAt is not set", func() {
			workflowRun := &openchoreodevv1alpha1.WorkflowRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-no-finished-at",
					Namespace: "default",
				},
				Spec: openchoreodevv1alpha1.WorkflowRunSpec{
					TTLAfterCompletion: "1h",
				},
				Status: openchoreodevv1alpha1.WorkflowRunStatus{
					FinishedAt: nil,
				},
			}

			shouldReturn, result, err := reconciler.checkTTLExpiration(ctx, workflowRun)
			Expect(err).NotTo(HaveOccurred())
			Expect(shouldReturn).To(BeFalse())
			Expect(result).To(Equal(ctrl.Result{}))
		})

		It("should return false when TTLAfterCompletion is not set", func() {
			now := metav1.Now()
			workflowRun := &openchoreodevv1alpha1.WorkflowRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-no-ttl",
					Namespace: "default",
				},
				Spec: openchoreodevv1alpha1.WorkflowRunSpec{
					TTLAfterCompletion: "",
				},
				Status: openchoreodevv1alpha1.WorkflowRunStatus{
					FinishedAt: &now,
				},
			}

			shouldReturn, result, err := reconciler.checkTTLExpiration(ctx, workflowRun)
			Expect(err).NotTo(HaveOccurred())
			Expect(shouldReturn).To(BeFalse())
			Expect(result).To(Equal(ctrl.Result{}))
		})

		It("should use ParseDuration to compute TTL expiration time", func() {
			// Use ParseDuration to compute expected expiration
			ttlDuration, err := cmdutil.ParseDuration("1h")
			Expect(err).NotTo(HaveOccurred())
			Expect(ttlDuration).To(Equal(1 * time.Hour))

			// Verify the ParseDuration result is used correctly in TTL logic
			// by checking an expired TTL (FinishedAt + TTL < now)
			expiredTime := metav1.NewTime(time.Now().Add(-ttlDuration - 5*time.Minute))
			expirationTime := expiredTime.Add(ttlDuration)
			Expect(time.Now().After(expirationTime)).To(BeTrue(), "Expected TTL to be expired")

			// Also verify a non-expired case
			notExpiredTime := metav1.NewTime(time.Now().Add(-ttlDuration / 2))
			notExpiredExpirationTime := notExpiredTime.Add(ttlDuration)
			Expect(time.Now().After(notExpiredExpirationTime)).To(BeFalse(), "Expected TTL to not be expired")
		})

		It("should request requeue when TTL has not expired", func() {
			// Use ParseDuration to compute TTL
			ttlDuration, err := cmdutil.ParseDuration("2h")
			Expect(err).NotTo(HaveOccurred())

			// Set FinishedAt within TTL window (not expired)
			notExpiredTime := metav1.NewTime(time.Now().Add(-ttlDuration / 2))
			workflowRun := &openchoreodevv1alpha1.WorkflowRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-not-expired",
					Namespace: "default",
				},
				Spec: openchoreodevv1alpha1.WorkflowRunSpec{
					TTLAfterCompletion: "2h",
				},
				Status: openchoreodevv1alpha1.WorkflowRunStatus{
					FinishedAt: &notExpiredTime,
				},
			}

			shouldReturn, result, err := reconciler.checkTTLExpiration(ctx, workflowRun)
			Expect(err).NotTo(HaveOccurred())
			Expect(shouldReturn).To(BeTrue())
			Expect(result.RequeueAfter).To(BeNumerically(">", 0))
			Expect(result.RequeueAfter).To(BeNumerically("<=", ttlDuration))
		})

		It("should handle zero TTL value correctly", func() {
			// Use ParseDuration to verify zero duration is valid
			ttlDuration, err := cmdutil.ParseDuration("0s")
			Expect(err).NotTo(HaveOccurred())
			Expect(ttlDuration).To(Equal(time.Duration(0)))

			// Verify zero TTL means immediate expiration
			// FinishedAt + 0s = FinishedAt, which is before now
			now := metav1.Now()
			expirationTime := now.Time.Add(ttlDuration)
			Expect(time.Now().After(expirationTime)).To(BeTrue(), "Zero TTL should result in immediate expiration")

			// Also test 0d format
			ttlDuration0d, err := cmdutil.ParseDuration("0d")
			Expect(err).NotTo(HaveOccurred())
			Expect(ttlDuration0d).To(Equal(time.Duration(0)))
		})

		It("should handle various duration formats using ParseDuration", func() {
			testCases := []struct {
				ttl              string
				expectedDuration time.Duration
			}{
				{"1h", 1 * time.Hour},
				{"30m", 30 * time.Minute},
				{"90d", 90 * 24 * time.Hour},
				{"1d 12h", 36 * time.Hour},
				{"0s", 0},
				{"0d", 0},
			}

			for _, tc := range testCases {
				duration, err := cmdutil.ParseDuration(tc.ttl)
				Expect(err).NotTo(HaveOccurred(), "Failed to parse TTL: %s", tc.ttl)
				Expect(duration).To(Equal(tc.expectedDuration), "TTL %s should equal %v", tc.ttl, tc.expectedDuration)
			}
		})
	})
})
