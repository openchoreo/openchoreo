// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package revision

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

const (
	testNamespace        = "default"
	immutableSpecMessage = "revision spec is immutable; only status may change"

	// currentRelease is the ComponentRelease recorded on the test revision;
	// otherRelease is used to attempt changing it.
	currentRelease = "comp-abc123"
	otherRelease   = "comp-def456"

	// otherComponent is a component name used to attempt changing a binding's owner.
	otherComponent = "other"
)

func newReleaseBindingRevision() *openchoreov1alpha1.ReleaseBindingRevision {
	return &openchoreov1alpha1.ReleaseBindingRevision{
		ObjectMeta: metav1.ObjectMeta{GenerateName: "rb-", Namespace: testNamespace},
		Spec: openchoreov1alpha1.ReleaseBindingRevisionSpec{
			Revision:             1,
			Cause:                openchoreov1alpha1.RevisionCauseDeploy,
			ComponentReleaseName: currentRelease,
			SnapshotHash:         "hash-1",
			BindingSpec: openchoreov1alpha1.ReleaseBindingSpec{
				Owner:                           openchoreov1alpha1.ReleaseBindingOwner{ProjectName: "proj", ComponentName: "comp"},
				Environment:                     "dev",
				ReleaseName:                     currentRelease,
				ComponentTypeEnvironmentConfigs: &runtime.RawExtension{Raw: []byte(`{"replicas":2}`)},
				TraitEnvironmentConfigs: map[string]runtime.RawExtension{
					"ingress": {Raw: []byte(`{"host":"a.example.com"}`)},
				},
				WorkloadOverrides: &openchoreov1alpha1.WorkloadOverrideTemplateSpec{
					Container: &openchoreov1alpha1.ContainerOverride{
						Env: []openchoreov1alpha1.EnvVar{{Key: "LOG_LEVEL", Value: "info"}},
					},
				},
				State: openchoreov1alpha1.ReleaseStateActive,
			},
		},
	}
}

func newResourceReleaseBindingRevision() *openchoreov1alpha1.ResourceReleaseBindingRevision {
	return &openchoreov1alpha1.ResourceReleaseBindingRevision{
		ObjectMeta: metav1.ObjectMeta{GenerateName: "rrb-", Namespace: testNamespace},
		Spec: openchoreov1alpha1.ResourceReleaseBindingRevisionSpec{
			Revision:            1,
			Cause:               openchoreov1alpha1.RevisionCausePromote,
			ResourceReleaseName: "db-abc123",
			SnapshotHash:        "hash-1",
			BindingSpec: openchoreov1alpha1.ResourceReleaseBindingSpec{
				Owner:                          openchoreov1alpha1.ResourceReleaseBindingOwner{ProjectName: "proj", ResourceName: "db"},
				Environment:                    "dev",
				ResourceRelease:                "db-abc123",
				RetainPolicy:                   openchoreov1alpha1.ResourceRetainPolicyDelete,
				ResourceTypeEnvironmentConfigs: &runtime.RawExtension{Raw: []byte(`{"size":"small"}`)},
			},
		},
	}
}

// expectImmutable asserts that err is the apiserver's Invalid response carrying the
// revision spec immutability message.
func expectImmutable(err error) {
	GinkgoHelper()
	Expect(err).To(HaveOccurred())
	Expect(apierrors.IsInvalid(err)).To(BeTrue(), "expected an Invalid error, got %v", err)
	Expect(err.Error()).To(ContainSubstring(immutableSpecMessage))
}

