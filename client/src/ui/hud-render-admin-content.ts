import type { AdminContentDraftRow, AdminContentVersionSummary, ClientState } from '../state/types';
import { escapeHTML, lockedValue } from './hud-formatters';
import { hudSelection } from './hud-selection';
import type { AdminContentDraftUpdateInput } from './hud-types';

type AdminContentLootTableType = `${'loot'}_${'table'}`;
const ADMIN_CONTENT_LOOT_TABLE_TYPE = `${'loot'}_${'table'}` as AdminContentLootTableType;

export type AdminContentTypeID =
  | 'module'
  | 'item'
  | 'ship'
  | 'shop_product'
  | 'npc_template'
  | AdminContentLootTableType
  | 'craft_recipe'
  | 'production_building';

export interface AdminContentTypeConfig {
  id: AdminContentTypeID;
  label: string;
  shortLabel: string;
  emptyLabel: string;
}

export const ADMIN_CONTENT_EDITOR_TYPES: AdminContentTypeConfig[] = [
  { id: 'module', label: 'Modules', shortLabel: 'Mod', emptyLabel: 'No module draft rows loaded.' },
  { id: 'item', label: 'Items', shortLabel: 'Item', emptyLabel: 'No item draft rows loaded.' },
  { id: 'ship', label: 'Ships', shortLabel: 'Ship', emptyLabel: 'No ship draft rows loaded.' },
  { id: 'shop_product', label: 'Shop Products', shortLabel: 'Shop', emptyLabel: 'No shop product draft rows loaded.' },
  { id: 'npc_template', label: 'NPC Templates', shortLabel: 'NPC', emptyLabel: 'No NPC template draft rows loaded.' },
  { id: ADMIN_CONTENT_LOOT_TABLE_TYPE, label: 'Loot Tables', shortLabel: 'Loot', emptyLabel: 'No loot table draft rows loaded.' },
  { id: 'craft_recipe', label: 'Craft Recipes', shortLabel: 'Craft', emptyLabel: 'No craft recipe draft rows loaded.' },
  {
    id: 'production_building',
    label: 'Production Buildings',
    shortLabel: 'Prod',
    emptyLabel: 'No production building draft rows loaded.',
  },
];

export type AdminContentEditFieldID =
  | 'enabled'
  | 'display_name'
  | 'rarity'
  | 'max_stack'
  | 'weapon_damage'
  | 'shield_damage'
  | 'range'
  | 'cooldown'
  | 'energy'
  | 'rank'
  | 'ship_hp'
  | 'shop_price'
  | 'npc_hp'
  | 'loot_chance'
  | 'craft_required_rank'
  | 'production_rate';

export type AdminModuleEditFieldID =
  | 'weapon_damage'
  | 'shield_damage'
  | 'range'
  | 'cooldown'
  | 'energy'
  | 'rank'
  | 'rarity';

export type AdminContentEditPatch = Partial<Record<AdminContentEditFieldID, number | string | boolean>>;
export type AdminModuleEditPatch = Partial<Record<AdminModuleEditFieldID, number | string>>;

export interface AdminContentEditField {
  id: AdminContentEditFieldID;
  label: string;
  value: number | string | boolean;
  kind: 'number' | 'text' | 'rarity' | 'boolean';
  min?: number;
  max?: number;
  step?: string;
}

export type AdminModuleEditField = Omit<AdminContentEditField, 'id' | 'kind'> & {
  id: AdminModuleEditFieldID;
  kind: 'number' | 'rarity';
};

