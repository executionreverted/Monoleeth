import type { ClientState } from '../state/types';
import { OPERATIONS } from '../protocol/envelope';
import { hudSelection } from './hud-selection';
import type { InventoryTabID, ModuleFilterID, ModuleInventoryItem } from './hud-types';
import { escapeHTML, formatDurability, formatDuration, hasPendingOpPayloadField, realtimeReady } from './hud-formatters';

export function cargoPanel(state: ClientState, serverNow: number | null = state.lastServerTime): string {
  const inventory = state.inventory;
  const hangar = state.hangar;
  const loadout = state.loadout;
  const cargo = state.cargo;
  const wallet = state.wallet;
  if (!inventory || !hangar || !loadout || !cargo || !wallet) {
    return `
      <h2>Inventory</h2>
      <section class="inventory-system" data-inventory-system="true" data-active-inventory-tab="${hudSelection.selectedInventoryTab}">
        ${inventoryTabBar(hudSelection.selectedInventoryTab, null)}
        <div class="inventory-tab-panel" data-inventory-tab-panel="${hudSelection.selectedInventoryTab}">
          <div class="empty-line">Awaiting inventory, hangar, and loadout data.</div>
        </div>
      </section>
    `;
  }
  const activeShip =
    hangar.ships.find((ship) => ship.ship_id === hangar.active_ship_id) ?? hangar.ships[0] ?? null;
  const moduleItems = inventory.instances.filter((item) => item.module_slot_type && item.location !== 'ship_equipped');
  const selectedModule = selectedModuleItem(filteredModuleItems(moduleItems, hudSelection.selectedModuleFilter));
  const equippedCount = loadout.slots.filter((slot) => slot.item_instance_id).length;
  const tabContext = { state, serverNow, inventory, hangar, loadout, cargo, wallet, crafting: state.crafting, activeShip, moduleItems, selectedModule, equippedCount };
  return `
    <h2>Inventory</h2>
    <section class="inventory-system" data-inventory-system="true" data-active-inventory-tab="${hudSelection.selectedInventoryTab}">
      ${inventoryTabBar(hudSelection.selectedInventoryTab, tabContext)}
      <div class="inventory-tab-panel" data-inventory-tab-panel="${hudSelection.selectedInventoryTab}">
        ${inventoryTabPanel(hudSelection.selectedInventoryTab, tabContext)}
      </div>
    </section>
  `;
}

type InventoryTabContext = {
  inventory: NonNullable<ClientState['inventory']>;
  hangar: NonNullable<ClientState['hangar']>;
  loadout: NonNullable<ClientState['loadout']>;
  cargo: NonNullable<ClientState['cargo']>;
  wallet: NonNullable<ClientState['wallet']>;
  crafting: ClientState['crafting'];
  state: ClientState;
  serverNow: number | null;
  activeShip: NonNullable<ClientState['hangar']>['ships'][number] | null;
  moduleItems: ModuleInventoryItem[];
  selectedModule: ModuleInventoryItem | null;
  equippedCount: number;
};

export function inventoryTabBar(activeTab: InventoryTabID, context: InventoryTabContext | null): string {
  const tabCounts: Record<InventoryTabID, string> = {
    equipment: context ? `${context.equippedCount}/${context.loadout.slots.length}` : '--',
    inventory: context ? String(context.inventory.instances.length + context.inventory.stackable.length) : '--',
    cargo: context ? `${context.cargo.used}/${context.cargo.capacity}` : '--',
    crafting: context?.crafting ? `${context.crafting.recipes.length}/${context.crafting.active_jobs.length}` : '--',
  };
  return `
    <div class="inventory-tabs" role="tablist" aria-label="Inventory systems">
      ${inventoryTabButton('equipment', 'Equipment', tabCounts.equipment, activeTab)}
      ${inventoryTabButton('inventory', 'Inventory', tabCounts.inventory, activeTab)}
      ${inventoryTabButton('cargo', 'Cargo', tabCounts.cargo, activeTab)}
      ${inventoryTabButton('crafting', 'Crafting', tabCounts.crafting, activeTab)}
    </div>
  `;
}

