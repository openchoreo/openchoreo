// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package component

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/controller"
)

// ── test helpers ──────────────────────────────────────────────────────────────

const (
	itNamespace = "default"
	itTimeout   = time.Second * 15
	itInterval  = time.Millisecond * 250
)

func itReconciler() *Reconciler {
	return &Reconciler{
		Client: k8sClient,
		Scheme: k8sClient.Scheme(),
	}
}

// minimalCT returns a namespaced ComponentType with the given workloadType.
// resources includes one template whose ID matches workloadType (required by CRD validation).
func minimalCT(name, workloadType string) *openchoreov1alpha1.ComponentType {
	return &openchoreov1alpha1.ComponentType{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: itNamespace},
		Spec: openchoreov1alpha1.ComponentTypeSpec{
			WorkloadType: workloadType,
			Resources: []openchoreov1alpha1.ResourceTemplate{
				{
					ID:       workloadType,
					Template: &runtime.RawExtension{Raw: []byte(`{"apiVersion":"apps/v1","kind":"Deployment"}`)},
				},
			},
		},
	}
}

// minimalCCT returns a cluster-scoped ClusterComponentType.
func minimalCCT(name, workloadType string) *openchoreov1alpha1.ClusterComponentType {
	return &openchoreov1alpha1.ClusterComponentType{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: openchoreov1alpha1.ClusterComponentTypeSpec{
			WorkloadType: workloadType,
			Resources: []openchoreov1alpha1.ResourceTemplate{
				{
					ID:       workloadType,
					Template: &runtime.RawExtension{Raw: []byte(`{"apiVersion":"apps/v1","kind":"Deployment"}`)},
				},
			},
		},
	}
}

// minimalComp returns a Component with the ComponentFinalizer already set.
// fullCTName is the "{workloadType}/{ctName}" string (e.g. "deployment/myct").
func minimalComp(name, project, ctKind, fullCTName string, autoDeploy bool) *openchoreov1alpha1.Component {
	return &openchoreov1alpha1.Component{
		ObjectMeta: metav1.ObjectMeta{
			Name:       name,
			Namespace:  itNamespace,
			Finalizers: []string{ComponentFinalizer},
		},
		Spec: openchoreov1alpha1.ComponentSpec{
			Owner: openchoreov1alpha1.ComponentOwner{ProjectName: project},
			ComponentType: openchoreov1alpha1.ComponentTypeRef{
				Kind: openchoreov1alpha1.ComponentTypeRefKind(ctKind),
				Name: fullCTName,
			},
			AutoDeploy: autoDeploy,
		},
	}
}

// minimalWorkload returns a Workload owned by the given project/component.
func minimalWorkload(name, project, component, image string) *openchoreov1alpha1.Workload {
	return &openchoreov1alpha1.Workload{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: itNamespace},
		Spec: openchoreov1alpha1.WorkloadSpec{
			Owner: openchoreov1alpha1.WorkloadOwner{
				ProjectName:   project,
				ComponentName: component,
			},
			WorkloadTemplateSpec: openchoreov1alpha1.WorkloadTemplateSpec{
				Container: openchoreov1alpha1.Container{Image: image},
			},
		},
	}
}

// minimalProject returns a Project with the given DeploymentPipelineRef.
func minimalProject(name, pipelineRef string) *openchoreov1alpha1.Project {
	return &openchoreov1alpha1.Project{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: itNamespace},
		Spec:       openchoreov1alpha1.ProjectSpec{DeploymentPipelineRef: pipelineRef},
	}
}

// minimalPipeline returns a DeploymentPipeline with a single dev→staging promotion path.
func minimalPipeline(name string) *openchoreov1alpha1.DeploymentPipeline {
	return &openchoreov1alpha1.DeploymentPipeline{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: itNamespace},
		Spec: openchoreov1alpha1.DeploymentPipelineSpec{
			PromotionPaths: []openchoreov1alpha1.PromotionPath{
				{
					SourceEnvironmentRef: "dev",
					TargetEnvironmentRefs: []openchoreov1alpha1.TargetEnvironmentRef{
						{Name: "staging"},
					},
				},
			},
		},
	}
}

// forceDeleteObj strips finalizers then deletes obj; safe to call in AfterEach.
func forceDeleteObj(ctx context.Context, obj client.Object) {
	_ = k8sClient.Get(ctx, client.ObjectKeyFromObject(obj), obj)
	obj.SetFinalizers(nil)
	_ = k8sClient.Update(ctx, obj)
	_ = k8sClient.Delete(ctx, obj)
}

// conditionFor returns the named condition from Component.Status.Conditions, or nil.
func conditionFor(comp *openchoreov1alpha1.Component) *metav1.Condition {
	if comp == nil {
		return nil
	}
	for i := range comp.Status.Conditions {
		if comp.Status.Conditions[i].Type == string(ConditionReady) {
			return &comp.Status.Conditions[i]
		}
	}
	return nil
}