export function adminContentBlock(state: ClientState): string {
  if (!state.auth.session?.account?.admin) {
    return '';
  }
  const content = state.adminContent;
  const versions = content?.versions?.versions ?? [];
  const selectedType = selectedAdminContentType();
  const selectedConfig = adminContentTypeConfig(selectedType);
  const selectedList = content?.rowsByType[selectedType] ?? null;
  const rows = selectedList?.rows ?? [];
  const selected = findAdminContentDraftRow(state, selectedType, hudSelection.selectedAdminContentID) ?? rows[0] ?? null;
  const validation = content?.validation;
  const audit = content?.auditLog;
  const publish = content?.publish;

  return `
    <div class="systems-subhead">CMS</div>
    <div class="cms-console">
      <div class="cms-toolbar segmented segmented--ops">
        <button type="button" data-action="admin-content-refresh">Sync</button>
        <button type="button" data-action="admin-content-validate">Validate</button>
        <button type="button" data-action="admin-content-publish" ${validation && !validation.valid ? 'disabled' : ''}>Publish</button>
        <button type="button" data-action="admin-content-audit">Audit</button>
      </div>
      <div class="cms-type-filter" role="tablist" aria-label="CMS content type">
        ${ADMIN_CONTENT_EDITOR_TYPES.map((type) => typeButton(type, selectedType, content?.rowsByType[type.id]?.total ?? 0)).join('')}
      </div>
      <div class="cms-metrics">
        <span><em>Version</em><strong>${escapeHTML(currentVersionLabel(versions))}</strong></span>
        <span><em>${escapeHTML(selectedConfig.shortLabel)}</em><strong>${selectedList ? selectedList.total : lockedValue()}</strong></span>
        <span><em>Validation</em><strong>${validation ? (validation.valid ? 'valid' : `${validation.issues.length} issue`) : lockedValue()}</strong></span>
      </div>
      <div class="cms-grid">
        <section class="cms-panel">
          <header><strong>Versions</strong><span>${versions.length}</span></header>
          <div class="cms-list">
            ${
              versions.length
                ? versions.map((version) => versionRow(version)).join('')
                : '<div class="empty-line">No version payload loaded.</div>'
            }
          </div>
        </section>
        <section class="cms-panel">
          <header><strong>${escapeHTML(selectedConfig.label)}</strong><span>${selectedList ? selectedList.total : 0}</span></header>
          <div class="cms-list">
            ${
              rows.length
                ? rows.map((row) => contentRow(row, selectedType, row.content_id === selected?.content_id)).join('')
                : `<div class="empty-line">${escapeHTML(selectedConfig.emptyLabel)}</div>`
            }
          </div>
        </section>
      </div>
      ${selected ? contentDetail(selected, selectedType) : ''}
      ${validation && !validation.valid ? validationIssues(validation.issues) : ''}
      ${
        publish?.published
          ? `<div class="empty-line">Published ${escapeHTML(publish.version?.version ?? 'content')} (${publish.row_count} rows).</div>`
          : ''
      }
      ${
        audit?.entries.length
          ? `<div class="cms-audit-strip">${audit.entries
              .slice(0, 3)
              .map((entry) => `<span>${escapeHTML(entry.content_type)}:${escapeHTML(entry.content_id)}</span>`)
              .join('')}</div>`
          : ''
      }
    </div>
  `;
}

function typeButton(type: AdminContentTypeConfig, selectedType: AdminContentTypeID, count: number): string {
  return `
    <button type="button" data-action="admin-content-type-select" data-content-type="${type.id}" data-selected="${type.id === selectedType ? 'true' : 'false'}">
      <span>${escapeHTML(type.shortLabel)}</span>
      <em>${count}</em>
    </button>
  `;
}

function currentVersionLabel(versions: AdminContentVersionSummary[]): string {
  return versions.find((version) => version.current)?.version ?? versions[0]?.version ?? '--';
}

function versionRow(version: AdminContentVersionSummary): string {
  return `
    <div class="cms-version-row" data-current="${version.current ? 'true' : 'false'}">
      <div>
        <strong>${escapeHTML(version.version)}</strong>
        <small>${escapeHTML(version.status || 'unknown')}</small>
      </div>
      <button type="button" data-action="admin-content-rollback" data-version-id="${escapeHTML(version.id)}" ${version.current ? 'disabled' : ''}>Rollback</button>
    </div>
  `;
}

function contentRow(row: AdminContentDraftRow, contentType: AdminContentTypeID, selected: boolean): string {
  const name = contentDisplayName(row, contentType);
  return `
    <button type="button" class="cms-row" data-action="admin-content-select" data-content-type="${contentType}" data-content-id="${escapeHTML(row.content_id)}" data-selected="${selected ? 'true' : 'false'}">
      <span class="cms-row__mark" data-enabled="${row.enabled ? 'true' : 'false'}"></span>
      <span>
        <strong>${escapeHTML(name)}</strong>
        <small>${escapeHTML(row.content_id)}</small>
      </span>
      <em>${escapeHTML(rowBadge(row, contentType))}</em>
    </button>
  `;
}

function contentDetail(row: AdminContentDraftRow, contentType: AdminContentTypeID): string {
  const fields = adminContentEditFields(row);
  const editable = fields.length > 0;
  return `
    <section class="cms-detail">
      <header>
        <div class="cms-detail__heading">
          <span>${escapeHTML(`${adminContentTypeConfig(contentType).shortLabel} ${row.enabled ? 'enabled' : 'disabled'}`)}</span>
          <strong>${escapeHTML(contentDisplayName(row, contentType))}</strong>
        </div>
        <button type="button" class="cms-detail__edit" data-action="${contentType === 'module' ? 'admin-content-module-edit' : 'admin-content-edit'}" data-content-type="${contentType}" data-content-id="${escapeHTML(row.content_id)}" ${editable ? '' : 'disabled'}>Edit</button>
      </header>
      <div class="cms-stat-grid">
        ${contentSummaryStats(row, contentType).map(([label, value]) => cmsStat(label, value)).join('')}
      </div>
    </section>
  `;
}

