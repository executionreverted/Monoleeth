package server

import (
	"testing"

	"gameproject/internal/game/foundation"
	"gameproject/internal/game/realtime"
	"gameproject/internal/game/world"
	worldmaps "gameproject/internal/game/world/maps"
)

func TestIntelShareRejectsPlanetOnlyKnownByAnotherPlayerWithoutReceiverMutation(t *testing.T) {
	gameServer, _ := newTestServer(t, false)
	knower := createResolvedRuntimeSession(t, gameServer, "intel-share-knower@example.com", "Intel Knower")
	sender := createResolvedRuntimeSession(t, gameServer, "intel-share-unauthorized@example.com", "Intel Sender")
	receiver := createResolvedRuntimeSession(t, gameServer, "intel-share-unauthorized-receiver@example.com", "Intel Receiver")
	planetID := foundation.PlanetID("planet-intel-share-unauthorized")

	seedKnownClaimPlanetForTest(t, gameServer, knower.PlayerID, planetID, worldmaps.StarterMapID, world.Vec2{X: 1200, Y: 1300}, 2)

	response := gameServer.runtime.Gateway.HandleRequest(
		realtime.SessionID(sender.SessionID.String()),
		[]byte(`{"request_id":"request-intel-share-unauthorized","op":"intel.share","payload":{"planet_id":"`+planetID.String()+`","to_player_id":"`+receiver.PlayerID.String()+`"},"client_seq":1,"v":1}`),
	)
	if !response.HasError || response.Error.Error.Code != foundation.CodeNotFound {
		t.Fatalf("unauthorized intel.share response = %+v, want safe not found", response)
	}
	if _, ok, err := gameServer.runtime.Discovery.PlayerPlanetIntel(sender.PlayerID, planetID); err != nil || ok {
		t.Fatalf("sender discovery intel after rejected share ok=%v err=%v, want no mutation", ok, err)
	}
	if _, ok, err := gameServer.runtime.Intel.PlayerPlanetIntel(sender.PlayerID, planetID); err != nil || ok {
		t.Fatalf("sender runtime intel after rejected share ok=%v err=%v, want no mutation", ok, err)
	}
	if _, ok, err := gameServer.runtime.Discovery.PlayerPlanetIntel(receiver.PlayerID, planetID); err != nil || ok {
		t.Fatalf("receiver discovery intel after rejected share ok=%v err=%v, want no mutation", ok, err)
	}
	if _, ok, err := gameServer.runtime.Intel.PlayerPlanetIntel(receiver.PlayerID, planetID); err != nil || ok {
		t.Fatalf("receiver runtime intel after rejected share ok=%v err=%v, want no mutation", ok, err)
	}
	gameServer.runtime.mu.Lock()
	receiverEvents := len(gameServer.runtime.queuedEvents[receiver.SessionID])
	gameServer.runtime.mu.Unlock()
	if receiverEvents != 0 {
		t.Fatalf("receiver queued events after rejected share = %d, want none", receiverEvents)
	}
}