// fetchComp re-fetches a Component directly from the API server.
func fetchComp(ctx context.Context, name string) *openchoreov1alpha1.Component {
	comp := &openchoreov1alpha1.Component{}
	if err := k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: itNamespace}, comp); err != nil {
		return nil
	}
	return comp
}

// reconcileUntilCondition drives reconciliation until the component has the expected condition
// or the timeout expires. Returns the final condition.
func reconcileUntilCondition(
	ctx context.Context,
	r *Reconciler,
	name string,
	expectedReason controller.ConditionReason,
) {
	req := reconcile.Request{NamespacedName: types.NamespacedName{Name: name, Namespace: itNamespace}}
	GinkgoHelper()
	Eventually(func(g Gomega) {
		_, err := r.Reconcile(ctx, req)
		g.Expect(err).NotTo(HaveOccurred())
		comp := fetchComp(ctx, name)
		g.Expect(comp).NotTo(BeNil())
		cond := conditionFor(comp)
		g.Expect(cond).NotTo(BeNil(), "condition %q not set", ConditionReady)
		g.Expect(cond.Reason).To(Equal(string(expectedReason)))
	}, itTimeout, itInterval).Should(Succeed())
}

// ── ComponentType resolution ──────────────────────────────────────────────────

var _ = Describe("Component Controller — ComponentType resolution", func() {
	var r *Reconciler
	BeforeEach(func() {
		r = itReconciler()
	})

	Context("When component references a ClusterComponentType", func() {
		const (
			cctName  = "dep-cct"
			compName = "cct-comp"
			project  = "cct-proj"
		)
		var cct *openchoreov1alpha1.ClusterComponentType
		var comp *openchoreov1alpha1.Component

		BeforeEach(func() {
			By("Creating a ClusterComponentType")
			cct = minimalCCT(cctName, "deployment")
			Expect(k8sClient.Create(ctx, cct)).To(Succeed())

			By("Creating a Component referencing the ClusterComponentType with finalizer pre-seeded")
			comp = minimalComp(compName, project, string(openchoreov1alpha1.ComponentTypeRefKindClusterComponentType),
				"deployment/"+cctName, false)
			Expect(k8sClient.Create(ctx, comp)).To(Succeed())
		})

		AfterEach(func() {
			forceDeleteObj(ctx, comp)
			_ = k8sClient.Delete(ctx, cct)
		})

		It("should proceed past CT validation and fail at Workload (no Workload exists)", func() {
			By("Reconciling — CT found, next gate is Workload validation")
			reconcileUntilCondition(ctx, r, compName, ReasonWorkloadNotFound)

			By("Verifying condition status is False/WorkloadNotFound")
			c := fetchComp(ctx, compName)
			cond := conditionFor(c)
			Expect(cond).NotTo(BeNil())
			Expect(cond.Status).To(Equal(metav1.ConditionFalse))
		})
	})

	Context("When component specifies a workloadType that mismatches the ComponentType", func() {
		const (
			ctName   = "sfs-mismatch-ct"
			compName = "mismatch-comp"
			project  = "mismatch-proj"
		)
		var ct *openchoreov1alpha1.ComponentType
		var comp *openchoreov1alpha1.Component

		BeforeEach(func() {
			By("Creating a ComponentType with workloadType=statefulset")
			ct = minimalCT(ctName, "statefulset")
			Expect(k8sClient.Create(ctx, ct)).To(Succeed())

			By("Creating a Component with name 'deployment/<ctName>' (workloadType mismatch)")
			comp = minimalComp(compName, project, string(openchoreov1alpha1.ComponentTypeRefKindComponentType),
				"deployment/"+ctName, false)
			Expect(k8sClient.Create(ctx, comp)).To(Succeed())
		})

		AfterEach(func() {
			forceDeleteObj(ctx, comp)
			_ = k8sClient.Delete(ctx, ct)
		})

		It("should set ConditionReady=False/InvalidConfiguration", func() {
			reconcileUntilCondition(ctx, r, compName, ReasonInvalidConfiguration)

			c := fetchComp(ctx, compName)
			cond := conditionFor(c)
			Expect(cond).NotTo(BeNil())
			Expect(cond.Status).To(Equal(metav1.ConditionFalse))
		})
	})
})

// ── Dependency not-found conditions ──────────────────────────────────────────

