package social

import (
	"regexp"
	"sync"
	"time"
)

var (
	emailPattern       = regexp.MustCompile(`(?i)\b[A-Z0-9._%+\-]+@[A-Z0-9.\-]+\.[A-Z]{2,}\b`)
	bearerTokenPattern = regexp.MustCompile(`(?i)\b(bearer|token|session|password|secret)\s*[:=]\s*\S+`)
)

// PIIChatModerationPolicy redacts common user-authored secrets/PII from chat.
type PIIChatModerationPolicy struct{}

func NewPIIChatModerationPolicy() PIIChatModerationPolicy {
	return PIIChatModerationPolicy{}
}

func (PIIChatModerationPolicy) ModerateMessage(msg ChatMessage) (ChatMessage, bool, error) {
	content := emailPattern.ReplaceAllString(msg.Content, "[redacted:email]")
	content = bearerTokenPattern.ReplaceAllString(content, "[redacted:secret]")
	msg.Content = content
	return msg, true, nil
}

// MemoryChatModerationLogger stores safe moderation audit entries in memory.
type MemoryChatModerationLogger struct {
	mu      sync.RWMutex
	entries []ChatModerationLogEntry
}

func NewMemoryChatModerationLogger() *MemoryChatModerationLogger {
	return &MemoryChatModerationLogger{}
}

func (logger *MemoryChatModerationLogger) RecordChatModeration(entry ChatModerationLogEntry) error {
	if logger == nil {
		return nil
	}
	if entry.LoggedAt.IsZero() {
		entry.LoggedAt = time.Now().UTC()
	}
	logger.mu.Lock()
	defer logger.mu.Unlock()
	logger.entries = append(logger.entries, entry)
	return nil
}

func (logger *MemoryChatModerationLogger) Snapshot() []ChatModerationLogEntry {
	if logger == nil {
		return nil
	}
	logger.mu.RLock()
	defer logger.mu.RUnlock()
	result := make([]ChatModerationLogEntry, len(logger.entries))
	copy(result, logger.entries)
	return result
}
