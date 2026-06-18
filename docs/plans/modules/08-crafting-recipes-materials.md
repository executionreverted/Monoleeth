# Crafting, Recipes, And Materials

Date: 2026-06-17

## Purpose

Bu modül ham maddeleri işlenmiş materyale, modüle, gemi unlock'a ve building component'e dönüştürür.

Crafting oyunun ana güç üretim motorudur. Drop'lar çoğunlukla hammadde verir; bitmiş güç item'ları craft ile çıkar.

## Owns

```text
CraftingService
RecipeService
MaterialService
```

## Does Not Own

- NPC loot roll
- Market listing
- Planet production formula
- Wallet primitive
- Inventory primitive
- Ship unlock internals

Crafting çıktısı ship unlock ise `ShipService.UnlockShip` çağrılır.

## Material Categories

Raw:

- Iron Ore
- Carbon Shards
- Helium Dust
- Crystal Fragment
- Plasma Residue

Rare raw:

- Dark Matter Thread
- Quantum Core Fragment
- Nebula Pearl

Processed:

- Refined Alloy
- Laser Lens
- Shield Matrix
- Energy Cell
- Warp Coil
- Scanner Circuit

Planet-specific:

- Cryo Crystal
- Magma Alloy
- Helium Core
- Relic Dust
- Organic Circuit

Special:

- X Core Fragment
- X Core

## Recipe Definition

```text
recipe_id
category
output_type
output_item_id
output_amount
input_items_json
required_credits
required_rank
required_role_level
required_location_type
required_planet_type
craft_duration_seconds
tradeable_output
repeatable
```

Example:

```json
{
  "recipe_id": "laser_beta_t2",
  "output_item_id": "laser_beta_t2",
  "output_amount": 1,
  "inputs": [
    {"item_id": "refined_alloy", "qty": 200},
    {"item_id": "laser_lens", "qty": 40},
    {"item_id": "energy_cell", "qty": 25}
  ],
  "required_credits": 25000,
  "required_rank": 4,
  "required_role_level": {"crafting": 3},
  "craft_duration_seconds": 3600
}
```

## Crafting Location

Location types:

```text
station
owned_planet
planet_building
special_event_station
```

Location shape:

```text
station: location_id = station id
owned_planet: location_id = planet id
planet_building: location_id = building id, planet_id = planet id
special_event_station: location_id = event station id
```

Planet-local rule:

```text
Materials must exist at crafting location or allowed connected storage.
```

MVP:

- station craft uses account inventory
- planet craft uses planet storage
- planet-building craft also uses planet storage, but the building id must
  resolve to an active server-owned building on a planet owned by the player

Phase 06 implementation:

- `CraftingService.StartCraft` requires a `CraftLocationAuthorizer` for
  `owned_planet` and `planet_building` recipes before material reservation,
  wallet debit, or job creation.
- `production.CraftLocationAuthorizer` reads discovery planet ownership and
  production storage/building state. Unknown, unowned, or other-owned planets
  are rejected as not owned, and planet-building craft requires an active
  building on the owned planet.

## Commands

```text
StartCraft(player_id, recipe_id, location)
CancelCraft(player_id, job_id)
CompleteCraft(job_id)
ClaimCraftOutput(player_id, job_id)
GetAvailableRecipes(player_id, location)
```

MVP can auto-complete output on completion.

## Start Craft Flow

```go
func StartCraft(ctx context.Context, player PlayerID, recipeID string, loc Location) error {
	return db.Tx(ctx, func(tx Tx) error {
		recipe := recipeCatalog.Get(recipeID)
		state := progression.LoadState(tx, player)
		if err := recipe.ValidateRequirements(state, loc); err != nil {
			return err
		}
		if err := inventory.ReserveItems(tx, player, recipe.Inputs, loc, "craft_start", recipeID); err != nil {
			return err
		}
		if err := wallet.Debit(tx, player, "credits", recipe.RequiredCredits, "craft_fee", recipeID); err != nil {
			return err
		}
		job := CraftJob{
			PlayerID: player,
			RecipeID: recipeID,
			Location: loc,
			State: "running",
			CompletesAt: time.Now().Add(recipe.Duration),
		}
		tx.Crafting().Insert(job)
		return nil
	})
}
```

