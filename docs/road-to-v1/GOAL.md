# Road To v1 — GOAL

Bu mesajı aktif goal olarak set et ve tüm fazlar bitene kadar özerk devam et.

## Objective

`docs/road-to-v1/` altındaki tüm fazları (P01–P17) wave sırasına göre tamamlayarak
oyunu durable, güvenli, sosyal ve DarkOrbit-benzeri bir v1'e getir. Plan kaynağı
iki rapor: `docs/code-review/game-systems-code-review.md` ve
`docs/code-review/feature-gap-analysis.md`. Index ve takip:
`docs/road-to-v1/00-index.md`.

## Durum Snapshot (Son Pause: 2026-06-26)

Pause noktası. Resume eden buradan devam etsin. Faz statü doğrusu hep
`docs/road-to-v1/00-index.md` Progress Dashboard.

### Wave bazlı statü
- Wave 1: P01 Done, P03 Done, P04 Done.
- Wave 2: P02 Done, P05 90% (deep mu narrowing → P17), P06 Done, P16 Done.
- Wave 3: P07 Done, P09 Done (lane-F + lane-G), P08/P14 başlanmadı (DB engeli kalktı).
- Wave 4+: başlanmadı (P10, P13, P15, P11, P12, P17).
- Genel v1: ~55%.

### Bu session yapılanlar (commitler, en yeni üstte)
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
2. P14: CMS runtime application (HI-02/HI-08 — publish → canlı runtime'a yansıtma
   veya dürüst `pending_restart` raporlama; `runtime_applied`/`runtime_version`/
   `published_version`).
3. P08: Durable planet/production/routes (DB engeli kalktı — durable store
   adapter'ları + idempotent settlement + recovery worker).
4. Wave 4+: P10 social, P13 release gate, P15 AOI perf.
5. Wave 5-6: P11 endgame, P12 flavor, P17 runtime decomposition (+ P05 deep mu).

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
- [ ] Chat + party + clan MVP moderation/rate limit ile çalışıyor (P10).
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
