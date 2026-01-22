// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package project

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/labels"
)

const (
	testNamespace = "test-namespace"
	testPipeline  = "test-pipeline"
	testOrg       = "test-namespace"
)

var _ = Describe("Project Webhook", func() {
	var (
		obj       *openchoreov1alpha1.Project
		oldObj    *openchoreov1alpha1.Project
		validator Validator
		defaulter Defaulter
	)

	BeforeEach(func() {
		obj = &openchoreov1alpha1.Project{}
		oldObj = &openchoreov1alpha1.Project{}
		validator = Validator{}
		Expect(validator).NotTo(BeNil(), "Expected validator to be initialized")
		defaulter = Defaulter{}
		Expect(defaulter).NotTo(BeNil(), "Expected defaulter to be initialized")
		Expect(oldObj).NotTo(BeNil(), "Expected oldObj to be initialized")
		Expect(obj).NotTo(BeNil(), "Expected obj to be initialized")
	})

	AfterEach(func() {
	})

	createValidProject := func(name string, namespaceName string, namespace string, pipelineName string) *openchoreov1alpha1.Project {
		project := &openchoreov1alpha1.Project{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
				Labels: map[string]string{
					labels.LabelKeyName:          name,
					labels.LabelKeyNamespaceName: namespaceName,
				},
			},
			Spec: openchoreov1alpha1.ProjectSpec{
				DeploymentPipelineRef: pipelineName,
			},
		}
		return project
	}

	Context("When creating Project under Defaulting Webhook", func() {
		It("Should apply defaults correctly", func() {
			By("Creating a basic project")
			obj = createValidProject("test-project", testOrg, testNamespace, testPipeline)

			By("Calling the Default method")
			err := defaulter.Default(ctx, obj)

			By("Verifying defaulting runs without error")
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("When validating Project creation", func() {
		It("Should allow creation of a valid project", func() {
			By("Creating a valid project")
			obj = createValidProject("test-project", testOrg, testNamespace, testPipeline)

			By("Validating the project creation")
			_, err := validator.ValidateCreate(ctx, obj)

			By("Verifying validation succeeds")
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("When validating Project updates", func() {
		It("Should validate project updates correctly", func() {
			By("Creating old and new versions of the project")
			oldObj = createValidProject("test-project", testOrg, testNamespace, testPipeline)
			obj = createValidProject("test-project", testOrg, testNamespace, testPipeline)

			By("Validating the project update")
			_, err := validator.ValidateUpdate(ctx, oldObj, obj)

			By("Verifying validation succeeds")
			Expect(err).NotTo(HaveOccurred())
		})
	})
})
