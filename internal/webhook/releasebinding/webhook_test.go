// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package releasebinding

import (
	"encoding/json"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	admissionv1 "k8s.io/api/admission/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	openchoreodevv1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

var _ = Describe("ReleaseBinding Webhook", func() {
	var (
		validator Validator
		defaulter Defaulter
	)

	BeforeEach(func() {
		err := openchoreodevv1alpha1.AddToScheme(scheme.Scheme)
		Expect(err).NotTo(HaveOccurred())
		validator = Validator{}
		defaulter = Defaulter{decoder: admission.NewDecoder(scheme.Scheme)}
	})

	buildRequest := func(op admissionv1.Operation, obj *openchoreodevv1alpha1.ReleaseBinding, oldObj *openchoreodevv1alpha1.ReleaseBinding) admission.Request {
		raw, err := json.Marshal(obj)
		Expect(err).NotTo(HaveOccurred())
		req := admission.Request{
			AdmissionRequest: admissionv1.AdmissionRequest{
				Operation: op,
				Object:    runtime.RawExtension{Raw: raw},
			},
		}
		if oldObj != nil {
			oldRaw, err := json.Marshal(oldObj)
			Expect(err).NotTo(HaveOccurred())
			req.OldObject = runtime.RawExtension{Raw: oldRaw}
		}
		return req
	}

	Context("Defaulter webhook (Handle)", func() {
		It("should pass through a CREATE request unchanged when releaseName is already set", func() {
			obj := &openchoreodevv1alpha1.ReleaseBinding{}
			obj.Spec.ReleaseName = "v1"
			req := buildRequest(admissionv1.Create, obj, nil)
			resp := defaulter.Handle(ctx, req)
			Expect(resp.Allowed).To(BeTrue())
		})

		It("should pass through a CREATE request with empty releaseName", func() {
			obj := &openchoreodevv1alpha1.ReleaseBinding{}
			req := buildRequest(admissionv1.Create, obj, nil)
			resp := defaulter.Handle(ctx, req)
			Expect(resp.Allowed).To(BeTrue())
		})

		It("should preserve releaseName from old object on UPDATE when new object has empty releaseName", func() {
			oldObj := &openchoreodevv1alpha1.ReleaseBinding{}
			oldObj.Spec.ReleaseName = "auto-release-abc"
			newObj := &openchoreodevv1alpha1.ReleaseBinding{}
			newObj.Spec.ReleaseName = ""
			req := buildRequest(admissionv1.Update, newObj, oldObj)
			resp := defaulter.Handle(ctx, req)
			Expect(resp.Allowed).To(BeTrue())
			Expect(resp.Patches).NotTo(BeEmpty())
		})

		It("should not override releaseName on UPDATE when new object already has one", func() {
			oldObj := &openchoreodevv1alpha1.ReleaseBinding{}
			oldObj.Spec.ReleaseName = "old-release"
			newObj := &openchoreodevv1alpha1.ReleaseBinding{}
			newObj.Spec.ReleaseName = "new-release"
			req := buildRequest(admissionv1.Update, newObj, oldObj)
			resp := defaulter.Handle(ctx, req)
			Expect(resp.Allowed).To(BeTrue())
			Expect(resp.Patches).To(BeEmpty())
		})

		It("should return an error response when the request object cannot be decoded", func() {
			req := admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Operation: admissionv1.Create,
					Object:    runtime.RawExtension{Raw: []byte(`{invalid-json}`)},
				},
			}
			resp := defaulter.Handle(ctx, req)
			Expect(resp.Allowed).To(BeFalse())
		})

		It("should return an error response when the old object cannot be decoded on UPDATE", func() {
			newObj := &openchoreodevv1alpha1.ReleaseBinding{}
			newObj.Spec.ReleaseName = ""
			raw, err := json.Marshal(newObj)
			Expect(err).NotTo(HaveOccurred())
			req := admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Operation: admissionv1.Update,
					Object:    runtime.RawExtension{Raw: raw},
					OldObject: runtime.RawExtension{Raw: []byte(`{invalid-json}`)},
				},
			}
			resp := defaulter.Handle(ctx, req)
			Expect(resp.Allowed).To(BeFalse())
		})

		It("should handle UPDATE with empty OldObject without error", func() {
			newObj := &openchoreodevv1alpha1.ReleaseBinding{}
			newObj.Spec.ReleaseName = "v2"
			raw, err := json.Marshal(newObj)
			Expect(err).NotTo(HaveOccurred())
			req := admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Operation: admissionv1.Update,
					Object:    runtime.RawExtension{Raw: raw},
					// OldObject.Raw is empty — should not attempt to copy releaseName
				},
			}
			resp := defaulter.Handle(ctx, req)
			Expect(resp.Allowed).To(BeTrue())
		})
	})

	Context("Validator webhook", func() {
		It("should admit ReleaseBinding creation (no-op validator)", func() {
			obj := &openchoreodevv1alpha1.ReleaseBinding{}
			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should admit ReleaseBinding update (no-op validator)", func() {
			oldObj := &openchoreodevv1alpha1.ReleaseBinding{}
			newObj := &openchoreodevv1alpha1.ReleaseBinding{}
			_, err := validator.ValidateUpdate(ctx, oldObj, newObj)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should admit ReleaseBinding deletion (no-op validator)", func() {
			obj := &openchoreodevv1alpha1.ReleaseBinding{}
			_, err := validator.ValidateDelete(ctx, obj)
			Expect(err).NotTo(HaveOccurred())
		})
	})
})
