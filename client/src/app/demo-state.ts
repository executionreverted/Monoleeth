import { CLIENT_EVENTS, EventEnvelope, JsonObject } from '../protocol/envelope';

export function demoEvents(): EventEnvelope[] {
  return [
    event(CLIENT_EVENTS.entityEntered, {
      entity_id: 'player-local',
      entity_type: 'player',
      position: { x: 0, y: 0 },
      status_flags: ['local', 'self'],
      display: { label: 'Frontier-01', disposition: 'self' },
    }),
    event(CLIENT_EVENTS.entityEntered, {
      entity_id: 'npc-rake-01',
      entity_type: 'npc',
      position: { x: 180, y: -72 },
      status_flags: ['visible', 'hostile'],
      display: { label: 'Drone Rake', disposition: 'hostile' },
    }),
    event(CLIENT_EVENTS.entityEntered, {
      entity_id: 'loot-scrap-01',
      entity_type: 'loot',
      position: { x: -110, y: 86 },
      status_flags: ['visible'],
      display: { label: 'Scrap Cache', disposition: 'neutral' },
    }),
    event(CLIENT_EVENTS.entityEntered, {
      entity_id: 'signal-eris-04',
      entity_type: 'planet_signal',
      position: { x: 260, y: 150 },
      status_flags: ['known_intel'],
      display: { label: 'Unknown Signal', disposition: 'unknown' },
    }),
    event(CLIENT_EVENTS.worldSnapshot, {
      sector: { name: 'Demo Fringe', region: 'Fixture Belt', danger: 'locked', contested: false },
      minimap: {
        radar_range: 420,
        projection_radius: 1000,
        live_contacts: [
          { entity_id: 'player-local', entity_type: 'player', position: { x: 0, y: 0 }, disposition: 'self', status_flags: ['self'] },
          { entity_id: 'npc-rake-01', entity_type: 'npc', position: { x: 180, y: -72 }, disposition: 'hostile', status_flags: ['hostile'] },
          { entity_id: 'loot-scrap-01', entity_type: 'loot', position: { x: -110, y: 86 }, disposition: 'neutral', status_flags: ['loot'] },
          { entity_id: 'signal-eris-04', entity_type: 'planet_signal', position: { x: 260, y: 150 }, disposition: 'unknown', status_flags: ['unknown_signal'] },
        ],
        remembered: [],
      },
      entities: [
        {
          entity_id: 'player-local',
          entity_type: 'player',
          position: { x: 0, y: 0 },
          status_flags: ['self'],
          display: { label: 'Frontier-01', disposition: 'self' },
        },
        {
          entity_id: 'npc-rake-01',
          entity_type: 'npc',
          position: { x: 180, y: -72 },
          status_flags: ['hostile'],
          display: { label: 'Drone Rake', disposition: 'hostile' },
        },
        {
          entity_id: 'loot-scrap-01',
          entity_type: 'loot',
          position: { x: -110, y: 86 },
          status_flags: ['loot'],
          display: { label: 'Scrap Cache', disposition: 'neutral' },
        },
        {
          entity_id: 'signal-eris-04',
          entity_type: 'planet_signal',
          position: { x: 260, y: 150 },
          status_flags: ['unknown_signal'],
          display: { label: 'Unknown Signal', disposition: 'unknown' },
        },
      ],
      snapshot_cursor: 0,
    }),
    event(CLIENT_EVENTS.playerSnapshot, {
      callsign: 'Frontier-01',
      hp: 84,
      shield: 61,
      energy: 72,
      max_hp: 100,
      max_shield: 100,
      max_energy: 100,
      rank: 1,
    }),
    event(CLIENT_EVENTS.cargoSnapshot, {
      used: 17,
      capacity: 60,
      items: [
        { item_id: 'raw_ore', display_name: 'Raw Ore', category: 'resource', art_key: 'item.raw_ore', quantity: 11, unit_weight: 1, used_units: 11, location: 'ship_cargo', move_eligible: false, locked_reason: 'cargo_transfer_unavailable' },
        { item_id: 'salvage_thread', display_name: 'Salvage Thread', category: 'resource', art_key: 'item.salvage_thread', quantity: 6, unit_weight: 1, used_units: 6, location: 'ship_cargo', move_eligible: false, locked_reason: 'cargo_transfer_unavailable' },
      ],
    }),
    event(CLIENT_EVENTS.walletSnapshot, {
      credits: 1250,
      premium_paid: 0,
      premium_earned: 25,
    }),
    event(CLIENT_EVENTS.statsSnapshot, {
      speed: 180,
      radar_range: 420,
      weapon_range: 260,
      cargo_capacity: 60,
    }),
  ];
}

export function correctionEvent(entityID: string, position: { x: number; y: number }): EventEnvelope {
  return event(CLIENT_EVENTS.positionCorrected, {
    entity_id: entityID,
    position,
  });
}

function event(type: string, payload: JsonObject): EventEnvelope {
  demoSequence += 1;
  return {
    event_id: `demo-event-${demoSequence}`,
    type,
    payload,
    server_time: Date.now(),
    seq: demoSequence,
    v: 1,
  };
}

let demoSequence = 0;
