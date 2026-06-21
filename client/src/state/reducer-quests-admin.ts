import { CLIENT_EVENTS, JsonObject } from '../protocol/envelope';
import type {
  AbuseCoverageSummary,
  AdminInspectionSummary,
  AdminRepairCraftJobSummary,
  CommandLogSummary,
  MetricsSummary,
  QuestBoardSummary,
  QuestObjectiveSummary,
  QuestOfferSummary,
  QuestRewardSummary,
  QuestSummary,
  ReleaseGateSummary,
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

export function parseQuestBoardSummary(payload: JsonObject, fallback: QuestBoardSummary | null, serverTime?: number): QuestBoardSummary {
  const incomingGeneratedAt = roundedOptional(payload, 'generated_at');
  const incomingRevision = roundedOptional(payload, 'revision') ?? incomingGeneratedAt;
  const fallbackRevision = fallback?.revision ?? fallback?.generated_at ?? 0;
  if (fallback && incomingRevision !== undefined && incomingRevision < fallbackRevision) {
    return fallback;
  }
  const generatedAt = Math.max(0, Math.round(incomingGeneratedAt ?? fallback?.generated_at ?? 0));
  const revision = Math.max(0, Math.round(incomingRevision ?? fallback?.revision ?? generatedAt));
  const expirationReferenceTime = Math.max(0, Math.round(serverTime ?? generatedAt));
  const offers = Array.isArray(payload.offers)
    ? payload.offers
        .filter(isJsonObject)
        .map((offer) => parseQuestOffer(offer, expirationReferenceTime))
        .filter((offer): offer is QuestOfferSummary => offer !== null)
    : fallback?.offers ?? [];
  const active = Array.isArray(payload.active)
    ? payload.active
        .filter(isJsonObject)
        .map((quest) => parseQuestSummary(quest, null))
        .filter((quest): quest is QuestSummary => quest !== null)
    : fallback?.active ?? [];
  const counts = objectField(payload, 'counts') ?? {};
  const rerollCost = objectField(payload, 'reroll_cost') ?? {};
  return {
    offers,
    active,
    counts: {
      offers: Math.max(0, Math.round(numberField(counts, 'offers') ?? fallback?.counts.offers ?? offers.length)),
      active: Math.max(0, Math.round(numberField(counts, 'active') ?? fallback?.counts.active ?? countQuests(active, 'accepted'))),
      completed: Math.max(0, Math.round(numberField(counts, 'completed') ?? fallback?.counts.completed ?? countQuests(active, 'completed'))),
      claimable: Math.max(0, Math.round(numberField(counts, 'claimable') ?? fallback?.counts.claimable ?? active.filter((quest) => quest.can_claim).length)),
      claimed: Math.max(0, Math.round(numberField(counts, 'claimed') ?? fallback?.counts.claimed ?? countQuests(active, 'claimed'))),
    },
    reroll_cost: {
      currency_type: stringField(rerollCost, 'currency_type') ?? fallback?.reroll_cost.currency_type ?? 'credits',
      amount: Math.max(0, Math.round(numberField(rerollCost, 'amount') ?? fallback?.reroll_cost.amount ?? 0)),
    },
    can_reroll: booleanField(payload, 'can_reroll') ?? fallback?.can_reroll ?? false,
    locked_reason: stringField(payload, 'locked_reason') ?? fallback?.locked_reason ?? undefined,
    reset_at: optionalRoundedNumber(payload, 'reset_at', fallback?.reset_at),
    generated_at: generatedAt,
    revision,
  };
}

function parseQuestOffer(payload: JsonObject, expirationReferenceTime: number): QuestOfferSummary | null {
  const offerID = stringField(payload, 'offer_id') ?? '';
  if (!offerID) {
    return null;
  }
  const expiresAt = Math.max(0, Math.round(numberField(payload, 'expires_at') ?? 0));
  const expired = expiresAt > 0 && expirationReferenceTime > 0 && expiresAt <= expirationReferenceTime;
  return {
    offer_id: offerID,
    quest_type: stringField(payload, 'quest_type') ?? '',
    title: stringField(payload, 'title') ?? offerID,
    description: stringField(payload, 'description') ?? '',
    objectives: parseQuestObjectives(payload),
    rewards: parseQuestRewards(payload),
    expires_at: expiresAt,
    can_accept: !expired && (booleanField(payload, 'can_accept') ?? true),
    locked_reason: expired ? stringField(payload, 'locked_reason') ?? 'Offer expired.' : stringField(payload, 'locked_reason') ?? undefined,
  };
}

export function parseQuestSummary(payload: JsonObject, fallback: QuestSummary | null): QuestSummary | null {
  const questID = stringField(payload, 'quest_id') ?? fallback?.quest_id ?? '';
  if (!questID) {
    return null;
  }
  return {
    quest_id: questID,
    accepted_offer_id: stringField(payload, 'accepted_offer_id') ?? fallback?.accepted_offer_id ?? undefined,
    quest_type: stringField(payload, 'quest_type') ?? fallback?.quest_type ?? '',
    title: stringField(payload, 'title') ?? fallback?.title ?? questID,
    description: stringField(payload, 'description') ?? fallback?.description ?? '',
    state: stringField(payload, 'state') ?? fallback?.state ?? '',
    objectives: Array.isArray(payload.objectives) ? parseQuestObjectives(payload) : fallback?.objectives ?? [],
    rewards: Array.isArray(payload.rewards) ? parseQuestRewards(payload) : fallback?.rewards ?? [],
    accepted_at: Math.max(0, Math.round(numberField(payload, 'accepted_at') ?? fallback?.accepted_at ?? 0)),
    completed_at: optionalRoundedNumber(payload, 'completed_at', fallback?.completed_at),
    claimed_at: optionalRoundedNumber(payload, 'claimed_at', fallback?.claimed_at),
    can_claim: booleanField(payload, 'can_claim') ?? fallback?.can_claim ?? false,
  };
}

function parseQuestObjectives(payload: JsonObject): QuestObjectiveSummary[] {
  return Array.isArray(payload.objectives)
    ? payload.objectives
        .filter(isJsonObject)
        .map(parseQuestObjective)
        .filter((objective): objective is QuestObjectiveSummary => objective !== null)
    : [];
}

function parseQuestObjective(payload: JsonObject): QuestObjectiveSummary | null {
  const id = stringField(payload, 'id') ?? '';
  if (!id) {
    return null;
  }
  return {
    id,
    kind: stringField(payload, 'kind') ?? '',
    target: stringField(payload, 'target') ?? undefined,
    display_name: stringField(payload, 'display_name') ?? 'Objective',
    catalog_ref: stringField(payload, 'catalog_ref') ?? undefined,
    art_key: stringField(payload, 'art_key') ?? undefined,
    current: Math.max(0, Math.round(numberField(payload, 'current') ?? 0)),
    required: Math.max(0, Math.round(numberField(payload, 'required') ?? 0)),
    completed: booleanField(payload, 'completed') ?? false,
  };
}

function parseQuestRewards(payload: JsonObject): QuestRewardSummary[] {
  return Array.isArray(payload.rewards)
    ? payload.rewards
        .filter(isJsonObject)
        .map(parseQuestReward)
        .filter((reward): reward is QuestRewardSummary => reward !== null)
    : [];
}

function parseQuestReward(payload: JsonObject): QuestRewardSummary | null {
  const kind = stringField(payload, 'kind') ?? '';
  const amount = numberField(payload, 'amount') ?? 0;
  if (!kind || amount <= 0) {
    return null;
  }
  return {
    kind,
    currency_type: stringField(payload, 'currency_type') ?? undefined,
    item_id: stringField(payload, 'item_id') ?? undefined,
    role: stringField(payload, 'role') ?? undefined,
    display_name: stringField(payload, 'display_name') ?? 'Reward',
    catalog_ref: stringField(payload, 'catalog_ref') ?? undefined,
    art_key: stringField(payload, 'art_key') ?? undefined,
    amount: Math.max(0, Math.round(amount)),
  };
}

export function parseAdminInspection(payload: JsonObject, fallback: AdminInspectionSummary | null): AdminInspectionSummary {
  const inventory = objectField(payload, 'inventory') ?? {};
  const wallet = objectField(payload, 'wallet') ?? {};
  return {
    target: stringField(payload, 'target') ?? fallback?.target ?? '',
    inventory: {
      stackable_items: Math.max(0, Math.round(numberField(inventory, 'stackable_items') ?? fallback?.inventory.stackable_items ?? 0)),
      instance_items: Math.max(0, Math.round(numberField(inventory, 'instance_items') ?? fallback?.inventory.instance_items ?? 0)),
      item_ledger: Array.isArray(inventory.item_ledger)
        ? inventory.item_ledger
            .filter(isJsonObject)
            .map((entry) => ({
              ledger_id: stringField(entry, 'ledger_id') ?? '',
              item_id: stringField(entry, 'item_id') ?? '',
              quantity: Math.round(numberField(entry, 'quantity') ?? 0),
              action: stringField(entry, 'action') ?? '',
              balance_after: Math.round(numberField(entry, 'balance_after') ?? 0),
              location: stringField(entry, 'location') ?? '',
              reason: stringField(entry, 'reason') ?? '',
              created_at: Math.max(0, Math.round(numberField(entry, 'created_at') ?? 0)),
            }))
            .filter((entry) => entry.ledger_id !== '')
        : fallback?.inventory.item_ledger ?? [],
    },
    wallet: {
      balances: Array.isArray(wallet.balances)
        ? wallet.balances
            .filter(isJsonObject)
            .map((balance) => ({
              currency_type: stringField(balance, 'currency_type') ?? '',
              balance: Math.round(numberField(balance, 'balance') ?? 0),
            }))
            .filter((balance) => balance.currency_type !== '')
        : fallback?.wallet.balances ?? [],
      ledger: Array.isArray(wallet.ledger)
        ? wallet.ledger
            .filter(isJsonObject)
            .map((entry) => ({
              ledger_id: stringField(entry, 'ledger_id') ?? '',
              currency_type: stringField(entry, 'currency_type') ?? '',
              amount: Math.round(numberField(entry, 'amount') ?? 0),
              action: stringField(entry, 'action') ?? '',
              balance_after: Math.round(numberField(entry, 'balance_after') ?? 0),
              reason: stringField(entry, 'reason') ?? '',
              created_at: Math.max(0, Math.round(numberField(entry, 'created_at') ?? 0)),
            }))
            .filter((entry) => entry.ledger_id !== '')
        : fallback?.wallet.ledger ?? [],
    },
    generated_at: Math.max(0, Math.round(numberField(payload, 'generated_at') ?? fallback?.generated_at ?? 0)),
  };
}

export function parseAdminRepairCraftJob(payload: JsonObject, fallback: AdminRepairCraftJobSummary | null): AdminRepairCraftJobSummary {
  return {
    accepted: booleanField(payload, 'accepted') ?? fallback?.accepted ?? false,
    job_id: stringField(payload, 'job_id') ?? fallback?.job_id,
    status: stringField(payload, 'status') ?? fallback?.status ?? '',
    already_complete: booleanField(payload, 'already_complete') ?? fallback?.already_complete,
    message: stringField(payload, 'message') ?? fallback?.message,
  };
}

export function parseCommandLogSummary(payload: JsonObject, fallback: CommandLogSummary | null): CommandLogSummary {
  return {
    entries: Array.isArray(payload.entries)
      ? payload.entries
          .filter(isJsonObject)
          .map((entry) => ({
            request_id: stringField(entry, 'request_id') ?? '',
            operation: stringField(entry, 'operation') ?? '',
            status: stringField(entry, 'status') ?? '',
            error_code: stringField(entry, 'error_code') ?? undefined,
            duration_ms: Math.max(0, Math.round(numberField(entry, 'duration_ms') ?? 0)),
            timestamp: Math.max(0, Math.round(numberField(entry, 'timestamp') ?? 0)),
          }))
          .filter((entry) => entry.request_id !== '')
      : fallback?.entries ?? [],
    total: Math.max(0, Math.round(numberField(payload, 'total') ?? fallback?.total ?? 0)),
    generated_at: Math.max(0, Math.round(numberField(payload, 'generated_at') ?? fallback?.generated_at ?? 0)),
  };
}

export function parseMetricsSummary(payload: JsonObject, fallback: MetricsSummary | null): MetricsSummary {
  const snapshot = objectField(payload, 'snapshot') ?? {};
  return {
    snapshot: {
      counters: Array.isArray(snapshot.counters)
        ? snapshot.counters.filter(isJsonObject).map(parseMetricCounter)
        : fallback?.snapshot.counters ?? [],
      gauges: Array.isArray(snapshot.gauges)
        ? snapshot.gauges.filter(isJsonObject).map(parseMetricCounter)
        : fallback?.snapshot.gauges ?? [],
      durations: Array.isArray(snapshot.durations)
        ? snapshot.durations.filter(isJsonObject).map(parseMetricDuration)
        : fallback?.snapshot.durations ?? [],
    },
    generated_at: Math.max(0, Math.round(numberField(payload, 'generated_at') ?? fallback?.generated_at ?? 0)),
  };
}

function parseMetricCounter(payload: JsonObject): { name: string; value: number; labels: Array<{ name: string; value: string }> } {
  return {
    name: stringField(payload, 'name') ?? '',
    value: Math.round(numberField(payload, 'value') ?? 0),
    labels: parseMetricLabels(payload),
  };
}

function parseMetricDuration(payload: JsonObject): MetricsSummary['snapshot']['durations'][number] {
  return {
    name: stringField(payload, 'name') ?? '',
    labels: parseMetricLabels(payload),
    count: Math.max(0, Math.round(numberField(payload, 'count') ?? 0)),
    total: Math.max(0, Math.round(numberField(payload, 'total') ?? 0)),
    minimum: Math.max(0, Math.round(numberField(payload, 'minimum') ?? 0)),
    maximum: Math.max(0, Math.round(numberField(payload, 'maximum') ?? 0)),
    p50: Math.max(0, Math.round(numberField(payload, 'p50') ?? 0)),
    p95: Math.max(0, Math.round(numberField(payload, 'p95') ?? 0)),
    p99: Math.max(0, Math.round(numberField(payload, 'p99') ?? 0)),
  };
}

function parseMetricLabels(payload: JsonObject): Array<{ name: string; value: string }> {
  return Array.isArray(payload.labels)
    ? payload.labels
        .filter(isJsonObject)
        .map((label) => ({
          name: stringField(label, 'name') ?? '',
          value: stringField(label, 'value') ?? '',
        }))
        .filter((label) => label.name !== '')
    : [];
}

export function parseReleaseGateSummary(payload: JsonObject, fallback: ReleaseGateSummary | null): ReleaseGateSummary {
  const report = objectField(payload, 'report') ?? {};
  return {
    report: {
      covered: booleanField(report, 'covered') ?? fallback?.report.covered ?? false,
      passed: booleanField(report, 'passed') ?? fallback?.report.passed ?? false,
      missing: Array.isArray(report.missing)
        ? report.missing
            .filter(isJsonObject)
            .map((item) => ({
              module: stringField(item, 'module') ?? '',
              check: stringField(item, 'check') ?? '',
            }))
            .filter((item) => item.module !== '' && item.check !== '')
        : fallback?.report.missing ?? [],
    },
    coverage: Array.isArray(payload.coverage)
      ? payload.coverage
          .filter(isJsonObject)
          .map((item) => ({
            module: stringField(item, 'module') ?? '',
            passed: booleanField(item, 'passed') ?? false,
            missing: Array.isArray(item.missing) ? item.missing.filter((check): check is string => typeof check === 'string') : [],
            evidence: Math.max(0, Math.round(numberField(item, 'evidence') ?? 0)),
          }))
          .filter((item) => item.module !== '')
      : fallback?.coverage ?? [],
    evidence: Math.max(0, Math.round(numberField(payload, 'evidence') ?? fallback?.evidence ?? 0)),
    generated_at: Math.max(0, Math.round(numberField(payload, 'generated_at') ?? fallback?.generated_at ?? 0)),
  };
}

export function parseAbuseCoverageSummary(payload: JsonObject, fallback: AbuseCoverageSummary | null): AbuseCoverageSummary {
  const report = objectField(payload, 'report') ?? {};
  return {
    report: {
      passed: booleanField(report, 'passed') ?? fallback?.report.passed ?? false,
      missing: Array.isArray(report.missing) ? report.missing.filter((item): item is string => typeof item === 'string') : fallback?.report.missing ?? [],
    },
    coverage: Array.isArray(payload.coverage)
      ? payload.coverage
          .filter(isJsonObject)
          .map((item) => ({
            case: stringField(item, 'case') ?? '',
            evidence: Array.isArray(item.evidence)
              ? item.evidence
                  .filter(isJsonObject)
                  .map((evidence) => ({
                    package: stringField(evidence, 'package') ?? '',
                    test_name: stringField(evidence, 'test_name') ?? '',
                    note: stringField(evidence, 'note') ?? '',
                  }))
                  .filter((evidence) => evidence.test_name !== '')
              : [],
          }))
          .filter((item) => item.case !== '')
      : fallback?.coverage ?? [],
    generated_at: Math.max(0, Math.round(numberField(payload, 'generated_at') ?? fallback?.generated_at ?? 0)),
  };
}

export function applyQuestUpdate(board: QuestBoardSummary | null, quest: QuestSummary): QuestBoardSummary | null {
  if (!board) {
    return null;
  }
  const offers = quest.accepted_offer_id ? board.offers.filter((offer) => offer.offer_id !== quest.accepted_offer_id) : board.offers;
  const active = board.active.some((item) => item.quest_id === quest.quest_id)
    ? board.active.map((item) => (item.quest_id === quest.quest_id ? quest : item))
    : [...board.active, quest];
  return {
    ...board,
    offers,
    active,
    revision: Math.max(board.revision, quest.accepted_at, quest.completed_at ?? 0, quest.claimed_at ?? 0),
    counts: {
      offers: offers.length,
      active: countQuests(active, 'accepted'),
      completed: countQuests(active, 'completed'),
      claimable: active.filter((item) => item.can_claim).length,
      claimed: countQuests(active, 'claimed'),
    },
  };
}

function countQuests(quests: QuestSummary[], state: string): number {
  return quests.filter((quest) => quest.state === state).length;
}

export function questEventLog(eventType: string): string {
  switch (eventType) {
    case CLIENT_EVENTS.questAccepted:
      return 'Quest accepted.';
    case CLIENT_EVENTS.questProgressed:
      return 'Quest progress updated.';
    case CLIENT_EVENTS.questCompleted:
      return 'Quest completed.';
    case CLIENT_EVENTS.questRewardClaimed:
      return 'Quest reward claimed.';
    case CLIENT_EVENTS.questAbandoned:
      return 'Quest abandoned.';
    default:
      return 'Quest update received.';
  }
}
