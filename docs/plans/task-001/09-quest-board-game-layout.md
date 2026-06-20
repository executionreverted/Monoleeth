# Phase 09 - Quest Board Game Layout

## Goal

Make Quests a real selectable game board instead of a single generic card or a
renamed Galaxy panel. The surface should expose server-owned offers, active
quests, objective progress, rewards, accept/claim/reroll actions, and clean
locked/empty states without fake counts or client-authored progress.

## Problems Covered

- Task 001 goal promises Quests as a game system, but no previous phase owned
  it.
- The old Galaxy-style menu does not behave like a quest board.
- Quest selection, objectives, rewards, and available actions need a real
  list/detail/action layout.
- Progression unlocks and quest reward claims must not become client-authored.

## Required Reading

```text
docs/plans/task-001-goal.md
docs/plans/task-001/00-index.md
docs/todo.md
docs/plans/modules/01-player-progression-rank-role-skills.md
docs/plans/modules/10-quest-board-generation.md
docs/plans/modules/15-api-events-errors.md
docs/plans/modules/16-testing-observability-balancing.md
client/src/ui/hud.ts
client/src/state/reducer.ts
client/src/protocol/commands.ts
internal/game/server/quest_admin_observability_handlers.go
internal/game/quest/
```

## Design Contract

- Window label is `Quests`.
- Layout is list/detail/action:
  - left: selectable quest categories and rows
  - center: selected quest objectives and progress
  - right/footer: rewards and allowed actions
- Categories: available offers, active quests, claimable quests, completed
  quests, reroll/board state where supported.
- Objective progress and rewards come from server snapshots/events only.
- Accept, claim, and reroll are real contracts or absent/locked with player
  game copy.
- Progression unlocks and skill/rank effects remain server-owned. The UI can
  show requirements and rewards but cannot submit completion truth.

## API And Action-State Contract

Quest board payloads must include enough server-owned action state for the UI
to hide or enable actions without guessing:

- offer/quest id, display name, category/status, requirements, objectives,
  rewards, and safe art/catalog refs
- `can_accept`, `can_claim`, `can_reroll`, cooldown/available-at, and
  player-facing locked reason where relevant
- board version or revision for stale-event handling
- explicit semantics for `quest.progress`: either rename it to a board snapshot
  query or document it as payloadless snapshot refresh only
- event reconciliation fields such as `accepted_offer_id`, updated
  `quest_board`, changed active quest, and reward snapshots

The UI must not infer quest completion, reroll affordability, active quest
limits, objective progress, hidden target ids, or reward grant truth.

## Subagent Review Additions - 2026-06-20

- Add passive event reconciliation for quest board state. A `quest.accepted`
  event that carries only the quest must also remove or mark the stale offer,
  update board counts, or trigger an explicit board refresh.
- Decide the action-state rule for non-claimable quests. Disabled primary
  `Claim Locked` buttons should become quiet status copy unless the action is
  immediately meaningful.
- Browser smoke must exercise accept, claim, and reroll UI paths where real
  contracts exist. Server tests already cover duplicate claims; client smoke
  must prove the buttons are wired and do not fake progress.
- Reward grant reconciliation must refresh or merge wallet, XP/rank, inventory,
  cargo, and quest board state from server-owned payloads/events.

## Second Subagent Review Additions - 2026-06-20

- Add server-owned action-state payloads. Accept/reroll/claim availability must
  come from the server, not wallet-only or local selected-row heuristics.
- `quest.accepted` must remove or mark the stale board offer. Either include
  `accepted_offer_id` plus board revision or return a refreshed `quest_board`.
- Objective and reward rows need safe display labels, art keys, and catalog refs
  from the server. UI must not transform raw target ids or item ids into player
  copy, especially where future hidden targets may exist.
- Browser smoke must click real claim and reroll happy paths where contracts
  exist, not only accept. If fixtures do not provide claimable/rerollable state,
  record that as a test fixture blocker.
- Clarify `quest.progress`: current usage is closer to a snapshot refresh than
  a progress mutation. The protocol table and UI copy should make that explicit.

## Third Subagent Review Additions - 2026-06-20

- Quest UI still shows disabled primary `Claim Locked` for non-claimable
  quests. Replace it with quiet status copy unless a real claim is available.
- Extend quest board payloads with server-owned action state:
  `can_accept`, `can_claim`, `can_reroll`, cooldown/available-at, locked
  reason, and board revision. UI must not infer reroll/claim availability from
  wallet-only or selected-row heuristics.
- Change `quest.accepted` passive event to include `accepted_offer_id` plus
  board revision, or return a refreshed `quest_board`, so reducer paths remove
  stale offers even when the response path is absent.
- Objective targets and reward items need server display labels/art/catalog refs
  instead of UI prettifying raw target ids or item ids. This is also a hidden
  target leak guard for future quest types.
- Browser smoke must click real accept, claim, and reroll happy paths when
  fixtures expose them, or list the exact missing fixture blocker.

## Fourth Subagent Review Additions - 2026-06-20

- Add an offer expiry and board reset contract. Quest board payloads should
  include `expires_at`, board revision, and `reset_at` or equivalent refresh
  hints where offers can expire.
- Expired offers must either disappear, render as quiet unavailable state, or
  trigger a board refresh. The UI must not keep an enabled Accept button based
  on stale selected-row state.
- Reducer tests should cover stale board revisions and expired offers received
  through response and event-only paths.
