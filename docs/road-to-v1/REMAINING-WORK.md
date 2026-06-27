# Road to v1 — Kalan İşler & Referanslar

> Tarih: 2026-06-27 (güncellendi)
> Kaynak: `docs/road-to-v1/00-index.md` Progress Dashboard + her faz dosyası.
> Aktif goal: `docs/road-to-v1/GOAL.md`

## Mevcut Durum Snapshot

| # | Faz | Wave | Durum | Progress |
|---|---|:---:|---|---|
| 01 | Persistence Foundation | 1 | ✅ Done | 100% |
| 02 | Transactional Economy & Outbox | 2 | ✅ Done | 100% |
| 03 | Realtime Hardening | 1 | ✅ Done | 100% |
| 04 | Rate Limiting & Abuse Posture | 1 | ✅ Done | 100% |
| 05 | Map Worker Ownership & Concurrency | 2 | 🟡 In progress | 90% |
| 06 | Movement, Combat & Death Correctness | 2 | ✅ Done | 100% |
| 07 | Equipment & Progression Closure | 3 | ✅ Done | 100% |
| 08 | Durable Planet, Production & Routes | 3 | 🟡 In progress | 90% |
| 09 | CMS Completion & Balance Telemetry | 3 | ✅ Done | 100% |
| 10 | Social MVP | 4 | 🟡 In progress | 80% |
| 11 | First Endgame Loop (Signal Gate) | 5 | ⬜ Not started | 0% |
| 12 | DarkOrbit Flavor | 6 | ⬜ Not started | 0% |
| 13 | Observability, Simulation & Release Gate | 4 | ⬜ Not started | 0% |
| 14 | CMS Runtime Application & Content Safety | 3 | ✅ Done | 100% |
| 15 | World Performance & AOI/Aggro Optimization | 4 | ⬜ Not started | 0% |
| 16 | Production Config & Operational Hardening | 2 | ✅ Done | 100% |
| 17 | Runtime Decomposition & Maintainability | 6 | ⬜ Not started | 0% |

**Genel v1:** ~67%

---

## Kalan Fazlar (öncelik sırası)

### 🟡 P05 — Map Worker Ownership & Concurrency (Wave 2, 90%)

Kalan: deep `Runtime.mu` narrowing. Combat/loot/repair command handler'ları hâlâ
koordinasyon amacıyla `Runtime.mu` tutuyor (visibility/range check + value claim
atomik kalmalı). Derin daraltma, canlı-position visibility'yi owning service'e
taşımayı gerektiriyor → **P17'ye (Runtime Decomposition) ertelendi**.

- [ ] Combat/loot/repair handler'larında `Runtime.mu`'yu daralt: canlı-position
  visibility/range'i owning service (loot/combat) içine taşı, runtime sadece
  session/routing bookkeeping kilitlesin. Race coverage ekle.
- [ ] "Map A aktivitesi Map B arkasında serialize olmasın" — zaten tick path'te
  çözüldü, ama command-handler seviyesinde doğrula.

**Referanslar:**
- `docs/road-to-v1/05-map-ownership-concurrency.md`
- `internal/game/server/combat_loot_repair.go` (handler lock sites)
- `internal/game/server/runtime_world_snapshot.go` (tick path, zaten narrow)
- `internal/game/server/runtime_durable_outbox_realtime.go`
- `docs/todo.md` (P05/P17 deferral kaydı)

---

### 🟡 P08 — Durable Planet, Production & Routes (Wave 3, 90%)

✅ **DB engeli çözüldü (2026-06-26).** Durable store adapter'ları yazıldı:
- `contentdb.ClaimDurableLifecycleStore` — claim lifecycle plan JSON, idempotent
  replay, conflict rejection.
- `contentdb.BuildingMutationDurableStore` — building mutation plan JSON,
  idempotent replay, conflict rejection.
- `contentdb.SettlementDurableStore` — settlement plan JSON (planet + route
  window lookups), idempotent replay, conflict rejection.
- `contentdb.AutomationRouteDurableStore` — route CAS revision + reference-key
  dedup log + owner listing.
- `contentdb.ClaimProductionInitializationDurableStore` — production-init plan
  JSON, pending → complete advance, stale pending replay, pending-row filter,
  conflict rejection.
- Runtime core-store DB mode now wires these adapters into `Runtime` for claim
  lifecycle, claim production-init, settlement, building mutation, and
  automation route durable rows; dev/off mode keeps the safe in-memory fallback.
- Claim/settlement/building durable outbox rows now support DB-backed
  claim/publish/fail/lease-release/retry mutation so the existing tick-driven
  drain can run against committed Postgres rows.
- DB-only runtime restart smokes now prove one X Core debit across claim retry,
  pending claim production-init recovery, one durable route settlement window,
  and one missed durable claim outbox replay after runtime restart.
- Migrations 0019/0020/0021/0022, Postgres smoke coverage (persist/duplicate/
  conflict/advance/pending filter + DB-backed outbox publish), foundation
  `Quantity`/`Money` JSON round-trip fix.