var _ = Describe("ReleaseBindingRevision mutability", func() {
	var (
		rev *openchoreov1alpha1.ReleaseBindingRevision
		nn  types.NamespacedName
	)

	BeforeEach(func() {
		rev = newReleaseBindingRevision()
		Expect(k8sClient.Create(ctx, rev)).To(Succeed())
		nn = client.ObjectKeyFromObject(rev)
	})

	AfterEach(func() {
		Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, rev))).To(Succeed())
	})

	It("should create successfully", func() {
		fetched := &openchoreov1alpha1.ReleaseBindingRevision{}
		Expect(k8sClient.Get(ctx, nn, fetched)).To(Succeed())
		Expect(fetched.Spec.Revision).To(Equal(int64(1)))
		Expect(fetched.Spec.BindingSpec.ReleaseName).To(Equal(currentRelease))
	})

	It("should accept an update that leaves the spec unchanged", func() {
		fetched := &openchoreov1alpha1.ReleaseBindingRevision{}
		Expect(k8sClient.Get(ctx, nn, fetched)).To(Succeed())
		Expect(k8sClient.Update(ctx, fetched)).To(Succeed())
	})

	It("should accept metadata changes a controller makes (labels, annotations, ownerReferences)", func() {
		fetched := &openchoreov1alpha1.ReleaseBindingRevision{}
		Expect(k8sClient.Get(ctx, nn, fetched)).To(Succeed())
		fetched.Labels = map[string]string{"openchoreo.dev/release-binding": "comp-dev"}
		fetched.Annotations = map[string]string{"openchoreo.dev/note": "x"}
		fetched.OwnerReferences = []metav1.OwnerReference{{
			APIVersion: openchoreov1alpha1.GroupVersion.String(),
			Kind:       "ReleaseBinding",
			Name:       "comp-dev",
			UID:        types.UID("00000000-0000-0000-0000-000000000001"),
			Controller: ptr.To(true),
		}}
		Expect(k8sClient.Update(ctx, fetched)).To(Succeed())
	})

	DescribeTable("should reject spec changes with the immutability message",
		func(mutate func(s *openchoreov1alpha1.ReleaseBindingRevisionSpec)) {
			fetched := &openchoreov1alpha1.ReleaseBindingRevision{}
			Expect(k8sClient.Get(ctx, nn, fetched)).To(Succeed())
			original := fetched.Spec.DeepCopy()

			mutate(&fetched.Spec)
			expectImmutable(k8sClient.Update(ctx, fetched))

			By("leaving the stored spec unchanged")
			after := &openchoreov1alpha1.ReleaseBindingRevision{}
			Expect(k8sClient.Get(ctx, nn, after)).To(Succeed())
			Expect(after.Spec).To(Equal(*original))
		},
		Entry("spec.revision", func(s *openchoreov1alpha1.ReleaseBindingRevisionSpec) { s.Revision = 2 }),
		Entry("spec.cause", func(s *openchoreov1alpha1.ReleaseBindingRevisionSpec) {
			s.Cause = openchoreov1alpha1.RevisionCauseRedeploy
		}),
		Entry("spec.componentReleaseName", func(s *openchoreov1alpha1.ReleaseBindingRevisionSpec) {
			s.ComponentReleaseName = otherRelease
		}),
		Entry("spec.snapshotHash", func(s *openchoreov1alpha1.ReleaseBindingRevisionSpec) { s.SnapshotHash = "hash-2" }),
		Entry("spec.bindingSpec.releaseName", func(s *openchoreov1alpha1.ReleaseBindingRevisionSpec) {
			s.BindingSpec.ReleaseName = otherRelease
		}),
		Entry("spec.bindingSpec.environment", func(s *openchoreov1alpha1.ReleaseBindingRevisionSpec) {
			s.BindingSpec.Environment = "staging"
		}),
		Entry("spec.bindingSpec.owner.componentName", func(s *openchoreov1alpha1.ReleaseBindingRevisionSpec) {
			s.BindingSpec.Owner.ComponentName = otherComponent
		}),
		Entry("spec.bindingSpec.state", func(s *openchoreov1alpha1.ReleaseBindingRevisionSpec) {
			s.BindingSpec.State = openchoreov1alpha1.ReleaseStateUndeploy
		}),
		Entry("spec.bindingSpec.componentTypeEnvironmentConfigs value", func(s *openchoreov1alpha1.ReleaseBindingRevisionSpec) {
			s.BindingSpec.ComponentTypeEnvironmentConfigs = &runtime.RawExtension{Raw: []byte(`{"replicas":3}`)}
		}),
		Entry("spec.bindingSpec.componentTypeEnvironmentConfigs removed", func(s *openchoreov1alpha1.ReleaseBindingRevisionSpec) {
			s.BindingSpec.ComponentTypeEnvironmentConfigs = nil
		}),
		Entry("spec.bindingSpec.traitEnvironmentConfigs", func(s *openchoreov1alpha1.ReleaseBindingRevisionSpec) {
			s.BindingSpec.TraitEnvironmentConfigs["ingress"] = runtime.RawExtension{Raw: []byte(`{"host":"b.example.com"}`)}
		}),
		Entry("spec.bindingSpec.workloadOverrides env value", func(s *openchoreov1alpha1.ReleaseBindingRevisionSpec) {
			s.BindingSpec.WorkloadOverrides.Container.Env[0].Value = "debug"
		}),
		Entry("spec.bindingSpec.workloadOverrides files added", func(s *openchoreov1alpha1.ReleaseBindingRevisionSpec) {
			s.BindingSpec.WorkloadOverrides.Container.Files = []openchoreov1alpha1.FileVar{
				{Key: "app.conf", MountPath: "/etc/app", Value: "x=1"},
			}
		}),
	)

	It("should reject a spec change sent as a merge patch", func() {
		patch := client.RawPatch(types.MergePatchType, []byte(`{"spec":{"bindingSpec":{"releaseName":"`+otherRelease+`"}}}`))
		expectImmutable(k8sClient.Patch(ctx, rev, patch))
	})

	It("should persist status.health, status.active and status.knownGood through the status subresource", func() {
		fetched := &openchoreov1alpha1.ReleaseBindingRevision{}
		Expect(k8sClient.Get(ctx, nn, fetched)).To(Succeed())

		now := metav1.Now()
		fetched.Status = openchoreov1alpha1.BindingRevisionStatus{
			Health:       openchoreov1alpha1.RevisionHealthHealthy,
			Active:       true,
			KnownGood:    true,
			ObservedAt:   &now,
			HealthySince: &now,
			Conditions: []metav1.Condition{{
				Type: "Ready", Status: metav1.ConditionTrue, Reason: "Healthy", Message: "ok", LastTransitionTime: now,
			}},
		}
		Expect(k8sClient.Status().Update(ctx, fetched)).To(Succeed())

		after := &openchoreov1alpha1.ReleaseBindingRevision{}
		Expect(k8sClient.Get(ctx, nn, after)).To(Succeed())
		Expect(after.Status.Health).To(Equal(openchoreov1alpha1.RevisionHealthHealthy))
		Expect(after.Status.Active).To(BeTrue())
		Expect(after.Status.KnownGood).To(BeTrue())
		Expect(after.Status.Conditions).To(HaveLen(1))

		By("changing status again, as the controller will over the revision's life")
		after.Status.Health = openchoreov1alpha1.RevisionHealthDegraded
		after.Status.Active = false
		Expect(k8sClient.Status().Update(ctx, after)).To(Succeed())

		final := &openchoreov1alpha1.ReleaseBindingRevision{}
		Expect(k8sClient.Get(ctx, nn, final)).To(Succeed())
		Expect(final.Status.Health).To(Equal(openchoreov1alpha1.RevisionHealthDegraded))
		Expect(final.Status.Active).To(BeFalse())
		Expect(final.Status.KnownGood).To(BeTrue())
	})

	It("should ignore spec changes sent through the status subresource", func() {
		fetched := &openchoreov1alpha1.ReleaseBindingRevision{}
		Expect(k8sClient.Get(ctx, nn, fetched)).To(Succeed())

		fetched.Spec.ComponentReleaseName = otherRelease
		fetched.Status.Health = openchoreov1alpha1.RevisionHealthProgressing
		Expect(k8sClient.Status().Update(ctx, fetched)).To(Succeed())

		after := &openchoreov1alpha1.ReleaseBindingRevision{}
		Expect(k8sClient.Get(ctx, nn, after)).To(Succeed())
		Expect(after.Spec.ComponentReleaseName).To(Equal(currentRelease))
		Expect(after.Status.Health).To(Equal(openchoreov1alpha1.RevisionHealthProgressing))
	})

	It("should not persist status from an update to the main resource", func() {
		fetched := &openchoreov1alpha1.ReleaseBindingRevision{}
		Expect(k8sClient.Get(ctx, nn, fetched)).To(Succeed())
		generation := fetched.Generation

		fetched.Status.Health = openchoreov1alpha1.RevisionHealthHealthy
		fetched.Status.Active = true
		fetched.Status.KnownGood = true
		Expect(k8sClient.Update(ctx, fetched)).To(Succeed())

		after := &openchoreov1alpha1.ReleaseBindingRevision{}
		Expect(k8sClient.Get(ctx, nn, after)).To(Succeed())
		Expect(after.Status).To(Equal(openchoreov1alpha1.BindingRevisionStatus{}))
		Expect(after.Generation).To(Equal(generation))
	})
})

