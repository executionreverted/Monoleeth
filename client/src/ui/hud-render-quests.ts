import type { ClientState } from '../state/types';
import { hudSelection } from './hud-selection';
import type { QuestBoardSummary, QuestEntry, QuestOfferSummary, QuestSummary } from './hud-types';
import { clamp, escapeHTML, questObjectiveLabel, questRewardKindLabel, questRewardLabel } from './hud-formatters';

export function questsPanel(state: ClientState): string {
  return `
    <h2>Quests</h2>
    ${questBoardPanel(state)}
  `;
}

export function questBoardPanel(state: ClientState): string {
  const board = state.questBoard;
  if (!board) {
    return `
      <div class="systems-subhead">Quest Board</div>
      <div class="empty-line">Awaiting quest board.</div>
    `;
  }

  const sections = questBoardSections(board);
  const entries = [...sections.claimable, ...sections.active, ...sections.offers, ...sections.completed];
  const selected = selectedQuestEntry(entries);
  const canReroll = board.can_reroll;
  const rerollTitle = canReroll ? 'Reroll the quest board' : board.locked_reason || 'Reroll unavailable';

  return `
    <section class="quest-board" data-quest-board="true">
      <div class="quest-tabs" role="list" aria-label="Quest categories">
        ${questTab('Offers', board.counts.offers)}
        ${questTab('Active', board.counts.active)}
        ${questTab('Claimable', board.counts.claimable)}
        ${questTab('Completed', board.counts.completed + board.counts.claimed)}
      </div>
      <div class="quest-board__list">
        ${questSection('Claimable', sections.claimable, selected?.key)}
        ${questSection('Active', sections.active, selected?.key)}
        ${questSection('Offers', sections.offers, selected?.key)}
        ${questSection('Completed', sections.completed, selected?.key)}
      </div>
      <div class="quest-board__detail" data-quest-detail="${escapeHTML(selected?.key ?? '')}">
        ${selected ? questDetail(selected) : '<div class="empty-line">No quest entries.</div>'}
        <div class="quest-reroll">
          <div>
            <span>Reroll Cost</span>
            <strong>${board.reroll_cost.amount} ${escapeHTML(board.reroll_cost.currency_type.replace(/_/g, ' '))}</strong>
          </div>
          <button type="button" data-action="quest-reroll" ${canReroll ? '' : 'disabled'} title="${escapeHTML(rerollTitle)}">Reroll</button>
        </div>
      </div>
    </section>
  `;
}

export function questBoardSections(board: QuestBoardSummary): {
  offers: QuestEntry[];
  active: QuestEntry[];
  claimable: QuestEntry[];
  completed: QuestEntry[];
} {
  const offers = board.offers.map((offer) => questEntryForOffer(offer));
  const claimable = board.active.filter((quest) => quest.can_claim).map((quest) => questEntryForQuest(quest));
  const active = board.active
    .filter((quest) => !quest.can_claim && (quest.state === 'accepted' || (!quest.completed_at && !quest.claimed_at)))
    .map((quest) => questEntryForQuest(quest));
  const completed = board.active
    .filter((quest) => !quest.can_claim && (quest.state === 'completed' || quest.state === 'claimed' || quest.completed_at || quest.claimed_at))
    .map((quest) => questEntryForQuest(quest));
  return { offers, active, claimable, completed };
}

export function questEntryForOffer(offer: QuestOfferSummary): QuestEntry {
  return { key: `offer:${offer.offer_id}`, kind: 'offer', item: offer };
}

export function questEntryForQuest(quest: QuestSummary): QuestEntry {
  return { key: `quest:${quest.quest_id}`, kind: 'quest', item: quest };
}

export function selectedQuestEntry(entries: QuestEntry[]): QuestEntry | null {
  if (entries.length === 0) {
    hudSelection.selectedQuestKey = null;
    return null;
  }
  const selected = entries.find((entry) => entry.key === hudSelection.selectedQuestKey) ?? entries[0];
  hudSelection.selectedQuestKey = selected.key;
  return selected;
}

export function questTab(label: string, count: number): string {
  return `
    <span class="quest-tab" role="listitem">
      <em>${escapeHTML(label)}</em>
      <strong>${count}</strong>
    </span>
  `;
}