export function adminContentEditModal(state: ClientState, contentID?: string): string {
  if (!state.auth.session?.account?.admin) {
    return '';
  }
  const contentType = selectedAdminContentType();
  const row = findAdminContentDraftRow(state, contentType, contentID ?? hudSelection.selectedAdminContentID);
  if (!row) {
    return `<div class="empty-line">No ${escapeHTML(adminContentTypeConfig(contentType).shortLabel)} draft selected.</div>`;
  }
  const fields = adminContentEditFields(row);
  if (fields.length === 0) {
    return '<div class="empty-line">Selected draft has no editable CMS fields.</div>';
  }
  const legacyModuleForm = contentType === 'module' ? 'data-admin-content-module-form="true"' : '';
  return `
    <form class="cms-edit-form" data-admin-content-form="true" ${legacyModuleForm} data-content-id="${escapeHTML(row.content_id)}" data-content-type="${contentType}">
      <div class="cms-edit-summary">
        <strong>${escapeHTML(contentDisplayName(row, contentType))}</strong>
        <span>${escapeHTML(`${adminContentTypeConfig(contentType).label} / ${row.content_id}`)}</span>
      </div>
      <div class="cms-edit-grid">
        ${fields.map((field) => adminContentEditControl(field)).join('')}
      </div>
      <div class="cms-edit-actions">
        <button type="button" data-modal-close="button">Cancel</button>
        <button type="button" data-action="admin-content-save">Save</button>
      </div>
    </form>
  `;
}

export function adminModuleEditModal(state: ClientState, contentID?: string): string {
  hudSelection.selectedAdminContentType = 'module';
  return adminContentEditModal(state, contentID);
}

export function findAdminContentDraftRow(
  state: ClientState,
  contentType: string | null,
  contentID: string | null,
): AdminContentDraftRow | null {
  if (!state.auth.session?.account?.admin) {
    return null;
  }
  const type = normalizeAdminContentType(contentType);
  const rows = state.adminContent?.rowsByType[type]?.rows ?? [];
  return rows.find((row) => row.content_id === contentID) ?? rows[0] ?? null;
}

export function findAdminModuleDraftRow(state: ClientState, contentID: string | null): AdminContentDraftRow | null {
  return findAdminContentDraftRow(state, 'module', contentID);
}

export function adminContentEditPatchFromForm(form: HTMLFormElement): AdminContentEditPatch {
  const patch: AdminContentEditPatch = {};
  const controls = form.querySelectorAll<HTMLInputElement | HTMLSelectElement>('[data-admin-content-field]');
  for (const control of controls) {
    const field = control.dataset.adminContentField;
    if (!isAdminContentEditFieldID(field)) {
      continue;
    }
    if (control instanceof HTMLInputElement && control.type === 'checkbox') {
      patch[field] = control.checked;
      continue;
    }
    if (control.dataset.fieldKind === 'text' || control.dataset.fieldKind === 'rarity') {
      const value = control.value.trim();
      if (value) {
        patch[field] = value;
      }
      continue;
    }
    const numeric = Number(control.value);
    if (!Number.isFinite(numeric)) {
      continue;
    }
    patch[field] = normalizeNumberPatch(field, numeric);
  }
  return patch;
}

export function adminModuleEditPatchFromForm(form: HTMLFormElement): AdminModuleEditPatch {
  const patch = adminContentEditPatchFromForm(form);
  const modulePatch: AdminModuleEditPatch = {};
  for (const field of ['weapon_damage', 'shield_damage', 'range', 'cooldown', 'energy', 'rank', 'rarity'] as const) {
    const value = patch[field];
    if (typeof value === 'number' || typeof value === 'string') {
      modulePatch[field] = value;
    }
  }
  return modulePatch;
}