var _ = Describe("ResourceReleaseBindingRevision mutability", func() {
	var (
		rev *openchoreov1alpha1.ResourceReleaseBindingRevision
		nn  types.NamespacedName
	)

	BeforeEach(func() {
		rev = newResourceReleaseBindingRevision()
		Expect(k8sClient.Create(ctx, rev)).To(Succeed())
		nn = client.ObjectKeyFromObject(rev)
	})

	AfterEach(func() {
		Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, rev))).To(Succeed())
	})

	It("should create successfully", func() {
		fetched := &openchoreov1alpha1.ResourceReleaseBindingRevision{}
		Expect(k8sClient.Get(ctx, nn, fetched)).To(Succeed())
		Expect(fetched.Spec.ResourceReleaseName).To(Equal("db-abc123"))
	})

	DescribeTable("should reject spec changes with the immutability message",
		func(mutate func(s *openchoreov1alpha1.ResourceReleaseBindingRevisionSpec)) {
			fetched := &openchoreov1alpha1.ResourceReleaseBindingRevision{}
			Expect(k8sClient.Get(ctx, nn, fetched)).To(Succeed())
			original := fetched.Spec.DeepCopy()

			mutate(&fetched.Spec)
			expectImmutable(k8sClient.Update(ctx, fetched))

			By("leaving the stored spec unchanged")
			after := &openchoreov1alpha1.ResourceReleaseBindingRevision{}
			Expect(k8sClient.Get(ctx, nn, after)).To(Succeed())
			Expect(after.Spec).To(Equal(*original))
		},
		Entry("spec.revision", func(s *openchoreov1alpha1.ResourceReleaseBindingRevisionSpec) { s.Revision = 2 }),
		Entry("spec.cause", func(s *openchoreov1alpha1.ResourceReleaseBindingRevisionSpec) {
			s.Cause = openchoreov1alpha1.RevisionCauseOverride
		}),
		Entry("spec.resourceReleaseName", func(s *openchoreov1alpha1.ResourceReleaseBindingRevisionSpec) {
			s.ResourceReleaseName = secondResourceRelease
		}),
		Entry("spec.snapshotHash", func(s *openchoreov1alpha1.ResourceReleaseBindingRevisionSpec) { s.SnapshotHash = "hash-2" }),
		Entry("spec.bindingSpec.resourceRelease", func(s *openchoreov1alpha1.ResourceReleaseBindingRevisionSpec) {
			s.BindingSpec.ResourceRelease = secondResourceRelease
		}),
		Entry("spec.bindingSpec.retainPolicy", func(s *openchoreov1alpha1.ResourceReleaseBindingRevisionSpec) {
			s.BindingSpec.RetainPolicy = openchoreov1alpha1.ResourceRetainPolicyRetain
		}),
		Entry("spec.bindingSpec.resourceTypeEnvironmentConfigs", func(s *openchoreov1alpha1.ResourceReleaseBindingRevisionSpec) {
			s.BindingSpec.ResourceTypeEnvironmentConfigs = &runtime.RawExtension{Raw: []byte(`{"size":"large"}`)}
		}),
	)

	It("should persist status.health, status.active and status.knownGood through the status subresource", func() {
		fetched := &openchoreov1alpha1.ResourceReleaseBindingRevision{}
		Expect(k8sClient.Get(ctx, nn, fetched)).To(Succeed())

		fetched.Status.Health = openchoreov1alpha1.RevisionHealthHealthy
		fetched.Status.Active = true
		fetched.Status.KnownGood = true
		Expect(k8sClient.Status().Update(ctx, fetched)).To(Succeed())

		after := &openchoreov1alpha1.ResourceReleaseBindingRevision{}
		Expect(k8sClient.Get(ctx, nn, after)).To(Succeed())
		Expect(after.Status.Health).To(Equal(openchoreov1alpha1.RevisionHealthHealthy))
		Expect(after.Status.Active).To(BeTrue())
		Expect(after.Status.KnownGood).To(BeTrue())
	})

	It("should not persist status from an update to the main resource", func() {
		fetched := &openchoreov1alpha1.ResourceReleaseBindingRevision{}
		Expect(k8sClient.Get(ctx, nn, fetched)).To(Succeed())

		fetched.Status.Health = openchoreov1alpha1.RevisionHealthFailed
		fetched.Status.Active = true
		fetched.Status.KnownGood = true
		Expect(k8sClient.Update(ctx, fetched)).To(Succeed())

		after := &openchoreov1alpha1.ResourceReleaseBindingRevision{}
		Expect(k8sClient.Get(ctx, nn, after)).To(Succeed())
		Expect(after.Status).To(Equal(openchoreov1alpha1.BindingRevisionStatus{}))
	})
})
