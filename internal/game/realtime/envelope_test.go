package realtime

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"gameproject/internal/game/foundation"
)

func TestDecodeRequestEnvelopeRejectsMissingRequestID(t *testing.T) {
	_, err := DecodeRequestEnvelope([]byte(`{"op":"move_to","payload":{"x":10,"y":20},"client_seq":7,"v":1}`))

	requireInvalidPayload(t, err)
}

func TestDecodeRequestEnvelopeRejectsMissingOp(t *testing.T) {
	_, err := DecodeRequestEnvelope([]byte(`{"request_id":"request-1","payload":{"x":10,"y":20},"client_seq":7,"v":1}`))

	requireInvalidPayload(t, err)
}

func TestDecodeRequestEnvelopeRejectsUnsupportedProtocolVersion(t *testing.T) {
	_, err := DecodeRequestEnvelope([]byte(`{"request_id":"request-1","op":"move_to","payload":{"x":10,"y":20},"client_seq":7,"v":999}`))

	requireInvalidPayload(t, err)
}

func TestDecodeRequestEnvelopeRejectsInvalidOrMissingPayload(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{
			name: "missing payload",
			body: `{"request_id":"request-1","op":"move_to","client_seq":7,"v":1}`,
		},
		{
			name: "null payload",
			body: `{"request_id":"request-1","op":"move_to","payload":null,"client_seq":7,"v":1}`,
		},
		{
			name: "array payload",
			body: `{"request_id":"request-1","op":"move_to","payload":[],"client_seq":7,"v":1}`,
		},
		{
			name: "string payload",
			body: `{"request_id":"request-1","op":"move_to","payload":"bad","client_seq":7,"v":1}`,
		},
		{
			name: "malformed envelope",
			body: `{"request_id":"request-1","op":"move_to","payload":`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := DecodeRequestEnvelope([]byte(tt.body))
			requireInvalidPayload(t, err)
		})
	}
}

func TestRequestEnvelopeValidateRejectsInvalidRawPayload(t *testing.T) {
	envelope := NewRequestEnvelope(
		foundation.RequestID("request-1"),
		OperationMoveTo,
		json.RawMessage(`{`),
		7,
	)

	err := envelope.Validate()

	requireInvalidPayload(t, err)
}

func TestDecodeRequestEnvelopeAcceptsRegisteredPhase04Operation(t *testing.T) {
	envelope, err := DecodeRequestEnvelope([]byte(`{"request_id":"request-1","op":"move_to","payload":{"x":10,"y":20},"client_seq":7,"v":1}`))
	if err != nil {
		t.Fatalf("decode valid request envelope: %v", err)
	}

	if envelope.RequestID != foundation.RequestID("request-1") {
		t.Fatalf("request id = %q, want request-1", envelope.RequestID)
	}
	if envelope.Op != OperationMoveTo {
		t.Fatalf("op = %q, want %q", envelope.Op, OperationMoveTo)
	}
	if got := string(envelope.Payload); got != `{"x":10,"y":20}` {
		t.Fatalf("payload = %s, want move payload", got)
	}
}

func TestDecodeRequestEnvelopeAcceptsCombatUseSkillOperation(t *testing.T) {
	envelope, err := DecodeRequestEnvelope([]byte(`{"request_id":"request-1","op":"combat.use_skill","payload":{"skill_id":"basic_laser","target_id":"npc-1","client_timestamp":9999999999999},"client_seq":7,"v":1}`))
	if err != nil {
		t.Fatalf("decode combat request envelope: %v", err)
	}
	if envelope.Op != OperationCombatUseSkill {
		t.Fatalf("op = %q, want %q", envelope.Op, OperationCombatUseSkill)
	}
}

func TestDecodeRequestEnvelopeAcceptsShopCatalogOperation(t *testing.T) {
	cases := []struct {
		name string
		body string
		want Operation
	}{
		{
			name: "catalog",
			body: `{"request_id":"request-shop-catalog","op":"shop.catalog","payload":{},"client_seq":11,"v":1}`,
			want: OperationShopCatalog,
		},
		{
			name: "buy product",
			body: `{"request_id":"request-shop-buy","op":"shop.buy_product","payload":{"product_id":"product_module_laser_alpha_t1","quantity":1},"client_seq":12,"v":1}`,
			want: OperationShopBuyProduct,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			envelope, err := DecodeRequestEnvelope([]byte(tc.body))
			if err != nil {
				t.Fatalf("decode shop request envelope: %v", err)
			}
			if envelope.Op != tc.want {
				t.Fatalf("op = %q, want %q", envelope.Op, tc.want)
			}
		})
	}
}

