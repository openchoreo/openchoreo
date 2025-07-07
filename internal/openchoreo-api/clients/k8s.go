package k8s

import (
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"

	choreoapiv1 "github.com/openchoreo/openchoreo/api/v1"
)

func NewK8sClient() (client.Client, error) {
	config, err := rest.InClusterConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to create kubernetes config: %w", err)
	}

	scheme := runtime.NewScheme()
	if err := choreoapiv1.AddToScheme(scheme); err != nil {
		return nil, fmt.Errorf("failed to add OpenChoreo scheme: %w", err)
	}

	return client.New(config, client.Options{Scheme: scheme})
}
