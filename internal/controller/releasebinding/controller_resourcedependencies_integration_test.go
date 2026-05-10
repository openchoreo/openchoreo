// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package releasebinding

import (
	"encoding/json"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/controller/resourcereleasebinding"
	componentpipeline "github.com/openchoreo/openchoreo/internal/pipeline/component"
)

// depEnvTemplate is a Deployment template that projects ${dependencies.envVars}
// into container.env. Used by specs that need to observe the resolver path at
// the rendered manifest level (env vars resolved from the provider RRB land in
// the container spec).
var depEnvTemplate = &runtime.RawExtension{
	Raw: []byte(`{` +
		`"apiVersion":"apps/v1",` +
		`"kind":"Deployment",` +
		`"metadata":{"name":"test-deployment"},` +
		`"spec":{` +
		`"selector":{"matchLabels":{"app":"test"}},` +
		`"template":{` +
		`"metadata":{"labels":{"app":"test"}},` +
		`"spec":{"containers":[{` +
		`"name":"app",` +
		`"image":"nginx:latest",` +
		`"env":"${dependencies.envVars}"` +
		`}]}` +
		`}}}`,
	),
}

// depConcatTemplate concats configurations.toX() + dependencies.toX() for both
// volumes and volumeMounts. Locks that the typed structs shared between
// configurations.* and dependencies.* CEL helpers stay JSON-compatible: a
// regression that re-types one side would fail to render under this template.
var depConcatTemplate = &runtime.RawExtension{
	Raw: []byte(`{` +
		`"apiVersion":"apps/v1",` +
		`"kind":"Deployment",` +
		`"metadata":{"name":"test-deployment"},` +
		`"spec":{` +
		`"selector":{"matchLabels":{"app":"test"}},` +
		`"template":{` +
		`"metadata":{"labels":{"app":"test"}},` +
		`"spec":{` +
		`"containers":[{` +
		`"name":"app",` +
		`"image":"nginx:latest",` +
		`"env":"${dependencies.envVars}",` +
		`"volumeMounts":"${configurations.toContainerVolumeMounts() + dependencies.toContainerVolumeMounts()}"` +
		`}],` +
		`"volumes":"${configurations.toVolumes() + dependencies.toVolumes()}"` +
		`}` +
		`}}}`,
	),
}

// testReconcilerWithCachedClient returns a Reconciler wired to the manager-backed
// cached client. Resource-dep and connection resolver paths query their providers
// via custom field indexes that only work through the cache.
func testReconcilerWithCachedClient() *Reconciler {
	return &Reconciler{
		Client:   k8sCachedClient,
		Scheme:   k8sCachedClient.Scheme(),
		Pipeline: componentpipeline.NewPipeline(),
	}
}

// reconcileUntil reconciles in a loop until the predicate succeeds against the
// fetched binding. Absorbs the cache-sync delay between Create and the
// reconciler's first read through the cached client.
func reconcileUntil(
	r *Reconciler,
	name string,
	predicate func(*openchoreov1alpha1.ReleaseBinding) bool,
) *openchoreov1alpha1.ReleaseBinding {
	GinkgoHelper()
	var rb *openchoreov1alpha1.ReleaseBinding
	Eventually(func(g Gomega) {
		_, err := r.Reconcile(ctx, reconcile.Request{
			NamespacedName: types.NamespacedName{Namespace: ns, Name: name},
		})
		g.Expect(err).NotTo(HaveOccurred())
		rb = &openchoreov1alpha1.ReleaseBinding{}
		g.Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: name}, rb)).To(Succeed())
		g.Expect(predicate(rb)).To(BeTrue())
	}, timeout, interval).Should(Succeed())
	return rb
}

