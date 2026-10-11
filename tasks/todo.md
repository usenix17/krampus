# Krampus: migrations, hardening, API + UI overhaul, protobuf

Plan: `docs/superpowers/plans/2026-10-10-krampus-improvements.md`
Spec: `docs/superpowers/specs/2026-10-10-krampus-improvements-design.md`

## Phase 1: Migration system + bug/security fixes
- [ ] T1: Versioned migration runner (schema_migrations, legacy adoption, 4 migrations)
- [ ] T2: Decompress middleware rewrite (empty-body panic fix)
- [ ] T3: Voted rules deliver custom_message to Santa + voting tests
- [ ] T4: Config hardening (parseInt defaults, COOKIE_SECURE, STALE_MACHINE_HOURS)
- [ ] T5: Cookie flags (SameSite/Secure) + logout revokes cookie session
- [ ] T6: CORS wildcard must not reflect with credentials
- [ ] T7: machine_id validation, RegisterMachine id, events count err, OIDC alg pin, dead code

## Phase 2: Backend API
- [ ] T8: Pagination + search on rules/programs/proposals/users; event filters
- [ ] T9: CSV export (events, programs, rules)
- [ ] T10: Stats overview endpoint
- [ ] T11: Reject proposal, vote history, rule edit, machine events

## Phase 3: Frontend
- [ ] T12: Shared infra (toast, PageHeader), theme + dark mode, CSS cleanup
- [ ] T13: DataTable + convert pages to new API and MUI v7 Grid
- [ ] T14: Events/Programs filters + export buttons + post-action refresh
- [ ] T15: Dashboard analytics charts (@mui/x-charts)
- [ ] T16: Machine detail page, vote history + reject UI, rule edit dialog
- [ ] T17: Rebuild embedded frontend + end-to-end verification

## Phase 4: Protobuf
- [ ] T18: Protobuf event upload (research module, decode, shared store path)

## Verify
- [ ] go build/vet/test all green
- [ ] npm run build clean
- [ ] Migration proof against copy of real DB
- [ ] Browser click-through

## Review section (results)
(to be filled at completion)

## Prior work (completed earlier)
- F1 one-click allowlist, F2 Discord alerts, review fixes 1-11 (protobuf decode
  was the one deferred item; it is T18 above).
