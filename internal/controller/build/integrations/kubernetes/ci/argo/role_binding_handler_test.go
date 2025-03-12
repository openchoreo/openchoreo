package argo

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	rbacv1 "k8s.io/api/rbac/v1"

	"github.com/choreo-idp/choreo/internal/controller/build/integrations"
)

var _ = Describe("Role Binding", func() {
	var (
		buildCtx    *integrations.BuildContext
		roleBinding *rbacv1.RoleBinding
	)

	BeforeEach(func() {
		buildCtx = newTestBuildContext()
	})

	JustBeforeEach(func() {
		roleBinding = makeRoleBinding(buildCtx)
	})

	Context("Make name creation", func() {
		It("should have the correct name", func() {
			name := makeRoleBindingName()
			Expect(name).To(Equal("workflow-role-binding"))
		})
	})

	Context("Make role binding kind", func() {

		It("should create a role binding with the correct name and namespace", func() {
			Expect(roleBinding).NotTo(BeNil())
			Expect(roleBinding.Name).To(Equal("workflow-role-binding"))
			Expect(roleBinding.Namespace).To(Equal("choreo-ci-test-organization"))
		})

		It("should have the correct role ref", func() {
			Expect(roleBinding.RoleRef.Kind).To(Equal("Role"))
			Expect(roleBinding.RoleRef.Name).To(Equal("workflow-role"))
			Expect(roleBinding.RoleRef.APIGroup).To(Equal("rbac.authorization.k8s.io"))
		})

		It("should have the correct subject", func() {
			Expect(roleBinding.Subjects).To(HaveLen(1))
			subject := roleBinding.Subjects[0]
			Expect(subject.Kind).To(Equal("ServiceAccount"))
			Expect(subject.Name).To(Equal("workflow-sa"))
			Expect(subject.Namespace).To(Equal("choreo-ci-test-organization"))
		})
	})
})
