package apibudget

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
	"testing/quick"
)

func TestLogLevelConstants(t *testing.T) {
	// Verify iota ordering: Debug < Info < Warn < Error < Silent
	if LogLevelDebug >= LogLevelInfo {
		t.Error("LogLevelDebug should be less than LogLevelInfo")
	}
	if LogLevelInfo >= LogLevelWarn {
		t.Error("LogLevelInfo should be less than LogLevelWarn")
	}
	if LogLevelWarn >= LogLevelError {
		t.Error("LogLevelWarn should be less than LogLevelError")
	}
	if LogLevelError >= LogLevelSilent {
		t.Error("LogLevelError should be less than LogLevelSilent")
	}
}

func TestNewDefaultLogger(t *testing.T) {
	l := newDefaultLogger(LogLevelInfo)
	if l == nil {
		t.Fatal("newDefaultLogger returned nil")
	}
}

// setupTestLogger replaces slog.Default() with a logger that writes to a buffer,
// and returns the buffer and a cleanup function.
func setupTestLogger(t *testing.T) (*bytes.Buffer, func()) {
	t.Helper()
	var buf bytes.Buffer
	handler := slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})
	original := slog.Default()
	slog.SetDefault(slog.New(handler))
	return &buf, func() { slog.SetDefault(original) }
}

func TestDefaultLoggerFiltering_DebugLevel(t *testing.T) {
	buf, cleanup := setupTestLogger(t)
	defer cleanup()

	l := newDefaultLogger(LogLevelDebug)
	l.Debug("debug msg")
	l.Info("info msg")
	l.Warn("warn msg")
	l.Error("error msg")

	output := buf.String()
	for _, expected := range []string{"debug msg", "info msg", "warn msg", "error msg"} {
		if !strings.Contains(output, expected) {
			t.Errorf("expected output to contain %q at LogLevelDebug", expected)
		}
	}
}

func TestDefaultLoggerFiltering_InfoLevel(t *testing.T) {
	buf, cleanup := setupTestLogger(t)
	defer cleanup()

	l := newDefaultLogger(LogLevelInfo)
	l.Debug("debug msg")
	l.Info("info msg")
	l.Warn("warn msg")
	l.Error("error msg")

	output := buf.String()
	if strings.Contains(output, "debug msg") {
		t.Error("debug msg should be filtered at LogLevelInfo")
	}
	for _, expected := range []string{"info msg", "warn msg", "error msg"} {
		if !strings.Contains(output, expected) {
			t.Errorf("expected output to contain %q at LogLevelInfo", expected)
		}
	}
}

func TestDefaultLoggerFiltering_WarnLevel(t *testing.T) {
	buf, cleanup := setupTestLogger(t)
	defer cleanup()

	l := newDefaultLogger(LogLevelWarn)
	l.Debug("debug msg")
	l.Info("info msg")
	l.Warn("warn msg")
	l.Error("error msg")

	output := buf.String()
	for _, filtered := range []string{"debug msg", "info msg"} {
		if strings.Contains(output, filtered) {
			t.Errorf("%q should be filtered at LogLevelWarn", filtered)
		}
	}
	for _, expected := range []string{"warn msg", "error msg"} {
		if !strings.Contains(output, expected) {
			t.Errorf("expected output to contain %q at LogLevelWarn", expected)
		}
	}
}

func TestDefaultLoggerFiltering_ErrorLevel(t *testing.T) {
	buf, cleanup := setupTestLogger(t)
	defer cleanup()

	l := newDefaultLogger(LogLevelError)
	l.Debug("debug msg")
	l.Info("info msg")
	l.Warn("warn msg")
	l.Error("error msg")

	output := buf.String()
	for _, filtered := range []string{"debug msg", "info msg", "warn msg"} {
		if strings.Contains(output, filtered) {
			t.Errorf("%q should be filtered at LogLevelError", filtered)
		}
	}
	if !strings.Contains(output, "error msg") {
		t.Error("expected output to contain \"error msg\" at LogLevelError")
	}
}

func TestDefaultLoggerFiltering_SilentLevel(t *testing.T) {
	buf, cleanup := setupTestLogger(t)
	defer cleanup()

	l := newDefaultLogger(LogLevelSilent)
	l.Debug("debug msg")
	l.Info("info msg")
	l.Warn("warn msg")
	l.Error("error msg")

	output := buf.String()
	if output != "" {
		t.Errorf("expected no output at LogLevelSilent, got: %s", output)
	}
}

func TestLoggerInterface(t *testing.T) {
	// Verify that defaultLogger satisfies the Logger interface.
	var _ Logger = newDefaultLogger(LogLevelInfo)
}

// TestPropertyLogLevelFiltering verifies that for any LogLevel setting,
// log messages below that level are NOT output, and messages at or above
// that level DO appear in output.
// **Validates: Requirements 12.2**
func TestPropertyLogLevelFiltering(t *testing.T) {
	// logMethods maps each LogLevel (Debug=0..Error=3) to the corresponding
	// Logger method and a unique marker string.
	type logCall struct {
		method func(Logger, string)
		marker string
		level  LogLevel
	}

	calls := []logCall{
		{func(l Logger, m string) { l.Debug(m) }, "dbg_marker", LogLevelDebug},
		{func(l Logger, m string) { l.Info(m) }, "inf_marker", LogLevelInfo},
		{func(l Logger, m string) { l.Warn(m) }, "wrn_marker", LogLevelWarn},
		{func(l Logger, m string) { l.Error(m) }, "err_marker", LogLevelError},
	}

	f := func(levelRaw uint8) bool {
		// Map random uint8 to valid LogLevel range 0-4
		level := LogLevel(levelRaw % 5)

		// Set up a buffer-backed slog logger to capture output
		var buf bytes.Buffer
		handler := slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})
		original := slog.Default()
		slog.SetDefault(slog.New(handler))
		defer slog.SetDefault(original)

		logger := newDefaultLogger(level)

		// Call all 4 log methods with unique markers
		for _, c := range calls {
			c.method(logger, c.marker)
		}

		output := buf.String()

		// Verify filtering: messages below configured level must NOT appear,
		// messages at or above configured level must appear.
		for _, c := range calls {
			present := strings.Contains(output, c.marker)
			if c.level < level {
				// Below configured level: must NOT be in output
				if present {
					t.Logf("LogLevel=%d: marker %q should be filtered (method level %d < configured %d)",
						level, c.marker, c.level, level)
					return false
				}
			} else {
				// At or above configured level: must be in output
				// (except LogLevelSilent suppresses everything)
				if level == LogLevelSilent {
					if present {
						t.Logf("LogLevel=Silent: marker %q should not appear", c.marker)
						return false
					}
				} else {
					if !present {
						t.Logf("LogLevel=%d: marker %q should be present (method level %d >= configured %d)",
							level, c.marker, c.level, level)
						return false
					}
				}
			}
		}
		return true
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 200}); err != nil {
		t.Errorf("Property 19 (ログレベルフィルタリング) failed: %v", err)
	}
}
