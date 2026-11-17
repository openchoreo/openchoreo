// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package notifications

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptrace"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"unicode"

	corev1 "k8s.io/api/core/v1"
)

// Shared HTTP client for webhook notifications. Each attempt is bounded by the caller's context,
// so the client sets no timeout of its own.
var httpClient = &http.Client{}

// maxErrorBodyBytes bounds how much of an error response is read into the returned error.
const maxErrorBodyBytes = 64 << 10

const (
	// webhookURLKey holds the webhook URL: in the ConfigMap when it is inline, in the Secret when it
	// comes from a secret reference.
	webhookURLKey = "webhook.url"

	// redactedPlaceholder stands in for the parts of a webhook URL that can carry a credential.
	redactedPlaceholder = "<redacted>"

	// minScrubbedPieceLen is the shortest piece of a webhook URL's path or query that is removed
	// from text on its own. Shorter pieces are too common in ordinary text to be worth removing.
	minScrubbedPieceLen = 4
)

// WebhookConfig holds webhook configuration for sending alerts
type WebhookConfig struct {
	// URL can carry a credential in its path or query. Log it only through RedactWebhookURL.
	URL string
	// URLIsSecret is set when the URL comes from a secret reference. What the receiver sends
	// back is then never reported, since it could repeat the URL.
	URLIsSecret     bool
	Headers         map[string]string
	PayloadTemplate string // Optional JSON template with CEL expressions
}

// SendWebhookWithConfig sends an alert webhook using the provided configuration.
// It sends the alertDetails JSON object as the request body.
func SendWebhookWithConfig(ctx context.Context, config *WebhookConfig, alertDetails map[string]interface{}) error {
	if config.URL == "" {
		return newDeliveryError(ReasonChannelConfig, "webhook URL is missing", fmt.Errorf("webhook URL is required"))
	}
	if err := validateWebhookURL(config.URL); err != nil {
		return newDeliveryError(ReasonChannelConfig, "invalid webhook URL", err)
	}

	// Marshal alertDetails to JSON
	jsonBody, err := json.Marshal(alertDetails)
	if err != nil {
		return fmt.Errorf("failed to marshal alert details to JSON: %w", err)
	}

	// Note whether a connection to the receiver was made, to tell failures that came before the
	// receiver could send anything (its certificate included) from those that came after.
	var contacted, handshakeFailed atomic.Bool
	ctx = httptrace.WithClientTrace(ctx, &httptrace.ClientTrace{
		ConnectDone: func(_, _ string, err error) {
			if err == nil {
				contacted.Store(true)
			}
		},
		GotConn: func(httptrace.GotConnInfo) { contacted.Store(true) },
		TLSHandshakeDone: func(_ tls.ConnectionState, err error) {
			if err != nil {
				handshakeFailed.Store(true)
			}
		},
	})

	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, config.URL, bytes.NewBuffer(jsonBody))
	if err != nil {
		// The cause quotes the URL, which validateWebhookURL has already accepted, so leave it out.
		return newDeliveryError(ReasonChannelConfig, "invalid webhook URL",
			fmt.Errorf("failed to create HTTP request for %s", RedactWebhookURL(config.URL)))
	}

	// Set Content-Type header
	req.Header.Set("Content-Type", "application/json")

	// Add custom headers if provided
	for key, value := range config.Headers {
		req.Header.Set(key, value)
	}

	// Send the request using the shared HTTP client
	resp, err := httpClient.Do(req)
	if err != nil {
		progress := requestProgress{contacted: contacted.Load(), handshakeFailed: handshakeFailed.Load()}
		// Classify the transport error itself: safeCause replaces it with a fixed error once the
		// receiver was contacted.
		return newDeliveryError(ReasonOf(err), "", fmt.Errorf("failed to send webhook request to %s: %w",
			RedactWebhookURL(config.URL), safeCause(err, progress)))
	}
	defer resp.Body.Close()

	// Check response status
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		err := fmt.Errorf("webhook request failed with status code: %d", resp.StatusCode)
		// Read a bounded part of the response body for a better error message. A longer response is
		// left out, since cutting it could cut the webhook URL so that scrubbing no longer finds it.
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, maxErrorBodyBytes+1))
		switch {
		case readErr != nil || len(body) == 0:
		case len(body) > maxErrorBodyBytes:
			err = fmt.Errorf("webhook request failed with status code: %d, response omitted because it is too long", resp.StatusCode)
		default:
			err = fmt.Errorf("webhook request failed with status code: %d, %s",
				resp.StatusCode, describeResponse(string(body), config))
		}
		return newDeliveryError(httpStatusReason(resp.StatusCode), fmt.Sprintf("HTTP %d", resp.StatusCode), err)
	}

	return nil
}

