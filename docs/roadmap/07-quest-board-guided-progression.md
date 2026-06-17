# Phase 07: Quest Board And Guided Progression

## Status

- State: In progress
- Owner: Player guidance and controlled rewards
- Depends on: Phase 02, Phase 03, Phase 05, Phase 06
- Unlocks: rank milestones, X Core fragments, controlled reward faucets, player onboarding

## Goal

Build a quest board that gives players 10 offers, allows 3 active quests, progresses only from server events, and grants rewards exactly once.

## Source Specs

Read before implementation:

- `docs/plans/modules/10-quest-board-generation.md`
- `docs/plans/modules/01-player-progression-rank-role-skills.md`
- `docs/plans/modules/02-inventory-cargo-wallet-ledger.md`
- `docs/plans/modules/08-crafting-recipes-materials.md`
- `docs/2026-06-17-progression-economy-systems-design.md`

## Module Ownership

Owns:

- `QuestService`
- `QuestGenerationService`
- `QuestRewardService`

Does not own:

- combat kill validation
- scan validation
- craft completion
- inventory primitive
- wallet primitive
- XP primitive

## MVP Scope

Quest types:

- kill
- scan skeleton
- collect
- craft
- build skeleton
- deliver skeleton

Board rules:

- 10 available offers
- 3 active quests
- reroll for credits
- rewards generated upfront
- offers expire daily
- accepted quests do not expire in MVP or expire weekly

## TODO: Quest Catalog And Board

- [x] Define quest template model. Verified 2026-06-17 by `go test ./internal/game/quests -count=1`.
- [x] Define generated offer model. Verified 2026-06-17 by `go test ./internal/game/quests -count=1`.
- [x] Define player quest model. Verified 2026-06-17 by `go test ./internal/game/quests -count=1`.
- [x] Define quest state machine. Verified 2026-06-17 by `go test ./internal/game/quests -count=1`.
- [x] Define objective schemas for kill, collect, craft, scan, build, and deliver. Verified 2026-06-17 by `go test ./internal/game/quests -count=1`.
- [x] Define reward payload model. Verified 2026-06-17 by `go test ./internal/game/quests -count=1`.
- [x] Generate board from player snapshot and deterministic seed. Verified 2026-06-17 by `GOCACHE=/private/tmp/task-0071-go-build go test ./internal/game/quests -count=1`.
- [x] Filter quest templates by rank and requirements. Verified 2026-06-17 by `GOCACHE=/private/tmp/task-0071-go-build go test ./internal/game/quests -count=1`.
- [x] Weight offers by player needs. Verified 2026-06-17 by `GOCACHE=/private/tmp/task-0071-go-build go test ./internal/game/quests -count=1`.
- [x] Generate exactly 10 offers. Verified 2026-06-17 by `GOCACHE=/private/tmp/task-0071-go-build go test ./internal/game/quests -count=1`.
- [x] Generate reward payload upfront. Verified 2026-06-17 by `GOCACHE=/private/tmp/task-0071-go-build go test ./internal/game/quests -count=1`.
- [x] Store generated payload and reward payload. Verified 2026-06-17 by `go test ./internal/game/quests -count=1`.
- [ ] Expire old offers.

## TODO: Accept, Progress, Claim

- [ ] Implement `AcceptQuest`.
- [ ] Validate offer exists and is not expired.
- [ ] Validate active quest count is below 3.
- [ ] Validate requirements still met.
- [ ] Insert accepted quest.
- [ ] Remove or mark offer accepted.
- [ ] Implement event consumers for `combat.npc_killed`, `loot.picked_up`, and `craft.job_completed`.
- [ ] Add scanner and building event consumers as skeletons.
- [ ] Update progress only for matching active quests.
- [ ] Mark quest completed when objective is met.
- [ ] Prevent progress overflow after completion.
- [ ] Implement `ClaimReward`.
- [ ] Lock quest during claim.
- [ ] Mark claimed before granting or within a transaction-safe flow.
- [ ] Grant credits through wallet service.
- [ ] Grant items through inventory service.
- [ ] Grant XP through progression service.
- [ ] Use reference `quest_reward:<player_quest_id>`.

## TODO: Reroll

- [ ] Implement board reroll command.
- [ ] Debit reroll credits.
- [ ] Expire old unaccepted offers.
- [ ] Generate new offers.
- [ ] Add rare reward cap hook.
- [ ] Add reroll cost scaling hook.

## Tests

- [x] Board generates 10 offers. Verified 2026-06-17 by `GOCACHE=/private/tmp/task-0071-go-build go test ./internal/game/quests -count=1`.
- [x] Offer generation is deterministic for same seed. Verified 2026-06-17 by `GOCACHE=/private/tmp/task-0071-go-build go test ./internal/game/quests -count=1`.
- [ ] Accept fails for expired offer.
- [ ] Accept max 3 active quests.
- [ ] Reroll charges credits.
- [ ] Reroll does not affect accepted quests.
- [ ] Kill event progresses only matching quest.
- [ ] Loot event progresses only matching quest.
- [ ] Craft event progresses only matching quest.
- [ ] Completed quest does not progress further incorrectly.
- [ ] Reward claim grants exactly once.
- [ ] Duplicate reward claim does not duplicate XP/items/currency.
- [ ] Rare reward cap hook can block excessive rare offers.

## Abuse And Safety Checks

- [ ] Client cannot send quest progress directly.
- [ ] Reward duplicate blocked by quest state and ledger reference.
- [ ] Reroll rare farming has cost and cap hooks.
- [ ] Market quest wash-trade remains out of MVP.
- [ ] Reward error messages do not leak hidden quest targets.

## Done Criteria

- [ ] Quest board gives players directional tasks.
- [ ] Quest rewards are idempotent.
- [ ] Quest events integrate combat, loot, craft, and XP.
- [ ] MVP rank milestones can depend on quest completion.
- [ ] `go test ./...` passes.
- [ ] `git diff --check` passes.

## Resume Notes

If resuming here, check whether rewards are generated at offer time. If rewards are generated at claim time, review reroll and claim-manipulation risks before continuing.
