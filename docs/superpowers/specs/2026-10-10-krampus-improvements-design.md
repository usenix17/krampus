# Krampus Improvements: Migrations, Hardening, API, and UI

Date: 2026-10-10
Status: Approved

## Why

A full review of the codebase (Go backend and React frontend) found a set of
bugs, security weaknesses, and UX gaps, plus one structural problem: there is
no versioned database migration system, so schema changes cannot be applied
safely to existing deployments. The owner asked for a code and UI review,
sensible feature additions, and a guaranteed-safe SQLite migration path.

Success criteria:

- An existing `krampus.db` created by the current code upgrades in place with
  no manual steps and no data loss.
- A fresh database reaches the identical schema.
- The known bugs below are fixed and covered by tests.
- `go test ./...`, `go vet ./...`, and `npm run build` pass.
- The UI features are verified by running the server and exercising them in a
  browser.

## 1. Versioned migration system

Hand-rolled runner in `server/database/migrations.go`. No external migration
library: the need is ~80 lines of Go and a single-binary deployment.

- Table: `schema_migrations (version INTEGER PRIMARY KEY, name TEXT NOT NULL,
  applied_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP)`.
- Ordered slice of `{version int, name string, up func(*sql.Tx) error}`.
  Each migration runs in its own transaction; the version row is inserted in
  the same transaction as the DDL.
- Legacy-DB adoption: if `schema_migrations` is empty but the `users` table
  exists, record migration 001 as applied without executing it, then apply
  anything newer. Fresh databases execute 001 for real.
- Migration 001 (baseline): the full current schema including `rules.comment`
  and all existing indexes.
- Migration 002: one-time rule dedupe (keep MAX(id) per identifier+rule_type)
  followed by `CREATE UNIQUE INDEX idx_rules_identifier_type`. This replaces
  the destructive dedupe that currently runs on every boot.
- Migration 003: `UPDATE rules SET custom_message = comment WHERE
  custom_message IS NULL AND proposal_id IS NOT NULL` (backfill for the
  voted-rule message bug, section 2).
- Migration 004: indexes `idx_events_execution_time`, `idx_events_decision`,
  `idx_sessions_user_id`.
- `addColumnIfNotExists` and the boot-time dedupe are deleted.

Validation: tests cover fresh-DB creation, legacy-DB adoption, idempotent
re-runs, and the dedupe/backfill behavior. Additionally run the new binary
against a copy of `database/krampus.db` and diff schema and row counts.

## 2. Bug and security fixes

- `middleware/decompress.go`: rewrite. Guard empty bodies before indexing
  (currently panics on an empty POST to any Santa endpoint, remotely and
  unauthenticated when SANTA_SYNC_KEY is unset). Remove per-request raw-byte
  logging. Keep gzip/zlib/raw-deflate support under the 10 MiB body limit.
- Voted-rule block message: `services/voting.go` FinalizeProposal writes the
  message to `comment`, but Santa rule download reads `custom_message`, so
  voted rules deliver no block message. Fix: `custom_message` is the text
  Santa displays; `comment` is a human note. FinalizeProposal writes
  `custom_message`; migration 003 backfills existing rows.
- Cookies: set `SameSite=Lax` explicitly and `Secure` when the request is TLS
  or `COOKIE_SECURE=true` (new env var, default false for local HTTP).
- CORS: never send `Access-Control-Allow-Credentials: true` with a reflected
  origin when `*` is in the allow-list.
- `config.parseInt`: fall back to the documented default on parse error, not
  0 (a malformed VOTE_THRESHOLD currently auto-approves on the first vote).
- OIDC HS256 path: pin the expected algorithm from configuration instead of
  branching on the untrusted token header.
- `RegisterMachine`: return the real row id when the insert hits ON CONFLICT.
- `handlers/events.go`: handle the ignored count-query Scan error.
- `santa_sync` middleware: validate `machine_id` format (reject path
  traversal / absurd lengths); keep the no-key startup warning but make it
  prominent.
- Delete dead code: `services/plist.go` GeneratePlist, unused
  RevokeUserSessions (restore later if a "revoke user" feature lands).

## 3. Backend API additions

- Pagination and search: `page`, `per_page`, `search` on list endpoints for
  rules, programs, proposals, users. Events already paginates; it gains
  `search` plus machine and date-range filters. List responses become
  `{items, total, page, per_page}`. Frontend updates in lockstep; no
  compatibility shim.
- CSV export: `GET /api/events/export`, `/api/programs/export`,
  `/api/rules/export` stream CSV of the full filtered set (same filter
  params as the list endpoints).
- Analytics: `GET /api/stats/overview` returns allow/block counts per day for
  the last 30 days, top blocked binaries, events per machine, and machines
  with `last_sync` older than a threshold (env `STALE_MACHINE_HOURS`,
  default 24).
- Workflow: `POST /api/proposals/:id/reject` (admin) sets the REJECTED
  status, which exists in the schema but is currently unreachable;
  `GET /api/proposals/:id/votes` lists voters and their votes;
  `PUT /api/rules/:id` edits custom_message and comment;
  `GET /api/machines/:id/events` for the machine detail page.
- Protobuf event upload: on `eventupload`, detect
  `Content-Type: application/x-protobuf`, decode using the official
  northpolesec sync protos Go module, and map into the same event-store path
  as JSON. Fallback if the module is impractical: vendor protoc-generated
  code. The implementing step verifies the module builds before committing
  to it. Events are currently dropped silently for protobuf clients, which
  is Santa's default transport.

## 4. Frontend

- Fix the removed MUI v7 Grid API everywhere: `size={{ xs: 12, sm: 6 }}`
  instead of `item xs sm`.
- Delete conflicting Vite boilerplate: `index.css` dark-scheme globals,
  unused `App.css`, leftover svg assets. Hardcoded hex colors move into the
  theme.
- Shared infrastructure: a toast/notification context replacing the
  copy-pasted Snackbar blocks; a `PageHeader` component; a `DataTable`
  wrapper with server-side pagination and search bound to the new API shape;
  visible error and empty states on every page; aria-labels on icon-only
  buttons; memoized AuthContext value.
- Dark mode: toggle in the AppBar, persisted to localStorage, defaulting to
  the system preference.
- Dashboard: keep stat cards; add a 30-day allow/block trend chart and a
  top-blocked-binaries list using `@mui/x-charts` (stays within the MUI
  design system).
- New views: machine detail page (info, its events, sync-health badge);
  proposal vote history and an admin reject button; rule edit dialog.

## 5. Testing

- Go: migration runner (fresh, legacy adoption, idempotency, dedupe,
  backfill), voting service against a temp SQLite file (approve threshold,
  blocklist finalize, reject), decompress middleware (empty body, gzip,
  zlib), handler tests for the new endpoints.
- Frontend: `npm run build` passes; manual browser verification of dark
  mode, pagination, search, charts, machine detail, reject flow.

## 6. Execution

Phases, each ending in a conventional commit (no AI attribution):

1. Migration system + bug/security fixes + their tests.
2. Backend API additions + tests.
3. Frontend overhaul + features.
4. Protobuf event upload.

Constraints: SQLite stays `SetMaxOpenConns(1)` (the voting read-modify-write
depends on single-writer serialization); Santa sync endpoints remain
optional-auth because that matches Santa deployment reality.
