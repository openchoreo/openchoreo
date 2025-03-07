package kubernetes

import (
	"context"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/choreo-idp/choreo/internal/controller"
)

type namespaceHandler struct {
	kubernetesClient client.Client
}

var _ ResourceHandler[BuildContext] = (*namespaceHandler)(nil)

func NewNamespaceHandler(kubernetesClient client.Client) ResourceHandler[BuildContext] {
	return &namespaceHandler{
		kubernetesClient: kubernetesClient,
	}
}

func (h *namespaceHandler) KindName() string {
	return "KubernetesNamespace"
}

// NamespaceName has the format "choreo-ci-<org-name>"
func (h *namespaceHandler) Name(ctx context.Context, builtCtx *BuildContext) string {
	return MakeNamespaceName(builtCtx)
}

func (h *namespaceHandler) Get(ctx context.Context, builtCtx *BuildContext) (interface{}, error) {
	name := h.Name(ctx, builtCtx)
	namespace := &corev1.Namespace{}
	err := h.kubernetesClient.Get(ctx, client.ObjectKey{Name: name}, namespace)
	if apierrors.IsNotFound(err) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	return namespace, nil
}

func (h *namespaceHandler) Create(ctx context.Context, builtCtx *BuildContext) error {
	namespace := makeNamespace(builtCtx)
	return h.kubernetesClient.Create(ctx, namespace)
}

func MakeNamespaceName(builtCtx *BuildContext) string {
	return "choreo-ci-" + builtCtx.Build.Labels[controller.GetOrganizationName(builtCtx.Build)]
}

func makeNamespace(builtCtx *BuildContext) *corev1.Namespace {
	return &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:   MakeNamespaceName(builtCtx),
			Labels: makeNamespaceLabels(builtCtx),
		},
	}
}
