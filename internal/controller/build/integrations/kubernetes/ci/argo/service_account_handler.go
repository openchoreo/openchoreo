package argo

import (
	"context"
	"errors"

	"github.com/google/go-cmp/cmp"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/choreo-idp/choreo/internal/controller/build/common"
	"github.com/choreo-idp/choreo/internal/controller/build/integrations/kubernetes"
)

type serviceAccountHandler struct {
	kubernetesClient client.Client
}

var _ common.ResourceHandler[common.BuildContext] = (*serviceAccountHandler)(nil)

func NewServiceAccountHandler(kubernetesClient client.Client) common.ResourceHandler[common.BuildContext] {
	return &serviceAccountHandler{
		kubernetesClient: kubernetesClient,
	}
}

func (h *serviceAccountHandler) KindName() string {
	return "ArgoWorkflowServiceAccount"
}

func (h *serviceAccountHandler) Name(ctx context.Context, builtCtx *common.BuildContext) string {
	return makeServiceAccountName()
}

func (h *serviceAccountHandler) Get(ctx context.Context, builtCtx *common.BuildContext) (interface{}, error) {
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

func (h *serviceAccountHandler) Create(ctx context.Context, builtCtx *common.BuildContext) error {
	sa := makeServiceAccount(builtCtx)
	return h.kubernetesClient.Create(ctx, sa)
}

func (h *serviceAccountHandler) Update(ctx context.Context, builtCtx *common.BuildContext, currentState interface{}) error {
	currentSA, ok := currentState.(*corev1.ServiceAccount)
	if !ok {
		return errors.New("failed to cast current state to ServiceAccount")
	}
	newSA := makeServiceAccount(builtCtx)

	if h.shouldUpdate(currentSA, newSA) {
		newSA.ResourceVersion = currentSA.ResourceVersion
		return h.kubernetesClient.Update(ctx, newSA)
	}

	return nil
}

func makeServiceAccountName() string {
	return "workflow-sa"
}

func makeServiceAccount(builtCtx *common.BuildContext) *corev1.ServiceAccount {
	return &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      makeServiceAccountName(),
			Namespace: kubernetes.MakeNamespaceName(builtCtx),
			Labels:    kubernetes.MakeLabels(builtCtx),
		},
	}
}

func (h *serviceAccountHandler) shouldUpdate(current, new *corev1.ServiceAccount) bool {
	return !cmp.Equal(kubernetes.ExtractManagedLabels(current.Labels), kubernetes.ExtractManagedLabels(new.Labels))
}
