package foundation

import (
	"errors"
	"reflect"
	"testing"
)

func TestIdempotencyKeyHelpersProduceStableKeys(t *testing.T) {
	cases := []struct {
		name  string
		build func() (IdempotencyKey, error)
		want  string
	}{
		{
			name:  "quest reward",
			build: func() (IdempotencyKey, error) { return QuestRewardIdempotencyKey(QuestID("player-quest-9")) },
			want:  "quest_reward:player-quest-9",
		},
		{
			name:  "craft complete",
			build: func() (IdempotencyKey, error) { return CraftCompleteIdempotencyKey("craft-job-4") },
			want:  "craft_complete:craft-job-4",
		},
		{
			name:  "loot pickup",
			build: func() (IdempotencyKey, error) { return LootPickupIdempotencyKey("drop-8") },
			want:  "loot_pickup:drop-8",
		},
		{
			name:  "auction close",
			build: func() (IdempotencyKey, error) { return AuctionCloseIdempotencyKey(AuctionID("auction-3")) },
			want:  "auction_close:auction-3",
		},
		{
			name:  "premium webhook",
			build: func() (IdempotencyKey, error) { return PremiumWebhookIdempotencyKey("provider-event-5") },
			want:  "premium_webhook:provider-event-5",
		},
		{
			name: "offline settlement",
			build: func() (IdempotencyKey, error) {
				return OfflineSettlementIdempotencyKey(PlanetID("planet-4"), "window-20260617-10")
			},
			want: "offline_settlement:planet-4:window-20260617-10",
		},
		{
			name: "market buy",
			build: func() (IdempotencyKey, error) {
				return MarketBuyIdempotencyKey(ListingID("listing-9"), PlayerID("player-2"), RequestID("request-5"))
			},
			want: "market_buy:listing-9:player-2:request-5",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			key, err := tc.build()
			if err != nil {
				t.Fatalf("build key: %v", err)
			}

			if got := key.String(); got != tc.want {
				t.Fatalf("String() = %q, want %q", got, tc.want)
			}
			if err := key.Validate(); err != nil {
				t.Fatalf("Validate() = %v, want nil", err)
			}

			parsed, err := ParseIdempotencyKey(tc.want)
			if err != nil {
				t.Fatalf("parse stable key: %v", err)
			}
			if parsed != key {
				t.Fatalf("ParseIdempotencyKey() = %q, want %q", parsed, key)
			}
		})
	}
}

func TestIdempotencyKeyIsSeparateFromRequestID(t *testing.T) {
	if reflect.TypeOf(IdempotencyKey("")) == reflect.TypeOf(RequestID("")) {
		t.Fatal("IdempotencyKey and RequestID share the same Go type")
	}

	key, err := MarketBuyIdempotencyKey(ListingID("listing-9"), PlayerID("player-2"), RequestID("request-5"))
	if err != nil {
		t.Fatalf("build market buy key: %v", err)
	}
	if reflect.TypeOf(key) != reflect.TypeOf(IdempotencyKey("")) {
		t.Fatalf("domain key type = %v, want foundation.IdempotencyKey", reflect.TypeOf(key))
	}
	if reflect.TypeOf(key) == reflect.TypeOf(RequestID("")) {
		t.Fatalf("domain key type = %v, want separate type from RequestID", reflect.TypeOf(key))
	}

	if _, err := ParseRequestID(""); !errors.Is(err, ErrEmptyID) {
		t.Fatalf("ParseRequestID blank error = %v, want ErrEmptyID", err)
	}
	if _, err := ParseIdempotencyKey(""); !errors.Is(err, ErrEmptyIdempotencyKey) {
		t.Fatalf("ParseIdempotencyKey blank error = %v, want ErrEmptyIdempotencyKey", err)
	}
}

func TestIdempotencyKeyRejectsMalformedKeys(t *testing.T) {
	for _, value := range []string{
		"request-123",
		"unknown:part",
		"quest_reward:",
		"quest_reward:player-quest-9:extra",
		"offline_settlement:planet-4",
		"market_buy:listing-9:player-2",
	} {
		t.Run(value, func(t *testing.T) {
			_, err := ParseIdempotencyKey(value)
			if err == nil {
				t.Fatalf("ParseIdempotencyKey(%q) error = nil, want error", value)
			}
		})
	}
}

