// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clusteragent

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"strings"
	"sync"

	"github.com/gorilla/websocket"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/remotecommand"

	"github.com/openchoreo/openchoreo/internal/cluster-agent/messaging"
)

// Stream type prefixes for the exec WebSocket protocol (must match CLI).
const (
	execStreamStdin  = byte(0)
	execStreamStdout = byte(1)
	execStreamStderr = byte(2)
	execStreamResize = byte(3)
)

// execSession represents an active exec streaming session in the agent.
type execSession struct {
	requestID string
	stdinPipe *stdinPipeReader
	resizeCh  chan remotecommand.TerminalSize
	cancel    context.CancelFunc
	done      chan struct{}
	once      sync.Once
}

func newExecSession(requestID string, cancel context.CancelFunc) *execSession {
	return &execSession{
		requestID: requestID,
		stdinPipe: newStdinPipeReader(),
		resizeCh:  make(chan remotecommand.TerminalSize, 8),
		cancel:    cancel,
		done:      make(chan struct{}),
	}
}

func (s *execSession) close() {
	s.once.Do(func() {
		close(s.done)
		s.stdinPipe.Close()
		s.cancel()
	})
}

// stdinPipeReader implements io.Reader by consuming data from a channel.
type stdinPipeReader struct {
	ch     chan []byte
	buf    []byte
	closed bool
	mu     sync.Mutex
}

func newStdinPipeReader() *stdinPipeReader {
	return &stdinPipeReader{ch: make(chan []byte, 64)}
}

func (r *stdinPipeReader) Read(p []byte) (int, error) {
	r.mu.Lock()
	if len(r.buf) > 0 {
		n := copy(p, r.buf)
		r.buf = r.buf[n:]
		r.mu.Unlock()
		return n, nil
	}
	if r.closed {
		r.mu.Unlock()
		return 0, io.EOF
	}
	r.mu.Unlock()

	data, ok := <-r.ch
	if !ok || len(data) == 0 {
		return 0, io.EOF
	}

	n := copy(p, data)
	if n < len(data) {
		r.mu.Lock()
		r.buf = append(r.buf, data[n:]...)
		r.mu.Unlock()
	}
	return n, nil
}

func (r *stdinPipeReader) Write(data []byte) {
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return
	}
	// Hold the lock across the send so Close cannot close the channel concurrently.
	// Use a select with a default to avoid blocking the agent tunnel if the
	// consumer is slow; the Read side will drain the channel.
	select {
	case r.ch <- data:
	default:
		// Channel full — append to internal buffer so data is not lost.
		r.buf = append(r.buf, data...)
	}
	r.mu.Unlock()
}

func (r *stdinPipeReader) Close() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.closed {
		r.closed = true
		close(r.ch)
	}
}

// streamWriter sends data back through the agent tunnel as stream chunks.
type streamWriter struct {
	agent      *Agent
	requestID  string
	streamType byte
}

func (w *streamWriter) Write(p []byte) (int, error) {
	// Frame the data with the stream type prefix (matching CLI protocol)
	framed := make([]byte, 1+len(p))
	framed[0] = w.streamType
	copy(framed[1:], p)

	chunk := &messaging.HTTPTunnelStreamChunk{
		RequestID: w.requestID,
		Data:      framed,
		StreamID:  int(w.streamType),
	}
	if err := w.agent.sendStreamChunk(chunk); err != nil {
		return 0, err
	}
	return len(p), nil
}

// termSizeQueue implements remotecommand.TerminalSizeQueue
type termSizeQueue struct {
	session *execSession
}

func (q *termSizeQueue) Next() *remotecommand.TerminalSize {
	select {
	case size, ok := <-q.session.resizeCh:
		if !ok {
			return nil
		}
		return &size
	case <-q.session.done:
		return nil
	}
}

