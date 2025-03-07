package argo

import (
	"context"
	"github.com/choreo-idp/choreo/internal/controller/build/integrations/kubernetes"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type serviceAccountHandler struct {
	kubernetesClient client.Client
}

var _ kubernetes.ResourceHandler[kubernetes.BuildContext] = (*serviceAccountHandler)(nil)

func NewServiceAccountHandler(kubernetesClient client.Client) kubernetes.ResourceHandler[kubernetes.BuildContext] {
	return &serviceAccountHandler{
		kubernetesClient: kubernetesClient,
	}
}

func (h *serviceAccountHandler) KindName() string {
	return "ArgoWorkflowServiceAccount"
}

func (h *serviceAccountHandler) Name(ctx context.Context, builtCtx *kubernetes.BuildContext) string {
	return makeServiceAccountName()
}

func (h *serviceAccountHandler) Get(ctx context.Context, builtCtx *kubernetes.BuildContext) (interface{}, error) {
	name := makeServiceAccountName()
	sa := corev1.ServiceAccount{}
	err := h.kubernetesClient.Get(ctx, client.ObjectKey{Name: name, Namespace: kubernetes.MakeNamespaceName(builtCtx)}, &sa)
	if apierrors.IsNotFound(err) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	return sa, nil
}

func (h *serviceAccountHandler) Create(ctx context.Context, builtCtx *kubernetes.BuildContext) error {
	sa := makeServiceAccount(builtCtx)
	return h.kubernetesClient.Create(ctx, sa)
}

func makeServiceAccountName() string {
	return "workflow-sa"
}

func makeServiceAccount(builtCtx *kubernetes.BuildContext) *corev1.ServiceAccount {
	return &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      makeServiceAccountName(),
			Namespace: kubernetes.MakeNamespaceName(builtCtx),
		},
	}
}
