package webhook

import (
	"io"
	"log/slog"
)

func nilLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