// RedactWebhookURL returns the URL with everything but its scheme and host replaced, so that it can
// be logged. A value that is not an http(s) URL is replaced entirely, since it may be a secret
// that is not a URL at all.
func RedactWebhookURL(raw string) string {
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil || !isHTTPURL(u) {
		return redactedPlaceholder
	}
	return u.Scheme + "://" + u.Host + "/" + redactedPlaceholder
}

// validateWebhookURL checks that the URL is one a webhook can be sent to. Its errors leave the
// value out, including the parse error, which quotes parts of it.
func validateWebhookURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return errors.New("webhook URL is not a valid URL")
	}
	if !isHTTPURL(u) {
		return errors.New("webhook URL must be an http or https URL with a host")
	}
	return nil
}

func isHTTPURL(u *url.URL) bool {
	return (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}

// What is reported for a request that failed after the receiver was contacted. From then on an
// error's own text can repeat the webhook URL: the receiver chooses the names in its certificate
// and where it redirects to, and the errors for both quote them.
var (
	errReceiverTimeout             = errors.New("timed out waiting for the receiver")
	errRequestCanceled             = errors.New("the request was canceled")
	errNetworkAfterContact         = errors.New("a network error occurred after the receiver was contacted")
	errTLSHandshake                = errors.New("the TLS handshake with the receiver failed")
	errCertificateWrongHost        = errors.New("the receiver's TLS certificate is not valid for its host")
	errCertificateUnknownAuthority = errors.New("the receiver's TLS certificate is signed by an unknown authority")
	errCertificateExpired          = errors.New("the receiver's TLS certificate has expired or is not yet valid")
	errCertificateInvalid          = errors.New("the receiver's TLS certificate could not be verified")
	errCauseOmitted                = errors.New("the receiver's reply could not be processed; details are omitted because they could repeat the webhook URL")
)

// requestProgress is how far a failed request got, as its httptrace hooks saw it.
type requestProgress struct {
	contacted       bool // a connection to the receiver (or to a redirect target) was made
	handshakeFailed bool // a TLS handshake failed
}

// safeCause returns what to report as the cause of a failed request. A failure before the
// receiver was contacted (name resolution, connecting) involved only the host, so its cause is
// returned as it is. A later failure is reduced to one of the fixed errors above.
func safeCause(err error, progress requestProgress) error {
	if !progress.contacted {
		// net/http puts the full URL in its error; keep only what it wraps.
		for {
			var urlErr *url.Error
			if !errors.As(err, &urlErr) {
				return err
			}
			err = urlErr.Err
		}
	}

	var netErr net.Error
	var opErr *net.OpError
	var certErr *tls.CertificateVerificationError
	switch {
	case errors.Is(err, context.Canceled):
		return errRequestCanceled
	case errors.Is(err, context.DeadlineExceeded), errors.As(err, &netErr) && netErr.Timeout():
		return errReceiverTimeout
	case errors.As(err, &certErr):
		return certificateProblem(certErr)
	case progress.handshakeFailed:
		return errTLSHandshake
	case errors.As(err, &opErr), errors.Is(err, io.EOF), errors.Is(err, io.ErrUnexpectedEOF):
		return errNetworkAfterContact
	default:
		return errCauseOmitted
	}
}

// certificateProblem names what is wrong with the receiver's certificate without quoting it.
func certificateProblem(err *tls.CertificateVerificationError) error {
	var hostErr x509.HostnameError
	var authorityErr x509.UnknownAuthorityError
	var invalidErr x509.CertificateInvalidError
	switch {
	case errors.As(err, &hostErr):
		return errCertificateWrongHost
	case errors.As(err, &authorityErr):
		return errCertificateUnknownAuthority
	case errors.As(err, &invalidErr) && invalidErr.Reason == x509.Expired:
		return errCertificateExpired
	default:
		return errCertificateInvalid
	}
}

// scrubWebhookURL removes the webhook URL from text that may quote it, such as a receiver's
// error page. Besides the URL, its path and its query, the pieces of the path and query are
// removed one by one, so that they are caught however the text joins or escapes them.
func scrubWebhookURL(text, rawURL string) string {
	for _, part := range webhookURLParts(rawURL) {
		text = strings.ReplaceAll(text, part, redactedPlaceholder)
	}
	return text
}

// describeResponse returns the receiver's response for an error message. For a URL that comes
// from a secret the response is always left out. Otherwise it is given with the webhook URL
// scrubbed, or left out when it still quotes a piece of the URL in an escaped form.
func describeResponse(body string, config *WebhookConfig) string {
	if config.URLIsSecret {
		return "response omitted because the webhook URL is a secret"
	}
	scrubbed := scrubWebhookURL(body, config.URL)
	if quotesWebhookURL(scrubbed, config.URL) {
		return "response omitted because it quotes the webhook URL"
	}
	return "response: " + scrubbed
}

// quotesWebhookURL reports whether text contains a piece of the webhook URL's path, query or
// userinfo once escaping is set aside. Both are decoded and reduced to their letters and digits,
// so that "ab\u0026cdef" in the text matches "ab%26cdef" in the URL.
func quotesWebhookURL(text, rawURL string) bool {
	u, err := url.Parse(rawURL)
	if err != nil {
		// Nothing is known about the URL's pieces, so assume the worst.
		return true
	}
	skeleton := alphanumerics(decodeEscapes(text))
	for _, piece := range webhookURLPieces(u) {
		if s := alphanumerics(decodeEscapes(piece)); len(s) >= minScrubbedPieceLen && strings.Contains(skeleton, s) {
			return true
		}
	}
	return false
}

var (
	unicodeEscape = regexp.MustCompile(`\\u([0-9a-fA-F]{4})`)
	simpleEscape  = regexp.MustCompile(`\\["\\/bfnrt]`)
	percentEscape = regexp.MustCompile(`%([0-9a-fA-F]{2})`)
)

// decodeEscapes undoes JSON escapes, percent-encoding and HTML entities, a few times over to
// cover text that was escaped more than once.
func decodeEscapes(s string) string {
	for range 3 {
		decoded := unicodeEscape.ReplaceAllStringFunc(s, func(m string) string {
			n, _ := strconv.ParseInt(m[2:], 16, 32)
			return string(rune(n))
		})
		// What a simple escape such as \n stands for is never a letter or a digit, so a space will do.
		decoded = simpleEscape.ReplaceAllString(decoded, " ")
		decoded = percentEscape.ReplaceAllStringFunc(decoded, func(m string) string {
			n, _ := strconv.ParseUint(m[1:], 16, 8)
			return string([]byte{byte(n)})
		})
		decoded = html.UnescapeString(decoded)
		if decoded == s {
			break
		}
		s = decoded
	}
	return s
}

// alphanumerics returns the letters and digits of s.
func alphanumerics(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return r
		}
		return -1
	}, s)
}

