// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package notifications

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReasonOf(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want Reason
	}{
		{"delivery error", newDeliveryError(ReasonHTTP5xx, "HTTP 503", errors.New("boom")), ReasonHTTP5xx},
		{"wrapped delivery error", fmt.Errorf("send: %w", newDeliveryError(ReasonTemplate, "", nil)), ReasonTemplate},
		{"cancelled", fmt.Errorf("send: %w", context.Canceled), ReasonInterrupted},
		{"deadline", fmt.Errorf("send: %w", context.DeadlineExceeded), ReasonTimeout},
		{"smtp 451", &textproto.Error{Code: 451, Msg: "try later"}, ReasonSMTPTemporary},
		{"smtp 550", &textproto.Error{Code: 550, Msg: "no such user"}, ReasonSMTPPermanent},
		{"connection refused", &net.OpError{Op: "dial", Net: "tcp", Err: syscall.ECONNREFUSED}, ReasonConnection},
		{"unclassified", errors.New("something else"), ReasonUnknown},
		{"nil", nil, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, ReasonOf(tc.err))
		})
	}
}

func TestReasonRetryable(t *testing.T) {
	for _, r := range []Reason{ReasonTimeout, ReasonConnection, ReasonHTTP5xx, ReasonHTTP429, ReasonSMTPTemporary} {
		assert.True(t, r.Retryable(), r)
	}
	for _, r := range []Reason{ReasonHTTP4xx, ReasonSMTPPermanent, ReasonChannelConfig, ReasonTemplate, ReasonInterrupted, ReasonUnknown} {
		assert.False(t, r.Retryable(), r)
	}
}

func TestSendWebhookWithConfig_ClassifiesStatus(t *testing.T) {
	tests := []struct {
		status int
		want   Reason
	}{
		{http.StatusServiceUnavailable, ReasonHTTP5xx},
		{http.StatusTooManyRequests, ReasonHTTP429},
		{http.StatusBadRequest, ReasonHTTP4xx},
	}
	for _, tc := range tests {
		t.Run(http.StatusText(tc.status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.status)
			}))
			defer server.Close()

			err := SendWebhookWithConfig(context.Background(), &WebhookConfig{URL: server.URL}, map[string]interface{}{})
			require.Error(t, err)
			assert.Equal(t, tc.want, ReasonOf(err))
			assert.Equal(t, fmt.Sprintf("HTTP %d", tc.status), DetailOf(err))
		})
	}
}

func TestSendWebhookWithConfig_BoundsErrorBody(t *testing.T) {
	tests := []struct {
		name     string
		bodySize int
		want     string
	}{
		{name: "response within the bound", bodySize: maxErrorBodyBytes, want: "response: xxxx"},
		{name: "response over the bound", bodySize: maxErrorBodyBytes + 1, want: "response omitted because it is too long"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte(strings.Repeat("x", tt.bodySize)))
			}))
			defer server.Close()

			err := SendWebhookWithConfig(context.Background(), &WebhookConfig{URL: server.URL}, map[string]interface{}{})
			require.Error(t, err)
			assert.Equal(t, ReasonHTTP5xx, ReasonOf(err))
			assert.Contains(t, err.Error(), tt.want)
			assert.Less(t, len(err.Error()), maxErrorBodyBytes+256)
		})
	}
}

func TestSendWebhookWithConfig_InvalidURLIsChannelConfig(t *testing.T) {
	for _, raw := range []string{"ftp://example.com/hook", "http://", "://bad"} {
		err := SendWebhookWithConfig(context.Background(), &WebhookConfig{URL: raw}, map[string]interface{}{})
		require.Error(t, err, raw)
		assert.Equal(t, ReasonChannelConfig, ReasonOf(err), raw)
	}
}

func TestSendWebhookWithConfig_ClassifiesFailureAfterContact(t *testing.T) {
	// The receiver closes the connection without replying. The error text is reduced to a fixed
	// error, but the reason still comes from the transport error.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		conn, _, err := w.(http.Hijacker).Hijack()
		if err == nil {
			_ = conn.Close()
		}
	}))
	defer server.Close()

	err := SendWebhookWithConfig(context.Background(), &WebhookConfig{URL: server.URL + "/s3cr3t"}, map[string]interface{}{})
	require.Error(t, err)
	assert.Equal(t, ReasonConnection, ReasonOf(err))
	assert.NotContains(t, err.Error(), "s3cr3t")
}

