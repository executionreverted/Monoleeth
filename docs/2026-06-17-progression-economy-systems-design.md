# Progression and Economy Systems Design

Date: 2026-06-17

Language note: This document is intentionally written in Turkish because it is a working design note for us. Code-facing names are kept in English so the ideas can later become modules, services, tables, and events without losing meaning.

## Purpose

Bu doküman oyunun progression, gemi, modül, loot, craft, market, premium, ölüm, quest ve üretim otomasyonu sistemlerini detaylandırır.

Amaç şu:

```text
Kodu yazmaya başladığımızda her sistemi ayrı modül gibi ele alabilelim.
Hiçbir kritik oyun kuralı konuşmada kaybolmasın.
Ekonomi, progression ve server-authoritative kurallar baştan net olsun.
```

Bu doküman world system dokümanının üstüne oturur. World system tarafı infinite space, procedural universe, fog of war, planet discovery ve colonization gibi konuları tanımlar. Bu doküman ise oyuncunun güçlenmesini, item ekonomisini, gemi yaşam döngüsünü ve planet üretimini tanımlar.

## Core Fantasy

Oyuncu küçük bir gemiyle `0,0` yakınında başlar.

Başta yavaş, fakir ve zayıftır. Uzaya açılır, loot toplar, NPC keser, görev yapar, gezegen tarar, craft yapar, marketten alım satım yapar, gemisini ve modüllerini geliştirir.

Zamanla:

- Daha iyi gemiler açar.
- Daha yüksek tier modüller takar.
- Rank kazanır.
- Pilot pasifleriyle aynı gemiyi daha iyi kullanır.
- Daha derin uzayda daha yüksek level planetler kolonize eder.
- Planet üretim ağı kurar.
- Resource rotaları ve wormhole ağıyla kendi galactic network'ünü büyütür.
- Koordinat, intel, material, modül ve premium currency ekonomisinin parçası olur.

Core loop:

```text
Fly -> Fight/Gather/Scan -> Loot -> Craft/Sell -> Upgrade -> Colonize -> Produce -> Expand
```

Long-term loop:

```text
Explore -> Discover -> Claim -> Build -> Automate -> Trade -> Defend -> Push Deeper
```

## Core Decisions

- Server authoritative olacak.
- Client hiçbir zaman kendi başına damage, hit, loot, drop, craft, production veya ownership kararı vermez.
- Oyuncunun gücü üç ana kaynaktan gelir:
  - Ship chassis
  - Equipped modules
  - Rank/pilot passive skills
- Main level ve role level ayrı olacak.
- Rank yükseldikçe pilot passive skill point gelir.
- Yüksek rank, aynı gemiyi daha efektif kullanabilmek anlamına gelir.
- Gemiler doğrudan player-to-player trade edilemez.
- Gemiler craft, sistem marketi veya intergalactic auction ile elde edilir.
- Modüller, hammaddeler, processed materials ve premium currency büyük ölçüde trade edilebilir.
- Free elde edilmiş premium currency ile paid premium currency ayrı takip edilir.
- Crafting ekonomisini bozmamak için drop'lardan çoğunlukla hammadde düşer.
- Ölümde cargo'nun random `50%-100%` arası düşer.
- Gemi disable/destroyed hale gelir ve repair ister.
- En düşük starter gemi her zaman erişilebilir kalır.
- Planet production online ve offline çalışır.
- Offline production login/inspection anında server tarafından hesaplanır.
- Automation route'ları MVP'de sanal/resource-flow olarak çalışır.
- Uzun vadede route/convoy detect edilirse saldırılabilir hale gelebilir.

## Vocabulary

### Player

Hesap sahibi oyuncu.

Server tarafında player:

- Main level
- Rank
- Role levels
- Inventory
- Wallets
- Hangar
- Active ship
- Loadouts
- Planet ownership
- Known intel
- Quests
- Market orders
- Automation routes

gibi persistent state'e sahiptir.

### Main Level

Oyuncunun genel progression seviyesi.

Main level, oyunun bütün sistemlerinden gelen XP ile artabilir. Combat, quest, loot, scan, craft, construction gibi kaynaklardan gelen XP ana seviyeye katkı verebilir.

Main level oyuncunun genel hesabının büyüdüğünü gösterir, ama tek başına bütün gücü belirlemez.

### Rank

Rank, oyuncunun kalıcı güç ve erişim seviyesidir.

Rank şunları etkiler:

- Hangi level planetleri kolonize edebilir.
- Hangi tier gemileri efektif kullanabilir.
- Hangi tier modülleri tam verimle kullanabilir.
- Pilot passive skill point kazanımı.
- Belirli quest, board, market, auction veya feature unlockları.

Rank yükselince oyuncuya 1 pilot passive skill point gelir.

### Role Level

Oyuncunun yaptığı işe göre gelişen uzmanlık seviyeleri.

Önerilen role level'lar:

- Combat
- Scout
- Crafting
- Construction
- Trade/Hauling

Role level'lar oyunun holy-trinity hissini ve uzun vadeli uzmanlaşmayı destekler.

### Ship

Oyuncunun aktif kullandığı gemi chassis'i.

Ship doğrudan güç değil, bir platformdur:

- Base HP
- Base shield
- Base energy
- Movement speed
- Cargo capacity
- Radar baseline
- Module slot count
- Slot type distribution
- Passive chassis bonus
- Repair cost multiplier

gibi değerler taşır.

### Module

Gemiye takılan güç parçası.

Module üç ana tiptedir:

- Offensive
- Defensive
- Utility

Modüller geminin gerçek performansını belirleyen ana item'lardır.

### Loadout

Bir gemi üstündeki kayıtlı modül dizilimi.

Her geminin kendi loadout save'i olabilir.

Örnek:

```text
Slazar PvE Loadout
Slazar Scan Loadout
Slazar Cargo Run Loadout
```

### Inventory

Oyuncunun taşıdığı item'lar.

Inventory, ship cargo ve account storage ile karıştırılmamalıdır.

Önerilen ayrım:

- Ship cargo: Uzayda geminin üstünde taşınan ve ölümde düşebilen malzemeler.
- Account inventory: Kalıcı item deposu.
- Planet storage: Planet üzerinde duran local storage.
- Global storage: Belirli station/checkpoint/storage feature'ları ile erişilen merkezi depo.

### Cargo

Aktif geminin üstündeki taşınabilir mallar.

Cargo risklidir. Oyuncu ölürse cargo'nun random `50%-100%` arası düşebilir.

### Wallet

Oyuncunun para birimleri.

Önerilen wallet tipleri:

- Credits
- Premium currency paid
- Premium currency earned/free
- Reputation tokens
- Event tokens

Paid/free ayrımı premium currency'nin markette trade edilebilirliğini kontrol etmek için önemli.

### X Core

Şimdilik placeholder isim.

Planet kolonize etmek için gereken rare item.

X Core:

- Claim spam'i engeller.
- Planet ownership kararını anlamlı hale getirir.
- Quest, rare drop, weekly premium purchase, auction veya event reward olarak gelebilir.
- Ekonominin en kritik long-term scarce resource'larından biri olabilir.

## System Map

Kod tarafında sistemleri kabaca şöyle bölebiliriz:

```text
PlayerProgressionService
RankService
RoleXpService
PilotSkillTreeService

ShipService
HangarService
LoadoutService
ModuleService
StatAggregationService

InventoryService
CargoService
WalletService
TransactionLedgerService

LootService
DropOwnershipService
DeathService
RepairService

CraftingService
RecipeService
MaterialService

MarketService
AuctionService
PremiumEntitlementService

QuestService
QuestGenerationService
QuestRewardService

PlanetProductionService
AutomationRouteService
OfflineSettlementService

IntelItemService
CoordinateTradeService
```

Bu servislerin çoğu birbirine bağlıdır ama ownership net kalmalı.

