// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package releasebinding

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/labels"
	componentpipeline "github.com/openchoreo/openchoreo/internal/pipeline/component"
)

// ─── Minimal Templates ───────────────────────────────────────────────────────
//
// minimalTemplate is the minimal valid Kubernetes Deployment JSON required by
// the +kubebuilder:validation:Required constraint on ResourceTemplate.Template.
// It also satisfies pipeline.validateResources (apiVersion + kind + metadata.name).
var minimalTemplate = &runtime.RawExtension{
	Raw: []byte(`{` +
		`"apiVersion":"apps/v1",` +
		`"kind":"Deployment",` +
		`"metadata":{"name":"test-deployment"},` +
		`"spec":{` +
		`"selector":{"matchLabels":{"app":"test"}},` +
		`"template":{` +
		`"metadata":{"labels":{"app":"test"}},` +
		`"spec":{"containers":[{"name":"app","image":"nginx:latest"}]}` +
		`}}}`,
	),
}

// ─── Test Fixtures ────────────────────────────────────────────────────────────

// rbFixture returns a ReleaseBinding.  When withFinalizer is true the
// finalizer is pre-seeded so that the first Reconcile skips straight to
// business logic instead of just adding the finalizer and returning early.
func rbFixture(name, project, component, envName, releaseName string, withFinalizer bool) *openchoreov1alpha1.ReleaseBinding {
	rb := &openchoreov1alpha1.ReleaseBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
		},
		Spec: openchoreov1alpha1.ReleaseBindingSpec{
			Owner: openchoreov1alpha1.ReleaseBindingOwner{
				ProjectName:   project,
				ComponentName: component,
			},
			Environment: envName,
			ReleaseName: releaseName,
		},
	}
	if withFinalizer {
		rb.Finalizers = []string{ReleaseBindingFinalizer}
	}
	return rb
}

// crFixture returns a valid ComponentRelease whose owners match the given
// project/component.  The ResourceTemplate carries minimalTemplate (required
// by CRD validation) and the Workload carries a minimal container spec (the
// XValidation rule requires exactly one of container/containers to be set).
func crFixture(name, project, component string) *openchoreov1alpha1.ComponentRelease {
	return &openchoreov1alpha1.ComponentRelease{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
		},
		Spec: openchoreov1alpha1.ComponentReleaseSpec{
			Owner: openchoreov1alpha1.ComponentReleaseOwner{
				ProjectName:   project,
				ComponentName: component,
			},
			ComponentType: openchoreov1alpha1.ComponentTypeSpec{
				WorkloadType: "deployment",
				Resources: []openchoreov1alpha1.ResourceTemplate{
					{ID: "deployment", Template: minimalTemplate},
				},
			},
			// WorkloadTemplateSpec requires exactly one of container or containers.
			Workload: openchoreov1alpha1.WorkloadTemplateSpec{
				Containers: map[string]openchoreov1alpha1.Container{
					"app": {Image: "nginx:latest"},
				},
			},
		},
	}
}

// envFixture returns a minimal Environment.  When dpName is non-empty the
// Environment carries an explicit DataPlaneRef; otherwise the controller
// defaults to a DataPlane named "default" in the same namespace.
func envFixture(name, dpName string) *openchoreov1alpha1.Environment {
	env := &openchoreov1alpha1.Environment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
		},
	}
	if dpName != "" {
		env.Spec.DataPlaneRef = &openchoreov1alpha1.DataPlaneRef{
			Kind: openchoreov1alpha1.DataPlaneRefKindDataPlane,
			Name: dpName,
		}
	}
	return env
}

// dpFixture returns a minimal DataPlane.
func dpFixture(name string) *openchoreov1alpha1.DataPlane {
	return &openchoreov1alpha1.DataPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
		},
	}
}

// componentFixture returns a minimal Component.
// ComponentType.Name must match '^(deployment|statefulset|cronjob|job|proxy)/...' (CRD validation).
func componentFixture(name, project string) *openchoreov1alpha1.Component {
	return &openchoreov1alpha1.Component{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
		},
		Spec: openchoreov1alpha1.ComponentSpec{
			Owner: openchoreov1alpha1.ComponentOwner{
				ProjectName: project,
			},
			ComponentType: openchoreov1alpha1.ComponentTypeRef{
				Name: "deployment/my-service",
			},
		},
	}
}

// projectFixture returns a minimal Project.
func projectFixture(name string) *openchoreov1alpha1.Project {
	return &openchoreov1alpha1.Project{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
		},
	}
}

// releaseFixture returns a Release pre-labeled so that hasOwnedReleases
// will match it for the given ReleaseBinding owner fields.
func releaseFixture(name, namespace, project, component, envName string, extraFinalizers ...string) *openchoreov1alpha1.Release {
	rel := &openchoreov1alpha1.Release{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				labels.LabelKeyProjectName:     project,
				labels.LabelKeyComponentName:   component,
				labels.LabelKeyEnvironmentName: envName,
			},
		},
		Spec: openchoreov1alpha1.ReleaseSpec{
			Owner: openchoreov1alpha1.ReleaseOwner{
				ProjectName:   project,
				ComponentName: component,
			},
			EnvironmentName: envName,
			TargetPlane:     openchoreov1alpha1.TargetPlaneDataPlane,
		},
	}
	if len(extraFinalizers) > 0 {
		rel.Finalizers = extraFinalizers
	}
	return rel
}

// ─── Test Helpers ─────────────────────────────────────────────────────────────

// testReconciler returns a Reconciler wired to the global envtest client.
// Pipeline is intentionally nil — use testReconcilerWithPipeline for tests
// that need to exercise the rendering path.
func testReconciler() *Reconciler {
	return &Reconciler{
		Client: k8sClient,
		Scheme: k8sClient.Scheme(),
	}
}

