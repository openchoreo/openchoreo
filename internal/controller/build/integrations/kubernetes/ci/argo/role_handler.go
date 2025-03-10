package argo

import (
	"context"
	"errors"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/choreo-idp/choreo/internal/controller/build/integrations"
	"github.com/choreo-idp/choreo/internal/controller/build/integrations/kubernetes"
)

type roleHandler struct {
	kubernetesClient client.Client
}

var _ integrations.ResourceHandler[integrations.BuildContext] = (*roleHandler)(nil)

func NewRoleHandler(kubernetesClient client.Client) integrations.ResourceHandler[integrations.BuildContext] {
	return &roleHandler{
		kubernetesClient: kubernetesClient,
	}
}

func (h *roleHandler) KindName() string {
	return "ArgoWorkflowRole"
}

func (h *roleHandler) Name(ctx context.Context, builtCtx *integrations.BuildContext) string {
	return makeRoleName()
}

func (h *roleHandler) Get(ctx context.Context, builtCtx *integrations.BuildContext) (interface{}, error) {
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

func (h *roleHandler) Create(ctx context.Context, builtCtx *integrations.BuildContext) error {
	role := makeRole(builtCtx)
	return h.kubernetesClient.Create(ctx, role)
}

func (h *roleHandler) Update(ctx context.Context, builtCtx *integrations.BuildContext, currentState interface{}) error {
	currentRole, ok := currentState.(*rbacv1.Role)
	if !ok {
		return errors.New("failed to cast current state to Role")
	}
	newRole := makeRole(builtCtx)

	if h.shouldUpdate(currentRole, newRole) {
		newRole.ResourceVersion = currentRole.ResourceVersion
		return h.kubernetesClient.Update(ctx, newRole)
	}

	return nil
}

func makeRoleName() string {
	return "workflow-role"
}

func makeRole(builtCtx *integrations.BuildContext) *rbacv1.Role {
	return &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      makeRoleName(),
			Namespace: kubernetes.MakeNamespaceName(builtCtx),
			Labels:    kubernetes.MakeLabels(builtCtx),
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

func (h *roleHandler) shouldUpdate(current, new *rbacv1.Role) bool {
	// Compare the labels
	if !cmp.Equal(kubernetes.ExtractManagedLabels(current.Labels), kubernetes.ExtractManagedLabels(new.Labels)) {
		return true
	}
	if !cmp.Equal(current.Rules, new.Rules, cmpopts.EquateEmpty()) {
		return true
	}
	return false
}
