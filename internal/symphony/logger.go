package symphony

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type Logger struct {
	*slog.Logger
	file *os.File
	mu   sync.Mutex
}

func NewLogger(logsRoot string, stdout io.Writer) (*Logger, error) {
	if err := os.MkdirAll(logsRoot, 0o755); err != nil {
		return nil, err
	}
	filePath := filepath.Join(logsRoot, "symphony.log")
	file, err := os.OpenFile(filePath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, err
	}
	writer := io.MultiWriter(stdout, file)
	handler := slog.NewTextHandler(writer, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})
	return &Logger{Logger: slog.New(handler), file: file}, nil
}

func (l *Logger) Close() error {
	if l == nil || l.file == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.file.Close()
}

func truncateForLog(value string, maxBytes int) string {
	if maxBytes <= 0 || len(value) <= maxBytes {
		return value
	}
	return value[:maxBytes] + "...<truncated>"
}

func nowUTC() time.Time {
	return time.Now().UTC()
}

func kvString(args ...any) string {
	var b strings.Builder
	for i := 0; i+1 < len(args); i += 2 {
		if b.Len() > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(fmt.Sprintf("%v=%v", args[i], args[i+1]))
	}
	return b.String()
}
