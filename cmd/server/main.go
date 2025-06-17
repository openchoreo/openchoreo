// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"flag"
	"time"

	"github.com/openchoreo/openchoreo/server"
	"github.com/openchoreo/openchoreo/server/pkg/logging"
)

var shutdownTimeout = 5 * time.Second

var port = flag.Int("port", 8080, "port http server runs on")

func main() {

	logger := logging.NewLogger()
	var opts []server.Option
	opts = append(opts, server.WithLogger(logger), server.WithPort(*port))

	srv := server.New(logger, opts...)

	srv.Run(context.Background())
}
