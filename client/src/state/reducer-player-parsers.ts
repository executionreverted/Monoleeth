import { JsonObject } from '../protocol/envelope';
import type {
  CargoItemSummary,
  CargoSummary,
  ClientState,
  CraftingSummary,
  HangarSummary,
  InventorySummary,
  LoadoutSummary,
  ProgressionSummary,
  PublicSession,
  RepairQuote,
  ShipSummary,
  StatSummary,
  WalletSummary,
} from './types';
import {
  booleanField,
  isJsonObject,
  numberField,
  objectField,
  optionalRoundedNumber,
  stringField,
} from './reducer-helpers';

export function parseSessionReady(payload: JsonObject, serverTime: number): PublicSession {
  const account = objectField(payload, 'account');
  const player = objectField(payload, 'player');
  const roles = Array.isArray(payload.roles) ? payload.roles.filter((role): role is string => typeof role === 'string') : undefined;
  return {
    authenticated: payload.authenticated === true,
    account: account
      ? {
          email: stringField(account, 'email') ?? '',
          admin: booleanField(account, 'admin') ?? false,
        }
      : undefined,
    player: player ? { callsign: stringField(player, 'callsign') ?? '' } : undefined,
    roles,
    expires_at: numberField(payload, 'expires_at') ?? undefined,
    server_time: serverTime,
  };
}

export function parseCargoSummary(payload: JsonObject, fallback: CargoSummary | null): CargoSummary {
  const used = numberField(payload, 'used') ?? fallback?.used ?? 0;
  const capacity = numberField(payload, 'capacity') ?? numberField(payload, 'cargo_capacity') ?? fallback?.capacity ?? 0;
  const rawItems = Array.isArray(payload.items) ? payload.items : fallback?.items ?? [];
  const items = rawItems
    .filter(isJsonObject)
    .map(parseCargoItemSummary)
    .filter((item) => item.item_id !== '' && item.quantity > 0);

  return {
    used: Math.max(0, Math.round(used)),
    capacity: Math.max(0, Math.round(capacity)),
    items,
  };
}

function parseCargoItemSummary(item: JsonObject): CargoItemSummary {
  const parsed: CargoItemSummary = {
    item_id: stringField(item, 'item_id') ?? '',
    quantity: numberField(item, 'quantity') ?? 0,
  };
  const displayName = stringField(item, 'display_name');
  if (displayName) {
    parsed.display_name = displayName;
  }
  const category = stringField(item, 'category');
  if (category) {
    parsed.category = category;
  }
  const artKey = stringField(item, 'art_key');
  if (artKey) {
    parsed.art_key = artKey;
  }
  const rarity = stringField(item, 'rarity');
  if (rarity) {
    parsed.rarity = rarity;
  }
  const unitWeight = optionalRoundedNumber(item, 'unit_weight', undefined);
  if (unitWeight !== undefined) {
    parsed.unit_weight = unitWeight;
  }
  const usedUnits = optionalRoundedNumber(item, 'used_units', undefined);
  if (usedUnits !== undefined) {
    parsed.used_units = usedUnits;
  }
  const location = stringField(item, 'location');
  if (location) {
    parsed.location = location;
  }
  const moveEligible = booleanField(item, 'move_eligible');
  if (moveEligible !== null) {
    parsed.move_eligible = moveEligible;
  }
  const lockedReason = stringField(item, 'locked_reason');
  if (lockedReason) {
    parsed.locked_reason = lockedReason;
  }
  return parsed;
}

export function parseWalletSummary(payload: JsonObject, fallback: WalletSummary | null): WalletSummary {
  return {
    credits: Math.max(0, Math.round(numberField(payload, 'credits') ?? fallback?.credits ?? 0)),
    premium_paid: Math.max(0, Math.round(numberField(payload, 'premium_paid') ?? fallback?.premium_paid ?? 0)),
    premium_earned: Math.max(0, Math.round(numberField(payload, 'premium_earned') ?? fallback?.premium_earned ?? 0)),
  };
}

export function parseShipSummary(payload: JsonObject, fallback: ShipSummary | null): ShipSummary {
  return {
    active_ship_id: stringField(payload, 'active_ship_id') ?? fallback?.active_ship_id ?? '',
    display_name: stringField(payload, 'display_name') ?? fallback?.display_name ?? '',
    hull: Math.max(0, Math.round(numberField(payload, 'hull') ?? fallback?.hull ?? 0)),
    max_hull: Math.max(0, Math.round(numberField(payload, 'max_hull') ?? fallback?.max_hull ?? 0)),
    shield: Math.max(0, Math.round(numberField(payload, 'shield') ?? fallback?.shield ?? 0)),
    max_shield: Math.max(0, Math.round(numberField(payload, 'max_shield') ?? fallback?.max_shield ?? 0)),
    capacitor: Math.max(0, Math.round(numberField(payload, 'capacitor') ?? fallback?.capacitor ?? 0)),
    max_capacitor: Math.max(0, Math.round(numberField(payload, 'max_capacitor') ?? fallback?.max_capacitor ?? 0)),
    disabled: booleanField(payload, 'disabled') ?? fallback?.disabled ?? false,
    repair_state: stringField(payload, 'repair_state') ?? fallback?.repair_state ?? '',
  };
}

