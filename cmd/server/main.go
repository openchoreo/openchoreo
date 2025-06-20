// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"flag"
	"path/filepath"
	"time"

	"github.com/openchoreo/openchoreo/server"
	"github.com/openchoreo/openchoreo/server/pkg/logging"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
)

var shutdownTimeout = 5 * time.Second

var (
	port = flag.Int("port", 8080, "port http server runs on")
	dev  = flag.Bool("dev", false, "use development mode with custom kube config")
)

func main() {
	flag.Parse()

	logger := logging.NewLogger()

	var kubeConfig *rest.Config
	var err error

	if *dev {
		// out-cluster config
		var kubeconfig *string
		if home := homedir.HomeDir(); home != "" {
			kubeconfig = flag.String("kubeconfig", filepath.Join(home, ".kube", "config"), "(optional) absolute path to the kubeconfig file")
		} else {
			kubeconfig = flag.String("kubeconfig", "", "absolute path to the kubeconfig file")
		}
		logger.Info(*kubeconfig)
		// use the current context in kubeconfig
		kubeConfig, err = clientcmd.BuildConfigFromFlags("", *kubeconfig)
		if err != nil {
			logger.Error("Unable to get kubeconfig", err.Error())
			panic(err.Error())
		}
	} else {
		// Use in-cluster config
		kubeConfig, err = rest.InClusterConfig()
		if err != nil {
			logger.Error("Unable to get kubeconfig", err.Error())
			return
		}
	}

	srv := server.New(logger, *port, kubeConfig)

	srv.Run(context.Background())
}