// webhookURLPieces returns the path segments, the query keys and values, and the userinfo of
// the URL, as they are written in it.
func webhookURLPieces(u *url.URL) []string {
	pieces := strings.Split(u.EscapedPath(), "/")
	pieces = append(pieces, strings.FieldsFunc(u.RawQuery, func(r rune) bool { return r == '&' || r == ';' || r == '=' })...)
	if u.User != nil {
		pieces = append(pieces, u.User.Username())
		if password, ok := u.User.Password(); ok {
			pieces = append(pieces, password)
		}
	}
	return pieces
}

// webhookURLParts returns what scrubWebhookURL removes for the URL, longest first.
func webhookURLParts(rawURL string) []string {
	seen := map[string]struct{}{}
	var parts []string
	add := func(part string, minLen int) {
		if _, ok := seen[part]; ok || len(part) < minLen {
			return
		}
		seen[part] = struct{}{}
		parts = append(parts, part)
	}

	// "/" and "" are not worth hiding, and replacing them would mangle the text.
	add(rawURL, 2)
	if u, err := url.Parse(rawURL); err == nil {
		for _, whole := range []string{u.RequestURI(), u.EscapedPath(), u.Path, u.RawQuery} {
			add(whole, 2)
		}

		for _, piece := range webhookURLPieces(u) {
			add(piece, minScrubbedPieceLen)
			if unescaped, err := url.PathUnescape(piece); err == nil {
				add(unescaped, minScrubbedPieceLen)
			}
			if unescaped, err := url.QueryUnescape(piece); err == nil {
				add(unescaped, minScrubbedPieceLen)
			}
		}
	}

	sort.SliceStable(parts, func(i, j int) bool { return len(parts[i]) > len(parts[j]) })
	return parts
}