export function inventoryTabButton(tabID: InventoryTabID, label: string, meta: string, activeTab: InventoryTabID): string {
  const active = tabID === activeTab;
  return `
    <button
      type="button"
      role="tab"
      data-action="inventory-tab"
      data-inventory-tab="${tabID}"
      data-active="${active ? 'true' : 'false'}"
      aria-selected="${active ? 'true' : 'false'}">
      <span>${escapeHTML(label)}</span>
      <strong>${escapeHTML(meta)}</strong>
    </button>
  `;
}

export function inventoryTabPanel(tabID: InventoryTabID, context: InventoryTabContext): string {
  switch (tabID) {
    case 'inventory':
      return inventoryStoredPanel(context);
    case 'cargo':
      return cargoHoldPanel(context);
    case 'crafting':
      return craftingPanel(context);
    case 'equipment':
    default:
      return equipmentPanel(context);
  }
}

export function equipmentPanel(context: InventoryTabContext): string {
  const { activeShip, loadout, moduleItems, selectedModule, equippedCount } = context;
  return `
    <section class="loadout-console loadout-console--equipment" data-loadout-inventory-drop="true">
      <div class="loadout-ship">
        <div class="loadout-ship__core" aria-hidden="true"></div>
        <div class="loadout-ship__meta">
          <strong>${escapeHTML(activeShip?.display_name ?? (context.hangar.active_ship_id || 'Ship'))}</strong>
          <span>${equippedCount}/${loadout.slots.length} online</span>
        </div>
      </div>
      ${loadoutSlotGroups(loadout.slots)}
      ${moduleBayPanel(moduleItems, loadout.slots, selectedModule, 'Available modules')}
    </section>
  `;
}

export function inventoryStoredPanel(context: InventoryTabContext): string {
  const { inventory, loadout, moduleItems, selectedModule, state } = context;
  const stackRows = inventory.stackable
    .map(
      (item) => `
        <li data-item-id="${escapeHTML(item.item_id)}">
          <span title="${escapeHTML(publicInventoryStateLabel(item.location))}">${escapeHTML(item.display_name || item.item_id)}</span>
          <strong>${item.quantity}</strong>
          ${combatAmmoAction(item, state)}
        </li>
      `,
    )
    .join('');
  return `
    <section class="inventory-storage-console" data-inventory-storage="true">
      ${moduleBayPanel(moduleItems, loadout.slots, selectedModule, 'Account modules')}
      ${coordinateScrollPanel(inventory.instances, state)}
      <div class="inventory-stack-panel">
        <div class="module-bay__head">
          <strong>Stored cargo items</strong>
          <span>${inventory.stackable.length} stacks</span>
        </div>
        ${
          inventory.stackable.length > 0
            ? `<ul class="compact-list inventory-stack-list">${stackRows}</ul>`
            : '<div class="empty-line">No account cargo stacks.</div>'
        }
      </div>
    </section>
  `;
}

function combatAmmoAction(item: NonNullable<ClientState['inventory']>['stackable'][number], state: ClientState): string {
  if (!isLaserAmmoItem(item.item_id)) {
    return '';
  }
  const pending = hasPendingOpPayloadField(state, OPERATIONS.combatSelectAmmo, 'item_id', item.item_id);
  const selected = state.combatEngagement.activeAmmo.laser?.itemID === item.item_id;
  const enabled = realtimeReady(state) && !pending && item.location === 'account_inventory' && item.quantity > 0;
  const title = item.location !== 'account_inventory'
    ? 'Ammo must be in account inventory'
    : pending
      ? 'Ammo selection pending'
      : selected
        ? 'Laser ammo selected server-side'
        : 'Select laser ammo server-side';
  return `<button type="button" data-action="combat-ammo-select" data-ammo-family="laser" data-item-id="${escapeHTML(item.item_id)}" ${enabled ? '' : 'disabled'} title="${escapeHTML(title)}">${pending ? 'Selecting' : selected ? 'Selected' : 'Ammo'}</button>`;
}

function isLaserAmmoItem(itemID: string): boolean {
  return itemID.startsWith('ammunition_laser_');
}