export function applyDeathShipDisabled(state: ClientState, payload: JsonObject): ClientState {
  const shipPayload = objectField(payload, 'ship');
  const shipID = stringField(payload, 'ship_id') ?? stringField(shipPayload ?? {}, 'active_ship_id') ?? '';
  const disabledReason = stringField(payload, 'disabled_reason') ?? 'disabled';
  const ship = shipPayload
    ? parseShipSummary(
        {
          ...shipPayload,
          disabled: true,
          repair_state: stringField(shipPayload, 'repair_state') ?? disabledReason,
        },
        state.ship,
      )
    : state.ship && (!shipID || state.ship.active_ship_id === shipID)
      ? {
          ...state.ship,
          disabled: true,
          repair_state: disabledReason,
        }
      : state.ship;
  const quote = objectField(payload, 'repair_quote');

  return {
    ...state,
    ship,
    repairQuote: quote ? parseRepairQuote(quote, state.repairQuote) : state.repairQuote,
  };
}

export function parseStatSummary(payload: JsonObject, fallback: StatSummary | null): StatSummary {
  return {
    speed: Math.max(0, numberField(payload, 'speed') ?? fallback?.speed ?? 0),
    radar_range: Math.max(0, numberField(payload, 'radar_range') ?? fallback?.radar_range ?? 0),
    weapon_range: Math.max(0, numberField(payload, 'weapon_range') ?? fallback?.weapon_range ?? 0),
    cargo_capacity: Math.max(0, numberField(payload, 'cargo_capacity') ?? fallback?.cargo_capacity ?? 0),
    loot_pickup_range: Math.max(0, numberField(payload, 'loot_pickup_range') ?? fallback?.loot_pickup_range ?? 0),
    basic_laser_energy_cost: Math.max(0, numberField(payload, 'basic_laser_energy_cost') ?? fallback?.basic_laser_energy_cost ?? 0),
    basic_laser_cooldown_ms: Math.max(0, numberField(payload, 'basic_laser_cooldown_ms') ?? fallback?.basic_laser_cooldown_ms ?? 0),
  };
}

export function parseProgressionSummary(payload: JsonObject, fallback: ProgressionSummary | null): ProgressionSummary {
  return {
    main_level: Math.max(0, Math.round(numberField(payload, 'main_level') ?? fallback?.main_level ?? 0)),
    main_xp: Math.max(0, Math.round(numberField(payload, 'main_xp') ?? fallback?.main_xp ?? 0)),
    rank: Math.max(0, Math.round(numberField(payload, 'rank') ?? fallback?.rank ?? 0)),
    combat_level: optionalRoundedNumber(payload, 'combat_level', fallback?.combat_level),
    combat_xp: optionalRoundedNumber(payload, 'combat_xp', fallback?.combat_xp),
  };
}

export function parseInventorySummary(payload: JsonObject, fallback: InventorySummary | null): InventorySummary {
  const stackable = Array.isArray(payload.stackable)
    ? payload.stackable
        .filter(isJsonObject)
        .map(parseInventoryStack)
        .filter((item): item is InventorySummary['stackable'][number] => item !== null)
    : fallback?.stackable ?? [];
  const instances = Array.isArray(payload.instances)
    ? payload.instances
        .filter(isJsonObject)
        .map(parseInventoryInstance)
        .filter((item): item is InventorySummary['instances'][number] => item !== null)
    : fallback?.instances ?? [];
  const counts = objectField(payload, 'counts');

  return {
    stackable,
    instances,
    counts: {
      cargo_stacks: Math.max(0, Math.round(numberField(counts ?? {}, 'cargo_stacks') ?? fallback?.counts.cargo_stacks ?? 0)),
      storage_stacks: Math.max(0, Math.round(numberField(counts ?? {}, 'storage_stacks') ?? fallback?.counts.storage_stacks ?? 0)),
      equipped_instances: Math.max(
        0,
        Math.round(numberField(counts ?? {}, 'equipped_instances') ?? fallback?.counts.equipped_instances ?? 0),
      ),
    },
  };
}