export function buildAdminContentDraftUpdate(
  row: AdminContentDraftRow,
  patch: AdminContentEditPatch,
): AdminContentDraftUpdateInput {
  const contentType = normalizeAdminContentType(row.content_type ?? hudSelection.selectedAdminContentType);
  const displayJSON = cloneJsonObject(row.display_json);
  const dataJSON = cloneJsonObject(row.data_json);
  const available = new Set(adminContentEditFields(row).map((field) => field.id));
  let enabled = row.enabled;

  if (available.has('enabled') && typeof patch.enabled === 'boolean') {
    enabled = patch.enabled;
  }
  if (available.has('display_name') && typeof patch.display_name === 'string' && patch.display_name.trim() !== '') {
    setDisplayName(displayJSON, patch.display_name.trim());
  }
  if (available.has('rarity') && typeof patch.rarity === 'string' && patch.rarity.trim() !== '') {
    dataJSON.rarity = patch.rarity.trim();
  }
  patchNumberField(available, patch, 'max_stack', (value) => {
    dataJSON.max_stack = value;
  });
  patchNumberField(available, patch, 'weapon_damage', (value) => setWeaponDamageValue(dataJSON, value));
  patchNumberField(available, patch, 'shield_damage', (value) => setShieldDamageValue(dataJSON, value));
  patchNumberField(available, patch, 'range', (value) => setRangeValue(dataJSON, value));
  patchNumberField(available, patch, 'cooldown', (value) => setCooldownValue(dataJSON, value));
  patchNumberField(available, patch, 'energy', (value) => setEnergyCostValue(dataJSON, value));
  patchNumberField(available, patch, 'rank', (value) => setRankValue(dataJSON, value));
  patchNumberField(available, patch, 'ship_hp', (value) => setShipHPValue(dataJSON, value));
  patchNumberField(available, patch, 'shop_price', (value) => setShopPriceValue(dataJSON, value));
  patchNumberField(available, patch, 'npc_hp', (value) => setNPCHPValue(dataJSON, value));
  patchNumberField(available, patch, 'loot_chance', (value) => setLootChanceValue(dataJSON, value));
  patchNumberField(available, patch, 'craft_required_rank', (value) => setRankValue(dataJSON, value));
  patchNumberField(available, patch, 'production_rate', (value) => setProductionRateValue(dataJSON, value));

  return {
    contentType,
    contentID: row.content_id,
    enabled,
    displayJSON,
    dataJSON,
  };
}

export function buildAdminModuleDraftUpdate(
  row: AdminContentDraftRow,
  patch: AdminModuleEditPatch,
): AdminContentDraftUpdateInput {
  return buildAdminContentDraftUpdate(row, patch);
}

function validationIssues(issues: Array<{ path: string; code: string; message: string }>): string {
  return `
    <div class="cms-issues">
      ${issues
        .slice(0, 4)
        .map(
          (issue) =>
            `<div><strong>${escapeHTML(issue.code || 'invalid')}</strong><span>${escapeHTML(issue.path || '$')}</span><p>${escapeHTML(issue.message)}</p></div>`,
        )
        .join('')}
    </div>
  `;
}

function cmsStat(label: string, value: string | number | null | undefined): string {
  return `<div class="meta-row"><span>${escapeHTML(label)}</span><strong>${value === null || value === undefined || value === '' ? lockedValue() : escapeHTML(String(value))}</strong></div>`;
}

function adminContentTypeConfig(contentType: string): AdminContentTypeConfig {
  return ADMIN_CONTENT_EDITOR_TYPES.find((type) => type.id === contentType) ?? ADMIN_CONTENT_EDITOR_TYPES[0];
}

function selectedAdminContentType(): AdminContentTypeID {
  return normalizeAdminContentType(hudSelection.selectedAdminContentType);
}

function normalizeAdminContentType(value: string | null | undefined): AdminContentTypeID {
  return ADMIN_CONTENT_EDITOR_TYPES.some((type) => type.id === value) ? (value as AdminContentTypeID) : 'module';
}

function rowContentType(row: AdminContentDraftRow): AdminContentTypeID {
  return normalizeAdminContentType(row.content_type ?? hudSelection.selectedAdminContentType);
}

function contentDisplayName(row: AdminContentDraftRow, contentType = rowContentType(row)): string {
  return (
    jsonString(row.display_json.name) ??
    jsonString(row.display_json.display_name) ??
    jsonString(row.data_json.name) ??
    jsonString(row.data_json.display_name) ??
    jsonString(row.data_json.label) ??
    `${adminContentTypeConfig(contentType).shortLabel} draft`
  );
}

function rowBadge(row: AdminContentDraftRow, contentType: AdminContentTypeID): string {
  const data = row.data_json;
  switch (contentType) {
    case 'module':
    case 'item':
      return jsonString(data.rarity) ?? (row.enabled ? 'enabled' : 'disabled');
    case 'ship':
      return valueBadge('R', rankValue(data) ?? jsonNumber(data.rank_requirement));
    case 'shop_product':
      return valueBadge('$', shopPriceValue(data));
    case 'npc_template':
      return valueBadge('HP', npcHPValue(data));
    case ADMIN_CONTENT_LOOT_TABLE_TYPE:
      return valueBadge('%', lootChanceValue(data));
    case 'craft_recipe':
      return valueBadge('R', rankValue(data));
    case 'production_building':
      return valueBadge('/h', productionRateValue(data));
    default:
      return row.enabled ? 'enabled' : 'disabled';
  }
}

