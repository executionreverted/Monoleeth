package social

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
	"time"

	"gameproject/internal/game/foundation"
)

// ChatStore is the persistence boundary for chat messages and channel routing.
type ChatStore interface {
	AppendMessage(msg ChatMessage) error
	Messages(channelID ChannelID, limit int) ([]ChatMessage, error)
}

// PlayerNameResolver resolves the server-owned display name for a player.
type PlayerNameResolver interface {
	PlayerName(playerID foundation.PlayerID) (PlayerName, error)
}

// ChannelMembershipResolver resolves who can read/write a channel.
type ChannelMembershipResolver interface {
	ResolveChannel(input ResolveChannelInput) (ResolveChannelResult, error)
	ChannelMembers(channelID ChannelID) ([]foundation.PlayerID, error)
}

// MessageModerationHook can redact or reject a message before it is stored.
type MessageModerationHook interface {
	ModerateMessage(msg ChatMessage) (ChatMessage, bool, error)
}

// ChatModerationLogger records safe moderation decisions without raw content.
type ChatModerationLogger interface {
	RecordChatModeration(entry ChatModerationLogEntry) error
}

// ChatModerationAction identifies one safe audit action.
type ChatModerationAction string

const (
	ChatModerationActionRedacted ChatModerationAction = "redacted"
	ChatModerationActionRejected ChatModerationAction = "rejected"
)

// ChatModerationLogEntry is a content-safe audit record. It intentionally
// stores only a keyed content fingerprint/length, never raw message text.
type ChatModerationLogEntry struct {
	MessageID     MessageID            `json:"message_id"`
	ChannelKind   ChannelKind          `json:"channel_kind"`
	ChannelID     ChannelID            `json:"channel_id"`
	SenderID      foundation.PlayerID  `json:"sender_id"`
	Action        ChatModerationAction `json:"action"`
	Reason        string               `json:"reason"`
	ContentHMAC   string               `json:"content_hmac"`
	ContentLength int                  `json:"content_length"`
	LoggedAt      time.Time            `json:"logged_at"`
}

// RateLimitChecker checks whether a player is within the chat rate limit.
type RateLimitChecker interface {
	CheckChatRateLimit(playerID foundation.PlayerID, now time.Time) error
}

// ChatService owns server-authoritative chat message validation, routing,
// rate limiting, and moderation.
type ChatService struct {
	mu            sync.Mutex
	store         ChatStore
	names         PlayerNameResolver
	membership    ChannelMembershipResolver
	moderation    MessageModerationHook
	modLogger     ChatModerationLogger
	rateLimiter   RateLimitChecker
	clock         foundation.Clock
	moderationKey []byte
	messageSeq    uint64
	perPlayerLast map[foundation.PlayerID]time.Time
}

type ChatServiceConfig struct {
	Store       ChatStore
	Names       PlayerNameResolver
	Membership  ChannelMembershipResolver
	Moderation  MessageModerationHook
	ModLogger   ChatModerationLogger
	RateLimiter RateLimitChecker
	Clock       foundation.Clock
}

func NewChatService(config ChatServiceConfig) (*ChatService, error) {
	if config.Membership == nil {
		return nil, fmt.Errorf("membership resolver: %w", ErrInvalidChannelKind)
	}
	clock := config.Clock
	if clock == nil {
		clock = foundation.RealClock{}
	}
	return &ChatService{
		store:         config.Store,
		names:         config.Names,
		membership:    config.Membership,
		moderation:    config.Moderation,
		modLogger:     config.ModLogger,
		rateLimiter:   config.RateLimiter,
		clock:         clock,
		moderationKey: newModerationLogKey(),
		perPlayerLast: make(map[foundation.PlayerID]time.Time),
	}, nil
}

