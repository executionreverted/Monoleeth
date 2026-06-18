Bu mesajı aktif goal olarak oluştur ve tüm işler bitene kadar özerk devam et.

Objective:
Bu projede `AGENTS.md` talimatlarına uyarak, `docs/plans/modules/` altındaki son module planına kadar tüm phase/task implementasyonlarını tamamla, verify et, testleri çalıştır, roadmap ve todo kayıtlarını güncelle.

Çalışma kuralları:
- Her task/phase öncesi tüm bağlamı tekrar oku ve hafızanı tazele:
  - `AGENTS.md`
  - `docs/roadmap/00-index.md`
  - ilgili roadmap phase dosyaları
  - `docs/plans/modules/00-index.md`
  - ilgili module spec dosyaları
  - gerekiyorsa mimari/progression/world design dokümanları
  - `git status --short`
  - mevcut diff
- Kodlamaları Symphony kullanarak orkestrate et.
- Symphony-managed worker workspace içindeysen `AGENTS.md` içindeki Symphony notuna uy ve `docs/symphony-worker-rules.md` dosyasını işletim rehberi kabul et.
- Dokümana ve roadmap phase sırasına uy.
- Phase dependency’lerini bozma; bozmak zorunda kalırsan riski finalde ve ilgili todo’da belirt.
- Client-trust, economy ledger, idempotency, visibility/fog, transaction, rate-limit ve server-authoritative kurallarını ihlal etme.
- Kullanılan library/framework/SDK/API/CLI/cloud dokümantasyonu gerekiyorsa Context7 MCP ile güncel doküman oku.
- Context compact olursa veya bağlam daralırsa işe devam etmeden önce tekrar `AGENTS.md`, roadmap/module docs, `git status`, diff ve mevcut todo’ları oku.
- Amacımız `/Users/canersevince/gameproject/output/mockups/final-mockup.png` tasarımındaki HUD'u birebir kopyalamak. Oyun için objeler ve diğer assetler (iconlar - map arkaplanı vs.) subagent spawnlatıp asset ürettirebilirsin. Tek kriter mockup dosyasında ilgili alanla birebir / çok benzer olması assetin. Olabildiğince ona yakın tutacağız. Ayrıca @output/assets/hud-svg klasöründe icon-marker vs gibi şeyler var. İşine yararsa kullanırsın.

Her phase için akış:
1. Phase context’ini ve ilgili module spec’lerini oku.
2. Kısa implementation planı çıkar.
3. Symphony ile işi orkestrate et.
4. Küçük vertical slice’lar halinde implement et.
5. Gerekli testleri ekle/güncelle.
6. Narrow testleri çalıştır.
7. Phase sonrası subagentlarla code review yaptır.
8. Subagent bulgularından fixlenebilen bugları merge/commit öncesi hemen fixle.
9. Hemen fixlenemeyen veya bilinçli ertelenen bulguları `docs/todo.md` dosyasına, ileride fixlemek için yeterli açıklama ve bağlamla yaz.
10. İlgili roadmap phase checklist’ini sadece gerçekten implement edilip verify edilen işler için güncelle.
11. Full verification çalıştır:
    - `go test ./...`
    - `git diff --check`
12. Staged diff’i kontrol et, sadece ilgili dosyaları stage et.
13. Temiz ve minimal commit/merge yap.
14. Merge sonrası bir sonraki phase için plan yap ve devam et.

Done criteria:
- `docs/plans/modules/` altındaki son module planına kadar gerekli tüm phase/task’lar implement edilmiş olacak.
- Tüm ilgili roadmap phase dosyaları güncellenmiş olacak.
- Her phase için testler eklenmiş/güncellenmiş olacak.
- Her phase sonrası subagent code review yapılmış olacak.
- Fixlenebilen review bulguları merge öncesi fixlenmiş olacak.
- Fixlenemeyen review bulguları `docs/todo.md` içine açıklamalı yazılmış olacak.
- `go test ./...` başarılı olacak.
- `git diff --check` başarılı olacak.
- Son final mesajında:
  - hangi phase’lerin tamamlandığını,
  - hangi dokümanların güncellendiğini,
  - hangi testlerin çalıştığını,
  - hangi review bulgularının fixlendiğini,
  - `docs/todo.md` içine hangi kalemlerin eklendiğini,
  - kalan riskleri
  kısa ve net raporla.
- Her asset - ui generation öncesi birebir benzer asset üretmek için final-mockup.png incele, arayüz elementleri ve hud kodlarken tekrar incele. Unutma. HUD Birebir, assetler 90% benzer olmalı.

Sadece gerçek blocker varsa bana sor. Status mesajı atıp durma; tüm tasklar bitene kadar devam et. Karar vermeni gerektiren konularda oyun dökümanlarını oku - hala sıkıntı varsa darkorbit ve dark forest eth gibi oyunların mekaniklerinden örnek al, böyle kendi insiyatifinle doldurduğun alanlarla alakalı bir md dosyası oluştur ki neyi kendin uydurduğunu bilelim.
