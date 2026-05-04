# SQL / Database

Use this skill when the task touches relational databases, schemas, migrations, queries, indexes, ORMs, transactions, or persistent application data.

Do not load this skill just because an app has a database dependency. Use it when the current work changes or diagnoses database behavior.

## Activation

Use for:

- `.sql` files, migrations, seeds, schema files, stored procedures, database config, backup/restore scripts.
- PostgreSQL, MySQL/MariaDB, SQLite, SQL Server, Prisma, Drizzle, TypeORM, Sequelize, Knex, SQLAlchemy, Alembic, Django ORM, Laravel migrations/Eloquent, EF Core migrations/LINQ queries.
- Query bugs, slow endpoints caused by data access, pagination, filtering, sorting, uniqueness, transactions, locks, deadlocks, N+1 queries, multi-tenant isolation, and data integrity issues.

Do not use for:

- Generic backend work where no query/schema/data access is changed.
- Pure Redis/cache messaging unless SQL persistence is also involved.

## Core Rule

Treat database changes as durable production changes, not just code edits. Preserve data, define migrations explicitly, keep rollback/risk in mind, and never silently change semantics of existing records.

## 80/20 Database Workflow

1. Identify the actual database and ORM before writing code.
2. Read existing schema/migrations/models before inventing tables or fields.
3. Prefer migrations over manual schema drift.
4. Make queries parameterized by default.
5. Check indexes for every new frequent lookup, join, sort, or uniqueness rule.
6. Use transactions for multi-step writes that must succeed or fail together.
7. Consider production data volume before adding expensive joins, LIKE filters, or per-row loops.
8. Verify with the project’s real test/build commands when available.

## Schema Design

Prefer explicit constraints over application-only promises:

- `NOT NULL` when the field is required.
- `UNIQUE` for real uniqueness, not just pre-insert checks.
- Foreign keys where referential integrity matters.
- Check constraints for narrow valid ranges/statuses when supported.
- Timestamps with clear semantics: `created_at`, `updated_at`, optionally `deleted_at`.

Avoid:

- Adding nullable columns without deciding how old rows behave.
- Encoding many meanings into one string field.
- Storing numbers/dates as strings unless required by external format.
- Introducing soft deletes without updating uniqueness, filtering, and restore behavior.
- Changing enum/status values without migration/backfill strategy.

## Migrations

Good migrations are deterministic, reversible when practical, and safe for existing data.

Before writing a migration, answer:

- What happens to existing rows?
- Is there a default/backfill?
- Can this lock a large table?
- Does the app need a two-step deploy?
- Are indexes created safely for the target database?

For production-size PostgreSQL, avoid patterns that can lock hot tables unnecessarily. Prefer staged changes for risky migrations: add nullable column, backfill in batches, then enforce `NOT NULL` later.

## Query Safety

Never build SQL by concatenating untrusted input.

Prefer parameterized queries:

```ts
await db.query(
  'SELECT * FROM users WHERE email = $1 AND tenant_id = $2',
  [email, tenantId]
)
```

For dynamic sorting/filtering, whitelist identifiers:

```ts
const allowedSort = new Set(['created_at', 'name', 'email'])
const sort = allowedSort.has(input.sort) ? input.sort : 'created_at'
const rows = await db.query(`SELECT * FROM users ORDER BY ${sort} DESC LIMIT $1`, [limit])
```

Identifiers cannot usually be bound like values. Whitelist them.

## Transactions

Use a transaction when multiple writes form one logical operation:

```sql
BEGIN;
UPDATE accounts SET balance = balance - 100 WHERE id = 1;
UPDATE accounts SET balance = balance + 100 WHERE id = 2;
COMMIT;
```

In app code, ensure all queries inside the transaction use the transaction client/session, not the global database client.

Common AI mistake:

```ts
const tx = await db.transaction()
await tx.insert(order)
await db.insert(orderItems) // wrong: escapes transaction
await tx.commit()
```

## Indexes

Add indexes based on query shape, not vibes.

Index candidates:

