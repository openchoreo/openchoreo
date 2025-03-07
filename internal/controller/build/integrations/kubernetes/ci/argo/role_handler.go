package argo

import (
	"context"
	"github.com/choreo-idp/choreo/internal/controller/build/integrations/kubernetes"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type roleHandler struct {
	kubernetesClient client.Client
}

var _ kubernetes.ResourceHandler[kubernetes.BuildContext] = (*roleHandler)(nil)

func NewRoleHandler(kubernetesClient client.Client) kubernetes.ResourceHandler[kubernetes.BuildContext] {
	return &roleHandler{
		kubernetesClient: kubernetesClient,
	}
}

func (h *roleHandler) KindName() string {
	return "ArgoWorkflowRole"
}

func (h *roleHandler) Name(ctx context.Context, builtCtx *kubernetes.BuildContext) string {
	return makeRoleName()
}

func (h *roleHandler) Get(ctx context.Context, builtCtx *kubernetes.BuildContext) (interface{}, error) {
	name := makeRoleName()
	role := rbacv1.Role{}
	err := h.kubernetesClient.Get(ctx, client.ObjectKey{Name: name, Namespace: kubernetes.MakeNamespaceName(builtCtx)}, &role)
	if apierrors.IsNotFound(err) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	return role, nil
}

func (h *roleHandler) Create(ctx context.Context, builtCtx *kubernetes.BuildContext) error {
	role := makeRole(builtCtx)
	return h.kubernetesClient.Create(ctx, role)
}

func makeRoleName() string {
	return "workflow-role"
}

func makeRole(builtCtx *kubernetes.BuildContext) *rbacv1.Role {
	return &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      makeRoleName(),
			Namespace: kubernetes.MakeNamespaceName(builtCtx),
		},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups: []string{"argoproj.io"},
				Resources: []string{"workflowtaskresults"},
				Verbs:     []string{"create", "get", "list", "watch", "update", "patch"},
			},
		},
	}
}
