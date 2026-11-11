// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package lsp

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/openchoreo/openchoreo/tools/lint/targetschema"
	protocol "github.com/tliron/glsp/protocol_3_16"
)

// watchDebounceDelay coalesces filesystem events: editors stage writes in
// several steps and a single operation can emit multiple events, so a short
// per-path quiet window collapses the burst into one re-index.
const watchDebounceDelay = 100 * time.Millisecond

// startWatch watches the workspace directory tree so the index stays fresh when
// files change on disk outside the configured documents (edits in other
// editors, git operations, scaffolding tools). It is called after the initial
// workspace scan so the scan and the watcher never fight over the same files.
func (s *Server) startWatch(roots []string) {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		s.logAt(protocol.MessageTypeWarning, "failed to start filesystem watcher",
			[]any{"roots", roots, "error", err.Error()})
		return
	}

	for _, root := range roots {
		if err := watchDirectories(w, root); err != nil {
			s.logAt(protocol.MessageTypeWarning, "failed to register some watch paths",
				[]any{"root", root, "error", err.Error()})
		}
	}

	s.watchMu.Lock()
	if s.watchClosed {
		// StopWatcher raced the watcher creation; discard it.
		s.watchMu.Unlock()
		w.Close()
		return
	}
	s.watcher = w
	for path := range s.schemaDependencyPaths {
		_ = w.Add(filepath.Dir(path))
	}
	s.watchRoots = roots
	s.watchMu.Unlock()

	go s.watchLoop(w)
}

// watchDirectories recursively registers the workspace tree. fsnotify only
// reports events for the exact paths added, so subdirectories must be added
// explicitly; dot-directories (.git, .vscode) are skipped entirely.
func watchDirectories(w *fsnotify.Watcher, root string) error {
	return filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // skip unreachable subtrees
		}
		if !d.IsDir() {
			return nil
		}
		if path != root && isDotPath(path) {
			return filepath.SkipDir
		}
		return w.Add(path)
	})
}

// watchLoop forwards watcher events to the handler and drains error/close
// channels. It returns when the watcher is closed by StopWatcher.
func (s *Server) watchLoop(w *fsnotify.Watcher) {
	for {
		select {
		case err, ok := <-w.Errors:
			if !ok {
				return
			}
			s.logAt(protocol.MessageTypeWarning, "filesystem watcher error",
				[]any{"error", err.Error()})
		case event, ok := <-w.Events:
			if !ok {
				return
			}
			s.handleWatchEvent(w, event)
		}
	}
}

// handleWatchEvent registers newly created directories (so files inside them
// are observed) and schedules a coalesced re-index for changed YAML files.
func (s *Server) handleWatchEvent(w *fsnotify.Watcher, event fsnotify.Event) {
	path := event.Name

	// fsnotify's kqueue backend sees a same-directory rename (and some other
	// bursty changes) as an event with no target name: the kernel reports the
	// directory changed but the diff cannot name the paths involved, so it
	// emits an empty Event. Re-syncing the whole tree with disk is the only
	// reliable response.
	if path == "" {
		s.resyncIndexWithDisk()
		return
	}

	if event.Op&fsnotify.Create != 0 {
		if info, err := os.Stat(path); err == nil && info.IsDir() && !isDotPath(path) {
			if err := watchDirectories(w, path); err != nil {
				s.logAt(protocol.MessageTypeWarning, "failed to register created directory",
					[]any{"path", path, "error", err.Error()})
			}
			// A directory can already contain files by the time the watch is
			// registered (e.g. a working tree copied into place), and kqueue
			// never reports events that happened before the watch existed.
			// Catch those files up so nothing is silently missed.
			s.catchupWatchSubtree(path)
			return
		}
	}

	if isDotPath(path) {
		return
	}
	if !isYAMLWatchName(path) && !s.isSchemaDependency(path) {
		return
	}

	s.debounceWatch(path)
}

// debounceWatch coalesces events for a path: while an event arrives inside the
// debounce window, the previous timer is reset so only the final state of the
// burst is processed.
func (s *Server) debounceWatch(path string) {
	s.watchMu.Lock()
	defer s.watchMu.Unlock()

	if s.watchPending == nil {
		s.watchPending = make(map[string]*time.Timer)
	}
	if timer := s.watchPending[path]; timer != nil {
		timer.Stop()
	}
	s.watchPending[path] = time.AfterFunc(watchDebounceDelay, func() {
		s.watchMu.Lock()
		delete(s.watchPending, path)
		s.watchMu.Unlock()
		s.processWatchChange(path)
	})
}

// processWatchChange re-indexes a changed file and republishes diagnostics for
// every open document, which may reference the changed resource by name.
// Documents that are themselves open are skipped: the editor's in-memory
// buffer is the authoritative version, and it already revalidates via didChange.
func (s *Server) processWatchChange(path string) {
	changed := s.updateIndexedFile(path)
	if !changed {
		return
	}
	s.invalidatePostRenderForEdit(fileToURI(path))
	if s.isSchemaDependency(path) {
		s.postRenderMu.Lock()
		scope := s.postRenderScope
		s.postRenderMu.Unlock()
		if scope != "" {
			s.invalidatePostRenderForEdit(pathToDocumentURI(filepath.Join(scope, targetschema.ConfigName)))
		}
	}
	if changed {
		go s.triggerPreRenderSchemaValidationForPath(path)
		s.revalidateOpenDocumentsAfterWatch()
	}
}

