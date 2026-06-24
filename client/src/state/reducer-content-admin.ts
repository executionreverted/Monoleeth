import { JsonObject } from '../protocol/envelope';
import {
  AdminContentAuditEntry,
  AdminContentAuditLog,
  AdminContentDraftList,
  AdminContentDraftRow,
  AdminContentPublishSummary,
  AdminContentRollbackSummary,
  AdminContentState,
  AdminContentValidation,
  AdminContentVersionSummary,
  AdminContentVersionsSummary,
  ClientState,
} from './types';
import {
  booleanField,
  isJsonObject,
  numberField,
  objectField,
  optionalRoundedNumber,
  roundedOptional,
  stringField,
} from './reducer-helpers';

const adminContentPayloadKeys = [
  'content_versions',
  'content',
  'content_row',
  'validation',
  'content_publish',
  'content_rollback',
  'content_audit_log',
] as const;

export function hasAdminContentPayload(payload: JsonObject): boolean {
  return adminContentPayloadKeys.some((key) => objectField(payload, key) !== null);
}

export function applyAdminContentPayload(state: ClientState, payload: JsonObject): ClientState {
  if (!hasAdminContentPayload(payload)) {
    return state;
  }
  let adminContent = ensureAdminContentState(state.adminContent);

  const versions = objectField(payload, 'content_versions');
  if (versions) {
    adminContent = {
      ...adminContent,
      versions: parseVersions(versions, adminContent.versions),
    };
  }

  const list = objectField(payload, 'content');
  if (list) {
    const parsedList = parseDraftList(list, adminContent.rowsByType[stringField(list, 'content_type') ?? '']);
    if (parsedList) {
      adminContent = {
        ...adminContent,
        rowsByType: {
          ...adminContent.rowsByType,
          [parsedList.content_type]: parsedList,
        },
      };
    }
  }

  const row = objectField(payload, 'content_row');
  if (row) {
    const parsedRow = parseDraftRow(row);
    if (parsedRow) {
      const contentType = parsedRow.content_type ?? stringField(row, 'content_type') ?? '';
      const currentList = contentType ? adminContent.rowsByType[contentType] : null;
      adminContent = {
        ...adminContent,
        selectedRow: parsedRow,
        rowsByType:
          contentType && currentList
            ? {
                ...adminContent.rowsByType,
                [contentType]: upsertDraftRow(currentList, parsedRow),
              }
            : adminContent.rowsByType,
      };
    }
  }

  const validation = objectField(payload, 'validation');
  if (validation) {
    adminContent = {
      ...adminContent,
      validation: parseValidation(validation, adminContent.validation),
    };
  }

  const publish = objectField(payload, 'content_publish');
  if (publish) {
    const parsedPublish = parsePublish(publish, adminContent.publish);
    adminContent = {
      ...adminContent,
      publish: parsedPublish,
      validation: parsedPublish.validation,
      versions: mergeVersion(adminContent.versions, parsedPublish.version),
    };
  }

  const rollback = objectField(payload, 'content_rollback');
  if (rollback) {
    const parsedRollback = parseRollback(rollback, adminContent.rollback);
    adminContent = {
      ...adminContent,
      rollback: parsedRollback,
      validation: parsedRollback.validation,
      versions: mergeVersion(adminContent.versions, parsedRollback.version),
    };
  }

  const auditLog = objectField(payload, 'content_audit_log');
  if (auditLog) {
    adminContent = {
      ...adminContent,
      auditLog: parseAuditLog(auditLog, adminContent.auditLog),
    };
  }

  return {
    ...state,
    adminContent,
  };
}

function ensureAdminContentState(state: AdminContentState | null): AdminContentState {
  return (
    state ?? {
      versions: null,
      rowsByType: {},
      selectedRow: null,
      validation: null,
      publish: null,
      rollback: null,
      auditLog: null,
    }
  );
}

