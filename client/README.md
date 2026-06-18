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

Run the smoke check with its self-started Vite app server:

```bash
npm --cache /tmp/gameproject-npm-cache run smoke
```

Or point it at an already-running dev server:

```bash
npm --cache /tmp/gameproject-npm-cache run smoke -- --url http://127.0.0.1:5173
```

The smoke check starts its own local WebSocket fixture and verifies connect,
snapshot, move, combat, loot, scan, canvas, responsive layout, and hidden-data
rejection paths. The current prototype still uses an offline demo harness when
no WebSocket gateway is connected. Production Go WebSocket transport and
authenticated runtime adapters remain future work.
