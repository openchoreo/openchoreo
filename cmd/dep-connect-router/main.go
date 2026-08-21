// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"errors"
	"flag"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/openchoreo/openchoreo/internal/cmdutil"
	depconnectrouter "github.com/openchoreo/openchoreo/internal/dep-connect-router"
)

func main() {
	var (
		listenAddr      string
		labelSelector   string
		sniAnnotation   string
		refreshInterval time.Duration
		dialTimeout     time.Duration
		logLevel        string
	)

	flag.StringVar(&listenAddr, "listen", cmdutil.GetEnv("LISTEN_ADDR", ":8443"),
		"L4 listen address for the SNI router")
	flag.StringVar(&labelSelector, "label-selector",
		cmdutil.GetEnv("LABEL_SELECTOR", depconnectrouter.DefaultLabelSelector),
		"Label selector identifying dep-agent Services to route to")
	flag.StringVar(&sniAnnotation, "sni-annotation",
		cmdutil.GetEnv("SNI_ANNOTATION_KEY", depconnectrouter.DefaultSNIAnnotationKey),
		"Service annotation holding each dep-agent's SNI host")
	flag.DurationVar(&refreshInterval, "refresh-interval", depconnectrouter.DefaultRefreshInterval,
		"How often to refresh the dep-agent backend map")
	flag.DurationVar(&dialTimeout, "dial-timeout", depconnectrouter.DefaultDialTimeout,
		"Timeout for dialing a dep-agent backend")
	flag.StringVar(&logLevel, "log-level", cmdutil.GetEnv("LOG_LEVEL", "info"),
		"Log level (debug, info, warn, error)")
	flag.Parse()

	logger := cmdutil.SetupLogger(logLevel)

	router, err := depconnectrouter.New(depconnectrouter.Config{
		ListenAddr:       listenAddr,
		LabelSelector:    labelSelector,
		SNIAnnotationKey: sniAnnotation,
		RefreshInterval:  refreshInterval,
		DialTimeout:      dialTimeout,
	}, logger)
	if err != nil {
		logger.Error("failed to initialize dep-connect SNI router", "error", err)
		os.Exit(1)
	}

	logger.Info("starting OpenChoreo dep-connect SNI router", "listen", listenAddr)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := router.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		logger.Error("dep-connect SNI router failed", "error", err)
		os.Exit(1)
	}
	logger.Info("dep-connect SNI router shutdown completed")
}
