package social

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"gameproject/internal/game/foundation"
)

// ChannelKind identifies the resolution class of a chat channel.
type ChannelKind string

const (
	ChannelKindSystem   ChannelKind = "system"
	ChannelKindLocalMap ChannelKind = "local_map"
	ChannelKindParty    ChannelKind = "party"
	ChannelKindClan     ChannelKind = "clan"
)

// ChannelID is the server-resolved channel identifier for routing.
type ChannelID string

// ClanID identifies one durable clan.
type ClanID string

// PartyID identifies one transient party.
type PartyID string

// MessageID identifies one chat message.
type MessageID string

// PlayerName is the server-owned display name for chat attribution.
type PlayerName string

// ClanTag is a short clan tag (3-5 chars).
type ClanTag string

// ClanRank identifies one membership rank within a clan.
type ClanRank string

const (
	ClanRankOwner   ClanRank = "owner"
	ClanRankOfficer ClanRank = "officer"
	ClanRankMember  ClanRank = "member"
)

var (
	ErrInvalidChannelKind    = errors.New("invalid channel kind")
	ErrInvalidChannelID      = errors.New("invalid channel id")
	ErrInvalidMessageContent = errors.New("invalid message content")
	ErrChannelAccessDenied   = errors.New("channel access denied")
	ErrInvalidPlayerName     = errors.New("invalid player name")
	ErrInvalidClanTag        = errors.New("invalid clan tag")
	ErrInvalidClanName       = errors.New("invalid clan name")
	ErrClanAlreadyExists     = errors.New("clan already exists")
	ErrClanNotFound          = errors.New("clan not found")
	ErrAlreadyInClan         = errors.New("player already in a clan")
	ErrNotInClan             = errors.New("player not in clan")
	ErrInvalidClanRank       = errors.New("invalid clan rank")
	ErrPartyNotFound         = errors.New("party not found")
	ErrPartyFull             = errors.New("party full")
	ErrPartyInviteNotFound   = errors.New("party invite not found")
	ErrAlreadyInParty        = errors.New("player already in party")
	ErrNotInParty            = errors.New("player not in party")
	ErrInvalidPartyID        = errors.New("invalid party id")
	ErrMessageRateLimited    = errors.New("chat message rate limited")
)

const (
	maxMessageLength  = 500
	maxClanNameLength = 32
	minClanNameLength = 3
	maxClanTagLength  = 5
	minClanTagLength  = 3
	maxPartyMembers   = 6
)

// ChatMessage is one server-owned chat message.
type ChatMessage struct {
	MessageID   MessageID           `json:"message_id"`
	ChannelKind ChannelKind         `json:"channel_kind"`
	ChannelID   ChannelID           `json:"channel_id"`
	SenderID    foundation.PlayerID `json:"sender_id"`
	SenderName  PlayerName          `json:"sender_name"`
	Content     string              `json:"content"`
	SentAt      time.Time           `json:"sent_at"`
}

// ResolveChannelInput asks the server to resolve a player's channel for routing.
// The client sends intent (kind + optional scope); the server resolves membership.
type ResolveChannelInput struct {
	Kind     ChannelKind
	PlayerID foundation.PlayerID
	MapID    string
	PartyID  PartyID
	ClanID   ClanID
}

// ResolveChannelResult reports the authoritative channel id and read access.
type ResolveChannelResult struct {
	ChannelID ChannelID
	CanRead   bool
	CanWrite  bool
}

// SendChatInput is the player intent for sending a chat message.
type SendChatInput struct {
	Kind      ChannelKind
	ChannelID ChannelID
	SenderID  foundation.PlayerID
	Content   string
}

// SendChatResult reports the authoritative message after server validation.
type SendChatResult struct {
	Message ChatMessage
	Members []foundation.PlayerID
}

func ValidateChannelKind(kind ChannelKind) error {
	switch kind {
	case ChannelKindSystem, ChannelKindLocalMap, ChannelKindParty, ChannelKindClan:
		return nil
	default:
		return fmt.Errorf("%q: %w", kind, ErrInvalidChannelKind)
	}
}

func ValidateMessageContent(content string) error {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return fmt.Errorf("empty: %w", ErrInvalidMessageContent)
	}
	if len(trimmed) > maxMessageLength {
		return fmt.Errorf("too long %d: %w", len(trimmed), ErrInvalidMessageContent)
	}
	return nil
}

func ValidatePlayerName(name PlayerName) error {
	trimmed := strings.TrimSpace(string(name))
	if trimmed == "" {
		return fmt.Errorf("empty: %w", ErrInvalidPlayerName)
	}
	return nil
}

func ValidateClanTag(tag ClanTag) error {
	trimmed := strings.TrimSpace(string(tag))
	if len(trimmed) < minClanTagLength || len(trimmed) > maxClanTagLength {
		return fmt.Errorf("len %d: %w", len(trimmed), ErrInvalidClanTag)
	}
	return nil
}

func ValidateClanName(name string) error {
	trimmed := strings.TrimSpace(name)
	if len(trimmed) < minClanNameLength || len(trimmed) > maxClanNameLength {
		return fmt.Errorf("len %d: %w", len(trimmed), ErrInvalidClanName)
	}
	return nil
}

func ValidateClanRank(rank ClanRank) error {
	switch rank {
	case ClanRankOwner, ClanRankOfficer, ClanRankMember:
		return nil
	default:
		return fmt.Errorf("%q: %w", rank, ErrInvalidClanRank)
	}
}
