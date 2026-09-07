package logger

// Level is a log severity.
type Level int

const (
	// LevelDebug is the most verbose level.
	LevelDebug Level = iota
	// LevelInfo is the default operational level.
	LevelInfo
	// LevelWarn signals a recoverable problem.
	LevelWarn
	// LevelError signals a failure that needs attention.
	LevelError
)

// String returns the lowercase level name.
func (l Level) String() string {
	switch l {
	case LevelDebug:
		return "debug"
	case LevelInfo:
		return "info"
	case LevelWarn:
		return "warn"
	case LevelError:
		return "error"
	default:
		return "info"
	}
}

// ParseLevel maps a name to a Level, defaulting to LevelInfo.
func ParseLevel(s string) Level {
	switch s {
	case "debug", "DEBUG":
		return LevelDebug
	case "warn", "WARN", "warning":
		return LevelWarn
	case "error", "ERROR":
		return LevelError
	default:
		return LevelInfo
	}
}

// Logger is Grove's structured logging abstraction.
//
// Implementations must be safe for concurrent use. Field values are
// attached with With; implementations should treat the keys as
// informational and never fail a log call because of them.
type Logger interface {
	// Debug logs at debug level.
	Debug(msg string, fields ...any)
	// Info logs at info level.
	Info(msg string, fields ...any)
	// Warn logs at warn level.
	Warn(msg string, fields ...any)
	// Error logs at error level.
	Error(msg string, fields ...any)

	// With returns a child logger carrying the given key/value pairs.
	With(fields ...any) Logger
	// SetLevel changes the minimum enabled level.
	SetLevel(l Level)
}