// testReconcilerWithPipeline returns a Reconciler with a real component
// rendering Pipeline.  This unlocks reconcileRelease → r.Pipeline.Render(...)
// and all code paths beyond the undeploy check.
func testReconcilerWithPipeline() *Reconciler {
	return &Reconciler{
		Client:   k8sClient,
		Scheme:   k8sClient.Scheme(),
		Pipeline: componentpipeline.NewPipeline(),
	}
}

// reconcileRequest builds a reconcile.Request for the given name in the default test namespace.
func reconcileRequest(name string) reconcile.Request {
	return reconcile.Request{NamespacedName: types.NamespacedName{Namespace: ns, Name: name}}
}

// mustReconcile calls Reconcile and asserts no error is returned.
func mustReconcile(r *Reconciler, req reconcile.Request) reconcile.Result {
	GinkgoHelper()
	result, err := r.Reconcile(ctx, req)
	Expect(err).NotTo(HaveOccurred())
	return result
}

// fetchRB re-reads a ReleaseBinding from the API server.
func fetchRB(name string) *openchoreov1alpha1.ReleaseBinding {
	GinkgoHelper()
	rb := &openchoreov1alpha1.ReleaseBinding{}
	Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: name}, rb)).To(Succeed())
	return rb
}

// conditionFor finds a status condition by type; returns nil if absent.
func conditionFor(rb *openchoreov1alpha1.ReleaseBinding, condType string) *metav1.Condition {
	return apimeta.FindStatusCondition(rb.Status.Conditions, condType)
}

// forceDelete strips all finalizers from a ReleaseBinding in the default test namespace
// and then deletes it. Call this in AfterEach to guarantee cleanup even when a finalizer is present.
func forceDelete(name string) {
	rb := &openchoreov1alpha1.ReleaseBinding{}
	if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: name}, rb); err != nil {
		return // already gone
	}
	rb.Finalizers = nil
	_ = k8sClient.Update(ctx, rb)
	_ = k8sClient.Delete(ctx, rb)
}

// forceDeleteRelease strips all finalizers from a Release and then deletes it.
func forceDeleteRelease(name string) {
	rel := &openchoreov1alpha1.Release{}
	if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: name}, rel); err != nil {
		return
	}
	rel.Finalizers = nil
	_ = k8sClient.Update(ctx, rel)
	_ = k8sClient.Delete(ctx, rel)
}

// ─── Tests ────────────────────────────────────────────────────────────────────

const (
	ns       = "default"
	timeout  = 10 * time.Second
	interval = 250 * time.Millisecond
)