function coordinateScrollPanel(items: NonNullable<ClientState['inventory']>['instances'], state: ClientState): string {
  const scrolls = items.filter((item) => item.item_id === 'planet_coordinate_scroll');
  if (scrolls.length === 0) {
    return '';
  }
  return `
    <div class="inventory-stack-panel" data-coordinate-scrolls="true">
      <div class="module-bay__head">
        <strong>Coordinate scrolls</strong>
        <span>${scrolls.length} owned</span>
      </div>
      <ul class="compact-list inventory-stack-list">
        ${scrolls.map((item) => coordinateScrollRow(item, state)).join('')}
      </ul>
    </div>
  `;
}

function coordinateScrollRow(item: NonNullable<ClientState['inventory']>['instances'][number], state: ClientState): string {
  const pending = hasPendingOpPayloadField(state, OPERATIONS.intelCoordinateItemUse, 'item_instance_id', item.item_instance_id);
  const enabled = realtimeReady(state) && !pending && item.location === 'account_inventory';
  const title = item.location !== 'account_inventory'
    ? 'Coordinate scroll must be in account inventory'
    : pending
      ? 'Coordinate item use pending'
      : !realtimeReady(state)
        ? 'Realtime connection required'
        : 'Use this server-owned coordinate scroll';
  return `
    <li data-coordinate-item-id="${escapeHTML(item.item_instance_id)}">
      <span title="${escapeHTML(publicInventoryStateLabel(item.location))}">${escapeHTML(item.display_name || item.item_id)}</span>
      <button
        type="button"
        data-action="coordinate-item-use"
        data-item-instance-id="${escapeHTML(item.item_instance_id)}"
        ${enabled ? '' : 'disabled'}
        title="${escapeHTML(title)}">${pending ? 'Using' : 'Use'}</button>
    </li>
  `;
}

export function cargoHoldPanel(context: InventoryTabContext): string {
  const { cargo, wallet } = context;
  const cargoPercent = cargo.capacity > 0 ? Math.min(100, Math.round((cargo.used / cargo.capacity) * 100)) : 0;
  return `
    <section class="cargo-hold-console" data-cargo-tab="true">
      <div class="cargo-strip">
        <div>
          <span>Cargo</span>
          <strong>${cargo.used}/${cargo.capacity}</strong>
        </div>
        <div class="meter"><span style="width:${cargoPercent}%"></span></div>
        <div>
          <span>Credits</span>
          <strong>${wallet.credits}</strong>
        </div>
      </div>
      ${
        cargo.items.length > 0
          ? `<ul class="compact-list cargo-strip__list">
              ${cargo.items.map((item) => `<li><span title="${escapeHTML(cargoItemMeta(item))}">${escapeHTML(cargoItemLabel(item))}</span><strong>${item.quantity}</strong></li>`).join('')}
            </ul>`
          : '<div class="empty-line">Cargo hold empty.</div>'
      }
      <div class="empty-line">Cargo transfer unavailable.</div>
    </section>
  `;
}

export function craftingPanel(context: InventoryTabContext): string {
  const { crafting, state, serverNow } = context;
  if (!crafting) {
    return `
      <section class="crafting-locked-panel" data-crafting-tab="true" data-crafting-state="loading">
        <div class="empty-line">Awaiting crafting recipes from server.</div>
      </section>
    `;
  }

  return `
    <section class="crafting-locked-panel" data-crafting-tab="true" data-crafting-state="ready">
      <div class="module-bay__head">
        <strong>Crafting station</strong>
        <span>${crafting.recipes.length} recipes · ${crafting.active_jobs.length} jobs</span>
      </div>
      ${craftingJobPanel(crafting.active_jobs, crafting.recipes, state, serverNow)}
      ${craftingRecipePanel(crafting.recipes, state)}
    </section>
  `;
}

export function craftingBlockedPanel(): string {
  return `
    <section class="crafting-locked-panel" data-crafting-tab="true" data-crafting-state="loading">
      <div class="empty-line">Awaiting crafting recipes from server.</div>
    </section>
  `;
}

function craftingJobPanel(
  jobs: NonNullable<ClientState['crafting']>['active_jobs'],
  recipes: NonNullable<ClientState['crafting']>['recipes'],
  state: ClientState,
  serverNow: number | null,
): string {
  if (jobs.length === 0) {
    return '<div class="empty-line">No active craft jobs.</div>';
  }
  return `
    <ul class="compact-list crafting-job-list" data-crafting-jobs="true">
      ${jobs.map((job) => craftingJobRow(job, recipes.find((recipe) => recipe.recipe_id === job.recipe_id) ?? null, state, serverNow)).join('')}
    </ul>
  `;
}

