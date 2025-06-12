package server

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"

	"github.com/openchoreo/openchoreo/server/middleware"
	"github.com/openchoreo/openchoreo/server/request"
	"k8s.io/utils/env"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type resourceHandler struct {
	client client.Client
	proxy  *httputil.ReverseProxy
}

func createReverseProxy() *httputil.ReverseProxy {
	target, _ := url.Parse("https://localhost:40321")
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
				fmt.Errorf("Failed to parse resource type: %v", err)
				return
			}
			requestedResource = requested
		}

		// How do we handle get operations?
		// Add label selectors later
		switch requestedResource {
		case request.ProjectType:
			// Get organization from token
			newPath := fmt.Sprintf("/apis/core.choreo.dev/v1/namespaces/default-org/projects/%s", requestInfo.Params[request.ProjectType].Value)
			req.URL.Path = newPath
		case request.ComponentType:
			newPath := fmt.Sprintf("/apis/core.choreo.dev/v1/namespaces/default-org/components/%s",
				requestInfo.Params[request.ComponentType].Value)
			req.URL.Path = newPath
		case request.DeploymentType:
			newPath := fmt.Sprintf("/apis/core.choreo.dev/v1/namespaces/default-org/deployments/%s",
				requestInfo.Params[request.DeploymentType].Value)
			req.URL.Path = newPath
		}
		req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", token))
	}

	return proxy
}

func newResourceHandler() *resourceHandler {
	return &resourceHandler{
		proxy: createReverseProxy(),
	}
}

func (h *resourceHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.proxy.ServeHTTP(w, r)
}

// Returns a new Choreo api server
func NewServer() {
	newMux := http.NewServeMux()
	chain := middleware.WithRequestInfo(newResourceHandler())
	newMux.Handle("/api/v1/", chain)
	http.ListenAndServe(":8080", newMux)
}
