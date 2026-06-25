# Road To v1 — GOAL

Bu mesajı aktif goal olarak set et ve tüm fazlar bitene kadar özerk devam et.

## Objective

`docs/road-to-v1/` altındaki tüm fazları (P01–P17) wave sırasına göre tamamlayarak
oyunu durable, güvenli, sosyal ve DarkOrbit-benzeri bir v1'e getir. Plan kaynağı
iki rapor: `docs/code-review/game-systems-code-review.md` ve
`docs/code-review/feature-gap-analysis.md`. Index ve takip:
`docs/road-to-v1/00-index.md`.

## Çalışma Kuralları

- Her faz/iş öncesi context tazele: `AGENTS.md`, `docs/road-to-v1/00-index.md`,
  ilgili faz dosyası, `docs/todo.md`, `git status --short`, mevcut diff.
- `AGENTS.md` coding kurallarına uy: server-authoritative, küçük dosyalar, domain
  isimleri, `lock -> validate -> mutate -> ledger/event -> commit -> broadcast`,
  idempotency key, no monolith, no fake state.
- Client sadece intent gönderir; player id/damage/loot/wallet/ownership client'tan alınmaz.
- DB/pgx/Redis/NATS/library syntax gerekiyorsa Context7 MCP kullan.
- Symphony/orchestration kodunu gameplay domain kodundan ayrı tut.

## Smoke Test Kuralı

Her smoke/e2e test yalnızca TEK bir davranışı assert eder. Uzun mega-smoke yazma;
N davranış için N kısa test yaz.

## Paralel Çalışma (Symphony)

- `[P:wave-N/lane-X]` etiketli tasklar aynı wave içinde paralel Symphony agent'larına dağıtılabilir.
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