function craftingJobRow(
  job: NonNullable<ClientState['crafting']>['active_jobs'][number],
  recipe: NonNullable<ClientState['crafting']>['recipes'][number] | null,
  state: ClientState,
  serverNow: number | null,
): string {
  const pendingComplete = hasPendingOpPayloadField(state, OPERATIONS.craftingComplete, 'job_id', job.job_id);
  const pendingCancel = hasPendingOpPayloadField(state, OPERATIONS.craftingCancel, 'job_id', job.job_id);
  const status = craftingJobStatus(job, serverNow);
  const completed = job.state === 'completed' || job.state === 'complete';
  const completeDisabled = pendingComplete || pendingCancel || !realtimeReady(state) || !status.ready || completed;
  const cancelDisabled = pendingComplete || pendingCancel || !realtimeReady(state) || completed;
  const label = pendingComplete ? 'Pending' : status.ready && !completed ? 'Complete' : status.label;
  return `
    <li data-crafting-job-id="${escapeHTML(job.job_id)}">
      <span title="${escapeHTML(`Job ${job.job_id}`)}">${escapeHTML(recipe ? craftingOutputLabel(recipe) : job.recipe_id)}</span>
      <strong>${escapeHTML(status.detail)}</strong>
      <button
        type="button"
        data-action="crafting-complete"
        data-job-id="${escapeHTML(job.job_id)}"
        ${completeDisabled ? 'disabled' : ''}
        title="${escapeHTML(status.title)}">${escapeHTML(label)}</button>
      <button
        type="button"
        data-action="crafting-cancel"
        data-job-id="${escapeHTML(job.job_id)}"
        ${cancelDisabled ? 'disabled' : ''}
        title="${escapeHTML(pendingCancel ? 'Craft cancel pending.' : 'Cancel and refund this craft job.')}">${pendingCancel ? 'Pending' : 'Cancel'}</button>
    </li>
  `;
}

function craftingRecipePanel(recipes: NonNullable<ClientState['crafting']>['recipes'], state: ClientState): string {
  if (recipes.length === 0) {
    return '<div class="empty-line">No known crafting recipes.</div>';
  }
  return `
    <div class="module-grid crafting-recipe-grid" data-crafting-recipes="true">
      ${recipes.map((recipe) => craftingRecipeCard(recipe, state)).join('')}
    </div>
  `;
}

function craftingRecipeCard(recipe: NonNullable<ClientState['crafting']>['recipes'][number], state: ClientState): string {
  const pending = hasPendingOpPayloadField(state, OPERATIONS.craftingStart, 'recipe_id', recipe.recipe_id);
  const disabled = pending || !realtimeReady(state);
  const locationType = craftingStartLocationType(recipe);
  const locationTypeAttr = locationType ? ` data-location-type="${escapeHTML(locationType)}"` : '';
  return `
    <article class="module-card" data-crafting-recipe-id="${escapeHTML(recipe.recipe_id)}">
      <strong>${escapeHTML(craftingOutputLabel(recipe))}</strong>
      <span>${escapeHTML(craftingRecipeMeta(recipe))}</span>
      <small>${escapeHTML(craftingRecipeRequirements(recipe))}</small>
      <button
        type="button"
        data-action="crafting-start"
        data-recipe-id="${escapeHTML(recipe.recipe_id)}"
        ${locationTypeAttr}
        ${disabled ? 'disabled' : ''}
        title="${escapeHTML(disabled ? 'Craft start unavailable.' : 'Start this craft job.')}">${pending ? 'Pending' : 'Start'}</button>
    </article>
  `;
}

function craftingStartLocationType(recipe: NonNullable<ClientState['crafting']>['recipes'][number]): string {
  return recipe.required_location_type === 'station' ? 'station' : '';
}