// SendMessage validates, rate-limits, moderates, stores, and routes one chat
// message. The returned result includes the authoritative message and the
// list of player ids who should receive it.
func (svc *ChatService) SendMessage(input SendChatInput) (SendChatResult, error) {
	if err := ValidateChannelKind(input.Kind); err != nil {
		return SendChatResult{}, err
	}
	if err := ValidateMessageContent(input.Content); err != nil {
		return SendChatResult{}, err
	}

	now := svc.clock.Now()

	resolution, err := svc.membership.ResolveChannel(ResolveChannelInput{
		Kind:     input.Kind,
		PlayerID: input.SenderID,
		MapID:    "",
	})
	if err != nil {
		return SendChatResult{}, err
	}
	if !resolution.CanWrite {
		return SendChatResult{}, ErrChannelAccessDenied
	}

	if svc.rateLimiter != nil {
		if err := svc.rateLimiter.CheckChatRateLimit(input.SenderID, now); err != nil {
			return SendChatResult{}, fmt.Errorf("%w: %v", ErrMessageRateLimited, err)
		}
	} else {
		svc.mu.Lock()
		last := svc.perPlayerLast[input.SenderID]
		if !last.IsZero() && now.Sub(last) < chatDefaultCooldown {
			svc.mu.Unlock()
			return SendChatResult{}, ErrMessageRateLimited
		}
		svc.perPlayerLast[input.SenderID] = now
		svc.mu.Unlock()
	}

	senderName := PlayerName("")
	if svc.names != nil {
		name, err := svc.names.PlayerName(input.SenderID)
		if err == nil {
			senderName = name
		}
	}

	svc.mu.Lock()
	svc.messageSeq++
	msgID := MessageID(fmt.Sprintf("chat-%d", svc.messageSeq))
	svc.mu.Unlock()

	msg := ChatMessage{
		MessageID:   msgID,
		ChannelKind: input.Kind,
		ChannelID:   resolution.ChannelID,
		SenderID:    input.SenderID,
		SenderName:  senderName,
		Content:     strings.TrimSpace(input.Content),
		SentAt:      now,
	}

	if svc.moderation != nil {
		original := msg
		moderated, allowed, err := svc.moderation.ModerateMessage(msg)
		if err != nil {
			return SendChatResult{}, err
		}
		if !allowed {
			if err := svc.recordModeration(original, ChatModerationActionRejected, "policy_rejected"); err != nil {
				return SendChatResult{}, err
			}
			return SendChatResult{}, ErrInvalidMessageContent
		}
		if moderated.Content != original.Content {
			if err := svc.recordModeration(original, ChatModerationActionRedacted, "policy_redacted"); err != nil {
				return SendChatResult{}, err
			}
		}
		msg = moderated
	}

	if svc.store != nil {
		if err := svc.store.AppendMessage(msg); err != nil {
			return SendChatResult{}, err
		}
	}

	members, _ := svc.membership.ChannelMembers(resolution.ChannelID)

	return SendChatResult{
		Message: msg,
		Members: members,
	}, nil
}

func (svc *ChatService) recordModeration(msg ChatMessage, action ChatModerationAction, reason string) error {
	if svc.modLogger == nil {
		return nil
	}
	mac := hmac.New(sha256.New, svc.moderationKey)
	_, _ = mac.Write([]byte(msg.Content))
	return svc.modLogger.RecordChatModeration(ChatModerationLogEntry{
		MessageID:     msg.MessageID,
		ChannelKind:   msg.ChannelKind,
		ChannelID:     msg.ChannelID,
		SenderID:      msg.SenderID,
		Action:        action,
		Reason:        reason,
		ContentHMAC:   hex.EncodeToString(mac.Sum(nil)),
		ContentLength: len(msg.Content),
		LoggedAt:      svc.clock.Now(),
	})
}

func newModerationLogKey() []byte {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err == nil {
		return key
	}
	fallback := sha256.Sum256([]byte(time.Now().UTC().String()))
	return fallback[:]
}

const chatDefaultCooldown = 500 * time.Millisecond

// InMemoryChatStore is a process-local ring buffer for recent chat messages.
type InMemoryChatStore struct {
	mu       sync.RWMutex
	messages map[ChannelID][]ChatMessage
}

func NewInMemoryChatStore() *InMemoryChatStore {
	return &InMemoryChatStore{
		messages: make(map[ChannelID][]ChatMessage),
	}
}

func (store *InMemoryChatStore) AppendMessage(msg ChatMessage) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	store.messages[msg.ChannelID] = append(store.messages[msg.ChannelID], msg)
	return nil
}

func (store *InMemoryChatStore) Messages(channelID ChannelID, limit int) ([]ChatMessage, error) {
	store.mu.RLock()
	defer store.mu.RUnlock()
	all := store.messages[channelID]
	if limit <= 0 || limit > len(all) {
		limit = len(all)
	}
	start := len(all) - limit
	if start < 0 {
		start = 0
	}
	result := make([]ChatMessage, limit)
	copy(result, all[start:])
	return result, nil
}
