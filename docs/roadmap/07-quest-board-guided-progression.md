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
- [x] Expire old offers. Verified 2026-06-18 by clock-aware `QuestService.BoardOffers` pruning and expired accept cleanup tests with `go test ./internal/game/quests -count=1`.

## TODO: Accept, Progress, Claim

- [x] Implement `AcceptQuest`. Verified 2026-06-17 by `GOCACHE=/private/tmp/task-0072-go-build go test ./internal/game/quests -count=1`.
- [x] Validate offer exists and is not expired. Verified 2026-06-17 by `GOCACHE=/private/tmp/task-0072-go-build go test ./internal/game/quests -count=1`.
- [x] Validate active quest count is below 3. Verified 2026-06-17 by `GOCACHE=/private/tmp/task-0072-go-build go test ./internal/game/quests -count=1`.
- [x] Validate requirements still met. Verified 2026-06-17 by `GOCACHE=/private/tmp/task-0072-go-build go test ./internal/game/quests -count=1`.
- [x] Insert accepted quest. Verified 2026-06-17 by `GOCACHE=/private/tmp/task-0072-go-build go test ./internal/game/quests -count=1`.
- [x] Remove or mark offer accepted. Verified 2026-06-17 by `GOCACHE=/private/tmp/task-0072-go-build go test ./internal/game/quests -count=1`.
- [x] Implement event consumers and domain envelope routing for `combat.npc_killed`, `loot.picked_up`, and `craft.job_completed`. Verified 2026-06-18 by `go test ./internal/game/quests -run 'TestQuestDomainEvents(CompleteAndClaimQuestAuthorizedXP|UseStableDomainProgressKeys)' -count=1` and `go test ./internal/game/quests -count=1`.
- [x] Add scanner and building event consumers as skeletons. Verified 2026-06-17 for scan/build/deliver skeleton validation by `GOCACHE=/private/tmp/task-0073-go-build go test ./internal/game/quests -count=1`.
- [x] Update progress only for matching active quests. Verified 2026-06-17 by `GOCACHE=/private/tmp/task-0073-go-build go test ./internal/game/quests -count=1`.
- [x] Mark quest completed when objective is met. Verified 2026-06-17 by `GOCACHE=/private/tmp/task-0073-go-build go test ./internal/game/quests -count=1`.
- [x] Prevent progress overflow after completion. Verified 2026-06-17 by `GOCACHE=/private/tmp/task-0073-go-build go test ./internal/game/quests -count=1`.
- [x] Implement `ClaimReward`. Verified 2026-06-17 by `GOCACHE=/private/tmp/task-0074-go-build go test ./internal/game/quests -count=1`.
- [x] Lock quest during claim. Verified 2026-06-17 by `GOCACHE=/private/tmp/task-0074-go-build go test ./internal/game/quests -count=1`.
- [x] Mark claimed before granting or within a transaction-safe flow. Verified rollback-safe retry flow 2026-06-17 by `GOCACHE=/private/tmp/task-0074-go-build go test ./internal/game/quests -count=1`.
- [x] Grant credits through wallet service. Verified 2026-06-17 by `GOCACHE=/private/tmp/task-0074-go-build go test ./internal/game/quests -count=1`.
- [x] Grant items through a quest inventory boundary. Verified 2026-06-17 by `GOCACHE=/private/tmp/task-0074-go-build go test ./internal/game/quests -count=1`; concrete economy inventory adapter is tracked in `docs/todo.md`.
- [x] Grant XP through progression service. Verified 2026-06-17 by `GOCACHE=/private/tmp/task-0074-go-build go test ./internal/game/quests -count=1`.
- [x] Use reference `quest_reward:<player_quest_id>`. Verified 2026-06-17 by `GOCACHE=/private/tmp/task-0074-go-build go test ./internal/game/quests -count=1`.

## TODO: Reroll

- [x] Implement board reroll command. Verified 2026-06-17 by `GOCACHE=/private/tmp/task-0075-go-build go test ./internal/game/quests -count=1`.
- [x] Debit reroll credits. Verified 2026-06-17 by `GOCACHE=/private/tmp/task-0075-go-build go test ./internal/game/quests -count=1`.
- [x] Expire old unaccepted offers. Verified 2026-06-17 by `GOCACHE=/private/tmp/task-0075-go-build go test ./internal/game/quests -count=1`.
- [x] Generate new offers. Verified 2026-06-17 by `GOCACHE=/private/tmp/task-0075-go-build go test ./internal/game/quests -count=1`.
- [x] Add rare reward cap hook. Verified placeholder exposure 2026-06-17 by `GOCACHE=/private/tmp/task-0075-go-build go test ./internal/game/quests -count=1`.
- [x] Add reroll cost scaling hook. Verified 2026-06-17 by `GOCACHE=/private/tmp/task-0075-go-build go test ./internal/game/quests -count=1`.

## Tests