func TestIdempotencyKeyRejectsBlankValues(t *testing.T) {
	for _, value := range []string{"", " ", "\t"} {
		t.Run("parse "+value, func(t *testing.T) {
			_, err := ParseIdempotencyKey(value)
			if !errors.Is(err, ErrEmptyIdempotencyKey) {
				t.Fatalf("ParseIdempotencyKey(%q) error = %v, want ErrEmptyIdempotencyKey", value, err)
			}
		})
	}

	var key IdempotencyKey
	if !key.IsZero() {
		t.Fatal("zero key IsZero() = false, want true")
	}
	if err := key.Validate(); !errors.Is(err, ErrEmptyIdempotencyKey) {
		t.Fatalf("zero key Validate() = %v, want ErrEmptyIdempotencyKey", err)
	}
	if got := key.String(); got != "" {
		t.Fatalf("zero key String() = %q, want empty string", got)
	}
}

func TestIdempotencyKeyHelpersRejectDelimiterParts(t *testing.T) {
	cases := []struct {
		name  string
		build func() (IdempotencyKey, error)
	}{
		{
			name:  "craft complete delimiter",
			build: func() (IdempotencyKey, error) { return CraftCompleteIdempotencyKey("craft:job:4") },
		},
		{
			name: "offline settlement delimiter",
			build: func() (IdempotencyKey, error) {
				return OfflineSettlementIdempotencyKey(PlanetID("planet-4"), "window:20260617")
			},
		},
		{
			name: "market buy request delimiter",
			build: func() (IdempotencyKey, error) {
				return MarketBuyIdempotencyKey(ListingID("listing-9"), PlayerID("player-2"), RequestID("request:5"))
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := tc.build()
			if !errors.Is(err, ErrInvalidIdempotencyPart) {
				t.Fatalf("build key error = %v, want ErrInvalidIdempotencyPart", err)
			}
		})
	}
}

func TestIdempotencyKeyHelpersRejectBlankParts(t *testing.T) {
	cases := []struct {
		name  string
		build func() (IdempotencyKey, error)
	}{
		{
			name:  "quest reward player quest id",
			build: func() (IdempotencyKey, error) { return QuestRewardIdempotencyKey(QuestID("")) },
		},
		{
			name:  "craft complete job id",
			build: func() (IdempotencyKey, error) { return CraftCompleteIdempotencyKey(" ") },
		},
		{
			name:  "loot pickup drop id",
			build: func() (IdempotencyKey, error) { return LootPickupIdempotencyKey("\t") },
		},
		{
			name:  "auction close auction id",
			build: func() (IdempotencyKey, error) { return AuctionCloseIdempotencyKey(AuctionID("")) },
		},
		{
			name:  "premium webhook provider event id",
			build: func() (IdempotencyKey, error) { return PremiumWebhookIdempotencyKey("") },
		},
		{
			name: "offline settlement planet id",
			build: func() (IdempotencyKey, error) {
				return OfflineSettlementIdempotencyKey(PlanetID(""), "window-20260617-10")
			},
		},
		{
			name: "offline settlement window",
			build: func() (IdempotencyKey, error) {
				return OfflineSettlementIdempotencyKey(PlanetID("planet-4"), "")
			},
		},
		{
			name: "market buy listing id",
			build: func() (IdempotencyKey, error) {
				return MarketBuyIdempotencyKey(ListingID(""), PlayerID("player-2"), RequestID("request-5"))
			},
		},
		{
			name: "market buy buyer id",
			build: func() (IdempotencyKey, error) {
				return MarketBuyIdempotencyKey(ListingID("listing-9"), PlayerID(""), RequestID("request-5"))
			},
		},
		{
			name: "market buy request id",
			build: func() (IdempotencyKey, error) {
				return MarketBuyIdempotencyKey(ListingID("listing-9"), PlayerID("player-2"), RequestID(""))
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := tc.build()
			if !errors.Is(err, ErrEmptyIdempotencyPart) {
				t.Fatalf("build key error = %v, want ErrEmptyIdempotencyPart", err)
			}
		})
	}
}
