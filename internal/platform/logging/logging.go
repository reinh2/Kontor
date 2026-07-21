package logging

import (
	"log/slog"
	"os"
	"strings"
)

func New(environment string) *slog.Logger {
	level := slog.LevelInfo
	if strings.EqualFold(environment, "development") {
		level = slog.LevelDebug
	}
	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level}))
}
