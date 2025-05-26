// Copyright OpenChoreo Authors 2025
// SPDX-License-Identifier: Apache-2.0

package resources

import (
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"

	choreov1 "github.com/openchoreo/openchoreo/api/v1"
)

var (
	// Rename from 'scheme' to 'schemeInstance' to avoid conflict
	schemeInstance = runtime.NewScheme()
)

func init() {
	// Register standard Kubernetes types
	utilruntime.Must(clientgoscheme.AddToScheme(schemeInstance))
	// Register Choreo CRDs
	utilruntime.Must(choreov1.AddToScheme(schemeInstance))
}

// GetScheme returns the runtime scheme with all required types registered
func GetScheme() *runtime.Scheme {
	return schemeInstance
}