Örnek:

- `DeathService` cargo drop kararı verir ama item transferi için `CargoService` ve `LootService` kullanır.
- `CraftingService` recipe validate eder ama material düşmeyi `InventoryService` veya `PlanetStorageService` üzerinden yapar.
- `RankService` rank up yapar ama passive point grant etmek için `PilotSkillTreeService` event'i üretir.
- `PlanetProductionService` üretim hesaplar ama item eklemeyi storage servislerine bırakır.

## Player Progression

### Progression Goals

Progression sadece "level yükseldi, damage arttı" olmamalı.

Oyuncu şunu hissetmeli:

- Daha iyi gemileri açıyorum.
- Aynı gemiyi daha verimli kullanıyorum.
- Daha güçlü modülleri takabiliyorum.
- Daha derin uzayda hayatta kalabiliyorum.
- Daha yüksek level planetler kolonize edebiliyorum.
- Üretim ağım büyüyor.
- Daha değerli market aktivitelerine girebiliyorum.
- Riskli bölgelere girmek artık mantıklı hale geliyor.

### Main Level XP Sources

Main XP kaynakları:

- NPC combat
- PvP reward events, ileride
- Quest completion
- Loot pickup
- Successful planet scans
- Anomaly discovery
- Craft completion
- Construction/building completion
- Trade contract completion, ileride
- Event participation

Main level oyuncuyu genel olarak ödüllendirir.

Ancak her XP kaynağı aynı ağırlıkta olmamalı.

Örnek ağırlıklar:

```text
combat_xp -> main_xp 100%
quest_xp -> main_xp 100%
scan_xp -> main_xp 60%, scout_xp 100%
craft_xp -> main_xp 40%, crafting_xp 100%
construction_xp -> main_xp 40%, construction_xp 100%
loot_xp -> main_xp 20%
```

Bu rakamlar kesin değil, tuning knob.

### Rank Progression

Rank, main level'dan tamamen kopuk olmamalı ama birebir aynı da olmamalı.

Önerilen model:

```text
main level = general activity progression
rank = milestone progression
```

Rank up için sadece XP değil, milestone şartları da olabilir.

Örnek:

```text
Rank 2:
- Main level 5
- Complete starter combat questline
- Discover 1 planet signal

Rank 3:
- Main level 12
- Craft first Tier 2 module
- Earn 500 scout or combat role XP

Rank 5:
- Main level 30
- Colonize first planet
- Complete 3 board contracts
- Own at least 1 X Core consumed claim history
```

Bu model rank'i daha anlamlı yapar. Oyuncu sadece aynı mob'u keserek bütün oyunu bypass edemez.

### Rank Rewards

Her rank şu ödüllerden bazılarını verebilir:

- 1 pilot passive skill point
- Higher planet colonization cap
- Higher module effective tier cap
- New ship craft eligibility
- New board quest difficulty
- New auction tier visibility
- New production building tier
- New scanner/radar tier
- Extra loadout slot unlock, nadir
- Cosmetic title/badge

### Rank and Planet Colonization

Planet level ile rank ilişkisi net olmalı.

Basit kural:

```text
player_rank >= planet_level_required
```

Örnek:

```text
Level 3 planet -> Rank 3+
Level 7 planet -> Rank 7+
Level 12 planet -> Rank 12+
```

Daha yumuşak model:

```text
player_rank + colonization_bonus >= planet_level_required
```

Buradaki `colonization_bonus` özel module, guild buff, event item veya quest reward ile gelebilir.

İlk sürüm için basit kural daha iyi.

### Anti-Grind Abuse

Progression kaynakları sınırsız farm edilebilir ama tek kaynağa yüklenince verim düşebilir.

Önerilen anti-abuse araçları:

- Daily soft caps, hard cap değil
- Diminishing returns on same low-level NPC
- Quest reward scaling
- Deep-space risk/reward scaling
- Role XP source validation
- Bot davranışı için server-side pattern detection

Oyuncu grind edebilmeli, ama en verimli yol farklı sistemlere dokunmak olmalı.

## Role Level System

### Role Level Philosophy

Role level oyuncuya "ben bu oyunda ne yapıyorum?" sorusunun cevabını verir.

Her oyuncu savaşabilir, craft yapabilir, scan atabilir. Ama çok yapan oyuncu o rolde daha iyi olur.

### Combat Role

XP kaynakları:

- NPC kill
- Combat quest
- Elite/boss kill participation
- PvP participation, ileride

Bonus örnekleri:

- Small laser damage bonus
- Shield recharge under combat bonus
- NPC damage mitigation
- Weapon heat/energy efficiency
- Critical chance veya penetration küçük artışları

Combat role hiçbir zaman tek başına full power kaynağı olmamalı. Gemi ve modül hâlâ ana güç kaynağı olmalı.

### Scout Role

XP kaynakları:

- Successful planet scan
- Anomaly discovery
- New biome discovery
- Rare signal identification
- Sharing verified intel, sınırlı

Bonus örnekleri:

- Scan pulse chance bonus
- Scanner energy cost reduction
- Radar range bonus
- Better signal classification
- Lower false-positive chance
- Faster scan cycle

Scout role coordinate economy'yi güçlendirir.

### Crafting Role

XP kaynakları:

- Craft jobs
- Refining
- First-time recipe completions
- Rare craft success

Bonus örnekleri:

- Craft time reduction
- Material efficiency, küçük yüzde
- Extra chance for durability bonus
- Reduced crafting fee
- Access to advanced recipes

Crafting role direkt combat damage vermemeli. Ekonomi uzmanlığı vermeli.

### Construction Role

XP kaynakları:

- Planet building construction
- Building upgrade
- Route infrastructure
- Storage expansion
- Wormhole/relay support systems

Bonus örnekleri:

- Building time reduction
- Planet production efficiency
- Route loss reduction
- Storage capacity bonus
- Building upkeep reduction

Construction role planet network sahibi oyuncular için değerli olur.

### Trade/Hauling Role

Bu role MVP sonrası gelebilir.

XP kaynakları:

- Profitable market sale, abuse korumalı
- Contract delivery
- Route throughput
- Long-distance cargo delivery
- Auction participation, sınırlı

Bonus örnekleri:

- Cargo capacity bonus
- Route loss reduction
- Market fee reduction
- Better route UI/intel
- Insurance discount

Market manipulation abuse ihtimali olduğu için dikkatli tasarlanmalı.

## Pilot Passive Skill Tree

### Skill Point Source

Her rank up:

```text
+1 pilot passive skill point
```

Skill point account-level olabilir. Yani oyuncu farklı gemilere geçse bile pilot bilgisi kalır.

### Skill Tree Purpose

Pilot passive tree oyuncuya build identity verir.

Örnek build yönleri:

- Laser specialist
- Shield tank
- Fast scout
- Cargo runner
- Planet colonizer
- Energy efficiency pilot
- Support/aura pilot
- Stealth/jammer pilot

### Skill Tree Rules

Önerilen kurallar:

- Pasifler küçük ama anlamlı bonus verir.
- Tek pasif oyunu kırmamalı.
- Build path kararları olmalı.
- Respec mümkün olmalı ama bedelli olmalı.
- Premium respec olabilir ama credits ile de yapılabilmeli.
- PvP'de mandatory tek meta yaratmamalı.

### Example Passive Nodes

Laser branch:

```text
Efficient Emitters I
- Laser energy cost -2%

Focused Beam I
- Laser penetration +1%

Overheat Discipline
- Manual laser skill cooldown -3%
```

Shield branch:

```text
Shield Cycling I
- Shield regen +2%

Emergency Buffer
- When shield breaks, gain small temporary mitigation

Heavy Capacitors
- Max shield +3%, movement speed -1%
```

Scout branch:

