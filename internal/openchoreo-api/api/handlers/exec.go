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
	apierrors "k8s.io/apimachinery/pkg/api/errors"
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
	podName := query.Get("pod")
	commands := query["command"]
	tty := query.Get("tty") == "true"
	stdin := query.Get("stdin") == "true"

	ctx := r.Context()
	logger := h.logger.With("namespace", namespace, "component", componentName)
	logger.Info("Exec request received", "env", envName, "pod", podName, "container", container)

	// Resolve the target environment before authorizing so per-environment exec
	// conditions are evaluated against it (`env` may be omitted by the client).
	effectiveEnv, err := h.resolveEnvName(ctx, namespace, project, envName)
	if err != nil {
		status := http.StatusBadRequest
		var infraErr *execInfraError
		if errors.As(err, &infraErr) {
			status = http.StatusServiceUnavailable
		}
		logger.Warn("Failed to resolve environment for exec", "error", err)
		http.Error(w, fmt.Sprintf("failed to resolve environment: %v", err), status)
		return
	}

	// Authorize: check that the caller has component:exec permission for this environment.
	if h.authzChecker == nil {
		logger.Error("Authorization checker not configured")
		http.Error(w, "authorization not configured", http.StatusInternalServerError)
		return
	}
	if err := h.authzChecker.Check(ctx, svcpkg.CheckRequest{
		Action:       authz.ActionExecComponent,
		ResourceType: "component",
		ResourceID:   componentName,
		Hierarchy: authz.ResourceHierarchy{
			Namespace: namespace,
			Project:   project,
		},
		Context: authz.Context{
			Resource: authz.ResourceAttribute{
				Environment: svcpkg.FormatDualScopedResourceName(namespace, effectiveEnv, false),
			},
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

	// Resolve the pod to exec into
	podInfo, err := h.resolvePod(ctx, namespace, componentName, project, effectiveEnv, podName)
	if err != nil {
		status := http.StatusBadRequest
		var infraErr *execInfraError
		if errors.As(err, &infraErr) {
			logger.Error("Infrastructure error resolving pod for exec", "error", err)
			status = http.StatusServiceUnavailable
		} else {
			logger.Warn("Failed to resolve pod for exec", "error", err)
		}
		http.Error(w, fmt.Sprintf("failed to resolve pod: %v", err), status)
		return
	}

	// Validate container name if specified
	if container != "" {
		found := false
		for _, c := range podInfo.containers {
			if c == container {
				found = true
				break
			}
		}
		if !found {
			logger.Warn("Container not found in pod", "container", container,
				"pod", podInfo.podName, "available", podInfo.containers)
			http.Error(w, fmt.Sprintf("container %q not found in pod %q (available: %s)",
				container, podInfo.podName, strings.Join(podInfo.containers, ", ")), http.StatusBadRequest)
			return
		}
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
	containers   []string // container names present in the pod
	plane        execPlaneInfo
}

// resolveEnvName returns the effective environment, deriving the lowest env from
// the project's deployment pipeline when the `env` query param is omitted.
func (h *ExecHandler) resolveEnvName(ctx context.Context, namespace, project, envName string) (string, error) {
	if envName != "" {
		return envName, nil
	}
	if project == "" {
		return "", fmt.Errorf("--project or --env is required")
	}
	return h.resolveLowestEnvironment(ctx, namespace, project)
}

// resolvePod resolves the target pod for exec by traversing:
// component → environment → dataplane → pod (specific pod if podName set, else first Ready pod)
func (h *ExecHandler) resolvePod(ctx context.Context, namespace, componentName, project, envName, podName string) (*execPodInfo, error) {
	// Verify the component exists
	comp := &openchoreov1alpha1.Component{}
	if err := h.k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: componentName}, comp); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, fmt.Errorf("component %q not found in namespace %q", componentName, namespace)
		}
		return nil, infraErrorf("failed to look up component %q: %w", componentName, err)
	}

	// Resolve environment — if not specified, derive from the project's deployment pipeline.
	// resolveLowestEnvironment validates project existence, so no separate check is needed here.
	if envName == "" {
		if project == "" {
			return nil, fmt.Errorf("--project or --env is required")
		}
		resolvedEnv, err := h.resolveLowestEnvironment(ctx, namespace, project)
		if err != nil {
			return nil, err
		}
		envName = resolvedEnv
	} else if project != "" {
		// --env was provided but --project was also given: validate that the project exists.
		proj := &openchoreov1alpha1.Project{}
		if err := h.k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: project}, proj); err != nil {
			if apierrors.IsNotFound(err) {
				return nil, fmt.Errorf("project %q not found in namespace %q", project, namespace)
			}
			return nil, infraErrorf("failed to look up project %q: %w", project, err)
		}
	}

	env := &openchoreov1alpha1.Environment{}
	if err := h.k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: envName}, env); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, fmt.Errorf("environment %q not found in namespace %q", envName, namespace)
		}
		return nil, infraErrorf("failed to look up environment %q: %w", envName, err)
	}

	if env.Spec.DataPlaneRef == nil {
		return nil, fmt.Errorf("environment %q has no data plane reference", envName)
	}

	// Resolve data plane
	dpResult, err := controller.GetDataPlaneFromRef(ctx, h.k8sClient, env.Namespace, env.Spec.DataPlaneRef)
	if err != nil {
		return nil, infraErrorf("failed to resolve data plane: %w", err)
	}

	plane := resolveExecPlaneInfo(dpResult)
	if plane.planeID == "" {
		return nil, fmt.Errorf("failed to determine plane ID for environment %q", envName)
	}

	// Find a pod via the gateway K8s proxy
	var resolvedNamespace, resolvedPodName string
	var resolvedContainers []string
	if podName != "" {
		resolvedNamespace, resolvedPodName, resolvedContainers, err = h.findNamedPod(ctx, plane, namespace, componentName, envName, podName)
	} else {
		resolvedNamespace, resolvedPodName, resolvedContainers, err = h.findReadyPod(ctx, plane, namespace, componentName, envName)
	}
	if err != nil {
		return nil, err
	}

	return &execPodInfo{
		podNamespace: resolvedNamespace,
		podName:      resolvedPodName,
		containers:   resolvedContainers,
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

// podListItem is the minimal pod shape needed for exec resolution.
type podListItem struct {
	Metadata struct {
		Name      string `json:"name"`
		Namespace string `json:"namespace"`
	} `json:"metadata"`
	Spec struct {
		Containers []struct {
			Name string `json:"name"`
		} `json:"containers"`
	} `json:"spec"`
	Status struct {
		Phase      string `json:"phase"`
		Conditions []struct {
			Type   string `json:"type"`
			Status string `json:"status"`
		} `json:"conditions"`
	} `json:"status"`
}

func containerNamesFrom(pod podListItem) []string {
	names := make([]string, len(pod.Spec.Containers))
	for i, c := range pod.Spec.Containers {
		names[i] = c.Name
	}
	return names
}

// findReadyPod lists pods via the gateway K8s proxy and returns the first Ready pod.
func (h *ExecHandler) findReadyPod(ctx context.Context, plane execPlaneInfo, namespace, componentName, envName string) (string, string, []string, error) {
	if h.gatewayClient == nil {
		return "", "", nil, fmt.Errorf("gateway client is not configured")
	}

	q := url.Values{}
	q.Set("labelSelector", fmt.Sprintf(
		"openchoreo.dev/component=%s,openchoreo.dev/environment=%s,openchoreo.dev/namespace=%s",
		componentName, envName, namespace,
	))
	q.Set("limit", execPodListLimit)

	resp, err := h.gatewayClient.ProxyK8sRequest(ctx, plane.planeType, plane.planeID, plane.crNamespace, plane.crName, "api/v1/pods", q.Encode())
	if err != nil {
		return "", "", nil, infraErrorf("failed to list pods from data plane: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", "", nil, infraErrorf("failed to list pods (HTTP %d): %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var podList struct {
		Items []podListItem `json:"items"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&podList); err != nil {
		return "", "", nil, infraErrorf("failed to parse pod list: %w", err)
	}

	if len(podList.Items) == 0 {
		return "", "", nil, fmt.Errorf("no running pods found for component %q in environment %q", componentName, envName)
	}

	// Find first Ready pod
	for _, pod := range podList.Items {
		if pod.Status.Phase != podPhaseRunning {
			continue
		}
		for _, cond := range pod.Status.Conditions {
			if cond.Type == "Ready" && cond.Status == "True" {
				return pod.Metadata.Namespace, pod.Metadata.Name, containerNamesFrom(pod), nil
			}
		}
	}

	// Fallback: first Running pod even if not fully Ready
	for _, pod := range podList.Items {
		if pod.Status.Phase == podPhaseRunning {
			return pod.Metadata.Namespace, pod.Metadata.Name, containerNamesFrom(pod), nil
		}
	}

	return "", "", nil, fmt.Errorf("no running pods found for component %q in environment %q", componentName, envName)
}

// findNamedPod finds a specific pod by name and verifies it belongs to the component and is ready.
// It uses only a label selector (not fieldSelector) because the gateway proxy may not support
// fieldSelector passthrough — pod name matching is done in application code.
func (h *ExecHandler) findNamedPod(ctx context.Context, plane execPlaneInfo, namespace, componentName, envName, podName string) (string, string, []string, error) {
	if h.gatewayClient == nil {
		return "", "", nil, fmt.Errorf("gateway client is not configured")
	}

	q := url.Values{}
	q.Set("labelSelector", fmt.Sprintf(
		"openchoreo.dev/component=%s,openchoreo.dev/environment=%s,openchoreo.dev/namespace=%s",
		componentName, envName, namespace,
	))
	q.Set("limit", execPodListLimit)

	resp, err := h.gatewayClient.ProxyK8sRequest(ctx, plane.planeType, plane.planeID, plane.crNamespace, plane.crName, "api/v1/pods", q.Encode())
	if err != nil {
		return "", "", nil, infraErrorf("failed to list pods for component %q: %w", componentName, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", "", nil, infraErrorf("failed to list pods (HTTP %d): %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var podList struct {
		Items []podListItem `json:"items"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&podList); err != nil {
		return "", "", nil, infraErrorf("failed to parse pod list: %w", err)
	}

	// Filter by pod name in application code — fieldSelector may not be supported by the gateway proxy
	for _, pod := range podList.Items {
		if pod.Metadata.Name != podName {
			continue
		}
		for _, cond := range pod.Status.Conditions {
			if cond.Type == "Ready" && cond.Status == "True" {
				return pod.Metadata.Namespace, pod.Metadata.Name, containerNamesFrom(pod), nil
			}
		}
		return "", "", nil, fmt.Errorf("pod %q is not ready (phase: %s)", podName, pod.Status.Phase)
	}

	return "", "", nil, fmt.Errorf("pod %q not found for component %q in environment %q", podName, componentName, envName)
}

// resolveLowestEnvironment finds the root environment from the project's deployment pipeline.
func (h *ExecHandler) resolveLowestEnvironment(ctx context.Context, namespace, projectName string) (string, error) {
	proj := &openchoreov1alpha1.Project{}
	if err := h.k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: projectName}, proj); err != nil {
		if apierrors.IsNotFound(err) {
			return "", fmt.Errorf("project %q not found in namespace %q", projectName, namespace)
		}
		return "", infraErrorf("failed to look up project %q: %w", projectName, err)
	}

	if proj.Spec.DeploymentPipelineRef.Name == "" {
		return "", fmt.Errorf("project %q has no deployment pipeline configured", projectName)
	}

	pipeline := &openchoreov1alpha1.DeploymentPipeline{}
	if err := h.k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: proj.Spec.DeploymentPipelineRef.Name}, pipeline); err != nil {
		if apierrors.IsNotFound(err) {
			return "", fmt.Errorf("deployment pipeline %q not found in namespace %q", proj.Spec.DeploymentPipelineRef.Name, namespace)
		}
		return "", infraErrorf("failed to look up deployment pipeline %q: %w", proj.Spec.DeploymentPipelineRef.Name, err)
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

const (
	execStreamStderrByte = byte(2)
	podPhaseRunning      = "Running"
	// execPodListLimit caps the number of pods fetched per list call.
	// 100 covers typical deployment scales; both find functions use this value.
	execPodListLimit = "100"
)

// execInfraError wraps errors that are caused by infrastructure failures
// (gateway unreachable, data plane unavailable) rather than bad user input.
// ServeHTTP uses this to return 503 instead of 400 for those cases.
type execInfraError struct{ cause error }

func (e *execInfraError) Error() string { return e.cause.Error() }
func (e *execInfraError) Unwrap() error { return e.cause }

func infraErrorf(format string, args ...any) error {
	return &execInfraError{cause: fmt.Errorf(format, args...)}
}

func writeWSError(conn *websocket.Conn, msg string) {
	payload := msg + "\n"
	errMsg := make([]byte, 1+len(payload))
	errMsg[0] = execStreamStderrByte
	copy(errMsg[1:], payload)
	_ = conn.WriteMessage(websocket.BinaryMessage, errMsg)
	_ = conn.WriteMessage(websocket.CloseMessage,
		websocket.FormatCloseMessage(websocket.CloseInternalServerErr, msg))
}
