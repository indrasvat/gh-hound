package logging

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

type Options struct {
	StateHome string
	Level     string
}

type Configured struct {
	Logger *slog.Logger
	Path   string

	once sync.Once
	file *os.File
	err  error
}

func Configure(options Options) (*Configured, error) {
	level, err := parseLevel(options.Level)
	if err != nil {
		return nil, err
	}
	stateHome := options.StateHome
	if stateHome == "" {
		stateHome = os.Getenv("XDG_STATE_HOME")
	}
	if stateHome == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("resolve home dir: %w", err)
		}
		stateHome = filepath.Join(home, ".local", "state")
	}

	dir := filepath.Join(stateHome, "gh-hound")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create log dir: %w", err)
	}
	path := filepath.Join(dir, "gh-hound.log")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open log file: %w", err)
	}
	logger := slog.New(slog.NewJSONHandler(file, &slog.HandlerOptions{Level: level}))
	return &Configured{Logger: logger, Path: path, file: file}, nil
}

func (c *Configured) Close() error {
	if c == nil || c.file == nil {
		return nil
	}
	c.once.Do(func() {
		c.err = c.file.Close()
	})
	return c.err
}

func parseLevel(raw string) (slog.Level, error) {
	switch strings.ToLower(raw) {
	case "", "info":
		return slog.LevelInfo, nil
	case "debug":
		return slog.LevelDebug, nil
	case "warn", "warning":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return slog.LevelInfo, fmt.Errorf("invalid log level %q", raw)
	}
}
