# Space MORPG Client

Phase 11 browser client prototype.

## Commands

Use a temporary npm cache if the default home cache is not writable:

```bash
npm --cache /tmp/gameproject-npm-cache install
npm --cache /tmp/gameproject-npm-cache run lint
npm --cache /tmp/gameproject-npm-cache test
npm --cache /tmp/gameproject-npm-cache run build
```

Run the local client:

```bash
npm --cache /tmp/gameproject-npm-cache run dev -- --port 5173
```

Run it against the Phase 02 Go server:

```bash
GAME_ALLOWED_ORIGINS=http://127.0.0.1:5173 go run ./cmd/game-server
npm --cache /tmp/gameproject-npm-cache run dev -- --port 5173
```

Vite proxies `/api` and `/ws` to `http://127.0.0.1:8080` / `ws://127.0.0.1:8080`
by default. Override both proxy targets for isolated runs with
`GAME_CLIENT_PROXY_TARGET=http://127.0.0.1:<port>`.
The Go server still validates the browser `Origin`; do not use wildcard origins
with credentials.

Run the focused Phase09 bounded-map/portal browser proof explicitly:

```bash
npm --cache /tmp/gameproject-npm-cache run e2e:phase09-map
```

Browser/e2e coverage should remain small per-flow suites under
`client/tests/e2e/`; do not wire new smoke scripts into `check` until that is an
intentional release-gate change.
