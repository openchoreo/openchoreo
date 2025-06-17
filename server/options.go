package server

import "github.com/openchoreo/openchoreo/server/pkg/logging"

type Option func(*Server)

// WithPort sets the port for the server.
func WithPort(port int) Option {
	return func(s *Server) {
		s.port = port
	}
}

// WithLogger sets the logger for the server.
func WithLogger(logger *logging.Logger) Option {
	return func(s *Server) {
		s.logger = logger
	}
}
