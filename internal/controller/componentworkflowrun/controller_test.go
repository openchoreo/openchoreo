// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package componentworkflowrun

import (
	"context"
	"encoding/json"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	openchoreodevv1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	argoproj "github.com/openchoreo/openchoreo/internal/dataplane/kubernetes/types/argoproj.io/workflow/v1alpha1"
)

var _ = Describe("ComponentWorkflowRun Controller", func() {
	Context("When reconciling a ComponentWorkflowRun resource", func() {
		const resourceName = "test-componentworkflowrun"
		const workflowName = "test-workflow"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default",
		}

		BeforeEach(func() {
			By("Creating the ComponentWorkflowRun resource")
			componentworkflowrun := &openchoreodevv1alpha1.ComponentWorkflowRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName,
					Namespace: "default",
				},
				Spec: openchoreodevv1alpha1.ComponentWorkflowRunSpec{
					Owner: openchoreodevv1alpha1.ComponentWorkflowOwner{
						ProjectName:   "test-project",
						ComponentName: "test-component",
					},
					Workflow: openchoreodevv1alpha1.ComponentWorkflowRunConfig{
						Name: workflowName,
						SystemParameters: openchoreodevv1alpha1.SystemParametersValues{
							Repository: openchoreodevv1alpha1.RepositoryValues{
								URL: "https://github.com/openchoreo/test-repo",
								Revision: openchoreodevv1alpha1.RepositoryRevisionValues{
									Branch: "main",
								},
								AppPath: ".",
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, componentworkflowrun)).To(Succeed())
		})

		AfterEach(func() {
			By("Cleaning up the ComponentWorkflowRun resource")
			resource := &openchoreodevv1alpha1.ComponentWorkflowRun{}
			if err := k8sClient.Get(ctx, typeNamespacedName, resource); err == nil {
				// Remove finalizer if present to allow deletion
				if controllerutil.ContainsFinalizer(resource, BuildPlaneCleanupFinalizer) {
					controllerutil.RemoveFinalizer(resource, BuildPlaneCleanupFinalizer)
					Expect(k8sClient.Update(ctx, resource)).To(Succeed())
				}
				Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
			}
		})

		It("should successfully create the resource", func() {
			By("Verifying the ComponentWorkflowRun was created")
			resource := &openchoreodevv1alpha1.ComponentWorkflowRun{}
			Expect(k8sClient.Get(ctx, typeNamespacedName, resource)).To(Succeed())
			Expect(resource.Name).To(Equal(resourceName))
			Expect(resource.Spec.Workflow.Name).To(Equal(workflowName))
		})

		It("should handle reconciliation when resource not found", func() {
			By("Reconciling a non-existent resource")
			reconciler := &ComponentWorkflowRunReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			nonExistentName := types.NamespacedName{
				Name:      "non-existent",
				Namespace: "default",
			}

			result, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: nonExistentName,
			})

			Expect(err).NotTo(HaveOccurred())
			Expect(result.Requeue).To(BeFalse())
		})

		It("should have empty status initially", func() {
			By("Verifying the ComponentWorkflowRun has empty status")
			resource := &openchoreodevv1alpha1.ComponentWorkflowRun{}
			Expect(k8sClient.Get(ctx, typeNamespacedName, resource)).To(Succeed())
			Expect(resource.Status.Conditions).To(BeEmpty())
			Expect(resource.Status.RunReference).To(BeNil())
			Expect(resource.Status.Resources).To(BeNil())
			Expect(resource.Status.ImageStatus.Image).To(BeEmpty())
		})
	})

	Context("When working with status fields", func() {
		It("should correctly set and retrieve RunReference", func() {
			cwf := &openchoreodevv1alpha1.ComponentWorkflowRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-run-ref",
					Namespace: "default",
				},
				Status: openchoreodevv1alpha1.ComponentWorkflowRunStatus{
					RunReference: &openchoreodevv1alpha1.ResourceReference{
						APIVersion: "argoproj.io/v1alpha1",
						Kind:       "Workflow",
						Name:       "test-workflow-run",
						Namespace:  "build-namespace",
					},
				},
			}

			Expect(cwf.Status.RunReference).NotTo(BeNil())
			Expect(cwf.Status.RunReference.APIVersion).To(Equal("argoproj.io/v1alpha1"))
			Expect(cwf.Status.RunReference.Kind).To(Equal("Workflow"))
			Expect(cwf.Status.RunReference.Name).To(Equal("test-workflow-run"))
			Expect(cwf.Status.RunReference.Namespace).To(Equal("build-namespace"))
		})

		It("should correctly set and retrieve Resources", func() {
			resources := []openchoreodevv1alpha1.ResourceReference{
				{
					APIVersion: "v1",
					Kind:       "Secret",
					Name:       "registry-credentials",
					Namespace:  "build-namespace",
				},
				{
					APIVersion: "v1",
					Kind:       "ConfigMap",
					Name:       "build-config",
					Namespace:  "build-namespace",
				},
			}

			cwf := &openchoreodevv1alpha1.ComponentWorkflowRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-resources",
					Namespace: "default",
				},
				Status: openchoreodevv1alpha1.ComponentWorkflowRunStatus{
					Resources: &resources,
				},
			}

			Expect(cwf.Status.Resources).NotTo(BeNil())
			Expect(*cwf.Status.Resources).To(HaveLen(2))
			Expect((*cwf.Status.Resources)[0].Kind).To(Equal("Secret"))
			Expect((*cwf.Status.Resources)[0].Name).To(Equal("registry-credentials"))
			Expect((*cwf.Status.Resources)[1].Kind).To(Equal("ConfigMap"))
			Expect((*cwf.Status.Resources)[1].Name).To(Equal("build-config"))
		})

		It("should correctly set and retrieve ImageStatus", func() {
			cwf := &openchoreodevv1alpha1.ComponentWorkflowRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-image-status",
					Namespace: "default",
				},
				Status: openchoreodevv1alpha1.ComponentWorkflowRunStatus{
					ImageStatus: openchoreodevv1alpha1.ComponentWorkflowImage{
						Image: "registry.example.com/myapp:v1.0.0",
					},
				},
			}

			Expect(cwf.Status.ImageStatus.Image).To(Equal("registry.example.com/myapp:v1.0.0"))
		})
	})
})

