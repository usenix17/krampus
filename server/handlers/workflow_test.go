package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"krampus/server/database"
	"krampus/server/middleware"

	"github.com/gin-gonic/gin"
)

// withRole injects a fake authenticated identity into the gin context ahead
// of the real handler/middleware chain, mirroring what AuthMiddleware would
// have set from a validated token.
func withRole(userID int64, role string) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set("user_id", userID)
		c.Set("role", role)
		c.Set("username", "tester")
		c.Next()
	}
}

func TestRejectProposalHandler(t *testing.T) {
	initTestDB(t)
	exec(t, `INSERT INTO users (id, username, role) VALUES (1, 'creator', 'USER')`)
	exec(t, `INSERT INTO proposals (id, identifier, rule_type, proposed_policy, created_by)
		VALUES (1, 'ABCDEF', 'BINARY', 'ALLOWLIST', 1)`)

	router := gin.New()
	router.POST("/api/proposals/:id/reject",
		withRole(1, "ADMIN"), middleware.AdminMiddleware(), RejectProposal)

	req := httptest.NewRequest(http.MethodPost, "/api/proposals/1/reject", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s, want 200", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "Proposal rejected") {
		t.Fatalf("body=%s, want message about rejection", w.Body.String())
	}

	var status string
	if err := database.DB.QueryRow(`SELECT status FROM proposals WHERE id=1`).Scan(&status); err != nil {
		t.Fatalf("query status: %v", err)
	}
	if status != "REJECTED" {
		t.Fatalf("status=%q, want REJECTED", status)
	}

	// Rejecting again must fail (already finalized).
	req2 := httptest.NewRequest(http.MethodPost, "/api/proposals/1/reject", nil)
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, req2)
	if w2.Code != http.StatusBadRequest {
		t.Fatalf("second reject status=%d body=%s, want 400", w2.Code, w2.Body.String())
	}
}

func TestRejectProposalHandlerRequiresAdmin(t *testing.T) {
	initTestDB(t)
	exec(t, `INSERT INTO users (id, username, role) VALUES (1, 'creator', 'USER')`)
	exec(t, `INSERT INTO proposals (id, identifier, rule_type, proposed_policy, created_by)
		VALUES (1, 'ABCDEF', 'BINARY', 'ALLOWLIST', 1)`)

	router := gin.New()
	router.POST("/api/proposals/:id/reject",
		withRole(2, "USER"), middleware.AdminMiddleware(), RejectProposal)

	req := httptest.NewRequest(http.MethodPost, "/api/proposals/1/reject", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("status=%d body=%s, want 403", w.Code, w.Body.String())
	}
}