function valueBadge(prefix: string, value: number | null): string {
  return value === null ? '--' : `${prefix}${value}`;
}

function contentSummaryStats(row: AdminContentDraftRow, contentType: AdminContentTypeID): Array<[string, string | number | null]> {
  const data = row.data_json;
  switch (contentType) {
    case 'module':
      return [
        ['Rank', rankValue(data)],
        ['Tier', jsonNumber(data.tier)],
        ['Slot', jsonString(data.slot_type) ?? jsonString(data.module_category) ?? '--'],
        ['Weapon', weaponDamageValue(data)],
        ['Shield', shieldDamageValue(data)],
        ['Range', rangeValue(data)],
        ['Cooldown', cooldownValue(data) === null ? null : `${cooldownValue(data)}ms`],
        ['Energy', energyCostValue(data)],
      ];
    case 'item':
      return [
        ['Type', jsonString(data.item_type) ?? jsonString(data.category)],
        ['Rarity', jsonString(data.rarity)],
        ['Stack', jsonNumber(data.max_stack)],
        ['Weight', jsonNumber(data.weight_units) ?? jsonNumber(data.weight)],
      ];
    case 'ship':
      return [
        ['Rank', rankValue(data) ?? jsonNumber(data.rank_requirement)],
        ['Tier', jsonNumber(data.tier)],
        ['Role', jsonString(data.role_tag)],
        ['HP', shipHPValue(data)],
      ];
    case 'shop_product':
      return [
        ['Type', jsonString(data.product_type)],
        ['Price', shopPriceValue(data)],
        ['Currency', jsonString(objectValue(data.price_policy)?.currency_type) ?? jsonString(objectValue(data.price)?.currency_type)],
        ['Rank', jsonNumber(objectValue(data.availability)?.required_rank)],
      ];
    case 'npc_template':
      return [
        ['HP', npcHPValue(data)],
        ['Damage', jsonNumber(data.damage) ?? jsonNumber(objectValue(data.combat_stats)?.damage)],
        ['Rank', rankValue(data)],
        ['Loot', jsonString(data.loot_table_id)],
      ];
    case ADMIN_CONTENT_LOOT_TABLE_TYPE:
      return [
        ['First Chance', lootChanceValue(data)],
        ['Rows', Array.isArray(data.rows) ? data.rows.length : null],
        ['Owner Lock', jsonNumber(data.owner_lock_seconds)],
        ['Lifetime', jsonNumber(data.lifetime_seconds)],
      ];
    case 'craft_recipe':
      return [
        ['Rank', rankValue(data)],
        ['Output', jsonString(data.output_item_id) ?? jsonString(data.output_type)],
        ['Amount', jsonNumber(data.output_amount)],
        ['Duration', jsonNumber(data.craft_duration_ms) ?? jsonNumber(data.craft_duration_seconds)],
      ];
    case 'production_building':
      return [
        ['Rate', productionRateValue(data)],
        ['Level', jsonNumber(data.level)],
        ['Energy', jsonNumber(data.energy_cost_per_hour)],
        ['Output', firstItemID(data.outputs)],
      ];
    default:
      return [['Enabled', row.enabled ? 'yes' : 'no']];
  }
}

export function adminContentEditFields(row: AdminContentDraftRow): AdminContentEditField[] {
  const data = row.data_json;
  const displayName = contentDisplayName(row);
  const fields: AdminContentEditField[] = [{ id: 'enabled', label: 'Enabled', value: row.enabled, kind: 'boolean' }];
  switch (rowContentType(row)) {
    case 'module':
      fields.push(...adminModuleEditFields(row));
      break;
    case 'item':
      fields.push({ id: 'display_name', label: 'Name', value: displayName, kind: 'text' });
      pushNumberField(fields, 'max_stack', 'Max Stack', jsonNumber(data.max_stack), 1);
      pushRarityField(fields, data);
      break;
    case 'ship':
      pushNumberField(fields, 'ship_hp', 'Base HP', shipHPValue(data), 1);
      pushNumberField(fields, 'rank', 'Rank', rankValue(data) ?? jsonNumber(data.rank_requirement), 1);
      break;
    case 'shop_product':
      pushNumberField(fields, 'shop_price', 'Price', shopPriceValue(data), 0);
      break;
    case 'npc_template':
      pushNumberField(fields, 'npc_hp', 'NPC HP', npcHPValue(data), 1);
      break;
    case ADMIN_CONTENT_LOOT_TABLE_TYPE:
      pushNumberField(fields, 'loot_chance', 'First Chance', lootChanceValue(data), 0, 1, '0.001');
      break;
    case 'craft_recipe':
      pushNumberField(fields, 'craft_required_rank', 'Rank', rankValue(data), 1);
      break;
    case 'production_building':
      pushNumberField(fields, 'production_rate', 'Rate / Hour', productionRateValue(data), 0);
      break;
  }
  return fields;
}