// Unit tests for helper functions
var _ = Describe("Helper Functions", func() {
	Describe("getStepByTemplateName", func() {
		It("should find a node by template name", func() {
			nodes := argoproj.Nodes{
				"node-1": {
					Name:         "node-1",
					TemplateName: "build-step",
					Phase:        argoproj.NodeSucceeded,
				},
				"node-2": {
					Name:         "node-2",
					TemplateName: "push-step",
					Phase:        argoproj.NodeSucceeded,
				},
			}

			result := getStepByTemplateName(nodes, "push-step")
			Expect(result).NotTo(BeNil())
			Expect(result.TemplateName).To(Equal("push-step"))
			Expect(result.Name).To(Equal("node-2"))
		})

		It("should return nil when template not found", func() {
			nodes := argoproj.Nodes{
				"node-1": {
					Name:         "node-1",
					TemplateName: "build-step",
					Phase:        argoproj.NodeSucceeded,
				},
			}

			result := getStepByTemplateName(nodes, "non-existent")
			Expect(result).To(BeNil())
		})

		It("should handle empty nodes", func() {
			nodes := argoproj.Nodes{}
			result := getStepByTemplateName(nodes, "any-step")
			Expect(result).To(BeNil())
		})
	})

	Describe("getImageNameFromRunResource", func() {
		It("should extract image name from outputs", func() {
			imageName := argoproj.AnyString("my-registry/my-image:v1.0.0")
			outputs := argoproj.Outputs{
				Parameters: []argoproj.Parameter{
					{
						Name:  "image",
						Value: &imageName,
					},
				},
			}

			result := getImageNameFromRunResource(outputs)
			Expect(result).To(Equal(imageName))
		})

		It("should return empty string when image parameter not found", func() {
			outputs := argoproj.Outputs{
				Parameters: []argoproj.Parameter{
					{
						Name:  "other-param",
						Value: nil,
					},
				},
			}

			result := getImageNameFromRunResource(outputs)
			Expect(result).To(Equal(argoproj.AnyString("")))
		})

		It("should handle empty outputs", func() {
			outputs := argoproj.Outputs{
				Parameters: []argoproj.Parameter{},
			}

			result := getImageNameFromRunResource(outputs)
			Expect(result).To(Equal(argoproj.AnyString("")))
		})
	})

	Describe("extractWorkloadCRFromRunResource", func() {
		It("should extract workload CR from run resource", func() {
			workloadYAML := argoproj.AnyString(`apiVersion: openchoreo.dev/v1alpha1
kind: Workload
metadata:
  name: test-workload`)

			workflow := &argoproj.Workflow{
				Status: argoproj.WorkflowStatus{
					Nodes: argoproj.Nodes{
						"workload-node": {
							TemplateName: "workload-create-step",
							Phase:        argoproj.NodeSucceeded,
							Outputs: &argoproj.Outputs{
								Parameters: []argoproj.Parameter{
									{
										Name:  "workload-cr",
										Value: &workloadYAML,
									},
								},
							},
						},
					},
				},
			}

			result := extractWorkloadCRFromRunResource(workflow)
			Expect(result).To(ContainSubstring("kind: Workload"))
			Expect(result).To(ContainSubstring("test-workload"))
		})

		It("should return empty string when workload CR not found", func() {
			workflow := &argoproj.Workflow{
				Status: argoproj.WorkflowStatus{
					Nodes: argoproj.Nodes{
						"other-node": {
							TemplateName: "other-step",
							Phase:        argoproj.NodeSucceeded,
						},
					},
				},
			}

			result := extractWorkloadCRFromRunResource(workflow)
			Expect(result).To(Equal(""))
		})

		It("should return empty string when node phase is not succeeded", func() {
			workloadYAML := argoproj.AnyString("workload-content")
			workflow := &argoproj.Workflow{
				Status: argoproj.WorkflowStatus{
					Nodes: argoproj.Nodes{
						"workload-node": {
							TemplateName: "workload-create-step",
							Phase:        argoproj.NodeFailed,
							Outputs: &argoproj.Outputs{
								Parameters: []argoproj.Parameter{
									{
										Name:  "workload-cr",
										Value: &workloadYAML,
									},
								},
							},
						},
					},
				},
			}

			result := extractWorkloadCRFromRunResource(workflow)
			Expect(result).To(Equal(""))
		})
	})

	Describe("extractServiceAccountName", func() {
		It("should extract service account name from resource", func() {
			resource := map[string]any{
				"spec": map[string]any{
					"serviceAccountName": "my-service-account",
				},
			}

			result, err := extractServiceAccountName(resource)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal("my-service-account"))
		})

		It("should return error when spec not found", func() {
			resource := map[string]any{
				"metadata": map[string]any{},
			}

			_, err := extractServiceAccountName(resource)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("spec not found"))
		})

		It("should return error when serviceAccountName not found", func() {
			resource := map[string]any{
				"spec": map[string]any{
					"otherField": "value",
				},
			}

			_, err := extractServiceAccountName(resource)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("serviceAccountName not found"))
		})

		It("should return error when serviceAccountName is empty", func() {
			resource := map[string]any{
				"spec": map[string]any{
					"serviceAccountName": "",
				},
			}

			_, err := extractServiceAccountName(resource)
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("extractRunResourceNamespace", func() {
		It("should extract namespace from resource", func() {
			resource := map[string]any{
				"metadata": map[string]any{
					"namespace": "my-namespace",
					"name":      "my-resource",
				},
			}

			result, err := extractRunResourceNamespace(resource)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal("my-namespace"))
		})

		It("should return error when metadata not found", func() {
			resource := map[string]any{
				"spec": map[string]any{},
			}

			_, err := extractRunResourceNamespace(resource)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("metadata not found"))
		})

		It("should return error when namespace not found", func() {
			resource := map[string]any{
				"metadata": map[string]any{
					"name": "my-resource",
				},
			}

			_, err := extractRunResourceNamespace(resource)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("namespace not found"))
		})

		It("should return error when namespace is empty", func() {
			resource := map[string]any{
				"metadata": map[string]any{
					"namespace": "",
				},
			}

			_, err := extractRunResourceNamespace(resource)
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("convertToString", func() {
		It("should convert string to string", func() {
			result := convertToString("hello")
			Expect(result).To(Equal("hello"))
		})

		It("should convert int to string", func() {
			result := convertToString(42)
			Expect(result).To(Equal("42"))
		})

		It("should convert int32 to string", func() {
			result := convertToString(int32(42))
			Expect(result).To(Equal("42"))
		})

		It("should convert int64 to string", func() {
			result := convertToString(int64(42))
			Expect(result).To(Equal("42"))
		})

		It("should convert float32 to string", func() {
			result := convertToString(float32(3.14))
			Expect(result).To(ContainSubstring("3.14"))
		})

		It("should convert float64 to string", func() {
			result := convertToString(3.14159)
			Expect(result).To(ContainSubstring("3.14"))
		})

		It("should convert bool to string", func() {
			Expect(convertToString(true)).To(Equal("true"))
			Expect(convertToString(false)).To(Equal("false"))
		})

		It("should convert map to JSON string", func() {
			input := map[string]any{
				"key1": "value1",
				"key2": 42,
			}
			result := convertToString(input)
			var decoded map[string]any
			err := json.Unmarshal([]byte(result), &decoded)
			Expect(err).NotTo(HaveOccurred())
			Expect(decoded["key1"]).To(Equal("value1"))
		})

		It("should convert slice to JSON string", func() {
			input := []any{"item1", "item2", 3}
			result := convertToString(input)
			var decoded []any
			err := json.Unmarshal([]byte(result), &decoded)
			Expect(err).NotTo(HaveOccurred())
			Expect(decoded).To(HaveLen(3))
		})
	})

	Describe("convertParameterValuesToStrings", func() {
		It("should convert parameter values in workflow resource", func() {
			resource := map[string]any{
				"apiVersion": "argoproj.io/v1alpha1",
				"kind":       "Workflow",
				"spec": map[string]any{
					"arguments": map[string]any{
						"parameters": []any{
							map[string]any{
								"name":  "param1",
								"value": 42,
							},
							map[string]any{
								"name":  "param2",
								"value": true,
							},
						},
					},
				},
			}

			result := convertParameterValuesToStrings(resource)
			spec := result["spec"].(map[string]any)
			args := spec["arguments"].(map[string]any)
			params := args["parameters"].([]any)

			param1 := params[0].(map[string]any)
			Expect(param1["value"]).To(Equal("42"))

			param2 := params[1].(map[string]any)
			Expect(param2["value"]).To(Equal("true"))
		})

		It("should preserve non-parameter fields", func() {
			resource := map[string]any{
				"apiVersion": "argoproj.io/v1alpha1",
				"kind":       "Workflow",
				"metadata": map[string]any{
					"name": "test",
				},
			}

			result := convertParameterValuesToStrings(resource)
			Expect(result["apiVersion"]).To(Equal("argoproj.io/v1alpha1"))
			Expect(result["kind"]).To(Equal("Workflow"))
		})
	})
})
