// Package logger defines Grove's structured logging abstraction.
//
// The default implementation wraps the standard library log/slog so
// there are zero third-party dependencies. Swap it for zap, zerolog,
// or any backend by implementing the Logger interface.
package logger