function parseInventoryStack(payload: JsonObject): InventorySummary['stackable'][number] | null {
  const itemID = stringField(payload, 'item_id') ?? '';
  const quantity = numberField(payload, 'quantity') ?? 0;
  if (!itemID || quantity <= 0) {
    return null;
  }
  const parsed: InventorySummary['stackable'][number] = {
    item_id: itemID,
    display_name: stringField(payload, 'display_name') ?? undefined,
    quantity: Math.max(0, Math.round(quantity)),
    location: stringField(payload, 'location') ?? '',
  };
  const listEligible = booleanField(payload, 'list_eligible');
  if (listEligible !== null) {
    parsed.list_eligible = listEligible;
  }
  const lockedReason = stringField(payload, 'locked_reason');
  if (lockedReason) {
    parsed.locked_reason = lockedReason;
  }
  return parsed;
}

function parseInventoryInstance(payload: JsonObject): InventorySummary['instances'][number] | null {
  const itemInstanceID = stringField(payload, 'item_instance_id') ?? '';
  const itemID = stringField(payload, 'item_id') ?? '';
  if (!itemInstanceID || !itemID) {
    return null;
  }
  return {
    item_instance_id: itemInstanceID,
    item_id: itemID,
    display_name: stringField(payload, 'display_name') ?? undefined,
    location: stringField(payload, 'location') ?? '',
    rarity: stringField(payload, 'rarity') ?? undefined,
    item_type: stringField(payload, 'item_type') ?? undefined,
    module_slot_type: stringField(payload, 'module_slot_type') ?? undefined,
    module_category: stringField(payload, 'module_category') ?? undefined,
    durability_current: optionalRoundedNumber(payload, 'durability_current', undefined),
    durability_max: optionalRoundedNumber(payload, 'durability_max', undefined),
    bound_state: stringField(payload, 'bound_state') ?? undefined,
  };
}

export function parseHangarSummary(payload: JsonObject, fallback: HangarSummary | null): HangarSummary {
  const ships = Array.isArray(payload.ships)
    ? payload.ships
        .filter(isJsonObject)
        .map(parseHangarShip)
        .filter((ship): ship is HangarSummary['ships'][number] => ship !== null)
    : fallback?.ships ?? [];
  return {
    active_ship_id: stringField(payload, 'active_ship_id') ?? fallback?.active_ship_id ?? '',
    ships,
  };
}

function parseHangarShip(payload: JsonObject): HangarSummary['ships'][number] | null {
  const shipID = stringField(payload, 'ship_id') ?? '';
  if (!shipID) {
    return null;
  }
  return {
    ship_id: shipID,
    display_name: stringField(payload, 'display_name') ?? shipID,
    state: stringField(payload, 'state') ?? '',
    role: stringField(payload, 'role') ?? undefined,
    tier: optionalRoundedNumber(payload, 'tier', undefined),
    rank_requirement: optionalRoundedNumber(payload, 'rank_requirement', undefined),
    hull: Math.max(0, Math.round(numberField(payload, 'hull') ?? 0)),
    max_hull: Math.max(0, Math.round(numberField(payload, 'max_hull') ?? 0)),
    shield: Math.max(0, Math.round(numberField(payload, 'shield') ?? 0)),
    max_shield: Math.max(0, Math.round(numberField(payload, 'max_shield') ?? 0)),
    capacitor: optionalRoundedNumber(payload, 'capacitor', undefined),
    max_capacitor: optionalRoundedNumber(payload, 'max_capacitor', undefined),
    speed: optionalRoundedNumber(payload, 'speed', undefined),
    radar: optionalRoundedNumber(payload, 'radar', undefined),
    cargo_capacity: optionalRoundedNumber(payload, 'cargo_capacity', undefined),
    slot_offensive: optionalRoundedNumber(payload, 'slot_offensive', undefined),
    slot_defensive: optionalRoundedNumber(payload, 'slot_defensive', undefined),
    slot_utility: optionalRoundedNumber(payload, 'slot_utility', undefined),
    disabled: booleanField(payload, 'disabled') ?? false,
    active: booleanField(payload, 'active') ?? undefined,
    locked_reason: stringField(payload, 'locked_reason') ?? undefined,
  };
}

export function parseLoadoutSummary(payload: JsonObject, fallback: LoadoutSummary | null): LoadoutSummary {
  const slots = Array.isArray(payload.slots)
    ? payload.slots
        .filter(isJsonObject)
        .map(parseLoadoutSlot)
        .filter((slot): slot is LoadoutSummary['slots'][number] => slot !== null)
    : fallback?.slots ?? [];
  return {
    active_ship_id: stringField(payload, 'active_ship_id') ?? fallback?.active_ship_id ?? '',
    slots,
  };
}

