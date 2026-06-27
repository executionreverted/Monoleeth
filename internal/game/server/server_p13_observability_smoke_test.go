package server

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"gameproject/internal/game/auth"
	"gameproject/internal/game/economy"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/realtime"
	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestPhase13RuntimeTickRecordsOTelSpans(t *testing.T) {
	spanRecorder := tracetest.NewSpanRecorder()
	tracerProvider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(spanRecorder))
	t.Cleanup(func() { _ = tracerProvider.Shutdown(t.Context()) })
	gameServer, err := New(Config{
		SessionTTL:        time.Hour,
		TickDelta:         50 * time.Millisecond,
		ContentRepository: &fakeRuntimeRepository{bundle: runtimeTestBundleWithLaserDamage(t, 35)},
		PasswordHasher:    auth.PBKDF2PasswordHasher{Iterations: 2, SaltBytes: 8, KeyBytes: 16},
		TracerProvider:    tracerProvider,
	})
	if err != nil {
		t.Fatalf("New() error = %v, want nil", err)
	}

	createResolvedRuntimeSession(t, gameServer, "p13-tick-trace@example.com", "P13 Tick Trace")
	gameServer.runtime.tickAndCollectAOIEvents()

	spans := spanRecorder.Ended()
	requireEndedSpan(t, spans, "game.runtime.tick")
	aoiSpan := requireEndedSpan(t, spans, "game.runtime.tick.aoi")
	assertServerSpanAttribute(t, aoiSpan.Attributes(), "game.runtime.sessions", int64(1))
}

func TestPhase13P15RuntimeAOITickStabilityKeepsDurationBudget(t *testing.T) {
	gameServer, _ := newTestServer(t, false)
	const sessions = 128
	for index := 0; index < sessions; index++ {
		createResolvedRuntimeSession(
			t,
			gameServer,
			fmt.Sprintf("p13-load-%03d@example.com", index),
			fmt.Sprintf("P13 Load %03d", index),
		)
	}

	startedAt := time.Now()
	gameServer.runtime.tickAndCollectAOIEvents()
	elapsed := time.Since(startedAt)
	if elapsed > 3*time.Second {
		t.Fatalf("runtime AOI tick duration = %s with %d sessions, want <= 3s", elapsed, sessions)
	}
}

func TestPhase13CommandTickEconomyMutationRaceTarget(t *testing.T) {
	gameServer, _ := newTestServer(t, false)
	resolved := createResolvedRuntimeSession(t, gameServer, "p13-race@example.com", "P13 Race")
	accountLocation, err := economy.NewItemLocation(economy.LocationKindAccountInventory, resolved.PlayerID.String())
	if err != nil {
		t.Fatalf("account location: %v", err)
	}
	for _, seed := range []struct {
		itemID   string
		quantity int64
	}{
		{itemID: "iron_ore", quantity: 20},
		{itemID: "carbon_shards", quantity: 5},
	} {
		definition, ok := gameServer.runtime.itemCatalog[foundation.ItemID(seed.itemID)]
		if !ok {
			t.Fatalf("runtime item %q missing", seed.itemID)
		}
		addTestInventoryStack(t, gameServer, resolved.PlayerID, definition, seed.quantity, accountLocation, "p13-race-"+seed.itemID)
	}

	var wg sync.WaitGroup
	wg.Add(2)
	var commandResponse realtime.CachedResponse
	start := make(chan struct{})
	ready := make(chan struct{}, 2)
	gameServer.runtime.mu.Lock()
	go func() {
		defer wg.Done()
		ready <- struct{}{}
		<-start
		for index := 0; index < 20; index++ {
			gameServer.runtime.tickAndCollectAOIEvents()
		}
	}()
	go func() {
		defer wg.Done()
		ready <- struct{}{}
		<-start
		commandResponse = gameServer.runtime.Gateway.HandleRequest(
			realtime.SessionID(resolved.SessionID.String()),
			[]byte(`{"request_id":"request-p13-race-crafting","op":"crafting.start","payload":{"recipe_id":"refined_alloy_batch"},"client_seq":1,"v":1}`),
		)
	}()
	<-ready
	<-ready
	close(start)
	gameServer.runtime.mu.Unlock()
	wg.Wait()

	if commandResponse.HasError {
		t.Fatalf("concurrent crafting.start response error = %+v, want success", commandResponse.Error)
	}
}

func requireEndedSpan(t *testing.T, spans []sdktrace.ReadOnlySpan, name string) sdktrace.ReadOnlySpan {
	t.Helper()
	for _, span := range spans {
		if span.Name() == name {
			return span
		}
	}
	t.Fatalf("missing ended span %q in %+v", name, spanNames(spans))
	return nil
}

func spanNames(spans []sdktrace.ReadOnlySpan) []string {
	names := make([]string, 0, len(spans))
	for _, span := range spans {
		names = append(names, span.Name())
	}
	return names
}

func assertServerSpanAttribute(t *testing.T, attributes []attribute.KeyValue, key string, want int64) {
	t.Helper()
	for _, attr := range attributes {
		if string(attr.Key) == key {
			if got := attr.Value.AsInt64(); got != want {
				t.Fatalf("span attr %s = %d, want %d", key, got, want)
			}
			return
		}
	}
	t.Fatalf("span missing attr %s in %+v", key, attributes)
}
