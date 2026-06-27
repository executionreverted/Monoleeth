# Road To v1 — GOAL

Bu mesajı aktif goal olarak set et ve tüm fazlar bitene kadar özerk devam et.

## Objective

`docs/road-to-v1/` altındaki tüm fazları (P01–P17) wave sırasına göre tamamlayarak
oyunu durable, güvenli, sosyal ve DarkOrbit-benzeri bir v1'e getir. Plan kaynağı
iki rapor: `docs/code-review/game-systems-code-review.md` ve
`docs/code-review/feature-gap-analysis.md`. Index ve takip:
`docs/road-to-v1/00-index.md`.

## Durum Snapshot (Son Review: 2026-06-27)

Pause noktası. Resume eden buradan devam etsin. Faz statü doğrusu hep
`docs/road-to-v1/00-index.md` Progress Dashboard.

### Wave bazlı statü
- Wave 1: P01 Done, P03 Done, P04 Done.
- Wave 2: P02 Done, P05 90% (deep mu narrowing → P17), P06 Done, P16 Done.
- Wave 3: P07 Done, P08 90% (durable adapters + migrations + runtime DB wiring
  + DB outbox/recovery mutation support + restart survival smoke proof done),
  P09 Done, P14 Done (HI-02/HI-08 closed — rollback safety + honest
  `pending_restart`).
- Wave 4: P10 Done (chat/party/clan runtime, durable clan rows/read models,
  party shared-target realtime, real client panels, moderation redaction/logging,
  and contribution read models done), P13 20% (Prometheus-compatible `/metrics`
  endpoint exports runtime metric snapshots with production bearer-token guard;
  OTel/sim/load/race evidence remains), P15 70% (worker aggro target acquisition
  uses a player-only spatial index; AOI tick path reuses one per-map worker
  snapshot, versions public entity payloads, skips unchanged diffs, and emits
  tick sub-phase metrics).
- Wave 5-6: P11/P12/P17 not started.
- Genel v1: ~73%.

### Bu session yapılanlar (commitler, en yeni üstte)
- P13 lane-D metrics export slice — server now exposes Prometheus-compatible
  `GET /metrics` from the runtime `MetricRecorder`, production startup requires
  `GAME_METRICS_TOKEN`, configured scrapes require bearer auth, metric/label
  identifiers are normalized for text exposition, and release-gate metrics
  evidence references the endpoint smoke.
- P15 lane-G AOI diff slice — runtime tick now collects one worker snapshot per
  map and builds per-session AOI from that copy, public AOI payloads carry stable
  entity versions so unchanged entities do not produce update events, hidden
  entities remain filtered after snapshot sharing, and tick sub-phase durations
  are emitted for movement, aggro, AOI, and enqueue.
- P15 lane-F aggro spatial slice — worker-owned player spatial index now tracks
  player insert/update/remove plus move/settle/speed/tick movement paths, and
  `nearestAggroTarget` queries player candidates by aggro radius instead of
  scanning every player. Focused tests prove nearest-target parity, movement
  index refresh into aggro radius, and no full player scan via candidate count.
- P10 completion slice — default chat moderation redacts PII/secrets before
  storage/fanout, moderation audit logs keep only keyed HMAC fingerprints plus
  safe metadata, and party/clan contribution read models publish server-owned
  NPC-kill contribution totals with opaque occurrence ids.
- P10 clan/social client slice — Postgres-backed clan/membership rows in
  core-store DB mode, realtime `party.target.set` + `clan.create/join/leave`,
  durable clan bootstrap read models, client social state/panel, and review-fix
  hardening for social parser allowlists, per-recipient clan snapshots, and
  observability command coverage.
- P10 chat/party realtime slice — `chat.send`, `party.invite`,
  `party.accept`, and `party.leave` are runtime-wired with server-owned session
  identity, online callsign invite resolution, chat rate/moderation seams, and
  post-mutation social events.
- P08 restart-survival smoke slice — DB-only Postgres runtime restart tests prove
  one X Core debit across claim retry, pending claim production-init recovery,
  one route settlement window, and one missed durable claim outbox replay.