var _ = Describe("Component Controller — dependency not found conditions", func() {
	var r *Reconciler
	BeforeEach(func() {
		r = itReconciler()
	})

	Context("When Workload for component does not exist", func() {
		const (
			ctName   = "wl-notfound-ct"
			compName = "wl-notfound-comp"
			project  = "wl-notfound-proj"
		)
		var ct *openchoreov1alpha1.ComponentType
		var comp *openchoreov1alpha1.Component

		BeforeEach(func() {
			ct = minimalCT(ctName, "deployment")
			Expect(k8sClient.Create(ctx, ct)).To(Succeed())

			comp = minimalComp(compName, project, string(openchoreov1alpha1.ComponentTypeRefKindComponentType),
				"deployment/"+ctName, false)
			Expect(k8sClient.Create(ctx, comp)).To(Succeed())
		})

		AfterEach(func() {
			forceDeleteObj(ctx, comp)
			_ = k8sClient.Delete(ctx, ct)
		})

		It("should set ConditionReady=False/WorkloadNotFound", func() {
			reconcileUntilCondition(ctx, r, compName, ReasonWorkloadNotFound)

			c := fetchComp(ctx, compName)
			cond := conditionFor(c)
			Expect(cond.Status).To(Equal(metav1.ConditionFalse))
		})
	})

	Context("When Project for component does not exist", func() {
		const (
			ctName   = "proj-notfound-ct"
			compName = "proj-notfound-comp"
			wlName   = "proj-notfound-wl"
			project  = "proj-notfound-proj"
		)
		var ct *openchoreov1alpha1.ComponentType
		var wl *openchoreov1alpha1.Workload
		var comp *openchoreov1alpha1.Component

		BeforeEach(func() {
			ct = minimalCT(ctName, "deployment")
			Expect(k8sClient.Create(ctx, ct)).To(Succeed())

			wl = minimalWorkload(wlName, project, compName, "nginx:latest")
			Expect(k8sClient.Create(ctx, wl)).To(Succeed())

			comp = minimalComp(compName, project, string(openchoreov1alpha1.ComponentTypeRefKindComponentType),
				"deployment/"+ctName, false)
			Expect(k8sClient.Create(ctx, comp)).To(Succeed())
		})

		AfterEach(func() {
			forceDeleteObj(ctx, comp)
			_ = k8sClient.Delete(ctx, wl)
			_ = k8sClient.Delete(ctx, ct)
		})

		It("should set ConditionReady=False/ProjectNotFound", func() {
			// Use Eventually: Workload cache may need a moment to populate
			reconcileUntilCondition(ctx, r, compName, ReasonProjectNotFound)

			c := fetchComp(ctx, compName)
			cond := conditionFor(c)
			Expect(cond.Status).To(Equal(metav1.ConditionFalse))
		})
	})

	Context("When DeploymentPipeline for component does not exist", func() {
		const (
			ctName       = "dp-notfound-ct"
			compName     = "dp-notfound-comp"
			wlName       = "dp-notfound-wl"
			project      = "dp-notfound-proj"
			pipelineName = "dp-notfound-pipe"
		)
		var ct *openchoreov1alpha1.ComponentType
		var wl *openchoreov1alpha1.Workload
		var proj *openchoreov1alpha1.Project
		var comp *openchoreov1alpha1.Component

		BeforeEach(func() {
			ct = minimalCT(ctName, "deployment")
			Expect(k8sClient.Create(ctx, ct)).To(Succeed())

			wl = minimalWorkload(wlName, project, compName, "nginx:latest")
			Expect(k8sClient.Create(ctx, wl)).To(Succeed())

			proj = minimalProject(project, pipelineName)
			Expect(k8sClient.Create(ctx, proj)).To(Succeed())

			comp = minimalComp(compName, project, string(openchoreov1alpha1.ComponentTypeRefKindComponentType),
				"deployment/"+ctName, false)
			Expect(k8sClient.Create(ctx, comp)).To(Succeed())
		})

		AfterEach(func() {
			forceDeleteObj(ctx, comp)
			_ = k8sClient.Delete(ctx, proj)
			_ = k8sClient.Delete(ctx, wl)
			_ = k8sClient.Delete(ctx, ct)
		})

		It("should set ConditionReady=False/DeploymentPipelineNotFound", func() {
			reconcileUntilCondition(ctx, r, compName, ReasonDeploymentPipelineNotFound)

			c := fetchComp(ctx, compName)
			cond := conditionFor(c)
			Expect(cond.Status).To(Equal(metav1.ConditionFalse))
		})
	})
})

// ── Workflow validation ───────────────────────────────────────────────────────

