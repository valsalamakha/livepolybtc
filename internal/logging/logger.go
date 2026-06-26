// Package logging provides leveled console logging plus a structured event
// recorder that writes every trading event to both CSV and JSON-lines files.
package logging

import (
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"sync"
)

// Level is a log severity.
type Level int

const (
	// LevelDebug is the most verbose level.
	LevelDebug Level = iota
	// LevelInfo is the default level.
	LevelInfo
	// LevelWarn logs recoverable problems.
	LevelWarn
	// LevelError logs failures.
	LevelError
)

// ParseLevel converts a string to a Level, defaulting to LevelInfo.
func ParseLevel(s string) Level {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return LevelDebug
	case "warn", "warning":
		return LevelWarn
	case "error":
		return LevelError
	default:
		return LevelInfo
	}
}

func (l Level) String() string {
	switch l {
	case LevelDebug:
		return "DEBUG"
	case LevelInfo:
		return "INFO"
	case LevelWarn:
		return "WARN"
	case LevelError:
		return "ERROR"
	default:
		return "?"
	}
}

// Logger is a minimal leveled logger writing to an io.Writer.
type Logger struct {
	mu      sync.Mutex
	level   Level
	out     *log.Logger
	enabled bool
}

// New constructs a Logger at the given level. When console is false, output is
// discarded (events are still captured by the EventRecorder).
func New(level Level, console bool) *Logger {
	var w io.Writer = os.Stdout
	if !console {
		w = io.Discard
	}
	return &Logger{
		level:   level,
		out:     log.New(w, "", log.LstdFlags|log.Lmicroseconds),
		enabled: console,
	}
}

func (l *Logger) logf(lvl Level, format string, args ...any) {
	if lvl < l.level {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.out.Printf("[%s] %s", lvl, fmt.Sprintf(format, args...))
}

// Debugf logs at debug level.
func (l *Logger) Debugf(format string, args ...any) { l.logf(LevelDebug, format, args...) }

// Infof logs at info level.
func (l *Logger) Infof(format string, args ...any) { l.logf(LevelInfo, format, args...) }

// Warnf logs at warn level.
func (l *Logger) Warnf(format string, args ...any) { l.logf(LevelWarn, format, args...) }

// Errorf logs at error level.
func (l *Logger) Errorf(format string, args ...any) { l.logf(LevelError, format, args...) }
