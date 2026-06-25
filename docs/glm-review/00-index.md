# GLM Code Review — DarkOrbit-style Space MORPG (Go backend)

Production-focused review of the Go game server and browser client.
All findings cite exact `file:line` references. Reports are split by theme
but keep the 16 requested section numbers.

## Files in this review

| File                                  | Sections                                                                                                                                      |
| ------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------- |
| `01-executive-summary.md`             | §1 Executive Summary + critical-issues table                                                                                                  |
| `02-architecture-networking.md`       | §2 Game Server Architecture · §3 Multiplayer Networking & Protocol · §4 Real-Time State Synchronization                                       |
| `03-simulation-performance.md`        | §5 Movement, Combat & Projectile Logic · §7 Tick/Game Loop Performance · §8 Entity System & World Management · §13 Load, Scalability & Memory |
| `04-concurrency.md`                   | §6 Concurrency, Goroutines & Race Conditions                                                                                                  |
| `05-data-economy.md`                  | §9 Database, Persistence & Progression · §11 API / Backend Services                                                                           |
| `06-security-anticheat.md`            | §10 Security, Anti-Cheat & Abuse Prevention                                                                                                   |
| `07-quality-testing-observability.md` | §12 Error Handling, Logging & Observability · §14 Testing Gaps · §15 Code Quality & Go Idioms                                                 |
| `08-critical-fix-plan.md`             | §16 Critical Issues & Suggested Fix Plan                                                                                                      |

## Severity legend

- **CRITICAL** — economy loss / process crash / guaranteed exploit. Fix before any real players.
- **HIGH** — desync, cheating, or hard scalability ceiling under load.
- **MEDIUM** — correctness/ops risk, degrades under contention.
- **LOW** — cleanup / hardening / minor.

## Verification performed

- `go build ./cmd/game-server/` → **passes**
- `go vet ./internal/game/server/ ./internal/game/economy/ ./internal/game/auth/` → **passes**
- ~1,753 test functions exist across `internal/`
- Every file:line cited below was read or re-read during the review.
- If available - use symphony to organize your agents

## Symphony And Tooling

If you are running inside a Symphony-managed worker workspace, do not use this
file as your operating guide. Follow `docs/symphony-worker-rules.md` and the
task prompt instead. Do not spawn subagents, create Symphony tasks, dispatch
agents, or manage the Symphony queue.

Keep Symphony/orchestration code in Go.

Symphony code should stay separate from game server domain code. Do not mix
issue orchestration, OpenAI client logic, or workflow runner concerns into
gameplay modules.

## Code Shape

Prefer small, readable files with clear ownership.

Avoid large files. As a soft rule, when production code grows beyond 300-500
lines, consider splitting by responsibility.

Use domain-specific names instead of vague names:

- prefer `auth_session.go` over `utils.go`
- prefer `wallet_ledger.go` over `helpers.go`
- prefer `loot_pickup_handler.go` over `manager.go`
- prefer `route_settlement.go` over `common.go`

Avoid duplicate business rules. Good candidates for shared helpers:

- positive amount validation
- ownership checks
- idempotency checks
- ledger writes
- transaction/outbox patterns
- rate-limit middleware
- visibility/range validation primitives
- client-safe snapshot filtering
- no monolith - spaghetti code! small chunks of code for each module, readable good code!

Do not abstract too early. Small local duplication is acceptable while a rule is
still changing, but once it becomes a gameplay or economy invariant, centralize
it.

Keep functions focused:

- validation should be easy to find
- mutation should be transaction-scoped
- event publishing should happen after commit
- formatting/UI concerns should not live in domain services

Do not commit:

- secrets
- `.env`
- local logs
- `.symphony` workspaces
- generated dependency folders
- large temporary outputs