func TestListProposalVotesHandler(t *testing.T) {
	initTestDB(t)
	exec(t, `INSERT INTO users (id, username, role) VALUES (1, 'alice', 'USER'), (2, 'bob', 'USER')`)
	exec(t, `INSERT INTO proposals (id, identifier, rule_type, proposed_policy, created_by)
		VALUES (1, 'ABCDEF', 'BINARY', 'ALLOWLIST', 1)`)
	exec(t, `INSERT INTO votes (user_id, proposal_id, vote_type, created_at)
		VALUES (1, 1, 'ALLOWLIST', '2026-01-01 00:00:00')`)
	exec(t, `INSERT INTO votes (user_id, proposal_id, vote_type, created_at)
		VALUES (2, 1, 'BLOCKLIST', '2026-01-02 00:00:00')`)

	router := gin.New()
	router.GET("/api/proposals/:id/votes", ListProposalVotes)
	req := httptest.NewRequest(http.MethodGet, "/api/proposals/1/votes", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s, want 200", w.Code, w.Body.String())
	}

	var resp struct {
		Votes []struct {
			Username  string `json:"username"`
			VoteType  string `json:"vote_type"`
			CreatedAt string `json:"created_at"`
		} `json:"votes"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v body=%s", err, w.Body.String())
	}
	if len(resp.Votes) != 2 {
		t.Fatalf("votes=%d, want 2", len(resp.Votes))
	}
	if resp.Votes[0].Username != "alice" || resp.Votes[0].VoteType != "ALLOWLIST" {
		t.Fatalf("votes[0]=%+v, want alice/ALLOWLIST first (ordered by created_at)", resp.Votes[0])
	}
	if resp.Votes[1].Username != "bob" || resp.Votes[1].VoteType != "BLOCKLIST" {
		t.Fatalf("votes[1]=%+v, want bob/BLOCKLIST second", resp.Votes[1])
	}

	// Unknown proposal -> 404.
	req2 := httptest.NewRequest(http.MethodGet, "/api/proposals/999/votes", nil)
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, req2)
	if w2.Code != http.StatusNotFound {
		t.Fatalf("unknown proposal status=%d, want 404", w2.Code)
	}

	// Proposal with zero votes -> [] not null.
	exec(t, `INSERT INTO proposals (id, identifier, rule_type, proposed_policy, created_by)
		VALUES (2, 'GHIJKL', 'BINARY', 'ALLOWLIST', 1)`)
	req3 := httptest.NewRequest(http.MethodGet, "/api/proposals/2/votes", nil)
	w3 := httptest.NewRecorder()
	router.ServeHTTP(w3, req3)
	if w3.Code != http.StatusOK {
		t.Fatalf("status=%d, want 200", w3.Code)
	}
	if !strings.Contains(w3.Body.String(), `"votes":[]`) {
		t.Fatalf("body=%s, want votes:[] not null", w3.Body.String())
	}
}

func TestUpdateRuleHandler(t *testing.T) {
	initTestDB(t)
	exec(t, `INSERT INTO rules (id, identifier, policy, rule_type, custom_message, comment)
		VALUES (1, 'ABCDEF', 'ALLOWLIST', 'BINARY', 'old message', 'old comment')`)

	router := gin.New()
	router.PUT("/api/rules/:id", withRole(1, "ADMIN"), middleware.AdminMiddleware(), UpdateRule)

	// Change message only; comment must be preserved.
	req := httptest.NewRequest(http.MethodPut, "/api/rules/1",
		strings.NewReader(`{"custom_message":"new message"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s, want 200", w.Code, w.Body.String())
	}

	var msg, comment string
	if err := database.DB.QueryRow(`SELECT custom_message, comment FROM rules WHERE id=1`).Scan(&msg, &comment); err != nil {
		t.Fatalf("query: %v", err)
	}
	if msg != "new message" {
		t.Fatalf("custom_message=%q, want new message", msg)
	}
	if comment != "old comment" {
		t.Fatalf("comment=%q, want preserved old comment", comment)
	}

	// Explicit "" clears the message to an empty string (nil preserves,
	// "" clears) while the omitted comment is still preserved.
	reqClear := httptest.NewRequest(http.MethodPut, "/api/rules/1",
		strings.NewReader(`{"custom_message":""}`))
	reqClear.Header.Set("Content-Type", "application/json")
	wClear := httptest.NewRecorder()
	router.ServeHTTP(wClear, reqClear)
	if wClear.Code != http.StatusOK {
		t.Fatalf("clear status=%d body=%s, want 200", wClear.Code, wClear.Body.String())
	}
	if err := database.DB.QueryRow(`SELECT custom_message, comment FROM rules WHERE id=1`).Scan(&msg, &comment); err != nil {
		t.Fatalf("query after clear: %v", err)
	}
	if msg != "" {
		t.Fatalf("custom_message=%q, want empty string after explicit \"\"", msg)
	}
	if comment != "old comment" {
		t.Fatalf("comment=%q, want preserved old comment after clear", comment)
	}

	// Both nil -> 400.
	req2 := httptest.NewRequest(http.MethodPut, "/api/rules/1", strings.NewReader(`{}`))
	req2.Header.Set("Content-Type", "application/json")
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, req2)
	if w2.Code != http.StatusBadRequest {
		t.Fatalf("empty body status=%d body=%s, want 400", w2.Code, w2.Body.String())
	}

	// Unknown id -> 404.
	req3 := httptest.NewRequest(http.MethodPut, "/api/rules/999",
		strings.NewReader(`{"comment":"x"}`))
	req3.Header.Set("Content-Type", "application/json")
	w3 := httptest.NewRecorder()
	router.ServeHTTP(w3, req3)
	if w3.Code != http.StatusNotFound {
		t.Fatalf("unknown id status=%d body=%s, want 404", w3.Code, w3.Body.String())
	}
}

func TestListMachineEventsHandler(t *testing.T) {
	initTestDB(t)
	exec(t, `INSERT INTO machines (id, machine_id) VALUES (1, 'm1'), (2, 'm2')`)
	exec(t, `INSERT INTO events (machine_id, file_hash, decision, execution_time)
		VALUES ('m1', 'h1', 'ALLOW', '2026-01-02 10:00:00')`)
	exec(t, `INSERT INTO events (machine_id, file_hash, decision, execution_time)
		VALUES ('m1', 'h2', 'BLOCK', '2026-01-01 10:00:00')`)
	exec(t, `INSERT INTO events (machine_id, file_hash, decision, execution_time)
		VALUES ('m2', 'h3', 'ALLOW', '2026-01-01 10:00:00')`)

	router := gin.New()
	router.GET("/api/machines/:id/events", ListMachineEvents)

	req := httptest.NewRequest(http.MethodGet, "/api/machines/1/events", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s, want 200", w.Code, w.Body.String())
	}
	env := decodeEnvelope(t, w.Body.String())
	if env.Total != 2 {
		t.Fatalf("total=%d, want 2 (only machine 1's events)", env.Total)
	}
	var items []map[string]any
	if err := json.Unmarshal(env.Items, &items); err != nil {
		t.Fatalf("decode items: %v", err)
	}
	if len(items) != 2 || items[0]["file_hash"] != "h1" {
		t.Fatalf("items=%v, want h1 first (newest execution_time)", items)
	}

	// Unknown machine -> 404.
	req2 := httptest.NewRequest(http.MethodGet, "/api/machines/999/events", nil)
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, req2)
	if w2.Code != http.StatusNotFound {
		t.Fatalf("unknown machine status=%d, want 404", w2.Code)
	}
}