// findDeploymentManifest scans a RenderedRelease for the first Deployment manifest
// and returns it. The rendered Release may include auto-injected NetworkPolicies in
// addition to the templated Deployment, so callers cannot assume a fixed length.
func findDeploymentManifest(rendered *openchoreov1alpha1.RenderedRelease) appsv1.Deployment {
	GinkgoHelper()
	var deploy appsv1.Deployment
	for _, manifest := range rendered.Spec.Resources {
		var meta metav1.PartialObjectMetadata
		if err := json.Unmarshal(manifest.Object.Raw, &meta); err != nil {
			continue
		}
		if meta.Kind == "Deployment" && meta.APIVersion == "apps/v1" {
			Expect(json.Unmarshal(manifest.Object.Raw, &deploy)).To(Succeed())
			return deploy
		}
	}
	Fail("rendered Release should contain a Deployment manifest")
	return deploy
}

var _ = Describe("ReleaseBinding resource dependencies", func() {
	Context("when no provider ResourceReleaseBinding exists", func() {
		const (
			rbName   = "rb-rd-gate"
			crName   = "cr-rd-gate"
			envName  = "env-rd-gate"
			dpName   = "dp-rd-gate"
			compName = "comp-rd-gate"
			projName = "proj-rd-gate"
			depRef   = "missing-db"
		)

		AfterEach(func() {
			forceDelete(rbName)
			forceDeleteRelease(compName + "-" + envName)
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
				ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: projName},
			})
		})

		It("surfaces pending dep and blocks ReleaseSynced", func() {
			By("Creating fixtures (project, component, env, dp)")
			Expect(k8sClient.Create(ctx, projectFixture(projName))).To(Succeed())
			Expect(k8sClient.Create(ctx, dpFixture(dpName))).To(Succeed())
			Expect(k8sClient.Create(ctx, envFixture(envName, dpName))).To(Succeed())
			Expect(k8sClient.Create(ctx, componentFixture(compName, projName))).To(Succeed())

			By("Creating ComponentRelease with a resource dep on a non-existent Resource")
			cr := crFixture(crName, projName, compName)
			cr.Spec.Workload.Dependencies = &openchoreov1alpha1.WorkloadDependencies{
				Resources: []openchoreov1alpha1.WorkloadResourceDependency{
					{Ref: depRef, EnvBindings: map[string]string{"host": "DB_HOST"}},
				},
			}
			Expect(k8sClient.Create(ctx, cr)).To(Succeed())

			By("Creating ReleaseBinding pinned to the ComponentRelease")
			Expect(k8sClient.Create(ctx,
				rbFixture(rbName, projName, compName, envName, crName, true),
			)).To(Succeed())

			By("Reconciling until the gate engages (PendingResourceDependencies populated)")
			r := testReconcilerWithCachedClient()
			rb := reconcileUntil(r, rbName, func(rb *openchoreov1alpha1.ReleaseBinding) bool {
				return len(rb.Status.PendingResourceDependencies) > 0
			})

			By("ResourceDependencyTargets reflect the workload deps")
			Expect(rb.Status.ResourceDependencyTargets).To(HaveLen(1))
			Expect(rb.Status.ResourceDependencyTargets[0].ResourceName).To(Equal(depRef))
			Expect(rb.Status.ResourceDependencyTargets[0].Project).To(Equal(projName))
			Expect(rb.Status.ResourceDependencyTargets[0].Environment).To(Equal(envName))
			Expect(rb.Status.ResourceDependencyTargets[0].Namespace).To(Equal(ns))

			By("PendingResourceDependencies surface the not-found reason")
			Expect(rb.Status.PendingResourceDependencies).To(HaveLen(1))
			Expect(rb.Status.PendingResourceDependencies[0].ResourceName).To(Equal(depRef))
			Expect(rb.Status.PendingResourceDependencies[0].Project).To(Equal(projName))
			Expect(rb.Status.PendingResourceDependencies[0].Reason).To(ContainSubstring("not found"))

			By("ConditionResourceDependenciesReady=False/ResourceDependenciesPending")
			depCond := conditionFor(rb, string(ConditionResourceDependenciesReady))
			Expect(depCond).NotTo(BeNil())
			Expect(depCond.Status).To(Equal(metav1.ConditionFalse))
			Expect(depCond.Reason).To(Equal(string(ReasonResourceDependenciesPending)))

			By("ConditionReleaseSynced is NOT True (gate blocked sync)")
			syncedCond := conditionFor(rb, string(ConditionReleaseSynced))
			if syncedCond != nil {
				Expect(syncedCond.Status).NotTo(Equal(metav1.ConditionTrue))
			}
		})
	})

	Context("when a provider ResourceReleaseBinding is Ready with a value-kind output", func() {
		const (
			rbName     = "rb-rd-env"
			crName     = "cr-rd-env"
			envName    = "env-rd-env"
			dpName     = "dp-rd-env"
			compName   = "comp-rd-env"
			projName   = "proj-rd-env"
			depRef     = "test-db"
			rrbName    = "test-db-env-rd-env"
			outputHost = "10.0.0.5"
		)

		AfterEach(func() {
			forceDelete(rbName)
			forceDeleteRelease(compName + "-" + envName)
			_ = k8sClient.Delete(ctx, &openchoreov1alpha1.ResourceReleaseBinding{
				ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: rrbName},
			})
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
				ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: projName},
			})
		})

		It("wires resolved env var into rendered Deployment", func() {
			By("Creating fixtures (project, component, env, dp)")
			Expect(k8sClient.Create(ctx, projectFixture(projName))).To(Succeed())
			Expect(k8sClient.Create(ctx, dpFixture(dpName))).To(Succeed())
			Expect(k8sClient.Create(ctx, envFixture(envName, dpName))).To(Succeed())
			Expect(k8sClient.Create(ctx, componentFixture(compName, projName))).To(Succeed())

			By("Creating provider RRB and marking it Ready with status.outputs[host]=" + outputHost)
			rrb := &openchoreov1alpha1.ResourceReleaseBinding{
				ObjectMeta: metav1.ObjectMeta{Name: rrbName, Namespace: ns},
				Spec: openchoreov1alpha1.ResourceReleaseBindingSpec{
					Owner: openchoreov1alpha1.ResourceReleaseBindingOwner{
						ProjectName:  projName,
						ResourceName: depRef,
					},
					Environment:     envName,
					ResourceRelease: "irrelevant-for-this-test",
				},
			}
			Expect(k8sClient.Create(ctx, rrb)).To(Succeed())
			rrb.Status.Outputs = []openchoreov1alpha1.ResolvedResourceOutput{
				{Name: "host", Value: outputHost},
			}
			apimeta.SetStatusCondition(&rrb.Status.Conditions, metav1.Condition{
				Type:               string(resourcereleasebinding.ConditionReady),
				Status:             metav1.ConditionTrue,
				ObservedGeneration: rrb.Generation,
				Reason:             "Ready",
				Message:            "test fixture",
			})
			Expect(k8sClient.Status().Update(ctx, rrb)).To(Succeed())

			By("Creating ComponentRelease with dep on " + depRef + " and template that projects dependencies.envVars")
			cr := crFixture(crName, projName, compName)
			cr.Spec.ComponentType.Spec.Resources[0].Template = depEnvTemplate
			cr.Spec.Workload.Dependencies = &openchoreov1alpha1.WorkloadDependencies{
				Resources: []openchoreov1alpha1.WorkloadResourceDependency{
					{Ref: depRef, EnvBindings: map[string]string{"host": "DB_HOST"}},
				},
			}
			Expect(k8sClient.Create(ctx, cr)).To(Succeed())

			By("Creating ReleaseBinding pinned to the ComponentRelease")
			Expect(k8sClient.Create(ctx,
				rbFixture(rbName, projName, compName, envName, crName, true),
			)).To(Succeed())

			By("Reconciling until ReleaseSynced=True")
			r := testReconcilerWithCachedClient()
			rb := reconcileUntil(r, rbName, func(rb *openchoreov1alpha1.ReleaseBinding) bool {
				cond := conditionFor(rb, string(ConditionReleaseSynced))
				return cond != nil && cond.Status == metav1.ConditionTrue
			})

			By("ConditionResourceDependenciesReady=True/AllResourceDependenciesReady")
			depCond := conditionFor(rb, string(ConditionResourceDependenciesReady))
			Expect(depCond).NotTo(BeNil())
			Expect(depCond.Status).To(Equal(metav1.ConditionTrue))
			Expect(depCond.Reason).To(Equal(string(ReasonAllResourceDependenciesReady)))
			Expect(rb.Status.PendingResourceDependencies).To(BeEmpty())

			By("Fetching the rendered Release and parsing its Deployment manifest")
			rendered := &openchoreov1alpha1.RenderedRelease{}
			Expect(k8sClient.Get(ctx,
				types.NamespacedName{Namespace: ns, Name: compName + "-" + envName},
				rendered,
			)).To(Succeed())
			deploy := findDeploymentManifest(rendered)

			By("Container.env contains DB_HOST=" + outputHost)
			Expect(deploy.Spec.Template.Spec.Containers).To(HaveLen(1))
			Expect(deploy.Spec.Template.Spec.Containers[0].Env).To(ContainElement(corev1.EnvVar{
				Name:  "DB_HOST",
				Value: outputHost,
			}))
		})
	})

	Context("when configurations and dependencies both contribute volumes", func() {
		const (
			rbName       = "rb-rd-concat"
			crName       = "cr-rd-concat"
			envName      = "env-rd-concat"
			dpName       = "dp-rd-concat"
			compName     = "comp-rd-concat"
			projName     = "proj-rd-concat"
			depRef       = "test-db"
			rrbName      = "test-db-env-rd-concat"
			secretName   = "db-creds"
			configMapRef = "db-config"
		)

		AfterEach(func() {
			forceDelete(rbName)
			forceDeleteRelease(compName + "-" + envName)
			_ = k8sClient.Delete(ctx, &openchoreov1alpha1.ResourceReleaseBinding{
				ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: rrbName},
			})
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
				ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: projName},
			})
		})

		It("merges file-mount-* and r-* volumes into a 3-volume Pod", func() {
			By("Creating fixtures (project, component, env, dp)")
			Expect(k8sClient.Create(ctx, projectFixture(projName))).To(Succeed())
			Expect(k8sClient.Create(ctx, dpFixture(dpName))).To(Succeed())
			Expect(k8sClient.Create(ctx, envFixture(envName, dpName))).To(Succeed())
			Expect(k8sClient.Create(ctx, componentFixture(compName, projName))).To(Succeed())

			By("Creating provider RRB Ready with secretKeyRef + configMapKeyRef outputs")
			rrb := &openchoreov1alpha1.ResourceReleaseBinding{
				ObjectMeta: metav1.ObjectMeta{Name: rrbName, Namespace: ns},
				Spec: openchoreov1alpha1.ResourceReleaseBindingSpec{
					Owner: openchoreov1alpha1.ResourceReleaseBindingOwner{
						ProjectName:  projName,
						ResourceName: depRef,
					},
					Environment:     envName,
					ResourceRelease: "irrelevant-for-this-test",
				},
			}
			Expect(k8sClient.Create(ctx, rrb)).To(Succeed())
			rrb.Status.Outputs = []openchoreov1alpha1.ResolvedResourceOutput{
				{Name: "password", SecretKeyRef: &openchoreov1alpha1.SecretKeyRef{Name: secretName, Key: "password"}},
				{Name: "database", ConfigMapKeyRef: &openchoreov1alpha1.ConfigMapKeyRef{Name: configMapRef, Key: "name"}},
			}
			apimeta.SetStatusCondition(&rrb.Status.Conditions, metav1.Condition{
				Type:               string(resourcereleasebinding.ConditionReady),
				Status:             metav1.ConditionTrue,
				ObservedGeneration: rrb.Generation,
				Reason:             "Ready",
				Message:            "test fixture",
			})
			Expect(k8sClient.Status().Update(ctx, rrb)).To(Succeed())

			By("Creating ComponentRelease with container.files + dependencies.fileBindings + concat template")
			cr := crFixture(crName, projName, compName)
			cr.Spec.ComponentType.Spec.Resources[0].Template = depConcatTemplate
			cr.Spec.Workload.Container.Files = []openchoreov1alpha1.FileVar{
				{Key: "settings.json", MountPath: "/etc/config", Value: `{"app":"smoke"}`},
			}
			cr.Spec.Workload.Dependencies = &openchoreov1alpha1.WorkloadDependencies{
				Resources: []openchoreov1alpha1.WorkloadResourceDependency{
					{
						Ref: depRef,
						FileBindings: map[string]string{
							"password": "/etc/db/password",
							"database": "/etc/db/database",
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, cr)).To(Succeed())

			By("Creating ReleaseBinding pinned to the ComponentRelease")
			Expect(k8sClient.Create(ctx,
				rbFixture(rbName, projName, compName, envName, crName, true),
			)).To(Succeed())

			By("Reconciling until ReleaseSynced=True")
			r := testReconcilerWithCachedClient()
			rb := reconcileUntil(r, rbName, func(rb *openchoreov1alpha1.ReleaseBinding) bool {
				cond := conditionFor(rb, string(ConditionReleaseSynced))
				return cond != nil && cond.Status == metav1.ConditionTrue
			})
			Expect(rb.Status.PendingResourceDependencies).To(BeEmpty())

			By("Fetching rendered Release and parsing the Deployment")
			rendered := &openchoreov1alpha1.RenderedRelease{}
			Expect(k8sClient.Get(ctx,
				types.NamespacedName{Namespace: ns, Name: compName + "-" + envName},
				rendered,
			)).To(Succeed())
			deploy := findDeploymentManifest(rendered)

			By("Pod has 3 volumes: 1 file-mount-* (configurations) + 2 r-* (dependencies)")
			Expect(deploy.Spec.Template.Spec.Volumes).To(HaveLen(3))

			fileMountVols := volumesWithPrefix(deploy.Spec.Template.Spec.Volumes, "file-mount-")
			depVols := volumesWithPrefix(deploy.Spec.Template.Spec.Volumes, "r-")
			Expect(fileMountVols).To(HaveLen(1), "exactly one configurations-side volume")
			Expect(depVols).To(HaveLen(2), "exactly two dependency-side volumes")

			By("Volume name sets are disjoint between configurations and dependencies prefixes")
			fileNames := volumeNames(fileMountVols)
			depNames := volumeNames(depVols)
			for _, fn := range fileNames {
				Expect(depNames).NotTo(ContainElement(fn))
			}

			By("Container has 3 mounts mirroring the volume names")
			Expect(deploy.Spec.Template.Spec.Containers).To(HaveLen(1))
			mounts := deploy.Spec.Template.Spec.Containers[0].VolumeMounts
			Expect(mounts).To(HaveLen(3))
			mountNames := make([]string, 0, len(mounts))
			for _, m := range mounts {
				mountNames = append(mountNames, m.Name)
			}
			allVolumeNames := append(append([]string{}, fileNames...), depNames...)
			for _, mn := range mountNames {
				Expect(allVolumeNames).To(ContainElement(mn), "mount %q must reference a declared volume", mn)
			}

			By("Dependency-side volumes reference the provider's Secret and ConfigMap by name")
			secretFound, configMapFound := false, false
			for _, v := range depVols {
				switch {
				case v.Secret != nil && v.Secret.SecretName == secretName:
					secretFound = true
				case v.ConfigMap != nil && v.ConfigMap.LocalObjectReference.Name == configMapRef:
					configMapFound = true
				}
			}
			Expect(secretFound).To(BeTrue(), "expected a dependency volume backed by Secret %q", secretName)
			Expect(configMapFound).To(BeTrue(), "expected a dependency volume backed by ConfigMap %q", configMapRef)
		})
	})
})

// volumesWithPrefix filters volumes by name prefix.
func volumesWithPrefix(vols []corev1.Volume, prefix string) []corev1.Volume {
	out := make([]corev1.Volume, 0, len(vols))
	for _, v := range vols {
		if len(v.Name) >= len(prefix) && v.Name[:len(prefix)] == prefix {
			out = append(out, v)
		}
	}
	return out
}

// volumeNames extracts the Name field from a slice of volumes.
func volumeNames(vols []corev1.Volume) []string {
	out := make([]string, 0, len(vols))
	for _, v := range vols {
		out = append(out, v.Name)
	}
	return out
}