## Complete Craft Flow

```go
func CompleteCraft(ctx context.Context, jobID string) error {
	return db.Tx(ctx, func(tx Tx) error {
		job := tx.Crafting().Lock(jobID)
		if job.State != "running" {
			return ErrInvalidCraftState
		}
		if time.Now().Before(job.CompletesAt) {
			return ErrCraftNotReady
		}
		recipe := recipeCatalog.Get(job.RecipeID)
		inventory.CommitReservation(tx, job.ReservationID)

		switch recipe.OutputType {
		case "item":
			inventory.AddItem(tx, job.PlayerID, recipe.OutputItemID, recipe.OutputAmount, job.OutputLocation(), "craft_complete", jobID)
		case "ship_unlock":
			shipSvc.UnlockShipTx(tx, job.PlayerID, recipe.OutputItemID, "craft", jobID)
		}

		job.State = "completed"
		tx.Crafting().Save(job)
		progression.GrantCraftXP(tx, job.PlayerID, recipe, jobID)
		return nil
	})
}
```

## Cancellation

MVP'de cancellation olmayabilir.

Eğer olacaksa:

- Not ready jobs cancel edilebilir.
- Fee refund yok veya partial.
- Materials partial/full refund design gerekir.
- Abuse önlemek için cancellation cooldown olabilir.

Simpler:

```text
No cancellation in MVP.
```

## Craft XP

Craft XP:

```text
base = recipe.tier * recipe.duration_weight * recipe.value_weight
first_time_bonus optional
```

Abuse:

- low-tier spam diminishing return
- recipe cooldown for XP optional

Phase 06 implementation:

- `CraftingService` can accept a `CraftXPTracker` hook.
- Successful, non-duplicate craft XP grants emit `CraftXPObservation` metadata
  for later balancing: player/job, recipe source/version, category, output,
  location type, required rank, credit cost, duration, input counts, XP amounts,
  idempotency reference, and granted time.
- The MVP low-tier bucket is tracked as recipes with `required_rank <= 1` and
  `craft_duration <= 30m`. This is telemetry only; diminishing returns and
  daily soft caps remain a later balancing policy.

## X Core Craft

Fragment model:

```text
10 X Core Fragment
+ 50 Quantum Core Fragment
+ 500 Energy Cell
+ credits
-> 1 X Core
```

This creates:

- grind path
- market demand
- rare reward value

## Recipe Unlocks

Recipe availability from:

- rank
- role level
- quest unlock
- blueprint item
- planet building
- event

Quest/soulbound blueprint:

```text
unlock recipe for account
blueprint item non-tradeable
```

## Events Emitted

```text
craft.job_started
craft.job_cancelled
craft.job_completed
craft.output_claimed
recipe.unlocked
material.refined
player.craft_xp_granted
```

## Edge Cases

- Player starts craft twice with same materials.
- Materials reserved then job completion fails.
- Recipe definition changes while job running.
- Output location full.
- Ship unlock output already owned.
- Craft completion worker runs twice.
- Offline completion should process on login or background worker.

## Abuse Vectors

### Material Duplication

Risk:

- Reserve items but not consume on completion, then refund also happens.

Defense:

- reservation state machine
- transaction lock
- unique job completion

### Early Completion

Risk:

- Client sends complete before timer.

Defense:

- server `completes_at`
- server time only

### Recipe Requirement Bypass

Risk:

- Client sends hidden recipe or fake location.

Defense:

- server recipe catalog
- location ownership validation
- rank/role validation

### XP Craft Spam

Risk:

- Cheapest recipe spam for infinite XP.

Defense:

- diminishing returns
- XP based on recipe value
- daily soft cap by recipe/tier

## Testing Checklist

- Missing material fails.
- Missing credits fails.
- Rank too low fails.
- Wrong location fails.
- Start craft reserves materials.
- Complete before time fails.
- Complete after time creates output once.
- Duplicate complete no duplicate output.
- Ship unlock recipe idempotent.
- Craft XP granted once.

## Implementation Notes

MVP:

- no cancellation
- station/account inventory craft
- planet storage craft for processed materials
- module recipes
- basic ship unlock recipe
- X Core fragment recipe optional
