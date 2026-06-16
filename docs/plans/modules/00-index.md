# Module Specs Index

Date: 2026-06-17

Bu klasör, büyük progression/economy/world dokümanlarını kodlanabilir modül spec'lerine böler.

Amaç:

```text
Her modül tek başına okunabilsin.
Her modül için server ownership net olsun.
Kod yazarken "bu kural nerede konuşulmuştu?" sorusu azalısın.
Bug, exploit ve abuse riskleri baştan görülsün.
```

## Reading Order

Önerilen okuma ve kodlama sırası:

1. `01-player-progression-rank-role-skills.md`
2. `02-inventory-cargo-wallet-ledger.md`
3. `03-ship-hangar-loadout.md`
4. `04-module-stat-aggregation.md`
5. `05-combat-damage-targeting.md`
6. `06-loot-drop-ownership.md`
7. `07-death-repair-respawn.md`
8. `08-crafting-recipes-materials.md`
9. `09-market-auction-premium.md`
10. `10-quest-board-generation.md`
11. `11-planet-production-offline-settlement.md`
12. `12-automation-routes.md`
13. `13-intel-coordinate-trading.md`
14. `14-world-aoi-fog-security.md`
15. `15-api-events-errors.md`
16. `16-testing-observability-balancing.md`

## Service To File Map

```text
PlayerProgressionService
RankService
RoleXpService
PilotSkillTreeService
-> 01-player-progression-rank-role-skills.md

InventoryService
CargoService
WalletService
TransactionLedgerService
-> 02-inventory-cargo-wallet-ledger.md

ShipService
HangarService
LoadoutService
-> 03-ship-hangar-loadout.md

ModuleService
StatAggregationService
-> 04-module-stat-aggregation.md

CombatService
TargetingService
DamageService
EnergyService
CooldownService
-> 05-combat-damage-targeting.md

LootService
DropOwnershipService
-> 06-loot-drop-ownership.md

DeathService
RepairService
RespawnService
-> 07-death-repair-respawn.md

CraftingService
RecipeService
MaterialService
-> 08-crafting-recipes-materials.md

MarketService
AuctionService
PremiumEntitlementService
-> 09-market-auction-premium.md

QuestService
QuestGenerationService
QuestRewardService
-> 10-quest-board-generation.md

PlanetProductionService
OfflineSettlementService
-> 11-planet-production-offline-settlement.md

AutomationRouteService
RouteSettlementService
-> 12-automation-routes.md

IntelItemService
CoordinateTradeService
ShareService
-> 13-intel-coordinate-trading.md

AOIService
FogOfWarService
VisibilityService
ScannerVisibilityBridge
-> 14-world-aoi-fog-security.md

ApiGateway
RealtimeProtocol
EventBusContracts
ErrorModel
-> 15-api-events-errors.md

TestingStrategy
Observability
EconomyBalancing
SecurityReviewChecklist
-> 16-testing-observability-balancing.md
```

## Common Module Format

Her dosya mümkün olduğunca şu sırayı takip eder:

- Purpose
- Owns / Does Not Own
- Main Data
- Commands
- Queries
- Server Logic
- Events
- Edge Cases
- Abuse Vectors
- Testing Checklist
- Implementation Notes

## Security Language

Bu dosyalardaki "abuse vector" bölümleri saldırı öğretmek için değil, kendi oyunumuzun server-authoritative savunmasını tasarlamak içindir.

Kural:

```text
Client intent gönderir.
Server validate eder.
State değişimini sadece server yapar.
Economy/value transferleri ledger'a yazılır.
```

## Cross-Cutting Rules

Her modül için geçerli temel kurallar:

- Client hiçbir zaman authoritative değildir.
- Client'tan gelen entity id, amount, coordinate, timestamp, damage, XP, reward, price ve completion bilgisi güvenilmezdir.
- Her command idempotency key veya request id taşımalıdır.
- Ekonomi state'i değiştiren her işlem transaction içinde yapılmalıdır.
- Item/currency hareketleri ledger'a yazılmalıdır.
- Realtime broadcast DB commit'ten sonra yapılmalıdır.
- Redis cache truth değil, acceleration layer'dır.
- Bir modül başka modülün tablosunu doğrudan mutate etmemeli; servis veya domain command kullanmalıdır.
- Player-visible error mesajları kısa, internal log mesajları detaylı olmalıdır.
- Security-sensitive event'lerde player id, session id, IP/device fingerprint, world id ve reference id loglanmalıdır.

## MVP Boundary

İlk sürümde her modülün tam fantezisini değil, oynanabilir loop'u ayağa kaldıran minimumunu kodlamak doğru.

MVP için hedef loop:

```text
login
spawn starter ship
move/fight
kill NPC
loot raw materials
gain XP/rank
equip modules
craft basic module
discover/claim planet
produce resources
route resources
sell/buy on market
repair after death
```

## Later Expansion Boundary

MVP sonrası:

- Physical convoy routes
- Player-built stations
- Clan market
- Insurance
- Module enhancement/reroll
- Advanced rarity affixes
- PvP bounty
- Territory wars
- Event-specific live ops layers
- Advanced fraud automation

## Source Documents

Ana referanslar:

- `docs/2026-06-17-world-system-design.md`
- `docs/2026-06-17-progression-economy-systems-design.md`
- `docs/2026-06-16-space-morpg-architecture-notes.md`

