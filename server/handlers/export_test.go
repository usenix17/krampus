package handlers

import (
	"encoding/csv"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// doGetRaw is like doGet but returns the recorder so headers can be checked.
func doGetRaw(t *testing.T, handler gin.HandlerFunc, path string) *httptest.ResponseRecorder {
	t.Helper()
	router := gin.New()
	router.GET("/x", handler)
	req := httptest.NewRequest(http.MethodGet, "/x"+path, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

func parseCSV(t *testing.T, body string) [][]string {
	t.Helper()
	records, err := csv.NewReader(strings.NewReader(body)).ReadAll()
	if err != nil {
		t.Fatalf("parse csv: %v\nbody=%s", err, body)
	}
	return records
}

func TestExportEvents(t *testing.T) {
	initTestDB(t)
	seedEventsDB(t)

	w := doGetRaw(t, ExportEvents, "")
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	if ct := w.Header().Get("Content-Type"); !strings.Contains(ct, "text/csv") {
		t.Fatalf("Content-Type=%q, want text/csv", ct)
	}
	if cd := w.Header().Get("Content-Disposition"); !strings.Contains(cd, `filename="events.csv"`) {
		t.Fatalf("Content-Disposition=%q, want events.csv filename", cd)
	}

	records := parseCSV(t, w.Body.String())
	wantHeader := []string{"id", "machine_id", "file_hash", "file_path", "decision",
		"executing_user", "cert_sha256", "cert_cn", "bundle_id", "bundle_name",
		"bundle_path", "signing_id", "team_id", "quarantine_data_url",
		"quarantine_timestamp", "execution_time"}
	if len(records) == 0 {
		t.Fatalf("no records returned")
	}
	if strings.Join(records[0], ",") != strings.Join(wantHeader, ",") {
		t.Fatalf("header=%v, want %v", records[0], wantHeader)
	}
	rows := records[1:]
	if len(rows) != 3 {
		t.Fatalf("rows=%d, want 3", len(rows))
	}

	// NULL-able column (bundle_name index 9) renders as empty string for the
	// seeded row that has no bundle_name... all seeded rows set bundle_name,
	// so check file_path instead, which seedEventsDB never sets.
	filePathIdx := 3
	foundEmpty := false
	for _, r := range rows {
		if r[filePathIdx] == "" {
			foundEmpty = true
		}
	}
	if !foundEmpty {
		t.Fatalf("expected at least one row with empty file_path, rows=%v", rows)
	}

	// search param narrows rows, reusing buildEventsFilter.
	w = doGetRaw(t, ExportEvents, "?search=zebra")
	records = parseCSV(t, w.Body.String())
	if len(records)-1 != 2 {
		t.Fatalf("search rows=%d, want 2", len(records)-1)
	}
}

func TestExportEventsFormulaInjectionNeutralized(t *testing.T) {
	initTestDB(t)
	exec(t, `INSERT INTO machines (machine_id) VALUES ('m1')`)
	exec(t, `INSERT INTO events (machine_id, file_hash, file_path, decision, execution_time)
		VALUES ('m1', 'h1', '=HYPERLINK("http://evil.example","click")', 'ALLOW', '2026-01-03 10:00:00')`)

	w := doGetRaw(t, ExportEvents, "")
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	records := parseCSV(t, w.Body.String())
	if len(records) != 2 {
		t.Fatalf("records=%d, want header + 1 row", len(records))
	}
	filePathIdx := 3
	got := records[1][filePathIdx]
	if !strings.HasPrefix(got, "'=") {
		t.Fatalf("file_path=%q, want leading ' to neutralize formula", got)
	}
}

func TestExportPrograms(t *testing.T) {
	initTestDB(t)
	exec(t, `INSERT INTO machines (machine_id) VALUES ('m1')`)
	exec(t, `INSERT INTO events (machine_id, file_hash, bundle_name, decision, execution_time)
		VALUES ('m1', 'h1', 'zebra-app', 'ALLOW', '2026-01-03 10:00:00')`)
	exec(t, `INSERT INTO events (machine_id, file_hash, bundle_name, decision, execution_time)
		VALUES ('m1', 'h2', 'zebra-tool', 'BLOCK', '2026-01-02 10:00:00')`)
	exec(t, `INSERT INTO events (machine_id, file_hash, bundle_name, decision, execution_time)
		VALUES ('m1', 'h3', 'other', 'ALLOW', '2026-01-01 10:00:00')`)

	w := doGetRaw(t, ExportPrograms, "")
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	if ct := w.Header().Get("Content-Type"); !strings.Contains(ct, "text/csv") {
		t.Fatalf("Content-Type=%q, want text/csv", ct)
	}
	if cd := w.Header().Get("Content-Disposition"); !strings.Contains(cd, `filename="programs.csv"`) {
		t.Fatalf("Content-Disposition=%q, want programs.csv filename", cd)
	}

	records := parseCSV(t, w.Body.String())
	wantHeader := []string{"file_hash", "file_path", "bundle_id", "bundle_name",
		"cert_cn", "cert_sha256", "signing_id", "team_id", "execution_count",
		"allow_count", "block_count", "last_seen"}
	if strings.Join(records[0], ",") != strings.Join(wantHeader, ",") {
		t.Fatalf("header=%v, want %v", records[0], wantHeader)
	}
	rows := records[1:]
	if len(rows) != 3 {
		t.Fatalf("rows=%d, want 3", len(rows))
	}
	filePathIdx := 1
	for _, r := range rows {
		if r[filePathIdx] != "" {
			t.Fatalf("file_path=%q, want empty (NULL column)", r[filePathIdx])
		}
	}
}

func TestExportRules(t *testing.T) {
	initTestDB(t)
	exec(t, `INSERT INTO rules (identifier, policy, rule_type, created_at)
		VALUES ('zebra.one', 'ALLOWLIST', 'BINARY', '2026-01-03 00:00:00')`)
	exec(t, `INSERT INTO rules (identifier, policy, rule_type, created_at)
		VALUES ('zebra.two', 'ALLOWLIST', 'BINARY', '2026-01-02 00:00:00')`)
	exec(t, `INSERT INTO rules (identifier, policy, rule_type, created_at)
		VALUES ('other.three', 'BLOCKLIST', 'BINARY', '2026-01-01 00:00:00')`)

	w := doGetRaw(t, ExportRules, "")
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	if ct := w.Header().Get("Content-Type"); !strings.Contains(ct, "text/csv") {
		t.Fatalf("Content-Type=%q, want text/csv", ct)
	}
	if cd := w.Header().Get("Content-Disposition"); !strings.Contains(cd, `filename="rules.csv"`) {
		t.Fatalf("Content-Disposition=%q, want rules.csv filename", cd)
	}

	records := parseCSV(t, w.Body.String())
	wantHeader := []string{"id", "identifier", "policy", "rule_type",
		"custom_message", "comment", "created_at"}
	if strings.Join(records[0], ",") != strings.Join(wantHeader, ",") {
		t.Fatalf("header=%v, want %v", records[0], wantHeader)
	}
	rows := records[1:]
	if len(rows) != 3 {
		t.Fatalf("rows=%d, want 3", len(rows))
	}
	customMessageIdx := 4
	for _, r := range rows {
		if r[customMessageIdx] != "" {
			t.Fatalf("custom_message=%q, want empty (NULL column)", r[customMessageIdx])
		}
	}
}
