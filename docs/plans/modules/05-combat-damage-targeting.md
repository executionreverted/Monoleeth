# Combat, Damage, Targeting, Energy, And Cooldowns

Date: 2026-06-17

## Purpose

Bu modül server-authoritative combat loop'u tanımlar.

Kapsam:

- Target lock
- Auto/basic attack
- Manual skill usage
- Range validation
- Visibility / sight validation
- Energy cost
- Cooldown
- Hit/miss
- Shield/HP damage
- NPC death contribution
- Combat XP hooks
- Loot ownership hooks

## Owns

```text
CombatService
TargetingService
DamageService
EnergyService
CooldownService
AggroService
```

## Does Not Own

- Loot roll table
- XP table
- Inventory mutation
- Ship/module equip
- AOI/fog truth
- Death cargo drop

Combat sonuç üretir; diğer modüller ödül ve state transferini yapar.
For quest progress, `combat.npc_killed` must include the server-owned owner
player and NPC type; the client never supplies either value. NPC combat actors
composed from authoritative stat snapshots must carry that server-owned NPC
type before they can enter combat.

## Core Rule

Client sadece intent yollar.

```text
intent_attack(target_id)
intent_use_skill(skill_id, target_id or position)
intent_set_target(target_id)
intent_toggle_auto_attack(enabled)
```

Server karar verir:

- Target görülebilir mi?
- Target range içinde mi?
- Weapon cooldown hazır mı?
- Energy yeterli mi?
- Hit oldu mu?
- Damage ne kadar?
- Shield/HP nasıl değişti?
- Target öldü mü?
- Loot right kime ait?

## Combat State

```text
entity_id
entity_type
hp_current
shield_current
energy_current
target_id
combat_flags
last_damage_at
last_attack_at
cooldowns
aggro_table
```

Combat state active zone worker memory'sinde tutulabilir. Persistent state gereken durumlar DB'ye yazılır:

- player death
- module durability loss
- quest progress
- XP reward
- loot creation

## Target Validation

```go
func ValidateTarget(ctx CombatContext, attacker Entity, target Entity, stats EffectiveStats) error {
	if !attacker.Alive() {
		return ErrAttackerDead
	}
	if !target.Alive() {
		return ErrTargetDead
	}
	if attacker.WorldID != target.WorldID {
		return ErrDifferentWorld
	}
	if !ctx.Visibility.CanSee(attacker.ID, target.ID) {
		return ErrTargetNotVisible
	}
	if Distance(attacker.Pos, target.Pos) > stats.WeaponRange {
		return ErrOutOfRange
	}
	if ctx.Relationship.IsFriendly(attacker, target) && !ctx.Rules.AllowFriendlyFire {
		return ErrFriendlyTarget
	}
	return nil
}
```

Visibility burada kritik:

```text
AOI içinde değilse attack yok.
Fog/radar/sight detect etmemişse attack yok.
Client target id bilse bile server izin vermez.
```

## Auto Attack Loop

Server tick:

```text
for each combat entity with auto_attack enabled:
  if target invalid:
    stop auto attack
  if cooldown not ready:
    continue
  if energy not enough:
    continue
  execute basic attack
```

Go-ish:

```go
func TickAutoAttack(now time.Time, actor *CombatActor) {
	if !actor.AutoAttack.Enabled {
		return
	}
	target := world.GetEntity(actor.TargetID)
	if err := combat.ValidateTarget(actor, target); err != nil {
		actor.AutoAttack.Enabled = false
		return
	}
	if !actor.Cooldowns.Ready("basic_attack", now) {
		return
	}
	if actor.Energy < actor.Stats.BasicAttackEnergyCost {
		return
	}
	combat.ExecuteBasicAttack(now, actor, target)
}
```

## Manual Skill Flow

Request:

```json
{
  "op": "combat.use_skill",
  "skill_id": "rocket_launcher",
  "target_id": "npc_123",
  "client_seq": 12991
}
```

Server:

```text
load active ship stats
execute mutation inside the active ship combat lease shared with death disable
load equipped skill/module
validate target/position
validate cooldown
validate energy/ammo
validate state restrictions
consume energy/ammo
start cooldown
apply effect
broadcast result
```

Skill may be:

- targeted
- ground/space position
- self buff
- cone/area
- scan/jammer utility

## Energy Cost

Energy is combat throttle.

```go
func SpendEnergy(actor *CombatActor, cost int) error {
	if cost <= 0 {
		return ErrInvalidEnergyCost
	}
	if actor.Energy < cost {
		return ErrNotEnoughEnergy
	}
	actor.Energy -= cost
	return nil
}
```

Energy regen server tick:

```go
func RegenEnergy(actor *CombatActor, dt float64) {
	actor.Energy += actor.Stats.EnergyRegen * dt
	if actor.Energy > actor.Stats.EnergyMax {
		actor.Energy = actor.Stats.EnergyMax
	}
}
```

Laser basic attack can cost energy:

```text
basic_laser_cost = weapon.energy_cost
if energy < cost:
  no shot, cooldown not consumed
```

## Cooldown

Cooldown state:

```text
cooldown_key -> ready_at
```

Validation:

```go
func StartCooldown(cds Cooldowns, key string, now time.Time, duration time.Duration) error {
	if now.Before(cds[key]) {
		return ErrCooldownNotReady
	}
	cds[key] = now.Add(duration)
	return nil
}
```

Use server time only.

Never trust client timestamp.

## Hit / Miss

MVP can use deterministic roll:

```text
hitChance = baseAccuracy
          + attacker.tracking
          - target.evasion
          - distancePenalty
          - target.stealthPenalty
clamp 5%..95%
```

Server roll:

```go
func RollHit(rng RNG, hitChance float64) bool {
	if hitChance < 0.05 {
		hitChance = 0.05
	}
	if hitChance > 0.95 {
		hitChance = 0.95
	}
	return rng.Float64() <= hitChance
}
```

RNG seed:

- server-side
- event-specific
- not predictable by client

## Damage Formula v0

Simple:

```text
raw = weapon_damage
penetratedShield = raw * penetration
shieldDamage = raw - penetratedShield
hpDamage = penetratedShield

shield absorbs shieldDamage
overflow goes to HP
resistance applies by damage type
crit may multiply raw
```

Go-ish:

```go
func CalculateDamage(attacker EffectiveStats, target EffectiveStats, weapon WeaponStats, roll DamageRoll) DamageResult {
	raw := weapon.Damage
	if roll.Crit {
		raw *= attacker.CritMultiplier
	}

	raw *= 1 - target.ResistanceFor(weapon.DamageType)

	penetration := Clamp(attacker.Penetration+weapon.Penetration, 0, 0.8)
	hpDirect := raw * penetration
	shieldPart := raw - hpDirect

	return DamageResult{
		ShieldDamage: shieldPart,
		HPDamage:     hpDirect,
	}
}
```

Apply:

```go
func ApplyDamage(target *CombatActor, dmg DamageResult) AppliedDamage {
	shieldBefore := target.Shield
	shieldAbsorbed := Min(target.Shield, dmg.ShieldDamage)
	target.Shield -= shieldAbsorbed

	overflow := dmg.ShieldDamage - shieldAbsorbed
	totalHP := dmg.HPDamage + overflow
	target.HP -= totalHP
	if target.HP < 0 {
		target.HP = 0
	}

	return AppliedDamage{Shield: shieldAbsorbed, HP: totalHP, ShieldBefore: shieldBefore}
}
```

## Contribution And Loot Rights

NPC damage contribution:

```text
entity_id -> total_damage
entity_id -> last_damage_at
```

When NPC dies:

```text
valid contributors = damage within recent window and within participation rules
primary owner = highest valid contribution or first tag depending design
LootService creates owner-locked drop for primary owner/party
PlayerProgressionService grants combat XP
QuestService progresses kill quests
```

MVP:

```text
highest damage contributor gets loot owner lock
party sharing later
```

## PvP Hooks

PvP later can reuse same flow.

Additional checks:

- zone PvP rules
- faction/clan relation
- safe zone protection
- recent aggression flag
- cargo death rules
- reputation/bounty

## Events Emitted

```text
combat.target_set
combat.attack_started
combat.attack_resolved
combat.attack_missed
combat.damage_applied
combat.energy_spent
combat.cooldown_started
combat.entity_killed
combat.npc_killed
combat.player_killed
```

## Edge Cases

- Target leaves range between client click and server tick.
- Target enters fog/stealth before attack resolves.
- Attacker dies before projectile lands.
- Skill cooldown starts but damage fails; decide if cooldown consumed.
- Energy exactly equals cost; allow.
- Shield reaches 0 and overflow HP damage applies.
- NPC dies from two simultaneous attacks; death only processed once.
- Reconnect should not reset cooldowns.

## Abuse Vectors

### Range Spoofing

Risk:

- Client sends attack from impossible distance.

Defense:

- Server position truth
- Server range check
- Movement speed validation

### Hidden Target Attack

Risk:

- Client remembers entity id after it leaves radar/fog.

Defense:

- `Visibility.CanSee` required at attack time.
- AOI filter not enough; detection state matters.

### Cooldown Skipping

Risk:

- Client sends rapid skill requests.

Defense:

- Server cooldown map
- server time only
- rate limit noisy commands

### Energy Desync

Risk:

- Client displays enough energy but server state differs.

Defense:

- server rejects
- client reconciles from authoritative energy snapshot

### Kill Credit Farming

Risk:

- Player taps NPC once and gets full reward.

Defense:

- minimum contribution threshold
- recent damage window
- anti-leech distance check

## Testing Checklist

- Attack hidden target fails.
- Attack out of range fails.
- Cooldown prevents double attack.
- Energy shortage prevents attack.
- Damage formula shield overflow works.
- Simultaneous lethal damage processes death once.
- Contribution owner selected correctly.
- Reconnect does not reset cooldown.
- Client timestamp ignored.

## Implementation Notes

MVP combat:

- Targeted basic laser
- 1 manual rocket skill
- Energy cost
- Cooldown
- Shield/HP
- NPC death
- Loot owner = highest damage

Skillshots, projectiles with travel time, advanced line-of-sight and PvP rules can come later.