Kalan (runtime seviyesi):
- [x] Runtime'ın 4 in-memory durable store field'ını DB-backed adapter'a bağla
  (core-store DB mode); in-memory default dev/off fallback olarak kalır.
- [x] Scheduled outbox publisher + recovery worker DB store contracts
  (claim/production/route outbox drain, tick-driven).
- [x] Process-local claim/settlement/building/route idempotency map'lerini
  DB-backed key'lere bağla (core-store DB mode).
- [x] Restart-survival smoke: claim tek X Core consume, production window tek
  sefer, route settlement tek sefer, recovery worker miss replay.

**Smoke Tests:** claim tek X Core consume (restart sonrası), production window
tek sefer, route settlement tek sefer, recovery worker miss replay, restart
survival.

**Referanslar:**
- `docs/road-to-v1/08-durable-planet-production-routes.md`
- `internal/game/discovery/claim.go` + `claim_boundary.go` (InMemoryStore → DB)
- `internal/game/discovery/claim_outbox.go`
- `internal/game/production/building_mutation.go` (InMemoryStore → DB)
- `internal/game/production/route_settlement*.go`
- `internal/game/contentdb/` (örnek durable store pattern'leri: `market_store.go`,
  `auction_store.go`, `premium_store.go`, `loot_pickup_store.go`)
- `internal/game/server/runtime_durable_outbox.go` (existing DrainDurableOutboxes)
- `docs/2026-06-17-progression-economy-systems-design.md`

---

### ✅ P14 — CMS Runtime Application & Live Content Safety (Wave 3, DONE)

CMS publish → canlı runtime'a yansıtma veya dürüstçe `pending_restart` raporlama.
HI-02 ve HI-08 kapatıldı.

- [x] `[P:wave3/lane-H]` Runtime content version pointer + apply path (atomic
  swap path for explicitly safe-reload content classes).
- [x] `[P:wave3/lane-H]` Publish response report `runtime_applied`,
  `runtime_version`, `published_version`.
- [x] `[P:wave3/lane-H]` Classify content: safe-live-reload vs requires-restart.
  Current changed gameplay content types are conservative `restart_required`
  until all boot-wired read models can hot-swap together.
- [x] `[P:wave3/lane-I]` Active-reference readers: market listings, equipped
  modules, loot drops, NPC templates, routes, shop locks (equipped-module oku;
  gerisi aynı seam ile genişletilebilir).
- [x] `[P:wave3/lane-I]` Block/flag publish when a changed id has active
  references that require quiescence.

**Smoke Tests:** safe-reload apply path reflects catalog when explicitly safe;
changed boot-wired content returns `runtime_applied=false` + `pending_restart`;
rollback uses same publish safety; publish blocked when changed module id is
actively equipped; publish response reports published vs runtime version
honestly. (Green.)

**Referanslar:**
- `docs/road-to-v1/14-cms-runtime-application-content-safety.md`
- `internal/game/content/content_apply.go` (PlanRuntimeApply classification)
- `internal/game/admin/content_service.go` (validatePublishSafety + reader)
- `internal/game/server/runtime_content_apply.go` (Runtime.applyPublishedContent)
- `internal/game/server/runtime_content_safety.go` (ActiveEquippedModules)
- `internal/game/server/content_admin_handlers.go` (honest publish payload)

---

### 🟡 P10 — Social MVP (Chat, Party, Clan) (Wave 4, 80%)

✅ Domain package exists: `internal/game/social/`.
✅ Runtime chat/party realtime slice exists:
- `chat.send` resolves local-map/party/clan membership server-side and queues
  `chat.message` events after message commit.
- `party.invite`, `party.accept`, and `party.leave` use server-owned session
  identity; invite targets resolve from online callsign, not client-sent player
  ids.
- Chat rate-limit and moderation seams are wired through `ChatService`.
✅ Runtime clan/shared-target/client slice exists:
- `party.target.set` validates visible targets server-side and broadcasts
  shared-target realtime events after mutation.
- `clan.create`, `clan.join`, and `clan.leave` use Postgres-backed
  clan/membership rows in core-store DB mode, with in-memory dev fallback.
- Runtime bootstrap emits durable `clan.updated` read models after restart, and
  join/leave events are per-recipient so clients keep their own rank/membership.
- Client social state/panel parses server-owned social ids, opens from the HUD,
  and sends chat/party/clan intent-only commands.

- [x] Chat service domain: server-resolved channels, moderation hook seam,
  rate-limit seam + default cooldown, in-memory message store.
- [x] Party service domain: create/invite/accept/leave, leader transfer,
  server-owned membership.
- [x] Clan service domain: create/join/leave, owner rank, membership store,
  clan-chat membership list.
- [x] Channel membership resolver: local-map, party, clan, system channel
  routing.
- [x] Wire realtime ops/events: `chat.send`, `party.invite`, `party.accept`,
  `party.leave`.
