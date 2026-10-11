package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"krampus/server/database"

	"github.com/gin-gonic/gin"
)

// listEnvelope mirrors the uniform paginated response shape. Items is kept
// raw so each test can decode it into the concrete model it expects.
type listEnvelope struct {
	Items   json.RawMessage `json:"items"`
	Total   int             `json:"total"`
	Page    int             `json:"page"`
	PerPage int             `json:"per_page"`
}

func initTestDB(t *testing.T) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	if err := database.Initialize(filepath.Join(t.TempDir(), "t.db")); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	t.Cleanup(func() { database.Close() })
}

func exec(t *testing.T, query string, args ...any) {
	t.Helper()
	if _, err := database.DB.Exec(query, args...); err != nil {
		t.Fatalf("exec %q: %v", query, err)
	}
}

// doGet runs handler against path and returns the status code and raw body.
func doGet(t *testing.T, handler gin.HandlerFunc, path string) (int, string) {
	t.Helper()
	router := gin.New()
	router.GET("/x", handler)
	req := httptest.NewRequest(http.MethodGet, "/x"+path, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w.Code, w.Body.String()
}

func decodeEnvelope(t *testing.T, body string) listEnvelope {
	t.Helper()
	var env listEnvelope
	if err := json.Unmarshal([]byte(body), &env); err != nil {
		t.Fatalf("decode envelope from %q: %v", body, err)
	}
	return env
}

func TestListRulesPaginationSearch(t *testing.T) {
	initTestDB(t)
	exec(t, `INSERT INTO rules (identifier, policy, rule_type, created_at)
		VALUES ('zebra.one', 'ALLOWLIST', 'BINARY', '2026-01-03 00:00:00')`)
	exec(t, `INSERT INTO rules (identifier, policy, rule_type, created_at)
		VALUES ('zebra.two', 'ALLOWLIST', 'BINARY', '2026-01-02 00:00:00')`)
	exec(t, `INSERT INTO rules (identifier, policy, rule_type, created_at)
		VALUES ('other.three', 'BLOCKLIST', 'BINARY', '2026-01-01 00:00:00')`)

	// (a) envelope shape.
	code, body := doGet(t, ListRules, "")
	if code != http.StatusOK {
		t.Fatalf("status=%d body=%s", code, body)
	}
	env := decodeEnvelope(t, body)
	if env.Total != 3 || env.Page != 1 || env.PerPage != 25 {
		t.Fatalf("envelope=%+v, want total=3 page=1 per_page=25", env)
	}

	// (b) search narrows total.
	_, body = doGet(t, ListRules, "?search=zebra&per_page=2")
	env = decodeEnvelope(t, body)
	if env.Total != 2 {
		t.Fatalf("search total=%d, want 2", env.Total)
	}

	// (c) pagination returns the second item.
	_, body = doGet(t, ListRules, "?per_page=1&page=2")
	env = decodeEnvelope(t, body)
	var rules []map[string]any
	if err := json.Unmarshal(env.Items, &rules); err != nil {
		t.Fatalf("decode items: %v", err)
	}
	if len(rules) != 1 || rules[0]["identifier"] != "zebra.two" {
		t.Fatalf("page2 items=%v, want single zebra.two", rules)
	}

	// (d) empty DB yields [] not null.
	exec(t, `DELETE FROM rules`)
	_, body = doGet(t, ListRules, "")
	if !strings.Contains(body, `"items":[]`) {
		t.Fatalf("empty body=%s, want \"items\":[]", body)
	}
}

func TestListProposalsPaginationSearch(t *testing.T) {
	initTestDB(t)
	exec(t, `INSERT INTO users (id, username, role) VALUES (1, 'creator', 'USER')`)
	exec(t, `INSERT INTO proposals (identifier, rule_type, proposed_policy, created_by, created_at)
		VALUES ('zebra.one', 'BINARY', 'ALLOWLIST', 1, '2026-01-03 00:00:00')`)
	exec(t, `INSERT INTO proposals (identifier, rule_type, proposed_policy, created_by, created_at)
		VALUES ('zebra.two', 'BINARY', 'ALLOWLIST', 1, '2026-01-02 00:00:00')`)
	exec(t, `INSERT INTO proposals (identifier, rule_type, proposed_policy, created_by, created_at)
		VALUES ('other.three', 'BINARY', 'BLOCKLIST', 1, '2026-01-01 00:00:00')`)

	code, body := doGet(t, ListProposals, "")
	if code != http.StatusOK {
		t.Fatalf("status=%d body=%s", code, body)
	}
	env := decodeEnvelope(t, body)
	if env.Total != 3 || env.Page != 1 || env.PerPage != 25 {
		t.Fatalf("envelope=%+v, want total=3 page=1 per_page=25", env)
	}
	// Creator enrichment must survive.
	if !strings.Contains(body, `"creator_username":"creator"`) {
		t.Fatalf("body=%s, want creator_username enrichment", body)
	}

	_, body = doGet(t, ListProposals, "?search=zebra&per_page=2")
	env = decodeEnvelope(t, body)
	if env.Total != 2 {
		t.Fatalf("search total=%d, want 2", env.Total)
	}

	_, body = doGet(t, ListProposals, "?per_page=1&page=2")
	env = decodeEnvelope(t, body)
	var items []map[string]any
	if err := json.Unmarshal(env.Items, &items); err != nil {
		t.Fatalf("decode items: %v", err)
	}
	if len(items) != 1 || items[0]["identifier"] != "zebra.two" {
		t.Fatalf("page2 items=%v, want single zebra.two", items)
	}

	exec(t, `DELETE FROM proposals`)
	_, body = doGet(t, ListProposals, "")
	if !strings.Contains(body, `"items":[]`) {
		t.Fatalf("empty body=%s, want \"items\":[]", body)
	}
}

func TestListUsersPaginationSearch(t *testing.T) {
	initTestDB(t)
	exec(t, `INSERT INTO users (username, role, created_at)
		VALUES ('zebra_a', 'USER', '2026-01-03 00:00:00')`)
	exec(t, `INSERT INTO users (username, role, created_at)
		VALUES ('zebra_b', 'USER', '2026-01-02 00:00:00')`)
	exec(t, `INSERT INTO users (username, role, created_at)
		VALUES ('other_c', 'ADMIN', '2026-01-01 00:00:00')`)

	code, body := doGet(t, ListUsers, "")
	if code != http.StatusOK {
		t.Fatalf("status=%d body=%s", code, body)
	}
	env := decodeEnvelope(t, body)
	if env.Total != 3 || env.Page != 1 || env.PerPage != 25 {
		t.Fatalf("envelope=%+v, want total=3 page=1 per_page=25", env)
	}

	_, body = doGet(t, ListUsers, "?search=zebra&per_page=2")
	env = decodeEnvelope(t, body)
	if env.Total != 2 {
		t.Fatalf("search total=%d, want 2", env.Total)
	}

	_, body = doGet(t, ListUsers, "?per_page=1&page=2")
	env = decodeEnvelope(t, body)
	var items []map[string]any
	if err := json.Unmarshal(env.Items, &items); err != nil {
		t.Fatalf("decode items: %v", err)
	}
	if len(items) != 1 || items[0]["username"] != "zebra_b" {
		t.Fatalf("page2 items=%v, want single zebra_b", items)
	}

	exec(t, `DELETE FROM users`)
	_, body = doGet(t, ListUsers, "")
	if !strings.Contains(body, `"items":[]`) {
		t.Fatalf("empty body=%s, want \"items\":[]", body)
	}
}

func seedEventsDB(t *testing.T) {
	t.Helper()
	exec(t, `INSERT INTO machines (machine_id) VALUES ('m1')`)
	exec(t, `INSERT INTO machines (machine_id) VALUES ('m2')`)
	exec(t, `INSERT INTO events (machine_id, file_hash, bundle_name, decision, execution_time)
		VALUES ('m1', 'h1', 'zebra-app', 'ALLOW', '2026-01-03 10:00:00')`)
	exec(t, `INSERT INTO events (machine_id, file_hash, bundle_name, decision, execution_time)
		VALUES ('m1', 'h2', 'zebra-tool', 'BLOCK', '2026-01-02 10:00:00')`)
	exec(t, `INSERT INTO events (machine_id, file_hash, bundle_name, decision, execution_time)
		VALUES ('m2', 'h3', 'other', 'ALLOW', '2026-01-01 10:00:00')`)
}

func TestListEventsPaginationSearch(t *testing.T) {
	initTestDB(t)
	seedEventsDB(t)

	code, body := doGet(t, ListEvents, "")
	if code != http.StatusOK {
		t.Fatalf("status=%d body=%s", code, body)
	}
	env := decodeEnvelope(t, body)
	if env.Total != 3 || env.Page != 1 || env.PerPage != 25 {
		t.Fatalf("envelope=%+v, want total=3 page=1 per_page=25", env)
	}

	_, body = doGet(t, ListEvents, "?search=zebra&per_page=2")
	env = decodeEnvelope(t, body)
	if env.Total != 2 {
		t.Fatalf("search total=%d, want 2", env.Total)
	}

	_, body = doGet(t, ListEvents, "?per_page=1&page=2")
	env = decodeEnvelope(t, body)
	var items []map[string]any
	if err := json.Unmarshal(env.Items, &items); err != nil {
		t.Fatalf("decode items: %v", err)
	}
	if len(items) != 1 || items[0]["file_hash"] != "h2" {
		t.Fatalf("page2 items=%v, want single h2", items)
	}

	// machine_id filter.
	_, body = doGet(t, ListEvents, "?machine_id=m2")
	env = decodeEnvelope(t, body)
	if env.Total != 1 {
		t.Fatalf("machine_id total=%d, want 1", env.Total)
	}
	if err := json.Unmarshal(env.Items, &items); err != nil {
		t.Fatalf("decode items: %v", err)
	}
	if len(items) != 1 || items[0]["file_hash"] != "h3" {
		t.Fatalf("machine_id items=%v, want single h3", items)
	}

	// decision filter.
	_, body = doGet(t, ListEvents, "?decision=BLOCK")
	env = decodeEnvelope(t, body)
	if env.Total != 1 {
		t.Fatalf("decision total=%d, want 1", env.Total)
	}

	// Malformed date bound is rejected rather than silently matching nothing.
	code, body = doGet(t, ListEvents, "?from=garbage")
	if code != http.StatusBadRequest {
		t.Fatalf("from=garbage status=%d body=%s, want 400", code, body)
	}
	if !strings.Contains(body, "invalid date") {
		t.Fatalf("from=garbage body=%s, want invalid date error", body)
	}

	// from/to inclusive date-range filter (only the 2026-01-02 event).
	_, body = doGet(t, ListEvents, "?from=2026-01-02&to=2026-01-02")
	env = decodeEnvelope(t, body)
	if env.Total != 1 {
		t.Fatalf("date-range total=%d, want 1", env.Total)
	}
	if err := json.Unmarshal(env.Items, &items); err != nil {
		t.Fatalf("decode items: %v", err)
	}
	if len(items) != 1 || items[0]["file_hash"] != "h2" {
		t.Fatalf("date-range items=%v, want single h2", items)
	}

	exec(t, `DELETE FROM events`)
	_, body = doGet(t, ListEvents, "")
	if !strings.Contains(body, `"items":[]`) {
		t.Fatalf("empty body=%s, want \"items\":[]", body)
	}
}

func TestListProgramsPaginationSearch(t *testing.T) {
	initTestDB(t)
	exec(t, `INSERT INTO machines (machine_id) VALUES ('m1')`)
	// h1 x3, h2 x2, h3 x1 so execution_count ordering is deterministic.
	// Only one h1 event has the 'needle' path, so a search for it must still
	// aggregate over all three h1 rows (search gates group membership only).
	paths := []string{"/bin/needle-app", "/bin/plain", "/bin/plain"}
	for i := 0; i < 3; i++ {
		exec(t, `INSERT INTO events (machine_id, file_hash, file_path, bundle_name, decision, execution_time)
			VALUES ('m1', 'h1', ?, 'zebra-app', 'ALLOW', '2026-01-03 10:00:00')`, paths[i])
	}
	for i := 0; i < 2; i++ {
		exec(t, `INSERT INTO events (machine_id, file_hash, bundle_name, decision, execution_time)
			VALUES ('m1', 'h2', 'zebra-tool', 'ALLOW', '2026-01-02 10:00:00')`)
	}
	exec(t, `INSERT INTO events (machine_id, file_hash, bundle_name, decision, execution_time)
		VALUES ('m1', 'h3', 'other', 'ALLOW', '2026-01-01 10:00:00')`)

	code, body := doGet(t, ListPrograms, "")
	if code != http.StatusOK {
		t.Fatalf("status=%d body=%s", code, body)
	}
	env := decodeEnvelope(t, body)
	if env.Total != 3 || env.Page != 1 || env.PerPage != 25 {
		t.Fatalf("envelope=%+v, want total=3 (distinct hashes) page=1 per_page=25", env)
	}

	_, body = doGet(t, ListPrograms, "?search=zebra&per_page=2")
	env = decodeEnvelope(t, body)
	if env.Total != 2 {
		t.Fatalf("search total=%d, want 2", env.Total)
	}

	_, body = doGet(t, ListPrograms, "?per_page=1&page=2")
	env = decodeEnvelope(t, body)
	var items []map[string]any
	if err := json.Unmarshal(env.Items, &items); err != nil {
		t.Fatalf("decode items: %v", err)
	}
	if len(items) != 1 || items[0]["file_hash"] != "h2" {
		t.Fatalf("page2 items=%v, want single h2", items)
	}

	// Search must gate which programs appear without truncating their
	// aggregates: only one h1 event matches 'needle', yet counts cover all 3.
	_, body = doGet(t, ListPrograms, "?search=needle")
	env = decodeEnvelope(t, body)
	if env.Total != 1 {
		t.Fatalf("needle total=%d, want 1", env.Total)
	}
	if err := json.Unmarshal(env.Items, &items); err != nil {
		t.Fatalf("decode items: %v", err)
	}
	if len(items) != 1 || items[0]["file_hash"] != "h1" {
		t.Fatalf("needle items=%v, want single h1", items)
	}
	if got := items[0]["execution_count"]; got != float64(3) {
		t.Fatalf("needle execution_count=%v, want 3 (full aggregate)", got)
	}

	exec(t, `DELETE FROM events`)
	_, body = doGet(t, ListPrograms, "")
	if !strings.Contains(body, `"items":[]`) {
		t.Fatalf("empty body=%s, want \"items\":[]", body)
	}
}

// Real Santa agents report decisions like ALLOW_BINARY/BLOCK_BINARY, so the
// decision filter must prefix-match while the UI keeps sending ALLOW/BLOCK.
func TestListEventsDecisionPrefix(t *testing.T) {
	initTestDB(t)
	exec(t, `INSERT INTO machines (machine_id) VALUES ('m1')`)
	exec(t, `INSERT INTO events (machine_id, file_hash, decision, execution_time)
		VALUES ('m1', 'ha', 'ALLOW_BINARY', '2026-01-03 10:00:00')`)
	exec(t, `INSERT INTO events (machine_id, file_hash, decision, execution_time)
		VALUES ('m1', 'hb', 'BLOCK_BINARY', '2026-01-02 10:00:00')`)

	code, body := doGet(t, ListEvents, "?decision=ALLOW")
	if code != http.StatusOK {
		t.Fatalf("status=%d body=%s", code, body)
	}
	env := decodeEnvelope(t, body)
	if env.Total != 1 {
		t.Fatalf("decision=ALLOW total=%d, want 1 (ALLOW_BINARY row)", env.Total)
	}
	var items []map[string]any
	if err := json.Unmarshal(env.Items, &items); err != nil {
		t.Fatalf("decode items: %v", err)
	}
	if len(items) != 1 || items[0]["file_hash"] != "ha" {
		t.Fatalf("decision=ALLOW items=%v, want single ha", items)
	}

	_, body = doGet(t, ListEvents, "?decision=BLOCK")
	env = decodeEnvelope(t, body)
	if env.Total != 1 {
		t.Fatalf("decision=BLOCK total=%d, want 1 (BLOCK_BINARY row)", env.Total)
	}

	// Anything but the two canonical values is rejected so the GLOB pattern
	// parameter stays attacker-proof.
	for _, junk := range []string{"EVIL*", "ALLOWANCE", "[A]LLOW"} {
		code, body = doGet(t, ListEvents, "?decision="+junk)
		if code != http.StatusBadRequest {
			t.Fatalf("decision=%s status=%d body=%s, want 400", junk, code, body)
		}
	}
}

// Program allow/block aggregates must also count prefixed decisions.
func TestListProgramsDecisionPrefixCounts(t *testing.T) {
	initTestDB(t)
	exec(t, `INSERT INTO machines (machine_id) VALUES ('m1')`)
	exec(t, `INSERT INTO events (machine_id, file_hash, decision, execution_time)
		VALUES ('m1', 'h1', 'ALLOW_BINARY', '2026-01-03 10:00:00')`)
	exec(t, `INSERT INTO events (machine_id, file_hash, decision, execution_time)
		VALUES ('m1', 'h1', 'BLOCK_BINARY', '2026-01-02 10:00:00')`)

	code, body := doGet(t, ListPrograms, "")
	if code != http.StatusOK {
		t.Fatalf("status=%d body=%s", code, body)
	}
	env := decodeEnvelope(t, body)
	var items []map[string]any
	if err := json.Unmarshal(env.Items, &items); err != nil {
		t.Fatalf("decode items: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("items=%v, want single h1 program", items)
	}
	if got := items[0]["allow_count"]; got != float64(1) {
		t.Fatalf("allow_count=%v, want 1 (ALLOW_BINARY counted)", got)
	}
	if got := items[0]["block_count"]; got != float64(1) {
		t.Fatalf("block_count=%v, want 1 (BLOCK_BINARY counted)", got)
	}
}
