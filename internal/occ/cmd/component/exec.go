// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package component

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/gorilla/websocket"
	"golang.org/x/term"

	"github.com/openchoreo/openchoreo/internal/occ/auth"
	"github.com/openchoreo/openchoreo/internal/occ/cmd/config"
)

// Stream type prefixes for the exec WebSocket protocol.
// The first byte of each binary WebSocket message identifies the stream.
const (
	streamStdin  = byte(0)
	streamStdout = byte(1)
	streamStderr = byte(2)
	streamResize = byte(3)
)

type terminalSize struct {
	Width  uint16 `json:"width"`
	Height uint16 `json:"height"`
}

// wsWriter serializes all writes to a WebSocket connection.
// gorilla/websocket panics on concurrent writes.
type wsWriter struct {
	conn *websocket.Conn
	mu   sync.Mutex
}

func (w *wsWriter) write(messageType int, data []byte) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.conn.WriteMessage(messageType, data)
}

// Exec opens an interactive exec session to a component's running pod.
func (cp *Component) Exec(params ExecParams) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Default command to /bin/sh
	if len(params.Command) == 0 {
		params.Command = []string{"/bin/sh"}
	}

	conn, err := dialExecWebSocket(ctx, params)
	if err != nil {
		return err
	}
	defer conn.Close()

	ws := &wsWriter{conn: conn}

	// Put terminal in raw mode for interactive TTY sessions
	fd := int(os.Stdin.Fd())
	if term.IsTerminal(fd) && params.TTY {
		oldState, rawErr := term.MakeRaw(fd)
		if rawErr != nil {
			return fmt.Errorf("failed to set raw terminal mode: %w", rawErr)
		}
		defer func() { _ = term.Restore(fd, oldState) }()

		if w, h, sizeErr := term.GetSize(fd); sizeErr == nil {
			sendResize(ws, safeUint16(w), safeUint16(h))
		}
		go watchResize(ctx, ws, fd)
	}

	return streamExecIO(ctx, ws, conn, params.Stdin)
}

// dialExecWebSocket establishes a WebSocket connection to the exec endpoint.
func dialExecWebSocket(ctx context.Context, params ExecParams) (*websocket.Conn, error) {
	controlPlane, err := config.GetCurrentControlPlane()
	if err != nil {
		return nil, fmt.Errorf("failed to get control plane: %w", err)
	}

	credential, err := config.GetCurrentCredential()
	if err != nil {
		return nil, fmt.Errorf("failed to get credential: %w", err)
	}

	wsURL, err := buildExecWebSocketURL(controlPlane.URL, params)
	if err != nil {
		return nil, fmt.Errorf("failed to build exec URL: %w", err)
	}

	headers := http.Header{}
	if credential != nil && credential.Token != "" {
		currentToken := credential.Token
		if auth.IsTokenExpired(currentToken) {
			newToken, refreshErr := auth.RefreshToken()
			if refreshErr != nil {
				return nil, fmt.Errorf("failed to refresh token: %w", refreshErr)
			}
			currentToken = newToken
		}
		headers.Set("Authorization", "Bearer "+currentToken)
	}

	conn, resp, err := websocket.DefaultDialer.DialContext(ctx, wsURL, headers)
	if err != nil {
		if resp != nil && resp.StatusCode != http.StatusSwitchingProtocols {
			return nil, fmt.Errorf("exec connection failed (HTTP %d): %w", resp.StatusCode, err)
		}
		return nil, fmt.Errorf("failed to connect to exec endpoint: %w", err)
	}
	return conn, nil
}

// streamExecIO pumps stdin/stdout/stderr between the local terminal and
// the remote WebSocket exec session. It waits for the remote side to close
// the connection (stdout pump exits), not for local stdin EOF.
func streamExecIO(ctx context.Context, ws *wsWriter, conn *websocket.Conn, attachStdin bool) error {
	// Channel for the stdout pump — the authoritative signal that exec is done.
	stdoutDone := make(chan error, 1)

	if attachStdin {
		go pumpStdin(ws)
	}

	go pumpStdout(conn, stdoutDone)

	select {
	case err := <-stdoutDone:
		if err == nil {
			return nil
		}
		if websocket.IsCloseError(err,
			websocket.CloseNormalClosure,
			websocket.CloseGoingAway,
			websocket.CloseAbnormalClosure) {
			return nil
		}
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

// pumpStdin reads from os.Stdin and sends to the WebSocket.
// When stdin hits EOF it sends the EOF marker but does NOT signal completion;
// the session ends when the remote side closes the connection.
func pumpStdin(ws *wsWriter) {
	buf := make([]byte, 32*1024)
	for {
		n, readErr := os.Stdin.Read(buf)
		if n > 0 {
			msg := make([]byte, 1+n)
			msg[0] = streamStdin
			copy(msg[1:], buf[:n])
			if writeErr := ws.write(websocket.BinaryMessage, msg); writeErr != nil {
				return
			}
		}
		if readErr != nil {
			// stdin closed; signal EOF to remote but keep the session alive
			_ = ws.write(websocket.BinaryMessage, []byte{streamStdin})
			return
		}
	}
}

func pumpStdout(conn *websocket.Conn, errCh chan<- error) {
	for {
		msgType, msg, readErr := conn.ReadMessage()
		if readErr != nil {
			errCh <- readErr
			return
		}
		if msgType == websocket.CloseMessage {
			errCh <- nil
			return
		}
		if len(msg) < 2 {
			continue
		}
		switch msg[0] {
		case streamStdout:
			_, _ = os.Stdout.Write(msg[1:])
		case streamStderr:
			_, _ = os.Stderr.Write(msg[1:])
		}
	}
}

func buildExecWebSocketURL(controlPlaneURL string, params ExecParams) (string, error) {
	u, err := url.Parse(controlPlaneURL)
	if err != nil {
		return "", err
	}
	switch u.Scheme {
	case "https":
		u.Scheme = "wss"
	case "http":
		u.Scheme = "ws"
	}

	u.Path = fmt.Sprintf("/exec/namespaces/%s/components/%s", params.Namespace, params.Component)

	q := u.Query()
	if params.Project != "" {
		q.Set("project", params.Project)
	}
	if params.Environment != "" {
		q.Set("env", params.Environment)
	}
	if params.Container != "" {
		q.Set("container", params.Container)
	}
	if params.TTY {
		q.Set("tty", "true")
	}
	if params.Stdin {
		q.Set("stdin", "true")
	}
	for _, cmd := range params.Command {
		q.Add("command", cmd)
	}
	u.RawQuery = q.Encode()

	return u.String(), nil
}

func sendResize(ws *wsWriter, width, height uint16) {
	data, err := json.Marshal(terminalSize{Width: width, Height: height})
	if err != nil {
		return
	}
	msg := make([]byte, 1+len(data))
	msg[0] = streamResize
	copy(msg[1:], data)
	_ = ws.write(websocket.BinaryMessage, msg)
}

func safeUint16(v int) uint16 {
	if v < 0 {
		return 0
	}
	if v > 0xFFFF {
		return 0xFFFF
	}
	return uint16(v) //nolint:gosec // bounds checked above
}
