package social

import (
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
	rateLimiter   RateLimitChecker
	clock         foundation.Clock
	messageSeq    uint64
	perPlayerLast map[foundation.PlayerID]time.Time
}

type ChatServiceConfig struct {
	Store       ChatStore
	Names       PlayerNameResolver
	Membership  ChannelMembershipResolver
	Moderation  MessageModerationHook
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
		rateLimiter:   config.RateLimiter,
		clock:         clock,
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
		Kind:      input.Kind,
		PlayerID:  input.SenderID,
		MapID:     "",
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
		moderated, allowed, err := svc.moderation.ModerateMessage(msg)
		if err != nil {
			return SendChatResult{}, err
		}
		if !allowed {
			return SendChatResult{}, ErrInvalidMessageContent
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
		Message:  msg,
		Members:  members,
	}, nil
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
