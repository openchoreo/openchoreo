package argo

import (
	"context"
	"github.com/choreo-idp/choreo/internal/controller/build/integrations/kubernetes"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type roleBindingHandler struct {
	kubernetesClient client.Client
}

var _ kubernetes.ResourceHandler[kubernetes.BuildContext] = (*roleBindingHandler)(nil)

func NewRoleBindingHandler(kubernetesClient client.Client) kubernetes.ResourceHandler[kubernetes.BuildContext] {
	return &roleBindingHandler{
		kubernetesClient: kubernetesClient,
	}
}

func (h *roleBindingHandler) KindName() string {
	return "ArgoWorkflowRoleBinding"
}

func (h *roleBindingHandler) Name(ctx context.Context, builtCtx *kubernetes.BuildContext) string {
	return makeRoleBindingName()
}

func (h *roleBindingHandler) Get(ctx context.Context, builtCtx *kubernetes.BuildContext) (interface{}, error) {
	name := makeRoleBindingName()
	role := rbacv1.Role{}
	err := h.kubernetesClient.Get(ctx, client.ObjectKey{Name: name, Namespace: kubernetes.MakeNamespaceName(builtCtx)}, &role)
	if apierrors.IsNotFound(err) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	return role, nil
}

func (h *roleBindingHandler) Create(ctx context.Context, builtCtx *kubernetes.BuildContext) error {
	roleBinding := makeRoleBinding(builtCtx)
	return h.kubernetesClient.Create(ctx, roleBinding)
}

func makeRoleBindingName() string {
	return "workflow-role-binding"
}

func makeRoleBinding(builtCtx *kubernetes.BuildContext) *rbacv1.RoleBinding {
	return &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      makeRoleBindingName(),
			Namespace: kubernetes.MakeNamespaceName(builtCtx),
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      makeServiceAccountName(),
				Namespace: kubernetes.MakeNamespaceName(builtCtx),
			},
		},
		RoleRef: rbacv1.RoleRef{
			Kind:     "Role",
			Name:     makeRoleName(),
			APIGroup: "rbac.authorization.k8s.io",
		},
	}
}
