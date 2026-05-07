package logger

import (
	"context"
	"log/slog"
	"os"
	"strings"
)

type StructuredLogger struct {
	base *slog.Logger
}

func NewStructuredLogger(env string) *StructuredLogger {
	return NewStructuredLoggerWithLevel(env, "")
}

func NewStructuredLoggerWithLevel(env, configuredLevel string) *StructuredLogger {
	level := parseLevel(configuredLevel)
	if strings.TrimSpace(configuredLevel) == "" && env == "development" {
		level = slog.LevelDebug
	}

	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level})
	return &StructuredLogger{base: slog.New(handler)}
}

func parseLevel(value string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

func (l *StructuredLogger) Logger() *slog.Logger {
	return l.base
}

func (l *StructuredLogger) With(args ...any) *slog.Logger {
	return l.base.With(args...)
}

func (l *StructuredLogger) WithContext(ctx context.Context, args ...any) *slog.Logger {
	return slog.New(l.base.Handler()).With(args...).WithGroup("request").With(slog.Any("ctx", ctx))
}
