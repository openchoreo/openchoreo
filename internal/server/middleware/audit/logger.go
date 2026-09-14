// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package audit

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
)

// Logger handles emitting audit log events using structured logging. It is a
// pure reader of Event — EventID/Producer are stamped by buildEvent and
// EventTime by the surface adapter, never here, so a second sink can't see a
// different identity for the same event (see Emitter's doc comment).
type Logger struct {
	slogger *slog.Logger
}

// NewLogger creates an audit logger writing newline-delimited JSON to w.
//
// The handler is this package's own rather than the application logger's: a
// collector parses these records, so the format is a published contract, not a
// logging preference. audit.enabled is therefore the only kill switch — no
// application log level applies.
//
// Pass the same *os.File the application logger uses (os.Stdout). slog writes
// each record in one Write and os.File serializes writes per descriptor, so
// two handlers on one *os.File cannot interleave; wrapping either side in a
// buffered or separately-opened writer loses that.
func NewLogger(w io.Writer) *Logger {
	return &Logger{slogger: slog.New(slog.NewJSONHandler(w, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))}
}

// LogEvent emits an audit log event using slog. The attrs are derived from
// Event.MarshalJSON's output rather than built by hand, so the log stream and
// a sink that marshals the same *Event cannot publish different shapes.
//
// The "AUDIT-LOG" message is the collector's routing key, separating these
// records from the process's application logs, so it is stable across schema
// minor versions.
func (l *Logger) LogEvent(event *Event) {
	payload, err := json.Marshal(event)
	if err != nil {
		l.logRenderFailure(event, err)
		return
	}

	dec := json.NewDecoder(bytes.NewReader(payload))
	// Numbers stay json.Number so a value round-trips to the same bytes it
	// marshaled from, rather than through float64.
	dec.UseNumber()

	tok, err := dec.Token()
	if err != nil {
		l.logRenderFailure(event, err)
		return
	}
	if delim, ok := tok.(json.Delim); !ok || delim != '{' {
		l.logRenderFailure(event, fmt.Errorf("expected a JSON object, got %v", tok))
		return
	}

	attrs, err := decodeObjectAttrs(dec)
	if err != nil {
		l.logRenderFailure(event, err)
		return
	}

	l.slogger.Info("AUDIT-LOG", attrs...)
}

// logRenderFailure publishes an audit event's identity when its body cannot be
// rendered, rather than dropping the record silently. Reachable only through
// the map[string]any metadata fields, which could hold a non-marshalable value.
func (l *Logger) logRenderFailure(event *Event, err error) {
	l.slogger.Error("AUDIT-LOG-RENDER-FAILED",
		slog.String("schema_version", SchemaVersion),
		slog.String("event_id", event.EventID),
		slog.String("action", event.Action),
		slog.String("result", string(event.Result)),
		slog.String("error", err.Error()),
	)
}

// decodeObjectAttrs reads key/value pairs up to the object's closing brace as
// slog attrs, nested objects becoming slog.Group. A decoder rather than a
// map[string]any because map iteration would lose field order.
func decodeObjectAttrs(dec *json.Decoder) ([]any, error) {
	var attrs []any
	for {
		tok, err := dec.Token()
		if err != nil {
			return nil, err
		}
		if delim, ok := tok.(json.Delim); ok && delim == '}' {
			return attrs, nil
		}
		key, ok := tok.(string)
		if !ok {
			return nil, fmt.Errorf("expected an object key, got %v", tok)
		}

		valTok, err := dec.Token()
		if err != nil {
			return nil, err
		}
		if delim, ok := valTok.(json.Delim); ok {
			switch delim {
			case '{':
				group, err := decodeObjectAttrs(dec)
				if err != nil {
					return nil, err
				}
				if len(group) == 0 {
					// slog.JSONHandler drops an empty group, which would lose
					// the field the marshaled form still carries.
					attrs = append(attrs, slog.Any(key, map[string]any{}))
					continue
				}
				attrs = append(attrs, slog.Group(key, group...))
			case '[':
				arr, err := decodeArray(dec)
				if err != nil {
					return nil, err
				}
				attrs = append(attrs, slog.Any(key, arr))
			default:
				return nil, fmt.Errorf("unexpected delimiter %v for key %q", delim, key)
			}
			continue
		}
		attrs = append(attrs, slog.Any(key, valTok))
	}
}

// decodeArray reads array elements up to the closing bracket as plain Go
// values; an array has no keys to group by.
func decodeArray(dec *json.Decoder) ([]any, error) {
	items := []any{}
	for {
		tok, err := dec.Token()
		if err != nil {
			return nil, err
		}
		if delim, ok := tok.(json.Delim); ok {
			switch delim {
			case ']':
				return items, nil
			case '{':
				obj, err := decodeObjectMap(dec)
				if err != nil {
					return nil, err
				}
				items = append(items, obj)
				continue
			case '[':
				nested, err := decodeArray(dec)
				if err != nil {
					return nil, err
				}
				items = append(items, nested)
				continue
			}
		}
		items = append(items, tok)
	}
}

// decodeObjectMap reads an object nested inside an array into a map.
func decodeObjectMap(dec *json.Decoder) (map[string]any, error) {
	out := map[string]any{}
	for {
		tok, err := dec.Token()
		if err != nil {
			return nil, err
		}
		if delim, ok := tok.(json.Delim); ok && delim == '}' {
			return out, nil
		}
		key, ok := tok.(string)
		if !ok {
			return nil, fmt.Errorf("expected an object key, got %v", tok)
		}

		valTok, err := dec.Token()
		if err != nil {
			return nil, err
		}
		if delim, ok := valTok.(json.Delim); ok {
			switch delim {
			case '{':
				nested, err := decodeObjectMap(dec)
				if err != nil {
					return nil, err
				}
				out[key] = nested
			case '[':
				arr, err := decodeArray(dec)
				if err != nil {
					return nil, err
				}
				out[key] = arr
			default:
				return nil, fmt.Errorf("unexpected delimiter %v for key %q", delim, key)
			}
			continue
		}
		out[key] = valTok
	}
}
