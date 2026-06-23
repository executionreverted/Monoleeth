# Intel, Sharing, And Coordinate Trading

Date: 2026-06-17

## Purpose

Bu modül keşfedilen planet bilgilerinin oyuncu memory'sine, share sistemine ve trade edilebilir intel item'larına dönüşmesini yönetir.

Core idea:

```text
World shared.
Knowledge personal.
Knowledge can become economy.
```

## Owns

```text
IntelItemService
CoordinateTradeService
ShareService
```

## Does Not Own

- Procedural planet generation
- Scan roll
- Planet claim
- Market sale flow
- Live radar rendering

## Player Intel

```text
player_planet_intel
- player_id
- planet_id
- coordinates_x
- coordinates_y
- intel_state
- confidence
- last_seen_at
- source_type
- source_reference
```

Intel states:

```text
fresh
stale
verified
invalidated
colonized_by_other
```

## Intel Sources

```text
scan_success
share_received
coordinate_scroll_used
quest_reward
market_purchase
admin
```

## Share System

Player can share limited intel:

```text
share X game units per day
MVP: only planets
```

Share flow:

```go
func SharePlanetIntel(ctx context.Context, from PlayerID, to PlayerID, planetID PlanetID) error {
	return db.Tx(ctx, func(tx Tx) error {
		if !intel.PlayerKnowsPlanet(tx, from, planetID) {
			return ErrIntelNotOwned
		}
		if !share.HasDailyQuota(tx, from) {
			return ErrShareLimitReached
		}
		payload := intel.LoadPlanetIntel(tx, from, planetID)
		intel.UpsertPlayerIntel(tx, to, payload.WithSource("share_received"))
		share.ConsumeQuota(tx, from)
		mail.SendSystemMailTx(tx, to, "planet_intel_shared", payload)
		return nil
	})
}
```

## Intel Item

Coordinate scroll/star chart item:

```text
item_instance_id
intel_type
planet_id
coordinates
planet_level_known
planet_type_known
owner_known
last_verified_at
confidence
stale_state
```

Usage:

```text
consume item or mark used
write player_planet_intel
  show in known-intel/map memory
```

MVP can make scroll consumed on use.

## Creating Intel Item

Player turns known planet intel into item:

Validation:

- player knows planet
- intel not invalidated
- daily/weekly creation cap optional
- credit fee optional

Flow:

```text
lock player intel
create item instance with server-signed payload
optionally mark intel exported
ledger item creation
```

## Market Staleness

If planet gets colonized while scroll listed:

Options:

1. Auto-unlist listing.
2. Mark stale and require buyer confirmation.
3. Let it sell as stale intel.

Recommended MVP:

```text
Auto-mark listing stale and hide from default search.
Seller can reverify/relist.
```

Planet ownership event:

```text
planet.claimed -> IntelItemService marks matching listed intel stale
```

## Confidence

Intel can have confidence:

```text
100 = verified exact
70 = shared recent
40 = stale
0 = invalid
```

MVP:

- exact coordinate remains known
- owner/status can become stale

## Server-Signed Intel

Client never creates coordinate item payload.

Payload created server-side from known intel and planet DB/procedural materialized object.

```go
func CreateIntelItem(player PlayerID, planet PlanetIntel) ItemInstance {
	return ItemInstance{
		ItemID: "planet_coordinate_scroll",
		Metadata: map[string]any{
			"planet_id": planet.ID,
			"x": planet.X,
			"y": planet.Y,
			"last_verified_at": time.Now(),
			"confidence": 100,
		},
	}
}
```

## Events Emitted

```text
intel.discovered
intel.shared
intel.mail_sent
intel.item_created
intel.item_used
intel.marked_stale
intel.verified
coordinate_scroll.listing_staled
```

## Edge Cases

- Receiver already knows planet with fresher intel.
- Planet claimed after share but before receiver opens mail.
- Intel item listed, planet owner changes.
- Player tries to share planet they no longer know? Knowledge stays, but stale.
- Coordinates remain useful even if planet claimed.
- Stale intel sale fairness.

## Abuse Vectors

### Coordinate Forgery

Risk:

- Client creates fake coordinate item or edits payload.

Defense:

- item metadata server-side only
- item instance stored DB-side
- client cannot submit arbitrary intel payload

### Share Spam

Risk:

- Large clan instantly shares all discoveries.

Defense:

- daily quota
- share cost
- role/building unlock for larger sharing

### Market Stale Scam

Risk:

- Seller lists valuable planet intel after it becomes claimed.

Defense:

- claim event marks listings stale
- buyer UI shows last_verified_at/confidence
- optional auto-unlist

### Known-Intel Reveal Abuse

Risk:

- Player buys many cheap scrolls to reveal too many remote map memories
  instantly.

Defense:

- scrolls reveal only specific planet/intel point
- no area-wide reveal unless designed
- daily use limit optional

## Testing Checklist

- Share requires source intel.
- Share quota enforced.
- Receiver gets known-intel memory.
- Intel item creation uses server data.
- Intel item use writes player intel.
- Planet claimed marks listed intel stale.
- Existing fresher intel not overwritten by stale.
- Client cannot create arbitrary intel.

Current backend foundation:

- Phase07R adds `internal/game/intel` with in-memory tests for source intel
  ownership, non-invalidated visibility, server-created coordinate payloads,
  share/create/use idempotency, consume-once coordinate item state, reference
  mismatch rejection, and clone-on-read safety.
- Phase07S wires the intel service into authenticated realtime commands:
  `intel.share`, `intel.coordinate_item.create`, and
  `intel.coordinate_item.use`. Gateway handlers reject client-authored
  coordinate/source/ownership payloads, bridge discovery read-model intel into
  the intel domain, update discovery after share/use, and queue safe
  known-planets/detail refresh events.
- Phase07T backs coordinate items with the economy inventory service:
  coordinate item creation grants an exact server-authored
  `planet_coordinate_scroll` instance into the player's account inventory,
  records an item ledger increase, and emits an inventory snapshot; coordinate
  item use requires and removes that exact owned instance before committing the
  intel use, records an item ledger decrease, and emits inventory plus
  intel/discovery reconciliation events.
- Daily quotas, market listing staleness, durable DB rows, and cross-service
  transaction/compensation boundaries are still future slices.

## Implementation Notes

MVP:

- planet-only share
- daily share quota
- coordinate scroll item
- market stale marking
- confidence and last_seen fields
