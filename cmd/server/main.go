// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"flag"
	"time"

	"github.com/openchoreo/openchoreo/server"
	"github.com/openchoreo/openchoreo/server/pkg/logging"
	"k8s.io/client-go/rest"
)

var shutdownTimeout = 5 * time.Second

var (
	port    = flag.Int("port", 8080, "port http server runs on")
	dev     = flag.Bool("dev", false, "use development mode with custom kube config")
	token   = flag.String("token", "", "bearer token for authentication")
	apiPath = flag.String("api-path", "", "API path for kubernetes cluster")
)

func main() {
	flag.Parse()

	logger := logging.NewLogger()

	var kubeConfig *rest.Config
	var err error

	if *dev {
		// Use development kube config with flags
		if *token == "" || *apiPath == "" {
			logger.Error("Both token and api-path flags are required in dev mode")
			return
		}
		kubeConfig = createDevKubeConfig(*token, *apiPath)
	} else {
		// Use in-cluster config
		kubeConfig, err = rest.InClusterConfig()
		if err != nil {
			logger.Warn("Couldn't retrieve kube config")
			return
		}
	}

	srv := server.New(logger, *port, kubeConfig)

	srv.Run(context.Background())
}

func createDevKubeConfig(token, apiPath string) *rest.Config {
	return &rest.Config{
		BearerToken: token,
		APIPath:     apiPath,
	}
}
