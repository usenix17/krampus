# Krampus: migrations, hardening, API + UI overhaul, protobuf

Plan: `docs/superpowers/plans/2026-10-10-krampus-improvements.md`
Spec: `docs/superpowers/specs/2026-10-10-krampus-improvements-design.md`

## Phase 1: Migration system + bug/security fixes
- [x] T1: Versioned migration runner (schema_migrations, legacy adoption, 4 migrations)
- [x] T2: Decompress middleware rewrite (empty-body panic fix + bomb cap)
- [x] T3: Voted rules deliver custom_message to Santa + voting tests
- [x] T4: Config hardening (parseInt defaults, COOKIE_SECURE, STALE_MACHINE_HOURS)
- [x] T5: Cookie flags (SameSite/Secure) + logout revokes cookie session
- [x] T6: CORS wildcard must not reflect with credentials
- [x] T7: machine_id validation, RegisterMachine id, events count err, OIDC alg pin, dead code

## Phase 2: Backend API
- [x] T8: Pagination + search on rules/programs/proposals/users; event filters
- [x] T9: CSV export (events, programs, rules)
- [x] T10: Stats overview endpoint (+ execution_time UTC normalization, migration 005)
- [x] T11: Reject proposal, vote history, rule edit, machine events

## Phase 3: Frontend
- [x] T12: Shared infra (toast, PageHeader), theme + dark mode, CSS cleanup
- [x] T13: DataTable + convert pages to new API and MUI v7 Grid
- [x] T14: Events/Programs filters + export buttons + post-action refresh (+ decision prefix fix)
- [x] T15: Dashboard analytics charts (@mui/x-charts) + shared datetime util
- [x] T16: Machine detail page, vote history + reject UI, rule edit dialog
- [x] T17: Rebuild embedded frontend + end-to-end verification (found+fixed login contrast & session-expiry UTC bugs)

## Phase 4: Protobuf
- [ ] T18: Protobuf event upload (research module, decode, shared store path)

## Verify
- [ ] go build/vet/test all green
- [ ] npm run build clean
- [x] Migration proof against copy of real DB (T1: adoption + self-healing comment column, versions 1-6, integrity ok)
- [x] Browser click-through (login dark/light, dashboard charts, events filters, proposals vote dialog, machine detail)

## Review section (results)
Every task went through implement -> spec review -> quality review -> fix loop
with fresh subagents. Notable bugs caught and fixed along the way:
- Unauthenticated decompress panic on empty body (+ decompression-bomb cap).
- Voted rules delivered no block message to Santa (custom_message vs comment).
- CORS reflected arbitrary origins with credentials under a "*" allow-list.
- parseInt(VOTE_THRESHOLD) fell back to 0 -> auto-approve on first vote.
- Logout never revoked the httpOnly cookie session.
- OIDC HS256 path trusted the token header alg (algorithm-confusion shape).
- Programs search distorted allow/block aggregate counts.
- execution_time, finalized_at, and sessions.expires_at stored with a local
  tz offset but compared lexically against bare-UTC datetime('now') -- the
  session one caused real early-expiry; all normalized to canonical UTC with
  migrations 5 and 6.
- Decision filter/counts exact-matched ALLOW/BLOCK, missing real Santa
  ALLOW_BINARY/BLOCK_BINARY data -> switched to GLOB prefix matching.
- usePagedQuery stuck-loading on mount; page not reset on filter change.
- Login card white-on-white text in dark mode.

Verification: go build/vet/test ./... green; npm run build clean; real DB
migrated in place with no data loss; full browser click-through in both themes.

## Prior work (completed earlier)
- F1 one-click allowlist, F2 Discord alerts, review fixes 1-11 (protobuf decode
  was the one deferred item; it is T18 above).
