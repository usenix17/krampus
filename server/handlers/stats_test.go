package handlers

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"krampus/server/config"
)

type statsOverviewResponse struct {
	Totals struct {
		Machines         int `json:"machines"`
		Rules            int `json:"rules"`
		PendingProposals int `json:"pending_proposals"`
		Events           int `json:"events"`
	} `json:"totals"`
	Daily []struct {
		Date    string `json:"date"`
		Allowed int    `json:"allowed"`
		Blocked int    `json:"blocked"`
	} `json:"daily"`
	TopBlocked []struct {
		FileHash   string `json:"file_hash"`
		BundleName string `json:"bundle_name"`
		FilePath   string `json:"file_path"`
		Count      int    `json:"count"`
	} `json:"top_blocked"`
	MachineActivity []struct {
		MachineID string `json:"machine_id"`
		Hostname  string `json:"hostname"`
		Events    int    `json:"events"`
		LastSync  string `json:"last_sync"`
	} `json:"machine_activity"`
	StaleMachines []struct {
		MachineID string `json:"machine_id"`
		Hostname  string `json:"hostname"`
		LastSync  string `json:"last_sync"`
	} `json:"stale_machines"`
	StaleAfterHours int `json:"stale_after_hours"`
}

func TestStatsOverviewEmptyDB(t *testing.T) {
	initTestDB(t)
	config.AppConfig = &config.Config{StaleMachineHours: 24}

	code, body := doGet(t, StatsOverview, "")
	if code != http.StatusOK {
		t.Fatalf("status=%d body=%s", code, body)
	}

	// Arrays must marshal as [] not null when empty.
	for _, want := range []string{
		`"daily":[]`, `"top_blocked":[]`, `"machine_activity":[]`, `"stale_machines":[]`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("empty body=%s, want %s", body, want)
		}
	}

	var resp statsOverviewResponse
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Totals.Machines != 0 || resp.Totals.Rules != 0 ||
		resp.Totals.PendingProposals != 0 || resp.Totals.Events != 0 {
		t.Fatalf("totals=%+v, want all zero", resp.Totals)
	}
	if resp.StaleAfterHours != 24 {
		t.Fatalf("stale_after_hours=%d, want 24", resp.StaleAfterHours)
	}
}

