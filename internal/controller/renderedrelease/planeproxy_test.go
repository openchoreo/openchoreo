// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package renderedrelease

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"sync"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"

	kubernetesClient "github.com/openchoreo/openchoreo/internal/clients/kubernetes"
)

// planeProxy stands in for the cluster gateway and the plane's agent, so that specs
// run through the real ProxyClient. It serves the gateway's URL, forwards each request
// to envtest the way the agent does, and records it. Hooks change what happens to a
// request, such as a write whose answer is lost.
type planeProxy struct {
	server *httptest.Server
	api    http.RoundTripper
	apiURL *url.URL

	mu       sync.Mutex
	recorded []*planeRequest
	hooks    []*planeHook
}

// planeRequest is one request as the API server saw it.
type planeRequest struct {
	method, path, query, contentType string
	body                             []byte

	status     int
	answerType string
	answer     []byte

	// before and after are the object a write names, read from the API server around
	// the write. Nil when it does not exist, and for reads and creates.
	before, after *unstructured.Unstructured
}

// planeHook changes what the proxy does with the first request it matches, or with
// every one.
type planeHook struct {
	match func(*planeRequest) bool
	every bool

	before func()       // runs before the request is forwarded
	answer *planeAnswer // is sent, and the request is not forwarded
	after  func()       // runs once the API server has answered
	lose   bool         // sends 502 instead of the API server's answer, as when the agent disconnects

	used bool
}

type planeAnswer struct {
	status            int
	contentType, body string
}

// newPlaneProxy starts a proxy in front of envtest. It stops with the spec.
func newPlaneProxy() *planeProxy {
	GinkgoHelper()
	api, err := rest.TransportFor(cfg)
	Expect(err).NotTo(HaveOccurred())
	apiURL, err := url.Parse(cfg.Host)
	Expect(err).NotTo(HaveOccurred())

	p := &planeProxy{api: api, apiURL: apiURL}
	p.server = httptest.NewServer(p)
	DeferCleanup(p.server.Close)
	return p
}

// client returns the plane client production uses, with the proxy as its gateway.
func (p *planeProxy) client() client.Client {
	GinkgoHelper()
	c, err := kubernetesClient.NewProxyClient(p.server.URL, "dataplane/test", "_cluster", "test", nil)
	Expect(err).NotTo(HaveOccurred())
	return c
}

// hook adds a hook. A request uses the first one that matches.
func (p *planeProxy) hook(h planeHook) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.hooks = append(p.hooks, &h)
}

// requests returns what was recorded since the last reset, in order.
func (p *planeProxy) requests() []*planeRequest {
	p.mu.Lock()
	defer p.mu.Unlock()
	return slices.Clone(p.recorded)
}

// reset forgets the recorded requests and the hooks.
func (p *planeProxy) reset() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.recorded, p.hooks = nil, nil
}

// withMethod matches requests by their method.
func withMethod(method string) func(*planeRequest) bool {
	return func(r *planeRequest) bool { return r.method == method }
}

func (p *planeProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	defer GinkgoRecover() // hooks may assert

	// /api/proxy/{planeType}/{planeID}/{namespace}/{crName}/{target}/{path...}
	parts := strings.SplitN(strings.TrimPrefix(r.URL.Path, "/api/proxy/"), "/", 6)
	if len(parts) < 6 || parts[4] != "k8s" {
		http.Error(w, "invalid proxy URL format: "+r.URL.Path, http.StatusBadRequest)
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "failed to read request body", http.StatusBadRequest)
		return
	}
	req := &planeRequest{
		method:      r.Method,
		path:        "/" + parts[5],
		query:       r.URL.RawQuery,
		contentType: r.Header.Get("Content-Type"),
		body:        body,
	}

	hook := p.hookFor(req)
	if hook.before != nil {
		hook.before()
	}
	if hook.answer != nil {
		req.status, req.answerType, req.answer = hook.answer.status, hook.answer.contentType, []byte(hook.answer.body)
	} else if err := p.forward(r, req); err != nil {
		http.Error(w, "proxy request failed: "+err.Error(), http.StatusBadGateway)
		return
	}
	p.record(req)
	if hook.after != nil {
		hook.after()
	}
	if hook.lose {
		http.Error(w, "proxy request failed: agent disconnected", http.StatusBadGateway)
		return
	}
	w.Header().Set("Content-Type", req.answerType)
	w.WriteHeader(req.status)
	_, _ = w.Write(req.answer)
}

// hookFor returns the hook for a request, and an empty one when none matches.
func (p *planeProxy) hookFor(req *planeRequest) *planeHook {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, h := range p.hooks {
		if (h.every || !h.used) && h.match(req) {
			h.used = true
			return h
		}
	}
	return &planeHook{}
}

func (p *planeProxy) record(req *planeRequest) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.recorded = append(p.recorded, req)
}

// forward sends the request to the API server and fills in the answer. Like the agent,
// it finishes a request whose caller has gone away.
func (p *planeProxy) forward(r *http.Request, req *planeRequest) error {
	ctx := context.WithoutCancel(r.Context())
	namesObject := req.method != http.MethodGet && req.method != http.MethodPost
	if namesObject {
		req.before = p.object(ctx, req.path)
	}

	header := r.Header.Clone()
	header.Del("Accept-Encoding") // the transport decompresses only what it asked for itself
	status, answerType, answer, err := p.send(ctx, req.method, req.path, req.query, header, req.body)
	if err != nil {
		return err
	}
	req.status, req.answerType, req.answer = status, answerType, answer

	if namesObject {
		req.after = p.object(ctx, req.path)
	}
	return nil
}

// object reads the object at an API path, and returns nil when there is none.
func (p *planeProxy) object(ctx context.Context, path string) *unstructured.Unstructured {
	GinkgoHelper()
	status, _, body, err := p.send(ctx, http.MethodGet, path, "", http.Header{"Accept": {"application/json"}}, nil)
	Expect(err).NotTo(HaveOccurred())
	if status == http.StatusNotFound {
		return nil
	}
	Expect(status).To(Equal(http.StatusOK), "reading %s: %s", path, body)
	obj := &unstructured.Unstructured{}
	Expect(json.Unmarshal(body, obj)).To(Succeed())
	return obj
}

// send sends one request straight to the API server.
func (p *planeProxy) send(ctx context.Context, method, path, query string, header http.Header,
	body []byte) (status int, answerType string, answer []byte, err error) {
	target := *p.apiURL
	target.Path, target.RawQuery = path, query
	req, err := http.NewRequestWithContext(ctx, method, target.String(), bytes.NewReader(body))
	if err != nil {
		return 0, "", nil, err
	}
	req.Header = header
	resp, err := p.api.RoundTrip(req)
	if err != nil {
		return 0, "", nil, err
	}
	defer resp.Body.Close()
	answer, err = io.ReadAll(resp.Body)
	return resp.StatusCode, resp.Header.Get("Content-Type"), answer, err
}