export function questSection(label: string, entries: QuestEntry[], selectedKey: string | undefined): string {
  return `
    <section class="quest-section" data-quest-section="${escapeHTML(label.toLowerCase())}">
      <header><strong>${escapeHTML(label)}</strong><span>${entries.length}</span></header>
      ${
        entries.length > 0
          ? entries.map((entry) => questRow(entry, selectedKey)).join('')
          : `<div class="quest-section__empty">No ${escapeHTML(label.toLowerCase())} quests.</div>`
      }
    </section>
  `;
}

export function questRow(entry: QuestEntry, selectedKey: string | undefined): string {
  const item = entry.item;
  const objective = item.objectives[0];
  const selected = entry.key === selectedKey;
  const state = entry.kind === 'offer' ? 'offer' : entry.item.can_claim ? 'claim' : entry.item.state || 'active';
  return `
    <button class="quest-row" type="button" data-action="quest-select" data-quest-key="${escapeHTML(entry.key)}" data-selected="${selected ? 'true' : 'false'}" data-quest-state="${escapeHTML(state)}">
      <span class="quest-row__pip"></span>
      <span>
        <strong>${escapeHTML(item.title)}</strong>
        <em>${escapeHTML(objective ? questObjectiveLabel(objective) : questRewardLabel(item.rewards[0]))}</em>
      </span>
      <small>${escapeHTML(entry.kind === 'offer' ? item.quest_type : questStatusLabel(entry.item))}</small>
    </button>
  `;
}

export function questDetail(entry: QuestEntry): string {
  const item = entry.item;
  const objectives = item.objectives.length > 0 ? item.objectives : [];
  const rewards = item.rewards.length > 0 ? item.rewards : [];
  return `
    <article class="quest-detail-card">
      <div class="quest-detail-card__head">
        <span>${escapeHTML(entry.kind === 'offer' ? item.quest_type : questStatusLabel(entry.item))}</span>
        <strong>${escapeHTML(item.title)}</strong>
        <p>${escapeHTML(item.description || 'No description.')}</p>
      </div>
      <div class="quest-detail-card__grid">
        <section>
          <h3>Objectives</h3>
          ${
            objectives.length > 0
              ? objectives.map((objective) => questObjectiveRow(objective)).join('')
              : '<div class="empty-line">No objectives.</div>'
          }
        </section>
        <section>
          <h3>Rewards</h3>
          ${
            rewards.length > 0
              ? rewards.map((reward) => `<div class="quest-reward-row"><span>${escapeHTML(questRewardKindLabel(reward))}</span><strong>${escapeHTML(questRewardLabel(reward))}</strong></div>`).join('')
              : '<div class="empty-line">No rewards.</div>'
          }
        </section>
      </div>
      <div class="quest-actions">
        ${questActionButton(entry)}
      </div>
    </article>
  `;
}

export function questObjectiveRow(objective: QuestOfferSummary['objectives'][number]): string {
  const progress = objective.required > 0 ? clamp(objective.current / objective.required, 0, 1) : 0;
  return `
    <div class="quest-objective-row" data-complete="${objective.completed ? 'true' : 'false'}">
      <div>
        <span>${escapeHTML(objective.kind.replace(/_/g, ' '))}</span>
        <strong>${escapeHTML(questObjectiveLabel(objective))}</strong>
      </div>
      <div class="quest-progress" aria-hidden="true"><span style="width:${Math.round(progress * 100)}%"></span></div>
    </div>
  `;
}

export function questActionButton(entry: QuestEntry): string {
  if (entry.kind === 'offer') {
    if (!entry.item.can_accept) {
      return `<span class="quest-action-status">${escapeHTML(entry.item.locked_reason || 'Quest unavailable')}</span>`;
    }
    return `<button type="button" data-action="quest-accept" data-offer-id="${escapeHTML(entry.item.offer_id)}">Accept</button>`;
  }
  if (entry.item.can_claim) {
    return `<button type="button" data-action="quest-claim" data-quest-id="${escapeHTML(entry.item.quest_id)}">Claim</button>`;
  }
  return `<span class="quest-action-status">${escapeHTML(questStatusLabel(entry.item))}</span>`;
}

export function questStatusLabel(quest: QuestSummary): string {
  if (quest.can_claim) {
    return 'claimable';
  }
  if (quest.claimed_at || quest.state === 'claimed') {
    return 'claimed';
  }
  if (quest.completed_at || quest.state === 'completed') {
    return 'completed';
  }
  return quest.state || 'active';
}
