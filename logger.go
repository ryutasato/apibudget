package apibudget

import "log/slog"

// LogLevel はログレベルを表す。
type LogLevel int

const (
	LogLevelDebug  LogLevel = iota // デバッグレベル
	LogLevelInfo                   // 情報レベル
	LogLevelWarn                   // 警告レベル
	LogLevelError                  // エラーレベル
	LogLevelSilent                 // ログ出力なし
)

// Logger はログ出力を抽象化する。slog.Logger互換。
type Logger interface {
	Debug(msg string, args ...any)
	Info(msg string, args ...any)
	Warn(msg string, args ...any)
	Error(msg string, args ...any)
}

// defaultLogger は slog.Default() をラップし、LogLevel によるフィルタリングを行うデフォルト実装。
type defaultLogger struct {
	logger *slog.Logger
	level  LogLevel
}

// newDefaultLogger は指定されたログレベルでフィルタリングする slog ベースの Logger を返す。
func newDefaultLogger(level LogLevel) Logger {
	return &defaultLogger{
		logger: slog.Default(),
		level:  level,
	}
}

func (l *defaultLogger) Debug(msg string, args ...any) {
	if l.level <= LogLevelDebug {
		l.logger.Debug(msg, args...)
	}
}

func (l *defaultLogger) Info(msg string, args ...any) {
	if l.level <= LogLevelInfo {
		l.logger.Info(msg, args...)
	}
}

func (l *defaultLogger) Warn(msg string, args ...any) {
	if l.level <= LogLevelWarn {
		l.logger.Warn(msg, args...)
	}
}

func (l *defaultLogger) Error(msg string, args ...any) {
	if l.level <= LogLevelError {
		l.logger.Error(msg, args...)
	}
}