```text
Signal Tuning I
- Scan success chance +2%

Quiet Engines
- Signature radius -2%

Wide Sweep
- Scan radius +3%, scan energy cost +2%
```

Hauler branch:

```text
Cargo Packing I
- Cargo capacity +3%

Route Discipline
- Automation route loss chance -1%

Emergency Dump Protocol
- Death cargo drop minimum reduced slightly, PvE only maybe
```

Support branch:

```text
Energy Aura I
- Nearby party members gain tiny energy regen bonus

Relay Pilot
- Ally scan sharing range + small bonus

Shield Harmonizer
- Support modules gain efficiency
```

### Skill Tree Server Validation

Client sadece seçimi gönderir:

```text
unlock_skill_node(node_id)
```

Server validate eder:

- Player has available skill point
- Node exists
- Prerequisites met
- Rank requirement met
- Not already unlocked
- Branch constraints met
- Respec lock/cost valid

Server sonra state'i yazar ve stat aggregation event'i üretir.

## Ship System

### Ship Philosophy

Gemi oyuncunun karakter sınıfı gibi hissettirmeli.

DarkOrbit hissinde olduğu gibi:

- Tek aktif gemi var.
- Hangarda farklı gemiler var.
- Gemiler farklı slot sayıları, statlar ve roller sunar.
- Oyuncu duruma göre gemi swap eder.
- Gemi patlarsa repair edilene kadar başka gemiye geçebilir.

### Ship Acquisition

Gemiler şu yollarla elde edilir:

- Craft
- Credit purchase, system shop
- Premium purchase, weekly stock sınırlı
- Intergalactic Auction Hall
- Event reward, nadir
- Quest unlock, nadir/soulbound olabilir

Player-to-player gemi trade yok.

Bu karar önemli çünkü:

- Gemi progression'ı account achievement gibi kalır.
- Market direkt gemi flipping ekonomisine dönmez.
- Modül ve material ekonomisi daha önemli kalır.
- Premium shop/auction stock kontrollü olur.

### One Unlock Per Ship Type

Oyuncu aynı ship type'tan bir tane unlock edebilir.

Örnek:

```text
Slazar unlocked = true
```

Aynı Slazar'dan 5 tane alıp depolayamaz.

Bu model:

- Insurance/repair ekonomisini basitleştirir.
- Gemiyi item stack gibi manipüle etmeyi engeller.
- Hangarı "owned chassis list" yapar.

### Starter Ship Safety

En düşük seviye starter gemi her zaman erişilebilir olmalı.

Ölüm, fakirlik veya yanlış karar oyuncuyu oyundan kilitlememeli.

Kural:

```text
Player always has access to Rank 1 starter ship.
Starter ship cannot become permanently unavailable.
```

Starter ship kötü olabilir ama oyuncu onunla repair parası kasabilmeli.

### Ship Stats

Her ship definition şu alanlara sahip olabilir:

```text
ship_id
name
tier
rank_requirement
craft_requirement
credit_price
premium_price
auction_buy_now_price
base_hp
base_shield
base_energy
base_energy_regen
base_speed
base_cargo
base_radar
base_signature
repair_cost_multiplier
slot_offensive
slot_defensive
slot_utility
passive_bonus_id
role_tag
```

### Ship Archetypes

Oyunun holy-trinity hissi için gemi tipleri:

#### Fighter

Amaç:

- PvE combat
- PvP burst
- NPC farm

Özellikler:

- Offensive slot fazla
- Cargo düşük
- Shield orta
- Speed orta/yüksek
- Energy tüketimi yüksek

#### Tank/Hauler

Amaç:

- Resource taşıma
- Riskli trade route
- Planet supply

Özellikler:

- Cargo yüksek
- Shield/HP yüksek
- Offensive slot az
- Speed düşük
- Repair cost yüksek olabilir

#### Scout

Amaç:

- Planet keşfi
- Deep-space exploration
- Intel economy

Özellikler:

- Utility slot fazla
- Radar/scanner bonus
- Speed yüksek
- Cargo düşük/orta
- Combat zayıf

#### Support

Amaç:

- Party/clan support
- Energy regen aura
- Shield support
- Scan relay support

Özellikler:

- Utility/defensive slot dengeli
- Offensive düşük/orta
- Aura passive
- Teamplay için değerli

#### Miner/Industrial

Amaç:

- Gather/resource loop
- Planet production support
- Crafting material transport

Özellikler:

- Cargo yüksek
- Utility slot orta/yüksek
- Gathering bonus
- Combat düşük

### Ship Passive Examples

```text
Scout ship:
- Scanner pulse interval -5%
- Radar range +5%

Hauler ship:
- Cargo capacity +25%
- Route transfer fee -3%

Support ship:
- Nearby party energy regen +2%
- Ally shield module efficiency +1%

Fighter ship:
- Laser damage +4%
- Cargo capacity -10%
```

### Ship Effective Use By Rank

Yüksek tier gemi craft etmek tek başına tam güç vermeyebilir.

Önerilen model:

```text
if player_rank < ship_effective_rank:
  ship can be owned maybe
  but some stats are scaled down
```

Ancak ilk sürümde complexity yaratmamak için daha basit yapılabilir:

```text
player_rank >= ship_rank_requirement -> equip allowed
```

Benim önerim MVP için equip gate, ileride effective scaling.

### Ship Swap

Ship swap sadece güvenli yerlerde yapılmalı:

- Hangar
- Owned planet with hangar building
- Station/checkpoint
- Certain safe zones

Combat içinde instant ship swap olmamalı.

Swap validation:

- Not in combat
- Not in unsafe space
- Current ship state allows swap
- Target ship unlocked
- Target ship not disabled
- Cargo transfer rules valid

Cargo konusu önemli:

```text
If current ship has cargo > target cargo capacity:
  swap blocked or require unload
```

### Ship Loadouts

Her ship için birden fazla loadout slotu olabilir.

Loadout slot kaynakları:

- Default 1
- Credits ile unlock
- Premium ile unlock
- Quest reward, nadir

Premium loadout slot satışı pay-to-win değil, convenience olarak kabul edilebilir.

Loadout validation:

- Ship type matches
- Modules owned
- Modules not installed elsewhere unless allowed
- Slot type compatible
- Rank/tier requirements met
- Durability > 0

## Module System

### Module Philosophy

Modüller oyunun build sistemidir.

Gemi platformu verir, modül karakter verir.

Oyuncu şöyle kararlar almalı:

- Daha çok damage mi?
- Daha çok shield mı?
- Daha hızlı scan mi?
- Daha stealth mi?
- Daha çok cargo mu?
- Daha az enerji tüketimi mi?
- PvE mi, PvP mi, exploration mı?

### Module Categories

#### Offensive Modules

Örnekler:

- Laser Gun Alpha
- Laser Gun Beta
- Pulse Laser
- Piercing Beam
- Rocket Launcher
- Plasma Cutter
- EMP Projector

Stat alanları:

```text
damage
damage_type
range
cooldown
energy_cost
accuracy
tracking
penetration
crit_chance
crit_multiplier
heat_generation
ammo_type
```

#### Defensive Modules

Örnekler:

- Shield Generator
- Shield Battery
- Reflector
- Armor Plate
- Energy Barrier
- Thermal Dissipator

Stat alanları:

```text
max_shield
shield_regen
shield_delay
damage_reduction
reflect_percent
resistance_kinetic
resistance_laser
resistance_explosive
energy_reserve
emergency_trigger
```

#### Utility Modules

Örnekler:

- Scanner
- Radar
- Jammer
- Stealth Module
- Cryptography Module
- Cargo Expansion
- Warp Stabilizer
- Mining Beam
- Route Beacon

Stat alanları:

```text
radar_range
scan_power
scan_interval_modifier
signal_classification
jammer_strength
stealth_strength
signature_reduction
cargo_bonus
warp_energy_reduction
hack_power
route_efficiency
```

