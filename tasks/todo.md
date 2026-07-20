# Krampus: review fixes + two features

## Features
- [x] F1: One-click allowlist by cert/team ID from Programs & Events (admin, immediate)
- [x] F2: Discord alert on new proposal (webhook URL from env `DISCORD_WEBHOOK_URL`)

## Review fixes (1..11)
- [x] 1: Authenticate Santa sync endpoints (shared `SANTA_SYNC_KEY`, enforced when set)
- [x] 2: Fix CORS (reflect configured origins; only send credentials for allowed origin)
- [x] 3: Refuse default JWT secret in release mode
- [x] 4: Fix RuleDownload cursor pagination (parse + emit cursor consistently)
- [x] 5: Deduplicate rules (UNIQUE(identifier, rule_type) + upsert)
- [x] 6: Open SQLite with WAL + busy_timeout + foreign_keys=on
- [x] 7: Make preflight client_mode configurable (`SANTA_CLIENT_MODE`), stop hardcoding
- [~] 8: Protobuf event upload -- graceful handling + guidance. Full protobuf DECODE
        is deferred: it needs Santa's sync protobuf schema + generated Go and is a
        standalone task. Behavior now: acknowledge with 200, concise warning, doc note.
- [x] 9: Body-size limit on Santa endpoints (10 MiB, anti-DoS)
- [x] 10: Add Go unit tests (config, buildDSN, discord payload + POST, rule type)
- [x] 11: Enforce FK cascades (covered by fix 6: `_foreign_keys=on`)

## Verify (all done)
- [x] `go build ./server` -> exit 0
- [x] `go test ./server/...` -> all packages ok
- [x] `cd client && npm run build` -> built
- [x] Runtime smoke test: 401 w/o key, 200 w/ key, client_mode=MONITOR from env
- [x] Cursor: 150 rules -> page1=100+cursor, page2=50+no cursor (was: stuck at 100)
- [x] Unique index rejects dup; handler upsert updates in place (1 row)

## Review section (results)
- All 2 features + 10/11 bug fixes landed and verified. #8 (protobuf decode) is
  intentionally partial with a documented reason.
- New env vars: CORS_ALLOWED_ORIGINS, SANTA_SYNC_KEY, SANTA_CLIENT_MODE, DISCORD_WEBHOOK_URL.
- Note: the embedded `server/static` assets were regenerated from `client/dist`,
  so their hashed filenames changed in the diff.
