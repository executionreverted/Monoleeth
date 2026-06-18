# Player Progression, Rank, Role XP, And Pilot Skills

Date: 2026-06-17

## Purpose

Bu modül oyuncunun uzun vadeli büyümesini yönetir.

Kapsam:

- Main level
- Main XP
- Rank
- Rank milestone checks
- Role levels
- Role XP
- Pilot passive skill points
- Pilot passive tree unlocks

Bu modül geminin statını doğrudan değiştirmez. Stat etkilerini `StatAggregationService` tüketir.

## Owns

```text
PlayerProgressionService
RankService
RoleXpService
PilotSkillTreeService
```

Bu servisler şunları sahiplenir:

- XP grant validation
- Main level calculation
- Role level calculation
- Rank eligibility
- Rank up execution
- Passive skill point grants
- Skill node unlock validation
- Respec cost and execution

## Does Not Own

- Combat damage calculation
- Loot drop creation
- Quest reward generation
- Ship equip validation
- Module stat calculation
- Planet ownership
- Currency movement implementation

Bu modül reward almak için diğer modüllerden event alır. Ama ödülün kaynağını kendi uydurmaz.

Örnek:

```text
CombatService -> emits combat.npc_killed
QuestService -> emits quest.reward_claimed
CraftingService -> emits craft.job_completed
PlayerProgressionService -> grants XP from validated events
```

## Core Data

```text
players
- id
- main_level
- main_xp
- rank
- created_at
- updated_at

player_role_levels
- player_id
- role_type
- level
- xp
- updated_at

player_skill_points
- player_id
- total_points
- spent_points

player_skill_nodes
- player_id
- node_id
- unlocked_at

player_rank_history
- id
- player_id
- old_rank
- new_rank
- reason
- created_at
```

## XP Source Types

```text
combat
quest
loot
scan
craft
construction
route
event
admin_adjustment
```

Her XP grant bir source taşır:

```text
XPGrant{
  PlayerID
  SourceType
  SourceID
  MainXP
  RoleXP[]
  Reason
  IdempotencyKey
  ServerAuthority
}
```

`ServerAuthority` is not a client payload. It is supplied by the owning server
domain boundary that verified source completion. The progression service rejects
XP grants unless the authority matches the source family:

```text
combat -> CombatService
quest -> QuestService
loot -> LootService
scan -> ScannerService
craft -> CraftingService
construction -> PlanetProductionService
route -> AutomationRouteService
event -> EventService
admin_adjustment -> AdminService
```

This prevents a generic client request from inventing `source_type`,
`source_id`, or XP amounts and routing them straight into progression.

## Commands

```text
GrantXP(player_id, source_type, source_id, main_xp, role_xp[], idempotency_key)
TryRankUp(player_id)
UnlockPilotSkill(player_id, node_id)
RespecPilotSkills(player_id, payment_method)
GetProgressionSnapshot(player_id)
```

## Main Level Logic

Main level deterministic bir XP table ile hesaplanmalı.

Örnek:

```go
func LevelForXP(totalXP int64, table []LevelRow) int {
	level := 1
	for _, row := range table {
		if totalXP >= row.RequiredXP {
			level = row.Level
			continue
		}
		break
	}
	return level
}
```

MVP'de DB'de `main_level` saklanabilir ama source of truth `main_xp` olmalı.

Level up sırasında:

```text
old_level = player.main_level
new_level = LevelForXP(player.main_xp + grant)
if new_level > old_level:
  emit player.level_up
```

## Role XP Logic

Role XP main XP'den ayrı akar.

Örnek mapping:

```text
NPC kill:
- main_xp +100
- combat_xp +100

Successful planet scan:
- main_xp +60
- scout_xp +100

Craft job complete:
- main_xp +40
- crafting_xp +100
```

Go-ish:

```go
type RoleXP struct {
	Role   string
	Amount int64
}

func GrantRoleXP(state RoleState, grant RoleXP, table XPTable) RoleState {
	state.XP += grant.Amount
	state.Level = table.LevelForXP(state.XP)
	return state
}
```

## Rank Logic

Rank, sadece XP barı olmamalı. Milestone progression olarak çalışmalı.

Örnek requirement:

```text
Rank 4 requires:
- main_level >= 18
- combat_level >= 3 OR scout_level >= 3
- completed_questline = "starter_frontier"
- crafted_any_tier2_module = true
```

Implementation:

```go
func CanRankUp(ctx context.Context, player PlayerID, nextRank int) (bool, []string, error) {
	req := rankCatalog.Requirements(nextRank)
	state := progressionRepo.LoadRankCheckState(ctx, player)

	missing := make([]string, 0)
	for _, rule := range req.Rules {
		if !rule.IsMet(state) {
			missing = append(missing, rule.Code)
		}
	}
	return len(missing) == 0, missing, nil
}
```

Rank up transaction:

```text
BEGIN
  lock player row
  calculate next rank
  validate requirements
  update player rank
  insert rank history
  increment total skill points by 1
  insert ledger/event
COMMIT
emit player.rank_up
emit pilot.skill_point_granted
```

## Pilot Skill Tree

Skill tree pasif bonus verir.

Node data:

```text
node_id
branch
rank_requirement
role_requirement
prerequisite_nodes[]
cost_points
effects_json
max_rank
```

Unlock validation:

```go
func CanUnlockNode(player SkillState, node SkillNode) error {
	if player.AvailablePoints() < node.CostPoints {
		return ErrNotEnoughSkillPoints
	}
	if player.Rank < node.RankRequirement {
		return ErrRankTooLow
	}
	for _, prereq := range node.Prerequisites {
		if !player.HasNode(prereq) {
			return ErrMissingPrerequisite
		}
	}
	return nil
}
```

Effects example:

```json
{
  "stats": [
    {"stat": "laser_energy_cost_mult", "op": "mul", "value": 0.98},
    {"stat": "scan_success_bonus", "op": "add", "value": 0.02}
  ]
}
```

## Respec

Respec kuralları:

- Combat içinde yapılamaz.
- Station/hangar/planet safe area ister.
- Credits ile yapılabilir.
- Premium ile yapılabilir ama tek seçenek olmamalı.
- Respec sonrası stat snapshot invalid edilir.

Transaction:

```text
charge cost
delete player_skill_nodes
set spent_points = 0
emit pilot.skills_respecced
emit player.stats_invalidated
```

## Events Consumed

```text
combat.npc_killed
loot.picked_up
scan.planet_discovered
craft.job_completed
building.completed
quest.reward_claimed
route.delivery_completed
admin.xp_adjusted
```

## Events Emitted

```text
player.xp_gained
player.level_up
player.role_xp_gained
player.role_level_up
player.rank_up
pilot.skill_point_granted
pilot.skill_unlocked
pilot.skills_respecced
player.stats_invalidated
```

## Edge Cases

- Aynı kill event'i iki kez XP vermemeli.
- Quest reward claim retry olursa XP duplicate olmamalı.
- Rank up sırasında aynı anda iki request gelirse iki skill point vermemeli.
- Respec sırasında başka request skill unlock yapamamalı.
- Role XP negative adjustment desteklenecekse level düşüşü nasıl olacak net olmalı.
- Admin XP grant ayrı reason ile ledger/event'e yazılmalı.

## Abuse Vectors

Bu bölüm savunma içindir.

### Duplicate XP Claims

Risk:

- Client aynı quest claim request'ini tekrar yollar.
- Reconnect sırasında aynı event yeniden process edilir.

Defense:

- `idempotency_key`
- source event unique index
- reward claim state machine

### Low-Level Farm Botting

Risk:

- Oyuncu aynı düşük seviye NPC'yi sonsuz keser.

Defense:

- XP scaling by level difference
- daily soft diminishing returns
- activity pattern telemetry
- server-side movement/combat plausibility checks

### Skill Tree Client Tampering

Risk:

- Client locked node unlock request gönderir.

Defense:

- Server-only catalog validation
- prerequisite check
- rank/role check
- points check in transaction

### Rank Milestone Spoofing

Risk:

- Client "I completed requirement" der.

Defense:

- Requirements DB/event state'ten okunur.
- Client milestone completion gönderemez.

## Testing Checklist

- XP grant idempotent mi?
- Main level XP threshold testleri var mı?
- Role level threshold testleri var mı?
- Rank up double-click tek rank mı veriyor?
- Skill unlock prerequisite testleri var mı?
- Respec sonrası stat cache invalid oluyor mu?
- Quest reward XP duplicate olmuyor mu?
- Low-level XP scaling doğru mu?
- Admin adjustment audit log yazıyor mu?

## Implementation Notes

İlk kodlanacak minimum:

```text
GrantXP
RoleXP update
TryRankUp with simple requirement table
SkillPoint grant on rank up
UnlockPilotSkill
Stat invalidation event
```

MVP'de skill tree küçük tutulmalı:

```text
3 branches
5-7 node per branch
no active skills
passive-only
```