### Module Tier and Rarity

Modüller iki eksende ayrılabilir:

```text
tier = progression power band
rarity = quality/specialness
```

Örnek:

```text
Laser Gun Alpha T1 Common
Laser Gun Alpha T1 Rare
Laser Gun Beta T3 Common
Laser Gun Beta T3 Epic
```

Tier, base power'ı belirler.

Rarity, yan stat veya küçük bonus verebilir.

MVP için rarity çok karmaşık yapılmayabilir. Önce tier + deterministic recipe daha sağlıklı.

### Module Requirements

Module equip için:

- Ship has compatible slot
- Player rank meets requirement
- Role level meets requirement, bazı modüllerde
- Energy budget valid
- Module durability > 0
- Not soulbound to another player

### Energy Budget

Enerji ana kısıtlayıcı kaynaklardan biri olabilir.

Gemi:

```text
max_energy
energy_regen
```

Modüller:

```text
energy_upkeep
energy_per_use
```

Laser basic attack bile energy harcayabilir.

Bu, Dark Forest'tan gelen "energy production matters" fikrini ship combat'a bağlar.

Örnek:

```text
Laser shot cost = 8 energy
Ship energy regen = 4/sec
If energy < 8:
  cannot fire or delayed until enough energy
```

Bu sayede:

- Energy regen önemli stat olur.
- Shield/weapon/scanner arasında tradeoff oluşur.
- Planet üretimi ve laser ammo/energy cell üretimi combat ekonomisine bağlanabilir.

### Module Durability

Bazı modüllerin durability'si olabilir.

Durability düşme kaynakları:

- Ship death
- High-risk PvP zone death
- Overheat/fail event
- Rare environmental hazard

Ölümde durability düşüş örneği:

```text
for each equipped durable module:
  roll chance
  if success:
    durability -= 1
```

Durability 0 olursa:

- Module unequippable olabilir
- Repair ister
- Statları devre dışı kalır

Durability sistemi çok sert olmamalı. Oyuncuyu sinirlendirir ama ekonomi sink'i için değerli.

### Module Binding

Trade kuralları:

- Crafted normal modules tradeable olabilir.
- Quest unlock modules soulbound olabilir.
- Event special modules account-bound olabilir.
- Equipped module tradeable kalabilir ama unequip cooldown/fee olabilir.
- Damaged modules markette satılabilir ama açıkça durability görünmeli.

### Module Upgrade

MVP'de module upgrade şart değil.

Sonra eklenebilir:

- Enhance level
- Reroll secondary stat
- Fuse duplicate modules
- Calibration

Ama ilk sürüm için craft + tier + durability yeterli.

## Stat Aggregation

### Why It Matters

Oyuncunun gerçek statları çok yerden gelir:

- Base ship
- Equipped modules
- Pilot passive skills
- Role level bonuses
- Temporary buffs
- Planet/station buffs
- Party/support aura
- Debuffs/jammers

Bu yüzden tek bir `StatAggregationService` olmalı.

### Suggested Stat Categories

Core:

```text
hp_max
shield_max
shield_regen
energy_max
energy_regen
speed
cargo_capacity
```

Combat:

```text
weapon_damage
weapon_range
weapon_cooldown
accuracy
tracking
evasion
penetration
crit_chance
crit_multiplier
resist_laser
resist_explosive
resist_kinetic
```

Exploration:

```text
radar_range
scan_power
scan_radius
scan_interval
signal_detection_bonus
signal_classification_bonus
signature_radius
stealth_strength
jammer_strength
```

Economy:

```text
craft_speed
craft_fee_reduction
route_loss_reduction
storage_bonus
production_bonus
market_fee_reduction
```

### Aggregation Order

Önerilen sıra:

```text
base ship stats
+ flat module stats
+ flat passive bonuses
* percentage module modifiers
* percentage passive modifiers
* temporary buffs/debuffs
clamp min/max
```

Önemli:

- Server final statı hesaplar.
- Client sadece display için cached stat alır.
- Combat ve scan server-side cached stat snapshot kullanır.

### Cache Strategy

Statlar her tick baştan hesaplanmamalı.

Recalculate events:

- Ship changed
- Module equipped/unequipped
- Module durability changed to broken
- Skill unlocked/respecced
- Buff/debuff applied/expired
- Role level changed
- Rank changed

Server active session için stat snapshot cache tutabilir.

## Inventory, Cargo, Storage

### Inventory Types

#### Account Inventory

Kalıcı ve güvenli item deposu.

Ölümden etkilenmez.

İçerik:

- Modules
- Crafting materials
- Quest items
- Coordinate scrolls
- Cosmetics
- Consumables

#### Ship Cargo

Aktif geminin üstündeki taşınan mallar.

Ölümde düşebilir.

İçerik:

- Raw materials
- Looted resources
- Transported processed materials
- Quest delivery goods, bazıları

#### Planet Storage

Bir planet üzerinde duran local storage.

Production ve crafting için kritik.

İçerik:

- Planet-produced materials
- Imported materials
- Building inputs
- Route outputs

#### Global Storage

MVP'de sınırlı olabilir.

Global storage varsa her şeyi kolaylaştırır ama logistics gameplay'i zayıflatır.

Öneri:

- Başlangıçta safe station/account storage var.
- Planet crafting için material planet storage'da veya ilgili storage network içinde olmalı.
- Her item her yerden global erişilmemeli.

### Cargo Capacity

Cargo capacity ship statıdır.

Örnek:

```text
current_cargo_weight <= cargo_capacity
```

Item'larda:

```text
weight
stack_size
```

olabilir.

MVP için unit-based capacity daha basit:

```text
1 iron ore = 1 cargo unit
1 rare crystal = 2 cargo units
```

### Storage Capacity

Planet storage ve account storage capacity limitlidir.

Offline production storage capacity'ye takılır.

Kural:

```text
produced_amount = min(calculated_amount, free_storage_capacity)
```

Storage doluysa:

- Production durur
- Route output kaybolmaz, destination doluysa transfer fail olur veya source'ta bekler
- UI oyuncuya uyarı verir

MVP için basit:

```text
if destination storage full:
  route tick produces no transfer
```

## Resources and Materials

### Resource Categories

#### Raw Materials

NPC, loot, gather ve planet extraction kaynaklarından gelir.

Örnek:

- Iron Ore
- Carbon Shards
- Helium Dust
- Crystal Fragment
- Void Salt
- Plasma Residue
- Alien Biomass

#### Rare Raw Materials

Daha düşük drop/scan/gather şanslıdır.

Örnek:

- Dark Matter Thread
- Quantum Core Fragment
- Ancient Alloy Dust
- Nebula Pearl

#### Processed Materials

Planet building veya refinery ile üretilir.

Örnek:

- Refined Alloy
- Laser Lens
- Shield Matrix
- Energy Cell
- Warp Coil
- Scanner Circuit

#### Planet-Specific Materials

Belli planet type veya biome'da üretilir.

Örnek:

- Ice Planet -> Cryo Crystal
- Volcanic Planet -> Magma Alloy
- Gas Giant -> Helium Core
- Dead World -> Relic Dust
- Bio Planet -> Organic Circuit

Bu malzemeler trade ve logistics'i değerli yapar.

#### X Core and X Core Fragments

Planet colonization için ana rare item.

Önerilen kaynaklar:

- Rare quest reward
- Weekly premium purchase limit
- Deep-space anomaly
- Boss/elite event
- Auction lot
- Fragment crafting

X Core direkt çok nadir olabilir. Daha sürdürülebilir model:

```text
10 X Core Fragment + rare processed materials -> 1 X Core
```

### Resource Rarity

Önerilen rarity:

```text
common
uncommon
rare
epic
legendary
event
```

Rarity sadece drop chance değil, kullanım alanını da etkiler.

### Economy Principle

