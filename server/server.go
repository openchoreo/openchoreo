package server

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"

	"k8s.io/utils/env"

	"github.com/openchoreo/openchoreo/server/middleware"
	"github.com/openchoreo/openchoreo/server/pkg/logging"
	"github.com/openchoreo/openchoreo/server/request"
)

const (
	labelTemplate = "core.choreo.dev"
)

type resourceHandler struct {
	proxy *httputil.ReverseProxy
}

func createReverseProxy() *httputil.ReverseProxy {
	target, _ := url.Parse("https://localhost:58336")
	proxy := httputil.NewSingleHostReverseProxy(target)
	token := env.GetString("AUTH_TOKEN", "")

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

		// Add label selectors later
		// Get organization from token
		basePath := fmt.Sprintf("/apis/core.choreo.dev/v1/namespaces/default-org/%s/%s", string(requestedResource),
			requestInfo.Params[requestedResource].Value)
		req.URL.Path = basePath
		req.URL.RawQuery = addLabelSelectors(requestInfo)
		req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", token))
	}

	return proxy
}

func addLabelSelectors(info *request.RequestInfo) string {
	if len(info.Params) == 0 {
		return ""
	}

	var labels []string
	for resource, pair := range info.Params {
		if pair.Present {
			labels = append(labels, fmt.Sprintf("core.choreo.dev/%s=%s", strings.TrimSuffix(string(resource), "s"), pair.Value))
		}
	}

	if len(labels) > 0 {
		return fmt.Sprint("labelSelector=" + url.QueryEscape(strings.Join(labels, ",")))
	}
	return ""
}

func newResourceHandler() *resourceHandler {
	return &resourceHandler{
		proxy: createReverseProxy(),
	}
}

func (h *resourceHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.proxy.ServeHTTP(w, r)
}

// NewServer returns a new Choreo api server
func NewServer() {
	logger := logging.NewLogger()
	logger.Info("starting server")
	newMux := http.NewServeMux()
	var h http.Handler = newResourceHandler()
	h = middleware.WithRequestInfo(h)
	h = middleware.WithLogging(h, logger)
	h = middleware.WithTracing(h)
	newMux.Handle("/api/v1/", h)
	http.ListenAndServe(":8080", newMux)
}