// httpStatusReason classifies a non-2xx webhook response status.
func httpStatusReason(status int) Reason {
	switch {
	case status == http.StatusTooManyRequests:
		return ReasonHTTP429
	case status >= 500:
		return ReasonHTTP5xx
	default:
		return ReasonHTTP4xx
	}
}

// PrepareWebhookNotificationConfig prepares webhook notification configuration from ConfigMap and Secret
func PrepareWebhookNotificationConfig(configMap *corev1.ConfigMap, secret *corev1.Secret, logger *slog.Logger) (WebhookConfig, error) {
	// The URL is in the ConfigMap when it is inline, and in the Secret when it comes from a secret
	// reference. Secret values often end in a newline.
	webhookURL := configMap.Data[webhookURLKey]
	urlIsSecret := false
	if webhookURL == "" && secret != nil {
		webhookURL = strings.TrimSpace(string(secret.Data[webhookURLKey]))
		urlIsSecret = webhookURL != ""
	}
	if webhookURL == "" {
		return WebhookConfig{}, fmt.Errorf("webhook URL not found in ConfigMap or Secret")
	}

	// Parse headers from ConfigMap and Secret
	headers := make(map[string]string)
	if headerKeysStr, ok := configMap.Data["webhook.headers"]; ok && headerKeysStr != "" {
		headerKeys := strings.Split(headerKeysStr, ",")
		for _, key := range headerKeys {
			key = strings.TrimSpace(key)
			if key == "" {
				continue
			}
			// Try to get inline value from ConfigMap first
			if inlineValue, ok := configMap.Data[fmt.Sprintf("webhook.header.%s", key)]; ok {
				headers[key] = inlineValue
			} else if secret != nil && secret.Data != nil {
				// Try to get value from Secret (for secret-referenced headers)
				if secretValue, ok := secret.Data[fmt.Sprintf("webhook.header.%s", key)]; ok {
					headers[key] = string(secretValue)
				}
			}
		}
	}

	// Parse payload template if provided
	payloadTemplate := configMap.Data["webhook.payloadTemplate"]

	webhookConfig := WebhookConfig{
		URL:             webhookURL,
		URLIsSecret:     urlIsSecret,
		Headers:         headers,
		PayloadTemplate: payloadTemplate,
	}

	logger.Debug("Final webhook config",
		"url", RedactWebhookURL(webhookConfig.URL),
		"headerCount", len(webhookConfig.Headers),
		"hasPayloadTemplate", payloadTemplate != "")

	return webhookConfig, nil
}