- [x] Wire realtime ops/events: `clan.create`, `clan.join`, `clan.leave`.
- [x] Durable clan rows + durable/social read models.
- [x] Party shared-target realtime foundation.
- [ ] Contribution event foundation.
- [x] `[P:wave4/lane-A]` Client: chat panel + party panel + clan panel (real
  state only).

Kalan:
- [ ] Chat moderation redaction/logging policy (no PII leaks).
- [ ] Party/clan contribution event semantics and read models.

**Referanslar:**
- `docs/road-to-v1/10-social-mvp.md`
- `docs/code-review/feature-gap-analysis.md` (§social)
- `internal/game/social/`

---

### ⬜ P13 — Observability, Simulation & Release Gate (Wave 4, 0%)

- [ ] §14 simulation/race test coverage.
- [ ] §13 load/scalability evidence.
- [ ] Release gate yeşil (tüm module/check pair'ler).

**Referanslar:**
- `docs/road-to-v1/13-observability-simulation-release.md`
- `internal/game/observability/` (gate coverage, balance telemetry)
- `internal/game/observability/simulations/`

---

### ⬜ P15 — World Performance & AOI/Aggro Optimization (Wave 4, 0%)

- [ ] HI-07: Aggro/AOI hot path O(N×M) değil → spatial index / bucketing.
- [ ] AOI read projection immutable snapshot/copy-on-write (P05 ile ilişkili).

**Referanslar:**
- `docs/road-to-v1/15-world-performance-aoi-optimization.md`
- `internal/game/world/spatial/`
- `internal/game/world/visibility/`
- `internal/game/server/runtime_world_snapshot.go` (AOI diff path)

---

### ⬜ P11 — First Endgame Loop (Signal Gate) (Wave 5, 0%)

P06 + P07 dependency. Tek tekrarlanabilir endgame gate loop'u uçtan uca.

- [ ] Signal gate discovery → activation → reward loop.
- [ ] Endgame gate state machine + idempotency.

**Referanslar:**
- `docs/road-to-v1/11-first-endgame-signal-gate.md`
- `internal/game/discovery/` (hidden signals)
- `docs/2026-06-17-world-system-design.md`

---

### ⬜ P12 — DarkOrbit Flavor (Wave 6, 0%)

P07 + P11 dependency. Drones, P.E.T., ammo, honor (en az MVP).

- [ ] Drones (deploy/fight/cargo assist).
- [ ] Ammo system (ammo-consuming weapons).
- [ ] Honor metric + effects.
- [ ] P.E.T. (opsiyonel, minimal).

**Referanslar:**
- `docs/road-to-v1/12-darkorbit-flavor.md`
- `docs/code-review/feature-gap-analysis.md` (§DarkOrbit flavor)

---

### ⬜ P17 — Runtime Decomposition & Maintainability (Wave 6, 0%)

⚠️ P05 deep mu narrowing buraya ait. Continuous refactor, en sona.

- [ ] `Runtime` coordinator'lara böl (session, routing, combat, loot, economy
  projection).
- [ ] P05 deep mu narrowing (combat/loot/repair handler lock daraltma).
- [ ] Davranış regresyonu yok.

**Referanslar:**
- `docs/road-to-v1/17-runtime-decomposition-maintainability.md`
- `internal/game/server/runtime.go` (~1450+ satır, çok büyük)
- `docs/todo.md` (P05/P17 deferral kaydı)

---

## Wave Sırası

```text
Wave 1: P01 ✅ | P03 ✅ | P04 ✅
Wave 2: P02 ✅ | P05 🟡(90%, P17'ye ertelendi) | P06 ✅ | P16 ✅
Wave 3: P07 ✅ | P08 🟡 | P09 ✅ | P14 ✅
Wave 4: P10 🟡 | P13 ⬜ | P15 ⬜
Wave 5: P11 ⬜
Wave 6: P12 ⬜ | P17 ⬜(continuous)
```

## Blocker'lar / Riskler

1. ~~**P08 Postgres auth**~~ ✅ **Çözüldü (2026-06-26):** Port uyumsuzluğuydu
   (container 55432, `.env` 5432). `.env` 55432'ye hizalandı; smoke + full
   `go test ./...` yeşil. Not: `.env` locale-only, commite edilmez; DB portu
   gerçekten 5432 isteniyorsa container `POSTGRES_PORT` ile yeniden başlatılır.
2. **P05→P17**: Deep mu narrowing Runtime decomposition gerektiriyor, Wave 6'ya
   ait. Risk düşük (tick serialization zaten çözüldü, command-handler lock
   microsecond-scale, I/O yok).
3. **P14 offline active-reference gap:** Equipped-module safety currently reads
   runtime-known players. Durable/offline loadout-wide reader remains P17/P14
   follow-up before "all offline loadouts protected" claim.
4. **P07/P02 shop transaction gap:** non-starter ship shop buy path still needs
   single transaction boundary for wallet debit + hangar grant + idempotency.
5. **P09/P13 release gate gap:** balance telemetry helper exists; release-gate
   integration still belongs to P13.
6. **Mevcut blocker yok:** P08 runtime wiring, P10 realtime/client, P13, P15
   çalışmaya hazır.