func TestDecodeRequestEnvelopeAcceptsHangarAndLoadoutMutationOperations(t *testing.T) {
	cases := []struct {
		name string
		body string
		want Operation
	}{
		{
			name: "activate ship",
			body: `{"request_id":"request-hangar-activate","op":"hangar.activate_ship","payload":{"ship_id":"starter"},"client_seq":6,"v":1}`,
			want: OperationHangarActivateShip,
		},
		{
			name: "equip module",
			body: `{"request_id":"request-loadout-equip","op":"loadout.equip_module","payload":{"slot_id":"offensive_1","item_instance_id":"laser_alpha_t1-instance-2"},"client_seq":7,"v":1}`,
			want: OperationLoadoutEquipModule,
		},
		{
			name: "unequip module",
			body: `{"request_id":"request-loadout-unequip","op":"loadout.unequip_module","payload":{"slot_id":"offensive_1"},"client_seq":8,"v":1}`,
			want: OperationLoadoutUnequipModule,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			envelope, err := DecodeRequestEnvelope([]byte(tc.body))
			if err != nil {
				t.Fatalf("decode loadout request envelope: %v", err)
			}
			if envelope.Op != tc.want {
				t.Fatalf("op = %q, want %q", envelope.Op, tc.want)
			}
		})
	}
}

func TestDecodeRequestEnvelopeAcceptsStealthToggleOperation(t *testing.T) {
	envelope, err := DecodeRequestEnvelope([]byte(`{"request_id":"request-stealth-toggle","op":"stealth.toggle","payload":{"enabled":true},"client_seq":9,"v":1}`))
	if err != nil {
		t.Fatalf("decode stealth request envelope: %v", err)
	}
	if envelope.Op != OperationStealthToggle {
		t.Fatalf("op = %q, want %q", envelope.Op, OperationStealthToggle)
	}
}

func TestDecodeRequestEnvelopeAcceptsPortalEnterOperation(t *testing.T) {
	envelope, err := DecodeRequestEnvelope([]byte(`{"request_id":"request-portal-enter","op":"portal.enter","payload":{"portal_id":"east_gate"},"client_seq":10,"v":1}`))
	if err != nil {
		t.Fatalf("decode portal request envelope: %v", err)
	}
	if envelope.Op != OperationPortalEnter {
		t.Fatalf("op = %q, want %q", envelope.Op, OperationPortalEnter)
	}
	if EventMapTransferStarted != "map.transfer_started" ||
		EventMapTransferCompleted != "map.transfer_completed" ||
		EventMapTransferFailed != "map.transfer_failed" ||
		EventPlayerProtection != "player.protection_updated" {
		t.Fatalf("map transfer/protection event constants = %q/%q/%q/%q", EventMapTransferStarted, EventMapTransferCompleted, EventMapTransferFailed, EventPlayerProtection)
	}
}

func TestDecodeRequestEnvelopeAcceptsPortalEnterOperationAndTransferEvents(t *testing.T) {
	envelope, err := DecodeRequestEnvelope([]byte(`{"request_id":"request-portal-enter","op":"portal.enter","payload":{"portal_id":"east_gate"},"client_seq":10,"v":1}`))
	if err != nil {
		t.Fatalf("decode portal request envelope: %v", err)
	}
	if envelope.Op != OperationPortalEnter {
		t.Fatalf("op = %q, want %q", envelope.Op, OperationPortalEnter)
	}
	if EventMapTransferStarted != "map.transfer_started" ||
		EventMapTransferCompleted != "map.transfer_completed" ||
		EventMapTransferFailed != "map.transfer_failed" ||
		EventPlayerProtection != "player.protection_updated" {
		t.Fatalf("map transfer/protection event constants = %q/%q/%q/%q", EventMapTransferStarted, EventMapTransferCompleted, EventMapTransferFailed, EventPlayerProtection)
	}
}

