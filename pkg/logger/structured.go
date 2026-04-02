package logger

import (
	"context"
	"log/slog"
	"os"
)

type StructuredLogger struct {
	base *slog.Logger
}

func NewStructuredLogger(env string) *StructuredLogger {
	level := slog.LevelInfo
	if env == "development" {
		level = slog.LevelDebug
	}

	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level})
	return &StructuredLogger{base: slog.New(handler)}
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
