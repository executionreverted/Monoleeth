import type { ClientState } from '../state/types';
import type { HUDHelpTopicID, HUDModalID, HUDWindowID, InventoryStackItem, InventoryTabID, ModuleFilterID, ShopCategoryID } from './hud-types';

export function isControlElement(target: EventTarget | null): boolean {
  return target instanceof HTMLElement && Boolean(target.closest('button, input, select, textarea, a[href], [data-action]'));
}

export function isQuickActionKey(key: string): boolean {
  return key === '1' || key === '2' || key === '3' || key === '4' || key === '5' || key === '6';
}

export function parseLoadoutDragPayload(raw: string): { itemInstanceID: string; slotID?: string; moduleSlotType?: string } | null {
  if (!raw) {
    return null;
  }
  try {
    const parsed = JSON.parse(raw) as { itemInstanceID?: unknown; slotID?: unknown; moduleSlotType?: unknown };
    if (typeof parsed.itemInstanceID !== 'string' || parsed.itemInstanceID === '') {
      return null;
    }
    return {
      itemInstanceID: parsed.itemInstanceID,
      slotID: typeof parsed.slotID === 'string' && parsed.slotID !== '' ? parsed.slotID : undefined,
      moduleSlotType: typeof parsed.moduleSlotType === 'string' && parsed.moduleSlotType !== '' ? parsed.moduleSlotType : undefined,
    };
  } catch {
    return null;
  }
}

export function hasPendingOp(state: ClientState, op: string): boolean {
  return Object.values(state.pendingCommands).some((command) => command.op === op);
}

export function realtimeReady(state: ClientState): boolean {
  return state.auth.mode === 'demo' || state.connectionStatus === 'connected';
}

export function formatCooldown(milliseconds: number): string {
  if (milliseconds < 1000) {
    return '<1s';
  }
  return `${Math.ceil(milliseconds / 1000)}s`;
}

export function formatDuration(milliseconds: number): string {
  if (milliseconds < 1000) {
    return '<1s';
  }
  return `${(milliseconds / 1000).toFixed(milliseconds < 10_000 ? 1 : 0)}s`;
}

export function formatVec(position: { x: number; y: number }): string {
  return `${Math.round(position.x)},${Math.round(position.y)}`;
}

export function scanModeTimeDetail(nextPulseAt: number | null, fallback: string): string {
  if (!nextPulseAt) {
    return fallback;
  }
  const remaining = nextPulseAt - Date.now();
  if (remaining <= 0) {
    return fallback;
  }
  return formatCooldown(remaining);
}

export function lockedValue(): string {
  return '--';
}

export function formatPair(current?: number, max?: number): string {
  return current !== undefined && max !== undefined ? `${Math.round(current)}/${Math.round(max)}` : lockedValue();
}

export function formatDurability(current?: number, max?: number): string {
  if (current === undefined || max === undefined || max <= 0) {
    return lockedValue();
  }
  return `${Math.max(0, Math.round(current))}/${Math.round(max)}`;
}

export function formatPercent(current?: number, max?: number): string {
  if (current === undefined || max === undefined || max <= 0) {
    return lockedValue();
  }
  return `${Math.round((Math.max(0, Math.min(current, max)) / max) * 100)}%`;
}

export function formatCompactNumber(value: number): string {
  const abs = Math.abs(value);
  if (abs >= 1_000_000) {
    return `${(value / 1_000_000).toFixed(1).replace(/\.0$/, '')}M`;
  }
  if (abs >= 10_000) {
    return `${Math.round(value / 1_000)}K`;
  }
  return String(value);
}

export function normalizePanelID(value: string | undefined): HUDWindowID | null {
  return value === 'cargo' ||
    value === 'economy' ||
    value === 'quests' ||
    value === 'intel' ||
    value === 'systems' ||
    value === 'ops'
    ? value
    : null;
}

export function isShopCategoryID(value: string | undefined): value is ShopCategoryID {
  return typeof value === 'string' && value.trim().length > 0;
}

export function isInventoryTabID(value: string | undefined): value is InventoryTabID {
  return value === 'equipment' || value === 'inventory' || value === 'cargo' || value === 'crafting';
}

export function isModuleFilterID(value: string | undefined): value is ModuleFilterID {
  return value === 'all' || value === 'offensive' || value === 'defensive' || value === 'utility';
}

export function normalizeModalID(value: string | undefined): HUDModalID | null {
  if (value === 'target' || value === 'planets' || value === 'ship' || value === 'planet-detail' || value === 'tutorial') {
    return value;
  }
  return null;
}

export function isHelpTopicID(value: string | undefined): value is HUDHelpTopicID {
  return value === 'inventory' || value === 'shop' || value === 'quests' || value === 'planets' || value === 'hangar' || value === 'ops';
}