func TestStatsOverviewSeeded(t *testing.T) {
	initTestDB(t)
	config.AppConfig = &config.Config{StaleMachineHours: 24}

	// Machines: m1 synced just now (fresh), m2 never synced (stale/NULL).
	exec(t, `INSERT INTO machines (machine_id, hostname, last_sync)
		VALUES ('m1', 'fresh-host', datetime('now'))`)
	exec(t, `INSERT INTO machines (machine_id, hostname, last_sync)
		VALUES ('m2', 'stale-host', NULL)`)

	// Events across both machines, today and 2 days ago, allow + block.
	exec(t, `INSERT INTO events (machine_id, file_hash, bundle_name, file_path, decision, execution_time)
		VALUES ('m1', 'h1', 'good-app', '/bin/good', 'ALLOW_BINARY', datetime('now'))`)
	exec(t, `INSERT INTO events (machine_id, file_hash, bundle_name, file_path, decision, execution_time)
		VALUES ('m1', 'h2', 'bad-app', '/bin/bad', 'BLOCK_BINARY', datetime('now'))`)
	exec(t, `INSERT INTO events (machine_id, file_hash, bundle_name, file_path, decision, execution_time)
		VALUES ('m2', 'h2', 'bad-app', '/bin/bad', 'BLOCK_BINARY', datetime('now', '-2 days'))`)
	exec(t, `INSERT INTO events (machine_id, file_hash, bundle_name, file_path, decision, execution_time)
		VALUES ('m2', 'h2', 'bad-app', '/bin/bad', 'BLOCK_BINARY', datetime('now', '-2 days'))`)
	exec(t, `INSERT INTO events (machine_id, file_hash, bundle_name, file_path, decision, execution_time)
		VALUES ('m1', 'h3', 'other-app', '/bin/other', 'ALLOW_BINARY', datetime('now', '-2 days'))`)

	// A user, a pending and an approved proposal, and a rule.
	exec(t, `INSERT INTO users (id, username, role) VALUES (1, 'creator', 'USER')`)
	exec(t, `INSERT INTO proposals (identifier, rule_type, proposed_policy, created_by, status)
		VALUES ('pending.one', 'BINARY', 'ALLOWLIST', 1, 'PENDING')`)
	exec(t, `INSERT INTO proposals (identifier, rule_type, proposed_policy, created_by, status)
		VALUES ('approved.one', 'BINARY', 'ALLOWLIST', 1, 'APPROVED')`)
	exec(t, `INSERT INTO rules (identifier, policy, rule_type) VALUES ('good.app', 'ALLOWLIST', 'BINARY')`)

	code, body := doGet(t, StatsOverview, "")
	if code != http.StatusOK {
		t.Fatalf("status=%d body=%s", code, body)
	}

	var resp statsOverviewResponse
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}

	// Totals.
	if resp.Totals.Machines != 2 {
		t.Fatalf("totals.machines=%d, want 2", resp.Totals.Machines)
	}
	if resp.Totals.Rules != 1 {
		t.Fatalf("totals.rules=%d, want 1", resp.Totals.Rules)
	}
	if resp.Totals.PendingProposals != 1 {
		t.Fatalf("totals.pending_proposals=%d, want 1", resp.Totals.PendingProposals)
	}
	if resp.Totals.Events != 5 {
		t.Fatalf("totals.events=%d, want 5", resp.Totals.Events)
	}

	// Daily: two distinct days, with correct allow/block split.
	if len(resp.Daily) != 2 {
		t.Fatalf("daily=%+v, want 2 entries", resp.Daily)
	}
	var totalAllowed, totalBlocked int
	for _, d := range resp.Daily {
		totalAllowed += d.Allowed
		totalBlocked += d.Blocked
	}
	if totalAllowed != 2 || totalBlocked != 3 {
		t.Fatalf("daily totals allowed=%d blocked=%d, want 2/3 (daily=%+v)", totalAllowed, totalBlocked, resp.Daily)
	}

	// Top blocked: h2 blocked 3 times.
	if len(resp.TopBlocked) == 0 {
		t.Fatalf("top_blocked empty, want h2 entry")
	}
	found := false
	for _, tb := range resp.TopBlocked {
		if tb.FileHash == "h2" {
			found = true
			if tb.Count != 3 {
				t.Fatalf("top_blocked h2 count=%d, want 3", tb.Count)
			}
			if tb.BundleName != "bad-app" || tb.FilePath != "/bin/bad" {
				t.Fatalf("top_blocked h2=%+v, want bundle_name=bad-app file_path=/bin/bad", tb)
			}
		}
	}
	if !found {
		t.Fatalf("top_blocked=%+v, want entry for h2", resp.TopBlocked)
	}

	// Machine activity: ordered by events desc. m2 has 3 events, m1 has 3 events too (h1,h2,h3).
	// m1 events: h1, h2, h3 = 3. m2 events: h2, h2 = 2. So m1 should lead.
	if len(resp.MachineActivity) != 2 {
		t.Fatalf("machine_activity=%+v, want 2 entries", resp.MachineActivity)
	}
	if resp.MachineActivity[0].MachineID != "m1" || resp.MachineActivity[0].Events != 3 {
		t.Fatalf("machine_activity[0]=%+v, want m1 with 3 events", resp.MachineActivity[0])
	}
	if resp.MachineActivity[1].MachineID != "m2" || resp.MachineActivity[1].Events != 2 {
		t.Fatalf("machine_activity[1]=%+v, want m2 with 2 events", resp.MachineActivity[1])
	}
	if resp.MachineActivity[0].LastSync == "" {
		t.Fatalf("machine_activity[0] last_sync empty, want non-empty for m1")
	}

	// Stale machines: exactly m2 (NULL last_sync).
	if len(resp.StaleMachines) != 1 {
		t.Fatalf("stale_machines=%+v, want exactly 1 entry", resp.StaleMachines)
	}
	if resp.StaleMachines[0].MachineID != "m2" {
		t.Fatalf("stale_machines[0]=%+v, want m2", resp.StaleMachines[0])
	}
	if resp.StaleMachines[0].LastSync != "" {
		t.Fatalf("stale_machines[0].last_sync=%q, want empty string for NULL", resp.StaleMachines[0].LastSync)
	}

	if resp.StaleAfterHours != 24 {
		t.Fatalf("stale_after_hours=%d, want 24", resp.StaleAfterHours)
	}
}
