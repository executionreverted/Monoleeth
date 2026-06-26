package server

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"gameproject/internal/game/auth"
	gamecontent "gameproject/internal/game/content"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/modules"
	"gameproject/internal/game/realtime"
)

func runtimeCatalogItemDisplay(t *testing.T, runtime *Runtime, itemID string) string {
	t.Helper()
	raw, err := runtime.handleContentCatalog(realtime.CommandContext{}, realtime.RequestEnvelope{Payload: json.RawMessage(`{}`)})
	if err != nil {
		t.Fatalf("handleContentCatalog() error = %v", err)
	}
	var payload contentCatalogResponsePayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("decode content catalog: %v", err)
	}
	return requireRuntimeProjectedItem(t, payload.ContentCatalog, itemID).Display.DisplayName
}

func TestRuntimeApplyPublishedContentReflectsSafeReloadInCatalog(t *testing.T) {
	v1 := runtimeTestBundle(t)
	v2 := runtimeTestBundleWithItemShipShopProof(t)
	repo := &fakeRuntimeRepository{bundle: v1}

	runtime, err := NewRuntime(RuntimeConfig{
		WorldID:           foundation.WorldID("world-1"),
		ContentRepository: repo,
	})
	if err != nil {
		t.Fatalf("NewRuntime() error = %v", err)
	}

	if display := runtimeCatalogItemDisplay(t, runtime, "raw_ore"); display == "Auric Ore Bundle" {
		t.Fatalf("v1 catalog already shows published name %q before apply", display)
	}

	repo.bundle = v2
	plan := gamecontent.RuntimeApplyPlan{Class: gamecontent.ApplyClassSafeReload}
	outcome, err := runtime.applyPublishedContent(context.Background(), plan)
	if err != nil {
		t.Fatalf("applyPublishedContent() error = %v", err)
	}
	if !outcome.Applied || outcome.PendingRestart {
		t.Fatalf("outcome = %+v, want applied safe-reload", outcome)
	}
	if outcome.RuntimeVersion == "" {
		t.Fatalf("runtime_version empty after safe apply, want reloaded version")
	}

	if display := runtimeCatalogItemDisplay(t, runtime, "raw_ore"); display != "Auric Ore Bundle" {
		t.Fatalf("catalog item display = %q, want %q reflected without restart", display, "Auric Ore Bundle")
	}
}

func TestRuntimeApplyPublishedContentReportsPendingRestartForRestartRequired(t *testing.T) {
	v1 := runtimeTestBundle(t)
	v2 := runtimeTestBundleWithItemShipShopProof(t)
	repo := &fakeRuntimeRepository{bundle: v1}

	runtime, err := NewRuntime(RuntimeConfig{
		WorldID:           foundation.WorldID("world-1"),
		ContentRepository: repo,
	})
	if err != nil {
		t.Fatalf("NewRuntime() error = %v", err)
	}
	previousRuntimeVersion := runtime.contentCatalogVersion

	repo.bundle = v2
	plan := gamecontent.RuntimeApplyPlan{Class: gamecontent.ApplyClassRestartRequired}
	outcome, err := runtime.applyPublishedContent(context.Background(), plan)
	if err != nil {
		t.Fatalf("applyPublishedContent() error = %v", err)
	}
	if outcome.Applied || !outcome.PendingRestart {
		t.Fatalf("outcome = %+v, want pending restart", outcome)
	}
	if outcome.RuntimeVersion != previousRuntimeVersion {
		t.Fatalf("runtime_version = %q, want previous %q (projection must not drift)", outcome.RuntimeVersion, previousRuntimeVersion)
	}

	if display := runtimeCatalogItemDisplay(t, runtime, "raw_ore"); display == "Auric Ore Bundle" {
		t.Fatalf("catalog drifted to published name %q despite pending restart", display)
	}
}

func TestRuntimeApplyPublishedContentScrubsNoHiddenFields(t *testing.T) {
	v2 := runtimeTestBundleWithItemShipShopProof(t)
	repo := &fakeRuntimeRepository{bundle: runtimeTestBundle(t)}

	runtime, err := NewRuntime(RuntimeConfig{
		WorldID:           foundation.WorldID("world-1"),
		ContentRepository: repo,
	})
	if err != nil {
		t.Fatalf("NewRuntime() error = %v", err)
	}
	repo.bundle = v2
	if _, err := runtime.applyPublishedContent(context.Background(), gamecontent.RuntimeApplyPlan{Class: gamecontent.ApplyClassSafeReload}); err != nil {
		t.Fatalf("applyPublishedContent() error = %v", err)
	}
	raw, err := runtime.handleContentCatalog(realtime.CommandContext{}, realtime.RequestEnvelope{Payload: json.RawMessage(`{}`)})
	if err != nil {
		t.Fatalf("handleContentCatalog() error = %v", err)
	}
	if leaked := string(raw); strings.Contains(leaked, "loot_table") || strings.Contains(leaked, "spawn_area") || strings.Contains(leaked, "metadata_schema") {
		t.Fatalf("applied catalog leaked hidden fields: %s", leaked)
	}
}

func TestRuntimeActiveEquippedModulesReportsEquippedModuleDefinition(t *testing.T) {
	gameServer, httpServer := newTestServer(t, false)
	defer httpServer.Close()

	result, err := gameServer.runtime.Auth.Register(context.Background(), auth.RegisterInput{
		Email:    "equipped-safety@example.com",
		Password: "correct-password",
		Callsign: "Equipped Safety",
	})
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	if err := gameServer.runtime.ensurePlayerSession(result.Session); err != nil {
		t.Fatalf("ensurePlayerSession() error = %v", err)
	}

	laserInstanceID := starterModuleInstanceID(t, gameServer.runtime, result.Session.PlayerID, "laser_alpha_t1")
	if _, err := gameServer.runtime.Loadout.SaveLoadout(modules.SaveLoadoutInput{
		LoadoutID: "publish-safety-offensive-1",
		PlayerID:  result.Session.PlayerID,
		ShipID:    gamecontent.DefaultStarterShipID,
		Name:      "Publish Safety Offensive",
		SlotAssignments: modules.SlotAssignments{
			modules.ModuleSlotOffensive1: laserInstanceID,
		},
	}); err != nil {
		t.Fatalf("SaveLoadout() error = %v", err)
	}
	if _, err := gameServer.runtime.Loadout.ApplyLoadout(modules.ApplyLoadoutInput{
		PlayerID:  result.Session.PlayerID,
		LoadoutID: "publish-safety-offensive-1",
		RequestID: "apply-publish-safety-1",
	}); err != nil {
		t.Fatalf("ApplyLoadout() error = %v", err)
	}

	refs, err := gameServer.runtime.ActiveEquippedModules(context.Background())
	if err != nil {
		t.Fatalf("ActiveEquippedModules() error = %v", err)
	}
	found := false
	for _, ref := range refs {
		if ref.ModuleID == "laser_alpha_t1" && ref.PlayerID == string(result.Session.PlayerID) {
			found = true
		}
	}
	if !found {
		t.Fatalf("equipped refs = %+v, want laser_alpha_t1 reported for registered player", refs)
	}
}
