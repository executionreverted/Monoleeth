# Running The Local Game Client

This is the local real-client path. It uses mail/password auth, server-owned
cookie sessions, and the Go WebSocket gateway. Do not use `?demo=1` unless you
are explicitly testing the isolated fixture/demo path.

## Start The Server

From the repo root:

```bash
go run ./cmd/game-server
```

The default server address is `:8080`, and the default allowed browser origins
are `http://localhost:5173` and `http://127.0.0.1:5173`.

Useful local overrides:

```bash
GAME_SERVER_ADDR=127.0.0.1:8080 \
GAME_ALLOWED_ORIGINS=http://127.0.0.1:5173 \
go run ./cmd/game-server
```

Optional local admin seed:

```bash
GAME_ADMIN_EMAIL=admin@example.com \
GAME_ADMIN_PASSWORD='replace-with-local-secret' \
GAME_ADMIN_CALLSIGN=Admin \
go run ./cmd/game-server
```

The admin seed is for reproducible local/dev setup only. Never use a shared or
production password here.

## Start The Client

In another terminal:

```bash
cd client
npm --cache /tmp/gameproject-npm-cache run dev
```

Open:

```text
http://127.0.0.1:5173
```

Vite proxies `/api` and `/ws` to the Go server on `127.0.0.1:8080`.

## Verification

Before handing off changes:

```bash
go test ./...
cd client
npm --cache /tmp/gameproject-npm-cache run check
cd ..
git diff --check
```

The client check runs lint, typecheck, unit tests, and the real-server
Playwright smoke. Smoke screenshots are written under
`output/screenshots/ui-implementation/10/`.