func TestSendWebhookWithConfig_TimesOutWithContext(t *testing.T) {
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		<-release
	}))
	defer server.Close()
	defer close(release)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	err := SendWebhookWithConfig(ctx, &WebhookConfig{URL: server.URL + "/s3cr3t"}, map[string]interface{}{})
	require.Error(t, err)
	assert.Equal(t, ReasonTimeout, ReasonOf(err))
	assert.NotContains(t, err.Error(), "s3cr3t")
}

// fakeSMTPServer accepts connections and runs serve on each one.
func fakeSMTPServer(t *testing.T, serve func(conn net.Conn)) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer conn.Close()
				serve(conn)
			}()
		}
	}()
	return ln.Addr().String()
}

// smtpDialogue answers a plain SMTP session, replying rcptReply to RCPT.
func smtpDialogue(rcptReply string, received chan<- string) func(conn net.Conn) {
	return func(conn net.Conn) {
		r := bufio.NewReader(conn)
		write := func(s string) { _, _ = conn.Write([]byte(s + "\r\n")) }
		write("220 fake ESMTP")
		var data strings.Builder
		inData := false
		for {
			line, err := r.ReadString('\n')
			if err != nil {
				return
			}
			if inData {
				if line == ".\r\n" {
					inData = false
					received <- data.String()
					write("250 queued")
					continue
				}
				data.WriteString(line)
				continue
			}
			switch cmd := strings.ToUpper(strings.TrimSpace(line)); {
			case strings.HasPrefix(cmd, "EHLO"), strings.HasPrefix(cmd, "HELO"):
				write("250 fake")
			case strings.HasPrefix(cmd, "MAIL"):
				write("250 ok")
			case strings.HasPrefix(cmd, "RCPT"):
				write(rcptReply)
			case cmd == "DATA":
				inData = true
				write("354 go ahead")
			case cmd == "QUIT":
				write("221 bye")
				return
			default:
				write("250 ok")
			}
		}
	}
}

func TestSendMailContext_Sends(t *testing.T) {
	received := make(chan string, 1)
	addr := fakeSMTPServer(t, smtpDialogue("250 ok", received))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := sendMailContext(ctx, addr, nil, "from@example.com", []string{"to@example.com"}, []byte("Subject: hi\r\n\r\nhello\r\n"))
	require.NoError(t, err)
	assert.Contains(t, <-received, "hello")
}

func TestSendMailContext_TemporaryRejection(t *testing.T) {
	addr := fakeSMTPServer(t, smtpDialogue("451 try again later", make(chan string, 1)))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := sendMailContext(ctx, addr, nil, "from@example.com", []string{"to@example.com"}, []byte("x"))
	require.Error(t, err)
	assert.Equal(t, ReasonSMTPTemporary, ReasonOf(err))
	assert.Equal(t, "SMTP 451", DetailOf(err))
}

func TestSendMailContext_SilentServerTimesOut(t *testing.T) {
	addr := fakeSMTPServer(t, func(conn net.Conn) {
		// Accept the connection and never send a greeting.
		_, _ = bufio.NewReader(conn).ReadString('\n')
	})

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	start := time.Now()
	err := sendMailContext(ctx, addr, nil, "from@example.com", []string{"to@example.com"}, []byte("x"))
	require.Error(t, err)
	assert.Equal(t, ReasonTimeout, ReasonOf(err))
	assert.Less(t, time.Since(start), 2*time.Second)
}

func TestSendMailContext_CancelUnblocksSend(t *testing.T) {
	addr := fakeSMTPServer(t, func(conn net.Conn) {
		_, _ = bufio.NewReader(conn).ReadString('\n')
	})

	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(100*time.Millisecond, cancel)
	err := sendMailContext(ctx, addr, nil, "from@example.com", []string{"to@example.com"}, []byte("x"))
	require.Error(t, err)
	assert.Equal(t, ReasonInterrupted, ReasonOf(err))
}