var _ = Describe("ReleaseBinding Controller", func() {
	// All test resources live in the "default" namespace so that the suite
	// can use the standard envtest setup without creating additional namespaces.
	// Each Context uses distinct resource names to prevent inter-test interference.

	// ── Finalizer management ──────────────────────────────────────────────────

	Context("Finalizer management", func() {
		const rbName = "rb-finalizer-test"
		req := reconcileRequest(rbName)

		AfterEach(func() { forceDelete(rbName) })

		It("adds the finalizer on the first reconcile and returns early", func() {
			r := testReconciler()

			By("Creating a ReleaseBinding without a finalizer")
			Expect(k8sClient.Create(ctx,
				rbFixture(rbName, "proj", "comp", "env", "rel", false),
			)).To(Succeed())

			By("First reconcile adds the finalizer and returns without error")
			mustReconcile(r, req)

			By("Verifying the finalizer is present after the first reconcile")
			rb := fetchRB(rbName)
			Expect(rb.Finalizers).To(ContainElement(ReleaseBindingFinalizer))
		})

		It("does not add the finalizer more than once on repeated reconciles", func() {
			r := testReconciler()

			By("Creating a ReleaseBinding without a finalizer")
			Expect(k8sClient.Create(ctx,
				rbFixture(rbName, "proj", "comp", "env", "rel", false),
			)).To(Succeed())

			By("First reconcile adds the finalizer")
			mustReconcile(r, req)

			By("Second reconcile is a no-op for the finalizer")
			mustReconcile(r, req)

			By("Verifying the finalizer appears exactly once")
			rb := fetchRB(rbName)
			count := 0
			for _, f := range rb.Finalizers {
				if f == ReleaseBindingFinalizer {
					count++
				}
			}
			Expect(count).To(Equal(1), "finalizer should appear exactly once")
		})
	})

	// ── Missing ComponentRelease ──────────────────────────────────────────────

	Context("when the referenced ComponentRelease does not exist", func() {
		const rbName = "rb-no-cr"
		req := reconcileRequest(rbName)

		AfterEach(func() { forceDelete(rbName) })

		It("sets ReleaseSynced=False with reason ComponentReleaseNotFound", func() {
			r := testReconciler()

			By("Creating a ReleaseBinding whose ReleaseName points to a non-existent CR")
			Expect(k8sClient.Create(ctx,
				rbFixture(rbName, "proj", "comp", "env", "does-not-exist", true),
			)).To(Succeed())

			By("Reconciling — the controller should detect the missing ComponentRelease")
			mustReconcile(r, req)

			By("Verifying ReleaseSynced=False with ComponentReleaseNotFound reason")
			rb := fetchRB(rbName)
			cond := conditionFor(rb, string(ConditionReleaseSynced))
			Expect(cond).NotTo(BeNil(), "ReleaseSynced condition must be set")
			Expect(cond.Status).To(Equal(metav1.ConditionFalse))
			Expect(cond.Reason).To(Equal(string(ReasonComponentReleaseNotFound)))
		})
	})

	// ── Invalid ComponentRelease (owner mismatch) ─────────────────────────────

	Context("when the ComponentRelease has mismatched owners", func() {
		const (
			rbName = "rb-bad-cr"
			crName = "cr-bad-cr"
		)
		req := reconcileRequest(rbName)

		AfterEach(func() {
			forceDelete(rbName)
			_ = k8sClient.Delete(ctx, &openchoreov1alpha1.ComponentRelease{
				ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: crName},
			})
		})

		It("sets ReleaseSynced=False with reason InvalidReleaseConfiguration", func() {
			r := testReconciler()

			By("Creating a ComponentRelease owned by 'other-project'")
			Expect(k8sClient.Create(ctx, crFixture(crName, "other-project", "comp"))).To(Succeed())

			By("Creating a ReleaseBinding owned by 'my-project' that references the above CR")
			Expect(k8sClient.Create(ctx,
				rbFixture(rbName, "my-project", "comp", "env", crName, true),
			)).To(Succeed())

			By("Reconciling — the controller should detect the owner mismatch")
			mustReconcile(r, req)

			By("Verifying ReleaseSynced=False with InvalidReleaseConfiguration reason")
			rb := fetchRB(rbName)
			cond := conditionFor(rb, string(ConditionReleaseSynced))
			Expect(cond).NotTo(BeNil())
			Expect(cond.Status).To(Equal(metav1.ConditionFalse))
			Expect(cond.Reason).To(Equal(string(ReasonInvalidReleaseConfiguration)))
		})
	})

	// ── Missing Environment ───────────────────────────────────────────────────

	Context("when the referenced Environment does not exist", func() {
		const (
			rbName = "rb-no-env"
			crName = "cr-no-env"
		)
		req := reconcileRequest(rbName)

		AfterEach(func() {
			forceDelete(rbName)
			_ = k8sClient.Delete(ctx, &openchoreov1alpha1.ComponentRelease{
				ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: crName},
			})
		})

		It("sets ReleaseSynced=False with reason EnvironmentNotFound", func() {
			r := testReconciler()

			By("Creating a valid ComponentRelease")
			Expect(k8sClient.Create(ctx, crFixture(crName, "proj", "comp"))).To(Succeed())

			By("Creating a ReleaseBinding that references a non-existent Environment")
			Expect(k8sClient.Create(ctx,
				rbFixture(rbName, "proj", "comp", "does-not-exist-env", crName, true),
			)).To(Succeed())

			By("Reconciling — the controller should detect the missing Environment")
			mustReconcile(r, req)

			By("Verifying ReleaseSynced=False with EnvironmentNotFound reason")
			rb := fetchRB(rbName)
			cond := conditionFor(rb, string(ConditionReleaseSynced))
			Expect(cond).NotTo(BeNil())
			Expect(cond.Status).To(Equal(metav1.ConditionFalse))
			Expect(cond.Reason).To(Equal(string(ReasonEnvironmentNotFound)))
		})
	})

	// ── Missing DataPlane ─────────────────────────────────────────────────────

	Context("when the referenced DataPlane does not exist", func() {
		const (
			rbName  = "rb-no-dp"
			crName  = "cr-no-dp"
			envName = "env-no-dp"
		)
		req := reconcileRequest(rbName)

		AfterEach(func() {
			forceDelete(rbName)
			_ = k8sClient.Delete(ctx, &openchoreov1alpha1.ComponentRelease{
				ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: crName},
			})
			_ = k8sClient.Delete(ctx, &openchoreov1alpha1.Environment{
				ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: envName},
			})
		})

		It("sets ReleaseSynced=False with reason DataPlaneNotFound", func() {
			r := testReconciler()

			By("Creating a valid ComponentRelease")
			Expect(k8sClient.Create(ctx, crFixture(crName, "proj", "comp"))).To(Succeed())

			By("Creating an Environment with an explicit DataPlaneRef to a non-existent DataPlane")
			Expect(k8sClient.Create(ctx, envFixture(envName, "does-not-exist-dp"))).To(Succeed())

			By("Creating the ReleaseBinding")
			Expect(k8sClient.Create(ctx,
				rbFixture(rbName, "proj", "comp", envName, crName, true),
			)).To(Succeed())

			By("Reconciling — the controller should detect the missing DataPlane")
			mustReconcile(r, req)

			By("Verifying ReleaseSynced=False with DataPlaneNotFound reason")
			rb := fetchRB(rbName)
			cond := conditionFor(rb, string(ConditionReleaseSynced))
			Expect(cond).NotTo(BeNil())
			Expect(cond.Status).To(Equal(metav1.ConditionFalse))
			Expect(cond.Reason).To(Equal(string(ReasonDataPlaneNotFound)))
		})
	})

	// ── Missing Component ─────────────────────────────────────────────────────

	Context("when the referenced Component does not exist", func() {
		const (
			rbName  = "rb-no-comp"
			crName  = "cr-no-comp"
			envName = "env-no-comp"
			dpName  = "dp-no-comp"
		)
		req := reconcileRequest(rbName)

		AfterEach(func() {
			forceDelete(rbName)
			_ = k8sClient.Delete(ctx, &openchoreov1alpha1.ComponentRelease{
				ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: crName},
			})
			_ = k8sClient.Delete(ctx, &openchoreov1alpha1.Environment{
				ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: envName},
			})
			_ = k8sClient.Delete(ctx, &openchoreov1alpha1.DataPlane{
				ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: dpName},
			})
		})

		It("sets ReleaseSynced=False with reason ComponentNotFound", func() {
			r := testReconciler()

			By("Creating a ComponentRelease for a component that will not be created")
			Expect(k8sClient.Create(ctx, crFixture(crName, "proj", "does-not-exist-comp"))).To(Succeed())

			By("Creating DataPlane and Environment")
			Expect(k8sClient.Create(ctx, dpFixture(dpName))).To(Succeed())
			Expect(k8sClient.Create(ctx, envFixture(envName, dpName))).To(Succeed())

			By("Creating the ReleaseBinding (no matching Component created)")
			Expect(k8sClient.Create(ctx,
				rbFixture(rbName, "proj", "does-not-exist-comp", envName, crName, true),
			)).To(Succeed())

			By("Reconciling — the controller should detect the missing Component")
			mustReconcile(r, req)

			By("Verifying ReleaseSynced=False with ComponentNotFound reason")
			rb := fetchRB(rbName)
			cond := conditionFor(rb, string(ConditionReleaseSynced))
			Expect(cond).NotTo(BeNil())
			Expect(cond.Status).To(Equal(metav1.ConditionFalse))
			Expect(cond.Reason).To(Equal(string(ReasonComponentNotFound)))
		})
	})

	// ── Missing Project ───────────────────────────────────────────────────────

	Context("when the referenced Project does not exist", func() {
		const (
			rbName   = "rb-no-proj"
			crName   = "cr-no-proj"
			envName  = "env-no-proj"
			dpName   = "dp-no-proj"
			compName = "comp-no-proj"
		)
		req := reconcileRequest(rbName)

		AfterEach(func() {
			forceDelete(rbName)
			_ = k8sClient.Delete(ctx, &openchoreov1alpha1.ComponentRelease{
				ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: crName},
			})
			_ = k8sClient.Delete(ctx, &openchoreov1alpha1.Environment{
				ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: envName},
			})
			_ = k8sClient.Delete(ctx, &openchoreov1alpha1.DataPlane{
				ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: dpName},
			})
			_ = k8sClient.Delete(ctx, &openchoreov1alpha1.Component{
				ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: compName},
			})
		})

		It("sets ReleaseSynced=False with reason ProjectNotFound", func() {
			r := testReconciler()

			By("Creating all deps except the Project itself")
			Expect(k8sClient.Create(ctx, crFixture(crName, "does-not-exist-proj", compName))).To(Succeed())
			Expect(k8sClient.Create(ctx, dpFixture(dpName))).To(Succeed())
			Expect(k8sClient.Create(ctx, envFixture(envName, dpName))).To(Succeed())
			Expect(k8sClient.Create(ctx, componentFixture(compName, "does-not-exist-proj"))).To(Succeed())

			By("Creating the ReleaseBinding (no matching Project created)")
			Expect(k8sClient.Create(ctx,
				rbFixture(rbName, "does-not-exist-proj", compName, envName, crName, true),
			)).To(Succeed())

			By("Reconciling — the controller should detect the missing Project")
			mustReconcile(r, req)

			By("Verifying ReleaseSynced=False with ProjectNotFound reason")
			rb := fetchRB(rbName)
			cond := conditionFor(rb, string(ConditionReleaseSynced))
			Expect(cond).NotTo(BeNil())
			Expect(cond.Status).To(Equal(metav1.ConditionFalse))
			Expect(cond.Reason).To(Equal(string(ReasonProjectNotFound)))
		})
	})

	// ── Happy path: Release creation ──────────────────────────────────────────
	//
	// This context exercises the full rendering path by wiring a real
	// componentpipeline.Pipeline into the reconciler.  The ComponentRelease
	// carries a static Deployment template (no CEL expressions) so the pipeline
	// produces a deterministic output.

	Context("when all dependencies are present and State is not Undeploy", func() {
		const (
			project  = "happy-proj"
			compName = "happy-comp"
			envName  = "happy-env"
			dpName   = "happy-dp"
			rbName   = "rb-happy"
			crName   = "cr-happy"
		)
		req := reconcileRequest(rbName)
		expectedReleaseName := compName + "-" + envName // makeDataPlaneReleaseName format

		AfterEach(func() {
			forceDelete(rbName)
			forceDeleteRelease(expectedReleaseName)
			_ = k8sClient.Delete(ctx, &openchoreov1alpha1.ComponentRelease{
				ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: crName},
			})
			_ = k8sClient.Delete(ctx, &openchoreov1alpha1.Environment{
				ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: envName},
			})
			_ = k8sClient.Delete(ctx, &openchoreov1alpha1.DataPlane{
				ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: dpName},
			})
			_ = k8sClient.Delete(ctx, &openchoreov1alpha1.Component{
				ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: compName},
			})
			_ = k8sClient.Delete(ctx, &openchoreov1alpha1.Project{
				ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: project},
			})
		})

		It("creates a Release and sets ReleaseSynced=True", func() {
			r := testReconcilerWithPipeline()

			By("Creating all five dependencies (CR, dp, env, component, project)")
			Expect(k8sClient.Create(ctx, crFixture(crName, project, compName))).To(Succeed())
			Expect(k8sClient.Create(ctx, dpFixture(dpName))).To(Succeed())
			Expect(k8sClient.Create(ctx, envFixture(envName, dpName))).To(Succeed())
			Expect(k8sClient.Create(ctx, componentFixture(compName, project))).To(Succeed())
			Expect(k8sClient.Create(ctx, projectFixture(project))).To(Succeed())

			By("Creating the ReleaseBinding with the finalizer pre-set")
			Expect(k8sClient.Create(ctx,
				rbFixture(rbName, project, compName, envName, crName, true),
			)).To(Succeed())

			By("First reconcile: renders resources and creates the dataplane Release")
			result := mustReconcile(r, req)
			Expect(result.Requeue).To(BeTrue(),
				"first reconcile should request a requeue after creating the Release")

			By("Verifying the dataplane Release object was created with the expected name")
			createdRelease := &openchoreov1alpha1.Release{}
			Expect(k8sClient.Get(ctx,
				types.NamespacedName{Namespace: ns, Name: expectedReleaseName},
				createdRelease,
			)).To(Succeed(), "Release %q should exist after first reconcile", expectedReleaseName)

			By("Verifying the Release carries the expected owner labels")
			Expect(createdRelease.Labels).To(HaveKeyWithValue(labels.LabelKeyProjectName, project))
			Expect(createdRelease.Labels).To(HaveKeyWithValue(labels.LabelKeyComponentName, compName))
			Expect(createdRelease.Labels).To(HaveKeyWithValue(labels.LabelKeyEnvironmentName, envName))

			By("Verifying ReleaseSynced=True with ReasonReleaseCreated after first reconcile")
			rb := fetchRB(rbName)
			cond := conditionFor(rb, string(ConditionReleaseSynced))
			Expect(cond).NotTo(BeNil())
			Expect(cond.Status).To(Equal(metav1.ConditionTrue))
			Expect(cond.Reason).To(Equal(string(ReasonReleaseCreated)))

			By("Second reconcile (requeue): Release already exists — OperationResultNone path")
			result = mustReconcile(r, req)
			Expect(result.Requeue).To(BeFalse(),
				"second reconcile should not request requeue when Release is unchanged")

			By("Verifying ReleaseSynced=True with ReasonReleaseSynced after second reconcile")
			rb = fetchRB(rbName)
			cond = conditionFor(rb, string(ConditionReleaseSynced))
			Expect(cond).NotTo(BeNil())
			Expect(cond.Status).To(Equal(metav1.ConditionTrue))
			Expect(cond.Reason).To(Equal(string(ReasonReleaseSynced)))

			By("Verifying ResourcesReady=False/Progressing — Release status is empty (no data plane running)")
			cond = conditionFor(rb, string(ConditionResourcesReady))
			Expect(cond).NotTo(BeNil())
			Expect(cond.Status).To(Equal(metav1.ConditionFalse))
			Expect(cond.Reason).To(Equal(string(ReasonResourcesProgressing)))
		})
	})

	// ── Undeploy: no existing Release ────────────────────────────────────────

	Context("when State=Undeploy and no Release resources exist", func() {
		const (
			project  = "undeploy-proj"
			compName = "undeploy-comp"
			envName  = "undeploy-env"
			dpName   = "undeploy-dp"
			rbName   = "rb-undeploy"
			crName   = "cr-undeploy"
		)
		req := reconcileRequest(rbName)

		AfterEach(func() {
			forceDelete(rbName)
			_ = k8sClient.Delete(ctx, &openchoreov1alpha1.ComponentRelease{
				ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: crName},
			})
			_ = k8sClient.Delete(ctx, &openchoreov1alpha1.Environment{
				ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: envName},
			})
			_ = k8sClient.Delete(ctx, &openchoreov1alpha1.DataPlane{
				ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: dpName},
			})
			_ = k8sClient.Delete(ctx, &openchoreov1alpha1.Component{
				ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: compName},
			})
			_ = k8sClient.Delete(ctx, &openchoreov1alpha1.Project{
				ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: project},
			})
		})

		It("sets ReleaseSynced, ResourcesReady and Ready=False with ResourcesUndeployed reason", func() {
			r := testReconciler()

			By("Creating all dependencies")
			Expect(k8sClient.Create(ctx, crFixture(crName, project, compName))).To(Succeed())
			Expect(k8sClient.Create(ctx, dpFixture(dpName))).To(Succeed())
			Expect(k8sClient.Create(ctx, envFixture(envName, dpName))).To(Succeed())
			Expect(k8sClient.Create(ctx, componentFixture(compName, project))).To(Succeed())
			Expect(k8sClient.Create(ctx, projectFixture(project))).To(Succeed())

			By("Creating the ReleaseBinding with State=Undeploy")
			rb := rbFixture(rbName, project, compName, envName, crName, true)
			rb.Spec.State = openchoreov1alpha1.ReleaseStateUndeploy
			Expect(k8sClient.Create(ctx, rb)).To(Succeed())

			By("Reconciling — no Release exists, should mark as undeployed immediately")
			mustReconcile(r, req)

			By("Verifying all three conditions are False with ResourcesUndeployed reason")
			updated := fetchRB(rbName)
			for _, condType := range []string{
				string(ConditionReleaseSynced),
				string(ConditionResourcesReady),
				string(ConditionReady),
			} {
				cond := conditionFor(updated, condType)
				Expect(cond).NotTo(BeNil(), "condition %s should be set", condType)
				Expect(cond.Status).To(Equal(metav1.ConditionFalse), "condition %s should be False", condType)
				Expect(cond.Reason).To(Equal(string(ReasonResourcesUndeployed)),
					"condition %s should carry reason ResourcesUndeployed", condType)
			}
		})
	})

	// ── Undeploy: existing Release gets deleted ───────────────────────────────

	Context("when State=Undeploy and a Release exists", func() {
		const (
			project  = "undeploy-existing-proj"
			compName = "undeploy-existing-comp"
			envName  = "undeploy-existing-env"
			dpName   = "undeploy-existing-dp"
			rbName   = "rb-undeploy-existing"
			crName   = "cr-undeploy-existing"
		)
		// makeDataPlaneReleaseName format: {component}-{env}
		releaseName := compName + "-" + envName
		req := reconcileRequest(rbName)

		AfterEach(func() {
			forceDelete(rbName)
			forceDeleteRelease(releaseName)
			_ = k8sClient.Delete(ctx, &openchoreov1alpha1.ComponentRelease{
				ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: crName},
			})
			_ = k8sClient.Delete(ctx, &openchoreov1alpha1.Environment{
				ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: envName},
			})
			_ = k8sClient.Delete(ctx, &openchoreov1alpha1.DataPlane{
				ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: dpName},
			})
			_ = k8sClient.Delete(ctx, &openchoreov1alpha1.Component{
				ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: compName},
			})
			_ = k8sClient.Delete(ctx, &openchoreov1alpha1.Project{
				ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: project},
			})
		})

		It("deletes the Release and sets conditions to being-undeployed, then undeployed", func() {
			r := testReconciler()

			By("Creating all dependencies")
			Expect(k8sClient.Create(ctx, crFixture(crName, project, compName))).To(Succeed())
			Expect(k8sClient.Create(ctx, dpFixture(dpName))).To(Succeed())
			Expect(k8sClient.Create(ctx, envFixture(envName, dpName))).To(Succeed())
			Expect(k8sClient.Create(ctx, componentFixture(compName, project))).To(Succeed())
			Expect(k8sClient.Create(ctx, projectFixture(project))).To(Succeed())

			By("Creating the ReleaseBinding with State=Undeploy")
			rb := rbFixture(rbName, project, compName, envName, crName, true)
			rb.Spec.State = openchoreov1alpha1.ReleaseStateUndeploy
			Expect(k8sClient.Create(ctx, rb)).To(Succeed())

			By("Pre-creating a Release with the matching labels so handleUndeploy finds it")
			Expect(k8sClient.Create(ctx,
				releaseFixture(releaseName, ns, project, compName, envName),
			)).To(Succeed())

			By("First reconcile: handleUndeploy deletes the Release and sets being-undeployed conditions")
			mustReconcile(r, req)

			By("Verifying the Release is marked for deletion")
			Eventually(func() bool {
				existing := &openchoreov1alpha1.Release{}
				if err := k8sClient.Get(ctx,
					types.NamespacedName{Namespace: ns, Name: releaseName},
					existing,
				); err != nil {
					return apierrors.IsNotFound(err) // already gone is also fine
				}
				return !existing.DeletionTimestamp.IsZero()
			}, timeout, interval).Should(BeTrue(),
				"Release should be marked for deletion after undeploy reconcile")

			By("Verifying the conditions indicate 'being undeployed'")
			updated := fetchRB(rbName)
			for _, condType := range []string{
				string(ConditionReleaseSynced),
				string(ConditionResourcesReady),
				string(ConditionReady),
			} {
				cond := conditionFor(updated, condType)
				Expect(cond).NotTo(BeNil(), "condition %s should be set", condType)
				Expect(cond.Status).To(Equal(metav1.ConditionFalse))
				Expect(cond.Reason).To(Equal(string(ReasonResourcesUndeployed)))
			}

			By("Waiting for the Release to be fully deleted (it has no finalizer)")
			Eventually(func() bool {
				err := k8sClient.Get(ctx,
					types.NamespacedName{Namespace: ns, Name: releaseName},
					&openchoreov1alpha1.Release{},
				)
				return apierrors.IsNotFound(err)
			}, timeout, interval).Should(BeTrue(), "Release should be deleted")

			By("Second reconcile: no Release exists — marks as undeployed")
			mustReconcile(r, req)

			By("Verifying all conditions are False with ResourcesUndeployed reason")
			updated = fetchRB(rbName)
			for _, condType := range []string{
				string(ConditionReleaseSynced),
				string(ConditionResourcesReady),
				string(ConditionReady),
			} {
				cond := conditionFor(updated, condType)
				Expect(cond).NotTo(BeNil(), "condition %s should be set", condType)
				Expect(cond.Status).To(Equal(metav1.ConditionFalse))
				Expect(cond.Reason).To(Equal(string(ReasonResourcesUndeployed)))
			}
		})
	})

	// ── Finalization: no owned Releases ───────────────────────────────────────

	Context("Finalization with no owned Releases", func() {
		const rbName = "rb-finalize-empty"
		req := reconcileRequest(rbName)

		It("sets Finalizing condition then removes the finalizer when no Releases exist", func() {
			r := testReconciler()

			By("Creating a ReleaseBinding with the finalizer pre-set")
			rb := rbFixture(rbName, "proj", "comp", "env", "rel", true)
			Expect(k8sClient.Create(ctx, rb)).To(Succeed())

			By("Triggering deletion — DeletionTimestamp is set; the finalizer keeps the object alive")
			Expect(k8sClient.Delete(ctx, rb)).To(Succeed())

			By("Verifying the object is marked for deletion but still present")
			Eventually(func() bool {
				updated := &openchoreov1alpha1.ReleaseBinding{}
				if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: rbName}, updated); err != nil {
					return false
				}
				return !updated.DeletionTimestamp.IsZero()
			}, timeout, interval).Should(BeTrue())

			By("First finalize reconcile: sets the Finalizing=True condition and returns early")
			mustReconcile(r, req)

			By("Verifying the Finalizing condition is True after the first finalize reconcile")
			updated := fetchRB(rbName)
			cond := conditionFor(updated, string(ConditionFinalizing))
			Expect(cond).NotTo(BeNil(), "Finalizing condition must be present")
			Expect(cond.Status).To(Equal(metav1.ConditionTrue))
			Expect(updated.Finalizers).To(ContainElement(ReleaseBindingFinalizer),
				"finalizer must still be present after first finalize reconcile")

			By("Second finalize reconcile: no owned Releases found — removes finalizer")
			mustReconcile(r, req)

			By("Verifying the ReleaseBinding is deleted after the finalizer is removed")
			Eventually(func() bool {
				err := k8sClient.Get(ctx,
					types.NamespacedName{Namespace: ns, Name: rbName},
					&openchoreov1alpha1.ReleaseBinding{},
				)
				return apierrors.IsNotFound(err)
			}, timeout, interval).Should(BeTrue(),
				"ReleaseBinding should be fully deleted after finalizer removal")
		})
	})

	// ── Finalization: blocked by Release with finalizer ───────────────────────
	//
	// Mirrors the "blocked child" pattern from the component controller tests.
	// The ReleaseBinding should wait for its owned Release to be fully deleted
	// before removing its own finalizer.

	Context("Finalization blocked by an owned Release that has its own finalizer", func() {
		const (
			blockFinalizer = "test.openchoreo.dev/block-deletion"
			project        = "blocked-proj"
			compName       = "blocked-comp"
			envName        = "blocked-env"
			rbName         = "rb-finalize-blocked"
		)
		releaseName := compName + "-" + envName
		req := reconcileRequest(rbName)

		AfterEach(func() {
			forceDelete(rbName)
			forceDeleteRelease(releaseName)
		})

		It("waits for the blocked Release to finish terminating before removing its finalizer", func() {
			r := testReconciler()

			By("Creating the ReleaseBinding with the finalizer pre-set")
			rb := rbFixture(rbName, project, compName, envName, "rel", true)
			Expect(k8sClient.Create(ctx, rb)).To(Succeed())

			By("Creating an owned Release with matching labels AND a blocking finalizer")
			rel := releaseFixture(releaseName, ns, project, compName, envName, blockFinalizer)
			Expect(k8sClient.Create(ctx, rel)).To(Succeed())

			By("Triggering deletion of the ReleaseBinding")
			Expect(k8sClient.Delete(ctx, rb)).To(Succeed())

			By("Verifying ReleaseBinding is marked for deletion")
			Eventually(func() bool {
				updated := &openchoreov1alpha1.ReleaseBinding{}
				if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: rbName}, updated); err != nil {
					return false
				}
				return !updated.DeletionTimestamp.IsZero()
			}, timeout, interval).Should(BeTrue())

			By("First finalize reconcile: sets the Finalizing condition and returns early")
			mustReconcile(r, req)
			updated := fetchRB(rbName)
			cond := conditionFor(updated, string(ConditionFinalizing))
			Expect(cond).NotTo(BeNil())
			Expect(cond.Status).To(Equal(metav1.ConditionTrue))

			By("Second finalize reconcile: finds the Release, calls DeleteAllOf, requeues")
			result := mustReconcile(r, req)
			Expect(result.RequeueAfter).To(BeNumerically(">", 0),
				"controller must requeue while the Release is still terminating")

			By("Verifying the Release is marked for deletion but blocked by its finalizer")
			Eventually(func(g Gomega) {
				blockedRelease := &openchoreov1alpha1.Release{}
				g.Expect(k8sClient.Get(ctx,
					types.NamespacedName{Namespace: ns, Name: releaseName},
					blockedRelease,
				)).To(Succeed())
				g.Expect(blockedRelease.DeletionTimestamp.IsZero()).To(BeFalse(),
					"Release should be marked for deletion")
				g.Expect(blockedRelease.Finalizers).To(ContainElement(blockFinalizer),
					"Release should still have its blocking finalizer")
			}, timeout, interval).Should(Succeed())

			By("Verifying the ReleaseBinding still has its finalizer while the Release is blocked")
			updated = fetchRB(rbName)
			Expect(updated.Finalizers).To(ContainElement(ReleaseBindingFinalizer))
			Expect(updated.DeletionTimestamp.IsZero()).To(BeFalse())

			By("Reconciling multiple times while the Release is blocked — ReleaseBinding must not be deleted")
			for range 3 {
				result := mustReconcile(r, req)
				Expect(result.RequeueAfter).To(BeNumerically(">", 0))

				current := &openchoreov1alpha1.ReleaseBinding{}
				Expect(k8sClient.Get(ctx,
					types.NamespacedName{Namespace: ns, Name: rbName},
					current,
				)).To(Succeed(), "ReleaseBinding must still exist while Release is blocked")
				Expect(current.Finalizers).To(ContainElement(ReleaseBindingFinalizer))
			}

			By("Simulating the external process removing the Release's blocking finalizer")
			blockedRelease := &openchoreov1alpha1.Release{}
			Expect(k8sClient.Get(ctx,
				types.NamespacedName{Namespace: ns, Name: releaseName},
				blockedRelease,
			)).To(Succeed())
			blockedRelease.Finalizers = nil
			Expect(k8sClient.Update(ctx, blockedRelease)).To(Succeed())

			By("Waiting for the Release to be fully deleted by the API server")
			Eventually(func() bool {
				err := k8sClient.Get(ctx,
					types.NamespacedName{Namespace: ns, Name: releaseName},
					&openchoreov1alpha1.Release{},
				)
				return apierrors.IsNotFound(err)
			}, timeout, interval).Should(BeTrue())

			By("Reconciling until the ReleaseBinding is also deleted")
			Eventually(func() bool {
				_, _ = r.Reconcile(ctx, req)
				err := k8sClient.Get(ctx,
					types.NamespacedName{Namespace: ns, Name: rbName},
					&openchoreov1alpha1.ReleaseBinding{},
				)
				return apierrors.IsNotFound(err)
			}, timeout, interval).Should(BeTrue(),
				"ReleaseBinding should be deleted once the blocked Release is gone")
		})
	})

	// ── Release ownership conflict ────────────────────────────────────────────

	Context("when a Release with the expected name already exists but is owned by another resource", func() {
		const (
			project  = "conflict-proj"
			compName = "conflict-comp"
			envName  = "conflict-env"
			dpName   = "conflict-dp"
			rbName   = "rb-conflict"
			crName   = "cr-conflict"
		)
		releaseName := compName + "-" + envName
		req := reconcileRequest(rbName)

		AfterEach(func() {
			forceDelete(rbName)
			forceDeleteRelease(releaseName)
			_ = k8sClient.Delete(ctx, &openchoreov1alpha1.ComponentRelease{
				ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: crName},
			})
			_ = k8sClient.Delete(ctx, &openchoreov1alpha1.Environment{
				ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: envName},
			})
			_ = k8sClient.Delete(ctx, &openchoreov1alpha1.DataPlane{
				ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: dpName},
			})
			_ = k8sClient.Delete(ctx, &openchoreov1alpha1.Component{
				ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: compName},
			})
			_ = k8sClient.Delete(ctx, &openchoreov1alpha1.Project{
				ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: project},
			})
		})

		It("sets ReleaseSynced=False with reason ReleaseOwnershipConflict", func() {
			r := testReconcilerWithPipeline()

			By("Creating all dependencies")
			Expect(k8sClient.Create(ctx, crFixture(crName, project, compName))).To(Succeed())
			Expect(k8sClient.Create(ctx, dpFixture(dpName))).To(Succeed())
			Expect(k8sClient.Create(ctx, envFixture(envName, dpName))).To(Succeed())
			Expect(k8sClient.Create(ctx, componentFixture(compName, project))).To(Succeed())
			Expect(k8sClient.Create(ctx, projectFixture(project))).To(Succeed())

			By("Pre-creating a Release with the expected name but NO owner reference")
			// The Release has no ownerReferences → HasOwnerReference will return false
			// → controller returns "not owned by this ReleaseBinding" error
			Expect(k8sClient.Create(ctx, &openchoreov1alpha1.Release{
				ObjectMeta: metav1.ObjectMeta{
					Name:      releaseName,
					Namespace: ns,
					// Deliberately NO owner reference set
				},
				Spec: openchoreov1alpha1.ReleaseSpec{
					Owner: openchoreov1alpha1.ReleaseOwner{
						ProjectName:   project,
						ComponentName: compName,
					},
					EnvironmentName: envName,
					TargetPlane:     openchoreov1alpha1.TargetPlaneDataPlane,
				},
			})).To(Succeed())

			By("Creating the ReleaseBinding with the finalizer pre-set")
			Expect(k8sClient.Create(ctx,
				rbFixture(rbName, project, compName, envName, crName, true),
			)).To(Succeed())

			By("Reconciling — pipeline renders resources; CreateOrUpdate finds existing Release with wrong owner")
			mustReconcile(r, req)

			By("Verifying ReleaseSynced=False with ReleaseOwnershipConflict reason")
			rb := fetchRB(rbName)
			cond := conditionFor(rb, string(ConditionReleaseSynced))
			Expect(cond).NotTo(BeNil())
			Expect(cond.Status).To(Equal(metav1.ConditionFalse))
			Expect(cond.Reason).To(Equal(string(ReasonReleaseOwnershipConflict)))
		})
	})

	Context("buildMetadataContext", func() {
		var reconciler *Reconciler

		BeforeEach(func() {
			reconciler = &Reconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}
		})

		It("should include all required fields in pod selectors", func() {
			By("Creating test resources")
			namespaceName := "test-namespace"
			projectName := "test-project"
			componentName := "test-component"
			environmentName := "test-env"
			componentUID := types.UID("component-uid-123")
			projectUID := types.UID("project-uid-456")
			environmentUID := types.UID("environment-uid-789")
			dataPlaneUID := types.UID("dataplane-uid-abc")
			dataPlaneName := "test-dataplane"

			componentRelease := &openchoreov1alpha1.ComponentRelease{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-release",
					Namespace: namespaceName,
				},
				Spec: openchoreov1alpha1.ComponentReleaseSpec{
					Owner: openchoreov1alpha1.ComponentReleaseOwner{
						ProjectName:   projectName,
						ComponentName: componentName,
					},
				},
			}

			component := &openchoreov1alpha1.Component{
				ObjectMeta: metav1.ObjectMeta{
					Name:      componentName,
					Namespace: namespaceName,
					UID:       componentUID,
				},
			}

			project := &openchoreov1alpha1.Project{
				ObjectMeta: metav1.ObjectMeta{
					Name:      projectName,
					Namespace: namespaceName,
					UID:       projectUID,
				},
			}

			dataPlane := &openchoreov1alpha1.DataPlane{
				ObjectMeta: metav1.ObjectMeta{
					Name: dataPlaneName,
					UID:  dataPlaneUID,
				},
			}

			environment := &openchoreov1alpha1.Environment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      environmentName,
					Namespace: namespaceName,
					UID:       environmentUID,
				},
			}

			By("Building metadata context")
			metadataContext := reconciler.buildMetadataContext(
				componentRelease,
				component,
				project,
				dataPlane,
				environment,
				environmentName,
			)

			By("Verifying pod selectors include all required fields")
			Expect(metadataContext.PodSelectors).NotTo(BeNil())
			Expect(metadataContext.PodSelectors).To(HaveKeyWithValue(labels.LabelKeyNamespaceName, namespaceName))
			Expect(metadataContext.PodSelectors).To(HaveKeyWithValue(labels.LabelKeyProjectName, projectName))
			Expect(metadataContext.PodSelectors).To(HaveKeyWithValue(labels.LabelKeyComponentName, componentName))
			Expect(metadataContext.PodSelectors).To(HaveKeyWithValue(labels.LabelKeyEnvironmentName, environmentName))
			Expect(metadataContext.PodSelectors).To(HaveKeyWithValue(labels.LabelKeyComponentUID, string(componentUID)))
			Expect(metadataContext.PodSelectors).To(HaveKeyWithValue(labels.LabelKeyEnvironmentUID, string(environmentUID)))
			Expect(metadataContext.PodSelectors).To(HaveKeyWithValue(labels.LabelKeyProjectUID, string(projectUID)))

			By("Verifying pod selectors have exactly 7 entries")
			Expect(metadataContext.PodSelectors).To(HaveLen(7))

			By("Verifying standard labels also include all required fields")
			Expect(metadataContext.Labels).NotTo(BeNil())
			Expect(metadataContext.Labels).To(HaveKeyWithValue(labels.LabelKeyNamespaceName, namespaceName))
			Expect(metadataContext.Labels).To(HaveKeyWithValue(labels.LabelKeyProjectName, projectName))
			Expect(metadataContext.Labels).To(HaveKeyWithValue(labels.LabelKeyComponentName, componentName))
			Expect(metadataContext.Labels).To(HaveKeyWithValue(labels.LabelKeyEnvironmentName, environmentName))
			Expect(metadataContext.Labels).To(HaveKeyWithValue(labels.LabelKeyComponentUID, string(componentUID)))
			Expect(metadataContext.Labels).To(HaveKeyWithValue(labels.LabelKeyEnvironmentUID, string(environmentUID)))
			Expect(metadataContext.Labels).To(HaveKeyWithValue(labels.LabelKeyProjectUID, string(projectUID)))
		})
	})
})