var _ = Describe("Component Controller — Workflow validation", func() {
	var r *Reconciler
	BeforeEach(func() {
		r = itReconciler()
	})

	Context("When Workflow is not in ComponentType allowedWorkflows", func() {
		const (
			ctName   = "wf-notallowed-ct"
			compName = "wf-notallowed-comp"
			project  = "wf-notallowed-proj"
		)
		var ct *openchoreov1alpha1.ComponentType
		var comp *openchoreov1alpha1.Component

		BeforeEach(func() {
			By("Creating ComponentType with allowedWorkflows=[allowed-wf]")
			ct = minimalCT(ctName, "deployment")
			ct.Spec.AllowedWorkflows = []openchoreov1alpha1.WorkflowRef{
				{Name: "allowed-wf"},
			}
			Expect(k8sClient.Create(ctx, ct)).To(Succeed())

			By("Creating Component with workflow=other-wf (not in allowed list)")
			comp = minimalComp(compName, project, string(openchoreov1alpha1.ComponentTypeRefKindComponentType),
				"deployment/"+ctName, false)
			comp.Spec.Workflow = &openchoreov1alpha1.WorkflowRunConfig{Name: "other-wf"}
			Expect(k8sClient.Create(ctx, comp)).To(Succeed())
		})

		AfterEach(func() {
			forceDeleteObj(ctx, comp)
			_ = k8sClient.Delete(ctx, ct)
		})

		It("should set ConditionReady=False/WorkflowNotAllowed", func() {
			reconcileUntilCondition(ctx, r, compName, ReasonWorkflowNotAllowed)

			c := fetchComp(ctx, compName)
			cond := conditionFor(c)
			Expect(cond.Status).To(Equal(metav1.ConditionFalse))
		})
	})

	Context("When Workflow is in allowedWorkflows but Workflow resource does not exist", func() {
		const (
			ctName       = "wf-notfound-ct"
			compName     = "wf-notfound-comp"
			project      = "wf-notfound-proj"
			workflowName = "my-wf"
		)
		var ct *openchoreov1alpha1.ComponentType
		var comp *openchoreov1alpha1.Component

		BeforeEach(func() {
			By("Creating ComponentType with allowedWorkflows=[my-wf]")
			ct = minimalCT(ctName, "deployment")
			ct.Spec.AllowedWorkflows = []openchoreov1alpha1.WorkflowRef{
				{Name: workflowName},
			}
			Expect(k8sClient.Create(ctx, ct)).To(Succeed())

			By("Creating Component referencing my-wf (no Workflow resource exists)")
			comp = minimalComp(compName, project, string(openchoreov1alpha1.ComponentTypeRefKindComponentType),
				"deployment/"+ctName, false)
			comp.Spec.Workflow = &openchoreov1alpha1.WorkflowRunConfig{Name: workflowName}
			Expect(k8sClient.Create(ctx, comp)).To(Succeed())
		})

		AfterEach(func() {
			forceDeleteObj(ctx, comp)
			_ = k8sClient.Delete(ctx, ct)
		})

		It("should set ConditionReady=False/WorkflowNotFound", func() {
			reconcileUntilCondition(ctx, r, compName, ReasonWorkflowNotFound)

			c := fetchComp(ctx, compName)
			cond := conditionFor(c)
			Expect(cond.Status).To(Equal(metav1.ConditionFalse))
		})
	})
})

// ── Trait validation ──────────────────────────────────────────────────────────

