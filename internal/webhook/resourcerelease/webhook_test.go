// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package resourcerelease

import (
	"context"
	"encoding/json"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"

	openchoreodevv1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

func mustRaw(v any) *runtime.RawExtension {
	data, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return &runtime.RawExtension{Raw: data}
}

func validTemplate() *runtime.RawExtension {
	return mustRaw(map[string]any{
		"apiVersion": "v1",
		"kind":       "ConfigMap",
		"metadata":   map[string]any{"name": "smoke"},
	})
}

// validReleaseFixture builds a minimal ResourceRelease with a single-resource
// snapshot. Tests mutate the returned object before calling the validator.
func validReleaseFixture() *openchoreodevv1alpha1.ResourceRelease {
	return &openchoreodevv1alpha1.ResourceRelease{
		Spec: openchoreodevv1alpha1.ResourceReleaseSpec{
			Owner: openchoreodevv1alpha1.ResourceReleaseOwner{
				ProjectName:  "demo",
				ResourceName: "mysql",
			},
			ResourceType: openchoreodevv1alpha1.ResourceReleaseResourceType{
				Kind: openchoreodevv1alpha1.ResourceTypeRefKindResourceType,
				Name: "mysql",
				Spec: openchoreodevv1alpha1.ResourceTypeSpec{
					Resources: []openchoreodevv1alpha1.ResourceTypeManifest{
						{ID: "claim", Template: validTemplate()},
					},
				},
			},
		},
	}
}

var _ = Describe("ResourceRelease Webhook", func() {
	var (
		ctx       context.Context
		validator Validator
	)

	BeforeEach(func() {
		ctx = context.Background()
		validator = Validator{}
	})

	Context("ValidateCreate", func() {
		It("admits a release with no parameter schema and no parameters", func() {
			rr := validReleaseFixture()
			_, err := validator.ValidateCreate(ctx, rr)
			Expect(err).ToNot(HaveOccurred())
		})

		It("admits a release whose parameters satisfy the snapshot schema", func() {
			rr := validReleaseFixture()
			rr.Spec.ResourceType.Spec.Parameters = &openchoreodevv1alpha1.SchemaSection{
				OpenAPIV3Schema: mustRaw(map[string]any{
					"type": "object",
					"properties": map[string]any{
						"version": map[string]any{"type": "string"},
					},
					"required": []string{"version"},
				}),
			}
			rr.Spec.Parameters = mustRaw(map[string]any{"version": "8.0"})

			_, err := validator.ValidateCreate(ctx, rr)
			Expect(err).ToNot(HaveOccurred())
		})

		It("rejects a release whose parameters violate the snapshot schema", func() {
			rr := validReleaseFixture()
			rr.Spec.ResourceType.Spec.Parameters = &openchoreodevv1alpha1.SchemaSection{
				OpenAPIV3Schema: mustRaw(map[string]any{
					"type": "object",
					"properties": map[string]any{
						"version": map[string]any{"type": "string"},
					},
					"required": []string{"version"},
				}),
			}
			rr.Spec.Parameters = mustRaw(map[string]any{}) // missing required field

			_, err := validator.ValidateCreate(ctx, rr)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("parameters"))
			// Locks the wire shape: validation failures must be StatusReasonInvalid
			// so kube-apiserver returns 422 + Reason=Invalid (not 403). Downstream
			// detection via apierrors.IsInvalid depends on this.
			Expect(apierrors.IsInvalid(err)).To(BeTrue())
		})

		It("rejects a release with a malformed snapshot parameter schema", func() {
			rr := validReleaseFixture()
			rr.Spec.ResourceType.Spec.Parameters = &openchoreodevv1alpha1.SchemaSection{
				OpenAPIV3Schema: &runtime.RawExtension{Raw: []byte(`{not json`)},
			}

			_, err := validator.ValidateCreate(ctx, rr)
			Expect(err).To(HaveOccurred())
		})

		It("rejects a release whose embedded ResourceType template references applied.*", func() {
			// Schema-drift defense: the snapshot was cut against a previous
			// ResourceType, but the live ResourceType has since broken its CEL
			// surface. Re-validating the embedded snapshot at admission time
			// catches the regression instead of letting it surface only at
			// runtime via Synced=False, Reason=RenderingFailed.
			rr := validReleaseFixture()
			rr.Spec.ResourceType.Spec.Resources = []openchoreodevv1alpha1.ResourceTypeManifest{
				{
					ID: "claim",
					Template: mustRaw(map[string]any{
						"apiVersion": "v1",
						"kind":       "ConfigMap",
						"data":       map[string]any{"host": "${applied.claim.status.host}"},
					}),
				},
			}

			_, err := validator.ValidateCreate(ctx, rr)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("applied"))
		})

		It("rejects a release whose embedded ResourceType has an undeclared applied id in readyWhen", func() {
			rr := validReleaseFixture()
			rr.Spec.ResourceType.Spec.Resources = []openchoreodevv1alpha1.ResourceTypeManifest{
				{
					ID:        "claim",
					ReadyWhen: "${applied.bogus.status.ready}",
					Template:  validTemplate(),
				},
			}

			_, err := validator.ValidateCreate(ctx, rr)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("readyWhen"))
		})

		It("returns an error for non-ResourceRelease objects", func() {
			_, err := validator.ValidateCreate(ctx, &openchoreodevv1alpha1.ResourceType{})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("expected a ResourceRelease"))
		})
	})

	Context("ValidateUpdate", func() {
		It("is a no-op since spec immutability is CRD-enforced", func() {
			old := validReleaseFixture()
			rr := validReleaseFixture()
			_, err := validator.ValidateUpdate(ctx, old, rr)
			Expect(err).ToNot(HaveOccurred())
		})

		It("returns an error when the new object is not a ResourceRelease", func() {
			old := validReleaseFixture()
			_, err := validator.ValidateUpdate(ctx, old, &openchoreodevv1alpha1.ResourceType{})
			Expect(err).To(HaveOccurred())
		})

		It("returns an error when the old object is not a ResourceRelease", func() {
			rr := validReleaseFixture()
			_, err := validator.ValidateUpdate(ctx, &openchoreodevv1alpha1.ResourceType{}, rr)
			Expect(err).To(HaveOccurred())
		})
	})

	Context("ValidateDelete", func() {
		It("is a no-op", func() {
			rr := validReleaseFixture()
			_, err := validator.ValidateDelete(ctx, rr)
			Expect(err).ToNot(HaveOccurred())
		})

		It("returns an error for non-ResourceRelease objects", func() {
			_, err := validator.ValidateDelete(ctx, &openchoreodevv1alpha1.ResourceType{})
			Expect(err).To(HaveOccurred())
		})
	})
})
