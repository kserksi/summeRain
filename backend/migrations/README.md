# Schema migrations

The backend owns additive MySQL schema migrations through
`repository.ApplySchemaMigrations(db)`. Startup must stop immediately when that
function returns an error.

The runner:

- reserves one SQL connection and holds a database-scoped MySQL advisory lock;
- creates `schema_migrations` when needed;
- verifies the immutable SHA-256 checksum of every previously applied version;
- applies missing operations in version order;
- probes `information_schema` before adding columns or indexes;
- records a version only after all of its operations succeed.

MySQL DDL commits implicitly. Therefore every operation is additive and safe to
retry after interruption. Never edit an applied migration. Append a higher
version instead. Destructive renames, column drops, and table drops require a
separate compatibility and rollback plan and are intentionally absent from the
V2 bootstrap migrations.

Current versions:

- `2026071501`: add V2 pipeline metadata and indexes to `images`.
- `2026071502`: create V2 variants, upload sessions/parts, processing jobs, and
  transactional outbox tables.
- `2026071601`: add the singleton row used to serialize global upload-capacity
  reservations across backend instances.
- `2026071602`: add storage-reference lookup and bounded control-plane
  retention indexes.
- `2026071603`: add remote backend, endpoint, and bucket lineage columns and
  an index. Historical rows remain unclassified for the separate migration
  tool; server startup never infers their storage target.