// catchupWatchSubtree indexes any YAML already present under a freshly
// registered directory. Unlike processWatchChange it updates the whole tree in
// one pass and republishes once, since a burst of files can arrive before the
// directory's watch is active.
func (s *Server) catchupWatchSubtree(dir string) {
	changed := false

	_ = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || isDotPath(path) || (!isYAMLWatchName(path) && !s.isSchemaDependency(path)) {
			return nil
		}
		if s.updateIndexedFile(path) {
			changed = true
		}
		return nil
	})

	if changed {
		s.revalidateOpenDocumentsAfterWatch()
	}
}

// resyncIndexWithDisk reconciles the index with the on-disk YAML set for the
// watched roots. It is the response to kqueue flush events (empty Event),
// which carry no target path. Walking every file handles creates and updates;
// os.Stat on every indexed file handles renames and deletions whose names are
// no longer discoverable from the walk.
func (s *Server) resyncIndexWithDisk() {
	changed := false

	s.watchMu.Lock()
	roots := s.watchRoots
	s.watchMu.Unlock()
	if len(roots) == 0 {
		return
	}

	for _, root := range roots {
		_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if isDotPath(path) {
				if d.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			if !d.IsDir() && (isYAMLWatchName(path) || s.isSchemaDependency(path)) && s.updateIndexedFile(path) {
				s.invalidatePostRenderForEdit(fileToURI(path))
				changed = true
			}
			return nil
		})
	}

	for _, uri := range s.indexer.URIs() {
		if !strings.HasPrefix(uri, "file://") {
			continue
		}
		if _, err := os.Stat(fileURIToPath(uri)); err != nil {
			s.indexer.RemoveFile(uri)
			s.invalidatePostRenderForEdit(uri)
			changed = true
		}
	}

	if changed {
		s.revalidateOpenDocumentsAfterWatch()
	}
}

// file. Reports whether the index actually changed.
func (s *Server) updateIndexedFile(path string) bool {
	uri := fileToURI(path)

	// A file that no longer exists on disk must leave the index even when its
	// document is open in a client. kqueue reports a rename as a RENAME event
	// on the old name; skipping removals for open documents would keep the
	// stale name referenceable forever.
	if _, err := os.Stat(path); err != nil {
		if _, known := s.indexer.SourceContent(uri); !known {
			return false
		}
		s.indexer.RemoveFile(uri)
		return true
	}

	// The editor's in-memory buffer is authoritative for open documents, so
	// on-disk writes are not pushed back into the index.
	if s.GetDocument(uri) != nil {
		return false
	}

	content, err := os.ReadFile(path)
	if err != nil {
		s.logAt(protocol.MessageTypeWarning, "failed to read changed file",
			[]any{"path", path, "error", err.Error()})
		return false
	}
	if previous, ok := s.indexer.SourceContent(uri); ok && bytes.Equal(previous, content) {
		return false
	}
	if err := s.indexer.UpdateFile(uri, content); err != nil {
		s.logAt(protocol.MessageTypeWarning, "indexer update failed for changed file",
			[]any{"path", path, "error", err.Error()})
		return false
	}
	return true
}

// revalidateOpenDocumentsAfterWatch republishes diagnostics for every open
// document after the index changed on disk, since any of them may reference
// the affected resources by name.
func (s *Server) revalidateOpenDocumentsAfterWatch() {
	s.revalidateOpenDocumentsExcept("")
}

// StopWatcher stops the filesystem watcher and cancels pending debounced watch
// actions. Safe to call multiple times; invoked on shutdown from Start and by
// tests.
func (s *Server) StopWatcher() {
	s.watchMu.Lock()
	defer s.watchMu.Unlock()

	s.watchClosed = true
	for path, timer := range s.watchPending {
		timer.Stop()
		delete(s.watchPending, path)
	}
	if s.watcher != nil {
		s.watcher.Close()
		s.watcher = nil
	}
}

// isDotPath reports whether a path's base name starts with "." (hidden entries
// like .git are never indexed or watched).
func isDotPath(path string) bool {
	if filepath.Base(path) == targetschema.ConfigName {
		return false
	}
	return strings.HasPrefix(filepath.Base(path), ".")
}

// isYAMLWatchName reports whether a path names a YAML file.
func isYAMLWatchName(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return ext == ".yaml" || ext == ".yml"
}

// fileToURI builds the file:// URI the indexer uses as a document key, which
// must match the URIs handed to didOpen by clients.
func fileToURI(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		return path
	}
	if !strings.HasPrefix(abs, "file://") {
		return "file://" + abs
	}
	return abs
}