var _ = Describe("Component Controller — Trait validation", func() {
	var r *Reconciler
	BeforeEach(func() {
		r = itReconciler()
	})

	Context("When Component references a Trait that exists in allowedTraits but the Trait resource is missing", func() {
		const (
			ctName    = "trait-notfound-ct"
			compName  = "trait-notfound-comp"
			wlName    = "trait-notfound-wl"
			project   = "trait-notfound-proj"
			traitName = "trait-notfound-trait"
		)
		var ct *openchoreov1alpha1.ComponentType
		var wl *openchoreov1alpha1.Workload
		var comp *openchoreov1alpha1.Component

		BeforeEach(func() {
			By("Creating ComponentType with allowedTraits=[trait-notfound-trait]")
			ct = minimalCT(ctName, "deployment")
			ct.Spec.AllowedTraits = []openchoreov1alpha1.TraitRef{
				{Kind: openchoreov1alpha1.TraitRefKindTrait, Name: traitName},
			}
			Expect(k8sClient.Create(ctx, ct)).To(Succeed())

			By("Creating Workload (needed to pass Workload validation gate)")
			wl = minimalWorkload(wlName, project, compName, "nginx:latest")
			Expect(k8sClient.Create(ctx, wl)).To(Succeed())

			By("Creating Component referencing the missing trait")
			comp = minimalComp(compName, project, string(openchoreov1alpha1.ComponentTypeRefKindComponentType),
				"deployment/"+ctName, false)
			comp.Spec.Traits = []openchoreov1alpha1.ComponentTrait{
				{Kind: openchoreov1alpha1.TraitRefKindTrait, Name: traitName, InstanceName: "inst1"},
			}
			Expect(k8sClient.Create(ctx, comp)).To(Succeed())
		})

		AfterEach(func() {
			forceDeleteObj(ctx, comp)
			_ = k8sClient.Delete(ctx, wl)
			_ = k8sClient.Delete(ctx, ct)
		})

		It("should set ConditionReady=False/TraitNotFound", func() {
			// Use Eventually: Workload cache must be populated before this path is reached
			reconcileUntilCondition(ctx, r, compName, ReasonTraitNotFound)

			c := fetchComp(ctx, compName)
			cond := conditionFor(c)
			Expect(cond.Status).To(Equal(metav1.ConditionFalse))
		})
	})

	Context("When Component references a Trait not in ComponentType allowedTraits", func() {
		const (
			ctName   = "trait-notallowed-ct"
			compName = "trait-notallowed-comp"
			wlName   = "trait-notallowed-wl"
			project  = "trait-notallowed-proj"
		)
		var ct *openchoreov1alpha1.ComponentType
		var wl *openchoreov1alpha1.Workload
		var comp *openchoreov1alpha1.Component

		BeforeEach(func() {
			By("Creating ComponentType with allowedTraits=[allowed-trait]")
			ct = minimalCT(ctName, "deployment")
			ct.Spec.AllowedTraits = []openchoreov1alpha1.TraitRef{
				{Kind: openchoreov1alpha1.TraitRefKindTrait, Name: "allowed-trait"},
			}
			Expect(k8sClient.Create(ctx, ct)).To(Succeed())

			By("Creating Workload")
			wl = minimalWorkload(wlName, project, compName, "nginx:latest")
			Expect(k8sClient.Create(ctx, wl)).To(Succeed())

			By("Creating Component referencing other-trait (not in allowedTraits)")
			comp = minimalComp(compName, project, string(openchoreov1alpha1.ComponentTypeRefKindComponentType),
				"deployment/"+ctName, false)
			comp.Spec.Traits = []openchoreov1alpha1.ComponentTrait{
				{Kind: openchoreov1alpha1.TraitRefKindTrait, Name: "other-trait", InstanceName: "inst1"},
			}
			Expect(k8sClient.Create(ctx, comp)).To(Succeed())
		})

		AfterEach(func() {
			forceDeleteObj(ctx, comp)
			_ = k8sClient.Delete(ctx, wl)
			_ = k8sClient.Delete(ctx, ct)
		})

		It("should set ConditionReady=False/InvalidConfiguration", func() {
			reconcileUntilCondition(ctx, r, compName, ReasonInvalidConfiguration)

			c := fetchComp(ctx, compName)
			cond := conditionFor(c)
			Expect(cond.Status).To(Equal(metav1.ConditionFalse))
		})
	})
})

// ── Happy path — autoDeploy=false ─────────────────────────────────────────────

var _ = Describe("Component Controller — Happy path autoDeploy=false", func() {
	var r *Reconciler
	BeforeEach(func() {
		r = itReconciler()
	})

	Context("When all dependencies are present and autoDeploy is disabled", func() {
		const (
			ctName       = "no-autodeploy-ct"
			compName     = "no-autodeploy-comp"
			wlName       = "no-autodeploy-wl"
			project      = "no-autodeploy-proj"
			pipelineName = "no-autodeploy-pipe"
		)
		var ct *openchoreov1alpha1.ComponentType
		var wl *openchoreov1alpha1.Workload
		var proj *openchoreov1alpha1.Project
		var pipe *openchoreov1alpha1.DeploymentPipeline
		var comp *openchoreov1alpha1.Component

		BeforeEach(func() {
			ct = minimalCT(ctName, "deployment")
			Expect(k8sClient.Create(ctx, ct)).To(Succeed())

			wl = minimalWorkload(wlName, project, compName, "nginx:latest")
			Expect(k8sClient.Create(ctx, wl)).To(Succeed())

			proj = minimalProject(project, pipelineName)
			Expect(k8sClient.Create(ctx, proj)).To(Succeed())

			pipe = minimalPipeline(pipelineName)
			Expect(k8sClient.Create(ctx, pipe)).To(Succeed())

			comp = minimalComp(compName, project, string(openchoreov1alpha1.ComponentTypeRefKindComponentType),
				"deployment/"+ctName, false)
			Expect(k8sClient.Create(ctx, comp)).To(Succeed())
		})

		AfterEach(func() {
			forceDeleteObj(ctx, comp)
			_ = k8sClient.Delete(ctx, pipe)
			_ = k8sClient.Delete(ctx, proj)
			_ = k8sClient.Delete(ctx, wl)
			_ = k8sClient.Delete(ctx, ct)
		})

		It("should set ConditionReady=True/Reconciled and create no ComponentRelease or ReleaseBinding", func() {
			By("Reconciling until Ready/Reconciled")
			reconcileUntilCondition(ctx, r, compName, ReasonReconciled)

			By("Verifying condition is True")
			c := fetchComp(ctx, compName)
			cond := conditionFor(c)
			Expect(cond.Status).To(Equal(metav1.ConditionTrue))

			By("Verifying no ComponentRelease was created for this component")
			releaseList := &openchoreov1alpha1.ComponentReleaseList{}
			Expect(k8sClient.List(ctx, releaseList, client.InNamespace(itNamespace),
				client.MatchingFields{"spec.owner.componentName": compName})).To(Succeed())
			Expect(releaseList.Items).To(BeEmpty())

			By("Verifying no ReleaseBinding was created for this component")
			bindingList := &openchoreov1alpha1.ReleaseBindingList{}
			Expect(k8sClient.List(ctx, bindingList, client.InNamespace(itNamespace),
				client.MatchingFields{controller.IndexKeyReleaseBindingOwnerComponentName: compName})).To(Succeed())
			Expect(bindingList.Items).To(BeEmpty())

			By("Verifying Status.LatestRelease is nil")
			Expect(c.Status.LatestRelease).To(BeNil())
		})
	})
})