function parseLoadoutSlot(payload: JsonObject): LoadoutSummary['slots'][number] | null {
  const slotID = stringField(payload, 'slot_id') ?? '';
  const slotType = stringField(payload, 'slot_type') ?? '';
  if (!slotID || !slotType) {
    return null;
  }
  return {
    slot_id: slotID,
    slot_type: slotType,
    module_item_id: stringField(payload, 'module_item_id') ?? undefined,
    item_instance_id: stringField(payload, 'item_instance_id') ?? undefined,
    module_id: stringField(payload, 'module_id') ?? undefined,
    display_name: stringField(payload, 'display_name') ?? undefined,
    module_state: stringField(payload, 'module_state') ?? undefined,
    durability: optionalRoundedNumber(payload, 'durability', undefined),
    durability_max: optionalRoundedNumber(payload, 'durability_max', undefined),
  };
}

export function parseCraftingSummary(payload: JsonObject, fallback: CraftingSummary | null): CraftingSummary {
  const recipes = Array.isArray(payload.recipes)
    ? payload.recipes
        .filter(isJsonObject)
        .map(parseCraftingRecipe)
        .filter((recipe): recipe is CraftingSummary['recipes'][number] => recipe !== null)
    : fallback?.recipes ?? [];
  const activeJobs = Array.isArray(payload.active_jobs)
    ? payload.active_jobs
        .filter(isJsonObject)
        .map(parseCraftingJob)
        .filter((job): job is CraftingSummary['active_jobs'][number] => job !== null)
    : fallback?.active_jobs ?? [];
  return {
    recipes,
    active_jobs: activeJobs,
  };
}

function parseCraftingRecipe(payload: JsonObject): CraftingSummary['recipes'][number] | null {
  const recipeID = stringField(payload, 'recipe_id') ?? '';
  const output = objectField(payload, 'output');
  if (!recipeID || !output) {
    return null;
  }
  return {
    recipe_id: recipeID,
    category: stringField(payload, 'category') ?? '',
    output: {
      kind: stringField(output, 'kind') ?? '',
      item_id: stringField(output, 'item_id') ?? undefined,
      ship_id: stringField(output, 'ship_id') ?? undefined,
      quantity: Math.max(0, Math.round(numberField(output, 'quantity') ?? 0)),
      tradeable: booleanField(output, 'tradeable') ?? false,
    },
    inputs: Array.isArray(payload.inputs)
      ? payload.inputs
          .filter(isJsonObject)
          .map((input) => ({
            item_id: stringField(input, 'item_id') ?? '',
            quantity: Math.max(0, Math.round(numberField(input, 'quantity') ?? 0)),
          }))
          .filter((input) => input.item_id !== '' && input.quantity > 0)
      : [],
    required_credits: Math.max(0, Math.round(numberField(payload, 'required_credits') ?? 0)),
    required_rank: Math.max(0, Math.round(numberField(payload, 'required_rank') ?? 0)),
    required_role_levels: Array.isArray(payload.required_role_levels)
      ? payload.required_role_levels
          .filter(isJsonObject)
          .map((requirement) => ({
            role: stringField(requirement, 'role') ?? '',
            level: Math.max(0, Math.round(numberField(requirement, 'level') ?? 0)),
          }))
          .filter((requirement) => requirement.role !== '' && requirement.level > 0)
      : [],
    required_location_type: stringField(payload, 'required_location_type') ?? '',
    craft_duration_ms: Math.max(0, Math.round(numberField(payload, 'craft_duration_ms') ?? 0)),
    repeatable: booleanField(payload, 'repeatable') ?? false,
  };
}

function parseCraftingJob(payload: JsonObject): CraftingSummary['active_jobs'][number] | null {
  const jobID = stringField(payload, 'job_id') ?? '';
  const recipeID = stringField(payload, 'recipe_id') ?? '';
  if (!jobID || !recipeID) {
    return null;
  }
  return {
    job_id: jobID,
    recipe_id: recipeID,
    state: stringField(payload, 'state') ?? '',
    started_at: Math.max(0, Math.round(numberField(payload, 'started_at') ?? 0)),
    completes_at: Math.max(0, Math.round(numberField(payload, 'completes_at') ?? 0)),
  };
}

export function parseRepairQuote(payload: JsonObject, fallback: RepairQuote | null): RepairQuote {
  return {
    ship_id: stringField(payload, 'ship_id') ?? fallback?.ship_id ?? '',
    currency: stringField(payload, 'currency') ?? fallback?.currency ?? 'credits',
    cost: Math.max(0, Math.round(numberField(payload, 'cost') ?? fallback?.cost ?? 0)),
    disabled: booleanField(payload, 'disabled') ?? fallback?.disabled ?? false,
    quote_id: stringField(payload, 'quote_id') ?? fallback?.quote_id ?? '',
    issued_at_ms: Math.max(0, Math.round(numberField(payload, 'issued_at_ms') ?? fallback?.issued_at_ms ?? 0)),
    expires_at_ms: Math.max(0, Math.round(numberField(payload, 'expires_at_ms') ?? fallback?.expires_at_ms ?? 0)),
  };
}
