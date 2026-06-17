# Phase 07: Quest Board And Guided Progression

## Status

- State: Not started
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

- [ ] Define quest template model.
- [ ] Define generated offer model.
- [ ] Define player quest model.
- [ ] Define quest state machine.
- [ ] Define objective schemas for kill, collect, craft, scan, build, and deliver.
- [ ] Define reward payload model.
- [ ] Generate board from player snapshot and deterministic seed.
- [ ] Filter quest templates by rank and requirements.
- [ ] Weight offers by player needs.
- [ ] Generate exactly 10 offers.
- [ ] Generate reward payload upfront.
- [ ] Store generated payload and reward payload.
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

- [ ] Board generates 10 offers.
- [ ] Offer generation is deterministic for same seed.
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