- Browser smoke must include an expired offer or stale board refresh path, in
  addition to accept/claim/reroll happy paths where fixtures expose them.

## Implementation Evidence - 2026-06-20

- Quest board payloads now include server-owned action state: offer
  `can_accept`, quest `can_claim`, board `can_reroll`, player-facing locked
  reason, offer `expires_at`, and board `reset_at`.
- `quest.accepted` events include `accepted_offer_id`. The client reducer uses
  that id to remove stale offers even when the response path is absent, and
  reducer coverage asserts the event-only cleanup.
- The Quest HUD uses server action state instead of local wallet/selection
  heuristics for accept/reroll visibility. Non-claimable quests render quiet
  status copy instead of a disabled primary `Claim Locked` button.
- Browser smoke now fails if the Quest board lacks server action-state fields,
  if disabled quest primary buttons appear, or if `Claim Locked` leaks into
  player UI. Quest screenshots are also captured under
  `output/screenshots/task-001/09/`.
- Server coverage verifies quest board action state, accept offer removal,
  client-authored progress rejection, reward claim idempotency, and reroll
  wallet debit/board refresh.
- Quest objective/reward payloads now include server-owned `display_name`,
  `catalog_ref`, and `art_key` metadata. The HUD renders those display names
  instead of prettifying raw target/item ids, and browser smoke fails if quest
  UI text leaks exact raw objective targets or reward item ids.
- Quest board payloads now include an explicit `revision`. The reducer rejects
  older board revisions, advances revision from event-only quest updates, and
  fails closed when an offer is expired relative to server time.
- Browser smoke clicks a real `quest.reroll` happy path after a real accept and
  asserts the board refresh command/state. Browser claim remains documented as
  fixture-blocked: the current authenticated smoke kills `training_drone` and
  loots `raw_ore`, while MVP claimable board quests require deterministic
  `pirate`/`raider` kills or larger `iron_ore`/craft/deliver fixtures. Server
  tests still cover real claim and duplicate-claim idempotency.

## Implementation Plan

1. Build the board model.
   - Define or verify quest snapshot/query payloads for board offers, active
     quests, claimable rewards, completed state, reroll cooldown/cost, and
     selected detail.
   - Keep client state limited to selected row/category.

2. Replace generic panel structure.
   - Remove any Galaxy wording from the normal player HUD.
   - Render quest rows with stable ids, status, objective summary, and reward
     preview.
   - Render selected detail with objective progress bars/rows and rewards.

3. Wire real actions.
   - Accept uses server-owned availability and idempotency.
   - Claim uses server reward state machine and duplicate-claim protection.
   - Reroll uses server cooldown/cost/rate-limit or remains hidden.
   - `progression.unlock_skill` remains blocked unless implemented with a real
     server contract in this phase.

4. Reconcile events.
   - Quest accept/progress/claim/reroll responses update board state.
   - Passive quest events either carry safe payloads or trigger refresh queries.
   - Reward grants reconcile wallet, XP, inventory/cargo, rank, and quest state.

5. Add tests.
   - Selection and tab/category switching.
   - Accept/claim/reroll happy paths and locked paths.
   - Duplicate reward claim rejection/idempotency.
   - No client-authored objective progress.

## Likely Files

```text
internal/game/server/quest_admin_observability_handlers.go
internal/game/server/server_test.go
internal/game/realtime/envelope.go
internal/game/realtime/envelope_test.go
internal/game/quest/
client/src/protocol/envelope.ts
client/src/protocol/commands.ts
client/src/state/types.ts
client/src/state/reducer.ts
client/src/ui/hud.ts
client/src/styles.css
docs/plans/task-001/09-quest-board-game-layout.md
```

## Acceptance Criteria

- [x] Quests has an owning Task 001 phase and visible board layout.
- [x] The menu label and window title are `Quests`, not `Galaxy`.
- [x] Quest rows are selectable and show server-owned state.
- [x] Selected quest detail shows objectives, progress, and rewards.
- [x] Accept, claim, and reroll actions are real or absent/locked with game copy.
- [x] Reward claim is idempotent and cannot duplicate XP/currency/items.
- [x] Client cannot author quest completion/progress.
- [x] Passive quest events reconcile or trigger explicit refresh.
- [x] Quest board payload includes server-owned action state for accept, claim,
      and reroll availability.
- [x] `quest.accepted` event-only reducer path removes the accepted offer or
      refreshes the board.
- [x] Objective/reward UI renders server display metadata and does not print raw
      target/item ids.
- [x] Non-claimable quests do not render disabled primary `Claim Locked`
      controls in normal player UI.
- [x] Quest board payload includes expiry/reset state or a named blocker for
      offers that can expire.
- [x] Expired offers cannot remain enabled through stale local selection state.
- [x] Reducer and smoke cover stale board revision or expired offer refresh.
- [x] Browser smoke exercises real claim and reroll happy paths or names the
      missing fixture blocker.
- [x] Browser smoke captures quest board screenshots under
      `output/screenshots/task-001/09/` or the final Task 001 screenshot set.

## Verification

```bash
go test ./internal/game/server -run 'Test.*(Quest|Progression|Reward|Reroll)' -count=1
go test ./internal/game/realtime -run 'TestOperationRegistry' -count=1
cd client
npm --cache /tmp/gameproject-npm-cache run test -- --run src/protocol src/state
```
