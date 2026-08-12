// Package flowlog 记录可直接 tail 的对话执行流水。
package flowlog

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
)

var (
	mu     sync.RWMutex
	logger = slog.New(slog.NewTextHandler(io.Discard, nil))
)

// Configure 将流水同时输出到标准输出和指定文件。
func Configure(path string) (*os.File, string, error) {
	absolutePath, err := filepath.Abs(path)
	if err != nil {
		return nil, "", err
	}
	if err := os.MkdirAll(filepath.Dir(absolutePath), 0o755); err != nil {
		return nil, "", err
	}
	file, err := os.OpenFile(absolutePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, "", err
	}
	configured := slog.New(slog.NewTextHandler(io.MultiWriter(os.Stdout, file), &slog.HandlerOptions{Level: slog.LevelInfo}))
	mu.Lock()
	logger = configured
	mu.Unlock()
	return file, absolutePath, nil
}

func Info(message string, args ...any) {
	mu.RLock()
	current := logger
	mu.RUnlock()
	current.Info(message, args...)
}

func Error(message string, args ...any) {
	mu.RLock()
	current := logger
	mu.RUnlock()
	current.Error(message, args...)
}