export function adminModuleEditFields(row: AdminContentDraftRow): AdminModuleEditField[] {
  const data = row.data_json;
  const fields: AdminModuleEditField[] = [];
  pushNumberField(fields, 'weapon_damage', 'Weapon', weaponDamageValue(data), 0);
  pushNumberField(fields, 'shield_damage', 'Shield', shieldDamageValue(data), 0);
  pushNumberField(fields, 'range', 'Range', rangeValue(data), 0);
  pushNumberField(fields, 'cooldown', 'Cooldown', cooldownValue(data), 0);
  pushNumberField(fields, 'energy', 'Energy', energyCostValue(data), 0);
  pushNumberField(fields, 'rank', 'Rank', rankValue(data), 1);
  pushRarityField(fields, data);
  return fields;
}

function adminContentEditControl(field: AdminContentEditField): string {
  const label = escapeHTML(field.label);
  if (field.kind === 'boolean') {
    return `
      <label class="cms-edit-control cms-edit-control--toggle">
        <span>${label}</span>
        <input type="checkbox" data-admin-content-field="${field.id}" name="${field.id}" ${field.value === true ? 'checked' : ''} />
      </label>
    `;
  }
  if (field.kind === 'rarity') {
    return `
      <label class="cms-edit-control">
        <span>${label}</span>
        <select data-admin-content-field="${field.id}" data-admin-module-field="${field.id}" data-field-kind="rarity" name="${field.id}" required>
          ${rarityOptions(String(field.value))}
        </select>
      </label>
    `;
  }
  if (field.kind === 'text') {
    return `
      <label class="cms-edit-control">
        <span>${label}</span>
        <input type="text" value="${escapeHTML(String(field.value))}" data-admin-content-field="${field.id}" data-field-kind="text" name="${field.id}" required />
      </label>
    `;
  }
  return `
    <label class="cms-edit-control">
      <span>${label}</span>
      <input type="number" inputmode="numeric" min="${field.min ?? 0}" ${field.max === undefined ? '' : `max="${field.max}"`} step="${field.step ?? '1'}" value="${escapeHTML(String(field.value))}" data-admin-content-field="${field.id}" data-admin-module-field="${field.id}" name="${field.id}" required />
    </label>
  `;
}

function rarityOptions(current: string): string {
  const options = ['common', 'uncommon', 'rare', 'epic', 'legendary'];
  if (current && !options.includes(current)) {
    options.push(current);
  }
  return options
    .map((option) => `<option value="${escapeHTML(option)}" ${option === current ? 'selected' : ''}>${escapeHTML(option)}</option>`)
    .join('');
}

function pushNumberField(
  fields: AdminContentEditField[],
  id: AdminContentEditFieldID,
  label: string,
  value: number | null,
  min: number,
  max?: number,
  step?: string,
): void {
  if (value !== null) {
    fields.push({ id, label, value, kind: 'number', min, max, step });
  }
}

function pushRarityField(fields: AdminContentEditField[], data: Record<string, unknown>): void {
  const rarity = jsonString(data.rarity);
  if (rarity !== null) {
    fields.push({ id: 'rarity', label: 'Rarity', value: rarity, kind: 'rarity' });
  }
}

function isAdminContentEditFieldID(value: string | undefined): value is AdminContentEditFieldID {
  return (
    value === 'enabled' ||
    value === 'display_name' ||
    value === 'rarity' ||
    value === 'max_stack' ||
    value === 'weapon_damage' ||
    value === 'shield_damage' ||
    value === 'range' ||
    value === 'cooldown' ||
    value === 'energy' ||
    value === 'rank' ||
    value === 'ship_hp' ||
    value === 'shop_price' ||
    value === 'npc_hp' ||
    value === 'loot_chance' ||
    value === 'craft_required_rank' ||
    value === 'production_rate'
  );
}

function normalizeNumberPatch(field: AdminContentEditFieldID, numeric: number): number {
  const min = field === 'rank' || field === 'craft_required_rank' || field === 'ship_hp' || field === 'npc_hp' || field === 'max_stack' ? 1 : 0;
  if (field === 'loot_chance') {
    return Math.min(1, Math.max(0, numeric));
  }
  return Math.max(min, Math.round(numeric));
}

function patchNumberField(
  available: ReadonlySet<AdminContentEditFieldID>,
  patch: AdminContentEditPatch,
  field: AdminContentEditFieldID,
  apply: (value: number) => void,
): void {
  const value = patch[field];
  if (available.has(field) && typeof value === 'number' && Number.isFinite(value)) {
    apply(normalizeNumberPatch(field, value));
  }
}

