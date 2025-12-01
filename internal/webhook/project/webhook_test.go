// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package project

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	admissionv1 "k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apimachineryruntime "k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/labels"
)

const (
	testNamespace = "test-namespace"
	testPipeline  = "test-pipeline"
	testOrg       = "test-org"
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
		validator = Validator{
			client: k8sClient,
		}
		Expect(validator).NotTo(BeNil(), "Expected validator to be initialized")
		defaulter = Defaulter{}
		Expect(defaulter).NotTo(BeNil(), "Expected defaulter to be initialized")
		Expect(oldObj).NotTo(BeNil(), "Expected oldObj to be initialized")
		Expect(obj).NotTo(BeNil(), "Expected obj to be initialized")
		// TODO (user): Add any setup logic common to all tests
	})

	AfterEach(func() {
		// TODO (user): Add any teardown logic common to all tests
	})

	// Helper functions
	createFakeClientBuilder := func() *fake.ClientBuilder {
		scheme := apimachineryruntime.NewScheme()
		err := openchoreov1alpha1.AddToScheme(scheme)
		Expect(err).NotTo(HaveOccurred())

		err = admissionv1.AddToScheme(scheme)
		Expect(err).NotTo(HaveOccurred())

		return fake.NewClientBuilder().WithScheme(scheme)
	}

	createValidDeploymentPipeline := func(name string, namespace string) *openchoreov1alpha1.DeploymentPipeline {
		pipeline := &openchoreov1alpha1.DeploymentPipeline{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
				Labels: map[string]string{
					labels.LabelKeyName: name,
				},
			},
		}
		return pipeline
	}

	createValidProject := func(name string, orgName string, namespace string, pipelineName string) *openchoreov1alpha1.Project {
		project := &openchoreov1alpha1.Project{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
				Labels: map[string]string{
					labels.LabelKeyName:             name,
					labels.LabelKeyOrganizationName: orgName,
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
			// Currently no defaulting logic is implemented, but the test structure is in place
			By("Creating a basic project")
			obj = createValidProject("test-project", testOrg, testNamespace, testPipeline)

			By("Calling the Default method")
			err := defaulter.Default(ctx, obj)

			By("Verifying defaulting runs without error")
			Expect(err).NotTo(HaveOccurred())

			// If you implement actual defaulting logic, add assertions here
		})
	})

	Context("When validating Project creation", func() {
		It("Should deny creation if referenced deployment pipeline does not exist", func() {
			By("Setting up client with no deployment pipelines")
			validatorWithClient := Validator{
				client: createFakeClientBuilder().Build(),
			}

			By("Creating a project with non-existent deployment pipeline")
			obj = createValidProject("test-project", testNamespace, testNamespace, "non-existent-pipeline")

			By("Validating the project creation")
			_, err := validatorWithClient.ValidateCreate(ctx, obj)

			By("Verifying validation fails with appropriate error")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("deployment pipeline 'non-existent-pipeline' specified in project 'test-project' not found"))
		})

		It("Should allow creation of a valid project", func() {
			By("Creating a deployment pipeline")
			pipelineName := testPipeline
			pipeline := createValidDeploymentPipeline(pipelineName, testNamespace)

			By("Setting up client with pipeline")
			validatorWithResources := Validator{
				client: createFakeClientBuilder().WithObjects(pipeline).Build(),
			}

			By("Creating a valid project")
			obj = createValidProject("test-project", testNamespace, testNamespace, pipelineName)

			By("Validating the project creation")
			_, err := validatorWithResources.ValidateCreate(ctx, obj)

			By("Verifying validation succeeds")
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("When validating Project updates", func() {
		It("Should validate project updates correctly", func() {
			By("Creating a deployment pipeline")
			pipelineName := testPipeline
			pipeline := createValidDeploymentPipeline(pipelineName, testNamespace)

			By("Setting up client with pipeline")
			validatorWithResources := Validator{
				client: createFakeClientBuilder().WithObjects(pipeline).Build(),
			}

			By("Creating old and new versions of the project")
			oldObj = createValidProject("test-project", testNamespace, testNamespace, pipelineName)
			obj = createValidProject("test-project", testNamespace, testNamespace, pipelineName)

			// There is no updates to the project object, so the validation should pass
			By("Validating the project update")
			_, err := validatorWithResources.ValidateUpdate(ctx, oldObj, obj)

			By("Verifying validation succeeds")
			Expect(err).NotTo(HaveOccurred())
		})
	})
})
