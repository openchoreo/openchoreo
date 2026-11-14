// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"flag"
	"fmt"
	"log"
	"log/slog"
	"os"

	"github.com/openchoreo/openchoreo/tools/lsp"
)

// Flags accepted for vscode-languageclient compatibility. Declared package-level
// so flag.Parse() in main recognizes the names without exporting them.
var (
	stdioFlag       bool
	clientProcessID int
)

func main() {
	logLevel := flag.String("log-level", "info", "log level: debug, info, warn, error")
	logFile := flag.String("log-file", "", "optional file path for server logs (default: stderr)")
	// Accepted for compatibility with vscode-languageclient, which launches
	// stdio servers with these arguments. The server always speaks stdio.
	flag.BoolVar(&stdioFlag, "stdio", false, "accepted for vscode-languageclient compatibility")
	flag.IntVar(&clientProcessID, "clientProcessId", 0, "accepted for vscode-languageclient compatibility")
	flag.Parse()

	logger, closeFn, err := buildLogger(*logLevel, *logFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error configuring logger: %v\n", err)
		os.Exit(1)
	}
	if closeFn != nil {
		defer closeFn()
	}

	server := lsp.NewServer()
	server.SetLogger(logger)

	if err := server.Start(); err != nil {
		log.Fatal(err)
	}
}

func buildLogger(level, filePath string) (*slog.Logger, func(), error) {
	var levelVar slog.Level
	switch level {
	case "debug":
		levelVar = slog.LevelDebug
	case "warn", "warning":
		levelVar = slog.LevelWarn
	case "error":
		levelVar = slog.LevelError
	default:
		levelVar = slog.LevelInfo
	}

	out := os.Stderr
	var closeFn func()
	if filePath != "" {
		f, err := os.OpenFile(filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		if err != nil {
			return nil, nil, err
		}
		out = f
		closeFn = func() { _ = f.Close() }
	}

	handler := slog.NewTextHandler(out, &slog.HandlerOptions{Level: levelVar})
	return slog.New(handler), closeFn, nil
}