function weaponDamageValue(data: Record<string, unknown>): number | null {
  return statModifierValue(data, 'weapon_damage') ?? jsonNumber(data.weapon_damage) ?? jsonNumber(data.damage);
}

function shieldDamageValue(data: Record<string, unknown>): number | null {
  return statModifierValue(data, 'shield_damage') ?? jsonNumber(data.shield_damage);
}

function rangeValue(data: Record<string, unknown>): number | null {
  return statModifierValue(data, 'range') ?? jsonNumber(data.range) ?? jsonNumber(data.range_units);
}

function cooldownValue(data: Record<string, unknown>): number | null {
  return firstCooldownMS(data) ?? jsonNumber(objectValue(data.cooldown)?.duration_ms) ?? jsonNumber(data.cooldown_ms);
}

function energyCostValue(data: Record<string, unknown>): number | null {
  return jsonNumber(objectValue(data.energy)?.activation_cost) ?? jsonNumber(data.energy_cost);
}

function rankValue(data: Record<string, unknown>): number | null {
  return jsonNumber(data.required_rank) ?? jsonNumber(data.rank);
}

function shipHPValue(data: Record<string, unknown>): number | null {
  return jsonNumber(data.base_hp) ?? jsonNumber(objectValue(data.base_stats)?.hp) ?? jsonNumber(objectValue(data.base_stats)?.hull);
}

function shopPriceValue(data: Record<string, unknown>): number | null {
  return jsonNumber(objectValue(data.price_policy)?.amount) ?? jsonNumber(objectValue(data.price)?.amount) ?? jsonNumber(data.credit_price);
}

function npcHPValue(data: Record<string, unknown>): number | null {
  return jsonNumber(data.max_hp) ?? jsonNumber(objectValue(data.combat_stats)?.max_hp) ?? jsonNumber(objectValue(data.stats)?.max_hp);
}

function lootChanceValue(data: Record<string, unknown>): number | null {
  const firstRow = firstObject(data.rows) ?? firstObject(data.drops);
  return jsonNumber(firstRow?.chance) ?? jsonNumber(data.chance);
}

function productionRateValue(data: Record<string, unknown>): number | null {
  const firstOutput = firstObject(data.outputs);
  return jsonNumber(data.base_rate_per_hour) ?? jsonNumber(firstOutput?.amount_per_hour);
}

function firstItemID(value: unknown): string | null {
  const first = firstObject(value);
  return jsonString(first?.item_id);
}

function statModifierValue(data: Record<string, unknown>, stat: string): number | null {
  if (!Array.isArray(data.stat_modifiers)) {
    return null;
  }
  for (const item of data.stat_modifiers) {
    const modifier = objectValue(item);
    if (jsonString(modifier?.stat) === stat) {
      return jsonNumber(modifier?.value);
    }
  }
  return null;
}

function setDisplayName(displayJSON: Record<string, unknown>, value: string): void {
  if (jsonString(displayJSON.display_name) !== null) {
    displayJSON.display_name = value;
    return;
  }
  if (jsonString(displayJSON.name) !== null) {
    displayJSON.name = value;
    return;
  }
  displayJSON.display_name = value;
}

function setWeaponDamageValue(data: Record<string, unknown>, value: number): void {
  if (setStatModifierValue(data, 'weapon_damage', value)) {
    return;
  }
  if (jsonNumber(data.weapon_damage) !== null) {
    data.weapon_damage = value;
    return;
  }
  if (jsonNumber(data.damage) !== null) {
    data.damage = value;
  }
}

function setShieldDamageValue(data: Record<string, unknown>, value: number): void {
  if (setStatModifierValue(data, 'shield_damage', value)) {
    return;
  }
  if (jsonNumber(data.shield_damage) !== null) {
    data.shield_damage = value;
  }
}

function setRangeValue(data: Record<string, unknown>, value: number): void {
  if (setStatModifierValue(data, 'range', value)) {
    return;
  }
  if (jsonNumber(data.range) !== null) {
    data.range = value;
    return;
  }
  if (jsonNumber(data.range_units) !== null) {
    data.range_units = value;
  }
}

function setCooldownValue(data: Record<string, unknown>, value: number): void {
  if (Array.isArray(data.cooldowns)) {
    const first = objectValue(data.cooldowns[0]);
    if (jsonNumber(first?.duration_ms) !== null && first) {
      first.duration_ms = value;
      return;
    }
  }
  const cooldown = objectValue(data.cooldown);
  if (jsonNumber(cooldown?.duration_ms) !== null && cooldown) {
    cooldown.duration_ms = value;
    return;
  }
  if (jsonNumber(data.cooldown_ms) !== null) {
    data.cooldown_ms = value;
  }
}

