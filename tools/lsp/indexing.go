// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package lsp

import (
	"fmt"
	"path/filepath"
	"runtime/debug"
	"time"

	protocol "github.com/tliron/glsp/protocol_3_16"
)

// indexingProgressToken identifies the workspace scan in $/progress reports.
const indexingProgressToken = "openchoreo-indexing"

// indexProgressTimeout bounds how long we wait for the client's
// window/workDoneProgress/create acknowledgement before giving up on progress.
const indexProgressTimeout = 5 * time.Second

// beginIndexing runs the workspace scan once per session in a background
// goroutine so initialize responds promptly even for large workspaces.
func (s *Server) beginIndexing(roots []string) {
	s.indexOnce.Do(func() {
		go s.runIndexing(roots)
	})
}

// Indexed returns a channel that is closed once the initial workspace scan
// finishes, success or failure. Callers can block on it to observe the scan
// (used by tests and tooling that needs a warm index).
func (s *Server) Indexed() <-chan struct{} {
	return s.indexDone
}

// runIndexing scans the workspace and reports progress when the client
// declared WorkDoneProgress support. Failure is surfaced with a showMessage
// and an error log instead of failing initialize.
func (s *Server) runIndexing(roots []string) {
	defer close(s.indexDone)
	defer func() {
		if recovered := recover(); recovered != nil {
			s.reportIndexingPanic(roots, recovered)
		}
	}()

	// Use $/progress only if the client actually acknowledged the
	// window/workDoneProgress/create request: a client that advertises
	// WorkDoneProgress but replies -32601 (e.g. it registered no
	// server-to-client request handlers) must not receive progress frames.
	useProgress := s.indexProgress
	if useProgress {
		if err := s.requestProgressStartup(); err != nil {
			if s.logger != nil {
				s.logger.Debug("client rejected workDoneProgress/create; indexing without progress", "error", err.Error())
			}
			useProgress = false
		}
	}

	if useProgress {
		s.beginIndexProgress()
	}

	onProgress := func(scanned, total int) {
		if useProgress {
			s.reportIndexProgress(scanned, total)
		}
	}

	var finalMessage string
	err := s.indexer.IndexWorkspacesWithProgress(roots, onProgress)
	if err != nil {
		finalMessage = "workspace indexing failed"
		s.logAt(protocol.MessageTypeError, finalMessage, []any{
			"roots", roots, "error", err.Error(),
		})
		s.showAsync(protocol.MessageTypeError, "Failed to index workspace: "+err.Error())
		s.logAsync(protocol.MessageTypeError, "IndexWorkspaces failed: "+err.Error())
	} else {
		files := s.indexer.FileCount()
		resources := s.indexer.ResourceCount()
		finalMessage = fmt.Sprintf("Indexed %d file(s), %d resource(s).", files, resources)
		s.logAt(protocol.MessageTypeInfo, "workspace indexed", []any{
			"roots", roots, "files", files, "resources", resources,
		})
		s.logAsync(protocol.MessageTypeInfo, finalMessage)

		// Keep the index fresh for on-disk changes after the scan completes.
		s.startWatch(roots)
		go func() {
			for _, root := range roots {
				s.triggerPreRenderSchemaValidationForPath(filepath.Join(root, ".workspace-diagnostics.yaml"))
			}
		}()
	}

	if useProgress {
		s.endIndexProgress(finalMessage)
	}
}

// requestProgressStartup asks the client to register the indexing progress
// token via window/workDoneProgress/create and blocks on the reply, so
// $/progress is only used once the client acknowledges the request. Clients
// that never answer inside indexProgressTimeout fall back to no progress.
func (s *Server) requestProgressStartup() error {
	return s.CallRequest(indexProgressTimeout, protocol.ServerWindowWorkDoneProgressCreate,
		&protocol.WorkDoneProgressCreateParams{Token: s.indexingProgressToken()})
}

// beginIndexProgress sends the $/progress begin notification. The create
// request has already been acknowledged by the time this runs.
func (s *Server) beginIndexProgress() {
	s.SendAsync(protocol.MethodProgress, &protocol.ProgressParams{
		Token: s.indexingProgressToken(),
		Value: protocol.WorkDoneProgressBegin{
			Kind:  "begin",
			Title: "Indexing OpenChoreo workspace",
		},
	})
}

func (s *Server) reportIndexProgress(scanned, total int) {
	if total <= 0 {
		return
	}
	message := fmt.Sprintf("%d/%d files", scanned, total)
	percentage := protocol.UInteger(scanned * 100 / total)
	s.SendAsync(protocol.MethodProgress, &protocol.ProgressParams{
		Token: s.indexingProgressToken(),
		Value: protocol.WorkDoneProgressReport{
			Kind:       "report",
			Message:    &message,
			Percentage: &percentage,
		},
	})
}

func (s *Server) endIndexProgress(finalMessage string) {
	s.SendAsync(protocol.MethodProgress, &protocol.ProgressParams{
		Token: s.indexingProgressToken(),
		Value: protocol.WorkDoneProgressEnd{
			Kind:    "end",
			Message: &finalMessage,
		},
	})
}

func (s *Server) indexingProgressToken() protocol.ProgressToken {
	return protocol.ProgressToken{Value: indexingProgressToken}
}

// reportIndexingPanic logs an indexing crash and surfaces it to the client
// without failing the request that started the scan.
func (s *Server) reportIndexingPanic(roots []string, recovered any) {
	if s.logger != nil {
		s.logger.Error("panic during workspace indexing",
			"panic", recovered, "roots", roots, "stack", string(debug.Stack()))
	}
	s.showAsync(protocol.MessageTypeError,
		"Workspace indexing crashed. Check the server log for details.")
}
