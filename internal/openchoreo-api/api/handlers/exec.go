// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	"github.com/gorilla/websocket"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	authz "github.com/openchoreo/openchoreo/internal/authz/core"
	gatewayClient "github.com/openchoreo/openchoreo/internal/clients/gateway"
	"github.com/openchoreo/openchoreo/internal/controller"
	svcpkg "github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
)

// ExecHandler handles WebSocket exec requests for component pods.
type ExecHandler struct {
	k8sClient      client.Client
	gatewayClient  *gatewayClient.Client
	gatewayURL     string
	gatewayTLSConf *tls.Config
	authzChecker   *svcpkg.AuthzChecker
	logger         *slog.Logger
}

// NewExecHandler creates a new exec handler.
func NewExecHandler(k8sClient client.Client, gwClient *gatewayClient.Client, gatewayURL string, gwTLSConf *tls.Config, authzChecker *svcpkg.AuthzChecker, logger *slog.Logger) *ExecHandler {
	return &ExecHandler{
		k8sClient:      k8sClient,
		gatewayClient:  gwClient,
		gatewayURL:     gatewayURL,
		gatewayTLSConf: gwTLSConf,
		authzChecker:   authzChecker,
		logger:         logger.With("component", "exec-handler"),
	}
}

var execUpgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// ServeHTTP handles the exec WebSocket upgrade and bidirectional streaming.
// URL: /exec/namespaces/{namespace}/components/{component}?env=...&container=...&command=...&tty=...&stdin=...
func (h *ExecHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Parse URL path: /exec/namespaces/{namespace}/components/{component}
	path := strings.TrimPrefix(r.URL.Path, "/exec/namespaces/")
	parts := strings.SplitN(path, "/components/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		http.Error(w, "invalid exec URL: expected /exec/namespaces/{ns}/components/{name}", http.StatusBadRequest)
		return
	}
	namespace := parts[0]
	componentName := parts[1]

	query := r.URL.Query()
	project := query.Get("project")
	envName := query.Get("env")
	container := query.Get("container")
	commands := query["command"]
	tty := query.Get("tty") == "true"
	stdin := query.Get("stdin") == "true"

	ctx := r.Context()
	logger := h.logger.With("namespace", namespace, "component", componentName)

	// Authorize: check that the caller has component:exec permission.
	if h.authzChecker == nil {
		logger.Error("Authorization checker not configured")
		http.Error(w, "authorization not configured", http.StatusInternalServerError)
		return
	}
	{
		if err := h.authzChecker.Check(ctx, svcpkg.CheckRequest{
			Action:       authz.ActionExecComponent,
			ResourceType: "component",
			ResourceID:   componentName,
			Hierarchy: authz.ResourceHierarchy{
				Namespace: namespace,
				Project:   project,
			},
		}); err != nil {
			if errors.Is(err, svcpkg.ErrForbidden) {
				http.Error(w, "you do not have permission to exec into this component", http.StatusForbidden)
				return
			}
			logger.Error("Authorization check failed", "error", err)
			http.Error(w, "authorization check failed", http.StatusInternalServerError)
			return
		}
	}

	// Resolve the pod to exec into
	podInfo, err := h.resolvePod(ctx, namespace, componentName, project, envName)
	if err != nil {
		logger.Error("Failed to resolve pod for exec", "error", err)
		http.Error(w, fmt.Sprintf("failed to resolve pod: %v", err), http.StatusBadRequest)
		return
	}

	logger = logger.With("pod", podInfo.podName, "podNamespace", podInfo.podNamespace,
		"planeType", podInfo.plane.planeType, "planeID", podInfo.plane.planeID)

	// Upgrade client connection to WebSocket
	clientConn, err := execUpgrader.Upgrade(w, r, nil)
	if err != nil {
		logger.Error("Failed to upgrade to WebSocket", "error", err)
		return
	}
	defer clientConn.Close()

	// Build gateway exec WebSocket URL
	gwExecURL, err := h.buildGatewayExecURL(podInfo, container, commands, tty, stdin)
	if err != nil {
		logger.Error("Failed to build gateway exec URL", "error", err)
		writeWSError(clientConn, fmt.Sprintf("internal error: %v", err))
		return
	}

	// Connect to gateway exec WebSocket using the same TLS config as the gateway client.
	gwDialer := websocket.Dialer{
		TLSClientConfig: h.gatewayTLSConf,
	}
	gwConn, _, err := gwDialer.DialContext(ctx, gwExecURL, nil)
	if err != nil {
		logger.Error("Failed to connect to gateway exec endpoint", "error", err)
		writeWSError(clientConn, fmt.Sprintf("failed to connect to data plane: %v", err))
		return
	}
	defer gwConn.Close()

	logger.Info("Exec session established")

	// Bidirectional bridge: client ↔ gateway
	// Buffer of 2 so both goroutines can signal completion without blocking.
	done := make(chan struct{}, 2)

	// client → gateway
	go func() {
		defer func() { done <- struct{}{} }()
		for {
			msgType, msg, err := clientConn.ReadMessage()
			if err != nil {
				return
			}
			if err := gwConn.WriteMessage(msgType, msg); err != nil {
				return
			}
		}
	}()

	// gateway → client
	go func() {
		defer func() { done <- struct{}{} }()
		for {
			msgType, msg, err := gwConn.ReadMessage()
			if err != nil {
				// Gateway closed — forward the close status to the CLI.
				closeCode := websocket.CloseNormalClosure
				closeText := ""
				var ce *websocket.CloseError
				if errors.As(err, &ce) {
					closeCode = ce.Code
					closeText = ce.Text
				}
				_ = clientConn.WriteMessage(websocket.CloseMessage,
					websocket.FormatCloseMessage(closeCode, closeText))
				return
			}
			if err := clientConn.WriteMessage(msgType, msg); err != nil {
				return
			}
		}
	}()

	<-done
	logger.Info("Exec session ended")
}

