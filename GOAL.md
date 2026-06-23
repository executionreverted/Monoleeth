Bu mesajı aktif goal olarak oluştur ve tüm işler bitene kadar özerk devam et.

Objective:
Bu projede `AGENTS.md` talimatlarına uyarak, `docs/plans/ui-implementation/` altındaki tüm UI implementation fazlarını sırayla tamamla. Amaç, mail/password hesap sistemiyle başlayan gerçek authenticated game client akışını kurmak ve daha önce implement edilmiş tüm backend oyun feature’larını browser arayüzünden gerçek server state’iyle çalışır hale getirmek.

Çalışma kuralları:
- Her task/phase öncesi tüm bağlamı tekrar oku ve hafızanı tazele:
  - `AGENTS.md`
  - `docs/plans/ui-implementation/00-index.md`
  - ilgili UI implementation phase dosyaları
  - `docs/todo.md`
  - gerekiyorsa ilgili backend module spec dosyaları under `docs/plans/modules/`
  - gerekiyorsa mimari/progression/world design dokümanları
  - `output/mockups/final-mockup.png`
  - `git status --short`
  - mevcut diff
- Eski `docs/roadmap/` fazları tarihsel kayıt olarak kalır; yeni işletim rehberi `docs/plans/ui-implementation/` dosyalarıdır.
- Mail/password auth gerçek olsun: password hash, session lifecycle, logout, session expiry, admin seed ve WebSocket session resolution server-owned olmalı.
- Browser client default olarak fake/demo state göstermemeli. Demo veya fixture akışı sadece açıkça işaretlenmiş test/dev modunda olabilir.
- Her UI değeri gerçek server snapshot/event/query’den gelmeli; server verisi yoksa panel boş, loading, locked veya disabled state göstermeli.
- Client sadece intent gönderir. Player id, damage, XP, loot, wallet amount, craft completion, market totals, planet ownership, current-map visibility, radar/stealth detection, or hidden-world truth client payload’ından alınmaz.
- Kullanılan library/framework/SDK/API/CLI/cloud dokümantasyonu gerekiyorsa Context7 MCP ile güncel doküman oku.
- Context compact olursa veya bağlam daralırsa işe devam etmeden önce tekrar `AGENTS.md`, UI implementation planları, `docs/todo.md`, `git status`, diff ve ilgili backend module docs oku.

Her phase için akış:
1. Phase dosyasını ve dependency phase’leri oku.
2. İlgili backend module specs’i ve mevcut kodu oku.
3. Kısa implementation planı çıkar.
4. Küçük vertical slice’lar halinde implement et.
5. Önce test yaz veya mevcut testi kırmızı/yeşil döngüye sok.
6. Narrow testleri çalıştır.
7. UI değişikliklerinde gerçek browser smoke ve screenshot kontrolü yap.
8. Server/client contract drift varsa aynı phase içinde dokümanı ve testleri güncelle.
9. Fixlenemeyen veya bilinçli ertelenen bulguları `docs/todo.md` içine açıklamalı yaz.
10. İlgili UI implementation phase checklist’ini sadece gerçekten implement edilip verify edilen işler için güncelle.
11. Full verification çalıştır:
    - `go test ./...`
    - `npm --cache /tmp/gameproject-npm-cache run check` in `client/`
    - `git diff --check`
12. Staged diff’i kontrol et, sadece ilgili dosyaları stage et.
13. Temiz ve minimal commit/merge yap.
14. Bir sonraki UI implementation phase için context tazele ve devam et.

Done criteria:
- Mail/password hesap sistemi, admin seed, session endpointleri ve authenticated WebSocket akışı çalışacak.
- Concrete Go game server transport browser client ile konuşacak.
- Default client açılışında fake/demo HP, cargo, wallet, quest, planet, loot veya NPC state’i kalmayacak.
- Backend’de implement edilmiş oyun feature’ları arayüzden gerçek command/query/event akışıyla kullanılabilecek:
  - world/AOI/movement
  - combat/loot/death/repair
  - progression/rank/roles/skills
  - ships/hangar/loadout/modules/stats
  - inventory/cargo/wallet/ledger
  - crafting
  - quest board/progress/rewards
  - discovery/scanner/radar-stealth visibility/intel/coordinate items
  - planet claim/production/buildings
  - automation routes
  - market/auction/premium
  - observability/admin/release gates
- UI `output/mockups/final-mockup.png` kararına yaklaşacak; görsel kararlar gerçek data states ile uyumlu olacak.
- Tüm phase dosyaları güncellenmiş olacak.
- `go test ./...` başarılı olacak.
- `client` check/smoke başarılı olacak.
- `git diff --check` başarılı olacak.
- Son final mesajında:
  - hangi UI implementation phase’lerinin tamamlandığını,
  - hangi backend feature’larının gerçek UI’ya bağlandığını,
  - hangi dokümanların güncellendiğini,
  - hangi testlerin/browser smoke kontrollerinin çalıştığını,
  - `docs/todo.md` içine hangi kalemlerin eklendiğini,
  - kalan riskleri
  kısa ve net raporla.

Sadece gerçek blocker varsa bana sor. Status mesajı atıp durma; tüm tasklar bitene kadar devam et. Karar vermeni gerektiren konularda oyun dökümanlarını oku; hala sıkıntı varsa DarkOrbit ve Dark Forest ETH gibi oyunların mekaniklerinden örnek al, ama kendi inisiyatifinle doldurduğun alanları ayrıca dokümante et ki neyin varsayım olduğunu bilelim.
