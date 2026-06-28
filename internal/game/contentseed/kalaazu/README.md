# Kalaazu Default Seed Inputs

Copied on: 2026-06-28

Source repository:

- https://github.com/manulaiko/Kalaazu

Source database folder:

- https://github.com/manulaiko/Kalaazu/tree/develop/Persistence/database

License:

- MIT
- https://raw.githubusercontent.com/manulaiko/Kalaazu/develop/LICENSE

Checked-in source dumps:

- `testdata/maps.sql`
- `testdata/maps_npcs.sql`
- `testdata/npcs.sql`
- `testdata/items.sql`
- `testdata/ships.sql`
- `testdata/maps_portals.sql`

These files are seed inputs only. Runtime code must not fetch GitHub or read
these raw SQL files directly as live gameplay truth. The content seed pipeline
parses these dumps into validated `content.Snapshot` rows, publishes them into
the content database when empty, and runtime then loads the current published DB
snapshot.

When a Kalaazu row has no equivalent in the current project schema, the importer
must count it in an import report instead of silently dropping it.
