package logger

import (
	"io"
	"log/slog"
	"os"
)

// StdLogger is the default Logger, backed by the standard library
// log/slog in JSON form. Zero dependencies, trivially replaceable.
type StdLogger struct {
	inner *slog.Logger
	level *slog.LevelVar
}

// StdOptions tunes the StdLogger.
type StdOptions struct {
	// Out receives the JSON lines; defaults to os.Stderr.
	Out io.Writer
	// Level is the minimum enabled level; defaults to LevelInfo.
	Level Level
	// AddSource includes file/line information.
	AddSource bool
}

// New returns a StdLogger writing JSON to stderr.
func New(opts ...StdOptions) *StdLogger {
	o := StdOptions{Out: os.Stderr, Level: LevelInfo}
	if len(opts) > 0 {
		o = opts[0]
	}
	if o.Out == nil {
		o.Out = os.Stderr
	}
	lvl := &slog.LevelVar{}
	lvl.Set(toSlogLevel(o.Level))
	h := slog.NewJSONHandler(o.Out, &slog.HandlerOptions{
		Level:     lvl,
		AddSource: o.AddSource,
	})
	return &StdLogger{inner: slog.New(h), level: lvl}
}

// toSlogLevel converts a Grove level to a slog level.
func toSlogLevel(l Level) slog.Level {
	switch l {
	case LevelDebug:
		return slog.LevelDebug
	case LevelWarn:
		return slog.LevelWarn
	case LevelError:
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// Debug logs at debug level.
func (l *StdLogger) Debug(msg string, fields ...any) { l.inner.Debug(msg, fields...) }

// Info logs at info level.
func (l *StdLogger) Info(msg string, fields ...any) { l.inner.Info(msg, fields...) }

// Warn logs at warn level.
func (l *StdLogger) Warn(msg string, fields ...any) { l.inner.Warn(msg, fields...) }

// Error logs at error level.
func (l *StdLogger) Error(msg string, fields ...any) { l.inner.Error(msg, fields...) }

// With returns a child logger carrying the given key/value pairs.
func (l *StdLogger) With(fields ...any) Logger {
	return &StdLogger{inner: l.inner.With(fields...), level: l.level}
}

// SetLevel changes the minimum enabled level.
func (l *StdLogger) SetLevel(level Level) { l.level.Set(toSlogLevel(level)) }

// Discard returns a Logger that drops everything. Useful in tests.
func Discard() Logger { return New(StdOptions{Out: io.Discard}) }
