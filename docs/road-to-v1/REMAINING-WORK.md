# Road to v1 — Kalan İşler & Referanslar

> Tarih: 2026-06-26 (güncellendi)
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
| 08 | Durable Planet, Production & Routes | 3 | ⬜ Not started | 0% |
| 09 | CMS Completion & Balance Telemetry | 3 | ✅ Done | 100% |
| 10 | Social MVP | 4 | ⬜ Not started | 0% |
| 11 | First Endgame Loop (Signal Gate) | 5 | ⬜ Not started | 0% |
| 12 | DarkOrbit Flavor | 6 | ⬜ Not started | 0% |
| 13 | Observability, Simulation & Release Gate | 4 | ⬜ Not started | 0% |
| 14 | CMS Runtime Application & Content Safety | 3 | ⬜ Not started | 0% |
| 15 | World Performance & AOI/Aggro Optimization | 4 | ⬜ Not started | 0% |
| 16 | Production Config & Operational Hardening | 2 | ✅ Done | 100% |
| 17 | Runtime Decomposition & Maintainability | 6 | ⬜ Not started | 0% |

**Genel v1:** ~55%

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

### ⬜ P08 — Durable Planet, Production & Routes (Wave 3, 0%)

✅ **DB engeli çözüldü (2026-06-26):** "SASL auth hatası" aslında port
uyumsuzluğuydu. Docker container `gameproject-postgres` host portu **55432**'de
çalışıyor ama `.env`/`GAME_CONTENT_DATABASE_URL` **5432**'ye işaret ediyordu
(orada yanlış şifreli native bir instance var). `.env` artık 55432'ye
hizalanmış; contentdb smoke test'leri + tam `go test ./...` yeşil. P08 artık
DB erişimine açık, store adapter'ları yazılıp smoke test'lerle doğrulanabilir.

- [ ] `[P:wave3/lane-D]` Durable claim lifecycle + X Core consume tek
  transaction/CAS içinde.
- [ ] `[P:wave3/lane-D]` Production settlement window'ları durable idempotent
  row olarak persist.
- [ ] `[P:wave3/lane-D]` Route settlement window'ları + storage ledger durable.
- [ ] `[P:wave3/lane-E]` Scheduled outbox publisher + recovery worker
  (claim/production/route için, request-driven değil).
- [ ] `[P:wave3/lane-E]` Process-local idempotency map'leri DB-backed key'lerle
  değiştir.

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

### ⬜ P14 — CMS Runtime Application & Live Content Safety (Wave 3, 0%)

CMS publish → canlı runtime'a yansıtma veya dürüstçe `pending_restart` raporlama.

- [ ] `[P:wave3/lane-H]` Runtime content version pointer + apply path (atomic
  swap for safe-reload content classes).
- [ ] `[P:wave3/lane-H]` Publish response report `runtime_applied`,
  `runtime_version`, `published_version`.
- [ ] `[P:wave3/lane-H]` Classify content: safe-live-reload vs requires-restart
  vs requires-migration.
- [ ] `[P:wave3/lane-I]` Active-reference readers: market listings, equipped
  modules, loot drops, NPC templates, routes, shop locks.
- [ ] `[P:wave3/lane-I]` Block/flag publish when a changed id has active
  references that require quiescence.

**Smoke Tests:** safe-reload field reflected without restart; restart-required
field returns `runtime_applied=false` + `pending_restart`; publish blocked when
changed module id actively equipped; publish response reports published vs
runtime version honestly.

**Referanslar:**
- `docs/road-to-v1/14-cms-runtime-application-content-safety.md`
- `internal/game/admin/` (ContentPublisher, ContentSnapshotReader)
- `internal/game/admin/content_diff.go` (P09 diff servisi — runtime apply için
  temel)
- `internal/game/server/runtime.go` (content load path)
- `internal/game/content/` (content bundle)

---

### ⬜ P10 — Social MVP (Chat, Party, Clan) (Wave 4, 0%)

- [ ] `[P:wave4/lane-A]` Chat send + channel resolution server-side; enforce
  `chat.send` rate limit.
- [ ] `[P:wave4/lane-A]` Chat moderation hooks + redaction/logging policy.
- [ ] `[P:wave4/lane-B]` Party invite/accept/leave with server-owned membership.
- [ ] `[P:wave4/lane-B]` Party shared-target + contribution event foundation.
- [ ] `[P:wave4/lane-C]` Clan create/join/leave + ranks + tag (durable rows).
- [ ] `[P:wave4/lane-C]` Clan chat channel bound to clan membership.
- [ ] `[P:wave4/lane-A]` Client: chat panel + party panel + clan panel (real
  state only).

**Referanslar:**
- `docs/road-to-v1/10-social-mvp.md`
- `docs/code-review/feature-gap-analysis.md` (§social)
- Henüz domain kodu yok — yeni paket gerekiyor (`internal/game/social/`).

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
Wave 3: P07 ✅ | P08 ⬜ | P09 ✅ | P14 ⬜
Wave 4: P10 ⬜ | P13 ⬜ | P15 ⬜
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
3. **Mevcut blocker yok:** P08 (DB artık açık), P14, P10, P13, P15 hepsi
   çalışmaya hazır.