function craftingJobStatus(job: NonNullable<ClientState['crafting']>['active_jobs'][number], serverNow: number | null): { ready: boolean; label: string; detail: string; title: string } {
  if (job.state === 'completed' || job.state === 'complete') {
    return { ready: false, label: 'Done', detail: 'Completed', title: 'Craft job already completed.' };
  }
  if (serverNow === null) {
    return { ready: false, label: 'Clock', detail: 'Server clock pending', title: 'Waiting for server time.' };
  }
  const remaining = job.completes_at - serverNow;
  if (remaining <= 0) {
    return { ready: true, label: 'Ready', detail: 'Ready', title: 'Craft job is ready to complete.' };
  }
  return {
    ready: false,
    label: formatDuration(remaining),
    detail: `${formatDuration(remaining)} remaining`,
    title: 'Craft job is still running.',
  };
}

function craftingOutputLabel(recipe: NonNullable<ClientState['crafting']>['recipes'][number]): string {
  const outputID = recipe.output.item_id || recipe.output.ship_id || recipe.output.kind || recipe.recipe_id;
  return `${outputID} x${recipe.output.quantity}`;
}

function craftingRecipeMeta(recipe: NonNullable<ClientState['crafting']>['recipes'][number]): string {
  const inputs = recipe.inputs.length > 0
    ? recipe.inputs.map((input) => `${input.item_id} x${input.quantity}`).join(', ')
    : 'No material inputs';
  return `${inputs} · ${formatDuration(recipe.craft_duration_ms)}`;
}

function craftingRecipeRequirements(recipe: NonNullable<ClientState['crafting']>['recipes'][number]): string {
  const requirements = [
    recipe.required_credits > 0 ? `${recipe.required_credits} credits` : null,
    recipe.required_rank > 0 ? `rank ${recipe.required_rank}` : null,
    recipe.required_location_type ? recipe.required_location_type.replace(/_/g, ' ') : null,
    ...recipe.required_role_levels.map((requirement) => `${requirement.role} ${requirement.level}`),
  ].filter(Boolean);
  return requirements.length > 0 ? requirements.join(' · ') : 'No extra requirements';
}

export function moduleBayPanel(
  moduleItems: ModuleInventoryItem[],
  slots: NonNullable<ClientState['loadout']>['slots'],
  selectedModule: ModuleInventoryItem | null,
  title: string,
): string {
  const visibleItems = filteredModuleItems(moduleItems, hudSelection.selectedModuleFilter);
  return `
    <div class="module-bay" data-loadout-inventory-drop="true">
      <div class="module-bay__head">
        <strong>${escapeHTML(title)}</strong>
        <span>${visibleItems.length}/${moduleItems.length} stored</span>
      </div>
      ${moduleFilterBar(moduleItems, hudSelection.selectedModuleFilter)}
      ${
        visibleItems.length > 0
          ? `<div class="module-grid" data-module-grid="true">${visibleItems.map((item) => moduleInventoryCard(item, slots, selectedModule?.item_instance_id ?? '')).join('')}</div>`
          : `<div class="empty-line">${escapeHTML(emptyModuleFilterCopy(hudSelection.selectedModuleFilter))}</div>`
      }
    </div>
  `;
}

export function moduleFilterBar(moduleItems: ModuleInventoryItem[], activeFilter: ModuleFilterID): string {
  const filterCounts: Record<ModuleFilterID, number> = {
    all: moduleItems.length,
    offensive: filteredModuleItems(moduleItems, 'offensive').length,
    defensive: filteredModuleItems(moduleItems, 'defensive').length,
    utility: filteredModuleItems(moduleItems, 'utility').length,
  };
  const filterOrder: ModuleFilterID[] = ['all', 'offensive', 'defensive', 'utility'];
  return `
    <div class="module-filter-bar" data-module-filter-bar="true" role="toolbar" aria-label="Module filters">
      ${filterOrder
        .map(
          (filterID) => `
            <button
              type="button"
              data-action="module-filter"
              data-module-filter="${filterID}"
              data-active="${activeFilter === filterID ? 'true' : 'false'}"
              aria-pressed="${activeFilter === filterID ? 'true' : 'false'}"
              title="${escapeHTML(publicModuleSlotGroupLabel(filterID))}">
              <span>${escapeHTML(publicModuleFilterShortLabel(filterID))}</span>
              <strong>${filterCounts[filterID]}</strong>
            </button>
          `,
        )
        .join('')}
    </div>
  `;
}

export function emptyModuleFilterCopy(filterID: ModuleFilterID): string {
  if (filterID === 'all') {
    return 'No modules in inventory.';
  }
  return `No ${publicModuleSlotGroupLabel(filterID).toLowerCase()} modules in inventory.`;
}