- [x] Board generates 10 offers. Verified 2026-06-17 by `GOCACHE=/private/tmp/task-0071-go-build go test ./internal/game/quests -count=1`.
- [x] Offer generation is deterministic for same seed. Verified 2026-06-17 by `GOCACHE=/private/tmp/task-0071-go-build go test ./internal/game/quests -count=1`.
- [x] Accept fails for expired offer. Verified 2026-06-17 by `GOCACHE=/private/tmp/task-0072-go-build go test ./internal/game/quests -count=1`.
- [x] Accept max 3 active quests. Verified 2026-06-17 by `GOCACHE=/private/tmp/task-0072-go-build go test ./internal/game/quests -count=1`.
- [x] Accept rejects another player's offer. Verified 2026-06-17 by `GOCACHE=/private/tmp/task-0072-go-build go test ./internal/game/quests -count=1`.
- [x] Duplicate accept returns the same accepted quest without creating another active quest. Verified 2026-06-17 by `GOCACHE=/private/tmp/task-0072-go-build go test ./internal/game/quests -count=1`.
- [x] Accept rechecks requirements at accept time. Verified 2026-06-17 by `GOCACHE=/private/tmp/task-0072-go-build go test ./internal/game/quests -count=1`.
- [x] Reroll charges credits. Verified 2026-06-17 by `GOCACHE=/private/tmp/task-0075-go-build go test ./internal/game/quests -count=1`.
- [x] Reroll does not affect accepted quests. Verified 2026-06-17 by `GOCACHE=/private/tmp/task-0075-go-build go test ./internal/game/quests -count=1`.
- [x] Kill event progresses only matching quest. Verified 2026-06-17 by `GOCACHE=/private/tmp/task-0073-go-build go test ./internal/game/quests -count=1`.
- [x] Loot event progresses only matching quest. Verified 2026-06-17 by `GOCACHE=/private/tmp/task-0073-go-build go test ./internal/game/quests -count=1`.
- [x] Craft event progresses only matching quest. Verified 2026-06-17 by `GOCACHE=/private/tmp/task-0073-go-build go test ./internal/game/quests -count=1`.
- [x] Completed quest does not progress further incorrectly. Verified 2026-06-17 by `GOCACHE=/private/tmp/task-0073-go-build go test ./internal/game/quests -count=1`.
- [x] Reward claim grants exactly once. Verified 2026-06-17 by `GOCACHE=/private/tmp/task-0074-go-build go test ./internal/game/quests -count=1`.
- [x] Duplicate reward claim does not duplicate XP/items/currency. Verified 2026-06-17 by `GOCACHE=/private/tmp/task-0074-go-build go test ./internal/game/quests -count=1`.
- [x] Rare reward cap hook can block excessive rare offers. Verified 2026-06-18 by generation and reroll cap-block tests with `go test ./internal/game/quests -count=1`.

## Abuse And Safety Checks

- [x] Client cannot send quest progress directly. Verified 2026-06-18 by realtime quest-operation allowlist and progress-op rejection test `go test ./internal/game/realtime -run TestOperationRegistryRejectsClientAuthoredQuestProgressOperations -count=1`.
- [x] Reward duplicate blocked by quest state and ledger reference. Verified 2026-06-17 by `GOCACHE=/private/tmp/task-0074-go-build go test ./internal/game/quests -count=1`.
- [x] Reroll rare farming has cost and cap hooks. Verified placeholder hooks 2026-06-17 by `GOCACHE=/private/tmp/task-0075-go-build go test ./internal/game/quests -count=1`.
- [x] Market quest wash-trade remains out of MVP. Verified 2026-06-18 by MVP catalog/type rejection test `go test ./internal/game/quests -run TestMVPQuestCatalogExcludesMarketQuestTypes -count=1`.
- [x] Reward error messages do not leak hidden quest targets. Verified 2026-06-18 by safe claim error envelope tests `go test ./internal/game/quests -run 'TestClaimReward(ErrorsDoNotLeakHiddenQuestTargets|InvalidStoredRewardPayloadUsesSafePublicError)' -count=1`.

## Done Criteria

- [x] Quest board gives players directional tasks. Verified 2026-06-18 by directional objective/target-region board test `go test ./internal/game/quests -run TestGenerateBoardOffersCarryDirectionalTargetsForPlayer -count=1`.
- [x] Quest rewards are idempotent. Verified 2026-06-17 by `GOCACHE=/private/tmp/task-0074-go-build go test ./internal/game/quests -count=1`.
- [x] Quest events integrate combat, loot, craft, and XP. Verified 2026-06-18 by domain-event-to-claim-to-XP and stable domain progress key tests `go test ./internal/game/quests -run 'TestQuestDomainEvents(CompleteAndClaimQuestAuthorizedXP|UseStableDomainProgressKeys)' -count=1`.
- [x] MVP rank milestones can depend on quest completion. Verified 2026-06-18 by quest reward reference rank milestone test `go test ./internal/game/progression -run TestTryRankUpRequiresQuestCompletionMilestoneFromQuestAuthority -count=1`.
- [x] `go test ./...` passes. Verified 2026-06-18 by `go test ./...`.
- [x] `git diff --check` passes. Verified 2026-06-18 by `git diff --check`.

## Resume Notes

If resuming here, check whether rewards are generated at offer time. If rewards are generated at claim time, review reroll and claim-manipulation risks before continuing.