function parseVersions(payload: JsonObject, fallback: AdminContentVersionsSummary | null): AdminContentVersionsSummary {
  const versions = Array.isArray(payload.versions)
    ? payload.versions.filter(isJsonObject).map(parseVersion).filter((version): version is AdminContentVersionSummary => version !== null)
    : fallback?.versions ?? [];
  return {
    versions,
    total: Math.max(0, Math.round(numberField(payload, 'total') ?? fallback?.total ?? versions.length)),
    limit: Math.max(0, Math.round(numberField(payload, 'limit') ?? fallback?.limit ?? versions.length)),
    offset: Math.max(0, Math.round(numberField(payload, 'offset') ?? fallback?.offset ?? 0)),
    generated_at: Math.max(0, Math.round(numberField(payload, 'generated_at') ?? fallback?.generated_at ?? 0)),
  };
}

function parseVersion(payload: JsonObject): AdminContentVersionSummary | null {
  const id = stringField(payload, 'id') ?? '';
  const version = stringField(payload, 'version') ?? '';
  if (!id || !version) {
    return null;
  }
  return {
    id,
    version,
    status: stringField(payload, 'status') ?? '',
    current: booleanField(payload, 'current') ?? false,
    notes: stringField(payload, 'notes') ?? undefined,
    balance_tag: stringField(payload, 'balance_tag') ?? undefined,
    created_by: stringField(payload, 'created_by') ?? undefined,
    created_at: Math.max(0, Math.round(numberField(payload, 'created_at') ?? 0)),
    published_by: stringField(payload, 'published_by') ?? undefined,
    published_at: optionalRoundedNumber(payload, 'published_at', undefined),
    rolled_back_from: stringField(payload, 'rolled_back_from') ?? undefined,
  };
}

function parseDraftList(payload: JsonObject, fallback: AdminContentDraftList | undefined): AdminContentDraftList | null {
  const contentType = stringField(payload, 'content_type') ?? fallback?.content_type ?? '';
  if (!contentType) {
    return null;
  }
  const rows = Array.isArray(payload.rows)
    ? payload.rows.filter(isJsonObject).map(parseDraftRow).filter((row): row is AdminContentDraftRow => row !== null)
    : fallback?.rows ?? [];
  return {
    content_type: contentType,
    rows,
    total: Math.max(0, Math.round(numberField(payload, 'total') ?? fallback?.total ?? rows.length)),
    limit: Math.max(0, Math.round(numberField(payload, 'limit') ?? fallback?.limit ?? rows.length)),
    offset: Math.max(0, Math.round(numberField(payload, 'offset') ?? fallback?.offset ?? 0)),
    generated_at: Math.max(0, Math.round(numberField(payload, 'generated_at') ?? fallback?.generated_at ?? 0)),
  };
}

function parseDraftRow(payload: JsonObject): AdminContentDraftRow | null {
  const contentID = stringField(payload, 'content_id') ?? '';
  if (!contentID) {
    return null;
  }
  return {
    content_type: stringField(payload, 'content_type') ?? undefined,
    content_id: contentID,
    draft_version: stringField(payload, 'draft_version') ?? undefined,
    enabled: booleanField(payload, 'enabled') ?? false,
    display_json: objectField(payload, 'display_json') ?? {},
    data_json: objectField(payload, 'data_json') ?? {},
    updated_by: stringField(payload, 'updated_by') ?? undefined,
  };
}

function upsertDraftRow(list: AdminContentDraftList, row: AdminContentDraftRow): AdminContentDraftList {
  const rows = list.rows.some((candidate) => candidate.content_id === row.content_id)
    ? list.rows.map((candidate) => (candidate.content_id === row.content_id ? row : candidate))
    : [row, ...list.rows];
  return {
    ...list,
    rows,
    total: Math.max(list.total, rows.length),
  };
}

function parseValidation(payload: JsonObject, fallback: AdminContentValidation | null): AdminContentValidation {
  return {
    valid: booleanField(payload, 'valid') ?? fallback?.valid ?? false,
    version: stringField(payload, 'version') ?? fallback?.version ?? '',
    checked_at: Math.max(0, Math.round(numberField(payload, 'checked_at') ?? fallback?.checked_at ?? 0)),
    issues: Array.isArray(payload.issues)
      ? payload.issues.filter(isJsonObject).map(parseValidationIssue)
      : fallback?.issues ?? [],
  };
}