export function cargoItemLabel(item: { item_id: string; display_name?: string }): string {
  return item.display_name || item.item_id;
}

export function cargoItemMeta(item: { category?: string; location?: string; used_units?: number; unit_weight?: number; locked_reason?: string }): string {
  return [
    item.category,
    item.location ? publicInventoryStateLabel(item.location) : undefined,
    item.used_units !== undefined ? `${item.used_units}u used` : undefined,
    item.unit_weight !== undefined ? `${item.unit_weight}u each` : undefined,
    item.locked_reason ? 'Move unavailable' : undefined,
  ].filter(Boolean).join(' · ');
}

export function publicInventoryStateLabel(value: string): string {
  switch (value) {
    case 'account_inventory':
      return 'Stored';
    case 'ship_equipped':
      return 'Equipped';
    case 'ship_cargo':
      return 'Cargo hold';
    case 'planet_storage':
      return 'Planet storage';
    case 'station_storage':
      return 'Station storage';
    case 'market_escrow':
    case 'auction_escrow':
      return 'Escrow';
    case 'crafting_reserved':
      return 'Reserved';
    case 'world_drop':
      return 'Drop';
    default:
      return value.replace(/_/g, ' ');
  }
}

export function selectedModuleItem(items: ModuleInventoryItem[]): ModuleInventoryItem | null {
  if (items.length === 0) {
    hudSelection.selectedModuleInstanceID = null;
    return null;
  }
  const selected = items.find((item) => item.item_instance_id === hudSelection.selectedModuleInstanceID) ?? items[0];
  hudSelection.selectedModuleInstanceID = selected.item_instance_id;
  return selected;
}

export function filteredModuleItems(items: ModuleInventoryItem[], filterID: ModuleFilterID): ModuleInventoryItem[] {
  if (filterID === 'all') {
    return items;
  }
  return items.filter((item) => item.module_slot_type === filterID);
}

export function loadoutSlotGroups(slots: NonNullable<ClientState['loadout']>['slots']): string {
  const slotTypes = uniqueSlotTypes(slots);
  return `
    <div class="loadout-slot-groups" data-loadout-slot-groups="true">
      ${slotTypes
        .map((slotType) => {
          const groupSlots = slots.filter((slot) => slot.slot_type === slotType);
          return `
            <section class="loadout-slot-group" data-loadout-slot-group="${escapeHTML(slotType)}" data-slot-group="${escapeHTML(slotType)}">
              <div class="loadout-slot-group__head">
                <strong>${escapeHTML(publicModuleSlotGroupLabel(slotType))}</strong>
                <span>${groupSlots.filter((slot) => slot.item_instance_id).length}/${groupSlots.length}</span>
              </div>
              <div class="loadout-slot-grid">
                ${groupSlots.map((slot) => loadoutSlotCard(slot)).join('')}
              </div>
            </section>
          `;
        })
        .join('')}
    </div>
  `;
}

export function uniqueSlotTypes(slots: NonNullable<ClientState['loadout']>['slots']): string[] {
  const preferred = ['offensive', 'defensive', 'utility'];
  const discovered = [...new Set(slots.map((slot) => slot.slot_type))];
  return [
    ...preferred.filter((slotType) => discovered.includes(slotType)),
    ...discovered.filter((slotType) => !preferred.includes(slotType)),
  ];
}

export function publicModuleSlotGroupLabel(slotType: string): string {
  switch (slotType) {
    case 'all':
      return 'All';
    case 'offensive':
      return 'Weapons';
    case 'defensive':
      return 'Defense';
    case 'utility':
      return 'Utility';
    default:
      return slotType.replace(/_/g, ' ');
  }
}

export function publicModuleFilterShortLabel(filterID: ModuleFilterID): string {
  switch (filterID) {
    case 'all':
      return 'All';
    case 'offensive':
      return 'WPN';
    case 'defensive':
      return 'DEF';
    case 'utility':
      return 'UTL';
  }
}

