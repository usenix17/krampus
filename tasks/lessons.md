# Lessons

## 2026-10-11 (overhaul branch)
- Dark-mode conversions: any surface with a FIXED background (the login
  card's white glass panel) must pin its text color explicitly; inherited
  theme text goes white-on-white in dark mode. Caught by the user, not by
  build/lint/review. Rule: screenshot every page in BOTH modes before
  presenting UI work.
- Binding a Go time.Time into a SQLite datetime column stores a local
  timezone offset suffix, which breaks lexical comparison against
  datetime('now') (bare UTC). Rule: always bind
  t.UTC().Format("2006-01-02 15:04:05"), and grep for this pattern when
  touching any datetime column (it recurred three times: execution_time,
  finalized_at, sessions.expires_at).
- Santa decision strings are prefixed (ALLOW_BINARY, BLOCK_BINARY); any
  exact match on ALLOW/BLOCK silently drops real-agent data. Use GLOB
  prefix matching, and validate the prefix input before building patterns.
- Commit subjects: enforce the 50-char limit at commit time, not at the
  end -- 9 of 25 needed a history rewrite before push.