// handleHTTPTunnelStreamInit handles a new exec stream request.
func (a *Agent) handleHTTPTunnelStreamInit(init *messaging.HTTPTunnelStreamInit) {
	logger := a.logger.With("requestID", init.RequestID, "path", init.Path)
	logger.Info("Received exec stream init")

	podNamespace, podName, err := parseExecPath(init.Path)
	if err != nil {
		logger.Error("Failed to parse exec path", "error", err)
		a.sendStreamClose(init.RequestID, fmt.Sprintf("invalid exec path: %v", err))
		return
	}

	params, err := url.ParseQuery(init.Query)
	if err != nil {
		logger.Error("Failed to parse exec query", "error", err)
		a.sendStreamClose(init.RequestID, fmt.Sprintf("invalid exec query: %v", err))
		return
	}

	commands := params["command"]
	if len(commands) == 0 {
		commands = []string{"/bin/sh"}
	}
	container := params.Get("container")
	tty := params.Get("tty") == "true"
	stdin := params.Get("stdin") == "true"

	logger = logger.With("pod", podName, "namespace", podNamespace)

	ctx, cancel := context.WithCancel(context.Background())
	session := newExecSession(init.RequestID, cancel)

	a.activeStreamsMu.Lock()
	a.activeStreams[init.RequestID] = session
	a.activeStreamsMu.Unlock()

	defer func() {
		session.close()
		a.activeStreamsMu.Lock()
		delete(a.activeStreams, init.RequestID)
		a.activeStreamsMu.Unlock()
	}()

	// Build the exec URL for the K8s API server
	execOpts := &corev1.PodExecOptions{
		Command: commands,
		Stdout:  true,
		Stderr:  true,
		Stdin:   stdin,
		TTY:     tty,
	}
	if container != "" {
		execOpts.Container = container
	}

	execURL := buildExecURL(a.k8sConfig.Host, podNamespace, podName, execOpts)

	// Ensure the scheme is registered
	_ = corev1.AddToScheme(scheme.Scheme)

	exec, err := remotecommand.NewSPDYExecutor(a.k8sConfig, "POST", execURL)
	if err != nil {
		logger.Error("Failed to create SPDY executor", "error", err)
		a.sendStreamClose(init.RequestID, fmt.Sprintf("failed to create executor: %v", err))
		return
	}

	stdoutWriter := &streamWriter{agent: a, requestID: init.RequestID, streamType: execStreamStdout}
	stderrWriter := &streamWriter{agent: a, requestID: init.RequestID, streamType: execStreamStderr}

	streamOpts := remotecommand.StreamOptions{
		Stdout: stdoutWriter,
		Stderr: stderrWriter,
	}
	if stdin {
		streamOpts.Stdin = session.stdinPipe
	}
	if tty {
		streamOpts.Tty = true
		streamOpts.TerminalSizeQueue = &termSizeQueue{session: session}
	}

	// Send initial chunk so the gateway knows exec is active
	a.sendStreamChunkRaw(init.RequestID, []byte{execStreamStdout}, 1)

	logger.Info("Starting exec stream")

	if err := exec.StreamWithContext(ctx, streamOpts); err != nil {
		logger.Warn("Exec stream ended with error", "error", err)
	}

	logger.Info("Exec stream completed")
	a.sendStreamClose(init.RequestID, "")
}

// routeStreamChunk routes an incoming stream chunk to the correct exec session.
func (a *Agent) routeStreamChunk(chunk *messaging.HTTPTunnelStreamChunk) {
	a.activeStreamsMu.Lock()
	session, ok := a.activeStreams[chunk.RequestID]
	a.activeStreamsMu.Unlock()

	if !ok {
		a.logger.Warn("Received stream chunk for unknown session", "requestID", chunk.RequestID)
		return
	}

	if chunk.IsClose {
		session.close()
		return
	}

	if len(chunk.Data) < 1 {
		return
	}

	// Data is framed: first byte = stream type, rest = payload
	switch chunk.Data[0] {
	case execStreamStdin:
		if len(chunk.Data) > 1 {
			session.stdinPipe.Write(chunk.Data[1:])
		} else {
			session.stdinPipe.Close()
		}
	case execStreamResize:
		if len(chunk.Data) > 1 {
			var size struct {
				Width  uint16 `json:"width"`
				Height uint16 `json:"height"`
			}
			if err := json.Unmarshal(chunk.Data[1:], &size); err == nil {
				select {
				case session.resizeCh <- remotecommand.TerminalSize{Width: size.Width, Height: size.Height}:
				default:
				}
			}
		}
	}
}

func (a *Agent) sendStreamChunk(chunk *messaging.HTTPTunnelStreamChunk) error {
	data, err := json.Marshal(chunk)
	if err != nil {
		return fmt.Errorf("failed to marshal stream chunk: %w", err)
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	if a.conn == nil {
		return messaging.ErrNotConnected
	}
	return a.conn.WriteMessage(websocket.TextMessage, data)
}

func (a *Agent) sendStreamChunkRaw(requestID string, data []byte, streamID int) {
	chunk := &messaging.HTTPTunnelStreamChunk{
		RequestID: requestID,
		Data:      data,
		StreamID:  streamID,
	}
	_ = a.sendStreamChunk(chunk)
}

func (a *Agent) sendStreamClose(requestID string, errMsg string) {
	chunk := &messaging.HTTPTunnelStreamChunk{
		RequestID: requestID,
		IsClose:   true,
	}
	if errMsg != "" {
		chunk.Data = []byte(errMsg)
	}
	_ = a.sendStreamChunk(chunk)
}

// parseExecPath extracts namespace and pod name from a K8s exec path.
// Expected: /api/v1/namespaces/{namespace}/pods/{pod}/exec
func parseExecPath(path string) (namespace, podName string, err error) {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	// api/v1/namespaces/{ns}/pods/{pod}/exec → 7 parts
	if len(parts) < 7 {
		return "", "", fmt.Errorf("path too short: %s", path)
	}
	if parts[2] != "namespaces" || parts[4] != "pods" {
		return "", "", fmt.Errorf("unexpected path format: %s", path)
	}
	return parts[3], parts[5], nil
}

// buildExecURL constructs the full exec URL with query parameters.
func buildExecURL(host, namespace, podName string, opts *corev1.PodExecOptions) *url.URL {
	u, _ := url.Parse(fmt.Sprintf("%s/api/v1/namespaces/%s/pods/%s/exec", host, namespace, podName))
	q := u.Query()
	if opts.Stdout {
		q.Set("stdout", "true")
	}
	if opts.Stderr {
		q.Set("stderr", "true")
	}
	if opts.Stdin {
		q.Set("stdin", "true")
	}
	if opts.TTY {
		q.Set("tty", "true")
	}
	if opts.Container != "" {
		q.Set("container", opts.Container)
	}
	for _, cmd := range opts.Command {
		q.Add("command", cmd)
	}
	u.RawQuery = q.Encode()
	return u
}
