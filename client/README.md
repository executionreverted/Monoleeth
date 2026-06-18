# Space MORPG Client

Phase 11 browser client prototype.

## Commands

Use a temporary npm cache if the default home cache is not writable:

```bash
npm --cache /tmp/gameproject-npm-cache install
npm --cache /tmp/gameproject-npm-cache test
npm --cache /tmp/gameproject-npm-cache run build
```

Run the local client:

```bash
npm --cache /tmp/gameproject-npm-cache run dev -- --port 5173
```

Run the smoke check against an already-running dev server:

```bash
npm --cache /tmp/gameproject-npm-cache run smoke -- --url http://127.0.0.1:5173
```

The current prototype uses an explicit offline demo harness when no WebSocket
gateway is connected. Real server commands are limited to the Phase 04 realtime
contract: `move_to`, `stop`, `debug_snapshot`, and `debug_spawn_npc`.
