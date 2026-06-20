package server

import (
	"encoding/json"

	"gameproject/internal/game/realtime"
)

type stealthToggleIntent struct {
	Enabled *bool `json:"enabled"`
}

func (runtime *Runtime) handleStealthToggle(ctx realtime.CommandContext, request realtime.RequestEnvelope) (json.RawMessage, error) {
	if err := rejectTrustedPayload(request.Payload); err != nil {
		return nil, err
	}
	intent, err := decodeStealthToggleIntent(request.Payload)
	if err != nil {
		return nil, err
	}

	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	if err := runtime.validateShipCanMoveLocked(ctx.PlayerID); err != nil {
		return nil, err
	}
	if err := runtime.setPlayerStealthLocked(ctx.PlayerID, *intent.Enabled); err != nil {
		return nil, domainErrorForRuntime(err)
	}
	state := runtime.players[ctx.PlayerID]
	response := map[string]any{
		"accepted": true,
		"stealth": map[string]any{
			"enabled": *intent.Enabled,
		},
		"stats": state.Stats,
	}
	runtime.queueEventLocked(authSessionID(ctx.SessionID), realtime.EventStatsUpdated, state.Stats)
	return marshalPayload(response)
}

func decodeStealthToggleIntent(payload json.RawMessage) (stealthToggleIntent, error) {
	var intent stealthToggleIntent
	if err := decodeStrict(payload, &intent); err != nil {
		return stealthToggleIntent{}, err
	}
	if intent.Enabled == nil {
		return stealthToggleIntent{}, invalidPayload("Stealth enabled flag is required.", nil)
	}
	return intent, nil
}
