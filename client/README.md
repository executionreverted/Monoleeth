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

Vite proxies `/api` and `/ws` to `http://127.0.0.1:8080` / `ws://127.0.0.1:8080`.
The Go server still validates the browser `Origin`; do not use wildcard origins
with credentials.

Run the smoke check with its self-started Vite app server:

```bash
npm --cache /tmp/gameproject-npm-cache run smoke
```

Or point it at an already-running dev server:

```bash
npm --cache /tmp/gameproject-npm-cache run smoke -- --url http://127.0.0.1:5173
```

The smoke check still starts its own local WebSocket fixture while the browser
client is being moved onto the authenticated Go transport. Use the Phase 02 Go
server commands above for the real `/api` and `/ws` runtime path.
