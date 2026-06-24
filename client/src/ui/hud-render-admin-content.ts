import type { AdminContentDraftRow, AdminContentVersionSummary, ClientState } from '../state/types';
import { escapeHTML, lockedValue } from './hud-formatters';
import { hudSelection } from './hud-selection';

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
  const weaponDamage = statModifierValue(data, 'weapon_damage');
  const shieldDamage = statModifierValue(data, 'shield_damage');
  const cooldown = firstCooldownMS(data);
  const energy = jsonNumber(objectValue(data.energy)?.activation_cost);
  const rank = jsonNumber(data.required_rank);
  const tier = jsonNumber(data.tier);
  const slot = jsonString(data.slot_type) ?? jsonString(data.module_category) ?? '--';
  return `
    <section class="cms-detail">
      <header>
        <span>${escapeHTML(row.enabled ? 'enabled' : 'disabled')}</span>
        <strong>${escapeHTML(contentDisplayName(row))}</strong>
      </header>
      <div class="cms-stat-grid">
        ${cmsStat('Rank', rank)}
        ${cmsStat('Tier', tier)}
        ${cmsStat('Slot', slot)}
        ${cmsStat('Weapon', weaponDamage)}
        ${cmsStat('Shield', shieldDamage)}
        ${cmsStat('Cooldown', cooldown === null ? null : `${cooldown}ms`)}
        ${cmsStat('Energy', energy)}
      </div>
    </section>
  `;
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
