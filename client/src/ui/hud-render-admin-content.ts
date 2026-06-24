import type { AdminContentDraftRow, AdminContentVersionSummary, ClientState } from '../state/types';
import { escapeHTML, lockedValue } from './hud-formatters';
import { hudSelection } from './hud-selection';
import type { AdminContentDraftUpdateInput } from './hud-types';

export type AdminModuleEditFieldID =
  | 'weapon_damage'
  | 'shield_damage'
  | 'range'
  | 'cooldown'
  | 'energy'
  | 'rank'
  | 'rarity';

export type AdminModuleEditPatch = Partial<Record<AdminModuleEditFieldID, number | string>>;

export interface AdminModuleEditField {
  id: AdminModuleEditFieldID;
  label: string;
  value: number | string;
  kind: 'number' | 'rarity';
  min?: number;
}

export function adminContentBlock(state: ClientState): string {
  if (!state.auth.session?.account?.admin) {
    return '';
  }
  const content = state.adminContent;
  const versions = content?.versions?.versions ?? [];
  const moduleList = content?.rowsByType.module ?? null;
  const modules = moduleList?.rows ?? [];
  const selectedID = hudSelection.selectedAdminContentID ?? modules[0]?.content_id ?? null;
  const selected = modules.find((row) => row.content_id === selectedID) ?? modules[0] ?? null;
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
      <div class="cms-metrics">
        <span><em>Version</em><strong>${escapeHTML(currentVersionLabel(versions))}</strong></span>
        <span><em>Modules</em><strong>${moduleList ? moduleList.total : lockedValue()}</strong></span>
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
          <header><strong>Modules</strong><span>${moduleList ? moduleList.total : 0}</span></header>
          <div class="cms-list">
            ${
              modules.length
                ? modules.map((row) => moduleRow(row, row.content_id === selected?.content_id)).join('')
                : '<div class="empty-line">No module draft rows loaded.</div>'
            }
          </div>
        </section>
      </div>
      ${selected ? moduleDetail(selected) : ''}
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

function moduleRow(row: AdminContentDraftRow, selected: boolean): string {
  const name = contentDisplayName(row);
  const rarity = jsonString(row.data_json.rarity) ?? 'common';
  return `
    <button type="button" class="cms-row" data-action="admin-content-select" data-content-type="module" data-content-id="${escapeHTML(row.content_id)}" data-selected="${selected ? 'true' : 'false'}">
      <span class="cms-row__mark" data-enabled="${row.enabled ? 'true' : 'false'}"></span>
      <span>
        <strong>${escapeHTML(name)}</strong>
        <small>${escapeHTML(row.content_id)}</small>
      </span>
      <em>${escapeHTML(rarity)}</em>
    </button>
  `;
}

function moduleDetail(row: AdminContentDraftRow): string {
  const data = row.data_json;
  const weaponDamage = weaponDamageValue(data);
  const shieldDamage = shieldDamageValue(data);
  const range = rangeValue(data);
  const cooldown = cooldownValue(data);
  const energy = energyCostValue(data);
  const rank = rankValue(data);
  const tier = jsonNumber(data.tier);
  const slot = jsonString(data.slot_type) ?? jsonString(data.module_category) ?? '--';
  const editable = adminModuleEditFields(row).length > 0;
  return `
    <section class="cms-detail">
      <header>
        <div class="cms-detail__heading">
          <span>${escapeHTML(row.enabled ? 'enabled' : 'disabled')}</span>
          <strong>${escapeHTML(contentDisplayName(row))}</strong>
        </div>
        <button type="button" class="cms-detail__edit" data-action="admin-content-module-edit" data-content-id="${escapeHTML(row.content_id)}" ${editable ? '' : 'disabled'}>Edit</button>
      </header>
      <div class="cms-stat-grid">
        ${cmsStat('Rank', rank)}
        ${cmsStat('Tier', tier)}
        ${cmsStat('Slot', slot)}
        ${cmsStat('Weapon', weaponDamage)}
        ${cmsStat('Shield', shieldDamage)}
        ${cmsStat('Range', range)}
        ${cmsStat('Cooldown', cooldown === null ? null : `${cooldown}ms`)}
        ${cmsStat('Energy', energy)}
      </div>
    </section>
  `;
}

export function adminModuleEditModal(state: ClientState, contentID?: string): string {
  if (!state.auth.session?.account?.admin) {
    return '';
  }
  const row = findAdminModuleDraftRow(state, contentID ?? hudSelection.selectedAdminContentID ?? null);
  if (!row) {
    return '<div class="empty-line">No module draft selected.</div>';
  }
  const fields = adminModuleEditFields(row);
  if (fields.length === 0) {
    return '<div class="empty-line">Selected module has no editable LC1 fields.</div>';
  }
  return `
    <form class="cms-edit-form" data-admin-content-module-form="true" data-content-id="${escapeHTML(row.content_id)}" data-content-type="${escapeHTML(row.content_type ?? 'module')}">
      <div class="cms-edit-summary">
        <strong>${escapeHTML(contentDisplayName(row))}</strong>
        <span>${escapeHTML(row.content_id)}</span>
      </div>
      <div class="cms-edit-grid">
        ${fields.map((field) => adminModuleEditControl(field)).join('')}
      </div>
      <div class="cms-edit-actions">
        <button type="button" data-modal-close="button">Cancel</button>
        <button type="button" data-action="admin-content-module-save">Save</button>
      </div>
    </form>
  `;
}

export function findAdminModuleDraftRow(state: ClientState, contentID: string | null): AdminContentDraftRow | null {
  if (!state.auth.session?.account?.admin) {
    return null;
  }
  const modules = state.adminContent?.rowsByType.module?.rows ?? [];
  return modules.find((row) => row.content_id === contentID) ?? modules[0] ?? null;
}

export function adminModuleEditPatchFromForm(form: HTMLFormElement): AdminModuleEditPatch {
  const patch: AdminModuleEditPatch = {};
  const controls = form.querySelectorAll<HTMLInputElement | HTMLSelectElement>('[data-admin-module-field]');
  for (const control of controls) {
    const field = control.dataset.adminModuleField;
    if (!isAdminModuleEditFieldID(field)) {
      continue;
    }
    if (field === 'rarity') {
      const value = control.value.trim();
      if (value) {
        patch.rarity = value;
      }
      continue;
    }
    const numeric = Number(control.value);
    if (!Number.isFinite(numeric)) {
      continue;
    }
    patch[field] = Math.max(field === 'rank' ? 1 : 0, Math.round(numeric));
  }
  return patch;
}

export function buildAdminModuleDraftUpdate(
  row: AdminContentDraftRow,
  patch: AdminModuleEditPatch,
): AdminContentDraftUpdateInput {
  const dataJSON = cloneJsonObject(row.data_json);
  const available = new Set(adminModuleEditFields(row).map((field) => field.id));
  patchNumberField(available, patch, 'weapon_damage', (value) => setWeaponDamageValue(dataJSON, value));
  patchNumberField(available, patch, 'shield_damage', (value) => setShieldDamageValue(dataJSON, value));
  patchNumberField(available, patch, 'range', (value) => setRangeValue(dataJSON, value));
  patchNumberField(available, patch, 'cooldown', (value) => setCooldownValue(dataJSON, value));
  patchNumberField(available, patch, 'energy', (value) => setEnergyCostValue(dataJSON, value));
  patchNumberField(available, patch, 'rank', (value) => setRankValue(dataJSON, value));
  if (available.has('rarity') && typeof patch.rarity === 'string' && patch.rarity.trim() !== '') {
    dataJSON.rarity = patch.rarity.trim();
  }
  return {
    contentType: row.content_type ?? 'module',
    contentID: row.content_id,
    enabled: row.enabled,
    displayJSON: cloneJsonObject(row.display_json),
    dataJSON,
  };
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

function contentDisplayName(row: AdminContentDraftRow): string {
  return (
    jsonString(row.display_json.name) ??
    jsonString(row.display_json.display_name) ??
    jsonString(row.data_json.name) ??
    jsonString(row.data_json.display_name) ??
    'Unnamed module'
  );
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
  const rarity = jsonString(data.rarity);
  if (rarity !== null) {
    fields.push({ id: 'rarity', label: 'Rarity', value: rarity, kind: 'rarity' });
  }
  return fields;
}

function adminModuleEditControl(field: AdminModuleEditField): string {
  const label = escapeHTML(field.label);
  if (field.kind === 'rarity') {
    return `
      <label class="cms-edit-control">
        <span>${label}</span>
        <select data-admin-module-field="${field.id}" name="${field.id}" required>
          ${rarityOptions(String(field.value))}
        </select>
      </label>
    `;
  }
  return `
    <label class="cms-edit-control">
      <span>${label}</span>
      <input type="number" inputmode="numeric" min="${field.min ?? 0}" step="1" value="${escapeHTML(String(field.value))}" data-admin-module-field="${field.id}" name="${field.id}" required />
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
  fields: AdminModuleEditField[],
  id: AdminModuleEditFieldID,
  label: string,
  value: number | null,
  min: number,
): void {
  if (value !== null) {
    fields.push({ id, label, value, kind: 'number', min });
  }
}

function isAdminModuleEditFieldID(value: string | undefined): value is AdminModuleEditFieldID {
  return (
    value === 'weapon_damage' ||
    value === 'shield_damage' ||
    value === 'range' ||
    value === 'cooldown' ||
    value === 'energy' ||
    value === 'rank' ||
    value === 'rarity'
  );
}

function patchNumberField(
  available: ReadonlySet<AdminModuleEditFieldID>,
  patch: AdminModuleEditPatch,
  field: AdminModuleEditFieldID,
  apply: (value: number) => void,
): void {
  const value = patch[field];
  if (available.has(field) && typeof value === 'number' && Number.isFinite(value)) {
    apply(Math.max(field === 'rank' ? 1 : 0, Math.round(value)));
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
  return (
    firstCooldownMS(data) ??
    jsonNumber(objectValue(data.cooldown)?.duration_ms) ??
    jsonNumber(data.cooldown_ms)
  );
}

function energyCostValue(data: Record<string, unknown>): number | null {
  return jsonNumber(objectValue(data.energy)?.activation_cost) ?? jsonNumber(data.energy_cost);
}

function rankValue(data: Record<string, unknown>): number | null {
  return jsonNumber(data.required_rank) ?? jsonNumber(data.rank);
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
  if (jsonNumber(data.rank) !== null) {
    data.rank = value;
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