export function loadoutSlotCard(slot: NonNullable<ClientState['loadout']>['slots'][number]): string {
  const occupied = Boolean(slot.item_instance_id);
  const slotLabel = publicSlotLabel(slot.slot_type, slot.slot_id);
  const moduleName = slot.display_name || slot.module_item_id || slot.item_instance_id || 'Module';
  return `
    <div class="loadout-slot" data-loadout-slot-id="${escapeHTML(slot.slot_id)}" data-slot-type="${escapeHTML(slot.slot_type)}" data-occupied="${occupied ? 'true' : 'false'}" title="${escapeHTML(occupied ? `${moduleName} · drag to inventory` : `Drop ${slotLabel} module here`)}">
      ${
        occupied
          ? `<div class="slot-module-chip"
                draggable="true"
                data-module-tooltip="true"
                data-module-instance-id="${escapeHTML(slot.item_instance_id ?? '')}"
                data-equipped-slot-id="${escapeHTML(slot.slot_id)}"
                data-module-slot-type="${escapeHTML(slot.slot_type)}"
                aria-label="${escapeHTML(moduleName)}">
               <span class="module-hover-card" aria-hidden="true">
                 <strong>${escapeHTML(moduleName)}</strong>
                 <span>${escapeHTML(publicModuleSlotGroupLabel(slot.slot_type))} · ${escapeHTML(slot.module_state || 'online')}</span>
                 <span>Dur ${formatDurability(slot.durability, slot.durability_max)}</span>
                 <span>Drop on inventory bay to unequip.</span>
               </span>
             </div>`
          : '<div class="loadout-slot__empty" aria-label="Empty module slot"></div>'
      }
    </div>
  `;
}

export function moduleInventoryCard(
  item: ModuleInventoryItem,
  slots: NonNullable<ClientState['loadout']>['slots'],
  selectedID: string,
): string {
  const compatibleSlot =
    slots.find((slot) => slot.slot_type === item.module_slot_type && !slot.item_instance_id) ??
    slots.find((slot) => slot.slot_type === item.module_slot_type) ??
    null;
  const compatible = Boolean(compatibleSlot);
  return `
    <button class="module-card"
      type="button"
      data-action="module-select"
      data-module-tooltip="true"
      draggable="true"
      data-module-instance-id="${escapeHTML(item.item_instance_id)}"
      data-module-slot-type="${escapeHTML(item.module_slot_type ?? '')}"
      data-compatible="${compatible ? 'true' : 'false'}"
      data-selected="${item.item_instance_id === selectedID ? 'true' : 'false'}"
      aria-label="${escapeHTML(item.display_name || item.item_id)}">
      <span class="module-card__rarity">${escapeHTML(publicRarityBadge(item.rarity || item.bound_state || 'owned'))}</span>
      <span class="module-hover-card" aria-hidden="true">
        <strong>${escapeHTML(item.display_name || item.item_id)}</strong>
        <span>${escapeHTML(publicModuleSlotGroupLabel(item.module_slot_type ?? 'module'))}</span>
        <span>Dur ${formatDurability(item.durability_current, item.durability_max)}</span>
        <span>${escapeHTML(item.rarity || item.bound_state || publicInventoryStateLabel(item.location))}</span>
        <span>${compatibleSlot ? `Drop on ${escapeHTML(publicSlotLabel(compatibleSlot.slot_type, compatibleSlot.slot_id))}.` : 'No compatible slot online.'}</span>
      </span>
    </button>
  `;
}

export function publicRarityBadge(value: string): string {
  switch (value.toLowerCase()) {
    case 'common':
      return 'C';
    case 'uncommon':
      return 'U';
    case 'rare':
      return 'R';
    case 'epic':
      return 'E';
    case 'legendary':
      return 'L';
    default:
      return value.slice(0, 1).toUpperCase();
  }
}

export function publicSlotAbbreviation(slotType: string): string {
  switch (slotType) {
    case 'offensive':
      return 'OFF';
    case 'defensive':
      return 'DEF';
    case 'utility':
      return 'UTL';
    default:
      return slotType.slice(0, 3).toUpperCase();
  }
}

export function publicSlotLabel(slotType: string, slotID: string): string {
  return `${publicSlotAbbreviation(slotType)} ${slotOrdinal(slotID)}`;
}

export function slotOrdinal(slotID: string): string {
  const match = slotID.match(/(\d+)$/);
  return match?.[1] ?? slotID.replace(/_/g, ' ');
}