Drop'tan bitmiş güç item'ı az düşmeli.

Ana kaynak:

```text
raw materials -> processed materials -> modules/ships/buildings
```

Böylece crafting ölmez.

## Loot System

### Loot Philosophy

Loot oyuncuya kısa vadeli dopamine verir ama uzun vadeli craft ekonomisini bozmamalı.

Bu yüzden normal drop:

- Raw material
- Rare raw material
- Consumable
- X Core fragment, çok nadir
- Coordinate/intel item, nadir
- Cosmetic shard, event

olmalı.

Bitmiş module drop'u:

- Çok nadir
- Event/boss özel
- Belki broken/damaged haliyle

### Loot Visibility

Drop dünyada entity olarak görünür.

Kural:

```text
Drop is visible to players in AOI/radar visibility.
Drop ownership controls pickup, not visibility.
```

Yani drop herkes tarafından görülebilir ama ilk süre sadece sahibi alabilir.

Fog of war/AOI kuralı hâlâ geçerli:

- Oyuncu drop'un olduğu alanı görmüyorsa client'e gönderilmez.
- Server visible entity filter uygular.

### Drop Ownership Windows

Önerilen state:

```text
owner_locked -> public -> despawned
```

Örnek süreler:

```text
owner_lock_duration = 60 seconds
public_duration = 120 seconds
total_lifetime = 180 seconds
```

Kesin değerler tuning.

### Loot Pickup Rules

Pickup request:

```text
pickup_loot(drop_id)
```

Server validate eder:

- Drop exists
- Player is in pickup range
- Player can see/interact with drop
- Drop not expired
- Ownership window allows pickup
- Cargo/inventory has capacity
- Item is not restricted

Server sonra:

- Drop amount'u item olarak ekler
- Drop entity'yi delete eder veya amount azaltır
- XP/loot event verir
- Nearby clients'e drop_removed broadcast eder

### Loot XP

Etraftaki lootları almak küçük XP verebilir.

Ama abuse önlemek için:

- Loot XP item value ile sınırlı
- Aynı düşük value item'dan günlük diminishing return
- Player-created fake trade/drop pickup XP vermemeli

Kural:

```text
Only server-generated loot can grant loot XP.
```

### Death Cargo Drops

Oyuncu ölünce:

```text
drop_percent = random between 50% and 100%
```

Cargo içinden bu oran world drop'a dönüşür.

Önerilen detay:

- Quest critical non-drop item'lar düşmeyebilir.
- Soulbound item düşmez.
- Cargo'daki normal raw/processed materials düşer.
- Premium currency item olarak taşınmıyorsa düşmez.
- Coordinate scroll taşınıyorsa düşebilir, bu çok heyecanlı olabilir.

## Crafting System

### Crafting Philosophy

Crafting oyunun ana item üretim motorudur.

Drop sadece hammadde verir. Gerçek güç:

```text
materials + recipes + planet production + time + credits -> modules/ships/buildings
```

ile gelir.

### Craftable Categories

Craft edilebilir:

- Offensive modules
- Defensive modules
- Utility modules
- Ship chassis unlock items
- Processed materials
- Building parts
- Wormhole components
- Scanner parts
- Repair kits
- Energy cells/laser ammo
- X Core, fragment model seçilirse

Craft edilemeyen veya sınırlı:

- Premium cosmetics
- Some quest/soulbound unlocks
- Certain event-only modules

### Recipe Definition

Recipe data:

```text
recipe_id
output_item_id
output_amount
input_items[]
required_credits
required_rank
required_role_level
required_station_type
required_planet_type
craft_duration
is_repeatable
is_tradeable_output
```

Input example:

```text
Laser Gun Beta:
- 200 Refined Alloy
- 40 Laser Lens
- 15 Energy Cell
- 2 Rare Crystal
- 25,000 Credits
- Crafting role level 3
- Rank 4
```

### Crafting Location

Crafting her yerden yapılmamalı.

Önerilen kaynak lokasyonları:

- Account/station craft
- Planet craft
- Owned planet building craft
- Special station craft

Planet craft için:

```text
required materials must exist on that planet storage
or connected allowed storage route/network
```

Bu logistics gameplay'i canlı tutar.

### Crafting Job Flow

Flow:

```text
Player selects recipe
Server validates inputs/location/cost
Server reserves or consumes materials
Crafting job created
Timer starts
On completion output item is created
Crafting XP is granted
```

Materials reserve mi consume mu?

MVP için consume on start daha basit.

İptal edilirse:

- Full refund abuse yaratabilir.
- Partial refund daha iyi.

MVP'de cancellation olmayabilir.

### Crafting XP

Crafting XP:

- Recipe tier
- Craft duration
- Material value
- First-time craft bonus

ile hesaplanabilir.

Abuse koruması:

- Çok düşük tier recipe spam diminishing return
- Player-to-player material loop craft XP exploit kontrolü

### Crafting and Premium

Premium hızlandırma olabilir ama dikkatli:

Allowed:

- Queue slot
- Small time saver
- Cosmetic craft skin
- Convenience storage transfer

Riskli:

- Direkt top tier module satın alma
- Unlimited instant craft
- Material creation

Premium time saver oyunu öldürmeyecek limitte olmalı.

## Market System

### Market Philosophy

Market oyunun sosyal ekonomi motorudur.

Neredeyse her şey alınıp satılabilir:

- Raw materials
- Processed materials
- Modules
- Consumables
- Coordinate scrolls/intel packets
- Premium currency, sadece paid/eligible ise
- X Core fragments belki
- X Core belki kontrollü

Trade edilemeyenler:

- Ships
- Quest soulbound items
- Feature unlock quest items
- Some event/account-bound rewards
- Free-earned premium currency, tercihe göre

### Listing Types

MVP:

- Fixed price sell order
- Buy order later

İleride:

- Buy orders
- Bulk orders
- Contract delivery
- Clan market
- Region-specific market

### Market Listing Data

```text
listing_id
seller_player_id
item_id
quantity
unit_price
currency_type
created_at
expires_at
trade_flags
metadata
```

Coordinate scroll için metadata:

```text
planet_id
known_coordinates
intel_confidence
last_verified_at
stale_status
```

### Trade Fees

Market fee economy sink olmalı.

Örnek:

```text
listing_fee = small upfront credits
sale_fee = 3%-8%
```

Trade/Hauling role veya premium subscription varsa küçük fee reduction verilebilir, ama p2w görünmemeli.

### Premium Currency Trading

Premium currency trade edilecekse paid/free ayrımı şart.

Wallet:

```text
premium_paid
premium_earned
```

Trade allowed:

```text
premium_paid -> tradeable
premium_earned -> non-tradeable or limited
```

Neden:

- Quest/event ile verilen premium sonsuz market enflasyonu yaratmasın.
- Chargeback/fraud takibi kolaylaşsın.
- Real-money economy riski kontrol edilsin.

### Market Abuse Controls

Gerekli kontroller:

- Price sanity logs
- Fraud/chargeback lock
- Newly purchased premium trade cooldown
- RMT suspicious transaction detection
- Same IP/device abuse signals
- Market tax
- Listing expiration
- Trade lock for stolen/fraud currency

MVP'de otomasyon basit olabilir ama ledger mutlaka tutulmalı.

## Intergalactic Auction Hall

### Auction Philosophy

Intergalactic Auction Hall world-based server-generated premium/credit sink'tir.

Oyuncular burada sistem tarafından üretilen özel lot'lara teklif verir.

Amaç:

- Kontrollü nadir ship/module erişimi
- Credit sink
- Premium sink
- Weekly excitement
- Stock bittiğinde overpriced buy-now alternatifi

### Ship Acquisition Through Auction

Örnek:

Normal shop:

```text
Tier 3 Ship: Slazar
Credit price: 500,000
Premium price: 300
Premium weekly stock: 100
```

