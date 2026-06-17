# Gameplay Packages

`internal/game` contains server-authoritative gameplay domain code. It is intentionally separate from Symphony orchestration code under `internal/symphony`.

Package boundaries:

- `foundation`: shared gameplay primitives such as IDs, clocks, RNG, amounts, idempotency keys, and domain errors.
- `contracts`: client/server request, response, and API envelope contracts.
- `catalog`: versioned static gameplay definitions such as recipes, quests, loot tables, ships, modules, and balancing data.
- `events`: domain and realtime event envelope helpers.
- `testutil`: deterministic gameplay test helpers, fixtures, event recorders, and event assertions.

Gameplay packages may import the Go standard library and other packages under `internal/game`. They must not import `internal/symphony`; Symphony may orchestrate work around the game, but gameplay domain packages must not depend on orchestration, OpenAI clients, local trackers, prompts, or workspace tooling.
