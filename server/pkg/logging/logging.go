package logging

import (
	"context"
	"log/slog"
	"os"
	"time"
)

type Logger struct {
	logger *slog.Logger
}

// NewLogger creates a new Logger instance with text formatting
func NewLogger() *Logger {
	opts := &slog.HandlerOptions{
		Level: slog.LevelInfo,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			// Format time in a more readable way
			if a.Key == slog.TimeKey {
				return slog.Attr{
					Key:   slog.TimeKey,
					Value: slog.StringValue(a.Value.Time().Format(time.RFC3339)),
				}
			}
			return a
		},
	}

	handler := slog.NewTextHandler(os.Stdout, opts)
	return &Logger{
		logger: slog.New(handler),
	}
}

// With returns a new logger with the given attributes
func (l *Logger) With(args ...any) *Logger {
	return &Logger{
		logger: l.logger.With(args...),
	}
}

// Debug logs at debug level
func (l *Logger) Debug(msg string, args ...any) {
	l.logger.Debug(msg, args...)
}

// Info logs at info level
func (l *Logger) Info(msg string, args ...any) {
	l.logger.Info(msg, args...)
}

// Warn logs at warn level
func (l *Logger) Warn(msg string, args ...any) {
	l.logger.Warn(msg, args...)
}

// Error logs at error level
func (l *Logger) Error(msg string, args ...any) {
	l.logger.Error(msg, args...)
}

// ContextKey is a type for context keys
type ContextKey string

const (
	// LoggerContextKey is the key used to store the logger in the context
	LoggerContextKey ContextKey = "logger"
)

// WithLogger adds a logger to the context
func WithLogger(ctx context.Context, logger *Logger) context.Context {
	return context.WithValue(ctx, LoggerContextKey, logger)
}

// FromContext retrieves the logger from the context
func FromContext(ctx context.Context) *Logger {
	if logger, ok := ctx.Value(LoggerContextKey).(*Logger); ok {
		return logger
	}
	return NewLogger()
}
