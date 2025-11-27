// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"flag"
	"os"
	"time"

	clustergateway "github.com/openchoreo/openchoreo/internal/cluster-gateway"
	"github.com/openchoreo/openchoreo/internal/cmdutil"
)

const (
	defaultPort              = 8443
	defaultReadTimeout       = 60 * time.Second
	defaultWriteTimeout      = 60 * time.Second
	defaultIdleTimeout       = 120 * time.Second
	defaultShutdownTimeout   = 30 * time.Second
	defaultHeartbeatInterval = 30 * time.Second
	defaultHeartbeatTimeout  = 90 * time.Second
)

func main() {
	var (
		port              int
		serverCertPath    string
		serverKeyPath     string
		readTimeout       time.Duration
		writeTimeout      time.Duration
		idleTimeout       time.Duration
		shutdownTimeout   time.Duration
		heartbeatInterval time.Duration
		heartbeatTimeout  time.Duration
		logLevel          string
	)

	flag.IntVar(&port, "port", cmdutil.GetEnvInt("AGENT_SERVER_PORT", defaultPort), "Server port")
	flag.StringVar(&serverCertPath, "server-cert",
		cmdutil.GetEnv("SERVER_CERT_PATH", "/certs/tls.crt"),
		"Path to server certificate")
	flag.StringVar(&serverKeyPath, "server-key",
		cmdutil.GetEnv("SERVER_KEY_PATH", "/certs/tls.key"),
		"Path to server private key")
	flag.DurationVar(&readTimeout, "read-timeout", defaultReadTimeout, "HTTP read timeout")
	flag.DurationVar(&writeTimeout, "write-timeout", defaultWriteTimeout, "HTTP write timeout")
	flag.DurationVar(&idleTimeout, "idle-timeout", defaultIdleTimeout, "HTTP idle timeout")
	flag.DurationVar(&shutdownTimeout, "shutdown-timeout", defaultShutdownTimeout, "Graceful shutdown timeout")
	flag.DurationVar(&heartbeatInterval, "heartbeat-interval", defaultHeartbeatInterval, "Heartbeat ping interval")
	flag.DurationVar(&heartbeatTimeout, "heartbeat-timeout", defaultHeartbeatTimeout, "Heartbeat timeout duration")
	flag.StringVar(&logLevel, "log-level", cmdutil.GetEnv("LOG_LEVEL", "info"), "Log level (debug, info, warn, error)")
	flag.Parse()

	logger := cmdutil.SetupLogger(logLevel)

	logger.Info("starting OpenChoreo Cluster Gateway",
		"port", port,
		"serverCert", serverCertPath,
		"serverKey", serverKeyPath,
		"heartbeatInterval", heartbeatInterval,
		"heartbeatTimeout", heartbeatTimeout,
		"note", "Client CA certificates are loaded dynamically from DataPlane CRs",
	)

	config := &clustergateway.Config{
		Port:              port,
		ServerCertPath:    serverCertPath,
		ServerKeyPath:     serverKeyPath,
		ReadTimeout:       readTimeout,
		WriteTimeout:      writeTimeout,
		IdleTimeout:       idleTimeout,
		ShutdownTimeout:   shutdownTimeout,
		HeartbeatInterval: heartbeatInterval,
		HeartbeatTimeout:  heartbeatTimeout,
	}

	srv := clustergateway.New(config, logger)
	if err := srv.Start(); err != nil {
		logger.Error("server failed", "error", err)
		os.Exit(1)
	}
}
