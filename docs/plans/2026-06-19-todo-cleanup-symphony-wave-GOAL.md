# TODO Cleanup Symphony Wave Goal

Bu mesajı aktif goal olarak oluştur ve iş bitene kadar bu goal kapsamında devam et.

## Objective

`docs/plans/2026-06-19-todo-cleanup-symphony-wave.md` planını Symphony kullanarak uygula. Amaç, `docs/todo.md` içindeki gerçekten yapılabilir açık işleri küçük, conflict-safe wave'ler halinde kapatmak; durable repository/outbox veya henüz eksik runtime contract gerektiren işleri açık blocker olarak bırakmak.

## Required Context

Başlamadan önce oku:

```text
AGENTS.md
docs/todo.md
docs/symphony-operating-model.md
docs/symphony-worker-rules.md
docs/plans/2026-06-19-todo-cleanup-symphony-wave.md
docs/plans/ui-implementation/00-index.md
```

İlgili task'a göre ayrıca oku:

```text
docs/roadmap/06-death-repair-crafting.md
docs/roadmap/07-quest-board-guided-progression.md
docs/roadmap/08-world-discovery-planets-intel.md
docs/roadmap/09-planet-production-routes.md
docs/roadmap/10-market-auction-premium.md
docs/roadmap/12-observability-balancing-release-gates.md
docs/plans/modules/00-index.md
```

## Execution Rules

- Ana Codex oturumu Symphony project manager olsun.
- Worker prompt'larında `AGENTS.md` veya `docs/symphony-operating-model.md` okutma.
- Worker'lar `docs/symphony-worker-rules.md` okusun, subagent spawnlamasın, Symphony queue yönetmesin, commit atmasın.
- Her wave'de 2-5 bağımsız task aç; write set çakışıyorsa sequential ilerle.
- Her worker diff'ini apply etmeden önce incele.
- Diff'leri ana repoya tek tek uygula, narrow testleri çalıştır, sonra commit at.
- `docs/todo.md` sadece gerçekten implement edilip verify edilen işler için güncellensin.
- Durable/outbox/DB transaction/row-lock gerektiren açıkları küçük patch diye kapatmaya çalışma.

## First Wave

Plan dosyasındaki Wave 1 ile başla:

1. Indexed wallet ledger reference lookup.
2. Discovery claim retry repair.
3. Owner-checked route operation wrappers.
4. Browser fake-count guard rails.

Bu dört task'ın write setleri büyük ölçüde ayrık. Yine de her tamamlanan worker diff'ini ayrı review/apply/test/commit döngüsünden geçir.

## Verification

Her applied task sonrası plan dosyasındaki narrow testleri çalıştır. Wave sonunda:

```bash
go test ./...
cd client && npm --cache /tmp/gameproject-npm-cache run check
git diff --check
```

Client dosyası değişmediyse full client check'i wave sonunda yine çalıştır; Phase 10 hardening gate bunu istiyor.

## Final Report

Final mesajında kısa raporla:

- hangi Symphony task'ları açıldı ve tamamlandı
- hangi wave/task'lar implement edildi
- hangi dosyalar değişti
- hangi `docs/todo.md` maddeleri kapandı veya açık kaldı
- hangi testler geçti
- kalan blocker/riskler
