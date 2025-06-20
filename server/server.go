// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package server

import (
	"context"
	"crypto/tls"
	"crypto/x509"
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

func createReverseProxy(logger *logging.Logger, kubeConfig *rest.Config) (*httputil.ReverseProxy, error) {
	target, err := url.Parse(kubeConfig.Host)
	if err != nil {
		return nil, err
	}
	proxy := httputil.NewSingleHostReverseProxy(target)
	token := kubeConfig.BearerToken

	tlsConfig := &tls.Config{
		InsecureSkipVerify: kubeConfig.TLSClientConfig.Insecure,
	}

	if len(kubeConfig.TLSClientConfig.CAData) > 0 {
		caCertPool := x509.NewCertPool()
		if ok := caCertPool.AppendCertsFromPEM(kubeConfig.TLSClientConfig.CAData); !ok {
			panic(fmt.Errorf("failed to append CA cert from kubeconfig"))
		}
		tlsConfig.RootCAs = caCertPool
	}

	// Load client cert for mTLS if present
	if len(kubeConfig.TLSClientConfig.CertData) > 0 && len(kubeConfig.TLSClientConfig.KeyData) > 0 {
		cert, err := tls.X509KeyPair(kubeConfig.TLSClientConfig.CertData, kubeConfig.TLSClientConfig.KeyData)
		if err != nil {
			panic(fmt.Errorf("failed to load client cert/key from kubeconfig: %w", err))
		}
		tlsConfig.Certificates = []tls.Certificate{cert}
	}

	proxy.Transport = &http.Transport{
		TLSClientConfig: tlsConfig,
	}

	originalDirector := proxy.Director

	proxy.Director = func(req *http.Request) {
		originalDirector(req)
		requestInfo := req.Context().Value(request.RequestInfoKey{}).(*request.RequestInfo)

		splitPath, err := request.SplitPath(req.URL.Path)
		if err != nil {
			logger.Error("Failed to split request path", "error", err, "path", req.URL.Path)
			return
		}

		// TODO account for /status or /events endpoints or resource name
		requestedResource, err := request.ParseResourceType(splitPath[len(splitPath)-1])
		if err != nil {
			// If last path element is not a resource type, try the previous one
			requested, err := request.ParseResourceType(splitPath[len(splitPath)-2])
			if err != nil {
				logger.Error("Failed to parse resource type",
					"error", err,
					"path", req.URL.Path,
					"attempted_resource", splitPath[len(splitPath)-2])
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
		default:
			logger.Error("Unsupported HTTP method",
				"method", req.Method,
				"path", req.URL.Path,
				"resource", requestedResource)
			return
		}

		req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", token))
	}

	return proxy, nil
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

// New returns a new Choreo api server instance
func New(logger *logging.Logger, port int, kubeConfig *rest.Config) *Server {
	srv := &Server{
		port:       port,
		logger:     logger,
		kubeConfig: kubeConfig,
	}

	proxy, err := createReverseProxy(logger, kubeConfig)
	if err != nil {
		logger.Error("Failed to create proxy", err.Error())
	}

	newMux := http.NewServeMux()
	var h http.Handler = proxy
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
