// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package project

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

const (
	testNamespace = "test-namespace"
	testPipeline  = "test-pipeline"
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
		defaulter = Defaulter{}
	})

	createValidProject := func(name string, namespace string, pipelineName string) *openchoreov1alpha1.Project {
		return &openchoreov1alpha1.Project{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
				Labels:    map[string]string{},
			},
			Spec: openchoreov1alpha1.ProjectSpec{
				DeploymentPipelineRef: openchoreov1alpha1.DeploymentPipelineRef{
					Name: pipelineName,
				},
			},
		}
	}

	Context("Defaulter webhook", func() {
		It("should apply defaults and set DeploymentPipelineRef.Kind when it is empty", func() {
			obj = createValidProject("test-project", testNamespace, testPipeline)
			obj.Spec.DeploymentPipelineRef.Kind = ""
			err := defaulter.Default(ctx, obj)
			Expect(err).NotTo(HaveOccurred())
			Expect(obj.Spec.DeploymentPipelineRef.Kind).To(Equal(openchoreov1alpha1.DeploymentPipelineRefKindDeploymentPipeline))
		})

		It("should not overwrite DeploymentPipelineRef.Kind when it is already set", func() {
			obj = createValidProject("test-project", testNamespace, testPipeline)
			obj.Spec.DeploymentPipelineRef.Kind = openchoreov1alpha1.DeploymentPipelineRefKindDeploymentPipeline
			err := defaulter.Default(ctx, obj)
			Expect(err).NotTo(HaveOccurred())
			Expect(obj.Spec.DeploymentPipelineRef.Kind).To(Equal(openchoreov1alpha1.DeploymentPipelineRefKindDeploymentPipeline))
		})

		It("should return an error when given a non-Project object", func() {
			wrongObj := &openchoreov1alpha1.Component{}
			err := defaulter.Default(ctx, wrongObj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("expected"))
		})
	})

	Context("ValidateCreate", func() {
		It("should allow creation of a valid project", func() {
			obj = createValidProject("test-project", testNamespace, testPipeline)
			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should return an error when given a non-Project object", func() {
			wrongObj := &openchoreov1alpha1.Component{}
			_, err := validator.ValidateCreate(ctx, wrongObj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("expected a Project object"))
		})
	})

	Context("ValidateUpdate", func() {
		It("should validate project updates correctly", func() {
			oldObj = createValidProject("test-project", testNamespace, testPipeline)
			obj = createValidProject("test-project", testNamespace, testPipeline)
			_, err := validator.ValidateUpdate(ctx, oldObj, obj)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should return an error when newObj is not a Project", func() {
			wrongObj := &openchoreov1alpha1.Component{}
			_, err := validator.ValidateUpdate(ctx, oldObj, wrongObj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("expected a Project object"))
		})
	})

	Context("ValidateDelete", func() {
		It("should allow deletion of a valid project", func() {
			obj = createValidProject("test-project", testNamespace, testPipeline)
			_, err := validator.ValidateDelete(ctx, obj)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should return an error when given a non-Project object", func() {
			wrongObj := &openchoreov1alpha1.Component{}
			_, err := validator.ValidateDelete(ctx, wrongObj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("expected a Project object"))
		})
	})
})