- P08 lane-E runtime DB wiring slice — core-store DB mode injects Postgres-backed
  claim lifecycle, claim production-init, settlement, building mutation, and
  automation route durable stores; DB-backed claim/settlement/building outbox
  rows now support claim/publish/fail/lease-release/retry.
- `fc30e15 game: make cms rollback and runtime apply safety honest` — rollback
  publish safety'den geçiyor; item/module/shop_product değişiklikleri boot-wired
  read model hot-swap tamamlanana kadar `pending_restart` raporluyor.
- `d9f23f5 game: harden claim production initialization durable store` — P08
  production-init Postgres adapter pending → complete advance, stale replay,
  pending-row filter, conflict rejection destekliyor.
- `6ee8163 game: add social MVP domain (chat, party, clan) with rate limits and tests` — P10 domain package done, runtime/client wiring pending.
- `d35f231` + `c2874e2` — P08 durable claim/building/settlement/route adapters
  + migrations/smoke coverage.
- P14 tamamlandı (CMS runtime application + live content safety): content
  classification, `Runtime.applyPublishedContent` projection path, honest publish
  payload (`runtime_applied`/`runtime_version`/`published_version`/
  `pending_restart`), broadened `validatePublishSafety` with equipped-module
  active-reference reader (HI-02 + HI-08 closed).