- Foreign keys used in joins.
- Columns used in frequent `WHERE` filters.
- Columns used in common `ORDER BY` with filters.
- Composite filters such as `(tenant_id, created_at)`.
- Cursor pagination order columns.
- Uniqueness rules such as `(tenant_id, slug)`.

Composite index order matters. Put equality filters first, then range/sort columns:

```sql
CREATE INDEX idx_events_tenant_created ON events (tenant_id, created_at DESC);
```

Avoid:

- Indexing every column.
- Adding indexes that duplicate existing composite indexes.
- Forgetting that indexes speed reads but slow writes and cost storage.

## Pagination

For large tables, prefer cursor/keyset pagination over huge offsets.

Offset is simple but degrades:

```sql
SELECT * FROM events ORDER BY created_at DESC LIMIT 50 OFFSET 50000;
```

Cursor is usually better:

```sql
SELECT * FROM events
WHERE created_at < $1
ORDER BY created_at DESC
LIMIT 50;
```

Use stable ordering. If timestamps can tie, add `id` as a tie-breaker.

## ORM Rules

ORMs do not remove database thinking.

Watch for:

- N+1 queries from lazy-loaded relations.
- Unbounded `.findMany()` / `.all()` calls.
- Filtering in memory instead of SQL.
- Missing tenant/user scoping.
- Accidentally selecting sensitive fields.
- Migration drift between model definitions and actual database.

Prefer explicit selects for API responses:

```ts
select: {
  id: true,
  email: true,
  name: true,
  createdAt: true,
}
```

Do not return entire ORM entities by default.

## Multi-Tenant / Authorization Data

Every tenant-scoped query must include the tenant/account/org scope at the database access layer, not only in the controller.

Bad:

```ts
const invoice = await db.invoice.findUnique({ where: { id } })
```

Better:

```ts
const invoice = await db.invoice.findFirst({
  where: { id, tenantId: ctx.tenantId },
})
```

This is one of the most common serious agent mistakes.

## Data Validation Boundary

Validate before writing, constrain inside the database, and normalize consistently.

Examples:

- Lowercase emails before unique checks if the product treats email as case-insensitive.
- Store money in integer minor units or a decimal type, not float.
- Store timestamps in UTC unless the app has a specific timezone model.
- Validate JSON payload shape before storing JSON blobs.

## Common Database Tasks

### Add a Field

- Add migration.
- Update model/type definitions.
- Backfill or define default behavior.
- Update create/update paths.
- Update response serializers only if public.
- Add/adjust tests.

### Add a Table

- Define primary key strategy.
- Add foreign keys and indexes.
- Decide cascade behavior.
- Add created/updated timestamps if useful.
- Add repository/service access methods.
- Add seed/test fixtures if project uses them.

### Optimize Slow Query

- Capture the actual SQL.
- Check query plan if available.
- Add/select the right index.
- Reduce selected columns.
- Avoid per-row queries.
- Move filtering/sorting into SQL.
- Verify behavior with realistic row counts.

## Common AI Mistakes

Avoid these:

- Creating a new database client per request instead of using the project’s connection lifecycle.
- Ignoring existing migrations and editing generated schema directly.
- Adding fields to TypeScript/Python models without a migration.
- Returning password hashes, tokens, internal IDs, or private columns from ORM entities.
- Using `LIKE '%term%'` on large tables without understanding performance.
- Deleting rows where the existing app expects soft delete.
- Replacing a safe parameterized query with string interpolation.
- Writing tenant-scoped features without tenant filters on reads and writes.
- Adding indexes blindly without checking existing ones.
- Using offset pagination for hot large feeds.

## Verification

Prefer project-specific commands. Common ones:

```bash
# Node ORMs
npm run typecheck
npm test
npx prisma migrate dev
npx prisma generate

# Python
pytest
alembic upgrade head
python manage.py migrate

# Laravel
php artisan migrate
php artisan test

# .NET
dotnet ef database update
dotnet test
```

If migrations cannot be run, state that clearly and inspect generated files carefully.