type execPlaneInfo struct {
	planeType   string
	planeID     string
	crNamespace string
	crName      string
}

type execPodInfo struct {
	podNamespace string
	podName      string
	plane        execPlaneInfo
}

// resolvePod resolves the target pod for exec by traversing:
// component → environment → dataplane → pod (first Ready pod matching labels)
func (h *ExecHandler) resolvePod(ctx context.Context, namespace, componentName, project, envName string) (*execPodInfo, error) {
	// Verify the component exists
	comp := &openchoreov1alpha1.Component{}
	if err := h.k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: componentName}, comp); err != nil {
		return nil, fmt.Errorf("component %q not found: %w", componentName, err)
	}

	// Resolve environment
	if envName == "" {
		// Find project to get deployment pipeline
		if project == "" {
			return nil, fmt.Errorf("--project or --env is required")
		}
		resolvedEnv, err := h.resolveLowestEnvironment(ctx, namespace, project)
		if err != nil {
			return nil, err
		}
		envName = resolvedEnv
	}

	env := &openchoreov1alpha1.Environment{}
	if err := h.k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: envName}, env); err != nil {
		return nil, fmt.Errorf("environment %q not found: %w", envName, err)
	}

	if env.Spec.DataPlaneRef == nil {
		return nil, fmt.Errorf("environment %q has no data plane reference", envName)
	}

	// Resolve data plane
	dpResult, err := controller.GetDataPlaneFromRef(ctx, h.k8sClient, env.Namespace, env.Spec.DataPlaneRef)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve data plane: %w", err)
	}

	plane := resolveExecPlaneInfo(dpResult)
	if plane.planeID == "" {
		return nil, fmt.Errorf("failed to determine plane ID for environment %q", envName)
	}

	// Find a pod via the gateway K8s proxy
	podNamespace, podName, err := h.findReadyPod(ctx, plane, namespace, componentName, envName)
	if err != nil {
		return nil, err
	}

	return &execPodInfo{
		podNamespace: podNamespace,
		podName:      podName,
		plane:        plane,
	}, nil
}

func resolveExecPlaneInfo(dpResult *controller.DataPlaneResult) execPlaneInfo {
	if dpResult.DataPlane != nil {
		dp := dpResult.DataPlane
		id := dp.Spec.PlaneID
		if id == "" {
			id = dp.Name
		}
		return execPlaneInfo{planeType: "dataplane", planeID: id, crNamespace: dp.Namespace, crName: dp.Name}
	}
	if dpResult.ClusterDataPlane != nil {
		cdp := dpResult.ClusterDataPlane
		id := cdp.Spec.PlaneID
		if id == "" {
			id = cdp.Name
		}
		return execPlaneInfo{planeType: "dataplane", planeID: id, crNamespace: "_cluster", crName: cdp.Name}
	}
	return execPlaneInfo{}
}

