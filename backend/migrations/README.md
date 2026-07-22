# Schema migrations

Starting with v2.0.3, server startup owns all MySQL bootstrap work through
`repository.BootstrapDatabase(ctx, db)`. Any bootstrap error is fatal: the
server must exit before it accepts traffic.

The bootstrap reserves one physical SQL connection, acquires the
database-scoped MySQL advisory lock on that connection, and keeps both until
every stage has finished. While holding the lock it:

1. creates or loads `schema_migrations`, then validates version continuity,
   known versions, and immutable SHA-256 checksums;
2. verifies that every object represented by an applied operation still exists,
   including recorded columns, indexes, tables, and the capacity-lock seed row;
3. removes incompatible legacy access-token data and columns when present;
4. applies the baseline GORM schema;
5. applies missing checksummed operations in version order, probing
   `information_schema` before adding columns or indexes and recording a
   version only after all of its operations succeed;
6. seeds required default configuration without overwriting existing values.

Ledger and applied-object validation happens before any destructive legacy
change. A missing recorded object, checksum mismatch, unknown version, version
gap, DDL failure, or seed failure aborts startup. The advisory lock is released
when the bootstrap returns.

MySQL DDL commits implicitly. Therefore every checksummed migration operation
is additive and safe to retry after interruption, but the bootstrap does not
promise transactional rollback of DDL that MySQL has already committed.
Correct the reported problem and run the bootstrap again. Never edit an applied
migration; append a higher version instead. Destructive renames, column drops,
and table drops require a separate compatibility and rollback plan and are
intentionally absent from the checksummed V2 migrations. The legacy
access-token cleanup is a dedicated compatibility stage outside that additive
migration list.

## First upgrade to v2.0.3

Stop every backend instance running v2.0.2 or earlier before starting the first
v2.0.3 instance. Do not perform a rolling deployment that overlaps old and new
binaries: older versions can run legacy cleanup, baseline migration, and
configuration seeding outside the advisory lock now owned by
`BootstrapDatabase`. After one v2.0.3 instance completes bootstrap successfully,
the remaining v2.0.3 instances may start normally and will serialize through
the same lock.

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
