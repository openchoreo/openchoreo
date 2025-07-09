// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package kubernetes

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	batchv1 "k8s.io/api/batch/v1"
	"k8s.io/utils/ptr"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/dataplane"
)

var _ = Describe("makeCronJob", func() {
	var (
		deployCtx *dataplane.DeploymentContext
		cronJob   *batchv1.CronJob
	)

	// Prepare fresh DeploymentContext before each test
	BeforeEach(func() {
		deployCtx = newTestDeploymentContext()
	})

	JustBeforeEach(func() {
		cronJob = makeCronJob(deployCtx)
	})

	Context("for a ScheduledTask component", func() {
		BeforeEach(func() {
			deployCtx.Component.Spec.Type = openchoreov1alpha1.ComponentTypeScheduledTask
		})

		It("should create a CronJob with correct name and namespace", func() {
			Expect(cronJob).NotTo(BeNil())
			Expect(cronJob.Name).To(Equal("my-component-my-main-track-a43a18e7"))
			Expect(cronJob.Namespace).To(Equal("dp-test-organiza-my-project-test-environ-04bdf416"))
		})

		expectedLabels := map[string]string{
			"organization-name":     "test-organization",
			"project-name":          "my-project",
			"environment-name":      "test-environment",
			"component-name":        "my-component",
			"component-type":        "ScheduledTask",
			"deployment-track-name": "my-main-track",
			"deployment-name":       "my-deployment",
			"managed-by":            "choreo-deployment-controller",
			"belong-to":             "user-workloads",
		}

		It("should create a CronJob with valid labels", func() {
			Expect(cronJob.Labels).To(BeComparableTo(expectedLabels))
		})

		It("should create a CronJob with correct Job labels", func() {
			Expect(cronJob.Spec.JobTemplate.Labels).To(BeComparableTo(expectedLabels))
		})

		It("should create a CronJob with correct Pod labels", func() {
			Expect(cronJob.Spec.JobTemplate.Spec.Template.Labels).To(BeComparableTo(expectedLabels))
		})

		It("should create a CronJob with correct Spec", func() {
			By("checking the schedule")
			// This will be empty if the configuration is not provided
			Expect(cronJob.Spec.Schedule).To(Equal(""))

			By("checking the concurrency policy")
			Expect(cronJob.Spec.ConcurrencyPolicy).To(Equal(batchv1.ForbidConcurrent))

			By("checking the timezone")
			Expect(cronJob.Spec.TimeZone).To(Equal(ptr.To("Etc/UTC")))
		})

		It("should create a CronJob with correct Job template", func() {
			By("checking the ActiveDeadlineSeconds")
			Expect(cronJob.Spec.JobTemplate.Spec.ActiveDeadlineSeconds).To(Equal(ptr.To(int64(300))))

			By("checking the BackoffLimit")
			Expect(cronJob.Spec.JobTemplate.Spec.BackoffLimit).To(Equal(ptr.To(int32(4))))

			By("checking the TTLSecondsAfterFinished")
			Expect(cronJob.Spec.JobTemplate.Spec.TTLSecondsAfterFinished).To(Equal(ptr.To(int32(360))))
		})

		It("should create a CronJob with a correct container", func() {
			containers := cronJob.Spec.JobTemplate.Spec.Template.Spec.Containers
			By("checking the container length")
			Expect(containers).To(HaveLen(1))

			By("checking the container")
			Expect(containers[0].Name).To(Equal("main"))
			Expect(containers[0].Image).To(Equal("my-image:latest"))
		})
	})

	Context("for a ScheduledTask component with a configuration", func() {
		BeforeEach(func() {
			deployCtx.Component.Spec.Type = openchoreov1alpha1.ComponentTypeScheduledTask
			deployCtx.DeployableArtifact.Spec.Configuration = &openchoreov1alpha1.Configuration{
				Application: &openchoreov1alpha1.Application{
					Task: &openchoreov1alpha1.TaskConfig{
						Disabled: true,
						Schedule: &openchoreov1alpha1.TaskSchedule{
							Cron:     "*/5 * * * *",
							Timezone: "Asia/Colombo",
						},
					},
				},
			}
		})

		It("should create a CronJob with correct schedule", func() {
			Expect(cronJob.Spec.Schedule).To(Equal("*/5 * * * *"))
		})

		It("should create a CronJob with correct timezone", func() {
			Expect(cronJob.Spec.TimeZone).To(Equal(ptr.To("Asia/Colombo")))
		})

		It("should create a CronJob with correct Suspend value", func() {
			Expect(cronJob.Spec.Suspend).To(Equal(ptr.To(true)))
		})
	})
})