Auction:

```text
Slazar auction lot
buy_now: 350 premium
or bidding credits/premium depending lot type
```

Stock bitmişse oyuncu auction buy-now ile fazla ödeyebilir.

Bu tek "mild p2w-ish" alan olabilir ama:

- Haftalık stock sınırlı.
- Craft alternatifi var.
- Normal oyuncu da farm ile alabilir.
- Premium daha pahalı/convenience şeklinde.

### Auction Lot Types

Örnekler:

- Ship unlock
- Rare module blueprint
- X Core
- X Core fragments
- Rare processed material bundle
- Cosmetic ship skin
- Coordinate intel cache, event
- Building blueprint

### Auction Data

```text
auction_id
world_id
lot_type
item_or_unlock_id
quantity
currency_type
start_price
current_bid
current_bidder
buy_now_price
starts_at
ends_at
status
```

### Auction Rules

- World bazlıdır.
- Lot'lar server tarafından generate edilir.
- Buy-now varsa anında kapanır.
- Bid refund ledger ile yapılır.
- Auction bitince winner'a item/unlock verilir.
- Ships item olarak değil unlock olarak grant edilir.
- Lot history saklanır.

### Auction Anti-Abuse

- Bid sniping olabilir, oyun tasarımına göre normal sayılabilir.
- Son saniye bid extension opsiyonel.
- Premium fraud riskinde won item trade lock.
- Sistem lot density ekonomiye göre ayarlanır.

## Premium System

### Premium Philosophy

Premium oyuncuya:

- Kozmetik
- Convenience
- Controlled time saver
- Limited weekly opportunity

verebilir.

Premium oyuncuya sınırsız direkt güç vermemeli.

### Allowed Premium Items

Güvenli alanlar:

- Ship skins
- Loadout slots
- Name change
- Chat title
- Badge
- Title above ship
- Cosmetic engine trails
- UI themes
- Hangar cosmetics
- Small time savers
- Weekly 1x X Core purchase right

### Weekly X Core Purchase

Kural:

```text
player can buy 1 X Core per week with premium currency
```

Balance:

- 7/24 farm yapan normal oyuncu 1 haftada bir X Core elde edebilmeli.
- Premium oyuncu aynı şeyi daha garanti/convenient yapar.
- Unlimited X Core satışı yok.

### Premium Stock

System shop'ta premium stock olabilir.

Örnek:

```text
Slazar premium stock per week per world = 100
```

Stock bitince:

- Craft devam eder.
- Credit purchase varsa devam eder veya sınırlı olabilir.
- Auction buy-now daha pahalı alternatif olur.

### Paid vs Earned Premium

Premium kaynakları:

- Paid purchase
- Quest reward
- Event reward
- Market purchase from another player

Takip:

```text
premium_paid
premium_earned
premium_market_acquired
```

Basit model:

- Paid premium tradeable
- Earned premium non-tradeable
- Market-acquired premium maybe tradeable false after acquisition

Bu kararı sonra netleştiririz.

## Death and Repair System

### Death Philosophy

Ölüm anlamlı olmalı ama oyuncuyu oyundan kilitlememeli.

Risk:

- Cargo drop
- Ship disabled
- Repair cost
- Module durability loss
- Time loss

Safety:

- Starter ship always available
- Repair path exists
- Checkpoint/nearest planet repair
- Old ship swap possible

### Death Flow

```text
Player ship HP reaches 0
Server marks ship disabled/destroyed
Server calculates cargo drop percent 50%-100%
Server creates loot drops from cargo
Server rolls module durability losses
Server moves player to respawn/checkpoint state
Player chooses repair or ship swap
```

### Respawn Location

Respawn seçenekleri:

- Checkpoint
- Nearest owned planet
- Nearest safe station
- Clan station, ileride

MVP:

```text
respawn at last checkpoint or nearest known safe planet
```

### Repair Options

Repair:

- Credits
- Premium currency, convenience
- Repair kit, craftable
- Planet repair building, ileride cheaper

Repair cost factors:

```text
ship tier
ship base value
damage severity
player rank
repair location
insurance/role bonuses
```

Basit formula:

```text
repair_cost = ship_credit_value * repair_rate * repair_location_modifier
```

Örnek:

```text
repair_rate = 0.08 to 0.15
```

### Module Durability Loss

Death sırasında:

```text
for module in equipped_modules:
  if module.has_durability:
    if roll(module_damage_chance):
      durability -= 1
```

PvP zone'da chance daha yüksek olabilir.

Low-level safe zones'da durability loss olmayabilir.

### Cargo Drop Percent

Kural:

```text
drop_percent = random(50, 100)
```

Bu hardcore his verir.

Sonra region'a göre ayarlanabilir:

```text
safe zone: 0%-20%
normal PvE: 30%-70%
perma PvP: 50%-100%
```

Ama konuştuğumuz karar perma/riskli alan için `50%-100%`.

### Insurance

MVP'de gerekli değil.

İleride:

- Cargo insurance
- Ship repair insurance
- Route insurance

eklenebilir.

## Combat Progression Touchpoints

### Combat Modes

Oyunda iki combat hissi olacak:

#### Idle/Auto Combat

Oyuncu target seçer veya auto combat açar.

Server:

- Range kontrol eder
- Line/sight/radar kontrol eder
- Cooldown kontrol eder
- Energy kontrol eder
- Damage hesaplar

Oyuncu NPC farm yaparken XP kazanır.

#### Manual Combat

Oyuncu:

- Rocket launcher basar
- Skill kullanır
- Scanner/jammer timing yapar
- Movement ile range oynar
- Energy yönetir

Manual combat daha yüksek skill ceiling verir.

### Server Authoritative Combat

Client gönderir:

```text
intent_attack(target_id)
intent_use_skill(skill_id, target/position)
```

Server validate eder:

- Player alive
- Target visible/in sight
- Target in range
- Weapon cooldown ready
- Energy enough
- Ammo/resource enough
- No state conflict

Server hesaplar:

- Hit/miss
- Damage
- Shield/HP application
- Aggro
- Loot rights
- XP contribution

### Combat XP

NPC öldürmek XP verir.

XP paylaşımı:

- Damage contribution
- Party rules
- Last hit maybe irrelevant
- Anti-leech distance/radar participation

MVP:

```text
XP goes to player/party with valid contribution
```

## Quest and Contract Board

### Board Philosophy

Board oyuncuya yön verir.

Oyuncu sonsuz uzayda ne yapacağını kaybetmemeli.

Board:

- Günlük/haftalık hedef verir.
- Oyuncuyu farklı sistemlere iter.
- Rare reward ihtimaliyle heyecan yaratır.
- X Core ve premium currency gibi değerli şeyleri kontrollü verir.

### Board Structure

Konuştuğumuz model:

```text
Board shows 10 available quests
Player can accept 3 active quests
Player can reroll board for credits
```

Board type yerine quest type kullanırız.

### Quest Types

Örnek quest tipleri:

- Kill X NPC
- Hunt pirate group
- Scan X signals
- Discover a planet of at least level N
- Collect X raw material
- Deliver X material to planet/station
- Craft X item
- Build/upgrade X building
- Loot X caches
- Travel to coordinate band
- Clear anomaly
- Sell X material on market, dikkat abuse
- Complete route transfer

### Quest Difficulty

Quest difficulty:

- Player rank
- Main level
- Role levels
- Distance from origin
- Known planet network
- Recent activities

ile scale olabilir.

Örnek:

```text
Rank 3 player:
- Kill 40 Tier 2 pirates
- Scan 3 unknown signals
- Deliver 200 Iron Ore

Rank 8 player:
- Kill 100 Void Raiders
- Discover level 7+ planet
- Craft 2 Shield Matrix modules
```

### Quest Rewards

Reward types:

- Credits
- Main XP
- Role XP
- Raw materials
- Processed materials
- X Core fragment
- X Core, rare
- Premium currency, rare
- Recipe/blueprint unlock
- Cosmetic/title/badge
- Coordinate/intel packet

### Reroll System

Player board'u credits karşılığı reroll edebilir.

Rules:

- Reroll cost scales with rank
- Daily free reroll maybe
- Cannot reroll active accepted quests unless abandoned
- Rare reward quests too frequently reroll-farm edilmemeli

### Quest State

Data:

```text
player_quest_id
template_id
generated_seed
state
progress
accepted_at
expires_at
completed_at
reward_claimed_at
```

State:

```text
offered
accepted
completed
claimed
expired
abandoned
```

## Planet Production System

### Production Philosophy

Planetler sadece ownership trophy olmamalı.

Planet:

- Material üretir.
- Building ister.
- Energy üretir/tüketir.
- Storage sağlar.
- Crafting chain'e bağlanır.
- Automation route ağına node olur.
- Oyuncunun galaxy network'ünü büyütür.

### Planet Production Inputs

Planet production şunlardan etkilenir:

- Planet type
- Planet level
- Biome
- Buildings
- Building level
- Energy availability
- Storage capacity
- Owner rank/construction bonuses
- Route import inputs
- Live ops modifiers

### Production Rates

Her building rate verir:

```text
resource_id
amount_per_hour
energy_cost_per_hour
input_materials_per_hour
storage_target
```

Örnek:

```text
Crystal Extractor Level 2:
- produces 40 Crystal Fragment / hour
- consumes 8 Energy / hour

Alloy Foundry Level 1:
- consumes 30 Iron Ore / hour
- consumes 5 Energy Cell / hour
- produces 10 Refined Alloy / hour
```

### Online Production

Oyuncu online iken tick:

```text
every production_tick:
  calculate elapsed time
  calculate production
  check inputs/storage/energy
  apply output
  update last_calculated_at
```

Production tick her saniye olmak zorunda değil.

MVP:

```text
tick every 60 seconds or 5 minutes
```

UI daha smooth göstermek için client estimate yapabilir ama truth server'dır.

### Offline Production

Oyuncu offline iken sürekli tick çalıştırmaya gerek yok.

Login/inspection anında:

```text
elapsed = now - last_calculated_at
simulate production over elapsed
apply storage cap
apply route transfers/losses
update last_calculated_at = now
```

Bu çok önemli çünkü 10 binlerce offline planet için sürekli job çalıştırmak gereksiz.

### Offline Production Limits

Cap gerekli.

Örnek:

```text
max_offline_hours = 24 or 72
```

Premium ile biraz artabilir ama dikkatli.

Storage capacity zaten doğal cap.

Kural:

```text
effective_elapsed = min(elapsed, max_offline_hours)
```

### Storage Capacity Clamp

Production storage'a sığdığı kadar işler.

```text
available_capacity = storage_capacity - current_storage
actual_output = min(calculated_output, available_capacity)
```

Input tüketen production için order önemli.

MVP:

- Aynı planet production'ları deterministic order ile hesaplanır.
- Daha sonra production priority UI eklenebilir.

## Planet Automation Routes

### Automation Fantasy

Oyuncu planet network kurar.

Planet X'te üretilen material otomatik Planet Y'ye veya storage'a akar.

UI'da star map üzerinde çizgiler görünür:

```text
Planet X -> Planet Y
40 Refined Alloy / h
Risk: 8%
Energy: 12 / h
```

Bu, strategy layer'ın kalbi olabilir.

### Route MVP Model

İlk sürümde route fiziksel convoy olmayacak.

Sanal transfer tick:

```text
every route_tick:
  take X resource from source
  roll route success/loss based on zone risk
  deliver remaining to destination
```

Risk:

- Source/destination distance
- Region risk
- PvP zone
- Deep-space level
- Route security modules/buildings
- Player construction/trade bonuses

### Route Data

```text
route_id
owner_player_id
source_planet_id
destination_type
destination_id
resource_id
amount_per_hour
energy_cost_per_hour
route_risk
loss_chance
enabled
last_calculated_at
```

Destination type:

```text
planet
storage
station
```

### Route Success/Loss

Basit model:

```text
if roll(loss_chance):
  transferred_amount = amount * loss_multiplier
else:
  transferred_amount = amount
```

Loss modelleri:

- Full tick lost
- Partial loss
- Delayed delivery

MVP için partial loss daha az sinir bozucu.

Örnek:

```text
loss_chance = 10%
loss_multiplier = random(0.50, 0.90)
```

### Route Requirements

Route kurmak için:

- Source planet owned
- Destination owned or accessible
- Resource exists in source production/storage
- Enough route capacity
- Enough energy
- Maybe route module/building
- Maybe rank/construction level

### Route UI

Star map/production graph:

- Planet nodes
- Lines between planets/storage
- Resource/hour labels
- Risk color
- Energy/hour
- On/off toggle
- Bottleneck warning
- Storage full warning

Bu UI oyunun "industrial network" hissini verir.

### Future Physical Convoys

Uzun vadede route'lar fiziksel hale gelebilir:

- Cargo drone spawns
- Route detectable by scanner
- Pirate/player can attack convoy
- Escort contracts
- Insurance
- Ambush gameplay

Ama MVP için sanal route daha doğru.

## Intel and Coordinate Economy

### Why It Belongs Here

World system dokümanı planet discovery'yi anlatıyor.

Ekonomi tarafında önemli olan:

```text
Knowledge can become an item.
```

Bu oyunu sosyal yapar.

### Intel Types

MVP:

- Planet coordinate/intel

Later:

- Anomaly location
- Rare resource field
- Enemy base sighting
- Wormhole signature
- Route risk report
- NPC boss spawn

### Share Module

Oyuncu başka oyuncuya veya clan üyesine planet intel paylaşabilir.

Kural:

```text
player can share X game units per day
```

İlk scope:

```text
Only planets can be shared.
```

Share sonucu:

- Receiver fog memory'de planet açılır.
- In-game mail gider.
- Intel record receiver'a yazılır.
- Confidence/last_seen bilgisi taşınır.

### Coordinate Scroll

Intel item haline gelebilir.

Ad önerileri:

- Star Chart
- Coordinate Scroll
- Planet Intel Packet
- Survey Report

Item data:

```text
intel_type = planet
planet_id
coordinates
planet_level
planet_type_known
owner_known
last_verified_at
confidence
```

### Intel Staleness

Eğer planet başkası tarafından kolonize edilirse:

- Scroll tamamen silinmeyebilir.
- Ama market listing stale hale gelir.
- Buyer uyarılır veya listing auto-unlist edilir.

Konuştuğumuz güçlü kural:

```text
If planet is colonized while scroll is listed,
market listing can be removed/stale.
```

Bu bilgi ekonomisini canlı yapar.

### Intel Abuse Controls

- Daily share limit
- Intel item creation cost
- Stale verification requirement
- Coordinates not client-predictable
- Server-only planet seed
- Fake intel item olmayacak, sadece server-signed intel

## Economy Faucets and Sinks

### Faucets

Oyuna değer sokan kaynaklar:

- NPC drops
- Gather nodes
- Planet production
- Quest rewards
- Daily/weekly board
- Event rewards
- Premium purchase
- Auction generated lots
- Scan discoveries

### Sinks

Oyundan değer çıkaran kaynaklar:

- Repair costs
- Crafting fees
- Market fees
- Auction fees/bids to system
- Route energy costs/losses
- Building construction costs
- Building upkeep, later
- Module repair
- Ship unlock costs
- Reroll board costs
- Respec costs
- Wormhole upkeep
- Storage expansion

### Important Balance Principle

Eğer planet production güçlü olacaksa, sink'ler de güçlü olmalı.

Yoksa birkaç ay sonra server ekonomisi şişer.