function parseValidationIssue(payload: JsonObject): AdminContentValidation['issues'][number] {
  return {
    path: stringField(payload, 'path') ?? '',
    code: stringField(payload, 'code') ?? '',
    message: stringField(payload, 'message') ?? '',
  };
}

function parsePublish(payload: JsonObject, fallback: AdminContentPublishSummary | null): AdminContentPublishSummary {
  const version = objectField(payload, 'version');
  const validation = objectField(payload, 'validation');
  return {
    published: booleanField(payload, 'published') ?? fallback?.published ?? false,
    idempotent: booleanField(payload, 'idempotent') ?? fallback?.idempotent ?? false,
    row_count: Math.max(0, Math.round(numberField(payload, 'row_count') ?? fallback?.row_count ?? 0)),
    version: version ? parseVersion(version) : fallback?.version ?? null,
    validation: validation ? parseValidation(validation, fallback?.validation ?? null) : fallback?.validation ?? null,
  };
}

function parseRollback(payload: JsonObject, fallback: AdminContentRollbackSummary | null): AdminContentRollbackSummary {
  const version = objectField(payload, 'version');
  const validation = objectField(payload, 'validation');
  return {
    rolled_back: booleanField(payload, 'rolled_back') ?? fallback?.rolled_back ?? false,
    idempotent: booleanField(payload, 'idempotent') ?? fallback?.idempotent ?? false,
    target_version_id: stringField(payload, 'target_version_id') ?? fallback?.target_version_id ?? '',
    version: version ? parseVersion(version) : fallback?.version ?? null,
    validation: validation ? parseValidation(validation, fallback?.validation ?? null) : fallback?.validation ?? null,
  };
}

function mergeVersion(
  versions: AdminContentVersionsSummary | null,
  version: AdminContentVersionSummary | null,
): AdminContentVersionsSummary | null {
  if (!versions || !version) {
    return versions;
  }
  const rows = versions.versions.some((candidate) => candidate.id === version.id)
    ? versions.versions.map((candidate) => (candidate.id === version.id ? version : candidate))
    : [version, ...versions.versions.map((candidate) => ({ ...candidate, current: false }))];
  return {
    ...versions,
    versions: rows,
    total: Math.max(versions.total, rows.length),
  };
}

function parseAuditLog(payload: JsonObject, fallback: AdminContentAuditLog | null): AdminContentAuditLog {
  const entries = Array.isArray(payload.entries)
    ? payload.entries.filter(isJsonObject).map(parseAuditEntry).filter((entry): entry is AdminContentAuditEntry => entry !== null)
    : fallback?.entries ?? [];
  return {
    entries,
    total: Math.max(0, Math.round(numberField(payload, 'total') ?? fallback?.total ?? entries.length)),
    limit: Math.max(0, Math.round(numberField(payload, 'limit') ?? fallback?.limit ?? entries.length)),
    offset: Math.max(0, Math.round(numberField(payload, 'offset') ?? fallback?.offset ?? 0)),
    generated_at: Math.max(0, Math.round(numberField(payload, 'generated_at') ?? fallback?.generated_at ?? 0)),
  };
}

function parseAuditEntry(payload: JsonObject): AdminContentAuditEntry | null {
  const id = stringField(payload, 'id') ?? '';
  if (!id) {
    return null;
  }
  return {
    id,
    content_version_id: stringField(payload, 'content_version_id') ?? undefined,
    content_type: stringField(payload, 'content_type') ?? '',
    content_id: stringField(payload, 'content_id') ?? '',
    field_path: stringField(payload, 'field_path') ?? '',
    old_value_json: objectField(payload, 'old_value_json') ?? undefined,
    new_value_json: objectField(payload, 'new_value_json') ?? undefined,
    actor_ref: stringField(payload, 'actor_ref') ?? undefined,
    note: stringField(payload, 'note') ?? undefined,
    balance_tag: stringField(payload, 'balance_tag') ?? undefined,
    created_at: Math.max(0, Math.round(roundedOptional(payload, 'created_at') ?? 0)),
  };
}
