// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"flag"
	"fmt"
	"log"
)

func main() {
	var (
		configDir        = flag.String("config-dir", "./config", "Path to the kubebuilder config directory")
		chartDir         = flag.String("chart-dir", "./install/helm/openchoreo-control-plane", "Path to the helm chart directory")
		controllerSubDir = flag.String("controller-subdir", "controller-manager", "Subdirectory within templates for controller resources")
	)
	flag.Parse()

	generator := NewGenerator(*configDir, *chartDir, *controllerSubDir)

	if err := generator.Run(); err != nil {
		log.Fatalf("Failed to generate helm chart: %v", err)
	}

	fmt.Println("Helm chart generated successfully")
}