// ── Happy path — autoDeploy=true ─────────────────────────────────────────────

var _ = Describe("Component Controller — Happy path autoDeploy=true", func() {
	var r *Reconciler
	BeforeEach(func() {
		r = itReconciler()
	})

	Context("When all dependencies are present and autoDeploy is enabled — first reconcile", func() {
		const (
			ctName       = "autodeploy-ct"
			compName     = "autodeploy-comp"
			wlName       = "autodeploy-wl"
			project      = "autodeploy-proj"
			pipelineName = "autodeploy-pipe"
		)
		var ct *openchoreov1alpha1.ComponentType
		var wl *openchoreov1alpha1.Workload
		var proj *openchoreov1alpha1.Project
		var pipe *openchoreov1alpha1.DeploymentPipeline
		var comp *openchoreov1alpha1.Component

		BeforeEach(func() {
			ct = minimalCT(ctName, "deployment")
			Expect(k8sClient.Create(ctx, ct)).To(Succeed())

			wl = minimalWorkload(wlName, project, compName, "nginx:latest")
			Expect(k8sClient.Create(ctx, wl)).To(Succeed())

			proj = minimalProject(project, pipelineName)
			Expect(k8sClient.Create(ctx, proj)).To(Succeed())

			pipe = minimalPipeline(pipelineName)
			Expect(k8sClient.Create(ctx, pipe)).To(Succeed())

			comp = minimalComp(compName, project, string(openchoreov1alpha1.ComponentTypeRefKindComponentType),
				"deployment/"+ctName, true)
			Expect(k8sClient.Create(ctx, comp)).To(Succeed())
		})

		AfterEach(func() {
			forceDeleteObj(ctx, comp)
			// Clean up ComponentRelease and ReleaseBinding created by autoDeploy
			releaseList := &openchoreov1alpha1.ComponentReleaseList{}
			if err := k8sClient.List(ctx, releaseList, client.InNamespace(itNamespace),
				client.MatchingFields{"spec.owner.componentName": compName}); err == nil {
				for i := range releaseList.Items {
					_ = k8sClient.Delete(ctx, &releaseList.Items[i])
				}
			}
			bindingList := &openchoreov1alpha1.ReleaseBindingList{}
			if err := k8sClient.List(ctx, bindingList, client.InNamespace(itNamespace),
				client.MatchingFields{controller.IndexKeyReleaseBindingOwnerComponentName: compName}); err == nil {
				for i := range bindingList.Items {
					_ = k8sClient.Delete(ctx, &bindingList.Items[i])
				}
			}
			_ = k8sClient.Delete(ctx, pipe)
			_ = k8sClient.Delete(ctx, proj)
			_ = k8sClient.Delete(ctx, wl)
			_ = k8sClient.Delete(ctx, ct)
		})

		It("should create ComponentRelease and ReleaseBinding and set ConditionReady=True/ComponentReleaseReady", func() {
			By("Reconciling until ComponentReleaseReady condition is set")
			reconcileUntilCondition(ctx, r, compName, ReasonComponentReleaseReady)

			By("Verifying condition is True")
			c := fetchComp(ctx, compName)
			cond := conditionFor(c)
			Expect(cond.Status).To(Equal(metav1.ConditionTrue))

			By("Verifying Status.LatestRelease is populated")
			Expect(c.Status.LatestRelease).NotTo(BeNil())
			releaseName := c.Status.LatestRelease.Name
			Expect(releaseName).To(HavePrefix(compName + "-"))
			Expect(c.Status.LatestRelease.ReleaseHash).NotTo(BeEmpty())

			By("Verifying ComponentRelease was created")
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{Name: releaseName, Namespace: itNamespace},
					&openchoreov1alpha1.ComponentRelease{})
			}, itTimeout, itInterval).Should(Succeed())

			By("Verifying ReleaseBinding was created for the root environment (dev)")
			expectedBindingName := compName + "-dev"
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{Name: expectedBindingName, Namespace: itNamespace},
					&openchoreov1alpha1.ReleaseBinding{})
			}, itTimeout, itInterval).Should(Succeed())
		})
	})

	Context("When hash is unchanged on second reconcile — no new ComponentRelease created", func() {
		const (
			ctName       = "autodeploy-dup-ct"
			compName     = "autodeploy-dup-comp"
			wlName       = "autodeploy-dup-wl"
			project      = "autodeploy-dup-proj"
			pipelineName = "autodeploy-dup-pipe"
		)
		var ct *openchoreov1alpha1.ComponentType
		var wl *openchoreov1alpha1.Workload
		var proj *openchoreov1alpha1.Project
		var pipe *openchoreov1alpha1.DeploymentPipeline
		var comp *openchoreov1alpha1.Component

		BeforeEach(func() {
			ct = minimalCT(ctName, "deployment")
			Expect(k8sClient.Create(ctx, ct)).To(Succeed())

			wl = minimalWorkload(wlName, project, compName, "nginx:latest")
			Expect(k8sClient.Create(ctx, wl)).To(Succeed())

			proj = minimalProject(project, pipelineName)
			Expect(k8sClient.Create(ctx, proj)).To(Succeed())

			pipe = minimalPipeline(pipelineName)
			Expect(k8sClient.Create(ctx, pipe)).To(Succeed())

			comp = minimalComp(compName, project, string(openchoreov1alpha1.ComponentTypeRefKindComponentType),
				"deployment/"+ctName, true)
			Expect(k8sClient.Create(ctx, comp)).To(Succeed())
		})

		AfterEach(func() {
			forceDeleteObj(ctx, comp)
			releaseList := &openchoreov1alpha1.ComponentReleaseList{}
			if err := k8sClient.List(ctx, releaseList, client.InNamespace(itNamespace),
				client.MatchingFields{"spec.owner.componentName": compName}); err == nil {
				for i := range releaseList.Items {
					_ = k8sClient.Delete(ctx, &releaseList.Items[i])
				}
			}
			bindingList := &openchoreov1alpha1.ReleaseBindingList{}
			if err := k8sClient.List(ctx, bindingList, client.InNamespace(itNamespace),
				client.MatchingFields{controller.IndexKeyReleaseBindingOwnerComponentName: compName}); err == nil {
				for i := range bindingList.Items {
					_ = k8sClient.Delete(ctx, &bindingList.Items[i])
				}
			}
			_ = k8sClient.Delete(ctx, pipe)
			_ = k8sClient.Delete(ctx, proj)
			_ = k8sClient.Delete(ctx, wl)
			_ = k8sClient.Delete(ctx, ct)
		})

		It("should not create a second ComponentRelease when spec is unchanged", func() {
			By("First reconcile creates ComponentRelease")
			reconcileUntilCondition(ctx, r, compName, ReasonComponentReleaseReady)

			c := fetchComp(ctx, compName)
			Expect(c.Status.LatestRelease).NotTo(BeNil())
			firstHash := c.Status.LatestRelease.ReleaseHash
			firstReleaseName := c.Status.LatestRelease.Name

			By("Waiting for ComponentRelease to be visible in cache")
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{Name: firstReleaseName, Namespace: itNamespace},
					&openchoreov1alpha1.ComponentRelease{})
			}, itTimeout, itInterval).Should(Succeed())

			By("Second reconcile with unchanged spec")
			req := reconcile.Request{NamespacedName: types.NamespacedName{Name: compName, Namespace: itNamespace}}
			_, err := r.Reconcile(ctx, req)
			Expect(err).NotTo(HaveOccurred())

			By("Verifying hash is still the same")
			c2 := fetchComp(ctx, compName)
			Expect(c2.Status.LatestRelease.ReleaseHash).To(Equal(firstHash))

			By("Verifying exactly one ComponentRelease exists for this component")
			Eventually(func(g Gomega) {
				releaseList := &openchoreov1alpha1.ComponentReleaseList{}
				g.Expect(k8sClient.List(ctx, releaseList, client.InNamespace(itNamespace),
					client.MatchingFields{"spec.owner.componentName": compName})).To(Succeed())
				g.Expect(releaseList.Items).To(HaveLen(1))
				g.Expect(releaseList.Items[0].Name).To(Equal(firstReleaseName))
			}, itTimeout, itInterval).Should(Succeed())
		})
	})

	Context("When Workload image changes — new ComponentRelease is created", func() {
		const (
			ctName       = "autodeploy-hashchange-ct"
			compName     = "autodeploy-hashchange-comp"
			wlName       = "autodeploy-hashchange-wl"
			project      = "autodeploy-hashchange-proj"
			pipelineName = "autodeploy-hashchange-pipe"
		)
		var ct *openchoreov1alpha1.ComponentType
		var wl *openchoreov1alpha1.Workload
		var proj *openchoreov1alpha1.Project
		var pipe *openchoreov1alpha1.DeploymentPipeline
		var comp *openchoreov1alpha1.Component

		BeforeEach(func() {
			ct = minimalCT(ctName, "deployment")
			Expect(k8sClient.Create(ctx, ct)).To(Succeed())

			wl = minimalWorkload(wlName, project, compName, "nginx:1.19")
			Expect(k8sClient.Create(ctx, wl)).To(Succeed())

			proj = minimalProject(project, pipelineName)
			Expect(k8sClient.Create(ctx, proj)).To(Succeed())

			pipe = minimalPipeline(pipelineName)
			Expect(k8sClient.Create(ctx, pipe)).To(Succeed())

			comp = minimalComp(compName, project, string(openchoreov1alpha1.ComponentTypeRefKindComponentType),
				"deployment/"+ctName, true)
			Expect(k8sClient.Create(ctx, comp)).To(Succeed())
		})

		AfterEach(func() {
			forceDeleteObj(ctx, comp)
			releaseList := &openchoreov1alpha1.ComponentReleaseList{}
			if err := k8sClient.List(ctx, releaseList, client.InNamespace(itNamespace),
				client.MatchingFields{"spec.owner.componentName": compName}); err == nil {
				for i := range releaseList.Items {
					_ = k8sClient.Delete(ctx, &releaseList.Items[i])
				}
			}
			bindingList := &openchoreov1alpha1.ReleaseBindingList{}
			if err := k8sClient.List(ctx, bindingList, client.InNamespace(itNamespace),
				client.MatchingFields{controller.IndexKeyReleaseBindingOwnerComponentName: compName}); err == nil {
				for i := range bindingList.Items {
					_ = k8sClient.Delete(ctx, &bindingList.Items[i])
				}
			}
			_ = k8sClient.Delete(ctx, pipe)
			_ = k8sClient.Delete(ctx, proj)
			_ = k8sClient.Delete(ctx, wl)
			_ = k8sClient.Delete(ctx, ct)
		})

		It("should create a new ComponentRelease with a different hash after image change", func() {
			By("First reconcile creates initial ComponentRelease")
			reconcileUntilCondition(ctx, r, compName, ReasonComponentReleaseReady)

			c := fetchComp(ctx, compName)
			Expect(c.Status.LatestRelease).NotTo(BeNil())
			firstHash := c.Status.LatestRelease.ReleaseHash
			firstReleaseName := c.Status.LatestRelease.Name

			By("Updating Workload image to trigger hash change")
			updatedWl := &openchoreov1alpha1.Workload{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: wlName, Namespace: itNamespace}, updatedWl)).To(Succeed())
			updatedWl.Spec.Container.Image = "nginx:1.21"
			Expect(k8sClient.Update(ctx, updatedWl)).To(Succeed())

			By("Reconciling until a new hash appears in Status.LatestRelease")
			req := reconcile.Request{NamespacedName: types.NamespacedName{Name: compName, Namespace: itNamespace}}
			Eventually(func(g Gomega) {
				_, err := r.Reconcile(ctx, req)
				g.Expect(err).NotTo(HaveOccurred())
				c2 := fetchComp(ctx, compName)
				g.Expect(c2).NotTo(BeNil())
				g.Expect(c2.Status.LatestRelease).NotTo(BeNil())
				g.Expect(c2.Status.LatestRelease.ReleaseHash).NotTo(Equal(firstHash))
			}, itTimeout, itInterval).Should(Succeed())

			By("Verifying Status.LatestRelease.Name has changed")
			c3 := fetchComp(ctx, compName)
			Expect(c3.Status.LatestRelease.Name).NotTo(Equal(firstReleaseName))

			By("Verifying new ComponentRelease exists")
			newReleaseName := c3.Status.LatestRelease.Name
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{Name: newReleaseName, Namespace: itNamespace},
					&openchoreov1alpha1.ComponentRelease{})
			}, itTimeout, itInterval).Should(Succeed())

			By("Verifying old ComponentRelease still exists (not auto-deleted)")
			err := k8sClient.Get(ctx, types.NamespacedName{Name: firstReleaseName, Namespace: itNamespace},
				&openchoreov1alpha1.ComponentRelease{})
			Expect(k8serrors.IsNotFound(err)).To(BeFalse(),
				"old ComponentRelease %q should still exist", firstReleaseName)
		})
	})
})
