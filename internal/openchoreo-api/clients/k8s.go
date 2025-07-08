package k8s

import (
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	choreoapiv1 "github.com/openchoreo/openchoreo/api/v1"
)

func NewK8sClient() (client.Client, error) {
	config, err := ctrl.GetConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to create kubernetes config: %w", err)
	}

	scheme := runtime.NewScheme()
	if err := choreoapiv1.AddToScheme(scheme); err != nil {
		return nil, fmt.Errorf("failed to add OpenChoreo scheme: %w", err)
	}

	return client.New(config, client.Options{Scheme: scheme})
}
