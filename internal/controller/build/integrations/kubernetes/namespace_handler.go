package kubernetes

import (
	"context"
	"errors"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/choreo-idp/choreo/internal/controller"
	"github.com/choreo-idp/choreo/internal/controller/build/integrations"
)

type namespaceHandler struct {
	kubernetesClient client.Client
}

var _ integrations.ResourceHandler[integrations.BuildContext] = (*namespaceHandler)(nil)

func NewNamespaceHandler(kubernetesClient client.Client) integrations.ResourceHandler[integrations.BuildContext] {
	return &namespaceHandler{
		kubernetesClient: kubernetesClient,
	}
}

func (h *namespaceHandler) KindName() string {
	return "KubernetesNamespace"
}

// NamespaceName has the format "choreo-ci-<org-name>"
func (h *namespaceHandler) Name(ctx context.Context, builtCtx *integrations.BuildContext) string {
	return MakeNamespaceName(builtCtx)
}

func (h *namespaceHandler) Get(ctx context.Context, builtCtx *integrations.BuildContext) (interface{}, error) {
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

func (h *namespaceHandler) Create(ctx context.Context, builtCtx *integrations.BuildContext) error {
	namespace := makeNamespace(builtCtx)
	return h.kubernetesClient.Create(ctx, namespace)
}

func (h *namespaceHandler) Update(ctx context.Context, builtCtx *integrations.BuildContext, currentState interface{}) error {
	currentNS, ok := currentState.(*corev1.Namespace)
	if !ok {
		return errors.New("failed to cast current state to Namespace")
	}
	newNS := makeNamespace(builtCtx)

	if h.shouldUpdate(currentNS, newNS) {
		newNS.ResourceVersion = currentNS.ResourceVersion
		return h.kubernetesClient.Update(ctx, newNS)
	}

	return nil
}

func MakeNamespaceName(builtCtx *integrations.BuildContext) string {
	return "choreo-ci-" + controller.GetOrganizationName(builtCtx.Build)
}

func makeNamespace(builtCtx *integrations.BuildContext) *corev1.Namespace {
	return &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:   MakeNamespaceName(builtCtx),
			Labels: MakeLabels(builtCtx),
		},
	}
}

func (h *namespaceHandler) shouldUpdate(current, new *corev1.Namespace) bool {
	// Compare the labels
	if !cmp.Equal(ExtractManagedLabels(current.Labels), ExtractManagedLabels(new.Labels)) {
		return true
	}

	if !cmp.Equal(current.Spec, new.Spec, cmpopts.EquateEmpty()) {
		return true
	}
	return false
}