function setEnergyCostValue(data: Record<string, unknown>, value: number): void {
  const energy = objectValue(data.energy);
  if (jsonNumber(energy?.activation_cost) !== null && energy) {
    energy.activation_cost = value;
    return;
  }
  if (jsonNumber(data.energy_cost) !== null) {
    data.energy_cost = value;
  }
}

function setRankValue(data: Record<string, unknown>, value: number): void {
  if (jsonNumber(data.required_rank) !== null) {
    data.required_rank = value;
    return;
  }
  if (jsonNumber(data.rank_requirement) !== null) {
    data.rank_requirement = value;
    return;
  }
  if (jsonNumber(data.rank) !== null) {
    data.rank = value;
  }
}

function setShipHPValue(data: Record<string, unknown>, value: number): void {
  const baseStats = objectValue(data.base_stats);
  if (jsonNumber(baseStats?.hp) !== null && baseStats) {
    baseStats.hp = value;
    return;
  }
  if (jsonNumber(baseStats?.hull) !== null && baseStats) {
    baseStats.hull = value;
    return;
  }
  if (jsonNumber(data.base_hp) !== null) {
    data.base_hp = value;
  }
}

function setShopPriceValue(data: Record<string, unknown>, value: number): void {
  const pricePolicy = objectValue(data.price_policy);
  if (jsonNumber(pricePolicy?.amount) !== null && pricePolicy) {
    pricePolicy.amount = value;
    return;
  }
  const price = objectValue(data.price);
  if (jsonNumber(price?.amount) !== null && price) {
    price.amount = value;
    return;
  }
  if (jsonNumber(data.credit_price) !== null) {
    data.credit_price = value;
  }
}

function setNPCHPValue(data: Record<string, unknown>, value: number): void {
  const combatStats = objectValue(data.combat_stats);
  if (jsonNumber(combatStats?.max_hp) !== null && combatStats) {
    combatStats.max_hp = value;
    return;
  }
  const stats = objectValue(data.stats);
  if (jsonNumber(stats?.max_hp) !== null && stats) {
    stats.max_hp = value;
    return;
  }
  if (jsonNumber(data.max_hp) !== null) {
    data.max_hp = value;
  }
}

function setLootChanceValue(data: Record<string, unknown>, value: number): void {
  const firstRow = firstObject(data.rows) ?? firstObject(data.drops);
  if (firstRow && jsonNumber(firstRow.chance) !== null) {
    firstRow.chance = value;
    return;
  }
  if (jsonNumber(data.chance) !== null) {
    data.chance = value;
  }
}

function setProductionRateValue(data: Record<string, unknown>, value: number): void {
  const firstOutput = firstObject(data.outputs);
  if (firstOutput && jsonNumber(firstOutput.amount_per_hour) !== null) {
    firstOutput.amount_per_hour = value;
    return;
  }
  if (jsonNumber(data.base_rate_per_hour) !== null) {
    data.base_rate_per_hour = value;
  }
}

function setStatModifierValue(data: Record<string, unknown>, stat: string, value: number): boolean {
  if (!Array.isArray(data.stat_modifiers)) {
    return false;
  }
  for (const item of data.stat_modifiers) {
    const modifier = objectValue(item);
    if (modifier && jsonString(modifier.stat) === stat) {
      modifier.value = value;
      return true;
    }
  }
  return false;
}

function firstCooldownMS(data: Record<string, unknown>): number | null {
  if (!Array.isArray(data.cooldowns)) {
    return null;
  }
  const first = objectValue(data.cooldowns[0]);
  return jsonNumber(first?.duration_ms);
}

function firstObject(value: unknown): Record<string, unknown> | null {
  return Array.isArray(value) ? objectValue(value[0]) : null;
}

function objectValue(value: unknown): Record<string, unknown> | null {
  return typeof value === 'object' && value !== null && !Array.isArray(value) ? (value as Record<string, unknown>) : null;
}

function jsonString(value: unknown): string | null {
  return typeof value === 'string' && value.trim() !== '' ? value : null;
}

function jsonNumber(value: unknown): number | null {
  return typeof value === 'number' && Number.isFinite(value) ? value : null;
}

function cloneJsonObject(value: Record<string, unknown>): Record<string, unknown> {
  return cloneJsonValue(value) as Record<string, unknown>;
}

function cloneJsonValue(value: unknown): unknown {
  if (Array.isArray(value)) {
    return value.map((item) => cloneJsonValue(item));
  }
  if (typeof value === 'object' && value !== null) {
    const cloned: Record<string, unknown> = {};
    for (const [key, child] of Object.entries(value)) {
      cloned[key] = cloneJsonValue(child);
    }
    return cloned;
  }
  return value;
}