- P09 lane-F tamamlandı (CMS diff API + audit action migration + quest compat test
  + live-Postgres publish concurrency coverage). Çözüldü: P08 DB auth blocker
  (port uyumsuzluğu — container 55432, `.env` 5432 → 55432'ye hizalandı).
- `2de6a59 game: add economy source/sink balance telemetry` (P09 lane-G).
- `912bc65 game: enable first non-starter ship shop purchase` (P07 lane-C).
- `c89aca2 game: make effective cargo capacity authoritative` (P07 lane-B).
- `cf72678 game: wire inventory.move command` (P07 lane-A).
- `8270ff2 game: wire progression.unlock_skill command` (P07 lane-A).
- `8d3ac42 docs: record P05 worker-ownership completion` (P05 docs).
- `7d1bbbd game: drain economy outbox through runtime tick publisher` (P02 done).

### Sırada (resume sırası)
1. Context tazele: `00-index.md`, `REMAINING-WORK.md`, ilgili faz dosyası.
2. P13: release gate + simulation/load/race evidence, including P15 AOI/aggro
   scaling evidence.
3. Wave 5-6: P11 endgame, P12 flavor, P17 runtime decomposition (+ P05 deep mu).

## Çalışma Kuralları

- Her faz/iş öncesi context tazele: `AGENTS.md`, `docs/road-to-v1/00-index.md`,
  ilgili faz dosyası okunsun - ihtiyaç varsa tabi -
- `AGENTS.md` coding kurallarına uy: server-authoritative, küçük dosyalar, domain
  isimleri, `lock -> validate -> mutate -> ledger/event -> commit -> broadcast`,
  idempotency key, no monolith, no fake state.
- Client sadece intent gönderir; player id/damage/loot/wallet/ownership dahil hiçbir trust gerektiren değişken client'tan alınmaz.
- DB/pgx/Redis/NATS/library syntax gerekiyorsa Context7 MCP kullan.
- Symphony/orchestration kodunu gameplay domain kodundan ayrı tut, symphony kodlarına dokunma.
- Symphony worker taskları gerektiğinde farklı agent backend/model ile çalışabilir:
  - default `codex`;
  - `crush` backend ile z.ai GLM, örn. `agent_model: "zai/glm-5.2"` ve
    `agent_endpoint: "https://api.z.ai/api/coding/paas/v4"`;
  - `crush` backend ile Sakana/OpenRouter, örn.
    `agent_model: "openrouter/sakana/fugu-ultra"`.
- Agent backend/model seçimi sadece orchestration ayarıdır; gameplay domain kodu
  buna göre davranış değiştirmez. API key/env değerleri dokümana, loga veya
  commite yazılmaz.

## Smoke Test Kuralı

Her smoke/e2e test yalnızca TEK bir davranışı assert eder. Uzun mega-smoke yazma;
N davranış için N kısa test yaz.

## Paralel Çalışma (Symphony)

- `[P:wave-N/lane-X]` etiketli tasklar aynı wave içinde paralel Symphony agent'larına dağıtılabilir.
- Symphony kullanacaksan uygun tasklarda `agent_backend`, `agent_model`,
  `agent_endpoint` override verilebilir. GLM/z.ai veya Sakana kullanılsa bile
  agent promptları İngilizce olsun.
- Aynı wave'deki lane'ler disjoint write set'e sahip olmalı (iki agent aynı dosyayı düzenlemez).
- Agent'lar codebase'de yalnız değil: başkasının edit'ini geri alma, uyumlan.

## Wave Sırası

```text
Wave 1: P01 | P03 | P04
Wave 2: P02 | P05 | P06 | P16
Wave 3: P07 | P08 | P09 | P14
Wave 4: P10 | P13 | P15
Wave 5: P11
Wave 6: P12 | P17 (continuous refactor, en sona)
```

## Her Faz İçin Akış

1. Faz dosyasını ve dependency fazları oku.
2. İlgili domain kodunu ve testleri oku.
3. Küçük vertical slice'lar halinde implement et.
4. Önce TEK davranışlık test yaz, kırmızı/yeşil döngüsü uygula.
5. Narrow testleri çalıştır; UI değişiminde gerçek browser smoke + screenshot.
6. Faz dosyasındaki checkbox'ları sadece gerçekten implement+verify edilen iş için işaretle.
7. `00-index.md` Progress Dashboard barını ve TODO tracker'ı güncelle.
8. Fixlenemeyen/ertelenen bulguyu `docs/todo.md` içine açıklamalı yaz.
9. Faz handoff öncesi full verification çalıştır.

## Full Verification

```bash
go test ./...
cd client && npm --cache /tmp/gameproject-npm-cache run check
git diff --check
```

## Done Criteria (v1)

- [ ] P01–P17 tüm faz done criteria karşılandı; `00-index.md` barları %100.
- [ ] Restart'ta core player/economy/world state kaybolmuyor (P01, P02, P08).
- [ ] Her value mutation transactional, idempotent, commit sonrası broadcast (P02).
- [ ] Slow client tick loop'u bloklamıyor; reconnect replay çalışıyor (P03).
- [ ] Her op'ta enforced rate limit var; abuse truth'u değiştiremiyor (P04, P16).
- [ ] Global mutex daraltıldı; aggro/AOI O(N×M) değil (P05, P15).
- [ ] Movement stop/disconnect settle; death/repair tam (P06).
- [ ] Equipment/skill effective stat ve cargo'yu doğru değiştiriyor (P07).
- [ ] CMS publish canlı runtime'a yansıyor veya dürüstçe pending_restart raporluyor (P09, P14).
- [x] Chat + party + clan MVP moderation/rate limit ile çalışıyor (P10).
- [ ] Tek tekrarlanabilir endgame gate loop'u uçtan uca çalışıyor (P11).
- [ ] Drones/ammo/honor (en az) shipped (P12).
- [ ] Release gate yeşil; simulation/load/race kanıtı var (P13).
- [ ] `Runtime` coordinator'lara bölündü; davranış regresyonu yok (P17).
- [ ] `go test ./...`, client `check`, `git diff --check` geçiyor.

## Final Rapor

Bitirince kısa ve net raporla: hangi fazlar tamamlandı, hangi backend feature'ları
gerçek UI'ya bağlandı, hangi dokümanlar güncellendi, hangi testler/browser smoke'lar
çalıştı, `docs/todo.md`'ye hangi kalemler eklendi, kalan riskler.

Sadece gerçek blocker varsa sor. Status mesajı atıp durma; tüm fazlar bitene kadar devam et.
