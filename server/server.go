// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package server

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/openchoreo/openchoreo/server/middleware"
	"github.com/openchoreo/openchoreo/server/pkg/logging"
	"github.com/openchoreo/openchoreo/server/request"
	"k8s.io/client-go/rest"
)

const (
	labelTemplate = "core.choreo.dev"
)

type resourceHandler struct {
	proxy *httputil.ReverseProxy
}

type Server struct {
	srv        *http.Server
	port       int
	logger     *logging.Logger
	kubeConfig *rest.Config
}

func (s *Server) Run(ctx context.Context) error {
	ctx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		s.logger.Info("Starting server on :" + strconv.Itoa(s.port))
		if err := s.srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			s.logger.Error("Server error", "error", err)
		}
	}()

	<-ctx.Done()
	s.logger.Info("Received shutdown signal, gracefully shutting down server...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var err error
	if err = s.Shutdown(shutdownCtx); err != nil {
		s.logger.Error("Error during server shutdown", "error", err)
	} else {
		s.logger.Info("Server gracefully shut down")
	}
	return err
}

func createReverseProxy(kubeConfig *rest.Config) *httputil.ReverseProxy {
	target, _ := url.Parse(kubeConfig.APIPath)
	proxy := httputil.NewSingleHostReverseProxy(target)
	token := kubeConfig.BearerToken

	// Configure TLS to skip verification
	proxy.Transport = &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true,
		},
	}

	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		originalDirector(req)
		requestInfo := req.Context().Value(request.RequestInfoKey{}).(*request.RequestInfo)

		splitPath, err := request.SplitPath(req.URL.Path)
		if err != nil {
			return
		}
		// TODO account for /status or /events endpoints or resource name
		requestedResource, err := request.ParseResourceType(splitPath[len(splitPath)-1])
		if err != nil {
			// If last path element is not a resource type, try the previous one
			requested, err := request.ParseResourceType(splitPath[len(splitPath)-2])
			if err != nil {
				// Add error log
				fmt.Errorf("Failed to parse resource type: %w", err)
				return
			}
			requestedResource = requested
		}
		switch req.Method {
		case http.MethodGet:
			// Get organization from token
			basePath := fmt.Sprintf("/apis/core.choreo.dev/v1/namespaces/default-org/%s/%s", string(requestedResource),
				requestInfo.Params[requestedResource])
			req.URL.Path = basePath
			req.URL.RawQuery = addLabelSelectors(requestInfo)
		case http.MethodPost:
			// Duplicate for now
			basePath := fmt.Sprintf("/apis/core.choreo.dev/v1/namespaces/default-org/%s/%s", string(requestedResource),
				requestInfo.Params[requestedResource])
			req.URL.Path = basePath
			req.URL.RawQuery = addLabelSelectors(requestInfo)
		}

		req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", token))
	}

	return proxy
}

// addLabelSelectors adds labelSelector queryparam with labels extracted from the request
func addLabelSelectors(info *request.RequestInfo) string {
	if len(info.Params) == 0 {
		return ""
	}

	var labels []string
	for resource, value := range info.Params {
		if value != "" {
			labels = append(labels, fmt.Sprintf("core.choreo.dev/%s=%s", strings.TrimSuffix(string(resource), "s"), value))
		}
	}

	if len(labels) > 0 {
		return fmt.Sprint("labelSelector=" + url.QueryEscape(strings.Join(labels, ",")))
	}
	return ""
}

func newResourceHandler(kubeConfig *rest.Config) *resourceHandler {
	return &resourceHandler{
		proxy: createReverseProxy(kubeConfig),
	}
}

func (h *resourceHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.proxy.ServeHTTP(w, r)
}

// New returns a new Choreo api server instance
func New(logger *logging.Logger, port int, kubeConfig *rest.Config) *Server {
	srv := &Server{
		port:       port,
		logger:     logger,
		kubeConfig: kubeConfig,
	}

	newMux := http.NewServeMux()
	var h http.Handler = newResourceHandler(srv.kubeConfig)
	h = middleware.WithRequestInfo(h)
	h = middleware.WithLogging(h, logger)
	h = middleware.WithTracing(h)
	newMux.Handle("/api/v1/", h)

	srv.srv = &http.Server{
		Addr:    "0.0.0.0:" + strconv.Itoa(srv.port),
		Handler: newMux,
	}

	return srv
}

// Shutdown gracefully shuts down the server
func (s *Server) Shutdown(ctx context.Context) error {
	return s.srv.Shutdown(ctx)
}