func TestOperationRegistryRejectsUnimplementedBrowserMutationContracts(t *testing.T) {
	disallowed := []Operation{
		Operation("crafting.start"),
		Operation("crafting.complete"),
		Operation("crafting.cancel"),
		Operation("inventory.move"),
		Operation("progression.unlock_skill"),
		Operation("progression.respec_skills"),
		Operation("discovery.claim_planet"),
		Operation("planet.building_build"),
		Operation("planet.building_upgrade"),
		Operation("route.create"),
		Operation("route.update"),
		Operation("route.enable"),
		Operation("route.disable"),
		Operation("route.settle"),
		Operation("intel.share"),
		Operation("intel.coordinate_item_create"),
		Operation("intel.coordinate_item_use"),
		Operation("coordinate_scroll.create"),
		Operation("coordinate_scroll.use"),
		Operation("mail.send"),
		Operation("social.friend_request"),
		Operation("social.party_invite"),
	}

	registry := OperationRegistry()
	for index, operation := range disallowed {
		if _, ok := registry[operation]; ok {
			t.Fatalf("operation %q is registered in realtime registry, want absent until its server-owned contract exists", operation)
		}
		if _, ok := LookupOperation(operation); ok {
			t.Fatalf("operation %q is accepted by LookupOperation, want rejected until its server-owned contract exists", operation)
		}
		body := fmt.Sprintf(
			`{"request_id":"request-unimplemented-%d","op":%q,"payload":{},"client_seq":7,"v":1}`,
			index,
			operation,
		)
		_, err := DecodeRequestEnvelope([]byte(body))
		requireInvalidPayload(t, err)
	}
}

func TestOperationRegistryRejectsClientAuthoredQuestProgressOperations(t *testing.T) {
	allowedQuestClientOperations := map[Operation]struct{}{
		Operation("quest.board"):        {},
		Operation("quest.accept"):       {},
		Operation("quest.progress"):     {},
		Operation("quest.reroll"):       {},
		Operation("quest.claim_reward"): {},
	}
	for operation := range OperationRegistry() {
		op := string(operation)
		if !isQuestOperationName(op) {
			continue
		}
		if _, ok := allowedQuestClientOperations[operation]; !ok {
			t.Fatalf("registered quest client operation %q is not explicitly allowed; quest progress must come from server events", op)
		}
	}

	disallowed := []Operation{
		Operation("quest.progress_objective"),
		Operation("quest.set_progress"),
		Operation("quest.complete_objective"),
		Operation("quest_progress"),
	}
	for index, operation := range disallowed {
		if _, ok := LookupOperation(operation); ok {
			t.Fatalf("operation %q is registered, want rejected", operation)
		}
		body := fmt.Sprintf(
			`{"request_id":"request-quest-progress-%d","op":%q,"payload":{"player_id":"player-1","quest_id":"quest-1","progress":{"current":999,"completed":true}},"client_seq":7,"v":1}`,
			index,
			operation,
		)
		_, err := DecodeRequestEnvelope([]byte(body))
		requireInvalidPayload(t, err)
	}
}

func isQuestOperationName(op string) bool {
	return op == "quest" || strings.HasPrefix(op, "quest.") || strings.HasPrefix(op, "quest_")
}

func TestEventEnvelopeMarshalsWithoutHiddenInternalFields(t *testing.T) {
	envelope := NewEventEnvelope(
		foundation.EventID("event-1"),
		EventAOIEntityEntered,
		json.RawMessage(`{"entity_id":"entity-1","kind":"npc","x":10,"y":20}`),
		182736123,
		99122,
	)

	payload, err := json.Marshal(envelope)
	if err != nil {
		t.Fatalf("json marshal event envelope: %v", err)
	}

	got := string(payload)
	want := `{"event_id":"event-1","type":"aoi.entity_entered","payload":{"entity_id":"entity-1","kind":"npc","x":10,"y":20},"server_time":182736123,"seq":99122,"v":1}`
	if got != want {
		t.Fatalf("event envelope JSON = %s, want %s", got, want)
	}

	for _, leaked := range []string{"internal", "hidden", "seed", "unfiltered"} {
		if strings.Contains(got, leaked) {
			t.Fatalf("event envelope leaked %q in %s", leaked, got)
		}
	}
}

func requireInvalidPayload(t *testing.T, err error) {
	t.Helper()

	if err == nil {
		t.Fatal("expected invalid payload error, got nil")
	}
	if !foundation.IsCode(err, foundation.CodeInvalidPayload) {
		t.Fatalf("error code mismatch: got %v, want %s", err, foundation.CodeInvalidPayload)
	}
}