// findReadyPod lists pods via the gateway K8s proxy and returns the first Ready pod.
func (h *ExecHandler) findReadyPod(ctx context.Context, plane execPlaneInfo, namespace, componentName, envName string) (string, string, error) {
	if h.gatewayClient == nil {
		return "", "", fmt.Errorf("gateway client is not configured")
	}

	// Build label selector to find the component's runtime pods.
	// Pod labels in the data plane use "openchoreo.dev/" prefixed keys.
	labelSelector := url.QueryEscape(fmt.Sprintf(
		"openchoreo.dev/component=%s,openchoreo.dev/environment=%s,openchoreo.dev/namespace=%s",
		componentName, envName, namespace,
	))

	// List pods across all namespaces in the data plane - use a namespace that's likely to match
	// The actual K8s namespace in the data plane is derived from the rendered release
	k8sPath := "api/v1/pods"
	rawQuery := "labelSelector=" + labelSelector + "&limit=10"

	resp, err := h.gatewayClient.ProxyK8sRequest(ctx, plane.planeType, plane.planeID, plane.crNamespace, plane.crName, k8sPath, rawQuery)
	if err != nil {
		return "", "", fmt.Errorf("failed to list pods from data plane: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", "", fmt.Errorf("failed to list pods (HTTP %d): %s", resp.StatusCode, string(body))
	}

	var podList struct {
		Items []struct {
			Metadata struct {
				Name      string `json:"name"`
				Namespace string `json:"namespace"`
			} `json:"metadata"`
			Status struct {
				Phase      string `json:"phase"`
				Conditions []struct {
					Type   string `json:"type"`
					Status string `json:"status"`
				} `json:"conditions"`
			} `json:"status"`
		} `json:"items"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&podList); err != nil {
		return "", "", fmt.Errorf("failed to parse pod list: %w", err)
	}

	const podPhaseRunning = "Running"

	if len(podList.Items) == 0 {
		return "", "", fmt.Errorf("no running pods found for component %q in environment %q", componentName, envName)
	}

	// Find first Ready pod
	for _, pod := range podList.Items {
		if pod.Status.Phase != podPhaseRunning {
			continue
		}
		for _, cond := range pod.Status.Conditions {
			if cond.Type == "Ready" && cond.Status == "True" {
				return pod.Metadata.Namespace, pod.Metadata.Name, nil
			}
		}
	}

	// Fallback: first Running pod even if not fully Ready
	for _, pod := range podList.Items {
		if pod.Status.Phase == podPhaseRunning {
			return pod.Metadata.Namespace, pod.Metadata.Name, nil
		}
	}

	return "", "", fmt.Errorf("no running pods found for component %q in environment %q", componentName, envName)
}

// resolveLowestEnvironment finds the root environment from the project's deployment pipeline.
func (h *ExecHandler) resolveLowestEnvironment(ctx context.Context, namespace, projectName string) (string, error) {
	proj := &openchoreov1alpha1.Project{}
	if err := h.k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: projectName}, proj); err != nil {
		return "", fmt.Errorf("project %q not found: %w", projectName, err)
	}

	if proj.Spec.DeploymentPipelineRef.Name == "" {
		return "", fmt.Errorf("project %q has no deployment pipeline configured", projectName)
	}

	pipeline := &openchoreov1alpha1.DeploymentPipeline{}
	if err := h.k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: proj.Spec.DeploymentPipelineRef.Name}, pipeline); err != nil {
		return "", fmt.Errorf("deployment pipeline not found: %w", err)
	}

	if len(pipeline.Spec.PromotionPaths) == 0 {
		return "", fmt.Errorf("deployment pipeline has no promotion paths")
	}

	// Find root environment (source that is never a target)
	targets := make(map[string]bool)
	for _, path := range pipeline.Spec.PromotionPaths {
		for _, target := range path.TargetEnvironmentRefs {
			targets[target.Name] = true
		}
	}

	for _, path := range pipeline.Spec.PromotionPaths {
		if path.SourceEnvironmentRef.Name != "" && !targets[path.SourceEnvironmentRef.Name] {
			return path.SourceEnvironmentRef.Name, nil
		}
	}

	return "", fmt.Errorf("no root environment found in deployment pipeline")
}

// buildGatewayExecURL constructs the WebSocket URL for the gateway exec endpoint.
func (h *ExecHandler) buildGatewayExecURL(podInfo *execPodInfo, container string, commands []string, tty, stdin bool) (string, error) {
	u, err := url.Parse(h.gatewayURL)
	if err != nil {
		return "", err
	}

	switch u.Scheme {
	case "https":
		u.Scheme = "wss"
	case "http":
		u.Scheme = "ws"
	}

	u.Path = fmt.Sprintf("/api/exec/%s/%s/%s/%s",
		podInfo.plane.planeType, podInfo.plane.planeID,
		podInfo.plane.crNamespace, podInfo.plane.crName)

	q := u.Query()
	q.Set("podNamespace", podInfo.podNamespace)
	q.Set("podName", podInfo.podName)
	if container != "" {
		q.Set("container", container)
	}
	if tty {
		q.Set("tty", "true")
	}
	if stdin {
		q.Set("stdin", "true")
	}
	for _, cmd := range commands {
		q.Add("command", cmd)
	}
	u.RawQuery = q.Encode()

	return u.String(), nil
}

const execStreamStderrByte = byte(2)

func writeWSError(conn *websocket.Conn, msg string) {
	payload := msg + "\n"
	errMsg := make([]byte, 1+len(payload))
	errMsg[0] = execStreamStderrByte
	copy(errMsg[1:], payload)
	_ = conn.WriteMessage(websocket.BinaryMessage, errMsg)
	_ = conn.WriteMessage(websocket.CloseMessage,
		websocket.FormatCloseMessage(websocket.CloseInternalServerErr, msg))
}