Özellikle sinks:

- Repair
- Craft
- Auction
- Market tax
- Route loss/upkeep
- Building upgrades

çok önemli.

## Server Authoritative Rules

### Client Cannot Decide

Client karar veremez:

- Damage
- Hit/miss
- Loot drop
- Pickup validity
- XP reward
- Craft completion
- Quest completion
- Market transaction
- Auction win
- Production output
- Offline production
- Route success
- Ship death
- Repair cost
- Premium entitlement
- Planet ownership

Client sadece intent gönderir.

### Server Events

Önerilen event isimleri:

```text
player.xp_gained
player.rank_up
player.role_level_up
pilot.skill_unlocked

ship.unlocked
ship.swapped
ship.destroyed
ship.repaired

module.equipped
module.unequipped
module.durability_changed

loot.created
loot.picked_up
loot.expired

craft.job_started
craft.job_completed

market.listing_created
market.sale_completed
auction.lot_created
auction.bid_placed
auction.lot_closed

quest.accepted
quest.progressed
quest.completed
quest.reward_claimed

planet.production_settled
route.transfer_settled
intel.shared
intel.item_created
```

### Transaction Ledger

Her değer transferi ledger'a yazılmalı.

Ledger:

- Debug
- Fraud detection
- Support ticket
- Economy analytics
- Rollback/reconciliation

için gerekli.

Ledger event:

```text
ledger_id
player_id
currency_or_item
delta
reason
source_system
reference_id
created_at
```

## Suggested Data Model

Bu birebir migration değil, modül düşünmek için blueprint.

### Player Progression

```text
players
- id
- username
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

player_rank_history
- id
- player_id
- old_rank
- new_rank
- reason
- created_at

player_skill_points
- player_id
- total_points
- spent_points

player_skill_nodes
- player_id
- node_id
- unlocked_at
```

### Ships and Loadouts

```text
ship_definitions
- ship_id
- name
- tier
- rank_requirement
- base_stats_json
- slot_layout_json
- prices_json
- craft_recipe_id

player_ships
- player_id
- ship_id
- unlocked_at
- state
- durability_state
- disabled_until
- last_repaired_at

player_active_ship
- player_id
- ship_id

ship_loadouts
- loadout_id
- player_id
- ship_id
- name
- slot_assignments_json
- created_at
- updated_at
```

### Modules and Items

```text
item_definitions
- item_id
- name
- item_type
- rarity
- stack_size
- weight
- trade_flags
- metadata_json

module_definitions
- item_id
- module_type
- tier
- required_rank
- stats_json
- energy_json
- durability_max

player_items
- item_instance_id
- player_id
- item_id
- quantity
- location_type
- location_id
- durability_current
- bound_state
- metadata_json
```

### Inventory and Wallet

```text
player_wallets
- player_id
- currency_type
- balance

wallet_ledger
- ledger_id
- player_id
- currency_type
- delta
- reason
- reference_id
- created_at

item_ledger
- ledger_id
- player_id
- item_id
- quantity_delta
- reason
- reference_id
- created_at
```

### Loot

```text
world_loot_drops
- drop_id
- world_id
- zone_id
- position_x
- position_y
- item_id
- quantity
- owner_player_id
- owner_lock_until
- public_until
- expires_at
- source_type
- source_id
- created_at
```

### Crafting

```text
recipes
- recipe_id
- output_item_id
- output_amount
- inputs_json
- requirements_json
- duration_seconds
- fees_json

crafting_jobs
- job_id
- player_id
- recipe_id
- location_type
- location_id
- state
- started_at
- completes_at
- completed_at
```

### Market and Auction

```text
market_listings
- listing_id
- seller_player_id
- item_instance_id
- item_id
- quantity
- unit_price
- currency_type
- status
- expires_at
- metadata_json

auction_lots
- auction_id
- world_id
- lot_type
- payload_json
- currency_type
- start_price
- current_bid
- current_bidder
- buy_now_price
- starts_at
- ends_at
- status

auction_bids
- bid_id
- auction_id
- bidder_player_id
- amount
- created_at
```

### Quests

```text
quest_templates
- template_id
- quest_type
- requirements_json
- rewards_json
- difficulty_rules_json

player_quest_offers
- offer_id
- player_id
- template_id
- generated_payload_json
- expires_at

player_quests
- player_quest_id
- player_id
- template_id
- state
- progress_json
- rewards_json
- accepted_at
- completed_at
- claimed_at
```

### Planet Production and Routes

```text
planet_storage
- planet_id
- item_id
- quantity

planet_buildings
- building_id
- planet_id
- building_type
- level
- state
- started_at
- completed_at

planet_production_state
- planet_id
- last_calculated_at
- production_config_json

automation_routes
- route_id
- owner_player_id
- source_planet_id
- destination_type
- destination_id
- resource_id
- amount_per_hour
- energy_cost_per_hour
- risk_json
- enabled
- last_calculated_at
```

### Intel

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

intel_items
- item_instance_id
- intel_type
- payload_json
- last_verified_at
- stale_state
```

## MVP Scope

İlk oynanabilir sürüm için minimum:

- Main level
- Rank
- Role XP skeleton
- Rank-up passive point grant
- Simple passive tree
- Hangar with starter + craftable ships
- Ship loadout
- Offensive/defensive/utility modules
- Server stat aggregation
- NPC loot raw material drops
- Owner-locked loot pickup
- Cargo capacity
- Death cargo drop
- Ship disabled + repair
- Basic crafting
- Fixed-price market
- Premium currency wallet split
- System shop weekly stock
- Simple auction hall
- Board with 10 quest offers / 3 active quests
- Planet production online/offline
- Virtual automation routes
- Planet intel share/item skeleton

MVP'de ertelenebilir:

- Full physical convoy combat
- Insurance
- Module enhancement/reroll
- Complex rarity affixes
- Region-specific markets
- Clan market
- Advanced fraud automation
- Full PvP bounty system
- Player-made stations

## Open Balance Questions

Sonra netleştirmemiz gereken başlıklar:

1. Rank kaç level olacak ve rank-up milestone'ları nasıl dağıtılacak?
2. Main level ile rank birebir bağlı mı, yoksa rank ayrıca milestone mu isteyecek?
3. Role level cap olacak mı?
4. Passive tree kaç branch olacak?
5. İlk 10 gemi archetype'ı ve slot dağılımı ne olacak?
6. İlk offensive/defensive/utility module listesi ne olacak?
7. Laser basic attack energy cost ne kadar olacak?
8. Death cargo drop safe/normal/PvP zone'a göre değişecek mi?
9. X Core direkt item mı, fragment craft sistemi mi?
10. Premium currency markette tamamen tradeable mı, yoksa paid-only mı?
11. Offline production max kaç saat birikecek?
12. Route loss chance hangi faktörlerden hesaplanacak?
13. Quest board rare reward oranları ne olacak?
14. Ship repair cost ne kadar sert olmalı?
15. Coordinate scroll stale olunca silinecek mi, yoksa "stale intel" olarak kalacak mı?

## Recommended Next Design Topics

Bu dokümandan sonra sırayla şu sistemleri ayrıca detaylandırmak mantıklı:

```text
1. Combat and Damage Formula
2. Ship and Module Catalog v0
3. Resource and Recipe Catalog v0
4. Planet Building and Production Catalog v0
5. Quest Board Generation Rules
6. Market/Auction Economy Rules
7. Offline Settlement Algorithm
8. API/Event Contracts
```

İlk kodlanacak modüller için en iyi sıra:

```text
PlayerProgressionService
InventoryService
WalletService
ShipService
ModuleService
StatAggregationService
LootService
CraftingService
QuestService
PlanetProductionService
AutomationRouteService
MarketService
AuctionService
```

Bu sırayla gidersek oyun loop'u adım adım ayağa kalkar.