export function publicEntityType(entityType: string): string {
  switch (entityType) {
    case 'npc':
      return 'hostile';
    case 'loot':
      return 'drop';
    case 'planet_signal':
      return 'signal';
    default:
      return entityType;
  }
}

export function dispositionForType(entityType: string): string {
  switch (entityType) {
    case 'npc':
      return 'hostile';
    case 'planet_signal':
      return 'unknown';
    case 'loot':
      return 'neutral';
    default:
      return 'friendly';
  }
}

export function clamp(value: number, min: number, max: number): number {
  return Math.min(max, Math.max(min, value));
}

export function uniqueNumbers(values: number[]): number[] {
  return [...new Set(values.map((value) => Math.max(1, Math.round(value))))].sort((left, right) => left - right);
}

export function scanStatusLabel(status: string): string {
  switch (status) {
    case 'planet_discovered':
      return 'Discovered';
    case 'no_signal':
      return 'No signal';
    case 'started':
      return 'Scanning';
    default:
      return status || 'Scanner';
  }
}

export function actionScanLabel(status: string): string {
  return status === 'planet_discovered' ? 'Found' : scanStatusLabel(status);
}

export function publicPlanetName(planet: NonNullable<ClientState['planetIntel']>['planets'][number]): string {
  const type = planet.planet_type ? planet.planet_type.replace(/_/g, ' ') : 'planet';
  const biome = planet.biome ? planet.biome.replace(/_/g, ' ') : 'unknown';
  return `${type} / ${biome}`;
}

export function publicAuctionName(payloadType: string, definitionID: string): string {
  const label = payloadType ? payloadType.replace(/_/g, ' ') : 'lot';
  return definitionID ? `${label} ${definitionID.replace(/_/g, ' ')}` : label;
}

export function questObjectiveLabel(objective: NonNullable<ClientState['questBoard']>['active'][number]['objectives'][number]): string {
  const label = objective.display_name || questObjectiveKindLabel(objective.kind);
  return `${objective.current}/${objective.required} ${label}`;
}

export function questRewardLabel(reward: NonNullable<ClientState['questBoard']>['offers'][number]['rewards'][number] | undefined): string {
  if (!reward) {
    return 'reward pending';
  }
  return `${reward.amount} ${reward.display_name || questRewardKindLabel(reward)}`;
}

export function questObjectiveKindLabel(kind: string): string {
  switch (kind) {
    case 'kill':
      return 'Destroy target';
    case 'collect':
      return 'Recover cargo';
    case 'craft':
      return 'Fabricate item';
    case 'scan':
      return 'Scan signal';
    case 'build':
      return 'Build structure';
    case 'deliver':
      return 'Deliver cargo';
    default:
      return 'Objective';
  }
}

export function questRewardKindLabel(reward: NonNullable<ClientState['questBoard']>['offers'][number]['rewards'][number]): string {
  switch (reward.kind) {
    case 'currency':
      return 'Currency';
    case 'item':
      return 'Item';
    case 'main_xp':
      return 'Pilot XP';
    case 'role_xp':
      return 'Role XP';
    default:
      return 'Reward';
  }
}

export function walletBalanceForCurrency(wallet: NonNullable<ClientState['wallet']>, currency: string): number | null {
  switch (currency) {
    case 'credits':
      return wallet.credits;
    case 'premium_paid':
      return wallet.premium_paid;
    case 'premium_earned':
      return wallet.premium_earned;
    default:
      return null;
  }
}

export function sellableInventoryStack(state: ClientState): NonNullable<ClientState['inventory']>['stackable'][number] | null {
  return sellableInventoryStacks(state)[0] ?? null;
}

export function sellableInventoryStacks(state: ClientState): NonNullable<ClientState['inventory']>['stackable'] {
  return (
    state.inventory?.stackable.filter(
      (item) =>
        item.list_eligible === true &&
        item.quantity > 0 &&
        (item.location === 'account_inventory' || item.location === 'ship_cargo'),
    ) ?? []
  );
}

export function inventoryStackListState(item: InventoryStackItem): string {
  return item.list_eligible === true ? 'List ready' : 'Market locked';
}

export function defaultListingPrice(itemID: string, market: NonNullable<ClientState['market']>): number {
  const matchingListing = market.listings.find(
    (listing) => listing.item_id === itemID && listing.status === 'active' && !listing.owned_by_you,
  );
  return Math.max(1, Math.round(matchingListing?.unit_price ?? 25));
}

export function escapeHTML(value: string): string {
  return value.replace(/[&<>"']/g, (char) => {
    switch (char) {
      case '&':
        return '&amp;';
      case '<':
        return '&lt;';
      case '>':
        return '&gt;';
      case '"':
        return '&quot;';
      default:
        return '&#39;';
    }
  });
}
